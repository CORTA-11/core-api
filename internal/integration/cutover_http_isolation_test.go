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
	router := v1.NewRouter(v1.RouterConfig{
		Manager: manager, Verifier: cutoverCredentialVerifier{userID: fixture.users.shared},
		Organizations: service.NewOrganizationApplication(fixture.runtimePool, codec),
		TeamTasks:     service.NewTeamTaskApplication(authorizer, codec),
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

type cutoverCredentialVerifier struct{ userID uuid.UUID }

func (verifier cutoverCredentialVerifier) Verify(context.Context, string, string) (identity.CredentialPrincipal, error) {
	return identity.CredentialPrincipal{UserPublicID: verifier.userID}, nil
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
