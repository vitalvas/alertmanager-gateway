package destination

import (
	"net/http"
	"strings"

	"github.com/vitalvas/alertmanager-gateway/internal/config"
)

// FlockWebhookTemplate is an example template for Flock chat notifications
const FlockWebhookTemplate = `{
  "text": "Alert: {{ .GroupLabels.alertname }} [{{ .Status | upper }}]",
  "attachments": [{
    "title": "{{ .CommonAnnotations.summary }}",
    "description": "{{ range .Alerts }}{{ if .Annotations.description }}• {{ .Annotations.description }}\n{{ end }}{{ end }}",
    "color": "{{ if eq .Status "firing" }}#ff0000{{ else }}#36a64f{{ end }}",
    "views": {
      "flockml": "<flockml>{{ if .CommonAnnotations.runbook_url }}<a href=\"{{ .CommonAnnotations.runbook_url }}\">Runbook</a> | {{ end }}<a href=\"{{ .ExternalURL }}\">Alertmanager</a></flockml>"
    },
    "forwards": true
  }],
  "flockml": "AlertCount: <strong>{{ len .Alerts }}</strong> | Receiver: <em>{{ .Receiver }}</em>"
}`

// JenkinsWebhookTemplate is an example template for Jenkins Generic Webhook Trigger
const JenkinsWebhookTemplate = `{
  "alertname": "{{ .GroupLabels.alertname }}",
  "status": "{{ .Status }}",
  "severity": "{{ .CommonLabels.severity }}",
  "alertCount": {{ len .Alerts }},
  "groupKey": "{{ .GroupKey }}",
  "commonLabels": {{ .CommonLabels | jsonencode }},
  "commonAnnotations": {{ .CommonAnnotations | jsonencode }},
  "alerts": [
    {{- range $i, $alert := .Alerts }}
    {{- if $i }},{{ end }}
    {
      "status": "{{ $alert.Status }}",
      "labels": {{ $alert.Labels | jsonencode }},
      "annotations": {{ $alert.Annotations | jsonencode }},
      "startsAt": "{{ $alert.StartsAt.Format "2006-01-02T15:04:05Z07:00" }}",
      "endsAt": "{{ $alert.EndsAt.Format "2006-01-02T15:04:05Z07:00" }}",
      "fingerprint": "{{ $alert.Fingerprint }}"
    }
    {{- end }}
  ]
}`

// JenkinsBuildTemplate is an example template for Jenkins buildWithParameters API
const JenkinsBuildTemplate = `ALERT_NAME={{ .GroupLabels.alertname | default "" | urlencode }}
STATUS={{ .Status | upper | urlencode }}
SEVERITY={{ .CommonLabels.severity | default "unknown" | urlencode }}
ALERT_COUNT={{ len .Alerts }}
GROUP_KEY={{ .GroupKey | default "" | urlencode }}
RECEIVER={{ .Receiver | default "" | urlencode }}
SUMMARY={{ .CommonAnnotations.summary | default "" | urlencode }}
DESCRIPTION={{ .CommonAnnotations.description | default "" | urlencode }}
EXTERNAL_URL={{ .ExternalURL | default "" | urlencode }}
{{- range $key, $value := .CommonLabels }}
LABEL_{{ printf "%s" $key | upper | replace "-" "_" | replace "." "_" }}={{ printf "%s" $value | urlencode }}
{{- end }}
{{- range $key, $value := .CommonAnnotations }}
ANNOTATION_{{ printf "%s" $key | upper | replace "-" "_" | replace "." "_" }}={{ printf "%s" $value | urlencode }}
{{- end }}
{{- if .Alerts }}
{{- $first := index .Alerts 0 }}
FIRST_ALERT_FINGERPRINT={{ $first.Fingerprint | default "" | urlencode }}
FIRST_ALERT_STARTS_AT={{ timeformat "2006-01-02T15:04:05Z07:00" $first.StartsAt | urlencode }}
{{- if not $first.EndsAt.IsZero }}
FIRST_ALERT_ENDS_AT={{ timeformat "2006-01-02T15:04:05Z07:00" $first.EndsAt | urlencode }}
{{- end }}
{{- range $key, $value := $first.Labels }}
FIRST_ALERT_LABEL_{{ printf "%s" $key | upper | replace "-" "_" | replace "." "_" }}={{ printf "%s" $value | urlencode }}
{{- end }}
{{- end }}`

// SlackWebhookTemplate is an example template for Slack incoming webhooks
const SlackWebhookTemplate = `{
  "text": "*Alert:* {{ .GroupLabels.alertname }} - {{ .Status | upper }}",
  "attachments": [
    {
      "color": "{{ if eq .Status "firing" }}danger{{ else }}good{{ end }}",
      "title": "{{ .CommonAnnotations.summary }}",
      "text": "{{ .CommonAnnotations.description }}",
      "fields": [
        {
          "title": "Severity",
          "value": "{{ .CommonLabels.severity | default "unknown" }}",
          "short": true
        },
        {
          "title": "Alert Count",
          "value": "{{ len .Alerts }}",
          "short": true
        }
      ],
      "footer": "Alertmanager",
      "footer_icon": "https://prometheus.io/assets/prometheus_logo_grey.svg",
      "ts": {{ now | unixtime }}
    }
  ]
}`

// TeamsWebhookTemplate is an example template for Microsoft Teams
const TeamsWebhookTemplate = `{
  "@type": "MessageCard",
  "@context": "https://schema.org/extensions",
  "summary": "{{ .GroupLabels.alertname }} - {{ .Status }}",
  "themeColor": "{{ if eq .Status "firing" }}FF0000{{ else }}00FF00{{ end }}",
  "title": "{{ .GroupLabels.alertname }}",
  "sections": [
    {
      "activityTitle": "{{ .CommonAnnotations.summary }}",
      "activitySubtitle": "{{ .CommonAnnotations.description }}",
      "facts": [
        {
          "name": "Status:",
          "value": "{{ .Status | upper }}"
        },
        {
          "name": "Severity:",
          "value": "{{ .CommonLabels.severity | default "unknown" }}"
        },
        {
          "name": "Alert Count:",
          "value": "{{ len .Alerts }}"
        }
      ]
    }
  ],
  "potentialAction": [
    {
      "@type": "OpenUri",
      "name": "View in Alertmanager",
      "targets": [
        {
          "os": "default",
          "uri": "{{ .ExternalURL }}"
        }
      ]
    }
  ]
}`

// TelegramBotTemplate is an example template for Telegram Bot API
const TelegramBotTemplate = `{
  "chat_id": "${TELEGRAM_CHAT_ID}",
  "parse_mode": "Markdown",
  "disable_web_page_preview": true,
  "text": "{{ if eq .Status "firing" }}[ALERT]{{ else }}[OK]{{ end }} *{{ .GroupLabels.alertname | replace "_" "\\_" | replace "*" "\\*" }}*\n\n{{ if .CommonAnnotations.summary }}{{ .CommonAnnotations.summary | replace "_" "\\_" | replace "*" "\\*" }}\n{{ end }}{{ if .CommonAnnotations.description }}{{ .CommonAnnotations.description | replace "_" "\\_" | replace "*" "\\*" }}\n{{ end }}\n*Status:* {{ .Status | upper }}\n*Severity:* {{ .CommonLabels.severity | default "unknown" | replace "_" "\\_" | replace "*" "\\*" }}\n*Alert Count:* {{ len .Alerts }}\n*Receiver:* {{ .Receiver | replace "_" "\\_" | replace "*" "\\*" }}{{ if .ExternalURL }}\n\n[View in Alertmanager]({{ .ExternalURL }}){{ end }}{{ if gt (len .Alerts) 1 }}\n\n*Alerts:*{{ range $i, $alert := .Alerts }}\n{{ add $i 1 }}. {{ $alert.Labels.alertname | default "Unknown" | replace "_" "\\_" | replace "*" "\\*" }}{{ if $alert.Labels.instance }} ({{ $alert.Labels.instance | replace "_" "\\_" | replace "*" "\\*" }}){{ end }}{{ end }}{{ end }}"
}`

// DiscordWebhookTemplate is an example template for Discord webhook
const DiscordWebhookTemplate = `{
  "username": "Alertmanager",
  "avatar_url": "https://prometheus.io/assets/prometheus_logo_grey.svg",
  "embeds": [{
    "title": "{{ if eq .Status "firing" }}[ALERT]{{ else }}[OK]{{ end }} {{ .GroupLabels.alertname }}",
    "description": "{{ .CommonAnnotations.summary }}",
    "color": {{ if eq .Status "firing" }}15158332{{ else }}3066993{{ end }},
    "fields": [
      {
        "name": "Status",
        "value": "{{ .Status | upper }}",
        "inline": true
      },
      {
        "name": "Severity",
        "value": "{{ .CommonLabels.severity | default "unknown" }}",
        "inline": true
      },
      {
        "name": "Alert Count",
        "value": "{{ len .Alerts }}",
        "inline": true
      }{{ if .CommonAnnotations.description }},
      {
        "name": "Description",
        "value": "{{ .CommonAnnotations.description }}",
        "inline": false
      }{{ end }}
    ],
    "footer": {
      "text": "Receiver: {{ .Receiver }} | Group: {{ .GroupKey }}"
    },
    "timestamp": "{{ now.Format "2006-01-02T15:04:05Z07:00" }}"{{ if .ExternalURL }},
    "url": "{{ .ExternalURL }}"{{ end }}
  }]
}`

// SplunkWebhookTemplate is an example template for Splunk HTTP Event Collector
const SplunkWebhookTemplate = `{
  "time": {{ now | unixtime }},
  "host": "alertmanager",
  "source": "alertmanager:webhook",
  "sourcetype": "prometheus:alert",
  "event": {
    "alert_name": "{{ .GroupLabels.alertname }}",
    "status": "{{ .Status }}",
    "severity": "{{ .CommonLabels.severity | default "unknown" }}",
    "group_key": "{{ .GroupKey }}",
    "receiver": "{{ .Receiver }}",
    "summary": "{{ .CommonAnnotations.summary }}",
    "description": "{{ .CommonAnnotations.description }}",
    "alert_count": {{ len .Alerts }},
    "firing_alerts": {{ len .Alerts }},
    "resolved_alerts": 0,
    "common_labels": {{ .CommonLabels | jsonencode }},
    "common_annotations": {{ .CommonAnnotations | jsonencode }},
    "external_url": "{{ .ExternalURL }}",
    "alerts": [
      {{- range $i, $alert := .Alerts }}
      {{- if $i }},{{ end }}
      {
        "status": "{{ $alert.Status }}",
        "fingerprint": "{{ $alert.Fingerprint }}",
        "starts_at": "{{ $alert.StartsAt.Format "2006-01-02T15:04:05Z07:00" }}",
        "ends_at": "{{ if not $alert.EndsAt.IsZero }}{{ $alert.EndsAt.Format "2006-01-02T15:04:05Z07:00" }}{{ else }}null{{ end }}",
        "labels": {{ $alert.Labels | jsonencode }},
        "annotations": {{ $alert.Annotations | jsonencode }}
      }
      {{- end }}
    ]
  }
}`

// VictoriaLogsWebhookTemplate is an example template for VictoriaLogs JSON ingestion
const VictoriaLogsWebhookTemplate = `{
  "_time": "{{ timeformat "2006-01-02T15:04:05.000Z" now }}",
  "_msg": "Prometheus Alert: {{ .GroupLabels.alertname }} - {{ .Status }}",
  "alert_name": "{{ .GroupLabels.alertname }}",
  "status": "{{ .Status }}",
  "severity": "{{ .CommonLabels.severity | default "unknown" }}",
  "service": "alertmanager",
  "source": "webhook",
  "group_key": "{{ .GroupKey }}",
  "receiver": "{{ .Receiver }}",
  "summary": "{{ .CommonAnnotations.summary }}",
  "description": "{{ .CommonAnnotations.description }}",
  "alert_count": {{ len .Alerts }},
  "firing_count": {{ len .Alerts }},
  "resolved_count": 0,
  "external_url": "{{ .ExternalURL }}",
  {{- range $key, $value := .CommonLabels }}
  "label_{{ $key }}": "{{ $value }}",
  {{- end }}
  {{- range $key, $value := .CommonAnnotations }}
  "annotation_{{ $key | replace "." "_" }}": "{{ $value }}",
  {{- end }}
  "raw_alerts": {{ .Alerts | jsonencode }}
}`

// WebhookDebugTemplate is a template for debugging that outputs all available data
const WebhookDebugTemplate = `{
  "webhook_version": "{{ .Version }}",
  "group_key": "{{ .GroupKey }}",
  "status": "{{ .Status }}",
  "receiver": "{{ .Receiver }}",
  "group_labels": {{ .GroupLabels | jsonencode }},
  "common_labels": {{ .CommonLabels | jsonencode }},
  "common_annotations": {{ .CommonAnnotations | jsonencode }},
  "external_url": "{{ .ExternalURL }}",
  "alert_count": {{ len .Alerts }},
  "truncated_alerts": {{ .TruncatedAlerts }},
  "alerts": {{ .Alerts | jsonencode }}
}`

// GitHubActionsTemplate is an example template for GitHub Actions repository dispatch
const GitHubActionsTemplate = `{
  "event_type": "prometheus-alert",
  "client_payload": {
    "alert_name": "{{ .GroupLabels.alertname }}",
    "status": "{{ .Status }}",
    "severity": "{{ .CommonLabels.severity | default "unknown" }}",
    "summary": "{{ .CommonAnnotations.summary }}",
    "description": "{{ .CommonAnnotations.description }}",
    "group_key": "{{ .GroupKey }}",
    "receiver": "{{ .Receiver }}",
    "alert_count": {{ len .Alerts }},
    "firing_count": {{ if eq .Status "firing" }}{{ len .Alerts }}{{ else }}0{{ end }},
    "resolved_count": {{ if eq .Status "resolved" }}{{ len .Alerts }}{{ else }}0{{ end }},
    "external_url": "{{ .ExternalURL }}",
    "timestamp": "{{ now.Format "2006-01-02T15:04:05Z07:00" }}",
    "labels": {{ .CommonLabels | jsonencode }},
    "annotations": {{ .CommonAnnotations | jsonencode }},
    "alerts": [
      {{- range $i, $alert := .Alerts }}
      {{- if $i }},{{ end }}
      {
        "status": "{{ $alert.Status }}",
        "fingerprint": "{{ $alert.Fingerprint }}",
        "starts_at": "{{ $alert.StartsAt.Format "2006-01-02T15:04:05Z07:00" }}",
        "ends_at": "{{ if not $alert.EndsAt.IsZero }}{{ $alert.EndsAt.Format "2006-01-02T15:04:05Z07:00" }}{{ else }}null{{ end }}",
        "generator_url": "{{ $alert.GeneratorURL }}",
        "labels": {{ $alert.Labels | jsonencode }},
        "annotations": {{ $alert.Annotations | jsonencode }}
      }
      {{- end }}
    ]
  }
}`

// MattermostWebhookTemplate is an example template for Mattermost incoming webhooks
const MattermostWebhookTemplate = `{
  "channel": "${MATTERMOST_CHANNEL}",
  "username": "Alertmanager",
  "icon_url": "https://prometheus.io/assets/prometheus_logo_grey.svg",
  "text": "{{ if eq .Status "firing" }}**FIRING**{{ else }}**RESOLVED**{{ end }} - {{ .GroupLabels.alertname }}",
  "attachments": [
    {
      "color": "{{ if eq .Status "firing" }}danger{{ else }}good{{ end }}",
      "title": "{{ .CommonAnnotations.summary }}",
      "text": "{{ .CommonAnnotations.description }}",
      "fields": [
        {
          "title": "Status",
          "value": "{{ .Status | upper }}",
          "short": true
        },
        {
          "title": "Severity",
          "value": "{{ .CommonLabels.severity | default "unknown" }}",
          "short": true
        },
        {
          "title": "Alert Count",
          "value": "{{ len .Alerts }}",
          "short": true
        },
        {
          "title": "Receiver",
          "value": "{{ .Receiver }}",
          "short": true
        }{{ if .CommonAnnotations.runbook_url }},
        {
          "title": "Runbook",
          "value": "[View Runbook]({{ .CommonAnnotations.runbook_url }})",
          "short": false
        }{{ end }}
      ],
      "footer": "Alertmanager",
      "footer_icon": "https://prometheus.io/assets/prometheus_logo_grey.svg",
      "ts": {{ now | unixtime }}{{ if .ExternalURL }},
      "title_link": "{{ .ExternalURL }}"{{ end }}
    }
  ]
}`

// RocketChatWebhookTemplate is an example template for RocketChat incoming webhooks
const RocketChatWebhookTemplate = `{
  "channel": "${ROCKETCHAT_CHANNEL}",
  "username": "Alertmanager",
  "avatar": "https://prometheus.io/assets/prometheus_logo_grey.svg",
  "text": "{{ if eq .Status "firing" }}**FIRING**{{ else }}**RESOLVED**{{ end }} - {{ .GroupLabels.alertname }}",
  "attachments": [
    {
      "color": "{{ if eq .Status "firing" }}#ff0000{{ else }}#36a64f{{ end }}",
      "title": "{{ .CommonAnnotations.summary }}",
      "text": "{{ .CommonAnnotations.description }}",
      "fields": [
        {
          "title": "Status",
          "value": "{{ .Status | upper }}",
          "short": true
        },
        {
          "title": "Severity", 
          "value": "{{ .CommonLabels.severity | default "unknown" }}",
          "short": true
        },
        {
          "title": "Alert Count",
          "value": "{{ len .Alerts }}",
          "short": true
        },
        {
          "title": "Receiver",
          "value": "{{ .Receiver }}",
          "short": true
        }{{ if gt (len .Alerts) 1 }},
        {
          "title": "Alerts",
          "value": "{{ range $i, $alert := .Alerts }}{{ if $i }}, {{ end }}{{ $alert.Labels.alertname | default "Unknown" }}{{ if $alert.Labels.instance }} ({{ $alert.Labels.instance }}){{ end }}{{ end }}",
          "short": false
        }{{ end }}{{ if .CommonAnnotations.runbook_url }},
        {
          "title": "Runbook",
          "value": "[View Documentation]({{ .CommonAnnotations.runbook_url }})",
          "short": false
        }{{ end }}
      ],
      "footer": "Alertmanager | Group: {{ .GroupKey }}",
      "ts": "{{ now.Format "2006-01-02T15:04:05Z07:00" }}"{{ if .ExternalURL }},
      "title_link": "{{ .ExternalURL }}"{{ end }}
    }
  ]
}`

// GetExampleTemplate returns an example template for a given service
func GetExampleTemplate(service string) string {
	switch strings.ToLower(service) {
	case "flock":
		return FlockWebhookTemplate
	case "jenkins":
		return JenkinsWebhookTemplate
	case "jenkins-build", "jenkins-buildwithparameters":
		return JenkinsBuildTemplate
	case "slack":
		return SlackWebhookTemplate
	case "teams", "msteams":
		return TeamsWebhookTemplate
	case "telegram":
		return TelegramBotTemplate
	case "discord":
		return DiscordWebhookTemplate
	case "splunk":
		return SplunkWebhookTemplate
	case "victorialogs", "victoria-logs", "vlogs":
		return VictoriaLogsWebhookTemplate
	case "github", "github-actions", "github-action":
		return GitHubActionsTemplate
	case "mattermost":
		return MattermostWebhookTemplate
	case "rocketchat", "rocket-chat":
		return RocketChatWebhookTemplate
	case "debug":
		return WebhookDebugTemplate
	default:
		return ""
	}
}

// GetExampleJQConfig returns an example destination configuration using jq engine
func GetExampleJQConfig(service string) *config.DestinationConfig {
	switch strings.ToLower(service) {
	case "flock":
		return &config.DestinationConfig{
			Name:        "flock-alerts-jq",
			Method:      http.MethodPost,
			URL:         "https://api.flock.com/hooks/sendMessage/${FLOCK_WEBHOOK_ID}",
			Format:      "json",
			Engine:      "jq",
			Transform:   FlockWebhookJQTemplate,
			SplitAlerts: false,
		}
	case "jenkins":
		return &config.DestinationConfig{
			Name:        "jenkins-trigger-jq",
			Method:      http.MethodPost,
			URL:         "https://jenkins.example.com/generic-webhook-trigger/invoke?token=${JENKINS_TOKEN}",
			Format:      "json",
			Engine:      "jq",
			Transform:   JenkinsWebhookJQTemplate,
			SplitAlerts: false,
			Headers: map[string]string{
				"X-Jenkins-Token": "${JENKINS_TOKEN}",
			},
		}
	case "jenkins-build", "jenkins-buildwithparameters":
		return &config.DestinationConfig{
			Name:        "jenkins-build-jq",
			Method:      http.MethodPost,
			URL:         "https://${JENKINS_USER}:${JENKINS_API_TOKEN}@jenkins.example.com/job/${JENKINS_JOB_NAME}/buildWithParameters",
			Format:      "form",
			Engine:      "jq",
			Transform:   JenkinsBuildJQTemplate,
			SplitAlerts: true, // Each alert triggers a separate build
			Headers: map[string]string{
				"Content-Type": "application/x-www-form-urlencoded",
			},
		}
	case "slack":
		return &config.DestinationConfig{
			Name:        "slack-alerts-jq",
			Method:      http.MethodPost,
			URL:         "https://hooks.slack.com/services/${SLACK_WEBHOOK_ID}",
			Format:      "json",
			Engine:      "jq",
			Transform:   SlackWebhookJQTemplate,
			SplitAlerts: false,
		}
	case "telegram":
		return &config.DestinationConfig{
			Name:        "telegram-bot-jq",
			Method:      http.MethodPost,
			URL:         "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage",
			Format:      "json",
			Engine:      "jq",
			Transform:   TelegramBotJQTemplate,
			SplitAlerts: false, // Group alerts in single message
		}
	case "discord":
		return &config.DestinationConfig{
			Name:        "discord-webhook-jq",
			Method:      http.MethodPost,
			URL:         "${DISCORD_WEBHOOK_URL}",
			Format:      "json",
			Engine:      "jq",
			Transform:   DiscordWebhookJQTemplate,
			SplitAlerts: false, // Group alerts in single embed
		}
	case "mattermost":
		return &config.DestinationConfig{
			Name:        "mattermost-alerts-jq",
			Method:      http.MethodPost,
			URL:         "${MATTERMOST_WEBHOOK_URL}",
			Format:      "json",
			Engine:      "jq",
			Transform:   MattermostWebhookJQTemplate,
			SplitAlerts: false, // Group alerts in single message
		}
	case "rocketchat", "rocket-chat":
		return &config.DestinationConfig{
			Name:        "rocketchat-alerts-jq",
			Method:      http.MethodPost,
			URL:         "${ROCKETCHAT_WEBHOOK_URL}",
			Format:      "json",
			Engine:      "jq",
			Transform:   RocketChatWebhookJQTemplate,
			SplitAlerts: false, // Group alerts in single message
		}
	case "splunk":
		return &config.DestinationConfig{
			Name:        "splunk-hec-jq",
			Method:      http.MethodPost,
			URL:         "https://splunk.example.com:8088/services/collector/event",
			Format:      "json",
			Engine:      "jq",
			Transform:   SplunkWebhookJQTemplate,
			SplitAlerts: true, // Splunk works better with individual events
			Headers: map[string]string{
				"Authorization": "Splunk ${SPLUNK_HEC_TOKEN}",
			},
		}
	case "victorialogs", "victoria-logs", "vlogs":
		return &config.DestinationConfig{
			Name:        "victoria-logs-jq",
			Method:      http.MethodPost,
			URL:         "https://victorialogs.example.com:9428/insert/jsonline",
			Format:      "json",
			Engine:      "jq",
			Transform:   VictoriaLogsWebhookJQTemplate,
			SplitAlerts: true, // VictoriaLogs works better with individual log entries
			Headers: map[string]string{
				"AccountID": "${VICTORIA_LOGS_ACCOUNT_ID}",
				"ProjectID": "${VICTORIA_LOGS_PROJECT_ID}",
			},
		}
	case "github", "github-actions", "github-action":
		return &config.DestinationConfig{
			Name:        "github-actions-jq",
			Method:      http.MethodPost,
			URL:         "https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/dispatches",
			Format:      "json",
			Engine:      "jq",
			Transform:   GitHubActionsJQTemplate,
			SplitAlerts: false, // Group alerts in single dispatch event
			Headers: map[string]string{
				"Authorization":       "Bearer ${GITHUB_TOKEN}",
				"Accept":              "application/vnd.github.v3+json",
				"X-GitHub-Media-Type": "github.v3",
			},
		}
	default:
		return nil
	}
}

// GetExampleConfig returns an example destination configuration
func GetExampleConfig(service string) *config.DestinationConfig {
	switch strings.ToLower(service) {
	case "flock":
		return &config.DestinationConfig{
			Name:        "flock-alerts",
			Method:      http.MethodPost,
			URL:         "https://api.flock.com/hooks/sendMessage/${FLOCK_WEBHOOK_ID}",
			Format:      "json",
			Engine:      "go-template",
			Template:    FlockWebhookTemplate,
			SplitAlerts: false,
		}
	case "jenkins":
		return &config.DestinationConfig{
			Name:        "jenkins-trigger",
			Method:      http.MethodPost,
			URL:         "https://jenkins.example.com/generic-webhook-trigger/invoke?token=${JENKINS_TOKEN}",
			Format:      "json",
			Engine:      "go-template",
			Template:    JenkinsWebhookTemplate,
			SplitAlerts: false,
			Headers: map[string]string{
				"X-Jenkins-Token": "${JENKINS_TOKEN}",
			},
		}
	case "jenkins-build", "jenkins-buildwithparameters":
		return &config.DestinationConfig{
			Name:        "jenkins-build",
			Method:      http.MethodPost,
			URL:         "https://${JENKINS_USER}:${JENKINS_API_TOKEN}@jenkins.example.com/job/${JENKINS_JOB_NAME}/buildWithParameters",
			Format:      "form",
			Engine:      "go-template",
			Template:    JenkinsBuildTemplate,
			SplitAlerts: true, // Each alert triggers a separate build
			Headers: map[string]string{
				"Content-Type": "application/x-www-form-urlencoded",
			},
		}
	case "slack":
		return &config.DestinationConfig{
			Name:        "slack-alerts",
			Method:      http.MethodPost,
			URL:         "https://hooks.slack.com/services/${SLACK_WEBHOOK_ID}",
			Format:      "json",
			Engine:      "go-template",
			Template:    SlackWebhookTemplate,
			SplitAlerts: false,
		}
	case "telegram":
		return &config.DestinationConfig{
			Name:        "telegram-bot",
			Method:      http.MethodPost,
			URL:         "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage",
			Format:      "json",
			Engine:      "go-template",
			Template:    TelegramBotTemplate,
			SplitAlerts: false, // Group alerts in single message
		}
	case "discord":
		return &config.DestinationConfig{
			Name:        "discord-webhook",
			Method:      http.MethodPost,
			URL:         "${DISCORD_WEBHOOK_URL}",
			Format:      "json",
			Engine:      "go-template",
			Template:    DiscordWebhookTemplate,
			SplitAlerts: false, // Group alerts in single embed
		}
	case "splunk":
		return &config.DestinationConfig{
			Name:        "splunk-hec",
			Method:      http.MethodPost,
			URL:         "https://splunk.example.com:8088/services/collector/event",
			Format:      "json",
			Engine:      "go-template",
			Template:    SplunkWebhookTemplate,
			SplitAlerts: true, // Splunk works better with individual events
			Headers: map[string]string{
				"Authorization": "Splunk ${SPLUNK_HEC_TOKEN}",
			},
		}
	case "victorialogs", "victoria-logs", "vlogs":
		return &config.DestinationConfig{
			Name:        "victoria-logs",
			Method:      http.MethodPost,
			URL:         "https://victorialogs.example.com:9428/insert/jsonline",
			Format:      "json",
			Engine:      "go-template",
			Template:    VictoriaLogsWebhookTemplate,
			SplitAlerts: true, // VictoriaLogs works better with individual log entries
			Headers: map[string]string{
				"AccountID": "${VICTORIA_LOGS_ACCOUNT_ID}",
				"ProjectID": "${VICTORIA_LOGS_PROJECT_ID}",
			},
		}
	case "github", "github-actions", "github-action":
		return &config.DestinationConfig{
			Name:        "github-actions",
			Method:      http.MethodPost,
			URL:         "https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/dispatches",
			Format:      "json",
			Engine:      "go-template",
			Template:    GitHubActionsTemplate,
			SplitAlerts: false, // Group alerts in single dispatch event
			Headers: map[string]string{
				"Authorization":       "Bearer ${GITHUB_TOKEN}",
				"Accept":              "application/vnd.github.v3+json",
				"X-GitHub-Media-Type": "github.v3",
			},
		}
	case "mattermost":
		return &config.DestinationConfig{
			Name:        "mattermost-alerts",
			Method:      http.MethodPost,
			URL:         "${MATTERMOST_WEBHOOK_URL}",
			Format:      "json",
			Engine:      "go-template",
			Template:    MattermostWebhookTemplate,
			SplitAlerts: false, // Group alerts in single message
		}
	case "rocketchat", "rocket-chat":
		return &config.DestinationConfig{
			Name:        "rocketchat-alerts",
			Method:      http.MethodPost,
			URL:         "${ROCKETCHAT_WEBHOOK_URL}",
			Format:      "json",
			Engine:      "go-template",
			Template:    RocketChatWebhookTemplate,
			SplitAlerts: false, // Group alerts in single message
		}
	default:
		return nil
	}
}
