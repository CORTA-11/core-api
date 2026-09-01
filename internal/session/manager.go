package session

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"time"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type Database interface {
	publicdb.DBTX
	Begin(context.Context) (pgx.Tx, error)
}

type Option func(*Manager)

// WithClock configures clock.
func WithClock(clock Clock) Option {
	return func(manager *Manager) {
		if clock != nil {
			manager.clock = clock
		}
	}
}

// WithRandom configures random.
func WithRandom(random io.Reader) Option {
	return func(manager *Manager) {
		if random != nil {
			manager.codec.random = random
		}
	}
}

type Manager struct {
	database Database
	queries  *publicdb.Queries
	csrf     CSRFProtector
	codec    tokenCodec
	clock    Clock
}

// NewManager creates a manager.
func NewManager(database Database, csrfSecret []byte, options ...Option) (*Manager, error) {
	if database == nil {
		return nil, ErrSessionDependency
	}
	csrf, err := NewCSRFProtector(csrfSecret)
	if err != nil {
		return nil, err
	}
	manager := &Manager{
		database: database,
		queries:  publicdb.New(database),
		csrf:     csrf,
		codec:    tokenCodec{random: rand.Reader},
		clock:    time.Now,
	}
	for _, option := range options {
		option(manager)
	}
	return manager, nil
}

// Issue handles the issue operation.
func (manager *Manager) Issue(ctx context.Context, userID uuid.UUID, userAgent string) (IssuedSession, error) {
	return manager.issueWithQueries(ctx, manager.queries, userID, userAgent)
}

// Rotate handles the rotate operation.
func (manager *Manager) Rotate(
	ctx context.Context,
	userID uuid.UUID,
	oldToken string,
	userAgent string,
) (IssuedSession, error) {
	tx, err := manager.database.Begin(ctx)
	if err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := manager.queries.WithTx(tx)
	if raw, parseErr := parseToken(oldToken); parseErr == nil {
		hash := hashToken(raw)
		if _, err := queries.RevokeSessionByHash(ctx, publicdb.RevokeSessionByHashParams{
			Now: pgTimestamp(manager.now()), TokenHash: hash[:],
		}); err != nil {
			return IssuedSession{}, ErrSessionDependency
		}
	}
	issued, err := manager.issueWithQueries(ctx, queries, userID, userAgent)
	if err != nil {
		return IssuedSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	return issued, nil
}

// Register creates a local account and its first browser session atomically.
// Password hashing is deliberately performed by the caller and session token
// generation is completed here before the database transaction is opened.
func (manager *Manager) Register(
	ctx context.Context,
	email string,
	displayName string,
	passwordHash string,
	passwordNormalization string,
	oldToken string,
	userAgent string,
) (IssuedSession, error) {
	token, raw, err := manager.codec.issue()
	if err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	tx, err := manager.database.Begin(ctx)
	if err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := manager.queries.WithTx(tx)
	user, err := queries.CreateUser(ctx, publicdb.CreateUserParams{
		Email: email, PasswordHash: passwordHash, DisplayName: displayName,
		PasswordNormalization: passwordNormalization,
	})
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" &&
			postgresError.ConstraintName == "users_email_canonical_unique" {
			return IssuedSession{}, ErrEmailAlreadyExists
		}
		return IssuedSession{}, ErrSessionDependency
	}
	if oldRaw, parseErr := parseToken(oldToken); parseErr == nil {
		hash := hashToken(oldRaw)
		if _, err := queries.RevokeSessionByHash(ctx, publicdb.RevokeSessionByHashParams{
			Now: pgTimestamp(manager.now()), TokenHash: hash[:],
		}); err != nil {
			return IssuedSession{}, ErrSessionDependency
		}
	}
	issued, err := manager.issuePreparedWithQueries(ctx, queries, user.UserID, userAgent, token, raw)
	if err != nil {
		return IssuedSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	return issued, nil
}

// issueWithQueries issues with queries.
func (manager *Manager) issueWithQueries(
	ctx context.Context,
	queries *publicdb.Queries,
	userID uuid.UUID,
	userAgent string,
) (IssuedSession, error) {
	token, raw, err := manager.codec.issue()
	if err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	return manager.issuePreparedWithQueries(ctx, queries, userID, userAgent, token, raw)
}

// issuePreparedWithQueries issues prepared with queries.
func (manager *Manager) issuePreparedWithQueries(
	ctx context.Context,
	queries *publicdb.Queries,
	userID uuid.UUID,
	userAgent string,
	token string,
	raw []byte,
) (IssuedSession, error) {
	now := manager.now()
	hash := hashToken(raw)
	row, err := queries.CreateSession(ctx, publicdb.CreateSessionParams{
		TokenHash: hash[:], UserAgent: NormalizeUserAgent(userAgent), Now: now,
		AbsoluteExpiresAt: now.Add(AbsoluteLifetime), UserPublicID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return IssuedSession{}, ErrSessionNotFound
	}
	if err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	user, err := queries.GetUserByID(ctx, userID)
	if err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	authentication := Authentication{
		Principal: Principal{UserID: userID, SessionID: row.SessionID},
		User:      User{ID: user.UserID, Email: user.Email, DisplayName: user.DisplayName},
		Session: metadata(row.SessionID, row.UserAgent, row.CreatedAt, row.LastSeenAt,
			row.AbsoluteExpiresAt, nil, true),
		rawToken: append([]byte(nil), raw...),
	}
	return IssuedSession{
		Authentication: authentication,
		RawToken:       token,
		CSRFToken:      manager.csrf.Derive(raw),
		PublicID:       row.SessionID,
		ExpiresAt:      row.AbsoluteExpiresAt,
	}, nil
}

// Authenticate handles the authenticate operation.
func (manager *Manager) Authenticate(ctx context.Context, token string) (Authentication, error) {
	raw, err := parseToken(token)
	if err != nil {
		return Authentication{}, ErrUnauthenticated
	}
	hash := hashToken(raw)
	now := manager.now()
	row, err := manager.queries.GetSessionByTokenHash(ctx, publicdb.GetSessionByTokenHashParams{
		TokenHash: hash[:], Now: now, IdleCutoff: now.Add(-IdleLifetime),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Authentication{}, ErrUnauthenticated
	}
	if err != nil {
		return Authentication{}, ErrSessionDependency
	}
	if row.RevokedAt.Valid || !now.Before(row.AbsoluteExpiresAt) || !now.Before(row.LastSeenAt.Add(IdleLifetime)) {
		return Authentication{}, ErrUnauthenticated
	}
	lastSeen := row.LastSeenAt
	if !now.Before(row.LastSeenAt.Add(TouchInterval)) {
		_, err := manager.queries.TouchSession(ctx, publicdb.TouchSessionParams{
			Now: now, TokenHash: hash[:], IdleCutoff: now.Add(-IdleLifetime),
			TouchBefore: now.Add(-TouchInterval),
		})
		if err != nil {
			return Authentication{}, ErrSessionDependency
		}
		lastSeen = now
	}
	return Authentication{
		Principal: Principal{UserID: row.UserPublicID, SessionID: row.SessionID},
		User:      User{ID: row.UserPublicID, Email: row.Email, DisplayName: row.DisplayName},
		Session: metadata(row.SessionID, row.UserAgent, row.CreatedAt, lastSeen,
			row.AbsoluteExpiresAt, timestampPointer(row.RevokedAt), true),
		rawToken: append([]byte(nil), raw...),
	}, nil
}

// ValidCSRF checks whether csrf is valid.
func (manager *Manager) ValidCSRF(authentication Authentication, candidate string) bool {
	return manager.csrf.Valid(authentication.rawToken, candidate)
}

// ValidTokenCSRF checks whether token csrf is valid.
func (manager *Manager) ValidTokenCSRF(token string, candidate string) bool {
	raw, err := parseToken(token)
	return err == nil && manager.csrf.Valid(raw, candidate)
}

// ParseableToken parseables token.
func ParseableToken(token string) bool {
	_, err := parseToken(token)
	return err == nil
}

// CSRFToken csrftos ken.
func (manager *Manager) CSRFToken(authentication Authentication) string {
	return manager.csrf.Derive(authentication.rawToken)
}

// List lists the requested resources.
func (manager *Manager) List(ctx context.Context, principal Principal) ([]Metadata, error) {
	now := manager.now()
	rows, err := manager.queries.ListUserSessions(ctx, publicdb.ListUserSessionsParams{
		UserPublicID: principal.UserID, Now: now, IdleCutoff: now.Add(-IdleLifetime),
		ResultLimit: MaximumListSize,
	})
	if err != nil {
		return nil, ErrSessionDependency
	}
	result := make([]Metadata, 0, len(rows))
	for _, row := range rows {
		result = append(result, metadata(row.SessionID, row.UserAgent, row.CreatedAt,
			row.LastSeenAt, row.AbsoluteExpiresAt, timestampPointer(row.RevokedAt),
			row.SessionID == principal.SessionID))
	}
	return result, nil
}

// RevokeCurrent revokes current.
func (manager *Manager) RevokeCurrent(ctx context.Context, token string) error {
	raw, err := parseToken(token)
	if err != nil {
		return ErrInvalidToken
	}
	hash := hashToken(raw)
	_, err = manager.queries.RevokeSessionByHash(ctx, publicdb.RevokeSessionByHashParams{
		Now: pgTimestamp(manager.now()), TokenHash: hash[:],
	})
	if err != nil {
		return ErrSessionDependency
	}
	return nil
}

// Revoke handles the revoke operation.
func (manager *Manager) Revoke(ctx context.Context, principal Principal, sessionID uuid.UUID) error {
	rows, err := manager.queries.RevokeUserSession(ctx, publicdb.RevokeUserSessionParams{
		Now: pgTimestamp(manager.now()), UserPublicID: principal.UserID, SessionPublicID: sessionID,
	})
	if err != nil {
		return ErrSessionDependency
	}
	if rows == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// RevokeAll revokes all.
func (manager *Manager) RevokeAll(ctx context.Context, principal Principal) error {
	if err := manager.queries.RevokeAllUserSessions(ctx, publicdb.RevokeAllUserSessionsParams{
		Now: pgTimestamp(manager.now()), UserPublicID: principal.UserID,
	}); err != nil {
		return ErrSessionDependency
	}
	return nil
}

// Cleanup handles the cleanup operation.
func (manager *Manager) Cleanup(ctx context.Context, batchSize int) (CleanupResult, error) {
	if batchSize == 0 {
		batchSize = DefaultBatchSize
	}
	if batchSize < 1 || batchSize > MaximumBatchSize {
		return CleanupResult{}, ErrInvalidBatchSize
	}
	now := manager.now()
	remaining := int64(batchSize)
	var result CleanupResult
	deleted, err := manager.queries.DeleteRevokedSessions(ctx, int32(remaining))
	if err != nil {
		return CleanupResult{}, ErrSessionDependency
	}
	result.RevokedDeleted = deleted
	remaining -= deleted
	if remaining > 0 {
		deleted, err = manager.queries.DeleteAbsoluteExpiredSessions(ctx, publicdb.DeleteAbsoluteExpiredSessionsParams{
			// #nosec G115 -- batchSize is validated at no more than 1000 above.
			Now: now, BatchLimit: int32(remaining),
		})
		if err != nil {
			return CleanupResult{}, ErrSessionDependency
		}
		result.AbsoluteExpiredDeleted = deleted
		remaining -= deleted
	}
	if remaining > 0 {
		deleted, err = manager.queries.DeleteIdleExpiredSessions(ctx, publicdb.DeleteIdleExpiredSessionsParams{
			// #nosec G115 -- batchSize is validated at no more than 1000 above.
			Now: now, IdleCutoff: now.Add(-IdleLifetime), BatchLimit: int32(remaining),
		})
		if err != nil {
			return CleanupResult{}, ErrSessionDependency
		}
		result.IdleExpiredDeleted = deleted
	}
	return result, nil
}

// now returns the current UTC time.
func (manager *Manager) now() time.Time { return manager.clock().UTC() }

// metadata builds session metadata.
func metadata(
	id uuid.UUID,
	userAgent string,
	createdAt time.Time,
	lastSeenAt time.Time,
	absoluteExpiresAt time.Time,
	revokedAt *time.Time,
	current bool,
) Metadata {
	idleExpiresAt := lastSeenAt.Add(IdleLifetime)
	if absoluteExpiresAt.Before(idleExpiresAt) {
		idleExpiresAt = absoluteExpiresAt
	}
	return Metadata{
		ID: id, UserAgent: userAgent, CreatedAt: createdAt, LastSeenAt: lastSeenAt,
		IdleExpiresAt: idleExpiresAt, AbsoluteExpiresAt: absoluteExpiresAt,
		RevokedAt: revokedAt, Current: current,
	}
}

// timestampPointer timestamps pointer.
func timestampPointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}

// pgTimestamp pgs timestamp.
func pgTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
