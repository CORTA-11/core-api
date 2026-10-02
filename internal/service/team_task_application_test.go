package service

import (
	"regexp"
	"strings"
	"testing"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
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

func TestUpdateTeamValidatesInputs(t *testing.T) {
	t.Parallel()
	auth := new(mockAuthorizer)
	app := NewTeamTaskApplication(auth, nil)
	// Missing principal or IDs
	_, err := app.UpdateTeam(t.Context(), session.Principal{}, uuid.Nil, uuid.Nil, nil, nil)
	assert.ErrorIs(t, err, authorization.ErrResourceNotFound)

	// Valid principal and IDs, but empty inputs
	p := session.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	_, err = app.UpdateTeam(t.Context(), p, uuid.New(), uuid.New(), nil, nil)
	assert.ErrorIs(t, err, ErrInvalidInput)

	emptyName := "   "
	_, err = app.UpdateTeam(t.Context(), p, uuid.New(), uuid.New(), &emptyName, nil)
	assert.ErrorIs(t, err, ErrInvalidInput)
}
