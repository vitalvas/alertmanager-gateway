package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// BackoffStrategy defines the type of backoff strategy
type BackoffStrategy string

const (
	ExponentialBackoff BackoffStrategy = "exponential"
	LinearBackoff      BackoffStrategy = "linear"
	ConstantBackoff    BackoffStrategy = "constant"
)

// Config holds retry configuration
type Config struct {
	MaxAttempts     int             `yaml:"max_attempts"`
	Backoff         BackoffStrategy `yaml:"backoff"`
	PerAlert        bool            `yaml:"per_alert"`
	BaseDelay       time.Duration   `yaml:"base_delay"`
	MaxDelay        time.Duration   `yaml:"max_delay"`
	Multiplier      float64         `yaml:"multiplier"`
	JitterEnabled   bool            `yaml:"jitter_enabled"`
	RetryableErrors []string        `yaml:"retryable_errors"`
}

// DefaultConfig returns default retry configuration
func DefaultConfig() *Config {
	return &Config{
		MaxAttempts:     3,
		Backoff:         ExponentialBackoff,
		PerAlert:        false,
		BaseDelay:       time.Second,
		MaxDelay:        30 * time.Second,
		Multiplier:      2.0,
		JitterEnabled:   true,
		RetryableErrors: []string{"timeout", "connection", "5xx"},
	}
}

// ErrorCategory represents categories of errors for retry decisions
type ErrorCategory int

const (
	ErrorCategoryUnknown ErrorCategory = iota
	ErrorCategoryTimeout
	ErrorCategoryConnection
	ErrorCategoryHTTP5xx
	ErrorCategoryHTTP4xx
	ErrorCategoryDNS
	ErrorCategoryCircuitOpen
)

// RetryableFunc is a function that can be retried
type RetryableFunc func(ctx context.Context, attempt int) error

// Result holds the result of a retry operation
type Result struct {
	Success      bool
	Attempts     int
	TotalDelay   time.Duration
	LastError    error
	ErrorHistory []error
}

// Retrier implements retry logic with configurable backoff strategies
type Retrier struct {
	config *Config
	logger *logrus.Entry
	rand   *rand.Rand
	mu     sync.Mutex
}

// NewRetrier creates a new retrier with the given configuration
func NewRetrier(config *Config, logger *logrus.Logger) *Retrier {
	if config == nil {
		config = DefaultConfig()
	}

	return &Retrier{
		config: config,
		logger: logger.WithField("component", "retrier"),
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Execute executes a function with retry logic
func (r *Retrier) Execute(ctx context.Context, fn RetryableFunc) *Result {
	result := &Result{
		ErrorHistory: make([]error, 0, r.config.MaxAttempts),
	}

	start := time.Now()

	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		r.logger.WithFields(logrus.Fields{
			"attempt":      attempt,
			"max_attempts": r.config.MaxAttempts,
		}).Debug("Executing retry attempt")

		err := fn(ctx, attempt)
		if err == nil {
			result.Success = true
			result.Attempts = attempt
			result.TotalDelay = time.Since(start)
			result.LastError = nil // Clear error on success

			r.logger.WithFields(logrus.Fields{
				"attempts":    attempt,
				"total_delay": result.TotalDelay,
			}).Debug("Retry succeeded")

			return result
		}

		result.LastError = err
		result.ErrorHistory = append(result.ErrorHistory, err)

		// Check if error is retryable
		if !r.isRetryableError(err) {
			r.logger.WithFields(logrus.Fields{
				"attempt": attempt,
				"error":   err.Error(),
			}).Debug("Error is not retryable, giving up")
			break
		}

		// Don't sleep after the last attempt
		if attempt < r.config.MaxAttempts {
			delay := r.calculateDelay(attempt)

			r.logger.WithFields(logrus.Fields{
				"attempt": attempt,
				"delay":   delay,
				"error":   err.Error(),
			}).Debug("Retry attempt failed, waiting before next attempt")

			select {
			case <-ctx.Done():
				result.LastError = ctx.Err()
				result.ErrorHistory = append(result.ErrorHistory, ctx.Err())
				break
			case <-time.After(delay):
				// Continue to next attempt
			}
		}
	}

	result.Attempts = r.config.MaxAttempts
	result.TotalDelay = time.Since(start)

	r.logger.WithFields(logrus.Fields{
		"attempts":    result.Attempts,
		"total_delay": result.TotalDelay,
		"last_error":  result.LastError.Error(),
	}).Warn("All retry attempts failed")

	return result
}

// calculateDelay calculates the delay for the given attempt number
func (r *Retrier) calculateDelay(attempt int) time.Duration {
	var delay time.Duration

	switch r.config.Backoff {
	case ExponentialBackoff:
		delay = time.Duration(float64(r.config.BaseDelay) * math.Pow(r.config.Multiplier, float64(attempt-1)))
	case LinearBackoff:
		delay = time.Duration(int64(r.config.BaseDelay) * int64(attempt))
	case ConstantBackoff:
		delay = r.config.BaseDelay
	default:
		delay = r.config.BaseDelay
	}

	// Apply maximum delay limit
	if delay > r.config.MaxDelay {
		delay = r.config.MaxDelay
	}

	// Apply jitter if enabled
	if r.config.JitterEnabled {
		r.mu.Lock()
		jitter := time.Duration(r.rand.Float64() * float64(delay) * 0.1) // ±10% jitter
		r.mu.Unlock()

		if r.rand.Float64() < 0.5 {
			delay -= jitter
		} else {
			delay += jitter
		}

		// Ensure delay is not negative
		if delay < 0 {
			delay = r.config.BaseDelay
		}
	}

	return delay
}

// IsRetryableError determines if an error should trigger a retry (exported for testing)
func (r *Retrier) IsRetryableError(err error) bool {
	return r.isRetryableError(err)
}

// isRetryableError determines if an error should trigger a retry
func (r *Retrier) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	category := r.categorizeError(err)

	switch category {
	case ErrorCategoryTimeout, ErrorCategoryConnection, ErrorCategoryHTTP5xx, ErrorCategoryDNS:
		return true
	case ErrorCategoryHTTP4xx, ErrorCategoryCircuitOpen:
		return false
	default:
		// For unknown errors, check against configured retryable patterns
		errStr := err.Error()
		for _, pattern := range r.config.RetryableErrors {
			if containsIgnoreCase(errStr, pattern) {
				return true
			}
		}
		return false
	}
}

// CategorizeError categorizes an error for retry decision making (exported for testing)
func (r *Retrier) CategorizeError(err error) ErrorCategory {
	return r.categorizeError(err)
}

// categorizeError categorizes an error for retry decision making
func (r *Retrier) categorizeError(err error) ErrorCategory {
	if err == nil {
		return ErrorCategoryUnknown
	}

	// Check for context errors
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrorCategoryTimeout
	}

	// Check for HTTP errors
	if httpErr, ok := err.(*HTTPError); ok {
		if httpErr.StatusCode >= 500 && httpErr.StatusCode < 600 {
			return ErrorCategoryHTTP5xx
		}
		if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
			return ErrorCategoryHTTP4xx
		}
	}

	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrorCategoryTimeout
		}
		return ErrorCategoryConnection
	}

	// Check for URL errors (often DNS-related)
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return ErrorCategoryTimeout
		}
		// Check if underlying error indicates DNS issues
		if containsIgnoreCase(urlErr.Err.Error(), "no such host") {
			return ErrorCategoryDNS
		}
		return ErrorCategoryConnection
	}

	// Check for circuit breaker errors
	if errors.Is(err, ErrCircuitOpen) {
		return ErrorCategoryCircuitOpen
	}

	// Check common error patterns
	errStr := err.Error()
	switch {
	case containsIgnoreCase(errStr, "timeout"):
		return ErrorCategoryTimeout
	case containsIgnoreCase(errStr, "connection refused"),
		containsIgnoreCase(errStr, "connection reset"),
		containsIgnoreCase(errStr, "broken pipe"),
		containsIgnoreCase(errStr, "no route to host"):
		return ErrorCategoryConnection
	case containsIgnoreCase(errStr, "no such host"),
		containsIgnoreCase(errStr, "dns"):
		return ErrorCategoryDNS
	default:
		return ErrorCategoryUnknown
	}
}

// HTTPError represents an HTTP error with status code
type HTTPError struct {
	StatusCode int
	Status     string
	Body       []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Status)
}

// NewHTTPError creates a new HTTP error
func NewHTTPError(resp *http.Response, body []byte) *HTTPError {
	return &HTTPError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       body,
	}
}

// Circuit breaker error
var ErrCircuitOpen = errors.New("circuit breaker is open")

// containsIgnoreCase checks if a string contains a substring (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}

	// Convert both strings to lowercase for comparison
	sLower := strings.ToLower(s)
	substrLower := strings.ToLower(substr)

	return strings.Contains(sLower, substrLower)
}

// Metrics holds metrics for retry operations
type Metrics struct {
	TotalRetries  int64
	SuccessfulOps int64
	FailedOps     int64
	TotalAttempts int64
	TotalDelay    time.Duration
	ErrorsByType  map[ErrorCategory]int64
	mu            sync.RWMutex
}

// NewMetrics creates new retry metrics
func NewMetrics() *Metrics {
	return &Metrics{
		ErrorsByType: make(map[ErrorCategory]int64),
	}
}

// RecordResult records the result of a retry operation
func (m *Metrics) RecordResult(result *Result) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalRetries++
	m.TotalAttempts += int64(result.Attempts)
	m.TotalDelay += result.TotalDelay

	if result.Success {
		m.SuccessfulOps++
	} else {
		m.FailedOps++

		// Categorize the final error
		if result.LastError != nil {
			retrier := &Retrier{} // Temporary instance for categorization
			category := retrier.categorizeError(result.LastError)
			m.ErrorsByType[category]++
		}
	}
}

// GetStats returns current retry statistics
func (m *Metrics) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := map[string]interface{}{
		"total_retries":  m.TotalRetries,
		"successful_ops": m.SuccessfulOps,
		"failed_ops":     m.FailedOps,
		"total_attempts": m.TotalAttempts,
		"total_delay_ms": m.TotalDelay.Milliseconds(),
		"errors_by_type": make(map[string]int64),
	}

	// Convert error categories to strings
	errorsByType := stats["errors_by_type"].(map[string]int64)
	for category, count := range m.ErrorsByType {
		switch category {
		case ErrorCategoryTimeout:
			errorsByType["timeout"] = count
		case ErrorCategoryConnection:
			errorsByType["connection"] = count
		case ErrorCategoryHTTP5xx:
			errorsByType["http_5xx"] = count
		case ErrorCategoryHTTP4xx:
			errorsByType["http_4xx"] = count
		case ErrorCategoryDNS:
			errorsByType["dns"] = count
		case ErrorCategoryCircuitOpen:
			errorsByType["circuit_open"] = count
		default:
			errorsByType["unknown"] = count
		}
	}

	// Calculate success rate
	if m.TotalRetries > 0 {
		stats["success_rate"] = float64(m.SuccessfulOps) / float64(m.TotalRetries)
	} else {
		stats["success_rate"] = 0.0
	}

	// Calculate average attempts per operation
	if m.TotalRetries > 0 {
		stats["avg_attempts"] = float64(m.TotalAttempts) / float64(m.TotalRetries)
	} else {
		stats["avg_attempts"] = 0.0
	}

	return stats
}
