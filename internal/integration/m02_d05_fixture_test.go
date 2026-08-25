//go:build isolation

package integration_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/CORTA-11/core-api/internal/dbroles"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

const d05RuntimePassword = "m02-d05-runtime-password"

type d05UserSet struct {
	shared   uuid.UUID
	alpha    uuid.UUID
	beta     uuid.UUID
	outsider uuid.UUID
}

type d05Team struct {
	id       int64
	publicID uuid.UUID
	slug     string
	taskID   uuid.UUID
}

type d05Organization struct {
	id       int64
	publicID uuid.UUID
	schema   string
	teams    [2]d05Team
}

type d05Fixture struct {
	adminPool   *pgxpool.Pool
	runtimePool *pgxpool.Pool
	source      tenancy.MigrationSet
	resolver    *tenancy.Resolver
	executor    *tenancy.Executor
	teamService service.TeamService
	taskService service.TaskService
	users       d05UserSet
	orgs        [2]d05Organization
}

func newD05Fixture(t *testing.T) *d05Fixture {
	t.Helper()
	ctx := context.Background()
	adminPool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, adminPool)
	databaseURL := testsupport.RequiredEnv(t, "TEST_DATABASE_URL")
	testsupport.ApplyMigrations(t, "db/migrations/public", databaseURL)
	require.NoError(t, dbroles.Configure(ctx, dbroles.Config{
		BootstrapDatabaseURL: databaseURL,
		RuntimePassword:      d05RuntimePassword,
		MigratorPassword:     "m02-d05-migrator-password",
		ProvisionerPassword:  "m02-d05-provisioner-password",
	}))

	source, err := tenancy.EmbeddedMigrations()
	require.NoError(t, err)
	fixture := &d05Fixture{
		adminPool: adminPool,
		source:    source,
		users: d05UserSet{
			shared:   uuid.MustParse("20000000-0000-4000-8000-000000000001"),
			alpha:    uuid.MustParse("20000000-0000-4000-8000-000000000002"),
			beta:     uuid.MustParse("20000000-0000-4000-8000-000000000003"),
			outsider: uuid.MustParse("20000000-0000-4000-8000-000000000004"),
		},
		orgs: [2]d05Organization{
			{publicID: uuid.MustParse("10000000-0000-4000-8000-000000000001")},
			{publicID: uuid.MustParse("10000000-0000-4000-8000-000000000002")},
		},
	}
	fixture.orgs[0].teams = [2]d05Team{
		{publicID: uuid.MustParse("30000000-0000-4000-8000-000000000001"), slug: "alpha-one", taskID: uuid.MustParse("40000000-0000-4000-8000-000000000001")},
		{publicID: uuid.MustParse("30000000-0000-4000-8000-000000000002"), slug: "alpha-two", taskID: uuid.MustParse("40000000-0000-4000-8000-000000000002")},
	}
	fixture.orgs[1].teams = [2]d05Team{
		{publicID: uuid.MustParse("30000000-0000-4000-8000-000000000003"), slug: "beta-one", taskID: uuid.MustParse("40000000-0000-4000-8000-000000000003")},
		{publicID: uuid.MustParse("30000000-0000-4000-8000-000000000004"), slug: "beta-two", taskID: uuid.MustParse("40000000-0000-4000-8000-000000000004")},
	}

	fixture.insertUsers(t)
	fixture.insertOrganizations(t)
	fixture.insertTenantData(t)
	fixture.runtimePool = openD05RuntimePool(t, databaseURL)
	fixture.resolver = tenancy.NewResolver(fixture.runtimePool, source)
	fixture.executor = tenancy.NewExecutor(fixture.runtimePool)
	fixture.teamService = service.NewTeamService(fixture.executor)
	fixture.taskService = service.NewTaskService(fixture.executor)
	return fixture
}

func (fixture *d05Fixture) insertUsers(t *testing.T) {
	t.Helper()
	users := []struct {
		publicID uuid.UUID
		email    string
		name     string
	}{
		{fixture.users.shared, "shared@m02-d05.example.test", "Shared User"},
		{fixture.users.alpha, "alpha@m02-d05.example.test", "Alpha User"},
		{fixture.users.beta, "beta@m02-d05.example.test", "Beta User"},
		{fixture.users.outsider, "outsider@m02-d05.example.test", "Outsider"},
	}
	for _, user := range users {
		_, err := fixture.adminPool.Exec(context.Background(), `
			INSERT INTO public.users (user_id, email, password_hash, display_name)
			VALUES ($1, $2, 'test-only-hash', $3)`, user.publicID, user.email, user.name)
		require.NoError(t, err)
	}
}

func (fixture *d05Fixture) insertOrganizations(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for index := range fixture.orgs {
		organization := &fixture.orgs[index]
		organization.schema = tenancy.CanonicalSchema(organization.publicID.String())
		require.NoError(t, fixture.adminPool.QueryRow(ctx, `
			INSERT INTO public.orgs (
				public_id, name, schema_name, lifecycle_state, tenant_version,
				tenant_checksum, provisioned_at
			) VALUES ($1, $2, $3, 'active', $4, $5, now())
			RETURNING id`, organization.publicID, fmt.Sprintf("D05 Organization %d", index+1),
			organization.schema, fixture.source.Version, fixture.source.Checksum).Scan(&organization.id))
		testsupport.CreateSchema(t, fixture.adminPool, organization.schema)
		testsupport.ApplyMigrations(t, "db/migrations/tenant", testsupport.DatabaseURLForSchema(t, organization.schema))
	}

	memberships := []struct {
		organization int
		user         uuid.UUID
	}{
		{0, fixture.users.shared},
		{0, fixture.users.alpha},
		{1, fixture.users.shared},
		{1, fixture.users.beta},
	}
	for _, membership := range memberships {
		_, err := fixture.adminPool.Exec(ctx, `
			INSERT INTO public.org_user (org_id, user_id)
			SELECT $1, id FROM public.users WHERE user_id = $2`,
			fixture.orgs[membership.organization].id, membership.user)
		require.NoError(t, err)
	}
}

func (fixture *d05Fixture) insertTenantData(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for organizationIndex := range fixture.orgs {
		organization := &fixture.orgs[organizationIndex]
		teamsTable := pgx.Identifier{organization.schema, "teams"}.Sanitize()
		membersTable := pgx.Identifier{organization.schema, "team_members"}.Sanitize()
		tasksTable := pgx.Identifier{organization.schema, "tasks"}.Sanitize()
		organizationUser := fixture.users.alpha
		if organizationIndex == 1 {
			organizationUser = fixture.users.beta
		}
		for teamIndex := range organization.teams {
			team := &organization.teams[teamIndex]
			require.NoError(t, fixture.adminPool.QueryRow(ctx, `INSERT INTO `+teamsTable+`
				(public_id, name, slug) VALUES ($1, $2, $3) RETURNING id`,
				team.publicID, fmt.Sprintf("D05 Team %d-%d", organizationIndex+1, teamIndex+1), team.slug).Scan(&team.id))
			for _, user := range []uuid.UUID{fixture.users.shared, organizationUser} {
				_, err := fixture.adminPool.Exec(ctx, `INSERT INTO `+membersTable+`
					(team_id, user_public_id, role) VALUES ($1, $2, 'viewer')`, team.id, user)
				require.NoError(t, err)
			}
			_, err := fixture.adminPool.Exec(ctx, `INSERT INTO `+tasksTable+`
				(public_id, team_id, description, status) VALUES ($1, $2, $3, 'todo')`,
				team.taskID, team.id, team.slug+" initial task")
			require.NoError(t, err)
		}
	}
}

func openD05RuntimePool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	config, err := pgxpool.ParseConfig(databaseURL)
	require.NoError(t, err)
	config.ConnConfig.User = "synodus_runtime"
	config.ConnConfig.Password = d05RuntimePassword
	config.MaxConns = 2
	runtimePool, err := pgxpool.NewWithConfig(context.Background(), config)
	require.NoError(t, err)
	t.Cleanup(runtimePool.Close)
	require.NoError(t, runtimePool.Ping(context.Background()))
	return runtimePool
}

func (fixture *d05Fixture) resolveOrganization(
	t *testing.T,
	user uuid.UUID,
	organization d05Organization,
) tenancy.OrganizationContext {
	t.Helper()
	resolved, err := fixture.resolver.ResolveOrganization(context.Background(), user, organization.publicID)
	require.NoError(t, err)
	return resolved
}

func (fixture *d05Fixture) resolveTeam(
	t *testing.T,
	user uuid.UUID,
	organization d05Organization,
	team d05Team,
) tenancy.TeamContext {
	t.Helper()
	resolvedOrganization := fixture.resolveOrganization(t, user, organization)
	resolvedTeam, err := fixture.resolver.ResolveTeam(context.Background(), resolvedOrganization, team.publicID)
	require.NoError(t, err)
	return resolvedTeam
}
