package dbroles

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigRequiresBootstrapURLAndRolePasswordsWithoutLeakingThem(t *testing.T) {
	t.Parallel()

	secret := "do-not-print-role-secret"
	values := map[string]string{
		"BOOTSTRAP_DATABASE_URL":  "postgres://admin:" + secret + "@db/app",
		"DB_RUNTIME_PASSWORD":     secret,
		"DB_MIGRATOR_PASSWORD":    "",
		"DB_PROVISIONER_PASSWORD": secret,
	}
	_, err := LoadConfig(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "DB_MIGRATOR_PASSWORD")
	assert.NotContains(t, err.Error(), secret)
}

func TestLoadConfigAcceptsCompleteRoleBootstrapConfiguration(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"BOOTSTRAP_DATABASE_URL":  "postgres://admin:secret@db/app",
		"DB_RUNTIME_PASSWORD":     "runtime-secret",
		"DB_MIGRATOR_PASSWORD":    "migrator-secret",
		"DB_PROVISIONER_PASSWORD": "provisioner-secret",
	}
	config, err := LoadConfig(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(config.BootstrapDatabaseURL, "postgres://"))
	assert.Equal(t, "runtime-secret", config.RuntimePassword)
}
