package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskDetailsValue(t *testing.T) {
	value, err := taskDetailsValue(nil)
	assert.NoError(t, err)
	assert.Empty(t, value)
	for _, text := range []string{"", " Instructions\nsecond line ", strings.Repeat("é", 4096)} {
		value, err = taskDetailsValue(&text)
		assert.NoError(t, err)
		assert.Equal(t, text, value)
	}
	for _, text := range []string{strings.Repeat("é", 4097), string([]byte{0xff})} {
		_, err = taskDetailsValue(&text)
		assert.ErrorIs(t, err, ErrInvalidInput)
	}
}
