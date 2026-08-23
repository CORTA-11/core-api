package tenancy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrganizationContextZeroValueIsInvalid(t *testing.T) {
	var organization OrganizationContext

	assert.ErrorIs(t, organization.validate(), ErrInvalidContext)
}

func TestTeamContextZeroValueIsInvalid(t *testing.T) {
	var team TeamContext

	assert.ErrorIs(t, team.validate(), ErrInvalidContext)
}

func TestResolverErrorsAreStable(t *testing.T) {
	assert.True(t, errors.Is(ErrOrganizationUnavailable, ErrOrganizationUnavailable))
	assert.True(t, errors.Is(ErrRegistryIntegrity, ErrRegistryIntegrity))
	assert.True(t, errors.Is(ErrInvalidCallback, ErrInvalidCallback))
}
