package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"openapi-sdkgen/internal/compiler/ir"
	"openapi-sdkgen/internal/diagnostic"
	"openapi-sdkgen/internal/generator"
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
		Commands: []cliCommand{
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

func TestBoolMetadataDrivesParserAliasesAndHelp(t *testing.T) {
	flags := newCommandFlagSet("inspect", "Options")
	showDocs := flags.Bool(0, helpOption{
		Name: "docs", Short: "d", Summary: "Show documentation",
	}, false)
	if err := flags.Flags.Parse([]string{"-d"}); err != nil {
		t.Fatal(err)
	}
	if !*showDocs {
		t.Fatal("short alias did not set the metadata-backed flag")
	}

	var output bytes.Buffer
	if err := renderHelp(&output, helpDocument{
		Usage:  "openapi-sdkgen inspect [options]",
		Groups: flags.Groups,
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "  -d, --docs") {
		t.Fatalf("help did not render the parser metadata:\n%s", output.String())
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

func TestCommandMetadataDrivesDispatchAndRootHelp(t *testing.T) {
	registries, err := newCLIRegistries()
	if err != nil {
		t.Fatal(err)
	}
	var received []string
	helpCalled := false
	application := &cliApplication{
		registries: registries,
		commands: []cliCommand{
			{
				Name:    "inspect",
				Summary: "Inspect an OpenAPI document",
				Run: func(args []string) error {
					received = append([]string(nil), args...)
					return nil
				},
				Help: func() error {
					helpCalled = true
					return nil
				},
			},
		},
	}
	application.options = newRootOptions(application)
	application.options[0].Aliases = []string{"docs"}

	if err := application.run([]string{"inspect", "document.yaml"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(received, " ") != "document.yaml" {
		t.Fatalf("command arguments = %#v", received)
	}
	if err := application.run([]string{"docs", "inspect"}); err != nil {
		t.Fatal(err)
	}
	if !helpCalled {
		t.Fatal("command help handler was not called")
	}
	if err := application.run([]string{"help", "inspect"}); err == nil ||
		!strings.Contains(err.Error(), `unknown command "help"`) {
		t.Fatalf("stale help alias error = %v", err)
	}

	output, _ := captureCLIOutput(t)
	if err := application.writeRootHelp(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "  inspect  Inspect an OpenAPI document\n") {
		t.Fatalf("root help did not use command metadata:\n%s", output.String())
	}
}

func TestCLIUsageErrorsExitWithScopedStderr(t *testing.T) {
	for _, test := range []struct {
		name   string
		args   []string
		stderr string
	}{
		{
			name: "unknown command",
			args: []string{"publish"},
			stderr: "openapi-sdkgen: unknown command \"publish\"\n" +
				"Try \"openapi-sdkgen --help\" for usage\n",
		},
		{
			name: "unknown flag",
			args: []string{"generate", "--unknown"},
			stderr: "openapi-sdkgen: parse generate arguments: flag provided but not defined: -unknown\n" +
				"Try \"openapi-sdkgen generate --help\" for usage\n",
		},
		{
			name: "missing required",
			args: []string{"generate"},
			stderr: "openapi-sdkgen: --input, --target, and --output are required\n" +
				"Try \"openapi-sdkgen generate --help\" for usage\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			commandArgs := append([]string{"-test.run=^TestCLIHelperProcess$", "--"}, test.args...)
			command := exec.Command(os.Args[0], commandArgs...)
			command.Env = append(os.Environ(), "OPENAPI_SDKGEN_TEST_HELPER=1")
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			exitError, ok := err.(*exec.ExitError)
			if !ok || exitError.ExitCode() != 1 {
				t.Fatalf("exit error = %v", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q", stdout.String())
			}
			if stderr.String() != test.stderr {
				t.Fatalf("stderr = %q", stderr.String())
			}
		})
	}
}

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("OPENAPI_SDKGEN_TEST_HELPER") != "1" {
		return
	}
	for index, argument := range os.Args {
		if argument == "--" {
			os.Args = append([]string{"openapi-sdkgen"}, os.Args[index+1:]...)
			main()
			return
		}
	}
	t.Fatal("missing helper-process argument separator")
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
