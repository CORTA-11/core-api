package session

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	IdleLifetime     = 30 * time.Minute
	AbsoluteLifetime = 12 * time.Hour
	TouchInterval    = 5 * time.Minute
	MaximumListSize  = 100
	DefaultBatchSize = 500
	MaximumBatchSize = 1000
)

var (
	ErrUnauthenticated   = errors.New("session is not authenticated")
	ErrSessionNotFound   = errors.New("session not found")
	ErrSessionDependency = errors.New("session dependency unavailable")
	ErrInvalidBatchSize  = errors.New("session cleanup batch size must be between 1 and 1000")
	ErrCredentialChanged = errors.New("credential changed concurrently")
)

type Principal struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
}

type User struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
}

type Metadata struct {
	ID                uuid.UUID  `json:"id"`
	UserAgent         string     `json:"user_agent,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	LastSeenAt        time.Time  `json:"last_seen_at"`
	IdleExpiresAt     time.Time  `json:"idle_expires_at"`
	AbsoluteExpiresAt time.Time  `json:"absolute_expires_at"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	Current           bool       `json:"current"`
}

type Authentication struct {
	Principal Principal
	User      User
	Session   Metadata
	rawToken  []byte
}

type IssuedSession struct {
	Authentication Authentication
	RawToken       string
	CSRFToken      string
	PublicID       uuid.UUID
	ExpiresAt      time.Time
}

type CleanupResult struct {
	RevokedDeleted         int64
	AbsoluteExpiredDeleted int64
	IdleExpiredDeleted     int64
}

func (result CleanupResult) Total() int64 {
	return result.RevokedDeleted + result.AbsoluteExpiredDeleted + result.IdleExpiredDeleted
}

type Clock func() time.Time

type Authenticator interface {
	Authenticate(context.Context, string) (Authentication, error)
}
