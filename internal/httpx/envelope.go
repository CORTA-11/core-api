package httpx

import (
	"net/http"
	"net/textproto"
	"strings"
)

const (
	allowedMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	allowedHeaders = "Content-Type, X-CSRF-Token, If-Match, Idempotency-Key"
)

var corsMethods = map[string]struct{}{
	http.MethodGet: {}, http.MethodPost: {}, http.MethodPut: {}, http.MethodPatch: {},
	http.MethodDelete: {}, http.MethodOptions: {},
}

var corsHeaders = map[string]struct{}{
	"Content-Type": {}, "X-Csrf-Token": {}, "If-Match": {}, "Idempotency-Key": {},
}

func CORS(policy OriginPolicy, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		addVary(writer.Header(), "Origin")
		origin := singleHeader(request.Header, "Origin")
		if request.Method == http.MethodOptions {
			addVary(writer.Header(), "Access-Control-Request-Method")
			addVary(writer.Header(), "Access-Control-Request-Headers")
			method := singleHeader(request.Header, "Access-Control-Request-Method")
			headers := singleHeader(request.Header, "Access-Control-Request-Headers")
			if !policy.Allows(origin) || !validCORSMethod(method) || !validCORSHeaders(headers) {
				_ = WriteProblem(writer, request, NewError(ProblemForbidden, nil))
				return
			}
			grantCORS(writer.Header(), origin)
			writer.Header().Set("Access-Control-Allow-Methods", allowedMethods)
			writer.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		if policy.Allows(origin) {
			grantCORS(writer.Header(), origin)
		}
		next.ServeHTTP(writer, request)
	})
}

func SecurityHeaders(environment string, authenticationResponse bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		header := writer.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		if authenticationResponse {
			header.Set("Cache-Control", "no-store")
		}
		if client, ok := ClientFromContext(request.Context()); ok && environment == "production" && client.Scheme == "https" {
			header.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(writer, request)
	})
}

func grantCORS(header http.Header, origin string) {
	header.Set("Access-Control-Allow-Origin", origin)
	header.Set("Access-Control-Allow-Credentials", "true")
}

func validCORSMethod(method string) bool {
	_, ok := corsMethods[method]
	return ok && method != ""
}

func validCORSHeaders(raw string) bool {
	if raw == "" {
		return true
	}
	seen := make(map[string]struct{})
	for _, value := range strings.Split(raw, ",") {
		name := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(value))
		if name == "" {
			return false
		}
		if _, ok := corsHeaders[name]; !ok {
			return false
		}
		if _, duplicate := seen[name]; duplicate {
			return false
		}
		seen[name] = struct{}{}
	}
	return true
}

func singleHeader(header http.Header, name string) string {
	values := header.Values(name)
	if len(values) != 1 || strings.Contains(values[0], "\x00") {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func addVary(header http.Header, value string) {
	for _, existing := range header.Values("Vary") {
		for _, item := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(item), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}
