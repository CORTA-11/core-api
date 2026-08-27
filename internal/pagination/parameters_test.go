package pagination

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDefaultsAndAcceptsBoundaries(t *testing.T) {
	parameters, err := Parse(url.Values{})
	require.NoError(t, err)
	assert.Equal(t, DefaultPageSize, parameters.PageSize)

	for _, size := range []string{"1", "100"} {
		parameters, err = Parse(url.Values{"page_size": {size}, "cursor": {"token"}})
		require.NoError(t, err)
		assert.Equal(t, "token", parameters.Cursor)
	}
}

func TestParseRejectsInvalidAndDuplicateParameters(t *testing.T) {
	for name, values := range map[string]url.Values{
		"zero": {"page_size": {"0"}}, "negative": {"page_size": {"-1"}},
		"over maximum": {"page_size": {"101"}}, "nonnumeric": {"page_size": {"many"}},
		"empty size": {"page_size": {""}}, "duplicate size": {"page_size": {"1", "2"}},
		"empty cursor": {"cursor": {""}}, "duplicate cursor": {"cursor": {"one", "two"}},
		"offset": {"offset": {"0"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(values)
			assert.ErrorIs(t, err, ErrInvalidParameters)
		})
	}
}
