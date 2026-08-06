package typescript

import (
	"bytes"
	"fmt"
	"strings"

	"openapi-sdkgen/internal/compiler/ir"
)

func emitClientArtifacts(document *ir.Document, manifest Manifest, plan *semanticModulePlan, links []generatedLink, streams []generatedStream) ([]Artifact, []byte, error) {
	if plan == nil {
		return nil, nil, fmt.Errorf("internal TypeScript target: prepared plan has no semantic modules")
	}
	typesSource, err := emitClientTypes(manifest, plan, links, streams)
	if err != nil {
		return nil, nil, err
	}
	factorySource, err := emitClientFactory(document, plan, links, streams)
	if err != nil {
		return nil, nil, err
	}
	indexSource, err := emitClientIndex(plan)
	if err != nil {
		return nil, nil, err
	}
	return []Artifact{
		{Path: plan.fixed["client-types"], Data: generatedSource(typesSource)},
		{Path: plan.fixed["client-factory"], Data: generatedSource(factorySource)},
		{Path: plan.fixed["client-index"], Data: generatedSource(indexSource)},
	}, indexSource, nil
}

func emitClientTypes(manifest Manifest, plan *semanticModulePlan, links []generatedLink, streams []generatedStream) ([]byte, error) {
	helpers, err := relativeModuleSpecifier(plan.fixed["client-types"], plan.fixed["route-helpers"])
	if err != nil {
		return nil, err
	}
	resources, err := relativeModuleSpecifier(plan.fixed["client-types"], plan.fixed["resource-index"])
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "import type { LinkCalls, OperationMethod, StreamCall } from %s\n", quoteTS(helpers))
	fmt.Fprintf(&output, "import type { Surface as Resources } from %s\n\n", quoteTS(resources))
	output.WriteString("/** Generated API client with route, operation-ID, and resource-oriented call surfaces. */\n")
	output.WriteString("export interface Client extends Resources {\n")
	output.WriteString("  /** Every non-hidden operation keyed by method and exact path. */\n")
	output.WriteString("  readonly $routes: {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		emitOperationJSDoc(&output, "    ", operation)
		route := manifestRouteKey(operation)
		fmt.Fprintf(&output, "    readonly %s: OperationMethod<%s>\n", quoteTS(route), quoteTS(route))
	}
	output.WriteString("  }\n")
	output.WriteString("  /** Operations with explicit IDs keyed by their exact OpenAPI operation ID. */\n")
	output.WriteString("  readonly $operations: {\n")
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" || operation.OperationID == "" {
			continue
		}
		emitOperationJSDoc(&output, "    ", operation)
		fmt.Fprintf(&output, "    readonly %s: OperationMethod<%s>\n", quoteTS(operation.OperationID), quoteTS(manifestRouteKey(operation)))
	}
	output.WriteString("  }\n")
	if len(links) > 0 {
		output.WriteString("  /** Typed response links keyed by OpenAPI operation ID. */\n")
		output.WriteString("  readonly $links: {\n")
		for _, source := range linkSourceOperations(links) {
			if source.OperationID == "" {
				continue
			}
			fmt.Fprintf(&output, "    readonly %s: LinkCalls<%s>\n", quoteTS(source.OperationID), quoteTS(operationRouteKey(source)))
		}
		output.WriteString("  }\n")
	}
	if len(streams) > 0 {
		output.WriteString("  /** Lazy typed response streams keyed by OpenAPI operation ID. */\n")
		output.WriteString("  readonly $streams: {\n")
		for _, stream := range streams {
			if stream.Operation.OperationID == "" {
				continue
			}
			fmt.Fprintf(&output, "    readonly %s: StreamCall<%s>\n", quoteTS(stream.Operation.OperationID), quoteTS(operationRouteKey(stream.Operation)))
		}
		output.WriteString("  }\n")
	}
	output.WriteString("}\n")
	return output.Bytes(), nil
}

func emitClientFactory(document *ir.Document, plan *semanticModulePlan, links []generatedLink, streams []generatedStream) ([]byte, error) {
	artifact := plan.fixed["client-factory"]
	configuration, err := relativeModuleSpecifier(artifact, "internal/runtime/configuration.ts")
	if err != nil {
		return nil, err
	}
	http, err := relativeModuleSpecifier(artifact, "internal/runtime/http.ts")
	if err != nil {
		return nil, err
	}
	registry, err := relativeModuleSpecifier(artifact, plan.fixed["client-registry"])
	if err != nil {
		return nil, err
	}
	resources, err := relativeModuleSpecifier(artifact, plan.fixed["resource-index"])
	if err != nil {
		return nil, err
	}
	types, err := relativeModuleSpecifier(artifact, plan.fixed["client-types"])
	if err != nil {
		return nil, err
	}
	wire, err := relativeModuleSpecifier(artifact, plan.fixed["schema-wire"])
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "import type { ClientOptions } from %s\n", quoteTS(configuration))
	fmt.Fprintf(&output, "import { createRequest } from %s\n", quoteTS(http))
	fmt.Fprintf(&output, "import { createCallableRegistry } from %s\n", quoteTS(registry))
	fmt.Fprintf(&output, "import { build as buildResources } from %s\n", quoteTS(resources))
	fmt.Fprintf(&output, "import type { Client } from %s\n", quoteTS(types))
	inputSchemas := hasVisibleInputSchemas(document)
	outputSchemas := hasVisibleResponseBodies(document)
	if inputSchemas && outputSchemas {
		fmt.Fprintf(&output, "import { inputSchemas, outputSchemas } from %s\n", quoteTS(wire))
	} else if inputSchemas {
		fmt.Fprintf(&output, "import { inputSchemas } from %s\n", quoteTS(wire))
	} else if outputSchemas {
		fmt.Fprintf(&output, "import { outputSchemas } from %s\n", quoteTS(wire))
	}
	output.WriteString("\n/**\n * Creates a generated API client.\n *\n * The base URL must include the selected API version prefix, such as `/v1`.\n *\n * @param options Deployment URL, fetch implementation, and transport defaults.\n * @returns A typed {@link Client}.\n */\n")
	output.WriteString("export function createClient(options: ClientOptions): Client {\n")
	output.WriteString("  const request = createRequest(options)\n")
	arguments := []string{"request"}
	if inputSchemas {
		arguments = append(arguments, "inputSchemas")
	} else if outputSchemas {
		arguments = append(arguments, "undefined")
	}
	if outputSchemas {
		arguments = append(arguments, "outputSchemas")
	}
	fmt.Fprintf(&output, "  const registry = createCallableRegistry(%s)\n", strings.Join(arguments, ", "))
	output.WriteString("  const resources = buildResources(registry)\n")
	output.WriteString("  return {\n")
	output.WriteString("    $routes: registry.routes,\n")
	output.WriteString("    $operations: registry.operations,\n")
	if len(links) > 0 {
		output.WriteString("    $links: registry.links,\n")
	}
	if len(streams) > 0 {
		output.WriteString("    $streams: registry.streams,\n")
	}
	output.WriteString("    ...resources,\n")
	output.WriteString("  }\n")
	output.WriteString("}\n")
	return output.Bytes(), nil
}

func emitClientIndex(plan *semanticModulePlan) ([]byte, error) {
	artifact := plan.fixed["client-index"]
	types, err := relativeModuleSpecifier(artifact, plan.fixed["client-types"])
	if err != nil {
		return nil, err
	}
	factory, err := relativeModuleSpecifier(artifact, plan.fixed["client-factory"])
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "export type { Client } from %s\n", quoteTS(types))
	fmt.Fprintf(&output, "export { createClient } from %s\n", quoteTS(factory))
	return output.Bytes(), nil
}

func publicRuntimeExportSource() []byte {
	return []byte(`export type { MediaCodec, MediaStreamReader } from "./runtime/codecs.js"
export type { ClientOptions, SecurityCredentialContext, SecurityCredentialProvider } from "./runtime/configuration.js"
export type { TransportError } from "./runtime/errors.js"
export type { LinkDefinition, LinkInputOverride, LinkInvocation, LinkParameterDefinition, RequiredLinkInvocation } from "./runtime/links.js"
export type { OperationCall } from "./runtime/callables.js"
export type { PaginateInput, PaginationPlan, PaginationProfile } from "./runtime/pagination.js"
export type { RawResponse, RawResponseFor, RequestMetadata, RequestOptions } from "./runtime/request.js"
export type { APIKeyCredential, HTTPBasicCredential, HTTPBearerCredential, HTTPCredential, MutualTLSCredential, OAuthCredential, SecurityCredential, SecurityCredentials, SecurityRequirementDefinition, SecuritySchemeDefinition } from "./runtime/security.js"
export type { Transport, TransportCapabilities } from "./runtime/transport.js"
`)
}
