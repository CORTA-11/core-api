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

func TestTeamDescriptionPersistsAcrossCreationAndUpdates(t *testing.T) {
	f := newTenantBoundaryFixture(t)
	ctx := context.Background()
	org := f.orgs[0]
	p := session.Principal{UserID: f.users.shared, SessionID: uuid.New()}
	_, err := f.adminPool.Exec(ctx, `UPDATE public.org_user SET role = 'owner' WHERE org_id = $1 AND user_id = (SELECT id FROM public.users WHERE user_id = $2)`, org.id, p.UserID)
	require.NoError(t, err)
	codec, err := pagination.NewCodec(pagination.CodecConfig{Active: pagination.Key{
		ID: "team-description-test", Secret: bytes.Repeat([]byte{0x72}, 32),
	}})
	require.NoError(t, err)
	app := service.NewTeamTaskApplication(authorization.NewAuthorizer(f.resolver, f.executor), codec)

	team, err := app.CreateTeam(ctx, p, org.publicID, "Research", "alpha@tenant-boundary.example.test", "  Persistent description  ")
	require.NoError(t, err)
	require.Equal(t, "Persistent description", team.Description)

	assertPersisted := func(expected string) {
		t.Helper()
		var stored string
		require.NoError(t, f.adminPool.QueryRow(ctx, "SELECT description FROM "+pgx.Identifier{org.schema, "teams"}.Sanitize()+" WHERE public_id = $1", team.ID).Scan(&stored))
		require.Equal(t, expected, stored)
		page, listErr := app.ListTeams(ctx, p, org.publicID, pagination.Parameters{PageSize: 100})
		require.NoError(t, listErr)
		for _, item := range page.Items {
			if item.ID == team.ID {
				require.Equal(t, expected, item.Description)
				return
			}
		}
		t.Fatal("created team missing from fresh list")
	}
	assertPersisted("Persistent description")

	leader := session.Principal{UserID: f.users.alpha, SessionID: uuid.New()}
	description := "Updated description"
	updated, err := app.UpdateTeam(ctx, leader, org.publicID, team.ID, nil, &description)
	require.NoError(t, err)
	require.Equal(t, description, updated.Description)
	assertPersisted(description)

	name := "Renamed Research"
	_, err = app.UpdateTeam(ctx, leader, org.publicID, team.ID, &name, nil)
	require.NoError(t, err)
	assertPersisted(description)

	description = ""
	_, err = app.UpdateTeam(ctx, leader, org.publicID, team.ID, nil, &description)
	require.NoError(t, err)
	assertPersisted("")

	withoutDescription, err := app.CreateTeam(ctx, p, org.publicID, "No description", "alpha@tenant-boundary.example.test", "")
	require.NoError(t, err)
	require.Empty(t, withoutDescription.Description)
}
