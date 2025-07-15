package transform

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTemplateValidator(t *testing.T) {
	validator := NewTemplateValidator(EngineTypeGoTemplate, `{{ .Status }}`)
	assert.NotNil(t, validator)
	assert.Equal(t, EngineTypeGoTemplate, validator.engineType)
	assert.Equal(t, `{{ .Status }}`, validator.template)
}

func TestTemplateValidator_Validate(t *testing.T) {
	tests := []struct {
		name           string
		engineType     EngineType
		template       string
		expectValid    bool
		expectError    string
		expectWarnings []string
	}{
		{
			name:        "valid go template",
			engineType:  EngineTypeGoTemplate,
			template:    `{{ .Status }}`,
			expectValid: true,
		},
		{
			name:        "empty template",
			engineType:  EngineTypeGoTemplate,
			template:    "",
			expectValid: false,
			expectError: "template cannot be empty",
		},
		{
			name:        "invalid go template",
			engineType:  EngineTypeGoTemplate,
			template:    `{{ .Status }`,
			expectValid: false,
			expectError: "template parse error",
		},
		{
			name:        "valid jq query",
			engineType:  EngineTypeJQ,
			template:    ".status",
			expectValid: true,
		},
		{
			name:        "invalid jq syntax",
			engineType:  EngineTypeJQ,
			template:    ".status |",
			expectValid: false,
			expectError: "jq parse error",
		},
		{
			name:        "unknown engine type",
			engineType:  EngineType("unknown"),
			template:    "template",
			expectValid: false,
			expectError: "unknown engine type: unknown",
		},
		{
			name:        "template with JSON output",
			engineType:  EngineTypeGoTemplate,
			template:    `{"status": "{{ .Status }}"}`,
			expectValid: true,
		},
		{
			name:           "template without status reference",
			engineType:     EngineTypeGoTemplate,
			template:       `{{ .Version }}`,
			expectValid:    true,
			expectWarnings: []string{"Template doesn't reference alert status"},
		},
		{
			name:        "template with typo",
			engineType:  EngineTypeGoTemplate,
			template:    `{{ .Labes.alertname }}`,
			expectValid: false,
			expectError: "can't evaluate field Labes",
		},
		{
			name:           "complex template",
			engineType:     EngineTypeGoTemplate,
			template:       strings.Repeat(`{{if .Status}}x{{end}}`, 11),
			expectValid:    true,
			expectWarnings: []string{"Complex template with 11 if statements"},
		},
		{
			name:        "template with functions",
			engineType:  EngineTypeGoTemplate,
			template:    `{{ .Status | upper | trim }}`,
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewTemplateValidator(tt.engineType, tt.template)
			result, err := validator.Validate()
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Equal(t, tt.expectValid, result.Valid)

			if tt.expectError != "" {
				assert.Contains(t, result.Error, tt.expectError)
			}

			for _, warning := range tt.expectWarnings {
				assert.Contains(t, result.Warnings, warning)
			}
		})
	}
}

func TestTemplateValidator_ValidateGoTemplate(t *testing.T) {
	validator := NewTemplateValidator(EngineTypeGoTemplate, `{{ .Status }} - {{ .CommonLabels.severity | upper }}`)

	result, err := validator.Validate()
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Valid)
	assert.Empty(t, result.Error)
	assert.NotEmpty(t, result.Info)

	// Should detect template functions usage
	found := false
	for _, info := range result.Info {
		if strings.Contains(info, "upper") {
			found = true
			break
		}
	}
	assert.True(t, found, "Should detect template function usage")
}

func TestValidationResult_String(t *testing.T) {
	// Valid result
	result := &ValidationResult{
		Valid:      true,
		OutputType: "json",
		OutputSize: 256,
		Info:       []string{"Uses template functions: upper"},
	}

	str := result.String()
	assert.Contains(t, str, "✓ Template is valid")
	assert.Contains(t, str, "Output type: json")
	assert.Contains(t, str, "Output size: 256 bytes")
	assert.Contains(t, str, "Uses template functions: upper")

	// Invalid result
	result2 := &ValidationResult{
		Valid:    false,
		Error:    "parse error",
		Warnings: []string{"Missing status field"},
	}

	str2 := result2.String()
	assert.Contains(t, str2, "✗ Template is invalid")
	assert.Contains(t, str2, "Error: parse error")
	assert.Contains(t, str2, "Missing status field")
}

func TestTemplateValidator_AnalyzeOutput(t *testing.T) {
	validator := NewTemplateValidator(EngineTypeGoTemplate, `{{ .Status }}`)
	result := &ValidationResult{
		Valid:    true,
		Warnings: []string{},
		Info:     []string{},
	}

	// Test string output
	validator.analyzeOutput("firing", result)
	assert.Equal(t, "string", result.OutputType)
	assert.Equal(t, 6, result.OutputSize)

	// Test JSON-like string
	result2 := &ValidationResult{Valid: true, Warnings: []string{}, Info: []string{}}
	validator.analyzeOutput(`{"status": "firing"}`, result2)
	assert.Contains(t, result2.Info, "Output looks like JSON but is returned as string")

	// Test empty output
	result3 := &ValidationResult{Valid: true, Warnings: []string{}, Info: []string{}}
	validator.analyzeOutput("  ", result3)
	assert.Contains(t, result3.Warnings, "Template produces empty output")

	// Test JSON output
	result4 := &ValidationResult{Valid: true, Warnings: []string{}, Info: []string{}}
	validator.analyzeOutput(map[string]interface{}{"status": "firing"}, result4)
	assert.Equal(t, "json", result4.OutputType)
	assert.Contains(t, result4.Info, "Output is valid JSON")

	// Test large output
	result5 := &ValidationResult{Valid: true, Warnings: []string{}, Info: []string{}}
	largeString := strings.Repeat("a", 1024*1024+1)
	validator.analyzeOutput(largeString, result5)
	assert.Contains(t, result5.Warnings[0], "Large output size")
}

func TestTemplateValidator_FindUsedFunctions(t *testing.T) {
	validator := NewTemplateValidator(EngineTypeGoTemplate, `{{ .Status | upper | trim }} {{ now | unixtime }}`)

	funcs := validator.findUsedFunctions(validator.template)
	assert.Contains(t, funcs, "upper")
	assert.Contains(t, funcs, "trim")
	assert.Contains(t, funcs, "now")
	assert.Contains(t, funcs, "unixtime")
}
