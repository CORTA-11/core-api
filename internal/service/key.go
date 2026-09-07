package service

import (
	"context"
	"time"

	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
)

type UserPublicKey struct {
	UserID    uuid.UUID `json:"user_id"`
	PublicKey string    `json:"public_key"`
	CreatedAt time.Time `json:"created_at"`
}

// UserKey is the per-user key material: the RSA public key plus the private key
// sealed with a password-derived key, so the server never sees either in the clear.
type UserKey struct {
	UserID              uuid.UUID `json:"user_id"`
	PublicKey           string    `json:"public_key"`
	EncryptedPrivateKey *string   `json:"encrypted_private_key,omitempty"`
	KEKSalt             *string   `json:"kek_salt,omitempty"`
	KEKIterations       *int32    `json:"kek_iterations,omitempty"`
	KEKAlgorithm        *string   `json:"kek_algorithm,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// UserKeyUpdate carries the fields a client may set for its own key pair.
type UserKeyUpdate struct {
	PublicKey           string  `json:"public_key"`
	EncryptedPrivateKey *string `json:"encrypted_private_key"`
	KEKSalt             *string `json:"kek_salt"`
	KEKIterations       *int32  `json:"kek_iterations"`
	KEKAlgorithm        *string `json:"kek_algorithm"`
}

// TeamKeyWrap is one member's RSA-wrapped copy of the team symmetric key.
type TeamKeyWrap struct {
	UserID    uuid.UUID `json:"user_id"`
	Key       string    `json:"key"`
	Algorithm string    `json:"algorithm"`
}

// TeamKey is one version of a team's symmetric key. Wraps returned to a caller
// are filtered to that caller; WrappedUserIDs always lists everyone covered.
type TeamKey struct {
	ID             int64         `json:"id"`
	TeamID         uuid.UUID     `json:"team_id"`
	Version        int32         `json:"version"`
	Status         string        `json:"status"`
	Algorithm      string        `json:"algorithm"`
	Wraps          []TeamKeyWrap `json:"wraps"`
	WrappedUserIDs []uuid.UUID   `json:"wrapped_user_ids"`
	CreatedBy      uuid.UUID     `json:"created_by"`
	CreatedAt      time.Time     `json:"created_at"`
}

// TeamKeyVersionInput describes the wraps of a new team key version.
type TeamKeyVersionInput struct {
	Algorithm string
	Wraps     []TeamKeyWrap
}

type KeyService interface {
	UpsertUserKeys(ctx context.Context, p session.Principal, input UserKeyUpdate) (*UserKey, error)
	GetUserKeys(ctx context.Context, p session.Principal) (*UserKey, error)
	GetPublicKeysForTeam(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID) ([]UserPublicKey, error)

	CreateTeamKey(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID, input TeamKeyVersionInput) (*TeamKey, error)
	ListTeamKeys(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID) ([]TeamKey, error)
}
