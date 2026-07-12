package typescript

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	sdkgen "github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/generator"
)

func TestVersionedTargetRuntimeParity(t *testing.T) {
	for _, test := range []struct {
		name         string
		document     string
		operationID  string
		input        string
		method       string
		responseBody string
	}{
		{
			name:        "OAS 3.0 query request",
			document:    `{"openapi":"3.0.3","info":{"title":"V30","version":"1"},"paths":{"/widgets":{"get":{"operationId":"listWidgets","parameters":[{"name":"limit","in":"query","schema":{"type":"integer"}}],"responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"type":"object","properties":{"id":{"type":"string"}}}}}}}}}}}`,
			operationID: "listWidgets", input: `{"query":{"limit":2}}`, method: "GET", responseBody: `{"id":"widget-1"}`,
		},
		{
			name:        "OAS 3.1 response decoding",
			document:    `{"openapi":"3.1.1","info":{"title":"V31","version":"1"},"paths":{"/widget":{"get":{"operationId":"getWidget","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"type":"object","properties":{"id":{"type":"string"}}}}}}}}}}}`,
			operationID: "getWidget", input: `null`, method: "GET", responseBody: `{"id":"widget-1"}`,
		},
		{
			name:        "OAS 3.2 query method",
			document:    `{"openapi":"3.2.0","info":{"title":"V32","version":"1"},"paths":{"/widgets":{"query":{"operationId":"queryWidgets","responses":{"204":{"description":"No Content"}}},"additionalOperations":{"PURGE":{"operationId":"purgeWidgets","responses":{"204":{"description":"No Content"}}}}}}}`,
			operationID: "queryWidgets", input: `null`, method: "QUERY", responseBody: `null`,
		},
		{
			name:        "OAS 3.2 additional operation",
			document:    `{"openapi":"3.2.0","info":{"title":"V32","version":"1"},"paths":{"/widgets":{"query":{"operationId":"queryWidgets","responses":{"204":{"description":"No Content"}}},"additionalOperations":{"PURGE":{"operationId":"purgeWidgets","responses":{"204":{"description":"No Content"}}}}}}}`,
			operationID: "purgeWidgets", input: `null`, method: "PURGE", responseBody: `null`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := sdkgen.Compile([]byte(test.document))
			if err != nil {
				t.Fatal(err)
			}
			runTargetRuntimeParity(t, document, test.operationID, test.input, test.method, test.responseBody)
		})
	}
}

func TestTargetsDecodeSchemaLessResponseMediaAsUnknownValues(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Unknown response","version":"1"},
  "paths":{"/value":{"get":{"operationId":"getValue","responses":{"200":{"description":"OK","content":{"application/json":{}}}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	runTargetRuntimeParity(t, document, "getValue", "null", "GET", `{"value":"decoded"}`)
}

func TestTargetsRejectMissingRequiredRuntimeInputsBeforeFetch(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Required","version":"1"},
  "paths":{"/widgets":{"post":{"operationId":"createWidget","parameters":[{"name":"limit","in":"query","required":true,"schema":{"type":"integer"}}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object"}}}},"responses":{"204":{"description":"No Content"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	typescriptArtifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	javascriptArtifacts, err := JavaScriptSourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	typescriptSource := filepath.Join(directory, "typescript-source")
	writeTargetArtifacts(t, typescriptSource, typescriptArtifacts)
	if err := os.WriteFile(filepath.Join(typescriptSource, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(typescriptSource, "tsconfig.json"), []byte(parityTSConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join("..", "..", "..", "test", "typescript", "node_modules", "typescript", "lib", "tsc.js")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("TypeScript compiler unavailable for runtime parity test: %v", err)
	}
	if output, err := exec.Command("node", tsc, "--project", filepath.Join(typescriptSource, "tsconfig.json")).CombinedOutput(); err != nil {
		t.Fatalf("compile generated TypeScript target: %v\n%s", err, output)
	}
	typescriptOutput := filepath.Join(directory, "typescript-output")
	if err := os.WriteFile(filepath.Join(typescriptOutput, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	javascriptOutput := filepath.Join(directory, "javascript-output")
	writeTargetArtifacts(t, javascriptOutput, javascriptArtifacts)
	script := `
import { pathToFileURL } from "node:url";
const [tsPath, jsPath] = process.argv.slice(1);
for (const path of [tsPath, jsPath]) {
  const { createClient } = await import(pathToFileURL(path).href);
  let fetched = false;
  const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { fetched = true; throw new Error("fetch must not run"); } });
  try { await api.$operations.createWidget({}); throw new Error("missing required input accepted"); }
  catch (error) {
    if (!String(error).includes("Missing required query parameter limit") && !String(error.cause).includes("Missing required query parameter limit")) throw error;
    if (fetched) throw new Error("fetch ran before required-input validation");
  }
}
`
	command := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(typescriptOutput, "index.js"), filepath.Join(javascriptOutput, "index.js"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("execute required-input runtime parity test: %v\n%s", err, output)
	}
}

func runTargetRuntimeParity(t *testing.T, document *ir.Document, operationID, input, method, responseBody string) {
	t.Helper()
	typescriptArtifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	javascriptArtifacts, err := JavaScriptSourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	typescriptSource := filepath.Join(directory, "typescript-source")
	writeTargetArtifacts(t, typescriptSource, typescriptArtifacts)
	if err := os.WriteFile(filepath.Join(typescriptSource, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(typescriptSource, "tsconfig.json"), []byte(parityTSConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join("..", "..", "..", "test", "typescript", "node_modules", "typescript", "lib", "tsc.js")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("TypeScript compiler unavailable for runtime parity test: %v", err)
	}
	if output, err := exec.Command("node", tsc, "--project", filepath.Join(typescriptSource, "tsconfig.json")).CombinedOutput(); err != nil {
		t.Fatalf("compile generated TypeScript target: %v\n%s", err, output)
	}
	typescriptOutput := filepath.Join(directory, "typescript-output")
	if err := os.WriteFile(filepath.Join(typescriptOutput, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	javascriptOutput := filepath.Join(directory, "javascript-output")
	writeTargetArtifacts(t, javascriptOutput, javascriptArtifacts)
	script := `
import { pathToFileURL } from "node:url";
const [tsPath, jsPath, operationID, inputJSON, method, responseBody] = process.argv.slice(1);
const ts = await import(pathToFileURL(tsPath).href);
const js = await import(pathToFileURL(jsPath).href);
async function invoke(createClient) {
  const requests = [];
  const api = createClient({ baseURL: "https://api.example.test/v1", fetch: async (url, init) => {
    requests.push({ method: init.method, url: String(url), body: typeof init.body === "string" ? init.body : String(init.body) });
    return new Response(responseBody === "null" ? null : responseBody, { status: responseBody === "null" ? 204 : 200, headers: { "content-type": "application/json" } });
  }});
  const input = JSON.parse(inputJSON);
  const output = input === null ? await api.$operations[operationID]() : await api.$operations[operationID](input);
  return { output, requests };
}
const left = await invoke(ts.createClient); const right = await invoke(js.createClient);
if (JSON.stringify(left) !== JSON.stringify(right)) throw new Error(JSON.stringify({ left, right }, null, 2));
if (left.requests[0].method !== method) throw new Error("method mismatch: " + left.requests[0].method);
`
	command := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(typescriptOutput, "index.js"), filepath.Join(javascriptOutput, "index.js"), operationID, input, method, responseBody)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("execute target runtime parity: %v\n%s", err, output)
	}
}

const parityTSConfig = `{
  "compilerOptions": {"target":"ES2022","module":"NodeNext","moduleResolution":"NodeNext","strict":true,"skipLibCheck":true,"rootDir":".","outDir":"../typescript-output"},
  "include": ["**/*.ts"]
}`

func TestTypeScriptAndJavaScriptTargetsHaveEquivalentRuntimeTransport(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.2.0",
  "info": {"title": "Parity", "version": "1"},
  "paths": {
    "/records/{recordID}": {
      "post": {
        "operationId": "updateRecord",
        "parameters": [
          {"name": "recordID", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "tag", "in": "query", "style": "form", "explode": true, "schema": {"type": "array", "items": {"type": "string"}}},
          {"name": "X-Trace", "in": "header", "schema": {"type": "string"}},
          {"name": "session", "in": "cookie", "schema": {"type": "string"}}
        ],
        "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "properties": {"displayName": {"type": "string"}}}}}},
        "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": {"type": "object", "properties": {"requestId": {"type": "string"}}}}}}}
      }
    }
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	typescriptArtifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	javascriptArtifacts, err := JavaScriptSourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()
	typescriptSource := filepath.Join(directory, "typescript-source")
	writeTargetArtifacts(t, typescriptSource, typescriptArtifacts)
	if err := os.WriteFile(filepath.Join(typescriptSource, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	config := `{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "strict": true,
    "skipLibCheck": true,
    "rootDir": ".",
    "outDir": "../typescript-output"
  },
  "include": ["**/*.ts"]
}`
	if err := os.WriteFile(filepath.Join(typescriptSource, "tsconfig.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join("..", "..", "..", "test", "typescript", "node_modules", "typescript", "lib", "tsc.js")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("TypeScript compiler unavailable for runtime parity test: %v", err)
	}
	command := exec.Command("node", tsc, "--project", filepath.Join(typescriptSource, "tsconfig.json"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compile generated TypeScript target: %v\n%s", err, output)
	}
	typescriptOutput := filepath.Join(directory, "typescript-output")
	if err := os.WriteFile(filepath.Join(typescriptOutput, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	javascriptOutput := filepath.Join(directory, "javascript-output")
	writeTargetArtifacts(t, javascriptOutput, javascriptArtifacts)
	script := `
import { pathToFileURL } from "node:url";
const [tsPath, jsPath] = process.argv.slice(1);
const ts = await import(pathToFileURL(tsPath).href);
const js = await import(pathToFileURL(jsPath).href);
async function invoke(createClient) {
  const requests = [];
  const api = createClient({
    baseURL: "https://api.example.test/v1",
    fetch: async (url, init) => {
      requests.push({
        method: init.method,
        url: String(url),
        headers: Object.fromEntries(new Headers(init.headers)),
        body: typeof init.body === "string" ? init.body : String(init.body),
      });
      return new Response(JSON.stringify({ requestId: "request-1" }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    },
  });
  const output = await api.$operations.updateRecord({
    path: { recordID: "record one" },
    query: { tag: ["one", "two"] },
    headerParams: { xTrace: "trace-1" },
    cookieParams: { session: "cookie-1" },
    body: { displayName: "Record One" },
  });
  return { output, requests };
}
const left = await invoke(ts.createClient);
const right = await invoke(js.createClient);
if (JSON.stringify(left) !== JSON.stringify(right)) {
  throw new Error("target transport differs:\n" + JSON.stringify({ left, right }, null, 2));
}
if (left.output.requestID !== "request-1") throw new Error("response did not decode");
`
	command = exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(typescriptOutput, "index.js"), filepath.Join(javascriptOutput, "index.js"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("execute target runtime parity test: %v\n%s", err, output)
	}
}

func writeTargetArtifacts(t *testing.T, directory string, artifacts []generator.Artifact) {
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
