//go:build isolation

package integration_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeTeamRLSDefaultsToDenyAndRechecksMembership(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	databaseURL := testsupport.RequiredEnv(t, "TEST_DATABASE_URL")
	testsupport.ApplyMigrations(t, "db/migrations/public", databaseURL)
	ctx := context.Background()

	source, err := tenancy.EmbeddedMigrations()
	require.NoError(t, err)
	organizationID := uuid.New()
	schema := tenancy.CanonicalSchema(organizationID.String())
	userPublicID := uuid.New()
	var organizationInternalID, userInternalID int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO public.orgs (
			public_id, name, schema_name, lifecycle_state, tenant_version,
			tenant_checksum, provisioned_at
		) VALUES ($1, 'RLS org', $2, 'active', $3, $4, now()) RETURNING id`,
		organizationID, schema, source.Version, source.Checksum).Scan(&organizationInternalID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO public.users (user_id, email, password_hash, display_name)
		VALUES ($1, 'rls@example.test', 'hash', 'RLS User') RETURNING id`, userPublicID).Scan(&userInternalID))
	_, err = pool.Exec(ctx, `INSERT INTO public.org_user (org_id, user_id) VALUES ($1, $2)`, organizationInternalID, userInternalID)
	require.NoError(t, err)
	testsupport.CreateSchema(t, pool, schema)
	testsupport.ApplyMigrations(t, "db/migrations/tenant", testsupport.DatabaseURLForSchema(t, schema))

	for _, table := range []string{"teams", "team_members", "tasks"} {
		var enabled, forced bool
		require.NoError(t, pool.QueryRow(ctx, `
			SELECT relrowsecurity, relforcerowsecurity
			FROM pg_class JOIN pg_namespace ON pg_namespace.oid = pg_class.relnamespace
			WHERE nspname = $1 AND relname = $2`, schema, table).Scan(&enabled, &forced))
		assert.True(t, enabled, table)
		assert.True(t, forced, table)
	}

	var firstTeamID, secondTeamID int64
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO `+pgx.Identifier{schema, "teams"}.Sanitize()+`
		(name, slug) VALUES ('First', 'first') RETURNING id`).Scan(&firstTeamID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO `+pgx.Identifier{schema, "teams"}.Sanitize()+`
		(name, slug) VALUES ('Second', 'second') RETURNING id`).Scan(&secondTeamID))
	_, err = pool.Exec(ctx, `INSERT INTO `+pgx.Identifier{schema, "team_members"}.Sanitize()+`
		(team_id, user_public_id, role) VALUES ($1, $2, 'viewer')`, firstTeamID, userPublicID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO `+pgx.Identifier{schema, "tasks"}.Sanitize()+`
		(team_id, description) VALUES ($1, 'visible'), ($2, 'hidden')`, firstTeamID, secondTeamID)
	require.NoError(t, err)

	queryAsRuntime := func(teamSetting string) ([]string, error) {
		tx, beginErr := pool.Begin(ctx)
		if beginErr != nil {
			return nil, beginErr
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		_, execErr := tx.Exec(ctx, `SET LOCAL ROLE synodus_runtime`)
		if execErr != nil {
			return nil, execErr
		}
		_, execErr = tx.Exec(ctx, `SELECT set_config('search_path', $1, true), set_config('app.user_id', $2, true), set_config('app.team_id', $3, true)`, schema, userPublicID.String(), teamSetting)
		if execErr != nil {
			return nil, execErr
		}
		tasks, queryErr := tenantdb.New(tx).GetTasks(ctx, 100)
		if queryErr != nil {
			return nil, queryErr
		}
		descriptions := make([]string, 0, len(tasks))
		for _, task := range tasks {
			descriptions = append(descriptions, task.Description)
		}
		return descriptions, nil
	}

	descriptions, err := queryAsRuntime("")
	require.NoError(t, err)
	assert.Empty(t, descriptions)
	descriptions, err = queryAsRuntime("invalid-team")
	assert.Error(t, err)
	assert.Empty(t, descriptions)
	descriptions, err = queryAsRuntime(strconv.FormatInt(firstTeamID, 10))
	require.NoError(t, err)
	assert.Equal(t, []string{"visible"}, descriptions)

	_, err = pool.Exec(ctx, `DELETE FROM `+pgx.Identifier{schema, "team_members"}.Sanitize()+` WHERE team_id = $1 AND user_public_id = $2`, firstTeamID, userPublicID)
	require.NoError(t, err)
	descriptions, err = queryAsRuntime(strconv.FormatInt(firstTeamID, 10))
	require.NoError(t, err)
	assert.Empty(t, descriptions)
}

func TestRuntimeCanCreateTeamAtomicallyButCannotMutateSecurityBoundary(t *testing.T) {
	pool, schema, userPublicID := rlsRuntimeFixture(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	_, err = tx.Exec(ctx, `SET LOCAL ROLE synodus_runtime`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `SELECT set_config('search_path', $1, true), set_config('app.user_id', $2, true), set_config('app.team_id', '', true)`, schema, userPublicID.String())
	require.NoError(t, err)
	var teamID int64
	require.NoError(t, tx.QueryRow(ctx, `SELECT id FROM create_team_with_creator('Created', 'created')`).Scan(&teamID))
	var role string
	require.NoError(t, tx.QueryRow(ctx, `SELECT role FROM team_members WHERE team_id = $1`, teamID).Scan(&role))
	assert.Equal(t, "team_admin", role)
	_, err = tx.Exec(ctx, `INSERT INTO team_members (team_id, user_public_id, role) VALUES ($1, $2, 'viewer')`, teamID, uuid.New())
	assert.Error(t, err)
	require.NoError(t, tx.Rollback(ctx))

	for _, statement := range []string{
		`SET ROLE synodus_owner`,
		`ALTER TABLE ` + pgx.Identifier{schema, "tasks"}.Sanitize() + ` DISABLE ROW LEVEL SECURITY`,
	} {
		testTx, beginErr := pool.Begin(ctx)
		require.NoError(t, beginErr)
		// SESSION AUTHORIZATION models a real runtime login. SET ROLE alone
		// retains the superuser session user's ability to assume any role.
		_, execErr := testTx.Exec(ctx, `SET LOCAL SESSION AUTHORIZATION synodus_runtime`)
		require.NoError(t, execErr)
		_, execErr = testTx.Exec(ctx, statement)
		assert.Error(t, execErr)
		_ = testTx.Rollback(ctx)
	}
}

func rlsRuntimeFixture(t *testing.T) (*pgxpool.Pool, string, uuid.UUID) {
	t.Helper()
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	databaseURL := testsupport.RequiredEnv(t, "TEST_DATABASE_URL")
	testsupport.ApplyMigrations(t, "db/migrations/public", databaseURL)
	ctx := context.Background()
	source, err := tenancy.EmbeddedMigrations()
	require.NoError(t, err)
	organizationID := uuid.New()
	schema := tenancy.CanonicalSchema(organizationID.String())
	userPublicID := uuid.New()
	var organizationInternalID, userInternalID int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO public.orgs (
			public_id, name, schema_name, lifecycle_state, tenant_version,
			tenant_checksum, provisioned_at
		) VALUES ($1, 'RLS creation org', $2, 'active', $3, $4, now()) RETURNING id`,
		organizationID, schema, source.Version, source.Checksum).Scan(&organizationInternalID))
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO public.users (user_id, email, password_hash, display_name)
		VALUES ($1, $2, 'hash', 'RLS Creation User') RETURNING id`,
		userPublicID, "rls-"+userPublicID.String()+"@example.test").Scan(&userInternalID))
	_, err = pool.Exec(ctx, `INSERT INTO public.org_user (org_id, user_id) VALUES ($1, $2)`, organizationInternalID, userInternalID)
	require.NoError(t, err)
	testsupport.CreateSchema(t, pool, schema)
	testsupport.ApplyMigrations(t, "db/migrations/tenant", testsupport.DatabaseURLForSchema(t, schema))
	return pool, schema, userPublicID
}
