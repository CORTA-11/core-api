//go:build integration

package integration_test

import (
	"context"
	"testing"

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
