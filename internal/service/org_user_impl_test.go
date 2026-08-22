package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrgUserServiceImpl(t *testing.T) {
	orgPublicID := uuid.New()
	userPublicID := uuid.New()
	orgID := int64(42)
	userID := int64(100)
	now := time.Now().UTC()

	orgColumns := []string{
		"id", "public_id", "name", "schema_name", "created_at", "updated_at", "deleted_at",
		"lifecycle_state", "tenant_version", "tenant_checksum", "reconcile_attempts", "next_attempt_at",
		"last_error_code", "last_error_detail", "last_attempt_at", "provisioned_at",
	}
	userColumns := []string{"id", "user_id", "email", "password_hash", "display_name", "created_at", "updated_at", "deleted_at"}

	t.Run("AddUserToOrg successfully resolves IDs and calls repository", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()

		mockPool.ExpectBegin()
		mockPool.ExpectExec("^SET LOCAL search_path TO public").WillReturnResult(pgxmock.NewResult("SET", 0))

		// 1. GetOrgID mock lookup
		mockPool.ExpectQuery("(?s)GetOrgID :one.*SELECT").
			WithArgs(orgPublicID).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(orgID))

		// 2. GetUserByID mock lookup
		mockPool.ExpectQuery("(?s)GetUserByID :one.*SELECT").
			WithArgs(userPublicID).
			WillReturnRows(pgxmock.NewRows(userColumns).
				AddRow(userID, userPublicID, "test@example.com", "hash", "John", now, now, pgtype.Timestamptz{Valid: false}))

		// 3. AddUserToOrg insert
		mockPool.ExpectQuery("(?s)AddUserToOrg :one.*INSERT").
			WithArgs(orgID, userID).
			WillReturnRows(pgxmock.NewRows([]string{"org_id", "user_id"}).AddRow(orgID, userID))

		mockPool.ExpectCommit()

		queries := publicdb.New(mockPool)
		svc := NewOrgUserService(mockPool, queries)

		err = svc.AddUserToOrg(context.Background(), orgPublicID, userPublicID)
		require.NoError(t, err)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("RemoveUserFromOrg successfully resolves IDs and calls repository", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()

		mockPool.ExpectBegin()
		mockPool.ExpectExec("^SET LOCAL search_path TO public").WillReturnResult(pgxmock.NewResult("SET", 0))

		// 1. GetOrgID mock lookup
		mockPool.ExpectQuery("(?s)GetOrgID :one.*SELECT").
			WithArgs(orgPublicID).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(orgID))

		// 2. GetUserByID mock lookup
		mockPool.ExpectQuery("(?s)GetUserByID :one.*SELECT").
			WithArgs(userPublicID).
			WillReturnRows(pgxmock.NewRows(userColumns).
				AddRow(userID, userPublicID, "test@example.com", "hash", "John", now, now, pgtype.Timestamptz{Valid: false}))

		// 3. RemoveUserFromOrg delete
		mockPool.ExpectQuery("(?s)RemoveUserFromOrg :one.*DELETE").
			WithArgs(orgID, userID).
			WillReturnRows(pgxmock.NewRows([]string{"org_id", "user_id"}).AddRow(orgID, userID))

		mockPool.ExpectCommit()

		queries := publicdb.New(mockPool)
		svc := NewOrgUserService(mockPool, queries)

		err = svc.RemoveUserFromOrg(context.Background(), orgPublicID, userPublicID)
		require.NoError(t, err)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("GetOrgsForUser returns mapped organizations", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()

		mockPool.ExpectBegin()
		mockPool.ExpectExec("^SET LOCAL search_path TO public").WillReturnResult(pgxmock.NewResult("SET", 0))

		// 1. GetUserByID mock lookup
		mockPool.ExpectQuery("(?s)GetUserByID :one.*SELECT").
			WithArgs(userPublicID).
			WillReturnRows(pgxmock.NewRows(userColumns).
				AddRow(userID, userPublicID, "test@example.com", "hash", "John", now, now, pgtype.Timestamptz{Valid: false}))

		// 2. GetOrgsForUser join query
		mockPool.ExpectQuery("(?s)GetOrgsForUser :many.*SELECT").
			WithArgs(userID, int32(100)).
			WillReturnRows(pgxmock.NewRows(orgColumns).
				AddRow(orgID, orgPublicID, "My Org", "org_schema", now, now, pgtype.Timestamptz{Valid: false},
					"active", int64(2), strings.Repeat("a", 64), int32(1), now,
					pgtype.Text{}, pgtype.Text{}, pgtype.Timestamptz{}, pgtype.Timestamptz{Time: now, Valid: true}))

		mockPool.ExpectCommit()

		queries := publicdb.New(mockPool)
		svc := NewOrgUserService(mockPool, queries)

		orgs, err := svc.GetOrgsForUser(context.Background(), userPublicID)
		require.NoError(t, err)
		require.Len(t, orgs, 1)
		assert.Equal(t, orgPublicID, orgs[0].PublicID)
		assert.Equal(t, "My Org", orgs[0].Name)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("GetUsersInOrg returns mapped users", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()

		mockPool.ExpectBegin()
		mockPool.ExpectExec("^SET LOCAL search_path TO public").WillReturnResult(pgxmock.NewResult("SET", 0))

		// 1. GetOrgID mock lookup
		mockPool.ExpectQuery("(?s)GetOrgID :one.*SELECT").
			WithArgs(orgPublicID).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(orgID))

		// 2. GetUsersInOrg join query
		mockPool.ExpectQuery("(?s)GetUsersInOrg :many.*SELECT").
			WithArgs(orgID, int32(100)).
			WillReturnRows(pgxmock.NewRows(userColumns).
				AddRow(userID, userPublicID, "test@example.com", "hash", "John", now, now, pgtype.Timestamptz{Valid: false}))

		mockPool.ExpectCommit()

		queries := publicdb.New(mockPool)
		svc := NewOrgUserService(mockPool, queries)

		users, err := svc.GetUsersInOrg(context.Background(), orgPublicID)
		require.NoError(t, err)
		require.Len(t, users, 1)
		assert.Equal(t, userPublicID, users[0].PublicID)
		assert.Equal(t, "test@example.com", users[0].Email)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})

	t.Run("GetNumberOfUsersInOrg returns row count", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mockPool.Close()

		mockPool.ExpectBegin()
		mockPool.ExpectExec("^SET LOCAL search_path TO public").WillReturnResult(pgxmock.NewResult("SET", 0))

		// 1. GetOrgID mock lookup
		mockPool.ExpectQuery("(?s)GetOrgID :one.*SELECT").
			WithArgs(orgPublicID).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(orgID))

		// 2. GetNumberOfUsersInOrg count query
		mockPool.ExpectQuery("(?s)GetNumberOfUsersInOrg :one.*SELECT").
			WithArgs(orgID).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(5)))

		mockPool.ExpectCommit()

		queries := publicdb.New(mockPool)
		svc := NewOrgUserService(mockPool, queries)

		count, err := svc.GetNumberOfUsersInOrg(context.Background(), orgPublicID)
		require.NoError(t, err)
		assert.Equal(t, int64(5), count)
		assert.NoError(t, mockPool.ExpectationsWereMet())
	})
}
