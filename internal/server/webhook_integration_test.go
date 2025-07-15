package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
)

// TestWebhookToDestinationFlow tests the complete flow from webhook to destination
func TestWebhookToDestinationFlow(t *testing.T) {
	// Create a mock destination server
	var receivedRequests []mockRequest
	mockDestination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedRequests = append(receivedRequests, mockRequest{
			Method:  r.Method,
			URL:     r.URL.String(),
			Headers: r.Header.Clone(),
			Body:    body,
		})
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"success"}`))
	}))
	defer mockDestination.Close()

	// Create test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "test-destination",
				URL:      mockDestination.URL + "/webhook",
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"alert":"{{.Status}}","count":{{len .Alerts}}}`,
				Enabled:  true,
				Headers: map[string]string{
					"X-Custom-Header": "test-value",
				},
			},
		},
	}

	// Create server
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
	srv, err := New(cfg, logger)
	require.NoError(t, err)

	// Create test server
	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Create test webhook payload
	webhookPayload := &alertmanager.WebhookPayload{
		Version:     "4",
		GroupKey:    "test-group",
		Status:      "firing",
		Receiver:    "test-receiver",
		GroupLabels: map[string]string{"alertname": "TestAlert"},
		Alerts: []alertmanager.Alert{
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "TestAlert",
					"severity":  "warning",
				},
				StartsAt:    time.Now(),
				Fingerprint: "test-alert-123",
			},
		},
	}

	// Send webhook request
	payloadBytes, err := json.Marshal(webhookPayload)
	require.NoError(t, err)

	resp, err := http.Post(
		ts.URL+"/webhook/test-destination",
		"application/json",
		bytes.NewReader(payloadBytes),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Verify response
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Wait for async request to complete with timeout
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for request. Received %d requests", len(receivedRequests))
		case <-ticker.C:
			if len(receivedRequests) >= 1 {
				goto verifyRequest
			}
		}
	}

verifyRequest:
	// Verify destination received the request
	require.Len(t, receivedRequests, 1)
	req := receivedRequests[0]

	assert.Equal(t, "POST", req.Method)
	assert.Equal(t, "/webhook", req.URL)
	assert.Equal(t, "test-value", req.Headers.Get("X-Custom-Header"))
	assert.Equal(t, "application/json", req.Headers.Get("Content-Type"))

	// Verify transformed payload
	var transformedPayload map[string]interface{}
	err = json.Unmarshal(req.Body, &transformedPayload)
	require.NoError(t, err)

	assert.Equal(t, "firing", transformedPayload["alert"])
	assert.Equal(t, float64(1), transformedPayload["count"])
}

// TestWebhookFlowWithAuthentication tests webhook flow with authentication enabled
func TestWebhookFlowWithAuthentication(t *testing.T) {
	mockDestination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDestination.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
			Auth: config.AuthConfig{
				Enabled:  true,
				Username: "webhook-user",
				Password: "webhook-pass",
			},
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "auth-test",
				URL:      mockDestination.URL,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"status":"{{.Status}}"}`,
				Enabled:  true,
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, err := New(cfg, logger)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Allow server to initialize
	time.Sleep(100 * time.Millisecond)

	webhookPayload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "auth-test-group",
		Status:   "firing",
		Alerts:   []alertmanager.Alert{{Status: "firing", Fingerprint: "test-auth-123", StartsAt: time.Now()}},
	}
	payloadBytes, err := json.Marshal(webhookPayload)
	require.NoError(t, err)

	// Test without authentication - should fail
	resp, err := http.Post(
		ts.URL+"/webhook/auth-test",
		"application/json",
		bytes.NewReader(payloadBytes),
	)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Test with authentication - should succeed
	req, err := http.NewRequest("POST", ts.URL+"/webhook/auth-test", bytes.NewReader(payloadBytes))
	require.NoError(t, err)
	req.SetBasicAuth("webhook-user", "webhook-pass")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err = client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestWebhookFlowWithMultipleDestinations tests webhook sent to multiple destinations
func TestWebhookFlowWithMultipleDestinations(t *testing.T) {
	// Create multiple mock destinations
	var dest1Requests, dest2Requests []mockRequest

	mockDest1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		dest1Requests = append(dest1Requests, mockRequest{Body: body})
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest1.Close()

	mockDest2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		dest2Requests = append(dest2Requests, mockRequest{Body: body})
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest2.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "dest1",
				URL:      mockDest1.URL,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"destination":"dest1","status":"{{.Status}}"}`,
				Enabled:  true,
			},
			{
				Name:      "dest2",
				URL:       mockDest2.URL,
				Method:    "POST",
				Format:    "json",
				Engine:    "jq",
				Transform: `{destination: "dest2", status: .status}`,
				Enabled:   true,
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, err := New(cfg, logger)
	require.NoError(t, err)

	router := srv.GetRouter()

	webhookPayload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "multi-dest-group",
		Status:   "resolved",
		Alerts:   []alertmanager.Alert{{Status: "firing", Fingerprint: "test-auth-123", StartsAt: time.Now()}},
	}
	payloadBytes, err := json.Marshal(webhookPayload)
	require.NoError(t, err)

	// Send to destination 1
	req1 := httptest.NewRequest("POST", "/webhook/dest1", bytes.NewReader(payloadBytes))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Send to destination 2
	req2 := httptest.NewRequest("POST", "/webhook/dest2", bytes.NewReader(payloadBytes))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	// Give async requests time to complete
	time.Sleep(100 * time.Millisecond)

	// Verify both destinations received their requests
	require.Len(t, dest1Requests, 1)
	require.Len(t, dest2Requests, 1)

	// Verify destination 1 payload (go-template)
	var payload1 map[string]interface{}
	err = json.Unmarshal(dest1Requests[0].Body, &payload1)
	require.NoError(t, err)
	assert.Equal(t, "dest1", payload1["destination"])
	assert.Equal(t, "resolved", payload1["status"])

	// Verify destination 2 payload (jq)
	var payload2 map[string]interface{}
	err = json.Unmarshal(dest2Requests[0].Body, &payload2)
	require.NoError(t, err)
	assert.Equal(t, "dest2", payload2["destination"])
	assert.Equal(t, "resolved", payload2["status"])
}

// TestWebhookFlowWithAlertSplitting tests webhook flow with alert splitting
func TestWebhookFlowWithAlertSplitting(t *testing.T) {
	var receivedRequests []mockRequest
	mockDestination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedRequests = append(receivedRequests, mockRequest{
			Method: r.Method,
			Body:   body,
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDestination.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:        "split-test",
				URL:         mockDestination.URL,
				Method:      "POST",
				Format:      "json",
				Engine:      "go-template",
				Template:    `{"alertname":"{{(index .Alerts 0).Labels.alertname}}","severity":"{{(index .Alerts 0).Labels.severity}}"}`,
				Enabled:     true,
				SplitAlerts: true,
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, err := New(cfg, logger)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Allow server to initialize
	time.Sleep(100 * time.Millisecond)

	// Create webhook payload with multiple alerts
	webhookPayload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "split-test-group",
		Status:   "firing",
		Alerts: []alertmanager.Alert{
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "HighCPU",
					"severity":  "warning",
				},
				Fingerprint: "high-cpu-123",
				StartsAt:    time.Now(),
			},
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "HighMemory",
					"severity":  "critical",
				},
				Fingerprint: "high-memory-456",
				StartsAt:    time.Now(),
			},
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "DiskFull",
					"severity":  "warning",
				},
				Fingerprint: "disk-full-789",
				StartsAt:    time.Now(),
			},
		},
	}

	payloadBytes, err := json.Marshal(webhookPayload)
	require.NoError(t, err)

	resp, err := http.Post(
		ts.URL+"/webhook/split-test",
		"application/json",
		bytes.NewReader(payloadBytes),
	)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Give async requests time to complete
	time.Sleep(200 * time.Millisecond)

	// Verify each alert was sent separately
	require.Len(t, receivedRequests, 3)

	// Verify each split request
	expectedAlerts := []struct {
		alertname string
		severity  string
	}{
		{"HighCPU", "warning"},
		{"HighMemory", "critical"},
		{"DiskFull", "warning"},
	}

	for i, expected := range expectedAlerts {
		var payload map[string]interface{}
		err = json.Unmarshal(receivedRequests[i].Body, &payload)
		require.NoError(t, err)
		assert.Equal(t, expected.alertname, payload["alertname"], "Alert %d", i)
		assert.Equal(t, expected.severity, payload["severity"], "Alert %d", i)
	}
}

// TestWebhookFlowWithRetry tests webhook flow with retry on failure
func TestWebhookFlowWithRetry(t *testing.T) {
	attemptCount := 0
	mockDestination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			// Fail the first attempt
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Succeed on second attempt
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDestination.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "retry-test",
				URL:      mockDestination.URL,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"status":"{{.Status}}"}`,
				Enabled:  true,
				Retry: config.RetryConfig{
					MaxAttempts: 3,
					Backoff:     "constant",
				},
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, err := New(cfg, logger)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Allow server to initialize
	time.Sleep(100 * time.Millisecond)

	webhookPayload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "retry-test-group",
		Status:   "firing",
		Alerts:   []alertmanager.Alert{{Status: "firing", Fingerprint: "test-auth-123", StartsAt: time.Now()}},
	}
	payloadBytes, err := json.Marshal(webhookPayload)
	require.NoError(t, err)

	resp, err := http.Post(
		ts.URL+"/webhook/retry-test",
		"application/json",
		bytes.NewReader(payloadBytes),
	)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Give time for retry to complete
	time.Sleep(100 * time.Millisecond)

	// Verify retry happened
	assert.Equal(t, 2, attemptCount, "Expected 2 attempts (1 failure + 1 retry success)")
}

// TestWebhookFlowWithInvalidPayload tests webhook flow with invalid payload
func TestWebhookFlowWithInvalidPayload(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "test-dest",
				URL:      "http://example.com",
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"status":"{{.Status}}"}`,
				Enabled:  true,
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, err := New(cfg, logger)
	require.NoError(t, err)

	router := srv.GetRouter()

	testCases := []struct {
		name               string
		payload            string
		expectedStatusCode int
		contentType        string
	}{
		{
			name:               "Invalid JSON",
			payload:            `{"invalid json`,
			expectedStatusCode: http.StatusBadRequest,
			contentType:        "application/json",
		},
		{
			name:               "Missing version field",
			payload:            `{"status":"firing"}`,
			expectedStatusCode: http.StatusBadRequest,
			contentType:        "application/json",
		},
		{
			name:               "Wrong content type",
			payload:            `{"version":"4","status":"firing"}`,
			expectedStatusCode: http.StatusBadRequest,
			contentType:        "text/plain",
		},
		{
			name:               "Empty body",
			payload:            ``,
			expectedStatusCode: http.StatusBadRequest,
			contentType:        "application/json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/webhook/test-dest", bytes.NewBufferString(tc.payload))
			req.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatusCode, w.Code)
		})
	}
}

// TestWebhookFlowWithFormattedOutput tests different output formats
func TestWebhookFlowWithFormattedOutput(t *testing.T) {
	var receivedRequests []mockRequest
	mockDestination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedRequests = append(receivedRequests, mockRequest{
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDestination.Close()

	testCases := []struct {
		name            string
		format          string
		template        string
		expectedType    string
		validatePayload func(t *testing.T, body []byte)
	}{
		{
			name:         "JSON format",
			format:       "json",
			template:     `{"alert":"{{.Status}}"}`,
			expectedType: "application/json",
			validatePayload: func(t *testing.T, body []byte) {
				var data map[string]interface{}
				err := json.Unmarshal(body, &data)
				require.NoError(t, err)
				assert.Equal(t, "firing", data["alert"])
			},
		},
		{
			name:         "Form format",
			format:       "form",
			template:     `alert={{.Status}}&count={{len .Alerts}}`,
			expectedType: "application/x-www-form-urlencoded",
			validatePayload: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "alert=firing")
				assert.Contains(t, string(body), "count=1")
			},
		},
		{
			name:         "Query format",
			format:       "query",
			template:     `{"alert":"{{.Status}}","severity":"warning"}`,
			expectedType: "application/x-www-form-urlencoded",
			validatePayload: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "alert=firing")
				assert.Contains(t, string(body), "severity=warning")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			receivedRequests = nil // Reset

			cfg := &config.Config{
				Server: config.ServerConfig{
					Port: 8080,
				},
				Destinations: []config.DestinationConfig{
					{
						Name:     fmt.Sprintf("format-%s", tc.format),
						URL:      mockDestination.URL,
						Method:   "POST",
						Format:   tc.format,
						Engine:   "go-template",
						Template: tc.template,
						Enabled:  true,
					},
				},
			}

			logger := logrus.New()
			logger.SetLevel(logrus.ErrorLevel)
			srv, err := New(cfg, logger)
			require.NoError(t, err)

			router := srv.GetRouter()

			webhookPayload := &alertmanager.WebhookPayload{
				Version:  "4",
				GroupKey: fmt.Sprintf("format-test-%s", tc.format),
				Status:   "firing",
				Alerts: []alertmanager.Alert{
					{Status: "firing", Fingerprint: "test-123", StartsAt: time.Now()},
				},
			}
			payloadBytes, err := json.Marshal(webhookPayload)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", fmt.Sprintf("/webhook/format-%s", tc.format), bytes.NewReader(payloadBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)

			// Give async request time to complete
			time.Sleep(100 * time.Millisecond)

			require.Len(t, receivedRequests, 1)
			assert.Equal(t, tc.expectedType, receivedRequests[0].ContentType)
			tc.validatePayload(t, receivedRequests[0].Body)
		})
	}
}

// Helper struct to capture mock requests
type mockRequest struct {
	Method      string
	URL         string
	Headers     http.Header
	Body        []byte
	ContentType string
}