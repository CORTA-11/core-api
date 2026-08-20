package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"
)

type ReadinessCheck func(context.Context) error

func healthLive() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
}

func healthReady(checks map[string]ReadinessCheck, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), timeout)
		defer cancel()
		failed := make([]string, 0)
		var mutex sync.Mutex
		var group sync.WaitGroup
		for name, check := range checks {
			group.Add(1)
			go func() {
				defer group.Done()
				if err := check(ctx); err != nil {
					mutex.Lock()
					failed = append(failed, name)
					mutex.Unlock()
				}
			}()
		}
		group.Wait()
		if len(failed) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		sort.Strings(failed)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(struct {
			Failed []string `json:"failed"`
		}{Failed: failed})
	}
}
