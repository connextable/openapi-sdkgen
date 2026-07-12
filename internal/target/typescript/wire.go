package typescript

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/compiler/naming"
)

func emitWireComponents(output *bytes.Buffer, document *ir.Document, name string, direction projection) error {
	names := make([]string, 0, len(document.ComponentSchemas))
	for schemaName := range document.ComponentSchemas {
		names = append(names, schemaName)
	}
	sort.Strings(names)
	fmt.Fprintf(output, "const %s: WireSchemas = {\n", name)
	for _, schemaName := range names {
		descriptor, err := wireSchemaDescriptor(document.ComponentSchemas[schemaName], direction)
		if err != nil {
			return fmt.Errorf("component %s wire schema: %w", schemaName, err)
		}
		fmt.Fprintf(output, "  %s: %s,\n", quoteTS(schemaName), descriptor)
	}
	output.WriteString("}\n\n")
	return nil
}

func emitJavaScriptWireComponents(output *bytes.Buffer, document *ir.Document, name string, direction projection) error {
	names := make([]string, 0, len(document.ComponentSchemas))
	for schemaName := range document.ComponentSchemas {
		names = append(names, schemaName)
	}
	sort.Strings(names)
	fmt.Fprintf(output, "const %s = {\n", name)
	for _, schemaName := range names {
		descriptor, err := wireSchemaDescriptor(document.ComponentSchemas[schemaName], direction)
		if err != nil {
			return fmt.Errorf("component %s wire schema: %w", schemaName, err)
		}
		fmt.Fprintf(output, "  %s: %s,\n", quoteTS(schemaName), descriptor)
	}
	output.WriteString("}\n\n")
	return nil
}

func wireSchemaDescriptor(schema map[string]any, direction projection) (string, error) {
	if len(schema) == 0 {
		return "{}", nil
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
		siblingDescriptor, err := wireSchemaDescriptor(siblings, direction)
		if err != nil {
			return "", err
		}
		return "{ allOf: [" + referenceDescriptor + ", " + siblingDescriptor + "] }", nil
	}

	var fields []string
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) > 0 {
		wireNames := make([]string, 0, len(properties))
		for wireName := range properties {
			wireNames = append(wireNames, wireName)
		}
		sort.Strings(wireNames)
		var entries []string
		for _, wireName := range wireNames {
			propertySchema, _ := properties[wireName].(map[string]any)
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
			nested, err := wireSchemaDescriptor(propertySchema, direction)
			if err != nil {
				return "", err
			}
			entries = append(entries, fmt.Sprintf("%s: { property: %s, schema: %s }", quoteTS(wireName), quoteTS(propertyName), nested))
		}
		if len(entries) > 0 {
			fields = append(fields, "properties: { "+strings.Join(entries, ", ")+" }")
		}
	}
	if items, ok := schema["items"].(map[string]any); ok {
		descriptor, err := wireSchemaDescriptor(items, direction)
		if err != nil {
			return "", err
		}
		fields = append(fields, "items: "+descriptor)
	}
	if prefixItems, ok := schema["prefixItems"].([]any); ok && len(prefixItems) > 0 {
		items := make([]string, 0, len(prefixItems))
		for _, value := range prefixItems {
			item, _ := value.(map[string]any)
			descriptor, err := wireSchemaDescriptor(item, direction)
			if err != nil {
				return "", err
			}
			items = append(items, descriptor)
		}
		fields = append(fields, "prefixItems: ["+strings.Join(items, ", ")+"]")
	}
	if additional, ok := schema["additionalProperties"].(map[string]any); ok {
		descriptor, err := wireSchemaDescriptor(additional, direction)
		if err != nil {
			return "", err
		}
		fields = append(fields, "additionalProperties: "+descriptor)
	}
	for _, keyword := range []string{"allOf", "oneOf", "anyOf"} {
		variants, _ := schema[keyword].([]any)
		if len(variants) == 0 {
			continue
		}
		items := make([]string, 0, len(variants))
		for _, value := range variants {
			variant, _ := value.(map[string]any)
			descriptor, err := wireSchemaDescriptor(variant, direction)
			if err != nil {
				return "", err
			}
			items = append(items, descriptor)
		}
		fields = append(fields, keyword+": ["+strings.Join(items, ", ")+"]")
	}
	if len(fields) == 0 {
		return "{}", nil
	}
	return "{ " + strings.Join(fields, ", ") + " }", nil
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
		schema, _ := media["schema"].(map[string]any)
		descriptor, err := wireSchemaDescriptor(schema, projectionInput)
		if err != nil {
			return "", false, err
		}
		entries = append(entries, "{ contentType: "+quoteTS(mediaType)+", schema: "+descriptor+" }")
	}
	return "[" + strings.Join(entries, ", ") + "]", len(entries) > 0, nil
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
		mediaTypes := make([]string, 0, len(content))
		for mediaType := range content {
			mediaTypes = append(mediaTypes, mediaType)
		}
		sort.Strings(mediaTypes)
		for _, mediaType := range mediaTypes {
			media, _ := content[mediaType].(map[string]any)
			schema, _ := media["schema"].(map[string]any)
			descriptor, err := wireSchemaDescriptor(schema, projectionOutput)
			if err != nil {
				return "", false, err
			}
			entries = append(entries, "{ status: "+quoteTS(status)+", contentType: "+quoteTS(mediaType)+", schema: "+descriptor+" }")
		}
	}
	return "[" + strings.Join(entries, ", ") + "]", len(entries) > 0, nil
}
