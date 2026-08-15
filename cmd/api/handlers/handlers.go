package handlers

import (
	"encoding/json"
	"net/http"

	middleware2 "github.com/CORTA-11/core-api/cmd/api/middleware"
	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Router struct {
	mux        *chi.Mux
	db         *pgxpool.Pool
	queries    *repository.Queries
	orgService service.OrgService
}

func NewRouter(db *pgxpool.Pool, queries *repository.Queries, orgService service.OrgService) *Router {
	return &Router{
		mux:        chi.NewRouter(),
		db:         db,
		queries:    queries,
		orgService: orgService,
	}
}

func (router *Router) SetupRoutes() {
	r := router.mux
	r.Use(middleware.Logger)
	r.Use(middleware2.CorsMiddleware)

	r.Get("/", router.handleRoot())

	// Organization routes
	r.Mount("/orgs", orgRouter(router))

	// Team routes
	r.Mount("/teams", teamRouter(router))
}

func orgRouter(router *Router) chi.Router {
	r := chi.NewRouter()

	r.Get("/", router.getOrgs())
	r.Post("/", router.createOrg())
	r.Put("/{orgID}", router.updateOrg())
	r.Delete("/{orgID}", router.deleteOrg())
	r.Post("/restore", router.restoreOrg())

	return r
}

func teamRouter(router *Router) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware2.SetOrgIDMiddleware)

	r.Get("/", router.getTeams())

	return r
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
