package typescript

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/compiler/naming"
)

type operationParameter struct {
	Name                  string
	Property              string
	Binding               string
	Description           string
	Location              string
	Style                 string
	Explode               bool
	Required              bool
	Deprecated            bool
	AllowReserved         bool
	EnvironmentControlled bool
	ContentType           string
	Schema                any
	Raw                   map[string]any
	Pointer               string
	Sort                  *ir.SortParameterPlan
}

type requestHeaderPolicy uint8

const (
	requestHeaderCallerManaged requestHeaderPolicy = iota
	requestHeaderEnvironmentControlled
)

var fetchEnvironmentControlledRequestHeaders = map[string]struct{}{
	"accept-charset":                 {},
	"accept-encoding":                {},
	"access-control-request-headers": {},
	"access-control-request-method":  {},
	"connection":                     {},
	"content-length":                 {},
	"cookie":                         {},
	"cookie2":                        {},
	"date":                           {},
	"dnt":                            {},
	"expect":                         {},
	"host":                           {},
	"keep-alive":                     {},
	"origin":                         {},
	"referer":                        {},
	"set-cookie":                     {},
	"te":                             {},
	"trailer":                        {},
	"transfer-encoding":              {},
	"upgrade":                        {},
	"via":                            {},
}

func classifyFetchRequestHeader(name string) requestHeaderPolicy {
	name = strings.ToLower(name)
	if _, exists := fetchEnvironmentControlledRequestHeaders[name]; exists ||
		strings.HasPrefix(name, "proxy-") ||
		strings.HasPrefix(name, "sec-") {
		return requestHeaderEnvironmentControlled
	}
	return requestHeaderCallerManaged
}

func operationParameters(document *ir.Document, operation ir.Operation) ([]operationParameter, error) {
	merged := make(map[string]operationParameter)
	order := make([]string, 0)
	sources := []struct {
		value   any
		pointer string
	}{
		{value: operation.PathItemRaw["parameters"], pointer: operationPathItemPointer(operation) + "/parameters"},
		{value: operation.Raw["parameters"], pointer: operation.Pointer + "/parameters"},
	}
	for _, source := range sources {
		values, _ := source.value.([]any)
		for index, value := range values {
			raw, _ := value.(map[string]any)
			pointer := source.pointer + "/" + strconv.Itoa(index)
			if reference, _ := raw["$ref"].(string); reference != "" {
				if resolvedPointer := localReferencePointer(reference); resolvedPointer != "" {
					pointer = resolvedPointer
				}
			}
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
			var sortPlan *ir.SortParameterPlan
			if value, exists := operation.SortParameters[key]; exists {
				copied := value
				sortPlan = &copied
			} else if value, exists := document.ParameterSortPlans[pointer]; exists {
				copied := value
				sortPlan = &copied
			}
			headerPolicy := requestHeaderCallerManaged
			if location == "header" {
				headerPolicy = classifyFetchRequestHeader(name)
			}
			merged[key] = operationParameter{
				Name: name, Property: name, Binding: stablePrivateIdentifier("operation-parameter", operationRouteKey(operation)+"\x00"+location+"\x00"+name), Description: description, Location: location, Style: style,
				Explode: explode, Required: boolValue(raw, "required"), Deprecated: boolValue(raw, "deprecated"), AllowReserved: boolValue(raw, "allowReserved"), ContentType: contentType, Schema: schema,
				EnvironmentControlled: headerPolicy == requestHeaderEnvironmentControlled, Raw: raw, Pointer: pointer, Sort: sortPlan,
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
	usedPathBindings := make(map[string]bool, len(operation.PathParameterOrder))
	for index := range result {
		if result[index].Location == "path" {
			result[index].Binding = readablePathParameterBinding(result[index].Name, usedPathBindings)
		}
	}
	return result, nil
}

func readablePathParameterBinding(name string, used map[string]bool) string {
	base, err := naming.Property(name)
	if err != nil || !isTypeScriptBindingIdentifier(base) {
		public, publicErr := naming.Public(name)
		if publicErr == nil && public != "" {
			runes := []rune(public)
			runes[0] = unicode.ToLower(runes[0])
			base = string(runes)
		}
	}
	if !isTypeScriptBindingIdentifier(base) {
		base = "pathParameter"
	}
	binding := base
	for suffix := 2; used[binding]; suffix++ {
		binding = base + strconv.Itoa(suffix)
	}
	used[binding] = true
	return binding
}

func isTypeScriptBindingIdentifier(value string) bool {
	runes := []rune(value)
	if len(runes) == 0 || !(runes[0] == '_' || runes[0] == '$' || unicode.IsLetter(runes[0])) {
		return false
	}
	for _, value := range runes[1:] {
		if value != '_' && value != '$' && !unicode.IsLetter(value) && !unicode.IsDigit(value) {
			return false
		}
	}
	return true
}

func operationPathItemPointer(operation ir.Operation) string {
	if before, _, found := strings.Cut(operation.Pointer, "/additionalOperations/"); found {
		return before
	}
	if index := strings.LastIndex(operation.Pointer, "/"); index >= 0 {
		return operation.Pointer[:index]
	}
	return operation.Pointer
}

func localReferencePointer(reference string) string {
	if !strings.HasPrefix(reference, "#") {
		return ""
	}
	decoded, err := url.PathUnescape(strings.TrimPrefix(reference, "#"))
	if err != nil || !strings.HasPrefix(decoded, "/") {
		return ""
	}
	return "#" + decoded
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

func clientParametersIn(document *ir.Document, operation ir.Operation, location string) ([]operationParameter, error) {
	parameters, err := clientOperationParameters(document, operation)
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

func clientOperationParameters(document *ir.Document, operation ir.Operation) ([]operationParameter, error) {
	parameters, err := operationParameters(document, operation)
	if err != nil {
		return nil, err
	}
	for index := range parameters {
		parameters[index] = projectClientParameter(parameters[index])
	}
	return parameters, nil
}

func projectClientParameter(parameter operationParameter) operationParameter {
	if parameter.EnvironmentControlled {
		parameter.Required = false
	}
	return parameter
}
