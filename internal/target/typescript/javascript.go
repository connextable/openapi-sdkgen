package typescript

import (
	"bytes"
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/compiler/naming"
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

// SupportsAddon reports JavaScript add-on support. Server output is currently
// intentionally TypeScript-only because it is a typed host integration API.
func (JavaScriptGenerator) SupportsAddon(generator.Addon) bool { return false }

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
	errors, err := emitJavaScriptErrors(document)
	if err != nil {
		return nil, err
	}
	declarations, err := emitJavaScriptDeclarationArtifacts(document, manifest)
	if err != nil {
		return nil, err
	}
	artifacts := []generator.Artifact{
		{Path: "index.js", Data: generatedSource([]byte("export * from \"./generated/client.js\"\nexport * from \"./generated/errors.js\"\n"))},
		{Path: "generated/client.js", Data: generatedSource(client)},
		{Path: "metadata.js", Data: generatedSource(metadata)},
		{Path: "generated/errors.js", Data: generatedSource(errors)},
		{Path: "generated/runtime.js", Data: generatedSource(javascriptRuntimeTemplate)},
	}
	artifacts = append(artifacts, declarations...)
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts, nil
}

func emitJavaScriptClient(document *ir.Document, manifest Manifest) ([]byte, error) {
	tree, err := buildResourceTree(document, manifest)
	if err != nil {
		return nil, err
	}
	hasPathOperations := resourceTreeHasPathOperations(tree)
	hasPagination := resourceTreeHasPagination(tree)
	var output bytes.Buffer
	output.WriteString("import { bindOperation, ")
	if hasPathOperations {
		output.WriteString("bindPathOperation, ")
	}
	if hasPagination {
		output.WriteString("createPaginator, ")
	}
	output.WriteString("createRequest } from \"./runtime.js\"\n\n")
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
	output.WriteString("  const _sdkgenRequest = createRequest(options)\n")
	operationsByID := make(map[string]ir.Operation, len(document.Operations))
	for _, operation := range document.Operations {
		operationsByID[operation.OperationID] = operation
	}
	for _, item := range manifest.Operations {
		if item.Visibility == "hidden" {
			continue
		}
		definition, err := operationDefinition(document, operationsByID[item.OperationID], item)
		if err != nil {
			return nil, err
		}
		hasInput := len(item.InputTypes) > 0
		fmt.Fprintf(&output, "  const %s = bindOperation(_sdkgenRequest, %s, %t)\n", javascriptOperationBindingName(item.OperationID), definition, hasInput)
		if item.Pagination != "" && len(item.PathParameterOrder) == 0 {
			fmt.Fprintf(&output, "  const %s = createPaginator(%s, %s)\n", javascriptPaginatorBindingName(item.OperationID), javascriptOperationBindingName(item.OperationID), quoteTS(item.Pagination))
		}
	}
	output.WriteString("\n")
	if err := emitJavaScriptResourceTreeValues(&output, tree); err != nil {
		return nil, err
	}
	output.WriteString("\n")
	output.WriteString("  return Object.freeze({\n    $operations: Object.freeze({\n")
	for _, item := range manifest.Operations {
		if item.Visibility == "hidden" {
			continue
		}
		fmt.Fprintf(&output, "      %s: %s,\n", quoteTS(item.OperationID), javascriptOperationBindingName(item.OperationID))
	}
	output.WriteString("    }),\n")
	for _, name := range sortedResourceChildNames(tree) {
		fmt.Fprintf(&output, "    %s: %s,\n", name, javascriptResourceBindingName(name))
	}
	output.WriteString("  })\n}\n")
	return output.Bytes(), nil
}

func emitJavaScriptErrors(document *ir.Document) ([]byte, error) {
	contracts, _, err := errorContracts(document)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if len(contracts) == 0 {
		output.WriteString("export {}\n")
		return output.Bytes(), nil
	}
	output.WriteString("import { isErrorCode } from \"./runtime.js\"\n\n")
	for _, contract := range contracts {
		fmt.Fprintf(&output, "export function is%s(error) { return isErrorCode(error, %s) }\n", contract.TypeName, quoteTS(contract.Code))
	}
	categories := make(map[string][]errorContract)
	for _, contract := range contracts {
		if contract.Category != "" {
			categories[contract.Category] = append(categories[contract.Category], contract)
		}
	}
	categoryNames := make([]string, 0, len(categories))
	for category := range categories {
		categoryNames = append(categoryNames, category)
	}
	sort.Strings(categoryNames)
	for _, category := range categoryNames {
		name, err := naming.Public(category)
		if err != nil {
			return nil, err
		}
		checks := make([]string, 0, len(categories[category]))
		for _, contract := range categories[category] {
			checks = append(checks, "is"+contract.TypeName+"(error)")
		}
		fmt.Fprintf(&output, "export function is%sError(error) { return %s }\n", name, strings.Join(checks, " || "))
	}
	return output.Bytes(), nil
}

func emitJavaScriptResourceTreeValues(output *bytes.Buffer, root *resourceNode) error {
	for _, name := range sortedResourceChildNames(root) {
		fmt.Fprintf(output, "  const %s = ", javascriptResourceBindingName(name))
		if err := emitJavaScriptResourceNodeValue(output, root.children[name], nil, "  "); err != nil {
			return err
		}
		output.WriteString("\n")
	}
	return nil
}

func emitJavaScriptResourceNodeValue(output *bytes.Buffer, node *resourceNode, bound map[string]string, indent string) error {
	if node.parameterChild == nil {
		return emitJavaScriptResourceNodeObject(output, node, bound, indent)
	}
	parameter := node.parameterChild.parameter
	nextBound := make(map[string]string, len(bound)+1)
	for name, value := range bound {
		nextBound[name] = value
	}
	nextBound[parameter.Name] = parameter.Property
	output.WriteString("Object.assign(\n")
	fmt.Fprintf(output, "%s  (%s) => (", indent, parameter.Property)
	if err := emitJavaScriptResourceNodeValue(output, node.parameterChild, nextBound, indent+"  "); err != nil {
		return err
	}
	output.WriteString("),\n")
	output.WriteString(indent + "  ")
	if err := emitJavaScriptResourceNodeObject(output, node, bound, indent+"  "); err != nil {
		return err
	}
	output.WriteString("\n" + indent + ")")
	return nil
}

func emitJavaScriptResourceNodeObject(output *bytes.Buffer, node *resourceNode, bound map[string]string, indent string) error {
	output.WriteString("{\n")
	memberIndent := indent + "  "
	for _, terminal := range sortedManifestOperationNames(node.operations) {
		fmt.Fprintf(output, "%s%s: ", memberIndent, terminal)
		if err := emitJavaScriptResourceOperationValue(output, node.operations[terminal], bound); err != nil {
			return err
		}
		output.WriteString(",\n")
	}
	if paginated, ok := paginatedResourceNodeOperation(node); ok {
		fmt.Fprintf(output, "%spaginate: %s,\n", memberIndent, javascriptPaginatorBindingName(paginated.OperationID))
	}
	for _, name := range sortedResourceChildNames(node) {
		fmt.Fprintf(output, "%s%s: ", memberIndent, name)
		if err := emitJavaScriptResourceNodeValue(output, node.children[name], bound, memberIndent); err != nil {
			return err
		}
		output.WriteString(",\n")
	}
	output.WriteString(indent + "}")
	return nil
}

func emitJavaScriptResourceOperationValue(output *bytes.Buffer, operation ManifestOperation, bound map[string]string) error {
	if len(operation.PathParameterOrder) == 0 {
		output.WriteString(javascriptOperationBindingName(operation.OperationID))
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
	fmt.Fprintf(output, "bindPathOperation(%s, { %s }, %t)", javascriptOperationBindingName(operation.OperationID), strings.Join(values, ", "), len(operation.InputTypes) > 1)
	return nil
}

func javascriptOperationBindingName(operationID string) string {
	return "_sdkgenOperation" + operationTypeName(operationID)
}

func javascriptPaginatorBindingName(operationID string) string {
	return "_sdkgenPaginator" + operationTypeName(operationID)
}

func javascriptResourceBindingName(property string) string {
	return "_sdkgenResource_" + property
}
