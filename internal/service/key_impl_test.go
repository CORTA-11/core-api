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
	"github.com/jackc/pgx/v5/pgtype"
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

const longPubKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

func TestKeyService_UpsertUserKeys(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	auth := new(mockAuthorizer)
	svc := NewKeyService(mockPool, auth)

	p := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	now := time.Now()

	t.Run("public key only", func(t *testing.T) {
		mockPool.ExpectBegin()
		encrypted, salt := pgtype.Text{}, pgtype.Text{}
		mockPool.ExpectQuery(`(?s)UpsertUserKeys.*INSERT`).
			WithArgs(p.UserID, longPubKey, encrypted, salt, pgtype.Int4{}, pbkdf2Algorithm).
			WillReturnRows(pgxmock.NewRows([]string{
				"user_id", "public_key", "encrypted_private_key", "kek_salt", "kek_iterations", "kek_algorithm", "created_at", "updated_at",
			}).AddRow(p.UserID, longPubKey, encrypted, salt, pgtype.Int4{}, pbkdf2Algorithm, now, now))
		mockPool.ExpectCommit()

		res, err := svc.UpsertUserKeys(context.Background(), p, UserKeyUpdate{PublicKey: longPubKey})
		require.NoError(t, err)
		assert.Equal(t, longPubKey, res.PublicKey)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("with private key material", func(t *testing.T) {
		mockPool.ExpectBegin()
		iterations := int32(210000)
		mockPool.ExpectQuery(`(?s)UpsertUserKeys.*INSERT`).
			WithArgs(p.UserID, longPubKey, pgtype.Text{String: "sealed-private-key-value-that-is-longer-than-sixty-four-characters-abcdefg1234", Valid: true}, pgtype.Text{String: "salt-value-longer-than-eight-characters", Valid: true}, pgtype.Int4{Int32: iterations, Valid: true}, pbkdf2Algorithm).
			WillReturnRows(pgxmock.NewRows([]string{
				"user_id", "public_key", "encrypted_private_key", "kek_salt", "kek_iterations", "kek_algorithm", "created_at", "updated_at",
			}).AddRow(p.UserID, longPubKey, pgtype.Text{String: "sealed-private-key-value-that-is-longer-than-sixty-four-characters-abcdefg1234", Valid: true}, pgtype.Text{String: "salt-value-longer-than-eight-characters", Valid: true}, pgtype.Int4{Int32: iterations, Valid: true}, pbkdf2Algorithm, now, now))
		mockPool.ExpectCommit()

		res, err := svc.UpsertUserKeys(context.Background(), p, UserKeyUpdate{
			PublicKey:           longPubKey,
			EncryptedPrivateKey: strptr("sealed-private-key-value-that-is-longer-than-sixty-four-characters-abcdefg1234"),
			KEKSalt:             strptr("salt-value-longer-than-eight-characters"),
			KEKIterations:       &iterations,
			KEKAlgorithm:        strptr(pbkdf2Algorithm),
		})
		require.NoError(t, err)
		require.NotNil(t, res.EncryptedPrivateKey)
		assert.Equal(t, "sealed-private-key-value-that-is-longer-than-sixty-four-characters-abcdefg1234", *res.EncryptedPrivateKey)
		require.NotNil(t, res.KEKIterations)
		assert.Equal(t, iterations, *res.KEKIterations)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("unauthenticated", func(t *testing.T) {
		_, err := svc.UpsertUserKeys(context.Background(), session.Principal{}, UserKeyUpdate{PublicKey: longPubKey})
		assert.ErrorIs(t, err, authorization.ErrUnauthenticated)
	})

	t.Run("invalid input short public key", func(t *testing.T) {
		_, err := svc.UpsertUserKeys(context.Background(), p, UserKeyUpdate{PublicKey: "short"})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("partial private key material rejected", func(t *testing.T) {
		_, err := svc.UpsertUserKeys(context.Background(), p, UserKeyUpdate{
			PublicKey:           longPubKey,
			EncryptedPrivateKey: strptr("sealed-private-key-value-that-is-longer-than-sixty-four-characters-abcdefg1234"),
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})
}

func TestKeyService_GetUserKeys(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	auth := new(mockAuthorizer)
	svc := NewKeyService(mockPool, auth)
	p := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	now := time.Now()

	t.Run("found", func(t *testing.T) {
		mockPool.ExpectBegin()
		mockPool.ExpectQuery(`(?s)GetUserPublicKey.*SELECT`).
			WithArgs(p.UserID).
			WillReturnRows(pgxmock.NewRows([]string{
				"user_id", "public_key", "encrypted_private_key", "kek_salt", "kek_iterations", "kek_algorithm", "created_at", "updated_at",
			}).AddRow(p.UserID, longPubKey, pgtype.Text{String: "sealed-private-key-value-that-is-longer-than-sixty-four-characters-abcdefg1234", Valid: true}, pgtype.Text{}, pgtype.Int4{}, pbkdf2Algorithm, now, now))
		mockPool.ExpectCommit()

		res, err := svc.GetUserKeys(context.Background(), p)
		require.NoError(t, err)
		assert.Equal(t, longPubKey, res.PublicKey)
	})

	t.Run("unauthenticated", func(t *testing.T) {
		_, err := svc.GetUserKeys(context.Background(), session.Principal{})
		assert.ErrorIs(t, err, authorization.ErrUnauthenticated)
	})
}

func TestKeyService_CreateTeamKey(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	queries := tenantdb.New(mockPool)
	auth := new(mockAuthorizer)
	auth.queries = queries
	svc := NewKeyService(mockPool, auth)

	p := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	orgID, teamID := uuid.New(), uuid.New()
	other := uuid.New()
	wraps := []TeamKeyWrap{{UserID: p.UserID, Key: longPubKey, Algorithm: rsaWrapAlgorithm}, {UserID: other, Key: longPubKey, Algorithm: rsaWrapAlgorithm}}

	t.Run("success creates version 1", func(t *testing.T) {
		now := time.Now()
		auth.On("WithinTeam", mock.Anything, p, orgID, teamID, authorization.PermissionFileUpload, mock.Anything).
			Return(nil).
			Once()

		wrapsJSON, err := json.Marshal(wraps)
		require.NoError(t, err)
		mockPool.ExpectQuery(`(?s)ResolveTeamContext.*SELECT`).
			WithArgs(teamID, p.UserID).
			WillReturnRows(pgxmock.NewRows([]string{"id", "public_id"}).AddRow(int64(7), teamID))
		mockPool.ExpectQuery(`(?s)CreateTeamKeyVersion.*synodus_commit_team_key`).
			WithArgs(int64(7), "aes-256-gcm", wrapsJSON, p.UserID).
			WillReturnRows(pgxmock.NewRows([]string{"id", "team_id", "version", "status", "algorithm", "wraps", "created_by", "created_at"}).
				AddRow(int64(1), int64(7), int32(1), "active", "aes-256-gcm", wrapsJSON, p.UserID, now))

		res, err := svc.CreateTeamKey(context.Background(), p, orgID, teamID, TeamKeyVersionInput{Wraps: wraps})
		require.NoError(t, err)
		assert.Equal(t, int32(1), res.Version)
		assert.Equal(t, teamID, res.TeamID)
		assert.Len(t, res.Wraps, 1)
		assert.Equal(t, p.UserID, res.Wraps[0].UserID)
		assert.Equal(t, []uuid.UUID{p.UserID, other}, res.WrappedUserIDs)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("no wraps rejected", func(t *testing.T) {
		_, err := svc.CreateTeamKey(context.Background(), p, orgID, teamID, TeamKeyVersionInput{Wraps: nil})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("caller not wrapped rejected", func(t *testing.T) {
		_, err := svc.CreateTeamKey(context.Background(), p, orgID, teamID, TeamKeyVersionInput{Wraps: []TeamKeyWrap{{UserID: other, Key: longPubKey, Algorithm: rsaWrapAlgorithm}}})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})
}

func TestKeyService_ListTeamKeys(t *testing.T) {
	mockPool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mockPool.Close()

	queries := tenantdb.New(mockPool)
	auth := new(mockAuthorizer)
	auth.queries = queries
	svc := NewKeyService(mockPool, auth)

	p := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	orgID, teamID := uuid.New(), uuid.New()
	other := uuid.New()
	now := time.Now()

	auth.On("WithinTeam", mock.Anything, p, orgID, teamID, authorization.PermissionTeamRead, mock.Anything).
		Return(nil).
		Once()

	wrapsJSON, err := json.Marshal([]TeamKeyWrap{{UserID: p.UserID, Key: longPubKey, Algorithm: rsaWrapAlgorithm}, {UserID: other, Key: longPubKey, Algorithm: rsaWrapAlgorithm}})
	require.NoError(t, err)

	mockPool.ExpectQuery(`(?s)ResolveTeamContext.*SELECT`).
		WithArgs(teamID, p.UserID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "public_id"}).AddRow(int64(7), teamID))
	mockPool.ExpectQuery(`(?s)ListTeamKeysForTeam.*SELECT`).
		WithArgs(int64(7), maximumListResults).
		WillReturnRows(pgxmock.NewRows([]string{"id", "team_id", "version", "status", "algorithm", "wraps", "created_by", "created_at"}).
			AddRow(int64(1), int64(7), int32(1), "active", "aes-256-gcm", wrapsJSON, p.UserID, now))

	res, err := svc.ListTeamKeys(context.Background(), p, orgID, teamID)
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Len(t, res[0].Wraps, 1)
	assert.Equal(t, p.UserID, res[0].Wraps[0].UserID)
	assert.Equal(t, teamID, res[0].TeamID)
	assert.Equal(t, []uuid.UUID{p.UserID, other}, res[0].WrappedUserIDs)
	assert.NoError(t, mockPool.ExpectationsWereMet())
}

func strptr(s string) *string { return &s }
