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
	assert.Equal(t, 15*time.Second, config.HTTPReadTimeout)
	assert.False(t, config.PprofEnabled)
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
	values["PPROF_ENABLED"] = "true"
	_, err := loadMap(values)
	require.Error(t, err)
	assert.ErrorContains(t, err, "JWT_SECRET")
	assert.ErrorContains(t, err, "PPROF_ENABLED")
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
