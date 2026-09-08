package service

import (
	"context"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestKeyAccessRequestService_Create(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	queries := tenantdb.New(mockPool)
	auth := new(mockAuthorizer)
	auth.queries = queries
	svc := NewKeyAccessApplication(auth)

	p := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	orgID, teamID := uuid.New(), uuid.New()
	requestID := uuid.New()
	now := time.Now()

	auth.On("WithinTeam", mock.Anything, p, orgID, teamID, authorization.PermissionFileRead, mock.Anything).
		Return(nil).
		Once()

	mockPool.ExpectQuery(`(?s)CreateKeyAccessRequest.*INSERT INTO team_key_access_requests`).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "public_id", "team_id", "requested_by", "status", "created_at", "decided_by", "decided_at", "team_public_id", "team_name", "requested_by_name",
		}).AddRow(int64(1), requestID, int64(7), p.UserID, "pending", now, pgtype.UUID{}, pgtype.Timestamptz{}, teamID, "Research Lab", "Researcher"))

	view, err := svc.CreateKeyAccessRequest(context.Background(), p, orgID, teamID)
	require.NoError(t, err)
	require.Equal(t, requestID, view.ID)
	assertEqualKeyAccessView(t, view, KeyAccessRequestView{
		ID:              requestID,
		TeamID:          teamID,
		TeamName:        "Research Lab",
		RequestedBy:     p.UserID,
		RequestedByName: "Researcher",
		Status:          "pending",
		CreatedAt:       now,
	})
	require.NoError(t, mockPool.ExpectationsWereMet())
}

func TestKeyAccessRequestService_List(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	queries := tenantdb.New(mockPool)
	auth := new(mockAuthorizer)
	auth.queries = queries
	svc := NewKeyAccessApplication(auth)

	p := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	orgID, teamID := uuid.New(), uuid.New()
	requester := uuid.New()
	now := time.Now()

	auth.On("WithinTeam", mock.Anything, p, orgID, teamID, authorization.PermissionFileRead, mock.Anything).
		Return(nil).
		Once()

	mockPool.ExpectQuery(`(?s)ListKeyAccessRequests.*JOIN teams`).
		WithArgs(maximumListResults).
		WillReturnRows(pgxmock.NewRows([]string{
			"public_id", "requested_by", "requested_by_name", "status", "created_at", "decided_at", "decided_by", "decided_by_name", "team_public_id", "team_name",
		}).AddRow(requester, requester, "New Member", "pending", now, pgtype.Timestamptz{}, pgtype.UUID{}, "", teamID, "Research Lab"))

	views, err := svc.ListKeyAccessRequests(context.Background(), p, orgID, teamID)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, requester, views[0].ID)
	assert.Equal(t, "pending", views[0].Status)
	require.NoError(t, mockPool.ExpectationsWereMet())
}

func TestKeyAccessRequestService_Decide(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	queries := tenantdb.New(mockPool)
	auth := new(mockAuthorizer)
	auth.queries = queries
	svc := NewKeyAccessApplication(auth)

	p := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	orgID, teamID := uuid.New(), uuid.New()
	requester := uuid.New()
	requestID := uuid.New()
	now := time.Now()
	decidedAt := now.Add(time.Minute)

	t.Run("granted", func(t *testing.T) {
		auth.On("WithinTeam", mock.Anything, p, orgID, teamID, authorization.PermissionKeyAccessDecide, mock.Anything).
			Return(nil).
			Once()

		mockPool.ExpectQuery(`(?s)GetKeyAccessRequest.*SELECT`).
			WithArgs(requestID).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "public_id", "team_id", "requested_by", "status", "created_at", "decided_by", "decided_at", "team_public_id", "team_name", "requested_by_name",
			}).AddRow(int64(1), requestID, int64(7), requester, "pending", now, pgtype.UUID{}, pgtype.Timestamptz{}, teamID, "Research Lab", "New Member"))

		decidedBy := p.UserID
		mockPool.ExpectQuery(`(?s)DecideKeyAccessRequest.*UPDATE team_key_access_requests`).
			WithArgs(keyAccessStatusGranted, requestID).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "public_id", "team_id", "requested_by", "status", "created_at", "decided_by", "decided_at", "team_public_id", "team_name", "requested_by_name", "decided_by_name",
			}).AddRow(int64(1), requestID, int64(7), requester, "granted", now, pgtype.UUID{Bytes: mustUUIDBytes(decidedBy), Valid: true}, pgtype.Timestamptz{Time: decidedAt, Valid: true}, teamID, "Research Lab", "New Member", "Team Leader"))

		view, err := svc.DecideKeyAccessRequest(context.Background(), p, orgID, teamID, requestID, keyAccessStatusGranted)
		require.NoError(t, err)
		require.NotNil(t, view.DecidedBy)
		assert.Equal(t, decidedBy, *view.DecidedBy)
		assert.Equal(t, "granted", view.Status)
		assert.Equal(t, "Team Leader", view.DecidedByName)
		require.NotNil(t, view.DecidedAt)
		require.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("invalid status rejected", func(t *testing.T) {
		_, err := svc.DecideKeyAccessRequest(context.Background(), p, orgID, teamID, requestID, "maybe")
		require.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("missing request is not found", func(t *testing.T) {
		resetAuth := new(mockAuthorizer)
		resetAuth.queries = tenantdb.New(mockPool)
		svc := NewKeyAccessApplication(resetAuth)
		resetAuth.On("WithinTeam", mock.Anything, p, orgID, teamID, authorization.PermissionKeyAccessDecide, mock.Anything).
			Return(nil).
			Once()
		mockPool.ExpectQuery(`(?s)GetKeyAccessRequest.*SELECT`).
			WithArgs(requestID).
			WillReturnError(pgx.ErrNoRows)

		_, err := svc.DecideKeyAccessRequest(context.Background(), p, orgID, teamID, requestID, keyAccessStatusDenied)
		require.ErrorIs(t, err, authorization.ErrResourceNotFound)
		require.NoError(t, mockPool.ExpectationsWereMet())
	})
}

func assertEqualKeyAccessView(t *testing.T, actual, expected KeyAccessRequestView) {
	t.Helper()
	require.Equal(t, expected.ID, actual.ID)
	require.Equal(t, expected.TeamID, actual.TeamID)
	require.Equal(t, expected.TeamName, actual.TeamName)
	require.Equal(t, expected.RequestedBy, actual.RequestedBy)
	require.Equal(t, expected.RequestedByName, actual.RequestedByName)
	require.Equal(t, expected.Status, actual.Status)
	require.Equal(t, expected.CreatedAt.UTC(), actual.CreatedAt.UTC())
	require.Equal(t, expected.DecidedBy, actual.DecidedBy)
	require.Equal(t, expected.DecidedAt, actual.DecidedAt)
	require.Equal(t, expected.DecidedByName, actual.DecidedByName)
}

func mustUUIDBytes(id uuid.UUID) [16]byte {
	return id
}
