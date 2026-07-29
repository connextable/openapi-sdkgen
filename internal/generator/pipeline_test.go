package generator

import (
	"strings"
	"testing"

	compiler "github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
)

type pipelineTarget struct {
	prepared bool
}

func (*pipelineTarget) Name() string { return "pipeline" }

func (target *pipelineTarget) Prepare(*ir.Document, Options) (Plan, []diagnostic.Diagnostic, error) {
	target.prepared = true
	return NewPlan(target.Name(), "value"), []diagnostic.Diagnostic{{
		Severity: diagnostic.SeverityWarning,
		Code:     "SDKGEN-W500",
		Phase:    diagnostic.PhaseTarget,
		Message:  "target warning",
	}}, nil
}

func (*pipelineTarget) Emit(Plan) ([]Artifact, error) { return nil, nil }

func TestPrepareCompilationPreservesCompilerDiagnosticsAndMergesTargetDiagnostics(t *testing.T) {
	target := &pipelineTarget{}
	result, err := PrepareCompilation(target, compiler.Result{
		Document: &ir.Document{},
		Diagnostics: []diagnostic.Diagnostic{{
			Severity: diagnostic.SeverityWarning,
			Code:     "SDKGEN-W100",
			Phase:    diagnostic.PhaseInput,
			Message:  "compiler warning",
		}},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !target.prepared || len(result.Diagnostics) != 2 {
		t.Fatalf("preparation = %#v, target prepared = %v", result, target.prepared)
	}
}

func TestPrepareCompilationSkipsTargetAfterCompilerError(t *testing.T) {
	target := &pipelineTarget{}
	result, err := PrepareCompilation(target, compiler.Result{
		Diagnostics: []diagnostic.Diagnostic{{
			Severity: diagnostic.SeverityError,
			Code:     "SDKGEN-E100",
			Phase:    diagnostic.PhaseInput,
			Message:  "input error",
		}},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if target.prepared || !diagnostic.HasErrors(result.Diagnostics) {
		t.Fatalf("preparation = %#v, target prepared = %v", result, target.prepared)
	}
}

func TestPrepareCompilationRejectsBrokenCompilerInvariant(t *testing.T) {
	_, err := PrepareCompilation(&pipelineTarget{}, compiler.Result{}, Options{})
	if err == nil || !strings.Contains(err.Error(), "neither a document nor an error diagnostic") {
		t.Fatalf("error = %v", err)
	}
}
