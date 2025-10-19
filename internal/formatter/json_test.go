package formatter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONFormatter_Format(t *testing.T) {
	formatter := NewJSONFormatter()

	tests := []struct {
		name    string
		data    interface{}
		wantErr bool
		check   func(t *testing.T, result []byte)
	}{
		{
			name: "simple map",
			data: map[string]interface{}{
				"message": "hello",
				"count":   42,
			},
			check: func(t *testing.T, result []byte) {
				var m map[string]interface{}
				require.NoError(t, json.Unmarshal(result, &m))
				assert.Equal(t, "hello", m["message"])
				assert.Equal(t, float64(42), m["count"])
			},
		},
		{
			name: "struct",
			data: struct {
				Name  string `json:"name"`
				Value int    `json:"value"`
			}{
				Name:  "test",
				Value: 123,
			},
			check: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `"name":"test"`)
				assert.Contains(t, string(result), `"value":123`)
			},
		},
		{
			name: "array",
			data: []string{"one", "two", "three"},
			check: func(t *testing.T, result []byte) {
				var arr []string
				require.NoError(t, json.Unmarshal(result, &arr))
				assert.Equal(t, []string{"one", "two", "three"}, arr)
			},
		},
		{
			name: "pre-formatted JSON bytes",
			data: []byte(`{"already":"json"}`),
			check: func(t *testing.T, result []byte) {
				assert.Equal(t, `{"already":"json"}`, string(result))
			},
		},
		{
			name: "pre-formatted JSON string",
			data: `{"string":"json"}`,
			check: func(t *testing.T, result []byte) {
				assert.Equal(t, `{"string":"json"}`, string(result))
			},
		},
		{
			name:    "invalid pre-formatted JSON bytes",
			data:    []byte(`{invalid json}`),
			wantErr: true,
		},
		{
			name:    "invalid pre-formatted JSON string",
			data:    `{invalid json}`,
			wantErr: true,
		},
		{
			name: "HTML characters not escaped",
			data: map[string]string{
				"html": "<script>alert('test')</script>",
			},
			check: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `<script>alert('test')</script>`)
				assert.NotContains(t, string(result), `\u003c`) // Should not escape <
			},
		},
		{
			name: "special characters",
			data: map[string]string{
				"special": "line1\nline2\ttab",
			},
			check: func(t *testing.T, result []byte) {
				var m map[string]string
				require.NoError(t, json.Unmarshal(result, &m))
				assert.Equal(t, "line1\nline2\ttab", m["special"])
			},
		},
		{
			name: "nil data",
			data: nil,
			check: func(t *testing.T, result []byte) {
				assert.Equal(t, "null", string(result))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatter.Format(tt.data)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)

				// Verify no trailing newline
				assert.NotEqual(t, '\n', result[len(result)-1])

				if tt.check != nil {
					tt.check(t, result)
				}
			}
		})
	}
}

func TestJSONFormatter_FormatWithIndent(t *testing.T) {
	formatter := NewJSONFormatterWithIndent()

	data := map[string]interface{}{
		"name": "test",
		"nested": map[string]interface{}{
			"value": 42,
		},
	}

	result, err := formatter.Format(data)
	require.NoError(t, err)

	// Check indentation
	assert.Contains(t, string(result), "  \"name\"")
	assert.Contains(t, string(result), "  \"nested\"")
	assert.Contains(t, string(result), "    \"value\"")

	// Verify valid JSON
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(result, &m))
}

func TestJSONFormatter_Properties(t *testing.T) {
	t.Run("content type", func(t *testing.T) {
		formatter := NewJSONFormatter()
		assert.Equal(t, "application/json", formatter.ContentType())
	})

	t.Run("name", func(t *testing.T) {
		formatter := NewJSONFormatter()
		assert.Equal(t, "json", formatter.Name())
	})
}
