package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/CORTA-11/core-api/internal/auth"
	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	http.Error(w, message, status)
}

func parseOrgIDParam(raw string) (int64, error) {
	return strconv.ParseInt(raw, 10, 64)
}

func requireClaims(w http.ResponseWriter, r *http.Request) (*auth.CustomClaims, bool) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	return claims, true
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	s := value.String
	return &s
}

func uuidPtr(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	id, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		return nil
	}
	s := id.String()
	return &s
}

func timestamptzPtr(value pgtype.Timestamptz) *string {
	if !value.Valid {
		return nil
	}
	s := value.Time.UTC().Format("2006-01-02T15:04:05.000Z07:00")
	return &s
}

func loadTeamMembership(
	r *http.Request,
	queries *repository.Queries,
	teamPublicID uuid.UUID,
	userID int64,
) (*auth.TeamMembership, error) {
	team, err := queries.GetTeamByPublicID(r.Context(), teamPublicID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errTeamNotFound
		}
		return nil, err
	}

	member, err := queries.GetTeamMembership(r.Context(), repository.GetTeamMembershipParams{
		TeamID: team.ID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errNotTeamMember
		}
		return nil, err
	}

	return &auth.TeamMembership{
		TeamID:   team.ID,
		PublicID: team.PublicID.String(),
		OrgID:    team.OrgID,
		Role:     member.Role,
	}, nil
}

var (
	errTeamNotFound  = errors.New("team not found")
	errNotTeamMember = errors.New("not a team member")
)
