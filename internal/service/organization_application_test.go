package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeResourceNameEnforcesOpenAPIBounds(t *testing.T) {
	t.Parallel()
	name, err := normalizeResourceName("  Applied Systems Lab  ")
	require.NoError(t, err)
	assert.Equal(t, "Applied Systems Lab", name)
	for _, invalid := range []string{"", " \t\n ", strings.Repeat("a", 201), string([]byte{0xff})} {
		_, err := normalizeResourceName(invalid)
		assert.ErrorIs(t, err, ErrInvalidInput)
	}
}
