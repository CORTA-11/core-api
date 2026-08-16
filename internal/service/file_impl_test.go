package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileObjectKey(t *testing.T) {
	orgID := uuid.MustParse("5dc89da9-f554-43ea-970b-d1ca15e2f921")

	key, err := fileObjectKey(orgID, 42, "report.pdf")
	require.NoError(t, err)
	assert.Equal(t, "orgs/5dc89da9-f554-43ea-970b-d1ca15e2f921/teams/42/files/report.pdf", key)
}

func TestFileObjectKeyIsolatesOrganizationsAndTeams(t *testing.T) {
	orgOne := uuid.MustParse("5dc89da9-f554-43ea-970b-d1ca15e2f921")
	orgTwo := uuid.MustParse("6018ffba-58c0-463b-9cc0-ecb4c8952c83")

	orgOneTeamOne, err := fileObjectKey(orgOne, 1, "report.pdf")
	require.NoError(t, err)
	orgOneTeamTwo, err := fileObjectKey(orgOne, 2, "report.pdf")
	require.NoError(t, err)
	orgTwoTeamOne, err := fileObjectKey(orgTwo, 1, "report.pdf")
	require.NoError(t, err)

	assert.NotEqual(t, orgOneTeamOne, orgOneTeamTwo)
	assert.NotEqual(t, orgOneTeamOne, orgTwoTeamOne)
}

func TestFileObjectKeyRejectsUnsafeFileNames(t *testing.T) {
	orgID := uuid.New()

	for _, fileName := range []string{"", ".", "..", "../secret", "dir/file", `dir\file`, "bad\x00name"} {
		t.Run(fileName, func(t *testing.T) {
			_, err := fileObjectKey(orgID, 1, fileName)
			assert.ErrorIs(t, err, ErrInvalidFileName)
		})
	}
}
