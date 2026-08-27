//go:build integration

package integration_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/seeding"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevelopmentSeedsCreateIdempotentOrganizationMembershipMatrix(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	databaseURL := testsupport.RequiredEnv(t, "TEST_DATABASE_URL")
	testsupport.ApplyMigrations(t, "db/migrations/public", databaseURL)
	ctx := context.Background()
	seedDirectory := filepath.Join(testsupport.RepositoryRoot(), "db/seeds")

	require.NoError(t, seeding.Apply(ctx, pool, seedDirectory))
	require.NoError(t, seeding.Apply(ctx, pool, seedDirectory))

	var organizationCount, userCount, membershipCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM public.orgs`).Scan(&organizationCount))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM public.users`).Scan(&userCount))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM public.org_user`).Scan(&membershipCount))
	assert.Equal(t, 3, organizationCount)
	assert.Equal(t, 3, userCount)
	assert.Equal(t, 6, membershipCount)

	expectedMemberships := map[string][]string{
		"admin@aratuwa.edu":  {"University of Aratuwa", "MedSync", "Pied Piper"},
		"leader@aratuwa.edu": {"University of Aratuwa", "MedSync"},
		"member@aratuwa.edu": {"University of Aratuwa"},
	}
	passwordHashes := make(map[string]struct{}, len(expectedMemberships))
	hasher, err := identity.NewPasswordHasher(identity.HashConfig{})
	require.NoError(t, err)
	for email, organizations := range expectedMemberships {
		var passwordHash, normalization string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT password_hash, password_normalization FROM public.users WHERE email = $1 AND deleted_at IS NULL`, email).
			Scan(&passwordHash, &normalization))
		verification, err := hasher.Verify(ctx, "synodus-demo-password", passwordHash)
		require.NoError(t, err)
		assert.True(t, verification.Match, email)
		assert.Equal(t, "nfc_v1", normalization, email)
		passwordHashes[passwordHash] = struct{}{}

		rows, err := pool.Query(ctx, `
			SELECT organization.name
			FROM public.org_user AS membership
			JOIN public.users AS app_user ON app_user.id = membership.user_id
			JOIN public.orgs AS organization ON organization.id = membership.org_id
			WHERE app_user.email = $1
			ORDER BY organization.name`, email)
		require.NoError(t, err)
		var actual []string
		for rows.Next() {
			var organization string
			require.NoError(t, rows.Scan(&organization))
			actual = append(actual, organization)
		}
		rows.Close()
		require.NoError(t, rows.Err())
		assert.ElementsMatch(t, organizations, actual, email)
	}
	assert.Len(t, passwordHashes, len(expectedMemberships), "seed users must have distinct salts and hashes")
}
