package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
)

// TestMemoryUsageRequirement tests that idle memory usage is < 100MB
func TestMemoryUsageRequirement(t *testing.T) {
	// Create minimal configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "test",
				URL:      "http://example.com",
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"alert":"{{.GroupLabels.alertname}}"}`,
				Enabled:  true,
			},
		},
	}

	// Create server
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, err := New(cfg, logger)
	assert.NoError(t, err)

	// Start server
	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Force GC and wait for it to complete
	runtime.GC()
	runtime.Gosched()
	time.Sleep(100 * time.Millisecond)

	// Measure memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Check memory usage
	allocMB := float64(m.Alloc) / 1024 / 1024
	sysMB := float64(m.Sys) / 1024 / 1024

	t.Logf("Memory Usage - Alloc: %.2f MB, Sys: %.2f MB", allocMB, sysMB)

	// Assert that allocated memory is less than 100MB
	assert.Less(t, allocMB, 100.0, "Allocated memory should be less than 100MB")
}

// TestThroughputRequirement tests that gateway can handle 1000 req/s
func TestThroughputRequirement(t *testing.T) {
	// Skip in short mode as this test takes time
	if testing.Short() {
		t.Skip("Skipping throughput test in short mode")
	}

	// Mock destination that responds quickly
	var requestCount int64
	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	// Configure gateway
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "perf-test",
				URL:      mockDest.URL,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"alert":"{{.GroupLabels.alertname}}","time":{{now | unixtime}}}`,
				Enabled:  true,
			},
		},
	}

	// Create server with optimizations enabled
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, err := New(cfg, logger)
	assert.NoError(t, err)

	// Start server
	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Prepare test payload
	webhook := &alertmanager.WebhookPayload{
		Version:     "4",
		GroupKey:    "perf-test",
		Status:      "firing",
		GroupLabels: map[string]string{"alertname": "PerfTest"},
		Alerts: []alertmanager.Alert{
			{
				Status:      "firing",
				Fingerprint: "perf-1",
				Labels:      map[string]string{"severity": "info"},
				StartsAt:    time.Now(),
			},
		},
	}
	payloadBytes, _ := json.Marshal(webhook)

	// Test parameters
	targetRPS := 1000
	testDuration := 5 * time.Second
	concurrency := 50

	// Channels for coordination
	start := make(chan struct{})
	done := make(chan struct{})

	// Stats
	var totalRequests int64
	var successfulRequests int64
	var failedRequests int64
	var totalLatency int64

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()

			client := &http.Client{
				Timeout: 5 * time.Second,
				Transport: &http.Transport{
					MaxIdleConns:        100,
					MaxIdleConnsPerHost: 10,
					IdleConnTimeout:     90 * time.Second,
				},
			}

			// Wait for start signal
			<-start

			// Calculate requests per worker
			requestsPerWorker := targetRPS / concurrency
			interval := time.Second / time.Duration(requestsPerWorker)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					startTime := time.Now()

					resp, err := client.Post(
						ts.URL+"/webhook/perf-test",
						"application/json",
						bytes.NewReader(payloadBytes),
					)

					latency := time.Since(startTime)
					atomic.AddInt64(&totalRequests, 1)
					atomic.AddInt64(&totalLatency, int64(latency))

					if err != nil {
						atomic.AddInt64(&failedRequests, 1)
					} else {
						if resp.StatusCode == http.StatusOK {
							atomic.AddInt64(&successfulRequests, 1)
						} else {
							atomic.AddInt64(&failedRequests, 1)
						}
						resp.Body.Close()
					}
				}
			}
		}(i)
	}

	// Start test
	testStart := time.Now()
	close(start)

	// Run for test duration
	time.Sleep(testDuration)
	close(done)

	// Wait for workers to finish
	wg.Wait()

	// Calculate results
	actualDuration := time.Since(testStart)
	total := atomic.LoadInt64(&totalRequests)
	successful := atomic.LoadInt64(&successfulRequests)
	failed := atomic.LoadInt64(&failedRequests)
	avgLatencyNs := atomic.LoadInt64(&totalLatency) / total
	avgLatencyMs := float64(avgLatencyNs) / 1e6
	actualRPS := float64(total) / actualDuration.Seconds()
	destRequests := atomic.LoadInt64(&requestCount)

	// Print results
	t.Logf("=== Throughput Test Results ===")
	t.Logf("Duration: %v", actualDuration)
	t.Logf("Total Requests: %d", total)
	t.Logf("Successful: %d (%.2f%%)", successful, float64(successful)/float64(total)*100)
	t.Logf("Failed: %d", failed)
	t.Logf("Actual RPS: %.2f", actualRPS)
	t.Logf("Average Latency: %.2f ms", avgLatencyMs)
	t.Logf("Destination Requests: %d", destRequests)

	// Assertions
	assert.Greater(t, actualRPS, 900.0, "Should achieve at least 900 req/s (90% of target)")
	assert.Less(t, float64(failed)/float64(total)*100, 1.0, "Error rate should be less than 1%")
	assert.Less(t, avgLatencyMs, 100.0, "Average latency should be less than 100ms")
}

// BenchmarkGatewayThroughput benchmarks the gateway throughput
func BenchmarkGatewayThroughput(b *testing.B) {
	// Mock destination
	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	// Configure gateway
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "bench",
				URL:      mockDest.URL,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"a":"{{.Status}}"}`,
				Enabled:  true,
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, _ := New(cfg, logger)

	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Prepare payload
	webhook := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "bench",
		Status:   "firing",
		Alerts:   []alertmanager.Alert{{Status: "firing", Fingerprint: "b1", StartsAt: time.Now()}},
	}
	payloadBytes, _ := json.Marshal(webhook)

	// Create reusable client
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.Post(
				ts.URL+"/webhook/bench",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
		}
	})
}

// TestLatencyPercentiles tests latency percentiles under load
func TestLatencyPercentiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping latency test in short mode")
	}

	// Mock destination with variable latency
	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Simulate variable processing time
		delay := time.Duration(1+int(time.Now().UnixNano()%10)) * time.Millisecond
		time.Sleep(delay)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	// Configure gateway
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "latency-test",
				URL:      mockDest.URL,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"alert":"{{.GroupLabels.alertname}}"}`,
				Enabled:  true,
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, _ := New(cfg, logger)

	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Test payload
	webhook := &alertmanager.WebhookPayload{
		Version:     "4",
		GroupKey:    "latency-test",
		Status:      "firing",
		GroupLabels: map[string]string{"alertname": "LatencyTest"},
		Alerts:      []alertmanager.Alert{{Status: "firing", Fingerprint: "l1", StartsAt: time.Now()}},
	}
	payloadBytes, _ := json.Marshal(webhook)

	// Collect latencies
	var latencies []time.Duration
	var mu sync.Mutex

	// Run requests
	concurrency := 20
	requestsPerWorker := 50
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 5 * time.Second}

			for j := 0; j < requestsPerWorker; j++ {
				start := time.Now()
				resp, err := client.Post(
					ts.URL+"/webhook/latency-test",
					"application/json",
					bytes.NewReader(payloadBytes),
				)
				if err == nil {
					resp.Body.Close()
					latency := time.Since(start)
					mu.Lock()
					latencies = append(latencies, latency)
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	// Calculate percentiles
	p50 := percentile(latencies, 50)
	p95 := percentile(latencies, 95)
	p99 := percentile(latencies, 99)

	t.Logf("=== Latency Percentiles ===")
	t.Logf("Samples: %d", len(latencies))
	t.Logf("P50: %v", p50)
	t.Logf("P95: %v", p95)
	t.Logf("P99: %v", p99)

	// Assertions
	assert.Less(t, p50, 50*time.Millisecond, "P50 latency should be less than 50ms")
	assert.Less(t, p95, 200*time.Millisecond, "P95 latency should be less than 200ms")
	assert.Less(t, p99, 500*time.Millisecond, "P99 latency should be less than 500ms")
}

// percentile calculates the percentile of durations
func percentile(durations []time.Duration, p float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// Sort durations
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Calculate percentile index
	index := int(float64(len(sorted)-1) * p / 100)
	return sorted[index]
}