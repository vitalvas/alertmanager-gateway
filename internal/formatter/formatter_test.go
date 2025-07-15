package formatter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFormatter(t *testing.T) {
	tests := []struct {
		name    string
		format  OutputFormat
		wantErr bool
		errMsg  string
	}{
		{
			name:    "json formatter",
			format:  FormatJSON,
			wantErr: false,
		},
		{
			name:    "form formatter",
			format:  FormatForm,
			wantErr: false,
		},
		{
			name:    "query formatter",
			format:  FormatQuery,
			wantErr: false,
		},
		{
			name:    "xml formatter",
			format:  FormatXML,
			wantErr: false,
		},
		{
			name:    "unknown format",
			format:  OutputFormat("unknown"),
			wantErr: true,
			errMsg:  "unknown format: unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter, err := NewFormatter(tt.format)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, formatter)
			} else {
				require.NoError(t, err)
				require.NotNil(t, formatter)
			}
		})
	}
}

func TestFormatData(t *testing.T) {
	data := map[string]interface{}{
		"message": "test",
		"value":   42,
	}

	req, err := FormatData(FormatJSON, data)
	require.NoError(t, err)
	require.NotNil(t, req)

	assert.NotEmpty(t, req.Body)
	assert.Equal(t, "application/json", req.ContentType)
	assert.Equal(t, "application/json", req.Headers.Get("Content-Type"))

	// Verify JSON is valid
	var result map[string]interface{}
	err = json.Unmarshal(req.Body, &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result["message"])
	assert.Equal(t, float64(42), result["value"])
}

func TestFormatData_InvalidFormat(t *testing.T) {
	data := map[string]interface{}{"test": "data"}

	_, err := FormatData(OutputFormat("invalid"), data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown format")
}

func TestIsValidFormat(t *testing.T) {
	tests := []struct {
		format string
		valid  bool
	}{
		{"json", true},
		{"form", true},
		{"query", true},
		{"xml", true},
		{"", false},
		{"JSON", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			assert.Equal(t, tt.valid, IsValidFormat(tt.format))
		})
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input   string
		want    OutputFormat
		wantErr bool
	}{
		{"json", FormatJSON, false},
		{"form", FormatForm, false},
		{"query", FormatQuery, false},
		{"xml", FormatXML, false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseFormat(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid format")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name     string
		data     interface{}
		expected OutputFormat
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: FormatJSON,
		},
		{
			name:     "json object",
			data:     `{"key": "value"}`,
			expected: FormatJSON,
		},
		{
			name:     "json array",
			data:     `["item1", "item2"]`,
			expected: FormatJSON,
		},
		{
			name:     "xml with declaration",
			data:     `<?xml version="1.0"?><root>content</root>`,
			expected: FormatXML,
		},
		{
			name:     "xml without declaration",
			data:     `<message>hello</message>`,
			expected: FormatXML,
		},
		{
			name:     "form data single",
			data:     `name=test`,
			expected: FormatForm,
		},
		{
			name:     "form data multiple",
			data:     `name=test&value=123`,
			expected: FormatForm,
		},
		{
			name:     "query parameters",
			data:     `search=query&page=1&limit=10`,
			expected: FormatForm,
		},
		{
			name:     "bytes json",
			data:     []byte(`{"status": "ok"}`),
			expected: FormatJSON,
		},
		{
			name:     "bytes xml",
			data:     []byte(`<status>ok</status>`),
			expected: FormatXML,
		},
		{
			name:     "bytes form",
			data:     []byte(`status=ok&count=5`),
			expected: FormatForm,
		},
		{
			name:     "structured data",
			data:     map[string]interface{}{"key": "value"},
			expected: FormatJSON,
		},
		{
			name:     "invalid json",
			data:     `{invalid json`,
			expected: FormatJSON, // Falls back to JSON
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectFormat(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatFromContentType(t *testing.T) {
	tests := []struct {
		contentType string
		expected    OutputFormat
	}{
		{"application/json", FormatJSON},
		{"text/json", FormatJSON},
		{"application/json; charset=utf-8", FormatJSON},
		{"application/x-www-form-urlencoded", FormatForm},
		{"application/xml", FormatXML},
		{"text/xml", FormatXML},
		{"application/xml; charset=utf-8", FormatXML},
		{"text/plain", ""},
		{"unknown/type", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := FormatFromContentType(tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatDataWithAutoDetection(t *testing.T) {
	tests := []struct {
		name         string
		data         interface{}
		expectedType string
	}{
		{
			name:         "json data",
			data:         map[string]string{"key": "value"},
			expectedType: "application/json",
		},
		{
			name:         "pre-formatted json",
			data:         `{"message": "test"}`,
			expectedType: "application/json",
		},
		{
			name:         "pre-formatted form",
			data:         `name=test&value=123`,
			expectedType: "application/x-www-form-urlencoded",
		},
		{
			name:         "pre-formatted xml",
			data:         `<message>hello</message>`,
			expectedType: "application/xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := FormatDataWithAutoDetection(tt.data)
			require.NoError(t, err)
			require.NotNil(t, req)

			assert.Equal(t, tt.expectedType, req.ContentType)
			assert.NotEmpty(t, req.Body)
		})
	}
}

func TestFormatDataWithContentType(t *testing.T) {
	data := map[string]string{"key": "value"}

	tests := []struct {
		name         string
		contentType  string
		expectedType string
	}{
		{
			name:         "json content type",
			contentType:  "application/json",
			expectedType: "application/json",
		},
		{
			name:         "form content type",
			contentType:  "application/x-www-form-urlencoded",
			expectedType: "application/x-www-form-urlencoded",
		},
		{
			name:         "xml content type",
			contentType:  "application/xml",
			expectedType: "application/xml",
		},
		{
			name:         "unknown content type falls back to auto-detection",
			contentType:  "unknown/type",
			expectedType: "application/json", // Falls back to JSON for structured data
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := FormatDataWithContentType(data, tt.contentType)
			require.NoError(t, err)
			require.NotNil(t, req)

			assert.Equal(t, tt.expectedType, req.ContentType)
			assert.NotEmpty(t, req.Body)
		})
	}
}

func TestGetAllFormats(t *testing.T) {
	formats := GetAllFormats()

	assert.Len(t, formats, 4)
	assert.Contains(t, formats, FormatJSON)
	assert.Contains(t, formats, FormatForm)
	assert.Contains(t, formats, FormatQuery)
	assert.Contains(t, formats, FormatXML)
}

func TestGetFormatDescription(t *testing.T) {
	tests := []struct {
		format      OutputFormat
		description string
	}{
		{FormatJSON, "JSON (JavaScript Object Notation)"},
		{FormatForm, "Form-encoded (application/x-www-form-urlencoded)"},
		{FormatQuery, "Query parameters (URL query string format)"},
		{FormatXML, "XML (eXtensible Markup Language)"},
		{OutputFormat("unknown"), "Unknown format"},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			result := GetFormatDescription(tt.format)
			assert.Equal(t, tt.description, result)
		})
	}
}

func TestFormatData_AllFormats(t *testing.T) {
	data := map[string]interface{}{
		"name":   "test",
		"value":  123,
		"active": true,
	}

	formats := []OutputFormat{FormatJSON, FormatForm, FormatQuery, FormatXML}

	for _, format := range formats {
		t.Run(string(format), func(t *testing.T) {
			req, err := FormatData(format, data)
			require.NoError(t, err)
			require.NotNil(t, req)

			assert.NotEmpty(t, req.Body)

			// Query formatter doesn't have content type (goes in URL)
			if format != FormatQuery {
				assert.NotEmpty(t, req.ContentType)
				assert.Equal(t, req.ContentType, req.Headers.Get("Content-Type"))
			} else {
				assert.Empty(t, req.ContentType)
			}

			// Verify the formatter name matches
			formatter, err := NewFormatter(format)
			require.NoError(t, err)
			assert.Equal(t, string(format), formatter.Name())
		})
	}
}
