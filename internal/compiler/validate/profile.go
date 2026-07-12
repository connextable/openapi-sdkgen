package validate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func Project(document *ir.Document) error {
	if document == nil {
		return fmt.Errorf("IR document is nil")
	}
	if document.OpenAPIVersion != "3.2.0" {
		return fmt.Errorf("project contract must use OpenAPI 3.2.0")
	}
	if document.Title == "" || document.ContractVersion == "" {
		return fmt.Errorf("project contract info.title and info.version are required")
	}
	if len(document.Servers) == 0 || document.Servers[0].URL != "/v1" {
		return fmt.Errorf("project contract first server must be /v1")
	}

	operationIDs := make(map[string]string)
	var validationErrors []string
	for _, operation := range document.Operations {
		if operation.OperationID == "" {
			validationErrors = append(validationErrors, fmt.Sprintf("%s %s: missing operationId", operation.Method, operation.Path))
		} else if previous, exists := operationIDs[operation.OperationID]; exists {
			validationErrors = append(validationErrors, fmt.Sprintf("%s %s: duplicate operationId %q also used by %s", operation.Method, operation.Path, operation.OperationID, previous))
		} else {
			operationIDs[operation.OperationID] = operation.Method + " " + operation.Path
		}
		for _, parameter := range operation.PathParameterOrder {
			if !declaresPathParameter(document.Raw, operation.Path, operation.PathItemRaw, operation.Raw, parameter) {
				validationErrors = append(validationErrors, fmt.Sprintf("%s %s: undeclared path parameter %q", operation.Method, operation.Path, parameter))
			}
		}
		if operation.Envelope != "data" && operation.Envelope != "none" {
			validationErrors = append(validationErrors, fmt.Sprintf("%s %s: invalid or missing x-envelope", operation.Method, operation.Path))
		}
		if operation.Concurrency != "required" && operation.Concurrency != "optional" && operation.Concurrency != "none" {
			validationErrors = append(validationErrors, fmt.Sprintf("%s %s: invalid or missing x-concurrency", operation.Method, operation.Path))
		}
		if operation.Idempotency != "required" && operation.Idempotency != "optional" && operation.Idempotency != "unsupported" {
			validationErrors = append(validationErrors, fmt.Sprintf("%s %s: invalid or missing x-idempotency", operation.Method, operation.Path))
		}
		if operation.Visibility != "public" && operation.Visibility != "internal" && operation.Visibility != "hidden" {
			validationErrors = append(validationErrors, fmt.Sprintf("%s %s: invalid or missing x-sdk-visibility", operation.Method, operation.Path))
		}
		if _, ok := operation.Raw["security"].([]any); !ok {
			validationErrors = append(validationErrors, fmt.Sprintf("%s %s: operation must declare security explicitly", operation.Method, operation.Path))
		}
		if operation.Pagination != "" && operation.Pagination != "cursor" && operation.Pagination != "offset" && operation.Pagination != "both" {
			validationErrors = append(validationErrors, fmt.Sprintf("%s %s: invalid x-pagination %q", operation.Method, operation.Path, operation.Pagination))
		}
		validationErrors = append(validationErrors, validateFilterOperators(document.Raw, operation)...)
	}
	if len(validationErrors) > 0 {
		sort.Strings(validationErrors)
		return fmt.Errorf("project contract validation failed:\n- %s", strings.Join(validationErrors, "\n- "))
	}
	return nil
}

func validateFilterOperators(document map[string]any, operation ir.Operation) []string {
	allowed := map[string]bool{"eq": true, "in": true, "gt": true, "gte": true, "lt": true, "lte": true}
	var result []string
	for _, source := range []any{operation.PathItemRaw["parameters"], operation.Raw["parameters"]} {
		parameters, _ := source.([]any)
		for _, value := range parameters {
			parameter, _ := value.(map[string]any)
			if reference, _ := parameter["$ref"].(string); reference != "" {
				parameter = resolveLocalParameter(document, reference)
			}
			metadata, exists := parameter["x-filter"]
			if !exists {
				continue
			}
			operator, _ := metadata.(string)
			if object, ok := metadata.(map[string]any); ok {
				operator, _ = object["operator"].(string)
			}
			name, _ := parameter["name"].(string)
			if !allowed[operator] {
				result = append(result, fmt.Sprintf("%s %s: query parameter %q has invalid x-filter operator %q", operation.Method, operation.Path, name, operator))
			}
		}
	}
	return result
}

func declaresPathParameter(document map[string]any, path string, pathItem, operation map[string]any, name string) bool {
	if len(pathItem) == 0 {
		paths, _ := document["paths"].(map[string]any)
		pathItem, _ = paths[path].(map[string]any)
	}
	return parameterListDeclares(document, pathItem["parameters"], name) || parameterListDeclares(document, operation["parameters"], name)
}

func parameterListDeclares(document map[string]any, value any, name string) bool {
	parameters, _ := value.([]any)
	for _, value := range parameters {
		parameter, _ := value.(map[string]any)
		if reference, _ := parameter["$ref"].(string); reference != "" {
			parameter = resolveLocalParameter(document, reference)
		}
		if parameter["in"] == "path" && parameter["name"] == name {
			return true
		}
	}
	return false
}

func resolveLocalParameter(document map[string]any, reference string) map[string]any {
	const prefix = "#/components/parameters/"
	if !strings.HasPrefix(reference, prefix) {
		return nil
	}
	components, _ := document["components"].(map[string]any)
	parameters, _ := components["parameters"].(map[string]any)
	parameter, _ := parameters[strings.TrimPrefix(reference, prefix)].(map[string]any)
	return parameter
}
