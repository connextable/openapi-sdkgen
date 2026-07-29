package generator

import (
	"errors"
	"strings"
	"testing"

	compiler "github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
)

type pipelineTarget struct {
	prepared   bool
	prepareErr error
	targetErr  bool
}

func (*pipelineTarget) Name() string { return "pipeline" }

func (target *pipelineTarget) Prepare(*ir.Document, Options) (Plan, []diagnostic.Diagnostic, error) {
	target.prepared = true
	value := diagnostic.Diagnostic{
		Severity: diagnostic.SeverityWarning,
		Code:     "SDKGEN-W500",
		Phase:    diagnostic.PhaseTarget,
		Message:  "target warning",
	}
	if target.targetErr {
		value.Severity = diagnostic.SeverityError
		value.Code = "SDKGEN-E500"
		value.Message = "target error"
	}
	return NewPlan(target.Name(), "value"), []diagnostic.Diagnostic{value}, target.prepareErr
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
	report := diagnostic.RenderHuman(result.Diagnostics, result.SkippedPhases)
	for _, phase := range []string{"- target:", "- emit:", "- publish:"} {
		if !strings.Contains(report, phase) {
			t.Fatalf("report missing %s:\n%s", phase, report)
		}
	}
}

func TestPrepareCompilationRejectsBrokenCompilerInvariant(t *testing.T) {
	_, err := PrepareCompilation(&pipelineTarget{}, compiler.Result{}, Options{})
	if err == nil || !strings.Contains(err.Error(), "neither a document nor an error diagnostic") {
		t.Fatalf("error = %v", err)
	}
}

func TestPrepareCompilationPreservesDiagnosticsWhenTargetFailsInternally(t *testing.T) {
	target := &pipelineTarget{prepareErr: errors.New("boom")}
	result, err := PrepareCompilation(target, compiler.Result{
		Document: &ir.Document{},
		Diagnostics: []diagnostic.Diagnostic{{
			Severity: diagnostic.SeverityWarning,
			Code:     "SDKGEN-W100",
			Phase:    diagnostic.PhaseInput,
			Message:  "compiler warning",
		}},
	}, Options{})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("error = %v", err)
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestPrepareCompilationRecordsSkippedEmitAndPublishAfterTargetErrors(t *testing.T) {
	result, err := PrepareCompilation(&pipelineTarget{targetErr: true}, compiler.Result{
		Document: &ir.Document{},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	report := diagnostic.RenderHuman(result.Diagnostics, result.SkippedPhases)
	for _, expected := range []string{"SDKGEN-E500", "- emit:", "- publish:"} {
		if !strings.Contains(report, expected) {
			t.Fatalf("report missing %s:\n%s", expected, report)
		}
	}
}
