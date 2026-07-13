package typescript

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/compiler/naming"
)

func emitWireComponents(output *bytes.Buffer, document *ir.Document, name string, direction projection) error {
	all := make(map[string]bool, len(document.ComponentSchemas)+len(document.Schemas))
	for schemaName := range document.ComponentSchemas {
		all[schemaName] = true
	}
	for schemaName := range document.Schemas {
		all[schemaName] = true
	}
	names := make([]string, 0, len(all))
	for schemaName := range all {
		names = append(names, schemaName)
	}
	sort.Strings(names)
	fmt.Fprintf(output, "const %s: WireSchemas = {\n", name)
	for _, schemaName := range names {
		value := any(document.ComponentSchemas[schemaName])
		if schema, ok := document.Schemas[schemaName]; ok {
			value = schema.Value
		}
		descriptor, err := wireSchemaDescriptor(value, direction)
		if err != nil {
			return fmt.Errorf("component %s wire schema: %w", schemaName, err)
		}
		fmt.Fprintf(output, "  %s: %s,\n", quoteTS(schemaName), descriptor)
	}
	output.WriteString("}\n\n")
	return nil
}

func wireSchemaDescriptor(value any, direction projection) (string, error) {
	return wireSchemaDescriptorScoped(value, direction, schemaRequiresFormatAssertion(value))
}

const formatAssertionVocabulary = "https://json-schema.org/draft/2020-12/vocab/format-assertion"

// schemaRequiresFormatAssertion recognizes the standard assertion vocabulary.
// The OpenAPI base dialect enables format annotation only, so a normal `format`
// remains documentation unless a schema resource explicitly requires this URI.
func schemaRequiresFormatAssertion(value any) bool {
	schema, ok := value.(map[string]any)
	if !ok {
		return false
	}
	vocabularies, _ := schema["$vocabulary"].(map[string]any)
	required, _ := vocabularies[formatAssertionVocabulary].(bool)
	return required
}

func wireSchemaDescriptorScoped(value any, direction projection, formatAssertion bool) (string, error) {
	if boolean, ok := value.(bool); ok {
		return fmt.Sprintf("{ boolean: %t }", boolean), nil
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return "{}", nil
	}
	if len(schema) == 0 {
		return "{}", nil
	}
	if dynamicReference, ok := schema["x-sdkgen-dynamic-reference"].(map[string]any); ok {
		anchor, _ := dynamicReference["anchor"].(string)
		reference, _ := dynamicReference["reference"].(string)
		name, err := componentSchemaReferenceName(reference)
		if err != nil || anchor == "" {
			if err == nil {
				err = fmt.Errorf("dynamic reference has no anchor")
			}
			return "", err
		}
		referenceDescriptor := "{ dynamicReference: { anchor: " + quoteTS(anchor) + ", fallback: { reference: " + quoteTS(name) + " } } }"
		if len(schema) == 1 {
			return referenceDescriptor, nil
		}
		siblings := make(map[string]any, len(schema)-1)
		for key, value := range schema {
			if key != "x-sdkgen-dynamic-reference" {
				siblings[key] = value
			}
		}
		siblingDescriptor, err := wireSchemaDescriptorScoped(siblings, direction, formatAssertion)
		if err != nil {
			return "", err
		}
		return "{ allOf: [" + referenceDescriptor + ", " + siblingDescriptor + "] }", nil
	}
	if reference, _ := schema["$ref"].(string); reference != "" {
		name, err := componentSchemaReferenceName(reference)
		if err != nil {
			return "", err
		}
		referenceDescriptor := "{ reference: " + quoteTS(name) + " }"
		if len(schema) == 1 {
			return referenceDescriptor, nil
		}
		siblings := make(map[string]any, len(schema)-1)
		for key, value := range schema {
			if key != "$ref" {
				siblings[key] = value
			}
		}
		siblingDescriptor, err := wireSchemaDescriptorScoped(siblings, direction, formatAssertion)
		if err != nil {
			return "", err
		}
		return "{ allOf: [" + referenceDescriptor + ", " + siblingDescriptor + "] }", nil
	}

	var fields []string
	if anchor, _ := schema["x-sdkgen-dynamic-anchor"].(string); anchor != "" {
		fields = append(fields, "dynamicAnchor: "+quoteTS(anchor))
	}
	if types := schemaTypes(schema["type"]); len(types) > 0 {
		values := make([]string, 0, len(types))
		for _, value := range types {
			values = append(values, quoteTS(value))
		}
		fields = append(fields, "types: ["+strings.Join(values, ", ")+"]")
	}
	if value, exists := schema["const"]; exists {
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("encode const: %w", err)
		}
		fields = append(fields, "constValue: "+string(encoded))
	}
	if values, ok := schema["enum"].([]any); ok && len(values) > 0 {
		encoded, err := json.Marshal(values)
		if err != nil {
			return "", fmt.Errorf("encode enum: %w", err)
		}
		fields = append(fields, "enumValues: "+string(encoded))
	}
	exclusiveMaximum, maximumIsExclusive := schema["exclusiveMaximum"].(bool)
	exclusiveMinimum, minimumIsExclusive := schema["exclusiveMinimum"].(bool)
	if maximumIsExclusive && exclusiveMaximum {
		if maximum, exists := schema["maximum"]; exists {
			encoded, err := json.Marshal(maximum)
			if err != nil {
				return "", fmt.Errorf("encode maximum: %w", err)
			}
			if len(encoded) > 0 && encoded[0] != '"' {
				fields = append(fields, "exclusiveMaximum: "+string(encoded))
			}
		}
	}
	if minimumIsExclusive && exclusiveMinimum {
		if minimum, exists := schema["minimum"]; exists {
			encoded, err := json.Marshal(minimum)
			if err != nil {
				return "", fmt.Errorf("encode minimum: %w", err)
			}
			if len(encoded) > 0 && encoded[0] != '"' {
				fields = append(fields, "exclusiveMinimum: "+string(encoded))
			}
		}
	}
	for _, keyword := range []string{"multipleOf", "maximum", "exclusiveMaximum", "minimum", "exclusiveMinimum", "minLength", "maxLength", "minItems", "maxItems", "minProperties", "maxProperties"} {
		if (keyword == "maximum" && maximumIsExclusive && exclusiveMaximum) || (keyword == "minimum" && minimumIsExclusive && exclusiveMinimum) || (keyword == "exclusiveMaximum" && maximumIsExclusive) || (keyword == "exclusiveMinimum" && minimumIsExclusive) {
			continue
		}
		value, exists := schema[keyword]
		if !exists {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("encode %s: %w", keyword, err)
		}
		if string(encoded) != "true" && string(encoded) != "false" && string(encoded) != "null" && len(encoded) > 0 && encoded[0] != '"' {
			fields = append(fields, keyword+": "+string(encoded))
		}
	}
	if value, ok := schema["pattern"].(string); ok {
		fields = append(fields, "pattern: "+quoteTS(value))
	}
	if value, ok := schema["format"].(string); ok && value != "" {
		fields = append(fields, "format: "+quoteTS(value))
		if formatAssertion || schemaRequiresFormatAssertion(schema) {
			fields = append(fields, "formatAssertion: true")
		}
	}
	if value, ok := schema["uniqueItems"].(bool); ok && value {
		fields = append(fields, "uniqueItems: true")
	}
	if value, ok := schema["contentEncoding"].(string); ok && value != "" {
		fields = append(fields, "contentEncoding: "+quoteTS(value))
	}
	if value, ok := schema["contentMediaType"].(string); ok && value != "" {
		fields = append(fields, "contentMediaType: "+quoteTS(value))
	}
	if value, exists := schema["contentSchema"]; exists {
		descriptor, err := wireSchemaDescriptorScoped(value, direction, formatAssertion)
		if err != nil {
			return "", err
		}
		fields = append(fields, "contentSchema: "+descriptor)
	}
	if xml, ok := schema["xml"].(map[string]any); ok && len(xml) > 0 {
		encoded, err := json.Marshal(xml)
		if err != nil {
			return "", fmt.Errorf("encode XML Object: %w", err)
		}
		fields = append(fields, "xml: "+string(encoded))
	}
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) > 0 {
		wireNames := make([]string, 0, len(properties))
		for wireName := range properties {
			wireNames = append(wireNames, wireName)
		}
		sort.Strings(wireNames)
		var entries []string
		for _, wireName := range wireNames {
			propertyValue := properties[wireName]
			propertySchema, _ := propertyValue.(map[string]any)
			if direction == projectionInput && boolValue(propertySchema, "readOnly") {
				continue
			}
			if direction == projectionOutput && boolValue(propertySchema, "writeOnly") {
				continue
			}
			propertyName, err := naming.Property(wireName)
			if err != nil {
				propertyName = wireName
			}
			nested, err := wireSchemaDescriptorScoped(propertyValue, direction, formatAssertion)
			if err != nil {
				return "", err
			}
			entries = append(entries, fmt.Sprintf("%s: { property: %s, schema: %s }", quoteTS(wireName), quoteTS(propertyName), nested))
		}
		if len(entries) > 0 {
			fields = append(fields, "properties: { "+strings.Join(entries, ", ")+" }")
		}
	}
	if patterns, ok := schema["patternProperties"].(map[string]any); ok && len(patterns) > 0 {
		names := make([]string, 0, len(patterns))
		for name := range patterns {
			names = append(names, name)
		}
		sort.Strings(names)
		entries := make([]string, 0, len(names))
		for _, name := range names {
			child, _ := patterns[name].(map[string]any)
			descriptor, err := wireSchemaDescriptorScoped(child, direction, formatAssertion)
			if err != nil {
				return "", err
			}
			entries = append(entries, quoteTS(name)+": "+descriptor)
		}
		fields = append(fields, "patternProperties: { "+strings.Join(entries, ", ")+" }")
	}
	if propertyNames, ok := schema["propertyNames"].(map[string]any); ok {
		descriptor, err := wireSchemaDescriptorScoped(propertyNames, direction, formatAssertion)
		if err != nil {
			return "", err
		}
		fields = append(fields, "propertyNames: "+descriptor)
	}
	if dependencies, ok := schema["dependentRequired"].(map[string]any); ok && len(dependencies) > 0 {
		names := make([]string, 0, len(dependencies))
		for name := range dependencies {
			names = append(names, name)
		}
		sort.Strings(names)
		entries := make([]string, 0, len(names))
		for _, name := range names {
			values, _ := dependencies[name].([]any)
			items := make([]string, 0, len(values))
			for _, value := range values {
				if dependency, ok := value.(string); ok {
					items = append(items, quoteTS(dependency))
				}
			}
			entries = append(entries, quoteTS(name)+": ["+strings.Join(items, ", ")+"]")
		}
		fields = append(fields, "dependentRequired: { "+strings.Join(entries, ", ")+" }")
	}
	if dependencies, ok := schema["dependentSchemas"].(map[string]any); ok && len(dependencies) > 0 {
		names := make([]string, 0, len(dependencies))
		for name := range dependencies {
			names = append(names, name)
		}
		sort.Strings(names)
		entries := make([]string, 0, len(names))
		for _, name := range names {
			child, _ := dependencies[name].(map[string]any)
			descriptor, err := wireSchemaDescriptorScoped(child, direction, formatAssertion)
			if err != nil {
				return "", err
			}
			entries = append(entries, quoteTS(name)+": "+descriptor)
		}
		fields = append(fields, "dependentSchemas: { "+strings.Join(entries, ", ")+" }")
	}
	if required, ok := schema["required"].([]any); ok && len(required) > 0 {
		names := make([]string, 0, len(required))
		for _, value := range required {
			name, ok := value.(string)
			if !ok {
				continue
			}
			property, _ := properties[name].(map[string]any)
			if direction == projectionInput && boolValue(property, "readOnly") {
				continue
			}
			if direction == projectionOutput && boolValue(property, "writeOnly") {
				continue
			}
			names = append(names, quoteTS(name))
		}
		sort.Strings(names)
		if len(names) > 0 {
			fields = append(fields, "required: ["+strings.Join(names, ", ")+"]")
		}
	}
	if items, exists := schema["items"]; exists {
		descriptor, err := wireSchemaDescriptorScoped(items, direction, formatAssertion)
		if err != nil {
			return "", err
		}
		fields = append(fields, "items: "+descriptor)
	}
	if contains, ok := schema["contains"].(map[string]any); ok {
		descriptor, err := wireSchemaDescriptorScoped(contains, direction, formatAssertion)
		if err != nil {
			return "", err
		}
		fields = append(fields, "contains: "+descriptor)
		for _, keyword := range []string{"minContains", "maxContains"} {
			if value, exists := schema[keyword]; exists {
				encoded, err := json.Marshal(value)
				if err != nil {
					return "", fmt.Errorf("encode %s: %w", keyword, err)
				}
				fields = append(fields, keyword+": "+string(encoded))
			}
		}
	}
	if prefixItems, ok := schema["prefixItems"].([]any); ok && len(prefixItems) > 0 {
		items := make([]string, 0, len(prefixItems))
		for _, value := range prefixItems {
			descriptor, err := wireSchemaDescriptorScoped(value, direction, formatAssertion)
			if err != nil {
				return "", err
			}
			items = append(items, descriptor)
		}
		fields = append(fields, "prefixItems: ["+strings.Join(items, ", ")+"]")
	}
	if additional, exists := schema["additionalProperties"]; exists {
		if boolean, ok := additional.(bool); ok && !boolean {
			fields = append(fields, "additionalProperties: false")
		} else {
			descriptor, err := wireSchemaDescriptorScoped(additional, direction, formatAssertion)
			if err != nil {
				return "", err
			}
			fields = append(fields, "additionalProperties: "+descriptor)
		}
	}
	for _, keyword := range []string{"unevaluatedProperties", "unevaluatedItems"} {
		value, exists := schema[keyword]
		if !exists {
			continue
		}
		if boolean, ok := value.(bool); ok && !boolean {
			fields = append(fields, keyword+": false")
			continue
		}
		descriptor, err := wireSchemaDescriptorScoped(value, direction, formatAssertion)
		if err != nil {
			return "", err
		}
		fields = append(fields, keyword+": "+descriptor)
	}
	for _, keyword := range []string{"allOf", "oneOf", "anyOf"} {
		variants, _ := schema[keyword].([]any)
		if len(variants) == 0 {
			continue
		}
		items := make([]string, 0, len(variants))
		for _, value := range variants {
			descriptor, err := wireSchemaDescriptorScoped(value, direction, formatAssertion)
			if err != nil {
				return "", err
			}
			items = append(items, descriptor)
		}
		fields = append(fields, keyword+": ["+strings.Join(items, ", ")+"]")
	}
	if negated, exists := schema["not"]; exists {
		descriptor, err := wireSchemaDescriptorScoped(negated, direction, formatAssertion)
		if err != nil {
			return "", err
		}
		fields = append(fields, "not: "+descriptor)
	}
	for _, keyword := range []string{"if", "then", "else"} {
		child, exists := schema[keyword]
		if !exists {
			continue
		}
		descriptor, err := wireSchemaDescriptorScoped(child, direction, formatAssertion)
		if err != nil {
			return "", err
		}
		fields = append(fields, keyword+": "+descriptor)
	}
	if discriminator, ok := schema["discriminator"].(map[string]any); ok {
		if property, ok := discriminator["propertyName"].(string); ok && property != "" {
			mapping, err := discriminatorWireMapping(schema, discriminator, direction, formatAssertion)
			if err != nil {
				return "", err
			}
			field := "discriminator: { property: " + quoteTS(property)
			if len(mapping) > 0 {
				field += ", mapping: { " + strings.Join(mapping, ", ") + " }"
			}
			if value, ok := discriminator["defaultMapping"].(string); ok && value != "" {
				descriptor, err := discriminatorReferenceDescriptor(value, direction)
				if err != nil {
					return "", err
				}
				field += ", defaultMapping: " + descriptor
			}
			fields = append(fields, field+" }")
		}
	}
	if len(fields) == 0 {
		return "{}", nil
	}
	return "{ " + strings.Join(fields, ", ") + " }", nil
}

func discriminatorWireMapping(schema, discriminator map[string]any, direction projection, formatAssertion bool) ([]string, error) {
	mapping := make(map[string]any)
	if explicit, ok := discriminator["mapping"].(map[string]any); ok {
		for name, value := range explicit {
			reference, ok := value.(string)
			if !ok {
				continue
			}
			mapping[name] = map[string]any{"$ref": normalizedDiscriminatorReference(reference)}
		}
	}
	variants, _ := schema["oneOf"].([]any)
	for _, variant := range variants {
		object, _ := variant.(map[string]any)
		reference, _ := object["$ref"].(string)
		name, err := componentSchemaReferenceName(reference)
		if err != nil || name == "" {
			continue
		}
		if _, exists := mapping[name]; !exists {
			mapping[name] = map[string]any{"$ref": reference}
		}
	}
	names := make([]string, 0, len(mapping))
	for name := range mapping {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]string, 0, len(names))
	for _, name := range names {
		descriptor, err := wireSchemaDescriptorScoped(mapping[name], direction, formatAssertion)
		if err != nil {
			return nil, err
		}
		entries = append(entries, quoteTS(name)+": "+descriptor)
	}
	return entries, nil
}

func discriminatorReferenceDescriptor(reference string, direction projection) (string, error) {
	return wireSchemaDescriptor(map[string]any{"$ref": normalizedDiscriminatorReference(reference)}, direction)
}

func normalizedDiscriminatorReference(reference string) string {
	if strings.HasPrefix(reference, "#/components/schemas/") || strings.Contains(reference, ":") || strings.HasPrefix(reference, "./") || strings.HasPrefix(reference, "../") {
		return reference
	}
	return "#/components/schemas/" + strings.ReplaceAll(strings.ReplaceAll(reference, "~", "~0"), "/", "~1")
}

func operationRequestWireBodies(document *ir.Document, operation ir.Operation) (string, bool, error) {
	body, ok := operation.Raw["requestBody"].(map[string]any)
	if !ok {
		return "", false, nil
	}
	body, err := resolveComponentObject(document, body, "requestBodies")
	if err != nil {
		return "", false, err
	}
	content, _ := body["content"].(map[string]any)
	mediaTypes := make([]string, 0, len(content))
	for mediaType := range content {
		mediaTypes = append(mediaTypes, mediaType)
	}
	sort.Strings(mediaTypes)
	entries := make([]string, 0, len(mediaTypes))
	for _, mediaType := range mediaTypes {
		media, _ := content[mediaType].(map[string]any)
		media, err := resolveMediaTypeObject(document, media)
		if err != nil {
			return "", false, err
		}
		schema := media["schema"]
		schemaObject, _ := schema.(map[string]any)
		descriptor := "{}"
		if !isBinaryMedia(mediaType, schemaObject) {
			var err error
			descriptor, err = wireSchemaDescriptor(schema, projectionInput)
			if err != nil {
				return "", false, err
			}
		}
		entry := "{ contentType: " + quoteTS(mediaType) + ", schema: " + descriptor
		if itemSchema, exists := media["itemSchema"]; exists {
			itemDescriptor, err := wireSchemaDescriptor(itemSchema, projectionInput)
			if err != nil {
				return "", false, err
			}
			entry += ", itemSchema: " + itemDescriptor
		}
		encodings, err := requestBodyWireEncodings(document, media)
		if err != nil {
			return "", false, err
		}
		if encodings != "" {
			entry += ", encoding: " + encodings
		}
		prefixEncoding, err := positionalMultipartWireEncodings(document, media["prefixEncoding"])
		if err != nil {
			return "", false, err
		}
		if prefixEncoding != "" {
			entry += ", prefixEncoding: " + prefixEncoding
		}
		itemEncoding, err := positionalMultipartWireEncoding(document, media["itemEncoding"])
		if err != nil {
			return "", false, err
		}
		if itemEncoding != "" {
			entry += ", itemEncoding: " + itemEncoding
		}
		entries = append(entries, entry+" }")
	}
	return "[" + strings.Join(entries, ", ") + "]", len(entries) > 0, nil
}

func requestBodyWireEncodings(document *ir.Document, media map[string]any) (string, error) {
	values, _ := media["encoding"].(map[string]any)
	if len(values) == 0 {
		return "", nil
	}
	names := sortedAnyKeys(values)
	entries := make([]string, 0, len(names))
	for _, name := range names {
		value, _ := values[name].(map[string]any)
		entry, err := multipartWireEncoding(document, value, name, projectionInput)
		if err != nil {
			return "", err
		}
		entries = append(entries, entry)
	}
	return "[" + strings.Join(entries, ", ") + "]", nil
}

func positionalMultipartWireEncodings(document *ir.Document, value any) (string, error) {
	values, _ := value.([]any)
	if len(values) == 0 {
		return "", nil
	}
	entries := make([]string, 0, len(values))
	for _, item := range values {
		encoding, _ := item.(map[string]any)
		entry, err := multipartWireEncoding(document, encoding, "", projectionInput)
		if err != nil {
			return "", err
		}
		entries = append(entries, entry)
	}
	return "[" + strings.Join(entries, ", ") + "]", nil
}

func positionalMultipartWireEncoding(document *ir.Document, value any) (string, error) {
	encoding, _ := value.(map[string]any)
	if len(encoding) == 0 {
		return "", nil
	}
	return multipartWireEncoding(document, encoding, "", projectionInput)
}

func multipartWireEncoding(document *ir.Document, value map[string]any, name string, direction projection) (string, error) {
	fields := make([]string, 0, 9)
	if name != "" {
		fields = append(fields, "name: "+quoteTS(name))
	}
	if contentType, _ := value["contentType"].(string); contentType != "" {
		fields = append(fields, "contentType: "+quoteTS(contentType))
	}
	if style, _ := value["style"].(string); style != "" {
		fields = append(fields, "style: "+quoteTS(style))
	}
	if explode, exists := value["explode"].(bool); exists {
		fields = append(fields, fmt.Sprintf("explode: %t", explode))
	}
	if allowReserved, exists := value["allowReserved"].(bool); exists {
		fields = append(fields, fmt.Sprintf("allowReserved: %t", allowReserved))
	}
	headers, err := multipartWireHeaders(document, value["headers"], direction)
	if err != nil {
		return "", err
	}
	if headers != "" {
		fields = append(fields, "headers: "+headers)
	}
	if nested, err := nestedMultipartWireEncodings(document, value["encoding"], direction); err != nil {
		return "", err
	} else if nested != "" {
		fields = append(fields, "encoding: "+nested)
	}
	if nested, err := positionalMultipartWireEncodingsForDirection(document, value["prefixEncoding"], direction); err != nil {
		return "", err
	} else if nested != "" {
		fields = append(fields, "prefixEncoding: "+nested)
	}
	if nested, err := positionalMultipartWireEncodingForDirection(document, value["itemEncoding"], direction); err != nil {
		return "", err
	} else if nested != "" {
		fields = append(fields, "itemEncoding: "+nested)
	}
	return "{ " + strings.Join(fields, ", ") + " }", nil
}

func nestedMultipartWireEncodings(document *ir.Document, value any, direction projection) (string, error) {
	values, _ := value.(map[string]any)
	if len(values) == 0 {
		return "", nil
	}
	names := sortedAnyKeys(values)
	entries := make([]string, 0, len(names))
	for _, name := range names {
		encoding, _ := values[name].(map[string]any)
		entry, err := multipartWireEncoding(document, encoding, name, direction)
		if err != nil {
			return "", err
		}
		entries = append(entries, entry)
	}
	return "[" + strings.Join(entries, ", ") + "]", nil
}

func multipartWireHeaders(document *ir.Document, value any, direction projection) (string, error) {
	headers, _ := value.(map[string]any)
	if len(headers) == 0 {
		return "", nil
	}
	names := sortedAnyKeys(headers)
	entries := make([]string, 0, len(names))
	for _, name := range names {
		header, _ := headers[name].(map[string]any)
		header, err := resolveComponentObject(document, header, "headers")
		if err != nil {
			return "", err
		}
		schema, contentType, err := responseHeaderSchema(document, header)
		if err != nil {
			return "", err
		}
		descriptor, err := wireSchemaDescriptor(schema, direction)
		if err != nil {
			return "", err
		}
		style, _ := header["style"].(string)
		if style == "" {
			style = "simple"
		}
		explode, hasExplode := header["explode"].(bool)
		if !hasExplode {
			explode = style == "form"
		}
		fields := []string{"name: " + quoteTS(name), "style: " + quoteTS(style), fmt.Sprintf("explode: %t", explode), "schema: " + descriptor}
		if boolValue(header, "required") {
			fields = append(fields, "required: true")
		}
		if contentType != "" {
			fields = append(fields, "contentType: "+quoteTS(contentType))
		}
		entries = append(entries, "{ "+strings.Join(fields, ", ")+" }")
	}
	return "[" + strings.Join(entries, ", ") + "]", nil
}

func operationResponseWireBodies(document *ir.Document, operation ir.Operation) (string, bool, error) {
	responses, _ := operation.Raw["responses"].(map[string]any)
	statuses := make([]string, 0, len(responses))
	for status := range responses {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	var entries []string
	for _, status := range statuses {
		response, _ := responses[status].(map[string]any)
		response, err := resolveComponentObject(document, response, "responses")
		if err != nil {
			return "", false, err
		}
		content, _ := response["content"].(map[string]any)
		headers, err := responseWireHeaders(document, response)
		if err != nil {
			return "", false, err
		}
		mediaTypes := make([]string, 0, len(content))
		for mediaType := range content {
			mediaTypes = append(mediaTypes, mediaType)
		}
		sort.Strings(mediaTypes)
		if len(mediaTypes) == 0 && headers != "" {
			entries = append(entries, "{ status: "+quoteTS(status)+", contentType: \"\", schema: {}, headers: "+headers+" }")
		}
		for _, mediaType := range mediaTypes {
			media, _ := content[mediaType].(map[string]any)
			media, err := resolveMediaTypeObject(document, media)
			if err != nil {
				return "", false, err
			}
			schema := media["schema"]
			schemaObject, _ := schema.(map[string]any)
			descriptor := "{}"
			if !isBinaryMedia(mediaType, schemaObject) {
				var err error
				descriptor, err = wireSchemaDescriptor(schema, projectionOutput)
				if err != nil {
					return "", false, err
				}
			}
			entry := "{ status: " + quoteTS(status) + ", contentType: " + quoteTS(mediaType) + ", schema: " + descriptor
			if itemSchema, exists := media["itemSchema"]; exists {
				itemDescriptor, err := wireSchemaDescriptor(itemSchema, projectionOutput)
				if err != nil {
					return "", false, err
				}
				entry += ", itemSchema: " + itemDescriptor
			}
			if prefixEncoding, err := positionalMultipartWireEncodingsForDirection(document, media["prefixEncoding"], projectionOutput); err != nil {
				return "", false, err
			} else if prefixEncoding != "" {
				entry += ", prefixEncoding: " + prefixEncoding
			}
			if itemEncoding, err := positionalMultipartWireEncodingForDirection(document, media["itemEncoding"], projectionOutput); err != nil {
				return "", false, err
			} else if itemEncoding != "" {
				entry += ", itemEncoding: " + itemEncoding
			}
			if headers != "" {
				entry += ", headers: " + headers
			}
			entries = append(entries, entry+" }")
		}
	}
	return "[" + strings.Join(entries, ", ") + "]", len(entries) > 0, nil
}

func positionalMultipartWireEncodingForDirection(document *ir.Document, value any, direction projection) (string, error) {
	encoding, _ := value.(map[string]any)
	if len(encoding) == 0 {
		return "", nil
	}
	return multipartWireEncoding(document, encoding, "", direction)
}

func positionalMultipartWireEncodingsForDirection(document *ir.Document, value any, direction projection) (string, error) {
	values, _ := value.([]any)
	if len(values) == 0 {
		return "", nil
	}
	entries := make([]string, 0, len(values))
	for _, item := range values {
		encoding, _ := item.(map[string]any)
		entry, err := multipartWireEncoding(document, encoding, "", direction)
		if err != nil {
			return "", err
		}
		entries = append(entries, entry)
	}
	return "[" + strings.Join(entries, ", ") + "]", nil
}

func responseWireHeaders(document *ir.Document, response map[string]any) (string, error) {
	headers, _ := response["headers"].(map[string]any)
	if len(headers) == 0 {
		return "", nil
	}
	names := sortedAnyKeys(headers)
	entries := make([]string, 0, len(names))
	for _, name := range names {
		header, _ := headers[name].(map[string]any)
		header, err := resolveComponentObject(document, header, "headers")
		if err != nil {
			return "", err
		}
		property, err := naming.Property(name)
		if err != nil {
			property = name
		}
		schema, contentType, err := responseHeaderSchema(document, header)
		if err != nil {
			return "", err
		}
		descriptor, err := wireSchemaDescriptor(schema, projectionOutput)
		if err != nil {
			return "", err
		}
		style, _ := header["style"].(string)
		if style == "" {
			style = "simple"
		}
		explode, hasExplode := header["explode"].(bool)
		if !hasExplode {
			explode = style == "form"
		}
		entry := "{ name: " + quoteTS(name) + ", property: " + quoteTS(property) + ", style: " + quoteTS(style) + ", explode: " + fmt.Sprint(explode) + ", schema: " + descriptor
		if contentType != "" {
			entry += ", contentType: " + quoteTS(contentType)
		}
		if boolValue(header, "required") {
			entry += ", required: true"
		}
		entries = append(entries, entry+" }")
	}
	return "[" + strings.Join(entries, ", ") + "]", nil
}
