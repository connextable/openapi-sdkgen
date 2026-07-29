package typescript

import (
	"sort"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

type operationParameter struct {
	Name          string
	Property      string
	Binding       string
	Description   string
	Location      string
	Style         string
	Explode       bool
	Required      bool
	Deprecated    bool
	AllowReserved bool
	ContentType   string
	Schema        any
}

func operationParameters(document *ir.Document, operation ir.Operation) ([]operationParameter, error) {
	merged := make(map[string]operationParameter)
	order := make([]string, 0)
	for _, source := range []any{operation.PathItemRaw["parameters"], operation.Raw["parameters"]} {
		values, _ := source.([]any)
		for _, value := range values {
			raw, _ := value.(map[string]any)
			var err error
			raw, err = resolveComponentObject(document, raw, "parameters")
			if err != nil {
				return nil, err
			}
			name, _ := raw["name"].(string)
			location, _ := raw["in"].(string)
			if name == "" || location == "" {
				continue
			}
			style, _ := raw["style"].(string)
			if style == "" {
				style = defaultParameterStyle(location)
			}
			explode, hasExplode := raw["explode"].(bool)
			if !hasExplode {
				explode = style == "form"
			}
			schema := raw["schema"]
			contentType := ""
			if content, ok := raw["content"].(map[string]any); ok {
				mediaTypes := make([]string, 0, len(content))
				for mediaType := range content {
					mediaTypes = append(mediaTypes, mediaType)
				}
				sort.Strings(mediaTypes)
				if len(mediaTypes) > 0 {
					contentType = mediaTypes[0]
					media, _ := content[contentType].(map[string]any)
					media, err = resolveMediaTypeObject(document, media)
					if err != nil {
						return nil, err
					}
					schema = media["schema"]
				}
			}
			key := location + "\x00" + name
			if _, exists := merged[key]; !exists {
				order = append(order, key)
			}
			description, _ := raw["description"].(string)
			merged[key] = operationParameter{
				Name: name, Property: name, Binding: stablePrivateIdentifier("operation-parameter", operation.OperationID+"\x00"+location+"\x00"+name), Description: description, Location: location, Style: style,
				Explode: explode, Required: boolValue(raw, "required"), Deprecated: boolValue(raw, "deprecated"), AllowReserved: boolValue(raw, "allowReserved"), ContentType: contentType, Schema: schema,
			}
		}
	}
	result := make([]operationParameter, 0, len(merged))
	for _, key := range order {
		result = append(result, merged[key])
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Location != "path" || result[j].Location != "path" {
			return false
		}
		return pathParameterIndex(operation.PathParameterOrder, result[i].Name) < pathParameterIndex(operation.PathParameterOrder, result[j].Name)
	})
	return result, nil
}

func defaultParameterStyle(location string) string {
	if location == "query" || location == "cookie" {
		return "form"
	}
	return "simple"
}

func pathParameterIndex(order []string, name string) int {
	for index, value := range order {
		if value == name {
			return index
		}
	}
	return len(order)
}

func parametersIn(document *ir.Document, operation ir.Operation, location string) ([]operationParameter, error) {
	parameters, err := operationParameters(document, operation)
	if err != nil {
		return nil, err
	}
	result := make([]operationParameter, 0, len(parameters))
	for _, parameter := range parameters {
		if parameter.Location == location {
			result = append(result, parameter)
		}
	}
	return result, nil
}
