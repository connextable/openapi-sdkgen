package typescript

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/compiler/naming"
)

func emitClient(document *ir.Document, manifest Manifest) ([]byte, error) {
	tree, err := buildResourceTree(document, manifest)
	if err != nil {
		return nil, err
	}
	hasPathOperations := resourceTreeHasPathOperations(tree)
	hasPagination := resourceTreeHasPagination(tree)
	var output bytes.Buffer
	output.WriteString("import {\n")
	output.WriteString("  bindOperation,\n")
	if hasPathOperations {
		output.WriteString("  bindPathOperation,\n")
	}
	if hasPagination {
		output.WriteString("  createPaginator,\n")
	}
	output.WriteString("  createRequest,\n")
	output.WriteString("  type ClientOptions,\n")
	output.WriteString("  type BinaryBody,\n")
	if hasPagination {
		output.WriteString("  type PaginateInput,\n")
	}
	output.WriteString("  type RequestOptions,\n")
	output.WriteString("  type RawResponseFor,\n")
	output.WriteString("  type WireSchemas,\n")
	output.WriteString("} from \"./runtime.js\"\n")
	output.WriteString("import type * as Contract from \"./types.js\"\n\n")
	output.WriteString("import type * as Errors from \"./errors.js\"\n\n")
	if hasVisibleInputSchemas(document) {
		if err := emitWireComponents(&output, document, "inputSchemas", projectionInput); err != nil {
			return nil, err
		}
	}
	if hasVisibleResponseBodies(document) {
		if err := emitWireComponents(&output, document, "outputSchemas", projectionOutput); err != nil {
			return nil, err
		}
	}
	output.WriteString("export {\n")
	output.WriteString("  APIError,\n")
	output.WriteString("  TransportErrorCode,\n")
	output.WriteString("  getErrorCode,\n")
	output.WriteString("  getRequestID,\n")
	output.WriteString("  isAPIError,\n")
	output.WriteString("  isErrorCode,\n")
	output.WriteString("} from \"./runtime.js\"\n")
	output.WriteString("export type { ClientOptions, OperationCall, PaginateInput, PaginationProfile, RawResponse, RawResponseFor, RequestMetadata, RequestOptions, TransportError } from \"./runtime.js\"\n\n")

	operationsByID := make(map[string]ir.Operation, len(document.Operations))
	for _, operation := range document.Operations {
		operationsByID[operation.OperationID] = operation
	}

	for _, item := range manifest.Operations {
		if item.Visibility == "hidden" {
			continue
		}
		operation := operationsByID[item.OperationID]
		if err := emitOperationTypes(&output, document, operation, item); err != nil {
			return nil, err
		}
	}

	output.WriteString("/** Type catalog keyed by OpenAPI operation ID. */\n")
	output.WriteString("export interface Operations {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		property, err := naming.Property(operation.OperationID)
		if err != nil {
			return nil, err
		}
		operationName := operationTypeName(operation.OperationID)
		emitOperationCatalogJSDoc(&output, "  ", operation)
		fmt.Fprintf(&output, "  readonly %s: {\n", property)
		fmt.Fprintf(&output, "    /** Complete generated input type. */\n")
		fmt.Fprintf(&output, "    readonly input: %s\n", operationInputAlias(operation))
		fmt.Fprintf(&output, "    /** Decoded successful output type. */\n")
		fmt.Fprintf(&output, "    readonly output: %s\n", qualifyClientType(document, operation.OutputType))
		fmt.Fprintf(&output, "    /** Generated server and transport error union. */\n")
		fmt.Fprintf(&output, "    readonly error: Errors.%sError\n", operationName)
		output.WriteString("  }\n")
	}
	output.WriteString("}\n\n")

	output.WriteString("/** Generated API client with operation-ID and resource-oriented call surfaces. */\n")
	output.WriteString("export interface Client {\n")
	output.WriteString("  /** Every public operation keyed by its exact OpenAPI operation ID. */\n")
	output.WriteString("  readonly $operations: {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		property, err := naming.Property(operation.OperationID)
		if err != nil {
			return nil, err
		}
		emitOperationJSDoc(&output, "    ", operation)
		fmt.Fprintf(&output, "    readonly %s: %s\n", property, operationFunctionType(document, operation))
	}
	output.WriteString("  }\n")
	if err := emitResourceTreeInterface(&output, document, tree); err != nil {
		return nil, err
	}
	output.WriteString("}\n\n")

	output.WriteString("/**\n")
	output.WriteString(" * Creates a generated API client.\n")
	output.WriteString(" *\n")
	output.WriteString(" * The base URL must include the selected API version prefix, such as `/v1`.\n")
	output.WriteString(" *\n")
	output.WriteString(" * @param options Deployment URL, fetch implementation, and transport defaults.\n")
	output.WriteString(" * @returns A typed {@link Client}.\n")
	output.WriteString(" *\n")
	output.WriteString(" * @example\n")
	output.WriteString(" * ```ts\n")
	output.WriteString(" * const api = createClient({ baseURL: \"https://api.example.com/v1\" })\n")
	output.WriteString(" * ```\n")
	output.WriteString(" */\n")
	output.WriteString("export function createClient(options: ClientOptions): Client {\n")
	output.WriteString("  const request = createRequest(options)\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		property, err := naming.Property(operation.OperationID)
		if err != nil {
			return nil, err
		}
		outputType := qualifyClientType(document, operation.OutputType)
		definition, err := operationDefinition(document, operationsByID[operation.OperationID], operation)
		if err != nil {
			return nil, err
		}
		inputType := "never"
		hasInput := false
		if len(operation.InputTypes) > 0 {
			inputType = operationTypeName(operation.OperationID) + "Input"
			hasInput = true
		}
		fmt.Fprintf(&output, "  const %s = bindOperation<%s, %s, %sOptions, %sRawResponse>(request, %s, %t) as %sCall\n", property, inputType, outputType, operationTypeName(operation.OperationID), operationTypeName(operation.OperationID), definition, hasInput, operationTypeName(operation.OperationID))
		if operation.Pagination != "" && len(operation.PathParameterOrder) == 0 {
			itemType, err := operationItemType(document, operationsByID[operation.OperationID])
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(&output, "  const paginate%s = createPaginator<%s, %sInput, %s, %s, %sOptions>(%s, %s)\n", operationTypeName(operation.OperationID), qualifyClientType(document, itemType), operationTypeName(operation.OperationID), outputType, quoteTS(operation.Pagination), operationTypeName(operation.OperationID), property, quoteTS(operation.Pagination))
		}
	}
	output.WriteString("\n")
	if err := emitResourceTreeValues(&output, document, tree); err != nil {
		return nil, err
	}
	output.WriteString("\n  return {\n")
	output.WriteString("    $operations: {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		property, _ := naming.Property(operation.OperationID)
		fmt.Fprintf(&output, "      %s,\n", property)
	}
	output.WriteString("    },\n")
	for _, name := range sortedResourceChildNames(tree) {
		fmt.Fprintf(&output, "    %s,\n", name)
	}
	output.WriteString("  }\n")
	output.WriteString("}\n")
	return output.Bytes(), nil
}

func emitOperationTypes(output *bytes.Buffer, document *ir.Document, operation ir.Operation, item ManifestOperation) error {
	operationName := operationTypeName(operation.OperationID)
	if err := emitOperationOptions(output, document, operationName, operation); err != nil {
		return err
	}
	if parameters, err := parametersIn(document, operation, "path"); err != nil {
		return err
	} else if len(parameters) > 0 {
		if err := emitParameterType(output, document, operation, operationName+"PathInput", "path"); err != nil {
			return err
		}
	}
	if parameters, err := parametersIn(document, operation, "query"); err != nil {
		return err
	} else if len(parameters) > 0 || operation.Pagination != "" {
		if err := emitQueryTypes(output, document, operation, operationName, parameters); err != nil {
			return err
		}
	}
	if parameters, err := parametersIn(document, operation, "header"); err != nil {
		return err
	} else if len(parameters) > 0 {
		if err := emitParameterType(output, document, operation, operationName+"HeaderInput", "header"); err != nil {
			return err
		}
	}
	if parameters, err := parametersIn(document, operation, "cookie"); err != nil {
		return err
	} else if len(parameters) > 0 {
		if err := emitParameterType(output, document, operation, operationName+"CookieInput", "cookie"); err != nil {
			return err
		}
	}
	if body, ok := operation.Raw["requestBody"].(map[string]any); ok {
		resolvedBody, err := resolveComponentObject(document, body, "requestBodies")
		if err != nil {
			return err
		}
		bodyType, err := requestBodyType(document, resolvedBody)
		if err != nil {
			return err
		}
		qualifiedBodyType := qualifyClientType(document, bodyType)
		bodyDescription, _ := resolvedBody["description"].(string)
		if bodyDescription == "" {
			bodyDescription = "Request body for `" + operation.OperationID + "` (`" + operation.Method + " " + operation.Path + "`)."
		}
		fmt.Fprintf(output, "/**\n * %s\n *\n * Type: %s\n */\n", sanitizeComment(bodyDescription), jsDocTypeReference(qualifiedBodyType))
		fmt.Fprintf(output, "export type %sBodyInput = %s\n\n", operationName, qualifiedBodyType)
	}
	if len(item.InputTypes) > 0 {
		fmt.Fprintf(output, "/** Complete input for `%s` (`%s %s`). */\n", operation.OperationID, operation.Method, operation.Path)
		fmt.Fprintf(output, "export interface %sInput {\n", operationName)
		for _, inputType := range item.InputTypes {
			field := strings.TrimPrefix(inputType, operationName)
			field = strings.TrimSuffix(field, "Input")
			property, err := aggregateInputProperty(field)
			if err != nil {
				return err
			}
			optional := ""
			valueType := inputType
			if field == "Body" {
				body, _ := operation.Raw["requestBody"].(map[string]any)
				resolvedBody, err := resolveComponentObject(document, body, "requestBodies")
				if err != nil {
					return err
				}
				if !boolValue(resolvedBody, "required") {
					optional = "?"
					valueType += " | undefined"
				}
			}
			fmt.Fprintf(output, "  /** Generated %s input. See %s. */\n", strings.ToLower(field), jsDocTypeReference(inputType))
			fmt.Fprintf(output, "  readonly %s%s: %s\n", property, optional, valueType)
		}
		output.WriteString("}\n\n")
	}
	if len(item.PathParameterOrder) > 0 {
		resourceInput := "never"
		if len(item.InputTypes) > 1 {
			resourceInput = "Omit<" + operationName + "Input, \"path\">"
		}
		fmt.Fprintf(output, "/** Input remaining after the resource path is bound for `%s`. */\n", operation.OperationID)
		fmt.Fprintf(output, "export type %sResourceInput = %s\n\n", operationName, resourceInput)
	}

	outputType := qualifyClientType(document, item.OutputType)
	rawResponseType, err := operationRawResponseType(document, operation)
	if err != nil {
		return err
	}
	qualifiedRawResponseType := qualifyClientType(document, rawResponseType)
	if err := emitRawResponseJSDoc(output, document, operation); err != nil {
		return err
	}
	fmt.Fprintf(output, "export type %sRawResponse = %s\n\n", operationName, qualifiedRawResponseType)
	emitOutputJSDoc(output, operation, item, outputType)
	fmt.Fprintf(output, "export type %sOutput = %s\n", operationName, outputType)
	output.WriteString("\n")
	if err := emitOperationCallTypes(output, document, operation, item); err != nil {
		return err
	}
	return nil
}

func emitOperationCallTypes(output *bytes.Buffer, document *ir.Document, operation ir.Operation, item ManifestOperation) error {
	operationName := operationTypeName(operation.OperationID)
	inputType := "never"
	if len(item.InputTypes) > 0 {
		inputType = operationName + "Input"
	}
	emitOperationJSDoc(output, "", item)
	if err := emitOperationCallInterface(output, document, operation, operationName+"Call", inputType, operationName+"Output", operationName+"RawResponse"); err != nil {
		return err
	}
	if len(item.PathParameterOrder) > 0 {
		emitOperationJSDoc(output, "", item)
		resourceInput := operationName + "ResourceInput"
		if len(item.InputTypes) <= 1 {
			resourceInput = "never"
		}
		if err := emitOperationCallInterface(output, document, operation, operationName+"ResourceCall", resourceInput, operationName+"Output", operationName+"RawResponse"); err != nil {
			return err
		}
	}
	return nil
}

func emitOperationCallInterface(output *bytes.Buffer, document *ir.Document, operation ir.Operation, callName, inputType, outputType, rawType string) error {
	operationName := operationTypeName(operation.OperationID)
	requiresOptions := operation.Idempotency == "required" || operation.Concurrency == "required"
	mediaOutputs, err := operationMediaOutputTypes(document, operation)
	if err != nil {
		return err
	}
	mediaTypes := make([]string, 0, len(mediaOutputs))
	for mediaType := range mediaOutputs {
		mediaTypes = append(mediaTypes, mediaType)
	}
	sort.Strings(mediaTypes)
	fmt.Fprintf(output, "export interface %s {\n", callName)
	if len(mediaTypes) > 1 {
		for _, mediaType := range mediaTypes {
			optionsType := "Omit<" + operationName + "Options, \"accept\"> & { readonly accept: " + quoteTS(mediaType) + " }"
			emitCallSignature(output, inputType, optionsType, qualifyClientType(document, mediaOutputs[mediaType]), false)
			emitRawCallSignature(output, inputType, optionsType, "Extract<"+rawType+", { readonly contentType: "+quoteTS(mediaType)+" }>", false)
		}
	}
	emitCallSignature(output, inputType, operationName+"Options", outputType, !requiresOptions)
	optionsOptional := ""
	if !requiresOptions {
		optionsOptional = "?"
	}
	emitRawCallSignature(output, inputType, operationName+"Options", rawType, optionsOptional == "?")
	output.WriteString("}\n\n")
	return nil
}

func emitRawCallSignature(output *bytes.Buffer, inputType, optionsType, resultType string, optionsOptional bool) {
	optional := ""
	if optionsOptional {
		optional = "?"
	}
	output.WriteString("  /**\n")
	output.WriteString("   * Sends the request and returns the decoded body with HTTP response metadata.\n")
	output.WriteString("   *\n")
	if inputType != "never" {
		output.WriteString("   * @param input Generated operation input.\n")
	}
	output.WriteString("   * @param options Per-request transport options.\n")
	fmt.Fprintf(output, "   * @returns Decoded response and HTTP metadata as %s.\n", jsDocTypeReference(resultType))
	output.WriteString("   */\n")
	if inputType == "never" {
		fmt.Fprintf(output, "  raw(options%s: %s): Promise<%s>\n", optional, optionsType, resultType)
		return
	}
	fmt.Fprintf(output, "  raw(input: %s, options%s: %s): Promise<%s>\n", inputType, optional, optionsType, resultType)
}

func emitCallSignature(output *bytes.Buffer, inputType, optionsType, resultType string, optionsOptional bool) {
	optional := ""
	if optionsOptional {
		optional = "?"
	}
	output.WriteString("  /**\n")
	output.WriteString("   * Sends the request and returns the decoded response body.\n")
	output.WriteString("   *\n")
	if inputType != "never" {
		output.WriteString("   * @param input Generated operation input.\n")
	}
	output.WriteString("   * @param options Per-request transport options.\n")
	fmt.Fprintf(output, "   * @returns Decoded response body as %s.\n", jsDocTypeReference(resultType))
	output.WriteString("   */\n")
	if inputType == "never" {
		fmt.Fprintf(output, "  (options%s: %s): Promise<%s>\n", optional, optionsType, resultType)
		return
	}
	fmt.Fprintf(output, "  (input: %s, options%s: %s): Promise<%s>\n", inputType, optional, optionsType, resultType)
}

func aggregateInputProperty(field string) (string, error) {
	switch field {
	case "Header":
		return "headerParams", nil
	case "Cookie":
		return "cookieParams", nil
	default:
		return naming.Property(field)
	}
}

func emitParameterType(output *bytes.Buffer, document *ir.Document, operation ir.Operation, typeName, location string) error {
	parameters, err := parametersIn(document, operation, location)
	if err != nil {
		return err
	}
	locationLabel := strings.ToUpper(location[:1]) + location[1:]
	fmt.Fprintf(output, "/** %s parameters for `%s` (`%s %s`). */\n", locationLabel, operation.OperationID, operation.Method, operation.Path)
	fmt.Fprintf(output, "export interface %s {\n", typeName)
	for _, parameter := range parameters {
		valueType, err := schemaType(document, parameter.Schema, projectionInput)
		if err != nil {
			return err
		}
		valueType = qualifyClientType(document, valueType)
		optional := "?"
		if parameter.Required {
			optional = ""
		} else {
			valueType += " | undefined"
		}
		emitOperationParameterJSDoc(output, "  ", parameter, locationLabel)
		fmt.Fprintf(output, "  readonly %s%s: %s\n", parameter.Property, optional, valueType)
	}
	output.WriteString("}\n\n")
	return nil
}

func emitOperationParameterJSDoc(output *bytes.Buffer, indent string, parameter operationParameter, locationLabel string) {
	documentation := make(map[string]any, len(parameter.Schema)+2)
	for key, value := range parameter.Schema {
		documentation[key] = value
	}
	if parameter.Description != "" {
		documentation["description"] = parameter.Description
	}
	if parameter.Deprecated {
		documentation["deprecated"] = true
	}
	emitSchemaValueJSDoc(output, indent, documentation, locationLabel+" parameter `"+sanitizeComment(parameter.Name)+"`.")
}

func emitOperationOptions(output *bytes.Buffer, document *ir.Document, operationName string, operation ir.Operation) error {
	parts := []string{`Omit<RequestOptions, "accept" | "idempotencyKey" | "ifMatch">`}
	mediaTypes, err := operationResponseMediaTypes(document, operation)
	if err != nil {
		return err
	}
	if len(mediaTypes) > 1 {
		quoted := make([]string, 0, len(mediaTypes))
		for _, mediaType := range mediaTypes {
			quoted = append(quoted, quoteTS(mediaType))
		}
		parts = append(parts, "{\n  /** Requested successful response media type. */\n  readonly accept?: "+strings.Join(quoted, " | ")+" | undefined\n}")
	}
	switch operation.Idempotency {
	case "required":
		parts = append(parts, "{\n  /** Required idempotency key sent through the `Idempotency-Key` header. */\n  readonly idempotencyKey: string\n}")
	case "optional":
		parts = append(parts, "{\n  /** Optional idempotency key sent through the `Idempotency-Key` header. */\n  readonly idempotencyKey?: string | undefined\n}")
	}
	switch operation.Concurrency {
	case "required":
		parts = append(parts, "{\n  /** Required entity tag sent through the `If-Match` header. */\n  readonly ifMatch: string\n}")
	case "optional":
		parts = append(parts, "{\n  /** Optional entity tag sent through the `If-Match` header. */\n  readonly ifMatch?: string | undefined\n}")
	}
	fmt.Fprintf(output, "/**\n * Per-request transport options for `%s` (`%s %s`).\n", operation.OperationID, operation.Method, operation.Path)
	if operation.Idempotency == "required" {
		output.WriteString(" * Requires an idempotency key.\n")
	}
	if operation.Concurrency == "required" {
		output.WriteString(" * Requires an `If-Match` entity tag.\n")
	}
	if boolValue(operation.Raw, "deprecated") {
		output.WriteString(" * @deprecated This operation is deprecated.\n")
	}
	output.WriteString(" */\n")
	fmt.Fprintf(output, "export type %sOptions = %s\n\n", operationName, strings.Join(parts, " & "))
	return nil
}

func operationResponseMediaTypes(document *ir.Document, operation ir.Operation) ([]string, error) {
	responses, _ := operation.Raw["responses"].(map[string]any)
	seen := make(map[string]bool)
	var result []string
	for status, value := range responses {
		if !strings.HasPrefix(status, "2") {
			continue
		}
		response, _ := value.(map[string]any)
		var err error
		response, err = resolveComponentObject(document, response, "responses")
		if err != nil {
			return nil, err
		}
		content, _ := response["content"].(map[string]any)
		for mediaType := range content {
			if !seen[mediaType] {
				seen[mediaType] = true
				result = append(result, mediaType)
			}
		}
	}
	sort.Strings(result)
	return result, nil
}

func emitRawResponseJSDoc(output *bytes.Buffer, document *ir.Document, operation ir.Operation) error {
	fmt.Fprintf(output, "/**\n * Status- and media-aware raw response for `%s` (`%s %s`).\n", operation.OperationID, operation.Method, operation.Path)
	responses, _ := operation.Raw["responses"].(map[string]any)
	statuses := make([]string, 0, len(responses))
	for status := range responses {
		if strings.HasPrefix(status, "2") {
			statuses = append(statuses, status)
		}
	}
	sort.Strings(statuses)
	if len(statuses) > 0 {
		output.WriteString(" *\n * Successful responses:\n")
	}
	for _, status := range statuses {
		response, _ := responses[status].(map[string]any)
		resolved, err := resolveComponentObject(document, response, "responses")
		if err != nil {
			return err
		}
		description, _ := resolved["description"].(string)
		content, _ := resolved["content"].(map[string]any)
		mediaTypes := make([]string, 0, len(content))
		for mediaType := range content {
			mediaTypes = append(mediaTypes, mediaType)
		}
		sort.Strings(mediaTypes)
		if len(mediaTypes) == 0 {
			fmt.Fprintf(output, " * - `%s`", status)
			if description != "" {
				fmt.Fprintf(output, " — %s", sanitizeComment(description))
			}
			output.WriteString("\n")
		} else {
			for _, mediaType := range mediaTypes {
				fmt.Fprintf(output, " * - `%s %s`", status, sanitizeComment(mediaType))
				if description != "" {
					fmt.Fprintf(output, " — %s", sanitizeComment(description))
				}
				output.WriteString("\n")
			}
		}
		headers, _ := resolved["headers"].(map[string]any)
		headerNames := make([]string, 0, len(headers))
		for name := range headers {
			headerNames = append(headerNames, name)
		}
		sort.Strings(headerNames)
		for _, name := range headerNames {
			header, _ := headers[name].(map[string]any)
			resolvedHeader, err := resolveComponentObject(document, header, "headers")
			if err != nil {
				return err
			}
			headerDescription, _ := resolvedHeader["description"].(string)
			fmt.Fprintf(output, " *   - Header `%s`", sanitizeComment(name))
			if headerDescription != "" {
				fmt.Fprintf(output, " — %s", sanitizeComment(headerDescription))
			}
			output.WriteString("\n")
		}
	}
	if boolValue(operation.Raw, "deprecated") {
		output.WriteString(" *\n * @deprecated This operation is deprecated.\n")
	}
	output.WriteString(" */\n")
	return nil
}

func emitQueryTypes(output *bytes.Buffer, document *ir.Document, operation ir.Operation, operationName string, parameters []operationParameter) error {
	var filters []operationParameter
	_, hasSort := operation.Raw["x-sort"]
	for _, parameter := range parameters {
		isPaginationControl := operation.Pagination != "" && (parameter.Name == "cursor" || parameter.Name == "offset" || parameter.Name == "limit")
		if isPaginationControl || (hasSort && parameter.Name == "sort") {
			continue
		}
		filters = append(filters, parameter)
	}
	parts := make([]string, 0, 3)
	if operation.Pagination != "" {
		paginationType := map[string]string{
			"cursor": "Contract.CursorPaginationInput",
			"offset": "Contract.OffsetPaginationInput",
			"both":   "Contract.BothPaginationInput",
		}[operation.Pagination]
		if paginationType == "" {
			return fmt.Errorf("operation %s has unsupported pagination profile %q", operation.OperationID, operation.Pagination)
		}
		parts = append(parts, paginationType)
	}
	if len(filters) > 0 {
		filterType := operationName + "FilterInput"
		fmt.Fprintf(output, "/** Filter query parameters for `%s` (`%s %s`). */\n", operation.OperationID, operation.Method, operation.Path)
		fmt.Fprintf(output, "export type %s = {\n", filterType)
		for _, parameter := range filters {
			valueType, err := schemaType(document, parameter.Schema, projectionInput)
			if err != nil {
				return err
			}
			valueType = qualifyClientType(document, valueType)
			optional := "?"
			if parameter.Required {
				optional = ""
			} else {
				valueType += " | undefined"
			}
			emitOperationParameterJSDoc(output, "  ", parameter, "Query")
			fmt.Fprintf(output, "  readonly %s%s: %s\n", parameter.Property, optional, valueType)
		}
		output.WriteString("}\n\n")
		parts = append(parts, filterType)
	}
	if fields := operationSortFields(operation); len(fields) > 0 {
		sortType := operationName + "SortInput"
		quoted := make([]string, 0, len(fields))
		for _, field := range fields {
			quoted = append(quoted, quoteTS(field))
		}
		fmt.Fprintf(output, "/** Sort expression for `%s`. */\n", operation.OperationID)
		fmt.Fprintf(output, "export type %s = {\n", sortType)
		fmt.Fprintf(output, "  /** OpenAPI field selected for sorting. */\n  readonly field: %s\n", strings.Join(quoted, " | "))
		output.WriteString("  /** Sort direction. */\n  readonly direction: Contract.SortDirection\n")
		output.WriteString("}\n\n")
		parts = append(parts, "{\n  /** Ordered sort expressions applied by the server. */\n  readonly sort?: readonly "+sortType+"[] | undefined\n}")
	}
	if len(parts) == 0 {
		if err := emitParameterType(output, document, operation, operationName+"QueryInput", "query"); err != nil {
			return err
		}
		return nil
	}
	fmt.Fprintf(output, "/**\n * Complete query input for `%s` (`%s %s`).\n", operation.OperationID, operation.Method, operation.Path)
	for _, parameter := range parameters {
		if parameter.Description != "" {
			fmt.Fprintf(output, " * - `%s`: %s\n", parameter.Property, sanitizeComment(parameter.Description))
		}
	}
	output.WriteString(" */\n")
	fmt.Fprintf(output, "export type %sQueryInput = %s\n\n", operationName, strings.Join(parts, " & "))
	return nil
}

func operationSortFields(operation ir.Operation) []string {
	value := operation.Raw["x-sort"]
	if object, ok := value.(map[string]any); ok {
		for _, key := range []string{"fields", "allowedFields"} {
			if fields, exists := object[key]; exists {
				value = fields
				break
			}
		}
	}
	items, _ := value.([]any)
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		field, _ := item.(string)
		if object, ok := item.(map[string]any); ok {
			field, _ = object["field"].(string)
			if field == "" {
				field, _ = object["name"].(string)
			}
		}
		if field != "" && !seen[field] {
			seen[field] = true
			result = append(result, field)
		}
	}
	return result
}

func qualifyClientType(document *ir.Document, value string) string {
	var typeNames []string
	reachable := reachableComponentSchemas(document)
	names := make([]string, 0, len(document.ComponentSchemas))
	for name := range document.ComponentSchemas {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !reachable[name] {
			continue
		}
		schema := document.ComponentSchemas[name]
		if isErrorSchema(document, schema) {
			continue
		}
		for _, declaration := range componentDeclarations(name, schema) {
			if typeName, err := naming.Public(declaration.name); err == nil {
				typeNames = append(typeNames, typeName)
			}
		}
	}
	sort.Slice(typeNames, func(i, j int) bool { return len(typeNames[i]) > len(typeNames[j]) })
	for _, typeName := range typeNames {
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(typeName) + `\b`)
		value = pattern.ReplaceAllString(value, "Contract."+typeName)
	}
	return value
}

func requestBodyType(document *ir.Document, body map[string]any) (string, error) {
	content, _ := body["content"].(map[string]any)
	mediaTypes := make([]string, 0, len(content))
	for mediaType := range content {
		mediaTypes = append(mediaTypes, mediaType)
	}
	sort.Strings(mediaTypes)
	if len(mediaTypes) == 0 {
		return "unknown", nil
	}
	if len(mediaTypes) == 1 {
		if isTextMedia(mediaTypes[0]) {
			return "string", nil
		}
		media, _ := content[mediaTypes[0]].(map[string]any)
		schema, _ := media["schema"].(map[string]any)
		if isBinaryMedia(mediaTypes[0], schema) {
			return "BinaryBody", nil
		}
		return schemaType(document, schema, projectionInput)
	}
	variants := make([]string, 0, len(mediaTypes))
	for _, mediaType := range mediaTypes {
		media, _ := content[mediaType].(map[string]any)
		schema, _ := media["schema"].(map[string]any)
		valueType := "string"
		var err error
		if !isTextMedia(mediaType) {
			if isBinaryMedia(mediaType, schema) {
				valueType = "BinaryBody"
			} else {
				valueType, err = schemaType(document, schema, projectionInput)
			}
		}
		if err != nil {
			return "", err
		}
		variants = append(variants, fmt.Sprintf("{ readonly contentType: %s; readonly value: %s }", quoteTS(mediaType), valueType))
	}
	return strings.Join(variants, " | "), nil
}

type resourceNode struct {
	parameter      *operationParameter
	operations     map[string]ManifestOperation
	children       map[string]*resourceNode
	parameterChild *resourceNode
}

func buildResourceTree(document *ir.Document, manifest Manifest) (*resourceNode, error) {
	root := &resourceNode{children: make(map[string]*resourceNode)}
	for _, item := range manifest.Operations {
		if item.Visibility != "public" {
			continue
		}
		operation := findOperation(document, item.OperationID)
		parameters, err := parametersIn(document, operation, "path")
		if err != nil {
			return nil, err
		}
		byName := make(map[string]operationParameter, len(parameters))
		for _, parameter := range parameters {
			byName[parameter.Name] = parameter
		}
		node := root
		for _, part := range strings.Split(strings.Trim(operation.Path, "/"), "/") {
			if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
				name := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
				parameter, ok := byName[name]
				if !ok {
					return nil, fmt.Errorf("resource path %s has undeclared parameter %q", operation.Path, name)
				}
				if node.parameterChild == nil {
					node.parameterChild = &resourceNode{parameter: &parameter, children: make(map[string]*resourceNode)}
				} else if node.parameterChild.parameter.Name != parameter.Name {
					return nil, fmt.Errorf("resource path parameter collision below %s: %q and %q", operation.Path, node.parameterChild.parameter.Name, parameter.Name)
				}
				node = node.parameterChild
				continue
			}
			property, err := naming.Property(part)
			if err != nil {
				return nil, err
			}
			if node.children[property] == nil {
				node.children[property] = &resourceNode{children: make(map[string]*resourceNode)}
			}
			node = node.children[property]
		}
		terminal, err := resourceTerminalName(operation, strings.Split(strings.Trim(operation.Path, "/"), "/"))
		if err != nil {
			return nil, err
		}
		if node.operations == nil {
			node.operations = make(map[string]ManifestOperation)
		}
		if existing, ok := node.operations[terminal]; ok {
			return nil, fmt.Errorf("resource alias collision at %s.%s between %s and %s", operation.Path, terminal, existing.OperationID, item.OperationID)
		}
		node.operations[terminal] = item
	}
	if root.parameterChild != nil {
		return nil, fmt.Errorf("resource paths may not begin with a path parameter")
	}
	if err := validateResourceNodeSymbols(root, "api"); err != nil {
		return nil, err
	}
	return root, nil
}

func validateResourceNodeSymbols(node *resourceNode, path string) error {
	members := make(map[string]string, len(node.operations)+1)
	for terminal, operation := range node.operations {
		members[terminal] = "operation " + fmt.Sprintf("%q", operation.OperationID)
	}
	if operation, ok := paginatedResourceNodeOperation(node); ok {
		if previous, exists := members["paginate"]; exists {
			return fmt.Errorf("resource member collision at %s.paginate between %s and pagination for operation %q", path, previous, operation.OperationID)
		}
		members["paginate"] = "pagination for operation " + fmt.Sprintf("%q", operation.OperationID)
	}
	for name, child := range node.children {
		if previous, exists := members[name]; exists {
			return fmt.Errorf("resource member collision at %s.%s between %s and path segment %q", path, name, previous, name)
		}
		if err := validateResourceNodeSymbols(child, path+"."+name); err != nil {
			return err
		}
	}
	if node.parameterChild != nil {
		if err := validateResourceNodeSymbols(node.parameterChild, path+"(path parameter)"); err != nil {
			return err
		}
	}
	return nil
}

func sortedResourceChildNames(node *resourceNode) []string {
	result := make([]string, 0, len(node.children))
	for name := range node.children {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func resourceTreeHasPathOperations(node *resourceNode) bool {
	if node.parameterChild != nil {
		return true
	}
	for _, child := range node.children {
		if resourceTreeHasPathOperations(child) {
			return true
		}
	}
	return false
}

func resourceTreeHasPagination(node *resourceNode) bool {
	for _, operation := range node.operations {
		if operation.Pagination != "" && len(operation.PathParameterOrder) == 0 {
			return true
		}
	}
	if node.parameterChild != nil && resourceTreeHasPagination(node.parameterChild) {
		return true
	}
	for _, child := range node.children {
		if resourceTreeHasPagination(child) {
			return true
		}
	}
	return false
}

func emitResourceTreeInterface(output *bytes.Buffer, document *ir.Document, root *resourceNode) error {
	for _, name := range sortedResourceChildNames(root) {
		output.WriteString("  /** Resource-oriented operations generated from the OpenAPI path tree. */\n")
		fmt.Fprintf(output, "  readonly %s: ", name)
		if err := emitResourceNodeInterface(output, document, root.children[name], "  "); err != nil {
			return err
		}
		output.WriteString("\n")
	}
	return nil
}

func emitResourceNodeInterface(output *bytes.Buffer, document *ir.Document, node *resourceNode, indent string) error {
	output.WriteString("{\n")
	memberIndent := indent + "  "
	for _, terminal := range sortedManifestOperationNames(node.operations) {
		operation := node.operations[terminal]
		emitOperationJSDoc(output, memberIndent, operation)
		functionType := operationFunctionType(document, operation)
		if len(operation.PathParameterOrder) > 0 {
			functionType = operationResourceFunctionType(document, operation)
		}
		fmt.Fprintf(output, "%sreadonly %s: %s\n", memberIndent, terminal, functionType)
	}
	if paginated, ok := paginatedResourceNodeOperation(node); ok {
		itemType, err := operationItemType(document, findOperation(document, paginated.OperationID))
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "%s/** Lazily iterates every item from {@link Operations.%s} pagination. */\n", memberIndent, paginated.OperationID)
		fmt.Fprintf(output, "%sreadonly paginate: %s\n", memberIndent, paginationFunctionType(paginated, qualifyClientType(document, itemType)))
	}
	for _, name := range sortedResourceChildNames(node) {
		output.WriteString(memberIndent + "/** Nested resource path segment. */\n")
		fmt.Fprintf(output, "%sreadonly %s: ", memberIndent, name)
		if err := emitResourceNodeInterface(output, document, node.children[name], memberIndent); err != nil {
			return err
		}
		output.WriteString("\n")
	}
	if node.parameterChild != nil {
		parameter := node.parameterChild.parameter
		parameterType, err := schemaType(document, parameter.Schema, projectionInput)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "%s/** Selects one resource by the `%s` path parameter. */\n", memberIndent, parameter.Name)
		fmt.Fprintf(output, "%s(%s: %s): ", memberIndent, parameter.Property, qualifyClientType(document, parameterType))
		if err := emitResourceNodeInterface(output, document, node.parameterChild, memberIndent); err != nil {
			return err
		}
		output.WriteString("\n")
	}
	output.WriteString(indent + "}")
	return nil
}

func paginatedResourceNodeOperation(node *resourceNode) (ManifestOperation, bool) {
	var result ManifestOperation
	found := false
	for _, operation := range node.operations {
		if operation.Pagination == "" || len(operation.PathParameterOrder) > 0 {
			continue
		}
		if found {
			return ManifestOperation{}, false
		}
		result, found = operation, true
	}
	return result, found
}

func emitResourceTreeValues(output *bytes.Buffer, document *ir.Document, root *resourceNode) error {
	for _, name := range sortedResourceChildNames(root) {
		fmt.Fprintf(output, "  const %s = ", name)
		if err := emitResourceNodeValue(output, document, root.children[name], nil, "  "); err != nil {
			return err
		}
		output.WriteString("\n")
	}
	return nil
}

func emitResourceNodeValue(output *bytes.Buffer, document *ir.Document, node *resourceNode, bound map[string]string, indent string) error {
	if node.parameterChild == nil {
		return emitResourceNodeObject(output, document, node, bound, indent)
	}
	parameter := node.parameterChild.parameter
	parameterType, err := schemaType(document, parameter.Schema, projectionInput)
	if err != nil {
		return err
	}
	nextBound := make(map[string]string, len(bound)+1)
	for name, value := range bound {
		nextBound[name] = value
	}
	nextBound[parameter.Name] = parameter.Property
	output.WriteString("Object.assign(\n")
	fmt.Fprintf(output, "%s  (%s: %s) => (", indent, parameter.Property, qualifyClientType(document, parameterType))
	if err := emitResourceNodeValue(output, document, node.parameterChild, nextBound, indent+"  "); err != nil {
		return err
	}
	output.WriteString("),\n")
	output.WriteString(indent + "  ")
	if err := emitResourceNodeObject(output, document, node, bound, indent+"  "); err != nil {
		return err
	}
	output.WriteString("\n" + indent + ")")
	return nil
}

func emitResourceNodeObject(output *bytes.Buffer, document *ir.Document, node *resourceNode, bound map[string]string, indent string) error {
	output.WriteString("{\n")
	memberIndent := indent + "  "
	for _, terminal := range sortedManifestOperationNames(node.operations) {
		fmt.Fprintf(output, "%s%s: ", memberIndent, terminal)
		if err := emitResourceOperationValue(output, document, node.operations[terminal], bound); err != nil {
			return err
		}
		output.WriteString(",\n")
	}
	if paginated, ok := paginatedResourceNodeOperation(node); ok {
		fmt.Fprintf(output, "%spaginate: paginate%s,\n", memberIndent, operationTypeName(paginated.OperationID))
	}
	for _, name := range sortedResourceChildNames(node) {
		fmt.Fprintf(output, "%s%s: ", memberIndent, name)
		if err := emitResourceNodeValue(output, document, node.children[name], bound, memberIndent); err != nil {
			return err
		}
		output.WriteString(",\n")
	}
	output.WriteString(indent + "}")
	return nil
}

func emitResourceOperationValue(output *bytes.Buffer, document *ir.Document, operation ManifestOperation, bound map[string]string) error {
	property, err := naming.Property(operation.OperationID)
	if err != nil {
		return err
	}
	if len(operation.PathParameterOrder) == 0 {
		output.WriteString(property)
		return nil
	}
	values := make([]string, 0, len(operation.PathParameterOrder))
	for _, parameter := range operation.PathParameterOrder {
		value, ok := bound[parameter]
		if !ok {
			return fmt.Errorf("resource operation %s is missing bound path parameter %q", operation.OperationID, parameter)
		}
		propertyName, err := naming.Property(parameter)
		if err != nil {
			return err
		}
		values = append(values, propertyName+": "+value)
	}
	name := operationTypeName(operation.OperationID)
	hasInput := len(operation.InputTypes) > 1
	fmt.Fprintf(output, "bindPathOperation<%sInput, %sResourceInput, %s, %sOptions, %sRawResponse>(%s, { %s }, %t)", name, name, qualifyClientType(document, operation.OutputType), name, name, property, strings.Join(values, ", "), hasInput)
	return nil
}

func findOperation(document *ir.Document, operationID string) ir.Operation {
	for _, operation := range document.Operations {
		if operation.OperationID == operationID {
			return operation
		}
	}
	return ir.Operation{OperationID: operationID}
}

func sortedManifestOperationNames(operations map[string]ManifestOperation) []string {
	result := make([]string, 0, len(operations))
	for name := range operations {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func operationFunctionType(document *ir.Document, operation ManifestOperation) string {
	return operationTypeName(operation.OperationID) + "Call"
}

func operationResourceFunctionType(document *ir.Document, operation ManifestOperation) string {
	return operationTypeName(operation.OperationID) + "ResourceCall"
}

func operationInputAlias(operation ManifestOperation) string {
	if len(operation.InputTypes) == 0 {
		return "never"
	}
	return operationTypeName(operation.OperationID) + "Input"
}

func paginationFunctionType(operation ManifestOperation, itemType string) string {
	operationName := operationTypeName(operation.OperationID)
	optional := "?"
	if operation.Idempotency == "required" || operation.Concurrency == "required" {
		optional = ""
	}
	return "(input: PaginateInput<" + operationName + "Input, " + quoteTS(operation.Pagination) + ">, options" + optional + ": " + operationName + "Options) => AsyncIterable<" + itemType + ">"
}

func hasVisibleInputSchemas(document *ir.Document) bool {
	for _, operation := range document.Operations {
		if operation.Visibility != "hidden" {
			if operation.Raw["requestBody"] != nil {
				return true
			}
			if parameters, err := operationParameters(document, operation); err == nil && len(parameters) > 0 {
				return true
			}
		}
	}
	return false
}

func hasVisibleResponseBodies(document *ir.Document) bool {
	for _, operation := range document.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		responses, _ := operation.Raw["responses"].(map[string]any)
		for _, value := range responses {
			response, _ := value.(map[string]any)
			resolved, err := resolveComponentObject(document, response, "responses")
			if err == nil {
				if content, ok := resolved["content"].(map[string]any); ok && len(content) > 0 {
					return true
				}
			}
		}
	}
	return false
}

func operationDefinition(document *ir.Document, irOperation ir.Operation, operation ManifestOperation) (string, error) {
	var fields []string
	fields = append(fields,
		"operationID: "+quoteTS(operation.OperationID),
		"method: "+quoteTS(operation.Method),
		"path: "+quoteTS(operation.Path),
		"envelope: "+quoteTS(operation.Envelope),
	)
	parameters, err := operationParameters(document, irOperation)
	if err != nil {
		return "", err
	}
	usesInputSchemas := false
	if len(parameters) > 0 {
		items := make([]string, 0, len(parameters))
		for _, parameter := range parameters {
			descriptor, err := wireSchemaDescriptor(parameter.Schema, projectionInput)
			if err != nil {
				return "", err
			}
			fields := []string{
				"location: " + quoteTS(parameter.Location),
				"name: " + quoteTS(parameter.Name),
				"property: " + quoteTS(parameter.Property),
				"style: " + quoteTS(parameter.Style),
				fmt.Sprintf("explode: %t", parameter.Explode),
				"schema: " + descriptor,
			}
			if parameter.ContentType != "" {
				fields = append(fields, "contentType: "+quoteTS(parameter.ContentType))
			}
			items = append(items, "{ "+strings.Join(fields, ", ")+" }")
		}
		fields = append(fields, "parameters: ["+strings.Join(items, ", ")+"]")
		usesInputSchemas = true
	}
	requestBodies, hasRequestBodies, err := operationRequestWireBodies(document, irOperation)
	if err != nil {
		return "", err
	}
	if hasRequestBodies {
		fields = append(fields, "requestBodies: "+requestBodies)
		usesInputSchemas = true
	}
	if usesInputSchemas {
		fields = append(fields, "inputSchemas: inputSchemas")
	}
	responseBodies, hasResponseBodies, err := operationResponseWireBodies(document, irOperation)
	if err != nil {
		return "", err
	}
	if hasResponseBodies {
		fields = append(fields, "outputSchemas: outputSchemas", "responses: "+responseBodies)
	}
	contentTypes, err := requestBodyContentTypes(document, irOperation)
	if err != nil {
		return "", err
	}
	if len(contentTypes) == 1 {
		fields = append(fields, "contentType: "+quoteTS(contentTypes[0]))
	}
	if serverURL := operationServerURL(document, irOperation); serverURL != "" {
		fields = append(fields, "serverURL: "+quoteTS(serverURL))
	}
	return "{ " + strings.Join(fields, ", ") + " }", nil
}

func requestBodyContentTypes(document *ir.Document, operation ir.Operation) ([]string, error) {
	body, _ := operation.Raw["requestBody"].(map[string]any)
	body, err := resolveComponentObject(document, body, "requestBodies")
	if err != nil {
		return nil, err
	}
	content, _ := body["content"].(map[string]any)
	result := make([]string, 0, len(content))
	for mediaType := range content {
		result = append(result, mediaType)
	}
	sort.Strings(result)
	return result, nil
}

func operationServerURL(document *ir.Document, operation ir.Operation) string {
	for _, source := range []any{operation.Raw["servers"], operation.PathItemRaw["servers"]} {
		servers, _ := source.([]any)
		if len(servers) == 0 {
			continue
		}
		server, _ := servers[0].(map[string]any)
		url, _ := server["url"].(string)
		if len(document.Servers) == 0 || url != document.Servers[0].URL {
			return url
		}
		return ""
	}
	return ""
}

func emitOutputJSDoc(output *bytes.Buffer, operation ir.Operation, item ManifestOperation, outputType string) {
	fmt.Fprintf(output, "/**\n * Output of `%s` (`%s %s`).\n", operation.OperationID, operation.Method, operation.Path)
	if regexp.MustCompile(`^Contract\.[A-Za-z_$][A-Za-z0-9_$]*$`).MatchString(outputType) {
		fmt.Fprintf(output, " *\n * Schema: {@link %s}.\n", outputType)
	} else {
		fmt.Fprintf(output, " *\n * Type: %s.\n", jsDocTypeReference(outputType))
	}
	if item.Deprecated {
		output.WriteString(" *\n * @deprecated This operation is deprecated.\n")
	}
	output.WriteString(" */\n")
}

func jsDocTypeReference(typeName string) string {
	typeName = inlineJSDocType(typeName)
	switch typeName {
	case "unknown", "string", "number", "boolean", "void", "never", "null", "undefined":
		return "`" + typeName + "`"
	}
	if regexp.MustCompile(`^(?:Contract\.)?[A-Za-z_$][A-Za-z0-9_$]*$`).MatchString(typeName) {
		return "{@link " + typeName + "}"
	}
	return "`" + typeName + "`"
}

func inlineJSDocType(value string) string {
	for {
		start := strings.Index(value, "/**")
		if start < 0 {
			break
		}
		end := strings.Index(value[start+3:], "*/")
		if end < 0 {
			value = value[:start]
			break
		}
		value = value[:start] + value[start+3+end+2:]
	}
	return strings.Join(strings.Fields(value), " ")
}

func emitOperationCatalogJSDoc(output *bytes.Buffer, indent string, operation ManifestOperation) {
	comment := operation.Summary
	if comment == "" {
		comment = operation.OperationID
	}
	fmt.Fprintf(output, "%s/**\n", indent)
	fmt.Fprintf(output, "%s * %s\n", indent, sanitizeComment(comment))
	if operation.Description != "" {
		fmt.Fprintf(output, "%s *\n", indent)
		fmt.Fprintf(output, "%s * %s\n", indent, sanitizeComment(operation.Description))
	}
	fmt.Fprintf(output, "%s *\n", indent)
	fmt.Fprintf(output, "%s * Operation ID: `%s`. HTTP: `%s %s`.\n", indent, operation.OperationID, operation.Method, operation.Path)
	if operation.Deprecated {
		fmt.Fprintf(output, "%s *\n", indent)
		fmt.Fprintf(output, "%s * @deprecated This operation is deprecated.\n", indent)
	}
	fmt.Fprintf(output, "%s */\n", indent)
}

func emitOperationJSDoc(output *bytes.Buffer, indent string, operation ManifestOperation) {
	comment := operation.Summary
	if comment == "" {
		comment = operation.OperationID
	}
	fmt.Fprintf(output, "%s/**\n", indent)
	fmt.Fprintf(output, "%s * %s\n", indent, sanitizeComment(comment))
	if operation.Description != "" {
		fmt.Fprintf(output, "%s *\n", indent)
		fmt.Fprintf(output, "%s * %s\n", indent, sanitizeComment(operation.Description))
	}
	fmt.Fprintf(output, "%s *\n", indent)
	fmt.Fprintf(output, "%s * Operation ID: `%s`.\n", indent, operation.OperationID)
	fmt.Fprintf(output, "%s * HTTP: `%s %s`.\n", indent, operation.Method, operation.Path)
	if operation.Deprecated {
		fmt.Fprintf(output, "%s *\n", indent)
		fmt.Fprintf(output, "%s * @deprecated This operation is deprecated.\n", indent)
	}
	fmt.Fprintf(output, "%s *\n", indent)
	fmt.Fprintf(output, "%s * @example\n", indent)
	fmt.Fprintf(output, "%s * ```ts\n", indent)
	fmt.Fprintf(output, "%s * await %s\n", indent, operation.CallExpression)
	fmt.Fprintf(output, "%s * ```\n", indent)
	fmt.Fprintf(output, "%s */\n", indent)
}
