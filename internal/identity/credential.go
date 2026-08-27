package identity

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

const (
	PasswordNormalizationLegacyRaw = "legacy_raw"
	PasswordNormalizationNFCV1     = "nfc_v1"
	dummyPassword                  = "synodus-process-dummy-password"
)

var (
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrCredentialNotFound   = errors.New("credential not found")
	ErrCredentialDependency = errors.New("credential dependency unavailable")
)

type CredentialPrincipal struct {
	UserPublicID uuid.UUID
}

type StoredCredential struct {
	UserPublicID          uuid.UUID
	PasswordHash          string
	PasswordNormalization string
	Deleted               bool
}

type CredentialStore interface {
	CredentialByCanonicalEmail(context.Context, string) (StoredCredential, error)
}

type CredentialVerifier interface {
	Verify(context.Context, string, string) (CredentialPrincipal, error)
}

type credentialVerifier struct {
	store         CredentialStore
	hasher        PasswordHasher
	canonicalizer EmailCanonicalizer
	policy        PasswordPolicy
	dummyHash     string
}

func NewCredentialVerifier(ctx context.Context, store CredentialStore, hasher PasswordHasher) (CredentialVerifier, error) {
	if store == nil || hasher == nil {
		return nil, ErrCredentialDependency
	}
	dummyHash, err := hasher.Hash(ctx, dummyPassword)
	if err != nil {
		return nil, err
	}
	return &credentialVerifier{
		store:     store,
		hasher:    hasher,
		dummyHash: dummyHash,
	}, nil
}

func (verifier *credentialVerifier) Verify(
	ctx context.Context,
	email string,
	password string,
) (CredentialPrincipal, error) {
	canonical, emailErr := verifier.canonicalizer.Canonicalize(email)
	normalizedPassword, passwordErr := verifier.policy.Normalize(password)
	if emailErr != nil || passwordErr != nil {
		return verifier.denyWithDummy(ctx)
	}

	credential, err := verifier.store.CredentialByCanonicalEmail(ctx, canonical.Key)
	if err != nil {
		if errors.Is(err, ErrCredentialNotFound) {
			return verifier.denyWithDummy(ctx)
		}
		return CredentialPrincipal{}, ErrCredentialDependency
	}
	if credential.Deleted {
		return verifier.denyWithDummy(ctx)
	}

	passwordForVerification := normalizedPassword
	switch credential.PasswordNormalization {
	case PasswordNormalizationLegacyRaw:
		passwordForVerification = password
	case PasswordNormalizationNFCV1:
	default:
		return verifier.denyWithDummy(ctx)
	}

	verification, err := verifier.hasher.Verify(ctx, passwordForVerification, credential.PasswordHash)
	if errors.Is(err, ErrInvalidPasswordHash) {
		return verifier.denyWithDummy(ctx)
	}
	if err != nil {
		return CredentialPrincipal{}, err
	}
	if !verification.Match {
		return CredentialPrincipal{}, ErrInvalidCredentials
	}
	return CredentialPrincipal{UserPublicID: credential.UserPublicID}, nil
}

func (verifier *credentialVerifier) denyWithDummy(ctx context.Context) (CredentialPrincipal, error) {
	if _, err := verifier.hasher.Verify(ctx, dummyPassword, verifier.dummyHash); err != nil {
		return CredentialPrincipal{}, err
	}
	return CredentialPrincipal{}, ErrInvalidCredentials
}
