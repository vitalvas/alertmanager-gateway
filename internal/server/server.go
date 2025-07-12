package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
)

// Server represents the HTTP server
type Server struct {
	config     *config.Config
	router     *mux.Router
	httpServer *http.Server
	logger     *logrus.Logger
}

// New creates a new server instance
func New(cfg *config.Config, logger *logrus.Logger) *Server {
	s := &Server{
		config: cfg,
		logger: logger,
		router: mux.NewRouter(),
	}

	// Setup routes
	s.setupRoutes()

	// Setup middleware
	s.router.Use(s.loggingMiddleware)
	s.router.Use(s.recoveryMiddleware)

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      s.router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	return s
}

// Run starts the server with graceful shutdown
func (s *Server) Run() error {
	// Channel to listen for interrupt signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Channel to capture server errors
	serverErr := make(chan error, 1)

	// Start server in a goroutine
	go func() {
		s.logger.WithFields(logrus.Fields{
			"addr": s.httpServer.Addr,
		}).Info("Starting HTTP server")

		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Wait for interrupt signal or server error
	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case sig := <-stop:
		s.logger.WithField("signal", sig).Info("Received shutdown signal")
	}

	// Graceful shutdown
	return s.Shutdown()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() error {
	s.logger.Info("Shutting down server...")

	// Create a deadline for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown the server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	s.logger.Info("Server stopped gracefully")
	return nil
}

// setupRoutes configures all routes
func (s *Server) setupRoutes() {
	// Health check endpoints
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")
	s.router.HandleFunc("/health/live", s.handleHealthLive).Methods("GET")
	s.router.HandleFunc("/health/ready", s.handleHealthReady).Methods("GET")

	// Metrics endpoint (placeholder for now)
	s.router.HandleFunc("/metrics", s.handleMetrics).Methods("GET")

	// API endpoints
	api := s.router.PathPrefix("/api/v1").Subrouter()
	if s.config.Server.Auth.Enabled && s.config.Server.Auth.APIUsername != "" {
		api.Use(s.apiAuthMiddleware)
	} else if s.config.Server.Auth.Enabled {
		api.Use(s.authMiddleware)
	}
	api.HandleFunc("/destinations", s.handleListDestinations).Methods("GET")
	api.HandleFunc("/destinations/{name}", s.handleGetDestination).Methods("GET")
	api.HandleFunc("/test/{destination}", s.handleTestDestination).Methods("POST")

	// Webhook endpoints
	webhook := s.router.PathPrefix("/webhook").Subrouter()
	if s.config.Server.Auth.Enabled {
		webhook.Use(s.authMiddleware)
	}
	webhook.HandleFunc("/{destination}", s.handleWebhook).Methods("POST")

	// Default handler for unmatched routes
	s.router.NotFoundHandler = http.HandlerFunc(s.handleNotFound)
}

// Middleware functions

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Process request
		next.ServeHTTP(wrapped, r)

		// Log request details
		s.logger.WithFields(logrus.Fields{
			"method":      r.Method,
			"path":        r.URL.Path,
			"remote_addr": r.RemoteAddr,
			"status":      wrapped.statusCode,
			"duration_ms": time.Since(start).Milliseconds(),
			"user_agent":  r.UserAgent(),
		}).Info("HTTP request")
	})
}

func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				s.logger.WithFields(logrus.Fields{
					"error":  err,
					"method": r.Method,
					"path":   r.URL.Path,
				}).Error("Panic recovered")

				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			s.sendUnauthorized(w, "Basic authentication required")
			return
		}

		if username != s.config.Server.Auth.Username || password != s.config.Server.Auth.Password {
			s.sendUnauthorized(w, "Invalid credentials")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) apiAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			s.sendUnauthorized(w, "Basic authentication required")
			return
		}

		// Check API credentials first, fall back to regular credentials
		validUser := username == s.config.Server.Auth.APIUsername || username == s.config.Server.Auth.Username
		validPass := password == s.config.Server.Auth.APIPassword || password == s.config.Server.Auth.Password

		if !validUser || !validPass {
			s.sendUnauthorized(w, "Invalid credentials")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Helper functions

func (s *Server) sendUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Alertmanager Gateway"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintf(w, `{"status":"error","error":"%s"}`, message)
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}