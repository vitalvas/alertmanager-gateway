package alertmanager

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	// MaxPayloadSize is the maximum allowed size for a webhook payload (10MB)
	MaxPayloadSize = 10 * 1024 * 1024
)

// ParseWebhookPayload parses the incoming webhook request into a WebhookPayload
func ParseWebhookPayload(r *http.Request) (*WebhookPayload, error) {
	// Note: We don't strictly enforce Content-Type as Alertmanager
	// might not always set it correctly

	// Limit the request body size
	r.Body = http.MaxBytesReader(nil, r.Body, MaxPayloadSize)

	// Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if err.Error() == "http: request body too large" {
			return nil, ErrPayloadTooLarge
		}
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	// Parse JSON
	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	// Validate the payload
	if err := payload.IsValid(); err != nil {
		return nil, err
	}

	return &payload, nil
}

// MarshalJSON marshals the webhook payload to JSON
func (w *WebhookPayload) MarshalJSON() ([]byte, error) {
	// Use the standard JSON marshaler
	type Alias WebhookPayload
	return json.Marshal((*Alias)(w))
}

// UnmarshalJSON unmarshals JSON into a webhook payload
func (w *WebhookPayload) UnmarshalJSON(data []byte) error {
	// Use the standard JSON unmarshaler
	type Alias WebhookPayload
	aux := (*Alias)(w)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	return nil
}
