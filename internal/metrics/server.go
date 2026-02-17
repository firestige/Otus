// Package metrics implements metrics server.
package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server is the HTTP server for Prometheus metrics.
type Server struct {
	addr   string
	path   string
	server *http.Server
}

// NewServer creates a new metrics server.
func NewServer(addr, path string) *Server {
	if path == "" {
		path = "/metrics"
	}
	return &Server{
		addr: addr,
		path: path,
	}
}

// Start starts the metrics HTTP server.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle(s.path, promhttp.Handler())

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("starting metrics server", "addr", s.addr, "path", s.path)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the metrics server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	slog.Info("stopping metrics server")

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("metrics server shutdown failed: %w", err)
	}

	slog.Info("metrics server stopped")
	return nil
}
