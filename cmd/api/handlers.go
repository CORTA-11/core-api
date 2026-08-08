package main

import (
	"encoding/json"
	"net/http"

	appMiddleware "github.com/CORTA-11/core-api/internal/middleware"
	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Router struct {
	mux     *chi.Mux
	queries *repository.Queries
}

func NewRouter(queries *repository.Queries) *Router {
	return &Router{
		mux:     chi.NewRouter(),
		queries: queries,
	}
}

func (router *Router) SetupRoutes() {
	router.mux.Use(middleware.Logger)
	router.mux.Use(corsMiddleware)
	router.mux.Get("/", router.handleRoot())

	// Public routes
	router.mux.Route("/auth", func(r chi.Router) {
		r.Post("/register", router.registerUser())
		r.Post("/login", router.loginUser())
		r.Post("/refresh", router.refreshSession())
		r.Post("/logout", router.logoutUser())
	})

	// Protected routes (Any logged-in user)
	router.mux.Group(func(r chi.Router) {
		r.Use(appMiddleware.RequireAuth) // Validates JWT

		r.Get("/orgs", router.getOrgs())
		r.Get("/me", router.getMe())
	})

	// Admin-only routes
	router.mux.Group(func(r chi.Router) {
		r.Use(appMiddleware.RequireAuth)               // 1. Must be logged in
		r.Use(appMiddleware.RequireRoles("ORG_ADMIN")) // 2. Must be an admin

		r.Get("/admin/settings", router.getAdminSettings()) // New endpoint
	})
}

// getAdminSettings is a dummy handler to test the RBAC
func (router *Router) getAdminSettings() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If the code reaches here, we guarantee the user is an ORG_ADMIN
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"message": "Welcome to the admin panel!"}`)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (router *Router) Handler() http.Handler {
	return router.mux
}

// corsMiddleware allows the local Next.js UI to call the API directly during development.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://localhost:3000" || origin == "http://127.0.0.1:3000" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (router *Router) handleRoot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("Hello World")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (router *Router) getOrgs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgs, err := router.queries.GetOrgs(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(orgs); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
