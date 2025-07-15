package destination

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
)

func TestNewHTTPHandler(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.DestinationConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "destination config is required",
		},
		{
			name: "go-template without template",
			config: &config.DestinationConfig{
				Name:   "test",
				Engine: "go-template",
			},
			wantErr: true,
			errMsg:  "template is required for go-template engine",
		},
		{
			name: "valid jq config",
			config: &config.DestinationConfig{
				Name:      "test",
				URL:       "https://example.com/webhook",
				Method:    "POST",
				Format:    "json",
				Engine:    "jq",
				Transform: ".status",
			},
			wantErr: false,
		},
		{
			name: "jq without transform",
			config: &config.DestinationConfig{
				Name:   "test",
				Engine: "jq",
			},
			wantErr: true,
			errMsg:  "transform is required for jq engine",
		},
		{
			name: "unknown engine",
			config: &config.DestinationConfig{
				Name:   "test",
				Engine: "unknown",
			},
			wantErr: true,
			errMsg:  "unknown engine type",
		},
		{
			name: "valid go-template config",
			config: &config.DestinationConfig{
				Name:     "test",
				URL:      "https://example.com/webhook",
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"message": "{{ .Status }}"}`,
			},
			wantErr: false,
		},
		{
			name: "invalid template",
			config: &config.DestinationConfig{
				Name:     "test",
				Engine:   "go-template",
				Template: `{{ .Status }`,
			},
			wantErr: true,
			errMsg:  "failed to create transform engine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewHTTPHandler(tt.config, nil)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, handler)
			} else {
				require.NoError(t, err)
				require.NotNil(t, handler)
				assert.Equal(t, tt.config.Name, handler.Name())
			}
		})
	}
}

func TestHTTPHandler_Send(t *testing.T) {
	// Create test server
	var receivedBody []byte
	var receivedHeaders http.Header
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		receivedHeaders = r.Header
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	// Create handler
	cfg := &config.DestinationConfig{
		Name:     "test-destination",
		URL:      server.URL,
		Method:   "POST",
		Format:   "json",
		Engine:   "go-template",
		Template: `{"status": "{{ .Status }}", "alerts": {{ len .Alerts }}}`,
		Headers: map[string]string{
			"X-Custom-Header": "test-value",
		},
	}

	handler, err := NewHTTPHandler(cfg, nil)
	require.NoError(t, err)
	defer handler.Close()

	// Create test payload
	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		Receiver: "test",
		Alerts: []alertmanager.Alert{
			{
				Status:      "firing",
				Fingerprint: "abc123",
				StartsAt:    time.Now(),
				Labels: map[string]string{
					"alertname": "TestAlert",
				},
			},
		},
	}

	// Send alert
	err = handler.Send(context.Background(), payload)
	require.NoError(t, err)

	// Verify request
	assert.Equal(t, int32(1), requestCount.Load())
	assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))
	assert.Equal(t, "test-value", receivedHeaders.Get("X-Custom-Header"))

	// Verify body
	var data map[string]interface{}
	err = json.Unmarshal(receivedBody, &data)
	require.NoError(t, err)
	assert.Equal(t, "firing", data["status"])
	assert.Equal(t, float64(1), data["alerts"])
}

func TestHTTPHandler_SendSplit(t *testing.T) {
	// Create test server
	var requestCount atomic.Int32
	var receivedBodies []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create handler with split mode
	cfg := &config.DestinationConfig{
		Name:        "test-split",
		URL:         server.URL,
		Method:      "POST",
		Format:      "json",
		Engine:      "go-template",
		Template:    `{{ range .Alerts }}{"fingerprint": "{{ .Fingerprint }}", "alertname": "{{ .Labels.alertname }}"}{{ end }}`,
		SplitAlerts: true,
	}

	handler, err := NewHTTPHandler(cfg, nil)
	require.NoError(t, err)
	defer handler.Close()

	// Create test payload with multiple alerts
	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		Alerts: []alertmanager.Alert{
			{
				Status:      "firing",
				Fingerprint: "alert1",
				Labels:      map[string]string{"alertname": "Alert1"},
				StartsAt:    time.Now(),
			},
			{
				Status:      "firing",
				Fingerprint: "alert2",
				Labels:      map[string]string{"alertname": "Alert2"},
				StartsAt:    time.Now(),
			},
		},
	}

	// Send alerts
	err = handler.Send(context.Background(), payload)
	require.NoError(t, err)

	// Verify requests
	assert.Equal(t, int32(2), requestCount.Load())
	assert.Len(t, receivedBodies, 2)

	// Verify each alert was sent separately
	assert.Contains(t, receivedBodies[0], `"fingerprint":"alert1"`)
	assert.Contains(t, receivedBodies[0], `"alertname":"Alert1"`)
	assert.Contains(t, receivedBodies[1], `"fingerprint":"alert2"`)
	assert.Contains(t, receivedBodies[1], `"alertname":"Alert2"`)
}

func TestHTTPHandler_SendError(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	cfg := &config.DestinationConfig{
		Name:     "test-error",
		URL:      server.URL,
		Method:   "POST",
		Format:   "json",
		Engine:   "go-template",
		Template: `{"status": "{{ .Status }}"}`,
	}

	handler, err := NewHTTPHandler(cfg, nil)
	require.NoError(t, err)
	defer handler.Close()

	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		Alerts:   []alertmanager.Alert{},
	}

	// Send should fail
	err = handler.Send(context.Background(), payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "destination returned error")
	assert.Contains(t, err.Error(), "500")
}

func TestHTTPHandler_SendWithQueryParams(t *testing.T) {
	// Create test server
	var receivedURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// This would normally use query format, but for now we'll test with JSON
	cfg := &config.DestinationConfig{
		Name:     "test-query",
		URL:      server.URL + "/webhook",
		Method:   "GET",
		Format:   "json",
		Engine:   "go-template",
		Template: `{"status": "{{ .Status }}"}`,
	}

	handler, err := NewHTTPHandler(cfg, nil)
	require.NoError(t, err)
	defer handler.Close()

	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		Alerts:   []alertmanager.Alert{},
	}

	err = handler.Send(context.Background(), payload)
	require.NoError(t, err)

	assert.Equal(t, "/webhook", receivedURL)
}

func TestHTTPHandler_TransformError(t *testing.T) {
	cfg := &config.DestinationConfig{
		Name:     "test-transform-error",
		URL:      "https://example.com",
		Method:   "POST",
		Format:   "json",
		Engine:   "go-template",
		Template: `{{ .InvalidField }}`,
	}

	handler, err := NewHTTPHandler(cfg, nil)
	require.NoError(t, err)
	defer handler.Close()

	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		Alerts:   []alertmanager.Alert{},
	}

	err = handler.Send(context.Background(), payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to transform payload")
}
