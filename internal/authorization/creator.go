package authorization

import (
	"context"
	"fmt"
	"strings"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OrganizationCreator struct {
	pool *pgxpool.Pool
}

// NewOrganizationCreator creates an organization creator.
func NewOrganizationCreator(pool *pgxpool.Pool) *OrganizationCreator {
	return &OrganizationCreator{pool: pool}
}

type CreatedOrganization struct {
	PublicID uuid.UUID
	State    tenancy.LifecycleState
}

// Create atomically records provisioning intent and exactly one organization
// owner membership for the authenticated creator. It never creates a team or a
// tenant membership.
func (creator *OrganizationCreator) Create(
	ctx context.Context,
	principal session.Principal,
	name string,
) (CreatedOrganization, error) {
	if !authenticated(principal) {
		return CreatedOrganization{}, ErrUnauthenticated
	}
	name = strings.TrimSpace(name)
	if creator == nil || creator.pool == nil || name == "" {
		return CreatedOrganization{}, ErrOperationDenied
	}
	tx, err := creator.pool.Begin(ctx)
	if err != nil {
		return CreatedOrganization{}, fmt.Errorf("create organization transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := publicdb.New(tx)
	user, err := queries.GetUserByID(ctx, principal.UserID)
	if err != nil {
		return CreatedOrganization{}, ErrUnauthenticated
	}
	publicID := uuid.New()
	organization, err := queries.CreateOrg(ctx, publicdb.CreateOrgParams{
		Name: name, PublicID: publicID, SchemaName: tenancy.CanonicalSchema(publicID.String()),
	})
	if err != nil {
		return CreatedOrganization{}, fmt.Errorf("create organization registry: %w", err)
	}
	if _, err := queries.AddOrganizationOwnerMembership(ctx, publicdb.AddOrganizationOwnerMembershipParams{
		OrgID: organization.ID, UserID: user.ID,
	}); err != nil {
		return CreatedOrganization{}, fmt.Errorf("create organization owner: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CreatedOrganization{}, fmt.Errorf("commit organization creation: %w", err)
	}
	return CreatedOrganization{PublicID: organization.PublicID, State: tenancy.LifecycleState(organization.LifecycleState)}, nil
}
