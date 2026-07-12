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
	if len(artifacts) != 11 {
		t.Fatalf("artifact count = %d, want 11", len(artifacts))
	}
	for _, artifact := range artifacts {
		if strings.HasSuffix(artifact.Path, ".ts") && !strings.HasSuffix(artifact.Path, ".d.ts") {
			t.Fatalf("JavaScript target emitted TypeScript artifact %s", artifact.Path)
		}
		if !strings.HasSuffix(artifact.Path, ".js") && !strings.HasSuffix(artifact.Path, ".d.ts") {
			t.Fatalf("JavaScript target emitted unsupported artifact %s", artifact.Path)
		}
	}
	for _, path := range []string{"index.d.ts", "metadata.d.ts", "generated/client.d.ts", "generated/errors.d.ts", "generated/runtime.d.ts", "generated/types.d.ts"} {
		artifactByPath(t, artifacts, path)
	}
	client := string(artifactByPath(t, artifacts, "generated/client.js"))
	if !strings.Contains(client, `"getWidgets": _sdkgenOperationGetWidgets`) || !strings.Contains(client, "const _sdkgenResource_widgets =") || strings.Contains(client, "WireSchemas") {
		t.Fatalf("invalid JavaScript client source:\n%s", client)
	}
	metadata := string(artifactByPath(t, artifacts, "metadata.js"))
	if !strings.Contains(metadata, `export const openapi = Object.freeze({ document:`) || !strings.Contains(metadata, `version: "3.1.1"`) {
		t.Fatalf("JavaScript metadata missing version:\n%s", metadata)
	}
	indexSource := string(artifactByPath(t, artifacts, "index.js"))
	if strings.Contains(indexSource, "metadata.js") || !strings.Contains(indexSource, "errors.js") {
		t.Fatalf("JavaScript root export boundary changed:\n%s", indexSource)
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
const resourceValue = await api.widgets.get();
if (resourceValue.id !== "widget-1") throw new Error("resource API did not match operation API");
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

func TestJavaScriptResourceLocalsCannotCollideWithGeneratedBindings(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.1",
  "info": {"title": "Collisions", "version": "1"},
  "paths": {
    "/request": {"get": {"operationId": "request", "responses": {"204": {"description": "OK"}}}},
    "/id": {"get": {"operationId": "getID", "responses": {"204": {"description": "OK"}}}},
    "/i-d": {"get": {"operationId": "getIDDashed", "responses": {"204": {"description": "OK"}}}},
    "/operation-get-widgets": {"get": {"operationId": "getWidgets", "responses": {"204": {"description": "OK"}}}},
    "/paginate-get-widgets": {"get": {"operationId": "listWidgets", "responses": {"204": {"description": "OK"}}, "x-pagination": "cursor"}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := JavaScriptSourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	writeJavaScriptArtifacts(t, directory, artifacts)
	if err := os.WriteFile(filepath.Join(directory, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `import { pathToFileURL } from "node:url"; await import(pathToFileURL(process.argv[1]).href);`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(directory, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("generated JavaScript local collision: %v\n%s", err, output)
	}
}

func TestJavaScriptErrorsModuleExportsGeneratedGuards(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.1",
  "info": {"title": "Errors", "version": "1"},
  "paths": {"/widgets": {"get": {"operationId": "getWidgets", "responses": {"400": {"description": "Bad", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Problem"}}}}}}}},
  "components": {"schemas": {"Problem": {"type": "object", "x-error-category": "validation", "properties": {"error": {"type": "object", "properties": {"code": {"const": "validation_failed"}}}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := JavaScriptSourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	writeJavaScriptArtifacts(t, directory, artifacts)
	if err := os.WriteFile(filepath.Join(directory, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
import { pathToFileURL } from "node:url";
const errors = await import(pathToFileURL(process.argv[1]).href);
const runtime = await import(pathToFileURL(process.argv[2]).href);
const matching = new runtime.APIError({ code: "validation_failed", message: "bad" });
const other = new runtime.APIError({ code: "other", message: "other" });
if (!errors.isValidationFailedError(matching) || errors.isValidationFailedError(other) || !errors.isValidationError(matching) || errors.isValidationError(other)) throw new Error("generated error guards are incorrect");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(directory, "generated", "errors.js"), filepath.Join(directory, "generated", "runtime.js")).CombinedOutput(); err != nil {
		t.Fatalf("generated JavaScript errors module failed: %v\n%s", err, output)
	}
}

func TestJavaScriptDeclarationSidecarsTypecheckResourceAndOperationCalls(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.1",
  "info": {"title": "JavaScript declarations", "version": "1"},
  "paths": {"/todos": {"post": {
    "operationId": "createTodo",
    "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TodoInput"}}}},
    "responses": {"201": {"description": "Created", "content": {"application/json": {"schema": {"type": "object", "required": ["id"], "properties": {"id": {"type": "string"}}}}}}}
  }}},
  "components": {"schemas": {
    "TodoInput": {"type": "object", "required": ["title", "state"], "properties": {"title": {"type": "string", "default": "}"}, "state": {"$ref": "#/components/schemas/Status"}}},
    "Status": {"type": "string", "enum": ["open", "a}"]}
  }}
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := JavaScriptSourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	writeJavaScriptArtifacts(t, directory, artifacts)
	if err := os.WriteFile(filepath.Join(directory, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "tsconfig.json"), []byte(`{"compilerOptions":{"target":"ES2022","module":"NodeNext","moduleResolution":"NodeNext","strict":true,"allowJs":true,"checkJs":true,"noEmit":true,"skipLibCheck":false},"include":["consumer.js"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := `// @ts-check
import { createClient } from "./index.js";
const api = createClient({ baseURL: "https://api.example.test" });
void api.todos.create({ body: { title: "valid", state: "open" } });
void api.$operations.createTodo({ body: { title: "valid", state: "open" } });
`
	if err := os.WriteFile(filepath.Join(directory, "consumer.js"), []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join("..", "..", "..", "test", "typescript", "node_modules", "typescript", "lib", "tsc.js")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("TypeScript compiler unavailable for JavaScript declaration test: %v", err)
	}
	if output, err := exec.Command("node", tsc, "--project", filepath.Join(directory, "tsconfig.json")).CombinedOutput(); err != nil {
		t.Fatalf("valid JavaScript consumer did not typecheck: %v\n%s", err, output)
	}
	invalid := valid + `void api.todos.create({ body: { title: 123 } });
`
	if err := os.WriteFile(filepath.Join(directory, "consumer.js"), []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("node", tsc, "--project", filepath.Join(directory, "tsconfig.json")).CombinedOutput(); err == nil || !strings.Contains(string(output), "not assignable to type 'string'") {
		t.Fatalf("invalid JavaScript consumer was not rejected: %v\n%s", err, output)
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
