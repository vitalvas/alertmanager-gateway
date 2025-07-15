# API Documentation

## Overview

The Alertmanager Gateway exposes HTTP endpoints to receive webhook notifications from Prometheus Alertmanager and forward them to configured third-party systems.

## Base URL

```
http://localhost:8080
```

## Endpoints

### Webhook Endpoints

#### POST /webhook/{destination}

Receives alert notifications from Prometheus Alertmanager and forwards them to the configured destination.

**Path Parameters:**
- `destination` (string, required): The destination name as configured in the destinations list

**Request Headers:**
- `Content-Type: application/json`

**Request Body:**
Prometheus Alertmanager webhook payload format:

```json
{
  "version": "4",
  "groupKey": "{}:{alertname=\"example\"}",
  "truncatedAlerts": 0,
  "status": "firing",
  "receiver": "gateway",
  "groupLabels": {
    "alertname": "example"
  },
  "commonLabels": {
    "alertname": "example",
    "severity": "warning",
    "instance": "localhost:9090"
  },
  "commonAnnotations": {
    "summary": "Example alert summary",
    "description": "This is an example alert"
  },
  "externalURL": "http://alertmanager.example.com",
  "alerts": [
    {
      "status": "firing",
      "labels": {
        "alertname": "example",
        "severity": "warning",
        "instance": "localhost:9090"
      },
      "annotations": {
        "summary": "Example alert summary",
        "description": "This is an example alert"
      },
      "startsAt": "2024-01-01T12:00:00Z",
      "endsAt": "0001-01-01T00:00:00Z",
      "generatorURL": "http://prometheus.example.com/graph?expr=up%3D%3D0",
      "fingerprint": "d7f3b8c12a89f45e"
    }
  ]
}
```

**Response Codes:**
- `200 OK`: Alert successfully processed and forwarded
- `400 Bad Request`: Invalid request body or missing required fields
- `404 Not Found`: Destination not configured
- `500 Internal Server Error`: Processing or forwarding error
- `502 Bad Gateway`: Target system unreachable
- `503 Service Unavailable`: Gateway overloaded or in maintenance

**Response Body (Success):**
```json
{
  "status": "success",
  "destination": "slack",
  "forwarded_at": "2024-01-01T12:00:05Z"
}
```

**Response Body (Success - Split Alerts):**
```json
{
  "status": "success",
  "destination": "ticketing-system",
  "mode": "split",
  "results": [
    {
      "alert_fingerprint": "d7f3b8c12a89f45e",
      "status": "success",
      "response_code": 201,
      "ticket_id": "TICK-12345"
    },
    {
      "alert_fingerprint": "a8e4c7f29b12d56f",
      "status": "success",
      "response_code": 201,
      "ticket_id": "TICK-12346"
    }
  ],
  "summary": {
    "total": 2,
    "successful": 2,
    "failed": 0
  }
}
```

**Response Body (Error):**
```json
{
  "status": "error",
  "error": "Failed to forward alert",
  "details": "Connection timeout to target system"
}
```

### Health Check Endpoints

#### GET /health

Returns the health status of the gateway service.

**Response Codes:**
- `200 OK`: Service is healthy
- `503 Service Unavailable`: Service is unhealthy

**Response Body:**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime_seconds": 3600,
  "config_loaded": true,
  "destinations_count": 5
}
```

#### GET /health/live

Kubernetes liveness probe endpoint.

**Response Codes:**
- `200 OK`: Service is alive
- `503 Service Unavailable`: Service should be restarted

**Response Body:**
```json
{
  "status": "alive"
}
```

#### GET /health/ready

Kubernetes readiness probe endpoint.

**Response Codes:**
- `200 OK`: Service is ready to accept traffic
- `503 Service Unavailable`: Service is not ready

**Response Body:**
```json
{
  "status": "ready",
  "config_loaded": true
}
```

### Metrics Endpoint

#### GET /metrics

Prometheus metrics endpoint.

**Response:**
Prometheus text format metrics including:
- `alertmanager_gateway_requests_total`: Total requests by destination and status
- `alertmanager_gateway_request_duration_seconds`: Request processing duration
- `alertmanager_gateway_forward_duration_seconds`: Time to forward to target system
- `alertmanager_gateway_template_render_duration_seconds`: Template rendering time
- `alertmanager_gateway_active_requests`: Currently processing requests
- `alertmanager_gateway_destinations_configured`: Number of configured destinations

### Management API Endpoints

#### GET /api/v1/destinations

Lists all configured destinations.

**Query Parameters:**
- `include_disabled` (boolean, optional): Include disabled destinations in the response. Default: false

**Response Codes:**
- `200 OK`: Success
- `401 Unauthorized`: Missing or invalid authentication
- `500 Internal Server Error`: Server error

**Response Body:**
```json
{
  "status": "success",
  "destinations": [
    {
      "name": "slack",
      "url": "https://hooks.slack.com/services/***",
      "method": "POST",
      "format": "json",
      "engine": "go-template",
      "enabled": true,
      "split_alerts": false,
      "headers_count": 2,
      "template_size": 450
    },
    {
      "name": "custom-api",
      "url": "https://api.example.com/***",
      "method": "POST",
      "format": "json",
      "engine": "jq",
      "enabled": true,
      "split_alerts": true,
      "headers_count": 3,
      "template_size": 280
    }
  ],
  "total": 2,
  "enabled": 2,
  "disabled": 0
}
```

#### GET /api/v1/destinations/{name}

Get detailed information about a specific destination.

**Path Parameters:**
- `name` (string, required): Destination name

**Response Codes:**
- `200 OK`: Success
- `401 Unauthorized`: Missing or invalid authentication
- `404 Not Found`: Destination not found
- `500 Internal Server Error`: Server error

**Response Body:**
```json
{
  "status": "success",
  "destination": {
    "name": "slack",
    "url": "https://hooks.slack.com/services/***",
    "method": "POST",
    "format": "json",
    "engine": "go-template",
    "template": "*** (450 bytes)",
    "transform": null,
    "headers": {
      "Content-Type": "application/json",
      "X-Custom-Header": "*** (masked)"
    },
    "enabled": true,
    "split_alerts": false,
    "batch_size": 10,
    "parallel_requests": 1,
    "retry": {
      "max_attempts": 3,
      "backoff": "exponential"
    },
    "circuit_breaker": {
      "failure_threshold": 5,
      "timeout": "30s",
      "half_open_requests": 3
    }
  }
}
```

**Response Body (Split Mode Configuration):**
```json
{
  "status": "success",
  "destination": {
    "name": "ticketing-system",
    "url": "https://tickets.example.com/api/***",
    "method": "POST",
    "format": "json",
    "engine": "jq",
    "template": null,
    "transform": "*** (280 bytes)",
    "headers": {
      "Content-Type": "application/json",
      "Authorization": "*** (masked)"
    },
    "enabled": true,
    "split_alerts": true,
    "batch_size": 1,
    "parallel_requests": 5,
    "retry": {
      "max_attempts": 3,
      "backoff": "exponential"
    },
    "circuit_breaker": {
      "failure_threshold": 5,
      "timeout": "30s",
      "half_open_requests": 3
    }
  }
}
```

#### POST /api/v1/test/{destination}

Test/emulate message transformation for a specific destination without sending to the actual endpoint.

**Path Parameters:**
- `destination` (string, required): Destination name to test

**Request Headers:**
- `Content-Type: application/json`

**Request Body:**
Same as webhook endpoint - Prometheus Alertmanager webhook payload

**Response Codes:**
- `200 OK`: Transformation successful
- `400 Bad Request`: Invalid request or transformation error
- `404 Not Found`: Destination not configured
- `500 Internal Server Error`: Processing error

**Response Body (Success):**
```json
{
  "destination": "slack",
  "engine": "jq",
  "format": "json",
  "transformed_data": {
    "text": "Alert: DiskSpaceLow",
    "attachments": [
      {
        "color": "danger",
        "fields": [
          {
            "title": "Status",
            "value": "firing",
            "short": true
          },
          {
            "title": "Severity", 
            "value": "warning",
            "short": true
          }
        ],
        "text": "Disk space is running low on server1"
      }
    ]
  },
  "output_format": {
    "method": "POST",
    "url": "https://hooks.slack.com/services/XXX",
    "headers": {
      "Content-Type": "application/json"
    },
    "body": "{\"text\":\"Alert: DiskSpaceLow\",\"attachments\":[{\"color\":\"danger\",\"fields\":[{\"title\":\"Status\",\"value\":\"firing\",\"short\":true},{\"title\":\"Severity\",\"value\":\"warning\",\"short\":true}],\"text\":\"Disk space is running low on server1\"}]}"
  }
}
```

**Response Body (Split Mode):**
```json
{
  "destination": "ticketing-system",
  "engine": "jq",
  "format": "json",
  "split_alerts": true,
  "alert_count": 2,
  "transformed_data": [
    {
      "alert_fingerprint": "d7f3b8c12a89f45e",
      "output": {
        "title": "DiskSpaceLow",
        "description": "Disk space is running low",
        "severity": "warning",
        "source": "server1.example.com",
        "alert_id": "d7f3b8c12a89f45e",
        "started_at": "2024-01-01T12:00:00Z",
        "status": "firing",
        "group_key": "{}:{alertname=\"DiskSpaceLow\"}"
      }
    },
    {
      "alert_fingerprint": "a8e4c7f29b12d56f",
      "output": {
        "title": "HighMemoryUsage",
        "description": "Memory usage is above 90%",
        "severity": "critical",
        "source": "server2.example.com",
        "alert_id": "a8e4c7f29b12d56f",
        "started_at": "2024-01-01T12:05:00Z",
        "status": "firing",
        "group_key": "{}:{alertname=\"DiskSpaceLow\"}"
      }
    }
  ]
}
```

**Response Body (Error):**
```json
{
  "status": "error",
  "destination": "custom-api",
  "error": "Template transformation failed",
  "details": "jq compile error: Unknown function 'invalid_func' at line 3",
  "template": ".alerts | map(invalid_func(.labels))"
}
```

#### POST /api/v1/emulate/{destination}

Fully emulate the webhook processing for a specific destination, including HTTP request generation and optional sending.

**Path Parameters:**
- `destination` (string, required): Destination name to emulate

**Query Parameters:**
- `dry_run` (boolean, optional): Generate the request without sending it. Default: true

**Request Headers:**
- `Content-Type: application/json`

**Request Body:**
Same as webhook endpoint - Prometheus Alertmanager webhook payload

**Response Codes:**
- `200 OK`: Emulation successful
- `400 Bad Request`: Invalid request or processing error
- `404 Not Found`: Destination not configured
- `500 Internal Server Error`: Server error
- `502 Bad Gateway`: Target system unreachable (when dry_run=false)

**Response Body (Success - Dry Run):**
```json
{
  "status": "success",
  "destination": "slack",
  "dry_run": true,
  "transformation": {
    "engine": "go-template",
    "format": "json",
    "success": true,
    "duration_ms": 1.2
  },
  "request": {
    "method": "POST",
    "url": "https://hooks.slack.com/services/***",
    "headers": {
      "Content-Type": "application/json",
      "User-Agent": "Alertmanager-Gateway/1.0"
    },
    "body_size": 512,
    "body_preview": "{\"text\":\"🚨 Alert: HighCPU is firing\",\"attachments\":[{\"color\":\"danger\",\"fields\":[{\"title\":\"Severity\",\"value\":\"critical\",\"short\":true}]}]}"
  }
}
```

**Response Body (Success - Actual Request):**
```json
{
  "status": "success",
  "destination": "pagerduty",
  "dry_run": false,
  "transformation": {
    "engine": "jq",
    "format": "json",
    "success": true,
    "duration_ms": 0.8
  },
  "request": {
    "method": "POST",
    "url": "https://events.pagerduty.com/v2/enqueue",
    "headers": {
      "Content-Type": "application/json",
      "Authorization": "*** (masked)"
    },
    "body_size": 384
  },
  "response": {
    "status_code": 202,
    "status": "Accepted",
    "headers": {
      "X-Request-Id": ["abc123"],
      "Content-Type": ["application/json"]
    },
    "body": "{\"status\":\"success\",\"message\":\"Event processed\",\"dedup_key\":\"alert-12345\"}",
    "duration_ms": 127.5
  }
}
```

**Response Body (Error):**
```json
{
  "status": "error",
  "destination": "webhook",
  "error": "Request failed",
  "details": "Post \"https://api.example.com/webhook\": dial tcp: lookup api.example.com: no such host",
  "transformation": {
    "success": true,
    "duration_ms": 0.5
  },
  "request": {
    "method": "POST",
    "url": "https://api.example.com/webhook"
  }
}
```

#### GET /api/v1/info

Get system information about the gateway.

**Response Codes:**
- `200 OK`: Success
- `500 Internal Server Error`: Server error

**Response Body:**
```json
{
  "status": "success",
  "info": {
    "version": "1.0.0",
    "build_time": "2024-01-15T10:30:00Z",
    "go_version": "go1.21.5",
    "os": "linux",
    "arch": "amd64",
    "uptime_seconds": 3600,
    "start_time": "2024-01-15T09:30:00Z"
  },
  "resources": {
    "goroutines": 42,
    "memory": {
      "alloc_mb": 15.2,
      "total_alloc_mb": 127.8,
      "sys_mb": 35.4,
      "gc_count": 18
    },
    "cpu": {
      "num_cpu": 4,
      "go_max_procs": 4
    }
  },
  "configuration": {
    "destinations_total": 5,
    "destinations_enabled": 4,
    "destinations_disabled": 1,
    "auth_enabled": true,
    "metrics_enabled": true,
    "config_path": "/etc/alertmanager-gateway/config.yaml",
    "log_level": "info"
  }
}
```

#### GET /api/v1/health

Enhanced health check with detailed component status.

**Query Parameters:**
- `verbose` (boolean, optional): Include detailed component checks. Default: false

**Response Codes:**
- `200 OK`: All components healthy
- `503 Service Unavailable`: One or more components unhealthy

**Response Body (Simple):**
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "uptime_seconds": 3600,
  "destinations": {
    "total": 5,
    "healthy": 4,
    "unhealthy": 1
  }
}
```

**Response Body (Verbose):**
```json
{
  "status": "degraded",
  "timestamp": "2024-01-15T10:30:00Z",
  "uptime_seconds": 3600,
  "components": {
    "server": {
      "status": "healthy",
      "message": "Server is running"
    },
    "configuration": {
      "status": "healthy",
      "message": "Configuration loaded successfully",
      "details": {
        "loaded_at": "2024-01-15T09:30:00Z",
        "reload_count": 0
      }
    },
    "destinations": {
      "status": "degraded",
      "message": "1 destination has circuit breaker open",
      "details": {
        "slack": {
          "status": "healthy",
          "enabled": true,
          "last_success": "2024-01-15T10:25:00Z"
        },
        "pagerduty": {
          "status": "unhealthy",
          "enabled": true,
          "error": "Circuit breaker open",
          "failures": 5,
          "last_failure": "2024-01-15T10:28:00Z"
        },
        "webhook": {
          "status": "healthy",
          "enabled": true,
          "last_success": "2024-01-15T10:29:00Z"
        },
        "teams": {
          "status": "disabled",
          "enabled": false
        },
        "email": {
          "status": "healthy",
          "enabled": true,
          "last_success": "2024-01-15T10:27:00Z"
        }
      }
    },
    "memory": {
      "status": "healthy",
      "message": "Memory usage within limits",
      "details": {
        "usage_mb": 15.2,
        "limit_mb": 512,
        "percentage": 2.97
      }
    }
  },
  "warnings": [
    "Destination 'pagerduty' has circuit breaker open",
    "High error rate detected in last 5 minutes"
  ]
}
```

#### POST /api/v1/config/validate

Validate a configuration file or snippet.

**Request Headers:**
- `Content-Type: application/yaml` or `application/json`

**Request Body:**
Configuration in YAML or JSON format

**Response Codes:**
- `200 OK`: Configuration is valid
- `400 Bad Request`: Configuration is invalid
- `500 Internal Server Error`: Server error

**Response Body (Valid):**
```json
{
  "status": "success",
  "valid": true,
  "message": "Configuration is valid",
  "summary": {
    "destinations": 3,
    "auth_enabled": true,
    "warnings": [
      "Destination 'webhook' has no retry configuration",
      "Consider enabling metrics for production use"
    ]
  }
}
```

**Response Body (Invalid):**
```json
{
  "status": "error",
  "valid": false,
  "error": "Configuration validation failed",
  "errors": [
    {
      "field": "destinations[0].url",
      "error": "Invalid URL format",
      "value": "not-a-url"
    },
    {
      "field": "destinations[1].engine",
      "error": "Invalid engine type",
      "value": "invalid-engine",
      "allowed": ["go-template", "jq"]
    },
    {
      "field": "server.port",
      "error": "Port must be between 1 and 65535",
      "value": 70000
    }
  ]
}
```


## Authentication

### Server Authentication

When basic authentication is enabled in the server configuration, all endpoints require authentication:

```yaml
# config.yaml
server:
  auth:
    enabled: true
    username: "alertmanager"
    password: "secretpassword"
```

All requests must include the Authorization header:
```
Authorization: Basic <base64(username:password)>
```

Example:
```bash
# Create base64 encoded credentials
echo -n "alertmanager:secretpassword" | base64
# Result: YWxlcnRtYW5hZ2VyOnNlY3JldHBhc3N3b3Jk

# Use in requests
curl -X POST http://localhost:8080/webhook/slack \
  -H "Authorization: Basic YWxlcnRtYW5hZ2VyOnNlY3JldHBhc3N3b3Jk" \
  -H "Content-Type: application/json" \
  -d @alert.json
```

### Alertmanager Configuration

Configure Alertmanager to use basic auth:

```yaml
receivers:
  - name: 'gateway'
    webhook_configs:
      - url: 'http://localhost:8080/webhook/slack'
        http_config:
          basic_auth:
            username: 'alertmanager'
            password: 'secretpassword'
```

### Rate Limiting

The gateway includes built-in rate limiting for authentication failures:

- **Failed attempts limit**: 5 attempts per minute per IP address
- **Ban duration**: 15 minutes after exceeding the limit
- **Headers**: `Retry-After` header is set when rate limited
- **Response**: HTTP 429 Too Many Requests when rate limited

Example rate limit response:
```json
{
  "status": "error",
  "error": "Too many authentication failures",
  "retry_after": 900,
  "timestamp": "2024-01-15T10:30:00Z"
}
```

## Error Handling

All errors follow a consistent format:

```json
{
  "status": "error",
  "error": "Human-readable error message",
  "details": "Additional context or technical details",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

## Request Examples

### Forward Alert to Slack

```bash
curl -X POST http://localhost:8080/webhook/slack \
  -H "Authorization: Basic YWxlcnRtYW5hZ2VyOnNlY3JldHBhc3N3b3Jk" \
  -H "Content-Type: application/json" \
  -d '{
    "version": "4",
    "groupKey": "{}:{alertname=\"DiskSpaceLow\"}",
    "status": "firing",
    "receiver": "gateway",
    "groupLabels": {
      "alertname": "DiskSpaceLow"
    },
    "commonLabels": {
      "alertname": "DiskSpaceLow",
      "severity": "warning",
      "instance": "server1.example.com"
    },
    "commonAnnotations": {
      "summary": "Disk space is running low",
      "description": "Disk usage is above 90% on server1"
    },
    "alerts": [{
      "status": "firing",
      "labels": {
        "alertname": "DiskSpaceLow",
        "severity": "warning",
        "instance": "server1.example.com"
      },
      "annotations": {
        "summary": "Disk space is running low",
        "description": "Disk usage is above 90% on server1"
      },
      "startsAt": "2024-01-01T12:00:00Z"
    }]
  }'
```

### Check Health

```bash
curl http://localhost:8080/health
```

### List Destinations

```bash
curl http://localhost:8080/api/v1/destinations \
  -H "Authorization: Basic YWxlcnRtYW5hZ2VyOnNlY3JldHBhc3N3b3Jk"
```

### Test Transformation

```bash
curl -X POST http://localhost:8080/api/v1/test/slack \
  -H "Authorization: Basic YWxlcnRtYW5hZ2VyOnNlY3JldHBhc3N3b3Jk" \
  -H "Content-Type: application/json" \
  -d '{
    "version": "4",
    "groupKey": "{}:{alertname=\"TestAlert\"}",
    "status": "firing",
    "receiver": "gateway",
    "groupLabels": {
      "alertname": "TestAlert"
    },
    "commonLabels": {
      "alertname": "TestAlert",
      "severity": "info"
    },
    "commonAnnotations": {
      "summary": "This is a test alert",
      "description": "Testing transformation logic"
    },
    "alerts": [{
      "status": "firing",
      "labels": {
        "alertname": "TestAlert",
        "severity": "info",
        "instance": "test.example.com"
      },
      "annotations": {
        "summary": "This is a test alert",
        "description": "Testing transformation logic"
      },
      "startsAt": "2024-01-01T12:00:00Z"
    }]
  }'
```

### Get Destination Details

```bash
curl http://localhost:8080/api/v1/destinations/slack \
  -H "Authorization: Basic YWxlcnRtYW5hZ2VyOnNlY3JldHBhc3N3b3Jk"
```

### Emulate Request (Dry Run)

```bash
curl -X POST http://localhost:8080/api/v1/emulate/pagerduty?dry_run=true \
  -H "Authorization: Basic YWxlcnRtYW5hZ2VyOnNlY3JldHBhc3N3b3Jk" \
  -H "Content-Type: application/json" \
  -d '{
    "version": "4",
    "groupKey": "{}:{alertname=\"DiskFull\"}",
    "status": "firing",
    "receiver": "gateway",
    "alerts": [{
      "status": "firing",
      "labels": {
        "alertname": "DiskFull",
        "severity": "critical",
        "instance": "prod-server-01"
      },
      "startsAt": "2024-01-01T12:00:00Z"
    }]
  }'
```

### Get System Info

```bash
curl http://localhost:8080/api/v1/info \
  -H "Authorization: Basic YWxlcnRtYW5hZ2VyOnNlY3JldHBhc3N3b3Jk"
```

### Get Detailed Health

```bash
curl http://localhost:8080/api/v1/health?verbose=true \
  -H "Authorization: Basic YWxlcnRtYW5hZ2VyOnNlY3JldHBhc3N3b3Jk"
```

### Validate Configuration

```bash
curl -X POST http://localhost:8080/api/v1/config/validate \
  -H "Authorization: Basic YWxlcnRtYW5hZ2VyOnNlY3JldHBhc3N3b3Jk" \
  -H "Content-Type: application/yaml" \
  -d '
server:
  port: 8080
  auth:
    enabled: true
    username: alertmanager
    password: secretpassword

destinations:
  - name: test-webhook
    url: https://example.com/webhook
    method: POST
    format: json
    engine: go-template
    template: |
      {"alert": "{{.Status}}"}
    enabled: true
'
```

