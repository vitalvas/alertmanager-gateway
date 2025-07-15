package formatter

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// QueryFormatter formats data as URL query parameters
type QueryFormatter struct {
	// flattenArrays determines if arrays should be flattened as param[0], param[1] or param=val1&param=val2
	flattenArrays bool
}

// NewQueryFormatter creates a new query parameter formatter
func NewQueryFormatter() *QueryFormatter {
	return &QueryFormatter{
		flattenArrays: false, // Use param=val1&param=val2 by default
	}
}

// NewQueryFormatterWithArrayFlattening creates a new query formatter with array flattening enabled
func NewQueryFormatterWithArrayFlattening() *QueryFormatter {
	return &QueryFormatter{
		flattenArrays: true, // Use param[0]=val1&param[1]=val2
	}
}

// Format converts data to URL query parameter format
func (f *QueryFormatter) Format(data interface{}) ([]byte, error) {
	// Check if data is already pre-formatted
	if result, handled, err := validateAndHandlePreformatted(data); handled || err != nil {
		if err != nil {
			return nil, fmt.Errorf("invalid query data: %w", err)
		}
		return result, nil
	}

	// Convert data to query parameters
	values, err := f.toQueryValues(data, "")
	if err != nil {
		return nil, fmt.Errorf("failed to convert to query values: %w", err)
	}

	return buildEncodedString(values), nil
}

// ContentType returns empty string as query parameters don't have a body content type
func (f *QueryFormatter) ContentType() string {
	return "" // Query parameters are in URL, not body
}

// Name returns the formatter name
func (f *QueryFormatter) Name() string {
	return string(FormatQuery)
}

// toQueryValues converts data to url.Values recursively
func (f *QueryFormatter) toQueryValues(data interface{}, prefix string) (url.Values, error) {
	values := make(url.Values)

	if data == nil {
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
			return f.toQueryValues(val.Interface(), prefix)
		}

	default:
		return nil, fmt.Errorf("unsupported type: %v", val.Kind())
	}

	return values, nil
}

// handleMap processes map types
func (f *QueryFormatter) handleMap(val reflect.Value, prefix string) (url.Values, error) {
	values := make(url.Values)

	for _, key := range val.MapKeys() {
		keyStr := fmt.Sprintf("%v", key.Interface())
		fieldPrefix := f.buildKey(prefix, keyStr)

		fieldValues, err := f.toQueryValues(val.MapIndex(key).Interface(), fieldPrefix)
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
func (f *QueryFormatter) handleStruct(val reflect.Value, typ reflect.Type, prefix string) (url.Values, error) {
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
		if tag := fieldType.Tag.Get("query"); tag != "" {
			if tag == "-" {
				continue // Skip this field
			}
			if idx := strings.Index(tag, ","); idx != -1 {
				fieldName = tag[:idx]
			} else {
				fieldName = tag
			}
		} else if tag := fieldType.Tag.Get("form"); tag != "" {
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

		fieldValues, err := f.toQueryValues(field.Interface(), fieldPrefix)
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
func (f *QueryFormatter) handleSlice(val reflect.Value, prefix string) (url.Values, error) {
	values := make(url.Values)

	if f.flattenArrays {
		// Use indexed notation: param[0]=val1&param[1]=val2
		for i := 0; i < val.Len(); i++ {
			indexPrefix := f.buildKey(prefix, strconv.Itoa(i))

			itemValues, err := f.toQueryValues(val.Index(i).Interface(), indexPrefix)
			if err != nil {
				return nil, err
			}

			for k, v := range itemValues {
				for _, value := range v {
					values.Add(k, value)
				}
			}
		}
	} else {
		// Use repeated parameter notation: param=val1&param=val2
		key := f.buildKey(prefix, "")
		for i := 0; i < val.Len(); i++ {
			item := val.Index(i).Interface()

			// For primitive types, add directly
			switch v := item.(type) {
			case string:
				values.Add(key, v)
			case int, int8, int16, int32, int64:
				values.Add(key, fmt.Sprintf("%d", v))
			case uint, uint8, uint16, uint32, uint64:
				values.Add(key, fmt.Sprintf("%d", v))
			case float32, float64:
				values.Add(key, fmt.Sprintf("%g", v))
			case bool:
				values.Add(key, strconv.FormatBool(v))
			default:
				// For complex types, fall back to indexed notation
				indexPrefix := f.buildKey(prefix, strconv.Itoa(i))
				itemValues, err := f.toQueryValues(item, indexPrefix)
				if err != nil {
					return nil, err
				}
				for k, vals := range itemValues {
					for _, value := range vals {
						values.Add(k, value)
					}
				}
			}
		}
	}

	return values, nil
}

// buildKey constructs query parameter keys
func (f *QueryFormatter) buildKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	if key == "" {
		return prefix
	}

	// For query parameters, use dot notation for nested objects and bracket notation for arrays
	if _, err := strconv.Atoi(key); err == nil {
		// Key is numeric (array index)
		return prefix + "[" + key + "]"
	}
	// Key is string (object field)
	return prefix + "." + key
}
