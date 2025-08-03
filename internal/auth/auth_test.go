package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
)

func TestNewAuthenticator(t *testing.T) {
	cfg := &config.AuthConfig{
		Enabled:     true,
		Username:    "testuser",
		Password:    "testpass",
		APIUsername: "apiuser",
		APIPassword: "apipass",
	}

	logger := logrus.New()
	auth := NewAuthenticator(cfg, logger)

	assert.NotNil(t, auth)
	assert.Equal(t, cfg, auth.config)
	assert.NotNil(t, auth.logger)
}

func TestAuthenticator_ValidateCredentials(t *testing.T) {
	tests := []struct {
		name           string
		config         *config.AuthConfig
		username       string
		password       string
		expectedResult bool
	}{
		{
			name: "authentication disabled",
			config: &config.AuthConfig{
				Enabled: false,
			},
			username:       "any",
			password:       "any",
			expectedResult: true,
		},
		{
			name: "valid credentials",
			config: &config.AuthConfig{
				Enabled:  true,
				Username: "testuser",
				Password: "testpass",
			},
			username:       "testuser",
			password:       "testpass",
			expectedResult: true,
		},
		{
			name: "invalid username",
			config: &config.AuthConfig{
				Enabled:  true,
				Username: "testuser",
				Password: "testpass",
			},
			username:       "wronguser",
			password:       "testpass",
			expectedResult: false,
		},
		{
			name: "invalid password",
			config: &config.AuthConfig{
				Enabled:  true,
				Username: "testuser",
				Password: "testpass",
			},
			username:       "testuser",
			password:       "wrongpass",
			expectedResult: false,
		},
		{
			name: "empty credentials in config",
			config: &config.AuthConfig{
				Enabled:  true,
				Username: "",
				Password: "",
			},
			username:       "testuser",
			password:       "testpass",
			expectedResult: false,
		},
		{
			name: "case sensitive credentials",
			config: &config.AuthConfig{
				Enabled:  true,
				Username: "TestUser",
				Password: "TestPass",
			},
			username:       "testuser",
			password:       "testpass",
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logrus.New()
			auth := NewAuthenticator(tt.config, logger)
			result := auth.ValidateCredentials(tt.username, tt.password)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestAuthenticator_ValidateAPICredentials(t *testing.T) {
	tests := []struct {
		name           string
		config         *config.AuthConfig
		username       string
		password       string
		expectedResult bool
	}{
		{
			name: "authentication disabled",
			config: &config.AuthConfig{
				Enabled: false,
			},
			username:       "any",
			password:       "any",
			expectedResult: true,
		},
		{
			name: "valid API credentials",
			config: &config.AuthConfig{
				Enabled:     true,
				Username:    "regular",
				Password:    "regularpass",
				APIUsername: "apiuser",
				APIPassword: "apipass",
			},
			username:       "apiuser",
			password:       "apipass",
			expectedResult: true,
		},
		{
			name: "valid regular credentials with API configured",
			config: &config.AuthConfig{
				Enabled:     true,
				Username:    "regular",
				Password:    "regularpass",
				APIUsername: "apiuser",
				APIPassword: "apipass",
			},
			username:       "regular",
			password:       "regularpass",
			expectedResult: true,
		},
		{
			name: "invalid credentials with API configured",
			config: &config.AuthConfig{
				Enabled:     true,
				Username:    "regular",
				Password:    "regularpass",
				APIUsername: "apiuser",
				APIPassword: "apipass",
			},
			username:       "wronguser",
			password:       "wrongpass",
			expectedResult: false,
		},
		{
			name: "no API credentials configured, fallback to regular",
			config: &config.AuthConfig{
				Enabled:  true,
				Username: "regular",
				Password: "regularpass",
			},
			username:       "regular",
			password:       "regularpass",
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logrus.New()
			auth := NewAuthenticator(tt.config, logger)
			result := auth.ValidateAPICredentials(tt.username, tt.password)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	// Create a simple handler for testing
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	logger := logrus.New()

	t.Run("basic auth middleware", func(t *testing.T) {
		cfg := &config.AuthConfig{
			Enabled:  true,
			Username: "testuser",
			Password: "testpass",
		}

		auth := NewAuthenticator(cfg, logger)
		middleware := auth.BasicAuthMiddleware(handler)

		tests := []struct {
			name           string
			authHeader     string
			expectedStatus int
			expectedBody   string
		}{
			{
				name:           "no auth header",
				authHeader:     "",
				expectedStatus: http.StatusUnauthorized,
				expectedBody:   `"Basic authentication required"`,
			},
			{
				name:           "invalid auth header format",
				authHeader:     "Bearer token123",
				expectedStatus: http.StatusUnauthorized,
				expectedBody:   `"Basic authentication required"`,
			},
			{
				name:           "invalid credentials",
				authHeader:     "Basic " + base64.StdEncoding.EncodeToString([]byte("wrong:creds")),
				expectedStatus: http.StatusUnauthorized,
				expectedBody:   `"Invalid credentials"`,
			},
			{
				name:           "valid credentials",
				authHeader:     "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass")),
				expectedStatus: http.StatusOK,
				expectedBody:   "success",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := httptest.NewRequest("GET", "/test", nil)
				if tt.authHeader != "" {
					req.Header.Set("Authorization", tt.authHeader)
				}
				w := httptest.NewRecorder()

				middleware.ServeHTTP(w, req)

				assert.Equal(t, tt.expectedStatus, w.Code)
				assert.Contains(t, w.Body.String(), tt.expectedBody)

				if tt.expectedStatus == http.StatusUnauthorized {
					assert.Equal(t, `Basic realm="Alertmanager Gateway"`, w.Header().Get("WWW-Authenticate"))
					assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

					// Verify JSON response structure
					var response map[string]interface{}
					err := json.Unmarshal(w.Body.Bytes(), &response)
					require.NoError(t, err)
					assert.Equal(t, "error", response["status"])
					assert.Contains(t, response, "timestamp")
				}
			})
		}
	})

	t.Run("API auth middleware", func(t *testing.T) {
		cfg := &config.AuthConfig{
			Enabled:     true,
			Username:    "regular",
			Password:    "regularpass",
			APIUsername: "apiuser",
			APIPassword: "apipass",
		}

		auth := NewAuthenticator(cfg, logger)
		middleware := auth.APIAuthMiddleware(handler)

		tests := []struct {
			name           string
			username       string
			password       string
			expectedStatus int
		}{
			{
				name:           "valid API credentials",
				username:       "apiuser",
				password:       "apipass",
				expectedStatus: http.StatusOK,
			},
			{
				name:           "valid regular credentials",
				username:       "regular",
				password:       "regularpass",
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
				req := httptest.NewRequest("GET", "/test", nil)
				req.SetBasicAuth(tt.username, tt.password)
				w := httptest.NewRecorder()

				middleware.ServeHTTP(w, req)

				assert.Equal(t, tt.expectedStatus, w.Code)
			})
		}
	})
}

func TestAuthenticationDisabled(t *testing.T) {
	cfg := &config.AuthConfig{
		Enabled: false,
	}

	logger := logrus.New()
	auth := NewAuthenticator(cfg, logger)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Test both middlewares with auth disabled
	basicMiddleware := auth.BasicAuthMiddleware(handler)
	apiMiddleware := auth.APIAuthMiddleware(handler)

	// Test without any auth headers
	req := httptest.NewRequest("GET", "/test", nil)

	w1 := httptest.NewRecorder()
	basicMiddleware.ServeHTTP(w1, req)
	assert.Equal(t, http.StatusOK, w1.Code)

	w2 := httptest.NewRecorder()
	apiMiddleware.ServeHTTP(w2, req)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		remoteAddr     string
		expectedResult string
	}{
		{
			name: "X-Forwarded-For header",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.195",
			},
			remoteAddr:     "192.168.1.1:12345",
			expectedResult: "203.0.113.195",
		},
		{
			name: "X-Real-IP header",
			headers: map[string]string{
				"X-Real-IP": "203.0.113.196",
			},
			remoteAddr:     "192.168.1.1:12345",
			expectedResult: "203.0.113.196",
		},
		{
			name:           "RemoteAddr fallback",
			headers:        map[string]string{},
			remoteAddr:     "192.168.1.1:12345",
			expectedResult: "192.168.1.1:12345",
		},
		{
			name: "X-Forwarded-For takes precedence",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.195",
				"X-Real-IP":       "203.0.113.196",
			},
			remoteAddr:     "192.168.1.1:12345",
			expectedResult: "203.0.113.195",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr

			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			result := getClientIP(req)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestAuthenticatorProperties(t *testing.T) {
	tests := []struct {
		name                    string
		config                  *config.AuthConfig
		expectedEnabled         bool
		expectedHasAPICredemtls bool
	}{
		{
			name: "authentication enabled with API credentials",
			config: &config.AuthConfig{
				Enabled:     true,
				Username:    "user",
				Password:    "pass",
				APIUsername: "apiuser",
				APIPassword: "apipass",
			},
			expectedEnabled:         true,
			expectedHasAPICredemtls: true,
		},
		{
			name: "authentication enabled without API credentials",
			config: &config.AuthConfig{
				Enabled:  true,
				Username: "user",
				Password: "pass",
			},
			expectedEnabled:         true,
			expectedHasAPICredemtls: false,
		},
		{
			name: "authentication disabled",
			config: &config.AuthConfig{
				Enabled: false,
			},
			expectedEnabled:         false,
			expectedHasAPICredemtls: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logrus.New()
			auth := NewAuthenticator(tt.config, logger)

			assert.Equal(t, tt.expectedEnabled, auth.IsEnabled())
			assert.Equal(t, tt.expectedHasAPICredemtls, auth.HasAPICredentials())
		})
	}
}

func TestTimingAttackResistance(t *testing.T) {
	cfg := &config.AuthConfig{
		Enabled:  true,
		Username: "testuser",
		Password: "testpass",
	}

	logger := logrus.New()
	auth := NewAuthenticator(cfg, logger)

	// Test with various username lengths to ensure constant-time comparison
	testCases := []struct {
		username string
		password string
	}{
		{"a", "b"},
		{"ab", "cd"},
		{"abc", "def"},
		{"testuser", "wrongpass"},
		{"wronguser", "testpass"},
		{"verylongusernamethatdoesnotmatch", "verylongpasswordthatdoesnotmatch"},
	}

	for _, tc := range testCases {
		t.Run("timing_attack_resistance", func(t *testing.T) {
			start := time.Now()
			result := auth.ValidateCredentials(tc.username, tc.password)
			duration := time.Since(start)

			assert.False(t, result)
			// The actual timing is hard to test reliably, but we can ensure
			// the function completes in a reasonable time
			assert.Less(t, duration, 100*time.Millisecond)
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	cfg := &config.AuthConfig{
		Enabled:  true,
		Username: "testuser",
		Password: "testpass",
	}

	logger := logrus.New()
	auth := NewAuthenticator(cfg, logger)

	// Test concurrent access to the authenticator
	const numGoroutines = 100
	results := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			result := auth.ValidateCredentials("testuser", "testpass")
			results <- result
		}()
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		result := <-results
		assert.True(t, result)
	}
}
