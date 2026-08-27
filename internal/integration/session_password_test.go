//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswordChangeAtomicallyRevokesSessionsAndIssuesReplacement(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	ctx := context.Background()
	hasher, err := identity.NewPasswordHasher(identity.HashConfig{})
	require.NoError(t, err)
	currentPassword := "current-password-value"
	newPassword := "replacement-password-value"
	userID := createPasswordSessionTestUser(t, pool, hasher, currentPassword)
	now := time.Date(2026, 8, 27, 4, 0, 0, 0, time.UTC)
	randomData := append(bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 32)...)
	randomData = append(randomData, bytes.Repeat([]byte{0x33}, 32)...)
	manager, err := session.NewManager(pool, bytes.Repeat([]byte{0x4e}, 32),
		session.WithClock(func() time.Time { return now }), session.WithRandom(bytes.NewReader(randomData)))
	require.NoError(t, err)
	current, err := manager.Issue(ctx, userID, "Current browser")
	require.NoError(t, err)
	other, err := manager.Issue(ctx, userID, "Other browser")
	require.NoError(t, err)

	replacement, err := manager.ChangePassword(ctx, current.Authentication,
		currentPassword, newPassword, "Current browser", hasher)
	require.NoError(t, err)
	assert.NotEqual(t, current.RawToken, replacement.RawToken)
	assert.NotEmpty(t, replacement.CSRFToken)
	_, err = manager.Authenticate(ctx, current.RawToken)
	assert.ErrorIs(t, err, session.ErrUnauthenticated)
	_, err = manager.Authenticate(ctx, other.RawToken)
	assert.ErrorIs(t, err, session.ErrUnauthenticated)
	_, err = manager.Authenticate(ctx, replacement.RawToken)
	require.NoError(t, err)

	verifier, err := identity.NewCredentialVerifier(ctx,
		identity.NewPostgresCredentialStore(publicdb.New(pool)), hasher)
	require.NoError(t, err)
	_, err = verifier.Verify(ctx, "password-session@example.com", currentPassword)
	assert.ErrorIs(t, err, identity.ErrInvalidCredentials)
	verified, err := verifier.Verify(ctx, "password-session@example.com", newPassword)
	require.NoError(t, err)
	assert.Equal(t, userID, verified.UserPublicID)
}

func TestConcurrentPasswordChangesHaveOneCASWinner(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	testsupport.ApplyMigrations(t, "db/migrations/public", testsupport.RequiredEnv(t, "TEST_DATABASE_URL"))
	ctx := context.Background()
	baseHasher, err := identity.NewPasswordHasher(identity.HashConfig{})
	require.NoError(t, err)
	currentPassword := "concurrent-current-password"
	userID := createPasswordSessionTestUser(t, pool, baseHasher, currentPassword)
	now := time.Date(2026, 8, 27, 5, 0, 0, 0, time.UTC)
	firstManager, err := session.NewManager(pool, bytes.Repeat([]byte{0x5e}, 32),
		session.WithClock(func() time.Time { return now }),
		session.WithRandom(bytes.NewReader(append(
			bytes.Repeat([]byte{0x61}, 32), bytes.Repeat([]byte{0x63}, 32)...))))
	require.NoError(t, err)
	secondManager, err := session.NewManager(pool, bytes.Repeat([]byte{0x5e}, 32),
		session.WithClock(func() time.Time { return now }),
		session.WithRandom(bytes.NewReader(bytes.Repeat([]byte{0x62}, 64))))
	require.NoError(t, err)
	current, err := firstManager.Issue(ctx, userID, "Current")
	require.NoError(t, err)

	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	hasher := &barrierPasswordHasher{PasswordHasher: baseHasher, ready: ready, release: release}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index, change := range []struct {
		manager  *session.Manager
		password string
	}{{firstManager, "first-new-password-value"}, {secondManager, "second-new-password-value"}} {
		wait.Add(1)
		go func(index int, change struct {
			manager  *session.Manager
			password string
		}) {
			defer wait.Done()
			_, changeErr := change.manager.ChangePassword(ctx, current.Authentication,
				currentPassword, change.password, "Browser", hasher)
			results <- changeErr
		}(index, change)
	}
	<-ready
	<-ready
	close(release)
	wait.Wait()
	close(results)

	var successes, conflicts int
	for result := range results {
		if result == nil {
			successes++
		} else if errors.Is(result, session.ErrCredentialChanged) {
			conflicts++
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)
}

type barrierPasswordHasher struct {
	identity.PasswordHasher
	ready   chan<- struct{}
	release <-chan struct{}
}

func (hasher *barrierPasswordHasher) Verify(
	ctx context.Context,
	password string,
	encoded string,
) (identity.PasswordVerification, error) {
	verification, err := hasher.PasswordHasher.Verify(ctx, password, encoded)
	hasher.ready <- struct{}{}
	<-hasher.release
	return verification, err
}

func createPasswordSessionTestUser(
	t *testing.T,
	pool *pgxpool.Pool,
	hasher identity.PasswordHasher,
	password string,
) uuid.UUID {
	t.Helper()
	hash, err := hasher.Hash(context.Background(), password)
	require.NoError(t, err)
	var userID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		INSERT INTO public.users (email, password_hash, display_name, password_normalization)
		VALUES ('password-session@example.com', $1, 'Password User', 'nfc_v1')
		RETURNING user_id`, hash).Scan(&userID)
	require.NoError(t, err)
	return userID
}
