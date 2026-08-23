package service

import (
	"context"
	"fmt"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
)

// LegacyTeamLookup exists only while the task and file routes are migrated in
// the next two atomic commits. New production code must use tenancy.Resolver.
type LegacyTeamLookup interface {
	GetTeamID(ctx context.Context, slug, schema string) (int, error)
}

type legacyTeamLookup struct {
	pool    pgxPool
	queries *tenantdb.Queries
}

func NewLegacyTeamLookup(pool pgxPool, queries *tenantdb.Queries) LegacyTeamLookup {
	return &legacyTeamLookup{pool: pool, queries: queries}
}

func (lookup *legacyTeamLookup) GetTeamID(ctx context.Context, slug, schema string) (int, error) {
	tx, err := lookup.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("start legacy team lookup: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := setSchema(ctx, tx, schema); err != nil {
		return 0, err
	}
	teamID, err := lookup.queries.WithTx(tx).GetTeamID(ctx, slug)
	if err != nil {
		return 0, fmt.Errorf("look up legacy team: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit legacy team lookup: %w", err)
	}
	return int(teamID), nil
}
