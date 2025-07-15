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
      - GATEWAY_PASSWORD=your-secure-password
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
  -e GATEWAY_PASSWORD=your-password \
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
      auth:
        enabled: true
        username: "alertmanager"
        password: "${GATEWAY_PASSWORD}"
    
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
  gateway-password: <base64-encoded-password>
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
        - name: GATEWAY_PASSWORD
          valueFrom:
            secretKeyRef:
              name: gateway-secrets
              key: gateway-password
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
# Security
export GATEWAY_PASSWORD="your-secure-password"
export GATEWAY_API_PASSWORD="admin-password"

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
  auth:
    enabled: true
    username: "alertmanager"
    password: "${GATEWAY_PASSWORD}"
    api_username: "admin"
    api_password: "${GATEWAY_API_PASSWORD}"

destinations:
  # Critical alerts to PagerDuty
  - name: "pagerduty-critical"
    url: "${PAGERDUTY_URL}"
    method: "POST"
    format: "json"
    split_alerts: true
    batch_size: 1
    retry:
      max_attempts: 5
      backoff: "exponential"
    headers:
      Authorization: "Token token=${PAGERDUTY_TOKEN}"
      Content-Type: "application/json"
    template: |
      {
        "routing_key": "${PAGERDUTY_ROUTING_KEY}",
        "event_action": "{{ if eq .Alert.Status \"resolved\" }}resolve{{ else }}trigger{{ end }}",
        "dedup_key": "{{ .Alert.Fingerprint }}",
        "payload": {
          "summary": "{{ .Alert.Labels.alertname }}: {{ .Alert.Annotations.summary }}",
          "source": "{{ .Alert.Labels.instance }}",
          "severity": "{{ .Alert.Labels.severity }}",
          "component": "{{ .Alert.Labels.job }}",
          "group": "{{ .Alert.Labels.team | default \"ops\" }}",
          "class": "{{ .Alert.Labels.service | default \"unknown\" }}",
          "custom_details": {
            "description": "{{ .Alert.Annotations.description }}",
            "runbook_url": "{{ .Alert.Annotations.runbook_url }}",
            "alert_labels": {{ .Alert.Labels | toJson }},
            "alert_annotations": {{ .Alert.Annotations | toJson }}
          }
        }
      }

  # General alerts to Slack
  - name: "slack-general"
    url: "${SLACK_WEBHOOK_URL}"
    method: "POST"
    format: "json"
    headers:
      Content-Type: "application/json"
    template: |
      {
        "text": "{{ if eq .Status \"resolved\" }}✅ Resolved{{ else }}🚨 Alert{{ end }}: {{ .GroupLabels.alertname }}",
        "attachments": [{
          "color": "{{ if eq .Status \"resolved\" }}good{{ else if eq (.CommonLabels.severity | default \"warning\") \"critical\" }}danger{{ else }}warning{{ end }}",
          "fields": [
            {
              "title": "Summary",
              "value": "{{ .CommonAnnotations.summary }}",
              "short": false
            },
            {
              "title": "Severity",
              "value": "{{ .CommonLabels.severity | default \"unknown\" }}",
              "short": true
            },
            {
              "title": "Instance",
              "value": "{{ .CommonLabels.instance }}",
              "short": true
            }
          ],
          "footer": "Alertmanager Gateway",
          "ts": {{ now.Unix }}
        }]
      }
```

## Monitoring and Observability

### Prometheus Monitoring

Add the gateway as a Prometheus target:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'alertmanager-gateway'
    static_configs:
      - targets: ['alertmanager-gateway:8080']
    metrics_path: '/metrics'
    scrape_interval: 30s
```

### Key Metrics to Monitor

```promql
# Request rate
rate(http_requests_total[5m])

# Error rate
rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m])

# Response time
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))

# Webhook processing
rate(webhook_requests_total[5m])

# Alert processing by destination
rate(alerts_processed_total[5m])

# Authentication failures
rate(auth_attempts_total{result="failed"}[5m])
```

### Alerting Rules

```yaml
# alerting_rules.yml
groups:
  - name: alertmanager-gateway
    rules:
      - alert: GatewayDown
        expr: up{job="alertmanager-gateway"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Alertmanager Gateway is down"
          description: "Gateway instance {{ $labels.instance }} has been down for more than 1 minute"

      - alert: GatewayHighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m]) > 0.1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High error rate in Alertmanager Gateway"
          description: "Error rate is {{ $value | humanizePercentage }} for instance {{ $labels.instance }}"

      - alert: GatewayHighLatency
        expr: histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High latency in Alertmanager Gateway"
          description: "95th percentile latency is {{ $value }}s for instance {{ $labels.instance }}"
```

## Security Considerations

### Network Security

```bash
# Firewall rules (iptables example)
iptables -A INPUT -p tcp --dport 8080 -s alertmanager-ip -j ACCEPT
iptables -A INPUT -p tcp --dport 8080 -j DROP

# Nginx reverse proxy with SSL
server {
    listen 443 ssl;
    server_name gateway.example.com;
    
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    
    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Secrets Management

```bash
# Using Kubernetes secrets
kubectl create secret generic gateway-secrets \
  --from-literal=gateway-password=your-password \
  --from-literal=slack-webhook-url=https://... \
  --namespace=alertmanager-gateway

# Using Docker secrets
echo "your-password" | docker secret create gateway_password -
```

## Backup and Recovery

### Configuration Backup

```bash
# Backup configuration
kubectl get configmap gateway-config -o yaml > backup/config-$(date +%Y%m%d).yaml
kubectl get secret gateway-secrets -o yaml > backup/secrets-$(date +%Y%m%d).yaml

# Restore configuration
kubectl apply -f backup/config-20240101.yaml
kubectl apply -f backup/secrets-20240101.yaml
kubectl rollout restart deployment/alertmanager-gateway
```

### Disaster Recovery

```bash
# Health check script
#!/bin/bash
if ! curl -f http://localhost:8080/health/ready; then
    echo "Gateway unhealthy, restarting..."
    systemctl restart alertmanager-gateway
fi

# Add to crontab
*/5 * * * * /usr/local/bin/gateway-healthcheck.sh
```

## Scaling

### Horizontal Scaling

The gateway is stateless and can be scaled horizontally:

```yaml
# Kubernetes HPA
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: alertmanager-gateway-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: alertmanager-gateway
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

### Load Balancing

```nginx
# Nginx load balancer
upstream gateway_backend {
    server gateway-1:8080 max_fails=3 fail_timeout=30s;
    server gateway-2:8080 max_fails=3 fail_timeout=30s;
    server gateway-3:8080 max_fails=3 fail_timeout=30s;
}

server {
    location / {
        proxy_pass http://gateway_backend;
        proxy_next_upstream error timeout http_500 http_502 http_503;
    }
}
```

## Troubleshooting

### Common Issues

1. **Gateway won't start**
   ```bash
   # Check configuration
   ./gateway --config=config.yaml --validate
   
   # Check logs
   docker logs alertmanager-gateway
   ```

2. **Authentication failures**
   ```bash
   # Check credentials
   curl -u username:password http://localhost:8080/health
   
   # Check rate limiting
   curl http://localhost:8080/api/v1/health
   ```

3. **High memory usage**
   ```bash
   # Enable profiling
   export ENABLE_PPROF=true
   
   # Analyze memory
   go tool pprof http://localhost:8080/debug/pprof/heap
   ```

### Performance Tuning

```yaml
# High-performance configuration
server:
  read_timeout: "10s"
  write_timeout: "10s"

destinations:
  - name: "high-volume"
    parallel_requests: 5
    batch_size: 20
    retry:
      max_attempts: 3
      backoff: "exponential"
```

## Migration

### Upgrading

```bash
# Rolling update (Kubernetes)
kubectl set image deployment/alertmanager-gateway gateway=alertmanager-gateway:v2.0.0

# Blue-green deployment
kubectl apply -f deployment-v2.yaml
# Test new version
kubectl delete -f deployment-v1.yaml
```

### Configuration Migration

```bash
# Validate new configuration
./gateway-v2 --config=config-v2.yaml --validate

# Migrate with downtime
systemctl stop alertmanager-gateway
cp config-v2.yaml /etc/gateway/config.yaml
systemctl start alertmanager-gateway
```