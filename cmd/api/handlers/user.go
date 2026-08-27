package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/CORTA-11/core-api/internal/service"
)

// getUsers handles fetching all users
func (router *Router) getUsers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := router.userService.GetUsers(r.Context())
		if err != nil {
			slog.Error("failed to get users", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(users); err != nil {
			slog.Error("failed to encode users response", "error", err)
		}
	}
}

// createUserRequest defines the expected JSON payload for creating a user
type createUserRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// createUser handles creating a new user
func (router *Router) createUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request payload", http.StatusBadRequest)
			return
		}

		user, err := router.userService.CreateUser(r.Context(), req.Name, req.Email, req.Password)
		if err != nil {
			if errors.Is(err, service.ErrEmailAlreadyInUse) {
				http.Error(w, "Email already in use", http.StatusConflict)
				return
			}
			slog.Error("failed to create user", "error", err)
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(user); err != nil {
			slog.Error("failed to encode user response", "error", err)
		}
	}
}

// updateUserRequest defines the expected JSON payload for updating a user
type updateUserRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// updateUser handles updating an existing user
func (router *Router) updateUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Assuming you are using Chi router URL parameters like /users/{id}
		publicID := r.PathValue("id") // or chi.URLParam(r, "id") depending on your Chi version

		var req updateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request payload", http.StatusBadRequest)
			return
		}

		user, err := router.userService.UpdateUser(r.Context(), publicID, req.Name, req.Email, req.Password)
		if err != nil {
			slog.Error("failed to update user", "error", err, "public_id", publicID)
			http.Error(w, "Failed to update user", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(user); err != nil {
			slog.Error("failed to encode user response", "error", err)
		}
	}
}

// deleteUser handles soft-deleting a user
func (router *Router) deleteUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicID := r.PathValue("id") // or chi.URLParam(r, "id")

		user, err := router.userService.SoftDeleteUser(r.Context(), publicID)
		if err != nil {
			slog.Error("failed to delete user", "error", err, "public_id", publicID)
			http.Error(w, "Failed to delete user", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(user); err != nil {
			slog.Error("failed to encode user response", "error", err)
		}
	}
}

// loginUserRequest defines the expected JSON payload for user authentication
type loginUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginUserResponse defines the JSON response after successful login
type loginUserResponse struct {
	Token string        `json:"token"`
	User  *service.User `json:"user"`
}

// loginUser handles authenticating a user and returning a JWT token
func (router *Router) loginUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request payload", http.StatusBadRequest)
			return
		}

		token, user, err := router.userService.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			if errors.Is(err, service.ErrInvalidCredentials) {
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
				return
			}
			slog.Error("failed to login user", "error", err)
			http.Error(w, "Failed to login user", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := loginUserResponse{
			Token: token,
			User:  user,
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("failed to encode login response", "error", err)
		}
	}
}
