package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/transform"
)

// TestConcurrentWebhookProcessing tests concurrent webhook processing for race conditions
func TestConcurrentWebhookProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race condition test in short mode")
	}

	// Create a destination that tracks requests
	var requestCount int32
	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		// Simulate variable processing time
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(10)))
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "race-test",
				URL:      mockDest.URL,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"group":"{{.GroupKey}}","status":"{{.Status}}"}`,
				Enabled:  true,
			},
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, err := New(cfg, logger)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Allow server to initialize
	time.Sleep(100 * time.Millisecond)

	// Run concurrent webhook requests
	numGoroutines := 50
	requestsPerGoroutine := 10
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				webhook := &alertmanager.WebhookPayload{
					Version:  "4",
					GroupKey: fmt.Sprintf("group-%d-%d", routineID, j),
					Status:   "firing",
					Alerts: []alertmanager.Alert{
						{
							Status:      "firing",
							Fingerprint: fmt.Sprintf("fp-%d-%d", routineID, j),
							Labels: map[string]string{
								"alertname": fmt.Sprintf("Alert-%d-%d", routineID, j),
							},
							StartsAt: time.Now(),
						},
					},
				}

				payloadBytes, _ := json.Marshal(webhook)

				resp, err := http.Post(
					ts.URL+"/webhook/race-test",
					"application/json",
					bytes.NewReader(payloadBytes),
				)

				if err == nil {
					resp.Body.Close()
				}
			}
		}(i)
	}

	wg.Wait()

	// Wait for async processing to complete with timeout
	expectedRequests := int32(numGoroutines * requestsPerGoroutine)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			actualRequests := atomic.LoadInt32(&requestCount)
			t.Fatalf("Timeout waiting for requests to complete. Expected: %d, Got: %d", expectedRequests, actualRequests)
		case <-ticker.C:
			actualRequests := atomic.LoadInt32(&requestCount)
			if actualRequests >= expectedRequests {
				assert.Equal(t, expectedRequests, actualRequests, "All requests should be processed")
				return
			}
		}
	}
}

// TestConcurrentTemplateEngineAccess tests concurrent access to template engine
func TestConcurrentTemplateEngineAccess(t *testing.T) {
	// Template engine is created per template, so we'll test concurrent template creation/execution

	templates := []string{
		`{"alert":"{{.Status}}","count":{{len .Alerts}}}`,
		`[{{range $i,$a := .Alerts}}{{if $i}},{{end}}"{{$a.Labels.alertname}}"{{end}}]`,
		`{"timestamp":{{now | unixtime}},"status":"{{.Status | upper}}"}`,
	}

	webhooks := make([]*alertmanager.WebhookPayload, 5)
	for i := 0; i < 5; i++ {
		webhooks[i] = &alertmanager.WebhookPayload{
			Status: "firing",
			Alerts: []alertmanager.Alert{
				{
					Status:      "firing",
					Fingerprint: fmt.Sprintf("template-fp-%d", i),
					Labels: map[string]string{
						"alertname": fmt.Sprintf("Alert%d", i),
						"severity":  "warning",
					},
					StartsAt: time.Now(),
				},
			},
		}
	}

	// Run concurrent transformations
	numGoroutines := 20
	transformsPerGoroutine := 50
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*transformsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()

			for j := 0; j < transformsPerGoroutine; j++ {
				template := templates[j%len(templates)]
				webhook := webhooks[j%len(webhooks)]

				// Create new engine for each transformation (as done in real usage)
				engine, err := transform.NewGoTemplateEngine(template)
				if err != nil {
					errors <- err
					continue
				}

				result, err := engine.Transform(webhook)
				if err != nil {
					errors <- err
				} else if result == "" {
					errors <- fmt.Errorf("empty transformation result")
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Transformation error: %v", err)
	}
}

// TestConcurrentJQEngineAccess tests concurrent access to JQ engine
func TestConcurrentJQEngineAccess(t *testing.T) {
	// JQ engine is created per query, so we'll test concurrent query execution

	queries := []string{
		`.status`,
		`.alerts | length`,
		`.alerts | map(.labels.alertname)`,
		`{status: .status, count: (.alerts | length)}`,
	}

	webhooks := make([]*alertmanager.WebhookPayload, 3)
	for i := 0; i < 3; i++ {
		webhooks[i] = &alertmanager.WebhookPayload{
			Status: "firing",
			Alerts: []alertmanager.Alert{
				{
					Status:      "firing",
					Fingerprint: fmt.Sprintf("jq-fp-%d", i),
					Labels: map[string]string{
						"alertname": fmt.Sprintf("Alert%d", i),
					},
					StartsAt: time.Now(),
				},
			},
		}
	}

	// Run concurrent transformations
	numGoroutines := 15
	transformsPerGoroutine := 40
	var wg sync.WaitGroup
	var successCount int32

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < transformsPerGoroutine; j++ {
				query := queries[j%len(queries)]
				webhook := webhooks[j%len(webhooks)]

				// Create new engine for each query
				engine, err := transform.NewJQEngine(query)
				if err != nil {
					continue
				}

				_, err = engine.Transform(webhook)
				if err == nil {
					atomic.AddInt32(&successCount, 1)
				}
			}
		}()
	}

	wg.Wait()

	expectedCount := int32(numGoroutines * transformsPerGoroutine)
	assert.Equal(t, expectedCount, successCount, "All transformations should succeed")
}

// TestConcurrentDestinationAccess tests concurrent access to the same destination
func TestConcurrentDestinationAccess(t *testing.T) {
	var mu sync.Mutex
	requestBodies := make([][]byte, 0)

	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		mu.Lock()
		requestBodies = append(requestBodies, body)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080},
		Destinations: []config.DestinationConfig{
			{
				Name:        "concurrent-dest",
				URL:         mockDest.URL,
				Method:      "POST",
				Format:      "json",
				Engine:      "go-template",
				Template:    `{"id":"{{.GroupKey}}","alert":"{{.Alert.Labels.alertname}}"}`,
				Enabled:     true,
				SplitAlerts: true,
			},
		},
	}

	srv, err := New(cfg, logrus.New())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Allow server to initialize
	time.Sleep(100 * time.Millisecond)

	// Send multiple webhooks concurrently
	numWebhooks := 10
	alertsPerWebhook := 5
	var wg sync.WaitGroup

	for i := 0; i < numWebhooks; i++ {
		wg.Add(1)
		go func(webhookID int) {
			defer wg.Done()

			alerts := make([]alertmanager.Alert, alertsPerWebhook)
			for j := 0; j < alertsPerWebhook; j++ {
				alerts[j] = alertmanager.Alert{
					Status: "firing",
					Labels: map[string]string{
						"alertname": fmt.Sprintf("Alert-%d-%d", webhookID, j),
					},
					Fingerprint: fmt.Sprintf("fp-%d-%d", webhookID, j),
					StartsAt:    time.Now(),
				}
			}

			webhook := &alertmanager.WebhookPayload{
				Version:  "4",
				GroupKey: fmt.Sprintf("group-%d", webhookID),
				Status:   "firing",
				Alerts:   alerts,
			}

			payloadBytes, _ := json.Marshal(webhook)

			resp, err := http.Post(
				ts.URL+"/webhook/concurrent-dest",
				"application/json",
				bytes.NewReader(payloadBytes),
			)

			if err == nil {
				resp.Body.Close()
			}
		}(i)
	}

	wg.Wait()

	// Wait for all async processing to complete with timeout
	expectedRequests := numWebhooks * alertsPerWebhook
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var actualRequests int
	for {
		select {
		case <-timeout:
			mu.Lock()
			actualRequests = len(requestBodies)
			mu.Unlock()
			t.Fatalf("Timeout waiting for async processing to complete. Expected: %d, Got: %d", expectedRequests, actualRequests)
		case <-ticker.C:
			mu.Lock()
			actualRequests = len(requestBodies)
			mu.Unlock()

			if actualRequests >= expectedRequests {
				goto verifyData
			}
		}
	}

verifyData:
	// Verify all alerts were sent (split mode)
	mu.Lock()
	actualRequests = len(requestBodies)
	mu.Unlock()

	assert.Equal(t, expectedRequests, actualRequests, "All split alerts should be sent")

	// Verify no data corruption
	alertNames := make(map[string]bool)
	mu.Lock()
	for _, body := range requestBodies {
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err == nil {
			if alert, ok := data["alert"].(string); ok {
				alertNames[alert] = true
			}
		}
	}
	mu.Unlock()

	// Should have unique alert names
	assert.Equal(t, expectedRequests, len(alertNames), "All alerts should have unique names")
}

// TestConcurrentConfigAccess tests concurrent access to configuration
func TestConcurrentConfigAccess(_ *testing.T) {
	// Create initial config
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080},
		Destinations: []config.DestinationConfig{
			{
				Name:     "test-dest",
				URL:      "http://example.com",
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: "{}",
				Enabled:  true,
			},
		},
	}

	var wg sync.WaitGroup
	numReaders := 20
	numAccesses := 100

	// Concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < numAccesses; j++ {
				// Access various config fields
				_ = cfg.Server.Port
				_ = cfg.Destinations[0].Name
				_ = cfg.Destinations[0].Template
				dest := cfg.GetDestinationByName("test-dest")
				if dest != nil {
					_ = dest.URL
				}
			}
		}()
	}

	wg.Wait()
}

// TestConcurrentMetricsAccess tests concurrent access to metrics
func TestConcurrentMetricsAccess(t *testing.T) {
	// This test would require metrics to be enabled
	// For now, we'll test concurrent HTTP requests that would update metrics

	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080},
		Destinations: []config.DestinationConfig{
			{
				Name:     "metrics-test",
				URL:      mockDest.URL,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: "{}",
				Enabled:  true,
			},
		},
	}

	srv, err := New(cfg, logrus.New())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Allow server to initialize
	time.Sleep(100 * time.Millisecond)

	// Concurrent requests to different endpoints
	endpoints := []string{
		"/health",
		"/health/live",
		"/health/ready",
		"/api/v1/destinations",
		"/api/v1/info",
	}

	numGoroutines := 10
	requestsPerEndpoint := 20
	var wg sync.WaitGroup

	for _, endpoint := range endpoints {
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(ep string) {
				defer wg.Done()

				for j := 0; j < requestsPerEndpoint; j++ {
					resp, err := http.Get(ts.URL + ep)
					if err == nil {
						resp.Body.Close()
					}
				}
			}(endpoint)
		}
	}

	// Also send webhook requests
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			webhook := &alertmanager.WebhookPayload{
				Version:  "4",
				GroupKey: fmt.Sprintf("metrics-%d", id),
				Status:   "firing",
				Alerts:   []alertmanager.Alert{{Status: "firing", Fingerprint: fmt.Sprintf("m-%d", id), StartsAt: time.Now()}},
			}

			payloadBytes, _ := json.Marshal(webhook)

			for j := 0; j < requestsPerEndpoint; j++ {
				resp, err := http.Post(
					ts.URL+"/webhook/metrics-test",
					"application/json",
					bytes.NewReader(payloadBytes),
				)
				if err == nil {
					resp.Body.Close()
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestAuthRateLimiterConcurrency tests concurrent access to auth rate limiter
func TestAuthRateLimiterConcurrency(t *testing.T) {
	// Create a mock destination server
	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
			Auth: config.AuthConfig{
				Enabled:  true,
				Username: "test-user",
				Password: "test-pass",
			},
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "auth-test",
				URL:      mockDest.URL,
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: "{}",
				Enabled:  true,
			},
		},
	}

	srv, err := New(cfg, logrus.New())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Allow server to initialize
	time.Sleep(100 * time.Millisecond)

	webhook := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "auth-test",
		Status:   "firing",
		Alerts:   []alertmanager.Alert{{Status: "firing", Fingerprint: "auth-123", StartsAt: time.Now()}},
	}
	payloadBytes, _ := json.Marshal(webhook)

	// Concurrent auth attempts with different IPs
	numGoroutines := 20
	attemptsPerGoroutine := 10
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()

			client := &http.Client{}

			for j := 0; j < attemptsPerGoroutine; j++ {
				req, _ := http.NewRequest("POST", ts.URL+"/webhook/auth-test", bytes.NewReader(payloadBytes))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.%d", routineID))

				// Mix of valid and invalid credentials
				if j%3 == 0 {
					req.SetBasicAuth("wrong-user", "wrong-pass")
				} else {
					req.SetBasicAuth("test-user", "test-pass")
				}

				resp, err := client.Do(req)
				if err == nil {
					resp.Body.Close()
				}
			}
		}(i)
	}

	wg.Wait()
}
