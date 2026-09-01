package service

import (
	"context"
	"time"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/google/uuid"
)

// Organization is the client-facing registry view. The internal schema name is
// deliberately excluded so clients cannot treat it as a tenant identifier.
type Organization struct {
	PublicID       uuid.UUID `json:"public_id"`
	Name           string    `json:"name"`
	LifecycleState string    `json:"lifecycle_state"`
	TenantVersion  int64     `json:"tenant_version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	DeletedAt      time.Time `json:"deleted_at"`
}

// mapDBOrgToDomain maps dbor g to domain.
func mapDBOrgToDomain(row publicdb.Org) Organization {
	var deletedAt time.Time

	if row.DeletedAt.Valid {
		deletedAt = row.DeletedAt.Time
	}

	return Organization{
		PublicID:       row.PublicID,
		Name:           row.Name,
		LifecycleState: row.LifecycleState,
		TenantVersion:  row.TenantVersion,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		DeletedAt:      deletedAt,
	}
}

// OrgService manages organization registry records and their lifecycle intent.
// Tenant schema creation and migration belong to the provisioner.
type OrgService interface {
	GetOrgs(ctx context.Context) ([]Organization, error)
	CreateOrg(ctx context.Context, name string) (*Organization, error)
	UpdateOrg(ctx context.Context, publicID uuid.UUID, name string) (*Organization, error)
	SoftDeleteOrg(ctx context.Context, publicID uuid.UUID) (*Organization, error)
	RestoreOrg(ctx context.Context, publicID uuid.UUID) (*Organization, error)
}
