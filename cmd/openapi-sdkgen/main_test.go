package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/generator"
)

func TestGenerateWritesTypeScriptSourceTree(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "contract.json")
	if err := os.WriteFile(input, []byte(minimalDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "generated-client")
	if err := run([]string{"generate", "--input", input, "--target", "typescript", "--output", output}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"index.ts", "metadata.ts", "generated/types.ts", "generated/client.ts", "generated/errors.ts", "generated/index.ts", "generated/runtime.ts"} {
		if _, err := os.Stat(filepath.Join(output, expected)); err != nil {
			t.Fatalf("missing %s: %v", expected, err)
		}
	}
	for _, forbidden := range []string{"package.json", "tsconfig.json", "manifest.json", "README.md"} {
		if _, err := os.Stat(filepath.Join(output, forbidden)); !os.IsNotExist(err) {
			t.Fatalf("source output unexpectedly contains %s: %v", forbidden, err)
		}
	}
}

func TestGenerateWithServerWritesOnlyExplicitServerEntry(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "contract.json")
	if err := os.WriteFile(input, []byte(minimalDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "generated-client")
	if err := run([]string{"generate", "--input", input, "--target", "typescript", "--with", "server", "--output", output}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"server/runtime.ts", "server/webhooks.ts", "server/callbacks.ts"} {
		if _, err := os.Stat(filepath.Join(output, expected)); err != nil {
			t.Fatalf("missing %s: %v", expected, err)
		}
	}
	if _, err := os.Stat(filepath.Join(output, "server", "index.ts")); !os.IsNotExist(err) {
		t.Fatalf("server barrel unexpectedly exists: %v", err)
	}
	root, err := os.ReadFile(filepath.Join(output, "index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(root), "server") {
		t.Fatalf("client root imports server: %s", root)
	}
}

func TestGenerateRejectsRemovedPackageNameFlag(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "contract.json")
	if err := os.WriteFile(input, []byte(minimalDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "generated")
	err := run([]string{"generate", "--input", input, "--target", "typescript", "--output", output, "--package-name", "@example/client"})
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("unexpected output stat error = %v", err)
	}
}

func TestGenerateRejectsUnknownTarget(t *testing.T) {
	err := run([]string{"generate", "--input", "contract.json", "--target", "kotlin", "--output", "out"})
	if err == nil || !strings.Contains(err.Error(), "unsupported SDK target") {
		t.Fatalf("error = %v", err)
	}
}

func TestGenerateJavaScriptTarget(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "openapi.json")
	output := filepath.Join(directory, "generated")
	if err := os.WriteFile(input, []byte(`{"openapi":"3.0.3","info":{"title":"JavaScript","version":"1"},"paths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"generate", "--input", input, "--target", "javascript", "--output", output}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"index.js", "metadata.js", "generated/client.js", "generated/runtime.js"} {
		if _, err := os.Stat(filepath.Join(output, path)); err != nil {
			t.Fatalf("missing JavaScript artifact %s: %v", path, err)
		}
	}
}

func TestGenerateParsesRepeatableWithAddons(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "openapi.json")
	if err := os.WriteFile(input, []byte(`{"openapi":"3.0.3","info":{"title":"Add-on","version":"1"},"paths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown", args: []string{"--with", "worker"}, want: "unsupported SDK add-on"},
		{name: "duplicate", args: []string{"--with", "server", "--with", "server"}, want: "specified more than once"},
		{name: "javascript incompatible", args: []string{"--with", "server"}, want: `target "javascript" does not support add-on "server"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(directory, test.name)
			target := "typescript"
			if test.name == "javascript incompatible" {
				target = "javascript"
			}
			args := append([]string{"generate", "--input", input, "--target", target, "--output", output}, test.args...)
			err := run(args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("unexpected output stat error = %v", err)
			}
		})
	}
}

func TestRunRejectsInvalidArgumentsWithoutCreatingOutput(t *testing.T) {
	directory := t.TempDir()
	output := filepath.Join(directory, "output")
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown command", args: []string{"publish"}, want: "unknown command"},
		{name: "missing flags", args: []string{"generate"}, want: "required"},
		{name: "unexpected positional", args: []string{"generate", "extra"}, want: "unexpected arguments"},
		{name: "missing input", args: []string{"generate", "--input", filepath.Join(directory, "missing.json"), "--target", "typescript", "--output", output}, want: "compile"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := run(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("unexpected output stat error = %v", err)
			}
		})
	}
}

func TestSafeArtifactPathRejectsTraversal(t *testing.T) {
	for _, value := range []string{"", ".", "..", "../outside.ts", "/outside.ts"} {
		t.Run(value, func(t *testing.T) {
			if _, err := safeArtifactPath(value); err == nil {
				t.Fatalf("invalid path %q was accepted", value)
			}
		})
	}
}

func TestWriteArtifactsRejectsSymlinkOutput(t *testing.T) {
	directory := t.TempDir()
	outside := t.TempDir()
	output := filepath.Join(directory, "output")
	if err := os.Symlink(outside, output); err != nil {
		t.Fatal(err)
	}
	err := writeArtifacts(output, []generator.Artifact{{Path: "generated/client.ts", Data: []byte("export {}\n")}})
	if err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("writeArtifacts error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "generated", "client.ts")); !os.IsNotExist(err) {
		t.Fatalf("outside artifact stat error = %v", err)
	}
}

func TestWriteArtifactsRollsBackArtifactPathConflict(t *testing.T) {
	directory := t.TempDir()
	output := filepath.Join(directory, "output")
	err := writeArtifacts(output, []generator.Artifact{
		{Path: "generated", Data: []byte("not a directory\n")},
		{Path: "generated/client.ts", Data: []byte("export {}\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "create artifact directory") {
		t.Fatalf("writeArtifacts error = %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("partial output stat error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(directory, ".openapi-sdkgen-output-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("staging directories = %v, %v", matches, err)
	}
}

func TestWriteArtifactsPreservesExistingOutputAndRejectsDuplicatePaths(t *testing.T) {
	directory := t.TempDir()
	existing := filepath.Join(directory, "existing")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(existing, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeArtifacts(existing, []generator.Artifact{{Path: "client.ts", Data: []byte("export {}\n")}})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing output error = %v", err)
	}
	if value, err := os.ReadFile(sentinel); err != nil || string(value) != "keep" {
		t.Fatalf("sentinel = %q, %v", value, err)
	}

	output := filepath.Join(directory, "duplicate")
	err = writeArtifacts(output, []generator.Artifact{
		{Path: "client.ts", Data: []byte("first\n")},
		{Path: "./client.ts", Data: []byte("second\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate generated artifact") {
		t.Fatalf("duplicate artifact error = %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("duplicate output stat error = %v", err)
	}
}

const minimalDocument = `{
  "openapi": "3.2.0",
  "info": { "title": "Example API", "version": "1.2.3" },
  "paths": {
    "/health": {
      "get": {
        "operationId": "getHealth",
        "responses": {
          "200": {
            "description": "Healthy",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "required": ["status"],
                  "properties": { "status": { "type": "string" } }
                }
              }
            }
          }
        }
      }
    }
  }
}`
