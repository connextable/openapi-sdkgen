package sdkgen

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
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
		return resultFromCompile(nil, err, safeInputDisplay(input), collector), nil
	}
	var decoded any
	if err := yaml.Unmarshal(source.data, &decoded); err != nil {
		return resultFromCompile(nil, fmt.Errorf("decode OpenAPI input: %w", err), source.display, collector), nil
	}
	findings, err := reservedExtensionDiagnostics(source.data, source.display)
	if err != nil {
		return resultFromCompile(nil, err, source.display, collector), nil
	}
	collector.Extend(findings)
	if err := scanLocalReferenceDocuments(source, collector); err != nil {
		return Result{}, fmt.Errorf("internal source registry failure: %w", err)
	}
	if collector.HasErrors() {
		return reservedSourceScanResult(collector), nil
	}
	collector.Extend(unresolvedLocalReferenceDiagnostics(decoded, source.display))
	if collector.HasErrors() {
		return referenceSourceScanResult(collector), nil
	}
	document, err := compileInput(source, false, options)
	return resultFromCompile(document, err, source.display, collector), nil
}

func reservedSourceScanResult(collector *diagnostic.Collector) Result {
	reason := "reserved extension keywords were found before reference bundling"
	return Result{
		Diagnostics: displayDiagnosticSources(collector.Diagnostics()),
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
		Diagnostics: displayDiagnosticSources(collector.Diagnostics()),
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
	result.Diagnostics = displayDiagnosticSources(collector.Diagnostics())
	return result
}

func displayDiagnosticSources(values []diagnostic.Diagnostic) []diagnostic.Diagnostic {
	sources := make([]string, 0, len(values))
	for _, value := range values {
		if value.Location.Source != "" {
			sources = append(sources, value.Location.Source)
		}
		for _, related := range value.Related {
			if related.Source != "" {
				sources = append(sources, related.Source)
			}
		}
	}
	registry := diagnostic.NewSourceRegistry(sources)
	displayed := append([]diagnostic.Diagnostic(nil), values...)
	for index := range displayed {
		displayed[index].Location.Source = registry.Display(displayed[index].Location.Source)
		displayed[index].Related = append([]diagnostic.Location(nil), displayed[index].Related...)
		for relatedIndex := range displayed[index].Related {
			displayed[index].Related[relatedIndex].Source = registry.Display(displayed[index].Related[relatedIndex].Source)
		}
	}
	return diagnostic.Sort(displayed)
}

func classifyCompileError(err error) (diagnostic.Phase, string, string) {
	message := err.Error()
	switch {
	case strings.Contains(message, "decode"):
		return diagnostic.PhaseDecode, "SDKGEN-E110", "Unable to decode the OpenAPI document."
	case strings.Contains(message, "read OpenAPI"), strings.Contains(message, "input"),
		strings.Contains(message, "HTTP"), strings.Contains(message, "TLS"),
		strings.Contains(message, "offline"), strings.Contains(message, "lock"):
		return diagnostic.PhaseInput, "SDKGEN-E100", "Unable to load the OpenAPI input."
	case strings.Contains(message, "reference"), strings.Contains(message, "$ref"),
		strings.Contains(message, "bundle"):
		return diagnostic.PhaseReferences, "SDKGEN-E120", "Unable to resolve OpenAPI references."
	case strings.Contains(message, "normalize"):
		return diagnostic.PhaseNormalize, "SDKGEN-E130", "Unable to normalize the OpenAPI document."
	case strings.Contains(message, "OpenAPI"), strings.Contains(message, "openapi"),
		strings.Contains(message, "paths"), strings.Contains(message, "Path Item"):
		return diagnostic.PhaseOpenAPI, "SDKGEN-E140", "The OpenAPI document is invalid."
	default:
		return diagnostic.PhaseIR, "SDKGEN-E150", "Unable to build the SDK intermediate representation."
	}
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

var diagnosticURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

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
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return value
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}
