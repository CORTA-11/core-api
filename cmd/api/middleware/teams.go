package middleware

import (
	"context"
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

			orgIDStr, _ := OrgIDFromContext(ctx)
			orgID, _ := uuid.Parse(orgIDStr)
			schemaName := service.SchemaName(orgID)

			teamID, err := teamService.GetTeamID(ctx, slug, schemaName)
			if err != nil {
				http.Error(w, "failed to get teamID", http.StatusInternalServerError)
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
