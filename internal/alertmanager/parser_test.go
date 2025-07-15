package alertmanager

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWebhookPayload(t *testing.T) {
	validPayload := WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		Receiver: "test",
		GroupLabels: map[string]string{
			"alertname": "test",
		},
		CommonLabels: map[string]string{
			"alertname": "test",
			"severity":  "warning",
		},
		CommonAnnotations: map[string]string{
			"summary": "Test alert",
		},
		ExternalURL: "http://alertmanager.example.com",
		Alerts: []Alert{
			{
				Status:      "firing",
				Fingerprint: "abc123",
				StartsAt:    time.Now(),
				Labels: map[string]string{
					"alertname": "test",
					"instance":  "server1",
				},
				Annotations: map[string]string{
					"summary": "Test alert",
				},
				GeneratorURL: "http://prometheus.example.com",
			},
		},
	}

	validJSON, err := json.Marshal(validPayload)
	require.NoError(t, err)

	tests := []struct {
		name        string
		request     func() *http.Request
		wantErr     bool
		errContains string
	}{
		{
			name: "valid payload",
			request: func() *http.Request {
				req := httptest.NewRequest("POST", "/webhook/test", bytes.NewReader(validJSON))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			wantErr: false,
		},
		{
			name: "valid payload without content type",
			request: func() *http.Request {
				return httptest.NewRequest("POST", "/webhook/test", bytes.NewReader(validJSON))
			},
			wantErr: false,
		},
		{
			name: "invalid JSON",
			request: func() *http.Request {
				req := httptest.NewRequest("POST", "/webhook/test", strings.NewReader("{invalid}"))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			wantErr:     true,
			errContains: "invalid JSON",
		},
		{
			name: "empty body",
			request: func() *http.Request {
				return httptest.NewRequest("POST", "/webhook/test", strings.NewReader(""))
			},
			wantErr:     true,
			errContains: "invalid JSON",
		},
		{
			name: "payload too large",
			request: func() *http.Request {
				// Create a payload larger than MaxPayloadSize
				largeData := make([]byte, MaxPayloadSize+1)
				return httptest.NewRequest("POST", "/webhook/test", bytes.NewReader(largeData))
			},
			wantErr:     true,
			errContains: "payload too large",
		},
		{
			name: "missing required fields",
			request: func() *http.Request {
				invalidPayload := `{"version": "4", "status": "firing"}`
				return httptest.NewRequest("POST", "/webhook/test", strings.NewReader(invalidPayload))
			},
			wantErr:     true,
			errContains: "missing groupKey",
		},
		{
			name: "invalid alert",
			request: func() *http.Request {
				invalidPayload := WebhookPayload{
					Version:  "4",
					GroupKey: "test-group",
					Status:   "firing",
					Alerts: []Alert{
						{
							Status: "invalid",
						},
					},
				}
				data, _ := json.Marshal(invalidPayload)
				return httptest.NewRequest("POST", "/webhook/test", bytes.NewReader(data))
			},
			wantErr:     true,
			errContains: "alert[0] validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.request()
			payload, err := ParseWebhookPayload(req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, payload)
			} else {
				require.NoError(t, err)
				require.NotNil(t, payload)
				assert.Equal(t, "4", payload.Version)
				assert.Equal(t, "test-group", payload.GroupKey)
			}
		})
	}
}

func TestWebhookPayload_MarshalJSON(t *testing.T) {
	payload := WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		GroupLabels: map[string]string{
			"alertname": "test",
		},
		Alerts: []Alert{
			{
				Status:      "firing",
				Fingerprint: "abc123",
				StartsAt:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					"alertname": "test",
				},
			},
		},
	}

	data, err := json.Marshal(&payload)
	require.NoError(t, err)

	// Verify the JSON contains expected fields
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "4", result["version"])
	assert.Equal(t, "test-group", result["groupKey"])
	assert.Equal(t, "firing", result["status"])

	// Verify alerts
	alerts, ok := result["alerts"].([]interface{})
	require.True(t, ok)
	require.Len(t, alerts, 1)

	alert, ok := alerts[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "firing", alert["status"])
	assert.Equal(t, "abc123", alert["fingerprint"])
}

func TestWebhookPayload_UnmarshalJSON(t *testing.T) {
	jsonData := `{
		"version": "4",
		"groupKey": "test-group",
		"status": "firing",
		"receiver": "test",
		"groupLabels": {
			"alertname": "test"
		},
		"commonLabels": {
			"severity": "warning"
		},
		"commonAnnotations": {
			"summary": "Test alert"
		},
		"externalURL": "http://alertmanager.example.com",
		"alerts": [
			{
				"status": "firing",
				"labels": {
					"alertname": "test",
					"instance": "server1"
				},
				"annotations": {
					"summary": "Test alert"
				},
				"startsAt": "2024-01-01T12:00:00Z",
				"endsAt": "0001-01-01T00:00:00Z",
				"generatorURL": "http://prometheus.example.com",
				"fingerprint": "abc123"
			}
		]
	}`

	var payload WebhookPayload
	err := json.Unmarshal([]byte(jsonData), &payload)
	require.NoError(t, err)

	assert.Equal(t, "4", payload.Version)
	assert.Equal(t, "test-group", payload.GroupKey)
	assert.Equal(t, "firing", payload.Status)
	assert.Equal(t, "test", payload.Receiver)
	assert.Equal(t, "test", payload.GroupLabels["alertname"])
	assert.Equal(t, "warning", payload.CommonLabels["severity"])
	assert.Equal(t, "Test alert", payload.CommonAnnotations["summary"])
	assert.Equal(t, "http://alertmanager.example.com", payload.ExternalURL)

	require.Len(t, payload.Alerts, 1)
	alert := payload.Alerts[0]
	assert.Equal(t, "firing", alert.Status)
	assert.Equal(t, "abc123", alert.Fingerprint)
	assert.Equal(t, "test", alert.Labels["alertname"])
	assert.Equal(t, "server1", alert.Labels["instance"])
}
