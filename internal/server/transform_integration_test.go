package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
)

// TestTransformPipelineGoTemplate tests the complete transformation pipeline with Go templates
func TestTransformPipelineGoTemplate(t *testing.T) {
	testCases := []struct {
		name            string
		template        string
		alerts          []alertmanager.Alert
		expectedPayload string
		validatePayload func(t *testing.T, body []byte)
	}{
		{
			name: "Complex template with functions",
			template: `{
				"timestamp": {{now | unixtime}},
				"alert_count": {{len .Alerts}},
				"status": "{{.Status | upper}}",
				"severity_list": [{{range $i, $a := .Alerts}}{{if $i}},{{end}}"{{$a.Labels.severity}}"{{end}}],
				"summary": "{{.GroupLabels.alertname | default "Unknown"}} is {{.Status}}"
			}`,
			alerts: []alertmanager.Alert{
				{Status: "firing", Labels: map[string]string{"severity": "critical"}, Fingerprint: "fp-1", StartsAt: time.Now()},
				{Status: "firing", Labels: map[string]string{"severity": "warning"}, Fingerprint: "fp-2", StartsAt: time.Now()},
			},
			validatePayload: func(t *testing.T, body []byte) {
				t.Logf("Response body: %s", string(body))

				var data map[string]interface{}
				err := json.Unmarshal(body, &data)
				require.NoError(t, err)

				// The template produces a number without quotes, so it should be parsed as float64
				// Check timestamp is recent
				timestampRaw, ok := data["timestamp"]
				require.True(t, ok, "timestamp field should exist")

				// If the JSON unmarshaling worked correctly, timestamp should be float64
				timestamp, ok := timestampRaw.(float64)
				require.True(t, ok, "timestamp should be a float64, got %T: %v", timestampRaw, timestampRaw)

				assert.InDelta(t, time.Now().Unix(), timestamp, 5)

				assert.Equal(t, float64(2), data["alert_count"])
				assert.Equal(t, "FIRING", data["status"])

				severities := data["severity_list"].([]interface{})
				assert.Equal(t, []interface{}{"critical", "warning"}, severities)

				assert.Equal(t, "TestAlert is firing", data["summary"])
			},
		},
		{
			name: "Template with conditional logic",
			template: `{
				"alert_type": "{{if eq .Status "firing"}}ALERT{{else}}RECOVERY{{end}}",
				"critical_count": {{range .Alerts}}{{if eq .Labels.severity "critical"}}1{{end}}{{end}},
				"has_critical": {{range .Alerts}}{{if eq .Labels.severity "critical"}}true{{break}}{{end}}{{else}}false{{end}}
			}`,
			alerts: []alertmanager.Alert{
				{Status: "firing", Labels: map[string]string{"severity": "warning"}, Fingerprint: "fp-3", StartsAt: time.Now()},
				{Status: "firing", Labels: map[string]string{"severity": "critical"}, Fingerprint: "fp-4", StartsAt: time.Now()},
			},
			validatePayload: func(t *testing.T, body []byte) {
				// Clean up the template output (remove extra spaces)
				cleaned := strings.ReplaceAll(string(body), " ", "")
				cleaned = strings.ReplaceAll(cleaned, "\n", "")
				cleaned = strings.ReplaceAll(cleaned, "\t", "")

				assert.Contains(t, cleaned, `"alert_type":"ALERT"`)
				assert.Contains(t, cleaned, `"critical_count":1`)
				assert.Contains(t, cleaned, `"has_critical":true`)
			},
		},
		{
			name: "Template with alert iteration",
			template: `{
				"alerts": [
					{{range $i, $alert := .Alerts}}
					{{if $i}},{{end}}{
						"name": "{{$alert.Labels.alertname}}",
						"fingerprint": "{{$alert.Fingerprint}}",
						"duration": "{{$alert.StartsAt | since}}"
					}
					{{end}}
				]
			}`,
			alerts: []alertmanager.Alert{
				{
					Status:      "firing",
					Labels:      map[string]string{"alertname": "HighCPU"},
					Fingerprint: "abc123",
					StartsAt:    time.Now().Add(-5 * time.Minute),
				},
				{
					Status:      "firing",
					Labels:      map[string]string{"alertname": "HighMemory"},
					Fingerprint: "def456",
					StartsAt:    time.Now().Add(-10 * time.Minute),
				},
			},
			validatePayload: func(t *testing.T, body []byte) {
				var data map[string]interface{}
				err := json.Unmarshal(body, &data)
				require.NoError(t, err)

				alerts := data["alerts"].([]interface{})
				assert.Len(t, alerts, 2)

				alert1 := alerts[0].(map[string]interface{})
				assert.Equal(t, "HighCPU", alert1["name"])
				assert.Equal(t, "abc123", alert1["fingerprint"])
				assert.Contains(t, alert1["duration"], "5m") // ~5 minutes ago

				alert2 := alerts[1].(map[string]interface{})
				assert.Equal(t, "HighMemory", alert2["name"])
				assert.Equal(t, "def456", alert2["fingerprint"])
				assert.Contains(t, alert2["duration"], "10m") // ~10 minutes ago
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedRequest mockRequest
			mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				capturedRequest.Body = body
				w.WriteHeader(http.StatusOK)
			}))
			defer mockDest.Close()

			cfg := &config.Config{
				Server: config.ServerConfig{Port: 8080},
				Destinations: []config.DestinationConfig{
					{
						Name:     "template-test",
						URL:      mockDest.URL,
						Method:   "POST",
						Format:   "json",
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

			ts := httptest.NewServer(srv.GetRouter())
			defer ts.Close()

			// Allow server to initialize
			time.Sleep(100 * time.Millisecond)

			webhookPayload := &alertmanager.WebhookPayload{
				Version:     "4",
				GroupKey:    "template-test-group",
				Status:      "firing",
				GroupLabels: map[string]string{"alertname": "TestAlert"},
				Alerts:      tc.alerts,
			}

			payloadBytes, err := json.Marshal(webhookPayload)
			require.NoError(t, err)

			resp, err := http.Post(
				ts.URL+"/webhook/template-test",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			require.NoError(t, err)
			resp.Body.Close()

			// Give async request time to complete
			time.Sleep(100 * time.Millisecond)

			tc.validatePayload(t, capturedRequest.Body)
		})
	}
}

// TestTransformPipelineJQ tests the complete transformation pipeline with JQ
func TestTransformPipelineJQ(t *testing.T) {
	testCases := []struct {
		name            string
		transform       string
		alerts          []alertmanager.Alert
		validatePayload func(t *testing.T, body []byte)
	}{
		{
			name: "Complex JQ transformation",
			transform: `{
				status: .status | ascii_upcase,
				alert_count: .alerts | length,
				critical_alerts: [.alerts[] | select(.labels.severity == "critical") | .labels.alertname],
				summary: (.groupLabels.alertname // "Unknown") + " is " + .status,
				grouped_by_severity: .alerts | group_by(.labels.severity) | map({severity: .[0].labels.severity, count: length})
			}`,
			alerts: []alertmanager.Alert{
				{Status: "firing", Labels: map[string]string{"alertname": "HighCPU", "severity": "critical"}, Fingerprint: "fp-cpu", StartsAt: time.Now()},
				{Status: "firing", Labels: map[string]string{"alertname": "HighMemory", "severity": "critical"}, Fingerprint: "fp-mem", StartsAt: time.Now()},
				{Status: "firing", Labels: map[string]string{"alertname": "DiskUsage", "severity": "warning"}, Fingerprint: "fp-disk", StartsAt: time.Now()},
			},
			validatePayload: func(t *testing.T, body []byte) {
				var data map[string]interface{}
				err := json.Unmarshal(body, &data)
				require.NoError(t, err)

				assert.Equal(t, "FIRING", data["status"])
				assert.Equal(t, float64(3), data["alert_count"])

				criticalAlerts := data["critical_alerts"].([]interface{})
				assert.Contains(t, criticalAlerts, "HighCPU")
				assert.Contains(t, criticalAlerts, "HighMemory")

				assert.Equal(t, "TestGroup is firing", data["summary"])

				grouped := data["grouped_by_severity"].([]interface{})
				assert.Len(t, grouped, 2)
			},
		},
		{
			name: "JQ with custom object construction",
			transform: `.alerts | map({
				id: .fingerprint,
				alert: .labels.alertname,
				level: (if .labels.severity == "critical" then "🔴" else "🟡" end),
				duration_seconds: (now - (.startsAt | fromdateiso8601))
			})`,
			alerts: []alertmanager.Alert{
				{
					Fingerprint: "abc123",
					Labels:      map[string]string{"alertname": "HighCPU", "severity": "critical"},
					StartsAt:    time.Now().Add(-30 * time.Second),
				},
				{
					Fingerprint: "def456",
					Labels:      map[string]string{"alertname": "HighMemory", "severity": "warning"},
					StartsAt:    time.Now().Add(-60 * time.Second),
				},
			},
			validatePayload: func(t *testing.T, body []byte) {
				var data []map[string]interface{}
				err := json.Unmarshal(body, &data)
				require.NoError(t, err)

				assert.Len(t, data, 2)

				assert.Equal(t, "abc123", data[0]["id"])
				assert.Equal(t, "HighCPU", data[0]["alert"])
				assert.Equal(t, "🔴", data[0]["level"])
				assert.InDelta(t, 30, data[0]["duration_seconds"], 5)

				assert.Equal(t, "def456", data[1]["id"])
				assert.Equal(t, "HighMemory", data[1]["alert"])
				assert.Equal(t, "🟡", data[1]["level"])
				assert.InDelta(t, 60, data[1]["duration_seconds"], 5)
			},
		},
		{
			name: "JQ with filtering and aggregation",
			transform: `{
				total: .alerts | length,
				firing: [.alerts[] | select(.status == "firing")] | length,
				resolved: [.alerts[] | select(.status == "resolved")] | length,
				alerts_by_status: .alerts | group_by(.status) | map({status: .[0].status, names: map(.labels.alertname)})
			}`,
			alerts: []alertmanager.Alert{
				{Status: "firing", Labels: map[string]string{"alertname": "Alert1"}, Fingerprint: "fp-a1", StartsAt: time.Now()},
				{Status: "firing", Labels: map[string]string{"alertname": "Alert2"}, Fingerprint: "fp-a2", StartsAt: time.Now()},
				{Status: "resolved", Labels: map[string]string{"alertname": "Alert3"}},
			},
			validatePayload: func(t *testing.T, body []byte) {
				var data map[string]interface{}
				err := json.Unmarshal(body, &data)
				require.NoError(t, err)

				assert.Equal(t, float64(3), data["total"])
				assert.Equal(t, float64(2), data["firing"])
				assert.Equal(t, float64(1), data["resolved"])

				byStatus := data["alerts_by_status"].([]interface{})
				assert.Len(t, byStatus, 2)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedRequest mockRequest
			mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				capturedRequest.Body = body
				w.WriteHeader(http.StatusOK)
			}))
			defer mockDest.Close()

			cfg := &config.Config{
				Server: config.ServerConfig{Port: 8080},
				Destinations: []config.DestinationConfig{
					{
						Name:      "jq-test",
						URL:       mockDest.URL,
						Method:    "POST",
						Format:    "json",
						Engine:    "jq",
						Transform: tc.transform,
						Enabled:   true,
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
				Version:     "4",
				GroupKey:    "jq-test-group",
				Status:      "firing",
				GroupLabels: map[string]string{"alertname": "TestGroup"},
				Alerts:      tc.alerts,
			}

			// Set StartsAt times in ISO format for JQ
			for i := range webhookPayload.Alerts {
				if !webhookPayload.Alerts[i].StartsAt.IsZero() {
					// JQ expects ISO8601 format
					webhookPayload.Alerts[i].StartsAt = webhookPayload.Alerts[i].StartsAt.UTC()
				}
			}

			payloadBytes, err := json.Marshal(webhookPayload)
			require.NoError(t, err)

			resp, err := http.Post(
				ts.URL+"/webhook/jq-test",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			require.NoError(t, err)
			resp.Body.Close()

			// Give async request time to complete
			time.Sleep(100 * time.Millisecond)

			tc.validatePayload(t, capturedRequest.Body)
		})
	}
}

// TestTransformPipelineErrorHandling tests error handling in the transformation pipeline
func TestTransformPipelineErrorHandling(t *testing.T) {
	testCases := []struct {
		name             string
		engine           string
		template         string
		transform        string
		expectedError    bool
		destinationCalls int // How many times destination should be called
	}{
		{
			name:             "Invalid Go template syntax",
			engine:           "go-template",
			template:         `{{.InvalidField.NestedField}}`,
			expectedError:    true,
			destinationCalls: 0,
		},
		{
			name:             "Invalid JQ syntax",
			engine:           "jq",
			transform:        `.invalid | syntax error`,
			expectedError:    true,
			destinationCalls: 0,
		},
		{
			name:             "Go template runtime error",
			engine:           "go-template",
			template:         `{{range .NonExistentField}}{{.}}{{end}}`,
			expectedError:    false, // Template executes but with empty result
			destinationCalls: 1,
		},
		{
			name:             "JQ filter returns null",
			engine:           "jq",
			transform:        `.nonexistent`,
			expectedError:    false,
			destinationCalls: 1,
		},
		{
			name:             "Template produces invalid JSON",
			engine:           "go-template",
			template:         `{invalid json}`,
			expectedError:    false, // Template executes, formatter handles invalid JSON
			destinationCalls: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			callCount := 0
			mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				callCount++
				w.WriteHeader(http.StatusOK)
			}))
			defer mockDest.Close()

			destConfig := config.DestinationConfig{
				Name:    "error-test",
				URL:     mockDest.URL,
				Method:  "POST",
				Format:  "json",
				Engine:  tc.engine,
				Enabled: true,
			}

			if tc.engine == "go-template" {
				destConfig.Template = tc.template
			} else {
				destConfig.Transform = tc.transform
			}

			cfg := &config.Config{
				Server:       config.ServerConfig{Port: 8080},
				Destinations: []config.DestinationConfig{destConfig},
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
				GroupKey: fmt.Sprintf("error-%s-group", tc.engine),
				Status:   "firing",
				Alerts:   []alertmanager.Alert{{Status: "firing", Fingerprint: "test-123", StartsAt: time.Now()}},
			}

			payloadBytes, err := json.Marshal(webhookPayload)
			require.NoError(t, err)

			resp, err := http.Post(
				ts.URL+"/webhook/error-test",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			require.NoError(t, err)
			resp.Body.Close()

			// Webhook should always return 200 (async processing)
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			// Give async request time to complete
			time.Sleep(100 * time.Millisecond)

			assert.Equal(t, tc.destinationCalls, callCount, "Unexpected number of destination calls")
		})
	}
}

// TestTransformPipelineWithHeaders tests header handling in transformation
func TestTransformPipelineWithHeaders(t *testing.T) {
	var capturedHeaders http.Header
	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080},
		Destinations: []config.DestinationConfig{
			{
				Name:     "header-test",
				URL:      mockDest.URL,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"alert":"{{.Status}}"}`,
				Enabled:  true,
				Headers: map[string]string{
					"X-Custom-Header": "static-value",
					"X-Alert-Status":  "{{.Status}}",
					"X-Alert-Count":   "{{len .Alerts}}",
					"Authorization":   "Bearer ${API_TOKEN}",
				},
			},
		},
	}

	// Set environment variable for testing
	t.Setenv("API_TOKEN", "test-token-123")

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, err := New(cfg, logger)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	webhookPayload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "parallel-test-group",
		Status:   "firing",
		Alerts: []alertmanager.Alert{
			{Status: "firing", Fingerprint: "fp-1", StartsAt: time.Now()},
			{Status: "firing", Fingerprint: "fp-2", StartsAt: time.Now()},
		},
	}

	payloadBytes, err := json.Marshal(webhookPayload)
	require.NoError(t, err)

	resp, err := http.Post(
		ts.URL+"/webhook/header-test",
		"application/json",
		bytes.NewReader(payloadBytes),
	)
	require.NoError(t, err)
	resp.Body.Close()

	// Give async request time to complete
	time.Sleep(100 * time.Millisecond)

	// Verify headers
	assert.Equal(t, "static-value", capturedHeaders.Get("X-Custom-Header"))
	assert.Equal(t, "{{.Status}}", capturedHeaders.Get("X-Alert-Status")) // Headers are not templated currently
	assert.Equal(t, "{{len .Alerts}}", capturedHeaders.Get("X-Alert-Count"))
	assert.Equal(t, "Bearer test-token-123", capturedHeaders.Get("Authorization")) // Env var expanded
}