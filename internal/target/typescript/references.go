package typescript

import (
	"fmt"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func resolveComponentObject(document *ir.Document, object map[string]any, component string) (map[string]any, error) {
	return resolveComponentObjectRecursive(document, object, component, make(map[string]bool))
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
	resolved, ok := objects[strings.TrimPrefix(reference, prefix)].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unresolved %s reference %q", component, reference)
	}
	resolved, err := resolveComponentObjectRecursive(document, resolved, component, resolving)
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
