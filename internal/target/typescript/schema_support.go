package typescript

import (
	"fmt"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

// validateSchemaSupport makes the target's JSON Schema boundary explicit.
// A static TypeScript declaration can represent structural schemas, literals,
// composition, nullability, and metadata. Keywords whose validation or
// reference semantics are not implemented must fail at generation time rather
// than being erased while traversing raw OpenAPI maps.
func validateSchemaSupport(document *ir.Document) error {
	return validateSchemaSupportForTarget(document, "TypeScript")
}

func validateServerInboundSchemaSupport(document *ir.Document) error {
	unsupported := unsupportedSchemasInOpenAPIValue(document.Raw["webhooks"], openAPIPointer("webhooks"))
	if len(unsupported) == 0 {
		return nil
	}
	sort.Strings(unsupported)
	return fmt.Errorf("TypeScript server add-on does not yet implement these OpenAPI Schema Object features:\n- %s", strings.Join(unsupported, "\n- "))
}

func validateSchemaSupportForTarget(document *ir.Document, target string) error {
	var unsupported []string
	for _, name := range sortedSchemaNames(document.ComponentSchemas) {
		unsupported = append(unsupported, unsupportedSchemaFeatures(document.ComponentSchemas[name], openAPIPointer("components", "schemas", name))...)
	}
	components, _ := document.Raw["components"].(map[string]any)
	// libopenapi's typed component-schema map only contains object schemas.
	// Inspect the raw map too, so valid JSON Schema booleans cannot disappear
	// before the target boundary has a chance to reject them.
	if schemas, ok := components["schemas"].(map[string]any); ok {
		names := make([]string, 0, len(schemas))
		for name := range schemas {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if _, ok := schemas[name].(bool); ok {
				unsupported = append(unsupported, unsupportedSchemaFeatures(schemas[name], openAPIPointer("components", "schemas", name))...)
			}
		}
	}
	unsupported = append(unsupported, unsupportedSchemasInOpenAPIValue(components, openAPIPointer("components"))...)
	for _, operation := range document.Operations {
		operationPath := openAPIPointer("paths", operation.Path, strings.ToLower(operation.Method))
		unsupported = append(unsupported, unsupportedSchemasInOpenAPIValue(operation.PathItemRaw, openAPIPointer("paths", operation.Path))...)
		unsupported = append(unsupported, unsupportedSchemasInOpenAPIValue(operation.Raw, operationPath)...)
	}
	if len(unsupported) == 0 {
		return nil
	}
	sort.Strings(unsupported)
	return fmt.Errorf("%s target does not yet implement these OpenAPI Schema Object features:\n- %s", target, strings.Join(unsupported, "\n- "))
}

func sortedSchemaNames(schemas map[string]map[string]any) []string {
	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func unsupportedSchemasInOpenAPIValue(value any, path string) []string {
	switch typed := value.(type) {
	case map[string]any:
		var result []string
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			item := typed[key]
			if strings.HasPrefix(key, "x-") {
				continue
			}
			if isLiteralOpenAPIValue(key) {
				continue
			}
			if key == "schema" {
				result = append(result, unsupportedSchemaFeatures(item, appendOpenAPIPointer(path, key))...)
				continue
			}
			result = append(result, unsupportedSchemasInOpenAPIValue(item, appendOpenAPIPointer(path, key))...)
		}
		return result
	case []any:
		var result []string
		for index, item := range typed {
			result = append(result, unsupportedSchemasInOpenAPIValue(item, appendOpenAPIPointer(path, fmt.Sprint(index)))...)
		}
		return result
	default:
		return nil
	}
}

func isLiteralOpenAPIValue(key string) bool {
	switch key {
	case "const", "default", "enum", "example", "examples", "value", "dataValue", "serializedValue":
		return true
	default:
		return false
	}
}

func unsupportedSchemaFeatures(value any, path string) []string {
	if _, ok := value.(bool); ok {
		return nil
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	var result []string
	unsupported := map[string]string{
		"$anchor":        "anchor-based reference resolution",
		"$dynamicAnchor": "dynamic anchor resolution",
		"$dynamicRef":    "dynamic reference resolution",
	}
	keys := make([]string, 0, len(schema))
	for key := range schema {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := schema[key]
		if key == "$ref" {
			if reference, ok := value.(string); ok {
				if _, err := componentSchemaReferenceName(reference); err != nil {
					result = append(result, fmt.Sprintf("%s (%v)", appendOpenAPIPointer(path, key), err))
				}
			}
		}
		if feature, exists := unsupported[key]; exists {
			result = append(result, fmt.Sprintf("%s (%s)", appendOpenAPIPointer(path, key), feature))
		}
		if key == "$vocabulary" && hasUnsupportedRequiredVocabulary(value) {
			result = append(result, fmt.Sprintf("%s (custom JSON Schema vocabularies)", appendOpenAPIPointer(path, key)))
		}
		result = append(result, unsupportedSchemaChildren(key, value, appendOpenAPIPointer(path, key))...)
	}
	return result
}

func hasUnsupportedRequiredVocabulary(value any) bool {
	vocabularies, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for uri, enabled := range vocabularies {
		required, _ := enabled.(bool)
		if required && !strings.HasPrefix(uri, "https://json-schema.org/draft/2020-12/vocab/") && uri != "https://spec.openapis.org/oas/3.1/dialect/base" {
			return true
		}
	}
	return false
}

var schemaReferenceMetadataKeywords = map[string]bool{
	"title": true, "description": true, "default": true, "example": true,
	"examples": true, "deprecated": true, "externalDocs": true, "format": true,
	// These are evaluated by the enclosing object-property projection before
	// schemaType resolves the referenced component.
	"readOnly": true, "writeOnly": true,
}

func unsupportedSchemaChildren(key string, value any, path string) []string {
	switch key {
	case "allOf", "anyOf", "oneOf", "prefixItems":
		values, _ := value.([]any)
		var result []string
		for index, item := range values {
			result = append(result, unsupportedSchemaFeatures(item, appendOpenAPIPointer(path, fmt.Sprint(index)))...)
		}
		return result
	case "additionalProperties":
		if _, ok := value.(bool); ok {
			return nil
		}
		return unsupportedSchemaFeatures(value, path)
	case "contentSchema", "else", "if", "items", "not", "then", "unevaluatedItems", "unevaluatedProperties":
		return unsupportedSchemaFeatures(value, path)
	case "properties", "patternProperties", "dependentSchemas":
		values, _ := value.(map[string]any)
		keys := make([]string, 0, len(values))
		for name := range values {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		var result []string
		for _, name := range keys {
			result = append(result, unsupportedSchemaFeatures(values[name], appendOpenAPIPointer(path, name))...)
		}
		return result
	default:
		return nil
	}
}
