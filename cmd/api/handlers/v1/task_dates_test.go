package v1

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskRequestDates(t *testing.T) {
	var request taskRequest
	require.NoError(t, json.Unmarshal([]byte(`{"description":"Task","status":"todo","start_date":"2026-08-10T09:00:00Z","due_date":"2026-08-12T09:00:00Z"}`), &request))
	assert.True(t, request.dates.SetStartDate)
	assert.True(t, request.dates.SetDueDate)
	require.NotNil(t, request.dates.StartDate)
	require.NotNil(t, request.dates.DueDate)
	assert.Equal(t, "2026-08-10T09:00:00Z", request.dates.StartDate.Format(time.RFC3339))
	assert.Equal(t, "2026-08-12T09:00:00Z", request.dates.DueDate.Format(time.RFC3339))

	require.NoError(t, json.Unmarshal([]byte(`{"start_date":null,"due_date":null}`), &request))
	assert.True(t, request.dates.SetStartDate)
	assert.True(t, request.dates.SetDueDate)
	assert.Nil(t, request.dates.StartDate)
	assert.Nil(t, request.dates.DueDate)

	require.NoError(t, json.Unmarshal([]byte(`{"description":"Move","status":"done"}`), &request))
	assert.False(t, request.dates.SetStartDate)
	assert.False(t, request.dates.SetDueDate)
}

func TestTaskRequestRejectsInvalidDates(t *testing.T) {
	for _, data := range []string{
		`{"start_date":"not-a-date"}`, `{"due_date":"2026-02-30T09:00:00Z"}`,
		`{"start_date":42}`, `{"due_date":{}}`,
	} {
		var request taskRequest
		assert.Error(t, json.Unmarshal([]byte(data), &request))
	}
}
