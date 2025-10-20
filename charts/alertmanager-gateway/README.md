# Alertmanager Gateway Helm Chart

Universal adapter for Prometheus Alertmanager webhooks that transforms and routes alerts to various third-party notification systems.

## Installation

### Install from OCI Registry (Recommended)

```bash
helm install my-gateway oci://ghcr.io/vitalvas/alertmanager-gateway/charts/alertmanager-gateway
```

### Install specific version

```bash
helm install my-gateway oci://ghcr.io/vitalvas/alertmanager-gateway/charts/alertmanager-gateway --version <VERSION>
```

### Install from local chart

```bash
helm install my-gateway ./charts/alertmanager-gateway
```

### Install with custom values

```bash
helm install my-gateway oci://ghcr.io/vitalvas/alertmanager-gateway/charts/alertmanager-gateway \
  --version <VERSION> \
  -f values.yaml
```

## Configuration

The following table lists the configurable parameters of the chart and their default values.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Image repository | `ghcr.io/vitalvas/alertmanager-gateway` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `image.tag` | Image tag (set to release version automatically) | `""` (uses chart version) |
| `service.type` | Service type | `ClusterIP` |
| `service.port` | Service port (external) | `80` |
| `service.targetPort` | Container target port | `8080` |
| `ingress.enabled` | Enable ingress | `false` |
| `resources.limits.cpu` | CPU limit | `200m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `64Mi` |
| `config.destinations` | Destination configurations | `[]` |

## Examples

### Basic installation with Slack webhook

```yaml
config:
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
          "text": "*Alert:* {{ .GroupLabels.alertname }} - {{ .Status | upper }}",
          "attachments": [{
            "color": "{{ if eq .Status \"firing\" }}danger{{ else }}good{{ end }}",
            "title": "{{ .CommonAnnotations.summary }}",
            "text": "{{ .CommonAnnotations.description }}"
          }]
        }
```

### With ingress enabled

```yaml
ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
  hosts:
    - host: alertmanager-gateway.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: alertmanager-gateway-tls
      hosts:
        - alertmanager-gateway.example.com
```

## Prometheus Alertmanager Configuration

Configure Alertmanager to send webhooks to the gateway:

```yaml
receivers:
  - name: 'slack-notifications'
    webhook_configs:
      - url: 'http://my-gateway-alertmanager-gateway/webhook/slack'
        send_resolved: true
```

## Upgrading

```bash
helm upgrade my-gateway oci://ghcr.io/vitalvas/alertmanager-gateway/charts/alertmanager-gateway \
  --version <VERSION> \
  -f values.yaml
```

## Uninstalling

```bash
helm uninstall my-gateway
```
