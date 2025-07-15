package server

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
)

type MemoryStats struct {
	Alloc      uint64
	TotalAlloc uint64
	Sys        uint64
	NumGC      uint32
	Goroutines int
}

func getMemoryStats(_ *testing.T) MemoryStats {
	var m runtime.MemStats
	runtime.GC() // Force garbage collection
	runtime.ReadMemStats(&m)

	return MemoryStats{
		Alloc:      m.Alloc,
		TotalAlloc: m.TotalAlloc,
		Sys:        m.Sys,
		NumGC:      m.NumGC,
		Goroutines: runtime.NumGoroutine(),
	}
}

func createSimpleWebhook(alertCount int) *alertmanager.WebhookPayload {
	alerts := make([]alertmanager.Alert, alertCount)

	for i := 0; i < alertCount; i++ {
		alerts[i] = alertmanager.Alert{
			Status: "firing",
			Labels: map[string]string{
				"alertname": "TestAlert",
				"instance":  fmt.Sprintf("test-instance-%d", i),
				"job":       "test-job",
				"severity":  "warning",
			},
			Annotations: map[string]string{
				"summary":     fmt.Sprintf("Test alert %d", i),
				"description": "This is a test alert for performance testing",
			},
			StartsAt:     time.Now().Add(-5 * time.Minute),
			EndsAt:       time.Time{},
			GeneratorURL: "http://prometheus:9090/graph",
			Fingerprint:  fmt.Sprintf("test-fingerprint-%d", i),
		}
	}

	return &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "{}:{alertname=\"TestAlert\"}",
		Status:   "firing",
		Receiver: "test-receiver",
		GroupLabels: map[string]string{
			"alertname": "TestAlert",
		},
		CommonLabels: map[string]string{
			"alertname": "TestAlert",
			"job":       "test-job",
			"severity":  "warning",
		},
		CommonAnnotations: map[string]string{
			"summary": "Test alerts for performance testing",
		},
		ExternalURL: "http://alertmanager:9093",
		Alerts:      alerts,
	}
}