package middleware

import (
	"context"
	"net/http"

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

func WithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, orgIDKey, orgID)
}

func OrgIDFromContext(ctx context.Context) (string, bool) {
	orgID, ok := ctx.Value(orgIDKey).(string)
	return orgID, ok
}
