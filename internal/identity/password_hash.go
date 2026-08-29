package identity

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
)

const (
	maximumEncodedHashBytes  = 192
	maximumArgon2MemoryKiB   = 256 * 1024
	maximumArgon2Iterations  = 10
	maximumArgon2Parallel    = 16
	maximumArgon2SaltBytes   = 32
	maximumArgon2OutputBytes = 64
)

// ErrInvalidPasswordHash identifies malformed or over-capacity stored state
// without echoing any portion of that state.
var ErrInvalidPasswordHash = errors.New("invalid password hash")

type parsedArgon2IDHash struct {
	memoryKiB   uint32
	iterations  uint32
	parallelism uint8
	salt        []byte
	digest      []byte
}

// parseArgon2IDHash parses argon 2 idha sh.
func parseArgon2IDHash(encoded string) (parsedArgon2IDHash, error) {
	if len(encoded) == 0 || len(encoded) > maximumEncodedHashBytes {
		return parsedArgon2IDHash{}, ErrInvalidPasswordHash
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return parsedArgon2IDHash{}, ErrInvalidPasswordHash
	}

	parameterParts := strings.Split(parts[3], ",")
	if len(parameterParts) != 3 {
		return parsedArgon2IDHash{}, ErrInvalidPasswordHash
	}
	memory, ok := parseCanonicalUint(parameterParts[0], "m=", maximumArgon2MemoryKiB)
	if !ok {
		return parsedArgon2IDHash{}, ErrInvalidPasswordHash
	}
	iterations, ok := parseCanonicalUint(parameterParts[1], "t=", maximumArgon2Iterations)
	if !ok {
		return parsedArgon2IDHash{}, ErrInvalidPasswordHash
	}
	parallelism, ok := parseCanonicalUint(parameterParts[2], "p=", maximumArgon2Parallel)
	if !ok {
		return parsedArgon2IDHash{}, ErrInvalidPasswordHash
	}

	salt, ok := decodeBoundedBase64(parts[4], maximumArgon2SaltBytes)
	if !ok {
		return parsedArgon2IDHash{}, ErrInvalidPasswordHash
	}
	digest, ok := decodeBoundedBase64(parts[5], maximumArgon2OutputBytes)
	if !ok {
		return parsedArgon2IDHash{}, ErrInvalidPasswordHash
	}

	return parsedArgon2IDHash{
		memoryKiB:   uint32(memory),     // #nosec G115 -- parser ceiling is 256 MiB.
		iterations:  uint32(iterations), // #nosec G115 -- parser ceiling is ten.
		parallelism: uint8(parallelism), // #nosec G115 -- parser ceiling is sixteen.
		salt:        salt,
		digest:      digest,
	}, nil
}

// parseCanonicalUint parses canonical uint.
func parseCanonicalUint(field, prefix string, maximum uint64) (uint64, bool) {
	valueText, ok := strings.CutPrefix(field, prefix)
	if !ok || valueText == "" || (len(valueText) > 1 && valueText[0] == '0') {
		return 0, false
	}
	for _, character := range valueText {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	value, err := strconv.ParseUint(valueText, 10, 32)
	if err != nil || value == 0 || value > maximum || strconv.FormatUint(value, 10) != valueText {
		return 0, false
	}
	return value, true
}

// decodeBoundedBase64 decodes bounded base 64.
func decodeBoundedBase64(encoded string, maximumBytes int) ([]byte, bool) {
	if len(encoded) < 2 || len(encoded) > base64.RawStdEncoding.EncodedLen(maximumBytes) {
		return nil, false
	}
	decoded, err := base64.RawStdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(decoded) == 0 || len(decoded) > maximumBytes ||
		base64.RawStdEncoding.EncodeToString(decoded) != encoded {
		return nil, false
	}
	return decoded, true
}
