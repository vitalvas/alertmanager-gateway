package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// Health check handlers

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	health := map[string]interface{}{
		"status":             "healthy",
		"version":            "1.0.0", // TODO: Make this configurable
		"uptime_seconds":     time.Since(startTime).Seconds(),
		"config_loaded":      true,
		"destinations_count": len(s.config.Destinations),
	}

	s.sendJSON(w, http.StatusOK, health)
}

func (s *Server) handleHealthLive(w http.ResponseWriter, _ *http.Request) {
	response := map[string]string{
		"status": "alive",
	}
	s.sendJSON(w, http.StatusOK, response)
}

func (s *Server) handleHealthReady(w http.ResponseWriter, _ *http.Request) {
	// Check if we're ready to accept traffic
	ready := true
	status := "ready"

	if len(s.config.Destinations) == 0 {
		ready = false
		status = "no destinations configured"
	}

	response := map[string]interface{}{
		"status":        status,
		"config_loaded": true,
	}

	if ready {
		s.sendJSON(w, http.StatusOK, response)
	} else {
		s.sendJSON(w, http.StatusServiceUnavailable, response)
	}
}

// Metrics handler (placeholder)

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	// TODO: Implement Prometheus metrics
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "# HELP alertmanager_gateway_info Gateway information\n")
	fmt.Fprintf(w, "# TYPE alertmanager_gateway_info gauge\n")
	fmt.Fprintf(w, "alertmanager_gateway_info{version=\"1.0.0\"} 1\n")
}

// API handlers

func (s *Server) handleListDestinations(w http.ResponseWriter, _ *http.Request) {
	destinations := make([]map[string]interface{}, 0, len(s.config.Destinations))

	for _, dest := range s.config.Destinations {
		if dest.Enabled {
			destinations = append(destinations, map[string]interface{}{
				"name":    dest.Name,
				"path":    dest.Path,
				"method":  dest.Method,
				"format":  dest.Format,
				"enabled": dest.Enabled,
			})
		}
	}

	response := map[string]interface{}{
		"destinations": destinations,
	}

	s.sendJSON(w, http.StatusOK, response)
}

func (s *Server) handleGetDestination(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	dest := s.config.GetDestinationByName(name)
	if dest == nil {
		s.sendError(w, http.StatusNotFound, "Destination not found")
		return
	}

	response := map[string]interface{}{
		"name":          dest.Name,
		"path":          dest.Path,
		"method":        dest.Method,
		"url":           maskURL(dest.URL),
		"format":        dest.Format,
		"engine":        dest.Engine,
		"headers_count": len(dest.Headers),
		"enabled":       dest.Enabled,
		"split_alerts":  dest.SplitAlerts,
	}

	if dest.Engine == "go-template" {
		response["template_size"] = len(dest.Template)
	} else {
		response["transform_size"] = len(dest.Transform)
	}

	if dest.SplitAlerts {
		response["batch_size"] = dest.BatchSize
		response["parallel_requests"] = dest.ParallelRequests
	}

	response["retry"] = map[string]interface{}{
		"max_attempts": dest.Retry.MaxAttempts,
		"backoff":      dest.Retry.Backoff,
		"per_alert":    dest.Retry.PerAlert,
	}

	s.sendJSON(w, http.StatusOK, response)
}

func (s *Server) handleTestDestination(w http.ResponseWriter, _ *http.Request) {
	// TODO: Implement test destination handler
	s.sendError(w, http.StatusNotImplemented, "Test endpoint not yet implemented")
}

// Webhook handler

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	destination := vars["destination"]

	s.logger.WithFields(logrus.Fields{
		"destination": destination,
		"path":        r.URL.Path,
	}).Debug("Received webhook request")

	// TODO: Implement webhook processing
	s.sendError(w, http.StatusNotImplemented, "Webhook processing not yet implemented")
}

// Not found handler

func (s *Server) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	s.sendError(w, http.StatusNotFound, "Endpoint not found")
}

// Helper functions

func (s *Server) sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.WithError(err).Error("Failed to encode JSON response")
	}
}

func (s *Server) sendError(w http.ResponseWriter, status int, message string) {
	response := map[string]interface{}{
		"status":     "error",
		"error":      message,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"request_id": generateRequestID(),
	}

	s.sendJSON(w, status, response)
}

func maskURL(url string) string {
	// Simple URL masking to hide sensitive parts
	if len(url) > 30 {
		return url[:20] + "***"
	}
	return url
}

func generateRequestID() string {
	// Simple request ID generation
	return fmt.Sprintf("%d-%d", time.Now().Unix(), runtime.NumGoroutine())
}

var startTime = time.Now()