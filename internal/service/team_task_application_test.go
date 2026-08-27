package service

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeterministicTeamSlugIsContractValidAndBounded(t *testing.T) {
	t.Parallel()
	pattern := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	for _, name := range []string{"Reproducibility Group", "  Systems / Safety  ", "研究團隊", strings.Repeat("A", 200)} {
		slug := deterministicTeamSlug(name)
		assert.True(t, pattern.MatchString(slug), slug)
		assert.LessOrEqual(t, len(slug), 100)
		assert.Equal(t, slug, deterministicTeamSlug(name))
	}
}

func TestValidateTaskWriteEnforcesOpenAPIContract(t *testing.T) {
	t.Parallel()
	description, status, err := validateTaskWrite("  Reproduce benchmark  ", "in_progress")
	require.NoError(t, err)
	assert.Equal(t, "Reproduce benchmark", description)
	assert.Equal(t, "in_progress", status)
	for _, input := range []struct{ description, status string }{
		{"", "todo"}, {"task", "in-progress"}, {"task", "future"}, {strings.Repeat("a", 4097), "done"},
	} {
		_, _, err := validateTaskWrite(input.description, input.status)
		assert.ErrorIs(t, err, ErrInvalidInput)
	}
}
