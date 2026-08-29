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

type TeamSharedKey struct {
	TeamID       uuid.UUID `json:"team_id"`
	UserID       uuid.UUID `json:"user_id"`
	EncryptedKey string    `json:"encrypted_key"`
	KeyVersion   int32     `json:"key_version"`
	CreatedAt    time.Time `json:"created_at"`
}

type KeyService interface {
	UpsertPublicKey(ctx context.Context, p session.Principal, publicKey string) (*UserPublicKey, error)
	GetPublicKey(ctx context.Context, p session.Principal, userID uuid.UUID) (*UserPublicKey, error)
	GetPublicKeysForTeam(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID) ([]UserPublicKey, error)

	UpsertTeamSharedKeys(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID, keys []TeamSharedKey) error
	GetTeamSharedKeyForUser(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID, userID uuid.UUID, version int32) (*TeamSharedKey, error)
	ListTeamSharedKeysForUser(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID, userID uuid.UUID) ([]TeamSharedKey, error)
}
