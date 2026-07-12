package typescript

import (
	"bytes"
	_ "embed"
	"fmt"
	"sort"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/generator"
)

//go:embed runtime.js
var javascriptRuntimeTemplate []byte

// JavaScriptGenerator emits a source-only, native ESM SDK. It shares the same
// compiler, wire transformations, and capability diagnostics as TypeScript;
// JavaScript consumers use the stable $operations surface instead of static
// declaration types.
type JavaScriptGenerator struct{}

// Name returns the CLI target name.
func (JavaScriptGenerator) Name() string { return "javascript" }

// Generate emits importable JavaScript source files without a package build.
func (JavaScriptGenerator) Generate(document *ir.Document, _ generator.Options) ([]generator.Artifact, error) {
	return JavaScriptSourceArtifacts(document)
}

// JavaScriptSourceArtifacts emits native ESM files a JavaScript application can
// import through a relative path immediately after generation.
func JavaScriptSourceArtifacts(document *ir.Document) ([]generator.Artifact, error) {
	if document == nil {
		return nil, fmt.Errorf("IR document is nil")
	}
	if err := validateSchemaSupportForTarget(document, "JavaScript"); err != nil {
		return nil, err
	}
	if err := validateOpenAPISupportForTarget(document, "JavaScript"); err != nil {
		return nil, err
	}
	if err := validateOperationSymbols(document); err != nil {
		return nil, err
	}
	manifest, err := buildManifest(document)
	if err != nil {
		return nil, err
	}
	client, err := emitJavaScriptClient(document, manifest)
	if err != nil {
		return nil, err
	}
	metadata, err := emitMetadata(document, false)
	if err != nil {
		return nil, err
	}
	artifacts := []generator.Artifact{
		{Path: "index.js", Data: generatedSource([]byte("export * from \"./generated/client.js\"\nexport * from \"./generated/metadata.js\"\n"))},
		{Path: "generated/client.js", Data: generatedSource(client)},
		{Path: "generated/metadata.js", Data: generatedSource(metadata)},
		{Path: "generated/runtime.js", Data: generatedSource(javascriptRuntimeTemplate)},
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts, nil
}

func emitJavaScriptClient(document *ir.Document, manifest Manifest) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("import { bindOperation, createRequest } from \"./runtime.js\"\n\n")
	if hasVisibleInputSchemas(document) {
		if err := emitJavaScriptWireComponents(&output, document, "inputSchemas", projectionInput); err != nil {
			return nil, err
		}
	}
	if hasVisibleResponseBodies(document) {
		if err := emitJavaScriptWireComponents(&output, document, "outputSchemas", projectionOutput); err != nil {
			return nil, err
		}
	}
	output.WriteString("export { APIError, TransportErrorCode, getErrorCode, getRequestID, isAPIError, isErrorCode } from \"./runtime.js\"\n\n")
	output.WriteString("/**\n * Creates a generated JavaScript API client.\n *\n * Operations are keyed by their exact OpenAPI operation ID under `$operations`.\n */\n")
	output.WriteString("export function createClient(options) {\n")
	output.WriteString("  const request = createRequest(options)\n")
	output.WriteString("  return Object.freeze({\n    $operations: Object.freeze({\n")
	operationsByID := make(map[string]ir.Operation, len(document.Operations))
	for _, operation := range document.Operations {
		operationsByID[operation.OperationID] = operation
	}
	for _, item := range manifest.Operations {
		if item.Visibility == "hidden" {
			continue
		}
		operation := operationsByID[item.OperationID]
		definition, err := operationDefinition(document, operation, item)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&output, "      %s: bindOperation(request, %s, %t),\n", quoteTS(item.OperationID), definition, len(item.InputTypes) > 0)
	}
	output.WriteString("    }),\n  })\n}\n")
	return output.Bytes(), nil
}
