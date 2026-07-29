package diagnostic

import (
	"strings"
	"testing"
)

func TestCollectorSortCountsAndRender(t *testing.T) {
	collector := &Collector{}
	collector.Add(Diagnostic{
		Severity: SeverityWarning,
		Code:     "SDKGEN-W002",
		Phase:    PhaseTarget,
		Location: Location{Source: "contract.yaml", Pointer: "#/paths/~1pets/get"},
		Message:  "redundant declaration",
		Related: []Location{
			{Source: "schema.yaml", Pointer: "#/B"},
			{Source: "schema.yaml", Pointer: "#/A"},
			{Source: "schema.yaml", Pointer: "#/A"},
		},
	})
	collector.Add(Diagnostic{
		Severity: SeverityError,
		Code:     "SDKGEN-E001",
		Phase:    PhaseOpenAPI,
		Location: Location{Source: "contract.yaml", Pointer: "#/paths"},
		Message:  "paths must be an object",
		Hint:     "replace paths with an object",
	})

	values := collector.Diagnostics()
	if len(values) != 2 || values[0].Code != "SDKGEN-E001" {
		t.Fatalf("diagnostics = %#v", values)
	}
	if got := len(values[1].Related); got != 2 {
		t.Fatalf("related locations = %d, want 2", got)
	}
	if counts := Count(values); counts.Errors != 1 || counts.Warnings != 1 {
		t.Fatalf("counts = %#v", counts)
	}
	report := RenderHuman(values, []SkippedPhase{{Phase: PhaseIR, Reason: "OpenAPI model unavailable"}})
	for _, want := range []string{
		"OpenAPI SDK generation: 1 error(s), 1 warning(s)",
		"Phase: openapi",
		"Phase: target",
		"Source: contract.yaml",
		"error [SDKGEN-E001] paths must be an object",
		"hint: replace paths with an object",
		"warning [SDKGEN-W002] redundant declaration",
		"related: schema.yaml#/A",
		"Skipped phases:",
		"- ir: OpenAPI model unavailable",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestSourceRegistryRedactsAndAssignsStableOpaqueOrdinals(t *testing.T) {
	first := "https://user:secret@example.test/openapi.json?token=alpha#one"
	second := "https://other:credential@example.test/openapi.json?token=beta#two"
	forward := NewSourceRegistry([]string{first, second})
	reverse := NewSourceRegistry([]string{second, first})
	for _, source := range []string{first, second} {
		if forward.Display(source) != reverse.Display(source) {
			t.Fatalf("display changed with discovery order: %q != %q", forward.Display(source), reverse.Display(source))
		}
		display := forward.Display(source)
		for _, secret := range []string{"user", "secret", "other", "credential", "alpha", "beta", "#one", "#two"} {
			if strings.Contains(display, secret) {
				t.Fatalf("display leaked %q: %s", secret, display)
			}
		}
		if !strings.Contains(display, "[source ") {
			t.Fatalf("collision display lacks opaque ordinal: %s", display)
		}
	}
}

func TestSafeSourceDisplayRedactsMalformedHTTPURL(t *testing.T) {
	source := "https://user:secret@example.test/%zz?token=alpha#fragment"
	display := SafeSourceDisplay(source)
	if display != "https://example.test/%zz" {
		t.Fatalf("display = %q", display)
	}
	for _, secret := range []string{"user", "secret", "token", "alpha", "fragment"} {
		if strings.Contains(display, secret) {
			t.Fatalf("display leaked %q: %s", secret, display)
		}
	}
}

func TestSanitizeSourcesCoversPrimaryAndRelatedLocations(t *testing.T) {
	values := SanitizeSources([]Diagnostic{{
		Location: Location{Source: "https://user:secret@example.test/%zz?token=alpha#fragment"},
		Related:  []Location{{Source: "https://other:secret@example.test/%zz?token=beta#fragment"}},
	}})
	if len(values) != 1 || len(values[0].Related) != 1 {
		t.Fatalf("diagnostics = %#v", values)
	}
	for _, source := range []string{values[0].Location.Source, values[0].Related[0].Source} {
		for _, secret := range []string{"user", "other", "secret", "token", "alpha", "beta", "fragment"} {
			if strings.Contains(source, secret) {
				t.Fatalf("source leaked %q: %s", secret, source)
			}
		}
	}
}
