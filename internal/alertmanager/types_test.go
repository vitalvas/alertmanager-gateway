package alertmanager

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookPayload_IsValid(t *testing.T) {
	validAlert := Alert{
		Status:      "firing",
		Fingerprint: "abc123",
		StartsAt:    time.Now(),
		Labels: map[string]string{
			"alertname": "test",
		},
	}

	tests := []struct {
		name    string
		payload WebhookPayload
		wantErr error
	}{
		{
			name: "valid payload",
			payload: WebhookPayload{
				Version:  "4",
				GroupKey: "test-group",
				Status:   "firing",
				Alerts:   []Alert{validAlert},
			},
			wantErr: nil,
		},
		{
			name: "missing version",
			payload: WebhookPayload{
				GroupKey: "test-group",
				Status:   "firing",
				Alerts:   []Alert{validAlert},
			},
			wantErr: ErrMissingVersion,
		},
		{
			name: "missing group key",
			payload: WebhookPayload{
				Version: "4",
				Status:  "firing",
				Alerts:  []Alert{validAlert},
			},
			wantErr: ErrMissingGroupKey,
		},
		{
			name: "invalid status",
			payload: WebhookPayload{
				Version:  "4",
				GroupKey: "test-group",
				Status:   "invalid",
				Alerts:   []Alert{validAlert},
			},
			wantErr: ErrInvalidStatus,
		},
		{
			name: "no alerts",
			payload: WebhookPayload{
				Version:  "4",
				GroupKey: "test-group",
				Status:   "firing",
				Alerts:   []Alert{},
			},
			wantErr: ErrNoAlerts,
		},
		{
			name: "invalid alert",
			payload: WebhookPayload{
				Version:  "4",
				GroupKey: "test-group",
				Status:   "firing",
				Alerts: []Alert{
					{
						Status:      "invalid",
						Fingerprint: "abc123",
						StartsAt:    time.Now(),
					},
				},
			},
			wantErr: ErrInvalidAlertStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.IsValid()
			if tt.wantErr != nil {
				require.Error(t, err)
				if _, ok := err.(*AlertValidationError); ok {
					assert.Contains(t, err.Error(), tt.wantErr.Error())
				} else {
					assert.Equal(t, tt.wantErr, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAlert_IsValid(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		alert   Alert
		wantErr error
	}{
		{
			name: "valid firing alert",
			alert: Alert{
				Status:      "firing",
				Fingerprint: "abc123",
				StartsAt:    now,
			},
			wantErr: nil,
		},
		{
			name: "valid resolved alert",
			alert: Alert{
				Status:      "resolved",
				Fingerprint: "abc123",
				StartsAt:    now,
				EndsAt:      now.Add(time.Hour),
			},
			wantErr: nil,
		},
		{
			name: "invalid status",
			alert: Alert{
				Status:      "pending",
				Fingerprint: "abc123",
				StartsAt:    now,
			},
			wantErr: ErrInvalidAlertStatus,
		},
		{
			name: "missing fingerprint",
			alert: Alert{
				Status:   "firing",
				StartsAt: now,
			},
			wantErr: ErrMissingFingerprint,
		},
		{
			name: "missing starts at",
			alert: Alert{
				Status:      "firing",
				Fingerprint: "abc123",
			},
			wantErr: ErrMissingStartsAt,
		},
		{
			name: "invalid time range",
			alert: Alert{
				Status:      "resolved",
				Fingerprint: "abc123",
				StartsAt:    now,
				EndsAt:      now.Add(-time.Hour),
			},
			wantErr: ErrInvalidTimeRange,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.alert.IsValid()
			assert.Equal(t, tt.wantErr, err)
		})
	}
}

func TestAlert_Helpers(t *testing.T) {
	alert := Alert{
		Status: "firing",
		Labels: map[string]string{
			"alertname": "HighCPU",
			"severity":  "critical",
			"instance":  "server1",
		},
		Annotations: map[string]string{
			"summary":     "High CPU usage",
			"description": "CPU usage is above 90%",
		},
	}

	t.Run("IsFiring", func(t *testing.T) {
		assert.True(t, alert.IsFiring())
		assert.False(t, alert.IsResolved())
	})

	t.Run("GetLabelValue", func(t *testing.T) {
		assert.Equal(t, "HighCPU", alert.GetLabelValue("alertname"))
		assert.Equal(t, "critical", alert.GetLabelValue("severity"))
		assert.Equal(t, "", alert.GetLabelValue("nonexistent"))
	})

	t.Run("GetAnnotationValue", func(t *testing.T) {
		assert.Equal(t, "High CPU usage", alert.GetAnnotationValue("summary"))
		assert.Equal(t, "", alert.GetAnnotationValue("nonexistent"))
	})

	t.Run("GetAlertName", func(t *testing.T) {
		assert.Equal(t, "HighCPU", alert.GetAlertName())
	})

	t.Run("GetSeverity", func(t *testing.T) {
		assert.Equal(t, "critical", alert.GetSeverity())
	})

	t.Run("nil maps", func(t *testing.T) {
		emptyAlert := Alert{Status: "firing"}
		assert.Equal(t, "", emptyAlert.GetLabelValue("any"))
		assert.Equal(t, "", emptyAlert.GetAnnotationValue("any"))
		assert.Equal(t, "", emptyAlert.GetAlertName())
		assert.Equal(t, "", emptyAlert.GetSeverity())
	})
}

func TestWebhookPayload_Clone(t *testing.T) {
	original := &WebhookPayload{
		Version:         "4",
		GroupKey:        "test-group",
		TruncatedAlerts: 5,
		Status:          "firing",
		Receiver:        "test-receiver",
		ExternalURL:     "http://example.com",
		GroupLabels: map[string]string{
			"alertname": "test",
		},
		CommonLabels: map[string]string{
			"severity": "critical",
		},
		CommonAnnotations: map[string]string{
			"summary": "Test summary",
		},
		Alerts: []Alert{
			{
				Status:      "firing",
				Fingerprint: "abc123",
				StartsAt:    time.Now(),
				Labels: map[string]string{
					"instance": "server1",
				},
				Annotations: map[string]string{
					"description": "Test description",
				},
			},
		},
	}

	clone := original.Clone()

	// Verify clone is equal
	assert.Equal(t, original.Version, clone.Version)
	assert.Equal(t, original.GroupKey, clone.GroupKey)
	assert.Equal(t, original.TruncatedAlerts, clone.TruncatedAlerts)
	assert.Equal(t, original.Status, clone.Status)
	assert.Equal(t, original.GroupLabels, clone.GroupLabels)
	assert.Equal(t, original.CommonLabels, clone.CommonLabels)
	assert.Equal(t, original.CommonAnnotations, clone.CommonAnnotations)
	assert.Equal(t, len(original.Alerts), len(clone.Alerts))

	// Verify deep copy - modify original
	original.GroupLabels["new"] = "value"
	original.Alerts[0].Labels["new"] = "value"

	// Clone should not be affected
	assert.NotContains(t, clone.GroupLabels, "new")
	assert.NotContains(t, clone.Alerts[0].Labels, "new")
}

func TestAlert_Clone(t *testing.T) {
	original := &Alert{
		Status:       "firing",
		Fingerprint:  "abc123",
		StartsAt:     time.Now(),
		EndsAt:       time.Now().Add(time.Hour),
		GeneratorURL: "http://example.com",
		Labels: map[string]string{
			"alertname": "test",
		},
		Annotations: map[string]string{
			"summary": "Test summary",
		},
	}

	clone := original.Clone()

	// Verify clone is equal
	assert.Equal(t, original.Status, clone.Status)
	assert.Equal(t, original.Fingerprint, clone.Fingerprint)
	assert.Equal(t, original.StartsAt, clone.StartsAt)
	assert.Equal(t, original.Labels, clone.Labels)
	assert.Equal(t, original.Annotations, clone.Annotations)

	// Verify deep copy
	original.Labels["new"] = "value"
	assert.NotContains(t, clone.Labels, "new")
}
