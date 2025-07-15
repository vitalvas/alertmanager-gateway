package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/destination"
)

// mockDestinationHandler is a mock implementation for testing
type mockDestinationHandler struct {
	sendFunc func(ctx context.Context, payload *alertmanager.WebhookPayload) error
	name     string
}

func (m *mockDestinationHandler) Send(ctx context.Context, payload *alertmanager.WebhookPayload) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, payload)
	}
	return nil
}

func (m *mockDestinationHandler) Name() string {
	return m.name
}

func (m *mockDestinationHandler) Close() error {
	return nil
}

func TestHandler_HandleWebhook(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		Destinations: []config.DestinationConfig{
			{
				Name:     "test-dest",
				Enabled:  true,
				URL:      "http://example.com/webhook",
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"status": "{{ .Status }}"}`,
			},
			{
				Name:    "disabled-dest",
				Enabled: false,
				URL:     "http://example.com/disabled",
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create handler with empty handlers map (we'll add mocks directly)
	handler := &Handler{
		config:   cfg,
		logger:   logger,
		handlers: make(map[string]destination.Handler),
	}

	// Add mock handler for test-dest
	handler.handlers["test-dest"] = &mockDestinationHandler{
		name: "test-dest",
	}

	// Create valid webhook payload
	validPayload := alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		Receiver: "test",
		GroupLabels: map[string]string{
			"alertname": "TestAlert",
		},
		CommonLabels: map[string]string{
			"alertname": "TestAlert",
			"severity":  "warning",
		},
		CommonAnnotations: map[string]string{
			"summary": "Test alert summary",
		},
		ExternalURL: "http://alertmanager.example.com",
		Alerts: []alertmanager.Alert{
			{
				Status:      "firing",
				Fingerprint: "abc123",
				StartsAt:    time.Now(),
				Labels: map[string]string{
					"alertname": "TestAlert",
					"instance":  "server1",
					"severity":  "warning",
				},
				Annotations: map[string]string{
					"summary": "Test alert summary",
				},
				GeneratorURL: "http://prometheus.example.com",
			},
		},
	}

	validJSON, err := json.Marshal(validPayload)
	require.NoError(t, err)

	tests := []struct {
		name           string
		destination    string
		requestBody    string
		expectedStatus int
		checkResponse  func(t *testing.T, resp map[string]interface{})
	}{
		{
			name:           "valid webhook",
			destination:    "test-dest",
			requestBody:    string(validJSON),
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "success", resp["status"])
				assert.Equal(t, "test-dest", resp["destination"])
				assert.Equal(t, float64(1), resp["alerts_count"])
				assert.Equal(t, "test-group", resp["group_key"])
				assert.NotNil(t, resp["processing_ms"])
			},
		},
		{
			name:           "destination not found",
			destination:    "nonexistent",
			requestBody:    string(validJSON),
			expectedStatus: http.StatusNotFound,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "error", resp["status"])
				assert.Contains(t, resp["error"], "Destination not found")
			},
		},
		{
			name:           "disabled destination",
			destination:    "disabled-dest",
			requestBody:    string(validJSON),
			expectedStatus: http.StatusNotFound,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "error", resp["status"])
				assert.Contains(t, resp["error"], "Destination not found")
			},
		},
		{
			name:           "invalid JSON",
			destination:    "test-dest",
			requestBody:    "{invalid json}",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "error", resp["status"])
				assert.Contains(t, resp["error"], "Invalid payload")
			},
		},
		{
			name:           "missing required fields",
			destination:    "test-dest",
			requestBody:    `{"version": "4", "status": "firing"}`,
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "error", resp["status"])
				assert.Contains(t, resp["error"], "Invalid payload")
			},
		},
		{
			name:           "payload too large",
			destination:    "test-dest",
			requestBody:    strings.Repeat("x", alertmanager.MaxPayloadSize+1),
			expectedStatus: http.StatusRequestEntityTooLarge,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "error", resp["status"])
				assert.Contains(t, resp["error"], "Invalid payload")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req := httptest.NewRequest("POST", "/webhook/"+tt.destination, strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Request-ID", "test-request-id")

			// Add mux vars
			req = mux.SetURLVars(req, map[string]string{
				"destination": tt.destination,
			})

			// Create response recorder
			w := httptest.NewRecorder()

			// Handle request
			handler.HandleWebhook(w, req)

			// Check status code
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Parse response
			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)

			// Check response
			tt.checkResponse(t, resp)
		})
	}
}

func TestWebhookResponse_JSON(t *testing.T) {
	resp := Response{
		Status:       "success",
		Destination:  "test",
		ReceivedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		AlertsCount:  5,
		GroupKey:     "test-group",
		ProcessingMS: 42,
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "success", result["status"])
	assert.Equal(t, "test", result["destination"])
	assert.Equal(t, "2024-01-01T12:00:00Z", result["received_at"])
	assert.Equal(t, float64(5), result["alerts_count"])
	assert.Equal(t, "test-group", result["group_key"])
	assert.Equal(t, float64(42), result["processing_ms"])
}

func TestErrorResponse_JSON(t *testing.T) {
	resp := ErrorResponse{
		Status:    "error",
		Error:     "Something went wrong",
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "error", result["status"])
	assert.Equal(t, "Something went wrong", result["error"])
	assert.Equal(t, "2024-01-01T12:00:00Z", result["timestamp"])
}

func TestHandler_sendJSONResponse(t *testing.T) {
	logger := logrus.New()
	handler := &Handler{logger: logger}

	data := map[string]string{
		"test": "value",
	}

	w := httptest.NewRecorder()
	handler.sendJSONResponse(w, http.StatusOK, data)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var result map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "value", result["test"])
}

func TestHandler_sendErrorResponse(t *testing.T) {
	logger := logrus.New()
	handler := &Handler{logger: logger}

	w := httptest.NewRecorder()
	handler.sendErrorResponse(w, http.StatusBadRequest, "Bad request")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "error", result["status"])
	assert.Equal(t, "Bad request", result["error"])
	assert.NotNil(t, result["timestamp"])
}
