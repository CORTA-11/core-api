package service

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params defines the configurable parameters for the Argon2id algorithm.
type Argon2Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultArgon2Params defines recommended Argon2id parameters based on OWASP guidelines.
var DefaultArgon2Params = Argon2Params{
	Memory:      65536, // 64 MB
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

type passwordService struct {
	params Argon2Params
}

// NewPasswordService creates a new instance of PasswordService.
func NewPasswordService() PasswordService {
	return &passwordService{
		params: DefaultArgon2Params,
	}
}

// HashPassword hashes a plain-text password using the Argon2id algorithm.
// It returns an encoded string representation in the standard Modular Crypt Format:
// $argon2id$v=19$m=65536,t=3,p=4$salt$hash
func (s *passwordService) HashPassword(password string) (string, error) {
	salt := make([]byte, s.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate random salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, s.params.Iterations, s.params.Memory, s.params.Parallelism, s.params.KeyLength)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encodedHash := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, s.params.Memory, s.params.Iterations, s.params.Parallelism, b64Salt, b64Hash)

	return encodedHash, nil
}

// VerifyPassword compares a plain-text password against a previously encoded Argon2id hash.
// It parses the parameters from the encoded hash to verify correct cryptographic comparison.
func (s *passwordService) VerifyPassword(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, errors.New("invalid Argon2id hash format")
	}

	if parts[1] != "argon2id" {
		return false, fmt.Errorf("unsupported argon2 variant: %s", parts[1])
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("failed to parse argon2 version: %w", err)
	}
	if version != argon2.Version {
		return false, fmt.Errorf("incompatible argon2 version: %d (expected %d)", version, argon2.Version)
	}

	var params Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.Memory, &params.Iterations, &params.Parallelism); err != nil {
		return false, fmt.Errorf("failed to parse argon2 params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("failed to decode argon2 salt: %w", err)
	}
	params.SaltLength = uint32(len(salt))

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("failed to decode argon2 hash: %w", err)
	}
	params.KeyLength = uint32(len(expectedHash))

	actualHash := argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, params.KeyLength)

	// ConstantTimeCompare prevents timing attacks.
	if subtle.ConstantTimeCompare(actualHash, expectedHash) == 1 {
		return true, nil
	}

	return false, nil
}
