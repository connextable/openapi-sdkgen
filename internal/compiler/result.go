package sdkgen

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
	"github.com/connextable/openapi-sdkgen/internal/openapiwalk"
	"go.yaml.in/yaml/v4"
)

// Result is the complete expected outcome of compilation. Diagnostics describe
// author input; the separate error return is reserved for unexpected compiler
// failures.
type Result struct {
	Document      *ir.Document
	Diagnostics   []diagnostic.Diagnostic
	SkippedPhases []diagnostic.SkippedPhase
}

// CompileResult compiles in-memory OpenAPI input and returns structured
// diagnostics for expected author errors.
func CompileResult(data []byte) (Result, error) {
	collector := &diagnostic.Collector{}
	findings, err := reservedExtensionDiagnostics(data, "in-memory OpenAPI document")
	if err == nil {
		collector.Extend(findings)
	}
	if collector.HasErrors() {
		return reservedSourceScanResult(collector), nil
	}
	var decoded any
	if err := yaml.Unmarshal(data, &decoded); err == nil {
		collector.Extend(pathItemReferenceDiagnostics(decoded, "in-memory OpenAPI document", false))
		collector.Extend(unresolvedLocalReferenceDiagnostics(decoded, "in-memory OpenAPI document"))
	}
	if collector.HasErrors() {
		return referenceSourceScanResult(collector), nil
	}
	document, err := compile(data, false)
	return resultFromCompile(document, err, "in-memory OpenAPI document", collector), nil
}

// CompileFileResultWithOptions compiles a file with structured diagnostics.
func CompileFileResultWithOptions(path string, options CompileOptions) (Result, error) {
	if options.InputBase != "" || options.InputReader != nil {
		return Result{}, fmt.Errorf("internal compiler invocation: CompileFileResultWithOptions does not accept stdin input options")
	}
	return CompileInputResultWithOptions(path, options)
}

// CompileInputResultWithOptions compiles a path, URL, or stdin source and
// collects expected input/transport/compiler findings.
func CompileInputResultWithOptions(input string, options CompileOptions) (Result, error) {
	collector := &diagnostic.Collector{}
	options.diagnostics = collector
	source, err := loadInputSource(input, options)
	if err != nil {
		return resultFromCompile(nil, phaseError(diagnostic.PhaseInput, err), safeInputDisplay(input), collector), nil
	}
	var decoded any
	if err := yaml.Unmarshal(source.data, &decoded); err != nil {
		return resultFromCompile(nil, phaseError(diagnostic.PhaseDecode, fmt.Errorf("decode OpenAPI input: %w", err)), source.display, collector), nil
	}
	findings, err := reservedExtensionDiagnostics(source.data, source.display)
	if err != nil {
		return resultFromCompile(nil, phaseError(diagnostic.PhaseDecode, err), source.display, collector), nil
	}
	collector.Extend(findings)
	if err := scanLocalReferenceDocuments(source, collector); err != nil {
		return Result{}, fmt.Errorf("internal source registry failure: %w", err)
	}
	if collector.HasErrors() {
		return reservedSourceScanResult(collector), nil
	}
	collector.Extend(pathItemReferenceDiagnostics(decoded, source.display, true))
	collector.Extend(unresolvedLocalReferenceDiagnostics(decoded, source.display))
	if collector.HasErrors() {
		return referenceSourceScanResult(collector), nil
	}
	document, err := compileInput(source, false, options)
	return resultFromCompile(document, err, source.display, collector), nil
}

func pathItemReferenceDiagnostics(value any, source string, allowExternal bool) []diagnostic.Diagnostic {
	document, _ := value.(map[string]any)
	paths, _ := document["paths"].(map[string]any)
	names := make([]string, 0, len(paths))
	for path := range paths {
		names = append(names, path)
	}
	sort.Strings(names)
	var result []diagnostic.Diagnostic
	for _, path := range names {
		if openapiwalk.IsExtensionKey([]string{"paths"}, path) {
			continue
		}
		pathItem, _ := paths[path].(map[string]any)
		reference, _ := pathItem["$ref"].(string)
		if reference == "" {
			continue
		}
		local, err := ir.IsLocalPathItemReference(reference)
		if err == nil && !local && allowExternal {
			continue
		}
		if err == nil && local {
			_, err = ir.ResolvePathItem(document, pathItem)
			if err == nil {
				continue
			}
			if _, found := resolveLocalReference(document, reference); !found {
				continue
			}
		}
		if err == nil {
			err = fmt.Errorf("external path item reference %q is not supported for in-memory input", reference)
		}
		result = append(result, diagnostic.Diagnostic{
			Severity: diagnostic.SeverityError,
			Code:     "SDKGEN-E120",
			Phase:    diagnostic.PhaseReferences,
			Location: diagnostic.Location{Source: source, Pointer: sourceJSONPointer([]string{"paths", path, "$ref"})},
			Message:  "Unable to resolve the Path Item reference.",
			Cause:    sanitizeDiagnosticCause(err.Error()),
		})
	}
	return result
}

func reservedSourceScanResult(collector *diagnostic.Collector) Result {
	reason := "reserved extension keywords were found before reference bundling"
	return Result{
		Diagnostics: diagnostic.Sort(collector.Diagnostics()),
		SkippedPhases: []diagnostic.SkippedPhase{
			{Phase: diagnostic.PhaseReferences, Reason: reason},
			{Phase: diagnostic.PhaseNormalize, Reason: reason},
			{Phase: diagnostic.PhaseOpenAPI, Reason: reason},
			{Phase: diagnostic.PhaseIR, Reason: reason},
		},
	}
}

func referenceSourceScanResult(collector *diagnostic.Collector) Result {
	reason := "reference resolution reported errors"
	return Result{
		Diagnostics: diagnostic.Sort(collector.Diagnostics()),
		SkippedPhases: []diagnostic.SkippedPhase{
			{Phase: diagnostic.PhaseNormalize, Reason: reason},
			{Phase: diagnostic.PhaseOpenAPI, Reason: reason},
			{Phase: diagnostic.PhaseIR, Reason: reason},
		},
	}
}

func resultFromCompile(document *ir.Document, err error, source string, collector *diagnostic.Collector) Result {
	result := Result{Document: document}
	if err != nil {
		phase, code, message := classifyCompileError(err)
		collector.Add(diagnostic.Diagnostic{
			Severity: diagnostic.SeverityError,
			Code:     code,
			Phase:    phase,
			Location: diagnostic.Location{Source: safeInputDisplay(source), Pointer: "#"},
			Message:  message,
			Cause:    sanitizeDiagnosticCause(err.Error()),
		})
		result.Document = nil
		result.SkippedPhases = skippedAfter(phase)
	}
	result.Diagnostics = diagnostic.Sort(collector.Diagnostics())
	return result
}

func displayDiagnosticSources(values []diagnostic.Diagnostic) []diagnostic.Diagnostic {
	return diagnostic.SanitizeSources(values)
}

func classifyCompileError(err error) (diagnostic.Phase, string, string) {
	var phased *compilePhaseError
	phase := diagnostic.PhaseIR
	if errors.As(err, &phased) {
		phase = phased.phase
	}
	switch phase {
	case diagnostic.PhaseInput:
		return phase, "SDKGEN-E100", "Unable to load the OpenAPI input."
	case diagnostic.PhaseDecode:
		return diagnostic.PhaseDecode, "SDKGEN-E110", "Unable to decode the OpenAPI document."
	case diagnostic.PhaseReferences:
		return diagnostic.PhaseReferences, "SDKGEN-E120", "Unable to resolve OpenAPI references."
	case diagnostic.PhaseNormalize:
		return diagnostic.PhaseNormalize, "SDKGEN-E130", "Unable to normalize the OpenAPI document."
	case diagnostic.PhaseOpenAPI:
		return diagnostic.PhaseOpenAPI, "SDKGEN-E140", "The OpenAPI document is invalid."
	default:
		return diagnostic.PhaseIR, "SDKGEN-E150", "Unable to build the SDK intermediate representation."
	}
}

type compilePhaseError struct {
	phase diagnostic.Phase
	err   error
}

func (value *compilePhaseError) Error() string { return value.err.Error() }
func (value *compilePhaseError) Unwrap() error { return value.err }

func phaseError(phase diagnostic.Phase, err error) error {
	if err == nil {
		return nil
	}
	var existing *compilePhaseError
	if errors.As(err, &existing) {
		return err
	}
	return &compilePhaseError{phase: phase, err: err}
}

func skippedAfter(phase diagnostic.Phase) []diagnostic.SkippedPhase {
	order := []diagnostic.Phase{
		diagnostic.PhaseInput,
		diagnostic.PhaseDecode,
		diagnostic.PhaseReferences,
		diagnostic.PhaseNormalize,
		diagnostic.PhaseOpenAPI,
		diagnostic.PhaseIR,
	}
	var result []diagnostic.SkippedPhase
	found := false
	for _, current := range order {
		if found {
			result = append(result, diagnostic.SkippedPhase{
				Phase:  current,
				Reason: fmt.Sprintf("prerequisite phase %s did not produce a usable document", phase),
			})
		}
		if current == phase {
			found = true
		}
	}
	return result
}

var diagnosticURLPattern = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)

func sanitizeDiagnosticCause(message string) string {
	message = diagnosticURLPattern.ReplaceAllStringFunc(message, safeInputDisplay)
	message = strings.Join(strings.Fields(message), " ")
	const limit = 1000
	runes := []rune(message)
	if len(runes) > limit {
		message = string(runes[:limit]) + "…"
	}
	return message
}

func safeInputDisplay(value string) string {
	return diagnostic.SafeSourceDisplay(value)
}
