package authorization

import (
	"context"
	"errors"
	"fmt"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maximumLockedOrganizationMemberships int32 = 1000

type MembershipManager struct {
	pool *pgxpool.Pool
}

// NewMembershipManager creates a membership manager.
func NewMembershipManager(pool *pgxpool.Pool) *MembershipManager {
	return &MembershipManager{pool: pool}
}

// ChangeRole locks the organization and its membership set before deciding.
// This serializes owner changes with operator owner assignment and prevents two
// concurrent transactions from each demoting the apparent last owner.
func (manager *MembershipManager) ChangeRole(
	ctx context.Context,
	principal session.Principal,
	organizationID uuid.UUID,
	targetUserID uuid.UUID,
	newRole OrganizationRole,
) error {
	if !authenticated(principal) {
		return ErrUnauthenticated
	}
	if manager == nil || manager.pool == nil || organizationID == uuid.Nil ||
		targetUserID == uuid.Nil || !ValidOrganizationRole(newRole) {
		return ErrOperationDenied
	}
	tx, err := manager.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin membership change: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := publicdb.New(tx)
	locked, err := queries.LockOrganizationMemberships(ctx, publicdb.LockOrganizationMembershipsParams{
		OrganizationPublicID: organizationID, Limit: maximumLockedOrganizationMemberships + 1,
	})
	if err != nil || len(locked) == 0 {
		return ErrResourceNotFound
	}
	if len(locked) > int(maximumLockedOrganizationMemberships) {
		return ErrOperationDenied
	}
	actor, err := queries.GetOrganizationMembership(ctx, publicdb.GetOrganizationMembershipParams{
		OrganizationPublicID: organizationID, UserPublicID: principal.UserID,
	})
	if err != nil {
		return ErrResourceNotFound
	}
	target, err := queries.GetOrganizationMembership(ctx, publicdb.GetOrganizationMembershipParams{
		OrganizationPublicID: organizationID, UserPublicID: targetUserID,
	})
	if err != nil {
		return ErrResourceNotFound
	}
	owners, err := queries.CountOrganizationOwners(ctx, actor.OrgID)
	if err != nil || owners < 1 {
		return ErrOperationDenied
	}
	actorRole := OrganizationRole(actor.Role)
	currentRole := OrganizationRole(target.Role)
	if !OrganizationAllows(actorRole, PermissionOrgMembersManage) {
		return ErrOperationDenied
	}
	ownerChange := currentRole == OrganizationRoleOwner || newRole == OrganizationRoleOwner
	if ownerChange && !OrganizationAllows(actorRole, PermissionOrgOwnersManage) {
		return ErrOperationDenied
	}
	if currentRole == OrganizationRoleOwner && newRole != OrganizationRoleOwner && owners <= 1 {
		return ErrOperationDenied
	}
	if _, err := queries.SetOrganizationMembershipRole(ctx, publicdb.SetOrganizationMembershipRoleParams{
		Role: string(newRole), OrgID: target.OrgID, UserID: target.UserID,
	}); err != nil {
		return fmt.Errorf("change membership role: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit membership change: %w", err)
	}
	return nil
}

// Remove handles the remove operation.
func (manager *MembershipManager) Remove(
	ctx context.Context,
	principal session.Principal,
	organizationID uuid.UUID,
	targetUserID uuid.UUID,
) error {
	if !authenticated(principal) {
		return ErrUnauthenticated
	}
	if manager == nil || manager.pool == nil || organizationID == uuid.Nil || targetUserID == uuid.Nil {
		return ErrOperationDenied
	}
	tx, err := manager.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin membership removal: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := publicdb.New(tx)
	locked, err := queries.LockOrganizationMemberships(ctx, publicdb.LockOrganizationMembershipsParams{
		OrganizationPublicID: organizationID, Limit: maximumLockedOrganizationMemberships + 1,
	})
	if err != nil || len(locked) == 0 {
		return ErrResourceNotFound
	}
	if len(locked) > int(maximumLockedOrganizationMemberships) {
		return ErrOperationDenied
	}
	actor, err := queries.GetOrganizationMembership(ctx, publicdb.GetOrganizationMembershipParams{
		OrganizationPublicID: organizationID, UserPublicID: principal.UserID,
	})
	if err != nil {
		return ErrResourceNotFound
	}
	target, err := queries.GetOrganizationMembership(ctx, publicdb.GetOrganizationMembershipParams{
		OrganizationPublicID: organizationID, UserPublicID: targetUserID,
	})
	if err != nil {
		return ErrResourceNotFound
	}
	owners, err := queries.CountOrganizationOwners(ctx, actor.OrgID)
	if err != nil || owners < 1 || !OrganizationAllows(OrganizationRole(actor.Role), PermissionOrgMembersManage) {
		return ErrOperationDenied
	}
	if OrganizationRole(target.Role) == OrganizationRoleOwner {
		if !OrganizationAllows(OrganizationRole(actor.Role), PermissionOrgOwnersManage) || owners <= 1 {
			return ErrOperationDenied
		}
	}
	_, err = queries.RemoveUserFromOrg(ctx, publicdb.RemoveUserFromOrgParams{OrgID: target.OrgID, UserID: target.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrResourceNotFound
	}
	if err != nil {
		return fmt.Errorf("remove organization membership: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit membership removal: %w", err)
	}
	return nil
}
