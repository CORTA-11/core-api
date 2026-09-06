package identity

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswordPolicyNormalize(t *testing.T) {
	t.Parallel()

	policy := PasswordPolicy{}
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "minimum code points", input: strings.Repeat("a", 12), want: strings.Repeat("a", 12)},
		{name: "maximum code points", input: strings.Repeat("a", 128), want: strings.Repeat("a", 128)},
		{name: "spaces remain valid", input: "correct horse battery staple", want: "correct horse battery staple"},
		{name: "Unicode remains valid", input: "research-\u5bc6\u7801-credential", want: "research-\u5bc6\u7801-credential"},
		{name: "normalizes to NFC", input: "re\u0301search-password-value", want: "r\u00e9search-password-value"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := policy.Normalize(test.input)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestPasswordPolicyRejectsInvalidInputWithoutDisclosure(t *testing.T) {
	t.Parallel()

	policy := PasswordPolicy{}
	secretWithControl := "secret-value-that-must-not-appear\n"
	tests := []struct {
		name  string
		input string
	}{
		{name: "short", input: strings.Repeat("a", 11)},
		{name: "long", input: strings.Repeat("a", 129)},
		{name: "invalid UTF-8", input: string([]byte{0xff, 'a'})},
		{name: "control", input: secretWithControl},
		{name: "over byte limit", input: strings.Repeat("\U0001f642", 257)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := policy.Normalize(test.input)
			assert.Empty(t, got)
			assert.ErrorIs(t, err, ErrPasswordPolicy)
			assert.NotContains(t, err.Error(), test.input)
		})
	}
}

func FuzzPasswordPolicyNormalizeNeverPanics(f *testing.F) {
	f.Add("correct horse battery staple")
	f.Add(string([]byte{0xff, 0x00}))
	f.Fuzz(func(t *testing.T, input string) {
		got, err := (PasswordPolicy{}).Normalize(input)
		if err != nil {
			assert.Empty(t, got)
			return
		}
		assert.True(t, utf8.ValidString(got))
		assert.GreaterOrEqual(t, utf8.RuneCountInString(got), 12)
		assert.LessOrEqual(t, utf8.RuneCountInString(got), 128)
		assert.LessOrEqual(t, len(got), 1024)
	})
}
