package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/CORTA-11/core-api/internal/repository"
	"github.com/jackc/pgx/v5"
)

func main() {

	ctx := context.Background()

	// TODO: Use pgxpool
	conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("unable to connect to database: %v\n", err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			log.Printf("error closing database connection: %v", err)
		}
	}()

	queries := repository.New(conn)

	r := http.NewServeMux()

	// TODO: Extract the handlers to a separate file
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("Hello World")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	r.HandleFunc("/orgs", func(w http.ResponseWriter, r *http.Request) {
		orgs, err := queries.GetOrgs(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		err = json.NewEncoder(w).Encode(orgs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	s := http.Server{
		WriteTimeout: time.Second * 5,
		ReadTimeout:  time.Second * 5,
		Addr:         ":8080",
		Handler:      r,
	}

	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}
