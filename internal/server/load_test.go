package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
)

type LoadTestResult struct {
	TotalRequests     int
	SuccessfulReqs    int
	FailedReqs        int
	TotalDuration     time.Duration
	AvgResponseTime   time.Duration
	P95ResponseTime   time.Duration
	P99ResponseTime   time.Duration
	RequestsPerSecond float64
	Errors            []string
}

func TestHighVolumeWebhookLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	// Test configuration
	concurrency := 50
	requestsPerWorker := 100
	totalRequests := concurrency * requestsPerWorker

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:         "localhost",
			Port:         8080,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "load-test",
				URL:      createMockServer(t),
				Enabled:  true,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"alert": "{{.GroupLabels.alertname}}", "status": "{{.Status}}"}`,
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise during load test

	srv, err := New(cfg, logger)
	require.NoError(t, err)

	testServer := httptest.NewServer(srv.GetRouter())
	defer testServer.Close()

	result := runLoadTest(t, testServer.URL+"/webhook/load-test", concurrency, requestsPerWorker)

	// Performance assertions
	assert.Equal(t, totalRequests, result.TotalRequests)
	assert.Greater(t, result.RequestsPerSecond, 100.0, "Should handle at least 100 req/s")
	assert.Less(t, result.AvgResponseTime.Milliseconds(), int64(1000), "Average response time should be < 1000ms")
	assert.Less(t, result.P99ResponseTime.Milliseconds(), int64(2000), "P99 response time should be < 2000ms")
	assert.Greater(t, float64(result.SuccessfulReqs)/float64(result.TotalRequests), 0.5, "Success rate should be > 50%")

	t.Logf("Load Test Results:")
	t.Logf("  Total Requests: %d", result.TotalRequests)
	t.Logf("  Successful: %d (%.2f%%)", result.SuccessfulReqs, float64(result.SuccessfulReqs)/float64(result.TotalRequests)*100)
	t.Logf("  Failed: %d", result.FailedReqs)
	t.Logf("  Duration: %v", result.TotalDuration)
	t.Logf("  Requests/sec: %.2f", result.RequestsPerSecond)
	t.Logf("  Avg Response Time: %v", result.AvgResponseTime)
	t.Logf("  P95 Response Time: %v", result.P95ResponseTime)
	t.Logf("  P99 Response Time: %v", result.P99ResponseTime)
}

func TestMemoryUsageUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "memory-test",
				URL:      createMockServer(t),
				Enabled:  true,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"alerts": [{{range $i, $alert := .Alerts}}{{if $i}},{{end}}{"name": "{{$alert.Labels.alertname}}", "instance": "{{$alert.Labels.instance}}"}{{end}}]}`,
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	srv, err := New(cfg, logger)
	require.NoError(t, err)

	testServer := httptest.NewServer(srv.GetRouter())
	defer testServer.Close()

	// Monitor memory usage during load
	memBefore := getMemoryStats(t)

	// Run moderate load for memory testing
	result := runLoadTest(t, testServer.URL+"/webhook/memory-test", 20, 50)

	memAfter := getMemoryStats(t)
	memDiff := memAfter.Alloc - memBefore.Alloc

	// Memory usage assertions
	assert.Less(t, memDiff, uint64(100*1024*1024), "Memory increase should be < 100MB")
	assert.Greater(t, float64(result.SuccessfulReqs)/float64(result.TotalRequests), 0.95, "Success rate should be > 95%")

	t.Logf("Memory Usage:")
	t.Logf("  Before: %d KB", memBefore.Alloc/1024)
	t.Logf("  After: %d KB", memAfter.Alloc/1024)
	t.Logf("  Difference: %d KB", memDiff/1024)
}

func TestConcurrentDestinations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	destinations := make([]config.DestinationConfig, 10)
	for i := 0; i < 10; i++ {
		destinations[i] = config.DestinationConfig{
			Name:     fmt.Sprintf("dest-%d", i),
			URL:      createMockServer(t),
			Enabled:  true,
			Method:   "POST",
			Format:   "json",
			Engine:   "go-template",
			Template: fmt.Sprintf(`{"destination": "%d", "alert": "{{.GroupLabels.alertname}}"}`, i),
		}
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
		Destinations: destinations,
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	srv, err := New(cfg, logger)
	require.NoError(t, err)

	testServer := httptest.NewServer(srv.GetRouter())
	defer testServer.Close()

	// Test all destinations concurrently
	var wg sync.WaitGroup
	results := make(chan LoadTestResult, 10)

	start := time.Now()
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(destIdx int) {
			defer wg.Done()
			url := fmt.Sprintf("%s/webhook/dest-%d", testServer.URL, destIdx)
			result := runLoadTest(t, url, 10, 20)
			results <- result
		}(i)
	}

	wg.Wait()
	close(results)
	totalDuration := time.Since(start)

	// Aggregate results
	totalRequests := 0
	successfulReqs := 0
	for result := range results {
		totalRequests += result.TotalRequests
		successfulReqs += result.SuccessfulReqs
	}

	overallThroughput := float64(totalRequests) / totalDuration.Seconds()

	assert.Greater(t, overallThroughput, 50.0, "Overall throughput should be > 50 req/s")
	assert.Greater(t, float64(successfulReqs)/float64(totalRequests), 0.5, "Success rate should be > 50%")

	t.Logf("Concurrent Destinations Test:")
	t.Logf("  Total Requests: %d", totalRequests)
	t.Logf("  Successful: %d", successfulReqs)
	t.Logf("  Duration: %v", totalDuration)
	t.Logf("  Overall Throughput: %.2f req/s", overallThroughput)
}

func runLoadTest(_ *testing.T, url string, concurrency, requestsPerWorker int) LoadTestResult {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var responseTimes []time.Duration
	var errors []string
	successCount := 0
	failCount := 0

	webhook := createSampleWebhook()
	payload, _ := json.Marshal(webhook)

	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 30 * time.Second}

			for j := 0; j < requestsPerWorker; j++ {
				reqStart := time.Now()

				req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
				if err != nil {
					mu.Lock()
					errors = append(errors, err.Error())
					failCount++
					mu.Unlock()
					continue
				}

				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				duration := time.Since(reqStart)

				mu.Lock()
				responseTimes = append(responseTimes, duration)
				if err != nil || resp.StatusCode >= 400 {
					if err != nil {
						errors = append(errors, err.Error())
					} else {
						errors = append(errors, fmt.Sprintf("HTTP %d", resp.StatusCode))
					}
					failCount++
				} else {
					successCount++
				}
				mu.Unlock()

				if resp != nil {
					resp.Body.Close()
				}
			}
		}()
	}

	wg.Wait()
	totalDuration := time.Since(start)

	// Calculate statistics
	totalRequests := len(responseTimes)
	var totalTime time.Duration
	for _, rt := range responseTimes {
		totalTime += rt
	}

	avgResponseTime := totalTime / time.Duration(totalRequests)

	// Sort for percentiles
	sortedTimes := make([]time.Duration, len(responseTimes))
	copy(sortedTimes, responseTimes)

	// Simple sort implementation
	for i := 0; i < len(sortedTimes); i++ {
		for j := i + 1; j < len(sortedTimes); j++ {
			if sortedTimes[i] > sortedTimes[j] {
				sortedTimes[i], sortedTimes[j] = sortedTimes[j], sortedTimes[i]
			}
		}
	}

	p95Index := int(float64(len(sortedTimes)) * 0.95)
	p99Index := int(float64(len(sortedTimes)) * 0.99)

	p95ResponseTime := sortedTimes[p95Index-1]
	p99ResponseTime := sortedTimes[p99Index-1]

	return LoadTestResult{
		TotalRequests:     totalRequests,
		SuccessfulReqs:    successCount,
		FailedReqs:        failCount,
		TotalDuration:     totalDuration,
		AvgResponseTime:   avgResponseTime,
		P95ResponseTime:   p95ResponseTime,
		P99ResponseTime:   p99ResponseTime,
		RequestsPerSecond: float64(totalRequests) / totalDuration.Seconds(),
		Errors:            errors,
	}
}

func createMockServer(t *testing.T) string {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Simulate processing time
		time.Sleep(1 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "received"}`))
	}))

	t.Cleanup(func() {
		server.Close()
	})

	return server.URL
}

func createSampleWebhook() *alertmanager.WebhookPayload {
	return &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "{}:{alertname=\"LoadTestAlert\"}",
		Status:   "firing",
		Receiver: "load-test-receiver",
		GroupLabels: map[string]string{
			"alertname": "LoadTestAlert",
		},
		CommonLabels: map[string]string{
			"alertname": "LoadTestAlert",
			"instance":  "load-test:9090",
			"job":       "load-test",
			"severity":  "warning",
		},
		CommonAnnotations: map[string]string{
			"summary":     "Load test alert",
			"description": "This is a load test alert",
		},
		ExternalURL: "http://alertmanager:9093",
		Alerts: []alertmanager.Alert{
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "LoadTestAlert",
					"instance":  "load-test:9090",
					"job":       "load-test",
					"severity":  "warning",
				},
				Annotations: map[string]string{
					"summary":     "Load test alert",
					"description": "This is a load test alert",
				},
				StartsAt:     time.Now().Add(-5 * time.Minute),
				EndsAt:       time.Time{},
				GeneratorURL: "http://prometheus:9090/graph",
				Fingerprint:  "load-test-fingerprint",
			},
		},
	}
}
