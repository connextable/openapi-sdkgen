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
	return validateOpenAPISupportWithServer(document, "TypeScript", false)
}

func validateOpenAPISupportForTarget(document *ir.Document, target string) error {
	return validateOpenAPISupportWithServer(document, target, false)
}

func validateOpenAPISupportWithServer(document *ir.Document, target string, includeServer bool) error {
	var unsupported []string
	root := document.Raw
	rootUnsupported := unsupportedRootFeatures(document, root)
	if includeServer {
		rootUnsupported = withoutServerAddonRootFeatures(rootUnsupported, rootSecurityIsInboundOnly(document))
	}
	unsupported = append(unsupported, rootUnsupported...)
	for _, operation := range document.Operations {
		path := openAPIPointer("paths", operation.Path, strings.ToLower(operation.Method))
		unsupported = append(unsupported, unsupportedOperationFeatures(document, operation.PathItemRaw, openAPIPointer("paths", operation.Path))...)
		unsupported = append(unsupported, unsupportedOperationFeatures(document, operation.Raw, path)...)
	}
	if includeServer {
		unsupported = withoutServerAddonFeatures(unsupported)
	}
	if len(unsupported) == 0 {
		return nil
	}
	sort.Strings(unsupported)
	return fmt.Errorf("%s target does not yet implement these OpenAPI features:\n- %s", target, strings.Join(unsupported, "\n- "))
}

func withoutServerAddonFeatures(features []string) []string {
	result := make([]string, 0, len(features))
	for _, feature := range features {
		if strings.Contains(feature, "(generated inbound webhook contracts)") || strings.Contains(feature, "(generated callback contracts)") {
			continue
		}
		result = append(result, feature)
	}
	return result
}

func withoutServerAddonRootFeatures(features []string, allowRootSecurity bool) []string {
	result := make([]string, 0, len(features))
	for _, feature := range features {
		if strings.Contains(feature, "(generated inbound webhook contracts)") ||
			strings.Contains(feature, "(generated callback contracts)") ||
			strings.HasPrefix(feature, "#/components/securitySchemes/") ||
			(allowRootSecurity && strings.HasPrefix(feature, "#/security ")) {
			continue
		}
		result = append(result, feature)
	}
	return result
}

func rootSecurityIsInboundOnly(document *ir.Document) bool {
	if !hasNonEmptyList(document.Raw["security"]) {
		return false
	}
	for _, operation := range document.Operations {
		security, declared := operation.Raw["security"]
		if !declared {
			return false
		}
		if hasNonEmptyList(security) {
			return false
		}
	}
	return true
}

func unsupportedRootFeatures(document *ir.Document, root map[string]any) []string {
	var result []string
	for _, feature := range []struct {
		key     string
		detail  string
		present func(any) bool
	}{
		{"webhooks", "generated inbound webhook contracts", hasNonEmptyObject},
	} {
		if feature.present(root[feature.key]) {
			result = append(result, fmt.Sprintf("%s (%s)", openAPIPointer(feature.key), feature.detail))
		}
	}
	components, _ := root["components"].(map[string]any)
	for _, feature := range []struct {
		key    string
		detail string
	}{
		{"callbacks", "generated callback contracts"},
	} {
		if hasNonEmptyObject(components[feature.key]) {
			result = append(result, fmt.Sprintf("%s (%s)", openAPIPointer("components", feature.key), feature.detail))
		}
	}
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
	if hasNonEmptyObject(operation["callbacks"]) {
		result = append(result, fmt.Sprintf("%s (generated callback contracts)", appendOpenAPIPointer(path, "callbacks")))
	}
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
	if content, _ := parameter["content"].(map[string]any); len(content) > 1 {
		result = append(result, appendOpenAPIPointer(path, "content")+" (Parameter Object content must define exactly one media type)")
	}
	if content, _ := parameter["content"].(map[string]any); len(content) == 1 {
		for _, mediaType := range sortedAnyKeys(content) {
			media, _ := content[mediaType].(map[string]any)
			resolved, err := resolveMediaTypeObject(document, media)
			if err != nil {
				result = append(result, appendOpenAPIPointer(appendOpenAPIPointer(path, "content"), mediaType)+" (reusable Media Type Object resolution)")
				continue
			}
			_ = resolved
		}
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
	result = append(result, unsupportedResponseHeaderFeatures(document, response["headers"], appendOpenAPIPointer(path, "headers"))...)
	content, _ := response["content"].(map[string]any)
	return append(result, unsupportedMediaFeatures(document, content, appendOpenAPIPointer(path, "content"), false)...)
}

func unsupportedResponseHeaderFeatures(document *ir.Document, value any, path string) []string {
	headers, _ := value.(map[string]any)
	var result []string
	for _, name := range sortedAnyKeys(headers) {
		header, _ := headers[name].(map[string]any)
		resolved, err := resolveComponentObject(document, header, "headers")
		if err != nil {
			continue
		}
		content, _ := resolved["content"].(map[string]any)
		if len(content) > 1 {
			result = append(result, appendOpenAPIPointer(appendOpenAPIPointer(path, name), "content")+" (Header Object content must define exactly one media type)")
			continue
		}
		result = append(result, unsupportedMediaFeatures(document, content, appendOpenAPIPointer(appendOpenAPIPointer(path, name), "content"), false)...)
	}
	return result
}

func unsupportedMediaFeatures(document *ir.Document, content map[string]any, path string, request bool) []string {
	var result []string
	for _, mediaType := range sortedAnyKeys(content) {
		media, _ := content[mediaType].(map[string]any)
		itemPath := appendOpenAPIPointer(path, mediaType)
		resolved, err := resolveMediaTypeObject(document, media)
		if err != nil {
			result = append(result, itemPath+" (reusable Media Type Object resolution)")
			continue
		}
		media = resolved
		normalizedMediaType := strings.ToLower(mediaType)
		streamingMultipart := strings.HasPrefix(normalizedMediaType, "multipart/") && media["itemSchema"] != nil
		streaming := isStreamMediaType(normalizedMediaType) || media["itemSchema"] != nil
		if streaming {
			if request {
				if _, hasItemSchema := media["itemSchema"]; !hasItemSchema {
					result = append(result, itemPath+" (streaming request encoder requires itemSchema)")
				}
			} else if _, hasItemSchema := media["itemSchema"]; !hasItemSchema {
				result = append(result, itemPath+" (streaming response API)")
			}
		}
		if hasNonEmptyObject(media["encoding"]) && (!request || (!strings.HasPrefix(normalizedMediaType, "multipart/") && normalizedMediaType != "application/x-www-form-urlencoded")) {
			result = append(result, appendOpenAPIPointer(itemPath, "encoding")+" (Encoding Object requires a form request media type)")
		}
		if hasNonEmptyObject(media["encoding"]) && (media["prefixEncoding"] != nil || media["itemEncoding"] != nil) {
			result = append(result, itemPath+" (encoding cannot be combined with prefixEncoding or itemEncoding)")
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
				supported := feature.key == "itemSchema" && streaming ||
					feature.key == "prefixEncoding" && strings.HasPrefix(normalizedMediaType, "multipart/") ||
					feature.key == "itemEncoding" && (request && strings.HasPrefix(normalizedMediaType, "multipart/") || streamingMultipart)
				if !supported {
					result = append(result, appendOpenAPIPointer(itemPath, feature.key)+" ("+feature.detail+")")
				}
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
