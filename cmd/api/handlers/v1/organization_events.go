package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/CORTA-11/core-api/internal/service"
)

func (handler *ResourceHandler) organizationEvents(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	parameters, err := pagination.Parse(request.URL.Query())
	if !ok || err != nil || handler.organizations == nil {
		handler.problem(writer, request, firstError(err, service.ErrInvalidInput))
		return
	}
	// Bound each connection so reconnects revalidate the session, and the stream
	// stays below the server write timeout. Snapshots make reconnects lossless.
	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	controller := http.NewResponseController(writer)
	var previous []byte
	started := false
	for {
		page, listErr := handler.organizations.List(ctx, authentication.Principal, parameters)
		if listErr != nil {
			if !started {
				handler.problem(writer, request, listErr)
			}
			return
		}
		// Cursors contain expiry times; only compare the visible rows.
		current, marshalErr := json.Marshal(page.Items)
		if marshalErr != nil {
			return
		}
		if !started {
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.Header().Set("Cache-Control", "no-cache, no-transform")
			writer.Header().Set("X-Accel-Buffering", "no")
			started = true
		}
		if !bytes.Equal(previous, current) {
			if _, err := fmt.Fprintf(writer, "event: organizations\ndata: %s\n\n", current); err != nil {
				return
			}
			previous = current
		} else {
			if _, err := fmt.Fprint(writer, ": heartbeat\n\n"); err != nil {
				return
			}
		}
		if err := controller.Flush(); err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
