package tenancy

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storageTeam(organizationPublicID uuid.UUID, teamID int64) TeamContext {
	userPublicID := uuid.New()
	organization := newOrganizationContext(1, organizationPublicID, userPublicID, CanonicalSchema(organizationPublicID.String()), 1, "checksum")
	return newTeamContext(organization, teamID, uuid.New())
}

func TestStorageObjectKeyPreservesLegacyMinIOLayout(t *testing.T) {
	organizationID := uuid.MustParse("5dc89da9-f554-43ea-970b-d1ca15e2f921")
	key, err := StorageObjectKey(storageTeam(organizationID, 42), "report.pdf")
	require.NoError(t, err)
	assert.Equal(t, "orgs/5dc89da9-f554-43ea-970b-d1ca15e2f921/teams/42/files/report.pdf", key)
}

func TestStorageObjectKeyRejectsInvalidScopeAndNames(t *testing.T) {
	_, err := StorageObjectKey(TeamContext{}, "report.pdf")
	assert.ErrorIs(t, err, ErrInvalidContext)
	team := storageTeam(uuid.New(), 1)
	for _, fileName := range []string{"", ".", "..", "../secret", "dir/file", `dir\file`, "bad\x00name"} {
		_, err := StorageObjectKey(team, fileName)
		assert.ErrorIs(t, err, ErrInvalidStorageName)
	}
}
