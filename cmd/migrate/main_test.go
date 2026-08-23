package main

import (
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMigrator struct {
	err     error
	step    int
	version uint
	dirty   bool
}

func migrationEnvironment(name string) string {
	if name == "MIGRATION_DATABASE_URL" {
		return "postgres://migrator"
	}
	return ""
}

func (m *fakeMigrator) Up() error                    { return m.err }
func (m *fakeMigrator) Down() error                  { return m.err }
func (m *fakeMigrator) Steps(step int) error         { m.step = step; return m.err }
func (m *fakeMigrator) Close() (error, error)        { return nil, nil }
func (m *fakeMigrator) Version() (uint, bool, error) { return m.version, m.dirty, m.err }

func TestRunPropagatesMigrationFailure(t *testing.T) {
	want := errors.New("database unavailable")
	err := run([]string{"up-all"}, func(name string) string {
		if name == "MIGRATION_DATABASE_URL" {
			return "postgres://migrator"
		}
		return "postgres://runtime"
	}, func(sourceURL, databaseURL string) (migrator, error) {
		assert.Equal(t, publicMigrationsDir, sourceURL)
		assert.Equal(t, "postgres://migrator", databaseURL)
		return &fakeMigrator{err: want}, nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, want)
}

func TestRunReportsPublicMigrationStatus(t *testing.T) {
	require.NoError(t, run([]string{"status"}, migrationEnvironment, func(string, string) (migrator, error) {
		return &fakeMigrator{version: 4}, nil
	}))
	require.Error(t, run([]string{"status"}, migrationEnvironment, func(string, string) (migrator, error) {
		return &fakeMigrator{version: 4, dirty: true}, nil
	}))
}

func TestRunTreatsNoChangeAsSuccess(t *testing.T) {
	err := run([]string{"up-all"}, migrationEnvironment, func(string, string) (migrator, error) {
		return &fakeMigrator{err: migrate.ErrNoChange}, nil
	})
	require.NoError(t, err)
}

func TestRunRejectsInvalidInvocation(t *testing.T) {
	factory := func(string, string) (migrator, error) { return &fakeMigrator{}, nil }
	assert.Error(t, run(nil, func(string) string { return "" }, factory))
	assert.Error(t, run([]string{"sideways"}, func(string) string { return "" }, factory))
}

func TestRunUsesSingleStepCommands(t *testing.T) {
	for _, test := range []struct {
		command string
		step    int
	}{{"up", 1}, {"down", -1}} {
		t.Run(test.command, func(t *testing.T) {
			migration := &fakeMigrator{}
			err := run([]string{test.command}, migrationEnvironment, func(string, string) (migrator, error) {
				return migration, nil
			})
			require.NoError(t, err)
			assert.Equal(t, test.step, migration.step)
		})
	}
}
