package retry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	StateClosed CircuitState = iota
	StateHalfOpen
	StateOpen
)

// String returns the string representation of circuit state
func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateHalfOpen:
		return "half-open"
	case StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Name                string        `yaml:"name"`
	FailureThreshold    int           `yaml:"failure_threshold"`
	SuccessThreshold    int           `yaml:"success_threshold"`
	Timeout             time.Duration `yaml:"timeout"`
	MaxConcurrentCalls  int           `yaml:"max_concurrent_calls"`
	SlidingWindowSize   int           `yaml:"sliding_window_size"`
	MinimumRequestCount int           `yaml:"minimum_request_count"`
}

// DefaultCircuitBreakerConfig returns default circuit breaker configuration
func DefaultCircuitBreakerConfig(name string) *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		Name:                name,
		FailureThreshold:    5,                // Open after 5 failures
		SuccessThreshold:    3,                // Close after 3 successes in half-open
		Timeout:             30 * time.Second, // Stay open for 30 seconds
		MaxConcurrentCalls:  10,               // Max 10 concurrent calls
		SlidingWindowSize:   20,               // Track last 20 requests
		MinimumRequestCount: 5,                // Need at least 5 requests before considering failure rate
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config        *CircuitBreakerConfig
	state         CircuitState
	failureCount  int
	successCount  int
	requests      []bool // true for success, false for failure
	requestIndex  int
	requestCount  int // Total requests recorded (up to sliding window size)
	lastFailTime  time.Time
	halfOpenTime  time.Time
	activeCalls   int
	mu            sync.RWMutex
	logger        *logrus.Entry
	onStateChange func(name string, from, to CircuitState)
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config *CircuitBreakerConfig, logger *logrus.Logger) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig("default")
	}

	cb := &CircuitBreaker{
		config:   config,
		state:    StateClosed,
		requests: make([]bool, config.SlidingWindowSize),
		logger:   logger.WithField("component", "circuit-breaker").WithField("name", config.Name),
	}

	cb.logger.WithField("config", config).Info("Circuit breaker initialized")

	return cb
}

// SetStateChangeCallback sets a callback for state changes
func (cb *CircuitBreaker) SetStateChangeCallback(callback func(name string, from, to CircuitState)) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onStateChange = callback
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(_ context.Context, fn func() error) error {
	// Check if we can proceed
	if err := cb.beforeCall(); err != nil {
		return err
	}

	defer cb.afterCall()

	// Execute the function
	err := fn()

	// Record the result
	cb.recordResult(err == nil)

	return err
}

// beforeCall checks if the call can proceed and increments active calls
func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Check if circuit is open
	if cb.state == StateOpen {
		// Check if timeout has elapsed
		if time.Since(cb.lastFailTime) >= cb.config.Timeout {
			cb.setState(StateHalfOpen)
			cb.halfOpenTime = time.Now()
			cb.logger.Debug("Circuit breaker transitioning to half-open state")
		} else {
			return fmt.Errorf("circuit breaker is open: %w", ErrCircuitOpen)
		}
	}

	// Check concurrent call limit
	if cb.activeCalls >= cb.config.MaxConcurrentCalls {
		return errors.New("circuit breaker: too many concurrent calls")
	}

	cb.activeCalls++
	return nil
}

// afterCall decrements active calls counter
func (cb *CircuitBreaker) afterCall() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.activeCalls--
}

// recordResult records the result of a call and updates circuit state
func (cb *CircuitBreaker) recordResult(success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Add to sliding window
	cb.requests[cb.requestIndex] = success
	cb.requestIndex = (cb.requestIndex + 1) % len(cb.requests)
	if cb.requestCount < len(cb.requests) {
		cb.requestCount++
	}

	switch cb.state {
	case StateClosed:
		if success {
			cb.failureCount = 0
		} else {
			cb.failureCount++
			if cb.shouldOpen() {
				cb.setState(StateOpen)
				cb.lastFailTime = time.Now()
				cb.logger.WithField("failure_count", cb.failureCount).Warn("Circuit breaker opened due to failures")
			}
		}

	case StateHalfOpen:
		if success {
			cb.successCount++
			if cb.successCount >= cb.config.SuccessThreshold {
				cb.setState(StateClosed)
				cb.successCount = 0
				cb.failureCount = 0
				cb.logger.Info("Circuit breaker closed after successful half-open period")
			}
		} else {
			cb.setState(StateOpen)
			cb.lastFailTime = time.Now()
			cb.successCount = 0
			cb.logger.Warn("Circuit breaker opened again after failure in half-open state")
		}

	case StateOpen:
		// Record the result but don't change state (handled in beforeCall)
		if !success {
			cb.lastFailTime = time.Now()
		}
	}
}

// shouldOpen determines if the circuit should open based on failure rate
func (cb *CircuitBreaker) shouldOpen() bool {
	if cb.failureCount >= cb.config.FailureThreshold {
		return true
	}

	// Check failure rate in sliding window
	totalRequests := cb.requestCount
	failures := 0

	// Count failures in the recorded requests
	for i := 0; i < totalRequests && i < len(cb.requests); i++ {
		if !cb.requests[i] {
			failures++
		}
	}

	if totalRequests >= cb.config.MinimumRequestCount {
		failureRate := float64(failures) / float64(totalRequests)
		threshold := float64(cb.config.FailureThreshold) / float64(cb.config.SlidingWindowSize)

		cb.logger.WithFields(logrus.Fields{
			"failure_rate": failureRate,
			"threshold":    threshold,
			"failures":     failures,
			"total":        totalRequests,
		}).Debug("Checking failure rate")

		return failureRate >= threshold
	}

	return false
}

// setState changes the circuit state and triggers callback
func (cb *CircuitBreaker) setState(newState CircuitState) {
	oldState := cb.state
	cb.state = newState

	cb.logger.WithFields(logrus.Fields{
		"from": oldState.String(),
		"to":   newState.String(),
	}).Info("Circuit breaker state changed")

	if cb.onStateChange != nil {
		go cb.onStateChange(cb.config.Name, oldState, newState)
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns circuit breaker statistics
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Calculate stats from sliding window
	totalRequests := cb.requestCount
	successes := 0

	for i := 0; i < totalRequests && i < len(cb.requests); i++ {
		if cb.requests[i] {
			successes++
		}
	}

	var successRate float64
	if totalRequests > 0 {
		successRate = float64(successes) / float64(totalRequests)
	}

	stats := map[string]interface{}{
		"name":           cb.config.Name,
		"state":          cb.state.String(),
		"failure_count":  cb.failureCount,
		"success_count":  cb.successCount,
		"active_calls":   cb.activeCalls,
		"total_requests": totalRequests,
		"successes":      successes,
		"success_rate":   successRate,
	}

	// Add time-based stats
	switch cb.state {
	case StateOpen:
		stats["time_since_open"] = time.Since(cb.lastFailTime).Milliseconds()
		stats["time_until_half_open"] = (cb.config.Timeout - time.Since(cb.lastFailTime)).Milliseconds()
	case StateHalfOpen:
		stats["time_in_half_open"] = time.Since(cb.halfOpenTime).Milliseconds()
	}

	return stats
}

// Reset resets the circuit breaker to its initial state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := cb.state
	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.requests = make([]bool, cb.config.SlidingWindowSize)
	cb.requestIndex = 0
	cb.requestCount = 0
	cb.lastFailTime = time.Time{}
	cb.halfOpenTime = time.Time{}

	cb.logger.WithField("from_state", oldState.String()).Info("Circuit breaker reset")

	if cb.onStateChange != nil && oldState != StateClosed {
		go cb.onStateChange(cb.config.Name, oldState, StateClosed)
	}
}

// CircuitBreakerManager manages multiple circuit breakers
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
	logger   *logrus.Logger
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(logger *logrus.Logger) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
		logger:   logger,
	}
}

// GetOrCreate gets an existing circuit breaker or creates a new one
func (m *CircuitBreakerManager) GetOrCreate(name string, config *CircuitBreakerConfig) *CircuitBreaker {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cb, exists := m.breakers[name]; exists {
		return cb
	}

	if config == nil {
		config = DefaultCircuitBreakerConfig(name)
	}

	cb := NewCircuitBreaker(config, m.logger)
	m.breakers[name] = cb

	return cb
}

// Get gets an existing circuit breaker
func (m *CircuitBreakerManager) Get(name string) (*CircuitBreaker, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cb, exists := m.breakers[name]
	return cb, exists
}

// Remove removes a circuit breaker
func (m *CircuitBreakerManager) Remove(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.breakers[name]; exists {
		delete(m.breakers, name)
		return true
	}
	return false
}

// GetAll returns all circuit breakers
func (m *CircuitBreakerManager) GetAll() map[string]*CircuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*CircuitBreaker, len(m.breakers))
	for name, cb := range m.breakers {
		result[name] = cb
	}
	return result
}

// GetStats returns stats for all circuit breakers
func (m *CircuitBreakerManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]interface{})
	for name, cb := range m.breakers {
		stats[name] = cb.GetStats()
	}
	return stats
}

// Reset resets all circuit breakers
func (m *CircuitBreakerManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cb := range m.breakers {
		cb.Reset()
	}
}
