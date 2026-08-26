package identity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailCanonicalizerCanonicalize(t *testing.T) {
	t.Parallel()

	canonicalizer := EmailCanonicalizer{}
	tests := []struct {
		name        string
		input       string
		wantDisplay string
		wantKey     string
	}{
		{name: "trims and folds ASCII", input: " Alice@Example.COM ", wantDisplay: "Alice@Example.COM", wantKey: "alice@example.com"},
		{name: "normalizes display to NFC", input: "Jose\u0301@Example.COM", wantDisplay: "Jos\u00e9@Example.COM", wantKey: "jos\u00e9@example.com"},
		{name: "uses full Unicode case folding", input: "STRA\u1e9eE@example.com", wantDisplay: "STRA\u1e9eE@example.com", wantKey: "strasse@example.com"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := canonicalizer.Canonicalize(test.input)
			require.NoError(t, err)
			assert.Equal(t, test.wantDisplay, got.Display)
			assert.Equal(t, test.wantKey, got.Key)
		})
	}
}

func TestEmailCanonicalizerRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	canonicalizer := EmailCanonicalizer{}
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: "   "},
		{name: "invalid UTF-8", input: string([]byte{0xff})},
		{name: "control", input: "alice\n@example.com"},
		{name: "over byte limit", input: strings.Repeat("a", 255)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := canonicalizer.Canonicalize(test.input)
			assert.ErrorIs(t, err, ErrInvalidEmail)
		})
	}
}
