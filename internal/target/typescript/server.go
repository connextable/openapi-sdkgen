package typescript

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/compiler/naming"
)

type webhookDefinition struct {
	name         string
	property     string
	typeName     string
	operationID  string
	method       string
	bodyType     string
	hasBody      bool
	bodyRequired bool
	bodyPlans    string
	parameters   string
	responseType string
	responsePlan string
	security     any
}

type callbackDefinition struct {
	name         string
	typeName     string
	expression   string
	operationID  string
	method       string
	bodyType     string
	hasBody      bool
	bodyRequired bool
	bodyPlans    string
	parameters   string
	responseType string
	responsePlan string
	security     any
}

func emitServerArtifacts(document *ir.Document) ([]Artifact, error) {
	webhooks, err := collectWebhooks(document)
	if err != nil {
		return nil, err
	}
	webhookSource, err := emitWebhooks(document, webhooks)
	if err != nil {
		return nil, err
	}
	callbacks, err := collectCallbacks(document)
	if err != nil {
		return nil, err
	}
	callbackSource, err := emitCallbacks(document, callbacks)
	if err != nil {
		return nil, err
	}
	return []Artifact{
		{Path: "server/runtime.ts", Data: generatedSource(serverRuntimeSource())},
		{Path: "server/webhooks.ts", Data: generatedSource(webhookSource)},
		{Path: "server/callbacks.ts", Data: generatedSource(callbackSource)},
	}, nil
}

func collectCallbacks(document *ir.Document) ([]callbackDefinition, error) {
	result := make([]callbackDefinition, 0)
	seenNames := make(map[string]callbackDefinition)
	for _, operation := range document.Operations {
		callbacks, _ := operation.Raw["callbacks"].(map[string]any)
		definitions, err := collectCallbackMap(document, callbacks, openAPIPointer("paths", operation.Path, strings.ToLower(operation.Method), "callbacks"))
		if err != nil {
			return nil, err
		}
		result = append(result, definitions...)
	}
	components, _ := document.Raw["components"].(map[string]any)
	componentCallbacks, _ := components["callbacks"].(map[string]any)
	definitions, err := collectCallbackMap(document, componentCallbacks, openAPIPointer("components", "callbacks"))
	if err != nil {
		return nil, err
	}
	result = append(result, definitions...)
	unique := make([]callbackDefinition, 0, len(result))
	for _, callback := range result {
		if previous, exists := seenNames[callback.typeName]; exists {
			if previous.expression == callback.expression && previous.method == callback.method && previous.operationID == callback.operationID && previous.bodyType == callback.bodyType && previous.bodyPlans == callback.bodyPlans && previous.responseType == callback.responseType && reflect.DeepEqual(previous.security, callback.security) {
				continue
			}
			return nil, fmt.Errorf("%s and %s both generate callback symbol %s", previous.name, callback.name, callback.typeName)
		}
		seenNames[callback.typeName] = callback
		unique = append(unique, callback)
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i].typeName < unique[j].typeName })
	return unique, nil
}

func collectCallbackMap(document *ir.Document, values map[string]any, path string) ([]callbackDefinition, error) {
	names := sortedAnyKeys(values)
	result := make([]callbackDefinition, 0, len(names))
	for _, name := range names {
		callback, ok := values[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be a Callback Object", appendOpenAPIPointer(path, name))
		}
		resolved, err := resolveComponentObject(document, callback, "callbacks")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", appendOpenAPIPointer(path, name), err)
		}
		typeName, err := naming.Public(name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", appendOpenAPIPointer(path, name), err)
		}
		for _, expression := range sortedAnyKeys(resolved) {
			pathItem, ok := resolved[expression].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s must be a Callback Path Item Object", appendOpenAPIPointer(appendOpenAPIPointer(path, name), expression))
			}
			for _, method := range serverHTTPMethods {
				operation, ok := pathItem[method].(map[string]any)
				if !ok {
					continue
				}
				operationID, _ := operation["operationId"].(string)
				if operationID == "" {
					operationID = name
				}
				operationPath := appendOpenAPIPointer(appendOpenAPIPointer(appendOpenAPIPointer(path, name), expression), method)
				parameters, err := inboundParameterDefinitions(document, pathItem, operation, operationPath, true)
				if err != nil {
					return nil, err
				}
				body, err := inboundBodyType(document, operation, operationPath)
				if err != nil {
					return nil, err
				}
				responseType, responsePlan, err := inboundResponseDefinition(document, operation, operationPath)
				if err != nil {
					return nil, err
				}
				security := operation["security"]
				if security == nil {
					security = document.Raw["security"]
				}
				result = append(result, callbackDefinition{name: appendOpenAPIPointer(path, name), typeName: typeName, expression: expression, operationID: operationID, method: strings.ToUpper(method), bodyType: body.typeName, hasBody: body.hasBody, bodyRequired: body.required, bodyPlans: body.plans, parameters: parameters, responseType: responseType, responsePlan: responsePlan, security: security})
			}
		}
	}
	return result, nil
}

func inboundResponseType(document *ir.Document, operation map[string]any, path string) (string, error) {
	responseType, _, err := inboundResponseDefinition(document, operation, path)
	return responseType, err
}

func inboundResponseDefinition(document *ir.Document, operation map[string]any, path string) (string, string, error) {
	responses, _ := operation["responses"].(map[string]any)
	statuses := sortedAnyKeys(responses)
	if len(statuses) == 0 {
		return "InboundResponse", "[]", nil
	}
	values := make([]string, 0, len(statuses)+1)
	plans := make([]string, 0, len(statuses))
	for _, status := range statuses {
		response, _ := responses[status].(map[string]any)
		resolved, err := resolveComponentObject(document, response, "responses")
		if err != nil {
			return "", "", fmt.Errorf("%s/responses/%s: %w", path, status, err)
		}
		statusType := "number"
		if status != "default" && !strings.ContainsAny(status, "Xx") {
			statusType = status
		}
		content, _ := resolved["content"].(map[string]any)
		headers, err := responseWireHeaders(document, resolved)
		if err != nil {
			return "", "", fmt.Errorf("%s/responses/%s/headers: %w", path, status, err)
		}
		headerValues, err := inboundResponseHeaderValuesType(document, resolved)
		if err != nil {
			return "", "", fmt.Errorf("%s/responses/%s/headers: %w", path, status, err)
		}
		headerField := "; readonly headers?: HeadersInit | undefined"
		if headerValues != "" {
			headerField += "; readonly headerValues?: " + headerValues + " | undefined"
		}
		mediaTypes := sortedAnyKeys(content)
		if len(mediaTypes) == 0 {
			values = append(values, "{ readonly status: "+statusType+headerField+"; readonly body?: never }")
			plan := "{ status: " + quoteTS(status)
			if headers != "" {
				plan += ", headers: " + headers
			}
			plans = append(plans, plan+" }")
			continue
		}
		for _, mediaType := range mediaTypes {
			media, _ := content[mediaType].(map[string]any)
			media, err = resolveMediaTypeObject(document, media)
			if err != nil {
				return "", "", fmt.Errorf("%s/responses/%s/content/%s: %w", path, status, mediaType, err)
			}
			schema, _ := media["schema"].(map[string]any)
			bodyType := "ArrayBuffer | Blob | ArrayBufferView"
			if isJSONMediaType(mediaType) || strings.Contains(strings.ToLower(mediaType), "xml") {
				bodyType, err = schemaType(document, schema, projectionOutput)
				if err != nil {
					return "", "", fmt.Errorf("%s/responses/%s/content/%s/schema: %w", path, status, mediaType, err)
				}
				bodyType = qualifyClientType(document, bodyType)
			} else if isTextMedia(mediaType) {
				bodyType = "string"
			} else if !isBinaryMedia(mediaType, schema) {
				bodyType, err = schemaType(document, schema, projectionOutput)
				if err != nil {
					return "", "", fmt.Errorf("%s/responses/%s/content/%s/schema: %w", path, status, mediaType, err)
				}
				bodyType = qualifyClientType(document, bodyType)
			}
			contentType := ""
			if len(mediaTypes) > 1 || !isJSONMediaType(mediaType) || !strings.EqualFold(mediaType, "application/json") {
				if strings.Contains(mediaType, "*") {
					contentType = "; readonly contentType: string"
				} else {
					contentType = "; readonly contentType: " + quoteTS(mediaType)
				}
			}
			values = append(values, "{ readonly status: "+statusType+contentType+headerField+"; readonly body: "+bodyType+" }")
			plan := "{ status: " + quoteTS(status) + ", contentType: " + quoteTS(mediaType)
			if schema != nil && !isBinaryMedia(mediaType, schema) {
				descriptor, descriptorErr := wireSchemaDescriptor(schema, projectionOutput)
				if descriptorErr != nil {
					return "", "", fmt.Errorf("%s/responses/%s/content/%s/schema: %w", path, status, mediaType, descriptorErr)
				}
				plan += ", schema: " + descriptor
			}
			if headers != "" {
				plan += ", headers: " + headers
			}
			plans = append(plans, plan+" }")
		}
	}
	return strings.Join(values, " | "), "[" + strings.Join(plans, ", ") + "]", nil
}

func inboundResponseHeaderValuesType(document *ir.Document, response map[string]any) (string, error) {
	headers, _ := response["headers"].(map[string]any)
	if len(headers) == 0 {
		return "", nil
	}
	fields := make([]string, 0, len(headers))
	for _, name := range sortedAnyKeys(headers) {
		header, _ := headers[name].(map[string]any)
		resolved, err := resolveComponentObject(document, header, "headers")
		if err != nil {
			return "", err
		}
		schema, _, err := responseHeaderSchema(document, resolved)
		if err != nil {
			return "", err
		}
		valueType, err := schemaType(document, schema, projectionOutput)
		if err != nil {
			return "", err
		}
		property, err := naming.Property(name)
		if err != nil {
			property = name
		}
		optional := "?"
		if boolValue(resolved, "required") {
			optional = ""
		}
		fields = append(fields, "readonly "+property+optional+": "+qualifyClientType(document, valueType))
	}
	return "Readonly<{ " + strings.Join(fields, "; ") + " }>", nil
}

func emitCallbacks(document *ir.Document, callbacks []callbackDefinition) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("import { collectInboundSecurityCandidates, decodeInboundBody, decodeInboundParameters, InboundRequestError, normalizeInboundMediaCodecs, requiresInboundAuthentication, responseFromHandler, type Authenticate, type InboundRequestContext, type InboundResponse, type InboundParameterDefinition, type InboundSchemas, type InboundSecuritySchemes } from \"./runtime.js\"\n")
	output.WriteString("import type { MediaCodec, WireSchemas } from \"../generated/runtime.js\"\n")
	if len(callbacks) > 0 {
		output.WriteString("import type * as Contract from \"../generated/types.js\"\n")
	}
	output.WriteString("\n")
	if err := emitInboundSchemas(&output, document); err != nil {
		return nil, err
	}
	if err := emitWireComponents(&output, document, "inputWireSchemas", projectionInput); err != nil {
		return nil, err
	}
	if err := emitWireComponents(&output, document, "outputSchemas", projectionOutput); err != nil {
		return nil, err
	}
	if err := emitInboundSecuritySchemes(&output, document); err != nil {
		return nil, err
	}
	for _, callback := range callbacks {
		fmt.Fprintf(&output, "/** Host-owned Callback endpoint for URL expression %s. No route is generated. */\n", quoteTS(callback.expression))
		fmt.Fprintf(&output, "export interface %sCallbackContext extends InboundRequestContext {\n", callback.typeName)
		if callback.hasBody {
			fmt.Fprintf(&output, "  readonly body: %s\n", callback.bodyType)
		} else {
			output.WriteString("  readonly body: undefined\n")
		}
		output.WriteString("}\n")
		fmt.Fprintf(&output, "export type %sCallbackResponse = %s\n", callback.typeName, callback.responseType)
	}
	for _, callback := range callbacks {
		security, err := json.Marshal(callback.security)
		if err != nil {
			return nil, fmt.Errorf("%s security metadata: %w", callback.name, err)
		}
		property, err := naming.Property(callback.typeName)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&output, "const %sCallbackDefinition = { operationID: %s, method: %s, parameters: %s satisfies readonly InboundParameterDefinition[], responses: %s, security: %s } as const\n", property, quoteTS(callback.operationID), quoteTS(callback.method), callback.parameters, callback.responsePlan, security)
	}
	output.WriteString("\n/** Application handlers keyed by generated Callback names. */\nexport interface CallbackHandlers {\n")
	for _, callback := range callbacks {
		property, err := naming.Property(callback.typeName)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&output, "  readonly %s?: (context: %sCallbackContext) => %sCallbackResponse | Promise<%sCallbackResponse>\n", property, callback.typeName, callback.typeName, callback.typeName)
	}
	output.WriteString("}\n\n/** Optional host authentication, media codecs, and host-bound path parameters for generated Callback endpoints. */\nexport interface CallbackHandlerOptions {\n  readonly authenticate?: Authenticate | undefined\n  readonly codecs?: Readonly<Record<string, MediaCodec<unknown>>> | undefined\n  readonly maxStreamItemBytes?: number | undefined\n  readonly pathParams?: Readonly<Record<string, Readonly<Record<string, string>>>> | undefined\n}\n\n/** Fetch-compatible endpoint for one host-mounted Callback route. */\nexport interface CallbackEndpoint {\n  fetch(request: Request): Promise<Response>\n}\n\n/** Named Callback endpoints ready for host route mounting. */\nexport interface CallbackEndpoints {\n")
	for _, callback := range callbacks {
		property, _ := naming.Property(callback.typeName)
		fmt.Fprintf(&output, "  readonly %s: CallbackEndpoint\n", property)
	}
	output.WriteString("}\n\n/**\n * Creates Fetch-native endpoints for dynamic OpenAPI Callback URLs.\n * The host chooses each concrete route and mounts the matching endpoint.\n */\nexport function createCallbackHandlers(handlers: CallbackHandlers, options: CallbackHandlerOptions = {}): CallbackEndpoints {\n  const inboundCodecs = normalizeInboundMediaCodecs(options.codecs)\n  return {\n")
	for _, callback := range callbacks {
		property, _ := naming.Property(callback.typeName)
		fmt.Fprintf(&output, "    %s: {\n      async fetch(request: Request): Promise<Response> {\n", property)
		fmt.Fprintf(&output, "        if (request.method !== %s) return new Response(\"Method Not Allowed\", { status: 405, headers: { allow: %s } })\n", quoteTS(callback.method), quoteTS(callback.method))
		fmt.Fprintf(&output, "        const handler = handlers.%s\n        if (handler === undefined) return new Response(\"Not Found\", { status: 404 })\n", property)
		fmt.Fprintf(&output, "        let params: Readonly<Record<string, unknown>>\n        try { params = await decodeInboundParameters(request, %sCallbackDefinition.parameters, inputSchemas, inputWireSchemas, inboundCodecs, options.pathParams?.%s) } catch (error) { if (error instanceof InboundRequestError) return error.response; throw error }\n        const context: InboundRequestContext = { request, operationID: %sCallbackDefinition.operationID, method: %sCallbackDefinition.method, path: new URL(request.url).pathname, params, security: %sCallbackDefinition.security, securityCandidates: collectInboundSecurityCandidates(request, %sCallbackDefinition.security, securitySchemes) }\n", property, property, property, property, property, property)
		output.WriteString("        if (requiresInboundAuthentication(context.security)) {\n          if (options.authenticate === undefined) return new Response(\"Unauthorized\", { status: 401 })\n          try { const denied = await options.authenticate(context); if (denied instanceof Response) return denied }\n          catch { return new Response(\"Internal Server Error\", { status: 500 }) }\n        }\n")
		if callback.hasBody {
			output.WriteString("        try {\n")
			fmt.Fprintf(&output, "          const body = await decodeInboundBody(request, { required: %t, plans: %s, schemas: inputSchemas, wireSchemas: inputWireSchemas, codecs: inboundCodecs, maxStreamItemBytes: options.maxStreamItemBytes }) as %s\n", callback.bodyRequired, callback.bodyPlans, callback.bodyType)
			fmt.Fprintf(&output, "          return await responseFromHandler(await handler({ ...context, body }), { schemas: outputSchemas, responses: %sCallbackDefinition.responses, codecs: inboundCodecs })\n", property)
			output.WriteString("        } catch (error) {\n          if (error instanceof InboundRequestError) return error.response\n          return new Response(\"Internal Server Error\", { status: 500 })\n        }\n")
		} else {
			output.WriteString("        try {\n")
			fmt.Fprintf(&output, "          return await responseFromHandler(await handler({ ...context, body: undefined }), { schemas: outputSchemas, responses: %sCallbackDefinition.responses, codecs: inboundCodecs })\n", property)
			output.WriteString("        } catch { return new Response(\"Internal Server Error\", { status: 500 }) }\n")
		}
		output.WriteString("      },\n    },\n")
	}
	output.WriteString("  }\n}\n")
	return output.Bytes(), nil
}

func collectWebhooks(document *ir.Document) ([]webhookDefinition, error) {
	values, _ := document.Raw["webhooks"].(map[string]any)
	names := sortedAnyKeys(values)
	result := make([]webhookDefinition, 0, len(names))
	for _, name := range names {
		item, ok := values[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be a Path Item Object", openAPIPointer("webhooks", name))
		}
		property, err := naming.Property(name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", openAPIPointer("webhooks", name), err)
		}
		typeName, err := naming.Public(name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", openAPIPointer("webhooks", name), err)
		}
		for _, method := range serverHTTPMethods {
			operation, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			operationPath := openAPIPointer("webhooks", name, method)
			parameters, err := inboundParameterDefinitions(document, item, operation, operationPath, true)
			if err != nil {
				return nil, err
			}
			operationID, _ := operation["operationId"].(string)
			if operationID == "" {
				operationID = name
			}
			body, err := inboundBodyType(document, operation, operationPath)
			if err != nil {
				return nil, err
			}
			responseType, responsePlan, err := inboundResponseDefinition(document, operation, operationPath)
			if err != nil {
				return nil, err
			}
			security := operation["security"]
			if security == nil {
				security = document.Raw["security"]
			}
			result = append(result, webhookDefinition{
				name: name, property: property, typeName: typeName, operationID: operationID,
				method: strings.ToUpper(method), bodyType: body.typeName, hasBody: body.hasBody, bodyRequired: body.required, bodyPlans: body.plans, parameters: parameters, responseType: responseType, responsePlan: responsePlan, security: security,
			})
		}
	}
	counts := make(map[string]int, len(result))
	for _, webhook := range result {
		counts[webhook.name]++
	}
	for index := range result {
		if counts[result[index].name] == 1 {
			continue
		}
		methodName, err := naming.Public(strings.ToLower(result[index].method))
		if err != nil {
			return nil, err
		}
		result[index].typeName += methodName
	}
	properties := make(map[string]string, len(result))
	types := make(map[string]string, len(result))
	for _, webhook := range result {
		if previous, exists := properties[webhook.property]; exists && previous != webhook.name {
			return nil, fmt.Errorf("%s and %s both generate Webhook handler name %s", openAPIPointer("webhooks", previous), openAPIPointer("webhooks", webhook.name), webhook.property)
		}
		if previous, exists := types[webhook.typeName]; exists && previous != webhook.name {
			return nil, fmt.Errorf("%s and %s both generate Webhook type %s", openAPIPointer("webhooks", previous), openAPIPointer("webhooks", webhook.name), webhook.typeName)
		}
		properties[webhook.property] = webhook.name
		types[webhook.typeName] = webhook.name
	}
	return result, nil
}

var serverHTTPMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace", "query"}

type inboundBodyDefinition struct {
	typeName string
	hasBody  bool
	required bool
	plans    string
}

func inboundBodyType(document *ir.Document, operation map[string]any, path string) (inboundBodyDefinition, error) {
	body, ok := operation["requestBody"].(map[string]any)
	if !ok {
		return inboundBodyDefinition{typeName: "undefined"}, nil
	}
	resolved, err := resolveComponentObject(document, body, "requestBodies")
	if err != nil {
		return inboundBodyDefinition{}, fmt.Errorf("%s/requestBody: %w", path, err)
	}
	content, _ := resolved["content"].(map[string]any)
	mediaTypes := sortedAnyKeys(content)
	required, _ := resolved["required"].(bool)
	if len(mediaTypes) == 0 {
		return inboundBodyDefinition{typeName: "undefined", hasBody: true, required: required, plans: "[]"}, nil
	}
	types := make([]string, 0, len(mediaTypes))
	plans := make([]string, 0, len(mediaTypes))
	for _, mediaType := range mediaTypes {
		media, _ := content[mediaType].(map[string]any)
		media, err = resolveMediaTypeObject(document, media)
		if err != nil {
			return inboundBodyDefinition{}, fmt.Errorf("%s/requestBody/content/%s: %w", path, mediaType, err)
		}
		valueType, plan, err := inboundBodyPlan(document, mediaType, media, path)
		if err != nil {
			return inboundBodyDefinition{}, err
		}
		if len(mediaTypes) > 1 {
			types = append(types, "{ readonly contentType: "+quoteTS(mediaType)+"; readonly value: "+valueType+" }")
		} else {
			types = append(types, valueType)
		}
		plans = append(plans, plan)
	}
	resultType := strings.Join(types, " | ")
	if !required {
		resultType += " | undefined"
	}
	return inboundBodyDefinition{typeName: resultType, hasBody: true, required: required, plans: "[" + strings.Join(plans, ", ") + "]"}, nil
}

func inboundBodyPlan(document *ir.Document, mediaType string, media map[string]any, path string) (string, string, error) {
	schemaValue := media["schema"]
	schema, _ := schemaValue.(map[string]any)
	if schema == nil {
		schema = map[string]any{}
	}
	stream := isStreamMediaType(mediaType) || media["itemSchema"] != nil
	value := "ArrayBuffer"
	if stream {
		itemSchema, exists := media["itemSchema"]
		if !exists {
			return "", "", fmt.Errorf("%s/requestBody/content/%s: sequential stream requires itemSchema", path, mediaType)
		}
		var err error
		value, err = schemaType(document, itemSchema, projectionInput)
		if err != nil {
			return "", "", fmt.Errorf("%s/requestBody/content/%s/itemSchema: %w", path, mediaType, err)
		}
		schemaValue = itemSchema
		schema, _ = itemSchema.(map[string]any)
		if schema == nil {
			schema = map[string]any{}
		}
	} else if !isBinaryMedia(mediaType, schema) {
		var err error
		value, err = schemaType(document, schemaValue, projectionInput)
		if err != nil {
			return "", "", fmt.Errorf("%s/requestBody/content/%s/schema: %w", path, mediaType, err)
		}
	}
	schemaSource, err := json.Marshal(inboundSchemaValue(schemaValue))
	if err != nil {
		return "", "", fmt.Errorf("%s/requestBody/content/%s/schema: encode validator schema: %w", path, mediaType, err)
	}
	wireSchema, err := wireSchemaDescriptor(schemaValue, projectionInput)
	if err != nil {
		return "", "", fmt.Errorf("%s/requestBody/content/%s/schema: %w", path, mediaType, err)
	}
	itemContentType := ""
	if itemEncoding, _ := media["itemEncoding"].(map[string]any); itemEncoding != nil {
		itemContentType, _ = itemEncoding["contentType"].(string)
	}
	if stream {
		value = "AsyncIterable<" + qualifyClientType(document, value) + ">"
	} else {
		value = qualifyClientType(document, value)
	}
	plan := "{ contentType: " + quoteTS(mediaType) + ", binary: " + fmt.Sprint(isBinaryMedia(mediaType, schema)) + ", stream: " + fmt.Sprint(stream) + ", itemContentType: " + quoteTS(itemContentType) + ", schema: " + string(schemaSource) + ", wireSchema: " + wireSchema + " }"
	encodings, err := requestBodyWireEncodings(document, media)
	if err != nil {
		return "", "", fmt.Errorf("%s/requestBody/content/%s/encoding: %w", path, mediaType, err)
	}
	if encodings != "" {
		plan = strings.TrimSuffix(plan, " }") + ", encoding: " + encodings + " }"
	}
	return value, plan, nil
}

// InboundSchema is object-shaped in generated source. Preserve boolean JSON
// Schema semantics through an explicit marker rather than turning `false` into
// an empty object and accidentally accepting every inbound value.
func inboundSchemaValue(value any) any {
	if boolean, ok := value.(bool); ok {
		return map[string]any{"x-sdkgen-boolean-schema": boolean}
	}
	return value
}

func isInboundRuntimeMediaType(mediaType string, schema map[string]any) bool {
	return isStreamMediaType(mediaType) || isJSONMediaType(mediaType) || isTextMedia(mediaType) || strings.Contains(strings.ToLower(mediaType), "xml") || strings.EqualFold(mediaType, "application/x-www-form-urlencoded") || strings.EqualFold(mediaType, "multipart/form-data") || isBinaryMedia(mediaType, schema)
}

func inboundParameterDefinitions(document *ir.Document, pathItem, operation map[string]any, path string, allowPath bool) (string, error) {
	parameters, err := operationParameters(document, ir.Operation{PathItemRaw: pathItem, Raw: operation})
	if err != nil {
		return "", fmt.Errorf("%s/parameters: %w", path, err)
	}
	entries := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		if parameter.Location == "path" && !allowPath {
			return "", fmt.Errorf("%s/parameters/%s: server add-on requires host route path binding for inbound path parameters", path, parameter.Name)
		}
		schema, err := json.Marshal(parameter.Schema)
		if err != nil {
			return "", fmt.Errorf("%s/parameters/%s: encode schema: %w", path, parameter.Name, err)
		}
		wireSchema, err := wireSchemaDescriptor(parameter.Schema, projectionInput)
		if err != nil {
			return "", fmt.Errorf("%s/parameters/%s: encode wire schema: %w", path, parameter.Name, err)
		}
		entry := "{ location: " + quoteTS(parameter.Location) + ", name: " + quoteTS(parameter.Name) + ", property: " + quoteTS(parameter.Property) + ", style: " + quoteTS(parameter.Style) + ", explode: " + fmt.Sprint(parameter.Explode) + ", allowReserved: " + fmt.Sprint(parameter.AllowReserved) + ", required: " + fmt.Sprint(parameter.Required) + ", schema: " + string(schema) + ", wireSchema: " + wireSchema
		if parameter.ContentType != "" {
			entry += ", contentType: " + quoteTS(parameter.ContentType)
		}
		entries = append(entries, entry+" }")
	}
	return "[" + strings.Join(entries, ", ") + "]", nil
}

func webhooksHaveBodies(webhooks []webhookDefinition) bool {
	for _, webhook := range webhooks {
		if webhook.hasBody {
			return true
		}
	}
	return false
}

func callbacksHaveBodies(callbacks []callbackDefinition) bool {
	for _, callback := range callbacks {
		if callback.hasBody {
			return true
		}
	}
	return false
}

func (definition webhookDefinition) bodyPlansOrEmpty() string {
	if definition.bodyPlans == "" {
		return "[]"
	}
	return definition.bodyPlans
}

func emitInboundSchemas(output *bytes.Buffer, document *ir.Document) error {
	schemas, err := json.Marshal(document.ComponentSchemas)
	if err != nil {
		return fmt.Errorf("encode inbound component schemas: %w", err)
	}
	fmt.Fprintf(output, "const inputSchemas: InboundSchemas = %s\n\n", schemas)
	return nil
}

func emitInboundSecuritySchemes(output *bytes.Buffer, document *ir.Document) error {
	components, _ := document.Raw["components"].(map[string]any)
	schemes, _ := components["securitySchemes"].(map[string]any)
	encoded, err := json.Marshal(schemes)
	if err != nil {
		return fmt.Errorf("encode inbound security schemes: %w", err)
	}
	fmt.Fprintf(output, "const securitySchemes: InboundSecuritySchemes = %s\n\n", encoded)
	return nil
}

func emitWebhooks(document *ir.Document, webhooks []webhookDefinition) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("import { collectInboundSecurityCandidates, decodeInboundBody, decodeInboundParameters, matchInboundRoute, InboundRequestError, normalizeInboundMediaCodecs, requiresInboundAuthentication, responseFromHandler, type Authenticate, type InboundRequestContext, type InboundResponse, type InboundParameterDefinition, type InboundSchemas, type InboundSecuritySchemes } from \"./runtime.js\"\n")
	output.WriteString("import type { MediaCodec, WireSchemas } from \"../generated/runtime.js\"\n")
	if len(webhooks) > 0 {
		output.WriteString("import type * as Contract from \"../generated/types.js\"\n")
	}
	output.WriteString("\n")
	if err := emitInboundSchemas(&output, document); err != nil {
		return nil, err
	}
	if err := emitWireComponents(&output, document, "inputWireSchemas", projectionInput); err != nil {
		return nil, err
	}
	if err := emitWireComponents(&output, document, "outputSchemas", projectionOutput); err != nil {
		return nil, err
	}
	if err := emitInboundSecuritySchemes(&output, document); err != nil {
		return nil, err
	}
	for _, webhook := range webhooks {
		fmt.Fprintf(&output, "export interface %sWebhookContext extends InboundRequestContext {\n", webhook.typeName)
		if webhook.hasBody {
			fmt.Fprintf(&output, "  readonly body: %s\n", webhook.bodyType)
		} else {
			output.WriteString("  readonly body: undefined\n")
		}
		output.WriteString("}\n")
		fmt.Fprintf(&output, "export type %sWebhookResponse = %s\n\n", webhook.typeName, webhook.responseType)
	}
	output.WriteString("/** Handlers keyed by each OpenAPI root Webhook Object name. */\n")
	output.WriteString("export interface WebhookHandlers {\n")
	for _, property := range webhookHandlerProperties(webhooks) {
		definitions := webhooksForProperty(webhooks, property)
		contexts := make([]string, 0, len(definitions))
		responses := make([]string, 0, len(definitions))
		for _, webhook := range definitions {
			contexts = append(contexts, webhook.typeName+"WebhookContext")
			responses = append(responses, webhook.typeName+"WebhookResponse")
		}
		contextType := strings.Join(contexts, " | ")
		responseType := strings.Join(responses, " | ")
		fmt.Fprintf(&output, "  readonly %s?: (context: %s) => %s | Promise<%s>\n", property, contextType, responseType, responseType)
	}
	output.WriteString("}\n\n")
	output.WriteString("/** Concrete host paths keyed by generated Webhook handler name. */\n")
	output.WriteString("export type WebhookRoutes = Readonly<Partial<Record<keyof WebhookHandlers, string>>>\n\n")
	output.WriteString("/** Options for a Fetch-native generated Webhook router. */\n")
	output.WriteString("export interface WebhookRouterOptions {\n  readonly routes: WebhookRoutes\n  readonly authenticate?: Authenticate | undefined\n  readonly codecs?: Readonly<Record<string, MediaCodec<unknown>>> | undefined\n  readonly maxStreamItemBytes?: number | undefined\n}\n\n")
	output.WriteString("/** Fetch-compatible generated inbound Webhook router. */\n")
	output.WriteString("export interface WebhookRouter {\n  fetch(request: Request): Promise<Response>\n}\n\n")
	for _, webhook := range webhooks {
		security, err := json.Marshal(webhook.security)
		if err != nil {
			return nil, fmt.Errorf("%s security metadata: %w", openAPIPointer("webhooks", webhook.name), err)
		}
		fmt.Fprintf(&output, "const %s = { operationID: %s, method: %s, parameters: %s satisfies readonly InboundParameterDefinition[], requestBodyPlans: %s, requestBodyRequired: %t, responses: %s, security: %s } as const\n", webhookDefinitionSymbol(webhook), quoteTS(webhook.operationID), quoteTS(webhook.method), webhook.parameters, webhook.bodyPlansOrEmpty(), webhook.bodyRequired, webhook.responsePlan, security)
	}
	if len(webhooks) > 0 {
		output.WriteString("\n")
	}
	output.WriteString("/**\n * Creates a Fetch-native router for the generated root Webhook Objects.\n * Webhook names are OpenAPI identifiers, so the host supplies their concrete paths.\n * Authentication policy stays in the host callback; generated code never verifies credentials.\n */\n")
	output.WriteString("export function createWebhookRouter(handlers: WebhookHandlers, options: WebhookRouterOptions): WebhookRouter {\n")
	output.WriteString("  const routes = options.routes\n  const inboundCodecs = normalizeInboundMediaCodecs(options.codecs)\n  const registrations = new Set<string>()\n")
	for _, webhook := range webhooks {
		fmt.Fprintf(&output, "  if (handlers.%s !== undefined) {\n", webhook.property)
		fmt.Fprintf(&output, "    const path = routes.%s\n", webhook.property)
		fmt.Fprintf(&output, "    if (typeof path !== \"string\" || !path.startsWith(\"/\") || path.includes(\"?\") || path.includes(\"#\")) throw new TypeError(%s)\n", quoteTS("Webhook route for "+webhook.name+" must be an absolute path without query or fragment"))
		fmt.Fprintf(&output, "    const key = %s + \" \" + path\n", quoteTS(webhook.method))
		fmt.Fprintf(&output, "    if (registrations.has(key)) throw new TypeError(%s + key)\n", quoteTS("Duplicate generated Webhook route: "))
		output.WriteString("    registrations.add(key)\n  }\n")
	}
	output.WriteString("  return {\n    async fetch(request: Request): Promise<Response> {\n      const pathname = new URL(request.url).pathname\n")
	for _, webhook := range webhooks {
		fmt.Fprintf(&output, "      const %sPathParameters = matchInboundRoute(routes.%s, pathname)\n      if (request.method === %s && %sPathParameters !== undefined) {\n", webhookDefinitionSymbol(webhook), webhook.property, quoteTS(webhook.method), webhookDefinitionSymbol(webhook))
		fmt.Fprintf(&output, "        const handler = handlers.%s\n", webhook.property)
		output.WriteString("        if (handler === undefined) return new Response(\"Not Found\", { status: 404 })\n")
		symbol := webhookDefinitionSymbol(webhook)
		fmt.Fprintf(&output, "        let params: Readonly<Record<string, unknown>>\n        try { params = await decodeInboundParameters(request, %s.parameters, inputSchemas, inputWireSchemas, inboundCodecs, %sPathParameters) } catch (error) { if (error instanceof InboundRequestError) return error.response; throw error }\n        const context: InboundRequestContext = { request, operationID: %s.operationID, method: %s.method, path: pathname, params, security: %s.security, securityCandidates: collectInboundSecurityCandidates(request, %s.security, securitySchemes) }\n", symbol, symbol, symbol, symbol, symbol, symbol)
		output.WriteString("        if (requiresInboundAuthentication(context.security)) {\n          if (options.authenticate === undefined) return new Response(\"Unauthorized\", { status: 401 })\n          try { const denied = await options.authenticate(context); if (denied instanceof Response) return denied }\n          catch { return new Response(\"Internal Server Error\", { status: 500 }) }\n        }\n")
		if webhook.hasBody {
			output.WriteString("        try {\n")
			fmt.Fprintf(&output, "          const body = await decodeInboundBody(request, { required: %s.requestBodyRequired, plans: %s.requestBodyPlans, schemas: inputSchemas, wireSchemas: inputWireSchemas, codecs: inboundCodecs, maxStreamItemBytes: options.maxStreamItemBytes }) as %s\n", symbol, symbol, webhook.bodyType)
			fmt.Fprintf(&output, "          return await responseFromHandler(await handler({ ...context, body }), { schemas: outputSchemas, responses: %s.responses, codecs: inboundCodecs })\n", symbol)
			output.WriteString("        } catch (error) {\n          if (error instanceof InboundRequestError) return error.response\n          return new Response(\"Internal Server Error\", { status: 500 })\n        }\n")
		} else {
			output.WriteString("        try {\n")
			fmt.Fprintf(&output, "          return await responseFromHandler(await handler({ ...context, body: undefined }), { schemas: outputSchemas, responses: %s.responses, codecs: inboundCodecs })\n", symbol)
			output.WriteString("        } catch { return new Response(\"Internal Server Error\", { status: 500 }) }\n")
		}
		output.WriteString("      }\n")
	}
	output.WriteString("      return new Response(\"Not Found\", { status: 404 })\n    },\n  }\n}\n")
	return output.Bytes(), nil
}

func webhookDefinitionSymbol(webhook webhookDefinition) string {
	property, err := naming.Property(webhook.typeName)
	if err != nil {
		return webhook.property
	}
	return property + "Webhook"
}

func webhookHandlerProperties(webhooks []webhookDefinition) []string {
	set := make(map[string]bool, len(webhooks))
	for _, webhook := range webhooks {
		set[webhook.property] = true
	}
	properties := make([]string, 0, len(set))
	for property := range set {
		properties = append(properties, property)
	}
	sort.Strings(properties)
	return properties
}

func webhooksForProperty(webhooks []webhookDefinition, property string) []webhookDefinition {
	result := make([]webhookDefinition, 0)
	for _, webhook := range webhooks {
		if webhook.property == property {
			result = append(result, webhook)
		}
	}
	return result
}

func serverRuntimeSource() []byte {
	return []byte(`import { decodeWireValue, decodeXML, encodeWireValue, encodeXML, validateWireValue, type MediaCodec, type MediaStreamReader, type WireEncodingDefinition, type WireHeaderDefinition, type WireSchema, type WireSchemas } from "../generated/runtime.js"

/** Metadata provided to host-owned inbound authentication policy. */
export interface InboundRequestContext {
  readonly request: Request
  readonly operationID: string
  readonly method: string
  readonly path: string
  /** Decoded query, header, and cookie parameters keyed by generated property name. */
  readonly params: Readonly<Record<string, unknown>>
  readonly security: unknown
  readonly securityCandidates: Readonly<Record<string, InboundSecurityCandidate>>
}

/** Raw candidate derived from one declared inbound security scheme. */
export interface InboundSecurityCandidate {
  readonly scheme: string
  readonly type: string
  readonly location?: "header" | "query" | "cookie"
  readonly name?: string
  readonly value?: string
}

/** Lossless Security Scheme Object map used only to collect request candidates. */
export type InboundSecuritySchemes = Readonly<Record<string, Readonly<Record<string, unknown>>>>

/** One generated inbound query, header, or cookie parameter. */
export interface InboundParameterDefinition {
  readonly location: "path" | "query" | "querystring" | "header" | "cookie"
  readonly name: string
  readonly property: string
  readonly style: string
  readonly explode: boolean
  readonly allowReserved: boolean
  readonly required: boolean
  readonly contentType?: string | undefined
  readonly schema: InboundSchema
  /** Full generated input schema: validates wire names then maps them to TS names. */
  readonly wireSchema: WireSchema
}

/** Decodes declared inbound parameters before host authentication and handler execution. */
export async function decodeInboundParameters(request: Request, definitions: readonly InboundParameterDefinition[], schemas: InboundSchemas, wireSchemas: WireSchemas, codecs: ReadonlyMap<string, MediaCodec<unknown>> | undefined = undefined, pathParameters: Readonly<Record<string, string>> = {}): Promise<Readonly<Record<string, unknown>>> {
  const result: Record<string, unknown> = {}
  const url = new URL(request.url)
  const cookies = parseInboundCookies(request.headers.get("cookie"))
  for (const definition of definitions) {
    let raw: unknown = definition.location === "path" ? pathParameters[definition.name] : definition.location === "header" ? request.headers.get(definition.name) : definition.location === "cookie" ? (definition.style === "cookie" ? cookies.raw[definition.name] : cookies.decoded[definition.name]) : definition.location === "querystring" ? url.search.slice(1) : url.searchParams.getAll(definition.name)
    if (definition.contentType === undefined && definition.location === "query" && (definition.style === "deepObject" || definition.style === "form" && definition.explode) && isRecord(resolveInboundSchema(definition.schema, schemas)["properties"])) raw = decodeInboundQueryObject(url, definition, schemas)
    if (definition.contentType === undefined && definition.location === "cookie" && definition.style === "cookie" && definition.explode && isRecord(resolveInboundSchema(definition.schema, schemas)["properties"])) raw = decodeInboundCookieObject(cookies.raw, definition, schemas)
    const absent = Array.isArray(raw) ? raw.length === 0 : isRecord(raw) ? Object.keys(raw).length === 0 : raw === undefined || raw === null
    if (absent) {
      if (definition.required) throw new InboundRequestError(new Response("Missing required parameter " + definition.name, { status: 400 }))
      continue
    }
    const value = await decodeInboundParameterContent(raw, definition, schemas, wireSchemas, codecs)
    try {
      validateWireValue(value, definition.wireSchema, wireSchemas, "decode")
      result[definition.property] = decodeWireValue(value, definition.wireSchema, wireSchemas)
    } catch (error) {
      throw new InboundRequestError(new Response("Invalid parameter " + definition.name + ": " + (error instanceof Error ? error.message : "invalid value"), { status: 400 }))
    }
  }
  return result
}

async function decodeInboundParameterContent(raw: unknown, definition: InboundParameterDefinition, schemas: InboundSchemas, wireSchemas: WireSchemas, codecs: ReadonlyMap<string, MediaCodec<unknown>> | undefined): Promise<unknown> {
  if (isRecord(raw)) return raw
  if (typeof raw !== "string" && !Array.isArray(raw)) return raw
  if (typeof raw === "string" && definition.location === "path") {
    if (definition.style === "label" && raw.startsWith(".")) raw = raw.slice(1)
    if (definition.style === "matrix" && raw.startsWith(";")) {
      const prefix = ";" + definition.name + "="
      if (raw.startsWith(prefix)) raw = raw.slice(prefix.length)
    }
  }
  const contentType = normalizeInboundMediaType(definition.contentType ?? "")
  const source = Array.isArray(raw) ? raw[0] : raw
  if ((contentType === "application/json" || contentType.endsWith("+json")) && source !== undefined) {
    try { return JSON.parse(source) } catch { throw new InboundRequestError(new Response("Invalid JSON parameter " + definition.name, { status: 400 })) }
  }
  if (contentType.includes("xml") && source !== undefined) return decodeXML(source, definition.wireSchema, wireSchemas)
  if (contentType === "application/x-www-form-urlencoded" && source !== undefined) return decodeInboundParameterForm(source, definition.schema, schemas)
  if (contentType !== "" && !contentType.startsWith("text/") && !isInboundBinaryMedia(contentType, definition.schema)) {
    const codec = inboundMediaCodec(codecs, contentType)
    if (codec?.decodeParameter === undefined) throw new InboundRequestError(new Response("Unsupported Media Type", { status: 415 }))
    return codec.decodeParameter(source ?? "", { contentType })
  }
  const schema = resolveInboundSchema(definition.schema, schemas)
  if (isRecord(schema["properties"])) return decodeInboundSerializedObject(source, definition, schema["properties"])
  return decodeInboundParameterValue(raw, definition.schema)
}

async function decodeInboundParameterForm(source: string, schema: InboundSchema, schemas: InboundSchemas): Promise<unknown> {
  const parsed = new URLSearchParams(source)
  const value: Record<string, unknown> = {}
  for (const [name, entry] of parsed) {
    const previous = value[name]
    value[name] = previous === undefined ? entry : Array.isArray(previous) ? [...previous, entry] : [previous, entry]
  }
  return decodeInboundFormValue(value, schema, schemas)
}

/** Reconstructs an object encoded with OpenAPI simple, label, matrix, or form parameter styles. */
function decodeInboundSerializedObject(source: string | undefined, definition: InboundParameterDefinition, properties: Readonly<Record<string, unknown>>): Readonly<Record<string, unknown>> {
  if (source === undefined) return {}
  let pairs: readonly (readonly [string, string])[]
  if (definition.style === "matrix") {
    if (definition.explode) pairs = source.split(";").filter(Boolean).flatMap((entry) => splitInboundParameterPair(entry))
    else pairs = splitInboundParameterTokens(source.startsWith(";" + definition.name + "=") ? source.slice(definition.name.length + 2) : source)
  } else if (definition.style === "label") {
    const value = source.startsWith(".") ? source.slice(1) : source
    pairs = definition.explode ? value.split(".").flatMap((entry) => splitInboundParameterPair(entry)) : splitInboundParameterTokens(value)
  } else {
    pairs = definition.explode ? source.split(",").flatMap((entry) => splitInboundParameterPair(entry)) : splitInboundParameterTokens(source)
  }
  const result: Record<string, unknown> = {}
  for (const [name, value] of pairs) {
    const property = properties[name]
    result[name] = isInboundSchema(property) ? decodeInboundParameterValue(value, property) : value
  }
  return result
}

function splitInboundParameterPair(value: string): readonly (readonly [string, string])[] {
  const separator = value.indexOf("=")
  return separator < 0 ? [] : [[value.slice(0, separator), value.slice(separator + 1)]]
}

function splitInboundParameterTokens(value: string): readonly (readonly [string, string])[] {
  const tokens = value.split(",")
  const pairs: [string, string][] = []
  for (let index = 0; index + 1 < tokens.length; index += 2) pairs.push([tokens[index]!, tokens[index + 1]!])
  return pairs
}

function decodeInboundQueryObject(url: URL, definition: InboundParameterDefinition, schemas: InboundSchemas): Readonly<Record<string, unknown>> {
  const schema = resolveInboundSchema(definition.schema, schemas)
  const properties = isRecord(schema["properties"]) ? schema["properties"] : {}
  const result: Record<string, unknown> = {}
  if (definition.style === "deepObject") {
    const prefix = definition.name + "["
    for (const [name, value] of url.searchParams) {
      if (!name.startsWith(prefix) || !name.endsWith("]")) continue
      const property = name.slice(prefix.length, -1)
      const propertySchema = properties[property]
      result[property] = isInboundSchema(propertySchema) ? decodeInboundParameterValue(value, propertySchema) : value
    }
    return result
  }
  for (const [property, propertySchema] of Object.entries(properties)) {
    const values = url.searchParams.getAll(property)
    if (values.length === 0) continue
    result[property] = isInboundSchema(propertySchema) ? decodeInboundParameterValue(values, propertySchema) : values[0]
  }
  return result
}

/** Reconstructs an OpenAPI 3.2 cookie-style exploded object without URI decoding cookie text. */
function decodeInboundCookieObject(cookies: Readonly<Record<string, string | readonly string[]>>, definition: InboundParameterDefinition, schemas: InboundSchemas): Readonly<Record<string, unknown>> {
  const schema = resolveInboundSchema(definition.schema, schemas)
  const properties = isRecord(schema["properties"]) ? schema["properties"] : {}
  const result: Record<string, unknown> = {}
  for (const [property, propertySchema] of Object.entries(properties)) {
    const value = cookies[property]
    if (value === undefined) continue
    result[property] = isInboundSchema(propertySchema) ? decodeInboundParameterValue(value, propertySchema) : value
  }
  return result
}

/** Matches a host webhook route template and returns decoded parameter values. */
export function matchInboundRoute(template: string | undefined, pathname: string): Readonly<Record<string, string>> | undefined {
  if (template === undefined || !template.startsWith("/")) return undefined
  const expected = template.split("/").slice(1)
  const actual = pathname.split("/").slice(1)
  if (expected.length !== actual.length) return undefined
  const result: Record<string, string> = {}
  for (let index = 0; index < expected.length; index++) {
    const segment = expected[index]!
    const value = actual[index]!
    const match = /^\{([^{}\/]+)\}$/.exec(segment)
    if (match === null) { if (segment !== value) return undefined; continue }
    try { result[match[1]!] = decodeURIComponent(value) }
    catch { return undefined }
  }
  return result
}

function decodeInboundParameterValue(raw: string | readonly string[], schema: InboundSchema): unknown {
  const values = Array.isArray(raw) ? raw : [raw]
  const types = Array.isArray(schema["type"]) ? schema["type"] : [schema["type"]]
  if (types.includes("array")) {
    const item = isInboundSchema(schema["items"]) ? schema["items"] : {}
    return values.flatMap((value) => value.split(",")).map((value) => decodeInboundParameterValue(value, item))
  }
  const value = values[0]!
  if (types.includes("integer")) { const number = Number(value); return Number.isInteger(number) ? number : value }
  if (types.includes("number")) { const number = Number(value); return Number.isFinite(number) ? number : value }
  if (types.includes("boolean")) return value === "true" ? true : value === "false" ? false : value
  return value
}

/** Coerces form field strings using the wire schema before inbound validation. */
async function decodeInboundFormValue(value: unknown, schema: InboundSchema | undefined, schemas: InboundSchemas, encoding: readonly WireEncodingDefinition[] | undefined = undefined, contentType: string | undefined = undefined, codecs: ReadonlyMap<string, MediaCodec<unknown>> | undefined = undefined): Promise<unknown> {
  if (schema === undefined) return value
  const resolved = resolveInboundSchema(schema, schemas)
  if (value instanceof Blob) {
    const normalized = normalizeInboundMediaType(contentType ?? value.type)
    if (normalized === "application/json" || normalized.endsWith("+json") || normalized.includes("xml") || codecs?.has(normalized) === true) return decodeInboundFormContent(await value.text(), resolved, schemas, normalized, codecs)
    return value
  }
  if (value instanceof ArrayBuffer || ArrayBuffer.isView(value)) return value
  if (typeof value === "string" && contentType !== undefined) {
    const decoded = await decodeInboundFormContent(value, resolved, schemas, contentType, codecs)
	if (decoded !== value) return decodeInboundFormValue(decoded, resolved, schemas, encoding, undefined, codecs)
  }
  if (Array.isArray(value)) {
    const item = isInboundSchema(resolved["items"]) ? resolved["items"] : undefined
    if (item !== undefined) return Promise.all(value.flatMap((entry) => Array.isArray(entry) ? entry : [entry]).map((entry) => decodeInboundFormValue(entry, item, schemas, encoding, contentType, codecs)))
    return value
  }
  if (isRecord(value)) {
    const properties = isRecord(resolved["properties"]) ? resolved["properties"] : {}
    const result: Record<string, unknown> = {}
    for (const [name, entry] of Object.entries(value)) {
      const property = properties[name]
      const definition = encoding?.find((candidate) => candidate.name === name)
      result[name] = isInboundSchema(property) ? await decodeInboundFormValue(entry, property, schemas, definition?.encoding, definition?.contentType, codecs) : entry
    }
    return result
  }
  if (typeof value === "string") return decodeInboundParameterValue(value, resolved)
  return value
}

async function decodeInboundFormContent(value: unknown, schema: InboundSchema, schemas: InboundSchemas, contentType: string | undefined, codecs: ReadonlyMap<string, MediaCodec<unknown>> | undefined): Promise<unknown> {
  if (typeof value !== "string" || contentType === undefined) return value
  const normalized = normalizeInboundMediaType(contentType)
  if (normalized === "application/json" || normalized.endsWith("+json")) {
    try { return JSON.parse(value) } catch { throw new InboundRequestError(new Response("Invalid form JSON field", { status: 400 })) }
  }
  if (normalized.includes("xml")) {
    try { return decodeInboundXML(value, schema, schemas) } catch { throw new InboundRequestError(new Response("Invalid form XML field", { status: 400 })) }
  }
	const codec = codecs?.get(normalized)
	if (codec?.decodeParameter === undefined) return value
	try { return await codec.decodeParameter(value, { contentType }) } catch { throw new InboundRequestError(new Response("Invalid form field", { status: 400 })) }
}

/** Return void to continue or a Response to reject the inbound request. */
export type Authenticate = (context: InboundRequestContext) => void | Response | Promise<void | Response>

/** Whether a non-empty effective OpenAPI Security Requirement Object applies. */
export function requiresInboundAuthentication(security: unknown): boolean {
  return Array.isArray(security) && security.length > 0 && !security.some((alternative) => isRecord(alternative) && Object.keys(alternative).length === 0)
}

/** Collects declared header/query/cookie credential candidates without authenticating them. */
export function collectInboundSecurityCandidates(request: Request, security: unknown, schemes: InboundSecuritySchemes): Readonly<Record<string, InboundSecurityCandidate>> {
  const result: Record<string, InboundSecurityCandidate> = {}
  if (!Array.isArray(security)) return result
  const url = new URL(request.url)
  const cookies = parseInboundCookies(request.headers.get("cookie"))
  for (const alternative of security) {
    if (!isRecord(alternative)) continue
    for (const name of Object.keys(alternative)) {
      if (result[name] !== undefined) continue
      const scheme = schemes[name]
      if (scheme === undefined || typeof scheme.type !== "string") continue
      if (scheme.type === "apiKey") {
        const location = scheme.in === "header" || scheme.in === "query" || scheme.in === "cookie" ? scheme.in : undefined
        const parameterName = typeof scheme.name === "string" ? scheme.name : undefined
        if (location === undefined || parameterName === undefined) continue
        const value = location === "header" ? request.headers.get(parameterName) ?? undefined : location === "query" ? url.searchParams.get(parameterName) ?? undefined : inboundCookieFirst(cookies.decoded[parameterName])
        result[name] = { scheme: name, type: "apiKey", location, name: parameterName, ...(value === undefined ? {} : { value }) }
        continue
      }
      const authorization = request.headers.get("authorization") ?? undefined
      result[name] = { scheme: name, type: scheme.type, ...(authorization === undefined ? {} : { value: authorization }) }
    }
  }
  return result
}

interface InboundCookies { readonly raw: Readonly<Record<string, string | readonly string[]>>; readonly decoded: Readonly<Record<string, string | readonly string[]>> }

function parseInboundCookies(header: string | null): InboundCookies {
  const raw: Record<string, string | string[]> = {}
  const decoded: Record<string, string | string[]> = {}
  if (header === null) return { raw, decoded }
  for (const item of header.split(";")) {
    const index = item.indexOf("=")
    if (index < 0) continue
    const name = item.slice(0, index).trim()
    if (name === "") continue
    const value = item.slice(index + 1).trim()
    appendInboundCookie(raw, name, value)
    try { appendInboundCookie(decoded, decodeURIComponent(name), decodeURIComponent(value)) }
    catch { appendInboundCookie(decoded, name, value) }
  }
  return { raw, decoded }
}

function appendInboundCookie(target: Record<string, string | string[]>, name: string, value: string): void {
  const previous = target[name]
  target[name] = previous === undefined ? value : Array.isArray(previous) ? [...previous, value] : [previous, value]
}

function inboundCookieFirst(value: string | readonly string[] | undefined): string | undefined {
  return Array.isArray(value) ? value[0] : value
}

/** Framework-neutral response produced by an inbound generated handler. */
export interface InboundResponse {
  readonly status: number
  readonly contentType?: string | undefined
  readonly headers?: HeadersInit | undefined
  /** Typed values keyed by generated response-header property names. */
  readonly headerValues?: Readonly<Record<string, unknown>> | undefined
  readonly body?: unknown
}

/** One generated response representation accepted by an inbound handler. */
export interface InboundResponseDefinition {
  /** Exact status code, status range (for example 2XX), or default. */
  readonly status: string
  /** Exact generated response media type, when the response has a body. */
  readonly contentType?: string | undefined
  /** Output-projected wire schema used to validate and encode the body. */
  readonly schema?: WireSchema | undefined
  /** Declared response headers validated before the response is returned. */
  readonly headers?: readonly WireHeaderDefinition[] | undefined
}

/** Generated response plans and component schemas for one inbound endpoint. */
export interface InboundResponseOptions {
  readonly schemas: WireSchemas
  readonly responses: readonly InboundResponseDefinition[]
  readonly codecs?: ReadonlyMap<string, MediaCodec<unknown>> | undefined
}

/** Decoding failure whose response is safe for the generated router to return. */
export class InboundRequestError extends Error {
  readonly response: Response
  constructor(response: Response) {
    super("Inbound request could not be decoded")
    this.response = response
  }
}

/** JSON Schema fragments used by generated inbound request validation. */
export type InboundSchema = Readonly<Record<string, unknown>>
/** Component schemas used to resolve local inbound $ref values. */
export type InboundSchemas = Readonly<Record<string, InboundSchema>>
/** One declared media representation selected from an inbound request body. */
export interface InboundBodyPlan {
  readonly contentType: string
  readonly binary: boolean
  readonly stream: boolean
  readonly itemContentType?: string | undefined
  readonly schema?: InboundSchema | undefined
  readonly wireSchema?: WireSchema | undefined
  readonly encoding?: readonly WireEncodingDefinition[] | undefined
}
/** Body contract selected from an OpenAPI Request Body Object. */
export interface InboundBodyOptions {
  readonly required: boolean
  readonly plans: readonly InboundBodyPlan[]
  readonly schemas: InboundSchemas
  /** Generated wire-name mapping for the decoded body or stream item. */
  readonly wireSchema?: WireSchema | undefined
  /** Generated component mappings used by wireSchema. */
  readonly wireSchemas?: WireSchemas | undefined
  /** Host codecs for declared custom inbound media types. */
  readonly codecs?: ReadonlyMap<string, MediaCodec<unknown>> | undefined
  /** Maximum byte count a custom inbound stream codec may request in one read. */
  readonly maxStreamItemBytes?: number | undefined
}

/** Decodes and validates one declared JSON, text, form, or XML request body. */
export async function decodeInboundBody(request: Request, options: InboundBodyOptions): Promise<unknown> {
  const rawContentType = request.headers.get("content-type")
  const contentType = rawContentType?.split(";", 1)[0]?.trim().toLowerCase()
  if (contentType === undefined && request.body === null && !options.required) return undefined
  const plan = contentType === undefined ? undefined : selectInboundBodyPlan(options.plans, contentType)
  if (plan === undefined || contentType === undefined) {
    throw new InboundRequestError(new Response("Unsupported Media Type", { status: 415 }))
  }
  const value = await decodeSelectedInboundBody(request, rawContentType ?? contentType, contentType, { ...options, ...plan })
  return options.plans.length === 1 || value === undefined ? value : { contentType: plan.contentType, value }
}

function selectInboundBodyPlan(plans: readonly InboundBodyPlan[], contentType: string): InboundBodyPlan | undefined {
  return plans.filter((plan) => inboundMediaTypeMatches(plan.contentType, contentType)).sort((left, right) => inboundMediaTypeMatchScore(right.contentType, contentType) - inboundMediaTypeMatchScore(left.contentType, contentType))[0]
}

function inboundMediaTypeMatchScore(pattern: string, actual: string): number {
  const normalized = normalizeInboundMediaType(pattern)
  if (normalized === normalizeInboundMediaType(actual)) return 3
  if (normalized.includes("*+")) return 2
  if (normalized.includes("*")) return 1
  return 0
}

async function decodeSelectedInboundBody(request: Request, rawContentType: string, contentType: string, options: InboundBodyOptions & InboundBodyPlan): Promise<unknown> {
  let value: unknown
  if (options.binary === true) {
    const bytes = await request.arrayBuffer()
    if (bytes.byteLength === 0 && options.required) throw new InboundRequestError(new Response("Request body is required", { status: 400 }))
    return bytes
  }
  if (options.stream === true) {
    if (request.body === null) {
      if (options.required) throw new InboundRequestError(new Response("Request body is required", { status: 400 }))
      return emptyInboundStream()
    }
    if (!isGeneratedInboundStreamMediaType(contentType)) {
      return decodeInboundCustomStream(request.body, contentType, options)
    }
    return decodeInboundStream(request.body, rawContentType, options.schema, options.schemas, options.required, options.itemContentType, options.wireSchema, options.wireSchemas, resolveInboundStreamItemBytes(options.maxStreamItemBytes))
  }
  if (!isGeneratedInboundMediaType(contentType, options.schema)) {
    const codec = inboundMediaCodec(options.codecs, contentType)
    if (codec?.decodeInbound === undefined) throw new InboundRequestError(new Response("Unsupported Media Type", { status: 415 }))
    try { value = await codec.decodeInbound(request, { contentType }) }
    catch { throw new InboundRequestError(new Response("Invalid request body", { status: 400 })) }
  } else if (contentType === "multipart/form-data") {
    let form: FormData
    try { form = await request.formData() } catch { throw new InboundRequestError(new Response("Invalid multipart form", { status: 400 })) }
    if ([...form.keys()].length === 0) {
      if (options.required) throw new InboundRequestError(new Response("Request body is required", { status: 400 }))
      return undefined
    }
    const result: Record<string, unknown> = {}
    for (const [name, item] of form) {
      const previous = result[name]
      result[name] = previous === undefined ? item : Array.isArray(previous) ? [...previous, item] : [previous, item]
    }
    value = await decodeInboundFormValue(result, options.schema, options.schemas, options.encoding, undefined, options.codecs)
  } else {
    const text = await request.text()
    if (text.trim() === "") {
      if (options.required) throw new InboundRequestError(new Response("Request body is required", { status: 400 }))
      return undefined
    }
    if (contentType === "application/json" || contentType.endsWith("+json")) {
    try { value = JSON.parse(text) } catch { throw new InboundRequestError(new Response("Invalid JSON", { status: 400 })) }
    } else if (contentType === "application/x-www-form-urlencoded") {
    const form = new URLSearchParams(text)
    const result: Record<string, unknown> = {}
    for (const [name, item] of form) {
      const previous = result[name]
      result[name] = previous === undefined ? item : Array.isArray(previous) ? [...previous, item] : [previous, item]
    }
      value = await decodeInboundFormValue(result, options.schema, options.schemas, options.encoding, undefined, options.codecs)
    } else if (contentType.includes("xml")) {
    try { value = decodeInboundXML(text, options.schema, options.schemas) }
    catch (cause) { throw new InboundRequestError(new Response("Invalid XML", { status: 400 })) }
    } else value = text
  }
  if (options.schema !== undefined) {
    const error = validateInboundValue(value, options.schema, options.schemas, "body")
    if (error !== undefined) throw new InboundRequestError(new Response("Invalid request body: " + error, { status: 400 }))
  }
  return decodeInboundWireValue(value, options.wireSchema, options.wireSchemas)
}

function decodeInboundWireValue(value: unknown, schema: WireSchema | undefined, schemas: WireSchemas | undefined): unknown {
  if (schema === undefined || value === undefined) return value
  try { return decodeWireValue(value, schema, schemas ?? {}) }
  catch { throw new InboundRequestError(new Response("Invalid request body", { status: 400 })) }
}

function isGeneratedInboundMediaType(contentType: string, schema: InboundSchema | undefined): boolean {
  return contentType === "application/json" || contentType.endsWith("+json") || contentType.startsWith("text/") || contentType.includes("xml") || contentType === "application/x-www-form-urlencoded" || contentType === "multipart/form-data" || isInboundBinaryMedia(contentType, schema)
}

function isGeneratedInboundStreamMediaType(contentType: string): boolean {
  return contentType.startsWith("multipart/") || contentType.includes("ndjson") || contentType.includes("jsonl") || contentType.includes("json-seq") || contentType.includes("event-stream")
}

export function normalizeInboundMediaCodecs(codecs: Readonly<Record<string, MediaCodec<unknown>>> | undefined): ReadonlyMap<string, MediaCodec<unknown>> {
  const result = new Map<string, MediaCodec<unknown>>()
  for (const [mediaType, codec] of Object.entries(codecs ?? {})) {
    const normalized = normalizeInboundMediaType(mediaType)
    if (normalized === "" || normalized.includes("/ ") || !normalized.includes("/")) throw new TypeError("invalid inbound codec media type " + mediaType)
    if (result.has(normalized)) throw new TypeError("duplicate inbound codec media type " + mediaType)
    result.set(normalized, codec)
  }
  return result
}

function inboundMediaCodec(codecs: ReadonlyMap<string, MediaCodec<unknown>> | undefined, contentType: string): MediaCodec<unknown> | undefined {
  if (codecs === undefined) return undefined
  return codecs.get(normalizeInboundMediaType(contentType))
}

function resolveInboundStreamItemBytes(value: number | undefined): number {
  const resolved = value ?? 1024 * 1024
  if (!Number.isSafeInteger(resolved) || resolved <= 0) throw new TypeError("maxStreamItemBytes must be a positive safe integer")
  return resolved
}

async function* decodeInboundCustomStream(body: ReadableStream<Uint8Array>, contentType: string, options: InboundBodyOptions & InboundBodyPlan): AsyncIterable<unknown> {
  const codec = inboundMediaCodec(options.codecs, contentType)
  if (codec?.decodeInboundStream === undefined) throw new InboundRequestError(new Response("Unsupported Media Type", { status: 415 }))
  const maxFrameBytes = resolveInboundStreamItemBytes(options.maxStreamItemBytes)
  const reader = createInboundMediaStreamReader(body, maxFrameBytes)
  let count = 0
  try {
    for await (const value of codec.decodeInboundStream(reader, { contentType, maxFrameBytes })) {
      if (options.schema !== undefined) {
        const error = validateInboundValue(value, options.schema, options.schemas, "stream item")
        if (error !== undefined) throw new InboundRequestError(new Response("Invalid stream item: " + error, { status: 400 }))
      }
      count++
      yield decodeInboundWireValue(value, options.wireSchema, options.wireSchemas)
    }
    if (options.required && count === 0) throw new InboundRequestError(new Response("Request body is required", { status: 400 }))
  } catch (error) {
    if (error instanceof InboundRequestError) throw error
    throw new InboundRequestError(new Response("Invalid stream item", { status: 400 }))
  } finally { await reader.cancel() }
}

function createInboundMediaStreamReader(body: ReadableStream<Uint8Array>, maximum: number): MediaStreamReader {
  const reader = body.getReader()
  let pending = new Uint8Array()
  let done = false
  let cancelled = false
  const cancel = async (reason?: unknown): Promise<void> => {
    if (cancelled) return
    cancelled = true
    try { await reader.cancel(reason) } finally { reader.releaseLock() }
  }
  return {
    async read(maxBytes: number): Promise<Uint8Array | null> {
      if (!Number.isSafeInteger(maxBytes) || maxBytes <= 0 || maxBytes > maximum) throw new TypeError("custom stream read exceeds maxStreamItemBytes")
      while (pending.byteLength === 0 && !done) {
        const next = await reader.read()
        done = next.done
        if (next.value !== undefined) pending = next.value
      }
      if (pending.byteLength === 0) { await cancel(); return null }
      const value = pending.slice(0, maxBytes)
      pending = pending.slice(value.byteLength)
      return value
    },
    cancel,
  }
}

async function* emptyInboundStream(): AsyncIterable<unknown> { return }

async function* decodeInboundStream(body: ReadableStream<Uint8Array>, contentType: string, schema: InboundSchema | undefined, schemas: InboundSchemas, required: boolean, itemContentType: string | undefined, wireSchema: WireSchema | undefined, wireSchemas: WireSchemas | undefined, maxFrameBytes: number): AsyncIterable<unknown> {
  if (contentType.toLowerCase().startsWith("multipart/")) {
    yield* decodeInboundMultipartStream(body, contentType, schema, schemas, required, itemContentType, wireSchema, wireSchemas, maxFrameBytes)
    return
  }
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let pending = ""
  let count = 0
  const emit = (source: string): unknown => {
    let value: unknown
    try { value = JSON.parse(source) } catch { throw new InboundRequestError(new Response("Invalid stream item", { status: 400 })) }
    if (schema !== undefined) {
      const error = validateInboundValue(value, schema, schemas, "stream item")
      if (error !== undefined) throw new InboundRequestError(new Response("Invalid stream item: " + error, { status: 400 }))
    }
    count++
    return decodeInboundWireValue(value, wireSchema, wireSchemas)
  }
  try {
    while (true) {
      const next = await reader.read()
      pending += decoder.decode(next.value, { stream: !next.done })
      if (contentType.includes("event-stream")) {
        let boundary: number
        while ((boundary = pending.search(/\r?\n\r?\n/)) >= 0) {
          const event = pending.slice(0, boundary)
          pending = pending.slice(boundary).replace(/^\r?\n\r?\n/, "")
          const data = event.split(/\r?\n/).filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trimStart()).join("\n")
          if (data !== "") yield emit(data)
        }
      } else if (contentType.includes("json-seq")) {
        const records = pending.split("\u001e")
        pending = records.pop() ?? ""
        for (const record of records) if (record.trim() !== "") yield emit(record.trim())
      } else {
        let newline: number
        while ((newline = pending.indexOf("\n")) >= 0) {
          const line = pending.slice(0, newline).replace(/\r$/, "")
          pending = pending.slice(newline + 1)
          if (line.trim() !== "") yield emit(line)
        }
      }
      if (new TextEncoder().encode(pending).byteLength > maxFrameBytes) throw new InboundRequestError(new Response("Stream item exceeds maxStreamItemBytes", { status: 400 }))
      if (next.done) break
    }
    if (pending.trim() !== "") {
      if (contentType.includes("event-stream")) {
        const data = pending.split(/\r?\n/).filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trimStart()).join("\n")
        if (data !== "") yield emit(data)
      } else yield emit(pending.trim().replace(/^\u001e/, ""))
    }
    if (required && count === 0) throw new InboundRequestError(new Response("Request body is required", { status: 400 }))
  } finally {
    try { await reader.cancel() } finally { reader.releaseLock() }
  }
}

async function* decodeInboundMultipartStream(body: ReadableStream<Uint8Array>, contentType: string, schema: InboundSchema | undefined, schemas: InboundSchemas, required: boolean, itemContentType: string | undefined, wireSchema: WireSchema | undefined, wireSchemas: WireSchemas | undefined, maxFrameBytes: number): AsyncIterable<unknown> {
  const match = /(?:^|;)\s*boundary=(?:"([^"]+)"|([^;\s]+))/i.exec(contentType)
  const boundary = match?.[1] ?? match?.[2]
  if (boundary === undefined || boundary === "") throw new InboundRequestError(new Response("Invalid multipart boundary", { status: 400 }))
  const encoder = new TextEncoder()
  const opening = encoder.encode("--" + boundary)
  const separator = encoder.encode("\r\n--" + boundary)
  const reader = body.getReader()
  let pending = new Uint8Array()
  let started = false
  let closed = false
  let count = 0
  try {
    while (!closed) {
      const next = await reader.read()
      if (next.value !== undefined) pending = appendInboundBytes(pending, next.value)
      while (!closed) {
        if (!started) {
          const index = findInboundBytes(pending, opening)
          if (index < 0) break
          const after = index + opening.length
          if (pending.length < after + 2) break
          if (pending[after] === 45 && pending[after + 1] === 45) { closed = true; pending = pending.slice(after + 2); continue }
          if (pending[after] !== 13 || pending[after + 1] !== 10) throw new InboundRequestError(new Response("Invalid multipart boundary", { status: 400 }))
          pending = pending.slice(after + 2)
          started = true
          continue
        }
        const index = findInboundBytes(pending, separator)
        if (index < 0) break
        const after = index + separator.length
        if (pending.length < after + 2) break
        const closing = pending[after] === 45 && pending[after + 1] === 45
        if (!closing && (pending[after] !== 13 || pending[after + 1] !== 10)) throw new InboundRequestError(new Response("Invalid multipart boundary", { status: 400 }))
        const part = pending.slice(0, index)
        pending = pending.slice(after + 2)
        yield decodeInboundMultipartPart(part, schema, schemas, itemContentType, wireSchema, wireSchemas)
        count++
        if (closing) closed = true
      }
      if (pending.byteLength > maxFrameBytes + 8192) throw new InboundRequestError(new Response("Multipart item exceeds maxStreamItemBytes", { status: 400 }))
      if (next.done) break
    }
    if (!closed) throw new InboundRequestError(new Response("Invalid multipart body", { status: 400 }))
    if (required && count === 0) throw new InboundRequestError(new Response("Request body is required", { status: 400 }))
  } finally {
    try { await reader.cancel() } finally { reader.releaseLock() }
  }
}

function decodeInboundMultipartPart(part: Uint8Array, schema: InboundSchema | undefined, schemas: InboundSchemas, itemContentType: string | undefined, wireSchema: WireSchema | undefined, wireSchemas: WireSchemas | undefined): unknown {
  const split = findInboundBytes(part, new Uint8Array([13, 10, 13, 10]))
  if (split < 0) throw new InboundRequestError(new Response("Invalid multipart part", { status: 400 }))
  const headers = parseInboundMultipartHeaders(new TextDecoder().decode(part.slice(0, split)))
  const bytes = part.slice(split + 4)
  const rawContentType = headers.get("content-type") ?? itemContentType?.split(",", 1)[0]?.trim() ?? "text/plain"
  const normalized = rawContentType.split(";", 1)[0]!.trim().toLowerCase()
  let value: unknown
  if (normalized === "application/json" || normalized.endsWith("+json")) {
    try { value = JSON.parse(new TextDecoder().decode(bytes)) } catch { throw new InboundRequestError(new Response("Invalid multipart JSON item", { status: 400 })) }
  } else if (normalized.includes("xml")) {
    try { value = decodeInboundXML(new TextDecoder().decode(bytes), schema, schemas) } catch { throw new InboundRequestError(new Response("Invalid multipart XML item", { status: 400 })) }
  } else if (isInboundBinaryMedia(normalized, schema)) {
    return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength)
  } else value = new TextDecoder().decode(bytes)
  if (schema !== undefined) {
    const error = validateInboundValue(value, schema, schemas, "multipart item")
    if (error !== undefined) throw new InboundRequestError(new Response("Invalid multipart item: " + error, { status: 400 }))
  }
  return decodeInboundWireValue(value, wireSchema, wireSchemas)
}

function isInboundBinaryMedia(contentType: string, schema: InboundSchema | undefined): boolean {
  return contentType === "application/octet-stream" || contentType.startsWith("image/") || contentType.startsWith("audio/") || contentType.startsWith("video/") || schema?.["format"] === "binary" || schema?.["contentEncoding"] === "binary"
}

function parseInboundMultipartHeaders(source: string): Headers {
  const headers = new Headers()
  for (const line of source.split("\r\n")) {
    const separator = line.indexOf(":")
    if (separator <= 0) throw new InboundRequestError(new Response("Invalid multipart header", { status: 400 }))
    headers.append(line.slice(0, separator).trim(), line.slice(separator + 1).trim())
  }
  return headers
}

function appendInboundBytes(left: Uint8Array, right: Uint8Array): Uint8Array { const result = new Uint8Array(left.length + right.length); result.set(left); result.set(right, left.length); return result }

function findInboundBytes(source: Uint8Array, wanted: Uint8Array): number {
  outer: for (let start = 0; start <= source.length - wanted.length; start++) { for (let index = 0; index < wanted.length; index++) if (source[start + index] !== wanted[index]) continue outer; return start }
  return -1
}

function inboundMediaTypeMatches(expected: string, actual: string): boolean {
  const normalized = expected.toLowerCase()
  if (normalized === actual || (normalized.endsWith("+json") && actual.endsWith("+json"))) return true
  const [expectedType, expectedSubtype] = normalized.split("/", 2)
  const [actualType, actualSubtype] = actual.split("/", 2)
  if (expectedType === undefined || expectedSubtype === undefined || actualType === undefined || actualSubtype === undefined) return false
  if (expectedType !== "*" && expectedType !== actualType) return false
  if (expectedSubtype === "*") return true
  if (expectedSubtype.startsWith("*+") && actualSubtype.endsWith(expectedSubtype.slice(1))) return true
  return false
}

interface InboundXMLNode { readonly name: string; readonly attributes: Readonly<Record<string, string>>; readonly children: InboundXMLNode[]; text: string }

function decodeInboundXML(source: string, schema: InboundSchema | undefined, schemas: InboundSchemas): unknown {
  const root = parseInboundXML(source)
  return decodeInboundXMLNode(root, schema ?? {}, schemas)
}

function parseInboundXML(source: string): InboundXMLNode {
  const tokens = source.match(/<!\[CDATA\[[\s\S]*?\]\]>|<!--[\s\S]*?-->|<\?[^]*?\?>|<[^>]+>|[^<]+/g) ?? []
  const roots: InboundXMLNode[] = []
  const stack: InboundXMLNode[] = []
  for (const token of tokens) {
    if (token.startsWith("<!--") || token.startsWith("<?")) continue
    if (token.startsWith("<![CDATA[")) { if (stack.length === 0) throw new TypeError("XML character data is outside the document element"); stack[stack.length - 1]!.text += token.slice(9, -3); continue }
    if (token.startsWith("<!")) throw new TypeError("XML declarations are not supported")
    if (token.startsWith("</")) { const name = token.slice(2, -1).trim(); const node = stack.pop(); if (node === undefined || node.name !== name) throw new TypeError("XML closing tag mismatch"); continue }
    if (token.startsWith("<")) {
      const closing = /\/>$/.test(token); const body = token.slice(1, closing ? -2 : -1).trim(); const match = /^([^\s/>]+)([\s\S]*)$/.exec(body)
      if (match === null) throw new TypeError("XML element has no name")
      const node: InboundXMLNode = { name: match[1]!, attributes: parseInboundXMLAttributes(match[2] ?? ""), children: [], text: "" }
      if (stack.length === 0) roots.push(node); else stack[stack.length - 1]!.children.push(node)
      if (!closing) stack.push(node)
      continue
    }
    if (stack.length === 0) { if (token.trim() !== "") throw new TypeError("XML text is outside the document element"); continue }
    stack[stack.length - 1]!.text += unescapeInboundXML(token)
  }
  if (stack.length !== 0 || roots.length !== 1) throw new TypeError("XML document is not balanced")
  return roots[0]!
}

function parseInboundXMLAttributes(source: string): Readonly<Record<string, string>> {
  const result: Record<string, string> = {}; const expression = /([^\s=]+)\s*=\s*("[^"]*"|'[^']*')/g; let match: RegExpExecArray | null
  while ((match = expression.exec(source)) !== null) result[match[1]!] = unescapeInboundXML(match[2]!.slice(1, -1))
  if (source.replace(expression, "").trim() !== "") throw new TypeError("XML attribute syntax is invalid")
  return result
}

function decodeInboundXMLNode(node: InboundXMLNode, schema: InboundSchema, schemas: InboundSchemas): unknown {
  const resolved = resolveInboundSchema(schema, schemas)
  if (schemaAcceptsType(resolved["type"], "array")) {
    const item = isInboundSchema(resolved["items"]) ? resolved["items"] : {}
    return node.children.map((child) => decodeInboundXMLNode(child, item, schemas))
  }
  const properties = isRecord(resolved["properties"]) ? resolved["properties"] : {}
  if (schemaAcceptsType(resolved["type"], "object") || Object.keys(properties).length !== 0) {
    const result: Record<string, unknown> = {}
    for (const [name, childSchema] of Object.entries(properties)) {
      if (!isInboundSchema(childSchema)) continue
      const xml = isRecord(childSchema["xml"]) ? childSchema["xml"] : {}
      const xmlName = typeof xml["name"] === "string" ? xml["name"] : name
      if (xml["attribute"] === true || xml["nodeType"] === "attribute") {
        if (node.attributes[xmlName] !== undefined) result[name] = decodeInboundXMLScalar(node.attributes[xmlName]!, childSchema)
        continue
      }
      if (schemaAcceptsType(resolveInboundSchema(childSchema, schemas)["type"], "array")) {
        const item = isInboundSchema(childSchema["items"]) ? childSchema["items"] : {}
        const container = xml["wrapped"] === true ? node.children.find((child) => child.name === xmlName) : node
        if (container !== undefined) result[name] = container.children.filter((child) => child.name === xmlName || child.name === (isRecord(item["xml"]) && typeof item["xml"]["name"] === "string" ? item["xml"]["name"] : name)).map((child) => decodeInboundXMLNode(child, item, schemas))
        continue
      }
      const child = node.children.find((entry) => entry.name === xmlName)
      if (child !== undefined) result[name] = decodeInboundXMLNode(child, childSchema, schemas)
    }
    return result
  }
  return decodeInboundXMLScalar(node.text, resolved)
}

function resolveInboundSchema(schema: InboundSchema, schemas: InboundSchemas): InboundSchema {
  const reference = typeof schema["$ref"] === "string" ? schema["$ref"] : undefined
  if (reference === undefined || !reference.startsWith("#/components/schemas/")) return schema
  const name = reference.slice("#/components/schemas/".length).replaceAll("~1", "/").replaceAll("~0", "~")
  return schemas[name] ?? schema
}

function decodeInboundXMLScalar(value: string, schema: InboundSchema): unknown {
  if (schemaAcceptsType(schema["type"], "integer")) { const parsed = Number(value); if (!Number.isInteger(parsed)) throw new TypeError("XML value is not an integer"); return parsed }
  if (schemaAcceptsType(schema["type"], "number")) { const parsed = Number(value); if (!Number.isFinite(parsed)) throw new TypeError("XML value is not a number"); return parsed }
  if (schemaAcceptsType(schema["type"], "boolean")) { if (value === "true") return true; if (value === "false") return false; throw new TypeError("XML value is not a boolean") }
  return value
}

function unescapeInboundXML(value: string): string { return value.replaceAll("&lt;", "<").replaceAll("&gt;", ">").replaceAll("&quot;", "\"").replaceAll("&apos;", "'").replaceAll("&amp;", "&") }

function validateInboundValue(value: unknown, schema: InboundSchema, schemas: InboundSchemas, path: string): string | undefined {
  if (schema["x-sdkgen-boolean-schema"] === false) return path + " is rejected by a false schema"
  const reference = typeof schema["$ref"] === "string" ? schema["$ref"] : undefined
  if (reference !== undefined) {
    const name = reference.startsWith("#/components/schemas/") ? reference.slice("#/components/schemas/".length).replaceAll("~1", "/").replaceAll("~0", "~") : undefined
    const target = name === undefined ? undefined : schemas[name]
    return target === undefined ? path + " references an unresolved schema" : validateInboundValue(value, target, schemas, path)
  }
  if (value === null && (schema["nullable"] === true || schemaAcceptsType(schema["type"], "null"))) return undefined
  if (value instanceof Blob && schemaAcceptsType(schema["type"], "string") && (schema["format"] === "binary" || schema["contentEncoding"] === "binary")) return undefined
  if (!schemaAcceptsValue(value, schema["type"])) return path + " has an invalid type"
  if ("const" in schema && JSON.stringify(value) !== JSON.stringify(schema["const"])) return path + " must equal its declared constant"
  if (Array.isArray(schema["enum"]) && !schema["enum"].some((item) => JSON.stringify(item) === JSON.stringify(value))) return path + " is not an allowed value"
  if (typeof value === "string") {
    if (typeof schema["minLength"] === "number" && value.length < schema["minLength"]) return path + " is shorter than minLength"
    if (typeof schema["maxLength"] === "number" && value.length > schema["maxLength"]) return path + " is longer than maxLength"
    if (typeof schema["pattern"] === "string" && !new RegExp(schema["pattern"]).test(value)) return path + " does not match pattern"
  }
  if (typeof value === "number") {
    if (typeof schema["minimum"] === "number" && value < schema["minimum"]) return path + " is below minimum"
    if (typeof schema["maximum"] === "number" && value > schema["maximum"]) return path + " is above maximum"
    if (schema["exclusiveMinimum"] === true && typeof schema["minimum"] === "number" && value <= schema["minimum"]) return path + " is not above exclusiveMinimum"
    if (schema["exclusiveMaximum"] === true && typeof schema["maximum"] === "number" && value >= schema["maximum"]) return path + " is not below exclusiveMaximum"
    if (typeof schema["exclusiveMinimum"] === "number" && value <= schema["exclusiveMinimum"]) return path + " is not above exclusiveMinimum"
    if (typeof schema["exclusiveMaximum"] === "number" && value >= schema["exclusiveMaximum"]) return path + " is not below exclusiveMaximum"
    if (typeof schema["multipleOf"] === "number" && value / schema["multipleOf"] % 1 !== 0) return path + " is not a multipleOf value"
    if (schema["type"] === "integer" && !Number.isInteger(value)) return path + " must be an integer"
  }
  if (Array.isArray(value)) {
    if (typeof schema["minItems"] === "number" && value.length < schema["minItems"]) return path + " has too few items"
    if (typeof schema["maxItems"] === "number" && value.length > schema["maxItems"]) return path + " has too many items"
    if (schema["uniqueItems"] === true && new Set(value.map((item) => JSON.stringify(item))).size !== value.length) return path + " has duplicate items"
    const prefixItems = Array.isArray(schema["prefixItems"]) ? schema["prefixItems"] : []
    for (let index = 0; index < prefixItems.length && index < value.length; index++) {
      if (isInboundSchema(prefixItems[index])) { const error = validateInboundValue(value[index], prefixItems[index], schemas, path + "[" + index + "]"); if (error !== undefined) return error }
    }
    if (isInboundSchema(schema["items"])) for (let index = prefixItems.length; index < value.length; index++) { const error = validateInboundValue(value[index], schema["items"], schemas, path + "[" + index + "]"); if (error !== undefined) return error }
  }
  if (isRecord(value)) {
	if (typeof schema["minProperties"] === "number" && Object.keys(value).length < schema["minProperties"]) return path + " has too few properties"
    if (typeof schema["maxProperties"] === "number" && Object.keys(value).length > schema["maxProperties"]) return path + " has too many properties"
    const required = Array.isArray(schema["required"]) ? schema["required"] : []
    for (const key of required) if (typeof key === "string" && !(key in value)) return path + "." + key + " is required"
    const properties = isRecord(schema["properties"]) ? schema["properties"] : {}
    for (const [key, propertySchema] of Object.entries(properties)) if (key in value && isInboundSchema(propertySchema)) { const error = validateInboundValue(value[key], propertySchema, schemas, path + "." + key); if (error !== undefined) return error }
    if (isInboundSchema(schema["additionalProperties"])) for (const [key, item] of Object.entries(value)) if (!(key in properties)) { const error = validateInboundValue(item, schema["additionalProperties"], schemas, path + "." + key); if (error !== undefined) return error }
    if (schema["additionalProperties"] === false) for (const key of Object.keys(value)) if (!(key in properties)) return path + "." + key + " is not allowed"
  }
  if (Array.isArray(schema["allOf"])) for (const part of schema["allOf"]) if (isInboundSchema(part)) { const error = validateInboundValue(value, part, schemas, path); if (error !== undefined) return error }
  return undefined
}

function schemaAcceptsValue(value: unknown, type: unknown): boolean {
  if (type === undefined) return true
  const types = Array.isArray(type) ? type : [type]
  return types.some((item) => item === "null" ? value === null : item === "string" ? typeof value === "string" : item === "number" ? typeof value === "number" : item === "integer" ? typeof value === "number" : item === "boolean" ? typeof value === "boolean" : item === "array" ? Array.isArray(value) : item === "object" ? isRecord(value) : false)
}

function schemaAcceptsType(type: unknown, wanted: string): boolean { return Array.isArray(type) ? type.includes(wanted) : type === wanted }
function isRecord(value: unknown): value is Record<string, unknown> { return typeof value === "object" && value !== null && !Array.isArray(value) }
function isInboundSchema(value: unknown): value is InboundSchema { return isRecord(value) }

/** Converts and validates a handler value into its declared Fetch Response representation. */
export async function responseFromHandler(value: InboundResponse, options?: InboundResponseOptions): Promise<Response> {
  const headers = new Headers(value.headers)
  if ((value.status === 204 || value.status === 205) && value.body !== undefined) throw new TypeError("Responses with status 204 or 205 must not include a body")
  const statusDefinitions = options?.responses.filter((definition) => inboundResponseStatusMatches(definition.status, value.status)) ?? []
  if (options !== undefined && statusDefinitions.length === 0) throw new TypeError("response status " + value.status + " is not declared by this endpoint")
	const generatedHeaderNames = await appendInboundResponseHeaderValues(headers, value.headerValues, statusDefinitions.flatMap((definition) => definition.headers ?? []), options?.schemas ?? {}, options?.codecs)
  if (value.body === undefined) {
    if (options !== undefined && !statusDefinitions.some((definition) => definition.contentType === undefined)) throw new TypeError("response status " + value.status + " requires a body")
	    await validateInboundResponseHeaders(headers, statusDefinitions.find((definition) => definition.contentType === undefined)?.headers, options?.schemas ?? {}, options?.codecs, generatedHeaderNames)
    return new Response(null, { status: value.status, headers })
  }
  const contentType = value.contentType ?? headers.get("content-type") ?? "application/json"
  if (!headers.has("content-type")) headers.set("content-type", contentType)
  const definition = statusDefinitions.filter((entry) => entry.contentType !== undefined && inboundMediaTypeMatches(entry.contentType, normalizeInboundMediaType(contentType))).sort((left, right) => inboundMediaTypeMatchScore(right.contentType ?? "", contentType) - inboundMediaTypeMatchScore(left.contentType ?? "", contentType))[0]
  if (options !== undefined && definition === undefined) throw new TypeError("response content type " + contentType + " is not declared for status " + value.status)
  if (definition?.schema !== undefined) validateWireValue(value.body, definition.schema, options!.schemas, "encode")
	await validateInboundResponseHeaders(headers, definition?.headers, options?.schemas ?? {}, options?.codecs, generatedHeaderNames)
  if (contentType === "application/json" || contentType.endsWith("+json")) return new Response(JSON.stringify(value.body), { status: value.status, headers })
  if (contentType.includes("xml")) return new Response(encodeXML(value.body, definition?.schema ?? {}, options?.schemas ?? {}), { status: value.status, headers })
  if (contentType.startsWith("text/")) return new Response(String(value.body), { status: value.status, headers })
  if (value.body instanceof Blob || value.body instanceof ArrayBuffer || ArrayBuffer.isView(value.body)) return new Response(value.body as BodyInit, { status: value.status, headers })
  const codec = options?.codecs?.get(normalizeInboundMediaType(contentType))
  if (codec?.encode === undefined) throw new TypeError("missing encode codec for " + contentType)
  return new Response(await codec.encode(value.body, { contentType }), { status: value.status, headers })
}

async function validateInboundResponseHeaders(headers: Headers, definitions: readonly WireHeaderDefinition[] | undefined, schemas: WireSchemas, codecs: ReadonlyMap<string, MediaCodec<unknown>> | undefined, generatedHeaderNames: ReadonlySet<string> = new Set()): Promise<void> {
  for (const definition of definitions ?? []) {
	if (generatedHeaderNames.has(definition.name.toLowerCase())) continue
    const value = headers.get(definition.name)
    if (value === null) {
      if (definition.required) throw new TypeError("missing required response header " + definition.name)
      continue
    }
    const decoded = await decodeInboundResponseHeaderValue(value, definition, schemas, codecs)
    validateWireValue(decoded, definition.schema, schemas, "decode")
  }
}

async function decodeInboundResponseHeaderValue(value: string, definition: WireHeaderDefinition, schemas: WireSchemas, codecs: ReadonlyMap<string, MediaCodec<unknown>> | undefined): Promise<unknown> {
  const contentType = normalizeInboundMediaType(definition.contentType ?? "")
  if (contentType === "application/json" || contentType.endsWith("+json")) {
    try { return JSON.parse(value) } catch { throw new TypeError("invalid JSON response header " + definition.name) }
  }
  if (contentType === "application/x-www-form-urlencoded") {
    const form: Record<string, string | string[]> = {}
    for (const [name, item] of new URLSearchParams(value)) {
      const previous = form[name]
      form[name] = previous === undefined ? item : Array.isArray(previous) ? [...previous, item] : [previous, item]
    }
    return form
  }
  if (contentType.includes("xml")) return decodeXML(value, definition.schema, schemas)
  if (contentType !== "") {
    if (contentType.startsWith("text/")) return value
    const codec = inboundMediaCodec(codecs, contentType)
    if (codec?.decodeParameter === undefined) throw new TypeError("missing decodeParameter codec for response header " + definition.name)
    return codec.decodeParameter(value, { contentType })
  }
  return decodeInboundSimpleHeader(value, definition.schema, schemas)
}

function decodeInboundSimpleHeader(value: string, schema: WireSchema, schemas: WireSchemas): unknown {
  const resolved = resolveInboundHeaderSchema(schema, schemas)
  if (resolved.types?.includes("array")) {
    const item = resolved.items ?? {}
    return value.split(",").map((entry) => decodeInboundSimpleHeaderScalar(entry, item, schemas))
  }
  if (resolved.types?.includes("object") || resolved.properties !== undefined) {
    const result: Record<string, unknown> = {}
    const tokens = value.split(",")
    if (definition.explode) {
      for (const token of tokens) {
        const separator = token.indexOf("=")
        if (separator < 0) continue
        const name = token.slice(0, separator)
        const property = resolved.properties?.[name]
        result[name] = decodeInboundSimpleHeaderScalar(token.slice(separator + 1), property?.schema ?? {}, schemas)
      }
    } else for (let index = 0; index + 1 < tokens.length; index += 2) {
      const name = tokens[index]!
      const property = resolved.properties?.[name]
      result[name] = decodeInboundSimpleHeaderScalar(tokens[index + 1]!, property?.schema ?? {}, schemas)
    }
    return result
  }
  return decodeInboundSimpleHeaderScalar(value, resolved, schemas)
}

function decodeInboundSimpleHeaderScalar(value: string, schema: WireSchema, schemas: WireSchemas): unknown {
  const resolved = resolveInboundHeaderSchema(schema, schemas)
  if (resolved.types?.includes("integer")) { const number = Number(value); return Number.isInteger(number) ? number : value }
  if (resolved.types?.includes("number")) { const number = Number(value); return Number.isFinite(number) ? number : value }
  if (resolved.types?.includes("boolean")) return value === "true" ? true : value === "false" ? false : value
  return value
}

function resolveInboundHeaderSchema(schema: WireSchema, schemas: WireSchemas): WireSchema {
  const referenced = schema.reference === undefined ? undefined : schemas[schema.reference]
  return referenced === undefined ? schema : resolveInboundHeaderSchema(referenced, schemas)
}

async function appendInboundResponseHeaderValues(headers: Headers, values: Readonly<Record<string, unknown>> | undefined, definitions: readonly WireHeaderDefinition[], schemas: WireSchemas, codecs: ReadonlyMap<string, MediaCodec<unknown>> | undefined): Promise<ReadonlySet<string>> {
  const result = new Set<string>()
  if (values === undefined) return result
  const byProperty = new Map(definitions.map((definition) => [definition.property, definition]))
  for (const [property, value] of Object.entries(values)) {
    const definition = byProperty.get(property)
    if (definition === undefined) throw new TypeError("undeclared response header property " + property)
    if (headers.has(definition.name)) throw new TypeError("response header is provided by both headers and headerValues: " + definition.name)
    validateWireValue(value, definition.schema, schemas, "encode")
    headers.set(definition.name, await encodeInboundResponseHeaderValue(value, definition, schemas, codecs))
    result.add(definition.name.toLowerCase())
  }
	return result
}

async function encodeInboundResponseHeaderValue(value: unknown, definition: WireHeaderDefinition, schemas: WireSchemas, codecs: ReadonlyMap<string, MediaCodec<unknown>> | undefined): Promise<string> {
  const encoded = encodeWireValue(value, definition.schema, schemas)
  const contentType = normalizeInboundMediaType(definition.contentType ?? "")
  if (contentType === "application/json" || contentType.endsWith("+json")) return JSON.stringify(encoded)
  if (contentType.includes("xml")) return encodeXML(encoded, definition.schema, schemas)
  if (contentType === "application/x-www-form-urlencoded") return encodeInboundHeaderForm(encoded)
  if (contentType !== "" && !contentType.startsWith("text/")) {
    const codec = inboundMediaCodec(codecs, contentType)
    if (codec?.encodeParameter === undefined) throw new TypeError("missing encodeParameter codec for response header " + definition.name)
    return codec.encodeParameter(encoded, { contentType })
  }
  return encodeInboundSimpleHeader(encoded, definition.explode ?? false)
}

function encodeInboundHeaderForm(value: unknown): string {
  if (!isRecord(value)) return String(value ?? "")
  const form = new URLSearchParams()
  for (const [name, item] of Object.entries(value)) {
    for (const entry of Array.isArray(item) ? item : [item]) form.append(name, isRecord(entry) || Array.isArray(entry) ? JSON.stringify(entry) : String(entry ?? ""))
  }
  return form.toString()
}

function encodeInboundSimpleHeader(value: unknown, explode = false): string {
  if (Array.isArray(value)) return value.map((item) => String(item ?? "")).join(",")
  if (isRecord(value)) return explode ? Object.entries(value).map(([name, item]) => name + "=" + String(item ?? "")).join(",") : Object.entries(value).flatMap(([name, item]) => [name, String(item ?? "")]).join(",")
  return String(value ?? "")
}

function normalizeInboundMediaType(value: string): string { return value.split(";", 1)[0]!.trim().toLowerCase() }

function inboundResponseStatusMatches(declared: string, actual: number): boolean {
  if (declared === "default") return true
  if (/^[1-5][0-9][0-9]$/.test(declared)) return Number(declared) === actual
  if (/^[1-5][Xx][Xx]$/.test(declared)) return Number(declared[0]) === Math.floor(actual / 100)
  return false
}
`)
}
