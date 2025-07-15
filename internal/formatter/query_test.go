package formatter

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewQueryFormatter(t *testing.T) {
	formatter := NewQueryFormatter()
	require.NotNil(t, formatter)
	assert.Equal(t, "query", formatter.Name())
	assert.Equal(t, "", formatter.ContentType()) // Query params don't have content type
}

func TestNewQueryFormatterWithArrayFlattening(t *testing.T) {
	formatter := NewQueryFormatterWithArrayFlattening()
	require.NotNil(t, formatter)
	assert.Equal(t, "query", formatter.Name())
}

func TestQueryFormatter_Format(t *testing.T) {
	formatter := NewQueryFormatter()

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
			name: "nested map with dot notation",
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "john",
					"age":  30,
				},
			},
			expected: "user.age=30&user.name=john",
		},
		{
			name: "array values (repeated parameters)",
			data: map[string]interface{}{
				"tags": []string{"go", "test", "query"},
			},
			expected: "tags=go&tags=test&tags=query",
		},
		{
			name: "primitive array values",
			data: map[string]interface{}{
				"numbers": []int{1, 2, 3},
			},
			expected: "numbers=1&numbers=2&numbers=3",
		},
		{
			name: "mixed types",
			data: map[string]interface{}{
				"string": "hello",
				"int":    42,
				"float":  3.14,
				"bool":   true,
			},
			expected: "bool=true&float=3.14&int=42&string=hello",
		},
		{
			name: "struct with query tags",
			data: struct {
				Name     string `query:"full_name"`
				Age      int    `query:"user_age"`
				Email    string `query:"-"` // Should be skipped
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
			name: "struct with form tags fallback",
			data: struct {
				Name string `form:"user_name"`
				ID   int    `form:"user_id"`
			}{
				Name: "Jane",
				ID:   123,
			},
			expected: "user_id=123&user_name=Jane",
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
				expectedValues, err := url.ParseQuery(tt.expected)
				require.NoError(t, err)

				actualValues, err := url.ParseQuery(string(result))
				require.NoError(t, err)

				assert.Equal(t, expectedValues, actualValues)
			}
		})
	}
}

func TestQueryFormatter_ArrayFlattening(t *testing.T) {
	flatteningFormatter := NewQueryFormatterWithArrayFlattening()
	normalFormatter := NewQueryFormatter()

	data := map[string]interface{}{
		"items":   []string{"a", "b", "c"},
		"numbers": []int{1, 2, 3},
	}

	// Test flattening formatter (uses indexed notation)
	flatResult, err := flatteningFormatter.Format(data)
	require.NoError(t, err)

	flatValues, err := url.ParseQuery(string(flatResult))
	require.NoError(t, err)

	// Should have indexed parameters
	assert.Equal(t, "a", flatValues.Get("items[0]"))
	assert.Equal(t, "b", flatValues.Get("items[1]"))
	assert.Equal(t, "c", flatValues.Get("items[2]"))

	// Test normal formatter (uses repeated parameters)
	normalResult, err := normalFormatter.Format(data)
	require.NoError(t, err)

	normalValues, err := url.ParseQuery(string(normalResult))
	require.NoError(t, err)

	// Should have repeated parameters
	assert.Equal(t, []string{"a", "b", "c"}, normalValues["items"])
	assert.Equal(t, []string{"1", "2", "3"}, normalValues["numbers"])
}

func TestQueryFormatter_ComplexNesting(t *testing.T) {
	formatter := NewQueryFormatter()

	data := map[string]interface{}{
		"alert": map[string]interface{}{
			"name":   "HighMemory",
			"status": "firing",
			"labels": map[string]string{
				"severity": "warning",
				"instance": "server2",
			},
		},
		"count": 5,
		"tags":  []string{"memory", "warning"},
	}

	result, err := formatter.Format(data)
	require.NoError(t, err)

	values, err := url.ParseQuery(string(result))
	require.NoError(t, err)

	// Check dot notation for nested objects
	assert.Equal(t, "HighMemory", values.Get("alert.name"))
	assert.Equal(t, "firing", values.Get("alert.status"))
	assert.Equal(t, "warning", values.Get("alert.labels.severity"))
	assert.Equal(t, "server2", values.Get("alert.labels.instance"))

	// Check simple values
	assert.Equal(t, "5", values.Get("count"))

	// Check arrays (repeated parameters)
	assert.Equal(t, []string{"memory", "warning"}, values["tags"])
}

func TestQueryFormatter_ComplexArrays(t *testing.T) {
	// Test arrays with complex objects
	normalFormatter := NewQueryFormatter()
	flatteningFormatter := NewQueryFormatterWithArrayFlattening()

	data := map[string]interface{}{
		"alerts": []map[string]interface{}{
			{
				"name":   "alert1",
				"status": "firing",
			},
			{
				"name":   "alert2",
				"status": "resolved",
			},
		},
	}

	// Normal formatter should use indexed notation for complex objects
	normalResult, err := normalFormatter.Format(data)
	require.NoError(t, err)

	normalValues, err := url.ParseQuery(string(normalResult))
	require.NoError(t, err)

	assert.Equal(t, "alert1", normalValues.Get("alerts[0].name"))
	assert.Equal(t, "firing", normalValues.Get("alerts[0].status"))
	assert.Equal(t, "alert2", normalValues.Get("alerts[1].name"))
	assert.Equal(t, "resolved", normalValues.Get("alerts[1].status"))

	// Flattening formatter should also use indexed notation
	flatResult, err := flatteningFormatter.Format(data)
	require.NoError(t, err)

	flatValues, err := url.ParseQuery(string(flatResult))
	require.NoError(t, err)

	assert.Equal(t, "alert1", flatValues.Get("alerts[0].name"))
	assert.Equal(t, "firing", flatValues.Get("alerts[0].status"))
	assert.Equal(t, "alert2", flatValues.Get("alerts[1].name"))
	assert.Equal(t, "resolved", flatValues.Get("alerts[1].status"))
}

func TestQueryFormatter_EmptyAndNilValues(t *testing.T) {
	formatter := NewQueryFormatter()

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

			// Should result in empty query string or minimal data
			t.Logf("Result for %s: %s", tt.name, string(result))
		})
	}
}
