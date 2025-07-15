package server

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof" // Enable pprof endpoints for profiling
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/webhook"
)

// Server represents the HTTP server
type Server struct {
	config         *config.Config
	router         *mux.Router
	httpServer     *http.Server
	logger         *logrus.Logger
	webhookHandler *webhook.Handler
}

// New creates a new server instance
func New(cfg *config.Config, logger *logrus.Logger) (*Server, error) {
	webhookHandler, err := webhook.NewHandler(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook handler: %w", err)
	}

	s := &Server{
		config:         cfg,
		logger:         logger,
		router:         mux.NewRouter(),
		webhookHandler: webhookHandler,
	}

	// Setup routes
	s.setupRoutes()

	// Setup middleware
	s.router.Use(s.securityHeadersMiddleware)
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

	return s, nil
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

	// Close webhook handler
	if err := s.webhookHandler.Close(); err != nil {
		s.logger.WithError(err).Error("Failed to close webhook handler")
	}

	s.logger.Info("Server stopped gracefully")
	return nil
}

// GetRouter returns the server's router for testing
func (s *Server) GetRouter() *mux.Router {
	return s.router
}

// setupRoutes configures all routes
func (s *Server) setupRoutes() {
	// Health check endpoints (keeping legacy endpoints for backward compatibility)
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")
	s.router.HandleFunc("/health/live", s.handleHealthLive).Methods("GET")
	s.router.HandleFunc("/health/ready", s.handleHealthReady).Methods("GET")

	// Metrics endpoint (placeholder for now)
	s.router.HandleFunc("/metrics", s.handleMetrics).Methods("GET")

	// Profiling endpoints (only in debug mode or when explicitly enabled)
	if os.Getenv("ENABLE_PPROF") == "true" {
		s.router.PathPrefix("/debug/pprof/").Handler(http.DefaultServeMux)
		s.logger.Warn("pprof endpoints enabled at /debug/pprof/")
	}

	// API endpoints
	apiRouter := s.router.PathPrefix("/api/v1").Subrouter()
	if s.config.Server.Auth.Enabled && s.config.Server.Auth.APIUsername != "" {
		apiRouter.Use(s.apiAuthMiddleware)
	} else if s.config.Server.Auth.Enabled {
		apiRouter.Use(s.authMiddleware)
	}

	// Register API routes
	s.RegisterAPIRoutes(apiRouter)

	// Webhook endpoints
	webhookRouter := s.router.PathPrefix("/webhook").Subrouter()
	if s.config.Server.Auth.Enabled {
		webhookRouter.Use(s.authMiddleware)
	}
	webhookRouter.Use(webhook.ValidationMiddleware(s.logger))
	webhookRouter.HandleFunc("/{destination}", s.webhookHandler.HandleWebhook).Methods("POST")

	// Default handler for unmatched routes
	s.router.NotFoundHandler = http.HandlerFunc(s.handleNotFound)
}

// Middleware functions

func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Security headers to prevent common web vulnerabilities
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		// Remove server information
		w.Header().Set("Server", "")

		next.ServeHTTP(w, r)
	})
}

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
