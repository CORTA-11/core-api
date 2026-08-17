package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/CORTA-11/core-api/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const teamIDKey contextKey = "teamID"

func TeamMiddleware(teamService service.TeamService, rdb *redis.Client) func(http.Handler) http.Handler {
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

			var teamID int

			// Check redis cache
			key := fmt.Sprintf("team:%s:schema:%s", slug, schemaName)
			cachedTeamID, err := rdb.Get(ctx, key).Result()

			switch {
			case err == nil:
				// Cache hit
				slog.InfoContext(ctx, "teamID found in Redis cache", "key", key)

				teamID, err = strconv.Atoi(cachedTeamID)
				if err != nil {
					slog.ErrorContext(ctx, "invalid cached team ID",
						"key", key,
						"value", cachedTeamID,
						"error", err.Error(),
					)
					http.Error(w, "invalid cached team ID", http.StatusInternalServerError)
					return
				}

			case err == redis.Nil:
				slog.InfoContext(ctx, "teamID cache miss", "key", key)

				teamID, err = teamService.GetTeamID(ctx, slug, schemaName)
				if err != nil {
					slog.ErrorContext(ctx, "failed to get team ID", "error", err)
					http.Error(w, "failed to get team ID", http.StatusInternalServerError)
					return
				}

				// Populate redis
				err := rdb.Set(ctx, key, teamID, 0).Err()
				if err != nil {
					slog.ErrorContext(ctx, "failed to set redis value", "error", err.Error())
				}

			default:
				slog.ErrorContext(ctx, "redis GET failed", "key", key, "error", err.Error())
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
