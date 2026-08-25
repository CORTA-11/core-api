package tenancy

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidStorageName = errors.New("invalid storage object name")

// StorageObjectKey is the only compatibility bridge from opaque trusted team
// scope to the MinIO layout whose team segment uses a numeric database ID. It
// deliberately exposes only the complete key, never the internal relational
// team identifier.
func StorageObjectKey(team TeamContext, fileName string) (string, error) {
	if team.validate() != nil {
		return "", ErrInvalidContext
	}
	if fileName == "" || fileName == "." || fileName == ".." ||
		strings.ContainsAny(fileName, `/\\`) || strings.ContainsRune(fileName, '\x00') {
		return "", ErrInvalidStorageName
	}
	return fmt.Sprintf(
		"orgs/%s/teams/%s/files/%s",
		team.organization.organizationPublicID.String(),
		strconv.FormatInt(team.teamID, 10),
		fileName,
	), nil
}
