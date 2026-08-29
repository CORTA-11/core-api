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

type mockAuthorizer struct {
	mock.Mock
	queries *tenantdb.Queries
}

func (m *mockAuthorizer) WithinOrganization(
	ctx context.Context,
	p session.Principal,
	orgID uuid.UUID,
	perm authorization.Permission,
	cb authorization.TenantCallback,
) error {
	args := m.Called(ctx, p, orgID, perm, cb)
	if cb != nil && args.Error(0) == nil {
		return cb(m.queries)
	}
	return args.Error(0)
}

func (m *mockAuthorizer) WithinTeam(
	ctx context.Context,
	p session.Principal,
	orgID uuid.UUID,
	teamID uuid.UUID,
	perm authorization.Permission,
	cb authorization.TenantCallback,
) error {
	args := m.Called(ctx, p, orgID, teamID, perm, cb)
	if cb != nil && args.Error(0) == nil {
		return cb(m.queries)
	}
	return args.Error(0)
}

func TestKeyService_UpsertPublicKey(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	auth := new(mockAuthorizer)
	svc := NewKeyService(mockPool, auth)

	p := session.Principal{
		UserID:    uuid.New(),
		SessionID: uuid.New(),
	}
	pubKey := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

	t.Run("success", func(t *testing.T) {
		mockPool.ExpectBegin()
		mockPool.ExpectQuery(`(?s)UpsertUserPublicKey.*INSERT`).
			WithArgs(p.UserID, pubKey).
			WillReturnRows(pgxmock.NewRows([]string{"user_id", "public_key", "created_at"}).
				AddRow(p.UserID, pubKey, time.Now()))
		mockPool.ExpectCommit()

		res, err := svc.UpsertPublicKey(context.Background(), p, pubKey)
		require.NoError(t, err)
		assert.Equal(t, pubKey, res.PublicKey)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("unauthenticated", func(t *testing.T) {
		_, err := svc.UpsertPublicKey(context.Background(), session.Principal{}, pubKey)
		assert.ErrorIs(t, err, authorization.ErrUnauthenticated)
	})

	t.Run("invalid input", func(t *testing.T) {
		_, err := svc.UpsertPublicKey(context.Background(), p, "short")
		assert.ErrorIs(t, err, ErrInvalidInput)
	})
}
