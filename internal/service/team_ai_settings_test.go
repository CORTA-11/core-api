package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTeamAISettingsRequireTeamAdmin(t *testing.T) {
	for _, role := range []authorization.TeamRole{authorization.TeamRoleResearchLead, authorization.TeamRoleResearcher, authorization.TeamRoleContributor, authorization.TeamRoleViewer} {
		require.False(t, authorization.TeamAllows(role, authorization.PermissionTeamUpdate))
	}
	require.True(t, authorization.TeamAllows(authorization.TeamRoleAdmin, authorization.PermissionTeamUpdate))
	auth := new(mockAuthorizer)
	p := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	orgID, teamID := uuid.New(), uuid.New()
	auth.On("WithinTeam", mock.Anything, p, orgID, teamID, authorization.PermissionTeamUpdate, mock.Anything).Return(authorization.ErrOperationDenied).Twice()
	app := NewAIApplication(auth, nil)
	_, err := app.GetSettings(context.Background(), p, orgID, teamID)
	require.ErrorIs(t, err, authorization.ErrOperationDenied)
	_, err = app.SaveSettings(context.Background(), p, orgID, teamID, TeamAISettingsInput{})
	require.ErrorIs(t, err, authorization.ErrOperationDenied)
	auth.AssertExpectations(t)
}

func TestTeamAISettingsPreserveAndNeverReturnToken(t *testing.T) {
	pool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer pool.Close()
	auth := new(mockAuthorizer)
	auth.queries = tenantdb.New(pool)
	auth.On("WithinTeam", mock.Anything, mock.Anything, mock.Anything, mock.Anything, authorization.PermissionTeamUpdate, mock.Anything).Return(nil)
	app := NewAIApplication(auth, nil)
	columns := []string{"endpoint_url", "model", "api_token"}
	pool.ExpectQuery("GetTeamAISettings").WillReturnRows(pgxmock.NewRows(columns).AddRow("https://provider.example", "old-model", "saved-secret"))
	pool.ExpectQuery("SaveTeamAISettings").WithArgs("https://provider.example", "new-model", "").WillReturnRows(pgxmock.NewRows(columns).AddRow("https://provider.example", "new-model", "saved-secret"))
	view, err := app.SaveSettings(context.Background(), session.Principal{}, uuid.New(), uuid.New(), TeamAISettingsInput{EndpointURL: "https://provider.example", Model: "new-model"})
	require.NoError(t, err)
	require.True(t, view.HasAPIToken)
	encoded, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "saved-secret")
	pool.ExpectQuery("GetTeamAISettings").WillReturnError(pgx.ErrNoRows)
	_, err = app.SaveSettings(context.Background(), session.Principal{}, uuid.New(), uuid.New(), TeamAISettingsInput{EndpointURL: "https://provider.example", Model: "new-model"})
	require.ErrorIs(t, err, ErrInvalidInput)
	require.NoError(t, pool.ExpectationsWereMet())
}

type capturingAIClient struct{ request AIProcessRequest }

func (c *capturingAIClient) Process(_ context.Context, request AIProcessRequest) (AIProcessResult, error) {
	c.request = request
	return AIProcessResult{}, nil
}

func TestAIProcessUsesSavedTeamSettingsAndDateRange(t *testing.T) {
	pool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer pool.Close()
	auth := new(mockAuthorizer)
	auth.queries = tenantdb.New(pool)
	p := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	orgID, teamID, messageID := uuid.New(), uuid.New(), uuid.New()
	auth.On("WithinTeam", mock.Anything, p, orgID, teamID, authorization.PermissionRealtimeConnect, mock.Anything).Return(nil).Once()
	client := &capturingAIClient{}
	app := NewAIApplication(auth, client)
	from := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	pool.ExpectQuery("GetTeamAISettings").WillReturnRows(pgxmock.NewRows([]string{"endpoint_url", "model", "api_token"}).AddRow("https://provider.example", "team-model", "secret"))
	pool.ExpectQuery("FROM list_bound_team_members").WithArgs(int32(10000)).WillReturnRows(pgxmock.NewRows([]string{"user_public_id", "display_name", "email", "role", "created_at"}))
	pool.ExpectQuery("ListAIChatMessages").WithArgs(from, to, int32(101)).WillReturnRows(pgxmock.NewRows([]string{"public_id", "sender_user_public_id", "message", "created_at"}).AddRow(messageID, p.UserID, "Please review the report.", from))
	_, err = app.Process(context.Background(), p, orgID, teamID, AIProcessInput{From: from, To: to})
	require.NoError(t, err)
	require.Equal(t, "secret", client.request.Provider.APIToken)
	require.Equal(t, "team-model", client.request.Provider.Model)
	require.Equal(t, teamID.String(), client.request.Context.TeamPublicID)
	require.Len(t, client.request.Context.Messages, 1)
	_, err = app.Process(context.Background(), p, orgID, teamID, AIProcessInput{From: to, To: from})
	require.ErrorIs(t, err, ErrInvalidInput)
	require.NoError(t, pool.ExpectationsWereMet())
}
