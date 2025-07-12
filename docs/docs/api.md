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
- `destination` (string, required): The destination name matching a configured destination path

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

### Administrative Endpoints

#### GET /api/v1/destinations

Lists all configured destinations.

**Response Body:**
```json
{
  "destinations": [
    {
      "name": "slack",
      "path": "/webhook/slack",
      "method": "POST",
      "format": "json",
      "enabled": true
    },
    {
      "name": "custom-api",
      "path": "/webhook/custom",
      "method": "POST",
      "format": "json",
      "enabled": true
    }
  ]
}
```

#### GET /api/v1/destinations/{name}

Get details of a specific destination.

**Path Parameters:**
- `name` (string, required): Destination name

**Response Body:**
```json
{
  "name": "slack",
  "path": "/webhook/slack",
  "method": "POST",
  "url": "https://hooks.slack.com/services/***",
  "format": "json",
  "engine": "go-template",
  "headers_count": 1,
  "template_size": 450,
  "enabled": true,
  "split_alerts": false,
  "retry": {
    "max_attempts": 3,
    "backoff": "exponential"
  }
}
```

**Response Body (Split Configuration):**
```json
{
  "name": "ticketing-system",
  "path": "/webhook/tickets",
  "method": "POST",
  "url": "https://tickets.example.com/api/***",
  "format": "json",
  "engine": "jq",
  "split_alerts": true,
  "batch_size": 1,
  "parallel_requests": 5,
  "enabled": true,
  "retry": {
    "max_attempts": 3,
    "backoff": "exponential",
    "per_alert": true
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

