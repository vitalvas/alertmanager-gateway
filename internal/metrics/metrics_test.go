package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestNewMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	assert.NotNil(t, metrics)
	assert.NotNil(t, metrics.HTTPRequestsTotal)
	assert.NotNil(t, metrics.HTTPRequestDuration)
	assert.NotNil(t, metrics.WebhooksReceived)
	assert.NotNil(t, metrics.AlertsProcessed)
	assert.NotNil(t, metrics.TransformationTime)
	assert.NotNil(t, metrics.DestinationRequests)
	assert.NotNil(t, metrics.AuthenticationAttempts)
	assert.NotNil(t, metrics.AlertSplittingTime)
	assert.NotNil(t, metrics.ActiveConnections)
}

func TestTimer(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	labels := prometheus.Labels{"method": "GET", "path": "/test"}
	timer := metrics.NewTimer(metrics.HTTPRequestDuration, labels)

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	timer.ObserveDuration()

	// Verify the histogram was updated
	histogram := metrics.HTTPRequestDuration.With(labels)
	assert.NotNil(t, histogram)
}

func TestRecordHTTPRequest(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	// Reset metrics for clean testing
	metrics.HTTPRequestsTotal.Reset()

	duration := 100 * time.Millisecond
	requestSize := int64(1024)
	responseSize := int64(2048)

	metrics.RecordHTTPRequest("GET", "/api/v1/test", "200", duration, requestSize, responseSize)

	// Verify counter was incremented
	counter := metrics.HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/test", "200")
	value := testutil.ToFloat64(counter)
	assert.Equal(t, float64(1), value)
}

func TestRecordWebhookProcessing(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	metrics.WebhooksReceived.Reset()

	duration := 50 * time.Millisecond
	metrics.RecordWebhookProcessing("test-dest", "success", "jq", "json", duration)

	// Verify webhook counter was incremented
	counter := metrics.WebhooksReceived.WithLabelValues("test-dest", "success")
	value := testutil.ToFloat64(counter)
	assert.Equal(t, float64(1), value)
}

func TestRecordAlert(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	metrics.AlertsProcessed.Reset()
	metrics.AlertsByStatus.Reset()
	metrics.AlertsBySeverity.Reset()
	metrics.TopDestinations.Reset()

	metrics.RecordAlert("webhook-dest", "firing", "critical")

	// Verify all counters were incremented
	processedCounter := metrics.AlertsProcessed.WithLabelValues("webhook-dest", "firing", "critical")
	assert.Equal(t, float64(1), testutil.ToFloat64(processedCounter))

	statusCounter := metrics.AlertsByStatus.WithLabelValues("firing")
	assert.Equal(t, float64(1), testutil.ToFloat64(statusCounter))

	severityCounter := metrics.AlertsBySeverity.WithLabelValues("critical")
	assert.Equal(t, float64(1), testutil.ToFloat64(severityCounter))

	destCounter := metrics.TopDestinations.WithLabelValues("webhook-dest")
	assert.Equal(t, float64(1), testutil.ToFloat64(destCounter))
}

func TestRecordTransformation(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	metrics.TransformationErrors.Reset()

	duration := 5 * time.Millisecond

	// Test successful transformation
	metrics.RecordTransformation("jq", "test-dest", duration, true)

	// Test failed transformation
	metrics.RecordTransformation("jq", "test-dest", duration, false)

	// Verify error counter was incremented for failed transformation
	errorCounter := metrics.TransformationErrors.WithLabelValues("jq", "test-dest", "execution_error")
	assert.Equal(t, float64(1), testutil.ToFloat64(errorCounter))
}

func TestRecordDestinationRequest(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	metrics.DestinationRequests.Reset()
	metrics.DestinationErrors.Reset()

	duration := 200 * time.Millisecond

	// Test successful request
	metrics.RecordDestinationRequest("webhook", "POST", "200", duration, true)

	// Test failed request
	metrics.RecordDestinationRequest("webhook", "POST", "500", duration, false)

	// Verify counters
	successCounter := metrics.DestinationRequests.WithLabelValues("webhook", "POST", "200")
	assert.Equal(t, float64(1), testutil.ToFloat64(successCounter))

	failCounter := metrics.DestinationRequests.WithLabelValues("webhook", "POST", "500")
	assert.Equal(t, float64(1), testutil.ToFloat64(failCounter))

	errorCounter := metrics.DestinationErrors.WithLabelValues("webhook", "request_failed")
	assert.Equal(t, float64(1), testutil.ToFloat64(errorCounter))
}

func TestRecordAuthAttempt(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	metrics.AuthenticationAttempts.Reset()

	// Test successful auth
	metrics.RecordAuthAttempt("testuser", true)

	// Test failed auth
	metrics.RecordAuthAttempt("testuser", false)

	// Verify counters
	successCounter := metrics.AuthenticationAttempts.WithLabelValues("success", "testuser")
	assert.Equal(t, float64(1), testutil.ToFloat64(successCounter))

	failCounter := metrics.AuthenticationAttempts.WithLabelValues("failure", "testuser")
	assert.Equal(t, float64(1), testutil.ToFloat64(failCounter))
}

func TestRecordRateLimited(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	metrics.RateLimitedRequests.Reset()

	metrics.RecordRateLimited("/webhook/test")

	counter := metrics.RateLimitedRequests.WithLabelValues("/webhook/test")
	assert.Equal(t, float64(1), testutil.ToFloat64(counter))
}

func TestRecordAlertSplitting(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	metrics.SplitAlertsTotal.Reset()
	metrics.SplitBatchesTotal.Reset()

	duration := 150 * time.Millisecond
	metrics.RecordAlertSplitting("test-dest", "parallel", duration, 10, 8, 2, 3)

	// Verify counters
	successCounter := metrics.SplitAlertsTotal.WithLabelValues("test-dest", "parallel", "success")
	assert.Equal(t, float64(8), testutil.ToFloat64(successCounter))

	failureCounter := metrics.SplitAlertsTotal.WithLabelValues("test-dest", "parallel", "failure")
	assert.Equal(t, float64(2), testutil.ToFloat64(failureCounter))

	batchCounter := metrics.SplitBatchesTotal.WithLabelValues("test-dest", "parallel")
	assert.Equal(t, float64(3), testutil.ToFloat64(batchCounter))
}

func TestMetricsIntegration(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	// Simulate a complete request flow
	start := time.Now()

	// 1. HTTP request received
	metrics.ActiveConnections.Inc()

	// 2. Authentication attempted
	metrics.RecordAuthAttempt("apiuser", true)

	// 3. Webhook processing
	webhookDuration := 75 * time.Millisecond
	metrics.RecordWebhookProcessing("slack", "success", "jq", "json", webhookDuration)

	// 4. Alert processing
	metrics.RecordAlert("slack", "firing", "warning")

	// 5. Transformation
	transformDuration := 10 * time.Millisecond
	metrics.RecordTransformation("jq", "slack", transformDuration, true)

	// 6. Destination request
	destDuration := 100 * time.Millisecond
	metrics.RecordDestinationRequest("slack", "POST", "200", destDuration, true)

	// 7. HTTP request completed
	totalDuration := time.Since(start)
	metrics.RecordHTTPRequest("POST", "/webhook/slack", "200", totalDuration, 1024, 512)
	metrics.ActiveConnections.Dec()

	// Verify all metrics were recorded
	httpCounter := metrics.HTTPRequestsTotal.WithLabelValues("POST", "/webhook/slack", "200")
	assert.Equal(t, float64(1), testutil.ToFloat64(httpCounter))

	authCounter := metrics.AuthenticationAttempts.WithLabelValues("success", "apiuser")
	assert.Equal(t, float64(1), testutil.ToFloat64(authCounter))

	webhookCounter := metrics.WebhooksReceived.WithLabelValues("slack", "success")
	assert.Equal(t, float64(1), testutil.ToFloat64(webhookCounter))

	destCounter := metrics.DestinationRequests.WithLabelValues("slack", "POST", "200")
	assert.Equal(t, float64(1), testutil.ToFloat64(destCounter))
}

func TestGaugeMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	// Test active connections gauge
	metrics.ActiveConnections.Set(5)
	assert.Equal(t, float64(5), testutil.ToFloat64(metrics.ActiveConnections))

	metrics.ActiveConnections.Inc()
	assert.Equal(t, float64(6), testutil.ToFloat64(metrics.ActiveConnections))

	metrics.ActiveConnections.Dec()
	assert.Equal(t, float64(5), testutil.ToFloat64(metrics.ActiveConnections))

	// Test banned IPs gauge
	metrics.BannedIPs.Set(3)
	assert.Equal(t, float64(3), testutil.ToFloat64(metrics.BannedIPs))

	// Test memory usage gauge
	metrics.MemoryUsage.Set(1024 * 1024) // 1MB
	assert.Equal(t, float64(1024*1024), testutil.ToFloat64(metrics.MemoryUsage))
}

func TestHistogramBuckets(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	// Simulate different durations
	durations := []time.Duration{
		1 * time.Millisecond,
		10 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
	}

	for _, d := range durations {
		// Simulate the duration
		start := time.Now()
		time.Sleep(d)
		elapsed := time.Since(start)

		metrics.HTTPRequestDuration.WithLabelValues("GET", "/test").Observe(elapsed.Seconds())
	}

	// Verify observations were recorded
	histogram := metrics.HTTPRequestDuration.WithLabelValues("GET", "/test")
	assert.NotNil(t, histogram)
}

func TestMetricsLabels(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	// Test that metrics properly handle different label combinations
	testCases := []struct {
		destination string
		status      string
		severity    string
	}{
		{"webhook1", "firing", "critical"},
		{"webhook1", "firing", "warning"},
		{"webhook1", "resolved", "critical"},
		{"webhook2", "firing", "critical"},
	}

	metrics.AlertsProcessed.Reset()

	for _, tc := range testCases {
		metrics.RecordAlert(tc.destination, tc.status, tc.severity)
	}

	// Verify each combination was recorded correctly
	for _, tc := range testCases {
		counter := metrics.AlertsProcessed.WithLabelValues(tc.destination, tc.status, tc.severity)
		assert.Equal(t, float64(1), testutil.ToFloat64(counter))
	}
}

func TestSystemCollector(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce log noise in tests

	collector := NewSystemCollector(metrics, logger)

	assert.NotNil(t, collector)
	assert.Equal(t, metrics, collector.GetMetrics())

	// Test config reload recording
	collector.RecordConfigReload(true)
	successCounter := metrics.ConfigReloads.WithLabelValues("success")
	assert.Equal(t, float64(1), testutil.ToFloat64(successCounter))

	collector.RecordConfigReload(false)
	failureCounter := metrics.ConfigReloads.WithLabelValues("failure")
	assert.Equal(t, float64(1), testutil.ToFloat64(failureCounter))

	// Test banned IPs update
	collector.UpdateBannedIPs(5)
	assert.Equal(t, float64(5), testutil.ToFloat64(metrics.BannedIPs))

	// Test system metrics collection
	collector.collectSystemMetrics()

	// Memory usage should be set to some positive value
	memUsage := testutil.ToFloat64(metrics.MemoryUsage)
	assert.Greater(t, memUsage, float64(0))
}

func TestHTTPMetricsMiddleware(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	// Create test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	// Create middleware
	middleware := HTTPMetricsMiddleware(metrics)
	handler := middleware(testHandler)

	// Test request
	req := httptest.NewRequest("GET", "/webhook/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify metrics were recorded
	counter := metrics.HTTPRequestsTotal.WithLabelValues("GET", "/webhook/{destination}", "200")
	assert.Equal(t, float64(1), testutil.ToFloat64(counter))
}

func TestActiveConnectionsMiddleware(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	// Initial active connections should be 0
	assert.Equal(t, float64(0), testutil.ToFloat64(metrics.ActiveConnections))

	// Create test handler that checks active connections
	var activeConnectionsDuringRequest float64
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		activeConnectionsDuringRequest = testutil.ToFloat64(metrics.ActiveConnections)
		w.WriteHeader(http.StatusOK)
	})

	// Create middleware
	middleware := ActiveConnectionsMiddleware(metrics)
	handler := middleware(testHandler)

	// Test request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// During request, active connections should be 1
	assert.Equal(t, float64(1), activeConnectionsDuringRequest)

	// After request, active connections should be back to 0
	assert.Equal(t, float64(0), testutil.ToFloat64(metrics.ActiveConnections))
}

func TestSanitizePath(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"/webhook/slack", "/webhook/{destination}"},
		{"/webhook/teams", "/webhook/{destination}"},
		{"/api/v1/destinations/test", "/api/v1/destinations/{name}"},
		{"/api/v1/test/slack", "/api/v1/test/{destination}"},
		{"/health", "/health"},
		{"/health/live", "/health/live"},
		{"/health/ready", "/health/ready"},
		{"/metrics", "/metrics"},
		{"/api/v1/config", "/api/v1/config"},
		{"/unknown/path", "/other"},
		{"/webhook/test?param=value", "/webhook/{destination}"},
	}

	for _, tc := range testCases {
		result := sanitizePath(tc.input)
		assert.Equal(t, tc.expected, result, "Failed for input: %s", tc.input)
	}
}

func TestIsAuthenticatedEndpoint(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
	}{
		{"/webhook/test", true},
		{"/api/v1/destinations", true},
		{"/api/v1/test/slack", true},
		{"/health", false},
		{"/metrics", false},
		{"/unknown", false},
	}

	for _, tc := range testCases {
		result := isAuthenticatedEndpoint(tc.path)
		assert.Equal(t, tc.expected, result, "Failed for path: %s", tc.path)
	}
}

func TestAuthMetricsMiddleware(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)

	// Create test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create middleware
	middleware := AuthMetricsMiddleware(metrics)
	handler := middleware(testHandler)

	// Test authenticated endpoint
	req := httptest.NewRequest("POST", "/webhook/test", nil)
	req.SetBasicAuth("testuser", "testpass")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Test non-authenticated endpoint
	req2 := httptest.NewRequest("GET", "/health", nil)
	w2 := httptest.NewRecorder()

	handler.ServeHTTP(w2, req2)

	// Verify response status
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestSystemCollectorStartStop(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithRegistry(registry)
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	collector := NewSystemCollector(metrics, logger)

	// Start collector
	collector.Start()

	// Give it a moment to run
	time.Sleep(100 * time.Millisecond)

	// Stop collector
	collector.Stop()

	// Verify collector was created properly
	assert.NotNil(t, collector.GetMetrics())
}
