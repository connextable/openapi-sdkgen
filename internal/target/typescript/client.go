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

func emitClient(document *ir.Document, manifest Manifest, links []generatedLink, streams []generatedStream) ([]byte, error) {
	fixedMembers := resourceCapabilityMembers(links, streams)
	tree, err := buildResourceTree(document, manifest, fixedMembers)
	if err != nil {
		return nil, err
	}
	hasPathOperations := resourceTreeHasPathOperations(tree)
	hasPagination := manifestHasPagination(manifest)
	var output bytes.Buffer
	output.WriteString("import {\n")
	output.WriteString("  assignCallableProperties,\n")
	output.WriteString("  bindOperation,\n")
	if len(streams) > 0 {
		output.WriteString("  bindStreamOperation,\n")
	}
	if hasPathOperations {
		output.WriteString("  bindPathOperation,\n")
	}
	output.WriteString("} from \"./runtime/callables.js\"\n")
	if hasPagination {
		output.WriteString("import { createPaginator, type PaginateInput } from \"./runtime/pagination.js\"\n")
	}
	output.WriteString("import { createRequest } from \"./runtime/http.js\"\n")
	if len(links) > 0 {
		output.WriteString("import { mergeLinkInput, resolveLinkInput, type LinkInvocation, type RequiredLinkInvocation } from \"./runtime/links.js\"\n")
		output.WriteString("import type { APIError } from \"./runtime/errors.js\"\n")
	}
	output.WriteString("import type { ClientOptions } from \"./runtime/configuration.js\"\n")
	output.WriteString("import type { BinaryBody, RawResponseFor, RequestOptions } from \"./runtime/request.js\"\n")
	output.WriteString("import type { TransportError } from \"./runtime/errors.js\"\n")
	output.WriteString("import type { OperationSurface, OperationTypeIdentity, RouteTypeIdentity } from \"./runtime/identity.js\"\n")
	if hasVisibleInputSchemas(document) && hasVisibleResponseBodies(document) {
		output.WriteString("import { inputSchemas, outputSchemas } from \"./schemas/wire.js\"\n")
	} else if hasVisibleInputSchemas(document) {
		output.WriteString("import { inputSchemas } from \"./schemas/wire.js\"\n")
	} else if hasVisibleResponseBodies(document) {
		output.WriteString("import { outputSchemas } from \"./schemas/wire.js\"\n")
	}
	output.WriteString("import type * as Contract from \"./schemas/index.js\"\n\n")
	output.WriteString("import type * as Errors from \"./errors.js\"\n\n")
	output.WriteString("export {\n")
	output.WriteString("  APIError,\n")
	output.WriteString("  TransportErrorCode,\n")
	output.WriteString("  getErrorCode,\n")
	output.WriteString("  getRequestID,\n")
	output.WriteString("  isAPIError,\n")
	output.WriteString("  isErrorCode,\n")
	output.WriteString("} from \"./runtime/errors.js\"\n")
	output.WriteString("export type { MediaCodec, MediaStreamReader } from \"./runtime/codecs.js\"\n")
	output.WriteString("export type { ClientOptions, SecurityCredentialContext, SecurityCredentialProvider } from \"./runtime/configuration.js\"\n")
	output.WriteString("export type { TransportError } from \"./runtime/errors.js\"\n")
	output.WriteString("export type { LinkDefinition, LinkInputOverride, LinkInvocation, LinkParameterDefinition, RequiredLinkInvocation } from \"./runtime/links.js\"\n")
	output.WriteString("export type { OperationCall } from \"./runtime/callables.js\"\n")
	output.WriteString("export type { PaginateInput, PaginationPlan, PaginationProfile } from \"./runtime/pagination.js\"\n")
	output.WriteString("export type { RawResponse, RawResponseFor, RequestMetadata, RequestOptions } from \"./runtime/request.js\"\n")
	output.WriteString("export type { APIKeyCredential, HTTPBasicCredential, HTTPBearerCredential, HTTPCredential, MutualTLSCredential, OAuthCredential, SecurityCredential, SecurityCredentials, SecurityRequirementDefinition, SecuritySchemeDefinition } from \"./runtime/security.js\"\n")
	output.WriteString("export type { Transport, TransportCapabilities } from \"./runtime/transport.js\"\n\n")

	operationsByRoute := make(map[string]ir.Operation, len(document.Operations))
	for _, operation := range document.Operations {
		operationsByRoute[operationRouteKey(operation)] = operation
	}
	resourceReachable := make(map[string]bool)
	resourceOperationIDs(tree, resourceReachable)
	exactCallTypes := make(map[string]string, len(manifest.Operations))

	for _, item := range manifest.Operations {
		if item.Visibility == "hidden" {
			continue
		}
		operation := operationsByRoute[manifestRouteKey(item)]
		if err := emitOperationTypes(&output, document, operation, item); err != nil {
			return nil, err
		}
	}

	output.WriteString("/** Generated route contracts keyed by HTTP method and exact OpenAPI path. */\n")
	output.WriteString("export interface Routes {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		routeKey := manifestRouteKey(operation)
		operationName := operationTypeName(routeKey)
		inputType := operationInputAlias(operation)
		resourceInputType := inputType
		if len(operation.PathParameterOrder) > 0 {
			resourceInputType = operationName + "ResourceInput"
		}
		paginationType := "never"
		hasPagination := false
		if operationsByRoute[routeKey].PaginationPlan != nil {
			itemType, err := operationItemTypeForScope(document, operationsByRoute[routeKey], typeRenderContract)
			if err != nil {
				return nil, err
			}
			optionsRequired, err := operationRequiresOptions(document, operationsByRoute[routeKey])
			if err != nil {
				return nil, err
			}
			paginationType = paginationFunctionType(operation, itemType, optionsRequired)
			hasPagination = true
		}
		linksType, err := routeLinksType(document, links, routeKey)
		if err != nil {
			return nil, err
		}
		hasLinks := linksType != "never"
		streamType := "never"
		hasStream := false
		if stream, exists := streamForRoute(streams, routeKey); exists {
			streamType, err = streamFunctionType(document, stream)
			if err != nil {
				return nil, err
			}
			hasStream = true
		}
		callType := operationCallWithCapabilities(operationName+"Call", routeKey, hasPagination, hasLinks, hasStream)
		exactCallTypes[routeKey] = callType
		resourceCallType := "never"
		if operation.Visibility == "internal" || !resourceReachable[routeKey] {
			resourceCallType = "never"
		} else {
			resourceCallType = operationName + "ResourceCall"
			if len(operation.PathParameterOrder) == 0 {
				resourceCallType = operationCallWithCapabilities(resourceCallType, routeKey, hasPagination, hasLinks, hasStream)
			}
		}
		emitOperationEntryJSDoc(&output, "  ", operation)
		fmt.Fprintf(&output, "  readonly %s: {\n", quoteTS(routeKey))
		fmt.Fprintf(&output, "    /** Complete generated input type. */\n")
		fmt.Fprintf(&output, "    readonly input: %s\n", inputType)
		fmt.Fprintf(&output, "    /** Input remaining after resource path binding. */\n")
		fmt.Fprintf(&output, "    readonly resourceInput: %s\n", resourceInputType)
		fmt.Fprintf(&output, "    /** Per-request transport options. */\n")
		fmt.Fprintf(&output, "    readonly options: %sOptions\n", operationName)
		fmt.Fprintf(&output, "    /** Decoded successful output type. */\n")
		fmt.Fprintf(&output, "    readonly output: %s\n", operation.renderOutput(typeRenderContract))
		fmt.Fprintf(&output, "    /** Generated server and transport error union. */\n")
		fmt.Fprintf(&output, "    readonly error: %s\n", operation.renderError(typeRenderContract))
		fmt.Fprintf(&output, "    /** Successful raw response union. */\n")
		fmt.Fprintf(&output, "    readonly rawResponse: %sRawResponse\n", operationName)
		fmt.Fprintf(&output, "    /** Exact operation call including fixed capabilities. */\n")
		fmt.Fprintf(&output, "    readonly call: OperationMethod<%s>\n", quoteTS(routeKey))
		fmt.Fprintf(&output, "    /** Resource-oriented call after path binding, when available. */\n")
		fmt.Fprintf(&output, "    readonly resourceCall: %s\n", resourceCallType)
		fmt.Fprintf(&output, "    /** Pagination capability, when declared. */\n")
		fmt.Fprintf(&output, "    readonly pagination: %s\n", paginationType)
		fmt.Fprintf(&output, "    /** Response-link capabilities, when declared. */\n")
		fmt.Fprintf(&output, "    readonly links: %s\n", linksType)
		fmt.Fprintf(&output, "    /** Streaming capability, when declared. */\n")
		fmt.Fprintf(&output, "    readonly stream: %s\n", streamType)
		output.WriteString("  }\n")
	}
	output.WriteString("}\n\n")
	output.WriteString("interface ExactOperationMethods {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		routeKey := manifestRouteKey(operation)
		fmt.Fprintf(&output, "  /** Exact operation method for `%s`. */\n", sanitizeComment(routeKey))
		fmt.Fprintf(&output, "  readonly %s: %s\n", quoteTS(routeKey), exactCallTypes[routeKey])
	}
	output.WriteString("}\n\n")
	output.WriteString("interface ExactRawCalls {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		routeKey := manifestRouteKey(operation)
		fmt.Fprintf(&output, "  /** Exact raw operation method for `%s`. */\n", sanitizeComment(routeKey))
		fmt.Fprintf(&output, "  readonly %s: %sRawCall\n", quoteTS(routeKey), operationTypeName(routeKey))
	}
	output.WriteString("}\n\n")
	output.WriteString("interface ResourceRawCalls {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		routeKey := manifestRouteKey(operation)
		rawCallType := "never"
		if operation.Visibility == "public" && resourceReachable[routeKey] {
			rawCallType = operationTypeName(routeKey) + "ResourceRawCall"
		}
		fmt.Fprintf(&output, "  /** Raw resource-operation call for `%s`. */\n", sanitizeComment(routeKey))
		fmt.Fprintf(&output, "  readonly %s: %s\n", quoteTS(routeKey), rawCallType)
	}
	output.WriteString("}\n\n")
	output.WriteString("/** Raw-call member shared by resource-oriented operation contracts. */\n")
	output.WriteString("export interface ResourceRawCapability<Route extends keyof Routes> { readonly raw: RawCall<Route> }\n\n")
	output.WriteString("type PublicType<Value> = Value extends BinaryBody\n")
	output.WriteString("  ? Value\n")
	output.WriteString("  : Value extends (...args: any[]) => any\n")
	output.WriteString("    ? Value\n")
	output.WriteString("    : Value extends readonly unknown[]\n")
	output.WriteString("      ? { [Key in keyof Value]: PublicType<Value[Key]> }\n")
	output.WriteString("      : Value extends object\n")
	output.WriteString("        ? { [Key in keyof Value]: PublicType<Value[Key]> }\n")
	output.WriteString("        : Value\n\n")
	output.WriteString("/** Complete generated input for one exact route. */\n")
	output.WriteString("export type RouteInput<Route extends keyof Routes> = PublicType<Routes[Route][\"input\"]>\n\n")
	output.WriteString("/** Generated input remaining after resource path binding for one exact route. */\n")
	output.WriteString("export type RouteResourceInput<Route extends keyof Routes> = PublicType<Routes[Route][\"resourceInput\"]>\n\n")
	output.WriteString("/** Per-request transport options for one exact route. */\n")
	output.WriteString("export type RouteOptions<Route extends keyof Routes> = PublicType<Routes[Route][\"options\"]>\n\n")
	output.WriteString("/** Decoded successful output for one exact route. */\n")
	output.WriteString("export type RouteOutput<Route extends keyof Routes> = PublicType<Routes[Route][\"output\"]>\n\n")
	output.WriteString("/** Successful raw response union for one exact route. */\n")
	output.WriteString("export type RouteRawResponse<Route extends keyof Routes> = PublicType<Routes[Route][\"rawResponse\"]>\n\n")
	output.WriteString("/** Exact generated operation method for one route. */\n")
	output.WriteString("export type OperationMethod<Route extends keyof Routes> = ExactOperationMethods[Route] & OperationTypeIdentity<Route, \"exact\">\n\n")
	output.WriteString("/** Exact raw operation method for one route. */\n")
	output.WriteString("export type OperationRawCall<Route extends keyof Routes> = ExactRawCalls[Route] & RouteTypeIdentity<Route>\n\n")
	output.WriteString("/** Resource-oriented operation call for one exact route. */\n")
	output.WriteString("export type ResourceCall<Route extends keyof Routes> = Routes[Route][\"resourceCall\"] & RouteTypeIdentity<Route> & OperationTypeIdentity<Route, \"resource\">\n\n")
	output.WriteString("/** Raw resource-operation call for one exact route. */\n")
	output.WriteString("export type RawCall<Route extends keyof Routes> = ResourceRawCalls[Route] & RouteTypeIdentity<Route>\n\n")
	output.WriteString("/** Streaming operation call for one exact route. */\n")
	output.WriteString("export type StreamCall<Route extends keyof Routes> = Routes[Route][\"stream\"] & RouteTypeIdentity<Route>\n\n")
	output.WriteString("/** Pagination operation call for one exact route. */\n")
	output.WriteString("export type PaginateCall<Route extends keyof Routes> = Routes[Route][\"pagination\"] & RouteTypeIdentity<Route>\n\n")
	output.WriteString("/** Response-link calls for one exact route. */\n")
	output.WriteString("export type LinkCalls<Route extends keyof Routes> = Routes[Route][\"links\"] & RouteTypeIdentity<Route>\n\n")
	output.WriteString("/** Complete generated contract for one exact route. */\n")
	output.WriteString("export interface RouteContract<Route extends keyof Routes> {\n")
	output.WriteString("  /** Complete generated input for the route. */\n")
	output.WriteString("  readonly input: RouteInput<Route>\n")
	output.WriteString("  /** Generated input remaining after resource path binding. */\n")
	output.WriteString("  readonly resourceInput: RouteResourceInput<Route>\n")
	output.WriteString("  /** Per-request transport options. */\n")
	output.WriteString("  readonly options: RouteOptions<Route>\n")
	output.WriteString("  /** Decoded successful output. */\n")
	output.WriteString("  readonly output: RouteOutput<Route>\n")
	output.WriteString("  /** Generated error response union. */\n")
	output.WriteString("  readonly error: PublicType<Routes[Route][\"error\"]>\n")
	output.WriteString("  /** Successful raw response union. */\n")
	output.WriteString("  readonly rawResponse: RouteRawResponse<Route>\n")
	output.WriteString("  /** Exact generated operation method. */\n")
	output.WriteString("  readonly call: OperationMethod<Route>\n")
	output.WriteString("  /** Resource-oriented generated operation call. */\n")
	output.WriteString("  readonly resourceCall: ResourceCall<Route>\n")
	output.WriteString("  /** Pagination operation call when available. */\n")
	output.WriteString("  readonly pagination: PaginateCall<Route>\n")
	output.WriteString("  /** Response-link calls when available. */\n")
	output.WriteString("  readonly links: LinkCalls<Route>\n")
	output.WriteString("  /** Streaming operation call when available. */\n")
	output.WriteString("  readonly stream: StreamCall<Route>\n")
	output.WriteString("}\n\n")

	output.WriteString("/** Compatibility aliases keyed only by explicit OpenAPI operation IDs. */\n")
	output.WriteString("export interface Operations {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" || operation.OperationID == "" {
			continue
		}
		emitOperationEntryJSDoc(&output, "  ", operation)
		fmt.Fprintf(&output, "  readonly %s: Routes[%s]\n", quoteTS(operation.OperationID), quoteTS(manifestRouteKey(operation)))
	}
	output.WriteString("}\n\n")
	if err := emitOperationTypeHelpers(&output, manifest); err != nil {
		return nil, err
	}

	output.WriteString("/** Generated API client with route, operation-ID, and resource-oriented call surfaces. */\n")
	output.WriteString("export interface Client {\n")
	output.WriteString("  /** Every non-hidden operation keyed by method and exact path. */\n")
	output.WriteString("  readonly $routes: {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		emitOperationJSDoc(&output, "    ", operation)
		routeKey := manifestRouteKey(operation)
		fmt.Fprintf(&output, "    readonly %s: Routes[%s][\"call\"]\n", quoteTS(routeKey), quoteTS(routeKey))
	}
	output.WriteString("  }\n")
	output.WriteString("  /** Operations with explicit IDs keyed by their exact OpenAPI operation ID. */\n")
	output.WriteString("  readonly $operations: {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" || operation.OperationID == "" {
			continue
		}
		emitOperationJSDoc(&output, "    ", operation)
		fmt.Fprintf(&output, "    readonly %s: Operations[%s][\"call\"]\n", quoteTS(operation.OperationID), quoteTS(operation.OperationID))
	}
	output.WriteString("  }\n")
	if err := emitLinkInterface(&output, document, links); err != nil {
		return nil, err
	}
	if err := emitStreamInterface(&output, document, streams); err != nil {
		return nil, err
	}
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
		routeKey := manifestRouteKey(operation)
		binding := operationValueName(routeKey)
		baseBinding := binding
		linkValue, err := routeLinksValue(links, routeKey)
		if err != nil {
			return nil, err
		}
		_, hasStream := streamForRoute(streams, routeKey)
		if operationsByRoute[routeKey].PaginationPlan != nil || linkValue != "" || hasStream {
			baseBinding = operationBaseValueName(routeKey)
		}
		outputType := operation.renderOutput(typeRenderContract)
		definition, err := operationDefinition(document, operationsByRoute[routeKey], operation)
		if err != nil {
			return nil, err
		}
		inputType := "never"
		hasInput := false
		inputOptional := false
		if len(operation.InputTypes) > 0 {
			inputType = operationTypeName(routeKey) + "Input"
			hasInput = true
			inputRequired, err := operationInputRequired(document, operationsByRoute[routeKey], operation.InputTypes, false)
			if err != nil {
				return nil, err
			}
			inputOptional = !inputRequired
		}
		fmt.Fprintf(&output, "  const %s = bindOperation<%s, %s, %sOptions, %sRawResponse>(request, %s, %t, %t) as %sCall\n", baseBinding, inputType, outputType, operationTypeName(routeKey), operationTypeName(routeKey), definition, hasInput, inputOptional, operationTypeName(routeKey))
		paginationPlan := operationsByRoute[routeKey].PaginationPlan
		if paginationPlan != nil {
			itemType, err := operationItemTypeForScope(document, operationsByRoute[routeKey], typeRenderContract)
			if err != nil {
				return nil, err
			}
			paginationBinding := operationPaginationValueName(routeKey)
			plan, err := paginationRuntimePlanExpression(*paginationPlan)
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(&output, "  const %s = createPaginator<%s, %sInput, unknown, %s, %s, %s, %sOptions>((input, requestOptions) => %s.raw(input, requestOptions).then((response) => response.data), %s)\n", paginationBinding, itemType, operationTypeName(routeKey), quoteTS(paginationPlan.Mode), quoteTS(paginationPlan.Request.Cursor), quoteTS(paginationPlan.Request.Offset), operationTypeName(routeKey), baseBinding, plan)
		}
	}
	if err := emitLinkValues(&output, document, links); err != nil {
		return nil, err
	}
	if err := emitStreamValues(&output, document, streams); err != nil {
		return nil, err
	}
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		routeKey := manifestRouteKey(operation)
		properties := make([]runtimeProperty, 0, 3)
		if operationsByRoute[routeKey].PaginationPlan != nil {
			properties = append(properties, runtimeProperty{key: "paginate", value: operationPaginationValueName(routeKey)})
		}
		linkValue, err := routeLinksValue(links, routeKey)
		if err != nil {
			return nil, err
		}
		if linkValue != "" {
			properties = append(properties, runtimeProperty{key: "links", value: linkValue})
		}
		if stream, exists := streamForRoute(streams, routeKey); exists {
			properties = append(properties, runtimeProperty{key: "stream", value: stablePrivateIdentifier("stream-value", operationRouteKey(stream.Operation))})
		}
		if len(properties) != 0 {
			fmt.Fprintf(&output, "  const %s = assignCallableProperties(%s, %s) as unknown as Routes[%s][\"call\"]\n", operationValueName(routeKey), operationBaseValueName(routeKey), runtimeObjectExpression(properties), quoteTS(routeKey))
		}
	}
	output.WriteString("\n")
	if err := emitResourceTreeValues(&output, document, tree); err != nil {
		return nil, err
	}
	output.WriteString("\n  return {\n")
	routeValues := make([]runtimeProperty, 0)
	operationValues := make([]runtimeProperty, 0)
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		routeKey := manifestRouteKey(operation)
		routeValues = append(routeValues, runtimeProperty{key: routeKey, value: operationValueName(routeKey)})
		if operation.OperationID != "" {
			operationValues = append(operationValues, runtimeProperty{key: operation.OperationID, value: operationValueName(routeKey)})
		}
	}
	fmt.Fprintf(&output, "    $routes: %s as unknown as Client[\"$routes\"],\n", runtimeObjectExpression(routeValues))
	fmt.Fprintf(&output, "    $operations: %s as unknown as Client[\"$operations\"],\n", runtimeObjectExpression(operationValues))
	if err := emitLinkReturnValue(&output, links); err != nil {
		return nil, err
	}
	if err := emitStreamReturnValue(&output, streams); err != nil {
		return nil, err
	}
	for _, name := range sortedResourceMemberNames(tree) {
		if tree.children[name] != nil {
			fmt.Fprintf(&output, "    %s,\n", name)
			continue
		}
		fmt.Fprintf(&output, "    %s: ", name)
		if err := emitResourceOperationValue(&output, document, tree.operations[name], nil); err != nil {
			return nil, err
		}
		output.WriteString(",\n")
	}
	if paginated, ok := paginatedResourceNodeOperation(tree); ok {
		fmt.Fprintf(&output, "    paginate: %s,\n", operationPaginationValueName(manifestRouteKey(paginated)))
	}
	output.WriteString("  }\n")
	output.WriteString("}\n")
	return output.Bytes(), nil
}

func emitOperationTypeHelpers(output *bytes.Buffer, manifest Manifest) error {
	output.WriteString("interface RouteRequestSections {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		routeKey := manifestRouteKey(operation)
		operationName := operationTypeName(routeKey)
		fmt.Fprintf(output, "  /** Request sections for `%s`. */\n", sanitizeComment(routeKey))
		fmt.Fprintf(output, "  readonly %s: {\n", quoteTS(routeKey))
		for _, inputType := range operation.InputTypes {
			descriptor, err := requestInputSection(operationName, inputType)
			if err != nil {
				return fmt.Errorf("operation %s: %w", routeKey, err)
			}
			fmt.Fprintf(output, "    /** Generated %s request section. */\n", descriptor.sectionKey)
			fmt.Fprintf(output, "    readonly %s: %s\n", descriptor.sectionKey, inputType)
		}
		output.WriteString("  }\n")
	}
	output.WriteString("}\n\n")

	output.WriteString("interface OperationRoutes {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" || operation.OperationID == "" {
			continue
		}
		fmt.Fprintf(output, "  /** Exact route for operation ID `%s`. */\n", sanitizeComment(operation.OperationID))
		fmt.Fprintf(output, "  readonly %s: %s\n", quoteTS(operation.OperationID), quoteTS(manifestRouteKey(operation)))
	}
	output.WriteString("}\n\n")

	output.WriteString("/** Operation ID or generated exact/resource method accepted by operation type helpers. */\n")
	output.WriteString("export type OperationSource = keyof OperationRoutes | OperationMethod<keyof Routes> | ResourceCall<keyof Routes>\n\n")
	output.WriteString("type OperationIdentityOf<Source extends OperationSource> =\n")
	output.WriteString("  Source extends keyof OperationRoutes\n")
	output.WriteString("    ? { readonly route: OperationRoutes[Source]; readonly surface: \"exact\" }\n")
	output.WriteString("    : Source extends OperationTypeIdentity<infer Route, infer Surface>\n")
	output.WriteString("      ? { readonly route: Route; readonly surface: Surface }\n")
	output.WriteString("      : never\n\n")
	output.WriteString("type SelectedOperationContract<Route extends keyof Routes, Surface extends OperationSurface> =\n")
	output.WriteString("  Omit<RouteContract<Route>, \"input\" | \"call\"> & {\n")
	output.WriteString("    /** Input accepted by the selected callable surface. */\n")
	output.WriteString("    readonly input: Surface extends \"resource\" ? RouteResourceInput<Route> : RouteInput<Route>\n")
	output.WriteString("    /** Generated method for the selected callable surface. */\n")
	output.WriteString("    readonly call: Surface extends \"resource\" ? ResourceCall<Route> : OperationMethod<Route>\n")
	output.WriteString("  }\n\n")
	output.WriteString("/** Generated contract selected by operation ID or generated method type. */\n")
	output.WriteString("export type OperationContract<Source extends OperationSource> =\n")
	output.WriteString("  OperationIdentityOf<Source> extends { readonly route: infer Route; readonly surface: infer Surface }\n")
	output.WriteString("    ? Route extends keyof Routes\n")
	output.WriteString("      ? Surface extends OperationSurface\n")
	output.WriteString("        ? SelectedOperationContract<Route, Surface>\n")
	output.WriteString("        : never\n")
	output.WriteString("      : never\n")
	output.WriteString("    : never\n\n")
	output.WriteString("/** Generated input selected by operation ID or generated method type. */\n")
	output.WriteString("export type OperationInput<Source extends OperationSource> = PublicType<OperationContract<Source>[\"input\"]>\n\n")
	output.WriteString("/** Generated output selected by operation ID or generated method type. */\n")
	output.WriteString("export type OperationOutput<Source extends OperationSource> = PublicType<OperationContract<Source>[\"output\"]>\n\n")

	output.WriteString("type RouteSourceForSection<Section extends PropertyKey> = {\n")
	output.WriteString("  [Route in keyof RouteRequestSections]: Section extends keyof RouteRequestSections[Route] ? Route : never\n")
	output.WriteString("}[keyof RouteRequestSections]\n\n")
	output.WriteString("type SelectedRouteSection<Route extends keyof RouteRequestSections, Section extends PropertyKey> =\n")
	output.WriteString("  Section extends keyof RouteRequestSections[Route] ? RouteRequestSections[Route][Section] : never\n\n")
	output.WriteString("type OperationIDSourceForSection<Section extends PropertyKey> = {\n")
	output.WriteString("  [ID in keyof OperationRoutes]: Section extends keyof RouteRequestSections[OperationRoutes[ID]] ? ID : never\n")
	output.WriteString("}[keyof OperationRoutes]\n\n")
	output.WriteString("type ExactOperationSourceForSection<Section extends PropertyKey> = {\n")
	output.WriteString("  [Route in keyof RouteRequestSections]: Section extends keyof RouteRequestSections[Route] ? OperationMethod<Route> : never\n")
	output.WriteString("}[keyof RouteRequestSections]\n\n")
	output.WriteString("type ResourceOperationSourceForSection<Section extends PropertyKey> = {\n")
	output.WriteString("  [Route in keyof RouteRequestSections]: Routes[Route][\"resourceCall\"] extends never\n")
	output.WriteString("    ? never\n")
	output.WriteString("    : Section extends \"path\"\n")
	output.WriteString("      ? never\n")
	output.WriteString("      : Section extends keyof RouteRequestSections[Route]\n")
	output.WriteString("        ? ResourceCall<Route>\n")
	output.WriteString("        : never\n")
	output.WriteString("}[keyof RouteRequestSections]\n\n")
	output.WriteString("type OperationSourceForSection<Section extends PropertyKey> =\n")
	output.WriteString("  | OperationIDSourceForSection<Section>\n")
	output.WriteString("  | ExactOperationSourceForSection<Section>\n")
	output.WriteString("  | ResourceOperationSourceForSection<Section>\n\n")
	output.WriteString("type SelectedOperationSections<Source extends OperationSource> =\n")
	output.WriteString("  OperationIdentityOf<Source> extends { readonly route: infer Route; readonly surface: infer Surface }\n")
	output.WriteString("    ? Route extends keyof RouteRequestSections\n")
	output.WriteString("      ? Surface extends \"resource\"\n")
	output.WriteString("        ? Omit<RouteRequestSections[Route], \"path\">\n")
	output.WriteString("        : RouteRequestSections[Route]\n")
	output.WriteString("      : never\n")
	output.WriteString("    : never\n\n")
	output.WriteString("type SelectedOperationSection<Source extends OperationSource, Section extends PropertyKey> =\n")
	output.WriteString("  Section extends keyof SelectedOperationSections<Source> ? SelectedOperationSections<Source>[Section] : never\n\n")

	for _, descriptor := range requestInputSectionDescriptors {
		fmt.Fprintf(output, "/** Generated %s request section for one exact route. */\n", descriptor.sectionKey)
		fmt.Fprintf(output, "export type Route%s<Route extends RouteSourceForSection<%s>> = PublicType<SelectedRouteSection<Route, %s>>\n\n", descriptor.publicHelperSuffix, quoteTS(descriptor.sectionKey), quoteTS(descriptor.sectionKey))
		fmt.Fprintf(output, "/** Generated %s request section selected by operation ID or generated method type. */\n", descriptor.sectionKey)
		fmt.Fprintf(output, "export type Operation%s<Source extends OperationSourceForSection<%s>> = PublicType<SelectedOperationSection<Source, %s>>\n\n", descriptor.publicHelperSuffix, quoteTS(descriptor.sectionKey), quoteTS(descriptor.sectionKey))
	}

	parameterLocations := make([]string, 0, len(requestInputSectionDescriptors)-1)
	for _, descriptor := range requestInputSectionDescriptors {
		if descriptor.parameterLocation {
			parameterLocations = append(parameterLocations, quoteTS(descriptor.sectionKey))
		}
	}
	output.WriteString("type RequestParameterLocation = " + strings.Join(parameterLocations, " | ") + "\n\n")
	output.WriteString("type RouteParameterLocation<Route extends keyof RouteRequestSections> =\n")
	output.WriteString("  Extract<keyof RouteRequestSections[Route], RequestParameterLocation>\n\n")
	output.WriteString("/** One generated parameter value for an exact route. */\n")
	output.WriteString("export type RouteParameter<\n")
	output.WriteString("  Route extends keyof RouteRequestSections,\n")
	output.WriteString("  Location extends RouteParameterLocation<Route>,\n")
	output.WriteString("  Name extends keyof SelectedRouteSection<Route, Location>,\n")
	output.WriteString("> = PublicType<Exclude<SelectedRouteSection<Route, Location>[Name], undefined>>\n\n")
	output.WriteString("type OperationParameterLocation<Source extends OperationSource> =\n")
	output.WriteString("  Extract<keyof SelectedOperationSections<Source>, RequestParameterLocation>\n\n")
	output.WriteString("/** One generated parameter value selected by operation ID or generated method type. */\n")
	output.WriteString("export type OperationParameter<\n")
	output.WriteString("  Source extends OperationSource,\n")
	output.WriteString("  Location extends OperationParameterLocation<Source>,\n")
	output.WriteString("  Name extends keyof SelectedOperationSection<Source, Location>,\n")
	output.WriteString("> = PublicType<Exclude<SelectedOperationSection<Source, Location>[Name], undefined>>\n\n")
	return nil
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

func resourceMemberOperationType(operation ManifestOperation) string {
	return operationResourceFunctionType(operation)
}

func resourceMemberOperation(node *resourceNode, name string) (ManifestOperation, bool) {
	operation, ok := node.operations[name]
	return operation, ok
}

func emitResourceMemberInterface(output *bytes.Buffer, document *ir.Document, node *resourceNode, name, indent string) error {
	operation, hasOperation := resourceMemberOperation(node, name)
	child := node.children[name]
	if hasOperation {
		emitResourceOperationJSDoc(output, indent, operation)
	} else {
		output.WriteString(indent + "/** Nested resource path segment. */\n")
	}
	fmt.Fprintf(output, "%sreadonly %s: ", indent, name)
	if hasOperation {
		output.WriteString(resourceMemberOperationType(operation))
		if child != nil {
			output.WriteString(" & ")
		}
	}
	if child != nil {
		return emitResourceNodeInterface(output, document, child, indent)
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

func manifestHasPagination(manifest Manifest) bool {
	for _, operation := range manifest.Operations {
		if operation.Visibility != "hidden" && operation.Pagination != "" {
			return true
		}
	}
	return false
}

func emitResourceTreeInterface(output *bytes.Buffer, document *ir.Document, root *resourceNode) error {
	for _, name := range sortedResourceMemberNames(root) {
		if err := emitResourceMemberInterface(output, document, root, name, "  "); err != nil {
			return err
		}
		output.WriteString("\n")
	}
	if paginated, ok := paginatedResourceNodeOperation(root); ok {
		fmt.Fprintf(output, "  /** Lazily iterates every item from exact operation `%s` pagination. */\n", sanitizeComment(paginated.OperationID))
		fmt.Fprintf(output, "  readonly paginate: PaginateCall<%s>\n", quoteTS(manifestRouteKey(paginated)))
	}
	return nil
}

func emitResourceNodeInterface(output *bytes.Buffer, document *ir.Document, node *resourceNode, indent string) error {
	output.WriteString("{\n")
	memberIndent := indent + "  "
	for _, name := range sortedResourceMemberNames(node) {
		if err := emitResourceMemberInterface(output, document, node, name, memberIndent); err != nil {
			return err
		}
		output.WriteString("\n")
	}
	if paginated, ok := paginatedResourceNodeOperation(node); ok {
		fmt.Fprintf(output, "%s/** Lazily iterates every item from exact operation `%s` pagination. */\n", memberIndent, sanitizeComment(paginated.OperationID))
		fmt.Fprintf(output, "%sreadonly paginate: PaginateCall<%s>\n", memberIndent, quoteTS(manifestRouteKey(paginated)))
	}
	if node.parameterChild != nil {
		parameter := node.parameterChild.parameter
		parameterType, err := schemaTypeForScope(document, parameter.Schema, projectionInput, typeRenderContract)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "%s/** Selects one resource by the `%s` path parameter. */\n", memberIndent, parameter.Name)
		fmt.Fprintf(output, "%s(%s: %s): ", memberIndent, parameter.Binding, parameterType)
		if err := emitResourceNodeInterface(output, document, node.parameterChild, memberIndent); err != nil {
			return err
		}
		output.WriteString("\n")
	}
	output.WriteString(indent + "}")
	return nil
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

func emitResourceTreeValues(output *bytes.Buffer, document *ir.Document, root *resourceNode) error {
	for _, name := range sortedResourceChildNames(root) {
		fmt.Fprintf(output, "  const %s = ", name)
		if err := emitResourceMemberValue(output, document, root, name, nil, "  "); err != nil {
			return err
		}
		output.WriteString("\n")
	}
	return nil
}

func emitResourceNodeValue(output *bytes.Buffer, document *ir.Document, node *resourceNode, bound []string, indent string) error {
	if node.parameterChild == nil {
		return emitResourceNodeObject(output, document, node, bound, indent)
	}
	parameter := node.parameterChild.parameter
	parameterType, err := schemaTypeForScope(document, parameter.Schema, projectionInput, typeRenderContract)
	if err != nil {
		return err
	}
	nextBound := append(append([]string{}, bound...), parameter.Binding)
	output.WriteString("assignCallableProperties(\n")
	fmt.Fprintf(output, "%s  (%s: %s) => (", indent, parameter.Binding, parameterType)
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

func emitResourceNodeObject(output *bytes.Buffer, document *ir.Document, node *resourceNode, bound []string, indent string) error {
	output.WriteString("{\n")
	memberIndent := indent + "  "
	for _, name := range sortedResourceMemberNames(node) {
		fmt.Fprintf(output, "%s%s: ", memberIndent, name)
		if err := emitResourceMemberValue(output, document, node, name, bound, memberIndent); err != nil {
			return err
		}
		output.WriteString(",\n")
	}
	if paginated, ok := paginatedResourceNodeOperation(node); ok {
		fmt.Fprintf(output, "%spaginate: %s,\n", memberIndent, operationPaginationValueName(manifestRouteKey(paginated)))
	}
	output.WriteString(indent + "}")
	return nil
}

func emitResourceMemberValue(output *bytes.Buffer, document *ir.Document, node *resourceNode, name string, bound []string, indent string) error {
	operation, hasOperation := node.operations[name]
	child := node.children[name]
	if hasOperation && child != nil {
		output.WriteString("assignCallableProperties(")
		if err := emitResourceOperationValue(output, document, operation, bound); err != nil {
			return err
		}
		output.WriteString(", ")
		if err := emitResourceNodeValue(output, document, child, bound, indent); err != nil {
			return err
		}
		output.WriteString(")")
		return nil
	}
	if hasOperation {
		return emitResourceOperationValue(output, document, operation, bound)
	}
	return emitResourceNodeValue(output, document, child, bound, indent)
}

func emitResourceOperationValue(output *bytes.Buffer, document *ir.Document, operation ManifestOperation, bound []string) error {
	routeKey := manifestRouteKey(operation)
	property := operationValueName(routeKey)
	if len(operation.PathParameterOrder) == 0 {
		output.WriteString(property)
		return nil
	}
	values := make([]string, 0, len(operation.PathParameterOrder))
	for index, parameter := range operation.PathParameterOrder {
		if index >= len(bound) {
			return fmt.Errorf("resource operation %s is missing bound path parameter %q", operation.OperationID, parameter)
		}
		values = append(values, quoteTS(parameter)+": "+bound[index])
	}
	name := operationTypeName(routeKey)
	hasInput := len(operation.InputTypes) > 1
	inputOptional := false
	if hasInput {
		inputRequired, err := operationInputRequired(document, findOperation(document, routeKey), operation.InputTypes, true)
		if err != nil {
			return err
		}
		inputOptional = !inputRequired
	}
	fmt.Fprintf(output, "bindPathOperation<%sInput, %sResourceInput, %s, %sOptions, %sRawResponse>(%s, { %s }, %t, %t)", name, name, operation.renderOutput(typeRenderContract), name, name, property, strings.Join(values, ", "), hasInput, inputOptional)
	return nil
}

func findOperation(document *ir.Document, identity string) ir.Operation {
	for _, operation := range document.Operations {
		if operationRouteKey(operation) == identity || operation.OperationID == identity {
			return operation
		}
	}
	return ir.Operation{RouteKey: identity}
}

func operationResourceFunctionType(operation ManifestOperation) string {
	return "ResourceCall<" + quoteTS(manifestRouteKey(operation)) + ">"
}

func operationCallWithCapabilities(callType, routeKey string, pagination, links, stream bool) string {
	if pagination {
		callType = "(" + callType + ") & { readonly paginate: PaginateCall<" + quoteTS(routeKey) + "> }"
	}
	if links {
		callType = "(" + callType + ") & { readonly links: LinkCalls<" + quoteTS(routeKey) + "> }"
	}
	if stream {
		callType = "(" + callType + ") & { readonly stream: StreamCall<" + quoteTS(routeKey) + "> }"
	}
	return callType
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
