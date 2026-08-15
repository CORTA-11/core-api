package service

import (
	"context"
	"time"
)

type Team struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TeamService interface {
	GetTeams(ctx context.Context, schema string) ([]Team, error)
	CreateTeam(ctx context.Context, name, schema string) (*Team, error)
}
