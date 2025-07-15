package destination

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/transform"
)

func TestJQExamples(t *testing.T) {
	now := time.Now()

	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		Receiver: "test-receiver",
		GroupLabels: map[string]string{
			"alertname": "HighCPU",
		},
		CommonLabels: map[string]string{
			"alertname": "HighCPU",
			"severity":  "critical",
		},
		CommonAnnotations: map[string]string{
			"summary":     "CPU usage is high",
			"description": "CPU > 90% for 5 minutes",
		},
		ExternalURL: "http://alertmanager.example.com",
		Alerts: []alertmanager.Alert{
			{
				Status:      "firing",
				Fingerprint: "critical123",
				StartsAt:    now,
				Labels: map[string]string{
					"alertname": "HighCPU",
					"severity":  "critical",
					"instance":  "server1",
				},
				Annotations: map[string]string{
					"description": "Server1 CPU > 90%",
				},
			},
			{
				Status:      "firing",
				Fingerprint: "warning456",
				StartsAt:    now,
				Labels: map[string]string{
					"alertname": "HighMemory",
					"severity":  "warning",
					"instance":  "server2",
				},
				Annotations: map[string]string{
					"description": "Server2 Memory > 80%",
				},
			},
		},
	}

	tests := []struct {
		name        string
		example     string
		checkResult func(t *testing.T, result interface{})
	}{
		{
			name:    "simple-status",
			example: "simple-status",
			checkResult: func(t *testing.T, result interface{}) {
				assert.Equal(t, "firing", result)
			},
		},
		{
			name:    "basic-alert",
			example: "basic-alert",
			checkResult: func(t *testing.T, result interface{}) {
				data := result.(map[string]interface{})
				assert.Equal(t, "firing", data["status"])
				assert.Equal(t, "HighCPU", data["alertname"])
				assert.Equal(t, 2, data["count"])
			},
		},
		{
			name:    "slack",
			example: "slack",
			checkResult: func(t *testing.T, result interface{}) {
				data := result.(map[string]interface{})
				assert.Contains(t, data["text"], "HighCPU")
				assert.Contains(t, data["text"], "FIRING")

				attachments := data["attachments"].([]interface{})
				require.Len(t, attachments, 1)

				attachment := attachments[0].(map[string]interface{})
				assert.Equal(t, "danger", attachment["color"])
			},
		},
		{
			name:    "filter-critical",
			example: "filter-critical",
			checkResult: func(t *testing.T, result interface{}) {
				data := result.(map[string]interface{})
				assert.Equal(t, "firing", data["status"])

				criticalAlerts := data["critical_alerts"].([]interface{})
				assert.Len(t, criticalAlerts, 1)
				assert.Equal(t, "HighCPU", criticalAlerts[0])

				assert.Equal(t, 1, data["count"])
			},
		},
		{
			name:    "group-by-severity",
			example: "group-by-severity",
			checkResult: func(t *testing.T, result interface{}) {
				data := result.(map[string]interface{})
				assert.Equal(t, "firing", data["status"])

				grouped := data["grouped"].([]interface{})
				assert.Len(t, grouped, 2) // critical and warning
			},
		},
		{
			name:    "custom-format",
			example: "custom-format",
			checkResult: func(t *testing.T, result interface{}) {
				data := result.(map[string]interface{})
				assert.Contains(t, data["message"], "2 alerts are firing")

				details := data["details"].(map[string]interface{})
				assert.Equal(t, "test-receiver", details["receiver"])
				assert.Equal(t, "test-group", details["group_key"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := GetJQExample(tt.example)
			require.NotEmpty(t, query)

			engine, err := transform.NewJQEngine(query)
			require.NoError(t, err)

			result, err := engine.Transform(payload)
			require.NoError(t, err)

			tt.checkResult(t, result)
		})
	}
}

func TestJQAlertSplitExample(t *testing.T) {
	now := time.Now()

	payload := &alertmanager.WebhookPayload{
		GroupKey: "test-group",
		Receiver: "test-receiver",
	}

	alert := &alertmanager.Alert{
		Status:      "firing",
		Fingerprint: "abc123",
		StartsAt:    now,
		Labels: map[string]string{
			"alertname": "TestAlert",
			"severity":  "critical",
		},
		Annotations: map[string]string{
			"description": "Test description",
		},
	}

	query := GetJQExample("alert-split")
	require.NotEmpty(t, query)

	engine, err := transform.NewJQEngine(query)
	require.NoError(t, err)

	result, err := engine.TransformAlert(alert, payload)
	require.NoError(t, err)

	data := result.(map[string]interface{})
	assert.Equal(t, "abc123", data["fingerprint"])
	assert.Equal(t, "TestAlert", data["alertname"])
	assert.Equal(t, "firing", data["status"])
	assert.Equal(t, "critical", data["severity"])
	assert.Equal(t, "test-group", data["group_key"])
	assert.Equal(t, "test-receiver", data["receiver"])
}

func TestGetJQExample(t *testing.T) {
	tests := []struct {
		name     string
		hasValue bool
	}{
		// Service-specific templates
		{"flock", true},
		{"jenkins", true},
		{"jenkins-build", true},
		{"jenkins-buildwithparameters", true},
		{"slack", true},
		{"teams", true},
		{"msteams", true},
		{"telegram", true},
		{"discord", true},
		{"mattermost", true},
		{"rocketchat", true},
		{"rocket-chat", true},
		{"splunk", true},
		{"victorialogs", true},
		{"victoria-logs", true},
		{"vlogs", true},
		{"github", true},
		{"github-actions", true},
		{"github-action", true},
		{"debug", true},
		// Pattern examples
		{"simple-status", true},
		{"basic-alert", true},
		{"slack-example", true},
		{"filter-critical", true},
		{"group-by-severity", true},
		{"custom-format", true},
		{"alert-split", true},
		// Unknown
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetJQExample(tt.name)
			if tt.hasValue {
				assert.NotEmpty(t, result)
			} else {
				assert.Empty(t, result)
			}
		})
	}
}

func TestGetExampleJQConfig(t *testing.T) {
	tests := []struct {
		service    string
		shouldFind bool
		check      func(t *testing.T, cfg *config.DestinationConfig)
	}{
		{
			service:    "flock",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "flock-alerts-jq", cfg.Name)
				assert.Equal(t, "POST", cfg.Method)
				assert.Contains(t, cfg.URL, "flock.com")
				assert.Equal(t, "json", cfg.Format)
				assert.Equal(t, "jq", cfg.Engine)
				assert.NotEmpty(t, cfg.Transform)
				assert.False(t, cfg.SplitAlerts)
			},
		},
		{
			service:    "jenkins",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "jenkins-trigger-jq", cfg.Name)
				assert.Equal(t, "jq", cfg.Engine)
				assert.NotEmpty(t, cfg.Transform)
				assert.Contains(t, cfg.Headers, "X-Jenkins-Token")
			},
		},
		{
			service:    "slack",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "slack-alerts-jq", cfg.Name)
				assert.Equal(t, "jq", cfg.Engine)
				assert.Contains(t, cfg.URL, "slack.com")
			},
		},
		{
			service:    "splunk",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "splunk-hec-jq", cfg.Name)
				assert.True(t, cfg.SplitAlerts)
				assert.Equal(t, "jq", cfg.Engine)
				assert.Contains(t, cfg.Headers["Authorization"], "Splunk")
			},
		},
		{
			service:    "unknown",
			shouldFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			cfg := GetExampleJQConfig(tt.service)
			if tt.shouldFind {
				require.NotNil(t, cfg)
				if tt.check != nil {
					tt.check(t, cfg)
				}
			} else {
				assert.Nil(t, cfg)
			}
		})
	}
}

func TestJQServiceTemplatesAreValid(t *testing.T) {
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

	alert := &alertmanager.Alert{
		Status:      "firing",
		Fingerprint: "alert123",
		StartsAt:    now,
		Labels: map[string]string{
			"alertname": "TestAlert",
			"severity":  "critical",
			"instance":  "server1",
		},
		Annotations: map[string]string{
			"description": "Test description",
		},
	}

	services := []string{
		"flock", "jenkins", "slack", "teams", "telegram", "discord",
		"mattermost", "rocketchat", "github", "debug",
	}

	splitServices := []string{
		"jenkins-build", "splunk", "victorialogs",
	}

	// Test grouped services
	for _, service := range services {
		t.Run("grouped/"+service, func(t *testing.T) {
			query := GetJQExample(service)
			require.NotEmpty(t, query, "Service %s should have a jq template", service)

			engine, err := transform.NewJQEngine(query)
			require.NoError(t, err, "Service %s jq template should compile", service)

			result, err := engine.Transform(payload)
			require.NoError(t, err, "Service %s jq template should transform successfully", service)
			assert.NotNil(t, result, "Service %s jq template should return a result", service)
		})
	}

	// Test split services
	for _, service := range splitServices {
		t.Run("split/"+service, func(t *testing.T) {
			query := GetJQExample(service)
			require.NotEmpty(t, query, "Service %s should have a jq template", service)

			engine, err := transform.NewJQEngine(query)
			require.NoError(t, err, "Service %s jq template should compile", service)

			result, err := engine.TransformAlert(alert, payload)
			require.NoError(t, err, "Service %s jq template should transform alert successfully", service)
			assert.NotNil(t, result, "Service %s jq template should return a result", service)
		})
	}
}
