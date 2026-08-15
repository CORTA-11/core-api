package service

import (
	"context"
	"time"

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

type OrgService interface {
	GetOrgs(ctx context.Context) ([]Organization, error)
	CreateOrg(ctx context.Context, name string) (*Organization, error)
	UpdateOrg(ctx context.Context, publicID uuid.UUID, name string) (*Organization, error)
	SoftDeleteOrg(ctx context.Context, publicID uuid.UUID) (*Organization, error)
	RestoreOrg(ctx context.Context, publicID uuid.UUID) (*Organization, error)
}
