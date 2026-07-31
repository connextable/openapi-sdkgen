package typescript

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"openapi-sdkgen/internal/compiler/ir"
)

type errorContract struct {
	Code        string
	Category    string
	Description string
	Details     string
	SchemaName  string
}

type aggregatedErrorContract struct {
	Code        string
	Category    string
	Description string
	Details     []string
	SchemaNames []string
}

func emitErrors(document *ir.Document) ([]byte, error) {
	contracts, _, err := errorContracts(document)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	output.WriteString("import type { APIError, TransportError, TransportErrorCode } from \"./runtime.js\"\n")
	if len(contracts) > 0 {
		output.WriteString("import { isErrorCode } from \"./runtime.js\"\n")
	}
	if errorContractsUseContractTypes(contracts) {
		output.WriteString("import type * as Contract from \"./types.js\"\n")
	}
	output.WriteString("\n")

	if len(contracts) == 0 {
		output.WriteString("/** Server-declared error codes. This contract currently declares none. */\n")
		output.WriteString("export type ServerErrorCode = never\n")
		output.WriteString("/** Maps server error codes to their typed details. This contract currently declares none. */\n")
		output.WriteString("export type ServerErrorDetailsByCode = Record<never, never>\n")
	} else {
		codes := make([]string, 0, len(contracts))
		for _, contract := range contracts {
			codes = append(codes, quoteTS(contract.Code))
		}
		output.WriteString("/** Union of every server-declared error code in this contract. */\n")
		fmt.Fprintf(&output, "export type ServerErrorCode = %s\n", strings.Join(codes, " | "))
		output.WriteString("/** Maps each server error code to its generated details type. */\n")
		output.WriteString("export interface ServerErrorDetailsByCode {\n")
		for _, contract := range contracts {
			description := contract.Description
			if description == "" {
				description = "Details carried by `" + contract.Code + "`."
			}
			fmt.Fprintf(&output, "  /** %s */\n", sanitizeComment(description))
			fmt.Fprintf(&output, "  readonly %s: %s\n", quoteTS(contract.Code), strings.Join(contract.Details, " | "))
		}
		output.WriteString("}\n")
	}
	output.WriteString("/** Server API error with code-correlated details. */\n")
	output.WriteString("export type ServerError<Code extends ServerErrorCode, Details = ServerErrorDetailsByCode[Code]> = APIError<Code, Details>\n")
	output.WriteString("/** Distributes a server error union over exact runtime codes. */\n")
	output.WriteString("export type ServerErrorByCodes<Code extends ServerErrorCode> = Code extends ServerErrorCode ? ServerError<Code> : never\n\n")

	categories := errorCategories(contracts)
	output.WriteString("/** Exact server error codes grouped by exact runtime category. */\n")
	output.WriteString("export interface ServerErrorCodesByCategory {\n")
	categoryNames := sortedAnyKeys(mapStringAny(categories))
	for _, category := range categoryNames {
		output.WriteString("  /** Codes declared in this category. */\n")
		fmt.Fprintf(&output, "  readonly %s: %s\n", quoteTS(category), strings.Join(quotedStrings(categories[category]), " | "))
	}
	output.WriteString("}\n")
	output.WriteString("/** Exact server error category names. */\n")
	output.WriteString("export type ServerErrorCategory = keyof ServerErrorCodesByCategory\n")
	output.WriteString("/** Server errors selected by an exact category with code/detail correlation. */\n")
	output.WriteString("export type ServerErrorByCategory<Category extends ServerErrorCategory> = ServerErrorByCodes<ServerErrorCodesByCategory[Category]>\n\n")
	if len(categories) > 0 {
		categoryValues := make([]runtimeProperty, 0, len(categoryNames))
		for _, category := range categoryNames {
			value, valueErr := runtimeJSONExpression(stringSliceAny(categories[category]))
			if valueErr != nil {
				return nil, fmt.Errorf("error category %q: %w", category, valueErr)
			}
			categoryValues = append(categoryValues, runtimeProperty{key: category, value: value})
		}
		fmt.Fprintf(&output, "const serverErrorCodesByCategory = %s as unknown as { readonly [Category in ServerErrorCategory]: readonly ServerErrorCodesByCategory[Category][] }\n", runtimeObjectExpression(categoryValues))
		output.WriteString("/** Checks whether an unknown value belongs to an exact server error category. */\n")
		output.WriteString("export function isErrorCategory<Category extends ServerErrorCategory>(error: unknown, category: Category): error is ServerErrorByCategory<Category> {\n")
		output.WriteString("  return serverErrorCodesByCategory[category].some((code) => isErrorCode(error, code))\n")
		output.WriteString("}\n\n")
	} else {
		output.WriteString("/** Checks whether an unknown value belongs to an exact server error category. */\n")
		output.WriteString("export function isErrorCategory<Category extends ServerErrorCategory>(_error: unknown, _category: Category): _error is ServerErrorByCategory<Category> {\n")
		output.WriteString("  return false\n")
		output.WriteString("}\n\n")
	}
	output.WriteString("/** Union of all server and SDK transport error codes. */\n")
	output.WriteString("export type ErrorCode = ServerErrorCode | TransportErrorCode\n\n")
	return append(bytes.TrimRight(output.Bytes(), "\n"), '\n'), nil
}

func errorContractsUseContractTypes(contracts []aggregatedErrorContract) bool {
	for _, contract := range contracts {
		for _, details := range contract.Details {
			if strings.Contains(details, "Contract.") {
				return true
			}
		}
	}
	return false
}

func errorContracts(document *ir.Document) ([]aggregatedErrorContract, map[string][]errorContract, error) {
	contracts, bySchema, failures := errorContractsDiagnostics(document)
	if len(failures) != 0 {
		return nil, nil, failures[0]
	}
	return contracts, bySchema, nil
}

func errorContractsDiagnostics(document *ir.Document) ([]aggregatedErrorContract, map[string][]errorContract, []error) {
	names := make([]string, 0, len(document.ComponentSchemas))
	for name := range document.ComponentSchemas {
		names = append(names, name)
	}
	sort.Strings(names)
	reachable := reachableErrorComponentSchemas(document)
	byCode := make(map[string][]errorContract)
	bySchema := make(map[string][]errorContract)
	var failures []error
	for _, schemaName := range names {
		if !reachable[schemaName] {
			continue
		}
		schema := document.ComponentSchemas[schemaName]
		codes, detailsSchema := schemaErrorCodes(document, schema)
		if len(codes) == 0 {
			continue
		}
		detailsType := "unknown"
		if len(detailsSchema) > 0 {
			value, err := schemaTypeForScope(document, detailsSchema, projectionOutput, typeRenderContract)
			if err != nil {
				failures = append(failures, fmt.Errorf("error %s details: %w", schemaName, err))
				continue
			}
			detailsType = value
		}
		category := document.ErrorCategories[schemaName]
		description, _ := schema["description"].(string)
		for _, code := range codes {
			contract := errorContract{Code: code, Category: category, Description: description, Details: detailsType, SchemaName: schemaName}
			byCode[code] = append(byCode[code], contract)
			bySchema[schemaName] = append(bySchema[schemaName], contract)
		}
	}
	for range names {
		changed := false
		for _, schemaName := range names {
			for _, reference := range schemaReferences(document.ComponentSchemas[schemaName]) {
				for _, contract := range bySchema[reference] {
					if !containsErrorContract(bySchema[schemaName], contract) {
						bySchema[schemaName] = append(bySchema[schemaName], contract)
						changed = true
					}
				}
			}
		}
		if !changed {
			break
		}
	}
	result := make([]aggregatedErrorContract, 0, len(byCode))
	for _, code := range sortedAnyKeys(mapStringAny(byCode)) {
		contributions := byCode[code]
		details := make(map[string]bool)
		categories := make(map[string][]string)
		schemaNames := make(map[string]bool)
		description := ""
		for _, contribution := range contributions {
			details[contribution.Details] = true
			schemaNames[contribution.SchemaName] = true
			if contribution.Category != "" {
				categories[contribution.Category] = append(categories[contribution.Category], contribution.SchemaName)
			}
			if description == "" && contribution.Description != "" {
				description = contribution.Description
			}
		}
		if len(categories) > 1 {
			categoryNames := sortedAnyKeys(mapStringAny(categories))
			parts := make([]string, 0, len(categoryNames))
			for _, category := range categoryNames {
				sort.Strings(categories[category])
				parts = append(parts, fmt.Sprintf("%q from schemas %s", category, strings.Join(uniqueStrings(categories[category]), ", ")))
			}
			failures = append(failures, fmt.Errorf("error code %q has conflicting non-empty categories: %s", code, strings.Join(parts, "; ")))
			continue
		}
		category := ""
		for value := range categories {
			category = value
		}
		result = append(result, aggregatedErrorContract{
			Code:        code,
			Category:    category,
			Description: description,
			Details:     sortedStringKeys(details),
			SchemaNames: sortedStringKeys(schemaNames),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Code < result[j].Code })
	return result, bySchema, failures
}

func reachableErrorComponentSchemas(document *ir.Document) map[string]bool {
	result := make(map[string]bool)
	var visitSchema func(map[string]any)
	visitSchema = func(schema map[string]any) {
		for _, name := range schemaReferences(schema) {
			if result[name] {
				continue
			}
			result[name] = true
			visitSchema(document.ComponentSchemas[name])
		}
	}
	for _, operation := range document.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		responses, _ := operation.Raw["responses"].(map[string]any)
		for status, value := range responses {
			if strings.HasPrefix(status, "2") {
				continue
			}
			response, _ := value.(map[string]any)
			resolved, err := resolveComponentObject(document, response, "responses")
			if err != nil {
				continue
			}
			content, _ := resolved["content"].(map[string]any)
			for _, mediaValue := range content {
				media, _ := mediaValue.(map[string]any)
				media, err = resolveMediaTypeObject(document, media)
				if err != nil {
					continue
				}
				schema, _ := media["schema"].(map[string]any)
				visitSchema(schema)
			}
		}
	}
	return result
}

func containsErrorContract(contracts []errorContract, candidate errorContract) bool {
	for _, contract := range contracts {
		if contract.Code == candidate.Code && contract.Details == candidate.Details && contract.Category == candidate.Category {
			return true
		}
	}
	return false
}

func errorCategories(contracts []aggregatedErrorContract) map[string][]string {
	result := make(map[string][]string)
	for _, contract := range contracts {
		if contract.Category != "" {
			result[contract.Category] = append(result[contract.Category], contract.Code)
		}
	}
	for category := range result {
		sort.Strings(result[category])
		result[category] = uniqueStrings(result[category])
	}
	return result
}

func quotedStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, quoteTS(value))
	}
	return result
}

func stringSliceAny(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func sortedStringKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func schemaErrorCodes(document *ir.Document, schema map[string]any) ([]string, map[string]any) {
	codes, errorSchema, recognized := recognizedErrorEnvelope(document, schema)
	if !recognized {
		return nil, nil
	}
	errorProperties, _ := errorSchema["properties"].(map[string]any)
	detailsSchema, _ := errorProperties["details"].(map[string]any)
	return codes, detailsSchema
}

func isErrorSchema(document *ir.Document, schema map[string]any) bool {
	codes, _ := schemaErrorCodes(document, schema)
	return len(codes) > 0
}

func operationErrorTypes(document *ir.Document, operation ir.Operation, bySchema map[string][]errorContract) ([]string, error) {
	responses, _ := operation.Raw["responses"].(map[string]any)
	byCode := make(map[string]map[string]bool)
	for status, value := range responses {
		if strings.HasPrefix(status, "2") {
			continue
		}
		response, _ := value.(map[string]any)
		var err error
		response, err = resolveComponentObject(document, response, "responses")
		if err != nil {
			return nil, err
		}
		for _, schemaName := range responseSchemaReferences(response) {
			for _, contract := range bySchema[schemaName] {
				if byCode[contract.Code] == nil {
					byCode[contract.Code] = make(map[string]bool)
				}
				byCode[contract.Code][contract.Details] = true
			}
		}
	}
	codes := sortedAnyKeys(mapStringAny(byCode))
	result := make([]string, 0, len(codes))
	for _, code := range codes {
		result = append(result, "ServerError<"+quoteTS(code)+", "+strings.Join(sortedStringKeys(byCode[code]), " | ")+">")
	}
	return result, nil
}

func operationErrorTypeExpression(document *ir.Document, operation ir.Operation, bySchema map[string][]errorContract) (typeExpression, error) {
	types, err := operationErrorTypes(document, operation, bySchema)
	if err != nil {
		return typeExpression{}, err
	}
	local := make([]string, 0, len(types)+1)
	contract := make([]string, 0, len(types)+1)
	for _, value := range types {
		local = append(local, value)
		contract = append(contract, "Errors."+value)
	}
	local = append(local, "TransportError")
	contract = append(contract, "TransportError")
	return scopedTypeExpression(strings.Join(local, " | "), strings.Join(contract, " | ")), nil
}

func responseSchemaReferences(response map[string]any) []string {
	content, _ := response["content"].(map[string]any)
	var result []string
	for _, value := range content {
		media, _ := value.(map[string]any)
		schema, _ := media["schema"].(map[string]any)
		result = append(result, schemaReferences(schema)...)
	}
	return result
}

func schemaReferences(schema map[string]any) []string {
	var result []string
	if reference, _ := schema["$ref"].(string); reference != "" {
		name, err := componentSchemaReferenceName(reference)
		if err != nil {
			return nil
		}
		return []string{name}
	}
	for _, keyword := range []string{"oneOf", "anyOf", "allOf"} {
		variants, _ := schema[keyword].([]any)
		for _, value := range variants {
			variant, _ := value.(map[string]any)
			result = append(result, schemaReferences(variant)...)
		}
	}
	return result
}
