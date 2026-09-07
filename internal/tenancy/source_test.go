package tenancy

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeLookupBuildsProvisioningURLFromSecretFile(t *testing.T) {
	secretDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(secretDir, "db_provisioner_password.txt"), []byte("p@ss/word\n"), 0o600))
	values := map[string]string{
		"LOCAL_SECRETS_DIR": secretDir,
		"DB_HOST":           "postgres",
		"DB_PORT":           "5432",
		"DB_NAME":           "appdb",
	}
	lookup := RuntimeLookup(func(name string) (string, bool) { value, ok := values[name]; return value, ok })

	raw, ok := lookup("PROVISIONING_DATABASE_URL")
	require.True(t, ok)
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	password, ok := parsed.User.Password()
	require.True(t, ok)
	assert.Equal(t, "synodus_provisioner", parsed.User.Username())
	assert.Equal(t, "p@ss/word", password)
}

func TestEmbeddedMigrationsAreOrderedAndChecksummed(t *testing.T) {
	first, err := EmbeddedMigrations()
	require.NoError(t, err)
	second, err := EmbeddedMigrations()
	require.NoError(t, err)
	require.Len(t, first.Migrations, 13)
	assert.Equal(t, int64(1), first.Migrations[0].Version)
	assert.Equal(t, int64(2), first.Migrations[1].Version)
	assert.Len(t, first.Migrations[0].Checksum, 64)
	assert.Len(t, first.Checksum, 64)
	assert.Equal(t, first, second)
}

func TestCanonicalSchemaRemovesUUIDDashes(t *testing.T) {
	assert.Equal(t, "org_30ee71539b4845608cbf972587a60fda", CanonicalSchema("30EE7153-9B48-4560-8CBF-972587A60FDA"))
}

func TestLoadConfigUsesBoundedDefaultsAndRejectsSecretsInErrors(t *testing.T) {
	lookup := func(name string) (string, bool) {
		if name == "PROVISIONING_DATABASE_URL" {
			return "postgres://provisioner:very-secret@example/database", true
		}
		if name == "DATABASE_URL" {
			return "postgres://runtime:very-secret@example/database", true
		}
		return "", false
	}
	cfg, err := LoadConfig(lookup)
	require.NoError(t, err)
	assert.Equal(t, "postgres://provisioner:very-secret@example/database", cfg.DatabaseURL)
	assert.Equal(t, DefaultConcurrency, cfg.Concurrency)
	assert.Equal(t, 5*time.Second, cfg.PollInterval)
	assert.Equal(t, DefaultMaxAttempts, cfg.MaxAttempts)

	badLookup := func(name string) (string, bool) {
		if name == "PROVISIONING_DATABASE_URL" {
			return "postgres://provisioner:very-secret@example/database", true
		}
		if name == "PROVISIONER_CONCURRENCY" {
			return "17", true
		}
		return "", false
	}
	_, err = LoadConfig(badLookup)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "very-secret")
}

func TestRetryDelayIsExponentiallyBounded(t *testing.T) {
	assert.Equal(t, 5*time.Second, retryDelay(5*time.Second, 30*time.Second, 1))
	assert.Equal(t, 20*time.Second, retryDelay(5*time.Second, 30*time.Second, 3))
	assert.Equal(t, 30*time.Second, retryDelay(5*time.Second, 30*time.Second, 10))
}

func TestPermanentErrorsExposeOnlySanitizedDetail(t *testing.T) {
	code, detail, permanent := errorFields(permanent("checksum_divergence", "migration checksum differs"))
	assert.True(t, permanent)
	assert.Equal(t, "checksum_divergence", code)
	assert.Equal(t, "migration checksum differs", detail)

	code, detail, permanent = errorFields(assert.AnError)
	assert.False(t, permanent)
	assert.Equal(t, "reconciliation_failed", code)
	assert.False(t, strings.Contains(detail, assert.AnError.Error()))
}
