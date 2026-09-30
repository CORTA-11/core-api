//go:build isolation

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamListingVisibilityBeforePagination(t *testing.T) {
	pool, schema, userID := rlsRuntimeFixture(t)
	ctx := context.Background()
	teamsTable := pgx.Identifier{schema, "teams"}.Sanitize()
	membersTable := pgx.Identifier{schema, "team_members"}.Sanitize()
	var ownTeamID int64
	_, err := pool.Exec(ctx, `INSERT INTO `+teamsTable+` (name, slug, created_at)
		VALUES ('Hidden first', 'hidden-first', '2026-01-01'), ('Hidden last', 'hidden-last', '2026-01-03')`)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO `+teamsTable+` (name, slug, created_at)
		VALUES ('Own team', 'own-team', '2026-01-02') RETURNING id`).Scan(&ownTeamID))
	_, err = pool.Exec(ctx, `INSERT INTO `+membersTable+` (team_id, user_public_id, role)
		VALUES ($1, $2, 'viewer')`, ownTeamID, userID)
	require.NoError(t, err)

	list := func(previous bool, limit int32) []string {
		t.Helper()
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = tx.Exec(ctx, `SET LOCAL ROLE synodus_runtime`)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `SELECT set_config('search_path', $1, true),
			set_config('app.user_id', $2, true), set_config('app.team_id', '', true)`, schema, userID.String())
		require.NoError(t, err)
		queries := tenantdb.New(tx)
		var rows []tenantdb.Team
		if previous {
			rows, err = queries.GetTeamsBefore(ctx, tenantdb.GetTeamsBeforeParams{
				BeforeCreatedAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), BeforePublicID: uuid.Nil, Limit: limit,
			})
		} else {
			rows, err = queries.GetTeamsAfter(ctx, tenantdb.GetTeamsAfterParams{
				AfterCreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), AfterPublicID: uuid.Nil, Limit: limit,
			})
		}
		require.NoError(t, err)
		names := make([]string, 0, len(rows))
		for _, row := range rows {
			names = append(names, row.Name)
		}
		return names
	}

	assert.Equal(t, []string{"Own team"}, list(false, 1))
	assert.Equal(t, []string{"Own team"}, list(true, 1))
	assert.Equal(t, []string{"Own team"}, list(false, 100))

	_, err = pool.Exec(ctx, `DELETE FROM `+membersTable+` WHERE team_id = $1 AND user_public_id = $2`, ownTeamID, userID)
	require.NoError(t, err)
	assert.Empty(t, list(false, 100))

	for _, role := range []string{"administrator", "owner"} {
		_, err = pool.Exec(ctx, `UPDATE public.org_user SET role = $1
			WHERE user_id = (SELECT id FROM public.users WHERE user_id = $2)`, role, userID)
		require.NoError(t, err)
		assert.Equal(t, []string{"Hidden first", "Own team", "Hidden last"}, list(false, 100), role)
		assert.Equal(t, []string{"Hidden last", "Own team", "Hidden first"}, list(true, 100), role)
	}
}
