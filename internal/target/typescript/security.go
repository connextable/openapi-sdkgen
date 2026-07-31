package typescript

import (
	"fmt"
	"sort"
	"strings"

	"openapi-sdkgen/internal/compiler/ir"
)

type operationSecurityRequirement struct {
	id    string
	names []string
	value map[string]any
}

func operationSecurityRequirements(document *ir.Document, operation ir.Operation) ([]operationSecurityRequirement, bool, error) {
	value, exists := operation.Raw["security"]
	if !exists {
		value, exists = document.Raw["security"]
	}
	if !exists {
		return nil, false, nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil, false, fmt.Errorf("security must be an array")
	}
	if len(values) == 0 {
		return nil, false, nil
	}
	result := make([]operationSecurityRequirement, 0, len(values))
	ids := make(map[string]int, len(values))
	for index, value := range values {
		requirement, ok := value.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("security requirement %d must be an object", index)
		}
		names := sortedAnyKeys(requirement)
		id := "anonymous"
		if len(names) > 0 {
			id = strings.Join(names, "__")
		}
		baseID := id
		if count := ids[baseID]; count > 0 {
			id = fmt.Sprintf("%s__%d", baseID, count+1)
		}
		ids[baseID]++
		result = append(result, operationSecurityRequirement{id: id, names: names, value: requirement})
	}
	return result, true, nil
}

func operationRequiresSecuritySelection(document *ir.Document, operation ir.Operation) (bool, error) {
	requirements, _, err := operationSecurityRequirements(document, operation)
	if err != nil {
		return false, err
	}
	return len(requirements) > 1, nil
}

// operationSecurityDefinition lowers an operation's effective OpenAPI Security
// Requirement Object. An absent operation field inherits the root field;
// explicit `security: []` disables that inheritance.
func operationSecurityDefinition(document *ir.Document, operation ir.Operation) (string, bool, error) {
	requirements, hasSecurity, err := operationSecurityRequirements(document, operation)
	if err != nil || !hasSecurity {
		return "", hasSecurity, err
	}
	components, _ := document.Raw["components"].(map[string]any)
	schemes, _ := components["securitySchemes"].(map[string]any)
	entries := make([]string, 0, len(requirements))
	for index, requirement := range requirements {
		definitions := make([]string, 0, len(requirement.names))
		for _, name := range requirement.names {
			scheme, ok := schemes[name].(map[string]any)
			if !ok {
				return "", false, fmt.Errorf("security requirement %d references unknown scheme %q", index, name)
			}
			definition, err := securitySchemeDefinition(name, scheme, requirement.value[name])
			if err != nil {
				return "", false, err
			}
			definitions = append(definitions, definition)
		}
		entries = append(entries, "{ id: "+quoteTS(requirement.id)+", schemes: ["+strings.Join(definitions, ", ")+"] }")
	}
	return "[" + strings.Join(entries, ", ") + "]", true, nil
}

func securitySchemeDefinition(name string, scheme map[string]any, scopesValue any) (string, error) {
	kind, _ := scheme["type"].(string)
	if kind == "" {
		return "", fmt.Errorf("security scheme %q is missing type", name)
	}
	fields := []string{"name: " + quoteTS(name), "type: " + quoteTS(kind)}
	switch kind {
	case "apiKey":
		location, _ := scheme["in"].(string)
		parameterName, _ := scheme["name"].(string)
		if location != "header" && location != "query" && location != "cookie" {
			return "", fmt.Errorf("apiKey security scheme %q has unsupported location %q", name, location)
		}
		if parameterName == "" {
			return "", fmt.Errorf("apiKey security scheme %q is missing name", name)
		}
		fields = append(fields, "location: "+quoteTS(location), "parameterName: "+quoteTS(parameterName))
	case "http":
		protocol, _ := scheme["scheme"].(string)
		if protocol == "" {
			return "", fmt.Errorf("http security scheme %q is missing scheme", name)
		}
		fields = append(fields, "scheme: "+quoteTS(strings.ToLower(protocol)))
	case "oauth2", "openIdConnect", "mutualTLS":
		// Flow/discovery metadata is preserved in metadata.js. Runtime credential
		// application only needs the standard scheme kind and requested scopes.
	default:
		return "", fmt.Errorf("security scheme %q has unsupported type %q", name, kind)
	}
	if bearerFormat, _ := scheme["bearerFormat"].(string); bearerFormat != "" {
		fields = append(fields, "bearerFormat: "+quoteTS(bearerFormat))
	}
	if flows, exists := scheme["flows"]; exists {
		encoded, err := runtimeJSONExpression(flows)
		if err != nil {
			return "", fmt.Errorf("security scheme %q flows: %w", name, err)
		}
		fields = append(fields, "flows: "+encoded)
	}
	if url, _ := scheme["openIdConnectUrl"].(string); url != "" {
		fields = append(fields, "openIdConnectUrl: "+quoteTS(url))
	}
	if url, _ := scheme["oauth2MetadataUrl"].(string); url != "" {
		fields = append(fields, "oauth2MetadataUrl: "+quoteTS(url))
	}
	if deprecated, _ := scheme["deprecated"].(bool); deprecated {
		fields = append(fields, "deprecated: true")
	}
	scopes, _ := scopesValue.([]any)
	if len(scopes) > 0 {
		values := make([]string, 0, len(scopes))
		for _, value := range scopes {
			scope, ok := value.(string)
			if !ok {
				return "", fmt.Errorf("security scheme %q has a non-string scope", name)
			}
			values = append(values, quoteTS(scope))
		}
		sort.Strings(values)
		fields = append(fields, "scopes: ["+strings.Join(values, ", ")+"]")
	}
	return "{ " + strings.Join(fields, ", ") + " }", nil
}
