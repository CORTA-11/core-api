// Package session implements PostgreSQL-authoritative browser sessions.
package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

const (
	tokenBytes        = 32
	tokenHashBytes    = sha256.Size
	encodedTokenBytes = 43
)

var ErrInvalidToken = errors.New("invalid session token")

type tokenCodec struct {
	random io.Reader
}

func (codec tokenCodec) issue() (string, []byte, error) {
	random := codec.random
	if random == nil {
		random = rand.Reader
	}
	raw := make([]byte, tokenBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", nil, err
	}
	return base64.RawURLEncoding.EncodeToString(raw), raw, nil
}

func parseToken(encoded string) ([]byte, error) {
	if len(encoded) != encodedTokenBytes {
		return nil, ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(raw) != tokenBytes || base64.RawURLEncoding.EncodeToString(raw) != encoded {
		return nil, ErrInvalidToken
	}
	return raw, nil
}

func hashToken(raw []byte) [tokenHashBytes]byte {
	return sha256.Sum256(raw)
}
