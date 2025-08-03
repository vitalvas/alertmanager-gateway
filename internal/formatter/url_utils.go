package formatter

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// validateAndHandlePreformatted checks if data is already formatted and validates it
func validateAndHandlePreformatted(data interface{}) ([]byte, bool, error) {
	// Check if data is already a string (pre-formatted)
	if str, ok := data.(string); ok {
		// Validate it's properly encoded by parsing it
		if _, err := url.ParseQuery(str); err != nil {
			return nil, false, fmt.Errorf("invalid encoded string: %w", err)
		}
		return []byte(str), true, nil
	}

	// Check if data is already a byte slice (pre-formatted)
	if bytes, ok := data.([]byte); ok {
		// Validate it's properly encoded by parsing it
		if _, err := url.ParseQuery(string(bytes)); err != nil {
			return nil, false, fmt.Errorf("invalid encoded data: %w", err)
		}
		return bytes, true, nil
	}

	return nil, false, nil
}

// buildEncodedString builds URL-encoded string from values with sorted keys
func buildEncodedString(values url.Values) []byte {
	// Sort keys for consistent output
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Build encoded string
	var parts []string
	for _, key := range keys {
		for _, value := range values[key] {
			parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(value))
		}
	}

	return []byte(strings.Join(parts, "&"))
}
