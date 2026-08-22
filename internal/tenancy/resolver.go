package tenancy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrOrganizationUnavailable deliberately combines unknown identities,
	// nonmembership, unavailable lifecycle states, stale tenants, and missing
	// schemas so callers cannot use resolution as an organization oracle.
	ErrOrganizationUnavailable = errors.New("organization is unavailable")
	// ErrTeamUnavailable deliberately combines invalid, unknown, and currently
	// unauthorized teams. D04 will strengthen it with team membership.
	ErrTeamUnavailable = errors.New("team is unavailable")
	// ErrInvalidContext rejects zero, incomplete, or internally forged contexts
	// before an executor acquires a database transaction.
	ErrInvalidContext = errors.New("tenant context is invalid")
	// ErrRegistryIntegrity identifies canonical registry corruption without
	// exposing the stored or expected schema name.
	ErrRegistryIntegrity = errors.New("tenant registry integrity violation")
)

// OrganizationContext is an opaque capability produced only by Resolver.
// Its internal identifiers and schema name must never cross an API boundary.
type OrganizationContext struct {
	organizationID       int64
	organizationPublicID uuid.UUID
	userID               int64
	userPublicID         uuid.UUID
	schemaName           string
	resolved             bool
}

func newOrganizationContext(
	organizationID int64,
	organizationPublicID uuid.UUID,
	userID int64,
	userPublicID uuid.UUID,
	schemaName string,
) OrganizationContext {
	return OrganizationContext{
		organizationID:       organizationID,
		organizationPublicID: organizationPublicID,
		userID:               userID,
		userPublicID:         userPublicID,
		schemaName:           schemaName,
		resolved:             true,
	}
}

func (organization OrganizationContext) validate() error {
	if !organization.resolved || organization.organizationID <= 0 || organization.userID <= 0 ||
		organization.organizationPublicID == uuid.Nil || organization.userPublicID == uuid.Nil ||
		organization.schemaName == "" ||
		organization.schemaName != CanonicalSchema(organization.organizationPublicID.String()) {
		return ErrInvalidContext
	}
	return nil
}

// Resolver turns public identities into opaque tenant capabilities after all
// current control-plane and catalog checks succeed.
type Resolver struct {
	pool    *pgxpool.Pool
	queries *publicdb.Queries
	source  MigrationSet
}

// NewResolver constructs a resolver for the migration state embedded in the
// serving binary.
func NewResolver(pool *pgxpool.Pool, source MigrationSet) *Resolver {
	return &Resolver{pool: pool, queries: publicdb.New(pool), source: source}
}

// ResolveOrganization requires a current non-deleted user membership, an
// active registry row at the exact embedded migration state, a canonical
// registry schema name, and an exact namespace catalog match.
func (r *Resolver) ResolveOrganization(
	ctx context.Context,
	userPublicID uuid.UUID,
	organizationPublicID uuid.UUID,
) (OrganizationContext, error) {
	if r == nil || r.pool == nil || r.queries == nil || userPublicID == uuid.Nil || organizationPublicID == uuid.Nil {
		return OrganizationContext{}, ErrOrganizationUnavailable
	}
	row, err := r.queries.ResolveOrganizationContext(ctx, publicdb.ResolveOrganizationContextParams{
		UserPublicID:         userPublicID,
		OrganizationPublicID: organizationPublicID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return OrganizationContext{}, ErrOrganizationUnavailable
	}
	if err != nil {
		return OrganizationContext{}, fmt.Errorf("resolve organization context: %w", err)
	}
	if row.SchemaName != CanonicalSchema(row.OrganizationPublicID.String()) {
		return OrganizationContext{}, ErrRegistryIntegrity
	}
	if row.LifecycleState != string(StateActive) || row.TenantVersion != r.source.Version ||
		strings.TrimSpace(row.TenantChecksum) != r.source.Checksum || !row.SchemaExists {
		return OrganizationContext{}, ErrOrganizationUnavailable
	}
	organization := newOrganizationContext(
		row.OrganizationID,
		row.OrganizationPublicID,
		row.UserID,
		row.UserPublicID,
		row.SchemaName,
	)
	if err := organization.validate(); err != nil {
		return OrganizationContext{}, ErrRegistryIntegrity
	}
	return organization, nil
}
