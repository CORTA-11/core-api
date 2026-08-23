package service

import (
	"context"
	"time"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
)

type Team struct {
	PublicID  uuid.UUID `json:"public_id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func mapDBTeamToDomain(row tenantdb.Team) Team {
	return Team{
		PublicID:  row.PublicID,
		Name:      row.Name,
		Slug:      row.Slug,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

type TeamService interface {
	GetTeams(ctx context.Context, organization tenancy.OrganizationContext) ([]Team, error)
	CreateTeam(ctx context.Context, organization tenancy.OrganizationContext, name string) (*Team, error)
}

type organizationExecutor interface {
	WithinOrganization(context.Context, tenancy.OrganizationContext, func(*tenantdb.Queries) error) error
}
