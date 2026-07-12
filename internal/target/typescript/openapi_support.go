package typescript

import (
	"fmt"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

// validateOpenAPISupport prevents valid HTTP/OpenAPI constructs from becoming
// inert raw fields. Features without an emitted client API or runtime behavior
// are rejected with their contract path until the target has an implementation.
func validateOpenAPISupport(document *ir.Document) error {
	return validateOpenAPISupportForTarget(document, "TypeScript")
}

func validateOpenAPISupportForTarget(document *ir.Document, target string) error {
	var unsupported []string
	root := document.Raw
	unsupported = append(unsupported, unsupportedRootFeatures(document, root)...)
	for _, operation := range document.Operations {
		path := openAPIPointer("paths", operation.Path, strings.ToLower(operation.Method))
		unsupported = append(unsupported, unsupportedOperationFeatures(document, operation.PathItemRaw, openAPIPointer("paths", operation.Path))...)
		unsupported = append(unsupported, unsupportedOperationFeatures(document, operation.Raw, path)...)
	}
	if len(unsupported) == 0 {
		return nil
	}
	sort.Strings(unsupported)
	return fmt.Errorf("%s target does not yet implement these OpenAPI features:\n- %s", target, strings.Join(unsupported, "\n- "))
}

func unsupportedRootFeatures(document *ir.Document, root map[string]any) []string {
	var result []string
	if self, _ := root["$self"].(string); self != "" {
		result = append(result, "#/$self (base URI resolution)")
	}
	if dialect, _ := root["jsonSchemaDialect"].(string); dialect != "" {
		result = append(result, "#/jsonSchemaDialect (dialect selection)")
	}
	for _, feature := range []struct {
		key     string
		detail  string
		present func(any) bool
	}{
		{"webhooks", "generated inbound webhook contracts", hasNonEmptyObject},
		{"security", "security requirement planning", hasNonEmptyList},
	} {
		if feature.present(root[feature.key]) {
			result = append(result, fmt.Sprintf("%s (%s)", openAPIPointer(feature.key), feature.detail))
		}
	}
	result = append(result, unsupportedServerVariables(root["servers"], openAPIPointer("servers"))...)
	components, _ := root["components"].(map[string]any)
	for _, feature := range []struct {
		key    string
		detail string
	}{
		{"callbacks", "generated callback contracts"},
		{"links", "generated link helpers"},
		{"mediaTypes", "reusable media type definitions"},
	} {
		if hasNonEmptyObject(components[feature.key]) {
			result = append(result, fmt.Sprintf("%s (%s)", openAPIPointer("components", feature.key), feature.detail))
		}
	}
	if hasNonEmptyObject(components["headers"]) {
		result = append(result, "#/components/headers (typed response header contracts)")
	}
	result = append(result, unsupportedSecuritySchemeFeatures(components["securitySchemes"], openAPIPointer("components", "securitySchemes"))...)
	result = append(result, unsupportedReusableComponentFeatures(document, components)...)
	return result
}

func unsupportedSecuritySchemeFeatures(value any, path string) []string {
	schemes, _ := value.(map[string]any)
	var result []string
	for _, name := range sortedAnyKeys(schemes) {
		scheme, _ := schemes[name].(map[string]any)
		keys := sortedAnyKeys(scheme)
		if len(keys) == 0 {
			result = append(result, appendOpenAPIPointer(path, name)+" (typed security providers)")
			continue
		}
		for _, key := range keys {
			if strings.HasPrefix(key, "x-") {
				continue
			}
			result = append(result, appendOpenAPIPointer(appendOpenAPIPointer(path, name), key)+" (typed security providers)")
		}
	}
	return result
}

func unsupportedReusableComponentFeatures(document *ir.Document, components map[string]any) []string {
	var result []string
	requestBodies, _ := components["requestBodies"].(map[string]any)
	for _, name := range sortedAnyKeys(requestBodies) {
		result = append(result, unsupportedRequestBodyFeatures(document, requestBodies[name], openAPIPointer("components", "requestBodies", name))...)
	}
	responses, _ := components["responses"].(map[string]any)
	for _, name := range sortedAnyKeys(responses) {
		response, _ := responses[name].(map[string]any)
		result = append(result, unsupportedResponseObjectFeatures(document, response, openAPIPointer("components", "responses", name))...)
	}
	parameters, _ := components["parameters"].(map[string]any)
	for _, name := range sortedAnyKeys(parameters) {
		result = append(result, unsupportedParameterObjectFeatures(document, parameters[name], openAPIPointer("components", "parameters", name))...)
	}
	pathItems, _ := components["pathItems"].(map[string]any)
	for _, name := range sortedAnyKeys(pathItems) {
		item, _ := pathItems[name].(map[string]any)
		result = append(result, unsupportedOperationFeatures(document, item, openAPIPointer("components", "pathItems", name))...)
	}
	return result
}

func unsupportedOperationFeatures(document *ir.Document, operation map[string]any, path string) []string {
	var result []string
	if servers, _ := operation["servers"].([]any); len(servers) > 1 {
		result = append(result, appendOpenAPIPointer(path, "servers")+" (scoped server selection)")
	}
	if hasNonEmptyList(operation["security"]) {
		result = append(result, fmt.Sprintf("%s (security requirement planning)", appendOpenAPIPointer(path, "security")))
	}
	if hasNonEmptyObject(operation["callbacks"]) {
		result = append(result, fmt.Sprintf("%s (generated callback contracts)", appendOpenAPIPointer(path, "callbacks")))
	}
	result = append(result, unsupportedServerVariables(operation["servers"], appendOpenAPIPointer(path, "servers"))...)
	result = append(result, unsupportedParameterFeatures(document, operation["parameters"], appendOpenAPIPointer(path, "parameters"))...)
	result = append(result, unsupportedRequestBodyFeatures(document, operation["requestBody"], appendOpenAPIPointer(path, "requestBody"))...)
	result = append(result, unsupportedResponseFeatures(document, operation["responses"], appendOpenAPIPointer(path, "responses"))...)
	return result
}

func unsupportedServerVariables(value any, path string) []string {
	servers, _ := value.([]any)
	var result []string
	for index, item := range servers {
		server, _ := item.(map[string]any)
		if hasNonEmptyObject(server["variables"]) {
			result = append(result, fmt.Sprintf("%s (server-variable selection)", appendOpenAPIPointer(appendOpenAPIPointer(path, fmt.Sprint(index)), "variables")))
		}
	}
	return result
}

func unsupportedParameterFeatures(document *ir.Document, value any, path string) []string {
	parameters, _ := value.([]any)
	var result []string
	for index, item := range parameters {
		result = append(result, unsupportedParameterObjectFeatures(document, item, appendOpenAPIPointer(path, fmt.Sprint(index)))...)
	}
	return result
}

func unsupportedParameterObjectFeatures(document *ir.Document, value any, path string) []string {
	parameter, _ := value.(map[string]any)
	var result []string
	if parameter["in"] == "querystring" {
		result = append(result, appendOpenAPIPointer(path, "in")+" (whole-querystring serialization)")
	}
	if allowReserved, _ := parameter["allowReserved"].(bool); allowReserved {
		result = append(result, appendOpenAPIPointer(path, "allowReserved")+" (reserved query character serialization)")
	}
	if allowEmptyValue, _ := parameter["allowEmptyValue"].(bool); allowEmptyValue {
		result = append(result, appendOpenAPIPointer(path, "allowEmptyValue")+" (empty-value parameter serialization)")
	}
	if content, _ := parameter["content"].(map[string]any); len(content) > 1 {
		result = append(result, appendOpenAPIPointer(path, "content")+" (Parameter Object content must define exactly one media type)")
	}
	if content, _ := parameter["content"].(map[string]any); len(content) == 1 {
		for _, mediaType := range sortedAnyKeys(content) {
			media, _ := content[mediaType].(map[string]any)
			schema, _ := media["schema"].(map[string]any)
			if !isJSONMediaType(mediaType) && schemaMayBeStructured(document, schema) {
				result = append(result, appendOpenAPIPointer(appendOpenAPIPointer(path, "content"), mediaType)+" (structured parameter content serialization)")
			}
		}
	}
	style, _ := parameter["style"].(string)
	if style == "cookie" {
		result = append(result, appendOpenAPIPointer(path, "style")+" (cookie parameter serialization)")
	}
	if (style == "spaceDelimited" || style == "pipeDelimited") && schemaMayBeObject(document, parameter["schema"]) {
		result = append(result, appendOpenAPIPointer(path, "style")+" (object serialization for "+style+")")
	}
	return result
}

func schemaMayBeObject(document *ir.Document, value any) bool {
	schema, _ := value.(map[string]any)
	schema = resolveSchemaReference(document, schema, make(map[string]bool))
	if schema["type"] == "object" {
		return true
	}
	_, hasProperties := schema["properties"]
	_, hasAdditional := schema["additionalProperties"]
	return hasProperties || hasAdditional
}

func schemaMayBeStructured(document *ir.Document, schema map[string]any) bool {
	schema = resolveSchemaReference(document, schema, make(map[string]bool))
	if schemaMayBeObject(document, schema) || schema["type"] == "array" {
		return true
	}
	_, hasItems := schema["items"]
	return hasItems
}

func unsupportedRequestBodyFeatures(document *ir.Document, value any, path string) []string {
	body, _ := value.(map[string]any)
	content, _ := body["content"].(map[string]any)
	return unsupportedMediaFeatures(document, content, appendOpenAPIPointer(path, "content"), true)
}

func unsupportedResponseFeatures(document *ir.Document, value any, path string) []string {
	responses, _ := value.(map[string]any)
	keys := sortedAnyKeys(responses)
	var result []string
	for _, status := range keys {
		response, _ := responses[status].(map[string]any)
		result = append(result, unsupportedResponseObjectFeatures(document, response, appendOpenAPIPointer(path, status))...)
	}
	return result
}

func unsupportedResponseObjectFeatures(document *ir.Document, response map[string]any, path string) []string {
	var result []string
	if hasNonEmptyObject(response["headers"]) {
		result = append(result, appendOpenAPIPointer(path, "headers")+" (typed response header contracts)")
	}
	if hasNonEmptyObject(response["links"]) {
		result = append(result, appendOpenAPIPointer(path, "links")+" (generated link helpers)")
	}
	content, _ := response["content"].(map[string]any)
	return append(result, unsupportedMediaFeatures(document, content, appendOpenAPIPointer(path, "content"), false)...)
}

func unsupportedMediaFeatures(document *ir.Document, content map[string]any, path string, request bool) []string {
	var result []string
	for _, mediaType := range sortedAnyKeys(content) {
		media, _ := content[mediaType].(map[string]any)
		itemPath := appendOpenAPIPointer(path, mediaType)
		normalizedMediaType := strings.ToLower(mediaType)
		schema, _ := media["schema"].(map[string]any)
		if strings.Contains(normalizedMediaType, "*") {
			result = append(result, itemPath+" (media-type wildcard negotiation)")
		}
		if strings.Contains(normalizedMediaType, "xml") {
			result = append(result, itemPath+" (XML media-type codec)")
		}
		if strings.Contains(normalizedMediaType, "event-stream") || strings.Contains(normalizedMediaType, "json-seq") || strings.Contains(normalizedMediaType, "ndjson") {
			if request {
				result = append(result, itemPath+" (streaming request encoder)")
			} else {
				result = append(result, itemPath+" (streaming response API)")
			}
		}
		if !isRuntimeMediaType(normalizedMediaType, schema) {
			result = append(result, itemPath+" (runtime media-type codec)")
		}
		if request && normalizedMediaType == "multipart/form-data" && multipartHasStructuredProperties(document, schema) {
			result = append(result, itemPath+" (structured multipart default encoding)")
		}
		if hasNonEmptyObject(media["encoding"]) {
			result = append(result, appendOpenAPIPointer(itemPath, "encoding")+" (multipart/form encoding)")
		}
		for _, feature := range []struct {
			key    string
			detail string
		}{
			{"itemSchema", "streaming item schema"},
			{"prefixEncoding", "positional multipart encoding"},
			{"itemEncoding", "streaming multipart item encoding"},
		} {
			if _, exists := media[feature.key]; exists {
				result = append(result, appendOpenAPIPointer(itemPath, feature.key)+" ("+feature.detail+")")
			}
		}
	}
	return result
}

func isRuntimeMediaType(mediaType string, schema map[string]any) bool {
	if isJSONMediaType(mediaType) || strings.HasPrefix(mediaType, "text/") || mediaType == "application/x-www-form-urlencoded" || mediaType == "multipart/form-data" {
		return true
	}
	return isBinaryMedia(mediaType, schema)
}

func multipartHasStructuredProperties(document *ir.Document, schema map[string]any) bool {
	schema = resolveSchemaReference(document, schema, make(map[string]bool))
	properties, _ := schema["properties"].(map[string]any)
	for _, name := range sortedAnyKeys(properties) {
		property, _ := properties[name].(map[string]any)
		if schemaMayBeStructured(document, property) {
			return true
		}
	}
	return false
}

func resolveSchemaReference(document *ir.Document, schema map[string]any, resolving map[string]bool) map[string]any {
	reference, _ := schema["$ref"].(string)
	if reference == "" {
		return schema
	}
	name, err := componentSchemaReferenceName(reference)
	if err != nil || resolving[name] {
		return schema
	}
	resolved, exists := document.ComponentSchemas[name]
	if !exists {
		return schema
	}
	resolving[name] = true
	defer delete(resolving, name)
	return resolveSchemaReference(document, resolved, resolving)
}

func sortedAnyKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func hasNonEmptyObject(value any) bool {
	values, ok := value.(map[string]any)
	return ok && len(values) > 0
}

func hasNonEmptyList(value any) bool {
	values, ok := value.([]any)
	return ok && len(values) > 0
}
