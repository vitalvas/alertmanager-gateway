package destination

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/formatter"
	"github.com/vitalvas/alertmanager-gateway/internal/transform"
)

// SplitStrategy defines how alerts should be split and processed
type SplitStrategy int

const (
	// SplitStrategySequential processes alerts one by one
	SplitStrategySequential SplitStrategy = iota
	// SplitStrategyParallel processes alerts concurrently
	SplitStrategyParallel
	// SplitStrategyBatch processes alerts in batches
	SplitStrategyBatch
	// SplitStrategyBatchParallel processes batches in parallel
	SplitStrategyBatchParallel
)

// SplittingConfig contains configuration for alert splitting
type SplittingConfig struct {
	Strategy         SplitStrategy
	BatchSize        int
	ParallelRequests int
	MaxConcurrency   int
}

// AlertSplitter handles the splitting and processing of alerts
type AlertSplitter struct {
	config *SplittingConfig
	logger *logrus.Entry
}

// NewAlertSplitter creates a new alert splitter
func NewAlertSplitter(destConfig *config.DestinationConfig, logger *logrus.Entry) *AlertSplitter {
	cfg := &SplittingConfig{
		Strategy:         SplitStrategySequential,
		BatchSize:        destConfig.BatchSize,
		ParallelRequests: destConfig.ParallelRequests,
		MaxConcurrency:   10, // Default max concurrency
	}

	// Determine strategy based on configuration
	switch {
	case cfg.ParallelRequests > 1 && cfg.BatchSize > 1:
		cfg.Strategy = SplitStrategyBatchParallel
	case cfg.ParallelRequests > 1:
		cfg.Strategy = SplitStrategyParallel
	case cfg.BatchSize > 1:
		cfg.Strategy = SplitStrategyBatch
	}

	// Apply defaults
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 1
	}
	if cfg.ParallelRequests <= 0 {
		cfg.ParallelRequests = 1
	}

	// Limit max concurrency
	if cfg.ParallelRequests > cfg.MaxConcurrency {
		cfg.ParallelRequests = cfg.MaxConcurrency
	}

	return &AlertSplitter{
		config: cfg,
		logger: logger,
	}
}

// SplitResult contains the results of alert splitting
type SplitResult struct {
	TotalAlerts   int
	SuccessCount  int
	FailureCount  int
	Errors        []error
	Duration      time.Duration
	ProcessedData []ProcessedAlert
}

// ProcessedAlert contains information about a processed alert
type ProcessedAlert struct {
	Index    int
	Alert    *alertmanager.Alert
	Success  bool
	Error    error
	Duration time.Duration
}

// AlertProcessor defines the interface for processing individual alerts or batches
type AlertProcessor interface {
	ProcessAlert(ctx context.Context, alert *alertmanager.Alert, payload *alertmanager.WebhookPayload) error
	ProcessBatch(ctx context.Context, alerts []alertmanager.Alert, payload *alertmanager.WebhookPayload) error
}

// HTTPAlertProcessor implements AlertProcessor for HTTP destinations
type HTTPAlertProcessor struct {
	handler *HTTPHandler
	engine  transform.Engine
	config  *config.DestinationConfig
}

// NewHTTPAlertProcessor creates a new HTTP alert processor
func NewHTTPAlertProcessor(handler *HTTPHandler) *HTTPAlertProcessor {
	return &HTTPAlertProcessor{
		handler: handler,
		engine:  handler.engine,
		config:  handler.config,
	}
}

// ProcessAlert processes a single alert
func (p *HTTPAlertProcessor) ProcessAlert(ctx context.Context, alert *alertmanager.Alert, payload *alertmanager.WebhookPayload) error {
	// All engines now support alert-specific transformation
	transformed, err := p.engine.TransformAlert(alert, payload)
	if err != nil {
		return fmt.Errorf("failed to transform alert: %w", err)
	}

	return p.sendTransformed(ctx, transformed)
}

// ProcessBatch processes a batch of alerts
func (p *HTTPAlertProcessor) ProcessBatch(ctx context.Context, alerts []alertmanager.Alert, payload *alertmanager.WebhookPayload) error {
	// Create batch payload
	batchPayload := &alertmanager.WebhookPayload{
		Version:           payload.Version,
		GroupKey:          payload.GroupKey,
		TruncatedAlerts:   payload.TruncatedAlerts,
		Status:            payload.Status,
		Receiver:          payload.Receiver,
		GroupLabels:       payload.GroupLabels,
		CommonLabels:      payload.CommonLabels,
		CommonAnnotations: payload.CommonAnnotations,
		ExternalURL:       payload.ExternalURL,
		Alerts:            alerts,
	}

	transformed, err := p.engine.Transform(batchPayload)
	if err != nil {
		return fmt.Errorf("failed to transform batch payload: %w", err)
	}

	return p.sendTransformed(ctx, transformed)
}

// sendTransformed sends the transformed data
func (p *HTTPAlertProcessor) sendTransformed(ctx context.Context, transformed interface{}) error {
	// Format the data
	req, err := formatter.FormatData(formatter.OutputFormat(p.config.Format), transformed)
	if err != nil {
		return fmt.Errorf("failed to format data: %w", err)
	}

	// Send the request
	resp, err := p.handler.sendRequest(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if !WrapResponse(resp).IsSuccess() {
		return fmt.Errorf("destination returned error: %s", resp.Status)
	}

	return nil
}

// Split processes alerts according to the configured strategy
func (s *AlertSplitter) Split(ctx context.Context, payload *alertmanager.WebhookPayload, processor AlertProcessor) *SplitResult {
	startTime := time.Now()
	result := &SplitResult{
		TotalAlerts:   len(payload.Alerts),
		ProcessedData: make([]ProcessedAlert, 0, len(payload.Alerts)),
	}

	if len(payload.Alerts) == 0 {
		result.Duration = time.Since(startTime)
		return result
	}

	switch s.config.Strategy {
	case SplitStrategySequential:
		s.processSequential(ctx, payload, processor, result)
	case SplitStrategyParallel:
		s.processParallel(ctx, payload, processor, result)
	case SplitStrategyBatch:
		s.processBatch(ctx, payload, processor, result)
	case SplitStrategyBatchParallel:
		s.processBatchParallel(ctx, payload, processor, result)
	}

	result.Duration = time.Since(startTime)

	// Count actual successes and failures from processed data
	successCount := 0
	failureCount := 0
	for _, processed := range result.ProcessedData {
		if processed.Success {
			successCount++
		} else {
			failureCount++
		}
	}
	result.SuccessCount = successCount
	result.FailureCount = failureCount

	s.logger.WithFields(logrus.Fields{
		"strategy":      s.getStrategyName(),
		"total_alerts":  result.TotalAlerts,
		"success_count": result.SuccessCount,
		"failure_count": result.FailureCount,
		"duration_ms":   result.Duration.Milliseconds(),
		"batch_size":    s.config.BatchSize,
		"parallel":      s.config.ParallelRequests,
	}).Info("Completed alert splitting")

	return result
}

// processSequential processes alerts one by one
func (s *AlertSplitter) processSequential(ctx context.Context, payload *alertmanager.WebhookPayload, processor AlertProcessor, result *SplitResult) {
	for i, alert := range payload.Alerts {
		alertStartTime := time.Now()
		processed := ProcessedAlert{
			Index: i,
			Alert: &alert,
		}

		err := processor.ProcessAlert(ctx, &alert, payload)
		processed.Duration = time.Since(alertStartTime)
		processed.Success = err == nil
		processed.Error = err

		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("alert %d: %w", i, err))
		}

		result.ProcessedData = append(result.ProcessedData, processed)
	}
}

// processParallel processes alerts concurrently
func (s *AlertSplitter) processParallel(ctx context.Context, payload *alertmanager.WebhookPayload, processor AlertProcessor, result *SplitResult) {
	semaphore := make(chan struct{}, s.config.ParallelRequests)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, alert := range payload.Alerts {
		wg.Add(1)
		go func(index int, a alertmanager.Alert) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			alertStartTime := time.Now()
			processed := ProcessedAlert{
				Index: index,
				Alert: &a,
			}

			err := processor.ProcessAlert(ctx, &a, payload)
			processed.Duration = time.Since(alertStartTime)
			processed.Success = err == nil
			processed.Error = err

			mu.Lock()
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("alert %d: %w", index, err))
			}
			result.ProcessedData = append(result.ProcessedData, processed)
			mu.Unlock()
		}(i, alert)
	}

	wg.Wait()
}

// processBatch processes alerts in batches sequentially
func (s *AlertSplitter) processBatch(ctx context.Context, payload *alertmanager.WebhookPayload, processor AlertProcessor, result *SplitResult) {
	alerts := payload.Alerts
	for i := 0; i < len(alerts); i += s.config.BatchSize {
		end := i + s.config.BatchSize
		if end > len(alerts) {
			end = len(alerts)
		}

		batch := alerts[i:end]
		batchStartTime := time.Now()

		err := processor.ProcessBatch(ctx, batch, payload)
		batchDuration := time.Since(batchStartTime)

		// Record each alert in the batch
		for j, alert := range batch {
			processed := ProcessedAlert{
				Index:    i + j,
				Alert:    &alert,
				Duration: batchDuration / time.Duration(len(batch)), // Approximate per-alert duration
				Success:  err == nil,
				Error:    err,
			}
			result.ProcessedData = append(result.ProcessedData, processed)
		}

		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("batch %d-%d: %w", i, end-1, err))
		}
	}
}

// processBatchParallel processes batches in parallel
func (s *AlertSplitter) processBatchParallel(ctx context.Context, payload *alertmanager.WebhookPayload, processor AlertProcessor, result *SplitResult) {
	alerts := payload.Alerts
	semaphore := make(chan struct{}, s.config.ParallelRequests)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := 0; i < len(alerts); i += s.config.BatchSize {
		end := i + s.config.BatchSize
		if end > len(alerts) {
			end = len(alerts)
		}

		batch := alerts[i:end]
		wg.Add(1)

		go func(startIdx int, batchAlerts []alertmanager.Alert) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			batchStartTime := time.Now()
			err := processor.ProcessBatch(ctx, batchAlerts, payload)
			batchDuration := time.Since(batchStartTime)

			mu.Lock()
			// Record each alert in the batch
			for j, alert := range batchAlerts {
				processed := ProcessedAlert{
					Index:    startIdx + j,
					Alert:    &alert,
					Duration: batchDuration / time.Duration(len(batchAlerts)),
					Success:  err == nil,
					Error:    err,
				}
				result.ProcessedData = append(result.ProcessedData, processed)
			}

			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("batch %d-%d: %w", startIdx, startIdx+len(batchAlerts)-1, err))
			}
			mu.Unlock()
		}(i, batch)
	}

	wg.Wait()
}

// getStrategyName returns a human-readable strategy name
func (s *AlertSplitter) getStrategyName() string {
	switch s.config.Strategy {
	case SplitStrategySequential:
		return "sequential"
	case SplitStrategyParallel:
		return "parallel"
	case SplitStrategyBatch:
		return "batch"
	case SplitStrategyBatchParallel:
		return "batch-parallel"
	default:
		return "unknown"
	}
}

// GetStrategyInfo returns information about the current strategy
func (s *AlertSplitter) GetStrategyInfo() map[string]interface{} {
	return map[string]interface{}{
		"strategy":          s.getStrategyName(),
		"batch_size":        s.config.BatchSize,
		"parallel_requests": s.config.ParallelRequests,
		"max_concurrency":   s.config.MaxConcurrency,
	}
}
