package v1

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateTeamRequestAcceptsDescription(t *testing.T) {
	for _, description := range []string{"Persistent description", ""} {
		decoder := json.NewDecoder(strings.NewReader(`{"name":"Research","leader_email":"leader@example.test","description":"` + description + `"}`))
		decoder.DisallowUnknownFields()
		var input createTeamRequest
		require.NoError(t, decoder.Decode(&input))
		require.Equal(t, description, input.Description)
	}
	var input createTeamRequest
	require.NoError(t, json.Unmarshal([]byte(`{"name":"Research","leader_email":"leader@example.test"}`), &input))
	require.Empty(t, input.Description)
}
