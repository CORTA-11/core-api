package service

import (
	"context"
	"errors"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type keyService struct {
	pool       pgxPool
	authorizer applicationAuthorizer
}

// NewKeyService creates a new instance of KeyService.
func NewKeyService(pool pgxPool, authorizer applicationAuthorizer) KeyService {
	return &keyService{
		pool:       pool,
		authorizer: authorizer,
	}
}

// UpsertPublicKey upserts public key.
func (s *keyService) UpsertPublicKey(ctx context.Context, p session.Principal, publicKey string) (*UserPublicKey, error) {
	if p.UserID == uuid.Nil {
		return nil, authorization.ErrUnauthenticated
	}
	if len(publicKey) < 64 || len(publicKey) > 8192 {
		return nil, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	queries := publicdb.New(tx)
	row, err := queries.UpsertUserPublicKey(ctx, publicdb.UpsertUserPublicKeyParams{
		UserID:    p.UserID,
		PublicKey: publicKey,
	})
	if err != nil {
		return nil, err
	}

	return &UserPublicKey{
		UserID:    row.UserID,
		PublicKey: row.PublicKey,
		CreatedAt: row.CreatedAt,
	}, tx.Commit(ctx)
}

// GetPublicKey gets public key.
func (s *keyService) GetPublicKey(ctx context.Context, p session.Principal, userID uuid.UUID) (*UserPublicKey, error) {
	if p.UserID == uuid.Nil {
		return nil, authorization.ErrUnauthenticated
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	queries := publicdb.New(tx)
	row, err := queries.GetUserPublicKey(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, authorization.ErrResourceNotFound
	}
	if err != nil {
		return nil, err
	}

	return &UserPublicKey{
		UserID:    row.UserID,
		PublicKey: row.PublicKey,
		CreatedAt: row.CreatedAt,
	}, tx.Commit(ctx)
}

// GetPublicKeysForTeam gets public keys for team.
func (s *keyService) GetPublicKeysForTeam(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID) ([]UserPublicKey, error) {
	var memberUUIDs []uuid.UUID
	err := s.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionTeamRead, func(queries *tenantdb.Queries) error {
		rows, err := queries.ListTeamMembersAfter(ctx, 1000)
		if err != nil {
			return err
		}
		memberUUIDs = make([]uuid.UUID, len(rows))
		for i, row := range rows {
			memberUUIDs[i] = row.UserPublicID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	publicdbQueries := publicdb.New(tx)
	rows, err := publicdbQueries.GetUserPublicKeys(ctx, memberUUIDs)
	if err != nil {
		return nil, err
	}

	views := make([]UserPublicKey, len(rows))
	for i, row := range rows {
		views[i] = UserPublicKey{
			UserID:    row.UserID,
			PublicKey: row.PublicKey,
			CreatedAt: row.CreatedAt,
		}
	}

	return views, tx.Commit(ctx)
}

// UpsertTeamSharedKeys upserts team shared keys.
func (s *keyService) UpsertTeamSharedKeys(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID, keys []TeamSharedKey) error {
	return s.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionFileUpload, func(queries *tenantdb.Queries) error {
		resolvedTeam, err := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{
			PublicID:     teamID,
			UserPublicID: p.UserID,
		})
		if err != nil {
			return err
		}

		for _, key := range keys {
			_, err = queries.UpsertTeamSharedKey(ctx, tenantdb.UpsertTeamSharedKeyParams{
				TeamID:       resolvedTeam.ID,
				UserID:       key.UserID,
				EncryptedKey: key.EncryptedKey,
				KeyVersion:   key.KeyVersion,
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// GetTeamSharedKeyForUser gets team shared key for user.
func (s *keyService) GetTeamSharedKeyForUser(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID, userID uuid.UUID, version int32) (*TeamSharedKey, error) {
	var key tenantdb.TeamSharedKey
	err := s.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionTeamRead, func(queries *tenantdb.Queries) error {
		resolvedTeam, err := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{
			PublicID:     teamID,
			UserPublicID: p.UserID,
		})
		if err != nil {
			return err
		}

		key, err = queries.GetTeamSharedKeyForUser(ctx, tenantdb.GetTeamSharedKeyForUserParams{
			TeamID:     resolvedTeam.ID,
			UserID:     userID,
			KeyVersion: version,
		})
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, authorization.ErrResourceNotFound
	}
	if err != nil {
		return nil, err
	}

	return &TeamSharedKey{
		TeamID:       teamID,
		UserID:       key.UserID,
		EncryptedKey: key.EncryptedKey,
		KeyVersion:   key.KeyVersion,
		CreatedAt:    key.CreatedAt,
	}, nil
}

// ListTeamSharedKeysForUser lists team shared keys for user.
func (s *keyService) ListTeamSharedKeysForUser(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID, userID uuid.UUID) ([]TeamSharedKey, error) {
	var rows []tenantdb.TeamSharedKey
	err := s.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionTeamRead, func(queries *tenantdb.Queries) error {
		resolvedTeam, err := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{
			PublicID:     teamID,
			UserPublicID: p.UserID,
		})
		if err != nil {
			return err
		}

		rows, err = queries.ListTeamSharedKeysForUser(ctx, tenantdb.ListTeamSharedKeysForUserParams{
			TeamID: resolvedTeam.ID,
			UserID: userID,
		})
		return err
	})
	if err != nil {
		return nil, err
	}

	views := make([]TeamSharedKey, len(rows))
	for i, row := range rows {
		views[i] = TeamSharedKey{
			TeamID:       teamID,
			UserID:       row.UserID,
			EncryptedKey: row.EncryptedKey,
			KeyVersion:   row.KeyVersion,
			CreatedAt:    row.CreatedAt,
		}
	}
	return views, nil
}
