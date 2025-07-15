# Configuration Examples

This directory contains example configurations for various popular integrations with the Alertmanager Gateway.

## Available Examples

### Communication Platforms

- **[slack.yaml](./slack.yaml)** - Slack integration with rich message formatting
- **[teams.yaml](./teams.yaml)** - Microsoft Teams webhook integration
- **[discord.yaml](./discord.yaml)** - Discord webhook with embeds
- **[telegram.yaml](./telegram.yaml)** - Telegram Bot API integration

### Incident Management

- **[pagerduty.yaml](./pagerduty.yaml)** - PagerDuty Events API v2 integration
- **[email.yaml](./email.yaml)** - Email notifications via SendGrid or generic webhook

### Monitoring & Analytics

- **[splunk.yaml](./splunk.yaml)** - Splunk HTTP Event Collector (HEC) integration
- **[multi-destination.yaml](./multi-destination.yaml)** - Advanced routing based on severity and labels

## Quick Start

1. Copy the example configuration that matches your needs:
   ```bash
   cp examples/slack.yaml config.yaml
   ```

2. Set the required environment variables:
   ```bash
   export GATEWAY_PASSWORD="your-secure-password"
   export SLACK_WEBHOOK_URL="https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
   ```

3. Start the gateway:
   ```bash
   alertmanager-gateway -config config.yaml
   ```

4. Configure Alertmanager to send webhooks to the gateway:
   ```yaml
   # alertmanager.yml
   receivers:
     - name: 'gateway'
       webhook_configs:
         - url: 'http://gateway:8080/webhook/slack'
           http_config:
             basic_auth:
               username: 'alertmanager'
               password: 'your-secure-password'
   ```

## Configuration Tips

### Environment Variables

All examples use environment variables for sensitive data:

- `${VARIABLE_NAME}` - Required variable
- `${VARIABLE_NAME:-default}` - Variable with default value

### Template Engines

- **go-template**: Best for simple transformations and when you need Go's template functions
- **jq**: More powerful for complex JSON transformations and filtering

### Output Formats

- **json**: Standard JSON output (most common)
- **form**: URL-encoded form data (for legacy systems)
- **query**: Query parameters (for GET requests)
- **xml**: XML output (rare, but supported)

### Alert Splitting

Enable `split_alerts: true` to send each alert individually. Useful for:
- Ticketing systems (one ticket per alert)
- Notification systems with message limits
- Systems that can't handle bulk updates

### Retry Configuration

```yaml
retry:
  max_attempts: 3          # Maximum number of retry attempts
  backoff: exponential     # Backoff strategy: exponential, linear, constant
  initial_delay: 1s        # Initial delay before first retry
  max_delay: 30s          # Maximum delay between retries
```

### Circuit Breaker

Protect against cascading failures:

```yaml
circuit_breaker:
  failure_threshold: 5     # Failures before circuit opens
  success_threshold: 2     # Successes needed to close circuit
  timeout: 30s            # Time before trying half-open state
  half_open_requests: 3   # Requests allowed in half-open state
```

## Common Patterns

### Severity-Based Routing

Route alerts to different destinations based on severity:

1. Create separate destination configs for each severity level
2. Use template conditions to filter alerts
3. Configure different retry/circuit breaker settings per severity

### Environment-Based Filtering

Send production alerts to PagerDuty, development to Slack:

```yaml
# In your transform
if .commonLabels.env == "production" then
  # Production alert handling
else
  # Non-production handling
end
```

### Time-Based Routing

Route alerts differently during business hours:

```go
{{$hour := now | date "15"}}
{{if and (ge $hour "09") (lt $hour "18")}}
  # Business hours logic
{{else}}
  # After hours logic
{{end}}
```

### Alert Deduplication

Use `fingerprint` or `groupKey` as unique identifiers:

```yaml
transform: |
  {
    dedup_key: .groupKey,
    alert_id: .alerts[0].fingerprint
  }
```

## Testing

Test your configuration without sending actual alerts:

```bash
# Test transformation
curl -X POST http://localhost:8080/api/v1/test/slack \
  -u alertmanager:your-secure-password \
  -H "Content-Type: application/json" \
  -d @test-webhook.json

# Dry run (generate request without sending)
curl -X POST http://localhost:8080/api/v1/emulate/slack?dry_run=true \
  -u alertmanager:your-secure-password \
  -H "Content-Type: application/json" \
  -d @test-webhook.json
```

## Security Best Practices

1. **Always enable authentication** in production
2. **Use environment variables** for sensitive data
3. **Set appropriate timeouts** to prevent hanging connections
4. **Enable retry logic** for critical destinations
5. **Monitor the gateway** using the `/metrics` endpoint
6. **Rotate credentials** regularly
7. **Use HTTPS** for external webhooks

## Troubleshooting

If alerts aren't being delivered:

1. Check gateway logs for errors
2. Use the test endpoint to validate transformations
3. Verify environment variables are set
4. Check destination service is accessible
5. Monitor circuit breaker status
6. Validate webhook payload format

For more help, see the [main documentation](../../docs/).