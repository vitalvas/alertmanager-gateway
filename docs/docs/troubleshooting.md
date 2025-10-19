# Troubleshooting Guide

This guide helps you diagnose and resolve common issues with the Alertmanager Gateway.

## Common Issues

### Gateway Won't Start

#### Symptom
The gateway fails to start or crashes immediately after starting.

#### Possible Causes & Solutions

1. **Port Already in Use**
   ```bash
   # Check if port is in use
   lsof -i :8080
   # or
   netstat -an | grep 8080
   ```
   
   Solution: Change the port in configuration or stop the conflicting service.

2. **Invalid Configuration**
   ```bash
   # Validate configuration
   alertmanager-gateway -config config.yaml -validate
   ```
   
   Common configuration errors:
   - Missing required fields
   - Invalid YAML syntax
   - Incorrect engine type (must be "go-template" or "jq")
   - Invalid URL format

3. **Missing Environment Variables**
   ```bash
   # Check if variables are set
   env | grep -E "GATEWAY_|SLACK_|API_"
   ```
   
   Solution: Export required environment variables or use a `.env` file.

4. **Permission Issues**
   ```bash
   # Check file permissions
   ls -la config.yaml
   ```
   
   Solution: Ensure the gateway has read access to the configuration file.

### Alerts Not Being Received

#### Symptom
Alertmanager shows successful webhook delivery, but the gateway doesn't process alerts.

#### Possible Causes & Solutions

1. **Network Connectivity Issues**
   - Verify network connectivity between Alertmanager and gateway
   - Check for firewall rules blocking traffic
   
   Test with curl:
   ```bash
   curl -v -X POST http://gateway:8080/webhook/destination \
     -H "Content-Type: application/json" \
     -d @test-alert.json
   ```

2. **Incorrect Webhook URL**
   - Verify the destination name in URL matches configuration
   - Check for typos in the path
   
   Correct format: `/webhook/{destination-name}`

3. **Network Connectivity**
   ```bash
   # From Alertmanager host
   curl -I http://gateway-host:8080/health
   ```

4. **Gateway Behind Proxy/Load Balancer**
   - Ensure proper header forwarding (X-Forwarded-For, X-Real-IP)
   - Check timeout settings on proxy/LB

### Alerts Not Forwarded to Destination

#### Symptom
Gateway receives alerts but doesn't forward them to the configured destination.

#### Possible Causes & Solutions

1. **Destination Disabled**
   Check if destination is enabled:
   ```yaml
   destinations:
     - name: slack
       enabled: false  # Should be true
   ```

2. **Template/Transform Errors**
   Test transformation:
   ```bash
   curl -X POST http://gateway:8080/api/v1/test/destination-name \
     -H "Content-Type: application/json" \
     -d @webhook-payload.json
   ```

3. **Invalid Template Syntax**
   Common template errors:
   - Missing closing braces `}}`
   - Incorrect function names
   - Invalid JSON output
   
   Example fix:
   ```yaml
   # Bad
   template: '{"text": {{.Status}}'  # Missing quotes and closing brace
   
   # Good
   template: '{"text": "{{.Status}}"}'
   ```

4. **JQ Transform Errors**
   Common JQ errors:
   - Undefined variables
   - Type mismatches
   - Invalid function calls
   
   Test JQ locally:
   ```bash
   echo '{"status":"firing"}' | jq '.status | ascii_upcase'
   ```


### High Memory Usage

#### Symptom
Gateway memory usage continuously increases.

#### Possible Causes & Solutions

1. **Template Cache Growth**
   - Templates are cached indefinitely by default
   - Consider restarting periodically or implementing cache limits

2. **Dead Letter Queue Growth**
   - Failed alerts accumulate in memory
   - Check for persistent failures to specific destinations
   - Consider implementing persistent dead letter queue

3. **Connection Pool Leaks**
   - Ensure all HTTP connections are properly closed
   - Check for destinations with very long timeouts

### Slow Performance

#### Symptom
High latency when processing alerts or API calls.

#### Possible Causes & Solutions

1. **Synchronous Processing Bottleneck**
   - Gateway processes webhooks asynchronously
   - Check destination response times
   
   Monitor with metrics:
   ```bash
   curl http://gateway:8080/metrics | grep duration
   ```

2. **Template Compilation Overhead**
   - Complex templates take time to compile
   - Templates are cached after first use
   - Simplify templates where possible

3. **Network Latency**
   - Check latency to destination endpoints
   - Consider increasing timeouts for slow endpoints

4. **Resource Constraints**
   - Check CPU and memory usage
   - Increase resources if needed
   - Consider running multiple gateway instances

### Connection Issues

#### Symptom
Gateway is not reachable or connections are being refused.

#### Debugging Steps

1. **Verify Gateway is Running**
   ```bash
   # Test gateway health
   curl -v http://gateway:8080/health
   ```

2. **Check Network Connectivity**
   Verify there are no network issues:
   - Check firewall rules
   - Verify DNS resolution
   - Test basic connectivity with ping/telnet

3. **Review Gateway Logs**
   Check gateway logs for connection errors:
   ```bash
   tail -f gateway.log | grep -i "connection\|error"
   ```

### Template Function Errors

#### Common Function Issues

1. **Time Functions**
   ```go
   # Wrong
   {{now | date "2006-01-02 15:04:05"}}  # Spaces need quotes
   
   # Right
   {{now | date "2006-01-02T15:04:05Z"}}
   ```

2. **Nil Pointer Errors**
   ```go
   # Wrong
   {{.CommonLabels.severity}}  # Fails if CommonLabels is nil
   
   # Right
   {{index .CommonLabels "severity" | default "unknown"}}
   ```

3. **Type Mismatches**
   ```go
   # Wrong
   {{.Alerts | len}}  # Wrong order
   
   # Right
   {{len .Alerts}}
   ```

### Debugging Techniques

#### Enable Debug Logging

Set log level in configuration or via environment:
```bash
export LOG_LEVEL=debug
./alertmanager-gateway -config config.yaml
```

#### Check Gateway Logs

Look for specific error patterns:
```bash
# Template errors
grep -i "template" gateway.log

# Connection errors
grep -i "connection\|timeout" gateway.log

# Connection errors
grep -i "connection\|refused" gateway.log
```

#### Use Test Endpoints

1. **Test Transformation**
   ```bash
   curl -X POST http://gateway:8080/api/v1/test/slack \
     -H "Content-Type: application/json" \
     -d @test-payload.json
   ```

2. **Emulate Request**
   ```bash
   curl -X POST http://gateway:8080/api/v1/emulate/slack?dry_run=true \
     -H "Content-Type: application/json" \
     -d @test-payload.json
   ```

3. **Check Destination Status**
   ```bash
   curl http://gateway:8080/api/v1/destinations | jq
   ```

#### Monitor Metrics

Key metrics to watch:
```bash
# Request rate
curl -s http://gateway:8080/metrics | grep 'gateway_requests_total'

# Error rate
curl -s http://gateway:8080/metrics | grep 'gateway_requests_total.*status="5'

# Processing duration
curl -s http://gateway:8080/metrics | grep 'gateway_request_duration_seconds'

# Active connections
curl -s http://gateway:8080/metrics | grep 'gateway_active_connections'
```

### Integration-Specific Issues

#### Slack
- **Invalid webhook URL**: Returns 404 or 403
- **Rate limits**: 1 message per second per webhook
- **Message too large**: Max 40,000 characters
- **Invalid JSON**: Check template output is valid JSON

#### PagerDuty
- **Invalid routing key**: Returns 400
- **Dedup key conflicts**: Use unique groupKey
- **Invalid severity**: Must be critical, error, warning, or info

#### Microsoft Teams
- **Webhook expired**: Regenerate webhook URL
- **Invalid MessageCard format**: Validate against schema
- **Size limits**: Max 28KB per message

#### Telegram
- **Invalid bot token**: Returns 401
- **Chat ID not found**: Verify chat ID is correct
- **Parse mode errors**: Use "Markdown" or "HTML"
- **Rate limits**: 30 messages per second

### Performance Tuning

#### Optimize Templates

1. **Minimize template complexity**
   ```yaml
   # Inefficient
   template: |
     {{range .Alerts}}{{range .Labels}}...{{end}}{{end}}
   
   # Better
   template: |
     {{.GroupLabels.alertname}}: {{len .Alerts}} alerts
   ```

2. **Pre-compute values**
   ```go
   {{$severity := index .CommonLabels "severity" | default "unknown"}}
   {{$severity}} appears here
   {{$severity}} and here
   ```

#### Connection Pooling

Configure appropriate connection settings:
```yaml
# In destination config
http_client:
  timeout: 30s
  max_idle_conns: 100
  max_idle_conns_per_host: 10
  idle_conn_timeout: 90s
```

#### Batch Processing

For high-volume alerts:
```yaml
split_alerts: true
batch_size: 50
parallel_requests: 5
```

### Recovery Procedures

#### Clear Connection Pool

1. Restart the gateway to clear connection pools
2. Check for connection leaks in destination systems
3. Increase connection timeouts if needed

#### Recover from Template Errors

1. Fix template syntax
2. Validate with test endpoint
3. Reload configuration (if hot-reload implemented)
4. Or restart gateway

## Getting Help

### Collect Diagnostic Information

When reporting issues, include:

1. **Gateway version**
   ```bash
   alertmanager-gateway -version
   ```

2. **Configuration** (sanitized)
   ```bash
   # Remove sensitive data
   grep -v "password\|token\|key" config.yaml
   ```

3. **Error logs**
   ```bash
   tail -n 100 gateway.log | grep -i error
   ```

4. **Test webhook payload**
   ```bash
   curl http://gateway:8080/api/v1/test/destination \
     -H "Content-Type: application/json" \
     -d @test.json
   ```

5. **Metrics snapshot**
   ```bash
   curl http://gateway:8080/metrics > metrics.txt
   ```

### Debug Mode

Run gateway with verbose logging:
```bash
alertmanager-gateway -config config.yaml -log-level debug 2>&1 | tee debug.log
```

### Health Checks

Implement monitoring:
```yaml
# Kubernetes
livenessProbe:
  httpGet:
    path: /health/live
    port: 8080
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /health/ready
    port: 8080
  periodSeconds: 5
```

### Common Log Patterns

```bash
# Successful forwarding
INFO[0010] Forwarding alert  destination=slack status=firing

# Template error
ERROR[0015] Template execution failed  error="template: :1:2: executing "" at <.Foo>: can't evaluate field Foo"

# Connection error
ERROR[0020] Failed to forward alert  error="Post \"https://example.com\": dial tcp: i/o timeout"

# Rate limit
WARN[0025] Rate limit exceeded  ip=10.0.0.1 attempts=5
```