package server

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vitalvas/alertmanager-gateway/internal/cache"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/transform"
)

func TestTemplateCacheMemoryEfficiency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	// Create cache with limited size to test eviction
	templateCache := cache.NewTemplateCache(100, 5*time.Minute)

	memBefore := getMemoryStats(t)

	// Generate many unique templates to test cache behavior
	templates := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		templates[i] = fmt.Sprintf(`{"alert_%d": "{{.GroupLabels.alertname}}", "index": "%d"}`, i, i)
	}

	// Add templates to cache
	for i, template := range templates {
		engine, err := transform.NewGoTemplateEngine(template)
		require.NoError(t, err)

		cacheKey := fmt.Sprintf("template_%d", i)
		templateCache.Set(cacheKey, engine)

		// Every 100 templates, check memory
		if i%100 == 0 {
			memCurrent := getMemoryStats(t)
			t.Logf("Templates: %d, Memory: %d KB", i+1, memCurrent.Alloc/1024)
		}
	}

	memAfter := getMemoryStats(t)
	memDiff := memAfter.Alloc - memBefore.Alloc

	stats := templateCache.Stats()

	// Memory should be reasonable even with many templates
	assert.Less(t, memDiff, uint64(50*1024*1024), "Template cache should use < 50MB")
	assert.LessOrEqual(t, stats.TotalSize, 100, "Cache should respect size limit")

	t.Logf("Template Cache Memory Test:")
	t.Logf("  Templates created: %d", len(templates))
	t.Logf("  Cache size: %d", stats.TotalSize)
	t.Logf("  Memory used: %d KB", memDiff/1024)
}

func TestHighVolumeTransformations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	memBefore := getMemoryStats(t)

	// Create a complex template
	template := `{
		"timestamp": "{{.Timestamp}}",
		"status": "{{.Status}}",
		"alerts": [
			{{range $i, $alert := .Alerts}}
			{{if $i}},{{end}}
			{
				"name": "{{$alert.Labels.alertname}}",
				"instance": "{{$alert.Labels.instance}}",
				"severity": "{{$alert.Labels.severity}}",
				"description": "{{$alert.Annotations.description}}",
				"starts_at": "{{$alert.StartsAt.Format \"2006-01-02T15:04:05Z\"}}",
				"fingerprint": "{{$alert.Fingerprint}}"
			}
			{{end}}
		],
		"receiver": "{{.Receiver}}",
		"external_url": "{{.ExternalURL}}"
	}`

	engine, err := transform.NewGoTemplateEngine(template)
	require.NoError(t, err)

	// Create sample webhook with multiple alerts
	webhook := createSimpleWebhook(100) // 100 alerts

	// Perform many transformations
	transformations := 1000
	for i := 0; i < transformations; i++ {
		_, err := engine.Transform(webhook)
		require.NoError(t, err)

		// Force GC every 100 transformations
		if i%100 == 0 {
			runtime.GC()
		}
	}

	memAfter := getMemoryStats(t)
	memDiff := memAfter.Alloc - memBefore.Alloc

	// Memory growth should be minimal for repeated transformations
	assert.Less(t, memDiff, uint64(10*1024*1024), "Memory growth should be < 10MB for transformations")

	t.Logf("High Volume Transformations:")
	t.Logf("  Transformations: %d", transformations)
	t.Logf("  Alerts per webhook: %d", len(webhook.Alerts))
	t.Logf("  Memory growth: %d KB", memDiff/1024)
}

func TestGoroutineLeakDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	initialGoroutines := runtime.NumGoroutine()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "goroutine-test",
				URL:      "http://example.com",
				Enabled:  true,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"test": "{{.Status}}"}`,
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Create and destroy multiple server instances
	for i := 0; i < 10; i++ {
		srv, err := New(cfg, logger)
		require.NoError(t, err)

		// Simulate some activity
		time.Sleep(10 * time.Millisecond)

		// Server cleanup should happen automatically when out of scope
		_ = srv
	}

	// Force garbage collection
	runtime.GC()
	runtime.GC() // Run twice to ensure cleanup
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	goroutineDiff := finalGoroutines - initialGoroutines

	// Should not leak significant goroutines
	assert.Less(t, goroutineDiff, 5, "Should not leak more than 5 goroutines")

	t.Logf("Goroutine Leak Test:")
	t.Logf("  Initial goroutines: %d", initialGoroutines)
	t.Logf("  Final goroutines: %d", finalGoroutines)
	t.Logf("  Difference: %d", goroutineDiff)
}

func TestJSONMarshallingPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	webhook := createLargeWebhook(1000) // Large webhook with 1000 alerts

	iterations := 1000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		_, err := json.Marshal(webhook)
		require.NoError(t, err)
	}

	duration := time.Since(start)
	avgTime := duration / time.Duration(iterations)

	// JSON marshalling should be fast
	assert.Less(t, avgTime.Microseconds(), int64(1000), "Average JSON marshalling should be < 1ms")

	t.Logf("JSON Marshalling Performance:")
	t.Logf("  Iterations: %d", iterations)
	t.Logf("  Total duration: %v", duration)
	t.Logf("  Average per operation: %v", avgTime)
	t.Logf("  Operations per second: %.0f", float64(iterations)/duration.Seconds())
}

func TestConcurrentCacheAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	templateCache := cache.NewTemplateCache(1000, 5*time.Minute)

	memBefore := getMemoryStats(t)

	// Create base templates
	templates := make([]string, 100)
	for i := 0; i < 100; i++ {
		templates[i] = fmt.Sprintf(`{"template_%d": "{{.GroupLabels.alertname}}"}`, i)
	}

	// Pre-populate cache
	for i, template := range templates {
		engine, err := transform.NewGoTemplateEngine(template)
		require.NoError(t, err)
		key := fmt.Sprintf("template_%d", i)
		templateCache.Set(key, engine)
	}

	// Concurrent access test
	concurrency := 50
	operationsPerWorker := 200

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < operationsPerWorker; j++ {
				templateIdx := (workerID + j) % len(templates)
				template := templates[templateIdx]
				key := fmt.Sprintf("template_%d", templateIdx)

				// Mix of reads and writes
				if j%10 == 0 {
					// Write operation
					engine, err := transform.NewGoTemplateEngine(template)
					require.NoError(t, err)
					templateCache.Set(key, engine)
				} else {
					// Read operation
					engine, found := templateCache.Get(key)
					assert.True(t, found)
					assert.NotNil(t, engine)
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	memAfter := getMemoryStats(t)
	memDiff := memAfter.Alloc - memBefore.Alloc

	totalOps := concurrency * operationsPerWorker
	opsPerSecond := float64(totalOps) / duration.Seconds()

	// Performance assertions
	assert.Greater(t, opsPerSecond, 10000.0, "Should handle > 10k cache operations per second")
	assert.Less(t, memDiff, uint64(20*1024*1024), "Memory growth should be < 20MB")

	t.Logf("Concurrent Cache Access:")
	t.Logf("  Workers: %d", concurrency)
	t.Logf("  Operations per worker: %d", operationsPerWorker)
	t.Logf("  Total operations: %d", totalOps)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Operations/sec: %.0f", opsPerSecond)
	t.Logf("  Memory growth: %d KB", memDiff/1024)
}

func createLargeWebhook(alertCount int) interface{} {
	alerts := make([]map[string]interface{}, alertCount)

	for i := 0; i < alertCount; i++ {
		alerts[i] = map[string]interface{}{
			"status": "firing",
			"labels": map[string]string{
				"alertname": "TestAlert",
				"instance":  fmt.Sprintf("test-instance-%d", i),
				"job":       "test-job",
				"severity":  "warning",
			},
			"annotations": map[string]string{
				"summary":     fmt.Sprintf("Test alert %d", i),
				"description": "This is a test alert for performance testing",
			},
			"startsAt":     time.Now().Add(-5 * time.Minute),
			"endsAt":       time.Time{},
			"generatorURL": "http://prometheus:9090/graph",
			"fingerprint":  fmt.Sprintf("test-fingerprint-%d", i),
		}
	}

	return map[string]interface{}{
		"version":  "4",
		"groupKey": "{}:{alertname=\"TestAlert\"}",
		"status":   "firing",
		"receiver": "test-receiver",
		"groupLabels": map[string]string{
			"alertname": "TestAlert",
		},
		"commonLabels": map[string]string{
			"alertname": "TestAlert",
			"job":       "test-job",
			"severity":  "warning",
		},
		"commonAnnotations": map[string]string{
			"summary": "Test alerts for performance testing",
		},
		"externalURL": "http://alertmanager:9093",
		"alerts":      alerts,
	}
}