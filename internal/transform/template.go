package transform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
)

// GoTemplateEngine implements the Engine interface for Go templates
type GoTemplateEngine struct {
	templateString string
	template       *template.Template
	mu             sync.RWMutex
}

// NewGoTemplateEngine creates a new Go template engine
func NewGoTemplateEngine(templateString string) (*GoTemplateEngine, error) {
	if templateString == "" {
		return nil, fmt.Errorf("template cannot be empty")
	}

	engine := &GoTemplateEngine{
		templateString: templateString,
	}

	// Validate and compile the template
	if err := engine.compile(); err != nil {
		return nil, err
	}

	return engine, nil
}

// compile compiles the template with custom functions
func (e *GoTemplateEngine) compile() error {
	tmpl := template.New("transform").Funcs(GetTemplateFuncs()).Option("missingkey=default")

	compiled, err := tmpl.Parse(e.templateString)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	e.mu.Lock()
	e.template = compiled
	e.mu.Unlock()

	return nil
}

// Transform applies the template to the webhook payload
func (e *GoTemplateEngine) Transform(payload *alertmanager.WebhookPayload) (interface{}, error) {
	e.mu.RLock()
	tmpl := e.template
	e.mu.RUnlock()

	if tmpl == nil {
		return nil, fmt.Errorf("template not compiled")
	}

	// Create context with the payload
	ctx := &TemplateContext{
		Version:           payload.Version,
		GroupKey:          payload.GroupKey,
		TruncatedAlerts:   payload.TruncatedAlerts,
		Status:            payload.Status,
		Receiver:          payload.Receiver,
		GroupLabels:       payload.GroupLabels,
		CommonLabels:      payload.CommonLabels,
		CommonAnnotations: payload.CommonAnnotations,
		ExternalURL:       payload.ExternalURL,
		Alerts:            payload.Alerts,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	result := strings.TrimSpace(buf.String())

	// Try to parse as JSON if it looks like JSON
	if strings.HasPrefix(result, "{") || strings.HasPrefix(result, "[") {
		var jsonResult interface{}
		if err := json.Unmarshal([]byte(result), &jsonResult); err == nil {
			return jsonResult, nil
		}
	}

	// Return as string if not JSON
	return result, nil
}

// TransformAlert transforms a single alert with access to the full payload context
func (e *GoTemplateEngine) TransformAlert(alert *alertmanager.Alert, payload *alertmanager.WebhookPayload) (interface{}, error) {
	e.mu.RLock()
	tmpl := e.template
	e.mu.RUnlock()

	if tmpl == nil {
		return nil, fmt.Errorf("template not compiled")
	}

	// Create context with single alert (for split mode)
	ctx := &AlertTemplateContext{
		Alert: alert,
		TemplateContext: TemplateContext{
			Version:           payload.Version,
			GroupKey:          payload.GroupKey,
			TruncatedAlerts:   payload.TruncatedAlerts,
			Status:            payload.Status,
			Receiver:          payload.Receiver,
			GroupLabels:       payload.GroupLabels,
			CommonLabels:      payload.CommonLabels,
			CommonAnnotations: payload.CommonAnnotations,
			ExternalURL:       payload.ExternalURL,
			Alerts:            []alertmanager.Alert{*alert}, // Only include the current alert
		},
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return nil, fmt.Errorf("failed to execute template for alert: %w", err)
	}

	result := strings.TrimSpace(buf.String())

	// Try to parse as JSON
	if strings.HasPrefix(result, "{") || strings.HasPrefix(result, "[") {
		var jsonResult interface{}
		if err := json.Unmarshal([]byte(result), &jsonResult); err == nil {
			return jsonResult, nil
		}
	}

	return result, nil
}

// Validate checks if the template is valid
func (e *GoTemplateEngine) Validate() error {
	// Template is validated during compilation
	return nil
}

// Name returns the engine name
func (e *GoTemplateEngine) Name() string {
	return string(EngineTypeGoTemplate)
}

// TemplateContext provides the context for template execution
type TemplateContext struct {
	Version           string               `json:"version"`
	GroupKey          string               `json:"groupKey"`
	TruncatedAlerts   int                  `json:"truncatedAlerts"`
	Status            string               `json:"status"`
	Receiver          string               `json:"receiver"`
	GroupLabels       map[string]string    `json:"groupLabels"`
	CommonLabels      map[string]string    `json:"commonLabels"`
	CommonAnnotations map[string]string    `json:"commonAnnotations"`
	ExternalURL       string               `json:"externalURL"`
	Alerts            []alertmanager.Alert `json:"alerts"`
}

// AlertTemplateContext provides context for single alert transformation
type AlertTemplateContext struct {
	*alertmanager.Alert
	TemplateContext
}
