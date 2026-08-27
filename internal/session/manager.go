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
	"github.com/jackc/pgx/v5/pgtype"
)

type Database interface {
	publicdb.DBTX
	Begin(context.Context) (pgx.Tx, error)
}

type Option func(*Manager)

func WithClock(clock Clock) Option {
	return func(manager *Manager) {
		if clock != nil {
			manager.clock = clock
		}
	}
}

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

func (manager *Manager) Issue(ctx context.Context, userID uuid.UUID, userAgent string) (IssuedSession, error) {
	return manager.issueWithQueries(ctx, manager.queries, userID, userAgent)
}

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

func (manager *Manager) issueWithQueries(
	ctx context.Context,
	queries *publicdb.Queries,
	userID uuid.UUID,
	userAgent string,
) (IssuedSession, error) {
	now := manager.now()
	token, raw, err := manager.codec.issue()
	if err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
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

func (manager *Manager) ValidCSRF(authentication Authentication, candidate string) bool {
	return manager.csrf.Valid(authentication.rawToken, candidate)
}

func (manager *Manager) ValidTokenCSRF(token string, candidate string) bool {
	raw, err := parseToken(token)
	return err == nil && manager.csrf.Valid(raw, candidate)
}

func ParseableToken(token string) bool {
	_, err := parseToken(token)
	return err == nil
}

func (manager *Manager) CSRFToken(authentication Authentication) string {
	return manager.csrf.Derive(authentication.rawToken)
}

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

func (manager *Manager) RevokeAll(ctx context.Context, principal Principal) error {
	if err := manager.queries.RevokeAllUserSessions(ctx, publicdb.RevokeAllUserSessionsParams{
		Now: pgTimestamp(manager.now()), UserPublicID: principal.UserID,
	}); err != nil {
		return ErrSessionDependency
	}
	return nil
}

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

func (manager *Manager) now() time.Time { return manager.clock().UTC() }

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

func timestampPointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}

func pgTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
