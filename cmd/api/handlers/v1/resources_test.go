package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type organizationServiceStub struct {
	page      service.OrganizationPage
	deleteErr error
}

type documentServiceStub struct {
	document service.DocumentProjection
	ticket   string
	err      error
}

func (stub documentServiceStub) List(context.Context, session.Principal, uuid.UUID, uuid.UUID) ([]service.DocumentView, error) {
	return nil, nil
}
func (stub documentServiceStub) Create(context.Context, session.Principal, uuid.UUID, uuid.UUID, string) (service.DocumentView, error) {
	return service.DocumentView{}, nil
}
func (stub documentServiceStub) Get(context.Context, session.Principal, uuid.UUID, uuid.UUID, uuid.UUID) (service.DocumentProjection, error) {
	return stub.document, stub.err
}
func (stub documentServiceStub) Update(context.Context, session.Principal, uuid.UUID, uuid.UUID, uuid.UUID, service.DocumentPatch) (service.DocumentProjection, error) {
	return stub.document, stub.err
}
func (stub documentServiceStub) Delete(context.Context, session.Principal, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return stub.err
}
func (stub documentServiceStub) IssueSocketTicket(context.Context, session.Principal, uuid.UUID, uuid.UUID, uuid.UUID) (string, error) {
	return stub.ticket, stub.err
}

func (stub organizationServiceStub) List(context.Context, session.Principal, pagination.Parameters) (service.OrganizationPage, error) {
	return stub.page, nil
}

func (organizationServiceStub) Create(context.Context, session.Principal, string) (service.OrganizationView, error) {
	return service.OrganizationView{}, nil
}

func (organizationServiceStub) Get(context.Context, session.Principal, uuid.UUID) (service.OrganizationView, error) {
	return service.OrganizationView{}, nil
}

func (organizationServiceStub) Update(context.Context, session.Principal, uuid.UUID, string) (service.OrganizationView, error) {
	return service.OrganizationView{}, nil
}

func (stub organizationServiceStub) Delete(context.Context, session.Principal, uuid.UUID) error {
	return stub.deleteErr
}

func (organizationServiceStub) Restore(context.Context, session.Principal, uuid.UUID) (service.OrganizationView, error) {
	return service.OrganizationView{}, nil
}

func TestOrganizationListUsesExactOpenAPIDTOAndNullableDeletion(t *testing.T) {
	t.Parallel()
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	now := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	handler := &ResourceHandler{organizations: organizationServiceStub{page: service.OrganizationPage{
		Items: []service.OrganizationView{{
			ID: organizationID, Name: "Applied Systems Lab", LifecycleState: "active",
			CreatedAt: now, UpdatedAt: now, DeletedAt: nil,
		}},
	}}}
	request := authenticatedResourceRequest(http.MethodGet, "/api/v1/orgs")
	response := httptest.NewRecorder()
	handler.listOrganizations(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	items := body["items"].([]any)
	organization := items[0].(map[string]any)
	assert.Equal(t, organizationID.String(), organization["id"])
	assert.Nil(t, organization["deleted_at"])
	assert.NotContains(t, organization, "public_id")
	assert.NotContains(t, organization, "tenant_version")
	assert.Contains(t, body, "next_cursor")
	assert.Contains(t, body, "previous_cursor")
}

func TestGetDocumentReturnsPersistedProjection(t *testing.T) {
	orgID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	teamID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	documentID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	handler := &ResourceHandler{documents: documentServiceStub{document: service.DocumentProjection{DocumentView: service.DocumentView{ID: documentID, TeamID: teamID, Title: "Notes"}, BodyHTML: "<p>Persisted</p>"}}}
	request := authenticatedResourceRequest(http.MethodGet, "/api/v1/orgs")
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("org_id", orgID.String())
	routeContext.URLParams.Add("team_id", teamID.String())
	routeContext.URLParams.Add("document_id", documentID.String())
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
	response := httptest.NewRecorder()
	handler.getDocument(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"id":"33333333-3333-4333-8333-333333333333","team_id":"22222222-2222-4222-8222-222222222222","title":"Notes","updated_by":"00000000-0000-0000-0000-000000000000","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z","body_html":"<p>Persisted</p>"}`, response.Body.String())
}

func TestIssueDocumentSocketTicketReturnsShortLivedToken(t *testing.T) {
	orgID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	teamID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	documentID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	handler := &ResourceHandler{documents: documentServiceStub{ticket: "signed-document-ticket"}}
	request := authenticatedResourceRequest(http.MethodPost, "/api/v1/orgs")
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("org_id", orgID.String())
	routeContext.URLParams.Add("team_id", teamID.String())
	routeContext.URLParams.Add("document_id", documentID.String())
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
	response := httptest.NewRecorder()

	handler.issueDocumentSocketTicket(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"token":"signed-document-ticket"}`, response.Body.String())
}

func authenticatedResourceRequest(method, target string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	authentication := session.Authentication{Principal: session.Principal{
		UserID:    uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		SessionID: uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
	}}
	return request.WithContext(context.WithValue(request.Context(), authenticationContextKey{}, authentication))
}
