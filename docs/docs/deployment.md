# Deployment Guide

## Overview

This guide covers deploying the Alertmanager Gateway in various environments, from development to production.

## Prerequisites

- Go 1.21+ (for building from source)
- Docker (for containerized deployment)  
- Kubernetes cluster (for Kubernetes deployment)
- Network access to target notification services

## Build from Source

### Standard Build

```bash
# Clone the repository
git clone https://github.com/vitalvas/alertmanager-gateway.git
cd alertmanager-gateway

# Build for local platform
go build -o gateway .

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o gateway-linux .
```

### Build with Version Information

```bash
# Build with version info
VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "dev")
COMMIT=$(git rev-parse --short HEAD)
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

go build \
  -ldflags "-X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
  -o gateway .
```

## Docker Deployment

### Using Docker Compose

Create a `docker-compose.yml`:

```yaml
version: '3.8'

services:
  alertmanager-gateway:
    image: alertmanager-gateway:latest
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/config.yaml
      - ./logs:/logs
    environment:
      - GATEWAY_LOG_LEVEL=info
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

### Running with Docker

```bash
# Run with basic configuration
docker run -d \
  --name alertmanager-gateway \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/config.yaml \
  alertmanager-gateway:latest

# Run with custom configuration
docker run -d \
  --name alertmanager-gateway \
  -p 8080:8080 \
  -v $(pwd)/config:/config \
  -v $(pwd)/logs:/logs \
  --env-file .env \
  alertmanager-gateway:latest --config=/config/config.yaml
```

## Kubernetes Deployment

### Basic Deployment

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: alertmanager-gateway

---
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway-config
  namespace: alertmanager-gateway
data:
  config.yaml: |
    server:
      host: "0.0.0.0"
      port: 8080
    
    destinations:
      - name: "slack"
        url: "${SLACK_WEBHOOK_URL}"
        method: "POST"
        format: "json"
        headers:
          Content-Type: "application/json"
        template: |
          {
            "text": "Alert: {{ .GroupLabels.alertname }}",
            "attachments": [{
              "color": "{{ if eq .Status \"resolved\" }}good{{ else }}danger{{ end }}",
              "text": "{{ .CommonAnnotations.summary }}"
            }]
          }

---
# secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: gateway-secrets
  namespace: alertmanager-gateway
type: Opaque
data:
  slack-webhook-url: <base64-encoded-slack-url>

---
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alertmanager-gateway
  namespace: alertmanager-gateway
spec:
  replicas: 2
  selector:
    matchLabels:
      app: alertmanager-gateway
  template:
    metadata:
      labels:
        app: alertmanager-gateway
    spec:
      containers:
      - name: gateway
        image: alertmanager-gateway:latest
        ports:
        - containerPort: 8080
        env:
        - name: SLACK_WEBHOOK_URL
          valueFrom:
            secretKeyRef:
              name: gateway-secrets
              key: slack-webhook-url
        volumeMounts:
        - name: config
          mountPath: /config.yaml
          subPath: config.yaml
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "500m"
      volumes:
      - name: config
        configMap:
          name: gateway-config

---
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: alertmanager-gateway
  namespace: alertmanager-gateway
spec:
  selector:
    app: alertmanager-gateway
  ports:
  - port: 8080
    targetPort: 8080
  type: ClusterIP
```

### High Availability Setup

```yaml
# For HA deployment with multiple replicas
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alertmanager-gateway
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
  template:
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app
                  operator: In
                  values:
                  - alertmanager-gateway
              topologyKey: kubernetes.io/hostname
```

## Production Configuration

### Environment Variables

```bash
# Security - configure in application config if needed

# Service URLs
export SLACK_WEBHOOK_URL="https://hooks.slack.com/services/..."
export PAGERDUTY_URL="https://events.pagerduty.com/v2/enqueue"

# Performance
export GATEWAY_LOG_LEVEL="info"
export GATEWAY_READ_TIMEOUT="30s"
export GATEWAY_WRITE_TIMEOUT="30s"

# Monitoring
export ENABLE_PPROF="false"  # Only enable for debugging
```

### Production Configuration Template

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: "30s"
  write_timeout: "30s"

destinations:
  # Critical alerts to PagerDuty
  - name: "pagerduty-critical"
    url: "${PAGERDUTY_URL}"
    method: "POST"
    format: "json"
    split_alerts: true
    batch_size: 1
