package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
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
		`Run "openapi-sdkgen <command> --help" for command details.` + "\n",
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

func TestRunRendersRootHelpAliases(t *testing.T) {
	expected := readHelpGolden(t, "root.txt")
	for _, args := range [][]string{nil, {"--help"}, {"-h"}, {"help"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			output, diagnostics := captureCLIOutput(t)
			if err := run(args); err != nil {
				t.Fatal(err)
			}
			if output.String() != expected {
				t.Fatalf("root help mismatch:\n%s", output.String())
			}
			if diagnostics.Len() != 0 {
				t.Fatalf("root help diagnostics = %q", diagnostics.String())
			}
		})
	}
}

func TestRunRendersGenerateHelpAliases(t *testing.T) {
	expected := readHelpGolden(t, "generate.txt")
	for _, args := range [][]string{
		{"generate", "--help"},
		{"generate", "-h"},
		{"help", "generate"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			output, diagnostics := captureCLIOutput(t)
			if err := run(args); err != nil {
				t.Fatal(err)
			}
			if output.String() != expected {
				t.Fatalf("generate help mismatch:\n%s", output.String())
			}
			if diagnostics.Len() != 0 {
				t.Fatalf("generate help diagnostics = %q", diagnostics.String())
			}
		})
	}
}

func TestRunUsageErrorsIncludeScopedHelpHints(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		hint string
	}{
		{name: "unknown command", args: []string{"publish"}, hint: `Try "openapi-sdkgen --help" for usage`},
		{name: "unknown help command", args: []string{"help", "publish"}, hint: `Try "openapi-sdkgen --help" for usage`},
		{name: "version with arguments", args: []string{"--version", "extra"}, hint: `Try "openapi-sdkgen --help" for usage`},
		{name: "unknown flag", args: []string{"generate", "--unknown"}, hint: `Try "openapi-sdkgen generate --help" for usage`},
		{name: "unexpected positional", args: []string{"generate", "extra"}, hint: `Try "openapi-sdkgen generate --help" for usage`},
		{name: "missing required", args: []string{"generate"}, hint: `Try "openapi-sdkgen generate --help" for usage`},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, diagnostics := captureCLIOutput(t)
			err := run(test.args)
			if err == nil || !strings.Contains(err.Error(), test.hint) {
				t.Fatalf("error = %v", err)
			}
			if output.Len() != 0 {
				t.Fatalf("usage error stdout = %q", output.String())
			}
			if diagnostics.Len() != 0 {
				t.Fatalf("usage error diagnostics = %q", diagnostics.String())
			}
		})
	}
}

func TestRunReportsDevelopmentAndInjectedVersions(t *testing.T) {
	for _, test := range []struct {
		name     string
		injected string
		expected string
	}{
		{name: "development", expected: "openapi-sdkgen dev\n"},
		{name: "injected", injected: "v1.2.3-rc.1", expected: "openapi-sdkgen 1.2.3-rc.1\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			previousVersion := version
			version = test.injected
			t.Cleanup(func() { version = previousVersion })
			output, diagnostics := captureCLIOutput(t)
			if err := run([]string{"--version"}); err != nil {
				t.Fatal(err)
			}
			if output.String() != test.expected {
				t.Fatalf("version output = %q", output.String())
			}
			if diagnostics.Len() != 0 {
				t.Fatalf("version diagnostics = %q", diagnostics.String())
			}
		})
	}
}

func TestVersionFromBuildInfoUsesModuleInstallsButNotCheckoutBuilds(t *testing.T) {
	module := debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}
	if got := versionFromBuildInfo(&module); got != "1.2.3" {
		t.Fatalf("module version = %q", got)
	}
	checkout := module
	checkout.Settings = []debug.BuildSetting{{Key: "vcs.revision", Value: "0123456789abcdef"}}
	if got := versionFromBuildInfo(&checkout); got != "" {
		t.Fatalf("checkout version = %q", got)
	}
}

func captureCLIOutput(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	previousOutput := standardOutput
	previousError := standardError
	standardOutput = &output
	standardError = &diagnostics
	t.Cleanup(func() {
		standardOutput = previousOutput
		standardError = previousError
	})
	return &output, &diagnostics
}

func readHelpGolden(t *testing.T, name string) string {
	t.Helper()
	value, err := os.ReadFile(filepath.Join("testdata", "help", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(value)
}
