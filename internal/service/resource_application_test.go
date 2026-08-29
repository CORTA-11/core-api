package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateResourceNormalizesCodeAndRejectsOvernightWindows(t *testing.T) {
	input, _, err := validateResource(ResourceWrite{Name: " Microscope ", Code: " ins-01 ", Kind: "instrument", Enabled: true,
		Availability: []AvailabilityWindow{{Weekday: 1, Start: "09:00", End: "17:00"}}})
	require.NoError(t, err)
	assert.Equal(t, "Microscope", input.Name)
	assert.Equal(t, "INS-01", input.Code)

	_, _, err = validateResource(ResourceWrite{Name: "GPU", Code: "GPU-1", Kind: "gpu",
		Availability: []AvailabilityWindow{{Weekday: 1, Start: "22:00", End: "06:00"}}})
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestWithinAvailabilityUsesUTCAndIncludesBoundaries(t *testing.T) {
	windows := []AvailabilityWindow{{Weekday: 1, Start: "09:00", End: "17:00"}}
	start := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	assert.True(t, withinAvailability(start, start.Add(8*time.Hour), windows))
	assert.False(t, withinAvailability(start.Add(-time.Minute), start.Add(time.Hour), windows))
	assert.False(t, withinAvailability(start, start.Add(24*time.Hour), windows))
}

func TestBookingDetailsRemainNilWhenRedacted(t *testing.T) {
	view := BookingView{DetailsVisible: false}
	assert.Nil(t, view.TeamPublicID)
	assert.Nil(t, view.TeamName)
	assert.Nil(t, view.RequestedByName)
	assert.Nil(t, view.Purpose)
}
