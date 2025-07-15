package destination

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/formatter"
	"github.com/vitalvas/alertmanager-gateway/internal/transform"
)

// Handler handles sending alerts to a destination
type Handler interface {
	// Send sends the alert data to the destination
	Send(ctx context.Context, payload *alertmanager.WebhookPayload) error

	// Name returns the destination name
	Name() string

	// Close cleans up any resources
	Close() error
}

// HTTPHandler is a generic HTTP destination handler
type HTTPHandler struct {
	config   *config.DestinationConfig
	client   *HTTPClient
	engine   transform.Engine
	logger   *logrus.Entry
	splitter *AlertSplitter
}

// NewHTTPHandler creates a new HTTP destination handler
func NewHTTPHandler(cfg *config.DestinationConfig, clientConfig *HTTPClientConfig) (*HTTPHandler, error) {
	if cfg == nil {
		return nil, fmt.Errorf("destination config is required")
	}

	// Create transform engine based on config
	var engine transform.Engine
	var err error

	switch cfg.Engine {
	case "go-template":
		if cfg.Template == "" {
			return nil, fmt.Errorf("template is required for go-template engine")
		}
		engine, err = transform.NewEngine(transform.EngineTypeGoTemplate, cfg.Template)
	case "jq":
		if cfg.Transform == "" {
			return nil, fmt.Errorf("transform is required for jq engine")
		}
		engine, err = transform.NewEngine(transform.EngineTypeJQ, cfg.Transform)
	default:
		return nil, fmt.Errorf("unknown engine type: %s", cfg.Engine)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create transform engine: %w", err)
	}

	// Create HTTP client
	if clientConfig == nil {
		clientConfig = DefaultHTTPClientConfig()
	}
	client := NewHTTPClient(clientConfig)

	logger := logrus.WithFields(logrus.Fields{
		"component":   "destination",
		"destination": cfg.Name,
	})

	// Create alert splitter
	splitter := NewAlertSplitter(cfg, logger)

	return &HTTPHandler{
		config:   cfg,
		client:   client,
		engine:   engine,
		logger:   logger,
		splitter: splitter,
	}, nil
}

// Send sends the alert data to the destination
func (h *HTTPHandler) Send(ctx context.Context, payload *alertmanager.WebhookPayload) error {
	startTime := time.Now()

	// Check if split mode is enabled
	if h.config.SplitAlerts {
		return h.sendSplit(ctx, payload)
	}

	// Transform the payload
	transformed, err := h.engine.Transform(payload)
	if err != nil {
		return fmt.Errorf("failed to transform payload: %w", err)
	}

	// Format the data
	req, err := formatter.FormatData(formatter.OutputFormat(h.config.Format), transformed)
	if err != nil {
		return fmt.Errorf("failed to format data: %w", err)
	}

	// Send the request
	resp, err := h.sendRequest(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check response
	if !WrapResponse(resp).IsSuccess() {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("destination returned error: %s (body: %s)", resp.Status, string(body))
	}

	h.logger.WithFields(logrus.Fields{
		"duration_ms": time.Since(startTime).Milliseconds(),
		"status_code": resp.StatusCode,
		"alerts_sent": len(payload.Alerts),
	}).Info("Successfully sent alerts to destination")

	return nil
}

// sendSplit sends alerts using the splitting logic
func (h *HTTPHandler) sendSplit(ctx context.Context, payload *alertmanager.WebhookPayload) error {
	if len(payload.Alerts) == 0 {
		return nil
	}

	// Create alert processor
	processor := NewHTTPAlertProcessor(h)

	// Process alerts using the splitter
	result := h.splitter.Split(ctx, payload, processor)

	// Check for errors
	if result.FailureCount > 0 {
		var errorMsg string
		if len(result.Errors) <= 3 {
			// Show all errors if there are few
			errorMsgs := make([]string, len(result.Errors))
			for i, err := range result.Errors {
				errorMsgs[i] = err.Error()
			}
			errorMsg = fmt.Sprintf("failed to process %d/%d alerts: %s",
				result.FailureCount, result.TotalAlerts, strings.Join(errorMsgs, "; "))
		} else {
			// Show summary if there are many errors
			errorMsg = fmt.Sprintf("failed to process %d/%d alerts (showing first 3): %s; %s; %s",
				result.FailureCount, result.TotalAlerts,
				result.Errors[0].Error(),
				result.Errors[1].Error(),
				result.Errors[2].Error())
		}

		// Return error only if all alerts failed
		if result.SuccessCount == 0 {
			return fmt.Errorf("%s", errorMsg)
		}

		// Log partial failures but don't return error
		h.logger.WithFields(logrus.Fields{
			"total_alerts":  result.TotalAlerts,
			"success_count": result.SuccessCount,
			"failure_count": result.FailureCount,
			"duration_ms":   result.Duration.Milliseconds(),
		}).Warn("Partial failure in alert splitting: " + errorMsg)
	}

	return nil
}

// sendRequest sends an HTTP request based on the destination configuration
func (h *HTTPHandler) sendRequest(ctx context.Context, req *formatter.Request) (*http.Response, error) {
	method := strings.ToUpper(h.config.Method)

	// Build URL with query parameters if needed
	targetURL := h.config.URL
	if len(req.QueryParams) > 0 {
		u, err := url.Parse(targetURL)
		if err != nil {
			return nil, fmt.Errorf("invalid destination URL: %w", err)
		}

		q := u.Query()
		for k, v := range req.QueryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		targetURL = u.String()
	}

	// Create HTTP request
	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, targetURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	for k, v := range req.Headers {
		httpReq.Header[k] = v
	}

	// Add custom headers from config
	for k, v := range h.config.Headers {
		httpReq.Header.Set(k, v)
	}

	// Execute request
	return h.client.Do(httpReq)
}

// Name returns the destination name
func (h *HTTPHandler) Name() string {
	return h.config.Name
}

// Close cleans up resources
func (h *HTTPHandler) Close() error {
	h.client.CloseIdleConnections()
	return nil
}
