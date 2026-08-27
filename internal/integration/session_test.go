//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/session"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionMigrationEnforcesBoundedSecretFreeStorage(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	ctx := context.Background()

	var owner string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT tableowner FROM pg_tables
		WHERE schemaname = 'public' AND tablename = 'sessions'`).Scan(&owner))
	assert.Equal(t, "synodus_owner", owner)

	var hashCheck, agentCheck bool
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT
			bool_or(pg_get_constraintdef(oid) LIKE '%octet_length(token_hash) = 32%'),
			bool_or(pg_get_constraintdef(oid) LIKE '%octet_length(user_agent) <= 256%')
		FROM pg_constraint WHERE conrelid = 'public.sessions'::regclass`).Scan(&hashCheck, &agentCheck))
	assert.True(t, hashCheck)
	assert.True(t, agentCheck)

	var cleanupIndexes int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = 'sessions'
		  AND indexname IN ('sessions_revoked_cleanup_idx', 'sessions_absolute_cleanup_idx', 'sessions_idle_cleanup_idx')`).Scan(&cleanupIndexes))
	assert.Equal(t, 3, cleanupIndexes)
}

func TestSessionExpiryRevocationAndCleanupArePostgresAuthoritative(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	ctx := context.Background()
	userID := createSessionTestUser(t, pool, "session-one@example.com")
	otherUserID := createSessionTestUser(t, pool, "session-two@example.com")
	now := time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC)
	randomData := make([]byte, 0, 8192)
	for value := 0; value < 256; value++ {
		randomData = append(randomData, bytes.Repeat([]byte{byte(value)}, 32)...)
	}
	manager, err := session.NewManager(pool, bytes.Repeat([]byte{0x5c}, 32),
		session.WithClock(func() time.Time { return now }),
		session.WithRandom(bytes.NewReader(randomData)))
	require.NoError(t, err)

	idleSession, err := manager.Issue(ctx, userID, "Browser Cafe\u0301")
	require.NoError(t, err)
	assert.Equal(t, "Browser Caf\u00e9", idleSession.Authentication.Session.UserAgent)

	now = now.Add(session.TouchInterval)
	_, err = manager.Authenticate(ctx, idleSession.RawToken)
	require.NoError(t, err)
	var touchedAt time.Time
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT last_seen_at FROM public.sessions WHERE session_id = $1`,
		idleSession.Authentication.Principal.SessionID).Scan(&touchedAt))
	assert.WithinDuration(t, now, touchedAt, 0)

	now = now.Add(session.IdleLifetime)
	_, err = manager.Authenticate(ctx, idleSession.RawToken)
	assert.ErrorIs(t, err, session.ErrUnauthenticated)

	now = time.Date(2026, 8, 28, 1, 0, 0, 0, time.UTC)
	current, err := manager.Issue(ctx, userID, "Current")
	require.NoError(t, err)
	other, err := manager.Issue(ctx, otherUserID, "Other")
	require.NoError(t, err)
	assert.ErrorIs(t, manager.Revoke(ctx, current.Authentication.Principal,
		other.Authentication.Principal.SessionID), session.ErrSessionNotFound)
	require.NoError(t, manager.RevokeCurrent(ctx, current.RawToken))
	_, err = manager.Authenticate(ctx, current.RawToken)
	assert.ErrorIs(t, err, session.ErrUnauthenticated)

	now = now.Add(session.AbsoluteLifetime)
	result, err := manager.Cleanup(ctx, 2)
	require.NoError(t, err)
	assert.LessOrEqual(t, result.Total(), int64(2))
	assert.Equal(t, int64(2), result.Total())
	assert.Error(t, func() error {
		_, cleanupErr := manager.Cleanup(ctx, session.MaximumBatchSize+1)
		return cleanupErr
	}())
}

func createSessionTestUser(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	var userID uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO public.users (email, password_hash, display_name, password_normalization)
		VALUES ($1, '$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHRzYWx0MTIzNA$YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXphYmNkZWY', 'Session User', 'nfc_v1')
		RETURNING user_id`, email).Scan(&userID)
	require.NoError(t, err)
	return userID
}
