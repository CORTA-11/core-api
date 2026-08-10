package main

import (
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/google/uuid"
)

func (router *Router) requireInternalKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := os.Getenv("INTERNAL_API_KEY")
		if key == "" {
			key = "dev-internal-key-change-me"
		}
		if r.Header.Get("X-Internal-Key") != key {
			writeError(w, http.StatusUnauthorized, "invalid internal key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// internalTeamAccess lets the socket-server verify team membership for a user.
func (router *Router) internalTeamAccess() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamPublicID, err := uuid.Parse(r.URL.Query().Get("team_public_id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid team_public_id")
			return
		}

		userID, err := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
		if err != nil || userID < 1 {
			writeError(w, http.StatusBadRequest, "invalid user_id")
			return
		}

		membership, err := loadTeamMembership(r, router.queries, teamPublicID, userID)
		if err != nil {
			if errors.Is(err, errTeamNotFound) {
				writeError(w, http.StatusNotFound, "team not found")
				return
			}
			if errors.Is(err, errNotTeamMember) {
				writeError(w, http.StatusForbidden, "not a team member")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to check membership")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"team_id":   membership.TeamID,
			"public_id": membership.PublicID,
			"org_id":    membership.OrgID,
			"role":      membership.Role,
		})
	}
}
