package formatter

import (
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewXMLFormatter(t *testing.T) {
	formatter := NewXMLFormatter()
	require.NotNil(t, formatter)
	assert.Equal(t, "xml", formatter.Name())
	assert.Equal(t, "application/xml", formatter.ContentType())
}

func TestNewXMLFormatterWithIndent(t *testing.T) {
	formatter := NewXMLFormatterWithIndent()
	require.NotNil(t, formatter)
	assert.Equal(t, "xml", formatter.Name())
}

func TestNewXMLFormatterWithRoot(t *testing.T) {
	formatter := NewXMLFormatterWithRoot("alert")
	require.NotNil(t, formatter)

	data := map[string]string{"name": "test"}
	result, err := formatter.Format(data)
	require.NoError(t, err)

	// Should use custom root element
	assert.Contains(t, string(result), "<alert>")
	assert.Contains(t, string(result), "</alert>")
}

func TestXMLFormatter_Format(t *testing.T) {
	formatter := NewXMLFormatter()

	tests := []struct {
		name        string
		data        interface{}
		expectError bool
		checkFunc   func(t *testing.T, result []byte)
	}{
		{
			name: "simple map",
			data: map[string]interface{}{
				"name":  "test",
				"value": "123",
			},
			checkFunc: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "<name>test</name>")
				assert.Contains(t, string(result), "<value>123</value>")
				assert.Contains(t, string(result), "<?xml version=\"1.0\" encoding=\"UTF-8\"?>")
			},
		},
		{
			name: "nested map",
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "john",
					"age":  30,
				},
			},
			checkFunc: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "<user>")
				assert.Contains(t, string(result), "<name>john</name>")
				assert.Contains(t, string(result), "<age>30</age>")
				assert.Contains(t, string(result), "</user>")
			},
		},
		{
			name: "array values",
			data: map[string]interface{}{
				"tags": []string{"go", "test", "xml"},
			},
			checkFunc: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "<tags>")
				assert.Contains(t, string(result), "<tag>go</tag>")
				assert.Contains(t, string(result), "<tag>test</tag>")
				assert.Contains(t, string(result), "<tag>xml</tag>")
				assert.Contains(t, string(result), "</tags>")
			},
		},
		{
			name: "mixed types",
			data: map[string]interface{}{
				"string": "hello",
				"int":    42,
				"float":  3.14,
				"bool":   true,
			},
			checkFunc: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "<string>hello</string>")
				assert.Contains(t, string(result), "<int>42</int>")
				assert.Contains(t, string(result), "<float>3.14</float>")
				assert.Contains(t, string(result), "<bool>true</bool>")
			},
		},
		{
			name: "struct with xml tags",
			data: struct {
				Name     string `xml:"full_name"`
				Age      int    `xml:"user_age"`
				Email    string `xml:"-"`             // Should be skipped
				Internal string `xml:"internal,attr"` // Should be attribute
			}{
				Name:     "John Doe",
				Age:      25,
				Email:    "john@example.com",
				Internal: "secret",
			},
			checkFunc: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "<full_name>John Doe</full_name>")
				assert.Contains(t, string(result), "<user_age>25</user_age>")
				assert.NotContains(t, string(result), "john@example.com")
				assert.Contains(t, string(result), `internal="secret"`)
			},
		},
		{
			name: "pre-formatted string",
			data: "<message>Hello World</message>",
			checkFunc: func(t *testing.T, result []byte) {
				assert.Equal(t, "<message>Hello World</message>", string(result))
			},
		},
		{
			name: "pre-formatted bytes",
			data: []byte("<?xml version=\"1.0\"?><test>data</test>"),
			checkFunc: func(t *testing.T, result []byte) {
				assert.Equal(t, "<?xml version=\"1.0\"?><test>data</test>", string(result))
			},
		},
		{
			name:        "invalid pre-formatted string",
			data:        "<invalid>unclosed tag",
			expectError: true,
		},
		{
			name:        "unsupported type",
			data:        make(chan int),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatter.Format(tt.data)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				// Validate that result is valid XML
				var temp interface{}
				err = xml.Unmarshal(result, &temp)
				require.NoError(t, err, "Result should be valid XML")

				if tt.checkFunc != nil {
					tt.checkFunc(t, result)
				}
			}
		})
	}
}

func TestXMLFormatter_WithIndent(t *testing.T) {
	formatter := NewXMLFormatterWithIndent()

	data := map[string]interface{}{
		"alert": map[string]interface{}{
			"name":   "TestAlert",
			"status": "firing",
		},
	}

	result, err := formatter.Format(data)
	require.NoError(t, err)

	resultStr := string(result)

	// Should contain indentation
	assert.Contains(t, resultStr, "\n")
	assert.Contains(t, resultStr, "  ") // Two spaces for indentation

	// Should be valid XML
	var temp interface{}
	err = xml.Unmarshal(result, &temp)
	require.NoError(t, err)
}

func TestXMLFormatter_ComplexNesting(t *testing.T) {
	formatter := NewXMLFormatter()

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

	resultStr := string(result)

	// Check structure
	assert.Contains(t, resultStr, "<alert>")
	assert.Contains(t, resultStr, "<name>HighCPU</name>")
	assert.Contains(t, resultStr, "<labels>")
	assert.Contains(t, resultStr, "<severity>critical</severity>")
	assert.Contains(t, resultStr, "<alerts>")
	assert.Contains(t, resultStr, "<alert>") // Singularized from "alerts" -> "alert"
	assert.Contains(t, resultStr, "<fingerprint>abc123</fingerprint>")

	// Validate XML structure
	var temp interface{}
	err = xml.Unmarshal(result, &temp)
	require.NoError(t, err)
}

func TestXMLFormatter_SpecialCharacters(t *testing.T) {
	formatter := NewXMLFormatter()

	data := map[string]interface{}{
		"message":     "Hello & Welcome",
		"html_tag":    "<script>alert('test')</script>",
		"quotes":      `"single" and 'double' quotes`,
		"unicode":     "Hello 世界",
		"numbers":     "123.45",
		"invalid_xml": "Invalid\x00character",
	}

	result, err := formatter.Format(data)
	require.NoError(t, err)

	// Should be valid XML despite special characters
	var temp interface{}
	err = xml.Unmarshal(result, &temp)
	require.NoError(t, err)

	resultStr := string(result)

	// Check that special characters are properly escaped
	assert.Contains(t, resultStr, "&amp;") // & should be escaped
	assert.Contains(t, resultStr, "&lt;")  // < should be escaped
	assert.Contains(t, resultStr, "&gt;")  // > should be escaped
}

func TestXMLFormatter_ElementNameSanitization(t *testing.T) {
	formatter := NewXMLFormatter()

	data := map[string]interface{}{
		"valid-name":    "test1",
		"123invalid":    "test2", // Starts with number
		"special@chars": "test3", // Contains special chars
		"":              "test4", // Empty name
		"with spaces":   "test5", // Contains spaces
	}

	result, err := formatter.Format(data)
	require.NoError(t, err)

	// Should be valid XML
	var temp interface{}
	err = xml.Unmarshal(result, &temp)
	require.NoError(t, err)

	resultStr := string(result)

	// Check that names are sanitized
	assert.Contains(t, resultStr, "<valid-name>test1</valid-name>")
	assert.Contains(t, resultStr, "<_23invalid>test2</_23invalid>") // numbers at start get _ prefix
	assert.Contains(t, resultStr, "<special_chars>test3</special_chars>")
	assert.Contains(t, resultStr, "<item>test4</item>")
	assert.Contains(t, resultStr, "<with_spaces>test5</with_spaces>")
}

func TestXMLFormatter_ArraySingularization(t *testing.T) {
	formatter := NewXMLFormatter()

	data := map[string]interface{}{
		"alerts":    []string{"alert1", "alert2"},
		"boxes":     []string{"box1", "box2"},
		"companies": []string{"comp1", "comp2"},
		"wolves":    []string{"wolf1", "wolf2"},
		"dishes":    []string{"dish1", "dish2"},
		"watches":   []string{"watch1", "watch2"},
		"items":     []string{"item1", "item2"},
		"unknowns":  []string{"unknown1", "unknown2"},
	}

	result, err := formatter.Format(data)
	require.NoError(t, err)

	resultStr := string(result)

	// Check singularization
	assert.Contains(t, resultStr, "<alert>")   // alerts -> alert (remove s)
	assert.Contains(t, resultStr, "<boxe>")    // boxes -> boxe (remove s, not proper English but algorithmic)
	assert.Contains(t, resultStr, "<company>") // companies -> company (ies -> y)
	assert.Contains(t, resultStr, "<wolf>")    // wolves -> wolf (ves -> f)
	assert.Contains(t, resultStr, "<dish>")    // dishes -> dish (es removal)
	assert.Contains(t, resultStr, "<watch>")   // watches -> watch (es removal)
	assert.Contains(t, resultStr, "<item>")    // items -> item (s removal)
	assert.Contains(t, resultStr, "<unknown>") // unknowns -> unknown (s removal)
}

func TestXMLFormatter_EmptyAndNilValues(t *testing.T) {
	formatter := NewXMLFormatter()

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

			// Should produce valid XML
			var temp interface{}
			err = xml.Unmarshal(result, &temp)
			require.NoError(t, err)

			// Should contain XML declaration and root element
			resultStr := string(result)
			assert.Contains(t, resultStr, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>")
			assert.Contains(t, resultStr, "<root>")
			assert.Contains(t, resultStr, "</root>")
		})
	}
}
