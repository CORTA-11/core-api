package main

import (
	"log"
	"net/http"
	"time"
)

func main() {

	r := http.NewServeMux()

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("Hello World")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	s := http.Server{
		WriteTimeout: time.Second * 5,
		ReadTimeout:  time.Second * 5,
		Addr:         ":8080",
		Handler:      r,
	}

	log.Fatal(s.ListenAndServe())
}
