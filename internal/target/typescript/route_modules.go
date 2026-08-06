package typescript

import (
	"bytes"
	"fmt"
	"strings"
)

func emitRouteArtifacts(manifest Manifest, plan *semanticModulePlan) ([]Artifact, error) {
	if plan == nil {
		return nil, fmt.Errorf("internal TypeScript target: prepared plan has no semantic modules")
	}
	index, err := emitRouteIndex(manifest, plan)
	if err != nil {
		return nil, err
	}
	inputs, err := emitRouteInputs(manifest, plan)
	if err != nil {
		return nil, err
	}
	helpers, err := emitRouteHelpers(manifest, plan)
	if err != nil {
		return nil, err
	}
	return []Artifact{
		{Path: plan.fixed["route-index"], Data: generatedSource(index)},
		{Path: plan.fixed["route-inputs"], Data: generatedSource(inputs)},
		{Path: plan.fixed["route-helpers"], Data: generatedSource(helpers)},
	}, nil
}

func emitRouteIndex(manifest Manifest, plan *semanticModulePlan) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("/** Generated route contracts keyed by HTTP method and exact OpenAPI path. */\n")
	output.WriteString("export interface Routes {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		route := manifestRouteKey(operation)
		specifier, err := operationModuleSpecifier(plan.fixed["route-index"], route, plan)
		if err != nil {
			return nil, err
		}
		emitOperationEntryJSDoc(&output, "  ", operation)
		fmt.Fprintf(&output, "  readonly %s: import(%s).Contract\n", quoteTS(route), quoteTS(specifier))
	}
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
	output.WriteString("/** Exact route selected by each explicit OpenAPI operation ID. */\n")
	output.WriteString("export interface OperationRoutes {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" || operation.OperationID == "" {
			continue
		}
		fmt.Fprintf(&output, "  readonly %s: %s\n", quoteTS(operation.OperationID), quoteTS(manifestRouteKey(operation)))
	}
	output.WriteString("}\n")
	return output.Bytes(), nil
}

func emitRouteInputs(manifest Manifest, plan *semanticModulePlan) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("/** Request sections keyed by exact generated route. */\n")
	output.WriteString("export interface RouteRequestSections {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		route := manifestRouteKey(operation)
		specifier, err := operationModuleSpecifier(plan.fixed["route-inputs"], route, plan)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&output, "  /** Request sections for `%s`. */\n", sanitizeComment(route))
		fmt.Fprintf(&output, "  readonly %s: import(%s).RequestInputs\n", quoteTS(route), quoteTS(specifier))
	}
	output.WriteString("}\n")
	return output.Bytes(), nil
}

func emitRouteHelpers(manifest Manifest, plan *semanticModulePlan) ([]byte, error) {
	request, err := relativeModuleSpecifier(plan.fixed["route-helpers"], "internal/runtime/request.ts")
	if err != nil {
		return nil, err
	}
	identity, err := relativeModuleSpecifier(plan.fixed["route-helpers"], "internal/runtime/identity.ts")
	if err != nil {
		return nil, err
	}
	index, err := relativeModuleSpecifier(plan.fixed["route-helpers"], plan.fixed["route-index"])
	if err != nil {
		return nil, err
	}
	inputs, err := relativeModuleSpecifier(plan.fixed["route-helpers"], plan.fixed["route-inputs"])
	if err != nil {
		return nil, err
	}

	var output bytes.Buffer
	fmt.Fprintf(&output, "import type { BinaryBody } from %s\n", quoteTS(request))
	fmt.Fprintf(&output, "import type { OperationSurface, OperationTypeIdentity, RouteTypeIdentity } from %s\n", quoteTS(identity))
	fmt.Fprintf(&output, "import type { OperationRoutes, Routes } from %s\n", quoteTS(index))
	fmt.Fprintf(&output, "import type { RouteRequestSections } from %s\n\n", quoteTS(inputs))

	output.WriteString("interface ExactRawCalls {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		route := manifestRouteKey(operation)
		specifier, err := operationModuleSpecifier(plan.fixed["route-helpers"], route, plan)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&output, "  readonly %s: import(%s).RawCall\n", quoteTS(route), quoteTS(specifier))
	}
	output.WriteString("}\n\n")
	output.WriteString("interface ResourceRawCalls {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		route := manifestRouteKey(operation)
		specifier, err := operationModuleSpecifier(plan.fixed["route-helpers"], route, plan)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&output, "  readonly %s: import(%s).ResourceRawCall\n", quoteTS(route), quoteTS(specifier))
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
	output.WriteString("export type OperationMethod<Route extends keyof Routes> = Routes[Route][\"call\"] & OperationTypeIdentity<Route, \"exact\">\n\n")
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
	output.WriteString("  readonly input: RouteInput<Route>\n")
	output.WriteString("  readonly resourceInput: RouteResourceInput<Route>\n")
	output.WriteString("  readonly options: RouteOptions<Route>\n")
	output.WriteString("  readonly output: RouteOutput<Route>\n")
	output.WriteString("  readonly error: PublicType<Routes[Route][\"error\"]>\n")
	output.WriteString("  readonly rawResponse: RouteRawResponse<Route>\n")
	output.WriteString("  readonly call: OperationMethod<Route>\n")
	output.WriteString("  readonly resourceCall: ResourceCall<Route>\n")
	output.WriteString("  readonly pagination: PaginateCall<Route>\n")
	output.WriteString("  readonly links: LinkCalls<Route>\n")
	output.WriteString("  readonly stream: StreamCall<Route>\n")
	output.WriteString("}\n\n")
	if err := emitRouteOperationHelpers(&output, manifest); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func emitRouteOperationHelpers(output *bytes.Buffer, manifest Manifest) error {
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
	output.WriteString("    readonly input: Surface extends \"resource\" ? RouteResourceInput<Route> : RouteInput<Route>\n")
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
	emitRequestSectionHelpers(output)
	return nil
}

func emitRequestSectionHelpers(output *bytes.Buffer) {
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
	output.WriteString("type RouteParameterLocation<Route extends keyof RouteRequestSections> = Extract<keyof RouteRequestSections[Route], RequestParameterLocation>\n\n")
	output.WriteString("/** One generated parameter value for an exact route. */\n")
	output.WriteString("export type RouteParameter<Route extends keyof RouteRequestSections, Location extends RouteParameterLocation<Route>, Name extends keyof SelectedRouteSection<Route, Location>> = PublicType<Exclude<SelectedRouteSection<Route, Location>[Name], undefined>>\n\n")
	output.WriteString("type OperationParameterLocation<Source extends OperationSource> = Extract<keyof SelectedOperationSections<Source>, RequestParameterLocation>\n\n")
	output.WriteString("/** One generated parameter value selected by operation ID or generated method type. */\n")
	output.WriteString("export type OperationParameter<Source extends OperationSource, Location extends OperationParameterLocation<Source>, Name extends keyof SelectedOperationSection<Source, Location>> = PublicType<Exclude<SelectedOperationSection<Source, Location>[Name], undefined>>\n")
}

func operationModuleSpecifier(from, route string, plan *semanticModulePlan) (string, error) {
	path, exists := plan.operationByRoute[route]
	if !exists {
		return "", fmt.Errorf("route %q has no operation module", route)
	}
	return relativeModuleSpecifier(from, path)
}
