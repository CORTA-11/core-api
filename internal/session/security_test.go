package session

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCSRFIsTokenBoundAndComparedStrictly(t *testing.T) {
	t.Parallel()

	protector, err := NewCSRFProtector(bytes.Repeat([]byte{0x42}, 32))
	require.NoError(t, err)
	raw := bytes.Repeat([]byte{0x24}, tokenBytes)
	token := protector.Derive(raw)
	assert.NotContains(t, token, "=")
	assert.True(t, protector.Valid(raw, token))
	assert.False(t, protector.Valid(append([]byte(nil), raw[:31]...), token))
	assert.False(t, protector.Valid(raw, token+"="))
	assert.False(t, protector.Valid(raw, token[:len(token)-1]+"A"))
}

func TestCookiePolicySeparatesProductionAndDevelopment(t *testing.T) {
	t.Parallel()

	production := CookiePolicy("production")
	assert.Equal(t, "__Host-synodus_session", production.Name)
	assert.True(t, production.Secure)
	assert.True(t, production.HttpOnly)
	assert.Equal(t, "/", production.Path)
	assert.Empty(t, production.Domain)
	assert.Equal(t, http.SameSiteLaxMode, production.SameSite)

	development := CookiePolicy("development")
	assert.Equal(t, "synodus_dev_session", development.Name)
	assert.False(t, development.Secure)
	assert.True(t, development.HttpOnly)
}

func TestNormalizeUserAgentUsesNFCAndCapsBytes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Caf\u00e9", NormalizeUserAgent("Cafe\u0301"))
	normalized := NormalizeUserAgent(string(bytes.Repeat([]byte("界"), 100)))
	assert.LessOrEqual(t, len(normalized), maximumUserAgentBytes)
	assert.True(t, len(normalized) > 0)
}
