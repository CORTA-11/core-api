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
	router.mux.Use(appMiddleware.CorsMiddleware)
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

		r.Get("/me", router.getMe())

		r.Route("/orgs", func(r chi.Router) {
			r.Get("/", router.getOrgs())
			r.Post("/", router.createOrg())
			r.Post("/restore", router.restoreOrg())

			r.Route("/{orgId}", func(r chi.Router) {
				// TODO: Add org-level middleware to check if the user is part of the org
				r.Put("/", router.updateOrg())
				r.Delete("/", router.deleteOrg())
				r.Get("/users", router.listOrgUsers())

				r.Route("/teams", func(r chi.Router) {
					// TODO: Verify team belongs to org
					// TODO: Add team-level middleware to check if the user is part of the team
					r.Get("/", router.listTeams())
					r.Post("/", router.createTeam())

					r.Route("/{teamPublicId}", func(r chi.Router) {
						r.Get("/", router.getTeam())

						r.Get("/members", router.listTeamMembers())
						r.Post("/members", router.addTeamMember())
						r.Delete("/members/{userId}", router.removeTeamMember())

						r.Put("/leader", router.assignTeamLeader())
						r.Post("/leave", router.leaveTeam())

						r.Route("/chat", func(r chi.Router) {
							r.Get("/messages", router.listChatMessages())
							r.Post("/messages", router.createChatMessage())
							r.Delete("/messages/{messageId}", router.deleteChatMessage())
						})
					})

				})
			})

		})

		// Admin-only routes
		router.mux.Group(func(r chi.Router) {
			r.Use(appMiddleware.RequireRoles("ORG_ADMIN"))
		})
	})
}

func (router *Router) Handler() http.Handler {
	return router.mux
}

func (router *Router) handleRoot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var response struct {
			Message string `json:"message"`
		}
		response.Message = "Hello World"

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
