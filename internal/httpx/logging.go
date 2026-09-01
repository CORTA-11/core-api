package httpx

import (
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/CORTA-11/core-api/internal/session"
	"github.com/go-chi/chi/v5"
)

// BoundaryLog boundarys log.
func BoundaryLog(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		tracked := &boundaryResponseWriter{ResponseWriter: writer}
		next.ServeHTTP(tracked, request)
		status := tracked.status
		if status == 0 {
			status = http.StatusOK
		}
		attributes := []any{
			"request_id", RequestIDFromContext(request.Context()),
			"method", request.Method,
			"route", routePattern(request),
			"status", status,
			"duration_us", time.Since(started).Microseconds(),
			"response_bytes", tracked.bytes,
		}
		if client, ok := ClientFromContext(request.Context()); ok {
			attributes = append(attributes, "client_prefix", maskedPrefix(client.Address))
		}
		if principal, ok := session.PrincipalFromContext(request.Context()); ok {
			if principal.UserID.Version() != 0 {
				attributes = append(attributes, "user_id", principal.UserID.String())
			}
			if principal.SessionID.Version() != 0 {
				attributes = append(attributes, "session_id", principal.SessionID.String())
			}
		}
		if problem := problemForStatus(status); problem != "" {
			attributes = append(attributes, "problem_class", problem)
		}
		logger.InfoContext(request.Context(), "request_boundary", attributes...)
	})
}

type boundaryResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

// WriteHeader writes header.
func (writer *boundaryResponseWriter) WriteHeader(status int) {
	if writer.status == 0 {
		writer.status = status
	}
	writer.ResponseWriter.WriteHeader(status)
}

// Write writes the supplied data.
func (writer *boundaryResponseWriter) Write(body []byte) (int, error) {
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	written, err := writer.ResponseWriter.Write(body)
	writer.bytes += written
	return written, err
}

// Unwrap returns the underlying error.
func (writer *boundaryResponseWriter) Unwrap() http.ResponseWriter { return writer.ResponseWriter }

// maskedPrefix maskeds prefix.
func maskedPrefix(address netip.Addr) string {
	address = address.Unmap()
	bits := 56
	if address.Is4() {
		bits = 24
	}
	return netip.PrefixFrom(address, bits).Masked().String()
}

// routePattern routes pattern.
func routePattern(request *http.Request) string {
	if routeContext := chi.RouteContext(request.Context()); routeContext != nil {
		return routeContext.RoutePattern()
	}
	return ""
}

// problemForStatus problems for status.
func problemForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return string(ProblemInvalidRequest)
	case http.StatusUnauthorized:
		return string(ProblemUnauthenticated)
	case http.StatusForbidden:
		return string(ProblemForbidden)
	case http.StatusNotFound:
		return string(ProblemNotFound)
	case http.StatusConflict:
		return string(ProblemConflict)
	case http.StatusPreconditionFailed:
		return string(ProblemPreconditionFailed)
	case http.StatusTooManyRequests:
		return string(ProblemRateLimited)
	case http.StatusServiceUnavailable:
		return string(ProblemDependencyUnavailable)
	case http.StatusInternalServerError:
		return string(ProblemInternalFailure)
	default:
		return ""
	}
}
