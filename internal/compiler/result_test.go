package sdkgen

import (
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
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
