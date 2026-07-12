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
	"github.com/connextable/openapi-sdkgen/internal/generator"
	"github.com/connextable/openapi-sdkgen/internal/target/typescript"
)

const usage = "usage: openapi-sdkgen generate --input <document> --target <target> --output <directory>"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "openapi-sdkgen: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprintln(os.Stdout, usage)
		return nil
	}
	if args[0] != "generate" {
		return fmt.Errorf("unknown command %q; %s", args[0], usage)
	}
	return generate(args[1:])
}

func generate(args []string) error {
	flags := flag.NewFlagSet("generate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := flags.String("input", "", "OpenAPI document path")
	targetName := flags.String("target", "", "SDK target")
	output := flags.String("output", "", "output directory")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse generate arguments: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if *input == "" || *targetName == "" || *output == "" {
		return errors.New("--input, --target, and --output are required")
	}

	registry, err := generator.NewRegistry(typescript.Generator{})
	if err != nil {
		return err
	}
	target, err := registry.Lookup(*targetName)
	if err != nil {
		return err
	}
	document, err := compiler.CompileFile(*input)
	if err != nil {
		return fmt.Errorf("compile %s: %w", *input, err)
	}
	artifacts, err := target.Generate(document, generator.Options{})
	if err != nil {
		return fmt.Errorf("generate %s SDK: %w", target.Name(), err)
	}
	if err := writeArtifacts(*output, artifacts); err != nil {
		return err
	}
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
