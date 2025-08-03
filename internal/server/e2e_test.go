package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
)

// TestE2ECompleteScenario tests a complete real-world scenario
func TestE2ECompleteScenario(t *testing.T) {
	// Create mock services
	var slackRequests, teamsRequests, splunkRequests []capturedRequest
	var mu sync.Mutex

	// Mock Slack webhook
	slackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		slackRequests = append(slackRequests, capturedRequest{
			Timestamp: time.Now(),
			Body:      body,
			Headers:   r.Header.Clone(),
		})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer slackServer.Close()

	// Mock Teams webhook with occasional failures
	teamsFailCount := 0
	teamsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		// Simulate intermittent failures
		if teamsFailCount < 1 {
			teamsFailCount++
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		mu.Lock()
		teamsRequests = append(teamsRequests, capturedRequest{
			Timestamp: time.Now(),
			Body:      body,
			Headers:   r.Header.Clone(),
		})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer teamsServer.Close()

	// Mock Splunk HEC
	splunkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		splunkRequests = append(splunkRequests, capturedRequest{
			Timestamp: time.Now(),
			Body:      body,
			Headers:   r.Header.Clone(),
		})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"text":"Success","code":0}`))
	}))
	defer splunkServer.Close()

	// Create configuration with multiple destinations
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
			Auth: config.AuthConfig{
				Enabled:     true,
				Username:    "alertmanager",
				Password:    "secure-webhook-pass",
				APIUsername: "api-user",
				APIPassword: "api-pass",
			},
		},
		Destinations: []config.DestinationConfig{
			{
				Name:   "slack-critical",
				URL:    slackServer.URL + "/services/webhook",
				Method: "POST",
				Format: "json",
				Engine: "go-template",
				Template: `{
					"text": ":rotating_light: *{{.Status | upper}}* - {{.GroupLabels.alertname}}",
					"attachments": [{
						"color": "{{if eq .Status "firing"}}danger{{else}}good{{end}}",
						"fields": [
							{{range $i, $alert := .Alerts}}
							{{if $i}},{{end}}{
								"title": "{{$alert.Labels.alertname}}",
								"value": "{{$alert.Labels.severity}} - {{$alert.Annotations.summary}}",
								"short": false
							}
							{{end}}
						],
						"footer": "AlertManager Gateway",
						"ts": {{now | unixtime}}
					}]
				}`,
				Enabled:     true,
				SplitAlerts: false,
				Headers: map[string]string{
					"X-Slack-Channel": "#alerts-critical",
				},
			},
			{
				Name:   "teams-all",
				URL:    teamsServer.URL + "/webhook",
				Method: "POST",
				Format: "json",
				Engine: "jq",
				Transform: `{
					"@type": "MessageCard",
					"@context": "https://schema.org/extensions",
					"summary": .groupLabels.alertname + " " + .status,
					"themeColor": (if .status == "firing" then "FF0000" else "00FF00" end),
					"sections": [{
						"activityTitle": .groupLabels.alertname,
						"activitySubtitle": "Status: " + .status,
						"facts": [
							.alerts[] | {
								"name": .labels.alertname + " (" + .labels.severity + ")",
								"value": .annotations.summary // "No summary"
							}
						]
					}]
				}`,
				Enabled: true,
				Retry: config.RetryConfig{
					MaxAttempts: 3,
					Backoff:     "exponential",
				},
			},
			{
				Name:   "splunk-metrics",
				URL:    splunkServer.URL + "/services/collector/event",
				Method: "POST",
				Format: "json",
				Engine: "jq",
				Transform: `{
					time: now,
					source: "alertmanager",
					sourcetype: "alert",
					event: {
						alertname: .labels.alertname,
						severity: .labels.severity,
						status: .status,
						fingerprint: .fingerprint,
						labels: .labels,
						annotations: .annotations
					}
				}`,
				Enabled:     true,
				SplitAlerts: true,
				Headers: map[string]string{
					"Authorization": "Splunk hec-token-12345",
				},
			},
		},
	}

	// Create server
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	srv, err := New(cfg, logger)
	require.NoError(t, err)

	// Start test server
	ts := httptest.NewServer(srv.GetRouter())
	defer ts.Close()

	// Allow server to initialize
	time.Sleep(100 * time.Millisecond)

	// Test scenario: Critical production alerts
	criticalAlerts := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "prod-critical-2024",
		Status:   "firing",
		Receiver: "critical-receiver",
		GroupLabels: map[string]string{
			"alertname": "ProductionCritical",
			"env":       "production",
		},
		Alerts: []alertmanager.Alert{
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "HighErrorRate",
					"severity":  "critical",
					"service":   "api-gateway",
				},
				Annotations: map[string]string{
					"summary":     "Error rate is above 5% for 5 minutes",
					"description": "The API gateway is experiencing high error rates",
				},
				Fingerprint: "error-rate-12345",
				StartsAt:    time.Now().Add(-10 * time.Minute),
			},
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "ServiceDown",
					"severity":  "critical",
					"service":   "payment-processor",
				},
				Annotations: map[string]string{
					"summary":     "Payment processor is unreachable",
					"description": "Cannot connect to payment processing service",
				},
				Fingerprint: "service-down-67890",
				StartsAt:    time.Now().Add(-2 * time.Minute),
			},
		},
	}

	// Send alerts to each destination
	for _, dest := range []string{"slack-critical", "teams-all", "splunk-metrics"} {
		payloadBytes, err := json.Marshal(criticalAlerts)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", ts.URL+"/webhook/"+dest, bytes.NewReader(payloadBytes))
		require.NoError(t, err)
		req.SetBasicAuth("alertmanager", "secure-webhook-pass")
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		// Accept both 200 (success) and 500 (destination failure) as valid responses
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, resp.StatusCode)
	}

	// Wait for async processing and retries with timeout
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	expectedRequests := 3 // slack, teams, splunk
	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for requests to complete")
		case <-ticker.C:
			mu.Lock()
			totalRequests := len(slackRequests) + len(teamsRequests) + len(splunkRequests)
			mu.Unlock()
			if totalRequests >= expectedRequests {
				goto verifyResults
			}
		}
	}

verifyResults:
	// Verify Slack received the alert
	mu.Lock()
	assert.GreaterOrEqual(t, len(slackRequests), 1)
	if len(slackRequests) > 0 {
		var slackPayload map[string]interface{}
		err = json.Unmarshal(slackRequests[0].Body, &slackPayload)
		require.NoError(t, err)

		assert.Contains(t, slackPayload["text"], "FIRING")
		assert.Contains(t, slackPayload["text"], "ProductionCritical")

		attachments := slackPayload["attachments"].([]interface{})
		assert.Len(t, attachments, 1)

		attachment := attachments[0].(map[string]interface{})
		assert.Equal(t, "danger", attachment["color"])

		fields := attachment["fields"].([]interface{})
		assert.Len(t, fields, 2) // Two alerts
	}

	// Verify Teams received the alert (after retry)
	assert.Len(t, teamsRequests, 1)
	if len(teamsRequests) > 0 {
		var teamsPayload map[string]interface{}
		err = json.Unmarshal(teamsRequests[0].Body, &teamsPayload)
		require.NoError(t, err)

		assert.Equal(t, "MessageCard", teamsPayload["@type"])
		assert.Equal(t, "FF0000", teamsPayload["themeColor"]) // Red for firing
		assert.Contains(t, teamsPayload["summary"], "ProductionCritical")
	}

	// Verify Splunk received individual events (split alerts)
	assert.Len(t, splunkRequests, 2) // Two alerts sent separately
	for i, req := range splunkRequests {
		var splunkEvent map[string]interface{}
		err = json.Unmarshal(req.Body, &splunkEvent)
		require.NoError(t, err)

		assert.Equal(t, "alertmanager", splunkEvent["source"])
		assert.Equal(t, "alert", splunkEvent["sourcetype"])

		event := splunkEvent["event"].(map[string]interface{})
		assert.NotEmpty(t, event["alertname"])
		assert.Equal(t, "critical", event["severity"])
		assert.Equal(t, "firing", event["status"])

		// Verify auth header
		assert.Equal(t, "Splunk hec-token-12345", req.Headers.Get("Authorization"))

		t.Logf("Splunk event %d: %+v", i, event)
	}
	mu.Unlock()

	// Test recovery scenario
	recoveryAlerts := *criticalAlerts
	recoveryAlerts.Status = "resolved"
	for i := range recoveryAlerts.Alerts {
		recoveryAlerts.Alerts[i].Status = "resolved"
		recoveryAlerts.Alerts[i].EndsAt = time.Now()
	}

	// Send recovery
	payloadBytes, err := json.Marshal(&recoveryAlerts)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", ts.URL+"/webhook/slack-critical", bytes.NewReader(payloadBytes))
	require.NoError(t, err)
	req.SetBasicAuth("alertmanager", "secure-webhook-pass")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	time.Sleep(100 * time.Millisecond)

	// Verify recovery notification
	mu.Lock()
	assert.Len(t, slackRequests, 2) // Original + recovery
	if len(slackRequests) > 1 {
		var recoveryPayload map[string]interface{}
		err = json.Unmarshal(slackRequests[1].Body, &recoveryPayload)
		require.NoError(t, err)

		assert.Contains(t, recoveryPayload["text"], "RESOLVED")

		attachments := recoveryPayload["attachments"].([]interface{})
		attachment := attachments[0].(map[string]interface{})
		assert.Equal(t, "good", attachment["color"]) // Green for resolved
	}
	mu.Unlock()
}

// TestE2EHighLoadScenario tests system under high load
func TestE2EHighLoadScenario(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high load test in short mode")
	}

	var totalRequests int32
	var successfulRequests int32

	// Mock destination that simulates variable latency
	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&totalRequests, 1)

		// Simulate variable processing time
		delay := time.Duration(10+atomic.LoadInt32(&totalRequests)%20) * time.Millisecond
		time.Sleep(delay)

		// Simulate 95% success rate
		if atomic.LoadInt32(&totalRequests)%20 == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		atomic.AddInt32(&successfulRequests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	// Configuration with parallel processing
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Destinations: []config.DestinationConfig{
			{
				Name:             "high-load-dest",
				URL:              mockDest.URL,
				Method:           "POST",
				Format:           "json",
				Engine:           "jq",
				Transform:        `{alert: .labels.alertname, time: now}`,
				Enabled:          true,
				SplitAlerts:      true,
				ParallelRequests: 5, // Process 5 alerts in parallel
				Retry: config.RetryConfig{
					MaxAttempts: 2,
					Backoff:     "constant",
				},
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

	// Generate high load - 100 webhooks with 10 alerts each
	numWebhooks := 100
	alertsPerWebhook := 10

	start := time.Now()
	var wg sync.WaitGroup

	for i := 0; i < numWebhooks; i++ {
		wg.Add(1)
		go func(webhookNum int) {
			defer wg.Done()

			// Create webhook with multiple alerts
			alerts := make([]alertmanager.Alert, alertsPerWebhook)
			for j := 0; j < alertsPerWebhook; j++ {
				alerts[j] = alertmanager.Alert{
					Status: "firing",
					Labels: map[string]string{
						"alertname": fmt.Sprintf("Alert-%d-%d", webhookNum, j),
						"severity":  "warning",
					},
					Fingerprint: fmt.Sprintf("fp-%d-%d", webhookNum, j),
					StartsAt:    time.Now(),
				}
			}

			webhook := &alertmanager.WebhookPayload{
				Version:  "4",
				GroupKey: fmt.Sprintf("high-load-group-%d", webhookNum),
				Status:   "firing",
				Alerts:   alerts,
			}

			payloadBytes, _ := json.Marshal(webhook)

			resp, err := http.Post(
				ts.URL+"/webhook/high-load-dest",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			if err == nil {
				resp.Body.Close()
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	// Wait for all async processing to complete
	time.Sleep(2 * time.Second)

	// Verify results
	totalExpected := int32(numWebhooks * alertsPerWebhook)
	actualTotal := atomic.LoadInt32(&totalRequests)
	actualSuccess := atomic.LoadInt32(&successfulRequests)

	t.Logf("High load test completed in %v", duration)
	t.Logf("Total requests: %d (including retries)", actualTotal)
	t.Logf("Successful requests: %d", actualSuccess)
	t.Logf("Success rate: %.2f%%", float64(actualSuccess)/float64(totalExpected)*100)

	// With retries, we should have high success rate
	assert.Greater(t, float64(actualSuccess)/float64(totalExpected), 0.90)

	// Processing should be reasonably fast
	assert.Less(t, duration, 5*time.Second)
}

// TestE2EAPIIntegration tests API endpoints in conjunction with webhooks
func TestE2EAPIIntegration(t *testing.T) {
	var destinationCalls sync.Map

	// Mock destination that tracks calls
	mockDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destName := r.Header.Get("X-Destination-Name")
		if destName != "" {
			count, _ := destinationCalls.LoadOrStore(destName, 0)
			destinationCalls.Store(destName, count.(int)+1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mockDest.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
			Auth: config.AuthConfig{
				Enabled:     true,
				Username:    "webhook-user",
				Password:    "webhook-pass",
				APIUsername: "api-admin",
				APIPassword: "api-secret",
			},
		},
		Destinations: []config.DestinationConfig{
			{
				Name:     "prod-alerts",
				URL:      mockDest.URL + "/prod",
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"env":"prod","alert":"{{.Status}}"}`,
				Enabled:  true,
				Headers: map[string]string{
					"X-Destination-Name": "prod-alerts",
				},
			},
			{
				Name:     "staging-alerts",
				URL:      mockDest.URL + "/staging",
				Method:   "POST",
				Format:   "json",
				Engine:   "go-template",
				Template: `{"env":"staging","alert":"{{.Status}}"}`,
				Enabled:  false, // Disabled by default
				Headers: map[string]string{
					"X-Destination-Name": "staging-alerts",
				},
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

	// Create API client
	apiClient := &http.Client{}

	// 1. List destinations via API
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/destinations?include_disabled=true", nil)
	req.SetBasicAuth("api-admin", "api-secret")

	resp, err := apiClient.Do(req)
	require.NoError(t, err)

	var destList struct {
		Total        int `json:"total"`
		Destinations []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"destinations"`
	}

	err = json.NewDecoder(resp.Body).Decode(&destList)
	resp.Body.Close()
	require.NoError(t, err)

	assert.Equal(t, 2, destList.Total)
	assert.Equal(t, "prod-alerts", destList.Destinations[0].Name)
	assert.True(t, destList.Destinations[0].Enabled)
	assert.Equal(t, "staging-alerts", destList.Destinations[1].Name)
	assert.False(t, destList.Destinations[1].Enabled)

	// 2. Test destination transformation
	testReq, _ := http.NewRequest("POST", ts.URL+"/api/v1/test/prod-alerts", bytes.NewBufferString("{}"))
	testReq.SetBasicAuth("api-admin", "api-secret")
	testReq.Header.Set("Content-Type", "application/json")

	resp, err = apiClient.Do(testReq)
	require.NoError(t, err)

	var testResult struct {
		Success bool `json:"success"`
		Result  struct {
			FormattedOutput string `json:"formatted_output"`
		} `json:"result"`
	}

	err = json.NewDecoder(resp.Body).Decode(&testResult)
	resp.Body.Close()
	require.NoError(t, err)

	assert.True(t, testResult.Success)
	assert.Contains(t, testResult.Result.FormattedOutput, `"env":"prod"`)

	// 3. Send webhook to enabled destination
	webhook := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "api-test-group",
		Status:   "firing",
		Alerts: []alertmanager.Alert{{
			Status:      "firing",
			Fingerprint: "test-fingerprint",
			StartsAt:    time.Now(),
		}},
	}

	webhookBytes, _ := json.Marshal(webhook)

	webhookReq, _ := http.NewRequest("POST", ts.URL+"/webhook/prod-alerts", bytes.NewReader(webhookBytes))
	webhookReq.SetBasicAuth("webhook-user", "webhook-pass")
	webhookReq.Header.Set("Content-Type", "application/json")

	resp, err = apiClient.Do(webhookReq)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 4. Try to send to disabled destination (should fail)
	webhookReq, _ = http.NewRequest("POST", ts.URL+"/webhook/staging-alerts", bytes.NewReader(webhookBytes))
	webhookReq.SetBasicAuth("webhook-user", "webhook-pass")
	webhookReq.Header.Set("Content-Type", "application/json")

	resp, err = apiClient.Do(webhookReq)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Wait for async processing
	time.Sleep(100 * time.Millisecond)

	// 5. Check system info
	infoReq, _ := http.NewRequest("GET", ts.URL+"/api/v1/info", nil)
	infoReq.SetBasicAuth("api-admin", "api-secret")

	resp, err = apiClient.Do(infoReq)
	require.NoError(t, err)

	var sysInfo map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&sysInfo)
	resp.Body.Close()
	require.NoError(t, err)

	assert.Equal(t, "1.0.0", sysInfo["version"])
	if numGoroutines, ok := sysInfo["num_goroutines"]; ok && numGoroutines != nil {
		assert.Greater(t, numGoroutines.(float64), float64(1))
	}

	// Verify destination calls
	prodCount, _ := destinationCalls.Load("prod-alerts")
	stagingCount, _ := destinationCalls.LoadOrStore("staging-alerts", 0)

	assert.Equal(t, 1, prodCount.(int))
	assert.Equal(t, 0, stagingCount.(int)) // Disabled destination shouldn't receive
}

type capturedRequest struct {
	Timestamp time.Time
	Body      []byte
	Headers   http.Header
}
