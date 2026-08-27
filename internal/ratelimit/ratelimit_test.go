package ratelimit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type limiterStub struct {
	decisions []Decision
	err       error
	calls     []limiterCall
	cleared   []limiterCall
}

type limiterCall struct {
	bucket   string
	identity string
	consume  bool
}

func (stub *limiterStub) Check(_ context.Context, policy Policy, identity string, consume bool) (Decision, error) {
	stub.calls = append(stub.calls, limiterCall{policy.Bucket, identity, consume})
	if stub.err != nil {
		return Decision{}, stub.err
	}
	if len(stub.decisions) == 0 {
		return Decision{Allowed: true}, nil
	}
	decision := stub.decisions[0]
	stub.decisions = stub.decisions[1:]
	return decision, nil
}

func (stub *limiterStub) Clear(_ context.Context, policy Policy, identity string) error {
	stub.cleared = append(stub.cleared, limiterCall{bucket: policy.Bucket, identity: identity})
	return stub.err
}

func TestPolicyValidationBoundsValues(t *testing.T) {
	assert.NoError(t, (Policy{Bucket: "login-ip", Limit: 20, Period: 15 * time.Minute, Burst: 20}).Validate())
	for _, policy := range []Policy{
		{}, {Bucket: "bad:bucket", Limit: 1, Period: time.Minute, Burst: 1},
		{Bucket: "x", Limit: 0, Period: time.Minute, Burst: 1},
		{Bucket: "x", Limit: 1, Period: 0, Burst: 1},
		{Bucket: "x", Limit: 1, Period: time.Minute, Burst: 0},
	} {
		assert.Error(t, policy.Validate())
	}
}

func TestLoginGuardConsumesIPBeforeAccountAndOnlyRecordsFailures(t *testing.T) {
	stub := &limiterStub{}
	guard, err := NewLoginGuard(stub, DefaultPolicies())
	require.NoError(t, err)
	account, decision, err := guard.Admit(context.Background(), netip.MustParseAddr("192.0.2.4"), " User@Example.com ")
	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	require.Len(t, stub.calls, 2)
	assert.Equal(t, limiterCall{"login-ip", "192.0.2.4", true}, stub.calls[0])
	assert.Equal(t, limiterCall{"account-failure", "user@example.com", false}, stub.calls[1])

	decision, err = guard.RecordFailure(context.Background(), account)
	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.True(t, stub.calls[2].consume)
	require.NoError(t, guard.ClearSuccess(context.Background(), account))
	require.Len(t, stub.cleared, 1)
}

func TestLoginGuardUsesFixedInvalidEmailSentinel(t *testing.T) {
	stub := &limiterStub{}
	guard, err := NewLoginGuard(stub, DefaultPolicies())
	require.NoError(t, err)
	_, _, err = guard.Admit(context.Background(), netip.MustParseAddr("192.0.2.4"), strings.Repeat("x", 2048))
	require.NoError(t, err)
	assert.Equal(t, InvalidEmailIdentity, stub.calls[1].identity)
}

func TestAdministrativeMiddlewareFailsClosedAndReturnsGenericRetry(t *testing.T) {
	stub := &limiterStub{decisions: []Decision{{Allowed: false, RetryAfter: 1500 * time.Millisecond}}}
	middleware, err := NewAdministrativeMiddleware(stub, DefaultPolicies().Administrative)
	require.NoError(t, err)
	handler := middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler reached") }))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/orgs", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	trusted, err := httpx.ParseTrustedProxies("")
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	trusted.Middleware(handler).ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.Equal(t, "2", recorder.Header().Get("Retry-After"))
	assert.Contains(t, recorder.Body.String(), `"type":"/problems/rate-limited"`)

	stub.err = errors.New("redis secret")
	recorder = httptest.NewRecorder()
	trusted.Middleware(handler).ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "redis secret")
}
