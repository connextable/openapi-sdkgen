package generator

import (
	"fmt"

	compiler "github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
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
		return result, nil
	}
	plan, targetDiagnostics, err := target.Prepare(compiled.Document, options)
	if err != nil {
		return Preparation{}, err
	}
	result.Plan = plan
	result.Diagnostics = diagnostic.Sort(append(result.Diagnostics, targetDiagnostics...))
	return result, nil
}
