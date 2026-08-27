//go:build integration

package integration_test

import (
	"context"
	"sync"
	"testing"

	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswordNormalizationMigrationPreservesLegacyCredentials(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	migrator := publicMigratorAt(t)
	require.NoError(t, migrator.Steps(6))

	ctx := context.Background()
	userID := uuid.New()
	const passwordHash = "unchanged-legacy-hash"
	_, err := pool.Exec(ctx, `
		INSERT INTO public.users (user_id, email, password_hash, display_name)
		VALUES ($1, 'legacy@example.test', $2, 'Legacy User')`, userID, passwordHash)
	require.NoError(t, err)
	require.NoError(t, migrator.Steps(1))

	var gotID uuid.UUID
	var gotHash, normalization string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT user_id, password_hash, password_normalization
		FROM public.users WHERE user_id = $1`, userID).Scan(&gotID, &gotHash, &normalization))
	assert.Equal(t, userID, gotID)
	assert.Equal(t, passwordHash, gotHash)
	assert.Equal(t, "legacy_raw", normalization)

	_, err = pool.Exec(ctx, `UPDATE public.users SET password_normalization = 'future_v2' WHERE user_id = $1`, userID)
	assert.Error(t, err)
}

func TestCredentialCompareAndSwapAllowsOneConcurrentUpgrade(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))

	ctx := context.Background()
	userID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO public.users (
			user_id, email, password_hash, display_name, password_normalization
		) VALUES ($1, 'cas@example.test', 'legacy-hash', 'CAS User', 'legacy_raw')`, userID)
	require.NoError(t, err)
	store := identity.NewPostgresCredentialStore(publicdb.New(pool))

	results := make(chan bool, 2)
	errorsSeen := make(chan error, 2)
	var wait sync.WaitGroup
	for _, newHash := range []string{"target-hash-one", "target-hash-two"} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			updated, updateErr := store.CompareAndSwapCredential(ctx, identity.CredentialCompareAndSwap{
				UserPublicID: userID, ExpectedHash: "legacy-hash",
				ExpectedNormalization: identity.PasswordNormalizationLegacyRaw,
				NewHash:               newHash, NewNormalization: identity.PasswordNormalizationNFCV1,
			})
			results <- updated
			errorsSeen <- updateErr
		}()
	}
	wait.Wait()
	close(results)
	close(errorsSeen)

	updatedCount := 0
	for updated := range results {
		if updated {
			updatedCount++
		}
	}
	for updateErr := range errorsSeen {
		require.NoError(t, updateErr)
	}
	assert.Equal(t, 1, updatedCount)

	current, err := store.CurrentCredentialByUserID(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, identity.PasswordNormalizationNFCV1, current.PasswordNormalization)
	assert.Contains(t, []string{"target-hash-one", "target-hash-two"}, current.PasswordHash)

	_, err = pool.Exec(ctx, `UPDATE public.users SET deleted_at = NOW() WHERE user_id = $1`, userID)
	require.NoError(t, err)
	updated, err := store.CompareAndSwapCredential(ctx, identity.CredentialCompareAndSwap{
		UserPublicID: userID, ExpectedHash: current.PasswordHash,
		ExpectedNormalization: identity.PasswordNormalizationNFCV1,
		NewHash:               "must-not-win", NewNormalization: identity.PasswordNormalizationNFCV1,
	})
	require.NoError(t, err)
	assert.False(t, updated)
}
