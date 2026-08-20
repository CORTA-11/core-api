package main

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSelectionAcceptsOnlyPublicUUIDsAndBoundedConcurrency(t *testing.T) {
	want := uuid.New()
	id, all, concurrency, err := parseSelection("reconcile", []string{"--organization", want.String()}, 4, true)
	require.NoError(t, err)
	assert.Equal(t, want, *id)
	assert.False(t, all)
	assert.Equal(t, 4, concurrency)

	_, _, _, err = parseSelection("reconcile", []string{"--all", "--concurrency", "16"}, 4, true)
	require.NoError(t, err)
	_, _, _, err = parseSelection("reconcile", []string{"--all", "--concurrency", "17"}, 4, true)
	require.Error(t, err)
	_, _, _, err = parseSelection("reconcile", []string{"--organization", "org_schema"}, 4, true)
	require.Error(t, err)
}

func TestParseSelectionRequiresUnambiguousReconcileTarget(t *testing.T) {
	_, _, _, err := parseSelection("reconcile", nil, 4, true)
	require.Error(t, err)
	_, _, _, err = parseSelection("reconcile", []string{"--organization", uuid.NewString(), "--all"}, 4, true)
	require.Error(t, err)
}
