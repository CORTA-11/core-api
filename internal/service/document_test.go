package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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
	application := NewDocumentApplication(authorizer, []byte("test-document-ticket-secret-value-123"))

	_, err := application.Create(context.Background(), principal, organizationID, teamID, "Notes")
	assert.ErrorIs(t, err, denied)
	assert.Equal(t, authorization.PermissionDocumentCreate, authorizer.permission)

	_, err = application.List(context.Background(), principal, organizationID, teamID)
	assert.ErrorIs(t, err, denied)
	assert.Equal(t, authorization.PermissionDocumentRead, authorizer.permission)

	_, err = application.IssueSocketTicket(context.Background(), principal, organizationID, teamID, uuid.New())
	assert.ErrorIs(t, err, denied)
	assert.Equal(t, authorization.PermissionRealtimeConnect, authorizer.permission)
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
