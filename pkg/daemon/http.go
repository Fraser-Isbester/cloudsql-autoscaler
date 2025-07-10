package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// HTTPServer provides health checks and metrics endpoints
type HTTPServer struct {
	port   int
	daemon *Daemon
	server *http.Server
}

// NewHTTPServer creates a new HTTP server
func NewHTTPServer(port int, daemon *Daemon) *HTTPServer {
	return &HTTPServer{
		port:   port,
		daemon: daemon,
	}
}

// Start starts the HTTP server
func (s *HTTPServer) Start() error {
	mux := http.NewServeMux()

	// Health check endpoints
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/healthz", s.healthHandler)
	mux.HandleFunc("/ready", s.readinessHandler)
	mux.HandleFunc("/readyz", s.readinessHandler)

	// Status endpoint
	mux.HandleFunc("/status", s.statusHandler)

	// Metrics endpoint (if Prometheus is enabled)
	if metricsEnabled {
		mux.Handle("/metrics", GetMetricsHandler())
	}

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// healthHandler responds to health check requests
func (s *HTTPServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"service":   "cloudsql-autoscaler",
	}

	json.NewEncoder(w).Encode(response)
}

// readinessHandler responds to readiness probe requests
func (s *HTTPServer) readinessHandler(w http.ResponseWriter, r *http.Request) {
	// Check if daemon is ready to process requests
	// This could include checking if GCP clients are initialized, etc.

	w.Header().Set("Content-Type", "application/json")

	if s.daemon == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		response := map[string]interface{}{
			"status": "not ready",
			"reason": "daemon not initialized",
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	w.WriteHeader(http.StatusOK)
	response := map[string]interface{}{
		"status":    "ready",
		"timestamp": time.Now().UTC(),
	}

	json.NewEncoder(w).Encode(response)
}

// statusHandler provides detailed daemon status
func (s *HTTPServer) statusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.daemon == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		response := map[string]interface{}{
			"error": "daemon not available",
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	status := s.daemon.GetStatus()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}
