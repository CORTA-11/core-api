//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
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
