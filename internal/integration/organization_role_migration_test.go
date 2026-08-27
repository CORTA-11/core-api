//go:build integration

package integration_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationRoleMigrationDeduplicatesWithoutGuessingOwner(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	databaseURL := testsupport.RequiredEnv(t, "TEST_DATABASE_URL")
	migrator, err := migrate.New("file://"+filepath.Join(testsupport.RepositoryRoot(), "db/migrations/public"), databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = migrator.Close() })
	require.NoError(t, migrator.Migrate(8))

	ctx := context.Background()
	organizationID := uuid.New()
	userID := uuid.New()
	var organizationInternalID, userInternalID int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO public.orgs (public_id, name, schema_name)
		VALUES ($1, 'legacy organization', $2) RETURNING id`,
		organizationID, tenancy.CanonicalSchema(organizationID.String())).Scan(&organizationInternalID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO public.users (user_id, email, password_hash, display_name)
		VALUES ($1, 'legacy@example.com', 'hash', 'Legacy User') RETURNING id`,
		userID).Scan(&userInternalID))
	_, err = pool.Exec(ctx, `INSERT INTO public.org_user (org_id, user_id) VALUES ($1, $2), ($1, $2)`,
		organizationInternalID, userInternalID)
	require.NoError(t, err)

	require.NoError(t, migrator.Migrate(9))
	var count int
	var role string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*), min(role) FROM public.org_user WHERE org_id = $1 AND user_id = $2`,
		organizationInternalID, userInternalID).Scan(&count, &role))
	assert.Equal(t, 1, count)
	assert.Equal(t, "member", role)
	var owners int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM public.org_user WHERE role = 'owner'`).Scan(&owners))
	assert.Zero(t, owners)

	_, err = pool.Exec(ctx, `INSERT INTO public.org_user (org_id, user_id) VALUES ($1, $2)`, organizationInternalID, userInternalID)
	assert.Error(t, err)
	_, err = pool.Exec(ctx, `UPDATE public.org_user SET role = 'unknown' WHERE org_id = $1`, organizationInternalID)
	assert.Error(t, err)
}
