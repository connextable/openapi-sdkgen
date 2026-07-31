package sdkgen

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"openapi-sdkgen/internal/diagnostic"
)

func unresolvedLocalReferenceDiagnostics(value any, source string) []diagnostic.Diagnostic {
	var result []diagnostic.Diagnostic
	scanUnresolvedLocalReferences(value, value, nil, source, &result)
	return diagnostic.Sort(result)
}

func scanUnresolvedLocalReferences(root, value any, path []string, source string, result *[]diagnostic.Diagnostic) {
	switch typed := value.(type) {
	case map[string]any:
		if reference, _ := typed["$ref"].(string); strings.HasPrefix(reference, "#/") {
			if _, found := resolveLocalReference(root, reference); !found {
				*result = append(*result, diagnostic.Diagnostic{
					Severity: diagnostic.SeverityError,
					Code:     "SDKGEN-E120",
					Phase:    diagnostic.PhaseReferences,
					Location: diagnostic.Location{
						Source:  source,
						Pointer: sourceJSONPointer(append(path, "$ref")),
					},
					Message: fmt.Sprintf("Local reference %q does not resolve to a value in this document.", reference),
					Hint:    "Correct the JSON Pointer or declare the referenced OpenAPI component.",
				})
			}
		}
		for name, child := range typed {
			if name == "$ref" || referenceTraversalOpaque(path, name, child) {
				continue
			}
			scanUnresolvedLocalReferences(root, child, append(path, name), source, result)
		}
	case []any:
		for index, child := range typed {
			scanUnresolvedLocalReferences(root, child, append(path, strconv.Itoa(index)), source, result)
		}
	}
}

func referenceScanLiteralKey(name string, value any) bool {
	switch name {
	case "const", "dataValue", "default", "enum", "example", "serializedValue", "value":
		return true
	case "examples":
		_, literal := value.([]any)
		return literal
	default:
		return false
	}
}

func resolveLocalReference(root any, reference string) (any, bool) {
	fragment, err := url.PathUnescape(strings.TrimPrefix(reference, "#"))
	if err != nil || !strings.HasPrefix(fragment, "/") {
		return nil, false
	}
	current := root
	for _, encoded := range strings.Split(strings.TrimPrefix(fragment, "/"), "/") {
		token, ok := decodeReferencePointerToken(encoded)
		if !ok {
			return nil, false
		}
		switch typed := current.(type) {
		case map[string]any:
			current, ok = typed[token]
			if !ok {
				return nil, false
			}
		case []any:
			index, indexErr := strconv.Atoi(token)
			if indexErr != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func decodeReferencePointerToken(value string) (string, bool) {
	var result strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '~' {
			result.WriteByte(value[index])
			continue
		}
		if index+1 >= len(value) {
			return "", false
		}
		index++
		switch value[index] {
		case '0':
			result.WriteByte('~')
		case '1':
			result.WriteByte('/')
		default:
			return "", false
		}
	}
	return result.String(), true
}
