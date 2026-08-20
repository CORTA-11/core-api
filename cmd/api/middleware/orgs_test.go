package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

type availabilityStub struct {
	availability tenancy.Availability
	err          error
}

func (s availabilityStub) Check(context.Context, uuid.UUID) (tenancy.Availability, error) {
	return s.availability, s.err
}

func TestRequireAvailableOrgFailsClosedBeforeTenantHandler(t *testing.T) {
	for _, test := range []struct {
		name    string
		checker availabilityStub
		status  int
	}{
		{"unknown", availabilityStub{availability: tenancy.AvailabilityUnknown}, http.StatusNotFound},
		{"provisioning or stale", availabilityStub{availability: tenancy.AvailabilityUnavailable}, http.StatusServiceUnavailable},
		{"lookup failure", availabilityStub{err: errors.New("database unavailable")}, http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			handler := OrgMiddleware(RequireAvailableOrg(test.checker)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				called = true
			})))
			request := httptest.NewRequest(http.MethodGet, "/teams", nil)
			request.Header.Set("X-Org-ID", uuid.NewString())
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, test.status, response.Code)
			assert.False(t, called)
		})
	}
}

func TestRequireAvailableOrgAllowsCurrentActiveTenant(t *testing.T) {
	called := false
	handler := OrgMiddleware(RequireAvailableOrg(availabilityStub{availability: tenancy.AvailabilityReady})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})))
	request := httptest.NewRequest(http.MethodGet, "/teams", nil)
	request.Header.Set("X-Org-ID", uuid.NewString())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.True(t, called)
}
