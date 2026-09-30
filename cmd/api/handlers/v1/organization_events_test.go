package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type changingOrganizations struct {
	organizationServiceStub
	calls     int
	principal session.Principal
}

func (stub *changingOrganizations) List(_ context.Context, principal session.Principal, _ pagination.Parameters) (service.OrganizationPage, error) {
	stub.calls++
	stub.principal = principal
	state := "provisioning"
	if stub.calls > 1 {
		state = "active"
	}
	return service.OrganizationPage{Items: []service.OrganizationView{{Name: "Test org", LifecycleState: state}}}, nil
}

type cancelOnFlush struct {
	*httptest.ResponseRecorder
	cancel  context.CancelFunc
	flushes int
}

func (writer *cancelOnFlush) Flush() {
	writer.ResponseRecorder.Flush()
	writer.flushes++
	if writer.flushes == 2 {
		writer.cancel()
	}
}

func TestOrganizationEventsFlushInitialAndChangedStateThroughRecovery(t *testing.T) {
	stub := &changingOrganizations{}
	handler := &ResourceHandler{organizations: stub}
	request := authenticatedResourceRequest(http.MethodGet, "/api/v1/orgs/events")
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	writer := &cancelOnFlush{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	httpx.Recover(http.HandlerFunc(handler.organizationEvents)).ServeHTTP(writer, request.WithContext(ctx))
	require.Equal(t, http.StatusOK, writer.Code)
	assert.Equal(t, "text/event-stream", writer.Header().Get("Content-Type"))
	assert.Equal(t, "no", writer.Header().Get("X-Accel-Buffering"))
	assert.Equal(t, 2, writer.flushes)
	assert.Equal(t, 2, stub.calls)
	authentication, _ := authenticationFrom(request)
	assert.Equal(t, authentication.Principal, stub.principal)
	assert.Contains(t, writer.Body.String(), "event: organizations\ndata: [")
	assert.Contains(t, writer.Body.String(), `"lifecycle_state":"provisioning"`)
	assert.Contains(t, writer.Body.String(), `"lifecycle_state":"active"`)
}

func TestOrganizationEventsRejectInvalidPaginationBeforeStreaming(t *testing.T) {
	handler := &ResourceHandler{organizations: organizationServiceStub{}}
	request := authenticatedResourceRequest(http.MethodGet, "/api/v1/orgs/events?page_size=invalid")
	writer := httptest.NewRecorder()
	handler.organizationEvents(writer, request)
	assert.Equal(t, http.StatusBadRequest, writer.Code)
	assert.Contains(t, writer.Header().Get("Content-Type"), "application/problem+json")
	assert.False(t, writer.Flushed)
}

func TestOrganizationEventsRequiresSession(t *testing.T) {
	router := NewRouter(RouterConfig{})
	writer := httptest.NewRecorder()
	router.Handler().ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/api/v1/orgs/events", nil))
	assert.NotEqual(t, http.StatusOK, writer.Code)
	assert.NotEqual(t, http.StatusNotFound, writer.Code)
	assert.NotContains(t, writer.Body.String(), "event: organizations")
}
