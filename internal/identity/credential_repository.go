package identity

import (
	"context"
	"errors"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PostgresCredentialStore struct {
	queries *publicdb.Queries
}

// NewPostgresCredentialStore creates a postgres credential store.
func NewPostgresCredentialStore(queries *publicdb.Queries) *PostgresCredentialStore {
	return &PostgresCredentialStore{queries: queries}
}

// CredentialByCanonicalEmail credentials by canonical email.
func (store *PostgresCredentialStore) CredentialByCanonicalEmail(
	ctx context.Context,
	canonicalEmail string,
) (StoredCredential, error) {
	row, err := store.queries.GetCredentialByCanonicalEmail(ctx, canonicalEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredCredential{}, ErrCredentialNotFound
	}
	if err != nil {
		return StoredCredential{}, ErrCredentialDependency
	}
	return StoredCredential{
		UserPublicID:          row.UserID,
		PasswordHash:          row.PasswordHash,
		PasswordNormalization: row.PasswordNormalization,
		Deleted:               row.DeletedAt.Valid,
	}, nil
}

// CompareAndSwapCredential compares and swap credential.
func (store *PostgresCredentialStore) CompareAndSwapCredential(
	ctx context.Context,
	update CredentialCompareAndSwap,
) (bool, error) {
	rowsAffected, err := store.queries.CompareAndSwapCredential(ctx, publicdb.CompareAndSwapCredentialParams{
		NewHash:               update.NewHash,
		NewNormalization:      update.NewNormalization,
		UserID:                update.UserPublicID,
		ExpectedHash:          update.ExpectedHash,
		ExpectedNormalization: update.ExpectedNormalization,
	})
	if err != nil {
		return false, ErrCredentialDependency
	}
	return rowsAffected == 1, nil
}

// CurrentCredentialByUserID currents credential by user id.
func (store *PostgresCredentialStore) CurrentCredentialByUserID(
	ctx context.Context,
	userID uuid.UUID,
) (StoredCredential, error) {
	row, err := store.queries.GetCurrentCredentialByUserID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredCredential{}, ErrCredentialNotFound
	}
	if err != nil {
		return StoredCredential{}, ErrCredentialDependency
	}
	return StoredCredential{
		UserPublicID:          row.UserID,
		PasswordHash:          row.PasswordHash,
		PasswordNormalization: row.PasswordNormalization,
		Deleted:               row.DeletedAt.Valid,
	}, nil
}
