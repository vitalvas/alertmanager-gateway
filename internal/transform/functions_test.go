package transform

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringFunctions(t *testing.T) {
	funcs := GetTemplateFuncs()

	// Test upper
	result := funcs["upper"].(func(string) string)("hello")
	assert.Equal(t, "HELLO", result)

	// Test lower
	result = funcs["lower"].(func(string) string)("HELLO")
	assert.Equal(t, "hello", result)

	// Test trim
	result = funcs["trim"].(func(string) string)("  hello  ")
	assert.Equal(t, "hello", result)

	// Test replace
	result = funcs["replace"].(func(string, string, string) string)("hello world", "world", "go")
	assert.Equal(t, "hello go", result)

	// Test split
	parts := funcs["split"].(func(string, string) []string)("a,b,c", ",")
	assert.Equal(t, []string{"a", "b", "c"}, parts)

	// Test join
	joined := funcs["join"].(func([]string, string) string)([]string{"a", "b", "c"}, "-")
	assert.Equal(t, "a-b-c", joined)

	// Test contains
	contains := funcs["contains"].(func(string, string) bool)("hello world", "world")
	assert.True(t, contains)
}

func TestTimeFunctions(t *testing.T) {
	funcs := GetTemplateFuncs()

	// Test now
	nowFunc := funcs["now"].(func() time.Time)
	now := nowFunc()
	assert.WithinDuration(t, time.Now(), now, time.Second)

	// Test unixtime
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	unix := unixTime(testTime)
	assert.Equal(t, int64(1704110400), unix)

	// Test unixtime with string
	unix = unixTime("2024-01-01T12:00:00Z")
	assert.Equal(t, int64(1704110400), unix)

	// Test timeformat
	formatted := timeFormat("2006-01-02", testTime)
	assert.Equal(t, "2024-01-01", formatted)

	// Test duration
	dur, err := parseDuration("5m30s")
	require.NoError(t, err)
	assert.Equal(t, 5*time.Minute+30*time.Second, dur)
}

func TestEncodingFunctions(t *testing.T) {
	funcs := GetTemplateFuncs()

	// Test base64
	encoded := funcs["base64"].(func(string) string)("hello")
	assert.Equal(t, "aGVsbG8=", encoded)

	// Test base64dec
	decoded, err := funcs["base64dec"].(func(string) (string, error))("aGVsbG8=")
	require.NoError(t, err)
	assert.Equal(t, "hello", decoded)

	// Test md5
	hash := funcs["md5"].(func(string) string)("hello")
	assert.Equal(t, "5d41402abc4b2a76b9719d911017c592", hash)

	// Test jsonencode
	data := map[string]string{"key": "value"}
	json, err := funcs["jsonencode"].(func(interface{}) (string, error))(data)
	require.NoError(t, err)
	assert.Equal(t, `{"key":"value"}`, json)

	// Test jsondecode
	decoded2, err := funcs["jsondecode"].(func(string) (interface{}, error))(`{"key":"value"}`)
	require.NoError(t, err)
	result, ok := decoded2.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "value", result["key"])
}

func TestLogicFunctions(t *testing.T) {
	// Test default
	assert.Equal(t, "default", defaultValue("default", ""))
	assert.Equal(t, "value", defaultValue("default", "value"))

	// Test empty
	assert.True(t, isEmpty(""))
	assert.True(t, isEmpty(nil))
	assert.True(t, isEmpty([]interface{}{}))
	assert.False(t, isEmpty("value"))
	assert.False(t, isEmpty(1))

	// Test coalesce
	assert.Equal(t, "first", coalesce("", nil, "first", "second"))
	assert.Equal(t, "value", coalesce("value", "other"))

	// Test ternary
	assert.Equal(t, "yes", ternary(true, "yes", "no"))
	assert.Equal(t, "no", ternary(false, "yes", "no"))

	// Test regex
	match, err := regexMatch(`^\d+$`, "123")
	require.NoError(t, err)
	assert.True(t, match)

	// Test regexreplace
	result, err := regexReplace(`\d+`, "X", "abc123def456")
	require.NoError(t, err)
	assert.Equal(t, "abcXdefX", result)
}

func TestMathFunctions(t *testing.T) {
	// Test add
	assert.Equal(t, 5, add(2, 3))

	// Test subtract
	assert.Equal(t, 7, subtract(10, 3))

	// Test multiply
	assert.Equal(t, 12, multiply(3, 4))

	// Test divide
	result, err := divide(10, 2)
	require.NoError(t, err)
	assert.Equal(t, 5, result)

	_, err = divide(10, 0)
	assert.Error(t, err)

	// Test modulo
	result, err = modulo(10, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, result)
}

func TestMapSliceFunctions(t *testing.T) {
	// Test first
	assert.Equal(t, "a", first([]string{"a", "b", "c"}))
	assert.Equal(t, 1, first([]interface{}{1, 2, 3}))
	assert.Nil(t, first([]interface{}{}))

	// Test last
	assert.Equal(t, "c", last([]string{"a", "b", "c"}))
	assert.Equal(t, 3, last([]interface{}{1, 2, 3}))

	// Test index
	assert.Equal(t, "b", index([]string{"a", "b", "c"}, 1))
	assert.Nil(t, index([]string{"a", "b", "c"}, 10))

	// Test slice
	assert.Equal(t, []string{"b", "c"}, slice([]string{"a", "b", "c", "d"}, 1, 3))

	// Test dict
	d, err := dict("key1", "value1", "key2", "value2")
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"key1": "value1", "key2": "value2"}, d)

	// Test list
	l := list("a", "b", "c")
	assert.Equal(t, []interface{}{"a", "b", "c"}, l)

	// Test keys
	m := map[string]interface{}{"a": 1, "b": 2}
	k := keys(m)
	assert.Contains(t, k, "a")
	assert.Contains(t, k, "b")
	assert.Len(t, k, 2)

	// Test length
	assert.Equal(t, 5, length("hello"))
	assert.Equal(t, 3, length([]string{"a", "b", "c"}))
	assert.Equal(t, 2, length(map[string]interface{}{"a": 1, "b": 2}))
}

func TestAlertFunctions(t *testing.T) {
	// Test with map[string]interface{}
	alert := map[string]interface{}{
		"labels": map[string]string{
			"alertname": "TestAlert",
			"severity":  "critical",
		},
		"fingerprint": "abc123",
	}

	assert.Equal(t, "critical", getSeverity(alert))
	assert.Equal(t, "TestAlert", getAlertName(alert))
	assert.Equal(t, "abc123", getFingerprint(alert))

	// Test with map[string]string (labels directly)
	labels := map[string]string{
		"alertname": "TestAlert",
		"severity":  "warning",
	}

	assert.Equal(t, "warning", getSeverity(labels))
	assert.Equal(t, "TestAlert", getAlertName(labels))
}

func TestFormattingFunctions(t *testing.T) {
	// Test indent
	text := "line1\nline2\nline3"
	indented := indent(2, text)
	assert.Equal(t, "  line1\n  line2\n  line3", indented)

	// Test nindent
	nindented := nindent(2, text)
	assert.Equal(t, "\n  line1\n  line2\n  line3", nindented)
}

func TestURLFunctions(t *testing.T) {
	funcs := GetTemplateFuncs()

	// Test urlquery
	encoded := funcs["urlquery"].(func(string) string)("hello world")
	assert.Equal(t, "hello+world", encoded)

	// Test urldecode
	decoded, err := funcs["urldecode"].(func(string) (string, error))("hello+world")
	require.NoError(t, err)
	assert.Equal(t, "hello world", decoded)

	// Test urlparse
	parsed, err := parseURL("https://example.com/path?query=1")
	require.NoError(t, err)
	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "example.com", parsed.Host)
	assert.Equal(t, "/path", parsed.Path)
	assert.Equal(t, "query=1", parsed.RawQuery)
}

func TestEdgeCases(t *testing.T) {
	// Test unixTime with nil
	assert.Equal(t, int64(0), unixTime(nil))
	assert.Equal(t, int64(0), unixTime("invalid"))

	// Test timeFormat with invalid input
	assert.Equal(t, "", timeFormat("2006", nil))
	assert.Equal(t, "invalid", timeFormat("2006", "invalid"))

	// Test dict with odd number of arguments
	_, err := dict("key")
	assert.Error(t, err)

	// Test dict with non-string key
	_, err = dict(123, "value")
	assert.Error(t, err)
}
