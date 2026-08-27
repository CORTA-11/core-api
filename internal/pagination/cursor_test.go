package pagination

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	fixedNow = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	orgID    = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	teamID   = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	taskID   = uuid.MustParse("33333333-3333-4333-8333-333333333333")
)

func newTestCodec(t testing.TB, now *time.Time, previous *Key) *Codec {
	t.Helper()
	codec, err := NewCodec(CodecConfig{
		Active: Key{ID: "active", Secret: []byte(strings.Repeat("a", 32))}, Previous: previous,
		Clock: func() time.Time { return *now },
	})
	require.NoError(t, err)
	return codec
}

func TestCursorRoundTripAndScopeBinding(t *testing.T) {
	now := fixedNow
	codec := newTestCodec(t, &now, nil)
	binding := Binding{RouteID: "listTasks", OrganizationID: &orgID, TeamID: &teamID}
	want := Cursor{Direction: DirectionNext, Sort: SortKey{Timestamp: now.Add(-time.Minute), ID: taskID}}
	token, err := codec.Issue(binding, want)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(token), MaximumTokenSize)
	assert.NotContains(t, token, "=")
	got, err := codec.Verify(token, binding)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	for name, wrong := range map[string]Binding{
		"route":        {RouteID: "listTeams", OrganizationID: &orgID, TeamID: &teamID},
		"organization": {RouteID: "listTasks", OrganizationID: &teamID, TeamID: &teamID},
		"team":         {RouteID: "listTasks", OrganizationID: &orgID, TeamID: &orgID},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := codec.Verify(token, wrong)
			assert.ErrorIs(t, err, ErrInvalidCursor)
		})
	}
}

func TestCursorRejectsMutationExpirationAndMalformedTokens(t *testing.T) {
	now := fixedNow
	codec := newTestCodec(t, &now, nil)
	binding := Binding{RouteID: "listOrganizations"}
	token, err := codec.Issue(binding, Cursor{Direction: DirectionPrevious, Sort: SortKey{Timestamp: now, ID: orgID}})
	require.NoError(t, err)
	parts := strings.Split(token, ".")
	mutated := parts[0] + "." + parts[1][:len(parts[1])-1] + "A." + parts[2]
	for name, candidate := range map[string]string{
		"mutation": mutated, "unknown key": "unknown." + parts[1] + "." + parts[2],
		"padding": parts[0] + "." + parts[1] + "=." + parts[2], "few parts": "a.b",
		"oversized": strings.Repeat("x", MaximumTokenSize+1), "empty": "",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := codec.Verify(candidate, binding)
			assert.ErrorIs(t, err, ErrInvalidCursor)
		})
	}
	now = now.Add(DefaultLifetime)
	_, err = codec.Verify(token, binding)
	assert.ErrorIs(t, err, ErrInvalidCursor)
}

func TestPreviousKeyOnlyVerifiesExistingCursor(t *testing.T) {
	now := fixedNow
	previousCodec, err := NewCodec(CodecConfig{
		Active: Key{ID: "previous", Secret: []byte(strings.Repeat("p", 32))},
		Clock:  func() time.Time { return now },
	})
	require.NoError(t, err)
	binding := Binding{RouteID: "listOrganizations"}
	oldToken, err := previousCodec.Issue(binding, Cursor{Direction: DirectionNext, Sort: SortKey{Timestamp: now, ID: orgID}})
	require.NoError(t, err)
	rotated := newTestCodec(t, &now, &Key{ID: "previous", Secret: []byte(strings.Repeat("p", 32))})
	_, err = rotated.Verify(oldToken, binding)
	require.NoError(t, err)
	newToken, err := rotated.Issue(binding, Cursor{Direction: DirectionNext, Sort: SortKey{Timestamp: now, ID: orgID}})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(newToken, "active."))
}

func TestCursorRejectsValidlySignedUnknownClaimsAndVersions(t *testing.T) {
	now := fixedNow
	codec := newTestCodec(t, &now, nil)
	binding := Binding{RouteID: "listOrganizations"}
	for name, raw := range map[string]string{
		"version":       `{"v":2,"r":"listOrganizations","d":"next","s":1,"i":"11111111-1111-4111-8111-111111111111","e":9999999999}`,
		"unknown claim": `{"v":1,"r":"listOrganizations","d":"next","s":1,"i":"11111111-1111-4111-8111-111111111111","e":9999999999,"x":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			payload := base64.RawURLEncoding.EncodeToString([]byte(raw))
			signed := "active." + payload
			token := signed + "." + base64.RawURLEncoding.EncodeToString(sign([]byte(strings.Repeat("a", 32)), signed))
			_, err := codec.Verify(token, binding)
			assert.ErrorIs(t, err, ErrInvalidCursor)
		})
	}
}

func FuzzVerifyNeverPanics(f *testing.F) {
	now := fixedNow
	codec := newTestCodec(f, &now, nil)
	binding := Binding{RouteID: "listOrganizations"}
	f.Add("")
	f.Add("active.e30.invalid")
	f.Fuzz(func(t *testing.T, token string) {
		_, _ = codec.Verify(token, binding)
	})
}
