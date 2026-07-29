// Package diagnostic defines target-neutral, source-aware generation
// diagnostics.
package diagnostic

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Severity classifies whether generation may continue.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Phase identifies the pipeline phase that produced a diagnostic.
type Phase string

const (
	PhaseInput      Phase = "input"
	PhaseDecode     Phase = "decode"
	PhaseReferences Phase = "references"
	PhaseNormalize  Phase = "normalize"
	PhaseOpenAPI    Phase = "openapi"
	PhaseIR         Phase = "ir"
	PhaseTarget     Phase = "target"
	PhaseEmit       Phase = "emit"
	PhasePublish    Phase = "publish"
)

// Location identifies one source document and an RFC 6901 JSON Pointer.
type Location struct {
	Source  string
	Pointer string
}

// Diagnostic is an actionable author-facing compiler or target finding.
// Cause contains only an explicitly sanitized summary; callers must never put
// credentials, URLs with secrets, or arbitrary transport errors in it.
type Diagnostic struct {
	Severity  Severity
	Code      string
	Phase     Phase
	Location  Location
	Related   []Location
	Target    string
	Route     string
	Operation string
	Message   string
	Hint      string
	Cause     string
}

// SkippedPhase explains a prerequisite-bound phase that could not run.
type SkippedPhase struct {
	Phase  Phase
	Reason string
}

// Counts summarizes a diagnostic set.
type Counts struct {
	Errors   int
	Warnings int
}

// Collector accumulates diagnostics without rendering them.
type Collector struct {
	diagnostics []Diagnostic
}

// Add appends a diagnostic after normalizing its related locations.
func (collector *Collector) Add(value Diagnostic) {
	value.Related = normalizeLocations(value.Related)
	collector.diagnostics = append(collector.diagnostics, value)
}

// Extend appends a diagnostic set.
func (collector *Collector) Extend(values []Diagnostic) {
	for _, value := range values {
		collector.Add(value)
	}
}

// Diagnostics returns a deterministic copy.
func (collector *Collector) Diagnostics() []Diagnostic {
	return Sort(collector.diagnostics)
}

// HasErrors reports whether any collected diagnostic blocks emission.
func (collector *Collector) HasErrors() bool {
	for _, value := range collector.diagnostics {
		if value.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Count returns complete severity totals.
func Count(values []Diagnostic) Counts {
	var result Counts
	for _, value := range values {
		switch value.Severity {
		case SeverityError:
			result.Errors++
		case SeverityWarning:
			result.Warnings++
		}
	}
	return result
}

// HasErrors reports whether a diagnostic set blocks emission.
func HasErrors(values []Diagnostic) bool {
	return Count(values).Errors != 0
}

// Sort returns a deterministic deep copy of values.
func Sort(values []Diagnostic) []Diagnostic {
	result := append([]Diagnostic(nil), values...)
	for index := range result {
		result[index].Related = normalizeLocations(result[index].Related)
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if rank := phaseRank(left.Phase) - phaseRank(right.Phase); rank != 0 {
			return rank < 0
		}
		if left.Location.Source != right.Location.Source {
			return left.Location.Source < right.Location.Source
		}
		if left.Location.Pointer != right.Location.Pointer {
			return left.Location.Pointer < right.Location.Pointer
		}
		if left.Severity != right.Severity {
			return severityRank(left.Severity) < severityRank(right.Severity)
		}
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.Target != right.Target {
			return left.Target < right.Target
		}
		if left.Route != right.Route {
			return left.Route < right.Route
		}
		if left.Operation != right.Operation {
			return left.Operation < right.Operation
		}
		if left.Message != right.Message {
			return left.Message < right.Message
		}
		if left.Hint != right.Hint {
			return left.Hint < right.Hint
		}
		return left.Cause < right.Cause
	})
	return result
}

// RenderHuman renders one complete deterministic report. The severity counts
// are deliberately first so long reports remain scannable.
func RenderHuman(values []Diagnostic, skipped []SkippedPhase) string {
	values = Sort(values)
	counts := Count(values)
	var output strings.Builder
	fmt.Fprintf(&output, "OpenAPI SDK generation: %d error(s), %d warning(s)\n", counts.Errors, counts.Warnings)
	var currentPhase Phase
	currentSource := "\x00"
	for _, value := range values {
		if value.Phase != currentPhase {
			currentPhase = value.Phase
			currentSource = "\x00"
			fmt.Fprintf(&output, "\nPhase: %s\n", value.Phase)
		}
		if value.Location.Source != currentSource {
			currentSource = value.Location.Source
			source := currentSource
			if source == "" {
				source = "(no source)"
			}
			fmt.Fprintf(&output, "Source: %s\n", source)
		}
		fmt.Fprintf(&output, "\n%s [%s] %s", value.Severity, value.Code, value.Message)
		if value.Location.Source != "" || value.Location.Pointer != "" {
			fmt.Fprintf(&output, "\n  at: %s%s", value.Location.Source, value.Location.Pointer)
		}
		if value.Target != "" {
			fmt.Fprintf(&output, "\n  target: %s", value.Target)
		}
		if value.Route != "" {
			fmt.Fprintf(&output, "\n  route: %s", value.Route)
		}
		if value.Operation != "" {
			fmt.Fprintf(&output, "\n  operation: %s", value.Operation)
		}
		for _, related := range value.Related {
			fmt.Fprintf(&output, "\n  related: %s%s", related.Source, related.Pointer)
		}
		if value.Hint != "" {
			fmt.Fprintf(&output, "\n  hint: %s", value.Hint)
		}
		if value.Cause != "" {
			fmt.Fprintf(&output, "\n  cause: %s", value.Cause)
		}
		output.WriteByte('\n')
	}
	skipped = normalizeSkipped(skipped)
	if len(skipped) != 0 {
		output.WriteString("\nSkipped phases:\n")
		for _, item := range skipped {
			fmt.Fprintf(&output, "- %s: %s\n", item.Phase, item.Reason)
		}
	}
	return output.String()
}

func normalizeLocations(values []Location) []Location {
	result := append([]Location(nil), values...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Source == result[j].Source {
			return result[i].Pointer < result[j].Pointer
		}
		return result[i].Source < result[j].Source
	})
	write := 0
	for _, value := range result {
		if write != 0 && result[write-1] == value {
			continue
		}
		result[write] = value
		write++
	}
	return result[:write]
}

func normalizeSkipped(values []SkippedPhase) []SkippedPhase {
	result := append([]SkippedPhase(nil), values...)
	sort.Slice(result, func(i, j int) bool {
		if rank := phaseRank(result[i].Phase) - phaseRank(result[j].Phase); rank != 0 {
			return rank < 0
		}
		return result[i].Reason < result[j].Reason
	})
	return result
}

func severityRank(value Severity) int {
	if value == SeverityError {
		return 0
	}
	return 1
}

func phaseRank(value Phase) int {
	for index, phase := range []Phase{
		PhaseInput,
		PhaseDecode,
		PhaseReferences,
		PhaseNormalize,
		PhaseOpenAPI,
		PhaseIR,
		PhaseTarget,
		PhaseEmit,
		PhasePublish,
	} {
		if value == phase {
			return index
		}
	}
	return 100
}

// SourceRegistry maps internal source identities to safe human displays. Its
// collision ordinals are assigned from canonical source identity order, never
// caller discovery order.
type SourceRegistry struct {
	display map[string]string
}

// NewSourceRegistry constructs a stable display registry.
func NewSourceRegistry(sources []string) *SourceRegistry {
	canonical := append([]string(nil), sources...)
	sort.Strings(canonical)
	canonical = deduplicateStrings(canonical)
	groups := make(map[string][]string)
	for _, source := range canonical {
		safe := safeSourceDisplay(source)
		groups[safe] = append(groups[safe], source)
	}
	display := make(map[string]string, len(canonical))
	for safe, identities := range groups {
		for index, identity := range identities {
			if len(identities) == 1 {
				display[identity] = safe
				continue
			}
			display[identity] = fmt.Sprintf("%s [source %d]", safe, index+1)
		}
	}
	return &SourceRegistry{display: display}
}

// Display returns the safe display for an internal source identity.
func (registry *SourceRegistry) Display(source string) string {
	if value, exists := registry.display[source]; exists {
		return value
	}
	return safeSourceDisplay(source)
}

func safeSourceDisplay(source string) string {
	value, err := url.Parse(source)
	if err != nil || value.Scheme == "" || value.Host == "" {
		return source
	}
	sanitized := *value
	sanitized.User = nil
	sanitized.RawQuery = ""
	sanitized.ForceQuery = false
	sanitized.Fragment = ""
	return sanitized.String()
}

func deduplicateStrings(values []string) []string {
	write := 0
	for _, value := range values {
		if write != 0 && values[write-1] == value {
			continue
		}
		values[write] = value
		write++
	}
	return values[:write]
}
