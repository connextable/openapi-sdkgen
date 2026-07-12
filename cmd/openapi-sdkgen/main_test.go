package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateWritesTypeScriptArtifacts(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "contract.json")
	if err := os.WriteFile(input, []byte(minimalDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "generated-client")
	if err := run([]string{"generate", "--input", input, "--target", "typescript", "--output", output, "--package-name", "@example/client"}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"manifest.json", "README.md", "generated/client.ts", "generated/runtime.ts"} {
		if _, err := os.Stat(filepath.Join(output, expected)); err != nil {
			t.Fatalf("missing %s: %v", expected, err)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(output, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), `"packageName": "@example/client"`) {
		t.Fatalf("manifest.json = %s", manifest)
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
