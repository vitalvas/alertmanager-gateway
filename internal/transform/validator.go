package transform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
)

// TemplateValidator validates templates
type TemplateValidator struct {
	engineType EngineType
	template   string
}

// NewTemplateValidator creates a new template validator
func NewTemplateValidator(engineType EngineType, template string) *TemplateValidator {
	return &TemplateValidator{
		engineType: engineType,
		template:   template,
	}
}

// Validate validates the template with sample data
func (v *TemplateValidator) Validate() (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Warnings: []string{},
		Info:     []string{},
	}

	if v.template == "" {
		result.Valid = false
		result.Error = "template cannot be empty"
		return result, nil
	}

	switch v.engineType {
	case EngineTypeGoTemplate:
		return v.validateGoTemplate()
	case EngineTypeJQ:
		return v.validateJQTemplate()
	default:
		result.Valid = false
		result.Error = fmt.Sprintf("unknown engine type: %s", v.engineType)
		return result, nil
	}
}

// validateGoTemplate validates a Go template
func (v *TemplateValidator) validateGoTemplate() (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Warnings: []string{},
		Info:     []string{},
	}

	// Try to parse the template
	tmpl := template.New("validate").Funcs(GetTemplateFuncs())
	_, err := tmpl.Parse(v.template)
	if err != nil {
		result.Valid = false
		result.Error = fmt.Sprintf("template parse error: %v", err)
		return result, nil
	}

	// Create sample data for validation
	samplePayload := v.createSamplePayload()

	// Try to execute with sample data
	engine, err := NewGoTemplateEngine(v.template)
	if err != nil {
		result.Valid = false
		result.Error = fmt.Sprintf("failed to create engine: %v", err)
		return result, nil
	}

	// Test with full payload
	output, err := engine.Transform(samplePayload)
	if err != nil {
		result.Valid = false
		result.Error = fmt.Sprintf("template execution error: %v", err)
		return result, nil
	}

	// Analyze output
	v.analyzeOutput(output, result)

	// Check for common issues
	v.checkCommonIssues(result)

	// Test with single alert (if split mode might be used)
	if len(samplePayload.Alerts) > 0 {
		alertOutput, err := engine.TransformAlert(&samplePayload.Alerts[0], samplePayload)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Template may not work in split mode: %v", err))
		} else {
			result.Info = append(result.Info, "Template works in both grouped and split modes")
			v.analyzeOutput(alertOutput, result)
		}
	}

	return result, nil
}

// createSamplePayload creates a sample webhook payload for validation
func (v *TemplateValidator) createSamplePayload() *alertmanager.WebhookPayload {
	now := time.Now()
	return &alertmanager.WebhookPayload{
		Version:  "4",
		GroupKey: "{}:{alertname=\"SampleAlert\"}",
		Status:   "firing",
		Receiver: "sample-receiver",
		GroupLabels: map[string]string{
			"alertname": "SampleAlert",
		},
		CommonLabels: map[string]string{
			"alertname": "SampleAlert",
			"severity":  "warning",
			"service":   "sample-service",
		},
		CommonAnnotations: map[string]string{
			"summary":     "Sample alert for validation",
			"description": "This is a sample alert used for template validation",
		},
		ExternalURL: "http://alertmanager.example.com",
		Alerts: []alertmanager.Alert{
			{
				Status:      "firing",
				Fingerprint: "a1b2c3d4e5f6",
				StartsAt:    now,
				Labels: map[string]string{
					"alertname": "SampleAlert",
					"severity":  "warning",
					"service":   "sample-service",
					"instance":  "sample-instance",
				},
				Annotations: map[string]string{
					"summary":     "Sample alert for validation",
					"description": "This is a sample alert used for template validation",
					"runbook_url": "https://example.com/runbooks/sample",
				},
				GeneratorURL: "http://prometheus.example.com/graph?expr=up%3D%3D0",
			},
		},
	}
}

// validateJQTemplate validates a jq query
func (v *TemplateValidator) validateJQTemplate() (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Warnings: []string{},
		Info:     []string{},
	}

	// Try to create the jq engine
	engine, err := NewJQEngine(v.template)
	if err != nil {
		result.Valid = false
		result.Error = fmt.Sprintf("jq parse error: %v", err)
		return result, nil
	}

	// Create sample data for validation
	samplePayload := v.createSamplePayload()

	// Test with full payload
	output, err := engine.Transform(samplePayload)
	if err != nil {
		result.Valid = false
		result.Error = fmt.Sprintf("jq execution error: %v", err)
		return result, nil
	}

	// Analyze the output
	v.analyzeOutput(output, result)

	// Test with single alert (split mode)
	if len(samplePayload.Alerts) > 0 {
		alertOutput, err := engine.TransformAlert(&samplePayload.Alerts[0], samplePayload)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("jq alert transformation failed: %v", err))
		} else {
			result.Info = append(result.Info, "jq query works with both grouped and split alert modes")
			// Analyze alert output size
			if alertStr, ok := alertOutput.(string); ok && len(alertStr) > 0 {
				result.Info = append(result.Info, fmt.Sprintf("split mode output size: %d characters", len(alertStr)))
			}
		}
	}

	// Additional jq-specific validations
	if result.OutputType == "null" {
		result.Warnings = append(result.Warnings, "jq query returns null - check if the query path exists")
	}

	// Check for common jq patterns
	if strings.Contains(v.template, ".alerts[]") {
		result.Info = append(result.Info, "detected array iteration pattern - good for processing multiple alerts")
	}
	if strings.Contains(v.template, "select(") {
		result.Info = append(result.Info, "detected filter pattern - good for conditional processing")
	}
	if strings.Contains(v.template, "map(") {
		result.Info = append(result.Info, "detected mapping pattern - good for transforming arrays")
	}

	return result, nil
}

// analyzeOutput analyzes the template output
func (v *TemplateValidator) analyzeOutput(output interface{}, result *ValidationResult) {
	switch out := output.(type) {
	case string:
		result.OutputType = "string"
		result.OutputSize = len(out)

		// Check if it looks like JSON
		if strings.HasPrefix(strings.TrimSpace(out), "{") ||
			strings.HasPrefix(strings.TrimSpace(out), "[") {
			result.Info = append(result.Info,
				"Output looks like JSON but is returned as string")
		}

		// Check for empty output
		if strings.TrimSpace(out) == "" {
			result.Warnings = append(result.Warnings,
				"Template produces empty output")
		}

	case map[string]interface{}, []interface{}:
		result.OutputType = "json"
		// Estimate size
		if data, err := json.Marshal(out); err == nil {
			result.OutputSize = len(data)
		}
		result.Info = append(result.Info, "Output is valid JSON")

	default:
		result.OutputType = fmt.Sprintf("%T", out)
	}

	// Warn about large outputs
	if result.OutputSize > 1024*1024 { // 1MB
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Large output size: %d bytes", result.OutputSize))
	}
}

// checkCommonIssues checks for common template issues
func (v *TemplateValidator) checkCommonIssues(result *ValidationResult) {
	template := v.template

	// Check for missing required fields
	if !strings.Contains(template, "Status") &&
		!strings.Contains(template, ".status") {
		result.Warnings = append(result.Warnings,
			"Template doesn't reference alert status")
	}

	// Check for common typos
	typos := map[string]string{
		"Labes":      "Labels",
		"Anotations": "Annotations",
		"StartsAt":   "StartsAt",
		"alertName":  "alertname",
	}

	for typo, correct := range typos {
		if strings.Contains(template, typo) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Possible typo: '%s' (should be '%s'?)", typo, correct))
		}
	}

	// Check for unsafe operations
	if strings.Contains(template, "exec") ||
		strings.Contains(template, "system") {
		result.Warnings = append(result.Warnings,
			"Template contains potentially unsafe operations")
	}

	// Check for complex logic
	ifCount := strings.Count(template, "{{if")
	rangeCount := strings.Count(template, "{{range")

	if ifCount > 10 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Complex template with %d if statements", ifCount))
	}

	if rangeCount > 5 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Complex template with %d range loops", rangeCount))
	}

	// Check for template functions usage
	usedFuncs := v.findUsedFunctions(template)
	if len(usedFuncs) > 0 {
		result.Info = append(result.Info,
			fmt.Sprintf("Uses template functions: %s", strings.Join(usedFuncs, ", ")))
	}
}

// findUsedFunctions finds which template functions are used
func (v *TemplateValidator) findUsedFunctions(template string) []string {
	funcs := GetTemplateFuncs()
	used := []string{}

	for name := range funcs {
		if strings.Contains(template, name+" ") ||
			strings.Contains(template, name+"|") ||
			strings.Contains(template, "|"+name) {
			used = append(used, name)
		}
	}

	return used
}

// ValidationResult holds the result of template validation
type ValidationResult struct {
	Valid      bool
	Error      string
	Warnings   []string
	Info       []string
	OutputType string
	OutputSize int
}

// String returns a string representation of the validation result
func (r *ValidationResult) String() string {
	var buf bytes.Buffer

	if r.Valid {
		buf.WriteString("✓ Template is valid\n")
	} else {
		buf.WriteString("✗ Template is invalid\n")
		if r.Error != "" {
			buf.WriteString(fmt.Sprintf("  Error: %s\n", r.Error))
		}
	}

	if r.OutputType != "" {
		buf.WriteString(fmt.Sprintf("  Output type: %s\n", r.OutputType))
	}

	if r.OutputSize > 0 {
		buf.WriteString(fmt.Sprintf("  Output size: %d bytes\n", r.OutputSize))
	}

	if len(r.Warnings) > 0 {
		buf.WriteString("  Warnings:\n")
		for _, w := range r.Warnings {
			buf.WriteString(fmt.Sprintf("    - %s\n", w))
		}
	}

	if len(r.Info) > 0 {
		buf.WriteString("  Info:\n")
		for _, i := range r.Info {
			buf.WriteString(fmt.Sprintf("    - %s\n", i))
		}
	}

	return buf.String()
}
