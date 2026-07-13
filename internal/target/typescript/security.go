package typescript

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

// operationSecurityDefinition lowers an operation's effective OpenAPI Security
// Requirement Object. An absent operation field inherits the root field;
// explicit `security: []` disables that inheritance.
func operationSecurityDefinition(document *ir.Document, operation ir.Operation) (string, bool, error) {
	value, exists := operation.Raw["security"]
	if !exists {
		value, exists = document.Raw["security"]
	}
	if !exists {
		return "", false, nil
	}
	requirements, ok := value.([]any)
	if !ok {
		return "", false, fmt.Errorf("security must be an array")
	}
	if len(requirements) == 0 {
		return "", false, nil
	}
	components, _ := document.Raw["components"].(map[string]any)
	schemes, _ := components["securitySchemes"].(map[string]any)
	entries := make([]string, 0, len(requirements))
	ids := make(map[string]int, len(requirements))
	for index, value := range requirements {
		requirement, ok := value.(map[string]any)
		if !ok {
			return "", false, fmt.Errorf("security alternative %d must be an object", index)
		}
		names := sortedAnyKeys(requirement)
		definitions := make([]string, 0, len(names))
		for _, name := range names {
			scheme, ok := schemes[name].(map[string]any)
			if !ok {
				return "", false, fmt.Errorf("security alternative %d references unknown scheme %q", index, name)
			}
			definition, err := securitySchemeDefinition(name, scheme, requirement[name])
			if err != nil {
				return "", false, err
			}
			definitions = append(definitions, definition)
		}
		id := "optional"
		if len(names) > 0 {
			id = strings.Join(names, "__")
		}
		baseID := id
		if count := ids[baseID]; count > 0 {
			id = fmt.Sprintf("%s__%d", baseID, count+1)
		}
		ids[baseID]++
		entries = append(entries, "{ id: "+quoteTS(id)+", schemes: ["+strings.Join(definitions, ", ")+"] }")
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
		encoded, err := json.Marshal(flows)
		if err != nil {
			return "", fmt.Errorf("security scheme %q flows: %w", name, err)
		}
		fields = append(fields, "flows: "+string(encoded))
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
