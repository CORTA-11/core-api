package authorization

import (
	"context"
	"errors"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type tenantResolver interface {
	ResolveOrganization(context.Context, uuid.UUID, uuid.UUID) (tenancy.OrganizationContext, error)
	ResolveTeam(context.Context, tenancy.OrganizationContext, uuid.UUID) (tenancy.TeamContext, error)
}

type tenantExecutor interface {
	WithinOrganizationQueries(context.Context, tenancy.OrganizationContext, tenancy.GeneratedQueriesCallback) error
	WithinTeamQueries(context.Context, tenancy.TeamContext, tenancy.GeneratedQueriesCallback) error
}

type Authorizer struct {
	resolver tenantResolver
	executor tenantExecutor
}

func NewAuthorizer(resolver tenantResolver, executor tenantExecutor) *Authorizer {
	return &Authorizer{resolver: resolver, executor: executor}
}

type TenantCallback func(*tenantdb.Queries) error

// WithinOrganization resolves the public organization ID, then re-reads the
// current lifecycle and membership role in the same transaction as callback.
func (authorizer *Authorizer) WithinOrganization(
	ctx context.Context,
	principal session.Principal,
	organizationID uuid.UUID,
	permission Permission,
	callback TenantCallback,
) error {
	if !authenticated(principal) {
		return ErrUnauthenticated
	}
	if authorizer == nil || authorizer.resolver == nil || authorizer.executor == nil ||
		organizationID == uuid.Nil || !ValidPermission(permission) || callback == nil {
		return ErrOperationDenied
	}
	organization, err := authorizer.resolver.ResolveOrganization(ctx, principal.UserID, organizationID)
	if err != nil {
		return ErrResourceNotFound
	}
	invoked := false
	err = authorizer.executor.WithinOrganizationQueries(ctx, organization,
		func(publicQueries *publicdb.Queries, tenantQueries *tenantdb.Queries) error {
			membership, lookupErr := publicQueries.GetOrganizationMembership(ctx, publicdb.GetOrganizationMembershipParams{
				OrganizationPublicID: organizationID,
				UserPublicID:         principal.UserID,
			})
			if errors.Is(lookupErr, pgx.ErrNoRows) {
				return ErrResourceNotFound
			}
			if lookupErr != nil {
				return ErrResourceNotFound
			}
			if !OrganizationAllows(OrganizationRole(membership.Role), permission) {
				return ErrOperationDenied
			}
			if organizationMutationRequiresOwner(permission) {
				owners, countErr := publicQueries.CountOrganizationOwners(ctx, membership.OrgID)
				if countErr != nil || owners < 1 {
					return ErrOperationDenied
				}
			}
			invoked = true
			return callback(tenantQueries)
		})
	if err != nil && !invoked && !errors.Is(err, ErrOperationDenied) && !errors.Is(err, ErrResourceNotFound) {
		return ErrResourceNotFound
	}
	return err
}

// WithinTeam binds authorization to the exact resolved team and re-reads its
// current membership role inside the state-changing tenant transaction.
func (authorizer *Authorizer) WithinTeam(
	ctx context.Context,
	principal session.Principal,
	organizationID uuid.UUID,
	teamID uuid.UUID,
	permission Permission,
	callback TenantCallback,
) error {
	if !authenticated(principal) {
		return ErrUnauthenticated
	}
	if authorizer == nil || authorizer.resolver == nil || authorizer.executor == nil ||
		organizationID == uuid.Nil || teamID == uuid.Nil || !ValidPermission(permission) || callback == nil {
		return ErrOperationDenied
	}
	organization, err := authorizer.resolver.ResolveOrganization(ctx, principal.UserID, organizationID)
	if err != nil {
		return ErrResourceNotFound
	}
	team, err := authorizer.resolver.ResolveTeam(ctx, organization, teamID)
	if err != nil {
		return ErrResourceNotFound
	}
	invoked := false
	err = authorizer.executor.WithinTeamQueries(ctx, team,
		func(_ *publicdb.Queries, tenantQueries *tenantdb.Queries) error {
			membership, lookupErr := tenantQueries.RevalidateTeamAuthorization(ctx, tenantdb.RevalidateTeamAuthorizationParams{
				TeamPublicID: teamID,
				UserPublicID: principal.UserID,
			})
			if lookupErr != nil {
				return ErrResourceNotFound
			}
			if !TeamAllows(TeamRole(membership.Role), permission) {
				return ErrOperationDenied
			}
			invoked = true
			return callback(tenantQueries)
		})
	if err != nil && !invoked && !errors.Is(err, ErrOperationDenied) && !errors.Is(err, ErrResourceNotFound) {
		return ErrResourceNotFound
	}
	return err
}

func authenticated(principal session.Principal) bool {
	return principal.UserID != uuid.Nil && principal.SessionID != uuid.Nil
}

func organizationMutationRequiresOwner(permission Permission) bool {
	switch permission {
	case PermissionOrgUpdate, PermissionOrgDelete, PermissionOrgRestore,
		PermissionOrgMembersManage, PermissionOrgOwnersManage, PermissionTeamCreate:
		return true
	default:
		return false
	}
}
