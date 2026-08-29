package apicontract

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func contractPath() string { return filepath.Join("..", "..", "api", "openapi.yaml") }

func TestOpenAPISourceContractIsValidAndClosed(t *testing.T) {
	document, err := Load(context.Background(), contractPath())
	require.NoError(t, err)
	assert.Equal(t, "3.1.2", document.OpenAPI)
	assert.Len(t, document.Paths.Map(), 24)

	legacyFragments := []string{"/users", "/files", "/pprof", "/api/v1/user", "X-Org-ID", "bearer", "jwt"}
	for path := range document.Paths.Map() {
		for _, legacy := range legacyFragments {
			assert.NotContains(t, strings.ToLower(path), strings.ToLower(legacy))
		}
	}
}

func TestValidatorAcceptsDocumentedLoginRequest(t *testing.T) {
	document, err := Load(context.Background(), contractPath())
	require.NoError(t, err)
	validator, err := NewValidator(document)
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPost, "http://example.test/api/v1/auth/login",
		strings.NewReader(`{"email":"researcher@example.com","password":"password-value"}`))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	operation, err := validator.Match(request)
	require.NoError(t, err)
	assert.Equal(t, "login", operation.OperationID())
	require.NoError(t, operation.ValidateRequest(context.Background()))
}

func TestValidatorAcceptsDocumentedRegistrationRequest(t *testing.T) {
	document, err := Load(context.Background(), contractPath())
	require.NoError(t, err)
	validator, err := NewValidator(document)
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPost, "http://example.test/api/v1/auth/register",
		strings.NewReader(`{"display_name":"Researcher","email":"researcher@example.com","password":"password-value-1"}`))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	operation, err := validator.Match(request)
	require.NoError(t, err)
	assert.Equal(t, "register", operation.OperationID())
	require.NoError(t, operation.ValidateRequest(context.Background()))
}

func TestOpenAPIOperationIDsAreUnique(t *testing.T) {
	document, err := Load(context.Background(), contractPath())
	require.NoError(t, err)
	seen := make(map[string]string)
	for path, item := range document.Paths.Map() {
		for method, operation := range item.Operations() {
			require.NotEmpty(t, operation.OperationID, "%s %s", method, path)
			previous, duplicate := seen[operation.OperationID]
			require.False(t, duplicate, "duplicate operationId %q on %s and %s %s", operation.OperationID, previous, method, path)
			seen[operation.OperationID] = method + " " + path
		}
	}
	assert.Len(t, seen, 37)
}

func TestInventoryMatchesOpenAPIBidirectionally(t *testing.T) {
	document, err := Load(context.Background(), contractPath())
	require.NoError(t, err)

	want := make(map[string]string, len(Routes))
	for _, route := range Routes {
		key := route.Method + " " + route.Pattern
		require.NotContains(t, want, key, "duplicate inventory route %s", key)
		want[key] = route.OperationID
	}
	got := make(map[string]string)
	for path, item := range document.Paths.Map() {
		for method, operation := range item.Operations() {
			got[strings.ToUpper(method)+" "+path] = operation.OperationID
		}
	}
	assert.Equal(t, diffRoutes(want, got), diffRoutes(got, want), "missing/extra route diff must be empty")
	assert.Equal(t, want, got)
}

func TestInventoryPoliciesMatchOpenAPI(t *testing.T) {
	document, err := Load(context.Background(), contractPath())
	require.NoError(t, err)
	for _, route := range Routes {
		operation := document.Paths.Find(route.Pattern).GetOperation(route.Method)
		require.NotNil(t, operation, "%s %s", route.Method, route.Pattern)
		if route.Authentication == AuthenticationRequired {
			require.NotNil(t, operation.Security, "%s must declare cookie authentication", route.OperationID)
			require.NotEmpty(t, *operation.Security, "%s must declare cookie authentication", route.OperationID)
		} else {
			assert.True(t, operation.Security == nil || len(*operation.Security) == 0,
				"%s must not require an authenticated session", route.OperationID)
		}
		hasCSRF := false
		for _, parameter := range operation.Parameters {
			if parameter.Value != nil && parameter.Value.In == openapi3.ParameterInHeader &&
				strings.EqualFold(parameter.Value.Name, "X-CSRF-Token") && parameter.Value.Required {
				hasCSRF = true
			}
		}
		assert.Equal(t, route.CSRF == CSRFRequired, hasCSRF, "%s CSRF policy", route.OperationID)
		assert.Equal(t, route.BodyLimit != BodyNone, operation.RequestBody != nil, "%s body policy", route.OperationID)
	}
}

func TestInventoryMetadataUsesClosedValues(t *testing.T) {
	for _, route := range Routes {
		assert.Contains(t, []AuthenticationPolicy{AuthenticationPublic, AuthenticationRequired, AuthenticationLogout}, route.Authentication)
		assert.Contains(t, []CSRFPolicy{CSRFNone, CSRFRequired}, route.CSRF)
		assert.Contains(t, []BodyLimitClass{BodyNone, BodyAuthJSON, BodyJSON}, route.BodyLimit)
		assert.Contains(t, []RateLimitClass{RateNone, RateLogin, RateRegistration, RateAdministrative}, route.RateLimit)
		if route.Permission != "" {
			assert.True(t, authorization.ValidPermission(route.Permission), "%s permission", route.OperationID)
		}
	}
}

func diffRoutes(left, right map[string]string) []string {
	var result []string
	for key, operationID := range left {
		if other, ok := right[key]; !ok {
			result = append(result, fmt.Sprintf("%s (%s)", key, operationID))
		} else if other != operationID {
			result = append(result, fmt.Sprintf("%s (%s != %s)", key, operationID, other))
		}
	}
	sort.Strings(result)
	return result
}

func TestEveryMediaExampleValidatesAgainstItsSchema(t *testing.T) {
	document, err := Load(context.Background(), contractPath())
	require.NoError(t, err)
	for path, item := range document.Paths.Map() {
		for method, operation := range item.Operations() {
			if operation.RequestBody != nil && operation.RequestBody.Value != nil {
				validateContentExamples(t, method+" "+path+" request", operation.RequestBody.Value.Content)
			}
			for status, response := range operation.Responses.Map() {
				if response.Value != nil {
					validateContentExamples(t, method+" "+path+" response "+status, response.Value.Content)
				}
			}
		}
	}
}

func validateContentExamples(t *testing.T, location string, content openapi3.Content) {
	t.Helper()
	for mediaName, media := range content {
		require.NotNil(t, media.Schema, "%s %s has example without schema", location, mediaName)
		require.NotNil(t, media.Schema.Value, "%s %s schema is unresolved", location, mediaName)
		if media.Example != nil {
			require.NoError(t, media.Schema.Value.VisitJSON(media.Example), "%s %s", location, mediaName)
		}
		for name, example := range media.Examples {
			require.NotNil(t, example.Value, "%s %s example %s is unresolved", location, mediaName, name)
			require.NoError(t, media.Schema.Value.VisitJSON(example.Value.Value), "%s %s example %s", location, mediaName, name)
		}
	}
}

func TestValidatorRejectsUndocumentedResponseStatus(t *testing.T) {
	document, err := Load(context.Background(), contractPath())
	require.NoError(t, err)
	validator, err := NewValidator(document)
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodGet, "http://example.test/api/v1/auth/session", nil)
	require.NoError(t, err)
	operation, err := validator.Match(request)
	require.NoError(t, err)
	err = operation.ValidateResponse(context.Background(), http.StatusTeapot, http.Header{}, nil)
	assert.Error(t, err)
}
