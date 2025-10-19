package server

import (
	"time"

	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
)

// Common response types

type ErrorResponse struct {
	Status    string    `json:"status"`
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"request_id"`
}

// Destination types

type DestinationSummary struct {
	Name        string `json:"name"`
	WebhookURL  string `json:"webhook_url"`
	Method      string `json:"method"`
	Format      string `json:"format"`
	Engine      string `json:"engine"`
	Enabled     bool   `json:"enabled"`
	SplitAlerts bool   `json:"split_alerts"`
	AuthEnabled bool   `json:"auth_enabled"`
	Description string `json:"description"`
}

type ListDestinationsResponse struct {
	Destinations []DestinationSummary `json:"destinations"`
	Total        int                  `json:"total"`
	Timestamp    time.Time            `json:"timestamp"`
}

type DestinationDetails struct {
	Name             string            `json:"name"`
	WebhookURL       string            `json:"webhook_url"`
	Method           string            `json:"method"`
	TargetURL        string            `json:"target_url"`
	Format           string            `json:"format"`
	Engine           string            `json:"engine"`
	Enabled          bool              `json:"enabled"`
	SplitAlerts      bool              `json:"split_alerts"`
	BatchSize        int               `json:"batch_size,omitempty"`
	ParallelRequests int               `json:"parallel_requests,omitempty"`
	Headers          map[string]string `json:"headers"`
	TemplateSize     int               `json:"template_size,omitempty"`
	TransformSize    int               `json:"transform_size,omitempty"`
	HasTemplate      bool              `json:"has_template"`
	HasTransform     bool              `json:"has_transform"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// Test and emulation types

type TestRequest struct {
	WebhookData *alertmanager.WebhookPayload `json:"webhook_data,omitempty"`
	Options     TestOptions                  `json:"options,omitempty"`
}

type TestOptions struct {
	ValidateOnly bool `json:"validate_only"`
	ShowDetails  bool `json:"show_details"`
}

type EmulateRequest struct {
	WebhookData *alertmanager.WebhookPayload `json:"webhook_data,omitempty"`
	DryRun      bool                         `json:"dry_run"`
	Options     EmulateOptions               `json:"options,omitempty"`
}

type EmulateOptions struct {
	Timeout         string `json:"timeout,omitempty"`
	FollowRedirects bool   `json:"follow_redirects"`
	ValidateSSL     bool   `json:"validate_ssl"`
}

type TransformationResult struct {
	TransformedData interface{}   `json:"transformed_data"`
	FormattedOutput string        `json:"formatted_output"`
	TransformTime   time.Duration `json:"transform_time"`
	OutputSize      int           `json:"output_size"`
	OutputFormat    string        `json:"output_format"`
	SplitMode       bool          `json:"split_mode"`
	AlertsProcessed int           `json:"alerts_processed"`
}

type TestResponse struct {
	Success       bool                  `json:"success"`
	Destination   string                `json:"destination"`
	Result        *TransformationResult `json:"result"`
	TestTimestamp time.Time             `json:"test_timestamp"`
	RequestID     string                `json:"request_id"`
}

type EmulationResult struct {
	TransformationResult
	HTTPMethod      string            `json:"http_method"`
	TargetURL       string            `json:"target_url"`
	Headers         map[string]string `json:"headers"`
	RequestSize     int               `json:"request_size"`
	HTTPStatusCode  int               `json:"http_status_code"`
	HTTPStatus      string            `json:"http_status"`
	HTTPError       string            `json:"http_error,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	Success         bool              `json:"success"`
	EmulationTime   time.Duration     `json:"emulation_time"`
}

type EmulateResponse struct {
	Success            bool             `json:"success"`
	Destination        string           `json:"destination"`
	DryRun             bool             `json:"dry_run"`
	Result             *EmulationResult `json:"result"`
	EmulationTimestamp time.Time        `json:"emulation_timestamp"`
	RequestID          string           `json:"request_id"`
}

// System information types

type ConfigInfo struct {
	DestinationsCount        int    `json:"destinations_count"`
	EnabledDestinationsCount int    `json:"enabled_destinations_count"`
	AuthEnabled              bool   `json:"auth_enabled"`
	ServerAddress            string `json:"server_address"`
	LogLevel                 string `json:"log_level"`
}

type SystemInfo struct {
	Version          string        `json:"version"`
	BuildTime        time.Time     `json:"build_time"`
	GoVersion        string        `json:"go_version"`
	NumCPU           int           `json:"num_cpu"`
	NumGoroutines    int           `json:"num_goroutines"`
	MemoryAlloc      uint64        `json:"memory_alloc"`
	MemoryTotalAlloc uint64        `json:"memory_total_alloc"`
	MemorySys        uint64        `json:"memory_sys"`
	NumGC            uint32        `json:"num_gc"`
	Uptime           time.Duration `json:"uptime"`
	Config           ConfigInfo    `json:"config"`
}

type HealthCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "healthy", "warning", "error"
	Message string `json:"message"`
}

type HealthResponse struct {
	Status              string        `json:"status"`
	Timestamp           time.Time     `json:"timestamp"`
	UptimeSeconds       float64       `json:"uptime_seconds"`
	ConfigLoaded        bool          `json:"config_loaded"`
	DestinationsCount   int           `json:"destinations_count"`
	EnabledDestinations int           `json:"enabled_destinations"`
	Checks              []HealthCheck `json:"checks"`
}

// Configuration validation types

type ConfigValidation struct {
	Valid     bool      `json:"valid"`
	Errors    []string  `json:"errors"`
	Warnings  []string  `json:"warnings"`
	Timestamp time.Time `json:"timestamp"`
}

// Statistics types

type DestinationStats struct {
	Name            string        `json:"name"`
	RequestCount    int64         `json:"request_count"`
	SuccessCount    int64         `json:"success_count"`
	ErrorCount      int64         `json:"error_count"`
	AverageLatency  time.Duration `json:"average_latency"`
	LastRequestTime time.Time     `json:"last_request_time"`
	LastError       string        `json:"last_error,omitempty"`
}

type SystemStats struct {
	TotalRequests       int64              `json:"total_requests"`
	TotalWebhooks       int64              `json:"total_webhooks"`
	TotalErrors         int64              `json:"total_errors"`
	UptimeSeconds       float64            `json:"uptime_seconds"`
	DestinationStats    []DestinationStats `json:"destination_stats"`
	MemoryUsage         uint64             `json:"memory_usage"`
	GoroutineCount      int                `json:"goroutine_count"`
	LastConfigReload    time.Time          `json:"last_config_reload"`
	CollectionTimestamp time.Time          `json:"collection_timestamp"`
}

// Webhook simulation types

type WebhookSimulation struct {
	AlertCount        int               `json:"alert_count"`
	Status            string            `json:"status"` // "firing", "resolved"
	Severity          string            `json:"severity"`
	CustomLabels      map[string]string `json:"custom_labels,omitempty"`
	CustomAnnotations map[string]string `json:"custom_annotations,omitempty"`
	Template          string            `json:"template,omitempty"` // For custom alert templates
}

type SimulationRequest struct {
	Destination string            `json:"destination"`
	Simulation  WebhookSimulation `json:"simulation"`
	Options     SimulationOptions `json:"options,omitempty"`
}

type SimulationOptions struct {
	GenerateMultiple bool   `json:"generate_multiple"`
	Count            int    `json:"count,omitempty"`
	Interval         string `json:"interval,omitempty"`
}

type SimulationResponse struct {
	Success           bool                           `json:"success"`
	Destination       string                         `json:"destination"`
	GeneratedWebhooks []*alertmanager.WebhookPayload `json:"generated_webhooks"`
	Results           []*EmulationResult             `json:"results"`
	SimulationTime    time.Duration                  `json:"simulation_time"`
	Timestamp         time.Time                      `json:"timestamp"`
	RequestID         string                         `json:"request_id"`
}
