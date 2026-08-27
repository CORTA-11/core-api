package httpx

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrustedClientIgnoresForwardingFromUntrustedPeer(t *testing.T) {
	policy, err := ParseTrustedProxies("")
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodGet, "http://api.example.test/resource", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("Forwarded", "for=203.0.113.9;proto=https;host=evil.example")
	request.Header.Set("X-Forwarded-For", "198.51.100.2")

	client, err := policy.Derive(request)
	require.NoError(t, err)
	assert.Equal(t, netip.MustParseAddr("192.0.2.10"), client.Address)
	assert.Equal(t, "http", client.Scheme)
	assert.Equal(t, "api.example.test", client.Host)
}

func TestTrustedClientRejectsMixedForwardingFamilies(t *testing.T) {
	policy, err := ParseTrustedProxies("10.0.0.0/8")
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodGet, "http://internal/", nil)
	request.RemoteAddr = "10.0.0.2:1234"
	request.Header.Set("Forwarded", "for=203.0.113.9")
	request.Header.Set("X-Forwarded-For", "203.0.113.9")
	_, err = policy.Derive(request)
	require.Error(t, err)
}

func TestTrustedClientWalksForwardedChainRightToLeft(t *testing.T) {
	policy, err := ParseTrustedProxies("10.0.0.0/8")
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodGet, "http://internal/", nil)
	request.RemoteAddr = "10.0.0.2:1234"
	request.Header.Set("Forwarded", `for=198.51.100.7;proto=https;host=api.example.com, for="[::ffff:10.0.0.3]:443"`)
	client, err := policy.Derive(request)
	require.NoError(t, err)
	assert.Equal(t, netip.MustParseAddr("198.51.100.7"), client.Address)
	assert.Equal(t, "https", client.Scheme)
	assert.Equal(t, "api.example.com", client.Host)
}

func TestTrustedClientSupportsXForwardedFamily(t *testing.T) {
	policy, err := ParseTrustedProxies("10.0.0.0/8")
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodGet, "http://internal/", nil)
	request.RemoteAddr = "10.0.0.2:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.8, 10.0.0.3")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "api.example.com")
	client, err := policy.Derive(request)
	require.NoError(t, err)
	assert.Equal(t, netip.MustParseAddr("198.51.100.8"), client.Address)
	assert.Equal(t, "https", client.Scheme)
	assert.Equal(t, "api.example.com", client.Host)
}

func TestTrustedClientRejectsMalformedOrOverlongTrustedChains(t *testing.T) {
	policy, err := ParseTrustedProxies("10.0.0.0/8")
	require.NoError(t, err)
	for _, forwarded := range []string{
		"for=not-an-ip",
		strings.Repeat("for=10.0.0.1,", 10) + "for=198.51.100.1",
	} {
		request := httptest.NewRequest(http.MethodGet, "http://internal/", nil)
		request.RemoteAddr = "10.0.0.2:1234"
		request.Header.Set("Forwarded", forwarded)
		_, err := policy.Derive(request)
		require.Error(t, err)
	}
}

func TestTrustedClientMiddlewareStoresOneTypedValue(t *testing.T) {
	policy, err := ParseTrustedProxies("")
	require.NoError(t, err)
	var got Client
	handler := policy.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		got, _ = ClientFromContext(request.Context())
	}))
	request := httptest.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	request.RemoteAddr = "[::ffff:192.0.2.4]:1234"
	handler.ServeHTTP(httptest.NewRecorder(), request)
	assert.Equal(t, netip.MustParseAddr("192.0.2.4"), got.Address)
}
