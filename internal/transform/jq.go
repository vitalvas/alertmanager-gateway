package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/itchyny/gojq"
	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
)

// JQEngine implements the jq transformation engine
type JQEngine struct {
	query         string
	compiledQuery *gojq.Code
	mu            sync.RWMutex
}

// NewJQEngine creates a new jq transformation engine
func NewJQEngine(query string) (*JQEngine, error) {
	if query == "" {
		return nil, fmt.Errorf("jq query cannot be empty")
	}

	// Compile the jq query
	q, err := gojq.Parse(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse jq query: %w", err)
	}

	// Compile the query for better performance
	compiledQuery, err := gojq.Compile(q)
	if err != nil {
		return nil, fmt.Errorf("failed to compile jq query: %w", err)
	}

	return &JQEngine{
		query:         query,
		compiledQuery: compiledQuery,
	}, nil
}

// Transform applies the jq transformation to the webhook payload
func (j *JQEngine) Transform(payload *alertmanager.WebhookPayload) (interface{}, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	// Convert payload to interface{} for jq processing
	var data interface{}
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload for jq: %w", err)
	}

	// Execute the jq query with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := j.executeQuery(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("jq transformation failed: %w", err)
	}

	return result, nil
}

// TransformAlert transforms a single alert using jq
func (j *JQEngine) TransformAlert(alert *alertmanager.Alert, payload *alertmanager.WebhookPayload) (interface{}, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	// Create context with both alert and payload data
	contextData := map[string]interface{}{
		"alert":   alert,
		"payload": payload,
	}

	// Convert to interface{} for jq processing
	var data interface{}
	jsonBytes, err := json.Marshal(contextData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal alert context: %w", err)
	}

	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal alert context for jq: %w", err)
	}

	// Execute the jq query with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := j.executeQuery(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("jq alert transformation failed: %w", err)
	}

	return result, nil
}

// executeQuery executes the compiled jq query with context and timeout
func (j *JQEngine) executeQuery(ctx context.Context, data interface{}) (interface{}, error) {
	// Create a channel to receive the result
	resultChan := make(chan interface{}, 1)
	errorChan := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				errorChan <- fmt.Errorf("jq query panicked: %v", r)
			}
		}()

		// Run the jq query
		iter := j.compiledQuery.Run(data)
		results := make([]interface{}, 0)

		for {
			v, ok := iter.Next()
			if !ok {
				break
			}
			if err, ok := v.(error); ok {
				errorChan <- err
				return
			}
			results = append(results, v)
		}

		// Return single result if only one, array if multiple
		if len(results) == 1 {
			resultChan <- results[0]
		} else {
			resultChan <- results
		}
	}()

	// Wait for result or timeout
	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errorChan:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("jq query timed out")
	}
}

// Validate checks if the jq query is valid
func (j *JQEngine) Validate() error {
	j.mu.RLock()
	defer j.mu.RUnlock()

	if j.compiledQuery == nil {
		return fmt.Errorf("jq query is not compiled")
	}

	// Test with sample data
	sampleData := map[string]interface{}{
		"version":  "4",
		"groupKey": "test",
		"status":   "firing",
		"receiver": "test",
		"groupLabels": map[string]interface{}{
			"alertname": "TestAlert",
		},
		"commonLabels": map[string]interface{}{
			"alertname": "TestAlert",
			"severity":  "warning",
		},
		"commonAnnotations": map[string]interface{}{
			"summary": "Test alert",
		},
		"externalURL": "http://alertmanager.example.com",
		"alerts": []interface{}{
			map[string]interface{}{
				"status":      "firing",
				"fingerprint": "test123",
				"labels": map[string]interface{}{
					"alertname": "TestAlert",
					"severity":  "warning",
				},
				"annotations": map[string]interface{}{
					"summary": "Test alert",
				},
				"startsAt": "2023-01-01T00:00:00Z",
				"endsAt":   "0001-01-01T00:00:00Z",
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := j.executeQuery(ctx, sampleData)
	return err
}

// Name returns the engine name
func (j *JQEngine) Name() string {
	return "jq"
}

// GetQuery returns the jq query string
func (j *JQEngine) GetQuery() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.query
}
