package service

import (
	"context"
	"fmt"

	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type teamService struct {
	pool    *pgxpool.Pool
	queries *repository.Queries
}

func NewTeamService(pool *pgxpool.Pool, queries *repository.Queries) TeamService {
	return &teamService{
		pool:    pool,
		queries: queries,
	}
}

func (t *teamService) GetTeams(ctx context.Context, schema string) ([]Team, error) {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %q", err)
	}
	defer tx.Rollback(ctx)

	if err := setSchema(ctx, tx, schema); err != nil {
		return nil, fmt.Errorf("failed to set search_path: %q", err)
	}

	qtx := t.queries.WithTx(tx)

	teams, err := qtx.GetTeams(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch teams: %q", err)
	}

	tx.Commit(ctx)

	domainTeams := make([]Team, 0, len(teams))

	for _, team := range teams {
		domainTeams = append(domainTeams, mapDBTeamToDomain(team))
	}

	return domainTeams, nil
}

func mapDBTeamToDomain(row repository.Team) Team {
	return Team{
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func (t *teamService) CreateTeam(ctx context.Context, name, schema string) (*Team, error) {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %q", err)
	}
	defer tx.Rollback(ctx)

	if err := setSchema(ctx, tx, schema); err != nil {
		return nil, fmt.Errorf("failed to set search_path: %q", err)
	}

	qtx := t.queries.WithTx(tx)

	team, err := qtx.CreateTeam(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to create team: %q", err)
	}

	tx.Commit(ctx)

	ret := mapDBTeamToDomain(team)

	return &ret, nil
}
