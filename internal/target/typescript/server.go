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
	bodySchema   string
	bodyRequired bool
	responseType string
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
	bodySchema   string
	bodyRequired bool
	responseType string
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
			if previous.expression == callback.expression && previous.method == callback.method && previous.operationID == callback.operationID && previous.bodyType == callback.bodyType && previous.responseType == callback.responseType && reflect.DeepEqual(previous.security, callback.security) {
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
			if err := rejectInboundParameters(pathItem, appendOpenAPIPointer(appendOpenAPIPointer(path, name), expression)); err != nil {
				return nil, err
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
				if err := rejectInboundParameters(operation, operationPath); err != nil {
					return nil, err
				}
				bodyType, hasBody, bodySchema, bodyRequired, err := inboundBodyType(document, operation, operationPath)
				if err != nil {
					return nil, err
				}
				responseType, err := inboundResponseType(document, operation, operationPath)
				if err != nil {
					return nil, err
				}
				security := operation["security"]
				if security == nil {
					security = document.Raw["security"]
				}
				result = append(result, callbackDefinition{name: appendOpenAPIPointer(path, name), typeName: typeName, expression: expression, operationID: operationID, method: strings.ToUpper(method), bodyType: bodyType, hasBody: hasBody, bodySchema: bodySchema, bodyRequired: bodyRequired, responseType: responseType, security: security})
			}
		}
	}
	return result, nil
}

func inboundResponseType(document *ir.Document, operation map[string]any, path string) (string, error) {
	responses, _ := operation["responses"].(map[string]any)
	statuses := sortedAnyKeys(responses)
	if len(statuses) == 0 {
		return "InboundResponse | Response", nil
	}
	values := make([]string, 0, len(statuses)+1)
	for _, status := range statuses {
		response, _ := responses[status].(map[string]any)
		resolved, err := resolveComponentObject(document, response, "responses")
		if err != nil {
			return "", fmt.Errorf("%s/responses/%s: %w", path, status, err)
		}
		statusType := "number"
		if status != "default" && !strings.ContainsAny(status, "Xx") {
			statusType = status
		}
		content, _ := resolved["content"].(map[string]any)
		mediaTypes := sortedAnyKeys(content)
		if len(mediaTypes) == 0 {
			values = append(values, "{ readonly status: "+statusType+"; readonly headers?: HeadersInit | undefined; readonly body?: never }")
			continue
		}
		if len(mediaTypes) != 1 || !isJSONMediaType(mediaTypes[0]) {
			return "", fmt.Errorf("%s/responses/%s/content: server add-on requires exactly one JSON media type", path, status)
		}
		media, _ := content[mediaTypes[0]].(map[string]any)
		schema, _ := media["schema"].(map[string]any)
		bodyType, err := schemaType(document, schema, projectionOutput)
		if err != nil {
			return "", fmt.Errorf("%s/responses/%s/content/%s/schema: %w", path, status, mediaTypes[0], err)
		}
		values = append(values, "{ readonly status: "+statusType+"; readonly headers?: HeadersInit | undefined; readonly body: "+qualifyClientType(document, bodyType)+" }")
	}
	values = append(values, "Response")
	return strings.Join(values, " | "), nil
}

func emitCallbacks(document *ir.Document, callbacks []callbackDefinition) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("import { decodeJSONBody, InboundRequestError, responseFromHandler, type Authenticate, type InboundRequestContext, type InboundResponse, type InboundSchemas } from \"./runtime.js\"\n")
	if len(callbacks) > 0 {
		output.WriteString("import type * as Contract from \"../generated/types.js\"\n")
	}
	output.WriteString("\n")
	if callbacksHaveBodies(callbacks) {
		if err := emitInboundSchemas(&output, document); err != nil {
			return nil, err
		}
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
		fmt.Fprintf(&output, "const %sCallbackDefinition = { operationID: %s, method: %s, security: %s } as const\n", property, quoteTS(callback.operationID), quoteTS(callback.method), security)
	}
	output.WriteString("\n/** Application handlers keyed by generated Callback names. */\nexport interface CallbackHandlers {\n")
	for _, callback := range callbacks {
		property, err := naming.Property(callback.typeName)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&output, "  readonly %s?: (context: %sCallbackContext) => %sCallbackResponse | Promise<%sCallbackResponse>\n", property, callback.typeName, callback.typeName, callback.typeName)
	}
	output.WriteString("}\n\n/** Optional host authentication for generated Callback endpoints. */\nexport interface CallbackHandlerOptions {\n  readonly authenticate?: Authenticate | undefined\n}\n\n/** Fetch-compatible endpoint for one host-mounted Callback route. */\nexport interface CallbackEndpoint {\n  fetch(request: Request): Promise<Response>\n}\n\n/** Named Callback endpoints ready for host route mounting. */\nexport interface CallbackEndpoints {\n")
	for _, callback := range callbacks {
		property, _ := naming.Property(callback.typeName)
		fmt.Fprintf(&output, "  readonly %s: CallbackEndpoint\n", property)
	}
	output.WriteString("}\n\n/**\n * Creates Fetch-native endpoints for dynamic OpenAPI Callback URLs.\n * The host chooses each concrete route and mounts the matching endpoint.\n */\nexport function createCallbackHandlers(handlers: CallbackHandlers, options: CallbackHandlerOptions = {}): CallbackEndpoints {\n  return {\n")
	for _, callback := range callbacks {
		property, _ := naming.Property(callback.typeName)
		fmt.Fprintf(&output, "    %s: {\n      async fetch(request: Request): Promise<Response> {\n", property)
		fmt.Fprintf(&output, "        if (request.method !== %s) return new Response(\"Method Not Allowed\", { status: 405, headers: { allow: %s } })\n", quoteTS(callback.method), quoteTS(callback.method))
		fmt.Fprintf(&output, "        const handler = handlers.%s\n        if (handler === undefined) return new Response(\"Not Found\", { status: 404 })\n", property)
		fmt.Fprintf(&output, "        const context: InboundRequestContext = { request, operationID: %sCallbackDefinition.operationID, method: %sCallbackDefinition.method, path: new URL(request.url).pathname, security: %sCallbackDefinition.security }\n", property, property, property)
		output.WriteString("        const denied = await options.authenticate?.(context)\n        if (denied instanceof Response) return denied\n")
		if callback.hasBody {
			output.WriteString("        try {\n")
			fmt.Fprintf(&output, "          const body = await decodeJSONBody(request, { required: %t, schema: %s, schemas: inputSchemas }) as %s\n", callback.bodyRequired, callback.bodySchema, callback.bodyType)
			output.WriteString("          return responseFromHandler(await handler({ ...context, body }))\n")
			output.WriteString("        } catch (error) {\n          if (error instanceof InboundRequestError) return error.response\n          throw error\n        }\n")
		} else {
			output.WriteString("        return responseFromHandler(await handler({ ...context, body: undefined }))\n")
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
		if err := rejectInboundParameters(item, openAPIPointer("webhooks", name)); err != nil {
			return nil, err
		}
		for _, method := range serverHTTPMethods {
			operation, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			operationPath := openAPIPointer("webhooks", name, method)
			if err := rejectInboundParameters(operation, operationPath); err != nil {
				return nil, err
			}
			operationID, _ := operation["operationId"].(string)
			if operationID == "" {
				operationID = name
			}
			bodyType, hasBody, bodySchema, bodyRequired, err := inboundBodyType(document, operation, operationPath)
			if err != nil {
				return nil, err
			}
			responseType, err := inboundResponseType(document, operation, operationPath)
			if err != nil {
				return nil, err
			}
			security := operation["security"]
			if security == nil {
				security = document.Raw["security"]
			}
			result = append(result, webhookDefinition{
				name: name, property: property, typeName: typeName, operationID: operationID,
				method: strings.ToUpper(method), bodyType: bodyType, hasBody: hasBody, bodySchema: bodySchema, bodyRequired: bodyRequired, responseType: responseType, security: security,
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

func inboundBodyType(document *ir.Document, operation map[string]any, path string) (string, bool, string, bool, error) {
	body, ok := operation["requestBody"].(map[string]any)
	if !ok {
		return "undefined", false, "", false, nil
	}
	resolved, err := resolveComponentObject(document, body, "requestBodies")
	if err != nil {
		return "", false, "", false, fmt.Errorf("%s/requestBody: %w", path, err)
	}
	content, _ := resolved["content"].(map[string]any)
	mediaTypes := sortedAnyKeys(content)
	if len(mediaTypes) != 1 || !isJSONMediaType(mediaTypes[0]) {
		return "", false, "", false, fmt.Errorf("%s/requestBody/content: server add-on requires exactly one JSON media type", path)
	}
	media, _ := content[mediaTypes[0]].(map[string]any)
	schema, _ := media["schema"].(map[string]any)
	value, err := schemaType(document, schema, projectionInput)
	if err != nil {
		return "", false, "", false, fmt.Errorf("%s/requestBody/content/%s/schema: %w", path, mediaTypes[0], err)
	}
	schemaSource, err := json.Marshal(schema)
	if err != nil {
		return "", false, "", false, fmt.Errorf("%s/requestBody/content/%s/schema: encode validator schema: %w", path, mediaTypes[0], err)
	}
	required, _ := resolved["required"].(bool)
	valueType := qualifyClientType(document, value)
	if !required {
		valueType += " | undefined"
	}
	return valueType, true, string(schemaSource), required, nil
}

func rejectInboundParameters(operation map[string]any, path string) error {
	parameters, _ := operation["parameters"].([]any)
	if len(parameters) == 0 {
		return nil
	}
	return fmt.Errorf("%s/parameters: server add-on does not yet implement inbound parameter decoding", path)
}

func (definition webhookDefinition) bodySchemaOrUndefined() string {
	if definition.bodySchema == "" {
		return "undefined"
	}
	return definition.bodySchema
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

func emitInboundSchemas(output *bytes.Buffer, document *ir.Document) error {
	schemas, err := json.Marshal(document.ComponentSchemas)
	if err != nil {
		return fmt.Errorf("encode inbound component schemas: %w", err)
	}
	fmt.Fprintf(output, "const inputSchemas: InboundSchemas = %s\n\n", schemas)
	return nil
}

func emitWebhooks(document *ir.Document, webhooks []webhookDefinition) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("import { decodeJSONBody, InboundRequestError, responseFromHandler, type Authenticate, type InboundRequestContext, type InboundResponse, type InboundSchemas } from \"./runtime.js\"\n")
	if len(webhooks) > 0 {
		output.WriteString("import type * as Contract from \"../generated/types.js\"\n")
	}
	output.WriteString("\n")
	if webhooksHaveBodies(webhooks) {
		if err := emitInboundSchemas(&output, document); err != nil {
			return nil, err
		}
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
	output.WriteString("export interface WebhookRouterOptions {\n  readonly routes: WebhookRoutes\n  readonly authenticate?: Authenticate | undefined\n}\n\n")
	output.WriteString("/** Fetch-compatible generated inbound Webhook router. */\n")
	output.WriteString("export interface WebhookRouter {\n  fetch(request: Request): Promise<Response>\n}\n\n")
	for _, webhook := range webhooks {
		security, err := json.Marshal(webhook.security)
		if err != nil {
			return nil, fmt.Errorf("%s security metadata: %w", openAPIPointer("webhooks", webhook.name), err)
		}
		fmt.Fprintf(&output, "const %s = { operationID: %s, method: %s, requestSchema: %s, requestBodyRequired: %t, security: %s } as const\n", webhookDefinitionSymbol(webhook), quoteTS(webhook.operationID), quoteTS(webhook.method), webhook.bodySchemaOrUndefined(), webhook.bodyRequired, security)
	}
	if len(webhooks) > 0 {
		output.WriteString("\n")
	}
	output.WriteString("/**\n * Creates a Fetch-native router for the generated root Webhook Objects.\n * Webhook names are OpenAPI identifiers, so the host supplies their concrete paths.\n * Authentication policy stays in the host callback; generated code never verifies credentials.\n */\n")
	output.WriteString("export function createWebhookRouter(handlers: WebhookHandlers, options: WebhookRouterOptions): WebhookRouter {\n")
	output.WriteString("  const routes = options.routes\n  const registrations = new Set<string>()\n")
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
		fmt.Fprintf(&output, "      if (request.method === %s && pathname === routes.%s) {\n", quoteTS(webhook.method), webhook.property)
		fmt.Fprintf(&output, "        const handler = handlers.%s\n", webhook.property)
		output.WriteString("        if (handler === undefined) return new Response(\"Not Found\", { status: 404 })\n")
		symbol := webhookDefinitionSymbol(webhook)
		fmt.Fprintf(&output, "        const context: InboundRequestContext = { request, operationID: %s.operationID, method: %s.method, path: pathname, security: %s.security }\n", symbol, symbol, symbol)
		output.WriteString("        const denied = await options.authenticate?.(context)\n        if (denied instanceof Response) return denied\n")
		if webhook.hasBody {
			output.WriteString("        try {\n")
			fmt.Fprintf(&output, "          const body = await decodeJSONBody(request, { required: %s.requestBodyRequired, schema: %s.requestSchema, schemas: inputSchemas }) as %s\n", symbol, symbol, webhook.bodyType)
			fmt.Fprintf(&output, "          return responseFromHandler(await handler({ ...context, body }))\n")
			output.WriteString("        } catch (error) {\n          if (error instanceof InboundRequestError) return error.response\n          throw error\n        }\n")
		} else {
			fmt.Fprintf(&output, "        return responseFromHandler(await handler({ ...context, body: undefined }))\n")
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
	return []byte(`/** Metadata provided to host-owned inbound authentication policy. */
export interface InboundRequestContext {
  readonly request: Request
  readonly operationID: string
  readonly method: string
  readonly path: string
  readonly security: unknown
}

/** Return void to continue or a Response to reject the inbound request. */
export type Authenticate = (context: InboundRequestContext) => void | Response | Promise<void | Response>

/** Framework-neutral response produced by an inbound generated handler. */
export interface InboundResponse {
  readonly status: number
  readonly headers?: HeadersInit | undefined
  readonly body?: unknown
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
/** Body contract selected from an OpenAPI Request Body Object. */
export interface InboundBodyOptions {
  readonly required: boolean
  readonly schema?: InboundSchema | undefined
  readonly schemas: InboundSchemas
}

/** Decodes and validates one JSON request body at the generated schema boundary. */
export async function decodeJSONBody(request: Request, options: InboundBodyOptions): Promise<unknown> {
  const contentType = request.headers.get("content-type")?.split(";", 1)[0]?.trim().toLowerCase()
  const text = await request.text()
  if (text.trim() === "") {
    if (options.required) throw new InboundRequestError(new Response("Request body is required", { status: 400 }))
    return undefined
  }
  if (contentType === undefined || !(contentType === "application/json" || contentType.endsWith("+json"))) {
    throw new InboundRequestError(new Response("Unsupported Media Type", { status: 415 }))
  }
  let value: unknown
  try { value = JSON.parse(text) } catch { throw new InboundRequestError(new Response("Invalid JSON", { status: 400 })) }
  if (options.schema !== undefined) {
    const error = validateInboundValue(value, options.schema, options.schemas, "body")
    if (error !== undefined) throw new InboundRequestError(new Response("Invalid request body: " + error, { status: 400 }))
  }
  return value
}

function validateInboundValue(value: unknown, schema: InboundSchema, schemas: InboundSchemas, path: string): string | undefined {
  const reference = typeof schema["$ref"] === "string" ? schema["$ref"] : undefined
  if (reference !== undefined) {
    const name = reference.startsWith("#/components/schemas/") ? reference.slice("#/components/schemas/".length).replaceAll("~1", "/").replaceAll("~0", "~") : undefined
    const target = name === undefined ? undefined : schemas[name]
    return target === undefined ? path + " references an unresolved schema" : validateInboundValue(value, target, schemas, path)
  }
  if (value === null && (schema["nullable"] === true || schemaAcceptsType(schema["type"], "null"))) return undefined
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

/** Converts a handler value into its Fetch Response representation. */
export function responseFromHandler(value: InboundResponse | Response): Response {
  if (value instanceof Response) return value
  const headers = new Headers(value.headers)
  if ((value.status === 204 || value.status === 205) && value.body !== undefined) throw new TypeError("Responses with status 204 or 205 must not include a body")
  if (value.body === undefined) return new Response(null, { status: value.status, headers })
  if (!headers.has("content-type")) headers.set("content-type", "application/json")
  return new Response(JSON.stringify(value.body), { status: value.status, headers })
}
`)
}
