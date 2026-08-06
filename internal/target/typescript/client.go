package typescript

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"openapi-sdkgen/internal/compiler/ir"
	"openapi-sdkgen/internal/compiler/naming"
)

type requestInputSectionDescriptor struct {
	suffix             string
	sectionKey         string
	publicHelperSuffix string
	parameterLocation  bool
}

var requestInputSectionDescriptors = []requestInputSectionDescriptor{
	{suffix: "PathInput", sectionKey: "path", publicHelperSuffix: "Path", parameterLocation: true},
	{suffix: "QueryInput", sectionKey: "query", publicHelperSuffix: "Query", parameterLocation: true},
	{suffix: "QuerystringInput", sectionKey: "querystring", publicHelperSuffix: "Querystring", parameterLocation: true},
	{suffix: "HeaderInput", sectionKey: "header", publicHelperSuffix: "Headers", parameterLocation: true},
	{suffix: "CookieInput", sectionKey: "cookie", publicHelperSuffix: "Cookies", parameterLocation: true},
	{suffix: "BodyInput", sectionKey: "body", publicHelperSuffix: "Body"},
}

func requestInputSection(operationName, inputType string) (requestInputSectionDescriptor, error) {
	for _, descriptor := range requestInputSectionDescriptors {
		if inputType == operationName+descriptor.suffix {
			return descriptor, nil
		}
	}
	return requestInputSectionDescriptor{}, fmt.Errorf("operation input type %q does not have a supported request section suffix", inputType)
}

func emitOperationTypes(output *bytes.Buffer, document *ir.Document, operation ir.Operation, item ManifestOperation) error {
	operationName := operationTypeName(operationRouteKey(operation))
	if err := emitOperationOptions(output, document, operationName, operation); err != nil {
		return err
	}
	if parameters, err := clientParametersIn(document, operation, "path"); err != nil {
		return err
	} else if len(parameters) > 0 {
		if err := emitParameterType(output, document, operation, operationName+"PathInput", "path"); err != nil {
			return err
		}
	}
	if parameters, err := clientParametersIn(document, operation, "query"); err != nil {
		return err
	} else if len(parameters) > 0 || operation.Pagination != "" || len(operation.SortParameters) > 0 {
		if err := emitQueryTypes(output, document, operation, operationName, parameters); err != nil {
			return err
		}
	}
	if parameters, err := clientParametersIn(document, operation, "querystring"); err != nil {
		return err
	} else if len(parameters) > 0 {
		if err := emitParameterType(output, document, operation, operationName+"QuerystringInput", "querystring"); err != nil {
			return err
		}
	}
	if parameters, err := clientParametersIn(document, operation, "header"); err != nil {
		return err
	} else if len(parameters) > 0 {
		if err := emitParameterType(output, document, operation, operationName+"HeaderInput", "header"); err != nil {
			return err
		}
	}
	if parameters, err := clientParametersIn(document, operation, "cookie"); err != nil {
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
		bodyType, err := requestBodyTypeForScope(document, resolvedBody, typeRenderContract)
		if err != nil {
			return err
		}
		bodyDescription, _ := resolvedBody["description"].(string)
		if bodyDescription == "" {
			bodyDescription = "Request body for `" + operation.OperationID + "` (`" + operation.Method + " " + operation.Path + "`)."
		}
		fmt.Fprintf(output, "/**\n * %s\n *\n * Type: %s\n */\n", sanitizeComment(bodyDescription), jsDocTypeReference(bodyType))
		fmt.Fprintf(output, "type %sBodyInput = %s\n\n", operationName, bodyType)
	}
	if len(item.InputTypes) > 0 {
		fmt.Fprintf(output, "/** Complete input for `%s` (`%s %s`). */\n", operation.OperationID, operation.Method, operation.Path)
		fmt.Fprintf(output, "interface %sInput {\n", operationName)
		for _, inputType := range item.InputTypes {
			field := strings.TrimPrefix(inputType, operationName)
			field = strings.TrimSuffix(field, "Input")
			property, err := aggregateInputProperty(field)
			if err != nil {
				return err
			}
			required, err := aggregateInputRequired(document, operation, field)
			if err != nil {
				return err
			}
			optional := "?"
			valueType := inputType
			if required {
				optional = ""
			} else {
				valueType += " | undefined"
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
		fmt.Fprintf(output, "type %sResourceInput = %s\n\n", operationName, resourceInput)
	}

	outputType := item.renderOutput(typeRenderContract)
	rawResponseType, err := operationRawResponseTypeForScope(document, operation, typeRenderContract)
	if err != nil {
		return err
	}
	if err := emitRawResponseJSDoc(output, document, operation); err != nil {
		return err
	}
	fmt.Fprintf(output, "type %sRawResponse = %s\n\n", operationName, rawResponseType)
	emitOutputJSDoc(output, operation, item, outputType)
	fmt.Fprintf(output, "type %sOutput = %s\n", operationName, outputType)
	output.WriteString("\n")
	if err := emitOperationCallTypes(output, document, operation, item); err != nil {
		return err
	}
	return nil
}

func emitOperationCallTypes(output *bytes.Buffer, document *ir.Document, operation ir.Operation, item ManifestOperation) error {
	operationName := operationTypeName(operationRouteKey(operation))
	routeKey := operationRouteKey(operation)
	quotedRoute := quoteTS(routeKey)
	inputType := "never"
	if len(item.InputTypes) > 0 {
		inputType = "RouteInput<" + quotedRoute + ">"
	}
	inputRequired, err := operationInputRequired(document, operation, item.InputTypes, false)
	if err != nil {
		return err
	}
	if err := emitOperationRawCallInterface(output, document, operation, operationName+"RawCall", inputType, inputType != "never" && !inputRequired, "RouteRawResponse<"+quotedRoute+">"); err != nil {
		return err
	}
	emitOperationJSDoc(output, "", item)
	if err := emitOperationCallInterface(output, document, operation, operationName+"Call", inputType, inputType != "never" && !inputRequired, "RouteOutput<"+quotedRoute+">", "OperationRawCall<"+quotedRoute+">", ""); err != nil {
		return err
	}
	if item.Visibility == "public" {
		emitResourceOperationJSDoc(output, "", item)
		resourceInput := inputType
		if len(item.PathParameterOrder) > 0 {
			resourceInput = "RouteResourceInput<" + quotedRoute + ">"
			if len(item.InputTypes) <= 1 {
				resourceInput = "never"
			}
		}
		resourceInputRequired, err := operationInputRequired(document, operation, item.InputTypes, true)
		if err != nil {
			return err
		}
		if err := emitOperationRawCallInterface(output, document, operation, operationName+"ResourceRawCall", resourceInput, resourceInput != "never" && !resourceInputRequired, "RouteRawResponse<"+quotedRoute+">"); err != nil {
			return err
		}
		if err := emitOperationCallInterface(output, document, operation, operationName+"ResourceCall", resourceInput, resourceInput != "never" && !resourceInputRequired, "RouteOutput<"+quotedRoute+">", "", "ResourceRawCapability<"+quotedRoute+">"); err != nil {
			return err
		}
	}
	return nil
}

func emitOperationCallInterface(output *bytes.Buffer, document *ir.Document, operation ir.Operation, callName, inputType string, inputOptional bool, outputType, rawCallType, rawCapabilityType string) error {
	optionsType := "RouteOptions<" + quoteTS(operationRouteKey(operation)) + ">"
	optionsRequired, err := operationRequiresSecuritySelection(document, operation)
	if err != nil {
		return err
	}
	mediaOutputs, err := operationMediaOutputTypesForScope(document, operation, typeRenderContract)
	if err != nil {
		return err
	}
	mediaTypes := make([]string, 0, len(mediaOutputs))
	for mediaType := range mediaOutputs {
		mediaTypes = append(mediaTypes, mediaType)
	}
	sort.Strings(mediaTypes)
	extends := ""
	if rawCapabilityType != "" {
		extends = " extends " + rawCapabilityType
	}
	fmt.Fprintf(output, "interface %s%s {\n", callName, extends)
	if len(mediaTypes) > 1 {
		for _, mediaType := range mediaTypes {
			mediaOptionsType := "Omit<" + optionsType + ", \"accept\"> & { readonly accept: " + quoteTS(mediaType) + " }"
			emitCallSignature(output, inputType, inputOptional, mediaOptionsType, mediaOutputs[mediaType], false)
		}
	}
	emitCallSignature(output, inputType, inputOptional, optionsType, outputType, !optionsRequired)
	if rawCallType != "" {
		output.WriteString("  /** Sends the request and returns the decoded body with HTTP response metadata. */\n")
		fmt.Fprintf(output, "  readonly raw: %s\n", rawCallType)
	}
	output.WriteString("}\n\n")
	return nil
}

func emitOperationRawCallInterface(output *bytes.Buffer, document *ir.Document, operation ir.Operation, callName, inputType string, inputOptional bool, rawType string) error {
	optionsType := "RouteOptions<" + quoteTS(operationRouteKey(operation)) + ">"
	optionsRequired, err := operationRequiresSecuritySelection(document, operation)
	if err != nil {
		return err
	}
	mediaOutputs, err := operationMediaOutputTypesForScope(document, operation, typeRenderContract)
	if err != nil {
		return err
	}
	mediaTypes := make([]string, 0, len(mediaOutputs))
	for mediaType := range mediaOutputs {
		mediaTypes = append(mediaTypes, mediaType)
	}
	sort.Strings(mediaTypes)
	fmt.Fprintf(output, "interface %s {\n", callName)
	if len(mediaTypes) > 1 {
		for _, mediaType := range mediaTypes {
			mediaOptionsType := "Omit<" + optionsType + ", \"accept\"> & { readonly accept: " + quoteTS(mediaType) + " }"
			emitRawCallSignature(output, inputType, inputOptional, mediaOptionsType, "Extract<"+rawType+", { readonly contentType: "+quoteTS(mediaType)+" }>", false)
		}
	}
	emitRawCallSignature(output, inputType, inputOptional, optionsType, rawType, !optionsRequired)
	output.WriteString("}\n\n")
	return nil
}

func emitRawCallSignature(output *bytes.Buffer, inputType string, inputOptional bool, optionsType, resultType string, optionsOptional bool) {
	optional := ""
	if optionsOptional {
		optional = "?"
	}
	if inputOptional {
		output.WriteString("  /** Sends the request with transport options and no generated operation input. */\n")
		fmt.Fprintf(output, "  (options%s: %s): Promise<%s>\n", optional, optionsType, resultType)
	}
	output.WriteString("  /**\n")
	output.WriteString("   * Sends the request and returns the decoded body with HTTP response metadata.\n")
	output.WriteString("   *\n")
	if inputType != "never" {
		output.WriteString("   * @param input Generated operation input.\n")
	}
	output.WriteString("   * @param options Per-request transport options.\n")
	output.WriteString("   * @returns Decoded response body with HTTP metadata.\n")
	output.WriteString("   */\n")
	if inputType == "never" {
		fmt.Fprintf(output, "  (options%s: %s): Promise<%s>\n", optional, optionsType, resultType)
		return
	}
	inputMarker := ""
	if inputOptional && optionsOptional {
		inputMarker = "?"
	} else if inputOptional {
		inputType += " | undefined"
	}
	fmt.Fprintf(output, "  (input%s: %s, options%s: %s): Promise<%s>\n", inputMarker, inputType, optional, optionsType, resultType)
}

func emitCallSignature(output *bytes.Buffer, inputType string, inputOptional bool, optionsType, resultType string, optionsOptional bool) {
	optional := ""
	if optionsOptional {
		optional = "?"
	}
	if inputOptional {
		output.WriteString("  /** Sends the request with transport options and no generated operation input. */\n")
		fmt.Fprintf(output, "  (options%s: %s): Promise<%s>\n", optional, optionsType, resultType)
	}
	output.WriteString("  /**\n")
	output.WriteString("   * Sends the request and returns the decoded response body.\n")
	output.WriteString("   *\n")
	if inputType != "never" {
		output.WriteString("   * @param input Generated operation input.\n")
	}
	output.WriteString("   * @param options Per-request transport options.\n")
	output.WriteString("   * @returns Decoded response body.\n")
	output.WriteString("   */\n")
	if inputType == "never" {
		fmt.Fprintf(output, "  (options%s: %s): Promise<%s>\n", optional, optionsType, resultType)
		return
	}
	inputMarker := ""
	if inputOptional && optionsOptional {
		inputMarker = "?"
	} else if inputOptional {
		inputType += " | undefined"
	}
	fmt.Fprintf(output, "  (input%s: %s, options%s: %s): Promise<%s>\n", inputMarker, inputType, optional, optionsType, resultType)
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

func aggregateInputRequired(document *ir.Document, operation ir.Operation, field string) (bool, error) {
	if field == "Body" {
		body, _ := operation.Raw["requestBody"].(map[string]any)
		resolvedBody, err := resolveComponentObject(document, body, "requestBodies")
		if err != nil {
			return false, err
		}
		return boolValue(resolvedBody, "required"), nil
	}
	location := strings.ToLower(field)
	parameters, err := clientParametersIn(document, operation, location)
	if err != nil {
		return false, err
	}
	for _, parameter := range parameters {
		if parameter.Required {
			return true, nil
		}
	}
	return false, nil
}

func operationInputRequired(document *ir.Document, operation ir.Operation, inputTypes []string, omitPath bool) (bool, error) {
	operationName := operationTypeName(operationRouteKey(operation))
	for _, inputType := range inputTypes {
		field := strings.TrimPrefix(inputType, operationName)
		field = strings.TrimSuffix(field, "Input")
		if omitPath && field == "Path" {
			continue
		}
		required, err := aggregateInputRequired(document, operation, field)
		if err != nil {
			return false, err
		}
		if required {
			return true, nil
		}
	}
	return false, nil
}

func emitParameterType(output *bytes.Buffer, document *ir.Document, operation ir.Operation, typeName, location string) error {
	parameters, err := clientParametersIn(document, operation, location)
	if err != nil {
		return err
	}
	locationLabel := strings.ToUpper(location[:1]) + location[1:]
	fmt.Fprintf(output, "/** %s parameters for `%s` (`%s %s`). */\n", locationLabel, operation.OperationID, operation.Method, operation.Path)
	fmt.Fprintf(output, "interface %s {\n", typeName)
	for _, parameter := range parameters {
		valueType, err := schemaTypeForScope(document, parameter.Schema, projectionInput, typeRenderContract)
		if err != nil {
			return err
		}
		optional := "?"
		if parameter.Required {
			optional = ""
		} else {
			valueType += " | undefined"
		}
		emitOperationParameterJSDoc(output, "  ", parameter, locationLabel)
		fmt.Fprintf(output, "  readonly %s%s: %s\n", quoteTS(parameter.Property), optional, valueType)
	}
	output.WriteString("}\n\n")
	return nil
}

func emitOperationParameterJSDoc(output *bytes.Buffer, indent string, parameter operationParameter, locationLabel string) {
	documentation := make(map[string]any, 2)
	if schema, ok := parameter.Schema.(map[string]any); ok {
		for key, value := range schema {
			documentation[key] = value
		}
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
	parts := []string{`Omit<RequestOptions, "accept">`}
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
	requirements, hasSecurity, err := operationSecurityRequirements(document, operation)
	if err != nil {
		return err
	}
	if hasSecurity && len(requirements) > 1 {
		ids := make([]string, 0, len(requirements))
		for _, requirement := range requirements {
			ids = append(ids, quoteTS(requirement.id))
		}
		parts = append(parts, "{\n  /** OpenAPI security requirement selected for this request. */\n  readonly securityRequirement: "+strings.Join(ids, " | ")+"\n}")
	}
	fmt.Fprintf(output, "/**\n * Per-request transport options for `%s` (`%s %s`).\n", operation.OperationID, operation.Method, operation.Path)
	if boolValue(operation.Raw, "deprecated") {
		output.WriteString(" * @deprecated This operation is deprecated.\n")
	}
	output.WriteString(" */\n")
	fmt.Fprintf(output, "type %sOptions = %s\n\n", operationName, strings.Join(parts, " & "))
	return nil
}

func operationResponseMediaTypes(document *ir.Document, operation ir.Operation) ([]string, error) {
	responses, _ := operation.Raw["responses"].(map[string]any)
	seen := make(map[string]bool)
	var result []string
	for status, value := range responses {
		if !isSuccessResponseStatus(status) {
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
		if isSuccessResponseStatus(status) {
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
	var sorts []operationParameter
	for _, parameter := range parameters {
		if parameter.Sort != nil {
			sorts = append(sorts, parameter)
			continue
		}
		filters = append(filters, parameter)
	}
	parts := make([]string, 0, 3)
	if len(filters) > 0 {
		filterType := operationName + "FilterInput"
		fmt.Fprintf(output, "/** Filter query parameters for `%s` (`%s %s`). */\n", operation.OperationID, operation.Method, operation.Path)
		fmt.Fprintf(output, "type %s = {\n", filterType)
		for _, parameter := range filters {
			valueType, err := schemaTypeForScope(document, parameter.Schema, projectionInput, typeRenderContract)
			if err != nil {
				return err
			}
			optional := "?"
			if parameter.Required {
				optional = ""
			} else {
				valueType += " | undefined"
			}
			emitOperationParameterJSDoc(output, "  ", parameter, "Query")
			fmt.Fprintf(output, "  readonly %s%s: %s\n", quoteTS(parameter.Property), optional, valueType)
		}
		output.WriteString("}\n\n")
		parts = append(parts, filterType)
	}
	for index, parameter := range sorts {
		sortType := operationName + "SortInput"
		if len(sorts) > 1 {
			sortType += fmt.Sprintf("%d", index+1)
		}
		members := make([]string, 0, len(parameter.Sort.Values))
		for _, value := range parameter.Sort.Values {
			members = append(members, "{ readonly field: "+quoteTS(value.Field)+"; readonly direction: "+quoteTS(value.Direction)+" }")
		}
		fmt.Fprintf(output, "/** Structured sort expression for exact query parameter `%s`. */\n", sanitizeComment(parameter.Name))
		fmt.Fprintf(output, "type %s = %s\n\n", sortType, strings.Join(members, " | "))
		optional := "?"
		valueType := "readonly " + sortType + "[]"
		if parameter.Required {
			optional = ""
		} else {
			valueType += " | undefined"
		}
		parts = append(parts, "{\n  /** Ordered sort expressions serialized to the declared OpenAPI enum. */\n  readonly "+quoteTS(parameter.Property)+optional+": "+valueType+"\n}")
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
	fmt.Fprintf(output, "type %sQueryInput = %s\n\n", operationName, strings.Join(parts, " & "))
	return nil
}

func requestBodyType(document *ir.Document, body map[string]any) (string, error) {
	return requestBodyTypeForScope(document, body, typeRenderLocal)
}

func requestBodyTypeForScope(document *ir.Document, body map[string]any, scope typeRenderScope) (string, error) {
	content, _ := body["content"].(map[string]any)
	mediaTypes := make([]string, 0, len(content))
	for mediaType := range content {
		mediaTypes = append(mediaTypes, mediaType)
	}
	sort.Strings(mediaTypes)
	if len(mediaTypes) == 0 {
		return "unknown", nil
	}
	if len(mediaTypes) == 1 && !strings.Contains(mediaTypes[0], "*") {
		media, _ := content[mediaTypes[0]].(map[string]any)
		media, err := resolveMediaTypeObject(document, media)
		if err != nil {
			return "", err
		}
		if isStreamingRequestMediaType(mediaTypes[0], media) {
			itemSchema, exists := media["itemSchema"]
			if !exists {
				return "", fmt.Errorf("streaming request body %s has no itemSchema", mediaTypes[0])
			}
			itemType, err := schemaTypeForScope(document, itemSchema, projectionInput, scope)
			if err != nil {
				return "", err
			}
			return "AsyncIterable<" + itemType + ">", nil
		}
		if isTextMedia(mediaTypes[0]) {
			return "string", nil
		}
		schema := media["schema"]
		schemaObject, _ := schema.(map[string]any)
		if schema == false {
			return "never", nil
		}
		if isBinaryMedia(mediaTypes[0], schemaObject) {
			return "BinaryBody", nil
		}
		return schemaTypeForScope(document, schema, projectionInput, scope)
	}
	variants := make([]string, 0, len(mediaTypes))
	for _, mediaType := range mediaTypes {
		media, _ := content[mediaType].(map[string]any)
		media, err := resolveMediaTypeObject(document, media)
		if err != nil {
			return "", err
		}
		schema := media["schema"]
		schemaObject, _ := schema.(map[string]any)
		valueType := "string"
		if schema == false {
			valueType = "never"
		} else if isStreamingRequestMediaType(mediaType, media) {
			itemSchema, exists := media["itemSchema"]
			if !exists {
				return "", fmt.Errorf("streaming request body %s has no itemSchema", mediaType)
			}
			itemType, err := schemaTypeForScope(document, itemSchema, projectionInput, scope)
			if err != nil {
				return "", err
			}
			valueType = "AsyncIterable<" + itemType + ">"
		} else if !isTextMedia(mediaType) {
			if isBinaryMedia(mediaType, schemaObject) {
				valueType = "BinaryBody"
			} else {
				valueType, err = schemaTypeForScope(document, schema, projectionInput, scope)
			}
		}
		if err != nil {
			return "", err
		}
		variants = append(variants, fmt.Sprintf("{ readonly contentType: %s; readonly value: %s }", quoteTS(mediaType), valueType))
	}
	return strings.Join(variants, " | "), nil
}

func isStreamingRequestMediaType(mediaType string, media map[string]any) bool {
	_, hasItemSchema := media["itemSchema"]
	return hasItemSchema
}

type resourceNode struct {
	parameter          *operationParameter
	parameterSignature string
	parameterChild     *resourceNode
	parameterBlocked   bool
	operations         map[string]ManifestOperation
	blockedOperations  map[string]bool
	children           map[string]*resourceNode
	childSources       map[string]string
	blockedChildren    map[string]bool
	pagination         *ManifestOperation
	suppressPagination bool
}

func newResourceNode() *resourceNode {
	return &resourceNode{
		operations:        make(map[string]ManifestOperation),
		blockedOperations: make(map[string]bool),
		children:          make(map[string]*resourceNode),
		childSources:      make(map[string]string),
		blockedChildren:   make(map[string]bool),
	}
}

func buildResourceTree(document *ir.Document, manifest Manifest, capabilities ...map[string]map[string]bool) (*resourceNode, error) {
	fixedMembers := map[string]map[string]bool{}
	if len(capabilities) != 0 {
		fixedMembers = capabilities[0]
	}
	if err := validateTemplatedResourcePaths(document); err != nil {
		return nil, err
	}
	root := newResourceNode()
	for _, item := range manifest.Operations {
		if item.Visibility != "public" {
			continue
		}
		operation := findOperation(document, manifestRouteKey(item))
		if hasDuplicateStrings(operation.PathParameterOrder) {
			continue
		}
		parameters, err := parametersIn(document, operation, "path")
		if err != nil {
			return nil, err
		}
		byName := make(map[string]operationParameter, len(parameters))
		for _, parameter := range parameters {
			byName[parameter.Name] = parameter
		}
		node := root
		omitted := false
		parts := resourcePathParts(operation.Path)
		for index, part := range parts {
			name, parameterPart, supported := resourcePathPart(part)
			if !supported {
				omitted = true
				break
			}
			if parameterPart {
				parameter, ok := byName[name]
				if !ok {
					return nil, fmt.Errorf("resource path %s has undeclared parameter %q", operation.Path, name)
				}
				if index == 0 || node.parameterBlocked {
					omitted = true
					break
				}
				signature, err := resourceParameterSignature(document, parameter)
				if err != nil {
					return nil, fmt.Errorf("resource path %s parameter %q: %w", operation.Path, name, err)
				}
				if node.parameterChild == nil {
					node.parameterChild = newResourceNode()
					node.parameterChild.parameter = &parameter
					node.parameterSignature = signature
				} else if node.parameterSignature != signature {
					node.parameterChild = nil
					node.parameterBlocked = true
					omitted = true
					break
				}
				node = node.parameterChild
				continue
			}
			property, err := naming.Property(part)
			if err != nil {
				omitted = true
				break
			}
			if node.blockedChildren[property] {
				omitted = true
				break
			}
			if source, exists := node.childSources[property]; exists && source != part {
				delete(node.children, property)
				delete(node.childSources, property)
				node.blockedChildren[property] = true
				omitted = true
				break
			}
			if node.children[property] == nil {
				node.children[property] = newResourceNode()
				node.childSources[property] = part
			}
			node = node.children[property]
		}
		if omitted {
			continue
		}
		terminal, err := resourceTerminalName(operation, parts)
		if err != nil {
			return nil, err
		}
		if node.blockedOperations[terminal] {
			continue
		}
		if _, ok := node.operations[terminal]; ok {
			delete(node.operations, terminal)
			node.blockedOperations[terminal] = true
			continue
		}
		node.operations[terminal] = item
	}
	resolveResourceNodeCollisions(root, fixedMembers)
	pruneEmptyResourceNodes(root)
	return root, nil
}

func validateTemplatedResourcePaths(document *ir.Document) error {
	paths := make(map[string]string)
	rawPaths, _ := document.Raw["paths"].(map[string]any)
	sourcePaths := make([]string, 0, len(rawPaths))
	for path := range rawPaths {
		if !strings.HasPrefix(path, "/") {
			continue
		}
		sourcePaths = append(sourcePaths, path)
	}
	if len(sourcePaths) == 0 {
		for _, operation := range document.Operations {
			sourcePaths = append(sourcePaths, operation.Path)
		}
	}
	sort.Strings(sourcePaths)
	for _, path := range sourcePaths {
		shape := regexp.MustCompile(`\{[^{}]+\}`).ReplaceAllString(path, "{}")
		if shape == path {
			continue
		}
		if previous, exists := paths[shape]; exists && previous != path {
			return fmt.Errorf("OpenAPI paths %q and %q have identical templated shape %q; path parameter names do not distinguish paths", previous, path, shape)
		}
		paths[shape] = path
	}
	return nil
}

func validateOperationIdentities(document *ir.Document) error {
	seenRoutes := make(map[string]string, len(document.Operations))
	seenIDs := make(map[string]string, len(document.Operations))
	for _, operation := range document.Operations {
		location := operation.Method + " " + operation.Path
		routeKey := operationRouteKey(operation)
		if previous, exists := seenRoutes[routeKey]; exists {
			return fmt.Errorf("OpenAPI route identity %q is duplicated by %s and %s", routeKey, previous, location)
		}
		seenRoutes[routeKey] = location
		if operation.OperationID == "" {
			continue
		}
		if previous, exists := seenIDs[operation.OperationID]; exists {
			return fmt.Errorf("OpenAPI operationId %q is duplicated by %s and %s", operation.OperationID, previous, location)
		}
		seenIDs[operation.OperationID] = location
	}
	return nil
}

func pruneEmptyResourceNodes(node *resourceNode) {
	if node.parameterChild != nil {
		pruneEmptyResourceNodes(node.parameterChild)
		if resourceNodeEmpty(node.parameterChild) {
			node.parameterChild = nil
			node.parameterSignature = ""
		}
	}
	for name, child := range node.children {
		pruneEmptyResourceNodes(child)
		if resourceNodeEmpty(child) {
			delete(node.children, name)
			delete(node.childSources, name)
		}
	}
}

func resourceParameterSignature(document *ir.Document, parameter operationParameter) (string, error) {
	inputType, err := schemaTypeForScope(document, parameter.Schema, projectionInput, typeRenderContract)
	if err != nil {
		return "", err
	}
	wireSchema, err := wireSchemaDescriptorForDocument(document, parameter.Schema, projectionInput)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		inputType,
		parameter.Location,
		parameter.Style,
		strconv.FormatBool(parameter.Explode),
		strconv.FormatBool(parameter.AllowReserved),
		parameter.ContentType,
		wireSchema,
	}, "\x00"), nil
}

func resolveResourceNodeCollisions(node *resourceNode, fixedMembers map[string]map[string]bool) {
	if node.parameterChild != nil {
		resolveResourceNodeCollisions(node.parameterChild, fixedMembers)
	}
	for _, child := range node.children {
		resolveResourceNodeCollisions(child, fixedMembers)
	}
	if operation, ok := paginatedResourceNodeOperation(node); ok && !node.suppressPagination {
		node.pagination = &operation
		delete(node.operations, "paginate")
		if _, exists := node.children["paginate"]; exists {
			delete(node.children, "paginate")
			delete(node.childSources, "paginate")
		}
	}
	for name, operation := range node.operations {
		child := node.children[name]
		if child == nil {
			continue
		}
		if child.parameterChild != nil {
			delete(node.operations, name)
			continue
		}
		for _, fixed := range resourceOperationFixedMembers(operation, fixedMembers) {
			removeResourceNodeMember(child, fixed)
		}
		if resourceNodeEmpty(child) {
			delete(node.children, name)
			delete(node.childSources, name)
		}
	}
}

func resourceOperationFixedMembers(operation ManifestOperation, fixedMembers map[string]map[string]bool) []string {
	result := []string{"raw"}
	if operation.Pagination != "" {
		result = append(result, "paginate")
	}
	for _, name := range []string{"links", "stream"} {
		if fixedMembers[manifestRouteKey(operation)][name] {
			result = append(result, name)
		}
	}
	return result
}

func resourceCapabilityMembers(links []generatedLink, streams []generatedStream) map[string]map[string]bool {
	result := make(map[string]map[string]bool)
	add := func(routeKey, name string) {
		if result[routeKey] == nil {
			result[routeKey] = make(map[string]bool)
		}
		result[routeKey][name] = true
	}
	for _, link := range links {
		add(operationRouteKey(link.SourceOperation), "links")
	}
	for _, stream := range streams {
		add(operationRouteKey(stream.Operation), "stream")
	}
	return result
}

func removeResourceNodeMember(node *resourceNode, name string) {
	delete(node.operations, name)
	delete(node.children, name)
	delete(node.childSources, name)
	if name == "paginate" {
		node.suppressPagination = true
	}
}

func resourceNodeEmpty(node *resourceNode) bool {
	if node.parameterChild != nil || len(node.operations) > 0 || len(node.children) > 0 {
		return false
	}
	if node.pagination != nil && !node.suppressPagination {
		return false
	}
	return true
}

func resourceOperationIDs(node *resourceNode, result map[string]bool) {
	for _, operation := range node.operations {
		result[manifestRouteKey(operation)] = true
	}
	if node.parameterChild != nil {
		resourceOperationIDs(node.parameterChild, result)
	}
	for _, child := range node.children {
		resourceOperationIDs(child, result)
	}
}

func sortedResourceMemberNames(node *resourceNode) []string {
	names := make(map[string]bool, len(node.operations)+len(node.children))
	for name := range node.operations {
		names[name] = true
	}
	for name := range node.children {
		names[name] = true
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func sortedResourceChildNames(node *resourceNode) []string {
	result := make([]string, 0, len(node.children))
	for name := range node.children {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func paginatedResourceNodeOperation(node *resourceNode) (ManifestOperation, bool) {
	if node.pagination != nil {
		return *node.pagination, !node.suppressPagination
	}
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

func findOperation(document *ir.Document, identity string) ir.Operation {
	for _, operation := range document.Operations {
		if operationRouteKey(operation) == identity || operation.OperationID == identity {
			return operation
		}
	}
	return ir.Operation{RouteKey: identity}
}

func operationInputAlias(operation ManifestOperation) string {
	if len(operation.InputTypes) == 0 {
		return "never"
	}
	return operationTypeName(manifestRouteKey(operation)) + "Input"
}

func paginationFunctionType(operation ManifestOperation, itemType string, optionsRequired bool) string {
	routeKey := quoteTS(manifestRouteKey(operation))
	cursor := operation.paginationRequest.Cursor
	offset := operation.paginationRequest.Offset
	if cursor == "" && (operation.Pagination == "cursor" || operation.Pagination == "both") {
		cursor = "cursor"
	}
	if offset == "" && (operation.Pagination == "offset" || operation.Pagination == "both") {
		offset = "offset"
	}
	optionsMarker := "?"
	if optionsRequired {
		optionsMarker = ""
	}
	return "(input: PaginateInput<RouteInput<" + routeKey + ">, " + quoteTS(operation.Pagination) + ", " + quoteTS(cursor) + ", " + quoteTS(offset) + ">, options" + optionsMarker + ": RouteOptions<" + routeKey + ">) => AsyncIterable<" + itemType + ">"
}

func operationValueName(operationID string) string {
	return stablePrivateIdentifier("operation-value", operationID)
}

func operationSlotType(routeKey, slot string) string {
	return "Routes[" + quoteTS(routeKey) + "][" + quoteTS(slot) + "]"
}

func operationBaseValueName(operationID string) string {
	return stablePrivateIdentifier("operation-base-value", operationID)
}

func operationPaginationValueName(operationID string) string {
	return stablePrivateIdentifier("operation-pagination-value", operationID)
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
				if headers, ok := resolved["headers"].(map[string]any); ok && len(headers) > 0 {
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
		"route: "+quoteTS(manifestRouteKey(operation)),
		"method: "+quoteTS(operation.Method),
		"path: "+quoteTS(operation.Path),
		"envelope: "+quoteTS(operation.Envelope),
	)
	if operation.OperationID != "" {
		fields = append(fields, "operationID: "+quoteTS(operation.OperationID))
	}
	parameters, err := clientOperationParameters(document, irOperation)
	if err != nil {
		return "", err
	}
	usesInputSchemas := false
	if len(parameters) > 0 {
		items := make([]string, 0, len(parameters))
		for _, parameter := range parameters {
			descriptor, err := wireSchemaDescriptorForDocument(document, parameter.Schema, projectionInput)
			if err != nil {
				return "", err
			}
			fields := []string{
				"location: " + quoteTS(parameter.Location),
				"name: " + quoteTS(parameter.Name),
				"property: " + quoteTS(parameter.Property),
				"style: " + quoteTS(parameter.Style),
				fmt.Sprintf("explode: %t", parameter.Explode),
				fmt.Sprintf("allowReserved: %t", parameter.AllowReserved),
				fmt.Sprintf("required: %t", parameter.Required),
				"schema: " + descriptor,
			}
			if parameter.ContentType != "" {
				fields = append(fields, "contentType: "+quoteTS(parameter.ContentType))
			}
			if parameter.Sort != nil {
				values := make([]runtimeProperty, 0, len(parameter.Sort.Values))
				for _, value := range parameter.Sort.Values {
					values = append(values, runtimeProperty{key: value.Field + "\x00" + value.Direction, value: quoteTS(value.Wire)})
				}
				fields = append(fields, "sort: "+runtimeObjectExpression(values))
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
		body, _ := irOperation.Raw["requestBody"].(map[string]any)
		resolvedBody, err := resolveComponentObject(document, body, "requestBodies")
		if err != nil {
			return "", err
		}
		if boolValue(resolvedBody, "required") {
			fields = append(fields, "requestBodyRequired: true")
		}
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
	security, hasSecurity, err := operationSecurityDefinition(document, irOperation)
	if err != nil {
		return "", err
	}
	if hasSecurity {
		fields = append(fields, "security: "+security)
	}
	contentTypes, err := requestBodyContentTypes(document, irOperation)
	if err != nil {
		return "", err
	}
	if len(contentTypes) == 1 {
		fields = append(fields, "contentType: "+quoteTS(contentTypes[0]))
	}
	fields = append(fields, "servers: "+operationServers(document, irOperation))
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

func operationServers(document *ir.Document, operation ir.Operation) string {
	values, pointer := effectiveOperationServers(document, operation)
	if len(values) == 0 {
		return `[{ id: "#", url: "/" }]`
	}
	entries := make([]string, 0, len(values))
	for index, value := range values {
		server, _ := value.(map[string]any)
		url, _ := server["url"].(string)
		fields := []string{"id: " + quoteTS(fmt.Sprintf("%s/%d", pointer, index)), "url: " + quoteTS(url)}
		variables, _ := server["variables"].(map[string]any)
		if len(variables) > 0 {
			names := sortedAnyKeys(variables)
			items := make([]string, 0, len(names))
			for _, name := range names {
				variable, _ := variables[name].(map[string]any)
				defaultValue, _ := variable["default"].(string)
				item := "{ name: " + quoteTS(name) + ", defaultValue: " + quoteTS(defaultValue)
				if enum, ok := variable["enum"].([]any); ok && len(enum) > 0 {
					values := make([]string, 0, len(enum))
					for _, value := range enum {
						if text, ok := value.(string); ok {
							values = append(values, quoteTS(text))
						}
					}
					item += ", enumValues: [" + strings.Join(values, ", ") + "]"
				}
				items = append(items, item+" }")
			}
			fields = append(fields, "variables: ["+strings.Join(items, ", ")+"]")
		}
		entries = append(entries, "{ "+strings.Join(fields, ", ")+" }")
	}
	return "[" + strings.Join(entries, ", ") + "]"
}

func effectiveOperationServers(document *ir.Document, operation ir.Operation) ([]any, string) {
	if values, exists := operation.Raw["servers"]; exists {
		servers, _ := values.([]any)
		return servers, openAPIPointer("paths", operation.Path, strings.ToLower(operation.Method), "servers")
	}
	if values, exists := operation.PathItemRaw["servers"]; exists {
		servers, _ := values.([]any)
		return servers, openAPIPointer("paths", operation.Path, "servers")
	}
	servers, _ := document.Raw["servers"].([]any)
	return servers, openAPIPointer("servers")
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

func emitOperationEntryJSDoc(output *bytes.Buffer, indent string, operation ManifestOperation) {
	comment := operation.Summary
	if comment == "" {
		comment = manifestRouteKey(operation)
	}
	fmt.Fprintf(output, "%s/**\n", indent)
	fmt.Fprintf(output, "%s * %s\n", indent, sanitizeComment(comment))
	if operation.Description != "" {
		fmt.Fprintf(output, "%s *\n", indent)
		fmt.Fprintf(output, "%s * %s\n", indent, sanitizeComment(operation.Description))
	}
	fmt.Fprintf(output, "%s *\n", indent)
	if operation.OperationID != "" {
		fmt.Fprintf(output, "%s * Operation ID: `%s`. HTTP: `%s %s`.\n", indent, operation.OperationID, operation.Method, operation.Path)
	} else {
		fmt.Fprintf(output, "%s * HTTP: `%s %s`.\n", indent, operation.Method, operation.Path)
	}
	if operation.Deprecated {
		fmt.Fprintf(output, "%s *\n", indent)
		fmt.Fprintf(output, "%s * @deprecated This operation is deprecated.\n", indent)
	}
	fmt.Fprintf(output, "%s */\n", indent)
}

func emitOperationJSDoc(output *bytes.Buffer, indent string, operation ManifestOperation) {
	emitOperationCallJSDoc(output, indent, operation, true)
}

func emitResourceOperationJSDoc(output *bytes.Buffer, indent string, operation ManifestOperation) {
	emitOperationCallJSDoc(output, indent, operation, false)
}

func emitOperationCallJSDoc(output *bytes.Buffer, indent string, operation ManifestOperation, includeHTTP bool) {
	comment := operation.Summary
	if comment == "" {
		comment = manifestRouteKey(operation)
	}
	fmt.Fprintf(output, "%s/**\n", indent)
	fmt.Fprintf(output, "%s * %s\n", indent, sanitizeComment(comment))
	if operation.Description != "" {
		fmt.Fprintf(output, "%s *\n", indent)
		fmt.Fprintf(output, "%s * %s\n", indent, sanitizeComment(operation.Description))
	}
	fmt.Fprintf(output, "%s *\n", indent)
	if operation.OperationID != "" {
		fmt.Fprintf(output, "%s * Operation ID: `%s`.\n", indent, operation.OperationID)
	}
	if includeHTTP {
		fmt.Fprintf(output, "%s * HTTP: `%s %s`.\n", indent, operation.Method, operation.Path)
	}
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
