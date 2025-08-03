# Configuration Guide

## Overview

The Alertmanager Gateway is configured using a YAML file that defines server settings, authentication, and destination configurations. This guide provides comprehensive examples for various use cases.

## Configuration Structure

```yaml
# Server configuration
server:
  port: 8080                    # HTTP server port
  read_timeout: 10s             # Maximum duration for reading request
  write_timeout: 10s            # Maximum duration for writing response

# Destination configurations
destinations:
  - name: "destination-name"    # Unique identifier for this destination
    url: "https://example.com"  # Target URL (supports env vars)
    method: "POST"              # HTTP method: POST, PUT, PATCH
    format: "json"              # Output format: json, form, query, xml
    engine: "go-template"       # Template engine: go-template, jq
    template: ""                # Go template (when engine is go-template)
    transform: ""               # JQ query (when engine is jq)
    headers:                    # Custom headers (supports env vars)
      Authorization: "Bearer ${API_TOKEN}"
    enabled: true               # Enable/disable destination
    split_alerts: false         # Send alerts individually
    batch_size: 10              # Alerts per batch (when splitting)
    parallel_requests: 5        # Concurrent requests (when splitting)
    retry:                      # Retry configuration
      max_attempts: 3           # Maximum retry attempts
      backoff: "exponential"    # Backoff strategy: exponential, linear, constant
      initial_delay: 1s         # Initial retry delay
      max_delay: 30s            # Maximum retry delay
    circuit_breaker:            # Circuit breaker configuration
      failure_threshold: 5      # Failures before opening
      success_threshold: 2      # Successes to close
      timeout: 30s              # Time before half-open
      half_open_requests: 3     # Requests in half-open state
```

## Basic Examples

### Slack Integration

Send alerts to Slack using incoming webhooks:

```yaml
server:
  port: 8080

destinations:
  - name: slack
    url: "${SLACK_WEBHOOK_URL}"
    method: POST
    format: json
    engine: go-template
    template: |
      {
        "text": "🚨 Alert: {{.GroupLabels.alertname}} is {{.Status}}",
        "attachments": [
          {
            "color": "{{if eq .Status "firing"}}danger{{else}}good{{end}}",
            "title": "{{.GroupLabels.alertname}}",
            "text": "{{range .Alerts}}• {{.Annotations.summary}}\n{{end}}",
            "fields": [
              {
                "title": "Severity",
                "value": "{{index .CommonLabels "severity" | default "unknown"}}",
                "short": true
              },
              {
                "title": "Alert Count",
                "value": "{{len .Alerts}}",
                "short": true
              }
            ],
            "footer": "Alertmanager",
            "ts": {{now | unixtime}}
          }
        ]
      }
    enabled: true
```

### PagerDuty Integration

Send critical alerts to PagerDuty:

```yaml
destinations:
  - name: pagerduty
    url: "https://events.pagerduty.com/v2/enqueue"
    method: POST
    format: json
    engine: jq
    transform: |
      {
        routing_key: "${env:ROUTING_KEY}",
        event_action: (if .status == "firing" then "trigger" else "resolve" end),
        dedup_key: .groupKey,
        payload: {
          summary: .groupLabels.alertname + " - " + .commonAnnotations.summary,
          severity: (if .commonLabels.severity == "critical" then "critical" 
                    elif .commonLabels.severity == "warning" then "warning"
                    else "info" end),
          source: .externalURL,
          custom_details: {
            firing_alerts: [.alerts[] | select(.status == "firing") | {
              fingerprint: .fingerprint,
              labels: .labels,
              starts_at: .startsAt
            }],
            resolved_alerts: [.alerts[] | select(.status == "resolved") | {
              fingerprint: .fingerprint,
              ends_at: .endsAt
            }]
          }
        }
      } | @json
    headers:
      Content-Type: "application/json"
      Accept: "application/vnd.pagerduty+json;version=2"
    enabled: true
    retry:
      max_attempts: 5
      backoff: exponential
```

### Microsoft Teams Integration

Send formatted alerts to Microsoft Teams:

```yaml
destinations:
  - name: teams
    url: "${TEAMS_WEBHOOK_URL}"
    method: POST
    format: json
    engine: go-template
    template: |
      {
        "@type": "MessageCard",
        "@context": "https://schema.org/extensions",
        "themeColor": "{{if eq .Status "firing"}}D73A49{{else}}28A745{{end}}",
        "summary": "{{.GroupLabels.alertname}} is {{.Status}}",
        "sections": [
          {
            "activityTitle": "{{.GroupLabels.alertname}}",
            "activitySubtitle": "{{.Status | title}} - {{len .Alerts}} alert(s)",
            "facts": [
              {{range $i, $alert := .Alerts}}
              {{if $i}},{{end}}
              {
                "name": "{{$alert.Labels.instance | default "unknown"}}",
                "value": "{{$alert.Annotations.summary | default $alert.Labels.alertname}}"
              }
              {{end}}
            ],
            "markdown": true
          }
        ],
        "potentialAction": [
          {
            "@type": "OpenUri",
            "name": "View in Alertmanager",
            "targets": [
              {
                "os": "default",
                "uri": "{{.ExternalURL}}"
              }
            ]
          }
        ]
      }
    enabled: true
```

## Advanced Examples

### Jenkins Build Trigger

Trigger Jenkins builds with alert parameters:

```yaml
destinations:
  - name: jenkins-build
    url: "https://jenkins.example.com/job/incident-response/buildWithParameters"
    method: POST
    format: form
    engine: go-template
    template: |
      token={{.Env.JENKINS_TOKEN}}&
      ALERT_NAME={{.GroupLabels.alertname}}&
      SEVERITY={{index .CommonLabels "severity" | default "unknown"}}&
      STATUS={{.Status}}&
      ALERT_COUNT={{len .Alerts}}&
      DESCRIPTION={{.CommonAnnotations.summary | urlquery}}&
      ALERTS_JSON={{. | json | urlquery}}
    headers:
      Authorization: "Basic ${JENKINS_AUTH}"
    enabled: true
    retry:
      max_attempts: 3
      backoff: linear
      initial_delay: 2s
```

### Splunk HTTP Event Collector

Send alerts to Splunk HEC:

```yaml
destinations:
  - name: splunk-hec
    url: "https://splunk.example.com:8088/services/collector"
    method: POST
    format: json
    engine: jq
    transform: |
      .alerts[] | {
        time: .startsAt | fromdateiso8601,
        source: "alertmanager",
        sourcetype: "alert",
        index: "alerts",
        event: {
          alert_name: .labels.alertname,
          severity: .labels.severity,
          status: .status,
          instance: .labels.instance,
          summary: .annotations.summary,
          description: .annotations.description,
          fingerprint: .fingerprint,
          labels: .labels,
          annotations: .annotations
        }
      }
    headers:
      Authorization: "Splunk ${SPLUNK_HEC_TOKEN}"
    enabled: true
    split_alerts: true
    parallel_requests: 10
```

### Ticketing System with Alert Splitting

Create individual tickets for each alert:

```yaml
destinations:
  - name: ticketing
    url: "https://api.tickets.example.com/v2/tickets"
    method: POST
    format: json
    engine: jq
    transform: |
      {
        title: .alert.labels.alertname + " - " + .alert.labels.instance,
        description: .alert.annotations.description,
        priority: (
          if .alert.labels.severity == "critical" then "P1"
          elif .alert.labels.severity == "warning" then "P2"
          else "P3" end
        ),
        tags: [.alert.labels | to_entries[] | .key + ":" + .value],
        custom_fields: {
          alert_fingerprint: .alert.fingerprint,
          alert_status: .alert.status,
          started_at: .alert.startsAt,
          alertmanager_group: .groupKey
        }
      }
    headers:
      Authorization: "Bearer ${TICKETING_API_TOKEN}"
      X-Auto-Create: "true"
    enabled: true
    split_alerts: true
    batch_size: 1
    retry:
      max_attempts: 5
      backoff: exponential
      initial_delay: 1s
      max_delay: 30s
```

### Webhook with Custom Headers

Send to a custom webhook with dynamic headers:

```yaml
destinations:
  - name: custom-webhook
    url: "https://api.example.com/webhooks/alerts"
    method: POST
    format: json
    engine: go-template
    template: |
      {
        "notification_id": "{{.GroupKey | base64}}",
        "timestamp": {{now | unixtime}},
        "alert_status": "{{.Status}}",
        "environment": "{{index .CommonLabels "env" | default "production"}}",
        "alerts": [
          {{range $i, $alert := .Alerts}}
          {{if $i}},{{end}}{
            "id": "{{$alert.Fingerprint}}",
            "name": "{{$alert.Labels.alertname}}",
            "severity": "{{$alert.Labels.severity | default "unknown"}}",
            "instance": "{{$alert.Labels.instance | default "unknown"}}",
            "started": "{{$alert.StartsAt.Format "2006-01-02T15:04:05Z07:00"}}",
            "summary": {{$alert.Annotations.summary | json}},
            "labels": {{$alert.Labels | json}}
          }
          {{end}}
        ]
      }
    headers:
      X-Webhook-Token: "${WEBHOOK_SECRET}"
      X-Alert-Count: "{{len .Alerts}}"
      X-Alert-Status: "{{.Status}}"
      X-Timestamp: "{{now | unixtime}}"
    enabled: true
```

### Multi-Format Example

Send alerts in different formats based on destination requirements:

```yaml
destinations:
  # JSON API endpoint
  - name: json-api
    url: "https://api.example.com/alerts"
    method: POST
    format: json
    engine: jq
    transform: |
      {
        alerts: .alerts | map({
          id: .fingerprint,
          name: .labels.alertname,
          severity: .labels.severity,
          status: .status
        })
      }
    enabled: true

  # Form-encoded legacy endpoint
  - name: legacy-form
    url: "https://legacy.example.com/alert-handler"
    method: POST
    format: form
    engine: go-template
    template: |
      alert_name={{.GroupLabels.alertname}}&
      alert_count={{len .Alerts}}&
      status={{.Status}}&
      {{range $i, $alert := .Alerts}}
      alert_{{$i}}_id={{$alert.Fingerprint}}&
      alert_{{$i}}_severity={{$alert.Labels.severity}}&
      {{end}}
      timestamp={{now | unixtime}}
    headers:
      X-API-Key: "${LEGACY_API_KEY}"
    enabled: true

  # Query parameter endpoint
  - name: query-endpoint
    url: "https://monitor.example.com/ingest"
    method: GET
    format: query
    engine: go-template
    template: |
      {
        "source": "alertmanager",
        "alert": "{{.GroupLabels.alertname}}",
        "status": "{{.Status}}",
        "count": "{{len .Alerts}}",
        "severity": "{{index .CommonLabels "severity" | default "unknown"}}",
        "time": "{{now | unixtime}}"
      }
    enabled: true

  # XML endpoint
  - name: xml-endpoint
    url: "https://xml-api.example.com/alerts"
    method: POST
    format: xml
    engine: go-template
    template: |
      {
        "alert": {
          "@status": "{{.Status}}",
          "@timestamp": "{{now | unixtime}}",
          "name": "{{.GroupLabels.alertname}}",
          "severity": "{{index .CommonLabels "severity" | default "unknown"}}",
          "alerts": [
            {{range $i, $alert := .Alerts}}
            {{if $i}},{{end}}{
              "fingerprint": "{{$alert.Fingerprint}}",
              "status": "{{$alert.Status}}",
              "instance": "{{$alert.Labels.instance | default "unknown"}}"
            }
            {{end}}
          ]
        }
      }
    headers:
      Content-Type: "application/xml"
    enabled: true
```

## Production Configuration

### High Availability Setup

Configuration for production environments with resilience features:

```yaml
server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

destinations:
  # Primary notification channel with circuit breaker
  - name: primary-slack
    url: "${SLACK_PRIMARY_WEBHOOK}"
    method: POST
    format: json
    engine: go-template
    template: |
      {
        "text": "{{.GroupLabels.alertname}} - {{.Status}}",
        "attachments": [{
          "color": "{{if eq .Status "firing"}}danger{{else}}good{{end}}",
          "text": "{{.CommonAnnotations.summary}}",
          "fields": [
            {"title": "Severity", "value": "{{.CommonLabels.severity}}", "short": true},
            {"title": "Count", "value": "{{len .Alerts}}", "short": true}
          ]
        }]
      }
    enabled: true
    retry:
      max_attempts: 5
      backoff: exponential
      initial_delay: 500ms
      max_delay: 30s
    circuit_breaker:
      failure_threshold: 5
      success_threshold: 2
      timeout: 60s
      half_open_requests: 3

  # Fallback notification channel
  - name: fallback-email
    url: "${EMAIL_WEBHOOK_URL}"
    method: POST
    format: json
    engine: jq
    transform: |
      {
        to: ["oncall@example.com"],
        subject: .groupLabels.alertname + " Alert - " + .status,
        body: "Alert: " + .groupLabels.alertname + "\n" +
              "Status: " + .status + "\n" +
              "Count: " + (.alerts | length | tostring) + "\n" +
              "Summary: " + .commonAnnotations.summary
      }
    enabled: true
    retry:
      max_attempts: 10
      backoff: exponential
      initial_delay: 1s
      max_delay: 60s

  # Critical alerts to PagerDuty
  - name: pagerduty-critical
    url: "https://events.pagerduty.com/v2/enqueue"
    method: POST
    format: json
    engine: jq
    transform: |
      # Only send critical alerts
      if (.commonLabels.severity == "critical" and .status == "firing") then
        {
          routing_key: "${env:PAGERDUTY_ROUTING_KEY}",
          event_action: "trigger",
          dedup_key: .groupKey,
          payload: {
            summary: .groupLabels.alertname,
            severity: "critical",
            source: .externalURL,
            custom_details: {
              alerts: [.alerts[] | {
                instance: .labels.instance,
                summary: .annotations.summary
              }]
            }
          }
        }
      else empty end
    enabled: true
    retry:
      max_attempts: 5
      backoff: exponential

  # Metrics aggregation for long-term storage
  - name: metrics-storage
    url: "https://metrics.example.com/v1/alerts"
    method: POST
    format: json
    engine: jq
    transform: |
      {
        timestamp: now | todateiso8601,
        alerts: .alerts | map({
          fingerprint: .fingerprint,
          alertname: .labels.alertname,
          severity: .labels.severity,
          status: .status,
          instance: .labels.instance,
          job: .labels.job,
          started_at: .startsAt,
          ended_at: .endsAt,
          duration_seconds: (
            if .endsAt != null then
              (.endsAt | fromdateiso8601) - (.startsAt | fromdateiso8601)
            else null end
          )
        })
      }
    headers:
      Authorization: "Bearer ${METRICS_API_TOKEN}"
    enabled: true
    split_alerts: false
    retry:
      max_attempts: 3
      backoff: linear
      initial_delay: 5s
```

## Environment Variables

The configuration system supports environment variable overrides using the `GATEWAY_` prefix and traditional variable substitution.

### Environment Variable Overrides

Environment variables with the `GATEWAY_` prefix automatically override configuration values:

- `GATEWAY_SERVER_HOST` → `server.host`
- `GATEWAY_SERVER_PORT` → `server.port`

Example:
```bash
# Override server configuration via environment variables
export GATEWAY_SERVER_HOST=0.0.0.0
export GATEWAY_SERVER_PORT=9090

# Start the gateway - these values will override config file settings
./alertmanager-gateway -config config.yaml
```

### Traditional Variable Substitution

The configuration also supports environment variable substitution using `${VAR_NAME}` syntax:

```yaml
# Example with environment variables
server:
  port: ${PORT:-8080}  # Use PORT env var, default to 8080

destinations:
  - name: webhook
    url: ${WEBHOOK_URL}
    headers:
      Authorization: "Bearer ${API_TOKEN}"
      X-Custom-Header: "${CUSTOM_VALUE:-default}"
```

Set environment variables:

```bash
export PORT=9090
export WEBHOOK_URL=https://api.example.com/webhook
export API_TOKEN=your-api-token
```

## Template Functions

### Go Templates

Available functions for go-template engine:

```yaml
# String functions
{{ .Status | upper }}                    # Convert to uppercase
{{ .Status | lower }}                    # Convert to lowercase
{{ .Status | title }}                    # Title case
{{ .Labels.alertname | replace "-" "_" }}# Replace characters

# Time functions
{{ now | unixtime }}                     # Current Unix timestamp
{{ now | date "2006-01-02" }}           # Format current time
{{ .StartsAt | timeformat "15:04:05" }} # Format time

# Logic functions
{{ .Labels.severity | default "unknown" }}  # Default value
{{ if eq .Status "firing" }}...{{ end }}   # Conditional
{{ range .Alerts }}...{{ end }}             # Loop

# Data functions
{{ len .Alerts }}                        # Count items
{{ .Labels | json }}                     # Convert to JSON
{{ index .Labels "alertname" }}          # Access map value
{{ . | base64 }}                         # Base64 encode
```

### JQ Transformations

Common JQ patterns for alert processing:

```yaml
# Filter alerts
.alerts[] | select(.labels.severity == "critical")

# Transform structure
{
  alert_count: .alerts | length,
  critical: [.alerts[] | select(.labels.severity == "critical")],
  warning: [.alerts[] | select(.labels.severity == "warning")]
}

# Group by field
.alerts | group_by(.labels.severity) | map({
  severity: .[0].labels.severity,
  count: length,
  alerts: map(.labels.alertname)
})

# Conditional logic
if .status == "firing" then "ALERT" else "OK" end

# String manipulation
.groupLabels.alertname + " - " + .status

# Time handling
.alerts[] | {
  alert: .labels.alertname,
  duration: (now - (.startsAt | fromdateiso8601))
}
```

## Validation

Validate your configuration:

```bash
# Check YAML syntax
yamllint config.yaml

# Validate with the gateway
curl -X POST http://localhost:8080/api/v1/config/validate \
  -H "Content-Type: application/yaml" \
  -d @config.yaml
```

## Best Practices

1. **Use Environment Variables**: Store sensitive data like tokens and passwords in environment variables
2. **Enable Retries**: Configure retry logic for production destinations
3. **Set Circuit Breakers**: Protect against cascading failures
4. **Split Critical Alerts**: Use separate destinations for critical vs. warning alerts
5. **Monitor the Gateway**: Use the `/metrics` endpoint for monitoring
6. **Test Transformations**: Use the `/api/v1/test/{destination}` endpoint to test templates
7. **Log Levels**: Set appropriate log levels for production (info or warn)
8. **Timeouts**: Configure reasonable timeouts for HTTP requests
9. **Backup Destinations**: Configure fallback destinations for critical alerts