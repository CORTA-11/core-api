package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type verifierStoreStub struct {
	record StoredCredential
	err    error
	keys   []string
}

func (store *verifierStoreStub) CredentialByCanonicalEmail(_ context.Context, key string) (StoredCredential, error) {
	store.keys = append(store.keys, key)
	return store.record, store.err
}

type verifierHasherStub struct {
	workCalls    int
	passwords    []string
	malformedHit int
}

func (hasher *verifierHasherStub) Hash(context.Context, string) (string, error) {
	return "process-dummy-hash", nil
}

func (hasher *verifierHasherStub) Verify(_ context.Context, password, encoded string) (PasswordVerification, error) {
	hasher.passwords = append(hasher.passwords, password)
	if encoded == "malformed" {
		hasher.malformedHit++
		return PasswordVerification{}, ErrInvalidPasswordHash
	}
	hasher.workCalls++
	switch encoded {
	case "stored-match":
		return PasswordVerification{Match: password == "re\u0301search-password-value" || password == "r\u00e9search-password-value"}, nil
	case "process-dummy-hash", "stored-wrong":
		return PasswordVerification{}, nil
	default:
		return PasswordVerification{}, errors.New("unexpected encoded hash")
	}
}

func TestCredentialVerifierReturnsPrincipalWithoutClaims(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	store := &verifierStoreStub{record: StoredCredential{
		UserPublicID:          userID,
		PasswordHash:          "stored-match",
		PasswordNormalization: PasswordNormalizationNFCV1,
	}}
	hasher := &verifierHasherStub{}
	verifier, err := NewCredentialVerifier(context.Background(), store, hasher)
	require.NoError(t, err)

	principal, err := verifier.Verify(context.Background(), " User@Example.COM ", "re\u0301search-password-value")
	require.NoError(t, err)
	assert.Equal(t, CredentialPrincipal{UserPublicID: userID}, principal)
	assert.Equal(t, []string{"user@example.com"}, store.keys)
	assert.Equal(t, []string{"r\u00e9search-password-value"}, hasher.passwords)
	assert.Equal(t, 1, hasher.workCalls)
}

func TestCredentialVerifierDeniesUniformlyWithOneWorkUnit(t *testing.T) {
	t.Parallel()

	validPassword := "research-password-value"
	tests := []struct {
		name     string
		email    string
		password string
		record   StoredCredential
		storeErr error
	}{
		{name: "unknown account", email: "unknown@example.com", password: validPassword, storeErr: ErrCredentialNotFound},
		{name: "deleted account", email: "user@example.com", password: validPassword, record: StoredCredential{Deleted: true, PasswordHash: "stored-match", PasswordNormalization: PasswordNormalizationNFCV1}},
		{name: "malformed hash", email: "user@example.com", password: validPassword, record: StoredCredential{PasswordHash: "malformed", PasswordNormalization: PasswordNormalizationNFCV1}},
		{name: "wrong password", email: "user@example.com", password: validPassword, record: StoredCredential{PasswordHash: "stored-wrong", PasswordNormalization: PasswordNormalizationNFCV1}},
		{name: "invalid email", email: "user\n@example.com", password: validPassword},
		{name: "invalid password", email: "user@example.com", password: "too-short"},
		{name: "unknown normalization", email: "user@example.com", password: validPassword, record: StoredCredential{PasswordHash: "stored-match", PasswordNormalization: "future_v2"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := &verifierStoreStub{record: test.record, err: test.storeErr}
			hasher := &verifierHasherStub{}
			verifier, err := NewCredentialVerifier(context.Background(), store, hasher)
			require.NoError(t, err)

			principal, err := verifier.Verify(context.Background(), test.email, test.password)
			assert.Equal(t, CredentialPrincipal{}, principal)
			assert.ErrorIs(t, err, ErrInvalidCredentials)
			assert.Equal(t, ErrInvalidCredentials.Error(), err.Error())
			assert.Equal(t, 1, hasher.workCalls)
		})
	}
}

func TestCredentialVerifierLegacyRawUsesExactHistoricalBytes(t *testing.T) {
	t.Parallel()

	store := &verifierStoreStub{record: StoredCredential{
		UserPublicID:          uuid.New(),
		PasswordHash:          "stored-match",
		PasswordNormalization: PasswordNormalizationLegacyRaw,
	}}
	hasher := &verifierHasherStub{}
	verifier, err := NewCredentialVerifier(context.Background(), store, hasher)
	require.NoError(t, err)

	_, err = verifier.Verify(context.Background(), "user@example.com", "re\u0301search-password-value")
	require.NoError(t, err)
	assert.Equal(t, "re\u0301search-password-value", hasher.passwords[0])
}
