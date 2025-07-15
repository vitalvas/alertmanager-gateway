package formatter

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// JSONFormatter formats data as JSON
type JSONFormatter struct {
	indent bool
}

// NewJSONFormatter creates a new JSON formatter
func NewJSONFormatter() *JSONFormatter {
	return &JSONFormatter{
		indent: false,
	}
}

// NewJSONFormatterWithIndent creates a new JSON formatter with indentation
func NewJSONFormatterWithIndent() *JSONFormatter {
	return &JSONFormatter{
		indent: true,
	}
}

// Format converts data to JSON format
func (f *JSONFormatter) Format(data interface{}) ([]byte, error) {
	// Check if data is already a byte slice (pre-formatted)
	if bytes, ok := data.([]byte); ok {
		// Validate it's valid JSON
		var temp interface{}
		if err := json.Unmarshal(bytes, &temp); err != nil {
			return nil, fmt.Errorf("invalid JSON data: %w", err)
		}
		return bytes, nil
	}

	// Check if data is already a string (pre-formatted)
	if str, ok := data.(string); ok {
		// Validate it's valid JSON
		var temp interface{}
		if err := json.Unmarshal([]byte(str), &temp); err != nil {
			return nil, fmt.Errorf("invalid JSON string: %w", err)
		}
		return []byte(str), nil
	}

	// Marshal the data
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false) // Don't escape HTML characters

	if f.indent {
		encoder.SetIndent("", "  ")
	}

	if err := encoder.Encode(data); err != nil {
		return nil, fmt.Errorf("failed to encode JSON: %w", err)
	}

	// Remove trailing newline added by encoder
	result := buf.Bytes()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}

	return result, nil
}

// ContentType returns the content type for JSON
func (f *JSONFormatter) ContentType() string {
	return "application/json"
}

// Name returns the formatter name
func (f *JSONFormatter) Name() string {
	return string(FormatJSON)
}
