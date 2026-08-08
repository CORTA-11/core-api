package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/CORTA-11/core-api/internal/auth"
	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type AuthUser struct {
	ID        int64  `json:"id"`
	OrgID     int64  `json:"org_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	OrgRole   string `json:"org_role"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// AuthResponse returns only the access token + user.
// Refresh token is delivered via httpOnly cookie only.
type AuthResponse struct {
	AccessToken string   `json:"access_token"`
	User        AuthUser `json:"user"`
}

type RegisterRequest struct {
	Mode        string `json:"mode"`
	OrgName     string `json:"org_name"`
	OrgPublicID string `json:"org_public_id"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Name        string `json:"name"`
}

func (router *Router) registerUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request payload", http.StatusBadRequest)
			return
		}

		req.Mode = strings.TrimSpace(req.Mode)
		req.Email = strings.TrimSpace(req.Email)
		req.Name = strings.TrimSpace(req.Name)
		req.OrgName = strings.TrimSpace(req.OrgName)
		req.OrgPublicID = strings.TrimSpace(req.OrgPublicID)

		if req.Email == "" || req.Password == "" || req.Name == "" {
			http.Error(w, "name, email, and password are required", http.StatusBadRequest)
			return
		}

		var orgID int64
		var orgRole repository.OrgRole

		switch req.Mode {
		case "create_org":
			if req.OrgName == "" {
				http.Error(w, "org_name is required when creating an organization", http.StatusBadRequest)
				return
			}

			org, err := router.queries.CreateOrg(r.Context(), req.OrgName)
			if err != nil {
				http.Error(w, "failed to create organization", http.StatusInternalServerError)
				return
			}

			orgID = org.ID
			orgRole = repository.OrgRoleORGADMIN

		case "join_org":
			if req.OrgPublicID == "" {
				http.Error(w, "org_public_id is required when joining an organization", http.StatusBadRequest)
				return
			}

			orgPublicID, err := uuid.Parse(req.OrgPublicID)
			if err != nil {
				http.Error(w, "invalid org_public_id", http.StatusBadRequest)
				return
			}

			orgID, err = router.queries.GetOrgID(r.Context(), orgPublicID)
			if err != nil {
				http.Error(w, "organization not found", http.StatusNotFound)
				return
			}

			orgRole = repository.OrgRoleORGUSER

		default:
			http.Error(w, `mode must be "create_org" or "join_org"`, http.StatusBadRequest)
			return
		}

		hashedPassword, err := auth.CreateHash(req.Password)
		if err != nil {
			http.Error(w, "failed to process password", http.StatusInternalServerError)
			return
		}

		user, err := router.queries.CreateUser(r.Context(), repository.CreateUserParams{
			OrgID:        orgID,
			Email:        req.Email,
			PasswordHash: hashedPassword,
			Name:         req.Name,
			AvatarUrl:    pgtype.Text{},
			OrgRole:      orgRole,
		})
		if err != nil {
			http.Error(w, "failed to create user", http.StatusInternalServerError)
			return
		}

		authUser := AuthUser{
			ID:      user.ID,
			OrgID:   user.OrgID,
			Email:   user.Email,
			Name:    user.Name,
			OrgRole: string(user.OrgRole),
		}

		response, err := router.issueAuthResponse(w, r, authUser)
		if err != nil {
			http.Error(w, "failed to create session", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		// #nosec G117 -- access_token is intentionally returned to the authenticated client
		_ = json.NewEncoder(w).Encode(response)
	}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (router *Router) loginUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request payload", http.StatusBadRequest)
			return
		}

		req.Email = strings.TrimSpace(req.Email)
		if req.Email == "" || req.Password == "" {
			http.Error(w, "email and password are required", http.StatusBadRequest)
			return
		}

		user, err := router.queries.GetUserByEmail(r.Context(), req.Email)
		if err != nil {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		match, err := auth.ComparePasswordAndHash(req.Password, user.PasswordHash)
		if err != nil || !match {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		if !user.Active {
			http.Error(w, "user account is deactivated", http.StatusForbidden)
			return
		}

		authUser := AuthUser{
			ID:      user.ID,
			OrgID:   user.OrgID,
			Email:   user.Email,
			Name:    user.Name,
			OrgRole: string(user.OrgRole),
		}

		response, err := router.issueAuthResponse(w, r, authUser)
		if err != nil {
			http.Error(w, "failed to create session", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// #nosec G117 -- access_token is intentionally returned to the authenticated client
		_ = json.NewEncoder(w).Encode(response)
	}
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (router *Router) refreshSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RefreshRequest
		// Body is optional; refresh token should come from httpOnly cookie.
		_ = json.NewDecoder(r.Body).Decode(&req)

		refreshToken := auth.RefreshTokenFromRequest(r, req.RefreshToken)
		if refreshToken == "" {
			http.Error(w, "refresh token missing", http.StatusUnauthorized)
			return
		}

		session, err := router.queries.GetSessionByRefreshToken(r.Context(), refreshToken)
		if err != nil {
			auth.ClearRefreshCookie(w)
			http.Error(w, "invalid refresh token", http.StatusUnauthorized)
			return
		}

		if session.IsBlocked || time.Now().After(session.ExpiresAt) {
			_ = router.queries.BlockSession(r.Context(), refreshToken)
			auth.ClearRefreshCookie(w)
			http.Error(w, "refresh token expired or revoked", http.StatusUnauthorized)
			return
		}

		user, err := router.queries.GetUserByID(r.Context(), session.UserID)
		if err != nil {
			auth.ClearRefreshCookie(w)
			http.Error(w, "user not found", http.StatusUnauthorized)
			return
		}

		if !user.Active {
			_ = router.queries.BlockSession(r.Context(), refreshToken)
			auth.ClearRefreshCookie(w)
			http.Error(w, "user account is deactivated", http.StatusForbidden)
			return
		}

		if err := router.queries.BlockSession(r.Context(), refreshToken); err != nil {
			http.Error(w, "failed to rotate session", http.StatusInternalServerError)
			return
		}

		authUser := AuthUser{
			ID:        user.ID,
			OrgID:     user.OrgID,
			Email:     user.Email,
			Name:      user.Name,
			OrgRole:   string(user.OrgRole),
			AvatarURL: textOrEmpty(user.AvatarUrl),
		}

		response, err := router.issueAuthResponse(w, r, authUser)
		if err != nil {
			http.Error(w, "failed to create session", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// #nosec G117 -- access_token is intentionally returned to the authenticated client
		_ = json.NewEncoder(w).Encode(response)
	}
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (router *Router) logoutUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LogoutRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		refreshToken := auth.RefreshTokenFromRequest(r, req.RefreshToken)
		if refreshToken != "" {
			_ = router.queries.BlockSession(r.Context(), refreshToken)
		}

		auth.ClearRefreshCookie(w)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (router *Router) getMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok {
			http.Error(w, "could not retrieve user from context", http.StatusInternalServerError)
			return
		}

		user, err := router.queries.GetUserByID(r.Context(), claims.UserID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		if !user.Active {
			http.Error(w, "user account is deactivated", http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AuthUser{
			ID:        user.ID,
			OrgID:     user.OrgID,
			Email:     user.Email,
			Name:      user.Name,
			OrgRole:   string(user.OrgRole),
			AvatarURL: textOrEmpty(user.AvatarUrl),
		})
	}
}

func (router *Router) issueAuthResponse(w http.ResponseWriter, r *http.Request, user AuthUser) (AuthResponse, error) {
	accessToken, err := auth.GenerateToken(user.ID, user.OrgID, user.OrgRole)
	if err != nil {
		return AuthResponse{}, err
	}

	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return AuthResponse{}, err
	}

	ttl := auth.RefreshTokenTTL()
	_, err = router.queries.CreateSession(r.Context(), repository.CreateSessionParams{
		UserID:       user.ID,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(ttl),
	})
	if err != nil {
		return AuthResponse{}, err
	}

	auth.SetRefreshCookie(w, refreshToken, ttl)

	return AuthResponse{
		AccessToken: accessToken,
		User:        user,
	}, nil
}

func textOrEmpty(value pgtype.Text) string {
	if value.Valid {
		return value.String
	}
	return ""
}
