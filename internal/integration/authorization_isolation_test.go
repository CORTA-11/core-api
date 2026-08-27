//go:build isolation

package integration_test

import (
	"context"
	"testing"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type revokingResolver struct {
	base      *tenancy.Resolver
	afterOrg  func()
	afterTeam func()
}

func (resolver revokingResolver) ResolveOrganization(ctx context.Context, userID, organizationID uuid.UUID) (tenancy.OrganizationContext, error) {
	organization, err := resolver.base.ResolveOrganization(ctx, userID, organizationID)
	if err == nil && resolver.afterOrg != nil {
		resolver.afterOrg()
		resolver.afterOrg = nil
	}
	return organization, err
}

func (resolver revokingResolver) ResolveTeam(ctx context.Context, organization tenancy.OrganizationContext, teamID uuid.UUID) (tenancy.TeamContext, error) {
	team, err := resolver.base.ResolveTeam(ctx, organization, teamID)
	if err == nil && resolver.afterTeam != nil {
		resolver.afterTeam()
		resolver.afterTeam = nil
	}
	return team, err
}

func TestAuthorizationRevalidatesRevocationLifecycleAndExactTeam(t *testing.T) {
	fixture := newTenantBoundaryFixture(t)
	ctx := context.Background()
	organization := fixture.orgs[0]
	team := organization.teams[0]
	userID := fixture.users.shared
	principal := session.Principal{UserID: userID, SessionID: uuid.New()}

	_, err := fixture.adminPool.Exec(ctx, `
		UPDATE public.org_user SET role = 'administrator'
		WHERE org_id = $1 AND user_id = (SELECT id FROM public.users WHERE user_id = $2)`, organization.id, userID)
	require.NoError(t, err)
	authorizer := authorization.NewAuthorizer(fixture.resolver, fixture.executor)
	called := false
	err = authorizer.WithinOrganization(ctx, principal, organization.publicID, authorization.PermissionOrgRead,
		func(*tenantdb.Queries) error { called = true; return nil })
	require.NoError(t, err)
	assert.True(t, called, "ownerless organizations remain readable")
	called = false
	err = authorizer.WithinOrganization(ctx, principal, organization.publicID, authorization.PermissionTeamCreate,
		func(*tenantdb.Queries) error { called = true; return nil })
	assert.ErrorIs(t, err, authorization.ErrOperationDenied)
	assert.False(t, called, "ownerless organizations deny administrative mutations")

	_, err = fixture.adminPool.Exec(ctx, `
		UPDATE public.org_user SET role = 'owner'
		WHERE org_id = $1 AND user_id = (SELECT id FROM public.users WHERE user_id = $2)`, organization.id, userID)
	require.NoError(t, err)
	membersTable := pgx.Identifier{organization.schema, "team_members"}.Sanitize()
	_, err = fixture.adminPool.Exec(ctx, `UPDATE `+membersTable+` SET role = 'viewer' WHERE team_id = $1 AND user_public_id = $2`, team.id, userID)
	require.NoError(t, err)

	authorizer = authorization.NewAuthorizer(fixture.resolver, fixture.executor)
	called = false
	err = authorizer.WithinOrganization(ctx, principal, organization.publicID, authorization.PermissionOrgRead,
		func(*tenantdb.Queries) error { called = true; return nil })
	require.NoError(t, err)
	assert.True(t, called)

	called = false
	err = authorizer.WithinTeam(ctx, principal, organization.publicID, team.publicID, authorization.PermissionTaskUpdate,
		func(*tenantdb.Queries) error { called = true; return nil })
	assert.ErrorIs(t, err, authorization.ErrOperationDenied)
	assert.False(t, called, "organization owner status must not grant team content access")

	called = false
	err = authorizer.WithinTeam(ctx, principal, organization.publicID, uuid.New(), authorization.PermissionTaskRead,
		func(*tenantdb.Queries) error { called = true; return nil })
	assert.ErrorIs(t, err, authorization.ErrResourceNotFound)
	assert.False(t, called)

	revoked := revokingResolver{base: fixture.resolver, afterOrg: func() {
		_, deleteErr := fixture.adminPool.Exec(ctx, `DELETE FROM public.org_user WHERE org_id = $1 AND user_id = (SELECT id FROM public.users WHERE user_id = $2)`, organization.id, userID)
		require.NoError(t, deleteErr)
	}}
	called = false
	err = authorization.NewAuthorizer(revoked, fixture.executor).WithinOrganization(ctx, principal, organization.publicID,
		authorization.PermissionOrgRead, func(*tenantdb.Queries) error { called = true; return nil })
	assert.ErrorIs(t, err, authorization.ErrResourceNotFound)
	assert.False(t, called)

	_, err = fixture.adminPool.Exec(ctx, `
		INSERT INTO public.org_user (org_id, user_id, role)
		SELECT $1, id, 'owner' FROM public.users WHERE user_id = $2`, organization.id, userID)
	require.NoError(t, err)
	lifecycleChanged := revokingResolver{base: fixture.resolver, afterOrg: func() {
		_, updateErr := fixture.adminPool.Exec(ctx, `
			UPDATE public.orgs SET lifecycle_state = 'provisioning', tenant_version = 0,
			tenant_checksum = '', provisioned_at = NULL WHERE id = $1`, organization.id)
		require.NoError(t, updateErr)
	}}
	called = false
	err = authorization.NewAuthorizer(lifecycleChanged, fixture.executor).WithinOrganization(ctx, principal, organization.publicID,
		authorization.PermissionOrgRead, func(*tenantdb.Queries) error { called = true; return nil })
	assert.ErrorIs(t, err, authorization.ErrResourceNotFound)
	assert.False(t, called)
}
