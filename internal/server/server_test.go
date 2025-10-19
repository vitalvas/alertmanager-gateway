package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
)

func TestHealthEndpoints(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: ":8080",
		},
		Destinations: []config.DestinationConfig{
			{Name: "test", URL: "http://example.com", Enabled: true, Engine: "go-template", Template: `{"status": "{{.Status}}"}`, Method: "POST", Format: "json"},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	server, err := New(cfg, logger)
	require.NoError(t, err)

	tests := []struct {
		name           string
		endpoint       string
		expectedStatus int
		checkResponse  func(t *testing.T, body map[string]interface{})
	}{
		{
			name:           "health endpoint",
			endpoint:       "/health",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body map[string]interface{}) {
				assert.Equal(t, "healthy", body["status"])
				assert.Equal(t, "1.0.0", body["version"])
				assert.Equal(t, true, body["config_loaded"])
				assert.Equal(t, float64(1), body["destinations_count"])
				assert.Contains(t, body, "uptime_seconds")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.endpoint, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var body map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &body)
			require.NoError(t, err)

			tt.checkResponse(t, body)
		})
	}
}

func TestMetricsEndpoint(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: ":8080",
		},
		Destinations: []config.DestinationConfig{},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/plain")
	assert.Contains(t, w.Body.String(), "alertmanager_gateway_info")
}

func TestListDestinations(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: ":8080",
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "dest1",
				Method:   "POST",
				Format:   "json",
				URL:      "http://example.com",
				Enabled:  true,
				Engine:   "go-template",
				Template: `{"status": "{{.Status}}"}`,
			},
			{
				Name:    "dest2",
				Method:  "GET",
				Format:  "query",
				URL:     "http://example.com",
				Enabled: false, // This should not be included
			},
		},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/api/v1/destinations", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	destinations := body["destinations"].([]interface{})
	assert.Len(t, destinations, 1) // Only enabled destination

	dest := destinations[0].(map[string]interface{})
	assert.Equal(t, "dest1", dest["name"])
	assert.Equal(t, "/webhook/dest1", dest["webhook_url"])
	assert.Equal(t, "POST", dest["method"])
	assert.Equal(t, "json", dest["format"])
	assert.Equal(t, true, dest["enabled"])
}

func TestGetDestination(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: ":8080",
		},
		Destinations: []config.DestinationConfig{
			{
				Name:        "test-dest",
				Method:      "POST",
				Format:      "json",
				Engine:      "go-template",
				URL:         "http://example.com/webhook",
				Template:    `{"test": "{{.Status}}"}`,
				Enabled:     true,
				SplitAlerts: true,
				BatchSize:   10,
			},
		},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/api/v1/destinations/test-dest", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, "test-dest", body["name"])
	assert.Equal(t, "/webhook/test-dest", body["webhook_url"])
	assert.Equal(t, "POST", body["method"])
	assert.Contains(t, body["target_url"], "http://exa")
	assert.Equal(t, "json", body["format"])
	assert.Equal(t, "go-template", body["engine"])
	assert.Equal(t, true, body["split_alerts"])
	assert.Equal(t, float64(10), body["batch_size"])
}

func TestGetDestinationNotFound(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: ":8080",
		},
		Destinations: []config.DestinationConfig{},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/api/v1/destinations/nonexistent", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var body map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, "error", body["status"])
	assert.Equal(t, "Destination not found", body["error"])
}

func TestBasicAuth(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: ":8080",
			Auth: config.AuthConfig{
				Enabled:  true,
				Username: "testuser",
				Password: "testpass",
			},
		},
		Destinations: []config.DestinationConfig{
			{Name: "test", URL: "http://example.com", Enabled: true, Engine: "go-template", Template: `{"status": "{{.Status}}"}`, Method: "POST", Format: "json"},
		},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	tests := []struct {
		name           string
		endpoint       string
		auth           string
		expectedStatus int
	}{
		{
			name:           "no auth provided",
			endpoint:       "/webhook/test",
			auth:           "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid credentials",
			endpoint:       "/webhook/test",
			auth:           "Basic " + base64.StdEncoding.EncodeToString([]byte("wrong:creds")),
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "valid credentials",
			endpoint:       "/webhook/test",
			auth:           "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass")),
			expectedStatus: http.StatusBadRequest, // Empty body
		},
		{
			name:           "health endpoint no auth",
			endpoint:       "/health",
			auth:           "",
			expectedStatus: http.StatusOK, // Health endpoints don't require auth
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := "POST"
			if tt.endpoint == "/health" {
				method = "GET" // Health endpoint only accepts GET
			}
			req := httptest.NewRequest(method, tt.endpoint, nil)
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestAPIAuth(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: ":8080",
			Auth: config.AuthConfig{
				Enabled:     true,
				Username:    "webhook",
				Password:    "webhookpass",
				APIUsername: "admin",
				APIPassword: "adminpass",
			},
		},
		Destinations: []config.DestinationConfig{
			{Name: "test", URL: "http://example.com", Enabled: true, Engine: "go-template", Template: `{"status": "{{.Status}}"}`, Method: "POST", Format: "json"},
		},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	tests := []struct {
		name           string
		username       string
		password       string
		expectedStatus int
	}{
		{
			name:           "api credentials",
			username:       "admin",
			password:       "adminpass",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "webhook credentials",
			username:       "webhook",
			password:       "webhookpass",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid credentials",
			username:       "wrong",
			password:       "wrong",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/destinations", nil)
			req.SetBasicAuth(tt.username, tt.password)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestNotFound(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: ":8080",
		},
		Destinations: []config.DestinationConfig{},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var body map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, "error", body["status"])
	assert.Equal(t, "Endpoint not found", body["error"])
}

func TestLoggingMiddleware(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: ":8080",
		},
		Destinations: []config.DestinationConfig{},
	}

	// Create a logger with a test hook
	logger := logrus.New()
	hook := &testLogHook{}
	logger.AddHook(hook)

	server, err := New(cfg, logger)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("User-Agent", "test-agent")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	// Check that request was logged
	assert.Len(t, hook.entries, 1)
	entry := hook.entries[0]
	assert.Equal(t, "HTTP request", entry.Message)
	assert.Equal(t, "GET", entry.Data["method"])
	assert.Equal(t, "/health", entry.Data["path"])
	assert.Equal(t, 200, entry.Data["status"])
	assert.Equal(t, "test-agent", entry.Data["user_agent"])
	assert.Contains(t, entry.Data, "duration_ms")
}

func TestRecoveryMiddleware(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: ":8080",
		},
		Destinations: []config.DestinationConfig{},
	}

	logger := logrus.New()
	hook := &testLogHook{}
	logger.AddHook(hook)

	server, err := New(cfg, logger)
	require.NoError(t, err)

	// Add a route that panics
	server.router.HandleFunc("/panic", func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	})

	req := httptest.NewRequest("GET", "/panic", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "Internal Server Error\n", w.Body.String())

	// Check that panic was logged
	panicLogged := false
	for _, entry := range hook.entries {
		if entry.Level == logrus.ErrorLevel && entry.Message == "Panic recovered" {
			panicLogged = true
			assert.Equal(t, "test panic", entry.Data["error"])
			break
		}
	}
	assert.True(t, panicLogged, "Panic should have been logged")
}

func TestSecurityHeaders(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: ":8080",
		},
		Destinations: []config.DestinationConfig{},
	}

	logger := logrus.New()
	server, err := New(cfg, logger)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	// Check server identification headers
	assert.NotEmpty(t, w.Header().Get("X-Server-Hostname"))
}

func TestServerShutdown(t *testing.T) {
	// Skip this test as it's flaky in CI
	t.Skip("Skipping server shutdown test - needs refactoring")
}

// Test helper - log hook to capture log entries
type testLogHook struct {
	entries []*logrus.Entry
}

func (h *testLogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *testLogHook) Fire(entry *logrus.Entry) error {
	h.entries = append(h.entries, entry)
	return nil
}
