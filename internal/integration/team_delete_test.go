//go:build isolation

package integration_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestTeamDeletionPermissionsAndRetention(t *testing.T) {
	f := newTenantBoundaryFixture(t)
	ctx := context.Background()
	org := f.orgs[0]
	team := org.teams[0]
	p := session.Principal{UserID: f.users.shared, SessionID: uuid.New()}
	codec, err := pagination.NewCodec(pagination.CodecConfig{Active: pagination.Key{
		ID: "team-delete-test", Secret: bytes.Repeat([]byte{0x71}, 32),
	}})
	require.NoError(t, err)
	app := service.NewTeamTaskApplication(authorization.NewAuthorizer(f.resolver, f.executor), codec)
	require.Error(t, app.DeleteTeam(ctx, p, org.publicID, team.publicID), "ordinary members cannot delete")
	members := pgx.Identifier{org.schema, "team_members"}.Sanitize()
	_, err = f.adminPool.Exec(ctx, "UPDATE "+members+" SET role = 'team_admin' WHERE team_id = $1 AND user_public_id = $2", team.id, p.UserID)
	require.NoError(t, err)
	require.Error(t, app.DeleteTeam(ctx, p, f.orgs[1].publicID, team.publicID), "team IDs cannot cross organizations")
	require.NoError(t, app.DeleteTeam(ctx, p, org.publicID, team.publicID))
	var retained bool
	require.NoError(t, f.adminPool.QueryRow(ctx, "SELECT deleted_at IS NOT NULL FROM "+pgx.Identifier{org.schema, "teams"}.Sanitize()+" WHERE id = $1", team.id).Scan(&retained))
	require.True(t, retained)
	page, err := app.ListTeams(ctx, p, org.publicID, pagination.Parameters{PageSize: 100})
	require.NoError(t, err)
	for _, item := range page.Items {
		require.NotEqual(t, team.publicID, item.ID, "deleted team is omitted from lists")
	}
	var taskCount int
	require.NoError(t, f.adminPool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{org.schema, "tasks"}.Sanitize()+" WHERE team_id = $1", team.id).Scan(&taskCount))
	require.Positive(t, taskCount, "team content is retained")
	_, err = f.resolver.ResolveTeam(ctx, f.resolveOrganization(t, p.UserID, org), team.publicID)
	require.Error(t, err, "deleted team is unavailable")
	require.Error(t, app.DeleteTeam(ctx, p, org.publicID, team.publicID))

	// Organization administrators can delete without membership, but not read content.
	other := org.teams[1]
	_, err = f.adminPool.Exec(ctx, "DELETE FROM "+members+" WHERE team_id = $1 AND user_public_id = $2", other.id, p.UserID)
	require.NoError(t, err)
	_, err = f.adminPool.Exec(ctx, `UPDATE public.org_user SET role = 'administrator' WHERE org_id = $1 AND user_id = (SELECT id FROM public.users WHERE user_id = $2)`, org.id, p.UserID)
	require.NoError(t, err)
	_, err = app.ListTasks(ctx, p, org.publicID, other.publicID, pagination.Parameters{PageSize: 10})
	require.Error(t, err)
	require.NoError(t, app.DeleteTeam(ctx, p, org.publicID, other.publicID))
}
