package formatter

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// OutputFormat represents the output format type
type OutputFormat string

const (
	// FormatJSON outputs data as JSON
	FormatJSON OutputFormat = "json"
	// FormatForm outputs data as form-encoded
	FormatForm OutputFormat = "form"
	// FormatQuery outputs data as query parameters
	FormatQuery OutputFormat = "query"
	// FormatXML outputs data as XML
	FormatXML OutputFormat = "xml"
)

// Formatter interface for output formatting
type Formatter interface {
	// Format converts data to the specific output format
	Format(data interface{}) ([]byte, error)

	// ContentType returns the HTTP content type for this format
	ContentType() string

	// Name returns the formatter name
	Name() string
}

// Request represents a formatted HTTP request
type Request struct {
	// Body is the request body (for POST/PUT)
	Body []byte

	// ContentType is the content type header
	ContentType string

	// QueryParams are URL query parameters (for GET or query format)
	QueryParams map[string]string

	// Headers are additional HTTP headers
	Headers http.Header
}

// NewFormatter creates a formatter based on the format type
func NewFormatter(format OutputFormat) (Formatter, error) {
	switch format {
	case FormatJSON:
		return NewJSONFormatter(), nil
	case FormatForm:
		return NewFormFormatter(), nil
	case FormatQuery:
		return NewQueryFormatter(), nil
	case FormatXML:
		return NewXMLFormatter(), nil
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
}

// FormatData formats data using the specified formatter
func FormatData(format OutputFormat, data interface{}) (*Request, error) {
	formatter, err := NewFormatter(format)
	if err != nil {
		return nil, err
	}

	body, err := formatter.Format(data)
	if err != nil {
		return nil, fmt.Errorf("failed to format data: %w", err)
	}

	req := &Request{
		Body:        body,
		ContentType: formatter.ContentType(),
		Headers:     make(http.Header),
	}

	// Set content type header
	if req.ContentType != "" {
		req.Headers.Set("Content-Type", req.ContentType)
	}

	return req, nil
}

// IsValidFormat checks if the format is valid
func IsValidFormat(format string) bool {
	switch OutputFormat(format) {
	case FormatJSON, FormatForm, FormatQuery, FormatXML:
		return true
	default:
		return false
	}
}

// ParseFormat parses a string into OutputFormat
func ParseFormat(format string) (OutputFormat, error) {
	if !IsValidFormat(format) {
		return "", fmt.Errorf("invalid format: %s", format)
	}
	return OutputFormat(format), nil
}

// DetectFormat attempts to auto-detect the format of data
func DetectFormat(data interface{}) OutputFormat {
	if data == nil {
		return FormatJSON // Default to JSON for nil data
	}

	switch v := data.(type) {
	case []byte:
		return detectFormatFromBytes(v)
	case string:
		return detectFormatFromString(v)
	default:
		// For structured data, default to JSON
		return FormatJSON
	}
}

// detectFormatFromBytes detects format from byte data
func detectFormatFromBytes(data []byte) OutputFormat {
	if len(data) == 0 {
		return FormatJSON
	}

	// Trim whitespace
	trimmed := strings.TrimSpace(string(data))

	// Check for JSON
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		var temp interface{}
		if json.Unmarshal([]byte(trimmed), &temp) == nil {
			return FormatJSON
		}
	}

	// Check for XML
	if strings.HasPrefix(trimmed, "<?xml") ||
		(strings.HasPrefix(trimmed, "<") && strings.HasSuffix(trimmed, ">")) {
		var temp interface{}
		if xml.Unmarshal([]byte(trimmed), &temp) == nil {
			return FormatXML
		}
	}

	// Check for form/query encoding (no spaces around = and &)
	if strings.Contains(trimmed, "=") && !strings.Contains(trimmed, " ") {
		if _, err := url.ParseQuery(trimmed); err == nil {
			// If it contains &, likely query/form parameters
			if strings.Contains(trimmed, "&") {
				return FormatForm
			}
			// Single key=value could be either, default to form
			return FormatForm
		}
	}

	// Default to JSON for unrecognized format
	return FormatJSON
}

// detectFormatFromString detects format from string data
func detectFormatFromString(data string) OutputFormat {
	return detectFormatFromBytes([]byte(data))
}

// FormatDataWithAutoDetection formats data with automatic format detection
func FormatDataWithAutoDetection(data interface{}) (*Request, error) {
	format := DetectFormat(data)
	return FormatData(format, data)
}

// FormatDataWithContentType formats data based on content type hint
func FormatDataWithContentType(data interface{}, contentType string) (*Request, error) {
	format := FormatFromContentType(contentType)
	if format == "" {
		// Fall back to auto-detection
		format = DetectFormat(data)
	}
	return FormatData(format, data)
}

// FormatFromContentType determines format from HTTP content type
func FormatFromContentType(contentType string) OutputFormat {
	// Normalize content type (remove charset, etc.)
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(ct, ";"); idx != -1 {
		ct = strings.TrimSpace(ct[:idx])
	}

	switch ct {
	case "application/json", "text/json":
		return FormatJSON
	case "application/x-www-form-urlencoded":
		return FormatForm
	case "application/xml", "text/xml":
		return FormatXML
	case "text/plain":
		// Could be form or query, need to inspect data
		return ""
	default:
		return ""
	}
}

// GetAllFormats returns all supported formats
func GetAllFormats() []OutputFormat {
	return []OutputFormat{FormatJSON, FormatForm, FormatQuery, FormatXML}
}

// GetFormatDescription returns a human-readable description of the format
func GetFormatDescription(format OutputFormat) string {
	switch format {
	case FormatJSON:
		return "JSON (JavaScript Object Notation)"
	case FormatForm:
		return "Form-encoded (application/x-www-form-urlencoded)"
	case FormatQuery:
		return "Query parameters (URL query string format)"
	case FormatXML:
		return "XML (eXtensible Markup Language)"
	default:
		return "Unknown format"
	}
}

// Format formats data to the specified format and returns the raw bytes
func Format(data interface{}, format string) ([]byte, error) {
	outputFormat, err := ParseFormat(format)
	if err != nil {
		return nil, err
	}

	formatter, err := NewFormatter(outputFormat)
	if err != nil {
		return nil, err
	}

	return formatter.Format(data)
}

// GetContentType returns the content type for the specified format
func GetContentType(format string) string {
	outputFormat, err := ParseFormat(format)
	if err != nil {
		return ""
	}

	formatter, err := NewFormatter(outputFormat)
	if err != nil {
		return ""
	}

	return formatter.ContentType()
}
