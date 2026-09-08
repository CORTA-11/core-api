package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNormalizeDocumentTitleAcceptsTrimmedTitle(t *testing.T) {
	t.Parallel()

	title, err := normalizeDocumentTitle("  Experiment notes  ")
	require.NoError(t, err)
	assert.Equal(t, "Experiment notes", title)
}

func TestDocumentApplicationUsesDocumentPermissions(t *testing.T) {
	t.Parallel()
	principal := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	organizationID, teamID := uuid.New(), uuid.New()
	denied := errors.New("stop after permission selection")
	authorizer := &documentAuthorizer{err: denied}
	application := NewDocumentApplication(authorizer, []byte("test-document-ticket-secret-value-123"), noopDocumentRoomCloser{})

	_, err := application.Create(context.Background(), principal, organizationID, teamID, "Notes")
	assert.ErrorIs(t, err, denied)
	assert.Equal(t, authorization.PermissionDocumentCreate, authorizer.permission)

	_, err = application.List(context.Background(), principal, organizationID, teamID)
	assert.ErrorIs(t, err, denied)
	assert.Equal(t, authorization.PermissionDocumentRead, authorizer.permission)

	title := "Updated notes"
	_, err = application.Update(context.Background(), principal, organizationID, teamID, uuid.New(), DocumentPatch{Title: &title})
	assert.ErrorIs(t, err, denied)
	assert.Equal(t, authorization.PermissionDocumentUpdate, authorizer.permission)

	err = application.Delete(context.Background(), principal, organizationID, teamID, uuid.New())
	assert.ErrorIs(t, err, denied)
	assert.Equal(t, authorization.PermissionDocumentDelete, authorizer.permission)

	_, err = application.IssueSocketTicket(context.Background(), principal, "Authenticated Editor", organizationID, teamID, uuid.New())
	assert.ErrorIs(t, err, denied)
	assert.Equal(t, authorization.PermissionRealtimeConnect, authorizer.permission)

	_, err = application.LoadState(context.Background(), principal.UserID, organizationID, teamID, uuid.New())
	assert.ErrorIs(t, err, denied)
	assert.Equal(t, authorization.PermissionDocumentRead, authorizer.permission)

	_, err = application.StoreState(context.Background(), principal.UserID, organizationID, teamID, uuid.New(), DocumentStateWrite{
		CanonicalState: []byte{1}, Title: "Converged notes", BodyHTML: "<p>Converged</p>",
	})
	assert.ErrorIs(t, err, denied)
	assert.Equal(t, authorization.PermissionDocumentUpdate, authorizer.permission)
}

type documentAuthorizer struct {
	permission authorization.Permission
	err        error
}

func (authorizer *documentAuthorizer) WithinOrganization(context.Context, session.Principal, uuid.UUID, authorization.Permission, authorization.TenantCallback) error {
	return authorizer.err
}

func (authorizer *documentAuthorizer) WithinTeam(_ context.Context, _ session.Principal, _ uuid.UUID, _ uuid.UUID, permission authorization.Permission, _ authorization.TenantCallback) error {
	authorizer.permission = permission
	return authorizer.err
}

var _ applicationAuthorizer = (*documentAuthorizer)(nil)

func TestNormalizeDocumentTitleRejectsEmptyAndOversizedTitles(t *testing.T) {
	t.Parallel()

	for _, title := range []string{"", "   ", strings.Repeat("a", 256)} {
		_, err := normalizeDocumentTitle(title)
		assert.ErrorIs(t, err, ErrInvalidInput)
	}
	_, err := normalizeDocumentTitle(strings.Repeat("界", 256))
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestStoreDocumentStateRejectsMissingCanonicalStateBeforeAuthorization(t *testing.T) {
	t.Parallel()
	principal := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	authorizer := &documentAuthorizer{err: errors.New("authorizer should not be called")}
	application := NewDocumentApplication(authorizer, nil, noopDocumentRoomCloser{})

	_, err := application.StoreState(context.Background(), principal.UserID, uuid.New(), uuid.New(), uuid.New(), DocumentStateWrite{Title: "Notes"})

	assert.ErrorIs(t, err, ErrInvalidInput)
	assert.Empty(t, authorizer.permission)
}

func TestDeletingDocumentClosesItsCollaborationRoom(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mockPool.Close)
	principal := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	organizationID, teamID, documentID := uuid.New(), uuid.New(), uuid.New()
	mockPool.ExpectExec("DELETE FROM documents").WithArgs(documentID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	authorizer := new(mockAuthorizer)
	authorizer.queries = tenantdb.New(mockPool)
	authorizer.On(
		"WithinTeam", mock.Anything, principal, organizationID, teamID,
		authorization.PermissionDocumentDelete, mock.Anything,
	).Return(nil)
	closer := &recordingDocumentRoomCloser{}
	application := NewDocumentApplication(authorizer, nil, closer)

	require.NoError(t, application.Delete(context.Background(), principal, organizationID, teamID, documentID))

	assert.Equal(t, []uuid.UUID{organizationID, teamID, documentID}, closer.scope)
	require.NoError(t, mockPool.ExpectationsWereMet())
}

func TestDocumentDeletionCanRetryWhenClosingItsRoomFails(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mockPool.Close)
	principal := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	organizationID, teamID, documentID := uuid.New(), uuid.New(), uuid.New()
	authorizer := new(mockAuthorizer)
	authorizer.queries = tenantdb.New(mockPool)
	authorizer.On(
		"WithinTeam", mock.Anything, principal, organizationID, teamID,
		authorization.PermissionDocumentDelete, mock.Anything,
	).Return(nil).Twice()
	closer := &recordingDocumentRoomCloser{errors: []error{errors.New("temporarily unavailable"), nil}}
	application := NewDocumentApplication(authorizer, nil, closer)

	err = application.Delete(context.Background(), principal, organizationID, teamID, documentID)
	assert.ErrorContains(t, err, "temporarily unavailable")
	mockPool.ExpectExec("DELETE FROM documents").WithArgs(documentID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	require.NoError(t, application.Delete(context.Background(), principal, organizationID, teamID, documentID))

	assert.Equal(t, 2, closer.calls)
	require.NoError(t, mockPool.ExpectationsWereMet())
}

type noopDocumentRoomCloser struct{}

func (noopDocumentRoomCloser) CloseDocumentRoom(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

type recordingDocumentRoomCloser struct {
	calls  int
	errors []error
	scope  []uuid.UUID
}

func (closer *recordingDocumentRoomCloser) CloseDocumentRoom(
	_ context.Context, organizationID, teamID, documentID uuid.UUID,
) error {
	closer.calls++
	closer.scope = []uuid.UUID{organizationID, teamID, documentID}
	if len(closer.errors) >= closer.calls {
		return closer.errors[closer.calls-1]
	}
	return nil
}
