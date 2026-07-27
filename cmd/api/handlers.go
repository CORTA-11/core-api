package main

import (
	"encoding/json"
	"net/http"

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
	router.mux.Get("/", router.handleRoot())
	router.mux.Get("/orgs", router.getOrgs())
}

func (router *Router) Handler() http.Handler {
	return router.mux
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
