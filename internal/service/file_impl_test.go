package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFileService_UploadValidation(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	auth := new(mockAuthorizer)
	svc := NewFileService(nil, "bucket", auth)

	p := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	orgID := uuid.New()
	teamID := uuid.New()

	t.Run("empty name", func(t *testing.T) {
		_, err := svc.UploadFile(context.Background(), p, orgID, teamID, "", "text/plain", strings.NewReader("data"), 4, []byte("iv-bytes"), 1)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("invalid size", func(t *testing.T) {
		_, err := svc.UploadFile(context.Background(), p, orgID, teamID, "file.txt", "text/plain", strings.NewReader("data"), 0, []byte("iv-bytes"), 1)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("oversized file", func(t *testing.T) {
		_, err := svc.UploadFile(context.Background(), p, orgID, teamID, "file.txt", "text/plain", strings.NewReader("data"), MaxFileUploadBytes+1, []byte("iv-bytes"), 1)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("missing iv", func(t *testing.T) {
		_, err := svc.UploadFile(context.Background(), p, orgID, teamID, "file.txt", "text/plain", strings.NewReader("data"), 4, nil, 1)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})
}

func TestFileService_ListFiles(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	auth := new(mockAuthorizer)
	auth.queries = tenantdb.New(mockPool)
	svc := NewFileService(nil, "bucket", auth)

	p := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	orgID := uuid.New()
	teamID := uuid.New()
	internalTeamID := int64(10)
	now := time.Now()

	t.Run("success", func(t *testing.T) {
		auth.On("WithinTeam", mock.Anything, p, orgID, teamID, authorization.PermissionFileRead, mock.Anything).
			Return(nil).Once()

		mockPool.ExpectQuery(`(?s)ResolveTeamContext.*SELECT`).
			WithArgs(teamID, p.UserID).
			WillReturnRows(pgxmock.NewRows([]string{"id", "public_id"}).AddRow(internalTeamID, teamID))

		mockPool.ExpectQuery(`(?s)ListFilesForTeam.*SELECT`).
			WithArgs(internalTeamID, maximumListResults).
			WillReturnRows(pgxmock.NewRows([]string{
				"id", "public_id", "team_id", "name", "size", "content_type", "object_key", "iv", "key_version", "uploaded_by", "created_at", "updated_at", "deleted_at",
			}).AddRow(int64(1), uuid.New(), internalTeamID, "doc.pdf", int64(1024), "application/pdf", "key", []byte("iv"), int32(1), p.UserID, now, now, nil))

		files, err := svc.ListFiles(context.Background(), p, orgID, teamID)
		require.NoError(t, err)
		require.Len(t, files, 1)
		assert.Equal(t, "doc.pdf", files[0].Name)
		assert.Equal(t, int64(1024), files[0].Size)
		auth.AssertExpectations(t)
	})
}
