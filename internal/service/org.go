package service

import (
	"context"
	"time"

	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/google/uuid"
)

type Organization struct {
	PublicID   uuid.UUID `json:"public_id"`
	Name       string    `json:"name"`
	SchemaName string    `json:"schema_name"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	DeletedAt  time.Time `json:"deleted_at"`
}

func mapDBOrgToDomain(row repository.Org) Organization {
	var deletedAt time.Time

	if row.DeletedAt.Valid {
		deletedAt = row.DeletedAt.Time
	}

	return Organization{
		PublicID:   row.PublicID,
		Name:       row.Name,
		SchemaName: row.SchemaName,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
		DeletedAt:  deletedAt,
	}
}

type OrgService interface {
	GetOrgs(ctx context.Context) ([]Organization, error)
	CreateOrg(ctx context.Context, name string) (*Organization, error)
	UpdateOrg(ctx context.Context, publicID uuid.UUID, name string) (*Organization, error)
	SoftDeleteOrg(ctx context.Context, publicID uuid.UUID) (*Organization, error)
	RestoreOrg(ctx context.Context, publicID uuid.UUID) (*Organization, error)
}
