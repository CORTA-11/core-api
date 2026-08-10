package main

import (
	"encoding/json"
	"net/http"

	appMiddleware "github.com/CORTA-11/core-api/internal/middleware"
	"github.com/CORTA-11/core-api/internal/realtime"
	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Router struct {
	mux       *chi.Mux
	db        *pgxpool.Pool
	queries   *repository.Queries
	publisher *realtime.Publisher
}

func NewRouter(db *pgxpool.Pool, queries *repository.Queries, publisher *realtime.Publisher) *Router {
	return &Router{
		mux:       chi.NewRouter(),
		db:        db,
		queries:   queries,
		publisher: publisher,
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

	// Internal routes for socket-server
	router.mux.Group(func(r chi.Router) {
		r.Use(router.requireInternalKey)
		r.Get("/internal/team-access", router.internalTeamAccess())
	})

	// Protected routes (Any logged-in user)
	router.mux.Group(func(r chi.Router) {
		r.Use(appMiddleware.RequireAuth)

		r.Get("/orgs", router.getOrgs())
		r.Get("/me", router.getMe())

		r.Get("/orgs/{orgId}/users", router.listOrgUsers())
		r.Get("/orgs/{orgId}/teams", router.listTeams())
		r.Post("/orgs/{orgId}/teams", router.createTeam())
		r.Get("/teams/{teamPublicId}", router.getTeam())
		r.Get("/teams/{teamPublicId}/members", router.listTeamMembers())
		r.Post("/teams/{teamPublicId}/members", router.addTeamMember())
		r.Delete("/teams/{teamPublicId}/members/{userId}", router.removeTeamMember())
		r.Put("/teams/{teamPublicId}/leader", router.assignTeamLeader())
		r.Post("/teams/{teamPublicId}/leave", router.leaveTeam())

		r.Get("/teams/{teamPublicId}/chat/messages", router.listChatMessages())
		r.Post("/teams/{teamPublicId}/chat/messages", router.createChatMessage())
		r.Delete("/teams/{teamPublicId}/chat/messages/{messageId}", router.deleteChatMessage())
	})

	// Admin-only routes
	router.mux.Group(func(r chi.Router) {
		r.Use(appMiddleware.RequireAuth)
		r.Use(appMiddleware.RequireRoles("ORG_ADMIN"))

		r.Get("/admin/settings", router.getAdminSettings())
	})
}

func (router *Router) getAdminSettings() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"message": "Welcome to the admin panel!"}`)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (router *Router) Handler() http.Handler {
	return router.mux
}

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
