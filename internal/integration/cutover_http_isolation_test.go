//go:build isolation

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	v1 "github.com/CORTA-11/core-api/cmd/api/handlers/v1"
	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/ratelimit"
	"github.com/CORTA-11/core-api/internal/realtime"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCutoverRouterBrowserOrganizationTeamTaskFlowAndAuthorizationNegatives(t *testing.T) {
	fixture := newTenantBoundaryFixture(t)
	ctx := context.Background()
	organization := fixture.orgs[0]
	otherOrganization := fixture.orgs[1]
	_, err := fixture.adminPool.Exec(ctx, `
		UPDATE public.org_user SET role = 'owner'
		WHERE org_id = $1 AND user_id = (SELECT id FROM public.users WHERE user_id = $2)`,
		organization.id, fixture.users.shared)
	require.NoError(t, err)

	manager, err := session.NewManager(fixture.runtimePool, bytes.Repeat([]byte{0x63}, 32))
	require.NoError(t, err)
	codec, err := pagination.NewCodec(pagination.CodecConfig{Active: pagination.Key{
		ID: "integration-v1", Secret: bytes.Repeat([]byte{0x71}, 32),
	}})
	require.NoError(t, err)
	redisClient := testsupport.OpenRedis(t)
	testsupport.FlushRedis(t, redisClient)
	limiter, err := ratelimit.NewRedis(redisClient, bytes.Repeat([]byte{0x52}, 32), time.Second)
	require.NoError(t, err)
	loginGuard, err := ratelimit.NewLoginGuard(limiter, ratelimit.DefaultPolicies())
	require.NoError(t, err)
	administrative, err := ratelimit.NewAdministrativeMiddleware(limiter, ratelimit.DefaultPolicies().Administrative)
	require.NoError(t, err)
	origins, err := httpx.ParseOriginPolicy("https://app.example", "test")
	require.NoError(t, err)
	trusted, err := httpx.ParseTrustedProxies("")
	require.NoError(t, err)
	authorizer := authorization.NewAuthorizer(fixture.resolver, fixture.executor)
	ticketSecret := bytes.Repeat([]byte{0x81}, 32)
	documents := service.NewDocumentApplication(authorizer, ticketSecret)
	chatChannel := "chat:cutover"
	chatService := service.NewChatApplication(authorizer, realtime.NewChatPublisher(redisClient, chatChannel), ticketSecret)
	router := v1.NewRouter(v1.RouterConfig{
		Manager: manager, Verifier: cutoverCredentialVerifier{users: map[string]uuid.UUID{
			"shared@tenant-boundary.example.test":   fixture.users.shared,
			"alpha@tenant-boundary.example.test":    fixture.users.alpha,
			"outsider@tenant-boundary.example.test": fixture.users.outsider,
		}},
		Organizations: service.NewOrganizationApplication(fixture.runtimePool, codec),
		TeamTasks:     service.NewTeamTaskApplication(authorizer, codec),
		Documents:     documents,
		Chat:          chatService,
		Environment:   "test", Origins: origins, TrustedProxies: trusted,
		LoginGuard: loginGuard, Administrative: administrative,
	})
	server := httptest.NewServer(router.Handler())
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}

	loginResponse := cutoverRequest(t, client, http.MethodPost, server.URL+"/api/v1/auth/login",
		`{"email":"shared@tenant-boundary.example.test","password":"test-password"}`, "", "")
	require.Equal(t, http.StatusOK, loginResponse.status)
	var loginBody struct {
		CSRFToken string `json:"csrf_token"`
	}
	require.NoError(t, json.Unmarshal(loginResponse.body, &loginBody))
	require.NotEmpty(t, loginBody.CSRFToken)

	organizations := cutoverRequest(t, client, http.MethodGet, server.URL+"/api/v1/orgs?page_size=1", "", "", "")
	require.Equal(t, http.StatusOK, organizations.status)
	assert.Contains(t, string(organizations.body), `"next_cursor"`)

	teamResponse := cutoverRequest(t, client, http.MethodPost,
		server.URL+"/api/v1/orgs/"+organization.publicID.String()+"/teams",
		`{"name":"Browser Research Team","leader_email":"shared@tenant-boundary.example.test"}`, loginBody.CSRFToken, "https://app.example")
	require.Equal(t, http.StatusCreated, teamResponse.status, string(teamResponse.body))
	var team struct {
		ID uuid.UUID `json:"id"`
	}
	require.NoError(t, json.Unmarshal(teamResponse.body, &team))
	require.NotEqual(t, uuid.Nil, team.ID)

	taskPath := server.URL + "/api/v1/orgs/" + organization.publicID.String() + "/teams/" + team.ID.String() + "/tasks"
	taskResponse := cutoverRequest(t, client, http.MethodPost, taskPath,
		`{"description":"Reproduce benchmark","status":"todo"}`, loginBody.CSRFToken, "https://app.example")
	require.Equal(t, http.StatusCreated, taskResponse.status, string(taskResponse.body))
	var task struct {
		ID uuid.UUID `json:"id"`
	}
	require.NoError(t, json.Unmarshal(taskResponse.body, &task))

	documentsPath := server.URL + "/api/v1/orgs/" + organization.publicID.String() + "/teams/" + team.ID.String() + "/documents"
	documentResponse := cutoverRequest(t, client, http.MethodPost, documentsPath,
		`{"title":"Persisted calibration notes"}`, loginBody.CSRFToken, "https://app.example")
	require.Equal(t, http.StatusCreated, documentResponse.status, string(documentResponse.body))
	var document struct {
		ID uuid.UUID `json:"id"`
	}
	require.NoError(t, json.Unmarshal(documentResponse.body, &document))
	documentTicketPath := documentsPath + "/" + document.ID.String() + "/socket-ticket"
	documentTicket := cutoverRequest(t, client, http.MethodPost, documentTicketPath, "", loginBody.CSRFToken, "https://app.example")
	require.Equal(t, http.StatusOK, documentTicket.status, string(documentTicket.body))
	assert.Contains(t, string(documentTicket.body), `"token"`)
	missingDocumentTicket := cutoverRequest(t, client, http.MethodPost,
		documentsPath+"/40000000-0000-4000-8000-000000000099/socket-ticket", "", loginBody.CSRFToken, "https://app.example")
	assert.Equal(t, http.StatusNotFound, missingDocumentTicket.status)
	crossTeamTicket := cutoverRequest(t, client, http.MethodPost,
		server.URL+"/api/v1/orgs/"+organization.publicID.String()+"/teams/"+organization.teams[0].publicID.String()+"/documents/"+document.ID.String()+"/socket-ticket",
		"", loginBody.CSRFToken, "https://app.example")
	assert.Equal(t, http.StatusNotFound, crossTeamTicket.status)
	crossOrganizationTicket := cutoverRequest(t, client, http.MethodPost,
		server.URL+"/api/v1/orgs/"+otherOrganization.publicID.String()+"/teams/"+team.ID.String()+"/documents/"+document.ID.String()+"/socket-ticket",
		"", loginBody.CSRFToken, "https://app.example")
	assert.Equal(t, http.StatusNotFound, crossOrganizationTicket.status)
	unauthenticatedTicket := cutoverRequest(t, &http.Client{Timeout: 5 * time.Second}, http.MethodPost, documentTicketPath, "", "", "https://app.example")
	assert.Equal(t, http.StatusUnauthorized, unauthenticatedTicket.status)
	badTicketCSRF := cutoverRequest(t, client, http.MethodPost, documentTicketPath, "", "invalid", "https://app.example")
	assert.Equal(t, http.StatusForbidden, badTicketCSRF.status)
	documentsTable := pgx.Identifier{organization.schema, "documents"}.Sanitize()
	_, err = fixture.adminPool.Exec(ctx, `UPDATE `+documentsTable+` SET body_html = '<p>Persisted body</p>' WHERE public_id = $1`, document.ID)
	require.NoError(t, err)
	documentPath := documentsPath + "/" + document.ID.String()
	openedDocument := cutoverRequest(t, client, http.MethodGet, documentPath, "", "", "")
	require.Equal(t, http.StatusOK, openedDocument.status, string(openedDocument.body))
	var projection struct {
		ID             uuid.UUID       `json:"id"`
		TeamID         uuid.UUID       `json:"team_id"`
		UpdatedBy      uuid.UUID       `json:"updated_by"`
		Title          string          `json:"title"`
		BodyHTML       string          `json:"body_html"`
		CanonicalState json.RawMessage `json:"canonical_state"`
	}
	require.NoError(t, json.Unmarshal(openedDocument.body, &projection))
	assert.Equal(t, document.ID, projection.ID)
	assert.Equal(t, team.ID, projection.TeamID)
	assert.Equal(t, fixture.users.shared, projection.UpdatedBy)
	assert.Equal(t, "Persisted calibration notes", projection.Title)
	assert.Equal(t, "<p>Persisted body</p>", projection.BodyHTML)
	assert.Nil(t, projection.CanonicalState)
	_, err = fixture.adminPool.Exec(ctx, `UPDATE public.org_user SET role = 'administrator'
		WHERE org_id = $1 AND user_id = (SELECT id FROM public.users WHERE user_id = $2)`, organization.id, fixture.users.alpha)
	require.NoError(t, err)
	administratorClient := cutoverLoginClient(t, server.URL, "alpha@tenant-boundary.example.test")
	administratorDenied := cutoverRequest(t, administratorClient, http.MethodGet, documentPath, "", "", "")
	assert.Equal(t, http.StatusNotFound, administratorDenied.status)
	operatorClient := cutoverLoginClient(t, server.URL, "outsider@tenant-boundary.example.test")
	operatorDenied := cutoverRequest(t, operatorClient, http.MethodGet, documentPath, "", "", "")
	assert.Equal(t, http.StatusNotFound, operatorDenied.status)

	missingDocument := cutoverRequest(t, client, http.MethodGet, documentsPath+"/40000000-0000-4000-8000-000000000099", "", "", "")
	assert.Equal(t, http.StatusNotFound, missingDocument.status)
	crossTeamDocument := cutoverRequest(t, client, http.MethodGet,
		server.URL+"/api/v1/orgs/"+organization.publicID.String()+"/teams/"+organization.teams[0].publicID.String()+"/documents/"+document.ID.String(), "", "", "")
	assert.Equal(t, http.StatusNotFound, crossTeamDocument.status)
	crossOrganizationDocument := cutoverRequest(t, client, http.MethodGet,
		server.URL+"/api/v1/orgs/"+otherOrganization.publicID.String()+"/teams/"+team.ID.String()+"/documents/"+document.ID.String(), "", "", "")
	assert.Equal(t, http.StatusNotFound, crossOrganizationDocument.status)
	assertSameProblem(t, missingDocument, crossTeamDocument, crossOrganizationDocument, administratorDenied, operatorDenied)
	unauthenticatedDocument := cutoverRequest(t, &http.Client{Timeout: 5 * time.Second}, http.MethodGet, documentPath, "", "", "")
	assert.Equal(t, http.StatusUnauthorized, unauthenticatedDocument.status)

	pubsub := redisClient.Subscribe(ctx, chatChannel)
	t.Cleanup(func() { _ = pubsub.Close() })
	chatPath := server.URL + "/api/v1/orgs/" + organization.publicID.String() + "/teams/" + team.ID.String() + "/chat"
	chatResponse := cutoverRequest(t, client, http.MethodPost, chatPath+"/messages",
		`{"message":"Browser chat e2e"}`, loginBody.CSRFToken, "https://app.example")
	require.Equal(t, http.StatusCreated, chatResponse.status, string(chatResponse.body))
	assert.Contains(t, string(chatResponse.body), `"message":"Browser chat e2e"`)
	socketTicket := cutoverRequest(t, client, http.MethodPost, chatPath+"/socket-ticket", "", loginBody.CSRFToken, "https://app.example")
	require.Equal(t, http.StatusOK, socketTicket.status, string(socketTicket.body))
	assert.Contains(t, string(socketTicket.body), `"token"`)
	event, err := pubsub.ReceiveMessage(ctx)
	require.NoError(t, err)
	assert.Contains(t, event.Payload, `"type":"message.created"`)

	wrongScope := cutoverRequest(t, client, http.MethodGet,
		server.URL+"/api/v1/orgs/"+otherOrganization.publicID.String()+"/teams/"+team.ID.String()+"/tasks",
		"", "", "")
	assert.Equal(t, http.StatusNotFound, wrongScope.status)

	membersTable := pgx.Identifier{organization.schema, "team_members"}.Sanitize()
	teamsTable := pgx.Identifier{organization.schema, "teams"}.Sanitize()
	_, err = fixture.adminPool.Exec(ctx, `UPDATE `+membersTable+` SET role = 'viewer'
		WHERE team_id = (SELECT id FROM `+teamsTable+` WHERE public_id = $1) AND user_public_id = $2`,
		team.ID, fixture.users.shared)
	require.NoError(t, err)
	denied := cutoverRequest(t, client, http.MethodPost, taskPath,
		`{"description":"Denied mutation","status":"todo"}`, loginBody.CSRFToken, "https://app.example")
	assert.Equal(t, http.StatusForbidden, denied.status)

	_, err = fixture.adminPool.Exec(ctx, `DELETE FROM `+membersTable+`
		WHERE team_id = (SELECT id FROM `+teamsTable+` WHERE public_id = $1) AND user_public_id = $2`,
		team.ID, fixture.users.shared)
	require.NoError(t, err)
	removed := cutoverRequest(t, client, http.MethodGet, taskPath, "", "", "")
	assert.Equal(t, http.StatusNotFound, removed.status)
	removedDocument := cutoverRequest(t, client, http.MethodGet, documentPath, "", "", "")
	assert.Equal(t, http.StatusNotFound, removedDocument.status)
	assertSameProblem(t, missingDocument, removedDocument)

	badCSRF := cutoverRequest(t, client, http.MethodDelete,
		fmt.Sprintf("%s/%s", taskPath, task.ID), "", "invalid", "https://app.example")
	assert.Equal(t, http.StatusForbidden, badCSRF.status)

	organizationPath := server.URL + "/api/v1/orgs/" + organization.publicID.String()
	deleted := cutoverRequest(t, client, http.MethodDelete, organizationPath, "", loginBody.CSRFToken, "https://app.example")
	require.Equal(t, http.StatusNoContent, deleted.status, string(deleted.body))
	retained := cutoverRequest(t, client, http.MethodGet, organizationPath, "", "", "")
	require.Equal(t, http.StatusOK, retained.status, string(retained.body))
	assert.Contains(t, string(retained.body), `"lifecycle_state":"deleting"`)
	restored := cutoverRequest(t, client, http.MethodPost, organizationPath+"/restore", "", loginBody.CSRFToken, "https://app.example")
	require.Equal(t, http.StatusOK, restored.status, string(restored.body))
}

type cutoverResponse struct {
	status int
	body   []byte
}

type cutoverCredentialVerifier struct{ users map[string]uuid.UUID }

func (verifier cutoverCredentialVerifier) Verify(_ context.Context, email, _ string) (identity.CredentialPrincipal, error) {
	userID, ok := verifier.users[email]
	if !ok {
		return identity.CredentialPrincipal{}, identity.ErrInvalidCredentials
	}
	return identity.CredentialPrincipal{UserPublicID: userID}, nil
}

func cutoverLoginClient(t *testing.T, serverURL, email string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	response := cutoverRequest(t, client, http.MethodPost, serverURL+"/api/v1/auth/login",
		fmt.Sprintf(`{"email":%q,"password":"test-password"}`, email), "", "")
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	return client
}

func assertSameProblem(t *testing.T, expected cutoverResponse, actual ...cutoverResponse) {
	t.Helper()
	normalize := func(body []byte) map[string]any {
		var problem map[string]any
		require.NoError(t, json.Unmarshal(body, &problem))
		delete(problem, "request_id")
		delete(problem, "instance")
		return problem
	}
	want := normalize(expected.body)
	for _, response := range actual {
		assert.Equal(t, want, normalize(response.body))
	}
}

func cutoverRequest(
	t *testing.T, client *http.Client, method, target, body, csrf, origin string,
) cutoverResponse {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), method, target, bytes.NewBufferString(body))
	require.NoError(t, err)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	response, err := client.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	var output bytes.Buffer
	_, err = output.ReadFrom(response.Body)
	require.NoError(t, err)
	return cutoverResponse{status: response.StatusCode, body: output.Bytes()}
}
