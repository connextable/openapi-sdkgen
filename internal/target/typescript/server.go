package typescript

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"openapi-sdkgen/internal/compiler/ir"
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
	sourceRouteKey    string
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
	callbacks, err := collectCallbacks(document)
	if err != nil {
		return nil, err
	}
	return emitPreparedServerArtifacts(document, webhooks, callbacks)
}

func emitPreparedServerArtifacts(document *ir.Document, webhooks []webhookDefinition, callbacks []callbackDefinition) ([]Artifact, error) {
	webhookSource, err := emitWebhooks(document, webhooks)
	if err != nil {
		return nil, err
	}
	callbackSource, err := emitCallbacks(document, callbacks)
	if err != nil {
		return nil, err
	}
	return []Artifact{
		{Path: "server/runtime.ts", Data: generatedSource(serverRuntimeTemplate)},
		{Path: "server/webhooks.ts", Data: generatedSource(webhookSource)},
		{Path: "server/callbacks.ts", Data: generatedSource(callbackSource)},
	}, nil
}

func collectCallbacks(document *ir.Document) ([]callbackDefinition, error) {
	result, failures := collectCallbacksDiagnostics(document)
	if len(failures) != 0 {
		return nil, failures[0]
	}
	return result, nil
}

func collectCallbacksDiagnostics(document *ir.Document) ([]callbackDefinition, []error) {
	result := make([]callbackDefinition, 0)
	var failures []error
	for _, operation := range document.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		callbacks, _ := operation.Raw["callbacks"].(map[string]any)
		for _, callbackName := range sortedAnyKeys(callbacks) {
			value := map[string]any{callbackName: callbacks[callbackName]}
			definitions, callbackFailures := collectCallbackMapDiagnostics(document, value, openAPIPointer("paths", operation.Path, strings.ToLower(operation.Method), "callbacks"), operationRouteKey(operation), operation.OperationID, "")
			failures = append(failures, callbackFailures...)
			result = append(result, definitions...)
		}
	}
	components, _ := document.Raw["components"].(map[string]any)
	componentCallbacks, _ := components["callbacks"].(map[string]any)
	for _, componentName := range sortedAnyKeys(componentCallbacks) {
		value := map[string]any{componentName: componentCallbacks[componentName]}
		definitions, callbackFailures := collectCallbackMapDiagnostics(document, value, openAPIPointer("components", "callbacks"), "", "", componentName)
		failures = append(failures, callbackFailures...)
		result = append(result, definitions...)
	}
	sort.Slice(result, func(i, j int) bool { return callbackIdentity(result[i]) < callbackIdentity(result[j]) })
	return result, failures
}

func collectCallbackMap(document *ir.Document, values map[string]any, path, sourceRouteKey, sourceOperationID, componentName string) ([]callbackDefinition, error) {
	result, failures := collectCallbackMapDiagnostics(document, values, path, sourceRouteKey, sourceOperationID, componentName)
	if len(failures) != 0 {
		return nil, failures[0]
	}
	return result, nil
}

func collectCallbackMapDiagnostics(document *ir.Document, values map[string]any, path, sourceRouteKey, sourceOperationID, componentName string) ([]callbackDefinition, []error) {
	names := sortedAnyKeys(values)
	result := make([]callbackDefinition, 0, len(names))
	var failures []error
	for _, name := range names {
		callback, ok := values[name].(map[string]any)
		if !ok {
			failures = append(failures, fmt.Errorf("%s must be a Callback Object", appendOpenAPIPointer(path, name)))
			continue
		}
		resolved, err := resolveComponentObject(document, callback, "callbacks")
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", appendOpenAPIPointer(path, name), err))
			continue
		}
		for _, expression := range sortedAnyKeys(resolved) {
			pathItem, ok := resolved[expression].(map[string]any)
			if !ok {
				failures = append(failures, fmt.Errorf("%s must be a Callback Path Item Object", appendOpenAPIPointer(appendOpenAPIPointer(path, name), expression)))
				continue
			}
			operations, resolvedPathItem, pathFailures := serverPathItemOperationsDiagnostics(document, pathItem, appendOpenAPIPointer(appendOpenAPIPointer(path, name), expression))
			failures = append(failures, pathFailures...)
			for _, item := range operations {
				method := item.method
				operation := item.operation
				operationID, _ := operation["operationId"].(string)
				if operationID == "" {
					operationID = name
				}
				operationPath := appendOpenAPIPointer(appendOpenAPIPointer(appendOpenAPIPointer(path, name), expression), item.key)
				parameters, paramsType, parameterErr := inboundParameterDefinitions(document, resolvedPathItem, operation, operationPath, true)
				body, bodyErr := inboundBodyType(document, operation, operationPath)
				responseType, responsePlan, responseErr := inboundResponseDefinition(document, operation, operationPath)
				for _, operationErr := range []error{parameterErr, bodyErr, responseErr} {
					if operationErr != nil {
						failures = append(failures, operationErr)
					}
				}
				if parameterErr != nil || bodyErr != nil || responseErr != nil {
					continue
				}
				security := operation["security"]
				if security == nil {
					security = document.Raw["security"]
				}
				identity := sourceRouteKey + "\x00" + componentName + "\x00" + name + "\x00" + expression + "\x00" + method
				result = append(result, callbackDefinition{
					name: appendOpenAPIPointer(path, name), sourceRouteKey: sourceRouteKey, sourceOperationID: sourceOperationID, componentName: componentName, callbackName: name,
					typeName: stablePrivateIdentifier("callback-type", identity), expression: expression, operationID: operationID, method: method,
					bodyType: body.typeName, hasBody: body.hasBody, bodyRequired: body.required, bodyPlans: body.plans, parameters: parameters, paramsType: paramsType,
					responseType: responseType, responsePlan: responsePlan, security: security,
				})
			}
		}
	}
	return result, failures
}

func callbackIdentity(callback callbackDefinition) string {
	return callback.sourceRouteKey + "\x00" + callback.componentName + "\x00" + callback.callbackName + "\x00" + callback.expression + "\x00" + callback.method
}

type callbackTreeNode struct {
	children map[string]*callbackTreeNode
	callback *callbackDefinition
}

func buildCallbackTree(callbacks []callbackDefinition, components, routeKeys bool) *callbackTreeNode {
	root := &callbackTreeNode{children: make(map[string]*callbackTreeNode)}
	for index := range callbacks {
		callback := &callbacks[index]
		if (callback.componentName != "") != components {
			continue
		}
		sourceKey := callback.sourceOperationID
		if routeKeys {
			sourceKey = callback.sourceRouteKey
		}
		if !components && sourceKey == "" {
			continue
		}
		keys := []string{sourceKey, callback.callbackName, callback.expression, callback.method}
		if components {
			keys = []string{callback.componentName, callback.expression, callback.method}
		}
		node := root
		for _, key := range keys {
			if node.children[key] == nil {
				node.children[key] = &callbackTreeNode{children: make(map[string]*callbackTreeNode)}
			}
			node = node.children[key]
		}
		node.callback = callback
	}
	return root
}

func callbackTreeKeys(node *callbackTreeNode) []string {
	keys := make([]string, 0, len(node.children))
	for key := range node.children {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func emitCallbackTypes(output *bytes.Buffer, name string, root *callbackTreeNode) {
	fmt.Fprintf(output, "export interface %s ", name)
	emitCallbackTree(output, root, nil, name, callbackTreeTypes)
	output.WriteString("\n\n")
}

type callbackTreeMode int

const (
	callbackTreeTypes callbackTreeMode = iota
	callbackTreeHandlers
	callbackTreeEndpoints
	callbackTreePathParams
)

func emitCallbackTree(output *bytes.Buffer, node *callbackTreeNode, path []string, rootType string, mode callbackTreeMode) {
	output.WriteString("{\n")
	indent := strings.Repeat("  ", len(path)+1)
	for _, key := range callbackTreeKeys(node) {
		child := node.children[key]
		optional := ""
		if mode == callbackTreeHandlers || mode == callbackTreePathParams {
			optional = "?"
		}
		fmt.Fprintf(output, "%sreadonly %s%s: ", indent, quoteTS(key), optional)
		if child.callback != nil {
			slot := rootType
			for _, item := range append(path, key) {
				slot += "[" + quoteTS(item) + "]"
			}
			switch mode {
			case callbackTreeTypes:
				fmt.Fprintf(output, "{ readonly context: %sContext; readonly input: %sContext; readonly output: %sResponse; readonly response: %sResponse; readonly handler: (context: %sContext) => %sResponse | Promise<%sResponse>; readonly endpoint: CallbackEndpoint }", child.callback.typeName, child.callback.typeName, child.callback.typeName, child.callback.typeName, child.callback.typeName, child.callback.typeName, child.callback.typeName)
			case callbackTreeHandlers:
				fmt.Fprintf(output, "%s[\"handler\"]", slot)
			case callbackTreeEndpoints:
				fmt.Fprintf(output, "%s[\"endpoint\"]", slot)
			case callbackTreePathParams:
				output.WriteString("Readonly<Record<string, string>>")
			}
		} else {
			emitCallbackTree(output, child, append(path, key), rootType, mode)
		}
		output.WriteString("\n")
	}
	output.WriteString(strings.Repeat("  ", len(path)) + "}")
}

func callbackLookupPath(callback callbackDefinition, routeKeys bool) (string, []string) {
	if callback.componentName != "" {
		return "ComponentCallbacks", []string{callback.componentName, callback.expression, callback.method}
	}
	if routeKeys {
		return "RouteCallbacks", []string{callback.sourceRouteKey, callback.callbackName, callback.expression, callback.method}
	}
	return "Callbacks", []string{callback.sourceOperationID, callback.callbackName, callback.expression, callback.method}
}

func callbackAccess(root string, callback callbackDefinition, routeKeys bool) string {
	_, path := callbackLookupPath(callback, routeKeys)
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

func callbackRootField(callback callbackDefinition, routeKeys bool) string {
	if callback.componentName != "" {
		return "componentCallbacks"
	}
	if routeKeys {
		return "routeCallbacks"
	}
	return "callbacks"
}

func callbackRuntimeTreeExpression(root *callbackTreeNode) string {
	properties := make([]runtimeProperty, 0, len(root.children))
	for _, key := range callbackTreeKeys(root) {
		child := root.children[key]
		value := ""
		if child.callback != nil {
			value = callbackEndpointSymbol(*child.callback)
		} else {
			value = callbackRuntimeTreeExpression(child)
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
	routeTree := buildCallbackTree(callbacks, false, true)
	operationTree := buildCallbackTree(callbacks, false, false)
	componentTree := buildCallbackTree(callbacks, true, false)
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
	emitCallbackTypes(&output, "RouteCallbacks", routeTree)
	emitCallbackTypes(&output, "Callbacks", operationTree)
	emitCallbackTypes(&output, "ComponentCallbacks", componentTree)
	for _, callback := range callbacks {
		security, err := runtimeJSONExpression(callback.security)
		if err != nil {
			return nil, fmt.Errorf("%s security metadata: %w", callback.name, err)
		}
		fmt.Fprintf(&output, "const %s = { operationID: %s, method: %s, parameters: %s satisfies readonly InboundParameterDefinition[], responses: %s, security: %s } as const\n", callbackDefinitionSymbol(callback), quoteTS(callback.operationID), quoteTS(callback.method), callback.parameters, callback.responsePlan, security)
	}
	output.WriteString("\n/** Application handlers keyed by exact Callback identity. */\nexport interface CallbackHandlers {\n  readonly routeCallbacks?: ")
	emitCallbackTree(&output, routeTree, nil, "RouteCallbacks", callbackTreeHandlers)
	output.WriteString("\n  readonly callbacks?: ")
	emitCallbackTree(&output, operationTree, nil, "Callbacks", callbackTreeHandlers)
	output.WriteString("\n  readonly componentCallbacks?: ")
	emitCallbackTree(&output, componentTree, nil, "ComponentCallbacks", callbackTreeHandlers)
	output.WriteString("\n}\n\n/** Optional host authentication, media codecs, and host-bound path parameters for generated Callback endpoints. */\nexport interface CallbackHandlerOptions {\n  readonly authenticate?: Authenticate | undefined\n  readonly codecs?: Readonly<Record<string, MediaCodec<unknown>>> | undefined\n  readonly maxStreamItemBytes?: number | undefined\n  readonly pathParams?: {\n    readonly routeCallbacks?: ")
	emitCallbackTree(&output, routeTree, nil, "RouteCallbacks", callbackTreePathParams)
	output.WriteString("\n    readonly callbacks?: ")
	emitCallbackTree(&output, operationTree, nil, "Callbacks", callbackTreePathParams)
	output.WriteString("\n    readonly componentCallbacks?: ")
	emitCallbackTree(&output, componentTree, nil, "ComponentCallbacks", callbackTreePathParams)
	output.WriteString("\n  } | undefined\n}\n\n/** Fetch-compatible endpoint for one host-mounted Callback route. */\nexport interface CallbackEndpoint {\n  fetch(request: Request): Promise<Response>\n}\n\n/** Callback endpoints preserving every exact source identity dimension. */\nexport interface CallbackEndpoints {\n  readonly routeCallbacks: ")
	emitCallbackTree(&output, routeTree, nil, "RouteCallbacks", callbackTreeEndpoints)
	output.WriteString("\n  readonly callbacks: ")
	emitCallbackTree(&output, operationTree, nil, "Callbacks", callbackTreeEndpoints)
	output.WriteString("\n  readonly componentCallbacks: ")
	emitCallbackTree(&output, componentTree, nil, "ComponentCallbacks", callbackTreeEndpoints)
	output.WriteString("\n}\n\n/**\n * Creates Fetch-native endpoints for dynamic OpenAPI Callback URLs.\n * The host chooses each concrete route and mounts the matching endpoint.\n */\nexport function createCallbackHandlers(handlers: CallbackHandlers, options: CallbackHandlerOptions = {}): CallbackEndpoints {\n  const inboundCodecs = normalizeInboundMediaCodecs(options.codecs)\n")
	for _, callback := range callbacks {
		definition := callbackDefinitionSymbol(callback)
		routeHandler := callbackAccess("handlers."+callbackRootField(callback, true), callback, true)
		aliasHandler := "undefined"
		if callback.componentName != "" {
			aliasHandler = routeHandler
		} else if callback.sourceOperationID != "" {
			aliasHandler = callbackAccess("handlers."+callbackRootField(callback, false), callback, false)
		}
		routePathParams := callbackAccess("options.pathParams?."+callbackRootField(callback, true), callback, true)
		aliasPathParams := "undefined"
		if callback.componentName != "" {
			aliasPathParams = routePathParams
		} else if callback.sourceOperationID != "" {
			aliasPathParams = callbackAccess("options.pathParams?."+callbackRootField(callback, false), callback, false)
		}
		fmt.Fprintf(&output, "  const %s: CallbackEndpoint = {\n    async fetch(request: Request): Promise<Response> {\n", callbackEndpointSymbol(callback))
		fmt.Fprintf(&output, "      if (request.method !== %s) return new Response(\"Method Not Allowed\", { status: 405, headers: { allow: %s } })\n", quoteTS(callback.method), quoteTS(callback.method))
		fmt.Fprintf(&output, "      const routeHandler = %s\n      const operationHandler = %s\n      if (routeHandler !== undefined && operationHandler !== undefined && routeHandler !== operationHandler) throw new TypeError(\"callback handler was supplied through both routeCallbacks and callbacks\")\n      const handler = routeHandler ?? operationHandler\n      if (handler === undefined) return new Response(\"Not Found\", { status: 404 })\n", routeHandler, aliasHandler)
		fmt.Fprintf(&output, "      const routePathParams = %s\n      const operationPathParams = %s\n      if (routePathParams !== undefined && operationPathParams !== undefined && routePathParams !== operationPathParams) throw new TypeError(\"callback path parameters were supplied through both routeCallbacks and callbacks\")\n      const pathParams = routePathParams ?? operationPathParams\n", routePathParams, aliasPathParams)
		fmt.Fprintf(&output, "      let params: %s\n      try { params = await decodeInboundParameters(request, %s.parameters, inputSchemas, inputWireSchemas, inboundCodecs, pathParams) as %s } catch (error) { if (error instanceof InboundRequestError) return error.response; throw error }\n      const context = { request, operationID: %s.operationID, method: %s.method, path: new URL(request.url).pathname, params, security: %s.security, securityCandidates: collectInboundSecurityCandidates(request, %s.security, securitySchemes) } as Omit<%sContext, \"body\">\n", callback.paramsType, definition, callback.paramsType, definition, definition, definition, definition, callback.typeName)
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
		"  return { routeCallbacks: %s as unknown as CallbackEndpoints[\"routeCallbacks\"], callbacks: %s as unknown as CallbackEndpoints[\"callbacks\"], componentCallbacks: %s as unknown as CallbackEndpoints[\"componentCallbacks\"] }\n}\n",
		callbackRuntimeTreeExpression(routeTree),
		callbackRuntimeTreeExpression(operationTree),
		callbackRuntimeTreeExpression(componentTree),
	)
	return output.Bytes(), nil
}

func collectWebhooks(document *ir.Document) ([]webhookDefinition, error) {
	result, failures := collectWebhooksDiagnostics(document)
	if len(failures) != 0 {
		return nil, failures[0]
	}
	return result, nil
}

func collectWebhooksDiagnostics(document *ir.Document) ([]webhookDefinition, []error) {
	values, _ := document.Raw["webhooks"].(map[string]any)
	names := sortedAnyKeys(values)
	result := make([]webhookDefinition, 0, len(names))
	var failures []error
	for _, name := range names {
		item, ok := values[name].(map[string]any)
		if !ok {
			failures = append(failures, fmt.Errorf("%s must be a Path Item Object", openAPIPointer("webhooks", name)))
			continue
		}
		definitions, webhookFailures := collectWebhookDiagnostics(document, name, item)
		failures = append(failures, webhookFailures...)
		result = append(result, definitions...)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].name == result[j].name {
			return result[i].method < result[j].method
		}
		return result[i].name < result[j].name
	})
	return result, failures
}

func collectWebhook(document *ir.Document, name string, item map[string]any) ([]webhookDefinition, error) {
	result, failures := collectWebhookDiagnostics(document, name, item)
	if len(failures) != 0 {
		return nil, failures[0]
	}
	return result, nil
}

func collectWebhookDiagnostics(document *ir.Document, name string, item map[string]any) ([]webhookDefinition, []error) {
	operations, resolvedItem, failures := serverPathItemOperationsDiagnostics(document, item, openAPIPointer("webhooks", name))
	result := make([]webhookDefinition, 0, len(operations))
	for _, itemOperation := range operations {
		method := itemOperation.method
		operation := itemOperation.operation
		operationPath := openAPIPointer("webhooks", name, itemOperation.key)
		parameters, paramsType, parameterErr := inboundParameterDefinitions(document, resolvedItem, operation, operationPath, true)
		operationID, _ := operation["operationId"].(string)
		if operationID == "" {
			operationID = name
		}
		body, bodyErr := inboundBodyType(document, operation, operationPath)
		responseType, responsePlan, responseErr := inboundResponseDefinition(document, operation, operationPath)
		for _, operationErr := range []error{parameterErr, bodyErr, responseErr} {
			if operationErr != nil {
				failures = append(failures, operationErr)
			}
		}
		if parameterErr != nil || bodyErr != nil || responseErr != nil {
			continue
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
	return result, failures
}

var serverHTTPMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace", "query"}

type serverPathItemOperation struct {
	key       string
	method    string
	operation map[string]any
}

func serverPathItemOperations(document *ir.Document, pathItem map[string]any, path string) ([]serverPathItemOperation, map[string]any, error) {
	result, resolved, failures := serverPathItemOperationsDiagnostics(document, pathItem, path)
	if len(failures) != 0 {
		return nil, nil, failures[0]
	}
	return result, resolved, nil
}

func serverPathItemOperationsDiagnostics(document *ir.Document, pathItem map[string]any, path string) ([]serverPathItemOperation, map[string]any, []error) {
	resolved, err := ir.ResolvePathItem(document.Raw, pathItem)
	if err != nil {
		return nil, nil, []error{fmt.Errorf("%s: %w", path, err)}
	}
	result := make([]serverPathItemOperation, 0)
	var failures []error
	for _, method := range serverHTTPMethods {
		if operation, ok := resolved[method].(map[string]any); ok {
			result = append(result, serverPathItemOperation{key: method, method: strings.ToUpper(method), operation: operation})
		}
	}
	additional, _ := resolved["additionalOperations"].(map[string]any)
	for _, method := range sortedAnyKeys(additional) {
		operation, ok := additional[method].(map[string]any)
		if !ok {
			failures = append(failures, fmt.Errorf("%s/additionalOperations/%s must be an Operation Object", path, method))
			continue
		}
		result = append(result, serverPathItemOperation{key: method, method: method, operation: operation})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].method < result[j].method })
	return result, resolved, failures
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
	parameters, err := operationParameters(document, ir.Operation{Pointer: path, PathItemRaw: pathItem, Raw: operation})
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
		if parameter.Sort != nil {
			values := make([]runtimeProperty, 0, len(parameter.Sort.Values))
			for _, value := range parameter.Sort.Values {
				values = append(values, runtimeProperty{key: value.Field + "\x00" + value.Direction, value: quoteTS(value.Wire)})
			}
			entry += ", sort: " + runtimeObjectExpression(values)
		}
		entries = append(entries, entry+" }")
		valueType, err := schemaTypeForScope(document, parameter.Schema, projectionInput, typeRenderContract)
		if err != nil {
			return "", "", fmt.Errorf("%s/parameters/%s: render input type: %w", path, parameter.Name, err)
		}
		if parameter.Sort != nil {
			members := make([]string, 0, len(parameter.Sort.Values))
			for _, value := range parameter.Sort.Values {
				members = append(members, "{ readonly field: "+quoteTS(value.Field)+"; readonly direction: "+quoteTS(value.Direction)+" }")
			}
			valueType = "readonly (" + strings.Join(members, " | ") + ")[]"
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
	output.WriteString("/** Webhook types keyed by exact root Webhook Object name and HTTP method. */\n")
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
