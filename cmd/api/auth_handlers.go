package main

import (
	"encoding/json"
	"net/http"

	"github.com/CORTA-11/core-api/internal/auth"
	"github.com/CORTA-11/core-api/internal/repository"
)

type RegisterRequest struct {
	OrgID    int64  `json:"org_id"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (router *Router) registerUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request payload", http.StatusBadRequest)
			return
		}

		// 1. Hash the password using Argon2id
		hashedPassword, err := auth.CreateHash(req.Password)
		if err != nil {
			http.Error(w, "failed to process password", http.StatusInternalServerError)
			return
		}

		// 2. Save the user to the database
		arg := repository.CreateUserParams{
			OrgID:        req.OrgID,
			Email:        req.Email,
			PasswordHash: hashedPassword,
			Name:         req.Name,
			OrgRole:      "ORG_USER", // Default role from your DB Enum
		}

		user, err := router.queries.CreateUser(r.Context(), arg)
		if err != nil {
			// In production, check if this is a duplicate email error
			http.Error(w, "failed to create user", http.StatusInternalServerError)
			return
		}

		// 3. Return the created user (without password hash)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(user)
	}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

func (router *Router) loginUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request payload", http.StatusBadRequest)
			return
		}

		// 1. Fetch the user from the database by email
		user, err := router.queries.GetUserByEmail(r.Context(), req.Email)
		if err != nil {
			// Return a generic error to prevent email enumeration attacks
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		// 2. Compare the provided password with the stored Argon2id hash
		match, err := auth.ComparePasswordAndHash(req.Password, user.PasswordHash)
		if err != nil || !match {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		// Optional: Check if the user is active
		if !user.Active {
			http.Error(w, "user account is deactivated", http.StatusForbidden)
			return
		}

		// 3. Generate the JWT Token
		tokenString, err := auth.GenerateToken(user.ID, user.OrgID, string(user.OrgRole))
		if err != nil {
			http.Error(w, "failed to generate token", http.StatusInternalServerError)
			return
		}

		// 4. Return the token to the client
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(LoginResponse{
			Token: tokenString,
		})
	}
}

// getMe returns the authenticated user's claims from the context
func (router *Router) getMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract the user data that our RequireAuth middleware put into the context
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok {
			http.Error(w, "could not retrieve user from context", http.StatusInternalServerError)
			return
		}

		// Return the user claims as JSON
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claims)
	}
}
