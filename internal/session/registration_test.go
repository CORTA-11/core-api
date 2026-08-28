package session

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterRollsBackAccountWhenSessionCreationFails(t *testing.T) {
	pool, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer pool.Close()
	now := time.Date(2026, 8, 28, 6, 0, 0, 0, time.UTC)
	userID := uuid.New()
	pool.ExpectBegin()
	pool.ExpectQuery("(?s)CreateUser :one.*INSERT").
		WithArgs("User@Example.com", "argon-hash", "Researcher", identity.PasswordNormalizationNFCV1).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "email", "password_hash", "display_name", "created_at", "updated_at",
			"deleted_at", "email_canonical", "password_normalization",
		}).AddRow(int64(1), userID, "User@Example.com", "argon-hash", "Researcher", now, now,
			pgtype.Timestamptz{}, "user@example.com", identity.PasswordNormalizationNFCV1))
	pool.ExpectQuery("(?s)CreateSession :one.*INSERT").
		WithArgs(pgxmock.AnyArg(), "Browser", now, now.Add(AbsoluteLifetime), userID).
		WillReturnError(errors.New("session insert failed"))
	pool.ExpectRollback()
	manager, err := NewManager(pool, bytes.Repeat([]byte{2}, 32),
		WithClock(func() time.Time { return now }), WithRandom(bytes.NewReader(bytes.Repeat([]byte{3}, 32))))
	require.NoError(t, err)
	issued, err := manager.Register(context.Background(), "User@Example.com", "Researcher", "argon-hash",
		identity.PasswordNormalizationNFCV1, "", "Browser")
	assert.ErrorIs(t, err, ErrSessionDependency)
	assert.Empty(t, issued.RawToken)
	assert.Empty(t, issued.CSRFToken)
	require.NoError(t, pool.ExpectationsWereMet())
}
