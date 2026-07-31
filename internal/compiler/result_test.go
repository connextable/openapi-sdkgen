package sdkgen

import (
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
	"openapi-sdkgen/internal/diagnostic"
)

func TestCompileResultSeparatesExpectedDiagnosticsFromInternalErrors(t *testing.T) {
	result, err := CompileResult([]byte(`{"openapi":"3.1.0","info":{"title":"Broken","version":"1"},"paths":[]}`))
	if err != nil {
		t.Fatalf("internal error = %v", err)
	}
	if result.Document != nil || !diagnostic.HasErrors(result.Diagnostics) {
		t.Fatalf("result = %#v", result)
	}
	if len(result.SkippedPhases) != 0 {
		t.Fatalf("skipped phases = %#v", result.SkippedPhases)
	}
	report := diagnostic.RenderHuman(result.Diagnostics, result.SkippedPhases)
	if !strings.Contains(report, "SDKGEN-E150") || strings.Contains(report, "Skipped phases:") {
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

func TestCompilerDiagnosticCauseRedactsMalformedURLSecrets(t *testing.T) {
	value := sanitizeDiagnosticCause("fetch https://user:secret@example.test/%zz?token=alpha#fragment failed")
	for _, secret := range []string{"user", "secret", "token", "alpha", "fragment"} {
		if strings.Contains(value, secret) {
			t.Fatalf("cause leaked %q: %s", secret, value)
		}
	}
	if !strings.Contains(value, "https://example.test/%zz") {
		t.Fatalf("cause = %s", value)
	}
}

func TestCompilerDiagnosticCauseRedactsMixedCaseURLScheme(t *testing.T) {
	value := sanitizeDiagnosticCause("fetch HTTPS://user:secret@example.test/spec?token=alpha#fragment failed")
	for _, secret := range []string{"user", "secret", "token", "alpha", "fragment"} {
		if strings.Contains(value, secret) {
			t.Fatalf("cause leaked %q: %s", secret, value)
		}
	}
}

func TestCompilerDiagnosticCauseIsSingleLineAndBounded(t *testing.T) {
	value := sanitizeDiagnosticCause(strings.Repeat("detail\n", 400))
	if strings.Contains(value, "\n") || len([]rune(value)) > 1001 || !strings.HasSuffix(value, "…") {
		t.Fatalf("cause = %q", value)
	}
}

func TestCompileErrorPhaseDoesNotDependOnInputName(t *testing.T) {
	result, err := CompileInputResultWithOptions(filepath.Join(t.TempDir(), "decode-reference-openapi.yaml"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	value := result.Diagnostics[0]
	if value.Phase != diagnostic.PhaseInput || value.Code != "SDKGEN-E100" {
		t.Fatalf("diagnostic = %#v", value)
	}
}

func TestPathItemReferenceErrorsUseReferencePhase(t *testing.T) {
	for _, paths := range []string{
		`{"/items":{"$ref":"https://example.test/path-item.yaml"}}`,
		`{"/items":{"$ref":"#/components/pathItems/Loop"}}`,
	} {
		result, err := CompileResult([]byte(`{
  "openapi":"3.1.0",
  "info":{"title":"Path references","version":"1"},
  "paths":` + paths + `,
  "components":{"pathItems":{"Loop":{"$ref":"#/components/pathItems/Loop"}}}
}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Diagnostics) != 1 {
			t.Fatalf("diagnostics = %#v", result.Diagnostics)
		}
		value := result.Diagnostics[0]
		if value.Phase != diagnostic.PhaseReferences || value.Code != "SDKGEN-E120" {
			t.Fatalf("diagnostic = %#v", value)
		}
	}
}

func TestPathItemReferenceDiagnosticsAccumulate(t *testing.T) {
	result, err := CompileResult([]byte(`{
  "openapi":"3.1.0",
  "info":{"title":"Path references","version":"1"},
  "paths":{
    "/external":{"$ref":"https://example.test/path-item.yaml"},
    "/loop":{"$ref":"#/components/pathItems/Loop"}
  },
  "components":{"pathItems":{"Loop":{"$ref":"#/components/pathItems/Loop"}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	for _, value := range result.Diagnostics {
		if value.Phase != diagnostic.PhaseReferences || value.Code != "SDKGEN-E120" {
			t.Fatalf("diagnostic = %#v", value)
		}
	}
}

func TestPathItemReferenceDiagnosticsAccumulateNonObjectTargets(t *testing.T) {
	result, err := CompileResult([]byte(`{
  "openapi":"3.1.0",
  "info":{"title":"Invalid path targets","version":"1"},
  "paths":{
    "/array":{"$ref":"#/components/x-targets/Array"},
    "/scalar":{"$ref":"#/components/x-targets/Scalar"}
  },
  "components":{"x-targets":{"Array":[],"Scalar":"not-a-path-item"}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	for _, value := range result.Diagnostics {
		if value.Phase != diagnostic.PhaseReferences || value.Code != "SDKGEN-E120" {
			t.Fatalf("diagnostic = %#v", value)
		}
	}
}

func TestValidLocalPathItemReferenceDoesNotProduceDiagnostics(t *testing.T) {
	result, err := CompileResult([]byte(`{
  "openapi":"3.1.0",
  "info":{"title":"Valid path reference","version":"1"},
  "paths":{"/items":{"$ref":"#/components/pathItems/Items"}},
  "components":{"pathItems":{"Items":{"get":{"responses":{"200":{"description":"OK"}}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Document == nil || diagnostic.HasErrors(result.Diagnostics) {
		t.Fatalf("result = %#v", result)
	}
}

func TestPathExtensionsMayContainOpaqueReferences(t *testing.T) {
	result, err := CompileResult([]byte(`{
  "openapi":"3.1.0",
  "info":{"title":"Opaque path extension","version":"1"},
  "paths":{
    "x-vendor-local":{"$ref":"#/missing"},
    "x-vendor-remote":{"$ref":"https://example.test/ignored.yaml"},
    "/items":{"get":{"responses":{
      "200":{"description":"OK"},
      "x-vendor-data":{"$ref":"#/also-missing"}
    }}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Document == nil || diagnostic.HasErrors(result.Diagnostics) {
		t.Fatalf("result = %#v", result)
	}
}
