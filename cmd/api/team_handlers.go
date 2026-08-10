package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/CORTA-11/core-api/internal/auth"
	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type teamResponse struct {
	ID          string  `json:"id"`
	PublicID    string  `json:"public_id"`
	OrgID       int64   `json:"org_id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	MyRole      *string `json:"my_role,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

type teamMemberResponse struct {
	UserID    int64   `json:"user_id"`
	Name      string  `json:"name"`
	Email     string  `json:"email"`
	AvatarURL *string `json:"avatar_url,omitempty"`
	Role      string  `json:"role"`
	JoinedAt  string  `json:"joined_at"`
}

type createTeamRequest struct {
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	LeaderUserID *int64  `json:"leader_user_id"`
}

type userIDRequest struct {
	UserID int64 `json:"user_id"`
}

func (router *Router) listTeams() http.HandlerFunc {
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

		// Org admins see every team so they can assign leaders / manage.
		if claims.OrgRole == "ORG_ADMIN" {
			teams, err := router.queries.ListTeamsByOrg(r.Context(), orgID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to list teams")
				return
			}
			out := make([]teamResponse, 0, len(teams))
			for _, team := range teams {
				var myRole *string
				membership, mErr := router.queries.GetTeamMembership(r.Context(), repository.GetTeamMembershipParams{
					TeamID: team.ID,
					UserID: claims.UserID,
				})
				if mErr == nil {
					role := string(membership.Role)
					myRole = &role
				}
				out = append(out, teamResponse{
					ID:          team.PublicID.String(),
					PublicID:    team.PublicID.String(),
					OrgID:       team.OrgID,
					Name:        team.Name,
					Description: textPtr(team.Description),
					MyRole:      myRole,
					CreatedAt:   team.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
				})
			}
			writeJSON(w, http.StatusOK, out)
			return
		}

		teams, err := router.queries.ListTeamsForUser(r.Context(), repository.ListTeamsForUserParams{
			OrgID:  orgID,
			UserID: claims.UserID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list teams")
			return
		}

		out := make([]teamResponse, 0, len(teams))
		for _, team := range teams {
			role := string(team.MyRole)
			out = append(out, teamResponse{
				ID:          team.PublicID.String(),
				PublicID:    team.PublicID.String(),
				OrgID:       team.OrgID,
				Name:        team.Name,
				Description: textPtr(team.Description),
				MyRole:      &role,
				CreatedAt:   team.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (router *Router) createTeam() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireClaims(w, r)
		if !ok {
			return
		}
		if claims.OrgRole != "ORG_ADMIN" {
			writeError(w, http.StatusForbidden, "forbidden: insufficient permissions")
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

		var req createTeamRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request payload")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}

		if req.LeaderUserID == nil || *req.LeaderUserID < 1 {
			writeError(w, http.StatusBadRequest, "leader_user_id is required")
			return
		}
		leaderID := *req.LeaderUserID

		leader, err := router.queries.GetUserByID(r.Context(), leaderID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusBadRequest, "leader user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to load leader")
			return
		}
		if leader.OrgID != orgID || !leader.Active {
			writeError(w, http.StatusBadRequest, "leader must be an active user in this organization")
			return
		}
		if string(leader.OrgRole) == "ORG_ADMIN" {
			writeError(w, http.StatusBadRequest, "assign a team leader who is an organization member, not an org admin")
			return
		}

		tx, err := router.db.Begin(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to start transaction")
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()
		qtx := router.queries.WithTx(tx)

		desc := pgtype.Text{}
		if req.Description != nil {
			trimmed := strings.TrimSpace(*req.Description)
			if trimmed != "" {
				desc = pgtype.Text{String: trimmed, Valid: true}
			}
		}

		team, err := qtx.CreateTeam(r.Context(), repository.CreateTeamParams{
			OrgID:         orgID,
			Name:          req.Name,
			Description:   desc,
			CreatedByUser: claims.UserID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create team")
			return
		}

		if _, err := qtx.AddTeamMember(r.Context(), repository.AddTeamMemberParams{
			TeamID: team.ID,
			UserID: leaderID,
			Role:   repository.TeamRoleTEAMLEADER,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to assign team leader")
			return
		}

		if _, err := qtx.CreateChannel(r.Context(), repository.CreateChannelParams{
			TeamID: team.ID,
			Name:   "general",
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create default channel")
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to commit team creation")
			return
		}

		var myRole *string
		if leaderID == claims.UserID {
			role := string(repository.TeamRoleTEAMLEADER)
			myRole = &role
		}

		writeJSON(w, http.StatusCreated, teamResponse{
			ID:          team.PublicID.String(),
			PublicID:    team.PublicID.String(),
			OrgID:       team.OrgID,
			Name:        team.Name,
			Description: textPtr(team.Description),
			MyRole:      myRole,
			CreatedAt:   team.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		})
	}
}

func (router *Router) getTeam() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireClaims(w, r)
		if !ok {
			return
		}

		team, access, ok := router.resolveTeamAccess(w, r, claims, true)
		if !ok {
			return
		}

		var myRole *string
		if access.membership != nil {
			role := string(access.membership.Role)
			myRole = &role
		}

		writeJSON(w, http.StatusOK, teamResponse{
			ID:          team.PublicID.String(),
			PublicID:    team.PublicID.String(),
			OrgID:       team.OrgID,
			Name:        team.Name,
			Description: textPtr(team.Description),
			MyRole:      myRole,
			CreatedAt:   team.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		})
	}
}

func (router *Router) listTeamMembers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireClaims(w, r)
		if !ok {
			return
		}
		team, _, ok := router.resolveTeamAccess(w, r, claims, true)
		if !ok {
			return
		}

		rows, err := router.queries.ListTeamMembers(r.Context(), team.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list members")
			return
		}

		out := make([]teamMemberResponse, 0, len(rows))
		for _, row := range rows {
			out = append(out, teamMemberResponse{
				UserID:    row.UserID,
				Name:      row.UserName,
				Email:     row.UserEmail,
				AvatarURL: textPtr(row.UserAvatarUrl),
				Role:      string(row.Role),
				JoinedAt:  row.JoinedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (router *Router) assignTeamLeader() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireClaims(w, r)
		if !ok {
			return
		}
		if claims.OrgRole != "ORG_ADMIN" {
			writeError(w, http.StatusForbidden, "forbidden: only organization admins can assign leaders")
			return
		}

		team, _, ok := router.resolveTeamAccess(w, r, claims, true)
		if !ok {
			return
		}

		var req userIDRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID < 1 {
			writeError(w, http.StatusBadRequest, "user_id is required")
			return
		}

		user, err := router.queries.GetUserByID(r.Context(), req.UserID)
		if err != nil || user.OrgID != team.OrgID || !user.Active {
			writeError(w, http.StatusBadRequest, "user must be an active member of this organization")
			return
		}
		if string(user.OrgRole) == "ORG_ADMIN" {
			writeError(w, http.StatusBadRequest, "organization admins cannot be team leaders for chat teams")
			return
		}

		existing, err := router.queries.GetTeamMembership(r.Context(), repository.GetTeamMembershipParams{
			TeamID: team.ID,
			UserID: req.UserID,
		})
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusInternalServerError, "failed to check membership")
				return
			}
			member, addErr := router.queries.AddTeamMember(r.Context(), repository.AddTeamMemberParams{
				TeamID: team.ID,
				UserID: req.UserID,
				Role:   repository.TeamRoleTEAMLEADER,
			})
			if addErr != nil {
				writeError(w, http.StatusInternalServerError, "failed to assign team leader")
				return
			}
			writeJSON(w, http.StatusOK, teamMemberResponse{
				UserID:    member.UserID,
				Name:      user.Name,
				Email:     user.Email,
				AvatarURL: textPtr(user.AvatarUrl),
				Role:      string(member.Role),
				JoinedAt:  member.JoinedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
			})
			return
		}

		if existing.Role == repository.TeamRoleTEAMLEADER {
			writeJSON(w, http.StatusOK, teamMemberResponse{
				UserID:    existing.UserID,
				Name:      user.Name,
				Email:     user.Email,
				AvatarURL: textPtr(user.AvatarUrl),
				Role:      string(existing.Role),
				JoinedAt:  existing.JoinedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
			})
			return
		}

		updated, err := router.queries.UpdateTeamMemberRole(r.Context(), repository.UpdateTeamMemberRoleParams{
			TeamID: team.ID,
			UserID: req.UserID,
			Role:   repository.TeamRoleTEAMLEADER,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to assign team leader")
			return
		}

		writeJSON(w, http.StatusOK, teamMemberResponse{
			UserID:    updated.UserID,
			Name:      user.Name,
			Email:     user.Email,
			AvatarURL: textPtr(user.AvatarUrl),
			Role:      string(updated.Role),
			JoinedAt:  updated.JoinedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		})
	}
}

func (router *Router) addTeamMember() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireClaims(w, r)
		if !ok {
			return
		}

		team, access, ok := router.resolveTeamAccess(w, r, claims, true)
		if !ok {
			return
		}
		isLeader := access.membership != nil && access.membership.Role == repository.TeamRoleTEAMLEADER
		if !access.isOrgAdmin && !isLeader {
			writeError(w, http.StatusForbidden, "forbidden: only team leaders or org admins can add members")
			return
		}

		var req userIDRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID < 1 {
			writeError(w, http.StatusBadRequest, "user_id is required")
			return
		}

		user, err := router.queries.GetUserByID(r.Context(), req.UserID)
		if err != nil || user.OrgID != team.OrgID || !user.Active {
			writeError(w, http.StatusBadRequest, "user must be an active member of this organization")
			return
		}
		if string(user.OrgRole) == "ORG_ADMIN" {
			writeError(w, http.StatusBadRequest, "organization admins cannot be added to team chat membership")
			return
		}

		_, err = router.queries.GetTeamMembership(r.Context(), repository.GetTeamMembershipParams{
			TeamID: team.ID,
			UserID: req.UserID,
		})
		if err == nil {
			writeError(w, http.StatusConflict, "user is already a team member")
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "failed to check membership")
			return
		}

		member, err := router.queries.AddTeamMember(r.Context(), repository.AddTeamMemberParams{
			TeamID: team.ID,
			UserID: req.UserID,
			Role:   repository.TeamRoleCONTRIBUTOR,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to add team member")
			return
		}

		writeJSON(w, http.StatusCreated, teamMemberResponse{
			UserID:    member.UserID,
			Name:      user.Name,
			Email:     user.Email,
			AvatarURL: textPtr(user.AvatarUrl),
			Role:      string(member.Role),
			JoinedAt:  member.JoinedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		})
	}
}

func (router *Router) removeTeamMember() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireClaims(w, r)
		if !ok {
			return
		}

		team, access, ok := router.resolveTeamAccess(w, r, claims, true)
		if !ok {
			return
		}
		isLeader := access.membership != nil && access.membership.Role == repository.TeamRoleTEAMLEADER
		if !access.isOrgAdmin && !isLeader {
			writeError(w, http.StatusForbidden, "forbidden: only team leaders or org admins can remove members")
			return
		}

		targetID, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
		if err != nil || targetID < 1 {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}

		target, err := router.queries.GetTeamMembership(r.Context(), repository.GetTeamMembershipParams{
			TeamID: team.ID,
			UserID: targetID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "member not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to load member")
			return
		}

		if target.Role == repository.TeamRoleTEAMLEADER {
			count, cErr := router.queries.CountTeamLeaders(r.Context(), team.ID)
			if cErr != nil {
				writeError(w, http.StatusInternalServerError, "failed to check leaders")
				return
			}
			if count <= 1 {
				writeError(w, http.StatusBadRequest, "cannot remove the only team leader")
				return
			}
		}

		if err := router.queries.RemoveTeamMember(r.Context(), repository.RemoveTeamMemberParams{
			TeamID: team.ID,
			UserID: targetID,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to remove member")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (router *Router) leaveTeam() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := requireClaims(w, r)
		if !ok {
			return
		}

		team, access, ok := router.resolveTeamAccess(w, r, claims, false)
		if !ok {
			return
		}
		if access.membership == nil {
			writeError(w, http.StatusForbidden, "forbidden: not a team member")
			return
		}
		if access.membership.Role == repository.TeamRoleTEAMLEADER {
			writeError(w, http.StatusBadRequest, "team leaders cannot leave the team")
			return
		}

		if err := router.queries.RemoveTeamMember(r.Context(), repository.RemoveTeamMemberParams{
			TeamID: team.ID,
			UserID: claims.UserID,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to leave team")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

type teamAccess struct {
	membership *auth.TeamMembership
	isOrgAdmin bool
}

// resolveTeamAccess loads the team and ensures the caller is a member, or an org admin when allowAdmin is true.
func (router *Router) resolveTeamAccess(
	w http.ResponseWriter,
	r *http.Request,
	claims *auth.CustomClaims,
	allowAdmin bool,
) (repository.Team, teamAccess, bool) {
	teamPublicID, err := uuid.Parse(chi.URLParam(r, "teamPublicId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid team id")
		return repository.Team{}, teamAccess{}, false
	}

	team, err := router.queries.GetTeamByPublicID(r.Context(), teamPublicID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "team not found")
			return repository.Team{}, teamAccess{}, false
		}
		writeError(w, http.StatusInternalServerError, "failed to load team")
		return repository.Team{}, teamAccess{}, false
	}

	if team.OrgID != claims.OrgID {
		writeError(w, http.StatusForbidden, "forbidden: wrong organization")
		return repository.Team{}, teamAccess{}, false
	}

	isOrgAdmin := claims.OrgRole == "ORG_ADMIN"
	membership, err := loadTeamMembership(r, router.queries, teamPublicID, claims.UserID)
	if err != nil {
		if errors.Is(err, errNotTeamMember) {
			if allowAdmin && isOrgAdmin {
				return team, teamAccess{isOrgAdmin: true}, true
			}
			writeError(w, http.StatusForbidden, "forbidden: not a team member")
			return repository.Team{}, teamAccess{}, false
		}
		if errors.Is(err, errTeamNotFound) {
			writeError(w, http.StatusNotFound, "team not found")
			return repository.Team{}, teamAccess{}, false
		}
		writeError(w, http.StatusInternalServerError, "failed to load membership")
		return repository.Team{}, teamAccess{}, false
	}

	return team, teamAccess{membership: membership, isOrgAdmin: isOrgAdmin}, true
}
