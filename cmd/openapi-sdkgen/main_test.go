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

func TestGenerateRejectsUnknownTarget(t *testing.T) {
	err := run([]string{"generate", "--input", "contract.json", "--target", "kotlin", "--output", "out"})
	if err == nil || !strings.Contains(err.Error(), "unsupported SDK target") {
		t.Fatalf("error = %v", err)
	}
}

func TestSafeArtifactPathRejectsTraversal(t *testing.T) {
	if _, err := safeArtifactPath("../outside.ts"); err == nil {
		t.Fatal("traversal path was accepted")
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
