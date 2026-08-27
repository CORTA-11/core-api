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

type fixedPasswordHasher struct{}

func (fixedPasswordHasher) Verify(context.Context, string, string) (identity.PasswordVerification, error) {
	return identity.PasswordVerification{Match: true}, nil
}

func (fixedPasswordHasher) Hash(context.Context, string) (string, error) {
	return "new-hash", nil
}

func TestChangePasswordDoesNotEmitReplacementOnTransactionFailure(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		configureTx func(pgxmock.PgxPoolIface, time.Time, uuid.UUID)
	}{
		{
			name: "replacement insert rolls back",
			configureTx: func(pool pgxmock.PgxPoolIface, now time.Time, userID uuid.UUID) {
				pool.ExpectBegin()
				pool.ExpectExec("(?s)CompareAndSwapCredential :execrows.*UPDATE").
					WithArgs("new-hash", identity.PasswordNormalizationNFCV1, userID,
						"old-hash", identity.PasswordNormalizationNFCV1).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				pool.ExpectExec("(?s)RevokeAllUserSessions :exec.*UPDATE").
					WithArgs(pgtype.Timestamptz{Time: now, Valid: true}, userID).
					WillReturnResult(pgxmock.NewResult("UPDATE", 2))
				pool.ExpectQuery("(?s)CreateSession :one.*INSERT").
					WithArgs(pgxmock.AnyArg(), "Browser", now, now.Add(AbsoluteLifetime), userID).
					WillReturnError(errors.New("insert failed"))
				pool.ExpectRollback()
			},
		},
		{
			name: "commit failure returns no secret",
			configureTx: func(pool pgxmock.PgxPoolIface, now time.Time, userID uuid.UUID) {
				pool.ExpectBegin()
				pool.ExpectExec("(?s)CompareAndSwapCredential :execrows.*UPDATE").
					WithArgs("new-hash", identity.PasswordNormalizationNFCV1, userID,
						"old-hash", identity.PasswordNormalizationNFCV1).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
				pool.ExpectExec("(?s)RevokeAllUserSessions :exec.*UPDATE").
					WithArgs(pgtype.Timestamptz{Time: now, Valid: true}, userID).
					WillReturnResult(pgxmock.NewResult("UPDATE", 2))
				pool.ExpectQuery("(?s)CreateSession :one.*INSERT").
					WithArgs(pgxmock.AnyArg(), "Browser", now, now.Add(AbsoluteLifetime), userID).
					WillReturnRows(pgxmock.NewRows([]string{
						"id", "session_id", "user_id", "token_hash", "user_agent", "created_at",
						"last_seen_at", "absolute_expires_at", "revoked_at",
					}).AddRow(int64(10), uuid.New(), int64(20), bytes.Repeat([]byte{1}, 32),
						"Browser", now, now, now.Add(AbsoluteLifetime), pgtype.Timestamptz{}))
				pool.ExpectCommit().WillReturnError(errors.New("commit failed"))
				pool.ExpectRollback()
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pool, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer pool.Close()
			now := time.Date(2026, 8, 27, 6, 0, 0, 0, time.UTC)
			userID := uuid.New()
			pool.ExpectQuery("(?s)GetCurrentCredentialByUserID :one.*SELECT").
				WithArgs(userID).
				WillReturnRows(pgxmock.NewRows([]string{
					"user_id", "password_hash", "password_normalization", "deleted_at",
				}).AddRow(userID, "old-hash", identity.PasswordNormalizationNFCV1, pgtype.Timestamptz{}))
			test.configureTx(pool, now, userID)
			manager, err := NewManager(pool, bytes.Repeat([]byte{2}, 32),
				WithClock(func() time.Time { return now }),
				WithRandom(bytes.NewReader(bytes.Repeat([]byte{3}, 32))))
			require.NoError(t, err)
			issued, err := manager.ChangePassword(context.Background(), Authentication{
				Principal: Principal{UserID: userID, SessionID: uuid.New()},
				User:      User{ID: userID, Email: "user@example.com", DisplayName: "User"},
			}, "current-password-value", "replacement-password-value", "Browser", fixedPasswordHasher{})
			assert.ErrorIs(t, err, ErrSessionDependency)
			assert.Empty(t, issued.RawToken)
			assert.Empty(t, issued.CSRFToken)
			require.NoError(t, pool.ExpectationsWereMet())
		})
	}
}
