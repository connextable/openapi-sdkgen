package typescript

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
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
	paramsType   string
	responseType string
	responsePlan string
	security     any
}

type callbackDefinition struct {
	name              string
	sourceOperationID string
	componentName     string
	callbackName      string
	typeName          string
	expression        string
	operationID       string
	method            string
	bodyType          string
	hasBody           bool
	bodyRequired      bool
	bodyPlans         string
	parameters        string
	paramsType        string
	responseType      string
	responsePlan      string
	security          any
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
	for _, operation := range document.Operations {
		callbacks, _ := operation.Raw["callbacks"].(map[string]any)
		definitions, err := collectCallbackMap(document, callbacks, openAPIPointer("paths", operation.Path, strings.ToLower(operation.Method), "callbacks"), operation.OperationID, "")
		if err != nil {
			return nil, err
		}
		result = append(result, definitions...)
	}
	components, _ := document.Raw["components"].(map[string]any)
	componentCallbacks, _ := components["callbacks"].(map[string]any)
	for _, componentName := range sortedAnyKeys(componentCallbacks) {
		value := map[string]any{componentName: componentCallbacks[componentName]}
		definitions, err := collectCallbackMap(document, value, openAPIPointer("components", "callbacks"), "", componentName)
		if err != nil {
			return nil, err
		}
		result = append(result, definitions...)
	}
	sort.Slice(result, func(i, j int) bool { return callbackIdentity(result[i]) < callbackIdentity(result[j]) })
	return result, nil
}

func collectCallbackMap(document *ir.Document, values map[string]any, path, sourceOperationID, componentName string) ([]callbackDefinition, error) {
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
		for _, expression := range sortedAnyKeys(resolved) {
			pathItem, ok := resolved[expression].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s must be a Callback Path Item Object", appendOpenAPIPointer(appendOpenAPIPointer(path, name), expression))
			}
			operations, resolvedPathItem, err := serverPathItemOperations(document, pathItem, appendOpenAPIPointer(appendOpenAPIPointer(path, name), expression))
			if err != nil {
				return nil, err
			}
			for _, item := range operations {
				method := item.method
				operation := item.operation
				operationID, _ := operation["operationId"].(string)
				if operationID == "" {
					operationID = name
				}
				operationPath := appendOpenAPIPointer(appendOpenAPIPointer(appendOpenAPIPointer(path, name), expression), method)
				parameters, paramsType, err := inboundParameterDefinitions(document, resolvedPathItem, operation, operationPath, true)
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
				identity := sourceOperationID + "\x00" + componentName + "\x00" + name + "\x00" + expression + "\x00" + method
				result = append(result, callbackDefinition{
					name: appendOpenAPIPointer(path, name), sourceOperationID: sourceOperationID, componentName: componentName, callbackName: name,
					typeName: stablePrivateIdentifier("callback-type", identity), expression: expression, operationID: operationID, method: method,
					bodyType: body.typeName, hasBody: body.hasBody, bodyRequired: body.required, bodyPlans: body.plans, parameters: parameters, paramsType: paramsType,
					responseType: responseType, responsePlan: responsePlan, security: security,
				})
			}
		}
	}
	return result, nil
}

func callbackIdentity(callback callbackDefinition) string {
	return callback.sourceOperationID + "\x00" + callback.componentName + "\x00" + callback.callbackName + "\x00" + callback.expression + "\x00" + callback.method
}

type callbackCatalogNode struct {
	children map[string]*callbackCatalogNode
	callback *callbackDefinition
}

func buildCallbackCatalog(callbacks []callbackDefinition, components bool) *callbackCatalogNode {
	root := &callbackCatalogNode{children: make(map[string]*callbackCatalogNode)}
	for index := range callbacks {
		callback := &callbacks[index]
		if (callback.componentName != "") != components {
			continue
		}
		keys := []string{callback.sourceOperationID, callback.callbackName, callback.expression, callback.method}
		if components {
			keys = []string{callback.componentName, callback.expression, callback.method}
		}
		node := root
		for _, key := range keys {
			if node.children[key] == nil {
				node.children[key] = &callbackCatalogNode{children: make(map[string]*callbackCatalogNode)}
			}
			node = node.children[key]
		}
		node.callback = callback
	}
	return root
}

func callbackCatalogKeys(node *callbackCatalogNode) []string {
	keys := make([]string, 0, len(node.children))
	for key := range node.children {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func emitCallbackTypeCatalog(output *bytes.Buffer, name string, root *callbackCatalogNode) {
	fmt.Fprintf(output, "export interface %s ", name)
	emitCallbackCatalogObject(output, root, nil, name, callbackCatalogTypes)
	output.WriteString("\n\n")
}

type callbackCatalogMode int

const (
	callbackCatalogTypes callbackCatalogMode = iota
	callbackCatalogHandlers
	callbackCatalogEndpoints
	callbackCatalogPathParams
)

func emitCallbackCatalogObject(output *bytes.Buffer, node *callbackCatalogNode, path []string, catalog string, mode callbackCatalogMode) {
	output.WriteString("{\n")
	indent := strings.Repeat("  ", len(path)+1)
	for _, key := range callbackCatalogKeys(node) {
		child := node.children[key]
		optional := ""
		if mode == callbackCatalogHandlers || mode == callbackCatalogPathParams {
			optional = "?"
		}
		fmt.Fprintf(output, "%sreadonly %s%s: ", indent, quoteTS(key), optional)
		if child.callback != nil {
			slot := catalog
			for _, item := range append(path, key) {
				slot += "[" + quoteTS(item) + "]"
			}
			switch mode {
			case callbackCatalogTypes:
				fmt.Fprintf(output, "{ readonly context: %sContext; readonly input: %sContext; readonly output: %sResponse; readonly response: %sResponse; readonly handler: (context: %sContext) => %sResponse | Promise<%sResponse>; readonly endpoint: CallbackEndpoint }", child.callback.typeName, child.callback.typeName, child.callback.typeName, child.callback.typeName, child.callback.typeName, child.callback.typeName, child.callback.typeName)
			case callbackCatalogHandlers:
				fmt.Fprintf(output, "%s[\"handler\"]", slot)
			case callbackCatalogEndpoints:
				fmt.Fprintf(output, "%s[\"endpoint\"]", slot)
			case callbackCatalogPathParams:
				output.WriteString("Readonly<Record<string, string>>")
			}
		} else {
			emitCallbackCatalogObject(output, child, append(path, key), catalog, mode)
		}
		output.WriteString("\n")
	}
	output.WriteString(strings.Repeat("  ", len(path)) + "}")
}

func callbackCatalogPath(callback callbackDefinition) (string, []string) {
	if callback.componentName != "" {
		return "ComponentCallbacks", []string{callback.componentName, callback.expression, callback.method}
	}
	return "Callbacks", []string{callback.sourceOperationID, callback.callbackName, callback.expression, callback.method}
}

func callbackAccess(root string, callback callbackDefinition) string {
	_, path := callbackCatalogPath(callback)
	for _, key := range path {
		root += "?." + "[" + quoteTS(key) + "]"
	}
	return root
}

func callbackDefinitionSymbol(callback callbackDefinition) string {
	return stablePrivateIdentifier("callback-definition", callbackIdentity(callback))
}

func callbackEndpointSymbol(callback callbackDefinition) string {
	return stablePrivateIdentifier("callback-endpoint", callbackIdentity(callback))
}

func callbackRootField(callback callbackDefinition) string {
	if callback.componentName != "" {
		return "componentCallbacks"
	}
	return "callbacks"
}

func callbackRuntimeCatalog(root *callbackCatalogNode) string {
	properties := make([]runtimeProperty, 0, len(root.children))
	for _, key := range callbackCatalogKeys(root) {
		child := root.children[key]
		value := ""
		if child.callback != nil {
			value = callbackEndpointSymbol(*child.callback)
		} else {
			value = callbackRuntimeCatalog(child)
		}
		properties = append(properties, runtimeProperty{key: key, value: value})
	}
	return runtimeObjectExpression(properties)
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
			schemaValue, hasSchema := media["schema"]
			schema, _ := schemaValue.(map[string]any)
			booleanSchema, isBooleanSchema := schemaValue.(bool)
			schemaIsFalse := isBooleanSchema && !booleanSchema
			binary := isBinaryMedia(mediaType, schema) && !schemaIsFalse
			bodyType := "ArrayBuffer | Blob | ArrayBufferView"
			if schemaIsFalse || isJSONMediaType(mediaType) || strings.Contains(strings.ToLower(mediaType), "xml") {
				bodyType, err = schemaTypeForScope(document, schemaValue, projectionOutput, typeRenderContract)
				if err != nil {
					return "", "", fmt.Errorf("%s/responses/%s/content/%s/schema: %w", path, status, mediaType, err)
				}
			} else if isTextMedia(mediaType) {
				bodyType = "string"
			} else if !binary {
				bodyType, err = schemaTypeForScope(document, schemaValue, projectionOutput, typeRenderContract)
				if err != nil {
					return "", "", fmt.Errorf("%s/responses/%s/content/%s/schema: %w", path, status, mediaType, err)
				}
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
			if hasSchema && !binary {
				descriptor, descriptorErr := wireSchemaDescriptorForDocument(document, schemaValue, projectionOutput)
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
		valueType, err := schemaTypeForScope(document, schema, projectionOutput, typeRenderContract)
		if err != nil {
			return "", err
		}
		optional := "?"
		if boolValue(resolved, "required") {
			optional = ""
		}
		fields = append(fields, "readonly "+quoteTS(name)+optional+": "+valueType)
	}
	return "Readonly<{ " + strings.Join(fields, "; ") + " }>", nil
}

func emitCallbacks(document *ir.Document, callbacks []callbackDefinition) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("import { collectInboundSecurityCandidates, decodeInboundBody, decodeInboundParameters, InboundRequestError, normalizeInboundMediaCodecs, requiresInboundAuthentication, responseFromHandler, type Authenticate, type InboundParameterValues, type InboundRequestContext, type InboundResponse, type InboundParameterDefinition, type InboundSchemas, type InboundSecuritySchemes } from \"./runtime.js\"\n")
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
	operationCatalog := buildCallbackCatalog(callbacks, false)
	componentCatalog := buildCallbackCatalog(callbacks, true)
	for _, callback := range callbacks {
		fmt.Fprintf(&output, "/** Host-owned Callback endpoint for URL expression %s. No route is generated. */\n", quoteTS(callback.expression))
		fmt.Fprintf(&output, "interface %sContext extends InboundRequestContext {\n  readonly params: %s\n", callback.typeName, callback.paramsType)
		if callback.hasBody {
			fmt.Fprintf(&output, "  readonly body: %s\n", callback.bodyType)
		} else {
			output.WriteString("  readonly body: undefined\n")
		}
		output.WriteString("}\n")
		fmt.Fprintf(&output, "type %sResponse = %s\n", callback.typeName, callback.responseType)
	}
	emitCallbackTypeCatalog(&output, "Callbacks", operationCatalog)
	emitCallbackTypeCatalog(&output, "ComponentCallbacks", componentCatalog)
	for _, callback := range callbacks {
		security, err := runtimeJSONExpression(callback.security)
		if err != nil {
			return nil, fmt.Errorf("%s security metadata: %w", callback.name, err)
		}
		fmt.Fprintf(&output, "const %s = { operationID: %s, method: %s, parameters: %s satisfies readonly InboundParameterDefinition[], responses: %s, security: %s } as const\n", callbackDefinitionSymbol(callback), quoteTS(callback.operationID), quoteTS(callback.method), callback.parameters, callback.responsePlan, security)
	}
	output.WriteString("\n/** Application handlers keyed by exact Callback identity. */\nexport interface CallbackHandlers {\n  readonly callbacks?: ")
	emitCallbackCatalogObject(&output, operationCatalog, nil, "Callbacks", callbackCatalogHandlers)
	output.WriteString("\n  readonly componentCallbacks?: ")
	emitCallbackCatalogObject(&output, componentCatalog, nil, "ComponentCallbacks", callbackCatalogHandlers)
	output.WriteString("\n}\n\n/** Optional host authentication, media codecs, and host-bound path parameters for generated Callback endpoints. */\nexport interface CallbackHandlerOptions {\n  readonly authenticate?: Authenticate | undefined\n  readonly codecs?: Readonly<Record<string, MediaCodec<unknown>>> | undefined\n  readonly maxStreamItemBytes?: number | undefined\n  readonly pathParams?: {\n    readonly callbacks?: ")
	emitCallbackCatalogObject(&output, operationCatalog, nil, "Callbacks", callbackCatalogPathParams)
	output.WriteString("\n    readonly componentCallbacks?: ")
	emitCallbackCatalogObject(&output, componentCatalog, nil, "ComponentCallbacks", callbackCatalogPathParams)
	output.WriteString("\n  } | undefined\n}\n\n/** Fetch-compatible endpoint for one host-mounted Callback route. */\nexport interface CallbackEndpoint {\n  fetch(request: Request): Promise<Response>\n}\n\n/** Callback endpoints preserving every exact source identity dimension. */\nexport interface CallbackEndpoints {\n  readonly callbacks: ")
	emitCallbackCatalogObject(&output, operationCatalog, nil, "Callbacks", callbackCatalogEndpoints)
	output.WriteString("\n  readonly componentCallbacks: ")
	emitCallbackCatalogObject(&output, componentCatalog, nil, "ComponentCallbacks", callbackCatalogEndpoints)
	output.WriteString("\n}\n\n/**\n * Creates Fetch-native endpoints for dynamic OpenAPI Callback URLs.\n * The host chooses each concrete route and mounts the matching endpoint.\n */\nexport function createCallbackHandlers(handlers: CallbackHandlers, options: CallbackHandlerOptions = {}): CallbackEndpoints {\n  const inboundCodecs = normalizeInboundMediaCodecs(options.codecs)\n")
	for _, callback := range callbacks {
		definition := callbackDefinitionSymbol(callback)
		handler := callbackAccess("handlers."+callbackRootField(callback), callback)
		pathParams := callbackAccess("options.pathParams?."+callbackRootField(callback), callback)
		fmt.Fprintf(&output, "  const %s: CallbackEndpoint = {\n    async fetch(request: Request): Promise<Response> {\n", callbackEndpointSymbol(callback))
		fmt.Fprintf(&output, "      if (request.method !== %s) return new Response(\"Method Not Allowed\", { status: 405, headers: { allow: %s } })\n", quoteTS(callback.method), quoteTS(callback.method))
		fmt.Fprintf(&output, "      const handler = %s\n      if (handler === undefined) return new Response(\"Not Found\", { status: 404 })\n", handler)
		fmt.Fprintf(&output, "      let params: %s\n      try { params = await decodeInboundParameters(request, %s.parameters, inputSchemas, inputWireSchemas, inboundCodecs, %s) as %s } catch (error) { if (error instanceof InboundRequestError) return error.response; throw error }\n      const context = { request, operationID: %s.operationID, method: %s.method, path: new URL(request.url).pathname, params, security: %s.security, securityCandidates: collectInboundSecurityCandidates(request, %s.security, securitySchemes) } as Omit<%sContext, \"body\">\n", callback.paramsType, definition, pathParams, callback.paramsType, definition, definition, definition, definition, callback.typeName)
		output.WriteString("      if (requiresInboundAuthentication(context.security)) {\n        if (options.authenticate === undefined) return new Response(\"Unauthorized\", { status: 401 })\n        try { const denied = await options.authenticate(context); if (denied instanceof Response) return denied }\n        catch { return new Response(\"Internal Server Error\", { status: 500 }) }\n      }\n")
		if callback.hasBody {
			output.WriteString("      try {\n")
			fmt.Fprintf(&output, "        const body = await decodeInboundBody(request, { required: %t, plans: %s, schemas: inputSchemas, wireSchemas: inputWireSchemas, codecs: inboundCodecs, maxStreamItemBytes: options.maxStreamItemBytes }) as %s\n", callback.bodyRequired, callback.bodyPlans, callback.bodyType)
			fmt.Fprintf(&output, "        return await responseFromHandler(await handler({ ...context, body }), { schemas: outputSchemas, responses: %s.responses, codecs: inboundCodecs })\n", definition)
			output.WriteString("      } catch (error) {\n        if (error instanceof InboundRequestError) return error.response\n        return new Response(\"Internal Server Error\", { status: 500 })\n      }\n")
		} else {
			output.WriteString("      try {\n")
			fmt.Fprintf(&output, "        return await responseFromHandler(await handler({ ...context, body: undefined }), { schemas: outputSchemas, responses: %s.responses, codecs: inboundCodecs })\n", definition)
			output.WriteString("      } catch { return new Response(\"Internal Server Error\", { status: 500 }) }\n")
		}
		output.WriteString("    },\n  }\n")
	}
	fmt.Fprintf(
		&output,
		"  return { callbacks: %s as unknown as CallbackEndpoints[\"callbacks\"], componentCallbacks: %s as unknown as CallbackEndpoints[\"componentCallbacks\"] }\n}\n",
		callbackRuntimeCatalog(operationCatalog),
		callbackRuntimeCatalog(componentCatalog),
	)
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
		operations, resolvedItem, err := serverPathItemOperations(document, item, openAPIPointer("webhooks", name))
		if err != nil {
			return nil, err
		}
		for _, itemOperation := range operations {
			method := itemOperation.method
			operation := itemOperation.operation
			operationPath := openAPIPointer("webhooks", name, method)
			parameters, paramsType, err := inboundParameterDefinitions(document, resolvedItem, operation, operationPath, true)
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
			methodName := method
			result = append(result, webhookDefinition{
				name: name, property: name, typeName: stablePrivateIdentifier("webhook-type", name+"\x00"+methodName), operationID: operationID,
				method: methodName, bodyType: body.typeName, hasBody: body.hasBody, bodyRequired: body.required, bodyPlans: body.plans, parameters: parameters, paramsType: paramsType, responseType: responseType, responsePlan: responsePlan, security: security,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].name == result[j].name {
			return result[i].method < result[j].method
		}
		return result[i].name < result[j].name
	})
	return result, nil
}

var serverHTTPMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace", "query"}

type serverPathItemOperation struct {
	key       string
	method    string
	operation map[string]any
}

func serverPathItemOperations(document *ir.Document, pathItem map[string]any, path string) ([]serverPathItemOperation, map[string]any, error) {
	resolved, err := ir.ResolvePathItem(document.Raw, pathItem)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	result := make([]serverPathItemOperation, 0)
	for _, method := range serverHTTPMethods {
		if operation, ok := resolved[method].(map[string]any); ok {
			result = append(result, serverPathItemOperation{key: method, method: strings.ToUpper(method), operation: operation})
		}
	}
	additional, _ := resolved["additionalOperations"].(map[string]any)
	for _, method := range sortedAnyKeys(additional) {
		operation, ok := additional[method].(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("%s/additionalOperations/%s must be an Operation Object", path, method)
		}
		result = append(result, serverPathItemOperation{key: method, method: method, operation: operation})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].method < result[j].method })
	return result, resolved, nil
}

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
	schemaValue, hasSchema := media["schema"]
	if !hasSchema {
		schemaValue = map[string]any{}
	}
	schema, _ := schemaValue.(map[string]any)
	if schema == nil {
		schema = map[string]any{}
	}
	booleanSchema, isBooleanSchema := schemaValue.(bool)
	schemaIsFalse := isBooleanSchema && !booleanSchema
	binary := isBinaryMedia(mediaType, schema) && !schemaIsFalse
	stream := isStreamMediaType(mediaType) || media["itemSchema"] != nil
	value := "ArrayBuffer"
	if stream {
		itemSchema, exists := media["itemSchema"]
		if !exists {
			return "", "", fmt.Errorf("%s/requestBody/content/%s: sequential stream requires itemSchema", path, mediaType)
		}
		var err error
		value, err = schemaTypeForScope(document, itemSchema, projectionInput, typeRenderContract)
		if err != nil {
			return "", "", fmt.Errorf("%s/requestBody/content/%s/itemSchema: %w", path, mediaType, err)
		}
		schemaValue = itemSchema
		schema, _ = itemSchema.(map[string]any)
		if schema == nil {
			schema = map[string]any{}
		}
	} else if schemaIsFalse || !binary {
		var err error
		value, err = schemaTypeForScope(document, schemaValue, projectionInput, typeRenderContract)
		if err != nil {
			return "", "", fmt.Errorf("%s/requestBody/content/%s/schema: %w", path, mediaType, err)
		}
	}
	schemaSource, err := runtimeJSONExpression(schemaValue)
	if err != nil {
		return "", "", fmt.Errorf("%s/requestBody/content/%s/schema: encode validator schema: %w", path, mediaType, err)
	}
	wireSchema, err := wireSchemaDescriptorForDocument(document, schemaValue, projectionInput)
	if err != nil {
		return "", "", fmt.Errorf("%s/requestBody/content/%s/schema: %w", path, mediaType, err)
	}
	itemContentType := ""
	if itemEncoding, _ := media["itemEncoding"].(map[string]any); itemEncoding != nil {
		itemContentType, _ = itemEncoding["contentType"].(string)
	}
	if stream {
		value = "AsyncIterable<" + value + ">"
	}
	plan := "{ contentType: " + quoteTS(mediaType) + ", binary: " + fmt.Sprint(binary) + ", stream: " + fmt.Sprint(stream) + ", itemContentType: " + quoteTS(itemContentType) + ", schema: " + schemaSource + ", wireSchema: " + wireSchema + " }"
	encodings, err := requestBodyWireEncodings(document, media)
	if err != nil {
		return "", "", fmt.Errorf("%s/requestBody/content/%s/encoding: %w", path, mediaType, err)
	}
	if encodings != "" {
		plan = strings.TrimSuffix(plan, " }") + ", encoding: " + encodings + " }"
	}
	return value, plan, nil
}

func isInboundRuntimeMediaType(mediaType string, schema map[string]any) bool {
	return isStreamMediaType(mediaType) || isJSONMediaType(mediaType) || isTextMedia(mediaType) || strings.Contains(strings.ToLower(mediaType), "xml") || strings.EqualFold(mediaType, "application/x-www-form-urlencoded") || strings.EqualFold(mediaType, "multipart/form-data") || isBinaryMedia(mediaType, schema)
}

func inboundParameterDefinitions(document *ir.Document, pathItem, operation map[string]any, path string, allowPath bool) (string, string, error) {
	parameters, err := operationParameters(document, ir.Operation{PathItemRaw: pathItem, Raw: operation})
	if err != nil {
		return "", "", fmt.Errorf("%s/parameters: %w", path, err)
	}
	entries := make([]string, 0, len(parameters))
	locations := map[string][]string{
		"path": {}, "query": {}, "querystring": {}, "headerParams": {}, "cookieParams": {},
	}
	for _, parameter := range parameters {
		if parameter.Location == "path" && !allowPath {
			return "", "", fmt.Errorf("%s/parameters/%s: server add-on requires host route path binding for inbound path parameters", path, parameter.Name)
		}
		schema, err := runtimeJSONExpression(parameter.Schema)
		if err != nil {
			return "", "", fmt.Errorf("%s/parameters/%s: encode schema: %w", path, parameter.Name, err)
		}
		wireSchema, err := wireSchemaDescriptorForDocument(document, parameter.Schema, projectionInput)
		if err != nil {
			return "", "", fmt.Errorf("%s/parameters/%s: encode wire schema: %w", path, parameter.Name, err)
		}
		entry := "{ location: " + quoteTS(parameter.Location) + ", name: " + quoteTS(parameter.Name) + ", property: " + quoteTS(parameter.Property) + ", style: " + quoteTS(parameter.Style) + ", explode: " + fmt.Sprint(parameter.Explode) + ", allowReserved: " + fmt.Sprint(parameter.AllowReserved) + ", required: " + fmt.Sprint(parameter.Required) + ", schema: " + schema + ", wireSchema: " + wireSchema
		if parameter.ContentType != "" {
			entry += ", contentType: " + quoteTS(parameter.ContentType)
		}
		entries = append(entries, entry+" }")
		valueType, err := schemaTypeForScope(document, parameter.Schema, projectionInput, typeRenderContract)
		if err != nil {
			return "", "", fmt.Errorf("%s/parameters/%s: render input type: %w", path, parameter.Name, err)
		}
		location := parameter.Location
		if location == "header" {
			location = "headerParams"
		} else if location == "cookie" {
			location = "cookieParams"
		}
		optional := "?"
		if parameter.Required {
			optional = ""
		}
		locations[location] = append(locations[location], "readonly "+quoteTS(parameter.Property)+optional+": "+valueType)
	}
	fields := make([]string, 0, 5)
	for _, location := range []string{"path", "query", "querystring", "headerParams", "cookieParams"} {
		fields = append(fields, "readonly "+location+": Readonly<{ "+strings.Join(locations[location], "; ")+" }>")
	}
	return "[" + strings.Join(entries, ", ") + "]", "Readonly<{ " + strings.Join(fields, "; ") + " }>", nil
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
	values := make(map[string]any, len(document.ComponentSchemas)+len(document.Schemas))
	for name, schema := range document.ComponentSchemas {
		values[name] = schema
	}
	for name, schema := range document.Schemas {
		values[name] = schema.Value
	}
	schemas, err := runtimeJSONExpression(values)
	if err != nil {
		return fmt.Errorf("encode inbound component schemas: %w", err)
	}
	fmt.Fprintf(output, "const inputSchemas: InboundSchemas = %s\n\n", schemas)
	return nil
}

func emitInboundSecuritySchemes(output *bytes.Buffer, document *ir.Document) error {
	components, _ := document.Raw["components"].(map[string]any)
	schemes, _ := components["securitySchemes"].(map[string]any)
	encoded, err := runtimeJSONExpression(schemes)
	if err != nil {
		return fmt.Errorf("encode inbound security schemes: %w", err)
	}
	fmt.Fprintf(output, "const securitySchemes: InboundSecuritySchemes = %s\n\n", encoded)
	return nil
}

func emitWebhooks(document *ir.Document, webhooks []webhookDefinition) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("import { collectInboundSecurityCandidates, decodeInboundBody, decodeInboundParameters, matchInboundRoute, InboundRequestError, normalizeInboundMediaCodecs, requiresInboundAuthentication, responseFromHandler, type Authenticate, type InboundParameterValues, type InboundRequestContext, type InboundResponse, type InboundParameterDefinition, type InboundSchemas, type InboundSecuritySchemes } from \"./runtime.js\"\n")
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
		fmt.Fprintf(&output, "interface %sContext extends InboundRequestContext {\n  readonly params: %s\n", webhook.typeName, webhook.paramsType)
		if webhook.hasBody {
			fmt.Fprintf(&output, "  readonly body: %s\n", webhook.bodyType)
		} else {
			output.WriteString("  readonly body: undefined\n")
		}
		output.WriteString("}\n")
		fmt.Fprintf(&output, "type %sResponse = %s\n\n", webhook.typeName, webhook.responseType)
	}
	output.WriteString("/** Type catalog keyed by exact root Webhook Object name and HTTP method. */\n")
	output.WriteString("export interface Webhooks {\n")
	for _, name := range webhookHandlerProperties(webhooks) {
		fmt.Fprintf(&output, "  readonly %s: {\n", quoteTS(name))
		for _, webhook := range webhooksForProperty(webhooks, name) {
			fmt.Fprintf(&output, "    readonly %s: { readonly context: %sContext; readonly input: %sContext; readonly output: %sResponse; readonly response: %sResponse; readonly handler: (context: %sContext) => %sResponse | Promise<%sResponse>; readonly endpoint: WebhookRouter }\n", quoteTS(webhook.method), webhook.typeName, webhook.typeName, webhook.typeName, webhook.typeName, webhook.typeName, webhook.typeName, webhook.typeName)
		}
		output.WriteString("  }\n")
	}
	output.WriteString("}\n\n")
	output.WriteString("/** Handlers keyed by each OpenAPI root Webhook Object name. */\n")
	output.WriteString("export interface WebhookHandlers {\n")
	for _, name := range webhookHandlerProperties(webhooks) {
		fmt.Fprintf(&output, "  readonly %s?: {\n", quoteTS(name))
		for _, webhook := range webhooksForProperty(webhooks, name) {
			slot := "Webhooks[" + quoteTS(name) + "][" + quoteTS(webhook.method) + "]"
			fmt.Fprintf(&output, "    readonly %s?: %s[\"handler\"]\n", quoteTS(webhook.method), slot)
		}
		output.WriteString("  }\n")
	}
	output.WriteString("}\n\n")
	output.WriteString("/** Concrete host paths keyed by generated Webhook handler name. */\n")
	output.WriteString("export type WebhookRoutes = Readonly<Partial<Record<keyof WebhookHandlers, string>>>\n\n")
	output.WriteString("/** Options for a Fetch-native generated Webhook router. */\n")
	output.WriteString("export interface WebhookRouterOptions {\n  readonly routes: WebhookRoutes\n  readonly authenticate?: Authenticate | undefined\n  readonly codecs?: Readonly<Record<string, MediaCodec<unknown>>> | undefined\n  readonly maxStreamItemBytes?: number | undefined\n}\n\n")
	output.WriteString("/** Fetch-compatible generated inbound Webhook router. */\n")
	output.WriteString("export interface WebhookRouter {\n  fetch(request: Request): Promise<Response>\n}\n\n")
	for _, webhook := range webhooks {
		security, err := runtimeJSONExpression(webhook.security)
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
		fmt.Fprintf(&output, "  if (handlers[%s]?.[%s] !== undefined) {\n", quoteTS(webhook.name), quoteTS(webhook.method))
		fmt.Fprintf(&output, "    const path = routes[%s]\n", quoteTS(webhook.name))
		fmt.Fprintf(&output, "    if (typeof path !== \"string\" || !path.startsWith(\"/\") || path.includes(\"?\") || path.includes(\"#\")) throw new TypeError(%s)\n", quoteTS("Webhook route for "+webhook.name+" must be an absolute path without query or fragment"))
		fmt.Fprintf(&output, "    const key = %s + \" \" + path\n", quoteTS(webhook.method))
		fmt.Fprintf(&output, "    if (registrations.has(key)) throw new TypeError(%s + key)\n", quoteTS("Duplicate generated Webhook route: "))
		output.WriteString("    registrations.add(key)\n  }\n")
	}
	output.WriteString("  return {\n    async fetch(request: Request): Promise<Response> {\n      const pathname = new URL(request.url).pathname\n")
	for _, webhook := range webhooks {
		fmt.Fprintf(&output, "      const %sPathParameters = matchInboundRoute(routes[%s], pathname)\n      if (handlers[%s]?.[%s] !== undefined && request.method === %s && %sPathParameters !== undefined) {\n", webhookDefinitionSymbol(webhook), quoteTS(webhook.name), quoteTS(webhook.name), quoteTS(webhook.method), quoteTS(webhook.method), webhookDefinitionSymbol(webhook))
		fmt.Fprintf(&output, "        const handler = handlers[%s]?.[%s]\n", quoteTS(webhook.name), quoteTS(webhook.method))
		output.WriteString("        if (handler === undefined) return new Response(\"Not Found\", { status: 404 })\n")
		symbol := webhookDefinitionSymbol(webhook)
		fmt.Fprintf(&output, "        let params: %s\n        try { params = await decodeInboundParameters(request, %s.parameters, inputSchemas, inputWireSchemas, inboundCodecs, %sPathParameters) as %s } catch (error) { if (error instanceof InboundRequestError) return error.response; throw error }\n        const context = { request, operationID: %s.operationID, method: %s.method, path: pathname, params, security: %s.security, securityCandidates: collectInboundSecurityCandidates(request, %s.security, securitySchemes) } as Omit<%sContext, \"body\">\n", webhook.paramsType, symbol, symbol, webhook.paramsType, symbol, symbol, symbol, symbol, webhook.typeName)
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
	return stablePrivateIdentifier("webhook-definition", webhook.name+"\x00"+webhook.method)
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
	return []byte(`import { decodeWireValue, decodeXML, defineOwnDataProperty, encodeWireValue, encodeXML, validateWireValue, type MediaCodec, type MediaStreamReader, type WireEncodingDefinition, type WireHeaderDefinition, type WireProperty, type WireSchema, type WireSchemas } from "../generated/runtime.js"

/** Metadata provided to host-owned inbound authentication policy. */
export interface InboundRequestContext {
  readonly request: Request
  readonly operationID: string
  readonly method: string
  readonly path: string
  /** Decoded parameters grouped by location and keyed by exact OpenAPI names. */
  readonly params: InboundParameterValues
  readonly security: unknown
  readonly securityCandidates: Readonly<Record<string, InboundSecurityCandidate>>
}

/** Exact decoded inbound parameter containers, separated by OpenAPI location. */
export interface InboundParameterValues {
  readonly path: Readonly<Record<string, unknown>>
  readonly query: Readonly<Record<string, unknown>>
  readonly querystring: Readonly<Record<string, unknown>>
  readonly headerParams: Readonly<Record<string, unknown>>
  readonly cookieParams: Readonly<Record<string, unknown>>
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
export async function decodeInboundParameters(request: Request, definitions: readonly InboundParameterDefinition[], schemas: InboundSchemas, wireSchemas: WireSchemas, codecs: ReadonlyMap<string, MediaCodec<unknown>> | undefined = undefined, pathParameters: Readonly<Record<string, string>> = {}): Promise<InboundParameterValues> {
  const result = {
    path: Object.create(null) as Record<string, unknown>,
    query: Object.create(null) as Record<string, unknown>,
    querystring: Object.create(null) as Record<string, unknown>,
    headerParams: Object.create(null) as Record<string, unknown>,
    cookieParams: Object.create(null) as Record<string, unknown>,
  }
  const url = new URL(request.url)
  const cookies = parseInboundCookies(request.headers.get("cookie"))
  for (const definition of definitions) {
    let raw: unknown = definition.location === "path" ? pathParameters[definition.name] : definition.location === "header" ? request.headers.get(definition.name) : definition.location === "cookie" ? (definition.style === "cookie" ? cookies.raw[definition.name] : cookies.decoded[definition.name]) : definition.location === "querystring" ? url.search.slice(1) : url.searchParams.getAll(definition.name)
    if (definition.contentType === undefined && definition.location === "query" && (definition.style === "deepObject" || definition.style === "form" && definition.explode) && inboundSchemaDescribesObject(resolveInboundSchema(definition.schema, schemas))) raw = decodeInboundQueryObject(url, definition, schemas, wireSchemas)
    if (definition.contentType === undefined && definition.location === "cookie" && definition.style === "cookie" && definition.explode && inboundSchemaDescribesObject(resolveInboundSchema(definition.schema, schemas))) raw = decodeInboundCookieObject(cookies.raw, definition, schemas, wireSchemas)
    const absent = Array.isArray(raw) ? raw.length === 0 : isRecord(raw) ? Object.keys(raw).length === 0 : raw === undefined || raw === null
    if (absent) {
      if (definition.required) throw new InboundRequestError(new Response("Missing required parameter " + definition.name, { status: 400 }))
      continue
    }
    const value = await decodeInboundParameterContent(raw, definition, schemas, wireSchemas, codecs)
    try {
      validateWireValue(value, definition.wireSchema, wireSchemas, "decode")
      const section = definition.location === "header" ? result.headerParams : definition.location === "cookie" ? result.cookieParams : result[definition.location]
      defineOwnDataProperty(section, definition.property, decodeWireValue(value, definition.wireSchema, wireSchemas))
    } catch (error) {
      throw new InboundRequestError(new Response("Invalid parameter " + definition.name + ": " + (error instanceof Error ? error.message : "invalid value"), { status: 400 }))
    }
  }
  return result
}

async function decodeInboundParameterContent(raw: unknown, definition: InboundParameterDefinition, schemas: InboundSchemas, wireSchemas: WireSchemas, codecs: ReadonlyMap<string, MediaCodec<unknown>> | undefined): Promise<unknown> {
  if (isRecord(raw)) return raw
  if (typeof raw !== "string" && !Array.isArray(raw)) return raw
  let normalized = raw as string | readonly string[]
  if (typeof normalized === "string" && definition.location === "path") {
    if (definition.style === "label" && normalized.startsWith(".")) normalized = normalized.slice(1)
    if (definition.style === "matrix" && normalized.startsWith(";")) {
      const prefix = ";" + definition.name + "="
      if (normalized.startsWith(prefix)) normalized = normalized.slice(prefix.length)
    }
  }
  const contentType = normalizeInboundMediaType(definition.contentType ?? "")
  const source = typeof normalized === "string" ? normalized : normalized[0]
  if ((contentType === "application/json" || contentType.endsWith("+json")) && source !== undefined) {
    try { return JSON.parse(source) } catch { throw new InboundRequestError(new Response("Invalid JSON parameter " + definition.name, { status: 400 })) }
  }
  if (contentType.includes("xml") && source !== undefined) {
    try { return decodeInboundXML(source, definition.schema, schemas, definition.wireSchema, wireSchemas) }
    catch { throw new InboundRequestError(new Response("Invalid XML parameter " + definition.name, { status: 400 })) }
  }
  if (contentType === "application/x-www-form-urlencoded" && source !== undefined) return decodeInboundParameterForm(source, definition.schema, schemas, definition.wireSchema, wireSchemas)
  if (contentType !== "" && !contentType.startsWith("text/") && !isInboundBinaryMedia(contentType, definition.schema)) {
    const codec = inboundMediaCodec(codecs, contentType)
    if (codec?.decodeParameter === undefined) throw new InboundRequestError(new Response("Unsupported Media Type", { status: 415 }))
    return codec.decodeParameter(source ?? "", { contentType })
  }
  const schema = inboundSchemaRecord(resolveInboundSchema(definition.schema, schemas))
  if (inboundSchemaDescribesObject(schema)) return decodeInboundSerializedObject(source, definition, schema, schemas, wireSchemas)
  return decodeInboundParameterValue(normalized, definition.schema, schemas, definition.wireSchema, wireSchemas)
}

async function decodeInboundParameterForm(source: string, schema: InboundSchema, schemas: InboundSchemas, wireSchema: WireSchema, wireSchemas: WireSchemas): Promise<unknown> {
  const parsed = new URLSearchParams(source)
  const value = Object.create(null) as Record<string, unknown>
  for (const [name, entry] of parsed) {
    const previous = value[name]
    defineOwnDataProperty(value, name, previous === undefined ? entry : Array.isArray(previous) ? [...previous, entry] : [previous, entry])
  }
  return decodeInboundFormValue(value, schema, schemas, wireSchema, wireSchemas)
}

/** Reconstructs an object encoded with OpenAPI simple, label, matrix, or form parameter styles. */
function decodeInboundSerializedObject(source: string | undefined, definition: InboundParameterDefinition, schema: Readonly<Record<string, unknown>>, schemas: InboundSchemas, wireSchemas: WireSchemas): Readonly<Record<string, unknown>> {
  if (source === undefined) return Object.create(null) as Readonly<Record<string, unknown>>
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
  const raw = Object.create(null) as Record<string, string>
  for (const [name, value] of pairs) {
    defineOwnDataProperty(raw, name, value)
  }
  return decodeInboundParameterObjectValue(raw, schema, schemas, definition.wireSchema, wireSchemas, true)
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

function decodeInboundQueryObject(url: URL, definition: InboundParameterDefinition, schemas: InboundSchemas, wireSchemas: WireSchemas): Readonly<Record<string, unknown>> {
  const schema = inboundSchemaRecord(resolveInboundSchema(definition.schema, schemas))
  const properties = isRecord(schema["properties"]) ? schema["properties"] : {}
  const raw = Object.create(null) as Record<string, string | readonly string[]>
  if (definition.style === "deepObject") {
    const prefix = definition.name + "["
    for (const [name, value] of url.searchParams) {
      if (!name.startsWith(prefix) || !name.endsWith("]")) continue
      const property = name.slice(prefix.length, -1)
      defineOwnDataProperty(raw, property, value)
    }
    return decodeInboundParameterObjectValue(raw, schema, schemas, definition.wireSchema, wireSchemas, true)
  }
  const names = new Set([...Object.keys(properties), ...url.searchParams.keys()])
  for (const property of names) {
    const values = url.searchParams.getAll(property)
    if (values.length === 0) continue
    defineOwnDataProperty(raw, property, values)
  }
  return decodeInboundParameterObjectValue(raw, schema, schemas, definition.wireSchema, wireSchemas, false)
}

/** Reconstructs an OpenAPI 3.2 cookie-style exploded object without URI decoding cookie text. */
function decodeInboundCookieObject(cookies: Readonly<Record<string, string | readonly string[]>>, definition: InboundParameterDefinition, schemas: InboundSchemas, wireSchemas: WireSchemas): Readonly<Record<string, unknown>> {
  const schema = inboundSchemaRecord(resolveInboundSchema(definition.schema, schemas))
  const properties = isRecord(schema["properties"]) ? schema["properties"] : {}
  const raw = Object.create(null) as Record<string, string | readonly string[]>
  const names = new Set([...Object.keys(properties), ...Object.keys(cookies)])
  for (const property of names) {
    const value = cookies[property]
    if (value === undefined) continue
    defineOwnDataProperty(raw, property, value)
  }
  return decodeInboundParameterObjectValue(raw, schema, schemas, definition.wireSchema, wireSchemas, false)
}

/** Matches a host webhook route template and returns decoded parameter values. */
export function matchInboundRoute(template: string | undefined, pathname: string): Readonly<Record<string, string>> | undefined {
  if (template === undefined || !template.startsWith("/")) return undefined
  const expected = template.split("/").slice(1)
  const actual = pathname.split("/").slice(1)
  if (expected.length !== actual.length) return undefined
  const result = Object.create(null) as Record<string, string>
  for (let index = 0; index < expected.length; index++) {
    const segment = expected[index]!
    const value = actual[index]!
    const match = /^\{([^{}\/]+)\}$/.exec(segment)
    if (match === null) { if (segment !== value) return undefined; continue }
    try { defineOwnDataProperty(result, match[1]!, decodeURIComponent(value)) }
    catch { return undefined }
  }
  return result
}

function decodeInboundParameterValue(raw: string | readonly string[], schema: InboundSchema, schemas: InboundSchemas, wireSchema: WireSchema | undefined = undefined, wireSchemas: WireSchemas = {}): unknown {
  if (wireSchema !== undefined) {
    wireSchema = materializeInboundWireSchema(wireSchema, wireSchemas)
    let fallback: unknown = Array.isArray(raw) ? raw[0] : raw
    for (const alternative of inboundWireSchemaAlternatives(wireSchema, wireSchemas)) {
      const candidate = decodeInboundParameterValueForWireSchema(raw, alternative, wireSchemas)
      fallback = candidate
      try { validateWireValue(candidate, wireSchema, wireSchemas, "decode"); return candidate }
      catch { /* Try the next correlated schema alternative. */ }
    }
    return fallback
  }
  const descriptor = inboundSchemaRecord(resolveInboundSchema(schema, schemas))
  const values = Array.isArray(raw) ? raw : [raw]
  const types = inboundSchemaTypes(descriptor, schemas)
  const candidates: unknown[] = []
  const value = values[0]!
  if (types.includes("string")) candidates.push(value)
  if (types.includes("boolean") && (value === "true" || value === "false")) candidates.push(value === "true")
  if (types.includes("integer")) { const number = Number(value); if (Number.isInteger(number)) candidates.push(number) }
  if (types.includes("number")) { const number = Number(value); if (Number.isFinite(number)) candidates.push(number) }
  if (types.includes("array")) {
    const entries = values.flatMap((value) => value.split(","))
    const array = entries.map((entry, index) => decodeInboundParameterValue(entry, inboundArrayItemSchema(descriptor, index), schemas, wireSchema === undefined ? undefined : inboundWireArrayItemSchema(wireSchema, index, wireSchemas), wireSchemas))
    if (values.length > 1) candidates.unshift(array)
    else candidates.push(array)
  }
  if (candidates.length === 0) candidates.push(value)
  return candidates[0]
}

function decodeInboundParameterValueForWireSchema(raw: string | readonly string[], schema: WireSchema, wireSchemas: WireSchemas): unknown {
  const values = Array.isArray(raw) ? raw : [raw]
  const value = values[0]!
  const types = inboundWireSchemaTypes(schema, wireSchemas)
  const candidates: unknown[] = [value]
  if (types.includes("boolean") && (value === "true" || value === "false")) candidates.push(value === "true")
  if (types.includes("integer")) { const number = Number(value); if (Number.isInteger(number)) candidates.push(number) }
  if (types.includes("number")) { const number = Number(value); if (Number.isFinite(number)) candidates.push(number) }
  if (types.includes("array")) {
    const entries = values.flatMap((entry) => entry.split(","))
    const array = entries.map((entry, index) => {
      const item = inboundWireArrayItemSchema(schema, index, wireSchemas)
      return item === undefined ? entry : decodeInboundParameterValue(entry, {}, {}, item, wireSchemas)
    })
    if (values.length > 1) candidates.unshift(array)
    else candidates.push(array)
  }
  if (candidates.length === 0) candidates.push(value)
  for (const candidate of candidates) {
    try { validateWireValue(candidate, schema, wireSchemas, "decode"); return candidate }
    catch { /* Try the next lossless representation. */ }
  }
  return candidates[0]
}

function decodeInboundParameterObjectValue(
  raw: Readonly<Record<string, string | readonly string[]>>,
  schema: InboundSchema,
  schemas: InboundSchemas,
  wireSchema: WireSchema,
  wireSchemas: WireSchemas,
  preserveUnknown: boolean,
): Readonly<Record<string, unknown>> {
  wireSchema = materializeInboundWireSchema(wireSchema, wireSchemas)
  let fallback = Object.create(null) as Record<string, unknown>
  for (const alternative of inboundWireSchemaAlternatives(wireSchema, wireSchemas)) {
    const result = Object.create(null) as Record<string, unknown>
    for (const [name, value] of Object.entries(raw)) {
      const property = inboundWirePropertySchema(alternative, name, wireSchemas) ?? inboundWirePropertySchema(wireSchema, name, wireSchemas)
      if (property === undefined && !preserveUnknown) continue
      defineOwnDataProperty(result, name, property === undefined ? value : decodeInboundParameterValue(value, schema, schemas, property, wireSchemas))
    }
    fallback = result
    try { validateWireValue(result, wireSchema, wireSchemas, "decode"); return result }
    catch { /* Try the next correlated object alternative. */ }
  }
  return fallback
}

function inboundSchemaDescribesObject(schema: InboundSchema): boolean {
  const descriptor = inboundSchemaRecord(schema)
  return schemaAcceptsType(descriptor["type"], "object") || isRecord(descriptor["properties"]) || isRecord(descriptor["patternProperties"]) || isInboundSchema(descriptor["additionalProperties"])
}

function inboundPropertySchema(schema: Readonly<Record<string, unknown>>, name: string): InboundSchema | undefined {
  const properties = isRecord(schema["properties"]) ? schema["properties"] : {}
  let result = isInboundSchema(properties[name]) ? properties[name] : undefined
  let matched = result !== undefined
  const patterns = isRecord(schema["patternProperties"]) ? schema["patternProperties"] : {}
  for (const [pattern, candidate] of Object.entries(patterns)) {
    if (!isInboundSchema(candidate) || !(new RegExp(pattern, "u")).test(name)) continue
    result = result === undefined ? candidate : mergeInboundSchemaValues(result, candidate)
    matched = true
  }
  if (!matched && isInboundSchema(schema["additionalProperties"])) result = schema["additionalProperties"]
  return result
}

function inboundSchemaTypes(schema: InboundSchema, schemas: InboundSchemas, seen: ReadonlySet<object> = new Set()): readonly string[] {
  if (!isRecord(schema) || seen.has(schema)) return []
  const nestedSeen = new Set(seen); nestedSeen.add(schema)
  const descriptor = inboundSchemaRecord(resolveInboundSchema(schema, schemas))
  const result = new Set<string>()
  const declared = Array.isArray(descriptor["type"]) ? descriptor["type"] : [descriptor["type"]]
  for (const value of declared) if (typeof value === "string") result.add(value)
  for (const keyword of ["allOf", "oneOf", "anyOf"]) {
    const variants = Array.isArray(descriptor[keyword]) ? descriptor[keyword] : []
    for (const variant of variants) if (isInboundSchema(variant)) for (const value of inboundSchemaTypes(variant, schemas, nestedSeen)) result.add(value)
  }
  return [...result]
}

function inboundArrayItemSchema(schema: Readonly<Record<string, unknown>>, index: number): InboundSchema {
  const prefixItems = Array.isArray(schema["prefixItems"]) ? schema["prefixItems"] : []
  if (isInboundSchema(prefixItems[index])) return prefixItems[index]
  return isInboundSchema(schema["items"]) ? schema["items"] : {}
}

function combineInboundWireSchemas(schemas: readonly WireSchema[]): WireSchema | undefined {
  if (schemas.length === 0) return undefined
  return schemas.length === 1 ? schemas[0] : { allOf: schemas }
}

function materializeInboundWireSchema(
  schema: WireSchema,
  schemas: WireSchemas,
  dynamicScope: readonly WireSchema[] = [],
  seen: ReadonlySet<WireSchema> = new Set(),
): WireSchema {
  if (seen.has(schema)) return schema
  const nestedSeen = new Set(seen); nestedSeen.add(schema)
  const scope = schema.dynamicAnchor === undefined ? dynamicScope : [...dynamicScope, schema]
  const result = { ...schema } as {
    reference?: string
    dynamicReference?: WireSchema["dynamicReference"]
    properties?: Readonly<Record<string, WireProperty>>
    patternProperties?: Readonly<Record<string, WireSchema>>
    dependentSchemas?: Readonly<Record<string, WireSchema>>
    items?: WireSchema
    prefixItems?: readonly WireSchema[]
    additionalProperties?: WireSchema | false
    unevaluatedProperties?: WireSchema | false
    unevaluatedItems?: WireSchema | false
    allOf?: readonly WireSchema[]
    oneOf?: readonly WireSchema[]
    anyOf?: readonly WireSchema[]
    contains?: WireSchema
    not?: WireSchema
    if?: WireSchema
    then?: WireSchema
    else?: WireSchema
    contentSchema?: WireSchema
  }
  const conjunctions = [...(schema.allOf ?? []).map((branch) => materializeInboundWireSchema(branch, schemas, scope, nestedSeen))]
  if (schema.reference !== undefined && schemas[schema.reference] !== undefined) {
    conjunctions.push(materializeInboundWireSchema(schemas[schema.reference]!, schemas, scope, nestedSeen))
    delete result.reference
  }
  if (schema.dynamicReference !== undefined) {
    const target = scope.find((candidate) => candidate.dynamicAnchor === schema.dynamicReference!.anchor) ?? schema.dynamicReference.fallback
    conjunctions.push(materializeInboundWireSchema(target, schemas, scope, nestedSeen))
    delete result.dynamicReference
  }
  if (schema.properties !== undefined) {
    result.properties = Object.fromEntries(Object.entries(schema.properties).map(([name, definition]) => [name, {
      ...definition,
      schema: materializeInboundWireSchema(definition.schema, schemas, scope, nestedSeen),
    }]))
  }
  if (schema.patternProperties !== undefined) result.patternProperties = Object.fromEntries(Object.entries(schema.patternProperties).map(([pattern, value]) => [pattern, materializeInboundWireSchema(value, schemas, scope, nestedSeen)]))
  if (schema.dependentSchemas !== undefined) result.dependentSchemas = Object.fromEntries(Object.entries(schema.dependentSchemas).map(([name, value]) => [name, materializeInboundWireSchema(value, schemas, scope, nestedSeen)]))
  if (schema.items !== undefined) result.items = materializeInboundWireSchema(schema.items, schemas, scope, nestedSeen)
  if (schema.prefixItems !== undefined) result.prefixItems = schema.prefixItems.map((item) => materializeInboundWireSchema(item, schemas, scope, nestedSeen))
  if (schema.additionalProperties !== undefined && schema.additionalProperties !== false) result.additionalProperties = materializeInboundWireSchema(schema.additionalProperties, schemas, scope, nestedSeen)
  if (schema.unevaluatedProperties !== undefined && schema.unevaluatedProperties !== false) result.unevaluatedProperties = materializeInboundWireSchema(schema.unevaluatedProperties, schemas, scope, nestedSeen)
  if (schema.unevaluatedItems !== undefined && schema.unevaluatedItems !== false) result.unevaluatedItems = materializeInboundWireSchema(schema.unevaluatedItems, schemas, scope, nestedSeen)
  if (schema.oneOf !== undefined) result.oneOf = schema.oneOf.map((branch) => materializeInboundWireSchema(branch, schemas, scope, nestedSeen))
  if (schema.anyOf !== undefined) result.anyOf = schema.anyOf.map((branch) => materializeInboundWireSchema(branch, schemas, scope, nestedSeen))
  if (schema.contains !== undefined) result.contains = materializeInboundWireSchema(schema.contains, schemas, scope, nestedSeen)
  if (schema.not !== undefined) result.not = materializeInboundWireSchema(schema.not, schemas, scope, nestedSeen)
  if (schema.if !== undefined) result.if = materializeInboundWireSchema(schema.if, schemas, scope, nestedSeen)
  if (schema.then !== undefined) result.then = materializeInboundWireSchema(schema.then, schemas, scope, nestedSeen)
  if (schema.else !== undefined) result.else = materializeInboundWireSchema(schema.else, schemas, scope, nestedSeen)
  if (schema.contentSchema !== undefined) result.contentSchema = materializeInboundWireSchema(schema.contentSchema, schemas, scope, nestedSeen)
  result.allOf = conjunctions.length === 0 ? undefined : conjunctions
  return result
}

function inboundWireSchemaAlternatives(schema: WireSchema, schemas: WireSchemas, seen: ReadonlySet<WireSchema> = new Set()): readonly WireSchema[] {
  if (seen.has(schema)) return [{}]
  const nestedSeen = new Set(seen); nestedSeen.add(schema)
  const own = { ...schema } as {
    reference?: string
    allOf?: readonly WireSchema[]
    oneOf?: readonly WireSchema[]
    anyOf?: readonly WireSchema[]
  }
  delete own.reference
  delete own.allOf
  delete own.oneOf
  delete own.anyOf
  let alternatives: WireSchema[] = [own]
  const conjunctions: (readonly WireSchema[])[] = []
  if (schema.dynamicReference !== undefined) conjunctions.push(inboundWireSchemaAlternatives(schema.dynamicReference.fallback, schemas, nestedSeen))
  if (schema.reference !== undefined && schemas[schema.reference] !== undefined) conjunctions.push(inboundWireSchemaAlternatives(schemas[schema.reference]!, schemas, nestedSeen))
  for (const branch of schema.allOf ?? []) conjunctions.push(inboundWireSchemaAlternatives(branch, schemas, nestedSeen))
  for (const choices of [schema.oneOf, schema.anyOf]) {
    if (choices === undefined) continue
    conjunctions.push(choices.flatMap((branch) => inboundWireSchemaAlternatives(branch, schemas, nestedSeen)))
  }
  for (const choices of conjunctions) {
    alternatives = alternatives.flatMap((base) => choices.map((choice) => ({ allOf: [base, choice] })))
  }
  return alternatives
}

function inboundWireSchemaTypes(schema: WireSchema, schemas: WireSchemas, seen: ReadonlySet<WireSchema> = new Set()): readonly string[] {
  if (seen.has(schema)) return []
  const nestedSeen = new Set(seen); nestedSeen.add(schema)
  const result = new Set(schema.types ?? [])
  if (schema.constValue !== undefined) result.add(inboundWireValueType(schema.constValue))
  for (const value of schema.enumValues ?? []) result.add(inboundWireValueType(value))
  if (schema.prefixItems !== undefined || schema.items !== undefined || schema.contains !== undefined || schema.minItems !== undefined || schema.maxItems !== undefined || schema.uniqueItems !== undefined) result.add("array")
  if (schema.properties !== undefined || schema.patternProperties !== undefined || schema.additionalProperties !== undefined || schema.required !== undefined || schema.minProperties !== undefined || schema.maxProperties !== undefined) result.add("object")
  if (schema.dynamicReference !== undefined) {
    for (const value of inboundWireSchemaTypes(schema.dynamicReference.fallback, schemas, nestedSeen)) result.add(value)
  }
  if (schema.reference !== undefined && schemas[schema.reference] !== undefined) {
    for (const value of inboundWireSchemaTypes(schemas[schema.reference]!, schemas, nestedSeen)) result.add(value)
  }
  for (const branch of schema.allOf ?? []) {
    for (const value of inboundWireSchemaTypes(branch, schemas, nestedSeen)) result.add(value)
  }
  return [...result]
}

function inboundWireValueType(value: unknown): string {
  if (value === null) return "null"
  if (Array.isArray(value)) return "array"
  if (typeof value === "number") return Number.isInteger(value) ? "integer" : "number"
  if (typeof value === "object") return "object"
  return typeof value
}

function inboundWireArrayItemSchema(schema: WireSchema, index: number, schemas: WireSchemas, seen: ReadonlySet<WireSchema> = new Set()): WireSchema | undefined {
  if (seen.has(schema)) return undefined
  const nestedSeen = new Set(seen); nestedSeen.add(schema)
  const result: WireSchema[] = []
  const direct = schema.prefixItems?.[index] ?? schema.items
  if (direct !== undefined) result.push(direct)
  if (schema.dynamicReference !== undefined) {
    const dynamic = inboundWireArrayItemSchema(schema.dynamicReference.fallback, index, schemas, nestedSeen)
    if (dynamic !== undefined) result.push(dynamic)
  }
  if (schema.reference !== undefined && schemas[schema.reference] !== undefined) {
    const referenced = inboundWireArrayItemSchema(schemas[schema.reference]!, index, schemas, nestedSeen)
    if (referenced !== undefined) result.push(referenced)
  }
  for (const branch of schema.allOf ?? []) {
    const nested = inboundWireArrayItemSchema(branch, index, schemas, nestedSeen)
    if (nested !== undefined) result.push(nested)
  }
  for (const keyword of ["oneOf", "anyOf"] as const) {
    const branches = schema[keyword]
    if (branches === undefined) continue
    const nested = branches.map((branch) => inboundWireArrayItemSchema(branch, index, schemas, nestedSeen) ?? {})
    result.push(keyword === "oneOf" ? { oneOf: nested } : { anyOf: nested })
  }
  return combineInboundWireSchemas(result)
}

function inboundWirePropertySchema(schema: WireSchema, name: string, schemas: WireSchemas, seen: ReadonlySet<WireSchema> = new Set()): WireSchema | undefined {
  if (seen.has(schema)) return undefined
  const nestedSeen = new Set(seen); nestedSeen.add(schema)
  const result: WireSchema[] = []
  let matched = false
  const direct = schema.properties?.[name]?.schema
  if (direct !== undefined) { result.push(direct); matched = true }
  for (const [pattern, candidate] of Object.entries(schema.patternProperties ?? {})) {
    if (!(new RegExp(pattern, "u")).test(name)) continue
    result.push(candidate)
    matched = true
  }
  if (!matched && schema.additionalProperties !== undefined && schema.additionalProperties !== false) result.push(schema.additionalProperties)
  if (schema.dynamicReference !== undefined) {
    const dynamic = inboundWirePropertySchema(schema.dynamicReference.fallback, name, schemas, nestedSeen)
    if (dynamic !== undefined) result.push(dynamic)
  }
  if (schema.reference !== undefined && schemas[schema.reference] !== undefined) {
    const referenced = inboundWirePropertySchema(schemas[schema.reference]!, name, schemas, nestedSeen)
    if (referenced !== undefined) result.push(referenced)
  }
  for (const branch of schema.allOf ?? []) {
    const nested = inboundWirePropertySchema(branch, name, schemas, nestedSeen)
    if (nested !== undefined) result.push(nested)
  }
  for (const keyword of ["oneOf", "anyOf"] as const) {
    const branches = schema[keyword]
    if (branches === undefined) continue
    const nested = branches.map((branch) => inboundWirePropertySchema(branch, name, schemas, nestedSeen) ?? {})
    result.push(keyword === "oneOf" ? { oneOf: nested } : { anyOf: nested })
  }
  return combineInboundWireSchemas(result)
}

function inboundWirePropertyNames(schema: WireSchema, schemas: WireSchemas, seen: ReadonlySet<WireSchema> = new Set()): readonly string[] {
  if (seen.has(schema)) return []
  const nestedSeen = new Set(seen); nestedSeen.add(schema)
  const result = new Set(Object.keys(schema.properties ?? {}))
  if (schema.dynamicReference !== undefined) {
    for (const name of inboundWirePropertyNames(schema.dynamicReference.fallback, schemas, nestedSeen)) result.add(name)
  }
  if (schema.reference !== undefined && schemas[schema.reference] !== undefined) {
    for (const name of inboundWirePropertyNames(schemas[schema.reference]!, schemas, nestedSeen)) result.add(name)
  }
  for (const keyword of ["allOf", "oneOf", "anyOf"] as const) {
    for (const branch of schema[keyword] ?? []) {
      for (const name of inboundWirePropertyNames(branch, schemas, nestedSeen)) result.add(name)
    }
  }
  return [...result]
}

/** Coerces form field strings using the wire schema before inbound validation. */
async function decodeInboundFormValue(value: unknown, schema: InboundSchema | undefined, schemas: InboundSchemas, wireSchema: WireSchema | undefined = undefined, wireSchemas: WireSchemas = {}, encoding: readonly WireEncodingDefinition[] | undefined = undefined, contentType: string | undefined = undefined, codecs: ReadonlyMap<string, MediaCodec<unknown>> | undefined = undefined): Promise<unknown> {
  if (schema === undefined) return value
  if (wireSchema !== undefined) wireSchema = materializeInboundWireSchema(wireSchema, wireSchemas)
  const resolved = resolveInboundSchema(schema, schemas)
  const descriptor = inboundSchemaRecord(resolved)
  if (value instanceof Blob) {
    const normalized = normalizeInboundMediaType(contentType ?? value.type)
    if (normalized === "application/json" || normalized.endsWith("+json") || normalized.includes("xml") || codecs?.has(normalized) === true) return decodeInboundFormContent(await value.text(), resolved, schemas, wireSchema, wireSchemas, normalized, codecs)
    return value
  }
  if (value instanceof ArrayBuffer || ArrayBuffer.isView(value)) return value
  if (typeof value === "string" && contentType !== undefined) {
    const decoded = await decodeInboundFormContent(value, resolved, schemas, wireSchema, wireSchemas, contentType, codecs)
	if (decoded !== value) return decodeInboundFormValue(decoded, resolved, schemas, wireSchema, wireSchemas, encoding, undefined, codecs)
  }
  if (Array.isArray(value)) {
    if (wireSchema !== undefined) {
      let fallback: unknown = value
      for (const alternative of inboundWireSchemaAlternatives(wireSchema, wireSchemas)) {
        if (!inboundWireSchemaTypes(alternative, wireSchemas).includes("array")) continue
        const entries = value.flatMap((entry) => Array.isArray(entry) ? entry : [entry])
        const decoded = await Promise.all(entries.map((entry, index) => {
          const item = inboundWireArrayItemSchema(alternative, index, wireSchemas)
          return item === undefined ? entry : decodeInboundFormValue(entry, {}, schemas, item, wireSchemas, encoding, contentType, codecs)
        }))
        fallback = decoded
        try { validateWireValue(decoded, wireSchema, wireSchemas, "decode"); return decoded }
        catch { /* Try the next correlated array alternative. */ }
      }
      return fallback
    }
    if (!Array.isArray(descriptor["prefixItems"]) && !isInboundSchema(descriptor["items"])) return value
    const entries = value.flatMap((entry) => Array.isArray(entry) ? entry : [entry])
    return Promise.all(entries.map((entry, index) => decodeInboundFormValue(entry, inboundArrayItemSchema(descriptor, index), schemas, wireSchema === undefined ? undefined : inboundWireArrayItemSchema(wireSchema, index, wireSchemas), wireSchemas, encoding, contentType, codecs)))
  }
  if (isRecord(value)) {
    let fallback = Object.create(null) as Record<string, unknown>
    const alternatives: readonly (WireSchema | undefined)[] = wireSchema === undefined ? [undefined] : inboundWireSchemaAlternatives(wireSchema, wireSchemas)
    for (const alternative of alternatives) {
      const result = Object.create(null) as Record<string, unknown>
      for (const [name, entry] of Object.entries(value)) {
        const property = inboundPropertySchema(descriptor, name)
        const definition = encoding?.find((candidate) => candidate.name === name)
        const wireProperty = alternative === undefined ? undefined : inboundWirePropertySchema(alternative, name, wireSchemas) ?? (wireSchema === undefined ? undefined : inboundWirePropertySchema(wireSchema, name, wireSchemas))
        defineOwnDataProperty(result, name, wireSchema !== undefined && wireProperty === undefined
          ? entry
          : await decodeInboundFormValue(entry, isInboundSchema(property) ? property : {}, schemas, wireProperty, wireSchemas, definition?.encoding, definition?.contentType, codecs))
      }
      fallback = result
      if (wireSchema === undefined) return result
      try { validateWireValue(result, wireSchema, wireSchemas, "decode"); return result }
      catch { /* Try the next correlated object alternative. */ }
    }
    return fallback
  }
  if (typeof value === "string") return decodeInboundParameterValue(value, resolved, schemas, wireSchema, wireSchemas)
  return value
}

async function decodeInboundFormContent(value: unknown, schema: InboundSchema, schemas: InboundSchemas, wireSchema: WireSchema | undefined, wireSchemas: WireSchemas, contentType: string | undefined, codecs: ReadonlyMap<string, MediaCodec<unknown>> | undefined): Promise<unknown> {
  if (typeof value !== "string" || contentType === undefined) return value
  const normalized = normalizeInboundMediaType(contentType)
  if (normalized === "application/json" || normalized.endsWith("+json")) {
    try { return JSON.parse(value) } catch { throw new InboundRequestError(new Response("Invalid form JSON field", { status: 400 })) }
  }
  if (normalized.includes("xml")) {
    try { return decodeInboundXML(value, schema, schemas, wireSchema, wireSchemas) } catch { throw new InboundRequestError(new Response("Invalid form XML field", { status: 400 })) }
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
  const result = Object.create(null) as Record<string, InboundSecurityCandidate>
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
        defineOwnDataProperty(result, name, { scheme: name, type: "apiKey", location, name: parameterName, ...(value === undefined ? {} : { value }) })
        continue
      }
      const authorization = request.headers.get("authorization") ?? undefined
      defineOwnDataProperty(result, name, { scheme: name, type: scheme.type, ...(authorization === undefined ? {} : { value: authorization }) })
    }
  }
  return result
}

interface InboundCookies { readonly raw: Readonly<Record<string, string | readonly string[]>>; readonly decoded: Readonly<Record<string, string | readonly string[]>> }

function parseInboundCookies(header: string | null): InboundCookies {
  const raw = Object.create(null) as Record<string, string | string[]>
  const decoded = Object.create(null) as Record<string, string | string[]>
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
  defineOwnDataProperty(target, name, previous === undefined ? value : Array.isArray(previous) ? [...previous, value] : [previous, value])
}

function inboundCookieFirst(value: string | readonly string[] | undefined): string | undefined {
  return typeof value === "string" ? value : value?.[0]
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
export type InboundSchema = Readonly<Record<string, unknown>> | boolean
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
    const result = Object.create(null) as Record<string, unknown>
    for (const [name, item] of form) {
      const previous = result[name]
      defineOwnDataProperty(result, name, previous === undefined ? item : Array.isArray(previous) ? [...previous, item] : [previous, item])
    }
    value = await decodeInboundFormValue(result, options.schema, options.schemas, options.wireSchema, options.wireSchemas, options.encoding, undefined, options.codecs)
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
    const result = Object.create(null) as Record<string, unknown>
    for (const [name, item] of form) {
      const previous = result[name]
      defineOwnDataProperty(result, name, previous === undefined ? item : Array.isArray(previous) ? [...previous, item] : [previous, item])
    }
      value = await decodeInboundFormValue(result, options.schema, options.schemas, options.wireSchema, options.wireSchemas, options.encoding, undefined, options.codecs)
    } else if (contentType.includes("xml")) {
    try { value = decodeInboundXML(text, options.schema, options.schemas, options.wireSchema, options.wireSchemas) }
    catch (cause) { throw new InboundRequestError(new Response("Invalid XML", { status: 400 })) }
    } else value = text
  }
  validateInboundWireValue(value, options.wireSchema, options.wireSchemas, "request body")
  return decodeInboundWireValue(value, options.wireSchema, options.wireSchemas)
}

function decodeInboundWireValue(value: unknown, schema: WireSchema | undefined, schemas: WireSchemas | undefined): unknown {
  if (schema === undefined || value === undefined) return value
  try { return decodeWireValue(value, schema, schemas ?? {}) }
  catch { throw new InboundRequestError(new Response("Invalid request body", { status: 400 })) }
}

function validateInboundWireValue(value: unknown, schema: WireSchema | undefined, schemas: WireSchemas | undefined, label: string): void {
  if (schema === undefined || value === undefined) return
  try { validateWireValue(value, schema, schemas ?? {}, "decode") }
  catch (error) { throw new InboundRequestError(new Response("Invalid " + label + ": " + (error instanceof Error ? error.message : "invalid value"), { status: 400 })) }
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
      validateInboundWireValue(value, options.wireSchema, options.wireSchemas, "stream item")
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
  let pending: Uint8Array<ArrayBufferLike> = new Uint8Array()
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
    validateInboundWireValue(value, wireSchema, wireSchemas, "stream item")
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
  let pending: Uint8Array<ArrayBufferLike> = new Uint8Array()
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
    try { value = decodeInboundXML(new TextDecoder().decode(bytes), schema, schemas, wireSchema, wireSchemas) } catch { throw new InboundRequestError(new Response("Invalid multipart XML item", { status: 400 })) }
  } else if (isInboundBinaryMedia(normalized, schema)) {
    return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength)
  } else value = new TextDecoder().decode(bytes)
  validateInboundWireValue(value, wireSchema, wireSchemas, "multipart item")
  return decodeInboundWireValue(value, wireSchema, wireSchemas)
}

function isInboundBinaryMedia(contentType: string, schema: InboundSchema | undefined): boolean {
  const descriptor = inboundSchemaRecord(schema)
  return contentType === "application/octet-stream" || contentType.startsWith("image/") || contentType.startsWith("audio/") || contentType.startsWith("video/") || descriptor["format"] === "binary" || descriptor["contentEncoding"] === "binary"
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

interface InboundXMLNode { readonly name: string; readonly attributes: Readonly<Record<string, string>>; readonly children: InboundXMLNode[]; text: string; hasText: boolean }

function decodeInboundXML(source: string, schema: InboundSchema | undefined, schemas: InboundSchemas, wireSchema: WireSchema | undefined = undefined, wireSchemas: WireSchemas | undefined = undefined): unknown {
  const root = parseInboundXML(source)
  const schemaXML = isRecord(inboundSchemaRecord(schema)["xml"]) ? inboundSchemaRecord(schema)["xml"] as Readonly<Record<string, unknown>> : undefined
  const rootName = wireSchema?.reference ?? wireSchema?.xml?.name ?? (typeof schemaXML?.["name"] === "string" ? schemaXML["name"] : "root")
  if (wireSchema !== undefined) wireSchema = materializeInboundWireSchema(wireSchema, wireSchemas ?? {})
  return decodeInboundXMLNode(root, schema ?? {}, schemas, wireSchema, wireSchemas ?? {}, false, rootName)
}

function parseInboundXML(source: string): InboundXMLNode {
  const tokens = source.match(/<!\[CDATA\[[\s\S]*?\]\]>|<!--[\s\S]*?-->|<\?[^]*?\?>|<[^>]+>|[^<]+/g) ?? []
  const roots: InboundXMLNode[] = []
  const stack: InboundXMLNode[] = []
  for (const token of tokens) {
    if (token.startsWith("<!--") || token.startsWith("<?")) continue
    if (token.startsWith("<![CDATA[")) { if (stack.length === 0) throw new TypeError("XML character data is outside the document element"); stack[stack.length - 1]!.text += token.slice(9, -3); stack[stack.length - 1]!.hasText = true; continue }
    if (token.startsWith("<!")) throw new TypeError("XML declarations are not supported")
    if (token.startsWith("</")) { const name = token.slice(2, -1).trim(); const node = stack.pop(); if (node === undefined || node.name !== name) throw new TypeError("XML closing tag mismatch"); continue }
    if (token.startsWith("<")) {
      const closing = /\/>$/.test(token); const body = token.slice(1, closing ? -2 : -1).trim(); const match = /^([^\s/>]+)([\s\S]*)$/.exec(body)
      if (match === null) throw new TypeError("XML element has no name")
      const node: InboundXMLNode = { name: match[1]!, attributes: parseInboundXMLAttributes(match[2] ?? ""), children: [], text: "", hasText: false }
      if (stack.length === 0) roots.push(node); else stack[stack.length - 1]!.children.push(node)
      if (!closing) stack.push(node)
      continue
    }
    if (stack.length === 0) { if (token.trim() !== "") throw new TypeError("XML text is outside the document element"); continue }
    stack[stack.length - 1]!.text += unescapeInboundXML(token)
    stack[stack.length - 1]!.hasText = true
  }
  if (stack.length !== 0 || roots.length !== 1) throw new TypeError("XML document is not balanced")
  return roots[0]!
}

function parseInboundXMLAttributes(source: string): Readonly<Record<string, string>> {
  const result = Object.create(null) as Record<string, string>; const expression = /([^\s=]+)\s*=\s*("[^"]*"|'[^']*')/g; let match: RegExpExecArray | null
  while ((match = expression.exec(source)) !== null) {
    const name = match[1]!
    if (Object.hasOwn(result, name)) throw new TypeError("duplicate XML attribute " + name)
    defineOwnDataProperty(result, name, unescapeInboundXML(match[2]!.slice(1, -1)))
  }
  if (source.replace(expression, "").trim() !== "") throw new TypeError("XML attribute syntax is invalid")
  return result
}

function decodeInboundXMLNode(node: InboundXMLNode, schema: InboundSchema, schemas: InboundSchemas, wireSchema: WireSchema | undefined = undefined, wireSchemas: WireSchemas = {}, correlated: boolean = false, rootName: string = "root"): unknown {
  if (wireSchema !== undefined && !correlated) {
    wireSchema = materializeInboundWireSchema(wireSchema, wireSchemas)
    let fallback: unknown
    let failure: unknown
    for (const alternative of inboundWireSchemaAlternatives(wireSchema, wireSchemas)) {
      try {
        const value = decodeInboundXMLNode(node, schema, schemas, alternative, wireSchemas, true, rootName)
        fallback = value
        validateWireValue(value, wireSchema, wireSchemas, "decode")
        return value
      } catch (error) { failure = error }
    }
    if (failure !== undefined) throw failure
    return fallback
  }
  const resolved = inboundSchemaRecord(resolveInboundSchema(schema, schemas))
  if (schemaAcceptsType(resolved["type"], "array") || wireSchema !== undefined && inboundWireSchemaTypes(wireSchema, wireSchemas).includes("array")) {
    const xml = isRecord(resolved["xml"]) ? resolved["xml"] : wireSchema?.xml ?? {}
    if (xml["wrapped"] === true) {
      if (node.name !== inboundXMLQualifiedName(xml, rootName)) throw new TypeError("unexpected XML array wrapper " + node.name)
      for (const name of Object.keys(node.attributes)) {
        if (name !== "xmlns" && !name.startsWith("xmlns:")) throw new TypeError("unexpected XML array wrapper attribute " + name)
      }
      if (node.text.trim() !== "") throw new TypeError("unexpected XML array wrapper text")
    }
    return node.children.map((child, index) => {
      const item = inboundArrayItemSchema(resolved, index)
      const resolvedItem = resolveInboundSchema(item, schemas)
      const wireItem = wireSchema === undefined ? undefined : inboundWireArrayItemSchema(wireSchema, index, wireSchemas)
      const itemDescriptor = inboundSchemaRecord(resolvedItem)
      const itemXML = isRecord(itemDescriptor["xml"]) ? itemDescriptor["xml"] : wireItem?.xml ?? {}
      const parentItemFallbackName = xml["wrapped"] === true ? rootName : inboundXMLQualifiedName(xml, rootName)
      const itemFallbackName = typeof itemXML?.name === "string" ? itemXML.name : parentItemFallbackName
      if (xml["wrapped"] === true && child.name !== inboundXMLQualifiedName(itemXML, rootName)) throw new TypeError("unexpected XML array item " + child.name)
      return decodeInboundXMLNode(child, resolvedItem, schemas, wireItem, wireSchemas, false, itemFallbackName)
    })
  }
  const properties = isRecord(resolved["properties"]) ? resolved["properties"] : {}
  const wirePropertyNames = wireSchema === undefined ? [] : inboundWirePropertyNames(wireSchema, wireSchemas)
  if (schemaAcceptsType(resolved["type"], "object") || Object.keys(properties).length !== 0 || wireSchema !== undefined && (inboundWireSchemaTypes(wireSchema, wireSchemas).includes("object") || wirePropertyNames.length !== 0)) {
    const result = Object.create(null) as Record<string, unknown>
    const consumedAttributes = new Set<string>()
    const consumedChildren = new Set<InboundXMLNode>()
    for (const name of new Set([...Object.keys(properties), ...wirePropertyNames])) {
      const childSchema = isInboundSchema(properties[name]) ? properties[name] : {}
      const resolvedChild = resolveInboundSchema(childSchema, schemas)
      const childDescriptor = inboundSchemaRecord(resolvedChild)
      const wireProperty = wireSchema === undefined ? undefined : inboundWirePropertySchema(wireSchema, name, wireSchemas)
      const xml = isRecord(childDescriptor["xml"]) ? childDescriptor["xml"] : wireProperty?.xml ?? {}
      const xmlName = inboundXMLQualifiedName(xml, name)
      if (xml["attribute"] === true || xml["nodeType"] === "attribute") {
        if (node.attributes[xmlName] !== undefined) {
          consumedAttributes.add(xmlName)
          defineOwnDataProperty(result, name, decodeInboundXMLScalar(node.attributes[xmlName]!, resolvedChild, wireProperty, wireSchemas))
        }
        continue
      }
      if (xml["nodeType"] === "text" || xml["nodeType"] === "cdata") {
        if (node.hasText) defineOwnDataProperty(result, name, decodeInboundXMLScalar(node.text, resolvedChild, wireProperty, wireSchemas))
        continue
      }
      if (schemaAcceptsType(childDescriptor["type"], "array") || wireProperty !== undefined && inboundWireSchemaTypes(wireProperty, wireSchemas).includes("array")) {
        const container = xml["wrapped"] === true ? node.children.find((child) => child.name === xmlName) : node
        if (container !== undefined) {
          if (container !== node) {
            consumedChildren.add(container)
            for (const name of Object.keys(container.attributes)) {
              if (name !== "xmlns" && !name.startsWith("xmlns:")) throw new TypeError("unexpected XML array wrapper attribute " + name)
            }
            if (container.text.trim() !== "") throw new TypeError("unexpected XML array wrapper text")
          }
          const values: unknown[] = []
          for (const child of container.children) {
            const item = inboundArrayItemSchema(childDescriptor, values.length)
            const resolvedItem = resolveInboundSchema(item, schemas)
            const itemDescriptor = inboundSchemaRecord(resolvedItem)
            const wireItem = wireProperty === undefined ? undefined : inboundWireArrayItemSchema(wireProperty, values.length, wireSchemas)
            const itemXML = isRecord(itemDescriptor["xml"]) ? itemDescriptor["xml"] : wireItem?.xml ?? {}
            const wrapperFallbackName = typeof xml?.name === "string" ? xml.name : name
            const parentItemFallbackName = xml["wrapped"] === true ? wrapperFallbackName : xmlName
            const itemFallbackName = typeof itemXML?.name === "string" ? itemXML.name : parentItemFallbackName
            const itemName = inboundXMLQualifiedName(itemXML, parentItemFallbackName)
            if (xml["wrapped"] === true && child.name !== itemName) throw new TypeError("unexpected XML array item " + child.name)
            if (xml["wrapped"] !== true && child.name !== xmlName && child.name !== itemName) continue
            consumedChildren.add(child)
            values.push(decodeInboundXMLNode(child, resolvedItem, schemas, wireItem, wireSchemas, false, itemFallbackName))
          }
          defineOwnDataProperty(result, name, values)
        }
        continue
      }
      const child = node.children.find((entry) => entry.name === xmlName)
      if (child !== undefined) {
        consumedChildren.add(child)
        const childFallbackName = typeof xml?.name === "string" ? xml.name : name
        defineOwnDataProperty(result, name, decodeInboundXMLNode(child, resolvedChild, schemas, wireProperty, wireSchemas, false, childFallbackName))
      }
    }
    for (const [name, value] of Object.entries(node.attributes)) {
      if (consumedAttributes.has(name)) continue
      if (name === "xmlns" || name.startsWith("xmlns:")) continue
      const wireProperty = wireSchema === undefined ? undefined : inboundWirePropertySchema(wireSchema, name, wireSchemas)
      defineOwnDataProperty(result, name, wireProperty === undefined ? value : decodeInboundXMLScalar(value, {}, wireProperty, wireSchemas))
    }
    for (const child of node.children) {
      if (consumedChildren.has(child)) continue
      const wireProperty = wireSchema === undefined ? undefined : inboundWirePropertySchema(wireSchema, child.name, wireSchemas)
      const value = wireProperty === undefined ? decodeInboundUnknownXMLNode(child) : decodeInboundXMLNode(child, {}, schemas, wireProperty, wireSchemas, false, child.name)
      const previous = result[child.name]
      defineOwnDataProperty(result, child.name, previous === undefined ? value : Array.isArray(previous) ? [...previous, value] : [previous, value])
    }
    return result
  }
  return decodeInboundXMLScalar(node.text, resolved, wireSchema, wireSchemas)
}

function decodeInboundUnknownXMLNode(node: InboundXMLNode): unknown {
  if (node.children.length === 0 && Object.keys(node.attributes).length === 0) return node.text
  const result = Object.create(null) as Record<string, unknown>
  for (const [name, value] of Object.entries(node.attributes)) defineOwnDataProperty(result, name, value)
  for (const child of node.children) {
    const value = decodeInboundUnknownXMLNode(child)
    const previous = result[child.name]
    defineOwnDataProperty(result, child.name, previous === undefined ? value : Array.isArray(previous) ? [...previous, value] : [previous, value])
  }
  if (node.text.trim() !== "") defineOwnDataProperty(result, "#text", node.text)
  return result
}

function inboundXMLQualifiedName(xml: Readonly<Record<string, unknown>> | WireSchema["xml"], fallback: string): string {
  const name = typeof xml?.name === "string" ? xml.name : fallback
  return typeof xml?.prefix === "string" && xml.prefix !== "" ? xml.prefix + ":" + name : name
}

function resolveInboundSchema(schema: InboundSchema, schemas: InboundSchemas, resolving: ReadonlySet<string> = new Set()): InboundSchema {
  if (typeof schema === "boolean") return schema
  const descriptor = inboundSchemaRecord(schema)
  const reference = typeof descriptor["$ref"] === "string" ? descriptor["$ref"] : undefined
  const name = reference === undefined ? undefined : inboundComponentReferenceName(reference)
  let resolved: InboundSchema = schema
  if (name !== undefined && !resolving.has(name)) {
    const target = schemas[name]
    if (target !== undefined) {
      const nestedResolving = new Set(resolving); nestedResolving.add(name)
      const base = resolveInboundSchema(target, schemas, nestedResolving)
      if (base === false) return false
      const siblings = Object.fromEntries(Object.entries(descriptor).filter(([key]) => key !== "$ref"))
      resolved = base === true ? siblings : mergeInboundSchemaRecords(inboundSchemaRecord(base), siblings)
    }
  }
  if (typeof resolved === "boolean") return resolved
  let effective = inboundSchemaRecord(resolved)
  for (const part of Array.isArray(effective["allOf"]) ? effective["allOf"] : []) {
    if (!isInboundSchema(part)) continue
    const nested = resolveInboundSchema(part, schemas, resolving)
    if (nested === false) return false
    if (nested !== true) effective = mergeInboundSchemaRecords(effective, inboundSchemaRecord(nested))
  }
  return effective
}

function mergeInboundSchemaRecords(left: Readonly<Record<string, unknown>>, right: Readonly<Record<string, unknown>>): Readonly<Record<string, unknown>> {
  const merged = { ...left, ...right }
  if (isRecord(left["properties"]) || isRecord(right["properties"])) {
    const leftProperties = isRecord(left["properties"]) ? left["properties"] : {}
    const rightProperties = isRecord(right["properties"]) ? right["properties"] : {}
    const properties = { ...leftProperties, ...rightProperties }
    for (const name of Object.keys(properties)) {
      const leftProperty = leftProperties[name]
      const rightProperty = rightProperties[name]
      if (isInboundSchema(leftProperty) && isInboundSchema(rightProperty)) properties[name] = mergeInboundSchemaValues(leftProperty, rightProperty)
    }
    merged["properties"] = properties
  }
  if (isInboundSchema(left["items"]) && isInboundSchema(right["items"])) merged["items"] = mergeInboundSchemaValues(left["items"], right["items"])
  const leftAllOf = Array.isArray(left["allOf"]) ? left["allOf"] : []
  const rightAllOf = Array.isArray(right["allOf"]) ? right["allOf"] : []
  const conjunctions = [...leftAllOf, ...rightAllOf]
  for (const keyword of ["oneOf", "anyOf"]) {
    const leftVariants = Array.isArray(left[keyword]) ? left[keyword] : []
    const rightVariants = Array.isArray(right[keyword]) ? right[keyword] : []
    if (leftVariants.length !== 0 && rightVariants.length !== 0) conjunctions.push({ [keyword]: leftVariants })
  }
  if (conjunctions.length !== 0) merged["allOf"] = conjunctions
  const leftPrefixItems = left["prefixItems"]
  const rightPrefixItems = right["prefixItems"]
  if (Array.isArray(leftPrefixItems) && Array.isArray(rightPrefixItems)) {
    const maximum = Math.max(leftPrefixItems.length, rightPrefixItems.length)
    merged["prefixItems"] = Array.from({ length: maximum }, (_, index) => {
      const leftItem = leftPrefixItems[index]
      const rightItem = rightPrefixItems[index]
      return isInboundSchema(leftItem) && isInboundSchema(rightItem) ? mergeInboundSchemaValues(leftItem, rightItem) : rightItem ?? leftItem
    })
  }
  return merged
}

function mergeInboundSchemaValues(left: InboundSchema, right: InboundSchema): InboundSchema {
  if (left === false || right === false) return false
  if (left === true) return right
  if (right === true) return left
  return mergeInboundSchemaRecords(left, right)
}

function inboundComponentReferenceName(reference: string): string | undefined {
  if (!reference.startsWith("#")) return undefined
  let pointer: string
  try { pointer = decodeURIComponent(reference.slice(1)) } catch { return undefined }
  const prefix = "/components/schemas/"
  if (!pointer.startsWith(prefix)) return undefined
  const token = pointer.slice(prefix.length)
  if (token === "" || token.includes("/")) return undefined
  return token.replaceAll("~1", "/").replaceAll("~0", "~")
}

function decodeInboundXMLScalar(value: string, schema: InboundSchema, wireSchema: WireSchema | undefined = undefined, wireSchemas: WireSchemas = {}): unknown {
  if (wireSchema !== undefined) return decodeInboundParameterValue(value, schema, {}, wireSchema, wireSchemas)
  const descriptor = inboundSchemaRecord(schema)
  if (schemaAcceptsType(descriptor["type"], "integer")) { const parsed = Number(value); if (!Number.isInteger(parsed)) throw new TypeError("XML value is not an integer"); return parsed }
  if (schemaAcceptsType(descriptor["type"], "number")) { const parsed = Number(value); if (!Number.isFinite(parsed)) throw new TypeError("XML value is not a number"); return parsed }
  if (schemaAcceptsType(descriptor["type"], "boolean")) { if (value === "true") return true; if (value === "false") return false; throw new TypeError("XML value is not a boolean") }
  return value
}

function unescapeInboundXML(value: string): string { return value.replaceAll("&lt;", "<").replaceAll("&gt;", ">").replaceAll("&quot;", "\"").replaceAll("&apos;", "'").replaceAll("&amp;", "&") }

function assertInboundJSONSerializable(value: unknown, active: WeakSet<object> = new WeakSet()): void {
  if (value === null || typeof value === "string" || typeof value === "boolean") return
  if (typeof value === "number") {
    if (!Number.isFinite(value)) throw new TypeError("JSON response numbers must be finite")
    return
  }
  if (value === undefined || typeof value === "function" || typeof value === "symbol" || typeof value === "bigint") throw new TypeError("JSON response contains a non-serializable value")
  if (active.has(value)) throw new TypeError("JSON response contains a cycle")
  active.add(value)
  if (Array.isArray(value)) {
    if (Object.getPrototypeOf(value) !== Array.prototype) throw new TypeError("JSON response arrays must use the standard array prototype")
    const keys = Object.keys(value)
    if (keys.length !== value.length || keys.some((key, index) => key !== String(index))) throw new TypeError("JSON response array has non-index properties")
    const names = Object.getOwnPropertyNames(value)
    const descriptors = Object.getOwnPropertyDescriptors(value) as Readonly<Record<string, PropertyDescriptor>>
    const descriptorValues = Object.values(descriptors) as readonly PropertyDescriptor[]
    if (names.length !== value.length + 1 || names.some((name) => name !== "length" && !/^(0|[1-9][0-9]*)$/.test(name)) || descriptorValues.some((descriptor) => !Object.hasOwn(descriptor, "value")) || Object.getOwnPropertySymbols(value).length !== 0) throw new TypeError("JSON response contains non-JSON array properties")
    for (let index = 0; index < value.length; index++) {
      if (!Object.hasOwn(value, index)) throw new TypeError("JSON response contains a sparse array")
      assertInboundJSONSerializable(descriptors[String(index)]!.value, active)
    }
  } else {
    const prototype = Object.getPrototypeOf(value)
    if (prototype !== Object.prototype && prototype !== null) throw new TypeError("JSON response objects must be plain records")
    const names = Object.getOwnPropertyNames(value)
    const descriptors = Object.getOwnPropertyDescriptors(value) as Readonly<Record<string, PropertyDescriptor>>
    const descriptorValues = Object.values(descriptors) as readonly PropertyDescriptor[]
    if (Object.getOwnPropertySymbols(value).length !== 0 || names.length !== Object.keys(value).length || descriptorValues.some((descriptor) => !Object.hasOwn(descriptor, "value"))) throw new TypeError("JSON response contains non-JSON properties")
    for (const descriptor of descriptorValues) assertInboundJSONSerializable(descriptor.value, active)
  }
  active.delete(value)
}

function schemaAcceptsType(type: unknown, wanted: string): boolean { return Array.isArray(type) ? type.includes(wanted) : type === wanted }
function isRecord(value: unknown): value is Record<string, unknown> { return typeof value === "object" && value !== null && !Array.isArray(value) }
function isInboundSchema(value: unknown): value is InboundSchema { return typeof value === "boolean" || isRecord(value) }
function inboundSchemaRecord(value: InboundSchema | undefined): Readonly<Record<string, unknown>> { return isRecord(value) ? value : {} }

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
  const normalizedContentType = normalizeInboundMediaType(contentType)
  if (!headers.has("content-type")) headers.set("content-type", contentType)
  const definition = statusDefinitions.filter((entry) => entry.contentType !== undefined && inboundMediaTypeMatches(entry.contentType, normalizeInboundMediaType(contentType))).sort((left, right) => inboundMediaTypeMatchScore(right.contentType ?? "", contentType) - inboundMediaTypeMatchScore(left.contentType ?? "", contentType))[0]
  if (options !== undefined && definition === undefined) throw new TypeError("response content type " + contentType + " is not declared for status " + value.status)
  if (definition?.schema !== undefined) validateWireValue(value.body, definition.schema, options!.schemas, "encode")
	await validateInboundResponseHeaders(headers, definition?.headers, options?.schemas ?? {}, options?.codecs, generatedHeaderNames)
  if (normalizedContentType === "application/json" || normalizedContentType.endsWith("+json")) {
    assertInboundJSONSerializable(value.body)
    return new Response(JSON.stringify(value.body), { status: value.status, headers })
  }
  if (normalizedContentType.includes("xml")) return new Response(encodeXML(value.body, definition?.schema ?? {}, options?.schemas ?? {}), { status: value.status, headers })
  if (normalizedContentType.startsWith("text/")) return new Response(String(value.body), { status: value.status, headers })
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
    const form = Object.create(null) as Record<string, string | string[]>
    for (const [name, item] of new URLSearchParams(value)) {
      const previous = form[name]
      defineOwnDataProperty(form, name, previous === undefined ? item : Array.isArray(previous) ? [...previous, item] : [previous, item])
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
  return decodeInboundSimpleHeader(value, definition.schema, schemas, definition.explode ?? false)
}

function decodeInboundSimpleHeader(value: string, schema: WireSchema, schemas: WireSchemas, explode: boolean): unknown {
  const resolved = resolveInboundHeaderSchema(schema, schemas)
  if (resolved.types?.includes("array")) {
    const item = resolved.items ?? {}
    return value.split(",").map((entry) => decodeInboundSimpleHeaderScalar(entry, item, schemas))
  }
  if (resolved.types?.includes("object") || resolved.properties !== undefined) {
    const result = Object.create(null) as Record<string, unknown>
    const tokens = value.split(",")
    if (explode) {
      for (const token of tokens) {
        const separator = token.indexOf("=")
        if (separator < 0) continue
        const name = token.slice(0, separator)
        const property = resolved.properties?.[name]
        defineOwnDataProperty(result, name, decodeInboundSimpleHeaderScalar(token.slice(separator + 1), property?.schema ?? {}, schemas))
      }
    } else for (let index = 0; index + 1 < tokens.length; index += 2) {
      const name = tokens[index]!
      const property = resolved.properties?.[name]
      defineOwnDataProperty(result, name, decodeInboundSimpleHeaderScalar(tokens[index + 1]!, property?.schema ?? {}, schemas))
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
