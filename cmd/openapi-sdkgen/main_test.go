package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/generator"
)

func TestGenerateWritesSelfContainedTypeScriptPackage(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "contract.json")
	if err := os.WriteFile(input, []byte(minimalDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "generated-client")
	if err := run([]string{"generate", "--input", input, "--target", "typescript", "--output", output, "--package-name", "@example/client"}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"package.json", "tsconfig.json", "src/index.ts", "manifest.json", "README.md", "generated/client.ts", "generated/runtime.ts"} {
		if _, err := os.Stat(filepath.Join(output, expected)); err != nil {
			t.Fatalf("missing %s: %v", expected, err)
		}
	}
	packageJSON, err := os.ReadFile(filepath.Join(output, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(packageJSON), `"name": "@example/client"`) || !strings.Contains(string(packageJSON), `"build": "tsc --project tsconfig.json"`) || strings.Contains(string(packageJSON), `"dependencies"`) {
		t.Fatalf("package.json = %s", packageJSON)
	}
}

func TestGenerateUsesNormalizedOutputDirectoryNameByDefault(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "contract.json")
	if err := os.WriteFile(input, []byte(minimalDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "Example API SDK")
	if err := run([]string{"generate", "--input", input, "--target", "typescript", "--output", output}); err != nil {
		t.Fatal(err)
	}
	packageJSON, err := os.ReadFile(filepath.Join(output, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(packageJSON), `"name": "example-api-sdk"`) {
		t.Fatalf("package.json = %s", packageJSON)
	}
}

func TestGenerateRejectsUnknownTarget(t *testing.T) {
	err := run([]string{"generate", "--input", "contract.json", "--target", "kotlin", "--output", "out"})
	if err == nil || !strings.Contains(err.Error(), "unsupported SDK target") {
		t.Fatalf("error = %v", err)
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

func TestDefaultPackageNameNormalizesAndFallsBack(t *testing.T) {
	for input, want := range map[string]string{
		"Example API SDK": "example-api-sdk",
		"💥":               "openapi-sdk",
		"._client_.":      "client",
	} {
		if value := defaultPackageName(input); value != want {
			t.Errorf("defaultPackageName(%q) = %q, want %q", input, value, want)
		}
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
