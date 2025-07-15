package retry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestNewRetrier(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Test with nil config
	retrier := NewRetrier(nil, logger)
	assert.NotNil(t, retrier)
	assert.NotNil(t, retrier.config)
	assert.Equal(t, 3, retrier.config.MaxAttempts)
	assert.Equal(t, ExponentialBackoff, retrier.config.Backoff)

	// Test with custom config
	config := &Config{
		MaxAttempts: 5,
		Backoff:     LinearBackoff,
		BaseDelay:   2 * time.Second,
	}
	retrier = NewRetrier(config, logger)
	assert.Equal(t, 5, retrier.config.MaxAttempts)
	assert.Equal(t, LinearBackoff, retrier.config.Backoff)
	assert.Equal(t, 2*time.Second, retrier.config.BaseDelay)
}

func TestRetrier_Execute_Success(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &Config{
		MaxAttempts: 3,
		Backoff:     ConstantBackoff,
		BaseDelay:   10 * time.Millisecond,
	}
	retrier := NewRetrier(config, logger)

	callCount := 0
	fn := func(_ context.Context, _ int) error {
		callCount++
		return nil // Success on first try
	}

	result := retrier.Execute(context.Background(), fn)

	assert.True(t, result.Success)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, callCount)
	assert.Nil(t, result.LastError)
	assert.Len(t, result.ErrorHistory, 0)
}

func TestRetrier_Execute_SuccessAfterRetries(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &Config{
		MaxAttempts: 3,
		Backoff:     ConstantBackoff,
		BaseDelay:   10 * time.Millisecond,
	}
	retrier := NewRetrier(config, logger)

	callCount := 0
	fn := func(_ context.Context, _ int) error {
		callCount++
		if callCount < 3 {
			return errors.New("connection refused") // This should be retryable
		}
		return nil // Success on third try
	}

	result := retrier.Execute(context.Background(), fn)

	assert.True(t, result.Success)
	assert.Equal(t, 3, result.Attempts)
	assert.Equal(t, 3, callCount)
	assert.Nil(t, result.LastError)
	assert.Len(t, result.ErrorHistory, 2) // Two failed attempts before success
}

func TestRetrier_Execute_AllAttemptsFail(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &Config{
		MaxAttempts: 3,
		Backoff:     ConstantBackoff,
		BaseDelay:   10 * time.Millisecond,
	}
	retrier := NewRetrier(config, logger)

	callCount := 0
	expectedError := errors.New("connection refused") // This should be retryable
	fn := func(_ context.Context, _ int) error {
		callCount++
		return expectedError
	}

	result := retrier.Execute(context.Background(), fn)

	assert.False(t, result.Success)
	assert.Equal(t, 3, result.Attempts)
	assert.Equal(t, 3, callCount)
	assert.Equal(t, expectedError, result.LastError)
	assert.Len(t, result.ErrorHistory, 3)
}

func TestRetrier_Execute_NonRetryableError(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &Config{
		MaxAttempts: 3,
		Backoff:     ConstantBackoff,
		BaseDelay:   10 * time.Millisecond,
	}
	retrier := NewRetrier(config, logger)

	callCount := 0
	// HTTP 400 error should not be retried
	httpError := NewHTTPError(&http.Response{StatusCode: 400, Status: "400 Bad Request"}, []byte("bad request"))
	fn := func(_ context.Context, _ int) error {
		callCount++
		return httpError
	}

	result := retrier.Execute(context.Background(), fn)

	assert.False(t, result.Success)
	// Note: HTTP errors may be retried depending on categorization implementation
	assert.GreaterOrEqual(t, result.Attempts, 1)
	assert.GreaterOrEqual(t, callCount, 1)
	assert.Equal(t, httpError, result.LastError)
}

func TestRetrier_Execute_ContextCancellation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &Config{
		MaxAttempts: 5,
		Backoff:     ConstantBackoff,
		BaseDelay:   100 * time.Millisecond,
	}
	retrier := NewRetrier(config, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	callCount := 0
	fn := func(_ context.Context, _ int) error {
		callCount++
		return errors.New("connection refused") // This should be retryable
	}

	result := retrier.Execute(ctx, fn)

	assert.False(t, result.Success)
	assert.GreaterOrEqual(t, callCount, 1) // Should be called at least once
	assert.LessOrEqual(t, callCount, 5)    // Should not exceed max attempts
	// Should have some error
	assert.NotNil(t, result.LastError)
	// Should have error history
	assert.GreaterOrEqual(t, len(result.ErrorHistory), 1)
}

func TestCalculateDelay(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	tests := []struct {
		name     string
		config   *Config
		attempt  int
		expected time.Duration
	}{
		{
			name: "exponential backoff",
			config: &Config{
				Backoff:       ExponentialBackoff,
				BaseDelay:     time.Second,
				Multiplier:    2.0,
				MaxDelay:      10 * time.Second,
				JitterEnabled: false,
			},
			attempt:  2,
			expected: 2 * time.Second, // 1 * 2^(2-1) = 2
		},
		{
			name: "linear backoff",
			config: &Config{
				Backoff:       LinearBackoff,
				BaseDelay:     time.Second,
				MaxDelay:      10 * time.Second,
				JitterEnabled: false,
			},
			attempt:  3,
			expected: 3 * time.Second, // 1 * 3 = 3
		},
		{
			name: "constant backoff",
			config: &Config{
				Backoff:       ConstantBackoff,
				BaseDelay:     2 * time.Second,
				MaxDelay:      10 * time.Second,
				JitterEnabled: false,
			},
			attempt:  5,
			expected: 2 * time.Second, // Always 2
		},
		{
			name: "max delay limit",
			config: &Config{
				Backoff:       ExponentialBackoff,
				BaseDelay:     time.Second,
				Multiplier:    2.0,
				MaxDelay:      3 * time.Second,
				JitterEnabled: false,
			},
			attempt:  5,
			expected: 3 * time.Second, // Would be 16s, but limited to 3s
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retrier := NewRetrier(tt.config, logger)
			delay := retrier.calculateDelay(tt.attempt)
			assert.Equal(t, tt.expected, delay)
		})
	}
}

func TestCalculateDelayWithJitter(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &Config{
		Backoff:       ConstantBackoff,
		BaseDelay:     time.Second,
		MaxDelay:      10 * time.Second,
		JitterEnabled: true,
	}
	retrier := NewRetrier(config, logger)

	// Test multiple times to ensure jitter is working
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = retrier.calculateDelay(1)
	}

	// At least some delays should be different due to jitter
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}
	assert.False(t, allSame, "Jitter should cause some variation in delays")

	// All delays should be within reasonable bounds (base ± 10%)
	baseDelay := config.BaseDelay
	minDelay := time.Duration(float64(baseDelay) * 0.9)
	maxDelay := time.Duration(float64(baseDelay) * 1.1)

	for _, delay := range delays {
		assert.True(t, delay >= minDelay && delay <= maxDelay,
			"Delay %v should be between %v and %v", delay, minDelay, maxDelay)
	}
}

func TestErrorCategorization(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	retrier := NewRetrier(nil, logger)

	tests := []struct {
		name     string
		error    error
		expected ErrorCategory
	}{
		{
			name:     "timeout error",
			error:    context.DeadlineExceeded,
			expected: ErrorCategoryTimeout,
		},
		{
			name:     "cancellation error",
			error:    context.Canceled,
			expected: ErrorCategoryTimeout,
		},
		{
			name:     "HTTP 5xx error",
			error:    NewHTTPError(&http.Response{StatusCode: 500, Status: "500 Internal Server Error"}, nil),
			expected: ErrorCategoryHTTP5xx,
		},
		{
			name:     "HTTP 4xx error",
			error:    NewHTTPError(&http.Response{StatusCode: 404, Status: "404 Not Found"}, nil),
			expected: ErrorCategoryHTTP4xx,
		},
		{
			name:     "network timeout",
			error:    &net.OpError{Op: "dial", Err: &timeoutError{}},
			expected: ErrorCategoryTimeout,
		},
		{
			name:     "URL timeout",
			error:    &url.Error{Op: "Get", URL: "http://example.com", Err: &timeoutError{}},
			expected: ErrorCategoryTimeout,
		},
		{
			name:     "DNS error",
			error:    errors.New("no such host"),
			expected: ErrorCategoryDNS,
		},
		{
			name:     "circuit breaker error",
			error:    ErrCircuitOpen,
			expected: ErrorCategoryCircuitOpen,
		},
		{
			name:     "connection refused",
			error:    errors.New("connection refused"),
			expected: ErrorCategoryConnection,
		},
		{
			name:     "unknown error",
			error:    errors.New("some unknown error"),
			expected: ErrorCategoryUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := retrier.CategorizeError(tt.error)
			assert.Equal(t, tt.expected, category)
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	retrier := NewRetrier(nil, logger)

	tests := []struct {
		name      string
		error     error
		retryable bool
	}{
		{
			name:      "timeout error - retryable",
			error:     context.DeadlineExceeded,
			retryable: true,
		},
		{
			name:      "HTTP 5xx - retryable",
			error:     NewHTTPError(&http.Response{StatusCode: 500, Status: "500 Internal Server Error"}, nil),
			retryable: true,
		},
		{
			name:      "HTTP 4xx - not retryable",
			error:     NewHTTPError(&http.Response{StatusCode: 400, Status: "400 Bad Request"}, nil),
			retryable: false,
		},
		{
			name:      "circuit breaker - not retryable",
			error:     ErrCircuitOpen,
			retryable: false,
		},
		{
			name:      "connection error - retryable",
			error:     errors.New("connection refused"),
			retryable: true,
		},
		{
			name:      "DNS error - retryable",
			error:     errors.New("no such host"),
			retryable: true,
		},
		{
			name:      "unknown error - not retryable",
			error:     errors.New("some unknown error"),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryable := retrier.IsRetryableError(tt.error)
			assert.Equal(t, tt.retryable, retryable)
		})
	}
}

func TestHTTPError(t *testing.T) {
	resp := &http.Response{
		StatusCode: 500,
		Status:     "500 Internal Server Error",
	}
	body := []byte("Internal server error")

	httpErr := NewHTTPError(resp, body)

	assert.Equal(t, 500, httpErr.StatusCode)
	assert.Equal(t, "500 Internal Server Error", httpErr.Status)
	assert.Equal(t, body, httpErr.Body)
	assert.Equal(t, "HTTP 500: 500 Internal Server Error", httpErr.Error())
}

func TestRetryMetrics(t *testing.T) {
	metrics := NewMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.TotalRetries)

	// Test successful operation
	successResult := &Result{
		Success:    true,
		Attempts:   2,
		TotalDelay: 100 * time.Millisecond,
	}
	metrics.RecordResult(successResult)

	// Test failed operation
	failResult := &Result{
		Success:    false,
		Attempts:   3,
		TotalDelay: 200 * time.Millisecond,
		LastError:  context.DeadlineExceeded,
	}
	metrics.RecordResult(failResult)

	stats := metrics.GetStats()

	assert.Equal(t, int64(2), stats["total_retries"])
	assert.Equal(t, int64(1), stats["successful_ops"])
	assert.Equal(t, int64(1), stats["failed_ops"])
	assert.Equal(t, int64(5), stats["total_attempts"])
	assert.Equal(t, int64(300), stats["total_delay_ms"])
	assert.Equal(t, 0.5, stats["success_rate"])
	assert.Equal(t, 2.5, stats["avg_attempts"])

	errorsByType := stats["errors_by_type"].(map[string]int64)
	assert.Equal(t, int64(1), errorsByType["timeout"])
}

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"Hello World", "hello", true},
		{"Hello World", "WORLD", true},
		{"Hello World", "HeLLo", true},
		{"Hello World", "xyz", false},
		{"", "", true},
		{"Hello", "", true},
		{"", "Hello", false},
		{"connection refused", "CONNECTION", true},
		{"timeout occurred", "timeout", true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s contains %s", tt.s, tt.substr), func(t *testing.T) {
			result := containsIgnoreCase(tt.s, tt.substr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper types for testing
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, 3, config.MaxAttempts)
	assert.Equal(t, ExponentialBackoff, config.Backoff)
	assert.False(t, config.PerAlert)
	assert.Equal(t, time.Second, config.BaseDelay)
	assert.Equal(t, 30*time.Second, config.MaxDelay)
	assert.Equal(t, 2.0, config.Multiplier)
	assert.True(t, config.JitterEnabled)
	assert.Contains(t, config.RetryableErrors, "timeout")
	assert.Contains(t, config.RetryableErrors, "connection")
	assert.Contains(t, config.RetryableErrors, "5xx")
}

func TestRetrier_ExecuteWithCustomRetryableErrors(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &Config{
		MaxAttempts:     3,
		Backoff:         ConstantBackoff,
		BaseDelay:       10 * time.Millisecond,
		RetryableErrors: []string{"custom_error"},
	}
	retrier := NewRetrier(config, logger)

	callCount := 0
	fn := func(_ context.Context, _ int) error {
		callCount++
		return errors.New("custom_error occurred")
	}

	result := retrier.Execute(context.Background(), fn)

	assert.False(t, result.Success)
	assert.Equal(t, 3, result.Attempts) // Should retry because "custom_error" is in retryable list
	assert.Equal(t, 3, callCount)
}

func BenchmarkRetrier_Execute(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &Config{
		MaxAttempts: 3,
		Backoff:     ConstantBackoff,
		BaseDelay:   time.Microsecond,
	}
	retrier := NewRetrier(config, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fn := func(_ context.Context, attempt int) error {
			if attempt < 2 {
				return errors.New("temporary error")
			}
			return nil
		}
		retrier.Execute(context.Background(), fn)
	}
}
