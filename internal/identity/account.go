package identity

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/text/unicode/norm"
)

const maximumDisplayNameCodePoints = 100

var (
	ErrInvalidDisplayName = errors.New("invalid display name")
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrAccountDependency  = errors.New("account dependency unavailable")
)

func NormalizeDisplayName(value string) (string, error) {
	if !utf8.ValidString(value) || len(value) > 255 {
		return "", ErrInvalidDisplayName
	}
	normalized := norm.NFC.String(strings.TrimSpace(value))
	codePoints := utf8.RuneCountInString(normalized)
	if codePoints == 0 || codePoints > maximumDisplayNameCodePoints || len(normalized) > 255 {
		return "", ErrInvalidDisplayName
	}
	for _, character := range normalized {
		if unicode.IsControl(character) {
			return "", ErrInvalidDisplayName
		}
	}
	return normalized, nil
}

type LocalAccountCreator struct {
	queries *publicdb.Queries
	hasher  PasswordHasher
}

func NewLocalAccountCreator(queries *publicdb.Queries, hasher PasswordHasher) *LocalAccountCreator {
	return &LocalAccountCreator{queries: queries, hasher: hasher}
}

func (creator *LocalAccountCreator) Create(
	ctx context.Context,
	email string,
	displayName string,
	password string,
) (CredentialPrincipal, error) {
	canonical, err := (EmailCanonicalizer{}).Canonicalize(email)
	if err != nil {
		return CredentialPrincipal{}, err
	}
	normalizedDisplayName, err := NormalizeDisplayName(displayName)
	if err != nil {
		return CredentialPrincipal{}, err
	}
	normalizedPassword, err := (PasswordPolicy{}).Normalize(password)
	if err != nil {
		return CredentialPrincipal{}, err
	}
	passwordHash, err := creator.hasher.Hash(ctx, normalizedPassword)
	if err != nil {
		return CredentialPrincipal{}, err
	}
	user, err := creator.queries.CreateUser(ctx, publicdb.CreateUserParams{
		Email:                 canonical.Display,
		PasswordHash:          passwordHash,
		DisplayName:           normalizedDisplayName,
		PasswordNormalization: PasswordNormalizationNFCV1,
	})
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" &&
			postgresError.ConstraintName == "users_email_canonical_unique" {
			return CredentialPrincipal{}, ErrEmailAlreadyExists
		}
		return CredentialPrincipal{}, ErrAccountDependency
	}
	return CredentialPrincipal{UserPublicID: user.UserID}, nil
}
