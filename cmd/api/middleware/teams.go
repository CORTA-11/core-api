package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/CORTA-11/core-api/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const teamIDKey contextKey = "teamID"

func TeamMiddleware(teamService service.TeamService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			slug := chi.URLParam(r, "team")
			if slug == "" {
				http.Error(w, "empty team param", http.StatusBadRequest)
				return
			}

			orgIDStr, ok := OrgIDFromContext(ctx)
			if !ok {
				slog.ErrorContext(ctx, "organization ID missing from request context")
				http.Error(w, "failed to get organization ID", http.StatusInternalServerError)
				return
			}

			orgID, err := uuid.Parse(orgIDStr)
			if err != nil {
				slog.ErrorContext(ctx, "invalid organization ID in request context", "error", err)
				http.Error(w, "failed to get organization ID", http.StatusInternalServerError)
				return
			}

			schemaName := service.SchemaName(orgID)

			teamID, err := teamService.GetTeamID(ctx, slug, schemaName)
			if err != nil {
				slog.ErrorContext(ctx, "failed to get team ID", "error", err)
				http.Error(w, "failed to get team ID", http.StatusInternalServerError)
				return
			}

			ctx = WithTeamID(ctx, teamID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func WithTeamID(ctx context.Context, teamID int) context.Context {
	return context.WithValue(ctx, teamIDKey, teamID)
}

func TeamIDFromContext(ctx context.Context) (int, bool) {
	teamID, ok := ctx.Value(teamIDKey).(int)
	return teamID, ok
}
