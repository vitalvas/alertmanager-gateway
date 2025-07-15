package alertmanager

import (
	"time"
)

// WebhookPayload represents the incoming webhook from Prometheus Alertmanager
type WebhookPayload struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	TruncatedAlerts   int               `json:"truncatedAlerts"`
	Status            string            `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []Alert           `json:"alerts"`
}

// Alert represents a single alert in the webhook payload
type Alert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

// IsValid validates the webhook payload
func (w *WebhookPayload) IsValid() error {
	if w.Version == "" {
		return ErrMissingVersion
	}

	if w.GroupKey == "" {
		return ErrMissingGroupKey
	}

	if w.Status != "firing" && w.Status != "resolved" {
		return ErrInvalidStatus
	}

	if len(w.Alerts) == 0 {
		return ErrNoAlerts
	}

	for i, alert := range w.Alerts {
		if err := alert.IsValid(); err != nil {
			return NewAlertValidationError(i, err)
		}
	}

	return nil
}

// IsValid validates a single alert
func (a *Alert) IsValid() error {
	if a.Status != "firing" && a.Status != "resolved" {
		return ErrInvalidAlertStatus
	}

	if a.Fingerprint == "" {
		return ErrMissingFingerprint
	}

	// StartsAt must be set for all alerts
	if a.StartsAt.IsZero() {
		return ErrMissingStartsAt
	}

	// EndsAt must be after StartsAt if set
	if !a.EndsAt.IsZero() && a.EndsAt.Before(a.StartsAt) {
		return ErrInvalidTimeRange
	}

	return nil
}

// IsFiring returns true if the alert is in firing state
func (a *Alert) IsFiring() bool {
	return a.Status == "firing"
}

// IsResolved returns true if the alert is in resolved state
func (a *Alert) IsResolved() bool {
	return a.Status == "resolved"
}

// GetLabelValue safely gets a label value
func (a *Alert) GetLabelValue(key string) string {
	if a.Labels == nil {
		return ""
	}
	return a.Labels[key]
}

// GetAnnotationValue safely gets an annotation value
func (a *Alert) GetAnnotationValue(key string) string {
	if a.Annotations == nil {
		return ""
	}
	return a.Annotations[key]
}

// GetAlertName returns the alert name from labels
func (a *Alert) GetAlertName() string {
	return a.GetLabelValue("alertname")
}

// GetSeverity returns the severity from labels
func (a *Alert) GetSeverity() string {
	return a.GetLabelValue("severity")
}

// Clone creates a deep copy of the webhook payload
func (w *WebhookPayload) Clone() *WebhookPayload {
	clone := &WebhookPayload{
		Version:         w.Version,
		GroupKey:        w.GroupKey,
		TruncatedAlerts: w.TruncatedAlerts,
		Status:          w.Status,
		Receiver:        w.Receiver,
		ExternalURL:     w.ExternalURL,
	}

	// Deep copy maps
	if w.GroupLabels != nil {
		clone.GroupLabels = make(map[string]string)
		for k, v := range w.GroupLabels {
			clone.GroupLabels[k] = v
		}
	}

	if w.CommonLabels != nil {
		clone.CommonLabels = make(map[string]string)
		for k, v := range w.CommonLabels {
			clone.CommonLabels[k] = v
		}
	}

	if w.CommonAnnotations != nil {
		clone.CommonAnnotations = make(map[string]string)
		for k, v := range w.CommonAnnotations {
			clone.CommonAnnotations[k] = v
		}
	}

	// Deep copy alerts
	if w.Alerts != nil {
		clone.Alerts = make([]Alert, len(w.Alerts))
		for i, alert := range w.Alerts {
			clone.Alerts[i] = *alert.Clone()
		}
	}

	return clone
}

// Clone creates a deep copy of an alert
func (a *Alert) Clone() *Alert {
	clone := &Alert{
		Status:       a.Status,
		StartsAt:     a.StartsAt,
		EndsAt:       a.EndsAt,
		GeneratorURL: a.GeneratorURL,
		Fingerprint:  a.Fingerprint,
	}

	if a.Labels != nil {
		clone.Labels = make(map[string]string)
		for k, v := range a.Labels {
			clone.Labels[k] = v
		}
	}

	if a.Annotations != nil {
		clone.Annotations = make(map[string]string)
		for k, v := range a.Annotations {
			clone.Annotations[k] = v
		}
	}

	return clone
}
