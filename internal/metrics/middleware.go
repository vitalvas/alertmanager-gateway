package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// HTTPMetricsMiddleware creates a middleware that collects HTTP metrics
func HTTPMetricsMiddleware(metrics *Metrics) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Create a response writer wrapper to capture metrics
			wrapper := &responseWriterWrapper{
				ResponseWriter: w,
				statusCode:     http.StatusOK, // Default status
			}

			// Process the request
			next.ServeHTTP(wrapper, r)

			// Record metrics
			duration := time.Since(start)
			path := sanitizePath(r.URL.Path)
			statusCode := strconv.Itoa(wrapper.statusCode)

			metrics.RecordHTTPRequest(
				r.Method,
				path,
				statusCode,
				duration,
				r.ContentLength,
				int64(wrapper.bytesWritten),
			)
		})
	}
}

// responseWriterWrapper wraps http.ResponseWriter to capture metrics
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

// WriteHeader captures the status code
func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// Write captures the response size
func (w *responseWriterWrapper) Write(data []byte) (int, error) {
	size, err := w.ResponseWriter.Write(data)
	w.bytesWritten += size
	return size, err
}

// sanitizePath normalizes paths for metrics to avoid high cardinality
func sanitizePath(path string) string {
	// Remove query parameters
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	// Normalize common patterns
	switch {
	case strings.HasPrefix(path, "/webhook/"):
		return "/webhook/{destination}"
	case strings.HasPrefix(path, "/api/v1/destinations/") && len(strings.Split(path, "/")) > 4:
		return "/api/v1/destinations/{name}"
	case strings.HasPrefix(path, "/api/v1/test/"):
		return "/api/v1/test/{destination}"
	case path == "/health" || path == "/health/live" || path == "/health/ready":
		return path
	case path == "/metrics":
		return "/metrics"
	case strings.HasPrefix(path, "/api/v1/"):
		return path
	default:
		// For unknown paths, use a generic label to avoid cardinality explosion
		return "/other"
	}
}

// AuthMetricsMiddleware creates middleware that records authentication metrics
func AuthMetricsMiddleware(_ *Metrics) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// We'll record the actual result in the auth package
			// This middleware is for future authentication metrics integration
			next.ServeHTTP(w, r)
		})
	}
}

// isAuthenticatedEndpoint determines if an endpoint requires authentication
func isAuthenticatedEndpoint(path string) bool {
	return strings.HasPrefix(path, "/webhook/") || strings.HasPrefix(path, "/api/v1/")
}

// ActiveConnectionsMiddleware tracks active HTTP connections
func ActiveConnectionsMiddleware(metrics *Metrics) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			metrics.ActiveConnections.Inc()
			defer metrics.ActiveConnections.Dec()

			next.ServeHTTP(w, r)
		})
	}
}
