package alertmanager

import (
	"errors"
	"fmt"
)

// Common validation errors
var (
	ErrMissingVersion     = errors.New("missing version field")
	ErrMissingGroupKey    = errors.New("missing groupKey field")
	ErrInvalidStatus      = errors.New("invalid status: must be 'firing' or 'resolved'")
	ErrNoAlerts           = errors.New("no alerts in payload")
	ErrInvalidAlertStatus = errors.New("invalid alert status: must be 'firing' or 'resolved'")
	ErrMissingFingerprint = errors.New("missing fingerprint")
	ErrMissingStartsAt    = errors.New("missing startsAt timestamp")
	ErrInvalidTimeRange   = errors.New("endsAt must be after startsAt")
	ErrInvalidJSON        = errors.New("invalid JSON payload")
	ErrPayloadTooLarge    = errors.New("payload too large")
)

// AlertValidationError represents an error validating a specific alert
type AlertValidationError struct {
	Index int
	Err   error
}

func (e *AlertValidationError) Error() string {
	return fmt.Sprintf("alert[%d] validation failed: %v", e.Index, e.Err)
}

func (e *AlertValidationError) Unwrap() error {
	return e.Err
}

// NewAlertValidationError creates a new alert validation error
func NewAlertValidationError(index int, err error) error {
	return &AlertValidationError{
		Index: index,
		Err:   err,
	}
}
