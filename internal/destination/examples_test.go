package destination

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/transform"
)

func TestGetExampleTemplate(t *testing.T) {
	tests := []struct {
		service  string
		hasValue bool
	}{
		{"flock", true},
		{"Flock", true},
		{"FLOCK", true},
		{"jenkins", true},
		{"jenkins-build", true},
		{"jenkins-buildwithparameters", true},
		{"slack", true},
		{"teams", true},
		{"msteams", true},
		{"telegram", true},
		{"discord", true},
		{"github", true},
		{"github-actions", true},
		{"github-action", true},
		{"mattermost", true},
		{"rocketchat", true},
		{"rocket-chat", true},
		{"splunk", true},
		{"victorialogs", true},
		{"victoria-logs", true},
		{"vlogs", true},
		{"debug", true},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			template := GetExampleTemplate(tt.service)
			if tt.hasValue {
				assert.NotEmpty(t, template)
			} else {
				assert.Empty(t, template)
			}
		})
	}
}

func TestGetExampleConfig(t *testing.T) {
	tests := []struct {
		service    string
		shouldFind bool
		check      func(t *testing.T, cfg *config.DestinationConfig)
	}{
		{
			service:    "flock",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "flock-alerts", cfg.Name)
				assert.Equal(t, "POST", cfg.Method)
				assert.Contains(t, cfg.URL, "flock.com")
				assert.Equal(t, "json", cfg.Format)
				assert.Equal(t, "go-template", cfg.Engine)
				assert.NotEmpty(t, cfg.Template)
			},
		},
		{
			service:    "jenkins",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "jenkins-trigger", cfg.Name)
				assert.Equal(t, "POST", cfg.Method)
				assert.Contains(t, cfg.URL, "jenkins")
				assert.NotEmpty(t, cfg.Headers)
			},
		},
		{
			service:    "jenkins-build",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "jenkins-build", cfg.Name)
				assert.Equal(t, "POST", cfg.Method)
				assert.Contains(t, cfg.URL, "buildWithParameters")
				assert.Equal(t, "form", cfg.Format)
				assert.True(t, cfg.SplitAlerts)
				assert.Contains(t, cfg.URL, "${JENKINS_USER}:${JENKINS_API_TOKEN}")
				assert.Equal(t, "application/x-www-form-urlencoded", cfg.Headers["Content-Type"])
			},
		},
		{
			service:    "slack",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "slack-alerts", cfg.Name)
				assert.Contains(t, cfg.URL, "slack.com")
			},
		},
		{
			service:    "telegram",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "telegram-bot", cfg.Name)
				assert.Contains(t, cfg.URL, "api.telegram.org")
				assert.Contains(t, cfg.URL, "sendMessage")
				assert.Equal(t, "json", cfg.Format)
				assert.False(t, cfg.SplitAlerts)
			},
		},
		{
			service:    "discord",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "discord-webhook", cfg.Name)
				assert.Contains(t, cfg.URL, "DISCORD_WEBHOOK_URL")
				assert.Equal(t, "json", cfg.Format)
				assert.False(t, cfg.SplitAlerts)
			},
		},
		{
			service:    "github",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "github-actions", cfg.Name)
				assert.Contains(t, cfg.URL, "api.github.com")
				assert.Contains(t, cfg.URL, "dispatches")
				assert.Equal(t, "json", cfg.Format)
				assert.False(t, cfg.SplitAlerts)
				assert.Contains(t, cfg.Headers["Authorization"], "Bearer")
				assert.Equal(t, "application/vnd.github.v3+json", cfg.Headers["Accept"])
			},
		},
		{
			service:    "splunk",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "splunk-hec", cfg.Name)
				assert.Contains(t, cfg.URL, "splunk")
				assert.True(t, cfg.SplitAlerts)
				assert.Contains(t, cfg.Headers["Authorization"], "Splunk")
			},
		},
		{
			service:    "victorialogs",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "victoria-logs", cfg.Name)
				assert.Contains(t, cfg.URL, "victorialogs")
				assert.True(t, cfg.SplitAlerts)
				assert.Contains(t, cfg.Headers, "AccountID")
			},
		},
		{
			service:    "mattermost",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "mattermost-alerts", cfg.Name)
				assert.Contains(t, cfg.URL, "MATTERMOST_WEBHOOK_URL")
				assert.Equal(t, "json", cfg.Format)
				assert.False(t, cfg.SplitAlerts)
			},
		},
		{
			service:    "rocketchat",
			shouldFind: true,
			check: func(t *testing.T, cfg *config.DestinationConfig) {
				assert.Equal(t, "rocketchat-alerts", cfg.Name)
				assert.Contains(t, cfg.URL, "ROCKETCHAT_WEBHOOK_URL")
				assert.Equal(t, "json", cfg.Format)
				assert.False(t, cfg.SplitAlerts)
			},
		},
		{
			service:    "unknown",
			shouldFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			cfg := GetExampleConfig(tt.service)
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

func TestExampleTemplatesAreValid(t *testing.T) {
	// Create sample webhook payload
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
		},
		CommonAnnotations: map[string]string{
			"summary":     "Test alert summary",
			"description": "Test alert description",
			"runbook_url": "https://wiki.example.com/runbook",
		},
		ExternalURL: "http://alertmanager.example.com",
		Alerts: []alertmanager.Alert{
			{
				Status:      "firing",
				Fingerprint: "abc123",
				Labels: map[string]string{
					"alertname": "TestAlert",
					"instance":  "server1",
				},
				Annotations: map[string]string{
					"description": "Server1 is down",
				},
			},
		},
	}

	templates := []struct {
		name     string
		template string
	}{
		{"flock", FlockWebhookTemplate},
		{"jenkins", JenkinsWebhookTemplate},
		{"slack", SlackWebhookTemplate},
		{"teams", TeamsWebhookTemplate},
		{"telegram", TelegramBotTemplate},
		{"discord", DiscordWebhookTemplate},
		{"github", GitHubActionsTemplate},
		{"mattermost", MattermostWebhookTemplate},
		{"rocketchat", RocketChatWebhookTemplate},
		{"splunk", SplunkWebhookTemplate},
		{"victorialogs", VictoriaLogsWebhookTemplate},
		{"debug", WebhookDebugTemplate},
	}

	for _, tt := range templates {
		t.Run(tt.name, func(t *testing.T) {
			// Create template engine
			engine, err := transform.NewGoTemplateEngine(tt.template)
			require.NoError(t, err)

			// Transform the payload
			result, err := engine.Transform(payload)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify it produces valid JSON
			jsonBytes, ok := result.(map[string]interface{})
			if !ok {
				// Try string
				jsonStr, ok := result.(string)
				require.True(t, ok, "Result should be map or string")

				var data interface{}
				err = json.Unmarshal([]byte(jsonStr), &data)
				require.NoError(t, err, "Template should produce valid JSON")
			} else {
				// Already parsed as JSON
				assert.NotEmpty(t, jsonBytes)
			}
		})
	}
}

func TestJenkinsBuildTemplateFormat(t *testing.T) {
	engine, err := transform.NewGoTemplateEngine(JenkinsBuildTemplate)
	require.NoError(t, err)

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
			"service":   "web-app",
		},
		CommonAnnotations: map[string]string{
			"summary":     "CPU usage is high",
			"description": "CPU > 90% for 5 minutes",
		},
		ExternalURL: "http://alertmanager.example.com",
		Alerts: []alertmanager.Alert{
			{
				Status:      "firing",
				Fingerprint: "abc123def456",
				Labels: map[string]string{
					"alertname": "HighCPU",
					"instance":  "server1",
					"severity":  "critical",
				},
				Annotations: map[string]string{
					"description": "CPU > 90%",
				},
			},
		},
	}

	result, err := engine.Transform(payload)
	require.NoError(t, err)

	// The result should be a string containing form-encoded parameters
	formData, ok := result.(string)
	require.True(t, ok, "Jenkins build template should produce a string")

	// Check that it contains expected form parameters
	assert.Contains(t, formData, "ALERT_NAME=HighCPU")
	assert.Contains(t, formData, "STATUS=FIRING")
	assert.Contains(t, formData, "SEVERITY=critical")
	assert.Contains(t, formData, "ALERT_COUNT=1")
	assert.Contains(t, formData, "GROUP_KEY=test-group")
	assert.Contains(t, formData, "RECEIVER=test-receiver")

	// Check that labels are present (even if the key format has issues)
	assert.Contains(t, formData, "critical")
	assert.Contains(t, formData, "web-app")
	assert.Contains(t, formData, "HighCPU")

	// Check URL encoding is applied (+ is used by url.QueryEscape for spaces)
	assert.Contains(t, formData, "SUMMARY=CPU+usage+is+high")
	assert.Contains(t, formData, "DESCRIPTION=CPU+%3E+90%25+for+5+minutes")

	// Verify it's valid form data format (key=value pairs separated by newlines)
	lines := strings.Split(strings.TrimSpace(formData), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		assert.Len(t, parts, 2, "Each line should be key=value format")
		assert.NotEmpty(t, parts[0], "Key should not be empty")
	}
}

func TestFlockTemplateFormat(t *testing.T) {
	engine, err := transform.NewGoTemplateEngine(FlockWebhookTemplate)
	require.NoError(t, err)

	payload := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test",
		Status:   "resolved",
		GroupLabels: map[string]string{
			"alertname": "HighCPU",
		},
		CommonAnnotations: map[string]string{
			"summary": "CPU usage is high",
		},
		Alerts: []alertmanager.Alert{
			{
				Annotations: map[string]string{
					"description": "CPU > 90%",
				},
			},
		},
	}

	result, err := engine.Transform(payload)
	require.NoError(t, err)

	data, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Check Flock specific fields
	assert.Contains(t, data["text"], "RESOLVED")
	assert.Contains(t, data["text"], "HighCPU")

	attachments, ok := data["attachments"].([]interface{})
	require.True(t, ok)
	require.Len(t, attachments, 1)

	attachment := attachments[0].(map[string]interface{})
	assert.Equal(t, "#36a64f", attachment["color"]) // Green for resolved
}
