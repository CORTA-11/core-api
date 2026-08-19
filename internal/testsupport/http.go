package testsupport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func AuthenticatedRequest(method, target, bearerToken, organizationID string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	request.Header.Set("Authorization", "Bearer "+bearerToken)
	if organizationID != "" {
		request.Header.Set("X-Organization-ID", organizationID)
	}
	return request
}

func AssertProblemStatus(t testing.TB, recorder *httptest.ResponseRecorder, status int) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("problem status = %d, want %d; body: %s", recorder.Code, status, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("problem Content-Type = %q, want application/problem+json", got)
	}
}
