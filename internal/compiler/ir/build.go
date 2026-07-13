package ir

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	openapidoc "github.com/connextable/openapi-sdkgen/internal/compiler/openapi"
)

var standardMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace", "query"}

func Build(document *openapidoc.Document) (*Document, error) {
	if document == nil {
		return nil, fmt.Errorf("OpenAPI document is nil")
	}
	versionLine := document.Version
	if versionLine == "" {
		var err error
		versionLine, err = openapidoc.DetectVersionLine(stringValue(document.Raw, "openapi"))
		if err != nil {
			return nil, err
		}
	}
	info, _ := document.Raw["info"].(map[string]any)
	result := &Document{
		Title:              stringValue(info, "title"),
		ContractVersion:    stringValue(info, "version"),
		OpenAPIVersion:     stringValue(document.Raw, "openapi"),
		OpenAPIVersionLine: string(versionLine),
		Servers:            readServers(document.Raw["servers"]),
		ComponentSchemas:   readComponentSchemas(document.Raw),
		Schemas:            readCompiledSchemas(document.Raw, versionLine),
		Raw:                document.Raw,
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
		if strings.HasPrefix(path, "x-") {
			continue
		}
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
			if method == "query" && versionLine != openapidoc.Version32 {
				return nil, fmt.Errorf("OpenAPI 3.2 feature at %s: query method is not available in OpenAPI %s.x", jsonPointer("paths", path, method), versionLine)
			}
			result.Operations = append(result.Operations, buildOperation(path, strings.ToUpper(method), pathItem, operation))
		}
		additional, _ := pathItem["additionalOperations"].(map[string]any)
		if len(additional) > 0 && versionLine != openapidoc.Version32 {
			return nil, fmt.Errorf("OpenAPI 3.2 feature at %s: additionalOperations is not available in OpenAPI %s.x", jsonPointer("paths", path, "additionalOperations"), versionLine)
		}
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

func readCompiledSchemas(raw map[string]any, version openapidoc.VersionLine) map[string]Schema {
	components, _ := raw["components"].(map[string]any)
	values, _ := components["schemas"].(map[string]any)
	result := make(map[string]Schema, len(values))
	dialect := defaultSchemaDialect(raw, version)
	base := documentResourceURI(raw)
	for _, name := range sortedKeys(values) {
		value := values[name]
		pointer := jsonPointer("components", "schemas", name)
		resource := base + "#" + pointer
		schemaDialect := dialect
		if object, ok := value.(map[string]any); ok {
			if identifier, ok := object["$id"].(string); ok && identifier != "" {
				resource = resolveSchemaResourceURI(base, identifier)
			}
			if declaredDialect, ok := object["$schema"].(string); ok && declaredDialect != "" {
				schemaDialect = declaredDialect
			}
		}
		result[name] = Schema{Name: name, Pointer: pointer, ResourceURI: resource, Dialect: schemaDialect, Value: value}
	}
	return result
}

func documentResourceURI(raw map[string]any) string {
	for _, key := range []string{"$self", "$id"} {
		if value, ok := raw[key].(string); ok && value != "" {
			return strings.TrimSuffix(value, "#")
		}
	}
	return "urn:openapi-sdkgen:document"
}

func defaultSchemaDialect(raw map[string]any, version openapidoc.VersionLine) string {
	if value, ok := raw["jsonSchemaDialect"].(string); ok && value != "" {
		return value
	}
	if version == openapidoc.Version30 {
		return "https://spec.openapis.org/oas/3.0/dialect/base"
	}
	return "https://spec.openapis.org/oas/3.1/dialect/base"
}

func resolveSchemaResourceURI(base, identifier string) string {
	if base == "" || strings.HasPrefix(base, "urn:") {
		return identifier
	}
	baseURI, baseErr := url.Parse(base)
	identifierURI, identifierErr := url.Parse(identifier)
	if baseErr != nil || identifierErr != nil {
		return identifier
	}
	return baseURI.ResolveReference(identifierURI).String()
}

func jsonPointer(parts ...string) string {
	encoded := make([]string, 0, len(parts))
	for _, part := range parts {
		encoded = append(encoded, strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1"))
	}
	return "/" + strings.Join(encoded, "/")
}

func resolvePathItem(document, pathItem map[string]any, resolving map[string]bool) (map[string]any, error) {
	reference, _ := pathItem["$ref"].(string)
	if reference == "" {
		return pathItem, nil
	}
	if !strings.HasPrefix(reference, "#/") {
		return nil, fmt.Errorf("external path item reference %q is not supported", reference)
	}
	if resolving[reference] {
		return nil, fmt.Errorf("cyclic path item reference %q", reference)
	}
	resolving[reference] = true
	defer delete(resolving, reference)
	resolved, err := localPathItemReference(document, reference)
	if err != nil {
		return nil, err
	}
	resolved, err = resolvePathItem(document, resolved, resolving)
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

func localPathItemReference(document map[string]any, reference string) (map[string]any, error) {
	var value any = document
	for _, token := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
		name, err := jsonPointerToken(token)
		if err != nil {
			return nil, fmt.Errorf("invalid path item reference %q: %w", reference, err)
		}
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unresolved path item reference %q", reference)
		}
		value, ok = object[name]
		if !ok {
			return nil, fmt.Errorf("unresolved path item reference %q", reference)
		}
	}
	pathItem, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unresolved path item reference %q", reference)
	}
	return pathItem, nil
}

func jsonPointerToken(token string) (string, error) {
	if token == "" || strings.Contains(token, "/") {
		return "", fmt.Errorf("must target one object")
	}
	var output strings.Builder
	for index := 0; index < len(token); index++ {
		if token[index] != '~' {
			output.WriteByte(token[index])
			continue
		}
		if index+1 >= len(token) {
			return "", fmt.Errorf("invalid JSON Pointer escape")
		}
		index++
		switch token[index] {
		case '0':
			output.WriteByte('~')
		case '1':
			output.WriteByte('/')
		default:
			return "", fmt.Errorf("invalid JSON Pointer escape")
		}
	}
	return output.String(), nil
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
