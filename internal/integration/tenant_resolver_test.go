//go:build integration || isolation

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type resolverFixture struct {
	pool           *pgxpool.Pool
	resolver       *tenancy.Resolver
	userPublicID   uuid.UUID
	organizationID uuid.UUID
	schemaName     string
}

func newResolverFixture(t *testing.T) resolverFixture {
	t.Helper()
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	source, err := tenancy.EmbeddedMigrations()
	require.NoError(t, err)

	ctx := context.Background()
	var userID int64
	var userPublicID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO public.users (email, password_hash, display_name)
		VALUES ('resolver@example.test', 'hash', 'Resolver')
		RETURNING id, user_id`).Scan(&userID, &userPublicID))

	organizationID := uuid.New()
	schemaName := tenancy.CanonicalSchema(organizationID.String())
	var internalOrganizationID int64
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO public.orgs (
			public_id, name, schema_name, lifecycle_state, tenant_version,
			tenant_checksum, provisioned_at
		) VALUES ($1, 'Resolver Org', $2, 'active', $3, $4, now())
		RETURNING id`, organizationID, schemaName, source.Version, source.Checksum).Scan(&internalOrganizationID))
	require.NoError(t, func() error {
		testsupport.CreateSchema(t, pool, schemaName)
		_, err := pool.Exec(ctx, `INSERT INTO public.org_user (org_id, user_id) VALUES ($1, $2)`, internalOrganizationID, userID)
		return err
	}())

	return resolverFixture{
		pool:           pool,
		resolver:       tenancy.NewResolver(pool, source),
		userPublicID:   userPublicID,
		organizationID: organizationID,
		schemaName:     schemaName,
	}
}

func TestResolveOrganizationRequiresTrustedCurrentMembership(t *testing.T) {
	fixture := newResolverFixture(t)
	ctx := context.Background()

	organization, err := fixture.resolver.ResolveOrganization(ctx, fixture.userPublicID, fixture.organizationID)
	require.NoError(t, err)
	require.NotEqual(t, tenancy.OrganizationContext{}, organization)

	t.Run("nonmember", func(t *testing.T) {
		_, err := fixture.resolver.ResolveOrganization(ctx, uuid.New(), fixture.organizationID)
		require.ErrorIs(t, err, tenancy.ErrOrganizationUnavailable)
	})

	t.Run("deleted user", func(t *testing.T) {
		_, err := fixture.pool.Exec(ctx, `UPDATE public.users SET deleted_at = now() WHERE user_id = $1`, fixture.userPublicID)
		require.NoError(t, err)
		_, err = fixture.resolver.ResolveOrganization(ctx, fixture.userPublicID, fixture.organizationID)
		require.ErrorIs(t, err, tenancy.ErrOrganizationUnavailable)
		_, err = fixture.pool.Exec(ctx, `UPDATE public.users SET deleted_at = NULL WHERE user_id = $1`, fixture.userPublicID)
		require.NoError(t, err)
	})

	t.Run("inactive", func(t *testing.T) {
		_, err := fixture.pool.Exec(ctx, `UPDATE public.orgs SET lifecycle_state = 'provisioning' WHERE public_id = $1`, fixture.organizationID)
		require.NoError(t, err)
		_, err = fixture.resolver.ResolveOrganization(ctx, fixture.userPublicID, fixture.organizationID)
		require.ErrorIs(t, err, tenancy.ErrOrganizationUnavailable)
		_, err = fixture.pool.Exec(ctx, `UPDATE public.orgs SET lifecycle_state = 'active' WHERE public_id = $1`, fixture.organizationID)
		require.NoError(t, err)
	})

	t.Run("stale version", func(t *testing.T) {
		_, err := fixture.pool.Exec(ctx, `UPDATE public.orgs SET tenant_version = tenant_version - 1 WHERE public_id = $1`, fixture.organizationID)
		require.NoError(t, err)
		_, err = fixture.resolver.ResolveOrganization(ctx, fixture.userPublicID, fixture.organizationID)
		require.ErrorIs(t, err, tenancy.ErrOrganizationUnavailable)
		_, err = fixture.pool.Exec(ctx, `UPDATE public.orgs SET tenant_version = tenant_version + 1 WHERE public_id = $1`, fixture.organizationID)
		require.NoError(t, err)
	})

	t.Run("stale checksum", func(t *testing.T) {
		_, err := fixture.pool.Exec(ctx, `UPDATE public.orgs SET tenant_checksum = repeat('0', 64) WHERE public_id = $1`, fixture.organizationID)
		require.NoError(t, err)
		_, err = fixture.resolver.ResolveOrganization(ctx, fixture.userPublicID, fixture.organizationID)
		require.ErrorIs(t, err, tenancy.ErrOrganizationUnavailable)
	})
}

func TestResolveOrganizationRejectsMissingAndNoncanonicalSchemas(t *testing.T) {
	fixture := newResolverFixture(t)
	ctx := context.Background()

	_, err := fixture.pool.Exec(ctx, `DROP SCHEMA `+`"`+fixture.schemaName+`"`)
	require.NoError(t, err)
	_, err = fixture.resolver.ResolveOrganization(ctx, fixture.userPublicID, fixture.organizationID)
	require.ErrorIs(t, err, tenancy.ErrOrganizationUnavailable)

	_, err = fixture.pool.Exec(ctx, `ALTER TABLE public.orgs DROP CONSTRAINT orgs_canonical_schema_check`)
	require.NoError(t, err)
	_, err = fixture.pool.Exec(ctx, `UPDATE public.orgs SET schema_name = 'public' WHERE public_id = $1`, fixture.organizationID)
	require.NoError(t, err)
	_, err = fixture.resolver.ResolveOrganization(ctx, fixture.userPublicID, fixture.organizationID)
	require.True(t, errors.Is(err, tenancy.ErrRegistryIntegrity))
}

func TestWithinOrganizationInstallsScopeAndCommits(t *testing.T) {
	fixture := newResolverFixture(t)
	applyTenantFixture(t, fixture)
	organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
	require.NoError(t, err)
	pool := openSingleConnectionPool(t)
	executor := tenancy.NewExecutor(pool)

	err = executor.WithinOrganization(context.Background(), organization, func(queries *tenantdb.Queries) error {
		_, err := queries.CreateTeam(context.Background(), tenantdb.CreateTeamParams{Name: "Committed", Slug: "committed"})
		return err
	})
	require.NoError(t, err)

	var count int
	require.NoError(t, fixture.pool.QueryRow(context.Background(), `SELECT count(*) FROM `+pgx.Identifier{fixture.schemaName, "teams"}.Sanitize()+` WHERE slug = 'committed'`).Scan(&count))
	assert.Equal(t, 1, count)
	assertConnectionHasDefaultScope(t, pool)
}

func TestWithinOrganizationRollsBackCallbackError(t *testing.T) {
	fixture := newResolverFixture(t)
	applyTenantFixture(t, fixture)
	organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
	require.NoError(t, err)
	pool := openSingleConnectionPool(t)
	executor := tenancy.NewExecutor(pool)
	wantErr := errors.New("callback failed")

	err = executor.WithinOrganization(context.Background(), organization, func(queries *tenantdb.Queries) error {
		_, createErr := queries.CreateTeam(context.Background(), tenantdb.CreateTeamParams{Name: "Rolled back", Slug: "rolled-back"})
		require.NoError(t, createErr)
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)

	var count int
	require.NoError(t, fixture.pool.QueryRow(context.Background(), `SELECT count(*) FROM `+pgx.Identifier{fixture.schemaName, "teams"}.Sanitize()+` WHERE slug = 'rolled-back'`).Scan(&count))
	assert.Zero(t, count)
	assertConnectionHasDefaultScope(t, pool)
}

func TestWithinOrganizationRejectsInvalidInputBeforeAcquire(t *testing.T) {
	pool := openSingleConnectionPool(t)
	executor := tenancy.NewExecutor(pool)
	before := pool.Stat().AcquireCount()

	err := executor.WithinOrganization(context.Background(), tenancy.OrganizationContext{}, func(*tenantdb.Queries) error { return nil })
	require.ErrorIs(t, err, tenancy.ErrInvalidContext)
	assert.Equal(t, before, pool.Stat().AcquireCount())

	fixture := newResolverFixture(t)
	organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
	require.NoError(t, err)
	err = executor.WithinOrganization(context.Background(), organization, nil)
	require.ErrorIs(t, err, tenancy.ErrInvalidCallback)
	assert.Equal(t, before, pool.Stat().AcquireCount())
}

func TestWithinOrganizationPreservesPanicAndRollsBack(t *testing.T) {
	fixture := newResolverFixture(t)
	applyTenantFixture(t, fixture)
	organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
	require.NoError(t, err)
	pool := openSingleConnectionPool(t)
	executor := tenancy.NewExecutor(pool)
	panicValue := &struct{ reason string }{reason: "panic value"}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = executor.WithinOrganization(context.Background(), organization, func(queries *tenantdb.Queries) error {
			_, createErr := queries.CreateTeam(context.Background(), tenantdb.CreateTeamParams{Name: "Panicked", Slug: "panicked"})
			require.NoError(t, createErr)
			panic(panicValue)
		})
	}()
	assert.Same(t, panicValue, recovered)
	assertTeamAbsent(t, fixture, "panicked")
	assertConnectionHasDefaultScope(t, pool)
}

func TestWithinOrganizationCancellationRollsBackWithDetachedContext(t *testing.T) {
	fixture := newResolverFixture(t)
	applyTenantFixture(t, fixture)
	organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
	require.NoError(t, err)
	pool := openSingleConnectionPool(t)
	executor := tenancy.NewExecutor(pool)
	ctx, cancel := context.WithCancel(context.Background())

	err = executor.WithinOrganization(ctx, organization, func(queries *tenantdb.Queries) error {
		_, createErr := queries.CreateTeam(ctx, tenantdb.CreateTeamParams{Name: "Canceled", Slug: "canceled"})
		require.NoError(t, createErr)
		cancel()
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	assertTeamAbsent(t, fixture, "canceled")
	assertConnectionHasDefaultScope(t, pool)
}

func TestWithinOrganizationRejectsSchemaRemovalBeforeCallback(t *testing.T) {
	fixture := newResolverFixture(t)
	organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
	require.NoError(t, err)
	_, err = fixture.pool.Exec(context.Background(), `DROP SCHEMA `+pgx.Identifier{fixture.schemaName}.Sanitize())
	require.NoError(t, err)
	pool := openSingleConnectionPool(t)
	executor := tenancy.NewExecutor(pool)
	called := false

	err = executor.WithinOrganization(context.Background(), organization, func(*tenantdb.Queries) error {
		called = true
		return nil
	})
	require.ErrorIs(t, err, tenancy.ErrOrganizationUnavailable)
	assert.False(t, called)
	assertConnectionHasDefaultScope(t, pool)
}

func TestWithinOrganizationCleansUpDeferredCommitFailure(t *testing.T) {
	fixture := newResolverFixture(t)
	applyTenantFixture(t, fixture)
	organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
	require.NoError(t, err)
	installDeferredCommitFailure(t, fixture)
	pool := openSingleConnectionPool(t)
	executor := tenancy.NewExecutor(pool)

	err = executor.WithinOrganization(context.Background(), organization, func(queries *tenantdb.Queries) error {
		_, createErr := queries.CreateTeam(context.Background(), tenantdb.CreateTeamParams{Name: "Commit failed", Slug: "commit-failed"})
		return createErr
	})
	require.Error(t, err)
	assertTeamAbsent(t, fixture, "commit-failed")
	assertConnectionHasDefaultScope(t, pool)
}

func TestWithinOrganizationDiscardsConnectionAfterRollbackFailure(t *testing.T) {
	fixture := newResolverFixture(t)
	applyTenantFixture(t, fixture)
	organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
	require.NoError(t, err)
	installBackendTerminationTrigger(t, fixture)
	pool := openSingleConnectionPool(t)
	executor := tenancy.NewExecutor(pool)
	connectionsBefore := pool.Stat().NewConnsCount()

	err = executor.WithinOrganization(context.Background(), organization, func(queries *tenantdb.Queries) error {
		_, createErr := queries.CreateTeam(context.Background(), tenantdb.CreateTeamParams{Name: "Terminated", Slug: "terminated"})
		return createErr
	})
	require.Error(t, err)
	assertConnectionHasDefaultScope(t, pool)
	assert.Greater(t, pool.Stat().NewConnsCount(), connectionsBefore)

	removeBackendTerminationTrigger(t, fixture)
	err = executor.WithinOrganization(context.Background(), organization, func(queries *tenantdb.Queries) error {
		_, createErr := queries.CreateTeam(context.Background(), tenantdb.CreateTeamParams{Name: "After termination", Slug: "after-termination"})
		return createErr
	})
	require.NoError(t, err)
	assertConnectionHasDefaultScope(t, pool)
}

func TestWithinOrganizationRetainedQueriesFailAfterCompletion(t *testing.T) {
	fixture := newResolverFixture(t)
	applyTenantFixture(t, fixture)
	organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
	require.NoError(t, err)
	pool := openSingleConnectionPool(t)
	executor := tenancy.NewExecutor(pool)

	var afterCommit *tenantdb.Queries
	require.NoError(t, executor.WithinOrganization(context.Background(), organization, func(queries *tenantdb.Queries) error {
		afterCommit = queries
		return nil
	}))
	_, err = afterCommit.GetTeams(context.Background(), 1)
	require.ErrorIs(t, err, pgx.ErrTxClosed)

	wantErr := errors.New("rollback retained queries")
	var afterRollback *tenantdb.Queries
	err = executor.WithinOrganization(context.Background(), organization, func(queries *tenantdb.Queries) error {
		afterRollback = queries
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)
	_, err = afterRollback.GetTeams(context.Background(), 1)
	require.ErrorIs(t, err, pgx.ErrTxClosed)
	assertConnectionHasDefaultScope(t, pool)
}

func TestResolveTeamRevalidatesOrganizationAndUsesParameterizedSlug(t *testing.T) {
	t.Run("valid and unknown team", func(t *testing.T) {
		fixture := newResolverFixture(t)
		applyTenantFixture(t, fixture)
		organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
		require.NoError(t, err)
		executor := tenancy.NewExecutor(fixture.pool)
		require.NoError(t, executor.WithinOrganization(context.Background(), organization, func(queries *tenantdb.Queries) error {
			_, createErr := queries.CreateTeam(context.Background(), tenantdb.CreateTeamParams{Name: "Research", Slug: "research"})
			return createErr
		}))

		team, err := fixture.resolver.ResolveTeam(context.Background(), organization, "research")
		require.NoError(t, err)
		require.NotEqual(t, tenancy.TeamContext{}, team)

		_, err = fixture.resolver.ResolveTeam(context.Background(), organization, "missing")
		require.ErrorIs(t, err, tenancy.ErrTeamUnavailable)
		_, err = fixture.resolver.ResolveTeam(context.Background(), organization, `research' OR true --`)
		require.ErrorIs(t, err, tenancy.ErrTeamUnavailable)
	})

	t.Run("membership revoked after organization resolution", func(t *testing.T) {
		fixture := newResolverFixture(t)
		applyTenantFixture(t, fixture)
		organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
		require.NoError(t, err)
		_, err = fixture.pool.Exec(context.Background(), `DELETE FROM public.org_user`)
		require.NoError(t, err)

		_, err = fixture.resolver.ResolveTeam(context.Background(), organization, "research")
		require.ErrorIs(t, err, tenancy.ErrOrganizationUnavailable)
	})

	t.Run("lifecycle changed after organization resolution", func(t *testing.T) {
		fixture := newResolverFixture(t)
		applyTenantFixture(t, fixture)
		organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
		require.NoError(t, err)
		_, err = fixture.pool.Exec(context.Background(), `UPDATE public.orgs SET lifecycle_state = 'provisioning'`)
		require.NoError(t, err)

		_, err = fixture.resolver.ResolveTeam(context.Background(), organization, "research")
		require.ErrorIs(t, err, tenancy.ErrOrganizationUnavailable)
	})
}

func TestResolveTeamRejectsInvalidInputBeforeDatabaseWork(t *testing.T) {
	fixture := newResolverFixture(t)
	organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
	require.NoError(t, err)
	before := fixture.pool.Stat().AcquireCount()

	_, err = fixture.resolver.ResolveTeam(context.Background(), organization, "")
	require.ErrorIs(t, err, tenancy.ErrTeamUnavailable)
	assert.Equal(t, before, fixture.pool.Stat().AcquireCount())
	_, err = fixture.resolver.ResolveTeam(context.Background(), tenancy.OrganizationContext{}, "research")
	require.ErrorIs(t, err, tenancy.ErrInvalidContext)
	assert.Equal(t, before, fixture.pool.Stat().AcquireCount())
}

func TestWithinTeamInstallsExactScopeAndCommits(t *testing.T) {
	fixture, team, teamID := resolvedTeamFixture(t)
	installTeamScopeTrigger(t, fixture)
	pool := openSingleConnectionPool(t)
	executor := tenancy.NewExecutor(pool)

	err := executor.WithinTeam(context.Background(), team, func(queries *tenantdb.Queries) error {
		_, createErr := queries.CreateTeam(context.Background(), tenantdb.CreateTeamParams{
			Name: strconv.FormatInt(teamID, 10), Slug: "team-scope-committed",
		})
		return createErr
	})
	require.NoError(t, err)

	var count int
	require.NoError(t, fixture.pool.QueryRow(context.Background(), `SELECT count(*) FROM `+pgx.Identifier{fixture.schemaName, "teams"}.Sanitize()+` WHERE slug = 'team-scope-committed'`).Scan(&count))
	assert.Equal(t, 1, count)
	assertConnectionHasDefaultScope(t, pool)
}

func TestWithinTeamRollsBackAndRejectsInvalidInputBeforeAcquire(t *testing.T) {
	fixture, team, teamID := resolvedTeamFixture(t)
	installTeamScopeTrigger(t, fixture)
	pool := openSingleConnectionPool(t)
	executor := tenancy.NewExecutor(pool)
	wantErr := errors.New("team callback failed")

	err := executor.WithinTeam(context.Background(), team, func(queries *tenantdb.Queries) error {
		_, createErr := queries.CreateTeam(context.Background(), tenantdb.CreateTeamParams{
			Name: strconv.FormatInt(teamID, 10), Slug: "team-scope-rolled-back",
		})
		require.NoError(t, createErr)
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)
	assertTeamAbsent(t, fixture, "team-scope-rolled-back")
	assertConnectionHasDefaultScope(t, pool)

	before := pool.Stat().AcquireCount()
	err = executor.WithinTeam(context.Background(), tenancy.TeamContext{}, func(*tenantdb.Queries) error { return nil })
	require.ErrorIs(t, err, tenancy.ErrInvalidContext)
	assert.Equal(t, before, pool.Stat().AcquireCount())
	err = executor.WithinTeam(context.Background(), team, nil)
	require.ErrorIs(t, err, tenancy.ErrInvalidCallback)
	assert.Equal(t, before, pool.Stat().AcquireCount())
}

func applyTenantFixture(t *testing.T, fixture resolverFixture) {
	t.Helper()
	testsupport.ApplyMigrations(t, "db/migrations/tenant", testsupport.DatabaseURLForSchema(t, fixture.schemaName))
	identifier := pgx.Identifier{fixture.schemaName}.Sanitize()
	_, err := fixture.pool.Exec(context.Background(), fmt.Sprintf(`
		CREATE FUNCTION %s.assert_organization_scope() RETURNS trigger
		LANGUAGE plpgsql AS $function$
		BEGIN
			IF current_setting('app.user_id', true) !~ '^[1-9][0-9]*$' THEN
				RAISE EXCEPTION 'missing user scope';
			END IF;
			IF COALESCE(current_setting('app.team_id', true), '') <> '' THEN
				RAISE EXCEPTION 'unexpected team scope';
			END IF;
			IF current_setting('search_path') <> '"pg_catalog", "' || TG_TABLE_SCHEMA || '"' THEN
				RAISE EXCEPTION 'invalid search path';
			END IF;
			RETURN NEW;
		END
		$function$;
		CREATE TRIGGER assert_organization_scope
		BEFORE INSERT ON %s.teams
		FOR EACH ROW EXECUTE FUNCTION %s.assert_organization_scope()`, identifier, identifier, identifier))
	require.NoError(t, err)
}

func installDeferredCommitFailure(t *testing.T, fixture resolverFixture) {
	t.Helper()
	identifier := pgx.Identifier{fixture.schemaName}.Sanitize()
	// #nosec G201 -- the only formatted value is an identifier generated from a
	// server-owned UUID and quoted by pgx.Identifier.
	_, err := fixture.pool.Exec(context.Background(), fmt.Sprintf(`
		CREATE FUNCTION %s.fail_deferred_commit() RETURNS trigger
		LANGUAGE plpgsql AS $function$
		BEGIN
			RAISE EXCEPTION 'test deferred commit failure';
		END
		$function$;
		CREATE CONSTRAINT TRIGGER fail_deferred_commit
		AFTER INSERT ON %s.teams
		DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW EXECUTE FUNCTION %s.fail_deferred_commit()`, identifier, identifier, identifier))
	require.NoError(t, err)
}

func resolvedTeamFixture(t *testing.T) (resolverFixture, tenancy.TeamContext, int64) {
	t.Helper()
	fixture := newResolverFixture(t)
	applyTenantFixture(t, fixture)
	organization, err := fixture.resolver.ResolveOrganization(context.Background(), fixture.userPublicID, fixture.organizationID)
	require.NoError(t, err)
	require.NoError(t, tenancy.NewExecutor(fixture.pool).WithinOrganization(context.Background(), organization, func(queries *tenantdb.Queries) error {
		_, createErr := queries.CreateTeam(context.Background(), tenantdb.CreateTeamParams{Name: "Team scope", Slug: "team-scope"})
		return createErr
	}))
	team, err := fixture.resolver.ResolveTeam(context.Background(), organization, "team-scope")
	require.NoError(t, err)
	var teamID int64
	require.NoError(t, fixture.pool.QueryRow(context.Background(), `SELECT id FROM `+pgx.Identifier{fixture.schemaName, "teams"}.Sanitize()+` WHERE slug = 'team-scope'`).Scan(&teamID))
	return fixture, team, teamID
}

func installTeamScopeTrigger(t *testing.T, fixture resolverFixture) {
	t.Helper()
	identifier := pgx.Identifier{fixture.schemaName}.Sanitize()
	qualifiedTeams := pgx.Identifier{fixture.schemaName, "teams"}.Sanitize()
	_, err := fixture.pool.Exec(context.Background(), `DROP TRIGGER assert_organization_scope ON `+qualifiedTeams)
	require.NoError(t, err)
	// #nosec G201 -- the only formatted value is a quoted canonical identifier.
	_, err = fixture.pool.Exec(context.Background(), fmt.Sprintf(`
		CREATE FUNCTION %s.assert_team_scope() RETURNS trigger
		LANGUAGE plpgsql AS $function$
		BEGIN
			IF current_setting('app.user_id', true) !~ '^[1-9][0-9]*$' THEN
				RAISE EXCEPTION 'missing user scope';
			END IF;
			IF current_setting('app.team_id', true) <> split_part(NEW.name, ':', 1) THEN
				RAISE EXCEPTION 'incorrect team scope';
			END IF;
			IF current_setting('search_path') <> '"pg_catalog", "' || TG_TABLE_SCHEMA || '"' THEN
				RAISE EXCEPTION 'invalid search path';
			END IF;
			RETURN NEW;
		END
		$function$;
		CREATE TRIGGER assert_team_scope
		BEFORE INSERT ON %s.teams
		FOR EACH ROW EXECUTE FUNCTION %s.assert_team_scope()`, identifier, identifier, identifier))
	require.NoError(t, err)
}

func installBackendTerminationTrigger(t *testing.T, fixture resolverFixture) {
	t.Helper()
	identifier := pgx.Identifier{fixture.schemaName}.Sanitize()
	// #nosec G201 -- the only formatted value is a quoted canonical identifier.
	_, err := fixture.pool.Exec(context.Background(), fmt.Sprintf(`
		CREATE FUNCTION %s.terminate_executor_backend() RETURNS trigger
		LANGUAGE plpgsql AS $function$
		BEGIN
			PERFORM pg_terminate_backend(pg_backend_pid());
			RETURN NEW;
		END
		$function$;
		CREATE TRIGGER terminate_executor_backend
		BEFORE INSERT ON %s.teams
		FOR EACH ROW EXECUTE FUNCTION %s.terminate_executor_backend()`, identifier, identifier, identifier))
	require.NoError(t, err)
}

func removeBackendTerminationTrigger(t *testing.T, fixture resolverFixture) {
	t.Helper()
	qualifiedTeams := pgx.Identifier{fixture.schemaName, "teams"}.Sanitize()
	qualifiedFunction := pgx.Identifier{fixture.schemaName, "terminate_executor_backend"}.Sanitize()
	_, err := fixture.pool.Exec(context.Background(), `DROP TRIGGER terminate_executor_backend ON `+qualifiedTeams+`; DROP FUNCTION `+qualifiedFunction+`()`) // #nosec G202 -- both identifiers are pgx-quoted.
	require.NoError(t, err)
}

func assertTeamAbsent(t *testing.T, fixture resolverFixture, slug string) {
	t.Helper()
	var count int
	require.NoError(t, fixture.pool.QueryRow(context.Background(), `SELECT count(*) FROM `+pgx.Identifier{fixture.schemaName, "teams"}.Sanitize()+` WHERE slug = $1`, slug).Scan(&count))
	assert.Zero(t, count)
}

func openSingleConnectionPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	config, err := pgxpool.ParseConfig(testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	require.NoError(t, err)
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(context.Background()))
	return pool
}

func assertConnectionHasDefaultScope(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var searchPath string
	var userSetting pgtype.Text
	var teamSetting pgtype.Text
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT current_setting('search_path'),
		       current_setting('app.user_id', true),
		       current_setting('app.team_id', true)`).Scan(&searchPath, &userSetting, &teamSetting))
	assert.Equal(t, `"$user", public`, searchPath)
	assert.Empty(t, userSetting.String)
	assert.Empty(t, teamSetting.String)
}
