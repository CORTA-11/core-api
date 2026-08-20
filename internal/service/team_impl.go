package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
)

type teamService struct {
	pool    pgxPool
	queries *tenantdb.Queries
}

func NewTeamService(pool pgxPool, queries *tenantdb.Queries) TeamService {
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
	defer func() { _ = tx.Rollback(ctx) }()

	if err := setSchema(ctx, tx, schema); err != nil {
		return nil, fmt.Errorf("failed to set search_path: %q", err)
	}

	qtx := t.queries.WithTx(tx)

	teams, err := qtx.GetTeams(ctx, listResultLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch teams: %q", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %q", err)
	}

	domainTeams := make([]Team, 0, len(teams))

	for _, team := range teams {
		domainTeams = append(domainTeams, mapDBTeamToDomain(team))
	}

	return domainTeams, nil
}

func (t *teamService) CreateTeam(ctx context.Context, name, schema string) (*Team, error) {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %q", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := setSchema(ctx, tx, schema); err != nil {
		return nil, fmt.Errorf("failed to set search_path: %q", err)
	}

	qtx := t.queries.WithTx(tx)

	slug := strings.ReplaceAll(strings.ToLower(name), " ", "-")

	team, err := qtx.CreateTeam(ctx, tenantdb.CreateTeamParams{
		Name: name,
		Slug: slug,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create team: %q", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %q", err)
	}

	ret := mapDBTeamToDomain(team)

	return &ret, nil
}

func (t *teamService) GetTeamID(ctx context.Context, slug, schema string) (int, error) {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return -1, fmt.Errorf("failed to start transaction: %q", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := setSchema(ctx, tx, schema); err != nil {
		return -1, fmt.Errorf("failed to set search_path: %q", err)
	}

	qtx := t.queries.WithTx(tx)

	teamID, err := qtx.GetTeamID(ctx, slug)
	if err != nil {
		return -1, fmt.Errorf("failed to get teamID: %q", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return -1, fmt.Errorf("failed to commit transaction: %q", err)
	}

	return int(teamID), nil
}
