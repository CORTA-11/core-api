package v1

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCollaborationSecret = "test-collaboration-service-secret-123"

type privateDocumentServiceStub struct {
	loaded     service.DocumentState
	stored     service.DocumentStateWrite
	editorID   uuid.UUID
	loadCalls  int
	storeCalls int
	err        error
}

func (stub *privateDocumentServiceStub) List(context.Context, session.Principal, uuid.UUID, uuid.UUID) ([]service.DocumentView, error) {
	return nil, nil
}
func (stub *privateDocumentServiceStub) Create(context.Context, session.Principal, uuid.UUID, uuid.UUID, string) (service.DocumentView, error) {
	return service.DocumentView{}, nil
}
func (stub *privateDocumentServiceStub) Get(context.Context, session.Principal, uuid.UUID, uuid.UUID, uuid.UUID) (service.DocumentProjection, error) {
	return service.DocumentProjection{}, nil
}
func (stub *privateDocumentServiceStub) Update(context.Context, session.Principal, uuid.UUID, uuid.UUID, uuid.UUID, service.DocumentPatch) (service.DocumentProjection, error) {
	return service.DocumentProjection{}, nil
}
func (stub *privateDocumentServiceStub) Delete(context.Context, session.Principal, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}
func (stub *privateDocumentServiceStub) IssueSocketTicket(context.Context, session.Principal, string, uuid.UUID, uuid.UUID, uuid.UUID) (string, error) {
	return "", nil
}
func (stub *privateDocumentServiceStub) LoadState(_ context.Context, editorID, _, _, _ uuid.UUID) (service.DocumentState, error) {
	stub.loadCalls++
	stub.editorID = editorID
	return stub.loaded, stub.err
}
func (stub *privateDocumentServiceStub) StoreState(_ context.Context, editorID, _, _, _ uuid.UUID, state service.DocumentStateWrite) (service.DocumentState, error) {
	stub.storeCalls++
	stub.editorID = editorID
	stub.stored = state
	return stub.loaded, stub.err
}

func TestPrivateDocumentStateRequiresServiceAuthentication(t *testing.T) {
	t.Parallel()
	stub := &privateDocumentServiceStub{}
	router := NewRouter(RouterConfig{Documents: stub, CollaborationServiceSecret: []byte(testCollaborationSecret), Environment: "test"})
	path := privateDocumentStatePath()

	for _, authorization := range []string{"", "Bearer wrong-service-secret-value-123"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", authorization)
		request.Header.Set(editorIDHeader, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
		response := httptest.NewRecorder()
		router.Handler().ServeHTTP(response, request)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	}
	assert.Zero(t, stub.loadCalls)
}

func TestPrivateDocumentStateLoadsCanonicalBytesWithoutBrowserSession(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 7, 9, 30, 0, 0, time.UTC)
	stub := &privateDocumentServiceStub{loaded: service.DocumentState{
		CanonicalState: []byte{1, 2, 3}, Title: "Shared notes", BodyHTML: "<p>Persisted</p>",
		UpdatedBy: uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"), UpdatedAt: now,
	}}
	router := NewRouter(RouterConfig{Documents: stub, CollaborationServiceSecret: []byte(testCollaborationSecret), Environment: "test"})
	request := authenticatedPrivateRequest(http.MethodGet, privateDocumentStatePath(), nil)
	response := httptest.NewRecorder()

	router.Handler().ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.JSONEq(t, `{"canonical_state":"AQID","title":"Shared notes","body_html":"<p>Persisted</p>","updated_by":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","updated_at":"2026-09-07T09:30:00Z"}`, response.Body.String())
	assert.Equal(t, uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"), stub.editorID)
}

func TestPrivateDocumentStateAtomicallyStoresStateAndProjections(t *testing.T) {
	t.Parallel()
	stub := &privateDocumentServiceStub{}
	router := NewRouter(RouterConfig{Documents: stub, CollaborationServiceSecret: []byte(testCollaborationSecret), Environment: "test"})
	body := bytes.NewBufferString(`{"canonical_state":"AQID","title":"Converged title","body_html":"<p>Converged body</p>"}`)
	request := authenticatedPrivateRequest(http.MethodPut, privateDocumentStatePath(), body)
	response := httptest.NewRecorder()

	router.Handler().ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, []byte{1, 2, 3}, stub.stored.CanonicalState)
	assert.Equal(t, "Converged title", stub.stored.Title)
	assert.Equal(t, "<p>Converged body</p>", stub.stored.BodyHTML)
	assert.Equal(t, uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"), stub.editorID)
}

func authenticatedPrivateRequest(method, target string, body *bytes.Buffer) *http.Request {
	var request *http.Request
	if body == nil {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, body)
	}
	request.Header.Set("Authorization", "Bearer "+testCollaborationSecret)
	request.Header.Set(editorIDHeader, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	request.Header.Set("Content-Type", "application/json")
	return request
}

func privateDocumentStatePath() string {
	return "/internal/v1/orgs/11111111-1111-4111-8111-111111111111/teams/22222222-2222-4222-8222-222222222222/documents/33333333-3333-4333-8333-333333333333/state"
}
