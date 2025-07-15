package transform

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
)

// GetTemplateFuncs returns custom template functions
func GetTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		// String functions
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"title":      toTitle,
		"trim":       strings.TrimSpace,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,
		"replace":    strings.ReplaceAll,
		"split":      strings.Split,
		"join":       strings.Join,
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"repeat":     strings.Repeat,

		// URL functions
		"urlquery":  url.QueryEscape,
		"urlencode": url.QueryEscape,
		"urldecode": url.QueryUnescape,
		"urlparse":  parseURL,

		// Time functions
		"now":        time.Now,
		"unixtime":   unixTime,
		"timeformat": timeFormat,
		"duration":   parseDuration,
		"since":      time.Since,
		"until":      time.Until,

		// Encoding functions
		"base64":     base64Encode,
		"base64dec":  base64Decode,
		"md5":        md5Hash,
		"jsonencode": jsonEncode,
		"jsondecode": jsonDecode,

		// Logic functions
		"default":      defaultValue,
		"empty":        isEmpty,
		"coalesce":     coalesce,
		"ternary":      ternary,
		"regex":        regexMatch,
		"regexreplace": regexReplace,

		// Math functions
		"add":    add,
		"sub":    subtract,
		"mul":    multiply,
		"div":    divide,
		"divide": divide,
		"mod":    modulo,

		// Map/slice functions
		"first":  first,
		"last":   last,
		"index":  index,
		"slice":  slice,
		"dict":   dict,
		"list":   list,
		"keys":   keys,
		"values": values,
		"len":    length,

		// Alert-specific functions
		"severity":    getSeverity,
		"alertname":   getAlertName,
		"fingerprint": getFingerprint,
		"startsAt":    getStartsAt,
		"endsAt":      getEndsAt,
		"labels":      getLabels,
		"annotations": getAnnotations,

		// Formatting functions
		"indent":  indent,
		"nindent": nindent,
		"printf":  fmt.Sprintf,
		"println": fmt.Sprintln,
	}
}

// String functions

func toTitle(s string) string {
	// Simple title case implementation
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

func parseURL(urlStr string) (*url.URL, error) {
	return url.Parse(urlStr)
}

// Time functions

func unixTime(t interface{}) int64 {
	switch v := t.(type) {
	case time.Time:
		return v.Unix()
	case *time.Time:
		if v != nil {
			return v.Unix()
		}
	case string:
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			return parsed.Unix()
		}
	}
	return 0
}

func timeFormat(format string, t interface{}) string {
	var tm time.Time
	switch v := t.(type) {
	case time.Time:
		tm = v
	case *time.Time:
		if v != nil {
			tm = *v
		}
	case string:
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return v
		}
		tm = parsed
	default:
		return ""
	}
	return tm.Format(format)
}

func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// Encoding functions

func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func base64Decode(s string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func md5Hash(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func jsonEncode(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func jsonDecode(s string) (interface{}, error) {
	var v interface{}
	err := json.Unmarshal([]byte(s), &v)
	return v, err
}

// Logic functions

func defaultValue(defaultVal, val interface{}) interface{} {
	if isEmpty(val) {
		return defaultVal
	}
	return val
}

func isEmpty(val interface{}) bool {
	switch v := val.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []interface{}:
		return len(v) == 0
	case map[string]interface{}:
		return len(v) == 0
	case bool:
		return !v
	case int, int8, int16, int32, int64:
		return v == 0
	case float32, float64:
		return v == 0.0
	}
	return false
}

func coalesce(vals ...interface{}) interface{} {
	for _, val := range vals {
		if !isEmpty(val) {
			return val
		}
	}
	return nil
}

func ternary(condition bool, trueVal, falseVal interface{}) interface{} {
	if condition {
		return trueVal
	}
	return falseVal
}

func regexMatch(pattern, s string) (bool, error) {
	return regexp.MatchString(pattern, s)
}

func regexReplace(pattern, replacement, s string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	return re.ReplaceAllString(s, replacement), nil
}

// Math functions

func add(a, b int) int {
	return a + b
}

func subtract(a, b int) int {
	return a - b
}

func multiply(a, b int) int {
	return a * b
}

func divide(a, b int) (int, error) {
	if b == 0 {
		return 0, fmt.Errorf("division by zero")
	}
	return a / b, nil
}

func modulo(a, b int) (int, error) {
	if b == 0 {
		return 0, fmt.Errorf("division by zero")
	}
	return a % b, nil
}

// Map/slice functions

func first(v interface{}) interface{} {
	switch arr := v.(type) {
	case []interface{}:
		if len(arr) > 0 {
			return arr[0]
		}
	case []string:
		if len(arr) > 0 {
			return arr[0]
		}
	}
	return nil
}

func last(v interface{}) interface{} {
	switch arr := v.(type) {
	case []interface{}:
		if len(arr) > 0 {
			return arr[len(arr)-1]
		}
	case []string:
		if len(arr) > 0 {
			return arr[len(arr)-1]
		}
	}
	return nil
}

func index(v interface{}, i int) interface{} {
	switch arr := v.(type) {
	case []interface{}:
		if i >= 0 && i < len(arr) {
			return arr[i]
		}
	case []string:
		if i >= 0 && i < len(arr) {
			return arr[i]
		}
	}
	return nil
}

func slice(v interface{}, start, end int) interface{} {
	switch arr := v.(type) {
	case []interface{}:
		if start < 0 {
			start = 0
		}
		if end > len(arr) {
			end = len(arr)
		}
		if start < end {
			return arr[start:end]
		}
	case []string:
		if start < 0 {
			start = 0
		}
		if end > len(arr) {
			end = len(arr)
		}
		if start < end {
			return arr[start:end]
		}
	}
	return nil
}

func dict(values ...interface{}) (map[string]interface{}, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict requires even number of arguments")
	}

	result := make(map[string]interface{})
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict keys must be strings")
		}
		result[key] = values[i+1]
	}
	return result, nil
}

func list(values ...interface{}) []interface{} {
	return values
}

func keys(m map[string]interface{}) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

func values(m map[string]interface{}) []interface{} {
	result := make([]interface{}, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	return result
}

func length(v interface{}) int {
	switch val := v.(type) {
	case string:
		return len(val)
	case []interface{}:
		return len(val)
	case []string:
		return len(val)
	case map[string]interface{}:
		return len(val)
	case map[string]string:
		return len(val)
	case []alertmanager.Alert:
		return len(val)
	default:
		// Use reflection for other slice types
		if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice {
			return rv.Len()
		}
	}
	return 0
}

// Alert-specific functions

func getSeverity(v interface{}) string {
	switch alert := v.(type) {
	case map[string]interface{}:
		if labels, ok := alert["labels"].(map[string]string); ok {
			return labels["severity"]
		}
	case map[string]string:
		return alert["severity"]
	}
	return ""
}

func getAlertName(v interface{}) string {
	switch alert := v.(type) {
	case map[string]interface{}:
		if labels, ok := alert["labels"].(map[string]string); ok {
			return labels["alertname"]
		}
	case map[string]string:
		return alert["alertname"]
	}
	return ""
}

func getFingerprint(v interface{}) string {
	if alert, ok := v.(map[string]interface{}); ok {
		if fp, ok := alert["fingerprint"].(string); ok {
			return fp
		}
	}
	return ""
}

func getStartsAt(v interface{}) time.Time {
	if alert, ok := v.(map[string]interface{}); ok {
		if t, ok := alert["startsAt"].(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

func getEndsAt(v interface{}) time.Time {
	if alert, ok := v.(map[string]interface{}); ok {
		if t, ok := alert["endsAt"].(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

func getLabels(v interface{}) map[string]string {
	if alert, ok := v.(map[string]interface{}); ok {
		if labels, ok := alert["labels"].(map[string]string); ok {
			return labels
		}
	}
	return map[string]string{}
}

func getAnnotations(v interface{}) map[string]string {
	if alert, ok := v.(map[string]interface{}); ok {
		if annotations, ok := alert["annotations"].(map[string]string); ok {
			return annotations
		}
	}
	return map[string]string{}
}

// Formatting functions

func indent(spaces int, s string) string {
	pad := strings.Repeat(" ", spaces)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func nindent(spaces int, s string) string {
	return "\n" + indent(spaces, s)
}
