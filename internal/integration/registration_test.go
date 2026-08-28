//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistrationStoresNormalizedCredentialAndUsableSession(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	ctx := context.Background()
	hasher, err := identity.NewPasswordHasher(identity.HashConfig{})
	require.NoError(t, err)
	password, err := (identity.PasswordPolicy{}).Normalize("re\u0301search-password-value")
	require.NoError(t, err)
	hash, err := hasher.Hash(ctx, password)
	require.NoError(t, err)
	manager, err := session.NewManager(pool, bytes.Repeat([]byte{0x41}, 32))
	require.NoError(t, err)
	issued, err := manager.Register(ctx, "User@Example.COM", "Researcher", hash,
		identity.PasswordNormalizationNFCV1, "", "Browser")
	require.NoError(t, err)
	authentication, err := manager.Authenticate(ctx, issued.RawToken)
	require.NoError(t, err)
	assert.Equal(t, issued.Authentication.User.ID, authentication.User.ID)
	assert.NotEmpty(t, issued.CSRFToken)

	var storedHash, normalization string
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT password_hash, password_normalization FROM public.users WHERE email_canonical = $1",
		"user@example.com").Scan(&storedHash, &normalization))
	verification, err := hasher.Verify(ctx, password, storedHash)
	require.NoError(t, err)
	assert.True(t, verification.Match)
	assert.Equal(t, identity.PasswordNormalizationNFCV1, normalization)

	_, err = manager.Register(ctx, "user@example.com", "Duplicate", hash,
		identity.PasswordNormalizationNFCV1, "", "Browser")
	assert.ErrorIs(t, err, session.ErrEmailAlreadyExists)
	var count int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM public.users").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestRegistrationLeavesNoAccountWhenSessionInsertFails(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		CREATE FUNCTION public.reject_registration_session() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'forced session failure'; END $$;
		CREATE TRIGGER reject_registration_session BEFORE INSERT ON public.sessions
		FOR EACH ROW EXECUTE FUNCTION public.reject_registration_session()`)
	require.NoError(t, err)
	manager, err := session.NewManager(pool, bytes.Repeat([]byte{0x42}, 32))
	require.NoError(t, err)
	_, err = manager.Register(ctx, "rollback@example.com", "Rollback User", "argon-hash",
		identity.PasswordNormalizationNFCV1, "", "Browser")
	assert.ErrorIs(t, err, session.ErrSessionDependency)
	var count int
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT count(*) FROM public.users WHERE email_canonical = 'rollback@example.com'").Scan(&count))
	assert.Zero(t, count)
}
