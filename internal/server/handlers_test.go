package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
)

func TestRegisterAPIRoutes(t *testing.T) {
	cfg := &config.Config{Destinations: []config.DestinationConfig{}}
	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	router := mux.NewRouter()
	server.RegisterAPIRoutes(router)

	// Test that routes are registered by checking route match
	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/destinations"},
		{"GET", "/destinations/test"},
		{"POST", "/test/test"},
		{"POST", "/emulate/test"},
		{"GET", "/info"},
		{"GET", "/health"},
		{"POST", "/config/validate"},
	}

	for _, route := range routes {
		req := httptest.NewRequest(route.method, route.path, nil)
		match := &mux.RouteMatch{}
		assert.True(t, router.Match(req, match), "Route %s %s should be registered", route.method, route.path)
	}
}

func TestHandleListDestinations(t *testing.T) {
	cfg := &config.Config{
		Destinations: []config.DestinationConfig{
			{
				Name:     "test1",
				Method:   "POST",
				URL:      "http://example.com",
				Format:   "json",
				Engine:   "go-template",
				Template: "{{.Status}}",
				Enabled:  true,
			},
			{
				Name:      "test2",
				Method:    "POST",
				URL:       "http://example.com",
				Format:    "form",
				Engine:    "jq",
				Transform: ".status",
				Enabled:   false,
			},
			{
				Name:     "test3",
				Method:   "PUT",
				URL:      "http://example.com",
				Format:   "json",
				Engine:   "go-template",
				Template: "{{.Status}}",
				Enabled:  true,
			},
		},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	t.Run("list enabled destinations only", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/destinations", nil)
		w := httptest.NewRecorder()

		server.handleListDestinations(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var response ListDestinationsResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, 2, response.Total)
		assert.Len(t, response.Destinations, 2)
		assert.Equal(t, "test1", response.Destinations[0].Name)
		assert.Equal(t, "test3", response.Destinations[1].Name)
		assert.True(t, response.Destinations[0].Enabled)
		assert.True(t, response.Destinations[1].Enabled)
	})

	t.Run("list all destinations including disabled", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/destinations?include_disabled=true", nil)
		w := httptest.NewRecorder()

		server.handleListDestinations(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response ListDestinationsResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, 3, response.Total)
		assert.Len(t, response.Destinations, 3)
	})

	t.Run("empty destinations list", func(t *testing.T) {
		emptyCfg := &config.Config{Destinations: []config.DestinationConfig{}}
		emptyServer, err := New(emptyCfg, logrus.New())
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/destinations", nil)
		w := httptest.NewRecorder()

		emptyServer.handleListDestinations(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response ListDestinationsResponse
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, 0, response.Total)
		assert.Len(t, response.Destinations, 0)
	})
}

func TestHandleGetDestination(t *testing.T) {
	cfg := &config.Config{
		Destinations: []config.DestinationConfig{
			{
				Name:     "test-destination",
				Method:   "POST",
				URL:      "https://api.example.com/webhook",
				Format:   "json",
				Engine:   "go-template",
				Template: "{{.Status}}",
				Enabled:  true,
				Headers: map[string]string{
					"Authorization": "Bearer secret-token",
					"Content-Type":  "application/json",
				},
				SplitAlerts:      true,
				BatchSize:        5,
				ParallelRequests: 3,
			},
		},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	t.Run("get existing destination", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/destinations/test-destination", nil)
		req = mux.SetURLVars(req, map[string]string{"name": "test-destination"})
		w := httptest.NewRecorder()

		server.handleGetDestination(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var response DestinationDetails
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "test-destination", response.Name)
		assert.Equal(t, "/webhook/test-destination", response.WebhookURL)
		assert.Equal(t, "POST", response.Method)
		assert.Contains(t, response.TargetURL, "api.exa") // URL should be masked
		assert.Equal(t, "json", response.Format)
		assert.Equal(t, "go-template", response.Engine)
		assert.True(t, response.Enabled)
		assert.True(t, response.SplitAlerts)
		assert.Equal(t, 5, response.BatchSize)
		assert.Equal(t, 3, response.ParallelRequests)
		assert.Equal(t, 11, response.TemplateSize) // "{{.Status}}" length
		assert.True(t, response.HasTemplate)
		// Check that sensitive headers are masked
		assert.Equal(t, "***", response.Headers["Authorization"])
		assert.Equal(t, "application/json", response.Headers["Content-Type"])
	})

	t.Run("get non-existing destination", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/destinations/nonexistent", nil)
		req = mux.SetURLVars(req, map[string]string{"name": "nonexistent"})
		w := httptest.NewRecorder()

		server.handleGetDestination(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "error", response.Status)
		assert.Equal(t, "Destination not found", response.Error)
	})
}

func TestHandleTestDestination(t *testing.T) {
	cfg := &config.Config{
		Destinations: []config.DestinationConfig{
			{
				Name:     "test-destination",
				Method:   "POST",
				URL:      "https://httpbin.org/post",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"status": "{{.Status}}", "count": {{len .Alerts}}}`,
				Enabled:  true,
			},
			{
				Name:    "disabled-destination",
				Enabled: false,
			},
		},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	t.Run("test enabled destination with sample data", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test/test-destination", bytes.NewBufferString("{}"))
		req = mux.SetURLVars(req, map[string]string{"destination": "test-destination"})
		w := httptest.NewRecorder()

		server.handleTestDestination(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response TestResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.True(t, response.Success)
		assert.Equal(t, "test-destination", response.Destination)
		assert.NotNil(t, response.Result)
		assert.Contains(t, response.Result.FormattedOutput, "firing") // Sample data status
		assert.Equal(t, "json", response.Result.OutputFormat)
		assert.Greater(t, response.Result.OutputSize, 0)
		assert.Equal(t, 1, response.Result.AlertsProcessed)
	})

	t.Run("test with custom webhook data", func(t *testing.T) {
		customWebhookData := &alertmanager.WebhookPayload{
			Status: "resolved",
			Alerts: []alertmanager.Alert{
				{Status: "resolved"},
				{Status: "resolved"},
			},
		}

		testReq := TestRequest{
			WebhookData: customWebhookData,
		}

		reqBody, err := json.Marshal(testReq)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/test/test-destination", bytes.NewBuffer(reqBody))
		req = mux.SetURLVars(req, map[string]string{"destination": "test-destination"})
		w := httptest.NewRecorder()

		server.handleTestDestination(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response TestResponse
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.True(t, response.Success)
		assert.Contains(t, response.Result.FormattedOutput, "resolved")
		assert.Equal(t, 2, response.Result.AlertsProcessed)
	})

	t.Run("test non-existing destination", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test/nonexistent", bytes.NewBufferString("{}"))
		req = mux.SetURLVars(req, map[string]string{"destination": "nonexistent"})
		w := httptest.NewRecorder()

		server.handleTestDestination(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "error", response.Status)
		assert.Equal(t, "Destination not found", response.Error)
	})

	t.Run("test disabled destination", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test/disabled-destination", bytes.NewBufferString("{}"))
		req = mux.SetURLVars(req, map[string]string{"destination": "disabled-destination"})
		w := httptest.NewRecorder()

		server.handleTestDestination(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "error", response.Status)
		assert.Equal(t, "Destination is disabled", response.Error)
	})

	t.Run("test with invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test/test-destination", bytes.NewBufferString("invalid json"))
		req = mux.SetURLVars(req, map[string]string{"destination": "test-destination"})
		w := httptest.NewRecorder()

		server.handleTestDestination(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "error", response.Status)
		assert.Equal(t, "Invalid JSON payload", response.Error)
	})
}

func TestHandleEmulateDestination(t *testing.T) {
	cfg := &config.Config{
		Destinations: []config.DestinationConfig{
			{
				Name:     "test-destination",
				Method:   "POST",
				URL:      "https://httpbin.org/post",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"message": "Alert {{.Status}}"}`,
				Enabled:  true,
			},
		},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	t.Run("dry run emulation", func(t *testing.T) {
		emulateReq := EmulateRequest{
			DryRun: true,
		}

		reqBody, err := json.Marshal(emulateReq)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/emulate/test-destination", bytes.NewBuffer(reqBody))
		req = mux.SetURLVars(req, map[string]string{"destination": "test-destination"})
		w := httptest.NewRecorder()

		server.handleEmulateDestination(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response EmulateResponse
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.True(t, response.Success)
		assert.True(t, response.DryRun)
		assert.Equal(t, "test-destination", response.Destination)
		assert.NotNil(t, response.Result)
		assert.Equal(t, "POST", response.Result.HTTPMethod)
		assert.Equal(t, "dry-run", response.Result.HTTPStatus)
		assert.Equal(t, 0, response.Result.HTTPStatusCode)
		assert.True(t, response.Result.Success)
	})

	t.Run("live emulation", func(t *testing.T) {
		emulateReq := EmulateRequest{
			DryRun: false,
		}

		reqBody, err := json.Marshal(emulateReq)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/emulate/test-destination", bytes.NewBuffer(reqBody))
		req = mux.SetURLVars(req, map[string]string{"destination": "test-destination"})
		w := httptest.NewRecorder()

		server.handleEmulateDestination(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response EmulateResponse
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "test-destination", response.Destination)
		assert.False(t, response.DryRun)
		assert.NotNil(t, response.Result)
		assert.Equal(t, "POST", response.Result.HTTPMethod)

		// Note: Since we're calling httpbin.org, the test might fail in some environments
		// In a real scenario, you might want to mock the HTTP client
		if response.Result.HTTPError != "" {
			t.Logf("HTTP request failed (expected in some test environments): %s", response.Result.HTTPError)
		} else {
			assert.Equal(t, 200, response.Result.HTTPStatusCode)
			assert.True(t, response.Result.Success)
		}
	})

	t.Run("emulate non-existing destination", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/emulate/nonexistent", bytes.NewBufferString("{}"))
		req = mux.SetURLVars(req, map[string]string{"destination": "nonexistent"})
		w := httptest.NewRecorder()

		server.handleEmulateDestination(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "error", response.Status)
		assert.Equal(t, "Destination not found", response.Error)
	})
}

func TestHandleSystemInfo(t *testing.T) {
	cfg := &config.Config{
		Destinations: []config.DestinationConfig{
			{Name: "dest1", URL: "http://example.com", Engine: "go-template", Template: "{{.Status}}", Enabled: true},
			{Name: "dest2", URL: "http://example.com", Engine: "go-template", Template: "{{.Status}}", Enabled: false},
			{Name: "dest3", URL: "http://example.com", Engine: "go-template", Template: "{{.Status}}", Enabled: true},
		},
		Server: config.ServerConfig{
			Address: ":8080",
			Auth: config.AuthConfig{
				Enabled: true,
			},
		},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/info", nil)
	w := httptest.NewRecorder()

	server.handleSystemInfo(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response SystemInfo
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "1.0.0", response.Version)
	assert.Greater(t, response.NumCPU, 0)
	assert.Greater(t, response.NumGoroutines, 0)
	assert.Greater(t, response.MemoryAlloc, uint64(0))
	assert.Equal(t, 3, response.Config.DestinationsCount)
	assert.Equal(t, 2, response.Config.EnabledDestinationsCount)
	assert.Equal(t, ":8080", response.Config.ServerAddress)
	assert.True(t, response.Config.AuthEnabled)
}

func TestHandleAPIHealth(t *testing.T) {
	cfg := &config.Config{
		Destinations: []config.DestinationConfig{
			{Name: "dest1", URL: "http://example.com", Engine: "go-template", Template: "{{.Status}}", Enabled: true},
			{Name: "dest2", URL: "http://example.com", Engine: "go-template", Template: "{{.Status}}", Enabled: true},
		},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.handleAPIHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response HealthResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.True(t, response.ConfigLoaded)
	assert.Equal(t, 2, response.DestinationsCount)
	assert.Equal(t, 2, response.EnabledDestinations)
	assert.Greater(t, response.UptimeSeconds, 0.0)
	assert.Len(t, response.Checks, 2) // destinations and memory checks
}

func TestHandleAPIHealthWithNoEnabledDestinations(t *testing.T) {
	cfg := &config.Config{
		Destinations: []config.DestinationConfig{
			{Name: "dest1", URL: "http://example.com", Engine: "go-template", Template: "{{.Status}}", Enabled: false},
		},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.handleAPIHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response HealthResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, 0, response.EnabledDestinations)
	assert.Len(t, response.Checks, 3) // destinations, memory, and enabled_destinations warning

	// Check for the warning about no enabled destinations
	hasWarning := false
	for _, check := range response.Checks {
		if check.Name == "enabled_destinations" && check.Status == "warning" {
			hasWarning = true
			break
		}
	}
	assert.True(t, hasWarning)
}

func TestHandleValidateConfig(t *testing.T) {
	cfg := &config.Config{Destinations: []config.DestinationConfig{}}
	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	configData := map[string]interface{}{
		"server": map[string]interface{}{
			"port": 8080,
		},
	}

	reqBody, err := json.Marshal(configData)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/config/validate", bytes.NewBuffer(reqBody))
	w := httptest.NewRecorder()

	server.handleValidateConfig(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response ConfigValidation
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.True(t, response.Valid)
	assert.Len(t, response.Errors, 0)
	assert.Len(t, response.Warnings, 1) // Not yet fully implemented warning
}

func TestUtilityFunctions(t *testing.T) {
	t.Run("maskSensitiveURL", func(t *testing.T) {
		tests := []struct {
			input    string
			expected string
		}{
			{"https://api.example.com/webhook", "https://api.exa***"},
			{"https://very-long-url-that-should-be-masked.example.com/webhook/path", "https://very-long-url-tha***.com/webhook/path"},
			{"short-url", "short-url"},
			{"", ""},
		}

		for _, test := range tests {
			result := maskSensitiveURL(test.input)
			assert.Equal(t, test.expected, result, "Input: %s", test.input)
		}
	})

	t.Run("maskSensitiveHeaders", func(t *testing.T) {
		headers := map[string]string{
			"Authorization": "Bearer secret-token",
			"Content-Type":  "application/json",
			"X-API-Key":     "secret-api-key",
			"User-Agent":    "AlertManager-Gateway/1.0",
			"password":      "secret-password",
		}

		masked := maskSensitiveHeaders(headers)

		assert.Equal(t, "***", masked["Authorization"])
		assert.Equal(t, "application/json", masked["Content-Type"])
		assert.Equal(t, "***", masked["X-API-Key"])
		assert.Equal(t, "AlertManager-Gateway/1.0", masked["User-Agent"])
		assert.Equal(t, "***", masked["password"])
	})

	t.Run("isSensitiveHeader", func(t *testing.T) {
		tests := []struct {
			header    string
			sensitive bool
		}{
			{"Authorization", true},
			{"authorization", true},
			{"AUTHORIZATION", true},
			{"X-API-Key", true},
			{"x-api-key", true},
			{"password", true},
			{"Password", true},
			{"Content-Type", false},
			{"User-Agent", false},
			{"Accept", false},
		}

		for _, test := range tests {
			result := isSensitiveHeader(test.header)
			assert.Equal(t, test.sensitive, result, "Header: %s", test.header)
		}
	})

	t.Run("generateDestinationDescription", func(t *testing.T) {
		dest1 := &config.DestinationConfig{Engine: "go-template", Format: "json"}
		desc1 := generateDestinationDescription(dest1)
		assert.Equal(t, "json destination using Go templates", desc1)

		dest2 := &config.DestinationConfig{Engine: "jq", Format: "form"}
		desc2 := generateDestinationDescription(dest2)
		assert.Equal(t, "form destination using jq transformations", desc2)
	})
}

func TestGetSampleWebhookData(t *testing.T) {
	sampleData := getSampleWebhookData()

	assert.NotNil(t, sampleData)
	assert.Equal(t, "4", sampleData.Version)
	assert.Equal(t, "firing", sampleData.Status)
	assert.Equal(t, "test-receiver", sampleData.Receiver)
	assert.Len(t, sampleData.Alerts, 1)
	assert.Equal(t, "ExampleAlert", sampleData.Alerts[0].Labels["alertname"])
	assert.Equal(t, "warning", sampleData.Alerts[0].Labels["severity"])
	assert.Equal(t, "firing", sampleData.Alerts[0].Status)
}

func TestCountEnabledDestinations(t *testing.T) {
	cfg := &config.Config{
		Destinations: []config.DestinationConfig{
			{Name: "dest1", URL: "http://example.com", Engine: "go-template", Template: "{{.Status}}", Enabled: true},
			{Name: "dest2", URL: "http://example.com", Engine: "go-template", Template: "{{.Status}}", Enabled: false},
			{Name: "dest3", URL: "http://example.com", Engine: "go-template", Template: "{{.Status}}", Enabled: true},
			{Name: "dest4", URL: "http://example.com", Engine: "go-template", Template: "{{.Status}}", Enabled: false},
		},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	count := server.countEnabledDestinations()
	assert.Equal(t, 2, count)
}

func TestJSONResponseHelpers(t *testing.T) {
	cfg := &config.Config{Destinations: []config.DestinationConfig{}}
	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	t.Run("sendJSON", func(t *testing.T) {
		w := httptest.NewRecorder()
		data := map[string]string{"test": "value"}

		server.sendJSON(w, http.StatusOK, data)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "value", response["test"])
	})

	t.Run("sendAPIError", func(t *testing.T) {
		w := httptest.NewRecorder()

		server.sendAPIError(w, http.StatusBadRequest, "Test error message")

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var response ErrorResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "error", response.Status)
		assert.Equal(t, "Test error message", response.Error)
		assert.NotEmpty(t, response.RequestID)
	})
}
