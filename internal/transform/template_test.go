package transform

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
)

func TestNewGoTemplateEngine(t *testing.T) {
	tests := []struct {
		name     string
		template string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid template",
			template: `{"message": "{{ .Status }}"}`,
			wantErr:  false,
		},
		{
			name:     "empty template",
			template: "",
			wantErr:  true,
			errMsg:   "template cannot be empty",
		},
		{
			name:     "invalid template syntax",
			template: `{"message": "{{ .Status }`,
			wantErr:  true,
			errMsg:   "failed to parse template",
		},
		{
			name:     "template with functions",
			template: `{"severity": "{{ .CommonLabels.severity | upper }}"}`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := NewGoTemplateEngine(tt.template)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, engine)
			} else {
				require.NoError(t, err)
				require.NotNil(t, engine)
				assert.Equal(t, tt.template, engine.templateString)
				assert.NotNil(t, engine.template)
			}
		})
	}
}

func TestGoTemplateEngine_Transform(t *testing.T) {
	now := time.Now()
	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		Receiver: "test",
		GroupLabels: map[string]string{
			"alertname": "TestAlert",
		},
		CommonLabels: map[string]string{
			"alertname": "TestAlert",
			"severity":  "critical",
			"service":   "api",
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
					"summary": "Server1 is down",
				},
			},
		},
	}

	tests := []struct {
		name        string
		template    string
		checkResult func(t *testing.T, result interface{})
		wantErr     bool
	}{
		{
			name:     "simple string template",
			template: `Alert: {{ .GroupLabels.alertname }} is {{ .Status }}`,
			checkResult: func(t *testing.T, result interface{}) {
				assert.Equal(t, "Alert: TestAlert is firing", result)
			},
		},
		{
			name:     "JSON template",
			template: `{"alertname": "{{ .GroupLabels.alertname }}", "status": "{{ .Status }}"}`,
			checkResult: func(t *testing.T, result interface{}) {
				data, ok := result.(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, "TestAlert", data["alertname"])
				assert.Equal(t, "firing", data["status"])
			},
		},
		{
			name: "template with functions",
			template: `{
				"severity": "{{ .CommonLabels.severity | upper }}",
				"alertCount": {{ len .Alerts }},
				"timestamp": {{ now | unixtime }}
			}`,
			checkResult: func(t *testing.T, result interface{}) {
				data, ok := result.(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, "CRITICAL", data["severity"])
				if alertCount, ok := data["alertCount"].(float64); ok {
					assert.Equal(t, 1.0, alertCount)
				} else if alertCount, ok := data["alertCount"].(int); ok {
					assert.Equal(t, 1, alertCount)
				} else {
					t.Errorf("alertCount has unexpected type: %T", data["alertCount"])
				}
				assert.NotNil(t, data["timestamp"])
			},
		},
		{
			name: "template with range",
			template: `{
				"alerts": [
					{{- range $i, $alert := .Alerts }}
					{{- if $i }},{{ end }}
					{
						"instance": "{{ $alert.Labels.instance }}",
						"summary": "{{ $alert.Annotations.summary }}"
					}
					{{- end }}
				]
			}`,
			checkResult: func(t *testing.T, result interface{}) {
				data, ok := result.(map[string]interface{})
				require.True(t, ok)
				alerts, ok := data["alerts"].([]interface{})
				require.True(t, ok)
				require.Len(t, alerts, 1)

				alert, ok := alerts[0].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, "server1", alert["instance"])
				assert.Equal(t, "Server1 is down", alert["summary"])
			},
		},
		{
			name:     "template with undefined field",
			template: `{{ .UndefinedField }}`,
			wantErr:  true,
		},
		{
			name:     "template with error",
			template: `{{ divide 1 0 }}`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := NewGoTemplateEngine(tt.template)
			require.NoError(t, err)

			result, err := engine.Transform(payload)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				tt.checkResult(t, result)
			}
		})
	}
}

func TestGoTemplateEngine_TransformAlert(t *testing.T) {
	now := time.Now()
	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		GroupLabels: map[string]string{
			"alertname": "TestAlert",
		},
		CommonLabels: map[string]string{
			"alertname": "TestAlert",
			"severity":  "critical",
		},
	}

	alert := &alertmanager.Alert{
		Status:      "firing",
		Fingerprint: "abc123",
		StartsAt:    now,
		Labels: map[string]string{
			"alertname": "TestAlert",
			"severity":  "critical",
			"instance":  "server1",
		},
		Annotations: map[string]string{
			"summary":     "Server is down",
			"description": "Server1 has been down for 5 minutes",
		},
	}

	tests := []struct {
		name        string
		template    string
		checkResult func(t *testing.T, result interface{})
	}{
		{
			name:     "access alert fields",
			template: `Alert {{ .Labels.alertname }} on {{ .Labels.instance }}`,
			checkResult: func(t *testing.T, result interface{}) {
				assert.Equal(t, "Alert TestAlert on server1", result)
			},
		},
		{
			name:     "access both alert and payload",
			template: `{{ .Labels.instance }} - {{ .GroupKey }}`,
			checkResult: func(t *testing.T, result interface{}) {
				assert.Equal(t, "server1 - test-group", result)
			},
		},
		{
			name: "JSON with alert data",
			template: `{
				"fingerprint": "{{ .Fingerprint }}",
				"instance": "{{ .Labels.instance }}",
				"summary": "{{ .Annotations.summary }}",
				"groupKey": "{{ .GroupKey }}"
			}`,
			checkResult: func(t *testing.T, result interface{}) {
				data, ok := result.(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, "abc123", data["fingerprint"])
				assert.Equal(t, "server1", data["instance"])
				assert.Equal(t, "Server is down", data["summary"])
				assert.Equal(t, "test-group", data["groupKey"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := NewGoTemplateEngine(tt.template)
			require.NoError(t, err)

			result, err := engine.TransformAlert(alert, payload)
			require.NoError(t, err)
			tt.checkResult(t, result)
		})
	}
}

func TestGoTemplateEngine_JSONParsing(t *testing.T) {
	engine, err := NewGoTemplateEngine(`{"valid": "json", "number": 42}`)
	require.NoError(t, err)

	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test",
		Status:   "firing",
		Alerts:   []alertmanager.Alert{},
	}

	result, err := engine.Transform(payload)
	require.NoError(t, err)

	// Should be parsed as JSON
	data, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "json", data["valid"])
	assert.Equal(t, float64(42), data["number"])
}

func TestGoTemplateEngine_ConcurrentAccess(t *testing.T) {
	engine, err := NewGoTemplateEngine(`{{ .Status }}`)
	require.NoError(t, err)

	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test",
		Status:   "firing",
		Alerts:   []alertmanager.Alert{},
	}

	// Run multiple goroutines
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			result, err := engine.Transform(payload)
			assert.NoError(t, err)
			assert.Equal(t, "firing", result)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestGoTemplateEngine_Properties(t *testing.T) {
	engine, err := NewGoTemplateEngine(`{{ .Status }}`)
	require.NoError(t, err)

	t.Run("validate", func(t *testing.T) {
		// Validate should always return nil for compiled templates
		assert.NoError(t, engine.Validate())
	})

	t.Run("name", func(t *testing.T) {
		assert.Equal(t, "go-template", engine.Name())
	})
}

func BenchmarkGoTemplateEngine_Transform(b *testing.B) {
	engine, _ := NewGoTemplateEngine(`{
		"alertname": "{{ .GroupLabels.alertname }}",
		"status": "{{ .Status }}",
		"severity": "{{ .CommonLabels.severity | upper }}",
		"alerts": {{ len .Alerts }}
	}`)

	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test",
		Status:   "firing",
		GroupLabels: map[string]string{
			"alertname": "TestAlert",
		},
		CommonLabels: map[string]string{
			"severity": "critical",
		},
		Alerts: make([]alertmanager.Alert, 10),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Transform(payload)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestTemplateContext_JSONMarshaling(t *testing.T) {
	ctx := &TemplateContext{
		Version:  "4",
		GroupKey: "test",
		Status:   "firing",
		GroupLabels: map[string]string{
			"alertname": "test",
		},
	}

	data, err := json.Marshal(ctx)
	require.NoError(t, err)

	var unmarshaled TemplateContext
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, ctx.Version, unmarshaled.Version)
	assert.Equal(t, ctx.GroupKey, unmarshaled.GroupKey)
	assert.Equal(t, ctx.Status, unmarshaled.Status)
	assert.Equal(t, ctx.GroupLabels, unmarshaled.GroupLabels)
}
