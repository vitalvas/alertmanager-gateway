package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/destination"
)

// Handler handles incoming webhook requests
type Handler struct {
	config   *config.Config
	logger   *logrus.Logger
	handlers map[string]destination.Handler
}

// NewHandler creates a new webhook handler
func NewHandler(cfg *config.Config, logger *logrus.Logger) (*Handler, error) {
	h := &Handler{
		config:   cfg,
		logger:   logger,
		handlers: make(map[string]destination.Handler),
	}

	// Initialize destination handlers
	for _, destCfg := range cfg.Destinations {
		if !destCfg.Enabled {
			continue
		}

		destHandler, err := destination.NewHTTPHandler(&destCfg, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create handler for destination %s: %w", destCfg.Name, err)
		}

		h.handlers[destCfg.Name] = destHandler
	}

	return h, nil
}

// HandleWebhook processes incoming webhook requests
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Get destination name from URL
	vars := mux.Vars(r)
	destName := vars["destination"]

	// Create a logger with request context
	logger := h.logger.WithFields(logrus.Fields{
		"destination": destName,
		"remote_addr": r.RemoteAddr,
		"request_id":  r.Header.Get("X-Request-ID"),
	})

	// Find destination configuration
	dest := h.config.GetDestinationByName(destName)
	if dest == nil {
		logger.Warn("Destination not found")
		h.sendErrorResponse(w, http.StatusNotFound, "Destination not found")
		return
	}

	// Parse the webhook payload
	payload, err := alertmanager.ParseWebhookPayload(r)
	if err != nil {
		logger.WithError(err).Error("Failed to parse webhook payload")

		// Determine appropriate error code
		statusCode := http.StatusBadRequest
		if errors.Is(err, alertmanager.ErrPayloadTooLarge) {
			statusCode = http.StatusRequestEntityTooLarge
		}

		h.sendErrorResponse(w, statusCode, fmt.Sprintf("Invalid payload: %v", err))
		return
	}

	// Log webhook details
	logger = logger.WithFields(logrus.Fields{
		"group_key":    payload.GroupKey,
		"status":       payload.Status,
		"alerts_count": len(payload.Alerts),
		"receiver":     payload.Receiver,
	})

	logger.Info("Received webhook payload")

	// Log individual alerts at debug level
	for i, alert := range payload.Alerts {
		logger.WithFields(logrus.Fields{
			"alert_index": i,
			"fingerprint": alert.Fingerprint,
			"status":      alert.Status,
			"alertname":   alert.GetAlertName(),
			"severity":    alert.GetSeverity(),
			"starts_at":   alert.StartsAt,
		}).Debug("Alert details")
	}

	// Get destination handler
	handler, exists := h.handlers[destName]
	if !exists {
		h.sendErrorResponse(w, http.StatusNotFound, "Destination handler not initialized")
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Send to destination
	err = handler.Send(ctx, payload)
	if err != nil {
		logger.WithError(err).Error("Failed to send alerts to destination")
		h.sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to send alerts: %v", err))
		return
	}

	// Success response
	response := Response{
		Status:       "success",
		Destination:  destName,
		ReceivedAt:   time.Now().UTC(),
		AlertsCount:  len(payload.Alerts),
		GroupKey:     payload.GroupKey,
		ProcessingMS: time.Since(start).Milliseconds(),
	}

	h.sendJSONResponse(w, http.StatusOK, response)
}

// Response represents the response for a webhook request
type Response struct {
	Status       string    `json:"status"`
	Destination  string    `json:"destination"`
	ReceivedAt   time.Time `json:"received_at"`
	AlertsCount  int       `json:"alerts_count"`
	GroupKey     string    `json:"group_key"`
	ProcessingMS int64     `json:"processing_ms"`
	Error        string    `json:"error,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Status    string    `json:"status"`
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
}

// sendJSONResponse sends a JSON response
func (h *Handler) sendJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.WithError(err).Error("Failed to encode JSON response")
	}
}

// sendErrorResponse sends an error response
func (h *Handler) sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := ErrorResponse{
		Status:    "error",
		Error:     message,
		Timestamp: time.Now().UTC(),
	}

	h.sendJSONResponse(w, statusCode, response)
}

// Close cleans up all destination handlers
func (h *Handler) Close() error {
	for name, handler := range h.handlers {
		if err := handler.Close(); err != nil {
			h.logger.WithError(err).WithField("destination", name).Error("Failed to close destination handler")
		}
	}
	return nil
}
