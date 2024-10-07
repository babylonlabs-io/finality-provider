package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Server represents the metrics server.
type Server struct {
	httpServer *http.Server
	logger     *zap.Logger
}

func Start(addr string, logger *zap.Logger) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	// Create the HTTP server with the custom ServeMux as the handler
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	// Store the logger in the server struct
	s := &Server{
		httpServer: server,
		logger:     logger,
	}

	// Start the metrics server in a goroutine.
	go func() {
		s.logger.Info("Metrics server is starting", zap.String("addr", addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Fatal("Metrics server failed to start", zap.Error(err))
		}
	}()

	return s
}

// Stop gracefully shuts down the metrics server.
func (s *Server) Stop(ctx context.Context) {
	s.logger.Info("Stopping metrics server")
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("Metrics server shutdown failed", zap.Error(err))
	} else {
		s.logger.Info("Metrics server stopped gracefully")
	}
}
