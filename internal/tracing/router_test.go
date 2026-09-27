package tracing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/CORTA-11/core-api/cmd/api/handlers/v1"
	"github.com/CORTA-11/core-api/internal/identity"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestOnlyPilotRoutesTraceAndPreserveErrors(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	router := v1.NewRouter(v1.RouterConfig{Environment: "test", TraceEnabled: true})
	const orgID = "11111111-1111-1111-1111-111111111111"
	const teamID = "22222222-2222-2222-2222-222222222222"
	const taskID = "33333333-3333-3333-3333-333333333333"
	cases := []struct {
		method, path, spanName string
		status                 int
	}{
		{http.MethodGet, "/health/live", "", http.StatusNoContent},
		{http.MethodPost, "/api/v1/auth/register", "", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/auth/login", "POST /api/v1/auth/login", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/auth/session", "GET /api/v1/auth/session", http.StatusUnauthorized},
		{http.MethodGet, "/api/v1/auth/user-keys", "GET /api/v1/auth/user-keys", http.StatusServiceUnavailable},
		{http.MethodPut, "/api/v1/auth/user-keys", "PUT /api/v1/auth/user-keys", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/orgs", "GET /api/v1/orgs", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/orgs/" + orgID, "GET /api/v1/orgs/{org_id}", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/orgs/" + orgID + "/teams", "GET /api/v1/orgs/{org_id}/teams", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/orgs/" + orgID + "/resources", "GET /api/v1/orgs/{org_id}/resources", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/orgs/" + orgID + "/bookings", "GET /api/v1/orgs/{org_id}/bookings", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/orgs/" + orgID + "/resource-requests", "GET /api/v1/orgs/{org_id}/resource-requests", http.StatusServiceUnavailable},
		{http.MethodGet, "/api/v1/orgs/" + orgID + "/teams/" + teamID + "/tasks", "GET /api/v1/orgs/{org_id}/teams/{team_id}/tasks", http.StatusServiceUnavailable},
		{http.MethodPatch, "/api/v1/orgs/" + orgID + "/teams/" + teamID + "/tasks/" + taskID, "PATCH /api/v1/orgs/{org_id}/teams/{team_id}/tasks/{task_id}", http.StatusServiceUnavailable},
	}
	want := make([]string, 0)
	for _, tc := range cases {
		response := httptest.NewRecorder()
		router.Handler().ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != tc.status {
			t.Fatalf("%s %s: status %d, want %d", tc.method, tc.path, response.Code, tc.status)
		}
		if tc.spanName != "" {
			want = append(want, tc.spanName)
		}
	}
	spans := exporter.GetSpans()
	if len(spans) != len(want) {
		t.Fatalf("got %d spans, want %d", len(spans), len(want))
	}
	for i, span := range spans {
		if span.Name != want[i] {
			t.Errorf("span %d name = %q, want %q", i, span.Name, want[i])
		}
		if strings.Contains(span.Name, orgID) || strings.Contains(span.Name, teamID) || strings.Contains(span.Name, taskID) {
			t.Errorf("span %d name contains a route ID", i)
		}
	}
}

type credentialFailureVerifier struct{}

func (credentialFailureVerifier) Verify(context.Context, string, string) (identity.CredentialPrincipal, error) {
	return identity.CredentialPrincipal{}, identity.ErrInvalidCredentials
}

func TestLoginCredentialVerificationIsChildOfServerSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	router := v1.NewRouter(v1.RouterConfig{Environment: "test", TraceEnabled: true, Verifier: credentialFailureVerifier{}})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"user@example.com","password":"private-password"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("login status = %d, want 401", response.Code)
	}
	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("got %d spans, want 2", len(spans))
	}
	child, parent := spans[0], spans[1]
	if child.Name != "credential verification" || parent.Name != "POST /api/v1/auth/login" {
		t.Fatalf("unexpected span names: %q, %q", child.Name, parent.Name)
	}
	if child.Parent.SpanID() != parent.SpanContext.SpanID() || child.SpanContext.TraceID() != parent.SpanContext.TraceID() {
		t.Fatal("credential verification is not a child of the login span")
	}
	for _, span := range spans {
		for _, attr := range span.Attributes {
			if strings.Contains(attr.Value.Emit(), "private-password") || strings.Contains(attr.Value.Emit(), "user@example.com") {
				t.Fatal("login span contains credentials")
			}
		}
	}
}
