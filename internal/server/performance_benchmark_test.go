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
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/formatter"
	"github.com/vitalvas/alertmanager-gateway/internal/transform"
)

// BenchmarkWebhookProcessing benchmarks the complete webhook processing pipeline
func BenchmarkWebhookProcessing(b *testing.B) {
	// Create a mock destination
	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "bench-dest",
				URL:      mockDest.URL,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"status":"{{.Status}}","count":{{len .Alerts}}}`,
				Enabled:  true,
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, err := New(cfg, logger)
	if err != nil {
		b.Fatal(err)
	}

	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Prepare webhook payload
	webhook := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "bench-group",
		Status:   "firing",
		Alerts: []alertmanager.Alert{
			{
				Status:      "firing",
				Labels:      map[string]string{"alertname": "BenchAlert"},
				Fingerprint: "bench-123",
				StartsAt:    time.Now(),
			},
		},
	}

	payloadBytes, _ := json.Marshal(webhook)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resp, err := http.Post(
			ts.URL+"/webhook/bench-dest",
			"application/json",
			bytes.NewReader(payloadBytes),
		)
		if err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}
	}
}

// BenchmarkWebhookProcessingParallel benchmarks parallel webhook processing
func BenchmarkWebhookProcessingParallel(b *testing.B) {
	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Simulate some processing time
		time.Sleep(time.Microsecond * 100)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "bench-parallel",
				URL:      mockDest.URL,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"alert":"{{.Status}}"}`,
				Enabled:  true,
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, err := New(cfg, logger)
	if err != nil {
		b.Fatal(err)
	}

	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	webhook := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "bench-parallel",
		Status:   "firing",
		Alerts:   []alertmanager.Alert{{Status: "firing", Fingerprint: "p-123", StartsAt: time.Now()}},
	}

	payloadBytes, _ := json.Marshal(webhook)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := http.Post(
				ts.URL+"/webhook/bench-parallel",
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

// BenchmarkTemplateTransformation benchmarks Go template transformation
func BenchmarkTemplateTransformation(b *testing.B) {
	template := `{
		"timestamp": {{now | unixtime}},
		"alerts": [
			{{range $i, $a := .Alerts}}
			{{if $i}},{{end}}{
				"name": "{{$a.Labels.alertname}}",
				"severity": "{{$a.Labels.severity}}"
			}
			{{end}}
		]
	}`

	// Create engine with the template
	engine, err := transform.NewGoTemplateEngine(template)
	if err != nil {
		b.Fatal(err)
	}

	webhook := &alertmanager.WebhookPayload{
		Status: "firing",
		Alerts: []alertmanager.Alert{
			{
				Status:      "firing",
				Fingerprint: "bench-cpu-1",
				Labels: map[string]string{
					"alertname": "HighCPU",
					"severity":  "warning",
				},
				StartsAt: time.Now(),
			},
			{
				Status:      "firing",
				Fingerprint: "bench-memory-1",
				Labels: map[string]string{
					"alertname": "HighMemory",
					"severity":  "critical",
				},
				StartsAt: time.Now(),
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := engine.Transform(webhook)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkJQTransformation benchmarks JQ transformation
func BenchmarkJQTransformation(b *testing.B) {
	query := `.alerts | map({
		name: .labels.alertname,
		severity: .labels.severity,
		timestamp: now
	})`

	// Create engine with the query
	engine, err := transform.NewJQEngine(query)
	if err != nil {
		b.Fatal(err)
	}

	webhook := &alertmanager.WebhookPayload{
		Status: "firing",
		Alerts: []alertmanager.Alert{
			{
				Status:      "firing",
				Fingerprint: "bench-cpu-2",
				Labels: map[string]string{
					"alertname": "HighCPU",
					"severity":  "warning",
				},
				StartsAt: time.Now(),
			},
			{
				Status:      "firing",
				Fingerprint: "bench-memory-2",
				Labels: map[string]string{
					"alertname": "HighMemory",
					"severity":  "critical",
				},
				StartsAt: time.Now(),
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := engine.Transform(webhook)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTemplateVsJQ compares template and JQ performance
func BenchmarkTemplateVsJQ(b *testing.B) {
	webhook := &alertmanager.WebhookPayload{
		Status: "firing",
		Alerts: []alertmanager.Alert{
			{Status: "firing", Fingerprint: "bench-1", Labels: map[string]string{"alertname": "Alert1", "severity": "warning"}, StartsAt: time.Now()},
			{Status: "firing", Fingerprint: "bench-2", Labels: map[string]string{"alertname": "Alert2", "severity": "critical"}, StartsAt: time.Now()},
			{Status: "firing", Fingerprint: "bench-3", Labels: map[string]string{"alertname": "Alert3", "severity": "info"}, StartsAt: time.Now()},
		},
	}

	b.Run("GoTemplate", func(b *testing.B) {
		template := `[{{range $i, $a := .Alerts}}{{if $i}},{{end}}{"alert":"{{$a.Labels.alertname}}","severity":"{{$a.Labels.severity}}"}{{end}}]`
		engine, err := transform.NewGoTemplateEngine(template)
		if err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = engine.Transform(webhook)
		}
	})

	b.Run("JQ", func(b *testing.B) {
		query := `.alerts | map({alert: .labels.alertname, severity: .labels.severity})`
		engine, err := transform.NewJQEngine(query)
		if err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = engine.Transform(webhook)
		}
	})
}

// BenchmarkFormatterPerformance benchmarks different output formatters
func BenchmarkFormatterPerformance(b *testing.B) {
	data := map[string]interface{}{
		"status": "firing",
		"alerts": []map[string]interface{}{
			{"name": "alert1", "severity": "critical", "value": 95.5},
			{"name": "alert2", "severity": "warning", "value": 75.2},
		},
		"metadata": map[string]interface{}{
			"source": "prometheus",
			"region": "us-east-1",
		},
	}

	b.Run("JSON", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := formatter.FormatData(formatter.FormatJSON, data)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Form", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := formatter.FormatData(formatter.FormatForm, data)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Query", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := formatter.FormatData(formatter.FormatQuery, data)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("XML", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := formatter.FormatData(formatter.FormatXML, data)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkAlertSplitting benchmarks alert splitting performance
func BenchmarkAlertSplitting(b *testing.B) {
	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	// Generate alerts
	numAlerts := 100
	alerts := make([]alertmanager.Alert, numAlerts)
	for i := 0; i < numAlerts; i++ {
		alerts[i] = alertmanager.Alert{
			Status: "firing",
			Labels: map[string]string{
				"alertname": fmt.Sprintf("Alert%d", i),
				"severity":  "warning",
			},
			Fingerprint: fmt.Sprintf("fp-%d", i),
			StartsAt:    time.Now(),
		}
	}

	webhook := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "bench-split",
		Status:   "firing",
		Alerts:   alerts,
	}

	b.Run("NoSplit", func(b *testing.B) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080},
			Destinations: []config.DestinationConfig{
				{
					Name:        "no-split",
					URL:         mockDest.URL,
					Method:      "POST",
					Format:      "json",
					Engine:      "go-template",
					Template:    `{"count":{{len .Alerts}}}`,
					Enabled:     true,
					SplitAlerts: false,
				},
			},
		}

		srv, _ := New(cfg, logrus.New())
		ts := httptest.NewServer(srv.GetRouter())
		defer ts.Close()

		payloadBytes, _ := json.Marshal(webhook)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, _ := http.Post(ts.URL+"/webhook/no-split", "application/json", bytes.NewReader(payloadBytes))
			resp.Body.Close()
		}
	})

	b.Run("Sequential", func(b *testing.B) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080},
			Destinations: []config.DestinationConfig{
				{
					Name:        "sequential",
					URL:         mockDest.URL,
					Method:      "POST",
					Format:      "json",
					Engine:      "go-template",
					Template:    `{"alert":"{{(index .Alerts 0).Labels.alertname}}"}`,
					Enabled:     true,
					SplitAlerts: true,
				},
			},
		}

		srv, _ := New(cfg, logrus.New())
		ts := httptest.NewServer(srv.GetRouter())
		defer ts.Close()

		payloadBytes, _ := json.Marshal(webhook)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, _ := http.Post(ts.URL+"/webhook/sequential", "application/json", bytes.NewReader(payloadBytes))
			resp.Body.Close()
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		cfg := &config.Config{
			Server: config.ServerConfig{Port: 8080},
			Destinations: []config.DestinationConfig{
				{
					Name:             "parallel",
					URL:              mockDest.URL,
					Method:           "POST",
					Format:           "json",
					Engine:           "go-template",
					Template:         `{"alert":"{{(index .Alerts 0).Labels.alertname}}"}`,
					Enabled:          true,
					SplitAlerts:      true,
					ParallelRequests: 10,
				},
			},
		}

		srv, _ := New(cfg, logrus.New())
		ts := httptest.NewServer(srv.GetRouter())
		defer ts.Close()

		payloadBytes, _ := json.Marshal(webhook)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, _ := http.Post(ts.URL+"/webhook/parallel", "application/json", bytes.NewReader(payloadBytes))
			resp.Body.Close()
		}
	})
}

// BenchmarkConcurrentWebhooks benchmarks handling concurrent webhook requests
func BenchmarkConcurrentWebhooks(b *testing.B) {
	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Simulate variable processing time
		time.Sleep(time.Microsecond * time.Duration(50+b.N%50))
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080},
		Destinations: []config.DestinationConfig{
			{
				Name:     "concurrent",
				URL:      mockDest.URL,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"id":"{{.GroupKey}}"}`,
				Enabled:  true,
			},
		},
	}

	srv, _ := New(cfg, logrus.New())
	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Prepare multiple different webhooks
	webhooks := make([][]byte, 10)
	for i := 0; i < 10; i++ {
		webhook := &alertmanager.WebhookPayload{
			Version:  "4",
			GroupKey: fmt.Sprintf("group-%d", i),
			Status:   "firing",
			Alerts:   []alertmanager.Alert{{Status: "firing", Fingerprint: fmt.Sprintf("fp-%d", i), StartsAt: time.Now()}},
		}
		webhooks[i], _ = json.Marshal(webhook)
	}

	b.ResetTimer()
	b.ReportAllocs()

	var wg sync.WaitGroup
	concurrency := 10

	for i := 0; i < b.N; i++ {
		wg.Add(concurrency)
		for j := 0; j < concurrency; j++ {
			go func(idx int) {
				defer wg.Done()
				payload := webhooks[idx%len(webhooks)]
				resp, err := http.Post(ts.URL+"/webhook/concurrent", "application/json", bytes.NewReader(payload))
				if err == nil {
					resp.Body.Close()
				}
			}(j)
		}
		wg.Wait()
	}
}
