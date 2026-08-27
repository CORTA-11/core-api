package session

import (
	"context"
	"errors"

	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/jackc/pgx/v5"
)

// ChangePassword performs expensive password verification and hashing before
// opening the transaction. The credential CAS, revocation, and replacement
// session insert then commit as one database operation.
func (manager *Manager) ChangePassword(
	ctx context.Context,
	authentication Authentication,
	currentPassword string,
	newPassword string,
	userAgent string,
	hasher identity.PasswordHasher,
) (IssuedSession, error) {
	if hasher == nil || authentication.Principal.UserID == [16]byte{} {
		return IssuedSession{}, ErrSessionDependency
	}
	credential, err := manager.queries.GetCurrentCredentialByUserID(ctx, authentication.Principal.UserID)
	if errors.Is(err, pgx.ErrNoRows) || credential.DeletedAt.Valid {
		return IssuedSession{}, identity.ErrInvalidCredentials
	}
	if err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	passwordForVerification := currentPassword
	policy := identity.PasswordPolicy{}
	normalizedCurrent, normalizationErr := policy.Normalize(currentPassword)
	switch credential.PasswordNormalization {
	case identity.PasswordNormalizationLegacyRaw:
	case identity.PasswordNormalizationNFCV1:
		if normalizationErr != nil {
			return IssuedSession{}, identity.ErrInvalidCredentials
		}
		passwordForVerification = normalizedCurrent
	default:
		return IssuedSession{}, identity.ErrInvalidCredentials
	}
	verification, err := hasher.Verify(ctx, passwordForVerification, credential.PasswordHash)
	if errors.Is(err, identity.ErrInvalidPasswordHash) || (err == nil && !verification.Match) {
		return IssuedSession{}, identity.ErrInvalidCredentials
	}
	if err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	normalizedNew, err := policy.Normalize(newPassword)
	if err != nil {
		return IssuedSession{}, err
	}
	newHash, err := hasher.Hash(ctx, normalizedNew)
	if err != nil {
		return IssuedSession{}, err
	}
	token, raw, err := manager.codec.issue()
	if err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	tokenHash := hashToken(raw)
	now := manager.now()

	tx, err := manager.database.Begin(ctx)
	if err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := manager.queries.WithTx(tx)
	rows, err := queries.CompareAndSwapCredential(ctx, publicdb.CompareAndSwapCredentialParams{
		NewHash: newHash, NewNormalization: identity.PasswordNormalizationNFCV1,
		UserID: authentication.Principal.UserID, ExpectedHash: credential.PasswordHash,
		ExpectedNormalization: credential.PasswordNormalization,
	})
	if err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	if rows != 1 {
		return IssuedSession{}, ErrCredentialChanged
	}
	if err := queries.RevokeAllUserSessions(ctx, publicdb.RevokeAllUserSessionsParams{
		Now: pgTimestamp(now), UserPublicID: authentication.Principal.UserID,
	}); err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	row, err := queries.CreateSession(ctx, publicdb.CreateSessionParams{
		TokenHash: tokenHash[:], UserAgent: NormalizeUserAgent(userAgent), Now: now,
		AbsoluteExpiresAt: now.Add(AbsoluteLifetime), UserPublicID: authentication.Principal.UserID,
	})
	if err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	if err := tx.Commit(ctx); err != nil {
		return IssuedSession{}, ErrSessionDependency
	}
	committed := Authentication{
		Principal: Principal{UserID: authentication.Principal.UserID, SessionID: row.SessionID},
		User:      authentication.User,
		Session: metadata(row.SessionID, row.UserAgent, row.CreatedAt, row.LastSeenAt,
			row.AbsoluteExpiresAt, nil, true),
		rawToken: append([]byte(nil), raw...),
	}
	return IssuedSession{
		Authentication: committed, RawToken: token, CSRFToken: manager.csrf.Derive(raw),
		PublicID: row.SessionID, ExpiresAt: row.AbsoluteExpiresAt,
	}, nil
}
