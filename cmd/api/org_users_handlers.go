package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type orgUserResponse struct {
	ID        int64   `json:"id"`
	Email     string  `json:"email"`
	Name      string  `json:"name"`
	OrgRole   string  `json:"org_role"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

func (router *Router) listOrgUsers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireClaims(w, r)
		if !ok {
			return
		}

		orgID, err := parseOrgIDParam(chi.URLParam(r, "orgId"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid org id")
			return
		}
		if claims.OrgID != orgID {
			writeError(w, http.StatusForbidden, "forbidden: wrong organization")
			return
		}

		// Org admins and team leaders both need this to assign people.
		users, err := router.queries.ListUsersByOrg(r.Context(), orgID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list users")
			return
		}

		out := make([]orgUserResponse, 0, len(users))
		for _, u := range users {
			out = append(out, orgUserResponse{
				ID:        u.ID,
				Email:     u.Email,
				Name:      u.Name,
				OrgRole:   string(u.OrgRole),
				AvatarURL: textPtr(u.AvatarUrl),
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}
