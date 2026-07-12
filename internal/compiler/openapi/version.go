package openapi

import (
	"fmt"
	"sort"
	"strings"
)

// validateVersionSpecificFeatures rejects syntax introduced after the source
// document's declared minor line. A later-version construct must never be
// silently lowered as though an older document had declared it.
func validateVersionSpecificFeatures(raw map[string]any, version VersionLine) error {
	if err := validateVersionedDocumentFields(raw, version); err != nil {
		return err
	}
	if version == Version30 {
		if _, exists := raw["paths"]; !exists {
			return versionFeatureError("#/paths", "paths is required by OpenAPI 3.0")
		}
		for _, key := range []string{"webhooks", "jsonSchemaDialect"} {
			if _, exists := raw[key]; exists {
				return versionFeatureError(pointer(key), key+" requires OpenAPI 3.1 or later")
			}
		}
		info, _ := raw["info"].(map[string]any)
		if _, exists := info["summary"]; exists {
			return versionFeatureError("#/info/summary", "info.summary requires OpenAPI 3.1 or later")
		}
		license, _ := info["license"].(map[string]any)
		if _, exists := license["identifier"]; exists {
			return versionFeatureError("#/info/license/identifier", "license.identifier requires OpenAPI 3.1 or later")
		}
		components, _ := raw["components"].(map[string]any)
		if _, exists := components["pathItems"]; exists {
			return versionFeatureError("#/components/pathItems", "components.pathItems requires OpenAPI 3.1 or later")
		}
		securitySchemes, _ := components["securitySchemes"].(map[string]any)
		for _, name := range sortedKeys(securitySchemes) {
			scheme, _ := securitySchemes[name].(map[string]any)
			if scheme["type"] == "mutualTLS" {
				return versionFeatureError(pointer("components", "securitySchemes", name, "type"), "mutualTLS requires OpenAPI 3.1 or later")
			}
		}
	}
	if version != Version32 {
		if _, exists := raw["$self"]; exists {
			return versionFeatureError("#/$self", "$self requires OpenAPI 3.2")
		}
		components, _ := raw["components"].(map[string]any)
		if _, exists := components["mediaTypes"]; exists {
			return versionFeatureError("#/components/mediaTypes", "components.mediaTypes requires OpenAPI 3.2")
		}
	}
	if err := validateVersionedPaths(raw, version); err != nil {
		return err
	}
	if version == Version30 {
		if err := validateOpenAPI30ReferenceObjects(raw, "#"); err != nil {
			return err
		}
	}
	return validateVersionedSchemas(raw, version)
}

func validateVersionedDocumentFields(raw map[string]any, version VersionLine) error {
	if err := validateServerFields(raw["servers"], "#/servers", version); err != nil {
		return err
	}
	if err := validateTagFields(raw["tags"], "#/tags", version); err != nil {
		return err
	}
	components, _ := raw["components"].(map[string]any)
	if err := validateSecuritySchemeFields(components["securitySchemes"], "#/components/securitySchemes", version); err != nil {
		return err
	}
	if err := validateExamplesFields(components["examples"], "#/components/examples", version); err != nil {
		return err
	}
	return nil
}

func validateSecuritySchemeFields(value any, path string, version VersionLine) error {
	if version == Version32 {
		return nil
	}
	schemes, _ := value.(map[string]any)
	for _, name := range sortedKeys(schemes) {
		scheme, _ := schemes[name].(map[string]any)
		for _, key := range []string{"oauth2MetadataUrl", "deprecated"} {
			if _, exists := scheme[key]; exists {
				return versionFeatureError(pointerFrom(path, name, key), "security scheme "+key+" requires OpenAPI 3.2")
			}
		}
		flows, _ := scheme["flows"].(map[string]any)
		if flow, exists := flows["deviceAuthorization"]; exists {
			if _, ok := flow.(map[string]any); ok {
				return versionFeatureError(pointerFrom(path, name, "flows", "deviceAuthorization"), "deviceAuthorization OAuth flow requires OpenAPI 3.2")
			}
		}
	}
	return nil
}

func validateServerFields(value any, path string, version VersionLine) error {
	if version == Version32 {
		return nil
	}
	servers, _ := value.([]any)
	for index, value := range servers {
		if err := validateServerObjectFields(value, pointerFrom(path, fmt.Sprint(index)), version); err != nil {
			return err
		}
	}
	return nil
}

func validateServerObjectFields(value any, path string, version VersionLine) error {
	if version == Version32 {
		return nil
	}
	server, _ := value.(map[string]any)
	if _, exists := server["name"]; exists {
		return versionFeatureError(pointerFrom(path, "name"), "server.name requires OpenAPI 3.2")
	}
	return nil
}

func validateTagFields(value any, path string, version VersionLine) error {
	if version == Version32 {
		return nil
	}
	tags, _ := value.([]any)
	for index, value := range tags {
		tag, _ := value.(map[string]any)
		for _, key := range []string{"summary", "parent", "kind"} {
			if _, exists := tag[key]; exists {
				return versionFeatureError(pointerFrom(path, fmt.Sprint(index), key), "tag."+key+" requires OpenAPI 3.2")
			}
		}
	}
	return nil
}

func validateExamplesFields(value any, path string, version VersionLine) error {
	if version == Version32 {
		return nil
	}
	examples, _ := value.(map[string]any)
	for _, name := range sortedKeys(examples) {
		example, _ := examples[name].(map[string]any)
		for _, key := range []string{"dataValue", "serializedValue"} {
			if _, exists := example[key]; exists {
				return versionFeatureError(pointerFrom(path, name, key), "Example Object "+key+" requires OpenAPI 3.2")
			}
		}
	}
	return nil
}

func validateVersionedPaths(raw map[string]any, version VersionLine) error {
	paths, _ := raw["paths"].(map[string]any)
	for _, path := range sortedKeys(paths) {
		if strings.HasPrefix(path, "x-") {
			continue
		}
		pathItem, _ := paths[path].(map[string]any)
		if err := validateVersionedPathItem(pathItem, pointer("paths", path), version); err != nil {
			return err
		}
	}
	webhooks, _ := raw["webhooks"].(map[string]any)
	for _, name := range sortedKeys(webhooks) {
		pathItem, _ := webhooks[name].(map[string]any)
		if err := validateVersionedPathItem(pathItem, pointer("webhooks", name), version); err != nil {
			return err
		}
	}
	components, _ := raw["components"].(map[string]any)
	pathItems, _ := components["pathItems"].(map[string]any)
	for _, name := range sortedKeys(pathItems) {
		item, _ := pathItems[name].(map[string]any)
		if err := validateVersionedPathItem(item, pointer("components", "pathItems", name), version); err != nil {
			return err
		}
	}
	if err := validateVersionedCallbacks(components["callbacks"], pointer("components", "callbacks"), version); err != nil {
		return err
	}
	parameters, _ := components["parameters"].(map[string]any)
	for _, name := range sortedKeys(parameters) {
		if err := validateQuerystringParameter(parameters[name], pointer("components", "parameters", name), version); err != nil {
			return err
		}
	}
	return nil
}

func validateVersionedPathItem(pathItem map[string]any, path string, version VersionLine) error {
	if err := validateServerFields(pathItem["servers"], pointerFrom(path, "servers"), version); err != nil {
		return err
	}
	if version != Version32 {
		if _, exists := pathItem["query"]; exists {
			return versionFeatureError(pointerFrom(path, "query"), "query requires OpenAPI 3.2")
		}
		if _, exists := pathItem["additionalOperations"]; exists {
			return versionFeatureError(pointerFrom(path, "additionalOperations"), "additionalOperations requires OpenAPI 3.2")
		}
	}
	if err := validateQuerystringParameters(pathItem["parameters"], pointerFrom(path, "parameters"), version); err != nil {
		return err
	}
	for _, method := range sortedKeys(pathItem) {
		operation, _ := pathItem[method].(map[string]any)
		if operation == nil {
			continue
		}
		if err := validateQuerystringParameters(operation["parameters"], pointerFrom(path, method, "parameters"), version); err != nil {
			return err
		}
		if err := validateServerFields(operation["servers"], pointerFrom(path, method, "servers"), version); err != nil {
			return err
		}
		if err := validateTagFields(operation["tags"], pointerFrom(path, method, "tags"), version); err != nil {
			return err
		}
		if err := validateVersionedCallbacks(operation["callbacks"], pointerFrom(path, method, "callbacks"), version); err != nil {
			return err
		}
	}
	return nil
}

func validateVersionedCallbacks(value any, path string, version VersionLine) error {
	callbacks, _ := value.(map[string]any)
	for _, name := range sortedKeys(callbacks) {
		callback, _ := callbacks[name].(map[string]any)
		for _, expression := range sortedKeys(callback) {
			pathItem, _ := callback[expression].(map[string]any)
			if err := validateVersionedPathItem(pathItem, pointerFrom(path, name, expression), version); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateQuerystringParameters(value any, path string, version VersionLine) error {
	if version == Version32 {
		return nil
	}
	parameters, _ := value.([]any)
	for index, value := range parameters {
		if err := validateQuerystringParameter(value, pointerFrom(path, fmt.Sprint(index)), version); err != nil {
			return err
		}
	}
	return nil
}

func validateQuerystringParameter(value any, path string, version VersionLine) error {
	if version == Version32 {
		return nil
	}
	parameter, _ := value.(map[string]any)
	if style, _ := parameter["style"].(string); style == "cookie" {
		return versionFeatureError(pointerFrom(path, "style"), "cookie parameter style requires OpenAPI 3.2")
	}
	if parameter["in"] == "querystring" {
		return versionFeatureError(pointerFrom(path, "in"), "querystring parameters require OpenAPI 3.2")
	}
	return nil
}

func validateVersionedSchemas(raw map[string]any, version VersionLine) error {
	components, _ := raw["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	for _, name := range sortedKeys(schemas) {
		if err := validateSchemaVersion(schemas[name], pointer("components", "schemas", name), version); err != nil {
			return err
		}
	}
	return validateSchemaValues(raw, "#", version)
}

func validateSchemaValues(value any, path string, version VersionLine) error {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range sortedKeys(typed) {
			if strings.HasPrefix(key, "x-") {
				continue
			}
			if key == "examples" {
				if err := validateExamplesFields(typed[key], pointerFrom(path, key), version); err != nil {
					return err
				}
				continue
			}
			if isLiteralOpenAPIValue(key) {
				// Example values and Schema annotations are arbitrary instance
				// data. Their keys must not be interpreted as OpenAPI or JSON
				// Schema syntax while checking the surrounding document version.
				continue
			}
			item := typed[key]
			child := pointerFrom(path, key)
			if path == "#/components" && key == "schemas" {
				// Component schemas are traversed as Schema Objects above. Their
				// arbitrary JSON Schema vocabulary is not Media Type syntax.
				continue
			}
			if key == "content" {
				if err := validateMediaTypeFeatures(item, child, version); err != nil {
					return err
				}
				continue
			}
			if key == "server" {
				if err := validateServerObjectFields(item, child, version); err != nil {
					return err
				}
			}
			if key == "responses" {
				if err := validateResponseFields(item, child, version); err != nil {
					return err
				}
				continue
			}
			if key == "schema" {
				if err := validateSchemaVersion(item, child, version); err != nil {
					return err
				}
				continue
			}
			if err := validateSchemaValues(item, child, version); err != nil {
				return err
			}
		}
	case []any:
		for index, item := range typed {
			if err := validateSchemaValues(item, pointerFrom(path, fmt.Sprint(index)), version); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateResponseFields(value any, path string, version VersionLine) error {
	responses, _ := value.(map[string]any)
	for _, status := range sortedKeys(responses) {
		response, _ := responses[status].(map[string]any)
		if version != Version32 {
			if _, exists := response["summary"]; exists {
				return versionFeatureError(pointerFrom(path, status, "summary"), "response.summary requires OpenAPI 3.2")
			}
		}
		if err := validateSchemaValues(response, pointerFrom(path, status), version); err != nil {
			return err
		}
	}
	return nil
}

func validateMediaTypeFeatures(value any, path string, version VersionLine) error {
	mediaTypes, _ := value.(map[string]any)
	for _, mediaType := range sortedKeys(mediaTypes) {
		media, _ := mediaTypes[mediaType].(map[string]any)
		mediaPath := pointerFrom(path, mediaType)
		for _, key := range []string{"itemSchema", "prefixEncoding", "itemEncoding"} {
			if _, exists := media[key]; exists && version != Version32 {
				return versionFeatureError(pointerFrom(mediaPath, key), key+" requires OpenAPI 3.2")
			}
		}
		for _, key := range sortedKeys(media) {
			if strings.HasPrefix(key, "x-") {
				continue
			}
			if key == "examples" {
				if err := validateExamplesFields(media[key], pointerFrom(mediaPath, key), version); err != nil {
					return err
				}
				continue
			}
			if isLiteralOpenAPIValue(key) {
				continue
			}
			item := media[key]
			child := pointerFrom(mediaPath, key)
			if key == "schema" || key == "itemSchema" {
				if err := validateSchemaVersion(item, child, version); err != nil {
					return err
				}
				continue
			}
			if err := validateSchemaValues(item, child, version); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSchemaVersion(value any, path string, version VersionLine) error {
	if version == Version31 {
		return validateOpenAPI32SchemaFields(value, path)
	}
	if version != Version30 {
		return nil
	}
	if err := validateOpenAPI32SchemaFields(value, path); err != nil {
		return err
	}
	schema, _ := value.(map[string]any)
	for _, key := range []string{"exclusiveMaximum", "exclusiveMinimum"} {
		if value, exists := schema[key]; exists {
			if _, isBoolean := value.(bool); !isBoolean {
				return versionFeatureError(pointerFrom(path, key), key+" must be boolean in OpenAPI 3.0")
			}
		}
	}
	if _, isTypeArray := schema["type"].([]any); isTypeArray {
		return versionFeatureError(pointerFrom(path, "type"), "type arrays require OpenAPI 3.1 or later")
	}
	for _, key := range sortedKeys(schema) {
		if openAPI31SchemaKeywords[key] {
			return versionFeatureError(pointerFrom(path, key), key+" requires OpenAPI 3.1 or later")
		}
	}
	return validateSchemaVersionChildren(schema, path)
}

func validateOpenAPI32SchemaFields(value any, path string) error {
	schema, _ := value.(map[string]any)
	if discriminator, _ := schema["discriminator"].(map[string]any); discriminator != nil {
		if _, exists := discriminator["defaultMapping"]; exists {
			return versionFeatureError(pointerFrom(path, "discriminator", "defaultMapping"), "discriminator.defaultMapping requires OpenAPI 3.2")
		}
	}
	if xml, _ := schema["xml"].(map[string]any); xml != nil {
		if _, exists := xml["nodeType"]; exists {
			return versionFeatureError(pointerFrom(path, "xml", "nodeType"), "xml.nodeType requires OpenAPI 3.2")
		}
	}
	for _, key := range []string{"additionalProperties", "contains", "contentSchema", "else", "if", "items", "not", "propertyNames", "then", "unevaluatedItems", "unevaluatedProperties"} {
		if nested, exists := schema[key]; exists {
			if err := validateOpenAPI32SchemaFields(nested, pointerFrom(path, key)); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		values, _ := schema[key].([]any)
		for index, nested := range values {
			if err := validateOpenAPI32SchemaFields(nested, pointerFrom(path, key, fmt.Sprint(index))); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"$defs", "dependentSchemas", "patternProperties", "properties"} {
		values, _ := schema[key].(map[string]any)
		for _, name := range sortedKeys(values) {
			if err := validateOpenAPI32SchemaFields(values[name], pointerFrom(path, key, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateOpenAPI30ReferenceObjects(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		if reference, _ := typed["$ref"].(string); reference != "" {
			if !isPathItemReferencePath(path) {
				for _, key := range sortedKeys(typed) {
					if key != "$ref" && !strings.HasPrefix(key, "x-") {
						return versionFeatureError(pointerFrom(path, key), "Reference Object siblings require OpenAPI 3.1 or later")
					}
				}
			}
		}
		for _, key := range sortedKeys(typed) {
			if strings.HasPrefix(key, "x-") {
				continue
			}
			if isLiteralOpenAPIValue(key) {
				continue
			}
			if err := validateOpenAPI30ReferenceObjects(typed[key], pointerFrom(path, key)); err != nil {
				return err
			}
		}
	case []any:
		for index, item := range typed {
			if err := validateOpenAPI30ReferenceObjects(item, pointerFrom(path, fmt.Sprint(index))); err != nil {
				return err
			}
		}
	}
	return nil
}

func isPathItemReferencePath(path string) bool {
	segments := strings.Split(strings.TrimPrefix(path, "#/"), "/")
	if len(segments) == 2 && (segments[0] == "paths" || segments[0] == "webhooks") {
		return true
	}
	if len(segments) == 3 && segments[0] == "components" && segments[1] == "pathItems" {
		return true
	}
	for index := len(segments) - 1; index >= 0; index-- {
		segment := segments[index]
		if segment == "callbacks" {
			// A callback map's direct value is a Path Item. Any deeper segment is
			// an operation, response, or another nested object and uses a Reference
			// Object, whose 3.0 siblings must be rejected.
			return len(segments) == index+3
		}
	}
	return false
}

func isLiteralOpenAPIValue(key string) bool {
	switch key {
	case "const", "default", "enum", "example", "value", "dataValue", "serializedValue":
		return true
	default:
		return false
	}
}

func validateSchemaVersionChildren(schema map[string]any, path string) error {
	for _, key := range []string{"additionalProperties", "contains", "contentSchema", "else", "if", "items", "not", "propertyNames", "then", "unevaluatedItems", "unevaluatedProperties"} {
		if value, exists := schema[key]; exists {
			if err := validateSchemaVersion(value, pointerFrom(path, key), Version30); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		values, _ := schema[key].([]any)
		for index, value := range values {
			if err := validateSchemaVersion(value, pointerFrom(path, key, fmt.Sprint(index)), Version30); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"dependentSchemas", "patternProperties", "properties"} {
		values, _ := schema[key].(map[string]any)
		for _, name := range sortedKeys(values) {
			if err := validateSchemaVersion(values[name], pointerFrom(path, key, name), Version30); err != nil {
				return err
			}
		}
	}
	return nil
}

var openAPI31SchemaKeywords = map[string]bool{
	"$anchor": true, "$comment": true, "$defs": true, "$dynamicAnchor": true,
	"$dynamicRef": true, "$id": true, "$schema": true, "$vocabulary": true,
	"const": true, "contains": true, "contentEncoding": true, "contentMediaType": true,
	"contentSchema": true, "dependentRequired": true, "dependentSchemas": true,
	"else": true, "examples": true, "if": true, "maxContains": true, "minContains": true,
	"patternProperties": true, "prefixItems": true, "propertyNames": true, "then": true,
	"unevaluatedItems": true, "unevaluatedProperties": true,
}

func versionFeatureError(path, detail string) error {
	return fmt.Errorf("OpenAPI version feature at %s: %s", path, detail)
}

func pointer(parts ...string) string { return pointerFrom("#", parts...) }

func pointerFrom(path string, parts ...string) string {
	for _, part := range parts {
		path += "/" + strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
	}
	return path
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
