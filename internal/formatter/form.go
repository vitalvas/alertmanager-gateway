package formatter

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// FormFormatter formats data as application/x-www-form-urlencoded
type FormFormatter struct{}

// NewFormFormatter creates a new form formatter
func NewFormFormatter() *FormFormatter {
	return &FormFormatter{}
}

// Format converts data to form-encoded format
func (f *FormFormatter) Format(data interface{}) ([]byte, error) {
	// Check if data is already pre-formatted
	if result, handled, err := validateAndHandlePreformatted(data); handled || err != nil {
		if err != nil {
			return nil, fmt.Errorf("invalid form data: %w", err)
		}
		return result, nil
	}

	// Convert data to form parameters
	values, err := f.toFormValues(data, "")
	if err != nil {
		return nil, fmt.Errorf("failed to convert to form values: %w", err)
	}

	return buildEncodedString(values), nil
}

// ContentType returns the content type for form data
func (f *FormFormatter) ContentType() string {
	return "application/x-www-form-urlencoded"
}

// Name returns the formatter name
func (f *FormFormatter) Name() string {
	return string(FormatForm)
}

// toFormValues converts data to url.Values recursively
func (f *FormFormatter) toFormValues(data interface{}, prefix string) (url.Values, error) {
	values := make(url.Values)

	if data == nil {
		if prefix != "" {
			key := f.buildKey(prefix, "")
			values.Add(key, "")
		}
		return values, nil
	}

	val := reflect.ValueOf(data)
	typ := reflect.TypeOf(data)

	// Dereference pointers
	for val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return values, nil
		}
		val = val.Elem()
		typ = typ.Elem()
	}

	switch val.Kind() {
	case reflect.Map:
		return f.handleMap(val, prefix)

	case reflect.Struct:
		return f.handleStruct(val, typ, prefix)

	case reflect.Slice, reflect.Array:
		return f.handleSlice(val, prefix)

	case reflect.String:
		key := f.buildKey(prefix, "")
		values.Add(key, val.String())

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		key := f.buildKey(prefix, "")
		values.Add(key, strconv.FormatInt(val.Int(), 10))

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		key := f.buildKey(prefix, "")
		values.Add(key, strconv.FormatUint(val.Uint(), 10))

	case reflect.Float32, reflect.Float64:
		key := f.buildKey(prefix, "")
		values.Add(key, strconv.FormatFloat(val.Float(), 'f', -1, 64))

	case reflect.Bool:
		key := f.buildKey(prefix, "")
		values.Add(key, strconv.FormatBool(val.Bool()))

	case reflect.Interface:
		if !val.IsNil() {
			return f.toFormValues(val.Interface(), prefix)
		}
		// For nil interface values, add empty parameter
		if prefix != "" {
			key := f.buildKey(prefix, "")
			values.Add(key, "")
		}

	default:
		return nil, fmt.Errorf("unsupported type: %v", val.Kind())
	}

	return values, nil
}

// handleMap processes map types
func (f *FormFormatter) handleMap(val reflect.Value, prefix string) (url.Values, error) {
	values := make(url.Values)

	for _, key := range val.MapKeys() {
		keyStr := fmt.Sprintf("%v", key.Interface())
		fieldPrefix := f.buildKey(prefix, keyStr)

		fieldValues, err := f.toFormValues(val.MapIndex(key).Interface(), fieldPrefix)
		if err != nil {
			return nil, err
		}

		for k, v := range fieldValues {
			for _, value := range v {
				values.Add(k, value)
			}
		}
	}

	return values, nil
}

// handleStruct processes struct types
func (f *FormFormatter) handleStruct(val reflect.Value, typ reflect.Type, prefix string) (url.Values, error) {
	values := make(url.Values)

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Skip unexported fields
		if !field.CanInterface() {
			continue
		}

		// Get field name from tag or use field name
		fieldName := fieldType.Name
		if tag := fieldType.Tag.Get("form"); tag != "" {
			if tag == "-" {
				continue // Skip this field
			}
			if idx := strings.Index(tag, ","); idx != -1 {
				fieldName = tag[:idx]
			} else {
				fieldName = tag
			}
		} else if tag := fieldType.Tag.Get("json"); tag != "" {
			if tag == "-" {
				continue // Skip this field
			}
			if idx := strings.Index(tag, ","); idx != -1 {
				fieldName = tag[:idx]
			} else {
				fieldName = tag
			}
		}

		if fieldName == "" {
			fieldName = fieldType.Name
		}

		fieldPrefix := f.buildKey(prefix, fieldName)

		fieldValues, err := f.toFormValues(field.Interface(), fieldPrefix)
		if err != nil {
			return nil, err
		}

		for k, v := range fieldValues {
			for _, value := range v {
				values.Add(k, value)
			}
		}
	}

	return values, nil
}

// handleSlice processes slice and array types
func (f *FormFormatter) handleSlice(val reflect.Value, prefix string) (url.Values, error) {
	values := make(url.Values)

	for i := 0; i < val.Len(); i++ {
		indexPrefix := f.buildKey(prefix, strconv.Itoa(i))

		itemValues, err := f.toFormValues(val.Index(i).Interface(), indexPrefix)
		if err != nil {
			return nil, err
		}

		for k, v := range itemValues {
			for _, value := range v {
				values.Add(k, value)
			}
		}
	}

	return values, nil
}

// buildKey constructs form parameter keys with proper nesting
func (f *FormFormatter) buildKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	if key == "" {
		return prefix
	}
	return prefix + "[" + key + "]"
}
