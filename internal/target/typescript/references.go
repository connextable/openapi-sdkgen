package typescript

import (
	"fmt"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func resolveComponentObject(document *ir.Document, object map[string]any, component string) (map[string]any, error) {
	return resolveComponentObjectRecursive(document, object, component, make(map[string]bool))
}

// resolveMediaTypeObject resolves an OpenAPI 3.2 Media Type Object reference.
// It shares the Component Object merge semantics used by other reusable
// OpenAPI surfaces.
func resolveMediaTypeObject(document *ir.Document, object map[string]any) (map[string]any, error) {
	return resolveComponentObject(document, object, "mediaTypes")
}

func resolveComponentObjectRecursive(document *ir.Document, object map[string]any, component string, resolving map[string]bool) (map[string]any, error) {
	reference, _ := object["$ref"].(string)
	if reference == "" {
		return object, nil
	}
	prefix := "#/components/" + component + "/"
	if !strings.HasPrefix(reference, prefix) {
		return nil, fmt.Errorf("external %s reference %q is not supported", component, reference)
	}
	if resolving[reference] {
		return nil, fmt.Errorf("cyclic %s reference %q", component, reference)
	}
	resolving[reference] = true
	defer delete(resolving, reference)
	components, _ := document.Raw["components"].(map[string]any)
	objects, _ := components[component].(map[string]any)
	name, err := componentReferenceName(reference, component)
	if err != nil {
		return nil, err
	}
	resolved, ok := objects[name].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unresolved %s reference %q", component, reference)
	}
	resolved, err = resolveComponentObjectRecursive(document, resolved, component, resolving)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]any, len(resolved)+len(object))
	for key, value := range resolved {
		merged[key] = value
	}
	for key, value := range object {
		if key != "$ref" {
			merged[key] = value
		}
	}
	return merged, nil
}

func componentReferenceName(reference, component string) (string, error) {
	prefix := "#/components/" + component + "/"
	if !strings.HasPrefix(reference, prefix) {
		return "", fmt.Errorf("external %s reference %q is not supported", component, reference)
	}
	token := strings.TrimPrefix(reference, prefix)
	if token == "" || strings.Contains(token, "/") {
		return "", fmt.Errorf("%s reference %q must target one component", component, reference)
	}
	var output strings.Builder
	for index := 0; index < len(token); index++ {
		if token[index] != '~' {
			output.WriteByte(token[index])
			continue
		}
		if index+1 >= len(token) {
			return "", fmt.Errorf("%s reference %q has an invalid JSON Pointer escape", component, reference)
		}
		index++
		switch token[index] {
		case '0':
			output.WriteByte('~')
		case '1':
			output.WriteByte('/')
		default:
			return "", fmt.Errorf("%s reference %q has an invalid JSON Pointer escape", component, reference)
		}
	}
	return output.String(), nil
}
