package identity

import (
	"context"
	"errors"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/jackc/pgx/v5"
)

type PostgresCredentialStore struct {
	queries *publicdb.Queries
}

func NewPostgresCredentialStore(queries *publicdb.Queries) *PostgresCredentialStore {
	return &PostgresCredentialStore{queries: queries}
}

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
