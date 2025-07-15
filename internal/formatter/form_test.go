package formatter

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFormFormatter(t *testing.T) {
	formatter := NewFormFormatter()
	require.NotNil(t, formatter)
	assert.Equal(t, "form", formatter.Name())
	assert.Equal(t, "application/x-www-form-urlencoded", formatter.ContentType())
}

func TestFormFormatter_Format(t *testing.T) {
	formatter := NewFormFormatter()

	tests := []struct {
		name     string
		data     interface{}
		expected string
		wantErr  bool
	}{
		{
			name: "simple map",
			data: map[string]interface{}{
				"name":  "test",
				"value": "123",
			},
			expected: "name=test&value=123",
		},
		{
			name: "nested map",
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "john",
					"age":  30,
				},
			},
			expected: "user[age]=30&user[name]=john",
		},
		{
			name: "array values",
			data: map[string]interface{}{
				"tags": []string{"go", "test", "form"},
			},
			expected: "tags[0]=go&tags[1]=test&tags[2]=form",
		},
		{
			name: "mixed types",
			data: map[string]interface{}{
				"string":  "hello",
				"int":     42,
				"float":   3.14,
				"bool":    true,
				"nothing": nil,
			},
			expected: "bool=true&float=3.14&int=42&nothing=&string=hello",
		},
		{
			name: "struct with tags",
			data: struct {
				Name     string `form:"full_name"`
				Age      int    `form:"user_age"`
				Email    string `form:"-"` // Should be skipped
				Internal string // Should use field name
			}{
				Name:     "John Doe",
				Age:      25,
				Email:    "john@example.com",
				Internal: "secret",
			},
			expected: "Internal=secret&full_name=John+Doe&user_age=25",
		},
		{
			name:     "pre-formatted string",
			data:     "key1=value1&key2=value2",
			expected: "key1=value1&key2=value2",
		},
		{
			name:     "pre-formatted bytes",
			data:     []byte("name=test&id=123"),
			expected: "name=test&id=123",
		},
		{
			name: "URL encoding",
			data: map[string]string{
				"message": "hello world!",
				"special": "key=value&more=data",
			},
			expected: "message=hello+world%21&special=key%3Dvalue%26more%3Ddata",
		},
		{
			name:     "invalid pre-formatted string",
			data:     "invalid=data=extra=equals",
			expected: "invalid=data%3Dextra%3Dequals", // url.ParseQuery can handle this
		},
		{
			name:    "unsupported type",
			data:    make(chan int),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatter.Format(tt.data)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				// Parse both expected and actual to compare as url.Values
				// This handles different ordering of parameters
				expectedValues, err := url.ParseQuery(tt.expected)
				require.NoError(t, err)

				actualValues, err := url.ParseQuery(string(result))
				require.NoError(t, err)

				assert.Equal(t, expectedValues, actualValues)
			}
		})
	}
}

func TestFormFormatter_ComplexNesting(t *testing.T) {
	formatter := NewFormFormatter()

	data := map[string]interface{}{
		"alert": map[string]interface{}{
			"name":   "HighCPU",
			"status": "firing",
			"labels": map[string]string{
				"severity": "critical",
				"instance": "server1",
			},
			"annotations": map[string]string{
				"summary":     "CPU usage high",
				"description": "CPU > 90%",
			},
		},
		"receiver": "webhook",
		"alerts": []map[string]interface{}{
			{
				"fingerprint": "abc123",
				"status":      "firing",
			},
			{
				"fingerprint": "def456",
				"status":      "resolved",
			},
		},
	}

	result, err := formatter.Format(data)
	require.NoError(t, err)

	// Parse the result to verify structure
	values, err := url.ParseQuery(string(result))
	require.NoError(t, err)

	// Check some expected keys exist
	assert.Contains(t, values, "alert[name]")
	assert.Contains(t, values, "alert[labels][severity]")
	assert.Contains(t, values, "alerts[0][fingerprint]")
	assert.Contains(t, values, "alerts[1][status]")
	assert.Contains(t, values, "receiver")

	// Check values
	assert.Equal(t, "HighCPU", values.Get("alert[name]"))
	assert.Equal(t, "critical", values.Get("alert[labels][severity]"))
	assert.Equal(t, "abc123", values.Get("alerts[0][fingerprint]"))
	assert.Equal(t, "resolved", values.Get("alerts[1][status]"))
	assert.Equal(t, "webhook", values.Get("receiver"))
}

func TestFormFormatter_EmptyAndNilValues(t *testing.T) {
	formatter := NewFormFormatter()

	tests := []struct {
		name string
		data interface{}
	}{
		{
			name: "nil data",
			data: nil,
		},
		{
			name: "empty map",
			data: map[string]interface{}{},
		},
		{
			name: "empty struct",
			data: struct{}{},
		},
		{
			name: "nil pointer",
			data: (*string)(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatter.Format(tt.data)
			require.NoError(t, err)

			// Should result in empty or minimal form data
			values, err := url.ParseQuery(string(result))
			require.NoError(t, err)

			// Should be empty or contain only empty values
			for key, vals := range values {
				t.Logf("Key: %s, Values: %v", key, vals)
			}
		})
	}
}
