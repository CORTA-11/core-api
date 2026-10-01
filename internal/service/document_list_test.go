package service

import (
	"context"
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

func TestDocumentListIncludesLastEditorDisplayName(t *testing.T) {
	for _, name := range []string{"Alex Smith", ""} {
		t.Run("editor name="+name, func(t *testing.T) {
			pool, err := pgxmock.NewPool()
			require.NoError(t, err)
			t.Cleanup(pool.Close)
			principal := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
			organizationID, teamID, documentID, editorID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
			pool.ExpectQuery("FROM teams").WithArgs(teamID, principal.UserID).
				WillReturnRows(pgxmock.NewRows([]string{"id", "public_id"}).AddRow(int64(1), teamID))
			pool.ExpectQuery("coalesce\\(synodus_user_display_name\\(last_updated_by\\), ''\\)::text AS updated_by_name").
				WithArgs(int64(1), int32(maximumListResults)).
				WillReturnRows(pgxmock.NewRows([]string{
					"id", "public_id", "team_id", "title", "last_updated_by", "created_at", "updated_at", "updated_by_name",
				}).AddRow(int64(2), documentID, int64(1), "Notes", editorID, time.Time{}, time.Time{}, name))
			authorizer := new(mockAuthorizer)
			authorizer.queries = tenantdb.New(pool)
			authorizer.On("WithinTeam", mock.Anything, principal, organizationID, teamID,
				authorization.PermissionDocumentRead, mock.Anything).Return(nil)
			application := NewDocumentApplication(authorizer, nil, noopDocumentRoomCloser{})

			documents, err := application.List(context.Background(), principal, organizationID, teamID)

			require.NoError(t, err)
			require.Len(t, documents, 1)
			assert.Equal(t, name, documents[0].UpdatedByName)
			assert.Equal(t, editorID, documents[0].UpdatedBy)
			assert.Equal(t, teamID, documents[0].TeamID)
			require.NoError(t, pool.ExpectationsWereMet())
		})
	}
}
