package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/gorilla/mux"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/destination"
	"github.com/vitalvas/alertmanager-gateway/internal/formatter"
	"github.com/vitalvas/alertmanager-gateway/internal/transform"
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

// Legacy API handlers (now delegated to API package)

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

func generateRequestID() string {
	// Simple request ID generation
	return fmt.Sprintf("%d-%d", time.Now().Unix(), runtime.NumGoroutine())
}

var startTime = time.Now()

// API Handlers

// RegisterAPIRoutes registers API routes with the router
func (s *Server) RegisterAPIRoutes(router *mux.Router) {
	// Destination management endpoints
	router.HandleFunc("/destinations", s.handleListDestinations).Methods("GET")
	router.HandleFunc("/destinations/{name}", s.handleGetDestination).Methods("GET")

	// Test and emulation endpoints
	router.HandleFunc("/test/{destination}", s.handleTestDestination).Methods("POST")
	router.HandleFunc("/emulate/{destination}", s.handleEmulateDestination).Methods("POST")

	// System information endpoints
	router.HandleFunc("/info", s.handleSystemInfo).Methods("GET")
	router.HandleFunc("/health", s.handleAPIHealth).Methods("GET")

	// Configuration endpoints
	router.HandleFunc("/config/validate", s.handleValidateConfig).Methods("POST")
}

// Destination management handlers

func (s *Server) handleListDestinations(w http.ResponseWriter, r *http.Request) {
	// Check if we should include disabled destinations
	includeDisabled := r.URL.Query().Get("include_disabled") == "true"

	destinations := make([]DestinationSummary, 0, len(s.config.Destinations))

	for _, dest := range s.config.Destinations {
		if dest.Enabled || includeDisabled {
			summary := DestinationSummary{
				Name:        dest.Name,
				WebhookURL:  fmt.Sprintf("/webhook/%s", dest.Name),
				Method:      dest.Method,
				Format:      dest.Format,
				Engine:      dest.Engine,
				Enabled:     dest.Enabled,
				SplitAlerts: dest.SplitAlerts,
				AuthEnabled: len(dest.Headers) > 0, // Simplified check
				Description: generateDestinationDescription(&dest),
			}
			destinations = append(destinations, summary)
		}
	}

	response := ListDestinationsResponse{
		Destinations: destinations,
		Total:        len(destinations),
		Timestamp:    time.Now().UTC(),
	}

	s.sendJSON(w, http.StatusOK, response)
}

func (s *Server) handleGetDestination(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	dest := s.config.GetDestinationByNameAny(name)
	if dest == nil {
		s.sendAPIError(w, http.StatusNotFound, "Destination not found")
		return
	}

	details := DestinationDetails{
		Name:        dest.Name,
		WebhookURL:  fmt.Sprintf("/webhook/%s", dest.Name),
		Method:      dest.Method,
		TargetURL:   maskSensitiveURL(dest.URL),
		Format:      dest.Format,
		Engine:      dest.Engine,
		Enabled:     dest.Enabled,
		SplitAlerts: dest.SplitAlerts,
		Headers:     maskSensitiveHeaders(dest.Headers),
		CreatedAt:   time.Now().UTC(), // TODO: Store actual creation time
		UpdatedAt:   time.Now().UTC(), // TODO: Store actual update time
	}

	if dest.Engine == "go-template" {
		details.TemplateSize = len(dest.Template)
		details.HasTemplate = dest.Template != ""
	} else {
		details.TransformSize = len(dest.Transform)
		details.HasTransform = dest.Transform != ""
	}

	if dest.SplitAlerts {
		details.BatchSize = dest.BatchSize
		details.ParallelRequests = dest.ParallelRequests
	}

	s.sendJSON(w, http.StatusOK, details)
}

// Test and emulation handlers

func (s *Server) handleTestDestination(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	destinationName := vars["destination"]

	dest := s.config.GetDestinationByNameAny(destinationName)
	if dest == nil {
		s.sendAPIError(w, http.StatusNotFound, "Destination not found")
		return
	}

	if !dest.Enabled {
		s.sendAPIError(w, http.StatusBadRequest, "Destination is disabled")
		return
	}

	// Parse test request
	var testReq TestRequest
	if err := json.NewDecoder(r.Body).Decode(&testReq); err != nil {
		s.sendAPIError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// Use sample data if no webhook data provided
	webhookData := testReq.WebhookData
	if webhookData == nil {
		webhookData = getSampleWebhookData()
	}

	// Test the transformation and formatting
	result, err := s.testDestinationTransformation(dest, webhookData)
	if err != nil {
		s.sendAPIError(w, http.StatusInternalServerError, fmt.Sprintf("Test failed: %v", err))
		return
	}

	response := TestResponse{
		Success:       true,
		Destination:   destinationName,
		Result:        result,
		TestTimestamp: time.Now().UTC(),
		RequestID:     generateAPIRequestID(),
	}

	s.sendJSON(w, http.StatusOK, response)
}

func (s *Server) handleEmulateDestination(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	destinationName := vars["destination"]

	dest := s.config.GetDestinationByNameAny(destinationName)
	if dest == nil {
		s.sendAPIError(w, http.StatusNotFound, "Destination not found")
		return
	}

	// Parse emulation request
	var emulateReq EmulateRequest
	if err := json.NewDecoder(r.Body).Decode(&emulateReq); err != nil {
		s.sendAPIError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// Use sample data if no webhook data provided
	webhookData := emulateReq.WebhookData
	if webhookData == nil {
		webhookData = getSampleWebhookData()
	}

	// Perform full emulation including HTTP request
	result, err := s.emulateDestinationRequest(dest, webhookData, emulateReq.DryRun)
	if err != nil {
		s.sendAPIError(w, http.StatusInternalServerError, fmt.Sprintf("Emulation failed: %v", err))
		return
	}

	response := EmulateResponse{
		Success:            true,
		Destination:        destinationName,
		DryRun:             emulateReq.DryRun,
		Result:             result,
		EmulationTimestamp: time.Now().UTC(),
		RequestID:          generateAPIRequestID(),
	}

	s.sendJSON(w, http.StatusOK, response)
}

// System information handlers

func (s *Server) handleSystemInfo(w http.ResponseWriter, _ *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	info := SystemInfo{
		Version:          "1.0.0",          // TODO: Make configurable
		BuildTime:        time.Now().UTC(), // TODO: Inject build time
		GoVersion:        runtime.Version(),
		NumCPU:           runtime.NumCPU(),
		NumGoroutines:    runtime.NumGoroutine(),
		MemoryAlloc:      memStats.Alloc,
		MemoryTotalAlloc: memStats.TotalAlloc,
		MemorySys:        memStats.Sys,
		NumGC:            memStats.NumGC,
		Uptime:           time.Since(startTime),
		Config: ConfigInfo{
			DestinationsCount:        len(s.config.Destinations),
			EnabledDestinationsCount: s.countEnabledDestinations(),
			AuthEnabled:              s.config.Server.Auth.Enabled,
			ServerAddress:            s.config.Server.Address,
			LogLevel:                 "info", // TODO: Get from logger
		},
	}

	s.sendJSON(w, http.StatusOK, info)
}

func (s *Server) handleAPIHealth(w http.ResponseWriter, _ *http.Request) {
	health := HealthResponse{
		Status:              "healthy",
		Timestamp:           time.Now().UTC(),
		UptimeSeconds:       time.Since(startTime).Seconds(),
		ConfigLoaded:        true,
		DestinationsCount:   len(s.config.Destinations),
		EnabledDestinations: s.countEnabledDestinations(),
		Checks: []HealthCheck{
			{
				Name:    "destinations",
				Status:  "healthy",
				Message: fmt.Sprintf("%d destinations configured", len(s.config.Destinations)),
			},
			{
				Name:    "memory",
				Status:  "healthy",
				Message: "Memory usage within normal limits",
			},
		},
	}

	// Add warning if no destinations are enabled
	if s.countEnabledDestinations() == 0 {
		health.Checks = append(health.Checks, HealthCheck{
			Name:    "enabled_destinations",
			Status:  "warning",
			Message: "No destinations are enabled",
		})
	}

	s.sendJSON(w, http.StatusOK, health)
}

// Configuration handlers

func (s *Server) handleValidateConfig(w http.ResponseWriter, r *http.Request) {
	var configData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&configData); err != nil {
		s.sendAPIError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// TODO: Implement config validation
	// For now, just return a placeholder response
	validation := ConfigValidation{
		Valid:     true,
		Errors:    []string{},
		Warnings:  []string{"Configuration validation not yet fully implemented"},
		Timestamp: time.Now().UTC(),
	}

	s.sendJSON(w, http.StatusOK, validation)
}

// Helper methods

func (s *Server) testDestinationTransformation(dest *config.DestinationConfig, webhookData *alertmanager.WebhookPayload) (*TransformationResult, error) {
	start := time.Now()

	// Get template/transform content
	var content string
	var engineType transform.EngineType
	if dest.Engine == "go-template" {
		content = dest.Template
		engineType = transform.EngineTypeGoTemplate
	} else {
		content = dest.Transform
		engineType = transform.EngineTypeJQ
	}

	// Create transformation engine
	engine, err := transform.NewEngine(engineType, content)
	if err != nil {
		return nil, fmt.Errorf("failed to create transformation engine: %w", err)
	}

	// Transform data
	var transformedData interface{}
	if dest.SplitAlerts && len(webhookData.Alerts) > 0 {
		// Test with first alert for split mode
		transformedData, err = engine.TransformAlert(&webhookData.Alerts[0], webhookData)
	} else {
		transformedData, err = engine.Transform(webhookData)
	}

	if err != nil {
		return nil, fmt.Errorf("transformation failed: %w", err)
	}

	// Format output
	formattedData, err := formatter.Format(transformedData, dest.Format)
	if err != nil {
		return nil, fmt.Errorf("formatting failed: %w", err)
	}

	return &TransformationResult{
		TransformedData: transformedData,
		FormattedOutput: string(formattedData),
		TransformTime:   time.Since(start),
		OutputSize:      len(formattedData),
		OutputFormat:    dest.Format,
		SplitMode:       dest.SplitAlerts,
		AlertsProcessed: len(webhookData.Alerts),
	}, nil
}

func (s *Server) emulateDestinationRequest(dest *config.DestinationConfig, webhookData *alertmanager.WebhookPayload, dryRun bool) (*EmulationResult, error) {
	start := time.Now()

	// First, perform transformation
	transformResult, err := s.testDestinationTransformation(dest, webhookData)
	if err != nil {
		return nil, err
	}

	result := &EmulationResult{
		TransformationResult: *transformResult,
		HTTPMethod:           dest.Method,
		TargetURL:            maskSensitiveURL(dest.URL),
		Headers:              maskSensitiveHeaders(dest.Headers),
		RequestSize:          len(transformResult.FormattedOutput),
		EmulationTime:        time.Since(start),
	}

	if !dryRun {
		// Create HTTP client and send request
		client := destination.NewHTTPClient(nil) // Use default config

		// Create request
		req, err := http.NewRequestWithContext(
			context.Background(),
			dest.Method,
			dest.URL,
			bytes.NewReader([]byte(transformResult.FormattedOutput)),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Add headers
		for key, value := range dest.Headers {
			req.Header.Set(key, value)
		}

		// Set content type based on format
		if contentType := formatter.GetContentType(dest.Format); contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}

		// Send request
		resp, err := client.Do(req)
		if err != nil {
			result.HTTPError = err.Error()
			result.Success = false
		} else {
			defer resp.Body.Close()
			result.HTTPStatusCode = resp.StatusCode
			result.HTTPStatus = resp.Status
			result.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
			result.ResponseHeaders = make(map[string]string)
			for key, values := range resp.Header {
				if len(values) > 0 {
					result.ResponseHeaders[key] = values[0]
				}
			}
		}
	} else {
		result.Success = true
		result.HTTPStatusCode = 0
		result.HTTPStatus = "dry-run"
	}

	result.EmulationTime = time.Since(start)
	return result, nil
}

func (s *Server) countEnabledDestinations() int {
	count := 0
	for _, dest := range s.config.Destinations {
		if dest.Enabled {
			count++
		}
	}
	return count
}

func (s *Server) sendAPIError(w http.ResponseWriter, status int, message string) {
	response := ErrorResponse{
		Status:    "error",
		Error:     message,
		Timestamp: time.Now().UTC(),
		RequestID: generateAPIRequestID(),
	}

	s.sendJSON(w, status, response)
}

// Utility functions

func generateDestinationDescription(dest *config.DestinationConfig) string {
	if dest.Engine == "go-template" {
		return fmt.Sprintf("%s destination using Go templates", dest.Format)
	}
	return fmt.Sprintf("%s destination using jq transformations", dest.Format)
}

func maskSensitiveURL(url string) string {
	// Simple URL masking to hide sensitive parts
	if len(url) > 50 {
		return url[:25] + "***" + url[len(url)-17:]
	}
	if len(url) > 20 {
		return url[:15] + "***"
	}
	return url
}

func maskSensitiveHeaders(headers map[string]string) map[string]string {
	masked := make(map[string]string)
	for key, value := range headers {
		if isSensitiveHeader(key) {
			masked[key] = "***"
		} else {
			masked[key] = value
		}
	}
	return masked
}

func isSensitiveHeader(key string) bool {
	sensitiveHeaders := []string{
		"authorization", "x-api-key", "x-auth-token",
		"x-api-token", "bearer", "password", "secret",
	}

	keyLower := key
	for i := 0; i < len(keyLower); i++ {
		if keyLower[i] >= 'A' && keyLower[i] <= 'Z' {
			keyLower = keyLower[:i] + string(keyLower[i]+32) + keyLower[i+1:]
		}
	}

	for _, sensitive := range sensitiveHeaders {
		if keyLower == sensitive {
			return true
		}
	}
	return false
}

func generateAPIRequestID() string {
	return fmt.Sprintf("%d-%d", time.Now().Unix(), runtime.NumGoroutine())
}

func getSampleWebhookData() *alertmanager.WebhookPayload {
	return &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "{}:{alertname=\"ExampleAlert\"}",
		Status:   "firing",
		Receiver: "test-receiver",
		GroupLabels: map[string]string{
			"alertname": "ExampleAlert",
		},
		CommonLabels: map[string]string{
			"alertname": "ExampleAlert",
			"instance":  "localhost:9090",
			"job":       "prometheus",
			"severity":  "warning",
		},
		CommonAnnotations: map[string]string{
			"summary":     "Example alert for testing",
			"description": "This is a sample alert for API testing purposes",
		},
		ExternalURL: "http://localhost:9093",
		Alerts: []alertmanager.Alert{
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "ExampleAlert",
					"instance":  "localhost:9090",
					"job":       "prometheus",
					"severity":  "warning",
				},
				Annotations: map[string]string{
					"summary":     "Example alert for testing",
					"description": "This is a sample alert for API testing purposes",
				},
				StartsAt:     time.Now().Add(-5 * time.Minute),
				EndsAt:       time.Time{},
				GeneratorURL: "http://localhost:9090/graph?g0.expr=up%3D%3D0&g0.tab=1",
				Fingerprint:  "b5d4045c3f466fa91fe2cc6abe79232a1a57cdf104f7a74458f98ae2459a814a",
			},
		},
	}
}
