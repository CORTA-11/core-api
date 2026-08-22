//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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
