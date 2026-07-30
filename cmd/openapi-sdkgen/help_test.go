package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
	"github.com/connextable/openapi-sdkgen/internal/generator"
)

type helpTestTarget string

func (target helpTestTarget) Name() string {
	return string(target)
}

func (target helpTestTarget) Prepare(*ir.Document, generator.Options) (generator.Plan, []diagnostic.Diagnostic, error) {
	return generator.NewPlan(target.Name(), target), nil, nil
}

func (helpTestTarget) Emit(generator.Plan) ([]generator.Artifact, error) {
	return nil, nil
}

func TestRenderHelpUsesStructuredGroupsAndRegistryValues(t *testing.T) {
	targets, err := generator.NewRegistry(helpTestTarget("typescript"), helpTestTarget("swift"))
	if err != nil {
		t.Fatal(err)
	}
	addons, err := generator.NewAddonRegistry(generator.AddonServer, generator.Addon("worker"))
	if err != nil {
		t.Fatal(err)
	}
	document := helpDocument{
		Description: "Generate application SDK source from an OpenAPI document.",
		Usage:       "openapi-sdkgen generate [options]",
		Groups: []helpOptionGroup{
			{
				Title: "Required",
				Options: []helpOption{
					{Name: "target", Metavariable: "name", Summary: "SDK target", Available: targets.Names},
				},
			},
			{
				Title: "Generation",
				Options: []helpOption{
					{Name: "with", Metavariable: "addon", Summary: "Add generated artifacts", Repeatable: true, Available: addons.Names},
				},
			},
			{
				Title: "Options",
				Options: []helpOption{
					{Name: "help", Short: "h", Summary: "Show help"},
				},
			},
		},
	}

	var output bytes.Buffer
	if err := renderHelp(&output, document); err != nil {
		t.Fatal(err)
	}
	const expected = `Generate application SDK source from an OpenAPI document.

Usage:
  openapi-sdkgen generate [options]

Required:
  --target <name>                 SDK target (available: swift, typescript)

Generation:
  --with <addon>                  Add generated artifacts (available: server, worker)
                                  (repeatable)

Options:
  -h, --help                      Show help

`
	if output.String() != expected {
		t.Fatalf("help output mismatch:\n%s", output.String())
	}
}

func TestRenderHelpIncludesCommandsExamplesAndFooter(t *testing.T) {
	document := helpDocument{
		Description: "Generate application SDK source from OpenAPI documents.",
		Usage:       "openapi-sdkgen <command> [options]",
		Commands: []helpCommand{
			{Name: "generate", Summary: "Generate SDK source"},
		},
		Groups: []helpOptionGroup{
			{
				Title: "Options",
				Options: []helpOption{
					{Name: "help", Short: "h", Summary: "Show help"},
				},
			},
		},
		Examples: []string{"openapi-sdkgen generate \\\n  --input ./openapi.yaml"},
		Footer:   `Run "openapi-sdkgen <command> --help" for command details.`,
	}

	var output bytes.Buffer
	if err := renderHelp(&output, document); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, expected := range []string{
		"Commands:\n  generate  Generate SDK source\n\n",
		"Examples:\n  openapi-sdkgen generate \\\n    --input ./openapi.yaml\n\n",
		`Run "openapi-sdkgen <command> --help" for command details.` + "\n\n",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("help output missing %q:\n%s", expected, rendered)
		}
	}
}

func TestRenderHelpRejectsEmptyDynamicAvailability(t *testing.T) {
	document := helpDocument{
		Description: "Generate SDK source.",
		Usage:       "openapi-sdkgen generate [options]",
		Groups: []helpOptionGroup{
			{
				Title: "Required",
				Options: []helpOption{
					{Name: "target", Summary: "SDK target", Available: func() []string { return nil }},
				},
			},
		},
	}

	var output bytes.Buffer
	err := renderHelp(&output, document)
	if err == nil || !strings.Contains(err.Error(), "--target has no available values") {
		t.Fatalf("error = %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("partial help was written: %q", output.String())
	}
}
