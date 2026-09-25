//go:build isolation

package integration_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestTeamAISettingsIsolationAndAdminWrites(t *testing.T) {
	pool, schema, userID := rlsRuntimeFixture(t)
	ctx := context.Background()
	var teamID, otherID int64
	require.NoError(t, pool.QueryRow(ctx, "INSERT INTO "+pgx.Identifier{schema, "teams"}.Sanitize()+" (name, slug) VALUES ('AI team', 'ai-team') RETURNING id").Scan(&teamID))
	require.NoError(t, pool.QueryRow(ctx, "INSERT INTO "+pgx.Identifier{schema, "teams"}.Sanitize()+" (name, slug) VALUES ('Other', 'other') RETURNING id").Scan(&otherID))
	_, err := pool.Exec(ctx, "INSERT INTO "+pgx.Identifier{schema, "team_members"}.Sanitize()+" (team_id, user_public_id, role) VALUES ($1, $2, 'team_admin')", teamID, userID)
	require.NoError(t, err)
	runtime := func(bound int64, fn func(*tenantdb.Queries) error) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err = tx.Exec(ctx, "SET LOCAL ROLE synodus_runtime"); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "SELECT set_config('search_path',$1,true), set_config('app.user_id',$2,true), set_config('app.team_id',$3,true)", schema, userID.String(), strconv.FormatInt(bound, 10)); err != nil {
			return err
		}
		if err = fn(tenantdb.New(tx)); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	save := func(q *tenantdb.Queries) error {
		_, err := q.SaveTeamAISettings(ctx, tenantdb.SaveTeamAISettingsParams{EndpointUrl: "https://provider.example", Model: "model", ApiToken: "test-token"})
		return err
	}
	require.NoError(t, runtime(teamID, save))
	require.Error(t, runtime(otherID, save))
	require.NoError(t, runtime(otherID, func(q *tenantdb.Queries) error {
		_, err := q.GetTeamAISettings(ctx)
		require.ErrorIs(t, err, pgx.ErrNoRows)
		return nil
	}))
	_, err = pool.Exec(ctx, "UPDATE "+pgx.Identifier{schema, "team_members"}.Sanitize()+" SET role='viewer' WHERE team_id=$1 AND user_public_id=$2", teamID, userID)
	require.NoError(t, err)
	require.Error(t, runtime(teamID, save))
	require.NoError(t, runtime(teamID, func(q *tenantdb.Queries) error {
		row, err := q.GetTeamAISettings(ctx)
		require.NoError(t, err)
		require.Equal(t, "test-token", row.ApiToken)
		return nil
	}))
	// The query includes range boundaries and excludes another team's messages.
	from := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	for _, row := range []struct {
		team int64
		at   time.Time
		text string
	}{
		{teamID, from.Add(-time.Second), "before"}, {teamID, from, "from"},
		{teamID, from.Add(time.Hour), "to"}, {teamID, from.Add(2 * time.Hour), "after"},
		{otherID, from, "other"},
	} {
		_, err = pool.Exec(ctx, "INSERT INTO "+pgx.Identifier{schema, "chat_messages"}.Sanitize()+" (team_id,sender_user_public_id,message,created_at) VALUES ($1,$2,$3,$4)", row.team, userID, row.text, row.at)
		require.NoError(t, err)
	}
	require.NoError(t, runtime(teamID, func(q *tenantdb.Queries) error {
		rows, err := q.ListAIChatMessages(ctx, tenantdb.ListAIChatMessagesParams{FromTime: from, ToTime: from.Add(time.Hour), Limit: 101})
		require.NoError(t, err)
		require.Len(t, rows, 2)
		require.Equal(t, "from", rows[0].Message)
		require.Equal(t, "to", rows[1].Message)
		return nil
	}))
}
