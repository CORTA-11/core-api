package handlers

import (
	"encoding/json"
	"net/http"
)

func (router *Router) getTeams() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		teams, err := router.queries.GetTeams(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err = json.NewEncoder(w).Encode(teams); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	})
}
