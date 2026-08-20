package main

import (
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMigrator struct {
	err  error
	step int
}

func (m *fakeMigrator) Up() error             { return m.err }
func (m *fakeMigrator) Down() error           { return m.err }
func (m *fakeMigrator) Steps(step int) error  { m.step = step; return m.err }
func (m *fakeMigrator) Close() (error, error) { return nil, nil }

func TestRunPropagatesMigrationFailure(t *testing.T) {
	want := errors.New("database unavailable")
	err := run([]string{"up-all"}, func(string) string { return "postgres://example" }, func(sourceURL, databaseURL string) (migrator, error) {
		assert.Equal(t, publicMigrationsDir, sourceURL)
		assert.Equal(t, "postgres://example", databaseURL)
		return &fakeMigrator{err: want}, nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, want)
}

func TestRunTreatsNoChangeAsSuccess(t *testing.T) {
	err := run([]string{"up-all"}, func(string) string { return "unused" }, func(string, string) (migrator, error) {
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
			err := run([]string{test.command}, func(string) string { return "" }, func(string, string) (migrator, error) {
				return migration, nil
			})
			require.NoError(t, err)
			assert.Equal(t, test.step, migration.step)
		})
	}
}
