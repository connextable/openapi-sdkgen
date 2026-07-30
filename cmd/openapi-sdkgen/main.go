// openapi-sdkgen compiles OpenAPI documents into client SDK packages.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	compiler "github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
	"github.com/connextable/openapi-sdkgen/internal/generator"
	"github.com/connextable/openapi-sdkgen/internal/target/typescript"
)

var standardInput io.Reader = os.Stdin
var standardOutput io.Writer = os.Stdout
var standardError io.Writer = os.Stderr

var errReportedDiagnostics = errors.New("generation blocked by reported diagnostics")

type internalGenerationError struct {
	label string
	cause error
}

func (value *internalGenerationError) Error() string { return value.label }
func (value *internalGenerationError) Unwrap() error { return value.cause }

func internalFailure(label string, cause error) error {
	return &internalGenerationError{label: label, cause: cause}
}

type generationRuntime struct {
	compile func(string, compiler.CompileOptions) (compiler.Result, error)
	prepare func(generator.Target, compiler.Result, generator.Options) (generator.Preparation, error)
	emit    func(generator.Target, generator.Plan) ([]generator.Artifact, error)
	publish func(string, []generator.Artifact) error
}

type cliRegistries struct {
	targets *generator.Registry
	addons  *generator.AddonRegistry
}

type generateFlagValues struct {
	input            *string
	inputBase        *string
	targetName       *string
	output           *string
	with             repeatedStrings
	remoteRefs       repeatedStrings
	schemaExtensions repeatedStrings
	httpHeaderEnv    rawStrings
	refLock          *string
	updateRefLock    *bool
	offline          *bool
	tlsClientCert    *string
	tlsClientKey     *string
	tlsCAFile        *string
}

var defaultGenerationRuntime = generationRuntime{
	compile: compiler.CompileInputResultWithOptions,
	prepare: generator.PrepareCompilation,
	emit: func(target generator.Target, plan generator.Plan) ([]generator.Artifact, error) {
		return target.Emit(plan)
	},
	publish: writeArtifacts,
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if !errors.Is(err, errReportedDiagnostics) {
			fmt.Fprintf(standardError, "openapi-sdkgen: %v\n", err)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	registries, err := newCLIRegistries()
	if err != nil {
		return err
	}
	return runWithRegistries(args, defaultGenerationRuntime, registries)
}

func runWithRegistries(args []string, runtime generationRuntime, registries cliRegistries) error {
	if len(args) == 0 {
		return writeRootHelp(registries)
	}
	switch args[0] {
	case "--help", "-h":
		if len(args) != 1 {
			return rootUsageError("help does not accept additional arguments")
		}
		return writeRootHelp(registries)
	case "help":
		switch {
		case len(args) == 1:
			return writeRootHelp(registries)
		case len(args) == 2 && args[1] == "generate":
			return writeGenerateHelp(registries)
		case len(args) == 2:
			return rootUsageError(fmt.Sprintf("unknown command %q", args[1]))
		default:
			return rootUsageError("help accepts at most one command")
		}
	case "generate":
		return generateWithRegistries(args[1:], runtime, registries)
	default:
		return rootUsageError(fmt.Sprintf("unknown command %q", args[0]))
	}
}

func generate(args []string) error {
	return generateWithRuntime(args, defaultGenerationRuntime)
}

func generateWithRuntime(args []string, runtime generationRuntime) error {
	registries, err := newCLIRegistries()
	if err != nil {
		return err
	}
	return generateWithRegistries(args, runtime, registries)
}

func generateWithRegistries(args []string, runtime generationRuntime, registries cliRegistries) error {
	flags, values := newGenerateFlagSet(registries)
	if err := flags.Flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return writeGenerateHelpWithFlags(registries, flags)
		}
		return generateUsageError(fmt.Sprintf("parse generate arguments: %v", err))
	}
	if flags.Flags.NArg() != 0 {
		return generateUsageError(fmt.Sprintf("unexpected arguments: %s", strings.Join(flags.Flags.Args(), " ")))
	}
	if *values.input == "" || *values.targetName == "" || *values.output == "" {
		return generateUsageError("--input, --target, and --output are required")
	}
	target, err := registries.targets.Lookup(*values.targetName)
	if err != nil {
		return err
	}
	options, err := registries.addons.Resolve(values.with)
	if err != nil {
		return err
	}
	if err := generator.ValidateTargetOptions(target, options); err != nil {
		return err
	}
	compiled, err := runtime.compile(*values.input, compiler.CompileOptions{
		InputBase:                *values.inputBase,
		InputReader:              standardInput,
		RemoteRefAllowlist:       values.remoteRefs,
		RefLockPath:              *values.refLock,
		UpdateRefLock:            *values.updateRefLock,
		Offline:                  *values.offline,
		SchemaExtensionManifests: values.schemaExtensions,
		HTTPHeaderEnv:            values.httpHeaderEnv,
		TLSClientCert:            *values.tlsClientCert,
		TLSClientKey:             *values.tlsClientKey,
		TLSCAFile:                *values.tlsCAFile,
	})
	if err != nil {
		writeDiagnostics(compiled.Diagnostics, compiled.SkippedPhases)
		return internalFailure("internal compiler failure", err)
	}
	prepared, err := runtime.prepare(target, compiled, options)
	if err != nil {
		writeDiagnostics(prepared.Diagnostics, prepared.SkippedPhases)
		return internalFailure(fmt.Sprintf("internal %s preparation failure", target.Name()), err)
	}
	writeDiagnostics(prepared.Diagnostics, prepared.SkippedPhases)
	if diagnostic.HasErrors(prepared.Diagnostics) {
		return errReportedDiagnostics
	}
	artifacts, err := runtime.emit(target, prepared.Plan)
	if err != nil {
		return internalFailure(fmt.Sprintf("internal %s emission failure", target.Name()), err)
	}
	if err := runtime.publish(*values.output, artifacts); err != nil {
		return internalFailure("internal output publication failure", err)
	}
	return nil
}

func newCLIRegistries() (cliRegistries, error) {
	targets, err := generator.NewRegistry(typescript.Generator{})
	if err != nil {
		return cliRegistries{}, err
	}
	addons, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		return cliRegistries{}, err
	}
	return cliRegistries{targets: targets, addons: addons}, nil
}

func newGenerateFlagSet(registries cliRegistries) (*commandFlagSet, *generateFlagValues) {
	const (
		requiredGroup = iota
		generationGroup
		inputGroup
		remoteReferenceGroup
		schemaExtensionGroup
	)
	flags := newCommandFlagSet(
		"generate",
		"Required",
		"Generation",
		"Input",
		"Remote references",
		"Schema extensions",
	)
	values := &generateFlagValues{}
	values.input = flags.String(requiredGroup, helpOption{
		Name: "input", Metavariable: "source",
		Summary: "OpenAPI file, file:// URL, HTTP(S) URL, or -",
	}, "")
	values.targetName = flags.String(requiredGroup, helpOption{
		Name: "target", Metavariable: "name", Summary: "SDK target",
		Available: registries.targets.Names,
	}, "")
	values.output = flags.String(requiredGroup, helpOption{
		Name: "output", Metavariable: "directory", Summary: "Fresh output directory",
	}, "")
	flags.Var(generationGroup, helpOption{
		Name: "with", Metavariable: "addon", Summary: "Add generated artifacts",
		Repeatable: true, Available: registries.addons.Names,
	}, &values.with)
	values.inputBase = flags.String(inputGroup, helpOption{
		Name: "input-base", Metavariable: "source",
		Summary: "Base location for relative references from stdin",
	}, "")
	flags.Var(inputGroup, helpOption{
		Name: "http-header-env", Metavariable: "header=env",
		Summary:    "Read an input request header from an environment variable",
		Repeatable: true,
	}, &values.httpHeaderEnv)
	values.tlsClientCert = flags.String(inputGroup, helpOption{
		Name: "tls-client-cert", Metavariable: "path",
		Summary: "PEM client certificate for an HTTPS input",
	}, "")
	values.tlsClientKey = flags.String(inputGroup, helpOption{
		Name: "tls-client-key", Metavariable: "path",
		Summary: "PEM private key for an HTTPS input",
	}, "")
	values.tlsCAFile = flags.String(inputGroup, helpOption{
		Name: "tls-ca-file", Metavariable: "path",
		Summary: "Additional PEM certificate authorities for an HTTPS input",
	}, "")
	flags.Var(remoteReferenceGroup, helpOption{
		Name: "allow-remote-ref", Metavariable: "origin",
		Summary: "Allow an exact HTTPS remote-reference origin", Repeatable: true,
	}, &values.remoteRefs)
	values.refLock = flags.String(remoteReferenceGroup, helpOption{
		Name: "ref-lock", Metavariable: "path",
		Summary: "Remote-reference and extension lock path",
	}, "")
	values.updateRefLock = flags.Bool(remoteReferenceGroup, helpOption{
		Name: "update-ref-lock", Summary: "Create or update the integrity lock",
	}, false)
	values.offline = flags.Bool(remoteReferenceGroup, helpOption{
		Name: "offline", Summary: "Use only locked cached remote references",
	}, false)
	flags.Var(schemaExtensionGroup, helpOption{
		Name: "schema-extension", Metavariable: "manifest",
		Summary: "Register a trusted schema-extension manifest", Repeatable: true,
	}, &values.schemaExtensions)
	return flags, values
}

func writeRootHelp(registries cliRegistries) error {
	document := helpDocument{
		Description: "openapi-sdkgen generates application SDK source from OpenAPI documents.",
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
		Footer: `Run "openapi-sdkgen <command> --help" for command details.`,
	}
	if registries.targets == nil || registries.addons == nil {
		return errors.New("CLI registries are not configured")
	}
	if err := renderHelp(standardOutput, document); err != nil {
		return fmt.Errorf("render root help: %w", err)
	}
	return nil
}

func writeGenerateHelp(registries cliRegistries) error {
	if registries.targets == nil || registries.addons == nil {
		return errors.New("CLI registries are not configured")
	}
	flags, _ := newGenerateFlagSet(registries)
	return writeGenerateHelpWithFlags(registries, flags)
}

func writeGenerateHelpWithFlags(registries cliRegistries, flags *commandFlagSet) error {
	if registries.targets == nil || registries.addons == nil {
		return errors.New("CLI registries are not configured")
	}
	groups := append([]helpOptionGroup(nil), flags.Groups...)
	groups = append(groups, helpOptionGroup{
		Title: "Options",
		Options: []helpOption{
			{Name: "help", Short: "h", Summary: "Show help"},
		},
	})
	document := helpDocument{
		Description: "Generate application SDK source from an OpenAPI document.",
		Usage:       "openapi-sdkgen generate [options]",
		Groups:      groups,
		Examples: []string{`openapi-sdkgen generate \
  --input ./openapi.yaml \
  --target typescript \
  --output ./src/generated/api`},
	}
	if err := renderHelp(standardOutput, document); err != nil {
		return fmt.Errorf("render generate help: %w", err)
	}
	return nil
}

func rootUsageError(message string) error {
	return fmt.Errorf("%s\nTry \"openapi-sdkgen --help\" for usage", message)
}

func generateUsageError(message string) error {
	return fmt.Errorf("%s\nTry \"openapi-sdkgen generate --help\" for usage", message)
}

func writeDiagnostics(values []diagnostic.Diagnostic, skipped []diagnostic.SkippedPhase) {
	if len(values) == 0 && len(skipped) == 0 {
		return
	}
	fmt.Fprint(standardError, diagnostic.RenderHuman(values, skipped))
}

type repeatedStrings []string

func (values *repeatedStrings) String() string {
	return strings.Join(*values, ",")
}

type rawStrings []string

func (values *rawStrings) String() string {
	return strings.Join(*values, ",")
}

func (values *rawStrings) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func (values *repeatedStrings) Set(value string) error {
	if value == "" {
		return errors.New("--with requires a non-empty add-on name")
	}
	*values = append(*values, value)
	return nil
}

func writeArtifacts(output string, artifacts []generator.Artifact) error {
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	seen := make(map[string]bool, len(artifacts))
	for _, artifact := range artifacts {
		cleanPath, err := safeArtifactPath(artifact.Path)
		if err != nil {
			return err
		}
		if seen[cleanPath] {
			return fmt.Errorf("duplicate generated artifact %q", cleanPath)
		}
		seen[cleanPath] = true
	}
	if info, err := os.Lstat(output); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("output path %s must not be a symlink", output)
		}
		return fmt.Errorf("output path %s already exists; choose a fresh directory", output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect output path %s: %w", output, err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("create output parent directory: %w", err)
	}
	staging, err := os.MkdirTemp(filepath.Dir(output), ".openapi-sdkgen-output-*")
	if err != nil {
		return fmt.Errorf("create output staging directory: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(staging)
		}
	}()
	for _, artifact := range artifacts {
		path := filepath.Join(staging, filepath.Clean(filepath.FromSlash(artifact.Path)))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create artifact directory %s: %w", filepath.Dir(path), err)
		}
		if err := writeFile(path, artifact.Data); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, output); err != nil {
		return fmt.Errorf("publish generated output %s: %w", output, err)
	}
	committed = true
	return nil
}

func safeArtifactPath(value string) (string, error) {
	cleanPath := filepath.Clean(filepath.FromSlash(value))
	if cleanPath == "." || filepath.IsAbs(cleanPath) || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid generated artifact path %q", value)
	}
	return cleanPath, nil
}

func writeFile(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".openapi-sdkgen-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary artifact %s: %w", path, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write generated artifact %s: %w", path, err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return fmt.Errorf("set generated artifact mode %s: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close generated artifact %s: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace generated artifact %s: %w", path, err)
	}
	return nil
}
