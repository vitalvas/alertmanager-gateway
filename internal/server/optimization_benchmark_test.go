package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/cache"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/destination"
	"github.com/vitalvas/alertmanager-gateway/internal/transform"
)

// BenchmarkTemplateCaching compares cached vs non-cached template performance
func BenchmarkTemplateCaching(b *testing.B) {
	template := `{
		"alert": "{{.GroupLabels.alertname}}",
		"status": "{{.Status}}",
		"count": {{len .Alerts}},
		"alerts": [
			{{range $i, $a := .Alerts}}
			{{if $i}},{{end}}{
				"fingerprint": "{{$a.Fingerprint}}",
				"severity": "{{$a.Labels.severity}}"
			}
			{{end}}
		]
	}`

	webhook := &alertmanager.WebhookPayload{
		Status:      "firing",
		GroupLabels: map[string]string{"alertname": "TestAlert"},
		Alerts: []alertmanager.Alert{
			{Fingerprint: "fp1", Labels: map[string]string{"severity": "critical"}},
			{Fingerprint: "fp2", Labels: map[string]string{"severity": "warning"}},
			{Fingerprint: "fp3", Labels: map[string]string{"severity": "info"}},
		},
	}

	b.Run("NonCached", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			engine, err := transform.NewGoTemplateEngine(template)
			if err != nil {
				b.Fatal(err)
			}
			_, err = engine.Transform(webhook)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Cached", func(b *testing.B) {
		// Initialize cache
		transform.InitTemplateCache(100, 1*time.Hour)

		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			engine, err := transform.NewCachedGoTemplateEngine(template)
			if err != nil {
				b.Fatal(err)
			}
			_, err = engine.Transform(webhook)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkConnectionPooling compares pooled vs non-pooled HTTP clients
func BenchmarkConnectionPooling(b *testing.B) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	payload := []byte(`{"alert":"test"}`)

	b.Run("NonPooled", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				client := &http.Client{
					Timeout: 10 * time.Second,
				}
				resp, err := client.Post(server.URL, "application/json", bytes.NewReader(payload))
				if err != nil {
					b.Fatal(err)
				}
				resp.Body.Close()
			}
		})
	})

	b.Run("Pooled", func(b *testing.B) {
		pool := destination.NewClientPool(nil)
		defer pool.Close()

		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				client := pool.GetClient("test", 10*time.Second)
				resp, err := client.Post(server.URL, "application/json", bytes.NewReader(payload))
				if err != nil {
					b.Fatal(err)
				}
				resp.Body.Close()
			}
		})
	})
}

// BenchmarkHTTPHandler benchmarks the basic HTTP handler
func BenchmarkHTTPHandler(b *testing.B) {
	// Mock destination server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Simulate some processing
		time.Sleep(time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Configuration
	destConfig := &config.DestinationConfig{
		Name:     "bench-dest",
		URL:      server.URL,
		Method:   "POST",
		Format:   "json",
		Engine:   "go-template",
		Template: `{"alert":"{{.GroupLabels.alertname}}","count":{{len .Alerts}}}`,
		Enabled:  true,
	}

	// Create handler
	handler, err := destination.NewHTTPHandler(destConfig, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Test payload
	webhook := &alertmanager.WebhookPayload{
		Version:     "4",
		GroupKey:    "bench-group",
		Status:      "firing",
		GroupLabels: map[string]string{"alertname": "BenchAlert"},
		Alerts: []alertmanager.Alert{
			{Status: "firing", Fingerprint: "bench-1", Labels: map[string]string{"severity": "warning"}},
			{Status: "firing", Fingerprint: "bench-2", Labels: map[string]string{"severity": "critical"}},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		err := handler.Send(ctx, webhook)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCacheLRU benchmarks the LRU cache implementation
func BenchmarkCacheLRU(b *testing.B) {
	cache := cache.NewTemplateCache(1000, 1*time.Hour)

	// Pre-populate cache
	for i := 0; i < 500; i++ {
		key := fmt.Sprintf("key-%d", i)
		value := fmt.Sprintf("value-%d", i)
		cache.Set(key, value)
	}

	b.Run("Get", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				key := fmt.Sprintf("key-%d", i%500)
				cache.Get(key)
				i++
			}
		})
	})

	b.Run("Set", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 500
			for pb.Next() {
				key := fmt.Sprintf("key-%d", i)
				value := fmt.Sprintf("value-%d", i)
				cache.Set(key, value)
				i++
			}
		})
	})

	b.Run("Mixed", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				if i%4 == 0 {
					// 25% writes
					key := fmt.Sprintf("key-new-%d", i)
					value := fmt.Sprintf("value-new-%d", i)
					cache.Set(key, value)
				} else {
					// 75% reads
					key := fmt.Sprintf("key-%d", i%500)
					cache.Get(key)
				}
				i++
			}
		})
	})

	// Print cache stats
	stats := cache.Stats()
	b.Logf("Cache stats: Hits=%d, Misses=%d, Evictions=%d, Size=%d",
		stats.Hits, stats.Misses, stats.Evictions, stats.TotalSize)
}

// BenchmarkMemoryAllocation benchmarks memory allocation patterns
func BenchmarkMemoryAllocation(b *testing.B) {
	template := `{"alert":"{{.GroupLabels.alertname}}","status":"{{.Status}}"}`

	webhook := &alertmanager.WebhookPayload{
		Status:      "firing",
		GroupLabels: map[string]string{"alertname": "TestAlert"},
		Alerts:      make([]alertmanager.Alert, 10),
	}

	b.Run("WithBufferPool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			engine, _ := transform.NewCachedGoTemplateEngine(template)
			_, _ = engine.Transform(webhook)
		}
	})

	b.Run("WithoutBufferPool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			engine, _ := transform.NewGoTemplateEngine(template)
			_, _ = engine.Transform(webhook)
		}
	})
}
