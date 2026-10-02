package v1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskRequestDetails(t *testing.T) {
	var request taskRequest
	require.NoError(t, json.Unmarshal([]byte(`{"description":"Title","details":"Instructions\nsecond line","status":"todo"}`), &request))
	require.NotNil(t, request.Details)
	assert.Equal(t, "Instructions\nsecond line", *request.Details)

	require.NoError(t, json.Unmarshal([]byte(`{"details":""}`), &request))
	require.NotNil(t, request.Details)
	assert.Empty(t, *request.Details)

	require.NoError(t, json.Unmarshal([]byte(`{"description":"Title","status":"done"}`), &request))
	assert.Nil(t, request.Details)
	assert.Error(t, json.Unmarshal([]byte(`{"details":42}`), &request))
}
