package authorization

import (
	"context"
	"testing"

	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAuthorizerDeniesMissingPrincipalAndUnknownInputs(t *testing.T) {
	t.Parallel()
	authorizer := NewAuthorizer(nil, nil)
	callback := func(*tenantdb.Queries) error { return nil }
	assert.ErrorIs(t, authorizer.WithinOrganization(context.Background(), session.Principal{}, uuid.New(), PermissionOrgRead, callback), ErrUnauthenticated)
	assert.ErrorIs(t, authorizer.WithinTeam(context.Background(), session.Principal{}, uuid.New(), uuid.New(), PermissionTaskRead, callback), ErrUnauthenticated)

	principal := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	assert.ErrorIs(t, authorizer.WithinOrganization(context.Background(), principal, uuid.New(), "unknown", callback), ErrOperationDenied)
	assert.ErrorIs(t, authorizer.WithinTeam(context.Background(), principal, uuid.New(), uuid.New(), "unknown", callback), ErrOperationDenied)
}
