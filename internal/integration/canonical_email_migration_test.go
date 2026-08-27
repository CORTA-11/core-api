//go:build integration

package integration_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func publicMigratorAt(t *testing.T) *migrate.Migrate {
	t.Helper()
	migrator, err := migrate.New(
		"file://"+filepath.Join(testsupport.RepositoryRoot(), "db/migrations/public"),
		testsupport.RequiredEnv(t, "TEST_DATABASE_URL"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = migrator.Close() })
	return migrator
}

func TestCanonicalEmailMigrationUpgradesVersionFiveInPlace(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	migrator := publicMigratorAt(t)
	require.NoError(t, migrator.Steps(5))

	ctx := context.Background()
	userID := uuid.New()
	const passwordHash = "preserved-password-hash"
	_, err := pool.Exec(ctx, `
		INSERT INTO public.users (user_id, email, password_hash, display_name)
		VALUES ($1, $2, $3, 'Upgrade User')`, userID, " Jose\u0301@Example.COM ", passwordHash)
	require.NoError(t, err)
	require.NoError(t, migrator.Steps(1))

	var gotID uuid.UUID
	var email, canonical, hash string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT user_id, email, email_canonical, password_hash
		FROM public.users WHERE user_id = $1`, userID).Scan(&gotID, &email, &canonical, &hash))
	assert.Equal(t, userID, gotID)
	assert.Equal(t, "Jos\u00e9@Example.COM", email)
	assert.Equal(t, "jos\u00e9@example.com", canonical)
	assert.Equal(t, passwordHash, hash)
}

func TestCanonicalEmailMigrationRejectsCollisionsWithoutMutation(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	migrator := publicMigratorAt(t)
	require.NoError(t, migrator.Steps(5))

	ctx := context.Background()
	firstID, secondID := uuid.New(), uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO public.users (user_id, email, password_hash, display_name)
		VALUES ($1, $2, 'hash-one', 'One'),
		       ($3, $4, 'hash-two', 'Two')`, firstID, "Stra\u00dfe@example.test", secondID, "STRASSE@example.test")
	require.NoError(t, err)

	err = migrator.Steps(1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), firstID.String())
	assert.Contains(t, err.Error(), secondID.String())
	assert.NotContains(t, err.Error(), "example.test")

	var canonicalColumnCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'users'
		  AND column_name = 'email_canonical'`).Scan(&canonicalColumnCount))
	assert.Zero(t, canonicalColumnCount)
}

func TestCanonicalEmailDatabaseAndGoImplementationsAgree(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))

	canonicalizer := identity.EmailCanonicalizer{}
	for _, input := range []string{
		" Alice@Example.COM ",
		"Jose\u0301@Example.COM",
		"STRA\u1e9eE@example.com",
		"\u0130@example.com",
	} {
		goEmail, err := canonicalizer.Canonicalize(input)
		require.NoError(t, err)
		var display, key string
		require.NoError(t, pool.QueryRow(context.Background(), `
			SELECT public.canonical_email_display($1), public.canonical_email_key($1)`, input).Scan(&display, &key))
		assert.Equal(t, goEmail.Display, display, input)
		assert.Equal(t, goEmail.Key, key, input)
	}
}

func TestCanonicalEmailIdentityEnforcesGlobalUniquenessAndRefusesDown(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	migrator := publicMigratorAt(t)
	require.NoError(t, migrator.Up())

	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO public.users (email, password_hash, display_name)
		VALUES ($1, 'hash-one', 'One')`, "Stra\u00dfe@example.test")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO public.users (email, password_hash, display_name)
		VALUES ('STRASSE@example.test', 'hash-two', 'Two')`)
	assert.Error(t, err)

	err = migrator.Steps(-1)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "roll forward") || strings.Contains(err.Error(), "refusing"))

	var canonical string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT email_canonical FROM public.users WHERE email = $1`, "Stra\u00dfe@example.test").Scan(&canonical))
	assert.Equal(t, "strasse@example.test", canonical)
}
