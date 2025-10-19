# Alertmanager Gateway

Universal adapter for Prometheus Alertmanager webhooks that transforms and routes alerts to various third-party notification systems.

## Features

- Receives webhooks from Prometheus Alertmanager
- Transforms alerts using Go templates or jq
- Routes to multiple destinations based on path
- Supports various output formats (JSON, Form, Query params)
- Split grouped alerts for individual processing
- Built-in authentication and security
- Prometheus metrics for monitoring
- Batch processing with parallel requests

## Quick Start

1. Create a configuration file `config.yaml`:

```yaml
server:
  address: ":8080"

destinations:
  - name: "slack"
    url: "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
    method: "POST"
    format: "json"
    engine: "go-template"
    template: |
      {
        "text": "{{if eq .Status \"firing\"}}Alert Firing{{else}}Alert Resolved{{end}}: {{.GroupLabels.alertname}}",
        "attachments": [{
          "color": "{{if eq .Status \"firing\"}}danger{{else}}good{{end}}",
          "title": "{{.GroupLabels.alertname}}",
          "text": "{{.CommonAnnotations.summary}}",
          "fields": [{
            "title": "Status",
            "value": "{{.Status}}",
            "short": true
          }, {
            "title": "Severity",
            "value": "{{index .CommonLabels \"severity\" | default \"unknown\"}}",
            "short": true
          }]
        }]
      }
    enabled: true
```

2. Run the gateway:

```bash
go run .
```

3. Configure Alertmanager:

```yaml
receivers:
  - name: 'slack'
    webhook_configs:
      - url: 'http://localhost:8080/webhook/slack'
```
