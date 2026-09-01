package identity

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

const (
	PasswordNormalizationLegacyRaw = "legacy_raw"
	PasswordNormalizationNFCV1     = "nfc_v1"
	dummyCredentialInput           = "synodus-fixed-dummy-input-v1" // #nosec G101 -- public fixed input, never an account credential.
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

type CredentialCompareAndSwap struct {
	UserPublicID          uuid.UUID
	ExpectedHash          string
	ExpectedNormalization string
	NewHash               string
	NewNormalization      string
}

type CredentialStore interface {
	CredentialByCanonicalEmail(context.Context, string) (StoredCredential, error)
	CompareAndSwapCredential(context.Context, CredentialCompareAndSwap) (bool, error)
	CurrentCredentialByUserID(context.Context, uuid.UUID) (StoredCredential, error)
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

// NewCredentialVerifier creates a credential verifier.
func NewCredentialVerifier(ctx context.Context, store CredentialStore, hasher PasswordHasher) (CredentialVerifier, error) {
	if store == nil || hasher == nil {
		return nil, ErrCredentialDependency
	}
	dummyHash, err := hasher.Hash(ctx, dummyCredentialInput)
	if err != nil {
		return nil, err
	}
	return &credentialVerifier{
		store:     store,
		hasher:    hasher,
		dummyHash: dummyHash,
	}, nil
}

// Verify handles the verify operation.
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
	if credential.PasswordNormalization == PasswordNormalizationLegacyRaw || verification.NeedsRehash {
		return verifier.upgradeCredential(ctx, credential, password, normalizedPassword)
	}
	return CredentialPrincipal{UserPublicID: credential.UserPublicID}, nil
}

// upgradeCredential upgrades credential.
func (verifier *credentialVerifier) upgradeCredential(
	ctx context.Context,
	credential StoredCredential,
	rawPassword string,
	normalizedPassword string,
) (CredentialPrincipal, error) {
	newHash, err := verifier.hasher.Hash(ctx, normalizedPassword)
	if err != nil {
		return CredentialPrincipal{}, err
	}
	updated, err := verifier.store.CompareAndSwapCredential(ctx, CredentialCompareAndSwap{
		UserPublicID:          credential.UserPublicID,
		ExpectedHash:          credential.PasswordHash,
		ExpectedNormalization: credential.PasswordNormalization,
		NewHash:               newHash,
		NewNormalization:      PasswordNormalizationNFCV1,
	})
	if err != nil {
		return CredentialPrincipal{}, ErrCredentialDependency
	}
	if updated {
		return CredentialPrincipal{UserPublicID: credential.UserPublicID}, nil
	}

	current, err := verifier.store.CurrentCredentialByUserID(ctx, credential.UserPublicID)
	if err != nil {
		if errors.Is(err, ErrCredentialNotFound) {
			return CredentialPrincipal{}, ErrInvalidCredentials
		}
		return CredentialPrincipal{}, ErrCredentialDependency
	}
	if current.Deleted {
		return CredentialPrincipal{}, ErrInvalidCredentials
	}
	currentPassword := normalizedPassword
	switch current.PasswordNormalization {
	case PasswordNormalizationLegacyRaw:
		currentPassword = rawPassword
	case PasswordNormalizationNFCV1:
	default:
		return CredentialPrincipal{}, ErrInvalidCredentials
	}
	verification, err := verifier.hasher.Verify(ctx, currentPassword, current.PasswordHash)
	if err != nil || !verification.Match {
		return CredentialPrincipal{}, ErrInvalidCredentials
	}
	return CredentialPrincipal{UserPublicID: credential.UserPublicID}, nil
}

// denyWithDummy denys with dummy.
func (verifier *credentialVerifier) denyWithDummy(ctx context.Context) (CredentialPrincipal, error) {
	if _, err := verifier.hasher.Verify(ctx, dummyCredentialInput, verifier.dummyHash); err != nil {
		return CredentialPrincipal{}, err
	}
	return CredentialPrincipal{}, ErrInvalidCredentials
}
