package httpx

import (
	"log"
	"log/slog"
	"net/http"
	"time"
)

const MaximumHeaderBytes = 32 << 10

type ServerTimeouts struct {
	ReadHeader time.Duration
	Read       time.Duration
	Write      time.Duration
	Idle       time.Duration
}

// NewServer creates a server.
func NewServer(address string, handler http.Handler, timeouts ServerTimeouts, logger *slog.Logger) *http.Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: timeouts.ReadHeader,
		ReadTimeout:       timeouts.Read,
		WriteTimeout:      timeouts.Write,
		IdleTimeout:       timeouts.Idle,
		MaxHeaderBytes:    MaximumHeaderBytes,
		ErrorLog:          log.New(redactedServerErrorWriter{logger: logger}, "", 0),
	}
}

type redactedServerErrorWriter struct{ logger *slog.Logger }

// Write writes the supplied data.
func (writer redactedServerErrorWriter) Write(message []byte) (int, error) {
	writer.logger.Error("HTTP server rejected a connection")
	return len(message), nil
}
