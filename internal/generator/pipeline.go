package generator

import (
	"fmt"

	compiler "openapi-sdkgen/internal/compiler"
	"openapi-sdkgen/internal/diagnostic"
)

// Preparation is the complete compiler and target preflight outcome.
type Preparation struct {
	Plan          Plan
	Diagnostics   []diagnostic.Diagnostic
	SkippedPhases []diagnostic.SkippedPhase
}

// PrepareCompilation owns the compiler result, preserves its diagnostics and
// skipped phases, and invokes the target only when a safe document exists.
func PrepareCompilation(target Target, compiled compiler.Result, options Options) (Preparation, error) {
	if target == nil {
		return Preparation{}, fmt.Errorf("internal generation pipeline: target is nil")
	}
	result := Preparation{
		Diagnostics:   append([]diagnostic.Diagnostic(nil), compiled.Diagnostics...),
		SkippedPhases: append([]diagnostic.SkippedPhase(nil), compiled.SkippedPhases...),
	}
	if compiled.Document == nil || diagnostic.HasErrors(compiled.Diagnostics) {
		if compiled.Document == nil && !diagnostic.HasErrors(compiled.Diagnostics) {
			return Preparation{}, fmt.Errorf("internal generation pipeline: compiler returned neither a document nor an error diagnostic")
		}
		result.SkippedPhases = append(result.SkippedPhases,
			diagnostic.SkippedPhase{Phase: diagnostic.PhaseTarget, Reason: "compiler preflight did not produce a safe document"},
			diagnostic.SkippedPhase{Phase: diagnostic.PhaseEmit, Reason: "compiler preflight did not produce a safe document"},
			diagnostic.SkippedPhase{Phase: diagnostic.PhasePublish, Reason: "compiler preflight did not produce a safe document"},
		)
		return result, nil
	}
	plan, targetDiagnostics, err := target.Prepare(compiled.Document, options)
	result.Diagnostics = diagnostic.Sort(append(result.Diagnostics, targetDiagnostics...))
	if err != nil {
		return result, err
	}
	result.Plan = plan
	if diagnostic.HasErrors(targetDiagnostics) {
		result.SkippedPhases = append(result.SkippedPhases,
			diagnostic.SkippedPhase{Phase: diagnostic.PhaseEmit, Reason: "target preflight reported errors"},
			diagnostic.SkippedPhase{Phase: diagnostic.PhasePublish, Reason: "target preflight reported errors"},
		)
	}
	return result, nil
}
