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

## Quick Start

1. Create a configuration file `config.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  auth:
    enabled: true
    username: "alertmanager"
    password: "${GATEWAY_PASSWORD}"

destinations:
  - name: "flock"
    path: "/webhook/flock"
    method: "POST"
    url: "https://api.flock.com/hooks/sendMessage/${FLOCK_WEBHOOK_ID}"
    format: "json"
    engine: "jq"
    transform: |
      {
        text: ("Alert: " + .groupLabels.alertname + " [" + .status + "]"),
        attachments: [{
          title: .commonAnnotations.summary,
          description: (.alerts | map(.annotations.description) | join("\n")),
          color: (if .status == "resolved" then "#36a64f" else "#ff0000" end)
        }]
      }
```

2. Run the gateway:

```bash
export GATEWAY_PASSWORD="your-secure-password"
export FLOCK_WEBHOOK_ID="your-flock-webhook-id"
go run ./cmd/alertmanager-gateway
```

3. Configure Alertmanager:

```yaml
receivers:
  - name: 'flock'
    webhook_configs:
      - url: 'http://localhost:8080/webhook/flock'
        http_config:
          basic_auth:
            username: 'alertmanager'
            password: 'your-secure-password'
```

## Development

### Prerequisites

- Go 1.21 or later
- Task (https://taskfile.dev)

### Setup

```bash
# Install dependencies
task deps

# Install development tools
task install-tools

# Run tests
task test

# Run linter
task lint

# Build binary
task build
```

### Available Tasks

Run `task` to see all available tasks.

## Documentation

See the [docs](docs/docs/) directory for detailed documentation:

- [Architecture](docs/docs/architecture.md)
- [API Reference](docs/docs/api.md)
- [Implementation Roadmap](docs/docs/roadmap.md)

## License

[License file](LICENSE)