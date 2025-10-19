# Usage Examples

This guide provides practical examples of using the Alertmanager Gateway in various scenarios.

## Table of Contents

- [Basic Setup](#basic-setup)
- [Template Examples](#template-examples)
- [JQ Transform Examples](#jq-transform-examples)
- [Alert Routing](#alert-routing)
- [Testing and Validation](#testing-and-validation)
- [Production Patterns](#production-patterns)
- [Integration Recipes](#integration-recipes)

## Basic Setup

### Simple Slack Integration

1. **Create configuration file** (`config.yaml`):
```yaml
server:
  port: 8080

destinations:
  - name: slack
    url: "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
    method: POST
    format: json
    engine: go-template
    template: |
      {
        "text": "Alert: {{.GroupLabels.alertname}} is {{.Status}}"
      }
    enabled: true
```

2. **Start the gateway**:
```bash
./alertmanager-gateway -config config.yaml
```

3. **Configure Alertmanager** (`alertmanager.yml`):
```yaml
route:
  receiver: 'gateway'

receivers:
  - name: 'gateway'
    webhook_configs:
      - url: 'http://localhost:8080/webhook/slack'
```

4. **Test with sample alert**:
```bash
curl -X POST http://localhost:8080/webhook/slack \
  -H "Content-Type: application/json" \
  -d '{
    "version": "4",
    "groupKey": "test",
    "status": "firing",
    "groupLabels": {"alertname": "TestAlert"},
    "alerts": []
  }'
```

## Template Examples

### Basic Alert Information

```yaml
engine: go-template
template: |
  {
    "alert_name": "{{.GroupLabels.alertname}}",
    "status": "{{.Status}}",
    "severity": "{{index .CommonLabels "severity" | default "unknown"}}",
    "alert_count": {{len .Alerts}},
    "timestamp": {{now | unixtime}}
  }
```

### Formatted Message with Emoji

```yaml
engine: go-template
template: |
  {
    "text": "{{if eq .Status "firing"}}🚨 ALERT{{else}}✅ RESOLVED{{end}}: {{.GroupLabels.alertname}}",
    "details": "{{.CommonAnnotations.summary}}",
    "affected_count": {{len .Alerts}}
  }
```

### Iterating Over Alerts

```yaml
engine: go-template
template: |
  {
    "alerts": [
      {{range $i, $alert := .Alerts}}
      {{if $i}},{{end}}
      {
        "instance": "{{$alert.Labels.instance}}",
        "severity": "{{$alert.Labels.severity}}",
        "started": "{{$alert.StartsAt | date "2006-01-02T15:04:05Z"}}",
        "summary": {{$alert.Annotations.summary | json}}
      }
      {{end}}
    ]
  }
```

### Conditional Formatting

```yaml
engine: go-template
template: |
  {
    "priority": "{{if eq (index .CommonLabels "severity") "critical"}}P1{{else if eq (index .CommonLabels "severity") "warning"}}P2{{else}}P3{{end}}",
    "notify": {{if eq (index .CommonLabels "severity") "critical"}}true{{else}}false{{end}},
    "color": "{{if eq .Status "firing"}}{{if eq (index .CommonLabels "severity") "critical"}}#FF0000{{else}}#FFA500{{end}}{{else}}#00FF00{{end}}"
  }
```

### Time-Based Logic

```yaml
engine: go-template
template: |
  {{$hour := now | date "15"}}
  {{$isBusinessHours := false}}
  {{if and (ge $hour "09") (lt $hour "18")}}
    {{$isBusinessHours = true}}
  {{end}}
  {
    "alert": "{{.GroupLabels.alertname}}",
    "urgent": {{if and (eq (index .CommonLabels "severity") "critical") $isBusinessHours}}true{{else}}false{{end}},
    "notify_channel": "{{if $isBusinessHours}}#alerts{{else}}#alerts-oncall{{end}}"
  }
```

## JQ Transform Examples

### Basic Transformation

```yaml
engine: jq
transform: |
  {
    alert: .groupLabels.alertname,
    status: .status,
    count: (.alerts | length)
  }
```

### Filtering Alerts

```yaml
engine: jq
transform: |
  {
    critical_alerts: [.alerts[] | select(.labels.severity == "critical")],
    warning_alerts: [.alerts[] | select(.labels.severity == "warning")],
    summary: {
      total: (.alerts | length),
      critical: ([.alerts[] | select(.labels.severity == "critical")] | length),
      warning: ([.alerts[] | select(.labels.severity == "warning")] | length)
    }
  }
```

### Complex Data Reshaping

```yaml
engine: jq
transform: |
  {
    notification: {
      title: .groupLabels.alertname + " - " + .status,
      body: .commonAnnotations.summary,
      severity: (.commonLabels.severity // "info"),
      metadata: {
        group_key: .groupKey,
        alert_count: (.alerts | length),
        firing_count: ([.alerts[] | select(.status == "firing")] | length),
        resolved_count: ([.alerts[] | select(.status == "resolved")] | length)
      }
    },
    alerts: .alerts | map({
      id: .fingerprint,
      name: .labels.alertname,
      instance: .labels.instance,
      duration: (
        if .status == "resolved" then
          ((.endsAt | fromdateiso8601) - (.startsAt | fromdateiso8601))
        else
          (now - (.startsAt | fromdateiso8601))
        end
      )
    })
  }
```

### Grouping and Aggregation

```yaml
engine: jq
transform: |
  .alerts |
  group_by(.labels.instance) |
  map({
    instance: .[0].labels.instance,
    alert_count: length,
    alerts: map(.labels.alertname) | unique,
    severities: map(.labels.severity) | unique,
    status: if any(.status == "firing") then "firing" else "resolved" end
  })
```

### Conditional Output

```yaml
engine: jq
transform: |
  if .commonLabels.severity == "critical" and .status == "firing" then
    {
      urgent: true,
      message: "CRITICAL: " + .groupLabels.alertname,
      escalate_to: ["oncall-primary", "oncall-secondary"],
      details: .alerts | map({
        instance: .labels.instance,
        summary: .annotations.summary
      })
    }
  else
    {
      urgent: false,
      message: .groupLabels.alertname + " - " + .status,
      details: (.alerts | length | tostring) + " alerts"
    }
  end
```

## Alert Routing

### Route by Severity

```yaml
destinations:
  # Critical alerts to PagerDuty
  - name: pagerduty-critical
    url: "https://events.pagerduty.com/v2/enqueue"
    engine: jq
    transform: |
      if .commonLabels.severity == "critical" then
        {
          routing_key: "${env:PAGERDUTY_KEY}",
          event_action: "trigger",
          dedup_key: .groupKey,
          payload: {
            summary: .groupLabels.alertname,
            severity: "critical",
            custom_details: .commonAnnotations
          }
        }
      else empty end
    enabled: true

  # Non-critical to Slack
  - name: slack-warnings
    url: "${SLACK_WEBHOOK}"
    engine: jq
    transform: |
      if .commonLabels.severity != "critical" then
        {
          text: .groupLabels.alertname + " [" + .commonLabels.severity + "]",
          attachments: [{
            color: "warning",
            text: .commonAnnotations.summary
          }]
        }
      else empty end
    enabled: true
```

### Route by Environment

```yaml
destinations:
  # Production alerts
  - name: prod-alerts
    url: "${PROD_WEBHOOK}"
    engine: jq
    transform: |
      if .commonLabels.env == "production" then
        {
          alert: .groupLabels.alertname,
          severity: .commonLabels.severity,
          instances: [.alerts[].labels.instance]
        }
      else empty end
    enabled: true

  # Development alerts
  - name: dev-alerts
    url: "${DEV_WEBHOOK}"
    engine: jq
    transform: |
      if .commonLabels.env != "production" then
        {
          environment: .commonLabels.env,
          alert: .groupLabels.alertname,
          count: (.alerts | length)
        }
      else empty end
    enabled: true
```

### Route by Service

```yaml
destinations:
  # Database team
  - name: database-team
    url: "${DB_TEAM_WEBHOOK}"
    engine: jq
    transform: |
      if .commonLabels.service | test("database|mysql|postgres") then
        {
          service: .commonLabels.service,
          alert: .groupLabels.alertname,
          details: .alerts
        }
      else empty end
    enabled: true

  # API team
  - name: api-team
    url: "${API_TEAM_WEBHOOK}"
    engine: jq
    transform: |
      if .commonLabels.service | test("api|gateway|backend") then
        {
          service: .commonLabels.service,
          alert: .groupLabels.alertname,
          endpoints: [.alerts[].labels.endpoint] | unique
        }
      else empty end
    enabled: true
```

## Testing and Validation

### Test Alert Transformation

```bash
# Create test payload
cat > test-alert.json << EOF
{
  "version": "4",
  "groupKey": "test-group",
  "status": "firing",
  "groupLabels": {
    "alertname": "HighCPU"
  },
  "commonLabels": {
    "severity": "warning",
    "env": "production"
  },
  "commonAnnotations": {
    "summary": "CPU usage above 80%"
  },
  "alerts": [{
    "status": "firing",
    "labels": {
      "alertname": "HighCPU",
      "instance": "server-1",
      "severity": "warning"
    },
    "startsAt": "2024-01-01T12:00:00Z"
  }]
}
EOF

# Test transformation
curl -X POST http://localhost:8080/api/v1/test/slack \
  -H "Content-Type: application/json" \
  -d @test-alert.json
```

### Validate Configuration

```bash
# Check specific destination
curl http://localhost:8080/api/v1/destinations/slack \
  -u username:password | jq

# Test with dry run
curl -X POST http://localhost:8080/api/v1/emulate/slack?dry_run=true \
  -H "Content-Type: application/json" \
  -d @test-alert.json
```

### Debugging Templates

```yaml
# Add debug information to template
engine: go-template
template: |
  {
    "debug": {
      "has_alerts": {{if .Alerts}}true{{else}}false{{end}},
      "alert_count": {{len .Alerts}},
      "status": "{{.Status}}",
      "severity": "{{index .CommonLabels "severity" | default "not-set"}}",
      "raw_labels": {{.CommonLabels | json}}
    },
    "message": "{{.GroupLabels.alertname}}"
  }
```

## Production Patterns

### High Availability Configuration

```yaml
server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

destinations:
  - name: primary
    url: "${PRIMARY_WEBHOOK}"
    method: POST
    format: json
    engine: go-template
    template: |
      {"alert": "{{.GroupLabels.alertname}}", "status": "{{.Status}}"}
    enabled: true
```

### Multi-Stage Processing

```yaml
destinations:
  # Stage 1: Enrich alert data
  - name: enrichment
    url: "${ENRICHMENT_SERVICE}"
    engine: jq
    transform: |
      {
        alert: .groupLabels.alertname,
        severity: .commonLabels.severity,
        enrichment_request: {
          instances: [.alerts[].labels.instance],
          service: .commonLabels.service
        }
      }
    enabled: true

  # Stage 2: Route based on enrichment
  - name: routing
    url: "${ROUTING_SERVICE}"
    engine: go-template
    template: |
      {
        "route_to": "{{if eq (index .CommonLabels "severity") "critical"}}oncall{{else}}team{{end}}",
        "alert_data": {{. | json}}
      }
    enabled: true
```

### Batch Processing

```yaml
destinations:
  - name: batch-processor
    url: "${BATCH_ENDPOINT}"
    method: POST
    format: json
    engine: jq
    transform: |
      {
        batch_id: (.groupKey | @base64),
        timestamp: now | todateiso8601,
        alerts: .alerts | map({
          id: .fingerprint,
          name: .labels.alertname,
          instance: .labels.instance,
          severity: .labels.severity
        })
      }
    enabled: true
    split_alerts: false
    batch_size: 100
```

### Alert Deduplication

```yaml
engine: jq
transform: |
  {
    dedup_id: (.groupKey + "_" + .status),
    alert: .groupLabels.alertname,
    status: .status,
    first_seen: (.alerts | min_by(.startsAt) | .startsAt),
    last_seen: (.alerts | max_by(.startsAt) | .startsAt),
    unique_instances: ([.alerts[].labels.instance] | unique),
    occurrence_count: (.alerts | length)
  }
```

## Integration Recipes

### Slack with Rich Formatting

```yaml
engine: go-template
template: |
  {
    "blocks": [
      {
        "type": "header",
        "text": {
          "type": "plain_text",
          "text": "{{if eq .Status "firing"}}🚨 Alert Firing{{else}}✅ Alert Resolved{{end}}"
        }
      },
      {
        "type": "section",
        "fields": [
          {
            "type": "mrkdwn",
            "text": "*Alert:*\n{{.GroupLabels.alertname}}"
          },
          {
            "type": "mrkdwn",
            "text": "*Severity:*\n{{index .CommonLabels "severity" | default "unknown"}}"
          }
        ]
      },
      {
        "type": "section",
        "text": {
          "type": "mrkdwn",
          "text": "*Summary:*\n{{.CommonAnnotations.summary}}"
        }
      },
      {
        "type": "section",
        "text": {
          "type": "mrkdwn",
          "text": "*Affected Instances:*\n{{range .Alerts}}• `{{.Labels.instance}}`\n{{end}}"
        }
      },
      {
        "type": "actions",
        "elements": [
          {
            "type": "button",
            "text": {
              "type": "plain_text",
              "text": "View in Alertmanager"
            },
            "url": "{{.ExternalURL}}"
          }
          {{if .CommonAnnotations.runbook_url}},
          {
            "type": "button",
            "text": {
              "type": "plain_text",
              "text": "Runbook"
            },
            "url": "{{.CommonAnnotations.runbook_url}}"
          }
          {{end}}
        ]
      }
    ]
  }
```

### JIRA Ticket Creation

```yaml
engine: jq
transform: |
  {
    fields: {
      project: { key: "OPS" },
      summary: "[Alert] " + .groupLabels.alertname,
      description: (
        "*Status:* " + .status + "\n" +
        "*Severity:* " + (.commonLabels.severity // "unknown") + "\n" +
        "*Summary:* " + .commonAnnotations.summary + "\n\n" +
        "*Affected Instances:*\n" +
        (.alerts | map("- " + .labels.instance) | join("\n")) + "\n\n" +
        "*Started:* " + (.alerts[0].startsAt) + "\n" +
        "*Alert Group:* " + .groupKey
      ),
      issuetype: { name: (
        if .commonLabels.severity == "critical" then "Bug"
        else "Task" end
      )},
      priority: { name: (
        if .commonLabels.severity == "critical" then "High"
        elif .commonLabels.severity == "warning" then "Medium"
        else "Low" end
      )},
      labels: ["monitoring", "alert", .commonLabels.env],
      components: [{ name: .commonLabels.service }]
    }
  }
```

### Webhook with Retry Logic

```yaml
destinations:
  - name: important-webhook
    url: "${WEBHOOK_URL}"
    method: POST
    format: json
    engine: go-template
    template: |
      {
        "alert": "{{.GroupLabels.alertname}}",
        "timestamp": {{now | unixtime}},
        "retry_safe": true
      }
    enabled: true
      half_open_requests: 3
```

### Custom Metrics Export

```yaml
engine: jq
transform: |
  {
    metrics: [
      {
        name: "alert_total",
        value: (.alerts | length),
        labels: {
          alertname: .groupLabels.alertname,
          status: .status
        }
      },
      {
        name: "alert_severity_count",
        value: ([.alerts[] | select(.labels.severity == "critical")] | length),
        labels: {
          alertname: .groupLabels.alertname,
          severity: "critical"
        }
      }
    ],
    timestamp: now
  }
```