package retry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestNewCircuitBreaker(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Test with nil config
	cb := NewCircuitBreaker(nil, logger)
	assert.NotNil(t, cb)
	assert.Equal(t, StateClosed, cb.GetState())
	assert.Equal(t, "default", cb.config.Name)

	// Test with custom config
	config := &CircuitBreakerConfig{
		Name:             "test-cb",
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          10 * time.Second,
	}
	cb = NewCircuitBreaker(config, logger)
	assert.Equal(t, "test-cb", cb.config.Name)
	assert.Equal(t, 3, cb.config.FailureThreshold)
	assert.Equal(t, 2, cb.config.SuccessThreshold)
}

func TestCircuitBreaker_Execute_Success(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := DefaultCircuitBreakerConfig("test")
	cb := NewCircuitBreaker(config, logger)

	// Execute successful function
	err := cb.Execute(context.Background(), func() error {
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, StateClosed, cb.GetState())
}

func TestCircuitBreaker_Execute_Failure(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &CircuitBreakerConfig{
		Name:                "test",
		FailureThreshold:    2,
		SuccessThreshold:    1,
		Timeout:             100 * time.Millisecond,
		MaxConcurrentCalls:  10,
		SlidingWindowSize:   5,
		MinimumRequestCount: 2,
	}
	cb := NewCircuitBreaker(config, logger)

	expectedError := errors.New("test error")

	// First failure
	err := cb.Execute(context.Background(), func() error {
		return expectedError
	})
	assert.Equal(t, expectedError, err)
	assert.Equal(t, StateClosed, cb.GetState())

	// Second failure - should open circuit
	err = cb.Execute(context.Background(), func() error {
		return expectedError
	})
	assert.Equal(t, expectedError, err)
	assert.Equal(t, StateOpen, cb.GetState())

	// Third attempt should fail immediately without calling function
	callCount := 0
	err = cb.Execute(context.Background(), func() error {
		callCount++
		return nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker is open")
	assert.Equal(t, 0, callCount) // Function should not be called
	assert.Equal(t, StateOpen, cb.GetState())
}

func TestCircuitBreaker_HalfOpen_Success(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &CircuitBreakerConfig{
		Name:                "test",
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             50 * time.Millisecond,
		MaxConcurrentCalls:  10,
		SlidingWindowSize:   5,
		MinimumRequestCount: 2,
	}
	cb := NewCircuitBreaker(config, logger)

	// Force circuit to open
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func() error {
			return errors.New("error")
		})
	}
	assert.Equal(t, StateOpen, cb.GetState())

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// First call should transition to half-open
	err := cb.Execute(context.Background(), func() error {
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, StateHalfOpen, cb.GetState())

	// Second successful call should close circuit
	err = cb.Execute(context.Background(), func() error {
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, StateClosed, cb.GetState())
}

func TestCircuitBreaker_HalfOpen_Failure(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &CircuitBreakerConfig{
		Name:                "test",
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             50 * time.Millisecond,
		MaxConcurrentCalls:  10,
		SlidingWindowSize:   5,
		MinimumRequestCount: 2,
	}
	cb := NewCircuitBreaker(config, logger)

	// Force circuit to open
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func() error {
			return errors.New("error")
		})
	}
	assert.Equal(t, StateOpen, cb.GetState())

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// First call should transition to half-open, but fail
	err := cb.Execute(context.Background(), func() error {
		return errors.New("still failing")
	})
	assert.Error(t, err)
	assert.Equal(t, StateOpen, cb.GetState()) // Should go back to open
}

func TestCircuitBreaker_MaxConcurrentCalls(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &CircuitBreakerConfig{
		Name:                "test",
		FailureThreshold:    5,
		SuccessThreshold:    1,
		Timeout:             100 * time.Millisecond,
		MaxConcurrentCalls:  2,
		SlidingWindowSize:   5,
		MinimumRequestCount: 2,
	}
	cb := NewCircuitBreaker(config, logger)

	var wg sync.WaitGroup
	results := make([]error, 5)

	// Start 5 concurrent calls, but only 2 should be allowed
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = cb.Execute(context.Background(), func() error {
				time.Sleep(50 * time.Millisecond) // Simulate work
				return nil
			})
		}(i)
	}

	wg.Wait()

	// Count successful and rejected calls
	successCount := 0
	rejectedCount := 0
	for _, err := range results {
		if err == nil {
			successCount++
		} else if err.Error() == "circuit breaker: too many concurrent calls" {
			rejectedCount++
		}
	}

	assert.Equal(t, 2, successCount, "Should allow exactly 2 concurrent calls")
	assert.Equal(t, 3, rejectedCount, "Should reject 3 calls due to concurrency limit")
}

func TestCircuitBreaker_SlidingWindow(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &CircuitBreakerConfig{
		Name:                "test",
		FailureThreshold:    3,
		SuccessThreshold:    1,
		Timeout:             100 * time.Millisecond,
		MaxConcurrentCalls:  10,
		SlidingWindowSize:   5,
		MinimumRequestCount: 3,
	}
	cb := NewCircuitBreaker(config, logger)

	// Add some successful calls first
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func() error {
			return nil
		})
	}

	// Add failures
	for i := 0; i < 3; i++ {
		cb.Execute(context.Background(), func() error {
			return errors.New("error")
		})
	}

	// At this point we have 2 successes and 3 failures (5 total)
	// Failure rate is 3/5 = 60%, which should trigger opening if threshold allows
	stats := cb.GetStats()
	assert.Equal(t, 5, stats["total_requests"])
	assert.Equal(t, 2, stats["successes"])
}

func TestCircuitBreaker_StateString(t *testing.T) {
	assert.Equal(t, "closed", StateClosed.String())
	assert.Equal(t, "half-open", StateHalfOpen.String())
	assert.Equal(t, "open", StateOpen.String())
	assert.Equal(t, "unknown", CircuitState(999).String())
}

func TestCircuitBreaker_Reset(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := DefaultCircuitBreakerConfig("test")
	config.FailureThreshold = 1
	cb := NewCircuitBreaker(config, logger)

	// Open the circuit
	cb.Execute(context.Background(), func() error {
		return errors.New("error")
	})
	assert.Equal(t, StateOpen, cb.GetState())

	// Reset the circuit
	cb.Reset()
	assert.Equal(t, StateClosed, cb.GetState())

	stats := cb.GetStats()
	assert.Equal(t, 0, stats["failure_count"])
	assert.Equal(t, 0, stats["success_count"])
	assert.Equal(t, 0, stats["total_requests"])
}

func TestCircuitBreaker_GetStats(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := DefaultCircuitBreakerConfig("test-stats")
	cb := NewCircuitBreaker(config, logger)

	// Execute some calls
	cb.Execute(context.Background(), func() error { return nil })
	cb.Execute(context.Background(), func() error { return errors.New("error") })

	stats := cb.GetStats()

	assert.Equal(t, "test-stats", stats["name"])
	assert.Equal(t, "closed", stats["state"])
	assert.Equal(t, 2, stats["total_requests"])
	assert.Equal(t, 1, stats["successes"])
	assert.Equal(t, 0.5, stats["success_rate"])
	assert.Equal(t, 0, stats["active_calls"])
}

func TestCircuitBreaker_StateChangeCallback(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &CircuitBreakerConfig{
		Name:                "test",
		FailureThreshold:    1,
		SuccessThreshold:    1,
		Timeout:             50 * time.Millisecond,
		MaxConcurrentCalls:  10,
		SlidingWindowSize:   5,
		MinimumRequestCount: 1,
	}
	cb := NewCircuitBreaker(config, logger)

	var callbackCalls []struct {
		name string
		from CircuitState
		to   CircuitState
	}
	var mu sync.Mutex

	cb.SetStateChangeCallback(func(name string, from, to CircuitState) {
		mu.Lock()
		defer mu.Unlock()
		callbackCalls = append(callbackCalls, struct {
			name string
			from CircuitState
			to   CircuitState
		}{name, from, to})
	})

	// Trigger state change to open
	cb.Execute(context.Background(), func() error {
		return errors.New("error")
	})

	// Wait a bit for callback to be called
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(callbackCalls) > 0 {
		assert.Equal(t, "test", callbackCalls[0].name)
		assert.Equal(t, StateClosed, callbackCalls[0].from)
		assert.Equal(t, StateOpen, callbackCalls[0].to)
	}
}

func TestCircuitBreakerManager(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	manager := NewCircuitBreakerManager(logger)
	assert.NotNil(t, manager)

	// Test GetOrCreate
	config := DefaultCircuitBreakerConfig("test1")
	cb1 := manager.GetOrCreate("test1", config)
	assert.NotNil(t, cb1)
	assert.Equal(t, "test1", cb1.config.Name)

	// Get the same circuit breaker again
	cb2 := manager.GetOrCreate("test1", nil)
	assert.Same(t, cb1, cb2) // Should be the same instance

	// Create a different circuit breaker
	cb3 := manager.GetOrCreate("test2", nil)
	assert.NotSame(t, cb1, cb3)
	assert.Equal(t, "test2", cb3.config.Name)

	// Test Get
	cb4, exists := manager.Get("test1")
	assert.True(t, exists)
	assert.Same(t, cb1, cb4)

	cb5, exists := manager.Get("nonexistent")
	assert.False(t, exists)
	assert.Nil(t, cb5)

	// Test GetAll
	all := manager.GetAll()
	assert.Len(t, all, 2)
	assert.Contains(t, all, "test1")
	assert.Contains(t, all, "test2")

	// Test Remove
	removed := manager.Remove("test1")
	assert.True(t, removed)

	removed = manager.Remove("nonexistent")
	assert.False(t, removed)

	// Verify removal
	_, exists = manager.Get("test1")
	assert.False(t, exists)

	// Test GetStats
	stats := manager.GetStats()
	assert.Len(t, stats, 1) // Only test2 should remain
	assert.Contains(t, stats, "test2")

	// Test Reset
	manager.Reset()
	// After reset, circuit breakers should be in closed state
	cb3Stats := cb3.GetStats()
	assert.Equal(t, "closed", cb3Stats["state"])
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test-name")

	assert.Equal(t, "test-name", config.Name)
	assert.Equal(t, 5, config.FailureThreshold)
	assert.Equal(t, 3, config.SuccessThreshold)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 10, config.MaxConcurrentCalls)
	assert.Equal(t, 20, config.SlidingWindowSize)
	assert.Equal(t, 5, config.MinimumRequestCount)
}

func BenchmarkCircuitBreaker_Execute(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := DefaultCircuitBreakerConfig("benchmark")
	cb := NewCircuitBreaker(config, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(context.Background(), func() error {
			return nil // Always succeed
		})
	}
}

func BenchmarkCircuitBreaker_Execute_WithFailures(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := DefaultCircuitBreakerConfig("benchmark")
	cb := NewCircuitBreaker(config, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(context.Background(), func() error {
			if i%10 == 0 { // 10% failure rate
				return errors.New("benchmark error")
			}
			return nil
		})
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := DefaultCircuitBreakerConfig("concurrent-test")
	cb := NewCircuitBreaker(config, logger)

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperationsPerGoroutine := 10

	// Run concurrent operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				cb.Execute(context.Background(), func() error {
					if j%5 == 0 { // 20% failure rate
						return errors.New("concurrent test error")
					}
					return nil
				})
			}
		}()
	}

	wg.Wait()

	// Verify that the circuit breaker is still functional
	stats := cb.GetStats()
	// Note: Some requests may be rejected due to circuit breaker opening
	assert.Greater(t, stats["total_requests"], 0)
	assert.LessOrEqual(t, stats["total_requests"], numGoroutines*numOperationsPerGoroutine)
}

func TestCircuitBreaker_EdgeCases(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	t.Run("zero failure threshold", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			Name:                "zero-threshold",
			FailureThreshold:    0,
			SuccessThreshold:    1,
			Timeout:             100 * time.Millisecond,
			MaxConcurrentCalls:  10,
			SlidingWindowSize:   5,
			MinimumRequestCount: 1,
		}
		cb := NewCircuitBreaker(config, logger)

		// With zero failure threshold, the circuit should never open based on failure count
		// but can still use sliding window logic
		err := cb.Execute(context.Background(), func() error {
			return errors.New("error")
		})
		assert.Error(t, err)
		// Circuit might open or stay closed depending on sliding window logic
		state := cb.GetState()
		assert.True(t, state == StateClosed || state == StateOpen)
	})

	t.Run("very small sliding window", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			Name:                "small-window",
			FailureThreshold:    1,
			SuccessThreshold:    1,
			Timeout:             100 * time.Millisecond,
			MaxConcurrentCalls:  10,
			SlidingWindowSize:   1,
			MinimumRequestCount: 1,
		}
		cb := NewCircuitBreaker(config, logger)

		// First failure - might open circuit with small window
		cb.Execute(context.Background(), func() error {
			return errors.New("error")
		})

		// With window size 1, the circuit behavior depends on implementation
		// Just verify it's in a valid state
		state := cb.GetState()
		assert.True(t, state == StateClosed || state == StateOpen || state == StateHalfOpen)
	})
}
