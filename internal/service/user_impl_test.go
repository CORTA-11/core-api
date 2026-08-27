package service

import (
	"context"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockTokenService struct {
	mock.Mock
}

func (m *mockTokenService) GenerateToken(userPublicID uuid.UUID, email string) (string, error) {
	args := m.Called(userPublicID, email)
	return args.String(0), args.Error(1)
}

func (m *mockTokenService) VerifyToken(tokenString string) (*JWTClaims, error) {
	args := m.Called(tokenString)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*JWTClaims), args.Error(1)
}

type mockIdentityHasher struct {
	mock.Mock
}

func (m *mockIdentityHasher) Hash(ctx context.Context, password string) (string, error) {
	args := m.Called(ctx, password)
	return args.String(0), args.Error(1)
}

func (m *mockIdentityHasher) Verify(ctx context.Context, password, encodedHash string) (identity.PasswordVerification, error) {
	args := m.Called(ctx, password, encodedHash)
	return args.Get(0).(identity.PasswordVerification), args.Error(1)
}

type mockCredentialVerifier struct {
	mock.Mock
}

func (m *mockCredentialVerifier) Verify(ctx context.Context, email, password string) (identity.CredentialPrincipal, error) {
	args := m.Called(ctx, email, password)
	return args.Get(0).(identity.CredentialPrincipal), args.Error(1)
}

func TestUserServiceImpl(t *testing.T) {
	userPublicID := uuid.New()
	email := "john@example.com"
	displayName := "John Doe"
	now := time.Now().UTC()

	columns := []string{"id", "user_id", "email", "password_hash", "display_name", "created_at", "updated_at", "deleted_at", "email_canonical", "password_normalization"}

	t.Run("GetUsers returns all users successfully", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()

		mockPool.ExpectQuery("(?s)GetAllUsers :many.*SELECT").
			WithArgs(int32(100)).
			WillReturnRows(pgxmock.NewRows(columns).
				AddRow(int64(1), userPublicID, email, "hashed-password", displayName, now, now, pgtype.Timestamptz{Valid: false}, email, identity.PasswordNormalizationNFCV1))

		tokenSvc := new(mockTokenService)
		passwordSvc := new(mockIdentityHasher)
		verifier := new(mockCredentialVerifier)
		queries := publicdb.New(mockPool)
		svc := NewUserService(queries, tokenSvc, passwordSvc, verifier)

		users, err := svc.GetUsers(context.Background())
		require.NoError(t, err)
		require.Len(t, users, 1)
		assert.Equal(t, userPublicID, users[0].PublicID)
		assert.Equal(t, email, users[0].Email)
		assert.Equal(t, displayName, users[0].Name)
	})

	t.Run("GetUserByID returns a single domain user", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()

		mockPool.ExpectQuery("(?s)GetUserByID :one.*SELECT").
			WithArgs(userPublicID).
			WillReturnRows(pgxmock.NewRows(columns).
				AddRow(int64(1), userPublicID, email, "hashed-password", displayName, now, now, pgtype.Timestamptz{Valid: false}, email, identity.PasswordNormalizationNFCV1))

		tokenSvc := new(mockTokenService)
		passwordSvc := new(mockIdentityHasher)
		verifier := new(mockCredentialVerifier)
		queries := publicdb.New(mockPool)
		svc := NewUserService(queries, tokenSvc, passwordSvc, verifier)

		user, err := svc.GetUserByID(context.Background(), userPublicID.String())
		require.NoError(t, err)
		require.NotNil(t, user)
		assert.Equal(t, userPublicID, user.PublicID)
		assert.Equal(t, email, user.Email)
	})

	t.Run("CreateUser creates user inside transaction and hashes password", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()

		mockPool.ExpectQuery("(?s)CreateUser :one.*INSERT").
			WithArgs(email, "hashed-password", displayName, identity.PasswordNormalizationNFCV1).
			WillReturnRows(pgxmock.NewRows(columns).
				AddRow(int64(1), userPublicID, email, "hashed-password", displayName, now, now, pgtype.Timestamptz{Valid: false}, email, identity.PasswordNormalizationNFCV1))

		tokenSvc := new(mockTokenService)
		passwordSvc := new(mockIdentityHasher)
		passwordSvc.On("Hash", mock.Anything, "password123-secure").Return("hashed-password", nil)
		verifier := new(mockCredentialVerifier)

		queries := publicdb.New(mockPool)
		svc := NewUserService(queries, tokenSvc, passwordSvc, verifier)

		user, err := svc.CreateUser(context.Background(), displayName, email, "password123-secure")
		require.NoError(t, err)
		require.NotNil(t, user)
		assert.Equal(t, userPublicID, user.PublicID)
		assert.Equal(t, email, user.Email)
		assert.NoError(t, mockPool.ExpectationsWereMet())
		passwordSvc.AssertExpectations(t)
	})

	t.Run("CreateUser returns ErrEmailAlreadyInUse when email already registered", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()

		mockPool.ExpectQuery("(?s)CreateUser :one.*INSERT").
			WithArgs(email, "hashed-password", displayName, identity.PasswordNormalizationNFCV1).
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "users_email_canonical_unique"})

		tokenSvc := new(mockTokenService)
		passwordSvc := new(mockIdentityHasher)
		passwordSvc.On("Hash", mock.Anything, "password123-secure").Return("hashed-password", nil)
		verifier := new(mockCredentialVerifier)

		queries := publicdb.New(mockPool)
		svc := NewUserService(queries, tokenSvc, passwordSvc, verifier)

		_, err = svc.CreateUser(context.Background(), displayName, email, "password123-secure")
		assert.ErrorIs(t, err, ErrEmailAlreadyInUse)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("Login authenticates valid credentials and returns JWT token", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()

		mockPool.ExpectQuery("(?s)GetUserByID :one.*SELECT").
			WithArgs(userPublicID).
			WillReturnRows(pgxmock.NewRows(columns).
				AddRow(int64(1), userPublicID, email, "hashed-password", displayName, now, now, pgtype.Timestamptz{Valid: false}, email, identity.PasswordNormalizationNFCV1))

		tokenSvc := new(mockTokenService)
		tokenSvc.On("GenerateToken", userPublicID, email).Return("valid-jwt-token", nil)

		passwordSvc := new(mockIdentityHasher)
		verifier := new(mockCredentialVerifier)
		verifier.On("Verify", mock.Anything, email, "password123-secure").
			Return(identity.CredentialPrincipal{UserPublicID: userPublicID}, nil)

		queries := publicdb.New(mockPool)
		svc := NewUserService(queries, tokenSvc, passwordSvc, verifier)

		token, user, err := svc.Login(context.Background(), email, "password123-secure")
		require.NoError(t, err)
		assert.Equal(t, "valid-jwt-token", token)
		assert.Equal(t, userPublicID, user.PublicID)
		tokenSvc.AssertExpectations(t)
		verifier.AssertExpectations(t)
	})

	t.Run("Login returns ErrInvalidCredentials on password mismatch", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()

		tokenSvc := new(mockTokenService)
		passwordSvc := new(mockIdentityHasher)
		verifier := new(mockCredentialVerifier)
		verifier.On("Verify", mock.Anything, email, "wrong-password-value").
			Return(identity.CredentialPrincipal{}, identity.ErrInvalidCredentials)

		queries := publicdb.New(mockPool)
		svc := NewUserService(queries, tokenSvc, passwordSvc, verifier)

		_, _, err = svc.Login(context.Background(), email, "wrong-password-value")
		assert.ErrorIs(t, err, ErrInvalidCredentials)
		verifier.AssertExpectations(t)
	})
}
