package transform

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
)

func TestNewJQEngine(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
		errMsg  string
	}{
		{
			name:  "valid simple query",
			query: ".status",
		},
		{
			name:  "valid complex query",
			query: ".alerts | map({name: .labels.alertname, severity: .labels.severity})",
		},
		{
			name:  "valid conditional query",
			query: "if .status == \"firing\" then \"ALERT\" else \"OK\" end",
		},
		{
			name:    "empty query",
			query:   "",
			wantErr: true,
			errMsg:  "jq query cannot be empty",
		},
		{
			name:    "invalid syntax",
			query:   ".status |",
			wantErr: true,
			errMsg:  "failed to parse jq query",
		},
		{
			name:    "invalid function",
			query:   ".status | invalid_function",
			wantErr: true,
			errMsg:  "failed to compile jq query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := NewJQEngine(tt.query)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, engine)
			} else {
				require.NoError(t, err)
				require.NotNil(t, engine)
				assert.Equal(t, "jq", engine.Name())
				assert.Equal(t, tt.query, engine.GetQuery())
			}
		})
	}
}

func TestJQEngine_Transform(t *testing.T) {
	now := time.Now()

	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		Receiver: "test-receiver",
		GroupLabels: map[string]string{
			"alertname": "TestAlert",
		},
		CommonLabels: map[string]string{
			"alertname": "TestAlert",
			"severity":  "critical",
			"service":   "web-app",
		},
		CommonAnnotations: map[string]string{
			"summary":     "Test alert summary",
			"description": "Test alert description",
		},
		ExternalURL: "http://alertmanager.example.com",
		Alerts: []alertmanager.Alert{
			{
				Status:      "firing",
				Fingerprint: "abc123",
				StartsAt:    now,
				Labels: map[string]string{
					"alertname": "TestAlert",
					"severity":  "critical",
					"instance":  "server1",
				},
				Annotations: map[string]string{
					"description": "Server is down",
				},
			},
		},
	}

	tests := []struct {
		name     string
		query    string
		expected interface{}
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "simple field access",
			query:    ".status",
			expected: "firing",
		},
		{
			name:     "nested field access",
			query:    ".commonLabels.severity",
			expected: "critical",
		},
		{
			name:  "object construction",
			query: "{status: .status, alertname: .groupLabels.alertname}",
			expected: map[string]interface{}{
				"status":    "firing",
				"alertname": "TestAlert",
			},
		},
		{
			name:     "array access",
			query:    ".alerts[0].fingerprint",
			expected: "abc123",
		},
		{
			name:  "array mapping",
			query: ".alerts | map(.labels.alertname)",
			expected: []interface{}{
				"TestAlert",
			},
		},
		{
			name:     "conditional expression",
			query:    "if .status == \"firing\" then \"ALERT\" else \"OK\" end",
			expected: "ALERT",
		},
		{
			name:     "string concatenation",
			query:    ".groupLabels.alertname + \" is \" + .status",
			expected: "TestAlert is firing",
		},
		{
			name:  "complex transformation",
			query: "{alert_name: .groupLabels.alertname, status: .status, count: (.alerts | length), severity: .commonLabels.severity}",
			expected: map[string]interface{}{
				"alert_name": "TestAlert",
				"status":     "firing",
				"count":      1,
				"severity":   "critical",
			},
		},
		{
			name:     "null result",
			query:    ".nonexistent",
			expected: nil,
		},
		{
			name:    "invalid field access",
			query:   ".status.invalid",
			wantErr: true,
			errMsg:  "jq transformation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := NewJQEngine(tt.query)
			require.NoError(t, err)

			result, err := engine.Transform(payload)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestJQEngine_TransformAlert(t *testing.T) {
	now := time.Now()

	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		Receiver: "test-receiver",
	}

	alert := &alertmanager.Alert{
		Status:      "firing",
		Fingerprint: "alert123",
		StartsAt:    now,
		Labels: map[string]string{
			"alertname": "HighCPU",
			"severity":  "warning",
			"instance":  "server1",
		},
		Annotations: map[string]string{
			"description": "CPU usage is high",
		},
	}

	tests := []struct {
		name     string
		query    string
		expected interface{}
		wantErr  bool
	}{
		{
			name:     "access alert data",
			query:    ".alert.fingerprint",
			expected: "alert123",
		},
		{
			name:     "access payload data",
			query:    ".payload.status",
			expected: "firing",
		},
		{
			name:  "combine alert and payload",
			query: "{fingerprint: .alert.fingerprint, group: .payload.groupKey, severity: .alert.labels.severity}",
			expected: map[string]interface{}{
				"fingerprint": "alert123",
				"group":       "test-group",
				"severity":    "warning",
			},
		},
		{
			name:     "alert label access",
			query:    ".alert.labels.alertname",
			expected: "HighCPU",
		},
		{
			name:     "alert annotation access",
			query:    ".alert.annotations.description",
			expected: "CPU usage is high",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := NewJQEngine(tt.query)
			require.NoError(t, err)

			result, err := engine.TransformAlert(alert, payload)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestJQEngine_Advanced(t *testing.T) {
	t.Run("validate", func(t *testing.T) {
		tests := []struct {
			name    string
			query   string
			wantErr bool
		}{
			{
				name:  "valid simple query",
				query: ".status",
			},
			{
				name:  "valid complex query",
				query: ".alerts | map(select(.status == \"firing\")) | length",
			},
			{
				name:  "valid conditional",
				query: "if .status == \"firing\" then \"red\" else \"green\" end",
			},
			{
				name:    "invalid operation",
				query:   ".status + .alerts",
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				engine, err := NewJQEngine(tt.query)
				require.NoError(t, err)

				err = engine.Validate()

				if tt.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})


	t.Run("concurrent access", func(t *testing.T) {
		engine, err := NewJQEngine(".status")
		require.NoError(t, err)

		payload := &alertmanager.WebhookPayload{
			Status: "firing",
		}

		// Run multiple transformations concurrently
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				result, err := engine.Transform(payload)
				assert.NoError(t, err)
				assert.Equal(t, "firing", result)
				done <- true
			}()
		}

		// Wait for all goroutines to complete
		for i := 0; i < 10; i++ {
			<-done
		}
	})


	t.Run("complex transformations", func(t *testing.T) {
		payload := &alertmanager.WebhookPayload{
			Version:  "4",
			GroupKey: "test",
			Status:   "firing",
			Alerts: []alertmanager.Alert{
				{
					Status: "firing",
					Labels: map[string]string{
						"alertname": "HighCPU",
						"severity":  "critical",
					},
					Annotations: map[string]string{
						"summary": "CPU is high",
					},
				},
				{
					Status: "firing",
					Labels: map[string]string{
						"alertname": "HighMemory",
						"severity":  "warning",
					},
					Annotations: map[string]string{
						"summary": "Memory is high",
					},
				},
			},
		}

		tests := []struct {
			name     string
			query    string
			expected interface{}
		}{
			{
				name:  "filter and map alerts",
				query: ".alerts | map(select(.labels.severity == \"critical\")) | map(.labels.alertname)",
				expected: []interface{}{
					"HighCPU",
				},
			},
			{
				name:  "group by severity",
				query: ".alerts | group_by(.labels.severity) | map({severity: .[0].labels.severity, count: length})",
				expected: []interface{}{
					map[string]interface{}{
						"severity": "critical",
						"count":    1,
					},
					map[string]interface{}{
						"severity": "warning",
						"count":    1,
					},
				},
			},
			{
				name:     "count alerts by status",
				query:    ".alerts | map(select(.status == \"firing\")) | length",
				expected: 2,
			},
			{
				name:     "create formatted message",
				query:    "\"Alert: \" + (.alerts | length | tostring) + \" firing alerts\"",
				expected: "Alert: 2 firing alerts",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				engine, err := NewJQEngine(tt.query)
				require.NoError(t, err)

				result, err := engine.Transform(payload)
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			})
		}
	})


	t.Run("error handling", func(t *testing.T) {
		tests := []struct {
			name    string
			query   string
			payload *alertmanager.WebhookPayload
			wantErr bool
			errMsg  string
		}{
			{
				name:  "division by zero",
				query: "1 / 0",
				payload: &alertmanager.WebhookPayload{
					Status: "firing",
				},
				wantErr: true,
			},
			{
				name:  "type error",
				query: ".status + .alerts",
				payload: &alertmanager.WebhookPayload{
					Status: "firing",
					Alerts: []alertmanager.Alert{{}},
				},
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				engine, err := NewJQEngine(tt.query)
				require.NoError(t, err)

				_, err = engine.Transform(tt.payload)

				if tt.wantErr {
					require.Error(t, err)
					if tt.errMsg != "" {
						assert.Contains(t, err.Error(), tt.errMsg)
					}
				} else {
					require.NoError(t, err)
				}
			})
		}
	})


	t.Run("performance timeout", func(t *testing.T) {
		// Create a query that should cause an error (simulating complex processing)
		engine, err := NewJQEngine(".status + .alerts")
		require.NoError(t, err)

		payload := &alertmanager.WebhookPayload{
			Status: "firing",
			Alerts: []alertmanager.Alert{{}},
		}

		_, err = engine.Transform(payload)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "jq transformation failed")
	})
}
