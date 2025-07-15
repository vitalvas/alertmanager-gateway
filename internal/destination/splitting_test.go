package destination

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
)

// MockAlertProcessor implements AlertProcessor for testing
type MockAlertProcessor struct {
	processAlertCalls []ProcessAlertCall
	processBatchCalls []ProcessBatchCall
	errors            []error
	delay             time.Duration
	mu                sync.Mutex
}

type ProcessAlertCall struct {
	Alert   *alertmanager.Alert
	Payload *alertmanager.WebhookPayload
}

type ProcessBatchCall struct {
	Alerts  []alertmanager.Alert
	Payload *alertmanager.WebhookPayload
}

func NewMockAlertProcessor() *MockAlertProcessor {
	return &MockAlertProcessor{
		processAlertCalls: make([]ProcessAlertCall, 0),
		processBatchCalls: make([]ProcessBatchCall, 0),
		errors:            make([]error, 0),
	}
}

func (m *MockAlertProcessor) ProcessAlert(_ context.Context, alert *alertmanager.Alert, payload *alertmanager.WebhookPayload) error {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.processAlertCalls = append(m.processAlertCalls, ProcessAlertCall{
		Alert:   alert,
		Payload: payload,
	})

	if len(m.errors) > 0 {
		err := m.errors[0]
		m.errors = m.errors[1:]
		return err
	}

	return nil
}

func (m *MockAlertProcessor) ProcessBatch(_ context.Context, alerts []alertmanager.Alert, payload *alertmanager.WebhookPayload) error {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.processBatchCalls = append(m.processBatchCalls, ProcessBatchCall{
		Alerts:  alerts,
		Payload: payload,
	})

	if len(m.errors) > 0 {
		err := m.errors[0]
		m.errors = m.errors[1:]
		return err
	}

	return nil
}

func (m *MockAlertProcessor) SetErrors(errors []error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = errors
}

func (m *MockAlertProcessor) SetDelay(delay time.Duration) {
	m.delay = delay
}

func (m *MockAlertProcessor) GetProcessAlertCalls() []ProcessAlertCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ProcessAlertCall{}, m.processAlertCalls...)
}

func (m *MockAlertProcessor) GetProcessBatchCalls() []ProcessBatchCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ProcessBatchCall{}, m.processBatchCalls...)
}

func TestNewAlertSplitter(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())

	tests := []struct {
		name             string
		config           *config.DestinationConfig
		expectedStrategy SplitStrategy
	}{
		{
			name: "sequential by default",
			config: &config.DestinationConfig{
				BatchSize:        0,
				ParallelRequests: 0,
			},
			expectedStrategy: SplitStrategySequential,
		},
		{
			name: "parallel when parallel_requests > 1",
			config: &config.DestinationConfig{
				BatchSize:        0,
				ParallelRequests: 3,
			},
			expectedStrategy: SplitStrategyParallel,
		},
		{
			name: "batch when batch_size > 1",
			config: &config.DestinationConfig{
				BatchSize:        5,
				ParallelRequests: 0,
			},
			expectedStrategy: SplitStrategyBatch,
		},
		{
			name: "batch parallel when both > 1",
			config: &config.DestinationConfig{
				BatchSize:        3,
				ParallelRequests: 2,
			},
			expectedStrategy: SplitStrategyBatchParallel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			splitter := NewAlertSplitter(tt.config, logger)
			require.NotNil(t, splitter)

			info := splitter.GetStrategyInfo()
			assert.Equal(t, tt.expectedStrategy, splitter.config.Strategy)
			assert.NotNil(t, info["strategy"])
		})
	}
}

func TestAlertSplitter_Sequential(t *testing.T) {
	config := &config.DestinationConfig{
		BatchSize:        1,
		ParallelRequests: 1,
	}
	logger := logrus.NewEntry(logrus.New())
	splitter := NewAlertSplitter(config, logger)

	payload := createTestPayload(3)
	processor := NewMockAlertProcessor()

	result := splitter.Split(context.Background(), payload, processor)

	assert.Equal(t, 3, result.TotalAlerts)
	assert.Equal(t, 3, result.SuccessCount)
	assert.Equal(t, 0, result.FailureCount)
	assert.Empty(t, result.Errors)
	assert.Len(t, result.ProcessedData, 3)

	// Check processor calls
	calls := processor.GetProcessAlertCalls()
	assert.Len(t, calls, 3)

	for i := 0; i < 3; i++ {
		assert.Equal(t, fmt.Sprintf("alert-%d", i), calls[i].Alert.Labels["name"])
		assert.True(t, result.ProcessedData[i].Success)
	}
}

func TestAlertSplitter_Parallel(t *testing.T) {
	config := &config.DestinationConfig{
		BatchSize:        1,
		ParallelRequests: 3,
	}
	logger := logrus.NewEntry(logrus.New())
	splitter := NewAlertSplitter(config, logger)

	payload := createTestPayload(5)
	processor := NewMockAlertProcessor()
	processor.SetDelay(10 * time.Millisecond) // Small delay to test concurrency

	startTime := time.Now()
	result := splitter.Split(context.Background(), payload, processor)
	duration := time.Since(startTime)

	assert.Equal(t, 5, result.TotalAlerts)
	assert.Equal(t, 5, result.SuccessCount)
	assert.Equal(t, 0, result.FailureCount)

	// With 3 parallel requests and 5 alerts, it should take roughly 2 "rounds"
	// Each round is 10ms, so total should be around 20ms (plus overhead)
	assert.Less(t, duration, 50*time.Millisecond, "Parallel processing should be faster")

	calls := processor.GetProcessAlertCalls()
	assert.Len(t, calls, 5)
}

func TestAlertSplitter_Batch(t *testing.T) {
	config := &config.DestinationConfig{
		BatchSize:        2,
		ParallelRequests: 1,
	}
	logger := logrus.NewEntry(logrus.New())
	splitter := NewAlertSplitter(config, logger)

	payload := createTestPayload(5)
	processor := NewMockAlertProcessor()

	result := splitter.Split(context.Background(), payload, processor)

	assert.Equal(t, 5, result.TotalAlerts)
	assert.Equal(t, 5, result.SuccessCount)
	assert.Equal(t, 0, result.FailureCount)

	// Check batch calls: 5 alerts with batch size 2 = 3 batches (2, 2, 1)
	batchCalls := processor.GetProcessBatchCalls()
	assert.Len(t, batchCalls, 3)

	assert.Len(t, batchCalls[0].Alerts, 2)
	assert.Len(t, batchCalls[1].Alerts, 2)
	assert.Len(t, batchCalls[2].Alerts, 1)

	// No individual alert calls should be made
	alertCalls := processor.GetProcessAlertCalls()
	assert.Len(t, alertCalls, 0)
}

func TestAlertSplitter_BatchParallel(t *testing.T) {
	config := &config.DestinationConfig{
		BatchSize:        2,
		ParallelRequests: 2,
	}
	logger := logrus.NewEntry(logrus.New())
	splitter := NewAlertSplitter(config, logger)

	payload := createTestPayload(6)
	processor := NewMockAlertProcessor()
	processor.SetDelay(10 * time.Millisecond)

	startTime := time.Now()
	result := splitter.Split(context.Background(), payload, processor)
	duration := time.Since(startTime)

	assert.Equal(t, 6, result.TotalAlerts)
	assert.Equal(t, 6, result.SuccessCount)
	assert.Equal(t, 0, result.FailureCount)

	// 6 alerts with batch size 2 = 3 batches
	// With 2 parallel requests, should take ~2 rounds = ~20ms
	assert.Less(t, duration, 50*time.Millisecond, "Batch parallel processing should be faster")

	batchCalls := processor.GetProcessBatchCalls()
	assert.Len(t, batchCalls, 3)
}

func TestAlertSplitter_WithErrors(t *testing.T) {
	config := &config.DestinationConfig{
		BatchSize:        1,
		ParallelRequests: 1,
	}
	logger := logrus.NewEntry(logrus.New())
	splitter := NewAlertSplitter(config, logger)

	payload := createTestPayload(3)
	processor := NewMockAlertProcessor()

	// Set errors for the first and third alerts
	processor.SetErrors([]error{
		fmt.Errorf("first alert error"),
		nil, // second alert succeeds
		fmt.Errorf("third alert error"),
	})

	result := splitter.Split(context.Background(), payload, processor)

	assert.Equal(t, 3, result.TotalAlerts)
	assert.Equal(t, 1, result.SuccessCount)
	assert.Equal(t, 2, result.FailureCount)
	assert.Len(t, result.Errors, 2)

	// Check specific results
	assert.False(t, result.ProcessedData[0].Success)
	assert.True(t, result.ProcessedData[1].Success)
	assert.False(t, result.ProcessedData[2].Success)

	assert.Contains(t, result.Errors[0].Error(), "first alert error")
	assert.Contains(t, result.Errors[1].Error(), "third alert error")
}

func TestAlertSplitter_EmptyPayload(t *testing.T) {
	config := &config.DestinationConfig{
		BatchSize:        2,
		ParallelRequests: 3,
	}
	logger := logrus.NewEntry(logrus.New())
	splitter := NewAlertSplitter(config, logger)

	payload := &alertmanager.WebhookPayload{
		Alerts: []alertmanager.Alert{},
	}
	processor := NewMockAlertProcessor()

	result := splitter.Split(context.Background(), payload, processor)

	assert.Equal(t, 0, result.TotalAlerts)
	assert.Equal(t, 0, result.SuccessCount)
	assert.Equal(t, 0, result.FailureCount)
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.ProcessedData)
}

func TestAlertSplitter_MaxConcurrency(t *testing.T) {
	config := &config.DestinationConfig{
		BatchSize:        1,
		ParallelRequests: 100, // Very high number
	}
	logger := logrus.NewEntry(logrus.New())
	splitter := NewAlertSplitter(config, logger)

	// Should be limited to max concurrency
	assert.Equal(t, 10, splitter.config.ParallelRequests)
	assert.Equal(t, 10, splitter.config.MaxConcurrency)
}

func TestAlertSplitter_StrategyNames(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())

	tests := []struct {
		batchSize        int
		parallelRequests int
		expectedName     string
	}{
		{1, 1, "sequential"},
		{1, 3, "parallel"},
		{3, 1, "batch"},
		{3, 3, "batch-parallel"},
	}

	for _, tt := range tests {
		t.Run(tt.expectedName, func(t *testing.T) {
			config := &config.DestinationConfig{
				BatchSize:        tt.batchSize,
				ParallelRequests: tt.parallelRequests,
			}
			splitter := NewAlertSplitter(config, logger)

			assert.Equal(t, tt.expectedName, splitter.getStrategyName())

			info := splitter.GetStrategyInfo()
			assert.Equal(t, tt.expectedName, info["strategy"])
		})
	}
}

func TestAlertSplitter_BatchErrorHandling(t *testing.T) {
	config := &config.DestinationConfig{
		BatchSize:        2,
		ParallelRequests: 1,
	}
	logger := logrus.NewEntry(logrus.New())
	splitter := NewAlertSplitter(config, logger)

	payload := createTestPayload(4)
	processor := NewMockAlertProcessor()

	// Set error for the second batch
	processor.SetErrors([]error{
		nil, // first batch succeeds
		fmt.Errorf("second batch error"),
	})

	result := splitter.Split(context.Background(), payload, processor)

	assert.Equal(t, 4, result.TotalAlerts)
	assert.Equal(t, 2, result.SuccessCount) // First batch (2 alerts)
	assert.Equal(t, 2, result.FailureCount) // Second batch (2 alerts)
	assert.Len(t, result.Errors, 1)

	// Check that first batch alerts are successful, second batch alerts failed
	assert.True(t, result.ProcessedData[0].Success)
	assert.True(t, result.ProcessedData[1].Success)
	assert.False(t, result.ProcessedData[2].Success)
	assert.False(t, result.ProcessedData[3].Success)
}

func TestAlertSplitter_ConcurrentSafety(t *testing.T) {
	config := &config.DestinationConfig{
		BatchSize:        1,
		ParallelRequests: 5,
	}
	logger := logrus.NewEntry(logrus.New())
	splitter := NewAlertSplitter(config, logger)

	payload := createTestPayload(10)
	processor := NewMockAlertProcessor()

	// Run multiple splitters concurrently
	var wg sync.WaitGroup
	results := make([]*SplitResult, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = splitter.Split(context.Background(), payload, processor)
		}(i)
	}

	wg.Wait()

	// All should succeed
	for i, result := range results {
		assert.Equal(t, 10, result.TotalAlerts, "Result %d should have processed all alerts", i)
		assert.Equal(t, 10, result.SuccessCount, "Result %d should have all successes", i)
		assert.Equal(t, 0, result.FailureCount, "Result %d should have no failures", i)
	}
}

func createTestPayload(alertCount int) *alertmanager.WebhookPayload {
	alerts := make([]alertmanager.Alert, alertCount)
	for i := 0; i < alertCount; i++ {
		alerts[i] = alertmanager.Alert{
			Status:      "firing",
			Fingerprint: fmt.Sprintf("fp-%d", i),
			Labels: map[string]string{
				"name":     fmt.Sprintf("alert-%d", i),
				"severity": "warning",
			},
			Annotations: map[string]string{
				"summary": fmt.Sprintf("Test alert %d", i),
			},
			StartsAt: time.Now(),
		}
	}

	return &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "test-group",
		Status:   "firing",
		Receiver: "test-receiver",
		GroupLabels: map[string]string{
			"alertname": "TestAlert",
		},
		CommonLabels: map[string]string{
			"alertname": "TestAlert",
			"severity":  "warning",
		},
		CommonAnnotations: map[string]string{
			"summary": "Test alerts",
		},
		ExternalURL: "http://alertmanager.test",
		Alerts:      alerts,
	}
}
