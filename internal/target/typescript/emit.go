package typescript

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"openapi-sdkgen/internal/compiler/ir"
	"openapi-sdkgen/internal/compiler/naming"
	"openapi-sdkgen/internal/diagnostic"
	"openapi-sdkgen/internal/generator"
)

//go:embed runtime/internal/*.ts
var runtimeTemplates embed.FS

//go:embed runtime/server/runtime.ts
var serverRuntimeTemplate []byte

type runtimeTemplateArtifact struct {
	source string
	path   string
}

var runtimeTemplateArtifacts = []runtimeTemplateArtifact{
	{source: "objects.ts", path: "internal/runtime/objects.ts"},
	{source: "identity.ts", path: "internal/runtime/identity.ts"},
	{source: "request.ts", path: "internal/runtime/request.ts"},
	{source: "errors.ts", path: "internal/runtime/errors.ts"},
	{source: "transport.ts", path: "internal/runtime/transport.ts"},
	{source: "security.ts", path: "internal/runtime/security.ts"},
	{source: "operation.ts", path: "internal/runtime/operation.ts"},
	{source: "configuration.ts", path: "internal/runtime/configuration.ts"},
	{source: "links.ts", path: "internal/runtime/links.ts"},
	{source: "callables.ts", path: "internal/runtime/callables.ts"},
	{source: "pagination.ts", path: "internal/runtime/pagination.ts"},
	{source: "codecs.ts", path: "internal/runtime/codecs.ts"},
	{source: "http.ts", path: "internal/runtime/http.ts"},
	{source: "constants.ts", path: "internal/runtime/constants.ts"},
}

func readRuntimeTemplate(name string) ([]byte, error) {
	source, err := runtimeTemplates.ReadFile("runtime/internal/" + name)
	if err != nil {
		return nil, fmt.Errorf("read TypeScript runtime template %q: %w", name, err)
	}
	return source, nil
}

func emitRuntimeTemplateArtifacts() ([]Artifact, error) {
	artifacts := make([]Artifact, 0, len(runtimeTemplateArtifacts))
	for _, template := range runtimeTemplateArtifacts {
		source, err := readRuntimeTemplate(template.source)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, Artifact{Path: template.path, Data: generatedSource(source)})
	}
	return artifacts, nil
}

// Artifact is a generated TypeScript source file.
type Artifact = generator.Artifact

// Generator emits TypeScript SDK source files from compiler IR.
type Generator struct{}

// Name returns the CLI target name.
func (Generator) Name() string { return "typescript" }

// SupportsAddon reports artifact sets available for TypeScript source output.
func (Generator) SupportsAddon(addon generator.Addon) bool {
	return addon == generator.AddonServer
}

type sourcePlan struct {
	document      *ir.Document
	includeServer bool
	manifest      *Manifest
	modules       *semanticModulePlan
	links         []generatedLink
	streams       []generatedStream
	webhooks      []webhookDefinition
	callbacks     []callbackDefinition
}

// Prepare validates author input for the TypeScript target.
func (Generator) Prepare(document *ir.Document, options generator.Options) (generator.Plan, []diagnostic.Diagnostic, error) {
	if document == nil {
		return generator.Plan{}, nil, fmt.Errorf("internal TypeScript target: IR document is nil")
	}
	plan, diagnostics, err := prepareSourcePlan(document, options.HasAddon(generator.AddonServer))
	if err != nil {
		return generator.Plan{}, diagnostics, err
	}
	return generator.NewPlan("typescript", plan), diagnostic.Sort(diagnostics), nil
}

// Emit emits a previously validated TypeScript plan.
func (Generator) Emit(plan generator.Plan) ([]generator.Artifact, error) {
	value, err := plan.Value("typescript")
	if err != nil {
		return nil, fmt.Errorf("internal TypeScript target: %w", err)
	}
	prepared, ok := value.(*sourcePlan)
	if !ok {
		return nil, fmt.Errorf("internal TypeScript target: unexpected plan type %T", value)
	}
	return emitSourcePlan(prepared)
}

// Generate is the compatibility convenience for direct target callers.
func (target Generator) Generate(document *ir.Document, options generator.Options) ([]generator.Artifact, error) {
	plan, diagnostics, err := target.Prepare(document, options)
	if err != nil {
		return nil, err
	}
	if diagnostic.HasErrors(diagnostics) {
		return nil, fmt.Errorf("%s", strings.TrimSpace(diagnostic.RenderHuman(diagnostics, nil)))
	}
	return target.Emit(plan)
}

type Manifest struct {
	Operations []ManifestOperation `json:"operations"`
}

type ManifestOperation struct {
	RouteKey           string   `json:"routeKey"`
	OperationID        string   `json:"operationID"`
	Summary            string   `json:"summary,omitempty"`
	Description        string   `json:"description,omitempty"`
	Method             string   `json:"method"`
	Path               string   `json:"path"`
	CallExpression     string   `json:"callExpression"`
	ResourceSegments   []string `json:"resourceSegments"`
	PathParameterOrder []string `json:"pathParameterOrder"`
	InputTypes         []string `json:"inputTypes"`
	OutputType         string   `json:"outputType"`
	ErrorType          string   `json:"errorType"`
	Envelope           string   `json:"envelope,omitempty"`
	Pagination         string   `json:"pagination,omitempty"`
	Auth               string   `json:"auth"`
	Visibility         string   `json:"visibility"`
	Deprecated         bool     `json:"deprecated"`
	outputExpression   typeExpression
	errorExpression    typeExpression
	paginationRequest  ir.PaginationRequestPlan
}

const generatedFileHeader = `// cspell:disable
/** @noprettier -- generated by openapi-sdkgen */
// <auto-generated />
// @generated by openapi-sdkgen
// Code generated by openapi-sdkgen. DO NOT EDIT.
// This file is generated. Do not edit, lint, or format it manually.
// @ts-nocheck
/* eslint-disable */
/* oxlint-disable */
/* biome-ignore-all lint: generated file */
/* deno-lint-ignore-file */
// dprint-ignore-file

`

// SourceArtifacts emits TypeScript source that a consumer compiles as part of
// its own application. It deliberately contains no package metadata or build
// configuration.
func SourceArtifacts(document *ir.Document) ([]Artifact, error) {
	return sourceArtifacts(document, false)
}

func sourceArtifacts(document *ir.Document, includeServer bool) ([]Artifact, error) {
	plan, diagnostics, err := prepareSourcePlan(document, includeServer)
	if err != nil {
		return nil, err
	}
	if diagnostic.HasErrors(diagnostics) {
		return nil, fmt.Errorf("%s", strings.TrimSpace(diagnostic.RenderHuman(diagnostics, nil)))
	}
	return emitSourcePlan(plan)
}

func prepareSourcePlan(document *ir.Document, includeServer bool) (*sourcePlan, []diagnostic.Diagnostic, error) {
	if document == nil {
		return nil, nil, fmt.Errorf("IR document is nil")
	}
	prepared, diagnostics, err := prepareKnownExtensions(document)
	if err != nil {
		return nil, nil, err
	}
	plan := &sourcePlan{document: prepared, includeServer: includeServer}
	targetDiagnostics := prepareTargetDiagnostics(plan)
	diagnostics = append(diagnostics, targetDiagnostics...)
	manifest, manifestErrors := buildManifestDiagnostics(prepared)
	for _, manifestErr := range manifestErrors {
		diagnostics = append(diagnostics, loweringPreparationDiagnostic(prepared, manifestErr))
	}
	helperManifest := manifest
	if len(manifestErrors) != 0 {
		helperManifest = visibilityManifest(prepared)
	} else {
		plan.manifest = &manifest
	}
	links, linkErrors := generatedLinksDiagnostics(prepared, helperManifest)
	for _, linksErr := range linkErrors {
		diagnostics = append(diagnostics, helperPreparationDiagnostic(prepared, "response links", "SDKGEN-E509", linksErr))
	}
	if len(linkErrors) == 0 {
		plan.links = links
	}
	streams, streamErrors := generatedStreamsDiagnostics(prepared, helperManifest)
	for _, streamsErr := range streamErrors {
		diagnostics = append(diagnostics, helperPreparationDiagnostic(prepared, "response streams", "SDKGEN-E510", streamsErr))
	}
	if len(streamErrors) == 0 {
		plan.streams = streams
	}
	if len(manifestErrors) == 0 && len(linkErrors) == 0 && len(streamErrors) == 0 {
		if plan.manifest != nil {
			if reconcileErr := reconcileResourceCapabilities(prepared, &manifest, links, streams); reconcileErr != nil {
				diagnostics = append(diagnostics, loweringPreparationDiagnostic(prepared, reconcileErr))
			}
			plan.manifest = &manifest
		}
	}
	if includeServer {
		webhooks, webhookErrors := collectWebhooksDiagnostics(prepared)
		for _, webhookErr := range webhookErrors {
			diagnostics = append(diagnostics, serverPreparationDiagnostic(prepared, "webhook contracts", webhookErr))
		}
		if len(webhookErrors) == 0 {
			plan.webhooks = webhooks
		}
		callbacks, callbackErrors := collectCallbacksDiagnostics(prepared)
		for _, callbackErr := range callbackErrors {
			diagnostics = append(diagnostics, serverPreparationDiagnostic(prepared, "callback contracts", callbackErr))
		}
		if len(callbackErrors) == 0 {
			plan.callbacks = callbacks
		}
	}
	if plan.manifest != nil && !diagnostic.HasErrors(diagnostics) {
		modules, moduleErr := buildSemanticModulePlan(prepared, *plan.manifest, plan.links, plan.streams)
		if moduleErr != nil {
			return nil, diagnostic.Sort(diagnostics), fmt.Errorf("build TypeScript semantic module plan: %w", moduleErr)
		}
		plan.modules = modules
	}
	return plan, diagnostic.Sort(diagnostics), nil
}

func reconcileResourceCapabilities(document *ir.Document, manifest *Manifest, links []generatedLink, streams []generatedStream) error {
	tree, err := buildResourceTree(document, *manifest, resourceCapabilityMembers(links, streams))
	if err != nil {
		return err
	}
	reachable := make(map[string]bool)
	resourceOperationIDs(tree, reachable)
	for index := range manifest.Operations {
		item := &manifest.Operations[index]
		if item.Visibility != "public" || reachable[item.RouteKey] {
			continue
		}
		operation := findOperation(document, item.RouteKey)
		item.CallExpression = exactOperationCall(document, operation, item.InputTypes)
		item.ResourceSegments = nil
	}
	return nil
}

func emitSourcePlan(plan *sourcePlan) ([]Artifact, error) {
	document := plan.document
	includeServer := plan.includeServer
	if plan.manifest == nil {
		return nil, fmt.Errorf("internal TypeScript target: prepared plan has no manifest")
	}
	manifest := *plan.manifest
	schemaArtifacts, typesSource, err := emitSchemaArtifacts(document, plan.modules)
	if err != nil {
		return nil, err
	}
	operationArtifacts, err := emitOperationArtifacts(document, manifest, plan.modules, plan.links, plan.streams)
	if err != nil {
		return nil, err
	}
	routeArtifacts, err := emitRouteArtifacts(manifest, plan.modules)
	if err != nil {
		return nil, err
	}
	registrySource, err := emitClientRegistry(document, manifest, plan.modules, plan.links, plan.streams)
	if err != nil {
		return nil, err
	}
	constantsSource, err := readRuntimeTemplate("constants.ts")
	if err != nil {
		return nil, err
	}
	enumsSource, err := emitEnums(document)
	if err != nil {
		return nil, err
	}
	errorsSource, err := emitErrors(document)
	if err != nil {
		return nil, err
	}
	metadataSource, err := emitMetadata(document, true)
	if err != nil {
		return nil, err
	}
	clientSource, err := emitClient(document, manifest, plan.links, plan.streams)
	if err != nil {
		return nil, err
	}
	if err := validateSourceExportSymbols(map[string][]byte{
		"client":    clientSource,
		"constants": constantsSource,
		"enums":     enumsSource,
		"errors":    errorsSource,
		"metadata":  metadataSource,
		"types":     typesSource,
	}); err != nil {
		return nil, err
	}
	indexSource := generatedIndexSource(enumsSource)
	runtimeArtifacts, err := emitRuntimeTemplateArtifacts()
	if err != nil {
		return nil, err
	}

	artifacts := []Artifact{
		{Path: "index.ts", Data: generatedSource([]byte("export * from \"./internal/index.js\"\n"))},
		{Path: "internal/client.ts", Data: generatedSource(clientSource)},
		{Path: "internal/enums.ts", Data: generatedSource(enumsSource)},
		{Path: "internal/errors.ts", Data: generatedSource(errorsSource)},
		{Path: "internal/index.ts", Data: generatedSource(indexSource)},
		{Path: "enums.ts", Data: generatedSource([]byte("export * from \"./internal/enums.js\"\n"))},
		{Path: "metadata.ts", Data: generatedSource(metadataSource)},
	}
	artifacts = append(artifacts, runtimeArtifacts...)
	artifacts = append(artifacts, schemaArtifacts...)
	artifacts = append(artifacts, operationArtifacts...)
	artifacts = append(artifacts, routeArtifacts...)
	artifacts = append(artifacts, Artifact{Path: plan.modules.fixed["client-registry"], Data: generatedSource(registrySource)})
	if includeServer {
		serverArtifacts, err := emitPreparedServerArtifacts(document, plan.webhooks, plan.callbacks)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, serverArtifacts...)
	}
	if err := validateGeneratedArtifacts(artifacts); err != nil {
		return nil, err
	}
	sort.Slice(artifacts, func(i, j int) bool {
		return artifactEmissionOrder(artifacts[i].Path) < artifactEmissionOrder(artifacts[j].Path)
	})
	return artifacts, nil
}

func artifactEmissionOrder(path string) string {
	if strings.HasPrefix(path, "internal/") {
		return "0/" + path
	}
	switch path {
	case "index.ts":
		return "1/" + path
	case "enums.ts":
		return "2/" + path
	case "metadata.ts":
		return "3/" + path
	default:
		return "4/" + path
	}
}

func generatedIndexSource(enumsSource []byte) []byte {
	var output strings.Builder
	output.WriteString("export type * from \"./schemas/index.js\"\n")
	output.WriteString("export type { BothPaginationInput, CursorPaginationInput, OffsetPaginationInput } from \"./runtime/pagination.js\"\n")
	for _, module := range []string{"enums", "errors", "client"} {
		fmt.Fprintf(&output, "export type * from %s\n", quoteTS("./"+module+".js"))
	}
	output.WriteString("export { SortDirection } from \"./runtime/constants.js\"\n")
	if exportedSymbols(string(enumsSource))["isEnumValue"] {
		output.WriteString("export { Enums, isEnumValue } from \"./enums.js\"\n")
	} else {
		output.WriteString("export { Enums } from \"./enums.js\"\n")
	}
	output.WriteString("export { isErrorCategory } from \"./errors.js\"\n")
	output.WriteString("export { createClient } from \"./client.js\"\n")
	output.WriteString("export { APIError, TransportErrorCode, getErrorCode, getRequestID, isAPIError, isErrorCode } from \"./runtime/errors.js\"\n")
	return []byte(output.String())
}

func generatedSource(source []byte) []byte {
	return append([]byte(generatedFileHeader), source...)
}

func validateSourceExportSymbols(modules map[string][]byte) error {
	owners := make(map[string]string)
	moduleNames := make([]string, 0, len(modules))
	for module := range modules {
		moduleNames = append(moduleNames, module)
	}
	sort.Strings(moduleNames)
	for _, module := range moduleNames {
		exports := exportedSymbols(string(modules[module]))
		for symbol := range exports {
			if previous, exists := owners[symbol]; exists && previous != module {
				return fmt.Errorf("generated source export %q is declared by both %s and %s modules", symbol, previous, module)
			}
			owners[symbol] = module
		}
	}
	return nil
}

func exportedSymbols(source string) map[string]bool {
	result := make(map[string]bool)
	inExportBlock := false
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if inExportBlock {
			if trimmed == "}" {
				inExportBlock = false
				continue
			}
			name := strings.TrimSuffix(trimmed, ",")
			if before, after, found := strings.Cut(name, " as "); found {
				_ = before
				name = after
			}
			if name != "" {
				result[name] = true
			}
			continue
		}
		if !strings.HasPrefix(trimmed, "export ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "export "))
		if rest == "{" || rest == "type {" {
			inExportBlock = true
			continue
		}
		if strings.HasPrefix(rest, "{") || strings.HasPrefix(rest, "type {") {
			open := strings.Index(rest, "{")
			close := strings.Index(rest, "}")
			if open >= 0 && close > open {
				for _, name := range strings.Split(rest[open+1:close], ",") {
					name = strings.TrimSpace(name)
					if before, after, found := strings.Cut(name, " as "); found {
						_ = before
						name = after
					}
					if name != "" {
						result[name] = true
					}
				}
			}
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "type", "interface", "const", "function", "class":
			result[strings.Split(fields[1], "<")[0]] = true
		}
	}
	return result
}

func buildManifest(document *ir.Document) (Manifest, error) {
	manifest, failures := buildManifestDiagnostics(document)
	if len(failures) != 0 {
		return Manifest{}, failures[0]
	}
	return manifest, nil
}

func buildManifestDiagnostics(document *ir.Document) (Manifest, []error) {
	manifest := Manifest{
		Operations: make([]ManifestOperation, 0, len(document.Operations)),
	}
	_, errorsBySchema, failures := errorContractsDiagnostics(document)
	for _, operation := range document.Operations {
		operationFailed := false
		outputExpression, err := operationOutputTypeExpression(document, operation)
		if err != nil {
			failures = append(failures, fmt.Errorf("operation %s output: %w", operationLabel(operation), err))
			operationFailed = true
		}
		inputTypes, inputErr := operationInputTypes(document, operation)
		if inputErr != nil {
			failures = append(failures, fmt.Errorf("operation %s input: %w", operationLabel(operation), inputErr))
			operationFailed = true
		}
		errorExpression, err := operationErrorTypeExpression(document, operation, errorsBySchema)
		if err != nil {
			failures = append(failures, fmt.Errorf("operation %s error: %w", operationLabel(operation), err))
			operationFailed = true
		}
		callExpression := ""
		var segments []string
		if inputErr == nil {
			var callErr error
			callExpression, segments, callErr = operationCall(document, operation, inputTypes)
			if callErr != nil {
				failures = append(failures, fmt.Errorf("operation %s call expression: %w", operationLabel(operation), callErr))
				operationFailed = true
			}
		}
		if operationFailed {
			continue
		}
		visibility := operation.Visibility
		if visibility == "" {
			visibility = "public"
		}
		manifest.Operations = append(manifest.Operations, ManifestOperation{
			RouteKey:           operationRouteKey(operation),
			OperationID:        operation.OperationID,
			Summary:            operation.Summary,
			Description:        operation.Description,
			Method:             operation.Method,
			Path:               operation.Path,
			CallExpression:     callExpression,
			ResourceSegments:   segments,
			PathParameterOrder: append([]string{}, operation.PathParameterOrder...),
			InputTypes:         append([]string{}, inputTypes...),
			OutputType:         outputExpression.render(typeRenderLocal),
			ErrorType:          errorExpression.render(typeRenderLocal),
			Envelope:           operation.Envelope,
			Pagination:         operation.Pagination,
			Auth:               operationAuth(operation),
			Visibility:         visibility,
			Deprecated:         boolValue(operation.Raw, "deprecated"),
			outputExpression:   outputExpression,
			errorExpression:    errorExpression,
			paginationRequest: func() ir.PaginationRequestPlan {
				if operation.PaginationPlan == nil {
					return ir.PaginationRequestPlan{}
				}
				return operation.PaginationPlan.Request
			}(),
		})
	}
	if len(failures) != 0 {
		return manifest, failures
	}
	tree, err := buildResourceTree(document, manifest)
	if err != nil {
		return Manifest{}, append(failures, err)
	}
	reachable := make(map[string]bool)
	resourceOperationIDs(tree, reachable)
	for index := range manifest.Operations {
		item := &manifest.Operations[index]
		if item.Visibility == "public" && !reachable[item.RouteKey] {
			operation := findOperation(document, item.RouteKey)
			item.CallExpression = exactOperationCall(document, operation, item.InputTypes)
			item.ResourceSegments = nil
		}
	}
	return manifest, failures
}

func visibilityManifest(document *ir.Document) Manifest {
	result := Manifest{Operations: make([]ManifestOperation, 0, len(document.Operations))}
	for _, operation := range document.Operations {
		visibility := operation.Visibility
		if visibility == "" {
			visibility = "public"
		}
		result.Operations = append(result.Operations, ManifestOperation{
			RouteKey:    operationRouteKey(operation),
			OperationID: operation.OperationID,
			Method:      operation.Method,
			Path:        operation.Path,
			Visibility:  visibility,
		})
	}
	return result
}

func (operation ManifestOperation) renderOutput(scope typeRenderScope) string {
	if operation.outputExpression.local == "" && operation.outputExpression.contract == "" {
		return operation.OutputType
	}
	return operation.outputExpression.render(scope)
}

func (operation ManifestOperation) renderError(scope typeRenderScope) string {
	if operation.errorExpression.local == "" && operation.errorExpression.contract == "" {
		return operation.ErrorType
	}
	return operation.errorExpression.render(scope)
}

func operationCall(document *ir.Document, operation ir.Operation, inputTypes []string) (string, []string, error) {
	if hasDuplicateStrings(operation.PathParameterOrder) {
		return exactOperationCall(document, operation, inputTypes), nil, nil
	}
	pathBindings, err := operationPathBindings(document, operation)
	if err != nil {
		return "", nil, err
	}
	parts := resourcePathParts(operation.Path)
	segments := make([]string, 0, len(parts))
	chain := "api"
	for _, part := range parts {
		name, parameterPart, supported := resourcePathPart(part)
		if !supported {
			return exactOperationCall(document, operation, inputTypes), nil, nil
		}
		if parameterPart {
			binding := pathBindings[name]
			if binding == "" {
				return exactOperationCall(document, operation, inputTypes), nil, nil
			}
			chain += "(" + binding + ")"
			continue
		}
		property, err := naming.Property(part)
		if err != nil {
			return exactOperationCall(document, operation, inputTypes), nil, nil
		}
		segments = append(segments, property)
		chain += "." + property
	}
	terminal, err := resourceTerminalName(operation, parts)
	if err != nil {
		return exactOperationCall(document, operation, inputTypes), nil, nil
	}
	if operation.Visibility == "internal" {
		return exactOperationCall(document, operation, inputTypes), segments, nil
	}
	return chain + "." + terminal + callInput(operation, inputTypes, len(operation.PathParameterOrder) > 0, operation.PathParameterOrder, pathBindings), segments, nil
}

func exactOperationCall(document *ir.Document, operation ir.Operation, inputTypes []string) string {
	pathBindings, _ := operationPathBindings(document, operation)
	if operation.OperationID != "" {
		return "api.$operations[" + quoteTS(operation.OperationID) + "]" + callInput(operation, inputTypes, false, operation.PathParameterOrder, pathBindings)
	}
	return "api.$routes[" + quoteTS(operationRouteKey(operation)) + "]" + callInput(operation, inputTypes, false, operation.PathParameterOrder, pathBindings)
}

func resourcePathParts(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func resourcePathPart(part string) (string, bool, bool) {
	if !strings.ContainsAny(part, "{}") {
		return part, false, true
	}
	if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") &&
		strings.Count(part, "{") == 1 && strings.Count(part, "}") == 1 && len(part) > 2 {
		return strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}"), true, true
	}
	return "", false, false
}

func hasDuplicateStrings(values []string) bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
}

// resourceTerminalName keeps literal path segments as namespaces. For example,
// POST /auth/login becomes api.auth.login.post(), rather than api.auth.login().
func resourceTerminalName(operation ir.Operation, parts []string) (string, error) {
	terminal, err := terminalName(operation, parts)
	if err != nil {
		return "", err
	}
	if len(parts) == 0 {
		return terminal, nil
	}
	last := parts[len(parts)-1]
	if strings.HasPrefix(last, "{") {
		return terminal, nil
	}
	property, err := naming.Property(last)
	if err != nil {
		return "", err
	}
	if terminal != property {
		return terminal, nil
	}
	return methodTerminal(operation.Method)
}

func methodTerminal(method string) (string, error) {
	switch method {
	case "GET":
		return "get", nil
	case "POST":
		return "post", nil
	case "PUT":
		return "replace", nil
	case "PATCH":
		return "patch", nil
	case "DELETE":
		return "delete", nil
	case "QUERY":
		return "query", nil
	default:
		return naming.Property(strings.ToLower(method))
	}
}

func operationPathBindings(document *ir.Document, operation ir.Operation) (map[string]string, error) {
	parameters, err := operationParameters(document, operation)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, parameter := range parameters {
		if parameter.Location == "path" {
			result[parameter.Name] = parameter.Binding
		}
	}
	return result, nil
}

func callInput(operation ir.Operation, inputTypes []string, pathBound bool, pathParameters []string, pathBindings map[string]string) string {
	var fields []string
	for _, inputType := range inputTypes {
		switch {
		case strings.HasSuffix(inputType, "PathInput"):
			if pathBound {
				continue
			}
			values := make([]string, 0, len(pathParameters))
			seen := make(map[string]bool, len(pathParameters))
			for _, parameter := range pathParameters {
				if seen[parameter] {
					continue
				}
				seen[parameter] = true
				binding := pathBindings[parameter]
				if binding == "" {
					binding = stablePrivateIdentifier("example-path-parameter", parameter)
				}
				values = append(values, quoteTS(parameter)+": "+binding)
			}
			fields = append(fields, "path: { "+strings.Join(values, ", ")+" }")
		case strings.HasSuffix(inputType, "QueryInput"):
			fields = append(fields, "query")
		case strings.HasSuffix(inputType, "QuerystringInput"):
			fields = append(fields, "querystring")
		case strings.HasSuffix(inputType, "HeaderInput"):
			fields = append(fields, "headerParams")
		case strings.HasSuffix(inputType, "CookieInput"):
			fields = append(fields, "cookieParams")
		case strings.HasSuffix(inputType, "BodyInput"):
			fields = append(fields, "body")
		}
	}
	var arguments []string
	if len(fields) > 0 {
		arguments = append(arguments, "{ "+strings.Join(fields, ", ")+" }")
	}
	return "(" + strings.Join(arguments, ", ") + ")"
}

func terminalName(operation ir.Operation, parts []string) (string, error) {
	last := ""
	if len(parts) > 0 {
		last = parts[len(parts)-1]
	}
	if isTerminalAction(operation, parts, len(parts)-1) {
		return naming.Property(last)
	}
	lowerID := strings.ToLower(operation.OperationID)
	switch operation.Method {
	case "GET":
		if strings.HasPrefix(lowerID, "list") || strings.HasPrefix(lowerID, "search") {
			return "list", nil
		}
		return "get", nil
	case "POST":
		if strings.HasPrefix(lowerID, "create") {
			return "create", nil
		}
		return "post", nil
	case "PUT":
		return "replace", nil
	case "PATCH":
		return "patch", nil
	case "DELETE":
		return "delete", nil
	case "QUERY":
		return "query", nil
	default:
		return naming.Property(strings.ToLower(operation.Method))
	}
}

func isTerminalAction(operation ir.Operation, parts []string, index int) bool {
	if index < 0 || index != len(parts)-1 || operation.Method == "GET" || len(parts) < 2 {
		return false
	}
	last := parts[index]
	if last == "" || strings.HasPrefix(last, "{") {
		return false
	}
	return strings.Contains(operation.OperationID, strings.ToUpper(last[:1])+last[1:]) || !strings.HasPrefix(strings.ToLower(operation.OperationID), "create")
}

func operationTypeName(operationID string) string {
	return stablePrivateIdentifier("operation-type", operationID)
}

func operationAuth(operation ir.Operation) string {
	security, exists := operation.Raw["security"]
	if !exists {
		return "inherited"
	}
	items, _ := security.([]any)
	if len(items) == 0 {
		return "public"
	}
	return "required"
}

func boolValue(values map[string]any, key string) bool {
	value, _ := values[key].(bool)
	return value
}
