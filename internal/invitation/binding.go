// Package invitation owns the cryptographic representation of organization invitations.
package invitation

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"

	"github.com/CORTA-11/core-api/internal/identity"
)

const TokenBytes = 32

var ErrInvalidToken = errors.New("invalid invitation token")

type Binding struct{ secret []byte }

func NewBinding(secret []byte) (*Binding, error) {
	if len(secret) < 32 {
		return nil, errors.New("invitation binding secret must contain at least 32 bytes")
	}
	return &Binding{secret: append([]byte(nil), secret...)}, nil
}

func (binding *Binding) EmailFingerprint(email string) ([]byte, error) {
	canonical, err := (identity.EmailCanonicalizer{}).Canonicalize(email)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, binding.secret)
	_, _ = mac.Write([]byte(canonical.Key))
	return mac.Sum(nil), nil
}

func (binding *Binding) GenerateToken(random io.Reader) (string, []byte, error) {
	raw := make([]byte, TokenBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256(raw)
	return token, hash[:], nil
}

func (binding *Binding) ParseToken(token string) ([]byte, []byte, error) {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(raw) != TokenBytes || base64.RawURLEncoding.EncodeToString(raw) != token {
		return nil, nil, ErrInvalidToken
	}
	hash := sha256.Sum256(raw)
	return raw, hash[:], nil
}
