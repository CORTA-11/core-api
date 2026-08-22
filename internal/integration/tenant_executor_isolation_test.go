//go:build isolation

package integration_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamScopesAlternateAcrossOrganizationsOnOneConnection(t *testing.T) {
	first := newResolverFixture(t)
	second := addResolverFixture(t, first, "second@example.test")
	applyTenantFixture(t, first)
	applyTenantFixture(t, second)

	ctx := context.Background()
	firstOrganization, err := first.resolver.ResolveOrganization(ctx, first.userPublicID, first.organizationID)
	require.NoError(t, err)
	secondOrganization, err := second.resolver.ResolveOrganization(ctx, second.userPublicID, second.organizationID)
	require.NoError(t, err)
	pool := openSingleConnectionPool(t)
	executor := tenancy.NewExecutor(pool)

	require.NoError(t, executor.WithinOrganization(ctx, firstOrganization, createScopedTeam(ctx, "First scope", "first-scope")))
	require.NoError(t, executor.WithinOrganization(ctx, secondOrganization, createScopedTeam(ctx, "Second scope", "second-scope")))
	firstTeam, err := first.resolver.ResolveTeam(ctx, firstOrganization, "first-scope")
	require.NoError(t, err)
	secondTeam, err := second.resolver.ResolveTeam(ctx, secondOrganization, "second-scope")
	require.NoError(t, err)
	firstTeamID := lookupTeamID(t, first, "first-scope")
	secondTeamID := lookupTeamID(t, second, "second-scope")
	installTeamScopeTrigger(t, first)
	installTeamScopeTrigger(t, second)

	operations := []struct {
		team   tenancy.TeamContext
		teamID int64
		slug   string
	}{
		{firstTeam, firstTeamID, "first-operation-one"},
		{secondTeam, secondTeamID, "second-operation-one"},
		{firstTeam, firstTeamID, "first-operation-two"},
		{secondTeam, secondTeamID, "second-operation-two"},
	}
	for _, operation := range operations {
		err := executor.WithinTeam(ctx, operation.team, func(queries *tenantdb.Queries) error {
			_, createErr := queries.CreateTeam(ctx, tenantdb.CreateTeamParams{
				Name: strconv.FormatInt(operation.teamID, 10) + ":" + operation.slug, Slug: operation.slug,
			})
			return createErr
		})
		require.NoError(t, err)
		assertConnectionHasDefaultScope(t, pool)
	}

	assert.Equal(t, 2, countOperationTeams(t, first, "first-operation-%"))
	assert.Equal(t, 2, countOperationTeams(t, second, "second-operation-%"))
	assert.Zero(t, countOperationTeams(t, first, "second-operation-%"))
	assert.Zero(t, countOperationTeams(t, second, "first-operation-%"))
}

func addResolverFixture(t *testing.T, first resolverFixture, email string) resolverFixture {
	t.Helper()
	ctx := context.Background()
	var userID int64
	var userPublicID uuid.UUID
	require.NoError(t, first.pool.QueryRow(ctx, `
		INSERT INTO public.users (email, password_hash, display_name)
		VALUES ($1, 'hash', 'Second Resolver')
		RETURNING id, user_id`, email).Scan(&userID, &userPublicID))
	source, err := tenancy.EmbeddedMigrations()
	require.NoError(t, err)
	organizationID := uuid.New()
	schemaName := tenancy.CanonicalSchema(organizationID.String())
	var internalOrganizationID int64
	require.NoError(t, first.pool.QueryRow(ctx, `
		INSERT INTO public.orgs (
			public_id, name, schema_name, lifecycle_state, tenant_version,
			tenant_checksum, provisioned_at
		) VALUES ($1, 'Second Resolver Org', $2, 'active', $3, $4, now())
		RETURNING id`, organizationID, schemaName, source.Version, source.Checksum).Scan(&internalOrganizationID))
	testsupport.CreateSchema(t, first.pool, schemaName)
	_, err = first.pool.Exec(ctx, `INSERT INTO public.org_user (org_id, user_id) VALUES ($1, $2)`, internalOrganizationID, userID)
	require.NoError(t, err)
	return resolverFixture{
		pool: first.pool, resolver: first.resolver, userPublicID: userPublicID,
		organizationID: organizationID, schemaName: schemaName,
	}
}

func createScopedTeam(ctx context.Context, name, slug string) func(*tenantdb.Queries) error {
	return func(queries *tenantdb.Queries) error {
		_, err := queries.CreateTeam(ctx, tenantdb.CreateTeamParams{Name: name, Slug: slug})
		return err
	}
}

func lookupTeamID(t *testing.T, fixture resolverFixture, slug string) int64 {
	t.Helper()
	var teamID int64
	require.NoError(t, fixture.pool.QueryRow(context.Background(), `SELECT id FROM `+pgx.Identifier{fixture.schemaName, "teams"}.Sanitize()+` WHERE slug = $1`, slug).Scan(&teamID))
	return teamID
}

func countOperationTeams(t *testing.T, fixture resolverFixture, pattern string) int {
	t.Helper()
	var count int
	query := fmt.Sprintf(`SELECT count(*) FROM %s WHERE slug LIKE $1`, pgx.Identifier{fixture.schemaName, "teams"}.Sanitize()) // #nosec G201 -- canonical pgx-quoted identifier.
	require.NoError(t, fixture.pool.QueryRow(context.Background(), query, pattern).Scan(&count))
	return count
}
