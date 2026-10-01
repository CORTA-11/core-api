//go:build isolation

package integration_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func exerciseContentAccessHTTP(t *testing.T, serverURL string, creator *http.Client, csrf string, organizationID, teamID uuid.UUID) {
	t.Helper()
	base := serverURL + "/api/v1/orgs/" + organizationID.String() + "/teams/" + teamID.String()
	created := cutoverRequest(t, creator, http.MethodPost, base+"/documents", `{"title":"Creator controlled notes"}`, csrf, "https://app.example")
	require.Equal(t, http.StatusCreated, created.status, string(created.body))
	var doc struct {
		ID uuid.UUID `json:"id"`
	}
	require.NoError(t, json.Unmarshal(created.body, &doc))
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	member := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	login := cutoverRequest(t, member, http.MethodPost, serverURL+"/api/v1/auth/login", `{"email":"alpha@tenant-boundary.example.test","password":"test-password"}`, "", "")
	require.Equal(t, http.StatusOK, login.status)
	var auth struct {
		CSRF string `json:"csrf_token"`
	}
	require.NoError(t, json.Unmarshal(login.body, &auth))
	path := base + "/documents/" + doc.ID.String()
	require.Equal(t, http.StatusNotFound, cutoverRequest(t, member, http.MethodGet, path, "", "", "").status)
	access := base + "/content-access"
	input := `{"kind":"document","resource_id":"` + doc.ID.String() + `"}`
	require.Equal(t, http.StatusForbidden, cutoverRequest(t, member, http.MethodPost, access+"/requests", input, "invalid", "https://app.example").status)
	require.Equal(t, http.StatusNoContent, cutoverRequest(t, member, http.MethodPost, access+"/requests", input, auth.CSRF, "https://app.example").status)
	snapshot := cutoverRequest(t, creator, http.MethodGet, access, "", "", "")
	require.Equal(t, http.StatusOK, snapshot.status)
	var view struct {
		Requests []struct {
			ID uuid.UUID `json:"public_id"`
		} `json:"requests"`
	}
	require.NoError(t, json.Unmarshal(snapshot.body, &view))
	require.Len(t, view.Requests, 1)
	requestPath := access + "/requests/" + view.Requests[0].ID.String()
	require.Equal(t, http.StatusNotFound, cutoverRequest(t, member, http.MethodPost, requestPath+"/approve", "", auth.CSRF, "https://app.example").status)
	require.Equal(t, http.StatusNoContent, cutoverRequest(t, creator, http.MethodPost, requestPath+"/deny", "", csrf, "https://app.example").status)
	require.Equal(t, http.StatusNotFound, cutoverRequest(t, member, http.MethodGet, path, "", "", "").status)
	require.Equal(t, http.StatusNoContent, cutoverRequest(t, member, http.MethodPost, access+"/requests", input, auth.CSRF, "https://app.example").status)
	require.Equal(t, http.StatusNoContent, cutoverRequest(t, creator, http.MethodPost, requestPath+"/approve", "", csrf, "https://app.example").status)
	require.Equal(t, http.StatusOK, cutoverRequest(t, member, http.MethodGet, path, "", "", "").status)
	// Reconnects must deliver the latest approved state with cookie authentication.
	response, err := member.Get(access + "/events")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "text/event-stream", response.Header.Get("Content-Type"))
	scanner := bufio.NewScanner(response.Body)
	require.True(t, scanner.Scan())
	require.Equal(t, "event: content-access", scanner.Text())
	require.True(t, scanner.Scan())
	require.True(t, strings.HasPrefix(scanner.Text(), "data: "))
	require.Contains(t, scanner.Text(), `"status":"granted"`)
	require.Contains(t, scanner.Text(), `"can_access":true`)
}
