package webhook

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestValidationMiddleware(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create a test handler that always returns OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with validation middleware
	middleware := ValidationMiddleware(logger)
	handler := middleware(testHandler)

	tests := []struct {
		name           string
		method         string
		contentType    string
		body           string
		userAgent      string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "valid POST request",
			method:         "POST",
			contentType:    "application/json",
			body:           `{"test": "data"}`,
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
		},
		{
			name:           "POST without content type",
			method:         "POST",
			contentType:    "",
			body:           `{"test": "data"}`,
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
		},
		{
			name:           "POST with Alertmanager user agent",
			method:         "POST",
			contentType:    "application/json",
			body:           `{"test": "data"}`,
			userAgent:      "Alertmanager/0.25.0",
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
		},
		{
			name:           "GET request not allowed",
			method:         "GET",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   "Method not allowed\n",
		},
		{
			name:           "PUT request not allowed",
			method:         "PUT",
			body:           `{"test": "data"}`,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   "Method not allowed\n",
		},
		{
			name:           "empty body",
			method:         "POST",
			contentType:    "application/json",
			body:           "",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Empty request body\n",
		},
		{
			name:           "non-JSON content type warning",
			method:         "POST",
			contentType:    "text/plain",
			body:           "plain text",
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, "/webhook/test", strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, "/webhook/test", nil)
			}
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.userAgent != "" {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			// Create response recorder
			w := httptest.NewRecorder()

			// Handle request
			handler.ServeHTTP(w, req)

			// Check response
			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Equal(t, tt.expectedBody, w.Body.String())
		})
	}
}

func TestValidationMiddleware_ContentLength(t *testing.T) {
	logger := logrus.New()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := ValidationMiddleware(logger)
	handler := middleware(testHandler)

	// Test with Content-Length: 0
	req := httptest.NewRequest("POST", "/webhook/test", nil)
	req.ContentLength = 0

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "Empty request body\n", w.Body.String())
}
