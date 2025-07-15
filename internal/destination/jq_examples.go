package destination

// JQ transformation examples equivalent to go-template examples

// Service-specific jq templates equivalent to go-template versions
const (
	// FlockWebhookJQTemplate is the jq equivalent of FlockWebhookTemplate
	FlockWebhookJQTemplate = `{
  text: ("🚨 Alert: " + .groupLabels.alertname + " [" + (.status | ascii_upcase) + "]"),
  attachments: [{
    title: .commonAnnotations.summary,
    description: ([.alerts[] | if .annotations.description then ("• " + .annotations.description) else empty end] | join("\n")),
    color: (if .status == "firing" then "#ff0000" else "#36a64f" end),
    views: {
      flockml: ("<flockml>" + 
        (if .commonAnnotations.runbook_url then ("<a href=\"" + .commonAnnotations.runbook_url + "\">📖 Runbook</a> | ") else "" end) +
        "<a href=\"" + .externalURL + "\">🔗 Alertmanager</a></flockml>")
    },
    forwards: true
  }],
  flockml: ("AlertCount: <strong>" + (.alerts | length | tostring) + "</strong> | Receiver: <em>" + .receiver + "</em>")
}`

	// JenkinsWebhookJQTemplate is the jq equivalent of JenkinsWebhookTemplate
	JenkinsWebhookJQTemplate = `{
  alertname: .groupLabels.alertname,
  status: .status,
  severity: (.commonLabels.severity // "unknown"),
  alertCount: (.alerts | length),
  groupKey: .groupKey,
  commonLabels: .commonLabels,
  commonAnnotations: .commonAnnotations,
  alerts: [
    .alerts[] | {
      status: .status,
      labels: .labels,
      annotations: .annotations,
      startsAt: (.startsAt // ""),
      endsAt: (.endsAt // ""),
      fingerprint: .fingerprint
    }
  ]
}`

	// JenkinsBuildJQTemplate is the jq equivalent of JenkinsBuildTemplate (for split mode)
	JenkinsBuildJQTemplate = `
("ALERT_NAME=" + (.alert.labels.alertname // "" | @uri)) + "\n" +
("STATUS=" + (.alert.status // "" | ascii_upcase | @uri)) + "\n" +
("SEVERITY=" + (.alert.labels.severity // "unknown" | @uri)) + "\n" +
("ALERT_COUNT=1") + "\n" +
("GROUP_KEY=" + (.payload.groupKey // "" | @uri)) + "\n" +
("RECEIVER=" + (.payload.receiver // "" | @uri)) + "\n" +
("SUMMARY=" + (.alert.annotations.summary // "" | @uri)) + "\n" +
("DESCRIPTION=" + (.alert.annotations.description // "" | @uri)) + "\n" +
("EXTERNAL_URL=" + (.payload.externalURL // "" | @uri)) + "\n" +
(.alert.labels | to_entries[] | "LABEL_" + (.key | ascii_upcase | gsub("-"; "_") | gsub("\\."; "_")) + "=" + (.value | @uri)) + "\n" +
(.alert.annotations | to_entries[] | "ANNOTATION_" + (.key | ascii_upcase | gsub("-"; "_") | gsub("\\."; "_")) + "=" + (.value | @uri)) + "\n" +
("ALERT_FINGERPRINT=" + (.alert.fingerprint // "" | @uri)) + "\n" +
("ALERT_STARTS_AT=" + (.alert.startsAt // "" | @uri))
`

	// SlackWebhookJQTemplate is the jq equivalent of SlackWebhookTemplate
	SlackWebhookJQTemplate = `{
  text: ("*Alert:* " + .groupLabels.alertname + " - " + (.status | ascii_upcase)),
  attachments: [{
    color: (if .status == "firing" then "danger" else "good" end),
    title: .commonAnnotations.summary,
    text: .commonAnnotations.description,
    fields: [
      {
        title: "Severity",
        value: (.commonLabels.severity // "unknown"),
        short: true
      },
      {
        title: "Alert Count",
        value: (.alerts | length | tostring),
        short: true
      }
    ],
    footer: "Alertmanager",
    footer_icon: "https://prometheus.io/assets/prometheus_logo_grey.svg",
    ts: now
  }]
}`

	// TeamsWebhookJQTemplate is the jq equivalent of TeamsWebhookTemplate
	TeamsWebhookJQTemplate = `{
  "@type": "MessageCard",
  "@context": "https://schema.org/extensions",
  summary: (.groupLabels.alertname + " - " + .status),
  themeColor: (if .status == "firing" then "FF0000" else "00FF00" end),
  title: .groupLabels.alertname,
  sections: [{
    activityTitle: .commonAnnotations.summary,
    activitySubtitle: .commonAnnotations.description,
    facts: [
      {
        name: "Status:",
        value: (.status | ascii_upcase)
      },
      {
        name: "Severity:",
        value: (.commonLabels.severity // "unknown")
      },
      {
        name: "Alert Count:",
        value: (.alerts | length | tostring)
      }
    ]
  }],
  potentialAction: [{
    "@type": "OpenUri",
    name: "View in Alertmanager",
    targets: [{
      os: "default",
      uri: .externalURL
    }]
  }]
}`

	// TelegramBotJQTemplate is the jq equivalent of TelegramBotTemplate
	TelegramBotJQTemplate = `{
  chat_id: "${TELEGRAM_CHAT_ID}",
  parse_mode: "Markdown",
  disable_web_page_preview: true,
  text: (
    (if .status == "firing" then "🚨" else "✅" end) + " *" + 
    (.groupLabels.alertname | gsub("_"; "\\\\_") | gsub("\\*"; "\\\\*")) + "*\n\n" +
    (if .commonAnnotations.summary then ("📝 " + (.commonAnnotations.summary | gsub("_"; "\\\\_") | gsub("\\*"; "\\\\*")) + "\n") else "" end) +
    (if .commonAnnotations.description then ("📋 " + (.commonAnnotations.description | gsub("_"; "\\\\_") | gsub("\\*"; "\\\\*")) + "\n") else "" end) +
    "\n🔹 *Status:* " + (.status | ascii_upcase) + 
    "\n🔹 *Severity:* " + ((.commonLabels.severity // "unknown") | gsub("_"; "\\\\_") | gsub("\\*"; "\\\\*")) +
    "\n🔹 *Alert Count:* " + (.alerts | length | tostring) +
    "\n🔹 *Receiver:* " + (.receiver | gsub("_"; "\\\\_") | gsub("\\*"; "\\\\*")) +
    (if .externalURL then ("\n\n[🔗 View in Alertmanager](" + .externalURL + ")") else "" end) +
    (if (.alerts | length) > 1 then 
      ("\n\n*Alerts:*" + 
       ([.alerts[] | ". " + ((.labels.alertname // "Unknown") | gsub("_"; "\\\\_") | gsub("\\*"; "\\\\*")) + 
        (if .labels.instance then (" (" + (.labels.instance | gsub("_"; "\\\\_") | gsub("\\*"; "\\\\*")) + ")") else "" end)] | 
        to_entries | map("\n" + (.key + 1 | tostring) + .value) | join("")))
    else "" end)
  )
}`

	// DiscordWebhookJQTemplate is the jq equivalent of DiscordWebhookTemplate
	DiscordWebhookJQTemplate = `{
  username: "Alertmanager",
  avatar_url: "https://prometheus.io/assets/prometheus_logo_grey.svg",
  embeds: [{
    title: ((if .status == "firing" then "🚨" else "✅" end) + " " + .groupLabels.alertname),
    description: .commonAnnotations.summary,
    color: (if .status == "firing" then 15158332 else 3066993 end),
    fields: [
      {
        name: "Status",
        value: (.status | ascii_upcase),
        inline: true
      },
      {
        name: "Severity", 
        value: (.commonLabels.severity // "unknown"),
        inline: true
      },
      {
        name: "Alert Count",
        value: (.alerts | length | tostring),
        inline: true
      }
    ] + (if .commonAnnotations.description then [{
      name: "Description",
      value: .commonAnnotations.description,
      inline: false
    }] else [] end),
    footer: {
      text: ("Receiver: " + .receiver + " | Group: " + .groupKey)
    },
    timestamp: (now | strftime("%Y-%m-%dT%H:%M:%SZ"))
  } + (if .externalURL then {url: .externalURL} else {} end)]
}`

	// MattermostWebhookJQTemplate is the jq equivalent of MattermostWebhookTemplate
	MattermostWebhookJQTemplate = `{
  channel: "${MATTERMOST_CHANNEL}",
  username: "Alertmanager",
  icon_url: "https://prometheus.io/assets/prometheus_logo_grey.svg",
  text: ((if .status == "firing" then "🚨 **FIRING**" else "✅ **RESOLVED**" end) + " - " + .groupLabels.alertname),
  attachments: [{
    color: (if .status == "firing" then "danger" else "good" end),
    title: .commonAnnotations.summary,
    text: .commonAnnotations.description,
    fields: [
      {
        title: "Status",
        value: (.status | ascii_upcase),
        short: true
      },
      {
        title: "Severity",
        value: (.commonLabels.severity // "unknown"),
        short: true
      },
      {
        title: "Alert Count",
        value: (.alerts | length | tostring),
        short: true
      },
      {
        title: "Receiver",
        value: .receiver,
        short: true
      }
    ] + (if .commonAnnotations.runbook_url then [{
      title: "Runbook",
      value: ("[View Runbook](" + .commonAnnotations.runbook_url + ")"),
      short: false
    }] else [] end),
    footer: "Alertmanager",
    footer_icon: "https://prometheus.io/assets/prometheus_logo_grey.svg",
    ts: now
  } + (if .externalURL then {title_link: .externalURL} else {} end)]
}`

	// RocketChatWebhookJQTemplate is the jq equivalent of RocketChatWebhookTemplate
	RocketChatWebhookJQTemplate = `{
  channel: "${ROCKETCHAT_CHANNEL}",
  username: "Alertmanager", 
  avatar: "https://prometheus.io/assets/prometheus_logo_grey.svg",
  text: ((if .status == "firing" then "🚨 **FIRING**" else "✅ **RESOLVED**" end) + " - " + .groupLabels.alertname),
  attachments: [{
    color: (if .status == "firing" then "#ff0000" else "#36a64f" end),
    title: .commonAnnotations.summary,
    text: .commonAnnotations.description,
    fields: [
      {
        title: "Status",
        value: (.status | ascii_upcase),
        short: true
      },
      {
        title: "Severity",
        value: (.commonLabels.severity // "unknown"),
        short: true
      },
      {
        title: "Alert Count", 
        value: (.alerts | length | tostring),
        short: true
      },
      {
        title: "Receiver",
        value: .receiver,
        short: true
      }
    ] + (if (.alerts | length) > 1 then [{
      title: "Alerts",
      value: ([.alerts[] | ((.labels.alertname // "Unknown") + (if .labels.instance then (" (" + .labels.instance + ")") else "" end))] | join(", ")),
      short: false
    }] else [] end) + (if .commonAnnotations.runbook_url then [{
      title: "Runbook", 
      value: ("[View Documentation](" + .commonAnnotations.runbook_url + ")"),
      short: false
    }] else [] end),
    footer: ("Alertmanager | Group: " + .groupKey),
    ts: (now | strftime("%Y-%m-%dT%H:%M:%SZ"))
  } + (if .externalURL then {title_link: .externalURL} else {} end)]
}`

	// SplunkWebhookJQTemplate is the jq equivalent of SplunkWebhookTemplate (for split mode)
	SplunkWebhookJQTemplate = `{
  time: now,
  host: "alertmanager",
  source: "alertmanager:webhook",
  sourcetype: "prometheus:alert",
  event: {
    alert_name: .alert.labels.alertname,
    status: .alert.status,
    severity: (.alert.labels.severity // "unknown"),
    group_key: .payload.groupKey,
    receiver: .payload.receiver,
    summary: .alert.annotations.summary,
    description: .alert.annotations.description,
    alert_count: 1,
    firing_alerts: 1,
    resolved_alerts: 0,
    common_labels: .payload.commonLabels,
    common_annotations: .payload.commonAnnotations,
    external_url: .payload.externalURL,
    alert_details: {
      status: .alert.status,
      fingerprint: .alert.fingerprint,
      starts_at: .alert.startsAt,
      ends_at: (if .alert.endsAt == "0001-01-01T00:00:00Z" then null else .alert.endsAt end),
      labels: .alert.labels,
      annotations: .alert.annotations
    }
  }
}`

	// VictoriaLogsWebhookJQTemplate is the jq equivalent of VictoriaLogsWebhookTemplate (for split mode)
	VictoriaLogsWebhookJQTemplate = `{
  _time: (now | strftime("%Y-%m-%dT%H:%M:%S.000Z")),
  _msg: ("Prometheus Alert: " + .alert.labels.alertname + " - " + .alert.status),
  alert_name: .alert.labels.alertname,
  status: .alert.status,
  severity: (.alert.labels.severity // "unknown"),
  service: "alertmanager",
  source: "webhook",
  group_key: .payload.groupKey,
  receiver: .payload.receiver,
  summary: .alert.annotations.summary,
  description: .alert.annotations.description,
  alert_count: 1,
  firing_count: 1,
  resolved_count: 0,
  external_url: .payload.externalURL,
  raw_alert: .alert
}`

	// GitHubActionsJQTemplate is the jq equivalent of GitHubActionsTemplate
	GitHubActionsJQTemplate = `{
  event_type: "prometheus-alert",
  client_payload: {
    alert_name: .groupLabels.alertname,
    status: .status,
    severity: (.commonLabels.severity // "unknown"),
    summary: .commonAnnotations.summary,
    description: .commonAnnotations.description,
    group_key: .groupKey,
    receiver: .receiver,
    alert_count: (.alerts | length),
    firing_count: (if .status == "firing" then (.alerts | length) else 0 end),
    resolved_count: (if .status == "resolved" then (.alerts | length) else 0 end),
    external_url: .externalURL,
    timestamp: (now | strftime("%Y-%m-%dT%H:%M:%SZ")),
    labels: .commonLabels,
    annotations: .commonAnnotations,
    alerts: [
      .alerts[] | {
        status: .status,
        fingerprint: .fingerprint,
        starts_at: .startsAt,
        ends_at: (if .endsAt == "0001-01-01T00:00:00Z" then null else .endsAt end),
        generator_url: .generatorURL,
        labels: .labels,
        annotations: .annotations
      }
    ]
  }
}`

	// WebhookDebugJQTemplate is the jq equivalent of WebhookDebugTemplate
	WebhookDebugJQTemplate = `{
  webhook_version: .version,
  group_key: .groupKey,
  status: .status,
  receiver: .receiver,
  group_labels: .groupLabels,
  common_labels: .commonLabels,
  common_annotations: .commonAnnotations,
  external_url: .externalURL,
  alert_count: (.alerts | length),
  truncated_alerts: .truncatedAlerts,
  alerts: .alerts
}`
)

// Simple jq examples for basic transformations
const (
	// JQSimpleStatusExample extracts just the status
	JQSimpleStatusExample = `.status`

	// JQBasicAlertExample creates a simple alert object
	JQBasicAlertExample = `{
  status: .status,
  alertname: .groupLabels.alertname,
  count: (.alerts | length)
}`

	// JQSlackExample creates Slack-compatible JSON using jq
	JQSlackExample = `{
  text: ("*Alert:* " + .groupLabels.alertname + " - " + (.status | ascii_upcase)),
  attachments: [
    {
      color: (if .status == "firing" then "danger" else "good" end),
      title: .commonAnnotations.summary,
      text: .commonAnnotations.description,
      fields: [
        {
          title: "Severity",
          value: (.commonLabels.severity // "unknown"),
          short: true
        },
        {
          title: "Alert Count", 
          value: (.alerts | length | tostring),
          short: true
        }
      ],
      footer: "Alertmanager",
      ts: now
    }
  ]
}`

	// JQFilterCriticalExample filters only critical alerts
	JQFilterCriticalExample = `{
  status: .status,
  critical_alerts: [.alerts[] | select(.labels.severity == "critical") | .labels.alertname],
  count: [.alerts[] | select(.labels.severity == "critical")] | length
}`

	// JQGroupBySeverityExample groups alerts by severity
	JQGroupBySeverityExample = `{
  status: .status,
  grouped: (.alerts | group_by(.labels.severity) | map({
    severity: .[0].labels.severity,
    count: length,
    alerts: map(.labels.alertname)
  }))
}`

	// JQCustomFormatExample creates a custom formatted message
	JQCustomFormatExample = `{
  message: ("🚨 " + (.alerts | length | tostring) + " alerts are " + .status),
  details: {
    receiver: .receiver,
    group_key: .groupKey,
    firing_alerts: [.alerts[] | select(.status == "firing") | .labels.alertname],
    resolved_alerts: [.alerts[] | select(.status == "resolved") | .labels.alertname]
  },
  timestamp: now
}`

	// JQAlertSplitExample for use with split alert mode
	JQAlertSplitExample = `{
  fingerprint: .alert.fingerprint,
  alertname: .alert.labels.alertname,
  status: .alert.status,
  severity: .alert.labels.severity,
  description: .alert.annotations.description,
  starts_at: .alert.startsAt,
  group_key: .payload.groupKey,
  receiver: .payload.receiver
}`
)

// GetJQExample returns example jq queries for different use cases
func GetJQExample(name string) string {
	switch name {
	// Service-specific templates (jq equivalents of go-template examples)
	case "flock":
		return FlockWebhookJQTemplate
	case "jenkins":
		return JenkinsWebhookJQTemplate
	case "jenkins-build", "jenkins-buildwithparameters":
		return JenkinsBuildJQTemplate
	case "slack":
		return SlackWebhookJQTemplate
	case "teams", "msteams":
		return TeamsWebhookJQTemplate
	case "telegram":
		return TelegramBotJQTemplate
	case "discord":
		return DiscordWebhookJQTemplate
	case "mattermost":
		return MattermostWebhookJQTemplate
	case "rocketchat", "rocket-chat":
		return RocketChatWebhookJQTemplate
	case "splunk":
		return SplunkWebhookJQTemplate
	case "victorialogs", "victoria-logs", "vlogs":
		return VictoriaLogsWebhookJQTemplate
	case "github", "github-actions", "github-action":
		return GitHubActionsJQTemplate
	case "debug":
		return WebhookDebugJQTemplate
	// Original pattern examples
	case "simple-status":
		return JQSimpleStatusExample
	case "basic-alert":
		return JQBasicAlertExample
	case "slack-example":
		return JQSlackExample
	case "filter-critical":
		return JQFilterCriticalExample
	case "group-by-severity":
		return JQGroupBySeverityExample
	case "custom-format":
		return JQCustomFormatExample
	case "alert-split":
		return JQAlertSplitExample
	default:
		return ""
	}
}
