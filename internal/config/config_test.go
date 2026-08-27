package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validEnvironment() map[string]string {
	return map[string]string{
		"DATABASE_URL":      "postgres://user:database-secret@db/app",
		"REDIS_URL":         "redis://redis:6379/0",
		"MINIO_ENDPOINT":    "minio:9000",
		"MINIO_ACCESS_KEY":  "access-key",
		"MINIO_SECRET_KEY":  "storage-secret",
		"MINIO_BUCKET_NAME": "files",
	}
}

func loadMap(values map[string]string) (Config, error) {
	return LoadFrom(func(name string) (string, bool) { value, ok := values[name]; return value, ok })
}

func TestLoadUsesSafeDevelopmentDefaults(t *testing.T) {
	config, err := loadMap(validEnvironment())
	require.NoError(t, err)
	assert.Equal(t, "development", config.Environment)
	assert.Equal(t, ":8080", config.HTTPAddr)
	assert.Equal(t, DevelopmentJWTSecret, config.JWTSecret)
	assert.Equal(t, DevelopmentCSRFSecret, config.CSRFSecret)
	assert.Equal(t, DevelopmentCursorKeyID, config.Cursor.ActiveKeyID)
	assert.Equal(t, DevelopmentCursorSecret, config.Cursor.ActiveSecret)
	assert.Equal(t, 15*time.Second, config.HTTPReadTimeout)
	assert.Equal(t, []string{"http://localhost:3000", "http://127.0.0.1:3000"}, config.HTTPOrigins.Values())
	assert.Empty(t, config.TrustedProxies.CIDRs())
	assert.False(t, config.PprofEnabled)
}

func TestLoadValidatesExactOrigins(t *testing.T) {
	tests := []string{
		"*", "null", "https://example.com,https://example.com", "https://user@example.com",
		"https://example.com/path", "https://example.com?query=x", "https://example.com#fragment",
		"http://example.com",
	}
	for _, origins := range tests {
		t.Run(origins, func(t *testing.T) {
			values := validEnvironment()
			values["HTTP_ALLOWED_ORIGINS"] = origins
			_, err := loadMap(values)
			require.Error(t, err)
			assert.ErrorContains(t, err, "HTTP_ALLOWED_ORIGINS")
		})
	}
}

func TestLoadRejectsNonHTTPSProductionOrigin(t *testing.T) {
	values := validEnvironment()
	values["APP_ENV"] = "production"
	values["JWT_SECRET"] = strings.Repeat("j", 32)
	values["CSRF_SECRET"] = strings.Repeat("c", 32)
	values["CURSOR_SECRET"] = strings.Repeat("u", 32)
	values["HTTP_ALLOWED_ORIGINS"] = "http://localhost:3000"
	_, err := loadMap(values)
	require.Error(t, err)
	assert.ErrorContains(t, err, "HTTP_ALLOWED_ORIGINS")
}

func TestLoadValidatesTrustedProxyCIDRs(t *testing.T) {
	values := validEnvironment()
	values["HTTP_TRUSTED_PROXY_CIDRS"] = "10.0.0.0/8,2001:db8::/32"
	config, err := loadMap(values)
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.0/8", "2001:db8::/32"}, config.TrustedProxies.CIDRs())

	values["HTTP_TRUSTED_PROXY_CIDRS"] = "10.0.0.1,not-a-cidr"
	_, err = loadMap(values)
	require.Error(t, err)
	assert.ErrorContains(t, err, "HTTP_TRUSTED_PROXY_CIDRS")
}

func TestLoadRequiresDependencies(t *testing.T) {
	_, err := loadMap(map[string]string{})
	require.Error(t, err)
	for _, name := range []string{"DATABASE_URL", "REDIS_URL", "MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_BUCKET_NAME"} {
		assert.ErrorContains(t, err, name)
	}
}

func TestLoadValidatesTimeoutsAndBooleans(t *testing.T) {
	values := validEnvironment()
	values["HTTP_READ_TIMEOUT"] = "forever"
	values["SHUTDOWN_TIMEOUT"] = "0s"
	values["PPROF_ENABLED"] = "sometimes"
	_, err := loadMap(values)
	require.Error(t, err)
	assert.ErrorContains(t, err, "HTTP_READ_TIMEOUT")
	assert.ErrorContains(t, err, "SHUTDOWN_TIMEOUT")
	assert.ErrorContains(t, err, "PPROF_ENABLED")
}

func TestLoadRejectsUnsafeProductionSettings(t *testing.T) {
	values := validEnvironment()
	values["APP_ENV"] = "production"
	values["JWT_SECRET"] = DevelopmentJWTSecret
	values["CSRF_SECRET"] = DevelopmentCSRFSecret
	values["PPROF_ENABLED"] = "true"
	_, err := loadMap(values)
	require.Error(t, err)
	assert.ErrorContains(t, err, "JWT_SECRET")
	assert.ErrorContains(t, err, "CSRF_SECRET")
	assert.ErrorContains(t, err, "CURSOR_SECRET")
	assert.ErrorContains(t, err, "PPROF_ENABLED")
}

func TestLoadRequiresPairedDistinctPreviousCursorKey(t *testing.T) {
	values := validEnvironment()
	values["CURSOR_PREVIOUS_KEY_ID"] = "previous"
	_, err := loadMap(values)
	require.Error(t, err)
	assert.ErrorContains(t, err, "CURSOR_PREVIOUS_SECRET")

	values["CURSOR_PREVIOUS_SECRET"] = strings.Repeat("p", 32)
	values["CURSOR_KEY_ID"] = "previous"
	_, err = loadMap(values)
	require.Error(t, err)
	assert.ErrorContains(t, err, "CURSOR_PREVIOUS_KEY_ID")
}

func TestLoadRejectsMalformedCursorKeyConfiguration(t *testing.T) {
	values := validEnvironment()
	values["CURSOR_KEY_ID"] = "contains.a.dot"
	values["CURSOR_SECRET"] = "short"
	_, err := loadMap(values)
	require.Error(t, err)
	assert.ErrorContains(t, err, "cursor key IDs and secrets")
}

func TestLoadRejectsReusedProductionCursorSecrets(t *testing.T) {
	values := validEnvironment()
	values["APP_ENV"] = "production"
	values["JWT_SECRET"] = strings.Repeat("j", 32)
	values["CSRF_SECRET"] = strings.Repeat("c", 32)
	values["CURSOR_SECRET"] = strings.Repeat("c", 32)
	_, err := loadMap(values)
	require.Error(t, err)
	assert.ErrorContains(t, err, "CURSOR_SECRET")
}

func TestLoadRejectsReusedProductionCSRFSecret(t *testing.T) {
	values := validEnvironment()
	values["APP_ENV"] = "production"
	values["JWT_SECRET"] = strings.Repeat("j", 32)
	values["CSRF_SECRET"] = strings.Repeat("j", 32)
	_, err := loadMap(values)
	require.Error(t, err)
	assert.ErrorContains(t, err, "CSRF_SECRET")

	values["DATABASE_URL"] = "postgres://user:database-secret-database-secret-xx@db/app"
	values["CSRF_SECRET"] = "database-secret-database-secret-xx"
	_, err = loadMap(values)
	require.Error(t, err)
	assert.ErrorContains(t, err, "CSRF_SECRET")
}

func TestLoadRejectsLegacyDevelopmentCredentialInProduction(t *testing.T) {
	values := validEnvironment()
	values["APP_ENV"] = "production"
	values["JWT_SECRET"] = "your-super-secret-key-change-in-production"
	_, err := loadMap(values)
	require.Error(t, err)
	assert.ErrorContains(t, err, "JWT_SECRET")
}

func TestLoadErrorsDoNotExposeSecrets(t *testing.T) {
	values := validEnvironment()
	secret := "do-not-print-this-secret"
	values["APP_ENV"] = "production"
	values["JWT_SECRET"] = secret
	values["MINIO_USE_SSL"] = secret
	_, err := loadMap(values)
	require.Error(t, err)
	assert.False(t, strings.Contains(err.Error(), secret))
}
