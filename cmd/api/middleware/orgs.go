package middleware

import (
	"context"
	"net/http"
	"strconv"
)

func SetOrgIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgIDStr := r.Header.Get("X-Org-ID")
		if orgIDStr == "" {
			http.Error(w, "organization id header is missing", http.StatusBadRequest)
			return
		}

		orgID, err := strconv.ParseInt(orgIDStr, 10, 64)
		if err != nil {
			http.Error(w, "organization id header is invalid", http.StatusBadRequest)
			return
		}

		r = r.WithContext(context.WithValue(r.Context(), "orgID", orgID))
		next.ServeHTTP(w, r)
	})
}
