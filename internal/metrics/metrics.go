package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all the Prometheus metrics for the application
type Metrics struct {
	// HTTP Request metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRequestSize     *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec

	// Webhook processing metrics
	WebhooksReceived      *prometheus.CounterVec
	WebhookProcessingTime *prometheus.HistogramVec
	AlertsProcessed       *prometheus.CounterVec

	// Transformation metrics
	TransformationTime   *prometheus.HistogramVec
	TransformationErrors *prometheus.CounterVec
	TemplateCompilations *prometheus.CounterVec

	// Destination metrics
	DestinationRequests *prometheus.CounterVec
	DestinationErrors   *prometheus.CounterVec
	DestinationDuration *prometheus.HistogramVec

	// Authentication metrics
	AuthenticationAttempts *prometheus.CounterVec
	RateLimitedRequests    *prometheus.CounterVec
	BannedIPs              prometheus.Gauge

	// Alert splitting metrics
	AlertSplittingTime *prometheus.HistogramVec
	SplitAlertsTotal   *prometheus.CounterVec
	SplitBatchesTotal  *prometheus.CounterVec

	// System metrics
	ActiveConnections prometheus.Gauge
	ConfigReloads     *prometheus.CounterVec
	MemoryUsage       prometheus.Gauge

	// Business metrics
	AlertsByStatus   *prometheus.CounterVec
	AlertsBySeverity *prometheus.CounterVec
	TopDestinations  *prometheus.CounterVec
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics() *Metrics {
	return NewMetricsWithRegistry(prometheus.DefaultRegisterer)
}

// NewMetricsWithRegistry creates metrics with a custom registry (useful for testing)
func NewMetricsWithRegistry(registerer prometheus.Registerer) *Metrics {
	factory := promauto.With(registerer)
	return &Metrics{
		// HTTP Request metrics
		HTTPRequestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_http_requests_total",
				Help: "Total number of HTTP requests processed",
			},
			[]string{"method", "path", "status_code"},
		),

		HTTPRequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "alertmanager_gateway_http_request_duration_seconds",
				Help:    "Duration of HTTP requests in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),

		HTTPRequestSize: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "alertmanager_gateway_http_request_size_bytes",
				Help:    "Size of HTTP requests in bytes",
				Buckets: prometheus.ExponentialBuckets(64, 2, 16), // 64B to 2MB
			},
			[]string{"method", "path"},
		),

		HTTPResponseSize: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "alertmanager_gateway_http_response_size_bytes",
				Help:    "Size of HTTP responses in bytes",
				Buckets: prometheus.ExponentialBuckets(64, 2, 16), // 64B to 2MB
			},
			[]string{"method", "path", "status_code"},
		),

		// Webhook processing metrics
		WebhooksReceived: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_webhooks_received_total",
				Help: "Total number of webhooks received",
			},
			[]string{"destination", "status"},
		),

		WebhookProcessingTime: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "alertmanager_gateway_webhook_processing_duration_seconds",
				Help:    "Time spent processing webhooks in seconds",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"destination", "engine", "format"},
		),

		AlertsProcessed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_alerts_processed_total",
				Help: "Total number of alerts processed",
			},
			[]string{"destination", "status", "severity"},
		),

		// Transformation metrics
		TransformationTime: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "alertmanager_gateway_transformation_duration_seconds",
				Help:    "Time spent on transformations in seconds",
				Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5},
			},
			[]string{"engine", "destination"},
		),

		TransformationErrors: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_transformation_errors_total",
				Help: "Total number of transformation errors",
			},
			[]string{"engine", "destination", "error_type"},
		),

		TemplateCompilations: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_template_compilations_total",
				Help: "Total number of template compilations",
			},
			[]string{"engine", "status"},
		),

		// Destination metrics
		DestinationRequests: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_destination_requests_total",
				Help: "Total number of requests sent to destinations",
			},
			[]string{"destination", "method", "status_code"},
		),

		DestinationErrors: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_destination_errors_total",
				Help: "Total number of destination request errors",
			},
			[]string{"destination", "error_type"},
		),

		DestinationDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "alertmanager_gateway_destination_request_duration_seconds",
				Help:    "Duration of destination requests in seconds",
				Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
			},
			[]string{"destination", "method"},
		),

		// Authentication metrics
		AuthenticationAttempts: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_auth_attempts_total",
				Help: "Total number of authentication attempts",
			},
			[]string{"result", "username"},
		),

		RateLimitedRequests: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_rate_limited_requests_total",
				Help: "Total number of rate-limited requests",
			},
			[]string{"endpoint"},
		),

		BannedIPs: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "alertmanager_gateway_banned_ips",
				Help: "Current number of banned IP addresses",
			},
		),

		// Alert splitting metrics
		AlertSplittingTime: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "alertmanager_gateway_alert_splitting_duration_seconds",
				Help:    "Time spent splitting alerts in seconds",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
			},
			[]string{"destination", "strategy"},
		),

		SplitAlertsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_split_alerts_total",
				Help: "Total number of alerts processed in split mode",
			},
			[]string{"destination", "strategy", "result"},
		),

		SplitBatchesTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_split_batches_total",
				Help: "Total number of batches processed in split mode",
			},
			[]string{"destination", "strategy"},
		),

		// System metrics
		ActiveConnections: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "alertmanager_gateway_active_connections",
				Help: "Current number of active HTTP connections",
			},
		),

		ConfigReloads: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_config_reloads_total",
				Help: "Total number of configuration reloads",
			},
			[]string{"status"},
		),

		MemoryUsage: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "alertmanager_gateway_memory_usage_bytes",
				Help: "Current memory usage in bytes",
			},
		),

		// Business metrics
		AlertsByStatus: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_alerts_by_status_total",
				Help: "Total number of alerts by status",
			},
			[]string{"status"},
		),

		AlertsBySeverity: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_alerts_by_severity_total",
				Help: "Total number of alerts by severity",
			},
			[]string{"severity"},
		),

		TopDestinations: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "alertmanager_gateway_top_destinations_total",
				Help: "Total requests by destination for trending analysis",
			},
			[]string{"destination"},
		),
	}
}

// Timer helps measure duration for histograms
type Timer struct {
	histogram *prometheus.HistogramVec
	labels    prometheus.Labels
	start     time.Time
}

// NewTimer creates a new timer for measuring durations
func (m *Metrics) NewTimer(histogram *prometheus.HistogramVec, labels prometheus.Labels) *Timer {
	return &Timer{
		histogram: histogram,
		labels:    labels,
		start:     time.Now(),
	}
}

// ObserveDuration observes the duration since timer creation
func (t *Timer) ObserveDuration() {
	duration := time.Since(t.start).Seconds()
	t.histogram.With(t.labels).Observe(duration)
}

// Helper methods for common metric operations

// RecordHTTPRequest records an HTTP request with all relevant metrics
func (m *Metrics) RecordHTTPRequest(method, path, statusCode string, duration time.Duration, requestSize, responseSize int64) {
	labels := prometheus.Labels{
		"method": method,
		"path":   path,
	}

	m.HTTPRequestsTotal.With(prometheus.Labels{
		"method":      method,
		"path":        path,
		"status_code": statusCode,
	}).Inc()

	m.HTTPRequestDuration.With(labels).Observe(duration.Seconds())
	m.HTTPRequestSize.With(labels).Observe(float64(requestSize))
	m.HTTPResponseSize.With(prometheus.Labels{
		"method":      method,
		"path":        path,
		"status_code": statusCode,
	}).Observe(float64(responseSize))
}

// RecordWebhookProcessing records webhook processing metrics
func (m *Metrics) RecordWebhookProcessing(destination, status, engine, format string, duration time.Duration) {
	m.WebhooksReceived.With(prometheus.Labels{
		"destination": destination,
		"status":      status,
	}).Inc()

	m.WebhookProcessingTime.With(prometheus.Labels{
		"destination": destination,
		"engine":      engine,
		"format":      format,
	}).Observe(duration.Seconds())
}

// RecordAlert records alert processing metrics
func (m *Metrics) RecordAlert(destination, status, severity string) {
	m.AlertsProcessed.With(prometheus.Labels{
		"destination": destination,
		"status":      status,
		"severity":    severity,
	}).Inc()

	m.AlertsByStatus.With(prometheus.Labels{"status": status}).Inc()
	m.AlertsBySeverity.With(prometheus.Labels{"severity": severity}).Inc()
	m.TopDestinations.With(prometheus.Labels{"destination": destination}).Inc()
}

// RecordTransformation records transformation metrics
func (m *Metrics) RecordTransformation(engine, destination string, duration time.Duration, success bool) {
	m.TransformationTime.With(prometheus.Labels{
		"engine":      engine,
		"destination": destination,
	}).Observe(duration.Seconds())

	if !success {
		m.TransformationErrors.With(prometheus.Labels{
			"engine":      engine,
			"destination": destination,
			"error_type":  "execution_error",
		}).Inc()
	}
}

// RecordDestinationRequest records destination request metrics
func (m *Metrics) RecordDestinationRequest(destination, method, statusCode string, duration time.Duration, success bool) {
	m.DestinationRequests.With(prometheus.Labels{
		"destination": destination,
		"method":      method,
		"status_code": statusCode,
	}).Inc()

	m.DestinationDuration.With(prometheus.Labels{
		"destination": destination,
		"method":      method,
	}).Observe(duration.Seconds())

	if !success {
		m.DestinationErrors.With(prometheus.Labels{
			"destination": destination,
			"error_type":  "request_failed",
		}).Inc()
	}
}

// RecordAuthAttempt records authentication attempt metrics
func (m *Metrics) RecordAuthAttempt(username string, success bool) {
	result := "failure"
	if success {
		result = "success"
	}

	m.AuthenticationAttempts.With(prometheus.Labels{
		"result":   result,
		"username": username,
	}).Inc()
}

// RecordRateLimited records rate limiting metrics
func (m *Metrics) RecordRateLimited(endpoint string) {
	m.RateLimitedRequests.With(prometheus.Labels{
		"endpoint": endpoint,
	}).Inc()
}

// RecordAlertSplitting records alert splitting metrics
func (m *Metrics) RecordAlertSplitting(destination, strategy string, duration time.Duration, _, successCount, failureCount, batches int) {
	m.AlertSplittingTime.With(prometheus.Labels{
		"destination": destination,
		"strategy":    strategy,
	}).Observe(duration.Seconds())

	m.SplitAlertsTotal.With(prometheus.Labels{
		"destination": destination,
		"strategy":    strategy,
		"result":      "success",
	}).Add(float64(successCount))

	m.SplitAlertsTotal.With(prometheus.Labels{
		"destination": destination,
		"strategy":    strategy,
		"result":      "failure",
	}).Add(float64(failureCount))

	m.SplitBatchesTotal.With(prometheus.Labels{
		"destination": destination,
		"strategy":    strategy,
	}).Add(float64(batches))
}
