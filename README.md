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

destinations:
  - name: "flock"
    method: "POST"
    url: "https://api.flock.com/hooks/sendMessage/${env:FLOCK_WEBHOOK_ID}"
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
export FLOCK_WEBHOOK_ID="your-flock-webhook-id"
go run .
```

3. Configure Alertmanager:

```yaml
receivers:
  - name: 'flock'
    webhook_configs:
      - url: 'http://localhost:8080/webhook/flock'
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

Complete documentation is available in the [docs](docs/docs/) directory:

- [Architecture](docs/docs/architecture.md) - System architecture and design patterns
- [API Reference](docs/docs/api.md) - REST API endpoints and usage
- [Usage Examples](docs/docs/usage-examples.md) - Practical examples and recipes

You can also view the documentation using MkDocs:

```bash
cd docs
mkdocs serve
```

## License

[License file](LICENSE)