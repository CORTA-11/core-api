package identity

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseArgon2IDHashAcceptsOnlyCanonicalBoundedEncoding(t *testing.T) {
	t.Parallel()

	valid := "$argon2id$v=19$m=65536,t=3,p=4$" +
		base64.RawStdEncoding.EncodeToString(make([]byte, 16)) + "$" +
		base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	parsed, err := parseArgon2IDHash(valid)
	require.NoError(t, err)
	assert.Equal(t, uint32(65536), parsed.memoryKiB)
	assert.Equal(t, uint32(3), parsed.iterations)
	assert.Equal(t, uint8(4), parsed.parallelism)
	assert.Len(t, parsed.salt, 16)
	assert.Len(t, parsed.digest, 32)
}

func TestParseArgon2IDHashRejectsHostileValues(t *testing.T) {
	t.Parallel()

	salt := base64.RawStdEncoding.EncodeToString(make([]byte, 16))
	digest := base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	valid := "$argon2id$v=19$m=65536,t=3,p=4$" + salt + "$" + digest
	tests := []struct {
		name string
		hash string
	}{
		{name: "oversized encoding", hash: strings.Repeat("x", maximumEncodedHashBytes+1)},
		{name: "wrong variant", hash: strings.Replace(valid, "argon2id", "argon2i", 1)},
		{name: "wrong version", hash: strings.Replace(valid, "v=19", "v=16", 1)},
		{name: "leading zero memory", hash: strings.Replace(valid, "m=65536", "m=065536", 1)},
		{name: "parameter order", hash: strings.Replace(valid, "m=65536,t=3,p=4", "t=3,m=65536,p=4", 1)},
		{name: "memory zero", hash: strings.Replace(valid, "m=65536", "m=0", 1)},
		{name: "memory ceiling", hash: strings.Replace(valid, "m=65536", "m=262145", 1)},
		{name: "iteration zero", hash: strings.Replace(valid, "t=3", "t=0", 1)},
		{name: "iteration ceiling", hash: strings.Replace(valid, "t=3", "t=11", 1)},
		{name: "parallelism zero", hash: strings.Replace(valid, "p=4", "p=0", 1)},
		{name: "parallelism ceiling", hash: strings.Replace(valid, "p=4", "p=17", 1)},
		{name: "padded salt", hash: strings.Replace(valid, salt, salt+"=", 1)},
		{name: "oversized salt", hash: strings.Replace(valid, salt, base64.RawStdEncoding.EncodeToString(make([]byte, 33)), 1)},
		{name: "empty salt", hash: strings.Replace(valid, "$"+salt+"$", "$$", 1)},
		{name: "oversized output", hash: strings.Replace(valid, digest, base64.RawStdEncoding.EncodeToString(make([]byte, 65)), 1)},
		{name: "empty output", hash: strings.TrimSuffix(valid, digest)},
		{name: "trailing field", hash: valid + "$extra"},
		{name: "noncanonical base64", hash: strings.Replace(valid, salt, strings.Repeat("_", len(salt)), 1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseArgon2IDHash(test.hash)
			assert.ErrorIs(t, err, ErrInvalidPasswordHash)
			assert.NotContains(t, err.Error(), test.hash)
		})
	}
}

func FuzzParseArgon2IDHashNeverPanics(f *testing.F) {
	f.Add("")
	f.Add("$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA")
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = parseArgon2IDHash(input)
	})
}
