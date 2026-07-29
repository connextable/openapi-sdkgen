package sdkgen

import (
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
	"go.yaml.in/yaml/v4"
)

func TestCompileResultSeparatesExpectedDiagnosticsFromInternalErrors(t *testing.T) {
	result, err := CompileResult([]byte(`{"openapi":"3.1.0","info":{"title":"Broken","version":"1"},"paths":[]}`))
	if err != nil {
		t.Fatalf("internal error = %v", err)
	}
	if result.Document != nil || !diagnostic.HasErrors(result.Diagnostics) {
		t.Fatalf("result = %#v", result)
	}
	if len(result.SkippedPhases) == 0 {
		t.Fatal("expected prerequisite-bound skipped phases")
	}
	report := diagnostic.RenderHuman(result.Diagnostics, result.SkippedPhases)
	if !strings.Contains(report, "SDKGEN-E140") || !strings.Contains(report, "Skipped phases:") {
		t.Fatalf("report = %s", report)
	}
}

func TestCompileInputResultCollectsTransportWarningWithoutWriting(t *testing.T) {
	t.Setenv("SDKGEN_TOKEN", "secret")
	result, err := CompileInputResultWithOptions("-", CompileOptions{
		InputReader:   strings.NewReader(`{"openapi":"3.1.0","info":{"title":"Input","version":"1"},"paths":{}}`),
		InputBase:     "http://example.test/openapi.json",
		HTTPHeaderEnv: []string{"Authorization=SDKGEN_TOKEN"},
	})
	if err != nil {
		t.Fatalf("internal error = %v", err)
	}
	if result.Document == nil || len(result.Diagnostics) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if got := result.Diagnostics[0]; got.Code != "SDKGEN-W101" || got.Severity != diagnostic.SeverityWarning {
		t.Fatalf("warning = %#v", got)
	}
}

func TestCompileResultAccumulatesIndependentUnresolvedLocalReferences(t *testing.T) {
	result, err := CompileResult([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "References", "version": "1"},
  "paths": {
    "/one": {"get": {"responses": {"200": {
      "description": "One",
      "content": {"application/json": {"schema": {"$ref": "#/components/schemas/MissingOne"}}}
    }}}},
    "/two": {"get": {"responses": {"200": {
      "description": "Two",
      "content": {"application/json": {"schema": {"$ref": "#/components/schemas/MissingTwo"}}}
    }}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Document != nil || len(result.Diagnostics) != 2 {
		t.Fatalf("result = %#v", result)
	}
	for _, value := range result.Diagnostics {
		if value.Code != "SDKGEN-E120" || value.Phase != diagnostic.PhaseReferences {
			t.Fatalf("diagnostic = %#v", value)
		}
	}
	report := diagnostic.RenderHuman(result.Diagnostics, result.SkippedPhases)
	for _, expected := range []string{"MissingOne", "MissingTwo", "Phase: references", "- normalize:", "- openapi:", "- ir:"} {
		if !strings.Contains(report, expected) {
			t.Fatalf("report missing %q:\n%s", expected, report)
		}
	}
}

func TestReferenceScanIgnoresLiteralAndVendorPayloadRefKeys(t *testing.T) {
	var value any
	err := yaml.Unmarshal([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Literal references", "version": "1"},
  "paths": {},
  "components": {
    "schemas": {
      "Payload": {
        "type": "object",
        "default": {"$ref": "#/not/a/schema"},
        "x-vendor-data": {"$ref": "#/also/not/a/schema"}
      }
    }
  }
}`), &value)
	if err != nil {
		t.Fatal(err)
	}
	if values := unresolvedLocalReferenceDiagnostics(value, "fixture"); len(values) != 0 {
		t.Fatalf("literal reference diagnostics = %#v", values)
	}
}

func TestCompilerDiagnosticCauseRedactsURLSecrets(t *testing.T) {
	value := sanitizeDiagnosticCause("fetch https://user:secret@example.test/openapi.json?token=alpha#fragment failed")
	for _, secret := range []string{"user", "secret", "alpha", "fragment"} {
		if strings.Contains(value, secret) {
			t.Fatalf("cause leaked %q: %s", secret, value)
		}
	}
	if !strings.Contains(value, "https://example.test/openapi.json") {
		t.Fatalf("cause = %s", value)
	}
}

func TestCompilerDiagnosticCauseIsSingleLineAndBounded(t *testing.T) {
	value := sanitizeDiagnosticCause(strings.Repeat("detail\n", 400))
	if strings.Contains(value, "\n") || len([]rune(value)) > 1001 || !strings.HasSuffix(value, "…") {
		t.Fatalf("cause = %q", value)
	}
}
