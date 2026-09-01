//go:build integration

package integration_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamIdentityMigrationUpgradesV2WithoutGuessingTaskOwnership(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	databaseURL := testsupport.RequiredEnv(t, "TEST_DATABASE_URL")
	testsupport.ApplyMigrations(t, "db/migrations/public", databaseURL)
	ctx := context.Background()

	organizationID := uuid.New()
	schema := tenancy.CanonicalSchema(organizationID.String())
	var organizationInternalID, userInternalID int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO public.orgs (public_id, name, schema_name)
		VALUES ($1, 'Upgrade organization', $2) RETURNING id`, organizationID, schema).Scan(&organizationInternalID))
	userPublicID := uuid.New()
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO public.users (user_id, email, password_hash, display_name)
		VALUES ($1, 'upgrade@example.test', 'hash', 'Upgrade User') RETURNING id`, userPublicID).Scan(&userInternalID))
	_, err := pool.Exec(ctx, `INSERT INTO public.org_user (org_id, user_id) VALUES ($1, $2)`, organizationInternalID, userInternalID)
	require.NoError(t, err)
	testsupport.CreateSchema(t, pool, schema)

	migrator, err := migrate.New(
		"file://"+filepath.Join(testsupport.RepositoryRoot(), "db/migrations/tenant"),
		testsupport.DatabaseURLForSchema(t, schema),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = migrator.Close() })
	require.NoError(t, migrator.Steps(2))

	var teamID int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO `+schema+`.teams (name, slug) VALUES ('Existing team', 'existing-team') RETURNING id`).Scan(&teamID))
	var ownedTaskID, unownedTaskID int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO `+schema+`.tasks (team_id, description) VALUES ($1, 'owned') RETURNING id`, teamID).Scan(&ownedTaskID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO `+schema+`.tasks (description) VALUES ('ambiguous') RETURNING id`).Scan(&unownedTaskID))

	err = migrator.Steps(1)
	require.NoError(t, err)

	var teamPublicID, taskPublicID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `SELECT public_id FROM `+schema+`.teams WHERE id = $1`, teamID).Scan(&teamPublicID))
	require.NotEqual(t, uuid.Nil, teamPublicID)
	require.NoError(t, pool.QueryRow(ctx, `SELECT public_id FROM `+schema+`.tasks WHERE id = $1`, ownedTaskID).Scan(&taskPublicID))
	require.NotEqual(t, uuid.Nil, taskPublicID)

	var preservedTeamID, quarantinedTeamID int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT team_id FROM `+schema+`.tasks WHERE id = $1`, ownedTaskID).Scan(&preservedTeamID))
	require.NoError(t, pool.QueryRow(ctx, `SELECT team_id FROM `+schema+`.tasks WHERE id = $1`, unownedTaskID).Scan(&quarantinedTeamID))
	assert.Equal(t, teamID, preservedTeamID)
	assert.NotEqual(t, teamID, quarantinedTeamID)

	var quarantine bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT is_quarantine FROM `+schema+`.teams WHERE id = $1`, quarantinedTeamID).Scan(&quarantine))
	assert.True(t, quarantine)
	var role string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT role FROM `+schema+`.team_members WHERE team_id = $1 AND user_public_id = $2`, teamID, userPublicID).Scan(&role))
	assert.Equal(t, "viewer", role)

	_, err = pool.Exec(ctx, `INSERT INTO `+schema+`.team_members (team_id, user_public_id, role) VALUES ($1, $2, 'owner')`, teamID, userPublicID)
	assert.Error(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO `+schema+`.team_members (team_id, user_public_id, role) VALUES ($1, $2, 'viewer')`, teamID, userPublicID)
	assert.Error(t, err)

	var nullable string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT is_nullable FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'tasks' AND column_name = 'team_id'`, schema).Scan(&nullable))
	assert.Equal(t, "NO", nullable)
}

func TestTeamIdentityMigrationDownFailsSafely(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	databaseURL := testsupport.RequiredEnv(t, "TEST_DATABASE_URL")
	testsupport.ApplyMigrations(t, "db/migrations/public", databaseURL)
	schema := "team_identity_down_test"
	testsupport.CreateSchema(t, pool, schema)
	migrator, err := migrate.New(
		"file://"+filepath.Join(testsupport.RepositoryRoot(), "db/migrations/tenant"),
		testsupport.DatabaseURLForSchema(t, schema),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = migrator.Close() })
	require.NoError(t, migrator.Steps(3))
	// Version 3 introduces durable team identities, membership, and quarantine
	// state. Its down migration must remain a forward-only safety barrier even as
	// newer tenant migrations are appended.
	err = migrator.Steps(-1)
	assert.Error(t, err)
	assert.False(t, errors.Is(err, migrate.ErrNoChange))
}
