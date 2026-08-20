package middleware

import (
	"context"
	"net/http"

	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/google/uuid"
)

type contextKey string

const orgIDKey contextKey = "orgID"

func OrgMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgIDStr := r.Header.Get("X-Org-ID")
		if orgIDStr == "" {
			http.Error(w, "organization id header is missing", http.StatusBadRequest)
			return
		}

		if err := uuid.Validate(orgIDStr); err != nil {
			http.Error(w, "invalid uuid", http.StatusBadRequest)
			return
		}

		ctx := WithOrgID(r.Context(), orgIDStr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type OrgAvailabilityChecker interface {
	Check(context.Context, uuid.UUID) (tenancy.Availability, error)
}

func RequireAvailableOrg(checker OrgAvailabilityChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			orgID, ok := OrgIDFromContext(r.Context())
			if !ok {
				http.Error(w, "organization context is missing", http.StatusInternalServerError)
				return
			}
			publicID, err := uuid.Parse(orgID)
			if err != nil {
				http.Error(w, "invalid organization context", http.StatusInternalServerError)
				return
			}
			availability, err := checker.Check(r.Context(), publicID)
			if err != nil {
				http.Error(w, "organization availability check failed", http.StatusServiceUnavailable)
				return
			}
			switch availability {
			case tenancy.AvailabilityUnknown:
				http.Error(w, "organization not found", http.StatusNotFound)
			case tenancy.AvailabilityUnavailable:
				http.Error(w, "organization is not available", http.StatusServiceUnavailable)
			case tenancy.AvailabilityReady:
				next.ServeHTTP(w, r)
			default:
				http.Error(w, "organization availability check failed", http.StatusServiceUnavailable)
			}
		})
	}
}

func WithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, orgIDKey, orgID)
}

func OrgIDFromContext(ctx context.Context) (string, bool) {
	orgID, ok := ctx.Value(orgIDKey).(string)
	return orgID, ok
}
