package session

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenCodecIssuesAndStrictlyParsesCanonicalTokens(t *testing.T) {
	t.Parallel()

	codec := tokenCodec{random: bytes.NewReader(bytes.Repeat([]byte{0xa5}, tokenBytes))}
	token, raw, err := codec.issue()
	require.NoError(t, err)
	assert.Len(t, raw, tokenBytes)
	assert.Len(t, token, encodedTokenBytes)
	assert.NotContains(t, token, "=")

	parsed, err := parseToken(token)
	require.NoError(t, err)
	assert.Equal(t, raw, parsed)

	for _, invalid := range []string{
		"", token + "=", token[:len(token)-1], token + "a",
		"///////////////////////////////////////////", // standard rather than URL-safe alphabet
	} {
		_, err := parseToken(invalid)
		assert.ErrorIs(t, err, ErrInvalidToken, invalid)
	}
}

func TestTokenCodecHashesRawToken(t *testing.T) {
	t.Parallel()

	raw := bytes.Repeat([]byte{0x11}, tokenBytes)
	first := hashToken(raw)
	second := hashToken(raw)
	assert.Equal(t, first, second)
	assert.Len(t, first, tokenHashBytes)
	assert.NotEqual(t, raw, first[:])
}
