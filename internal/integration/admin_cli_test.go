//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminCLICreatesCanonicalPolicyCompliantAccount(t *testing.T) {
	pool := testsupport.OpenPostgres(t)
	testsupport.ResetPostgres(t, pool)
	databaseURL := testsupport.RequiredEnv(t, "TEST_DATABASE_URL")
	testsupport.ApplyMigrations(t, "db/migrations/public", databaseURL)

	repositoryRoot := testsupport.RepositoryRoot()
	goCache := filepath.Join(repositoryRoot, ".cache", "go-build")
	adminBinary := filepath.Join(t.TempDir(), "admin")
	build := exec.CommandContext(context.Background(), "go", "build", "-o", adminBinary, "./cmd/admin")
	build.Dir = repositoryRoot
	build.Env = append(os.Environ(), "GOCACHE="+goCache)
	buildOutput, err := build.CombinedOutput()
	require.NoError(t, err, string(buildOutput))

	const password = "synodus-admin-password"
	runAdmin := func(email string) (string, string, error) {
		command := exec.CommandContext(context.Background(), adminBinary,
			"user", "create", "--email", email, "--display-name", "  CLI User  ", "--password-stdin")
		command.Dir = repositoryRoot
		command.Env = append(os.Environ(), "DATABASE_URL="+databaseURL)
		command.Stdin = strings.NewReader(password + "\n")
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		err := command.Run()
		return stdout.String(), stderr.String(), err
	}

	stdout, stderr, err := runAdmin(" CLI.User@Example.TEST ")
	require.NoError(t, err, stderr)
	assert.Contains(t, stdout, "created user ")
	assert.NotContains(t, stdout, password)
	assert.NotContains(t, stdout, "example.test")
	assert.Empty(t, stderr)

	var email, canonical, displayName, passwordHash, normalization string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT email, email_canonical, display_name, password_hash, password_normalization
		FROM public.users WHERE email_canonical = 'cli.user@example.test'`).Scan(
		&email, &canonical, &displayName, &passwordHash, &normalization,
	))
	assert.Equal(t, "CLI.User@Example.TEST", email)
	assert.Equal(t, "cli.user@example.test", canonical)
	assert.Equal(t, "CLI User", displayName)
	assert.Equal(t, identity.PasswordNormalizationNFCV1, normalization)
	hasher, err := identity.NewPasswordHasher(identity.HashConfig{})
	require.NoError(t, err)
	verification, err := hasher.Verify(context.Background(), password, passwordHash)
	require.NoError(t, err)
	assert.True(t, verification.Match)

	stdout, stderr, err = runAdmin("cli.user@example.test")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "email already exists")
	assert.NotContains(t, stderr, password)
	assert.NotContains(t, stderr, "cli.user@example.test")
}
