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
      Authorization: "Bearer ${env:API_TOKEN}"
    enabled: true               # Enable/disable destination
    split_alerts: false         # Send alerts individually
    batch_size: 10              # Alerts per batch (when splitting)
    parallel_requests: 5        # Concurrent requests (when splitting)
```

## Basic Examples

### Slack Integration

Send alerts to Slack using incoming webhooks:

```yaml
server:
  port: 8080

destinations:
  - name: slack
    url: "${env:SLACK_WEBHOOK_URL}"
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
      X-Webhook-Token: "${env:WEBHOOK_SECRET}"
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
      X-API-Key: "${env:LEGACY_API_KEY}"
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
  # Primary notification channel
  - name: primary-slack
    url: "${env:SLACK_PRIMARY_WEBHOOK}"
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

  # Fallback notification channel
  - name: fallback-email
    url: "${env:EMAIL_WEBHOOK_URL}"
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
