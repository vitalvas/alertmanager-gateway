package formatter

import (
	"encoding/xml"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// XMLFormatter formats data as XML
type XMLFormatter struct {
	indent   bool
	rootName string
}

// NewXMLFormatter creates a new XML formatter
func NewXMLFormatter() *XMLFormatter {
	return &XMLFormatter{
		indent:   false,
		rootName: "root",
	}
}

// NewXMLFormatterWithIndent creates a new XML formatter with indentation
func NewXMLFormatterWithIndent() *XMLFormatter {
	return &XMLFormatter{
		indent:   true,
		rootName: "root",
	}
}

// NewXMLFormatterWithRoot creates a new XML formatter with custom root element name
func NewXMLFormatterWithRoot(rootName string) *XMLFormatter {
	return &XMLFormatter{
		indent:   false,
		rootName: rootName,
	}
}

// Format converts data to XML format
func (f *XMLFormatter) Format(data interface{}) ([]byte, error) {
	// Check if data is already a byte slice (pre-formatted XML)
	if bytes, ok := data.([]byte); ok {
		// Validate it's valid XML
		var temp interface{}
		if err := xml.Unmarshal(bytes, &temp); err != nil {
			return nil, fmt.Errorf("invalid XML data: %w", err)
		}
		return bytes, nil
	}

	// Check if data is already a string (pre-formatted XML)
	if str, ok := data.(string); ok {
		// Validate it's valid XML
		var temp interface{}
		if err := xml.Unmarshal([]byte(str), &temp); err != nil {
			return nil, fmt.Errorf("invalid XML string: %w", err)
		}
		return []byte(str), nil
	}

	// Convert data to XML structure
	xmlData, err := f.toXMLElement(data, f.rootName)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to XML: %w", err)
	}

	// Marshal to XML
	var result []byte
	if f.indent {
		result, err = xml.MarshalIndent(xmlData, "", "  ")
	} else {
		result, err = xml.Marshal(xmlData)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to marshal XML: %w", err)
	}

	// Add XML declaration
	declaration := []byte(`<?xml version="1.0" encoding="UTF-8"?>`)
	if f.indent {
		declaration = append(declaration, '\n')
	}
	result = append(declaration, result...)

	return result, nil
}

// ContentType returns the content type for XML
func (f *XMLFormatter) ContentType() string {
	return "application/xml"
}

// Name returns the formatter name
func (f *XMLFormatter) Name() string {
	return "xml"
}

// XMLElement represents a generic XML element
type XMLElement struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Content  string     `xml:",chardata"`
	Children []XMLElement
}

// MarshalXML implements custom XML marshaling
func (e XMLElement) MarshalXML(enc *xml.Encoder, start xml.StartElement) error {
	start.Name = e.XMLName
	start.Attr = e.Attrs

	if err := enc.EncodeToken(start); err != nil {
		return err
	}

	if e.Content != "" && len(e.Children) == 0 {
		if err := enc.EncodeToken(xml.CharData(e.Content)); err != nil {
			return err
		}
	} else {
		for _, child := range e.Children {
			if err := enc.Encode(child); err != nil {
				return err
			}
		}
	}

	return enc.EncodeToken(xml.EndElement{Name: start.Name})
}

// toXMLElement converts data to XMLElement recursively
func (f *XMLFormatter) toXMLElement(data interface{}, name string) (XMLElement, error) {
	elem := XMLElement{
		XMLName: xml.Name{Local: f.sanitizeElementName(name)},
	}

	if data == nil {
		return elem, nil
	}

	val := reflect.ValueOf(data)
	typ := reflect.TypeOf(data)

	// Dereference pointers
	for val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return elem, nil
		}
		val = val.Elem()
		typ = typ.Elem()
	}

	switch val.Kind() {
	case reflect.Map:
		return f.handleMapXML(val, name)

	case reflect.Struct:
		return f.handleStructXML(val, typ, name)

	case reflect.Slice, reflect.Array:
		return f.handleSliceXML(val, name)

	case reflect.String:
		elem.Content = val.String()

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		elem.Content = strconv.FormatInt(val.Int(), 10)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		elem.Content = strconv.FormatUint(val.Uint(), 10)

	case reflect.Float32, reflect.Float64:
		elem.Content = strconv.FormatFloat(val.Float(), 'f', -1, 64)

	case reflect.Bool:
		elem.Content = strconv.FormatBool(val.Bool())

	case reflect.Interface:
		if !val.IsNil() {
			return f.toXMLElement(val.Interface(), name)
		}

	default:
		return elem, fmt.Errorf("unsupported type: %v", val.Kind())
	}

	return elem, nil
}

// handleMapXML processes map types for XML
func (f *XMLFormatter) handleMapXML(val reflect.Value, name string) (XMLElement, error) {
	elem := XMLElement{
		XMLName: xml.Name{Local: f.sanitizeElementName(name)},
	}

	for _, key := range val.MapKeys() {
		keyStr := fmt.Sprintf("%v", key.Interface())

		childElem, err := f.toXMLElement(val.MapIndex(key).Interface(), keyStr)
		if err != nil {
			return elem, err
		}

		elem.Children = append(elem.Children, childElem)
	}

	return elem, nil
}

// handleStructXML processes struct types for XML
func (f *XMLFormatter) handleStructXML(val reflect.Value, typ reflect.Type, name string) (XMLElement, error) {
	elem := XMLElement{
		XMLName: xml.Name{Local: f.sanitizeElementName(name)},
	}

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Skip unexported fields
		if !field.CanInterface() {
			continue
		}

		// Get field name from tag or use field name
		fieldName := fieldType.Name
		isAttr := false

		if tag := fieldType.Tag.Get("xml"); tag != "" {
			if tag == "-" {
				continue // Skip this field
			}

			parts := strings.Split(tag, ",")
			if len(parts) > 0 && parts[0] != "" {
				fieldName = parts[0]
			}

			// Check for attribute flag
			for _, part := range parts[1:] {
				if part == "attr" {
					isAttr = true
					break
				}
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

		if isAttr {
			// Add as attribute
			attrValue := f.getStringValue(field.Interface())
			elem.Attrs = append(elem.Attrs, xml.Attr{
				Name:  xml.Name{Local: f.sanitizeElementName(fieldName)},
				Value: attrValue,
			})
		} else {
			// Add as child element
			childElem, err := f.toXMLElement(field.Interface(), fieldName)
			if err != nil {
				return elem, err
			}
			elem.Children = append(elem.Children, childElem)
		}
	}

	return elem, nil
}

// handleSliceXML processes slice and array types for XML
func (f *XMLFormatter) handleSliceXML(val reflect.Value, name string) (XMLElement, error) {
	elem := XMLElement{
		XMLName: xml.Name{Local: f.sanitizeElementName(name)},
	}

	// Create a wrapper element for the array
	for i := 0; i < val.Len(); i++ {
		// Use singular form for individual items
		itemName := f.singularize(name)

		childElem, err := f.toXMLElement(val.Index(i).Interface(), itemName)
		if err != nil {
			return elem, err
		}

		elem.Children = append(elem.Children, childElem)
	}

	return elem, nil
}

// sanitizeElementName ensures the name is valid for XML elements
func (f *XMLFormatter) sanitizeElementName(name string) string {
	if name == "" {
		return "item"
	}

	// Replace invalid characters with underscores
	var result strings.Builder
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') || r == '_' || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}

	sanitized := result.String()
	if sanitized == "" || (sanitized[0] >= '0' && sanitized[0] <= '9') {
		sanitized = "item_" + sanitized
	}

	return sanitized
}

// singularize attempts to convert plural names to singular for array items
func (f *XMLFormatter) singularize(name string) string {
	if strings.HasSuffix(name, "ies") {
		return strings.TrimSuffix(name, "ies") + "y"
	}
	if strings.HasSuffix(name, "ves") {
		return strings.TrimSuffix(name, "ves") + "f"
	}
	if strings.HasSuffix(name, "ses") || strings.HasSuffix(name, "ches") || strings.HasSuffix(name, "shes") {
		return strings.TrimSuffix(name, "es")
	}
	if strings.HasSuffix(name, "s") && len(name) > 1 {
		return strings.TrimSuffix(name, "s")
	}
	return name + "_item"
}

// getStringValue converts any value to string representation
func (f *XMLFormatter) getStringValue(data interface{}) string {
	if data == nil {
		return ""
	}

	val := reflect.ValueOf(data)

	// Dereference pointers
	for val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return ""
		}
		val = val.Elem()
	}

	switch val.Kind() {
	case reflect.String:
		return val.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(val.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(val.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(val.Float(), 'f', -1, 64)
	case reflect.Bool:
		return strconv.FormatBool(val.Bool())
	default:
		return fmt.Sprintf("%v", data)
	}
}
