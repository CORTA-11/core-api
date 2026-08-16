package service

import (
	"context"
	"time"

	"github.com/CORTA-11/core-api/internal/repository"
)

type Team struct {
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func mapDBTeamToDomain(row repository.Team) Team {
	return Team{
		Name:      row.Name,
		Slug:      row.Slug,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

type TeamService interface {
	GetTeams(ctx context.Context, schema string) ([]Team, error)
	CreateTeam(ctx context.Context, name, schema string) (*Team, error)
	GetTeamID(ctx context.Context, slug, schema string) (int, error)
}
