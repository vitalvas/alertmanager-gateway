package transform

import (
	"fmt"

	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
)

// Engine defines the interface for transformation engines
type Engine interface {
	// Transform applies the transformation to the webhook payload
	Transform(payload *alertmanager.WebhookPayload) (interface{}, error)

	// TransformAlert transforms a single alert (for split mode)
	TransformAlert(alert *alertmanager.Alert, payload *alertmanager.WebhookPayload) (interface{}, error)

	// Validate checks if the transformation is valid
	Validate() error

	// Name returns the engine name
	Name() string
}

// EngineType represents the type of transformation engine
type EngineType string

const (
	// EngineTypeGoTemplate represents Go text/template engine
	EngineTypeGoTemplate EngineType = "go-template"

	// EngineTypeJQ represents jq transformation engine
	EngineTypeJQ EngineType = "jq"
)

// NewEngine creates a new transformation engine based on the type
func NewEngine(engineType EngineType, template string) (Engine, error) {
	switch engineType {
	case EngineTypeGoTemplate:
		return NewGoTemplateEngine(template)
	case EngineTypeJQ:
		return NewJQEngine(template)
	default:
		return nil, fmt.Errorf("unknown engine type: %s", engineType)
	}
}

// Context provides additional context for template rendering
type Context struct {
	// Payload is the full webhook payload
	Payload *alertmanager.WebhookPayload

	// Alert is the current alert (in split mode)
	Alert *alertmanager.Alert

	// Variables for custom data
	Variables map[string]interface{}
}
