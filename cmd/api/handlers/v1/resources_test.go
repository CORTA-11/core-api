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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type organizationServiceStub struct {
	page      service.OrganizationPage
	deleteErr error
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

func authenticatedResourceRequest(method, target string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	authentication := session.Authentication{Principal: session.Principal{
		UserID:    uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		SessionID: uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"),
	}}
	return request.WithContext(context.WithValue(request.Context(), authenticationContextKey{}, authentication))
}
