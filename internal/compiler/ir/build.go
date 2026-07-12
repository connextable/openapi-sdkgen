package ir

import (
	"fmt"
	"sort"
	"strings"

	openapidoc "github.com/connextable/openapi-sdkgen/internal/compiler/openapi"
)

var standardMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace", "query"}

func Build(document *openapidoc.Document) (*Document, error) {
	if document == nil {
		return nil, fmt.Errorf("OpenAPI document is nil")
	}
	info, _ := document.Raw["info"].(map[string]any)
	result := &Document{
		Title:            stringValue(info, "title"),
		ContractVersion:  stringValue(info, "version"),
		OpenAPIVersion:   stringValue(document.Raw, "openapi"),
		Servers:          readServers(document.Raw["servers"]),
		ComponentSchemas: readComponentSchemas(document.Raw),
		Raw:              document.Raw,
	}

	paths := map[string]any{}
	if value, exists := document.Raw["paths"]; exists {
		var ok bool
		paths, ok = value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("OpenAPI paths must be an object")
		}
	}
	pathNames := sortedKeys(paths)
	for _, path := range pathNames {
		pathItem, ok := paths[path].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path item %q must be an object", path)
		}
		pathItem, err := resolvePathItem(document.Raw, pathItem, make(map[string]bool))
		if err != nil {
			return nil, fmt.Errorf("path item %q: %w", path, err)
		}
		for _, method := range standardMethods {
			operation, ok := pathItem[method].(map[string]any)
			if !ok {
				continue
			}
			result.Operations = append(result.Operations, buildOperation(path, strings.ToUpper(method), pathItem, operation))
		}
		additional, _ := pathItem["additionalOperations"].(map[string]any)
		for _, method := range sortedKeys(additional) {
			operation, ok := additional[method].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("additional operation %q %q must be an object", method, path)
			}
			result.Operations = append(result.Operations, buildOperation(path, method, pathItem, operation))
		}
	}
	sort.SliceStable(result.Operations, func(i, j int) bool {
		if result.Operations[i].Path == result.Operations[j].Path {
			return result.Operations[i].Method < result.Operations[j].Method
		}
		return result.Operations[i].Path < result.Operations[j].Path
	})
	return result, nil
}

func resolvePathItem(document, pathItem map[string]any, resolving map[string]bool) (map[string]any, error) {
	reference, _ := pathItem["$ref"].(string)
	if reference == "" {
		return pathItem, nil
	}
	const prefix = "#/components/pathItems/"
	if !strings.HasPrefix(reference, prefix) {
		return nil, fmt.Errorf("external path item reference %q is not supported", reference)
	}
	if resolving[reference] {
		return nil, fmt.Errorf("cyclic path item reference %q", reference)
	}
	resolving[reference] = true
	defer delete(resolving, reference)
	components, _ := document["components"].(map[string]any)
	pathItems, _ := components["pathItems"].(map[string]any)
	resolved, ok := pathItems[strings.TrimPrefix(reference, prefix)].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unresolved path item reference %q", reference)
	}
	resolved, err := resolvePathItem(document, resolved, resolving)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]any, len(resolved)+len(pathItem))
	for key, value := range resolved {
		merged[key] = value
	}
	for key, value := range pathItem {
		if key != "$ref" {
			merged[key] = value
		}
	}
	return merged, nil
}

func buildOperation(path, method string, pathItemRaw, raw map[string]any) Operation {
	return Operation{
		OperationID:        stringValue(raw, "operationId"),
		Method:             method,
		Path:               path,
		Summary:            stringValue(raw, "summary"),
		Description:        stringValue(raw, "description"),
		Tags:               stringSlice(raw["tags"]),
		Visibility:         stringValue(raw, "x-sdk-visibility"),
		Envelope:           stringValue(raw, "x-envelope"),
		Concurrency:        stringValue(raw, "x-concurrency"),
		Idempotency:        stringValue(raw, "x-idempotency"),
		Pagination:         stringValue(raw, "x-pagination"),
		PathParameterOrder: templateParameters(path),
		PathItemRaw:        pathItemRaw,
		Raw:                raw,
	}
}

func readServers(value any) []Server {
	items, _ := value.([]any)
	servers := make([]Server, 0, len(items))
	for _, item := range items {
		server, ok := item.(map[string]any)
		if !ok {
			continue
		}
		servers = append(servers, Server{
			URL:         stringValue(server, "url"),
			Description: stringValue(server, "description"),
		})
	}
	return servers
}

func readComponentSchemas(raw map[string]any) map[string]map[string]any {
	components, _ := raw["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	result := make(map[string]map[string]any, len(schemas))
	for _, name := range sortedKeys(schemas) {
		if schema, ok := schemas[name].(map[string]any); ok {
			result[name] = schema
		}
	}
	return result
}

func stringValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func stringSlice(value any) []string {
	values, _ := value.([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func templateParameters(path string) []string {
	var result []string
	for {
		start := strings.IndexByte(path, '{')
		if start < 0 {
			return result
		}
		path = path[start+1:]
		end := strings.IndexByte(path, '}')
		if end < 0 {
			return result
		}
		result = append(result, path[:end])
		path = path[end+1:]
	}
}
