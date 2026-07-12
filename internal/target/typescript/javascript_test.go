package typescript

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sdkgen "github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/generator"
)

func TestJavaScriptSourceArtifactsAreNativeESMAndUseSharedOperationRuntime(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.1",
  "info": {"title": "JavaScript", "version": "1"},
  "paths": {"/widgets": {"get": {"operationId": "getWidgets", "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": {"type": "object", "properties": {"id": {"type": "string"}}}}}}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := JavaScriptSourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 4 {
		t.Fatalf("artifact count = %d, want 4", len(artifacts))
	}
	for _, artifact := range artifacts {
		if strings.HasSuffix(artifact.Path, ".ts") {
			t.Fatalf("JavaScript target emitted TypeScript artifact %s", artifact.Path)
		}
		if !strings.HasSuffix(artifact.Path, ".js") {
			t.Fatalf("JavaScript target emitted non-JavaScript artifact %s", artifact.Path)
		}
	}
	client := string(artifactByPath(t, artifacts, "generated/client.js"))
	if !strings.Contains(client, `"getWidgets": bindOperation`) || strings.Contains(client, "WireSchemas") {
		t.Fatalf("invalid JavaScript client source:\n%s", client)
	}
	metadata := string(artifactByPath(t, artifacts, "generated/metadata.js"))
	if !strings.Contains(metadata, `export const openapiVersion = "3.1.1"`) {
		t.Fatalf("JavaScript metadata missing version:\n%s", metadata)
	}

	directory := t.TempDir()
	writeJavaScriptArtifacts(t, directory, artifacts)
	index := filepath.Join(directory, "index.js")
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({
  baseURL: "https://api.example.test",
  fetch: async () => new Response(JSON.stringify({ id: "widget-1" }), {
    status: 200,
    headers: { "content-type": "application/json" },
  }),
});
const value = await api.$operations.getWidgets();
if (value.id !== "widget-1") throw new Error("unexpected result: " + JSON.stringify(value));
`
	command := exec.Command("node", "--input-type=module", "--eval", script, index)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated JavaScript ESM execution failed: %v\n%s", err, output)
	}
}

func TestJavaScriptGeneratorRegistersAsJavaScriptTarget(t *testing.T) {
	if got := (JavaScriptGenerator{}).Name(); got != "javascript" {
		t.Fatalf("target name = %q", got)
	}
}

func TestJavaScriptTargetNamesItsCapabilityDiagnostics(t *testing.T) {
	_, err := JavaScriptSourceArtifacts(&ir.Document{ComponentSchemas: map[string]map[string]any{
		"Dynamic": {"$dynamicRef": "#node"},
	}})
	if err == nil || !strings.Contains(err.Error(), "JavaScript target") {
		t.Fatalf("error = %v", err)
	}
}

func writeJavaScriptArtifacts(t *testing.T, directory string, artifacts []generator.Artifact) {
	t.Helper()
	for _, artifact := range artifacts {
		path := filepath.Join(directory, artifact.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, artifact.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
