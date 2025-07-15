package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"sync"
	"syscall"
	"time"

	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
)

var (
	gatewayURL   = flag.String("gateway", "http://localhost:8080", "Gateway URL")
	destination  = flag.String("destination", "test", "Destination name")
	duration     = flag.Duration("duration", 60*time.Second, "Test duration")
	concurrency  = flag.Int("concurrency", 10, "Number of concurrent clients")
	alertsPerReq = flag.Int("alerts", 10, "Alerts per request")
	interval     = flag.Duration("interval", 100*time.Millisecond, "Request interval")
	cpuprofile   = flag.String("cpuprofile", "", "Write CPU profile to file")
	memprofile   = flag.String("memprofile", "", "Write memory profile to file")
	pprofAddr    = flag.String("pprof", ":6060", "pprof server address")
)

func main() {
	flag.Parse()

	// Start pprof server
	go func() {
		log.Printf("Starting pprof server on %s", *pprofAddr)
		log.Println(http.ListenAndServe(*pprofAddr, nil))
	}()

	// CPU profiling
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			_ = f.Close()
			log.Fatal("could not start CPU profile: ", err)
		}
		defer f.Close()
		defer pprof.StopCPUProfile()
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create done channel
	done := make(chan struct{})

	// Stats tracking
	var (
		totalRequests   int64
		successRequests int64
		failedRequests  int64
		totalDuration   time.Duration
		mu              sync.Mutex
	)

	// Print initial memory stats
	printMemStats("Initial")

	// Start load generation
	log.Printf("Starting load test: %d concurrent clients, %d alerts per request", *concurrency, *alertsPerReq)
	startTime := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			client := &http.Client{
				Timeout: 10 * time.Second,
			}

			ticker := time.NewTicker(*interval)
			defer ticker.Stop()

			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					reqStart := time.Now()
					err := sendAlert(client, clientID)
					reqDuration := time.Since(reqStart)

					mu.Lock()
					totalRequests++
					totalDuration += reqDuration
					if err != nil {
						failedRequests++
						log.Printf("Client %d: Request failed: %v", clientID, err)
					} else {
						successRequests++
					}
					mu.Unlock()

					if time.Since(startTime) > *duration {
						return
					}
				}
			}
		}(i)
	}

	// Wait for duration or interrupt
	select {
	case <-time.After(*duration):
		log.Println("Test duration reached")
	case <-sigChan:
		log.Println("Received interrupt signal")
	}

	close(done)
	wg.Wait()

	// Print final stats
	elapsed := time.Since(startTime)
	mu.Lock()
	fmt.Printf("\n=== Load Test Results ===\n")
	fmt.Printf("Duration: %v\n", elapsed)
	fmt.Printf("Total Requests: %d\n", totalRequests)
	fmt.Printf("Successful: %d (%.2f%%)\n", successRequests, float64(successRequests)/float64(totalRequests)*100)
	fmt.Printf("Failed: %d (%.2f%%)\n", failedRequests, float64(failedRequests)/float64(totalRequests)*100)
	fmt.Printf("Requests/sec: %.2f\n", float64(totalRequests)/elapsed.Seconds())
	if totalRequests > 0 {
		fmt.Printf("Avg Response Time: %v\n", totalDuration/time.Duration(totalRequests))
	}
	mu.Unlock()

	// Print final memory stats
	printMemStats("Final")

	// Memory profiling
	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Printf("could not create memory profile: %v", err)
			return
		}
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			_ = f.Close()
			log.Printf("could not write memory profile: %v", err)
			return
		}
		_ = f.Close()
		log.Printf("Memory profile written to %s", *memprofile)
	}
}

func sendAlert(client *http.Client, clientID int) error {
	// Create test webhook payload
	alerts := make([]alertmanager.Alert, *alertsPerReq)
	for i := 0; i < *alertsPerReq; i++ {
		alerts[i] = alertmanager.Alert{
			Status: "firing",
			Labels: map[string]string{
				"alertname": fmt.Sprintf("TestAlert%d", i),
				"severity":  "warning",
				"instance":  fmt.Sprintf("test-instance-%d", clientID),
			},
			Annotations: map[string]string{
				"summary":     fmt.Sprintf("Test alert %d from client %d", i, clientID),
				"description": "This is a performance test alert",
			},
			StartsAt:    time.Now(),
			Fingerprint: fmt.Sprintf("test-%d-%d-%d", clientID, i, time.Now().UnixNano()),
		}
	}

	webhook := &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: fmt.Sprintf("test-group-%d", clientID),
		Status:   "firing",
		GroupLabels: map[string]string{
			"alertname": "TestAlert",
		},
		CommonLabels: map[string]string{
			"severity": "warning",
		},
		CommonAnnotations: map[string]string{
			"summary": "Performance test alerts",
		},
		Alerts: alerts,
	}

	payload, err := json.Marshal(webhook)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/webhook/%s", *gatewayURL, *destination)
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func printMemStats(label string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("\n=== %s Memory Stats ===\n", label)
	fmt.Printf("Alloc: %.2f MB\n", float64(m.Alloc)/1024/1024)
	fmt.Printf("TotalAlloc: %.2f MB\n", float64(m.TotalAlloc)/1024/1024)
	fmt.Printf("Sys: %.2f MB\n", float64(m.Sys)/1024/1024)
	fmt.Printf("NumGC: %d\n", m.NumGC)
	fmt.Printf("Goroutines: %d\n", runtime.NumGoroutine())
}
