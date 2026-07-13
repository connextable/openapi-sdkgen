package typescript

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sdkgen "github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/generator"
)

func TestVersionedTypeScriptRuntime(t *testing.T) {
	for _, test := range []struct {
		name, document, operationID, input, method, responseBody string
	}{
		{"OAS 3.0 query", `{"openapi":"3.0.3","info":{"title":"V30","version":"1"},"paths":{"/widgets":{"get":{"operationId":"listWidgets","parameters":[{"name":"limit","in":"query","schema":{"type":"integer"}}],"responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"type":"object","properties":{"id":{"type":"string"}}}}}}}}}}}`, "listWidgets", `{"query":{"limit":2}}`, "GET", `{"id":"widget-1"}`},
		{"OAS 3.1 response", `{"openapi":"3.1.1","info":{"title":"V31","version":"1"},"paths":{"/widget":{"get":{"operationId":"getWidget","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"type":"object","properties":{"id":{"type":"string"}}}}}}}}}}}`, "getWidget", "null", "GET", `{"id":"widget-1"}`},
		{"OAS 3.2 query", `{"openapi":"3.2.0","info":{"title":"V32","version":"1"},"paths":{"/widgets":{"query":{"operationId":"queryWidgets","responses":{"204":{"description":"No Content"}}}}}}`, "queryWidgets", "null", "QUERY", "null"},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := sdkgen.Compile([]byte(test.document))
			if err != nil {
				t.Fatal(err)
			}
			runTypeScriptRuntime(t, document, test.operationID, test.input, test.method, test.responseBody)
		})
	}
}

func TestTargetsRejectMissingRequiredRuntimeInputsBeforeFetch(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.2.0","info":{"title":"Required","version":"1"},"paths":{"/widgets":{"post":{"operationId":"createWidget","parameters":[{"name":"limit","in":"query","required":true,"schema":{"type":"integer"}}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object"}}}},"responses":{"204":{"description":"No Content"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let fetched = false;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { fetched = true; throw new Error("fetch must not run"); } });
try { await api.$operations.createWidget({}); throw new Error("missing required input accepted"); }
catch (error) {
  if (!String(error).includes("Missing required query parameter limit") && !String(error.cause).includes("Missing required query parameter limit")) throw error;
  if (fetched) throw new Error("fetch ran before required-input validation");
}
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript required-input runtime test: %v\n%s", err, output)
	}
}

func TestClosedObjectRuntimeRejectsUnexpectedRequestProperties(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Closed","version":"1"},"paths":{"/closed":{"post":{"operationId":"createClosed","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Closed"}}}},"responses":{"204":{"description":"No Content"}}}}},"components":{"schemas":{"Closed":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}},"additionalProperties":false}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let fetched = false;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { fetched = true; throw new Error("fetch must not run"); } });
try { await api.$operations.createClosed({ body: { id: "one", extra: true } }); throw new Error("closed object accepted extra property"); }
catch (error) {
  if (!String(error).includes("unexpected property extra") && !String(error.cause).includes("unexpected property extra")) throw error;
  if (fetched) throw new Error("fetch ran before closed-object validation");
}

`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript closed-object runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeAppliesFormatOnlyWhenFormatAssertionVocabularyIsRequired(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.1", "info":{"title":"Formats","version":"1"},
  "paths":{"/users":{"post":{"operationId":"createUser","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/AssertedUser"}}}},"responses":{"204":{"description":"No Content"}}}}},
  "components":{"schemas":{"AssertedUser":{"$vocabulary":{"https://json-schema.org/draft/2020-12/vocab/format-assertion":true},"type":"object","required":["email","id"],"properties":{"email":{"type":"string","format":"email"},"id":{"type":"string","format":"uuid"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let calls = 0;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { calls++; return new Response(null, { status: 204 }); } });
try { await api.$operations.createUser({ body: { email: "not-an-email", id: "not-a-uuid" } }); throw new Error("format assertion accepted invalid input"); }
catch (error) { if (!String(error).includes("must match format") && !String(error.cause).includes("must match format")) throw error; }
if (calls !== 0) throw new Error("format-invalid input reached fetch");
await api.$operations.createUser({ body: { email: "person@example.test", id: "123e4567-e89b-42d3-a456-426614174000" } });
if (calls !== 1) throw new Error("format-valid input did not reach fetch");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript format-assertion runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeAppliesOpenAPI30ExclusiveBoundsAndRejectsNonNullableNull(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.0.3", "info":{"title":"Bounds","version":"1"},
  "paths":{"/limits":{"post":{"operationId":"setLimit","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["limit"],"properties":{"limit":{"type":"number","maximum":5,"exclusiveMaximum":true}}}}}},"responses":{"204":{"description":"No Content"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let calls = 0;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { calls++; return new Response(null, { status: 204 }); } });
for (const body of [{ limit: 5 }, { limit: null }]) {
  try { await api.$operations.setLimit({ body }); throw new Error("invalid OpenAPI 3.0 bound/null input accepted"); }
  catch (error) { if (!String(error).includes("must be < 5") && !String(error).includes("expected number") && !String(error.cause).includes("must be < 5") && !String(error.cause).includes("expected number")) throw error; }
}
if (calls !== 0) throw new Error("invalid OpenAPI 3.0 input reached fetch");
await api.$operations.setLimit({ body: { limit: 4 } });
if (calls !== 1) throw new Error("valid OpenAPI 3.0 bound did not reach fetch");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript OpenAPI 3.0 bound runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeSupportsStandardFormatAssertionRegistry(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.1","info":{"title":"Formats","version":"1"},"paths":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { validateWireValue } = await import(pathToFileURL(process.argv[1]).href);
const valid = {
  "date-time": "2024-02-29T23:59:59Z", date: "2024-02-29", time: "23:59:59+09:00", duration: "P3Y6M4DT12H30M5S",
  email: "person@example.test", "idn-email": "사용자@예제.테스트", hostname: "api.example.test", "idn-hostname": "例え.テスト",
  ipv4: "192.0.2.1", ipv6: "2001:db8::1", uri: "https://example.test/a?b=c", "uri-reference": "/a/b?c=d", iri: "https://例え.テスト/✓", "iri-reference": "경로/✓",
  uuid: "00000000-0000-0000-0000-000000000000", "uri-template": "https://example.test/{id}{?page}", "json-pointer": "/a~1b/0", "relative-json-pointer": "1/a", regex: "^[a-z]+$",
};
for (const [format, value] of Object.entries(valid)) validateWireValue(value, { types: ["string"], format, formatAssertion: true }, {}, "decode");
const invalid = { "date-time": "2024-02-30T25:00:00Z", date: "2024-02-30", time: "24:00:00Z", duration: "P", email: "not-an-email", "idn-email": "missing-at", hostname: "-bad.example", "idn-hostname": "bad host", ipv4: "999.0.0.1", ipv6: "not-an-ip", uri: "/relative", "uri-reference": "bad space", iri: "/relative", "iri-reference": "bad space", uuid: "nope", "uri-template": "{broken", "json-pointer": "/bad~2", "relative-json-pointer": "01", regex: "[" };
for (const [format, value] of Object.entries(invalid)) {
  try { validateWireValue(value, { types: ["string"], format, formatAssertion: true }, {}, "decode"); throw new Error("invalid " + format + " accepted"); }
  catch (error) { if (String(error).includes("accepted")) throw error; }
}
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "generated", "runtime.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript format registry runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeResolvesDynamicReferencesAgainstTheOuterDynamicScope(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0", "info":{"title":"Dynamic","version":"1"},
  "paths":{"/tree":{"post":{"operationId":"createTree","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/StrictTree"}}}},"responses":{"204":{"description":"No Content"}}}}},
  "components":{"schemas":{
    "BaseTree":{"$id":"https://schemas.example.test/base-tree","$dynamicAnchor":"node","type":"object","properties":{"child":{"$dynamicRef":"#node"}}},
    "StrictTree":{"$id":"https://schemas.example.test/strict-tree","$dynamicAnchor":"node","allOf":[{"$ref":"#/components/schemas/BaseTree"},{"type":"object","required":["strict"],"properties":{"strict":{"const":true}}}]}
  }}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let fetched = false;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { fetched = true; throw new Error("fetch must not run"); } });
try { await api.$operations.createTree({ body: { strict: true, child: {} } }); throw new Error("dynamic reference used its static fallback"); }
catch (error) {
  if (!String(error).includes("missing required property strict") && !String(error.cause).includes("missing required property strict")) throw error;
  if (fetched) throw new Error("fetch ran before dynamic-reference validation");
}
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript dynamic-reference runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeResolvesDynamicReferencesAcrossContainedSchemaResources(t *testing.T) {
	for _, version := range []string{"3.1.1", "3.2.0"} {
		t.Run(version, func(t *testing.T) {
			directory := t.TempDir()
			input := filepath.Join(directory, "openapi.json")
			external := filepath.Join(directory, "schemas.json")
			if err := os.WriteFile(external, []byte(`{
  "BaseTree": {
    "$id": "https://schemas.example.test/base-tree",
    "$dynamicAnchor": "node",
    "type": "object",
    "properties": {"child": {"$dynamicRef": "#node"}}
  }
}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(input, []byte(`{
  "openapi": "`+version+`", "info": {"title": "External dynamic", "version": "1"},
  "paths": {"/tree": {"post": {"operationId": "createTree", "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/StrictTree"}}}}, "responses": {"204": {"description": "OK"}}}}},
  "components": {"schemas": {
    "BaseTree": {"$ref": "schemas.json#/BaseTree"},
    "StrictTree": {"$id": "https://schemas.example.test/strict-tree", "$dynamicAnchor": "node", "allOf": [
      {"$ref": "#/components/schemas/BaseTree"},
      {"type": "object", "required": ["strict"], "properties": {"strict": {"const": true}}}
    ]}
  }}
}`), 0o600); err != nil {
				t.Fatal(err)
			}
			document, err := sdkgen.CompileFile(input)
			if err != nil {
				t.Fatal(err)
			}
			output := compileTypeScriptArtifacts(t, document)
			script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let fetched = false;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { fetched = true; throw new Error("fetch must not run"); } });
try { await api.$operations.createTree({ body: { strict: true, child: {} } }); throw new Error("dynamic reference used its static fallback"); }
catch (error) {
  if (!String(error).includes("missing required property strict") && !String(error.cause).includes("missing required property strict")) throw error;
  if (fetched) throw new Error("fetch ran before dynamic-reference validation");
}
`
			if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
				t.Fatalf("execute TypeScript external dynamic-reference runtime test: %v\n%s", err, output)
			}
		})
	}
}

func TestRuntimeResolvesDynamicReferencesAcrossLockedRemoteSchemaResources(t *testing.T) {
	for _, version := range []string{"3.1.1", "3.2.0"} {
		t.Run(version, func(t *testing.T) {
			directory := t.TempDir()
			input := filepath.Join(directory, "openapi.json")
			remoteURL := "https://schemas.example.test/base-tree.json"
			remote := []byte(`{
  "BaseTree": {
    "$id": "https://schemas.example.test/base-tree",
    "$dynamicAnchor": "node",
    "type": "object",
    "properties": {"child": {"$dynamicRef": "#node"}}
  }
}`)
			digest := sha256.Sum256(remote)
			encodedDigest := hex.EncodeToString(digest[:])
			if err := os.Mkdir(filepath.Join(directory, ".openapi-sdkgen-cache"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, ".openapi-sdkgen-cache", encodedDigest), remote, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(input+".openapi-sdkgen.lock", []byte(`{"version":1,"references":{"`+remoteURL+`":"`+encodedDigest+`"}}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(input, []byte(`{
  "openapi": "`+version+`", "info": {"title": "Remote dynamic", "version": "1"},
  "paths": {"/tree": {"post": {"operationId": "createTree", "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/StrictTree"}}}}, "responses": {"204": {"description": "OK"}}}}},
  "components": {"schemas": {
    "BaseTree": {"$ref": "`+remoteURL+`#/BaseTree"},
    "StrictTree": {"$id": "https://schemas.example.test/strict-tree", "$dynamicAnchor": "node", "allOf": [
      {"$ref": "#/components/schemas/BaseTree"},
      {"type": "object", "required": ["strict"], "properties": {"strict": {"const": true}}}
    ]}
  }}
}`), 0o600); err != nil {
				t.Fatal(err)
			}
			document, err := sdkgen.CompileFileWithOptions(input, sdkgen.CompileOptions{RemoteRefAllowlist: []string{"https://schemas.example.test"}, Offline: true})
			if err != nil {
				t.Fatal(err)
			}
			output := compileTypeScriptArtifacts(t, document)
			script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let fetched = false;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { fetched = true; throw new Error("fetch must not run"); } });
try { await api.$operations.createTree({ body: { strict: true, child: {} } }); throw new Error("remote dynamic reference used its static fallback"); }
catch (error) {
  if (!String(error).includes("missing required property strict") && !String(error.cause).includes("missing required property strict")) throw error;
  if (fetched) throw new Error("fetch ran before remote dynamic-reference validation");
}
`
			if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
				t.Fatalf("execute TypeScript remote dynamic-reference runtime test: %v\n%s", err, output)
			}
		})
	}
}

func TestGeneratedResponseLinksFollowTypedTargetOperations(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0", "info":{"title":"Links","version":"1"},
  "paths":{
    "/orders/latest":{"get":{"operationId":"getLatestOrder","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}},"links":{"item":{"operationRef":"#/paths/~1items~1{itemID}/get","parameters":{"itemID":"$response.body#/id"},"server":{"url":"/links/{region}","variables":{"region":{"default":"eu west"}}}}}}}}},
    "/items/{itemID}":{"get":{"operationId":"getItem","parameters":[{"name":"itemID","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"type":"object","properties":{"id":{"type":"string"}}}}}}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const seen = [];
const api = createClient({ baseURL: "https://api.example.test", fetch: async (input) => {
  const url = new URL(String(input)); seen.push(url.pathname);
  if (url.pathname === "/orders/latest") { const response = new Response(JSON.stringify({ id: "item-1" }), { status: 200, headers: { "content-type": "application/json" } }); Object.defineProperty(response, "url", { value: "https://api.example.test/orders/latest" }); return response; }
  if (url.host === "api.example.test" && url.pathname === "/links/eu%20west/items/item-2") return new Response(JSON.stringify({ id: "item-2" }), { status: 200, headers: { "content-type": "application/json" } });
  throw new Error("unexpected path " + url.pathname);
} });
const source = await api.$operations.getLatestOrder.raw();
const item = await api.$links.getLatestOrder.item(source, { input: { path: { itemID: "item-2" } } });
if (item.id !== "item-2" || seen.join(",") !== "/orders/latest,/links/eu%20west/items/item-2") throw new Error("response link did not follow target operation");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript response-link runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedResponseLinksResolveRequestRuntimeExpressions(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0", "info":{"title":"Request links","version":"1"},
  "paths":{
    "/source":{"post":{"operationId":"createSource","parameters":[
      {"name":"page","in":"query","required":true,"schema":{"type":"integer"}},
      {"name":"x-source","in":"header","required":true,"schema":{"type":"string"}},
      {"name":"session","in":"cookie","required":true,"schema":{"type":"string"}}
    ],"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}}},"responses":{"200":{"description":"OK","links":{"follow":{"operationId":"getTarget","parameters":{"id":"$request.body#/id","page":"$request.query.page","trace":"$request.header.x-source","session":"$request.cookie.session"}}}}}}},
    "/target/{id}":{"get":{"operationId":"getTarget","parameters":[
      {"name":"id","in":"path","required":true,"schema":{"type":"string"}},
      {"name":"page","in":"query","required":true,"schema":{"type":"integer"}},
      {"name":"trace","in":"header","required":true,"schema":{"type":"string"}},
      {"name":"session","in":"cookie","required":true,"schema":{"type":"string"}}
    ],"responses":{"204":{"description":"OK"}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", transport: { capabilities: { cookieJar: true }, fetch: async (input, init) => {
  const url = new URL(String(input));
  const headers = new Headers(init?.headers);
  if (url.pathname === "/source") return new Response(null, { status: 200 });
  if (url.pathname !== "/target/source-id" || url.search !== "?page=3") throw new Error("request expressions produced unexpected URL " + url);
  if (headers.get("trace") !== "request-trace") throw new Error("request header expression was not resolved");
  if (headers.get("cookie") !== "session=session-id") throw new Error("request cookie expression was not resolved");
  return new Response(null, { status: 204 });
} } });
const sourceInput = { query: { page: 3 }, headerParams: { xSource: "request-trace" }, cookieParams: { session: "session-id" }, body: { id: "source-id" } };
const response = await api.$operations.createSource.raw(sourceInput);
await api.$links.createSource.follow(response, { sourceInput });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript request-expression Link runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedResponseLinksRejectUnknownRequestParameterExpressions(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0", "info":{"title":"Invalid request link","version":"1"},
  "paths":{
    "/source":{"get":{"operationId":"getSource","responses":{"200":{"description":"OK","links":{"follow":{"operationId":"getTarget","parameters":{"id":"$request.header.x-missing"}}}}}}},
    "/target/{id}":{"get":{"operationId":"getTarget","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"204":{"description":"OK"}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SourceArtifacts(document); err == nil || !strings.Contains(err.Error(), "unknown source header parameter") {
		t.Fatalf("Link with unknown request parameter expression error = %v", err)
	}
}

func TestGeneratedResponseLinksDispatchSameNameByStatus(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0", "info":{"title":"Status links","version":"1"},
  "paths":{
    "/choice":{"get":{"operationId":"getChoice","responses":{"200":{"description":"First","content":{"application/json":{"schema":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}},"links":{"next":{"operationId":"getFirst","parameters":{"id":"$response.body#/id"}}}},"201":{"description":"Second","content":{"application/json":{"schema":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}},"links":{"next":{"operationId":"getSecond","parameters":{"id":"$response.body#/id"}}}},"2XX":{"description":"Range","content":{"application/json":{"schema":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}},"links":{"next":{"operationId":"getRange","parameters":{"id":"$response.body#/id"}}}},"default":{"description":"Fallback","content":{"application/json":{"schema":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}},"links":{"next":{"operationId":"getFallback","parameters":{"id":"$response.body#/id"}}}}}}},
    "/first/{id}":{"get":{"operationId":"getFirst","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"204":{"description":"OK"}}}},
    "/second/{id}":{"get":{"operationId":"getSecond","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"204":{"description":"OK"}}}},
    "/range/{id}":{"get":{"operationId":"getRange","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"204":{"description":"OK"}}}},
    "/fallback/{id}":{"get":{"operationId":"getFallback","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"204":{"description":"OK"}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const seen = []; let choices = 0;
const api = createClient({ baseURL: "https://api.example.test", fetch: async (input) => {
  const url = new URL(String(input)); seen.push(url.pathname);
  if (url.pathname === "/choice") { choices++; return choices === 1 ? new Response('{"id":"two"}', { status: 201, headers: { "content-type": "application/json" } }) : choices === 2 ? new Response('{"id":"range"}', { status: 202, headers: { "content-type": "application/json" } }) : new Response('{"id":"fallback"}', { status: 500, headers: { "content-type": "application/json" } }); }
  if (url.pathname === "/second/two") return new Response(null, { status: 204 });
  if (url.pathname === "/range/range") return new Response(null, { status: 204 });
  if (url.pathname === "/fallback/fallback") return new Response(null, { status: 204 });
  throw new Error("unexpected link path " + url.pathname);
} });
const response = await api.$operations.getChoice.raw();
await api.$links.getChoice.next(response);
await api.$links.getChoice.next.byStatus.status201(response);
const range = await api.$operations.getChoice.raw(); await api.$links.getChoice.next(range); await api.$links.getChoice.next.byStatus.status2XX(range);
let fallback; try { await api.$operations.getChoice.raw(); throw new Error("fallback response did not fail"); } catch (error) { fallback = error; }
await api.$links.getChoice.next(fallback); await api.$links.getChoice.next.byStatus.statusDefault(fallback);
if (seen.join(",") !== "/choice,/second/two,/second/two,/choice,/range/range,/range/range,/choice,/fallback/fallback,/fallback/fallback") throw new Error("status-dependent Link dispatch mismatch: " + seen);
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript status-dependent link runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedStreamingRequestEncodesNDJSONItemsLazily(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Streaming request","version":"1"},
  "paths":{"/events":{"post":{"operationId":"publishEvents","requestBody":{"required":true,"content":{"application/x-ndjson":{"itemSchema":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}}},"responses":{"204":{"description":"Accepted"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let received = "";
const api = createClient({ baseURL: "https://api.example.test", fetch: async (_input, init) => {
  received = await new Response(init.body).text();
  if (new Headers(init.headers).get("content-type") !== "application/x-ndjson") throw new Error("stream content type missing");
  return new Response(null, { status: 204 });
} });
async function* events() { yield { id: "first" }; yield { id: "second" }; }
await api.$operations.publishEvents({ body: events() });
if (received !== "{\"id\":\"first\"}\n{\"id\":\"second\"}\n") throw new Error("stream request was not encoded as NDJSON: " + received);
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript streaming-request runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedPositionalMultipartRequestUsesDeclaredPartOrder(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Positional multipart","version":"1"},
  "paths":{"/bundle":{"post":{"operationId":"uploadBundle","requestBody":{"required":true,"content":{"multipart/mixed":{"schema":{"type":"array","prefixItems":[{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}},{"type":"string"}]},"prefixEncoding":[{"contentType":"application/json"}],"itemEncoding":{"contentType":"text/*","headers":{"x-part":{"required":true,"schema":{"type":"string"}}}}}}},"responses":{"204":{"description":"Accepted"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async (_input, init) => {
  const contentType = new Headers(init.headers).get("content-type") ?? "";
  const body = await new Response(init.body).text();
  if (!contentType.startsWith("multipart/mixed; boundary=")) throw new Error("missing mixed multipart boundary");
  if (!body.includes("Content-Type: application/json") || !body.includes('{"id":"first"}')) throw new Error("first positional part missing");
  if (!body.includes("Content-Type: text/plain") || !body.includes("x-part: second") || !body.includes("second")) throw new Error("second positional part missing");
  if (body.includes("Content-Disposition:")) throw new Error("unnamed multipart parts must not invent content disposition");
  return new Response(null, { status: 204 });
} });
await api.$operations.uploadBundle({ body: [{ id: "first" }, "second"] }, { multipartHeaders: { "1": { "x-part": "second" } }, multipartContentTypes: { "1": "text/plain" } });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript positional-multipart runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedStreamingMultipartRequestUsesItemEncoding(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Streaming multipart","version":"1"},
  "paths":{"/frames":{"post":{"operationId":"uploadFrames","requestBody":{"required":true,"content":{"multipart/mixed":{"itemSchema":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}},"itemEncoding":{"contentType":"application/json","headers":{"x-frame":{"required":true,"schema":{"type":"string"}}}}}}},"responses":{"204":{"description":"Accepted"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async (_input, init) => {
  const contentType = new Headers(init.headers).get("content-type") ?? "";
  const body = await new Response(init.body).text();
  if (!contentType.startsWith("multipart/mixed; boundary=")) throw new Error("missing streaming multipart boundary");
  if (!body.includes("Content-Type: application/json") || !body.includes("x-frame: frame-0") || !body.includes('{"id":"one"}')) throw new Error("first stream part missing");
  if (!body.includes("x-frame: frame-1") || !body.includes('{"id":"two"}')) throw new Error("second stream part missing");
  return new Response(null, { status: 204 });
} });
async function* frames() { yield { id: "one" }; yield { id: "two" }; }
await api.$operations.uploadFrames({ body: frames() }, { multipartHeaders: { "0": { "x-frame": "frame-0" }, "1": { "x-frame": "frame-1" } } });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript streaming-multipart runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedStructuredXMLParametersUseXMLWireSerialization(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0", "info":{"title":"XML parameters","version":"1"},
  "paths":{"/search":{"get":{"operationId":"findRecords","parameters":[
    {"name":"filter","in":"query","content":{"application/xml":{"schema":{"type":"object","properties":{"id":{"type":"string"}}}}}},
    {"name":"x-filter","in":"header","content":{"application/xml":{"schema":{"type":"object","properties":{"id":{"type":"string"}}}}}}
  ],"responses":{"204":{"description":"No Content"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async (input, init) => {
  const url = new URL(String(input));
  if (url.searchParams.get("filter") !== "<root><id>query</id></root>") throw new Error("XML query parameter was not serialized");
  if (new Headers(init.headers).get("x-filter") !== "<root><id>header</id></root>") throw new Error("XML header parameter was not serialized");
  return new Response(null, { status: 204 });
} });
await api.$operations.findRecords({ query: { filter: { id: "query" } }, headerParams: { xFilter: { id: "header" } } });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript XML-parameter runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeUsesAsyncCustomCodecsForParameterContent(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Parameter codecs","version":"1"},
  "paths":{"/records/{record}":{"get":{"operationId":"getRecord","parameters":[
    {"name":"record","in":"path","required":true,"content":{"application/cbor":{"schema":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}}},
    {"name":"filter","in":"query","content":{"application/cbor":{"schema":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}}},
    {"name":"whole","in":"querystring","content":{"application/cbor":{"schema":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}}},
    {"name":"X-Filter","in":"header","content":{"application/cbor":{"schema":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}}},
    {"name":"crumb","in":"cookie","content":{"application/cbor":{"schema":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}}}
  ],"responses":{"204":{"description":"No Content"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const codec = { encodeParameter: async (value) => "cbor:" + value.id };
const api = createClient({ baseURL: "https://api.example.test", codecs: { "application/cbor": codec }, transport: { capabilities: { cookieJar: true }, fetch: async (input, init) => {
  const url = new URL(String(input));
  if (url.pathname !== "/records/cbor%3Apath" || url.searchParams.get("filter") !== "cbor:query" || !url.search.includes("cbor%3Awhole")) throw new Error("custom path/query codec was not applied");
  const headers = new Headers(init.headers);
  if (headers.get("x-filter") !== "cbor:header" || headers.get("cookie") !== "crumb=cbor%3Acookie") throw new Error("custom header/cookie codec was not applied");
  return new Response(null, { status: 204 });
} } });
await api.$operations.getRecord({ path: { record: { id: "path" } }, query: { filter: { id: "query" }, whole: { id: "whole" } }, headerParams: { xFilter: { id: "header" } }, cookieParams: { crumb: { id: "cookie" } } });
let fetched = false;
const missing = createClient({ baseURL: "https://api.example.test", transport: { capabilities: { cookieJar: true }, fetch: async () => { fetched = true; throw new Error("fetch must not run"); } } });
try { await missing.$operations.getRecord({ path: { record: { id: "path" } } }); throw new Error("missing parameter codec was accepted"); }
catch (error) { if (!String(error).includes("missing parameter encode codec") && !String(error.cause).includes("missing parameter encode codec")) throw error; }
if (fetched) throw new Error("fetch ran without a required parameter codec");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript custom-parameter-codec runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeSelectsDeclaredRequestMediaRanges(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.2.0","info":{"title":"Request ranges","version":"1"},"paths":{"/message":{"post":{"operationId":"sendMessage","requestBody":{"required":true,"content":{"text/*":{"schema":{"type":"string"}}}},"responses":{"204":{"description":"No Content"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let calls = 0;
const api = createClient({ baseURL: "https://api.example.test", fetch: async (_input, init) => {
  calls++;
  if (new Headers(init.headers).get("content-type") !== "text/plain" || init.body !== "hello") throw new Error("request media range did not select text/plain");
  return new Response(null, { status: 204 });
} });
await api.$operations.sendMessage({ body: { contentType: "text/plain", value: "hello" } });
try { await api.$operations.sendMessage({ body: { contentType: "application/json", value: "bad" } }); throw new Error("undeclared request media was accepted"); }
catch (error) { if (!String(error).includes("not declared") && !String(error.cause).includes("not declared")) throw error; }
if (calls !== 1) throw new Error("fetch ran for undeclared request media");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript request-media-range runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedResponseStreamsDecodeNDJSONItemsLazily(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Streams","version":"1"},
  "paths":{"/logs":{"get":{"operationId":"tailLogs","responses":{"200":{"description":"OK","content":{"application/x-ndjson":{"itemSchema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}}}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const encoder = new TextEncoder();
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response(new ReadableStream({
  start(controller) { controller.enqueue(encoder.encode('{"event_id":"one"}\n{"event')); controller.enqueue(encoder.encode('_id":"two"}\n')); controller.close(); },
}), { status: 200, headers: { "content-type": "application/x-ndjson" } }) });
const events = [];
for await (const event of api.$streams.tailLogs()) events.push(event.eventID);
if (events.join(",") !== "one,two") throw new Error("NDJSON stream did not decode item schemas");
const oversized = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response('{"event_id":"too-long"}', { status: 200, headers: { "content-type": "application/x-ndjson" } }) });
try { for await (const _event of oversized.$streams.tailLogs({ maxStreamItemBytes: 4 })) { /* consume */ } throw new Error("oversized stream item was accepted"); }
catch (error) { if (!String(error).includes("exceeds 4 bytes") && !String(error.cause).includes("exceeds 4 bytes")) throw error; }
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript NDJSON stream runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedResponseStreamRawPreservesResponseBody(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Stream raw","version":"1"},
  "paths":{"/events":{"get":{"operationId":"listEvents","responses":{"200":{"description":"OK","content":{"application/x-ndjson":{"itemSchema":{"type":"object","properties":{"id":{"type":"string"}}}}}}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response('{"id":"one"}\n', { status: 200, headers: { "content-type": "application/x-ndjson" } }) });
const raw = await api.$operations.listEvents.raw();
if (raw.data !== undefined || await raw.response.text() !== '{"id":"one"}\n') throw new Error("stream raw response body was consumed");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript stream raw-response runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedResponseStreamDoesNotDispatchWhenAlreadyAborted(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Aborted stream","version":"1"},
  "paths":{"/events":{"get":{"operationId":"listEvents","responses":{"200":{"description":"OK","content":{"application/x-ndjson":{"itemSchema":{"type":"object","properties":{"id":{"type":"string"}}}}}}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient, isErrorCode, TransportErrorCode } = await import(pathToFileURL(process.argv[1]).href);
let calls = 0;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { calls++; throw new Error("fetch must not run"); } });
const controller = new AbortController(); controller.abort("stop");
try { await api.$streams.listEvents({ signal: controller.signal }).next(); throw new Error("aborted stream started"); }
catch (error) { if (!isErrorCode(error, TransportErrorCode.REQUEST_ABORTED)) throw error; }
if (calls !== 0) throw new Error("pre-aborted stream dispatched fetch");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript pre-aborted stream runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedResponseStreamClassifiesFetchFailureAsNetworkError(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Stream network error","version":"1"},
  "paths":{"/events":{"get":{"operationId":"listEvents","responses":{"200":{"description":"OK","content":{"application/x-ndjson":{"itemSchema":{"type":"object","properties":{"id":{"type":"string"}}}}}}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient, isErrorCode, TransportErrorCode } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { throw new Error("offline"); } });
try { await api.$streams.listEvents().next(); throw new Error("network failure was accepted"); }
catch (error) { if (!isErrorCode(error, TransportErrorCode.NETWORK_ERROR)) throw error; }
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript stream network-error runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedResponseStreamsDecodeJSONLines(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.2.0","info":{"title":"JSON Lines","version":"1"},"paths":{"/logs":{"get":{"operationId":"tailJSONLines","responses":{"200":{"description":"OK","content":{"application/jsonl":{"itemSchema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response('{"event_id":"one"}\n{"event_id":"two"}\n', { status: 200, headers: { "content-type": "application/jsonl" } }) });
const events = [];
for await (const event of api.$streams.tailJSONLines()) events.push(event.eventID);
if (events.join(",") !== "one,two") throw new Error("JSON Lines stream did not decode item schemas");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript JSON Lines stream runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeUsesRegisteredCustomResponseStreamCodec(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.2.0","info":{"title":"Custom stream","version":"1"},"paths":{"/events":{"get":{"operationId":"tailCustomEvents","responses":{"200":{"description":"OK","content":{"application/vnd.acme.events":{"itemSchema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let maxFrameBytes = 0;
const codec = { decodeStream: async function* (reader, context) {
  maxFrameBytes = context.maxFrameBytes;
  let pending = "";
  for (;;) {
    const bytes = await reader.read(3);
    if (bytes === null) break;
    pending += new TextDecoder().decode(bytes);
    let index;
    while ((index = pending.indexOf("\n")) >= 0) { const record = pending.slice(0, index); pending = pending.slice(index + 1); if (record !== "") yield JSON.parse(record); }
  }
} };
const api = createClient({ baseURL: "https://api.example.test", maxStreamItemBytes: 5, codecs: { "application/vnd.acme.events": codec }, fetch: async () => new Response('{"event_id":"one"}\n{"event_id":"two"}\n', { status: 200, headers: { "content-type": "application/vnd.acme.events" } }) });
const events = [];
for await (const event of api.$streams.tailCustomEvents()) events.push(event.eventID);
if (events.join(",") !== "one,two" || maxFrameBytes !== 5) throw new Error("custom stream codec did not receive bounded reader data");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript custom-stream-codec runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeUsesRegisteredCustomRequestStreamCodec(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.2.0","info":{"title":"Custom request stream","version":"1"},"paths":{"/events":{"post":{"operationId":"publishCustomEvents","requestBody":{"required":true,"content":{"application/vnd.acme.events":{"itemSchema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}}},"responses":{"204":{"description":"No Content"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const encoder = new TextEncoder();
const codec = { encodeStream: (items) => {
  const iterator = items[Symbol.asyncIterator]();
  return new ReadableStream({ async pull(controller) { const next = await iterator.next(); if (next.done) controller.close(); else controller.enqueue(encoder.encode(next.value.event_id + "\n")); }, async cancel(reason) { await iterator.return?.(reason); } });
} };
let sent = "";
const api = createClient({ baseURL: "https://api.example.test", codecs: { "application/vnd.acme.events": codec }, fetch: async (_input, init) => {
  if (new Headers(init.headers).get("content-type") !== "application/vnd.acme.events") throw new Error("custom stream content type missing");
  sent = await new Response(init.body).text();
  return new Response(null, { status: 204 });
} });
async function* events() { yield { eventID: "one" }; yield { eventID: "two" }; }
await api.$operations.publishCustomEvents({ body: events() });
if (sent !== "one\ntwo\n") throw new Error("custom request stream codec did not receive wire-transformed items");
let fetched = false;
const missing = createClient({ baseURL: "https://api.example.test", fetch: async () => { fetched = true; throw new Error("fetch must not run"); } });
try { await missing.$operations.publishCustomEvents({ body: events() }); throw new Error("missing custom stream codec was accepted"); }
catch (error) { if (!String(error).includes("missing encodeStream codec") && !String(error.cause).includes("missing encodeStream codec")) throw error; }
if (fetched) throw new Error("fetch ran without a custom stream codec");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript custom-request-stream-codec runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedStreamingMultipartResponseDecodesItemsAndHeaders(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Multipart stream","version":"1"},
  "paths":{"/frames":{"get":{"operationId":"tailFrames","responses":{"200":{"description":"OK","content":{"multipart/mixed":{"itemSchema":{"type":"object","required":["frame_id"],"properties":{"frame_id":{"type":"string"}}},"itemEncoding":{"contentType":"application/json","headers":{"x-frame":{"required":true,"schema":{"type":"string"}}}}}}}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const encoder = new TextEncoder();
const body = "--frames\r\ncontent-type: application/json\r\nx-frame: first\r\n\r\n{\"frame_id\":\"one\"}\r\n--frames\r\ncontent-type: application/json\r\nx-frame: second\r\n\r\n{\"frame_id\":\"two\"}\r\n--frames--\r\n";
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response(new ReadableStream({
  start(controller) { controller.enqueue(encoder.encode(body.slice(0, 37))); controller.enqueue(encoder.encode(body.slice(37, 113))); controller.enqueue(encoder.encode(body.slice(113))); controller.close(); },
}), { status: 200, headers: { "content-type": "multipart/mixed; boundary=frames" } }) });
const frames = [];
for await (const frame of api.$streams.tailFrames()) frames.push(frame.frameID);
if (frames.join(",") !== "one,two") throw new Error("streaming multipart response did not decode item schemas");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript streaming-multipart response runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedPositionalMultipartResponseDecodesCompleteBody(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Multipart response","version":"1"},
  "paths":{"/bundle":{"get":{"operationId":"getBundle","responses":{"200":{"description":"OK","content":{"multipart/mixed":{"schema":{"type":"array","prefixItems":[{"type":"object","required":["bundle_id"],"properties":{"bundle_id":{"type":"string"}}},{"type":"string"}]},"prefixEncoding":[{"contentType":"application/json","headers":{"x-part":{"required":true,"schema":{"type":"string"}}}},{"contentType":"text/plain"}]}}}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const body = "--bundle\r\ncontent-type: application/json\r\nx-part: manifest\r\n\r\n{\"bundle_id\":\"one\"}\r\n--bundle\r\ncontent-type: text/plain\r\n\r\nready\r\n--bundle--\r\n";
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response(body, { status: 200, headers: { "content-type": "multipart/mixed; boundary=bundle" } }) });
const bundle = await api.$operations.getBundle();
if (JSON.stringify(bundle) !== JSON.stringify([{ bundleID: "one" }, "ready"])) throw new Error("complete multipart response did not decode positional parts");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript positional-multipart response runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedNestedMultipartRequestAndResponseRoundTrip(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Nested multipart","version":"1"},
  "paths":{"/nested":{"post":{"operationId":"roundTripNested","requestBody":{"required":true,"content":{"multipart/mixed":{"schema":{"type":"array","prefixItems":[{"type":"array","items":{"type":"object","properties":{"top_id":{"type":"string"}}}},{"type":"array","prefixItems":[{"type":"object","required":["inner_id"],"properties":{"inner_id":{"type":"string"}}},{"type":"string"}]}]},"prefixEncoding":[{}, {"contentType":"multipart/mixed","prefixEncoding":[{"contentType":"application/json"},{"contentType":"text/plain"}]}]}}},"responses":{"200":{"description":"OK","content":{"multipart/mixed":{"schema":{"type":"array","prefixItems":[{"type":"array","items":{"type":"object","properties":{"top_id":{"type":"string"}}}},{"type":"array","prefixItems":[{"type":"object","required":["inner_id"],"properties":{"inner_id":{"type":"string"}}},{"type":"string"}]}]},"prefixEncoding":[{}, {"contentType":"multipart/mixed","prefixEncoding":[{"contentType":"application/json"},{"contentType":"text/plain"}]}]}}}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const responseBody = "--outer\r\ncontent-type: application/json\r\n\r\n[{\"top_id\":\"out\"}]\r\n--outer\r\ncontent-type: multipart/mixed; boundary=inner\r\n\r\n--inner\r\ncontent-type: application/json\r\n\r\n{\"inner_id\":\"out\"}\r\n--inner\r\ncontent-type: text/plain\r\n\r\nready\r\n--inner--\r\n\r\n--outer--\r\n";
const api = createClient({ baseURL: "https://api.example.test", fetch: async (_input, init) => {
  const contentType = new Headers(init.headers).get("content-type") ?? "";
  const body = await new Response(init.body).text();
  if (!contentType.startsWith("multipart/mixed; boundary=") || !body.includes("Content-Type: multipart/mixed; boundary=") || !body.includes("Content-Type: application/json") || !body.includes("inner")) throw new Error("nested multipart request was not encoded");
  return new Response(responseBody, { status: 200, headers: { "content-type": "multipart/mixed; boundary=outer" } });
} });
const result = await api.$operations.roundTripNested({ body: [[{ topID: "in" }], [{ innerID: "in" }, "queued"]] });
if (JSON.stringify(result) !== JSON.stringify([[{ topID: "out" }], [{ innerID: "out" }, "ready"]])) throw new Error("nested multipart response was not decoded");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript nested-multipart runtime test: %v\n%s", err, output)
	}
}

func TestBooleanFalseSchemaRuntimeRejectsResponseValue(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Never","version":"1"},"paths":{"/never":{"get":{"operationId":"getNever","responses":{"200":{"description":"Impossible","content":{"application/json":{"schema":false}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response(JSON.stringify({ value: true }), { status: 200, headers: { "content-type": "application/json" } }) });
try { await api.$operations.getNever(); throw new Error("false response schema accepted a value"); }
catch (error) { if (!String(error).includes("schema is false") && !String(error.cause).includes("schema is false")) throw error; }
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript false-schema response test: %v\n%s", err, output)
	}
}

func TestRuntimeRejectsUnevaluatedPropertiesAndItemsBeforeFetch(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Unevaluated","version":"1"},"paths":{"/object":{"post":{"operationId":"createObject","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","allOf":[{"properties":{"id":{"type":"string"}}},{"properties":{"name":{"type":"string"}}}],"unevaluatedProperties":false}}}},"responses":{"204":{"description":"No Content"}}}},"/array":{"post":{"operationId":"createArray","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"array","prefixItems":[{"type":"string"}],"unevaluatedItems":false}}}},"responses":{"204":{"description":"No Content"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let fetched = false;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { fetched = true; throw new Error("fetch must not run"); } });
for (const [operation, input, expected] of [["createObject", { body: { id: "one", name: "widget", extra: true } }, "unexpected unevaluated property extra"], ["createArray", { body: ["one", "extra"] }, "unexpected unevaluated item 1"]]) {
  try { await api.$operations[operation](input); throw new Error("unevaluated value accepted"); }
  catch (error) { if (!String(error).includes(expected) && !String(error.cause).includes(expected)) throw error; }
}
if (fetched) throw new Error("fetch ran before unevaluated-value validation");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript unevaluated runtime test: %v\n%s", err, output)
	}
}

func TestDiscriminatorRuntimeSelectsMappedBranch(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Pets","version":"1"},"paths":{"/pet":{"get":{"operationId":"getPet","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Pet"}}}}}}}},"components":{"schemas":{"Pet":{"oneOf":[{"$ref":"#/components/schemas/Cat"},{"$ref":"#/components/schemas/Dog"}],"discriminator":{"propertyName":"kind","mapping":{"cat":"#/components/schemas/Cat","dog":"#/components/schemas/Dog"}}},"Cat":{"type":"object","required":["kind","lives"],"properties":{"kind":{"const":"cat"},"lives":{"type":"integer"}}},"Dog":{"type":"object","required":["kind","barks"],"properties":{"kind":{"const":"dog"},"barks":{"type":"boolean"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response(JSON.stringify({ kind: "cat", lives: 9 }), { status: 200, headers: { "content-type": "application/json" } }) });
const pet = await api.$operations.getPet();
if (pet.kind !== "cat" || pet.lives !== 9) throw new Error("discriminator did not select the cat branch");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript discriminator runtime test: %v\n%s", err, output)
	}
}

func TestOpenAPI32DiscriminatorDefaultMappingSelectsAnyOfTransform(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.2.0","info":{"title":"Pets","version":"1"},"paths":{"/pet":{"get":{"operationId":"getPet","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Pet"}}}}}}}},"components":{"schemas":{"Pet":{"anyOf":[{"$ref":"#/components/schemas/Cat"},{"$ref":"#/components/schemas/OtherPet"}],"discriminator":{"propertyName":"kind","defaultMapping":"OtherPet"}},"Cat":{"type":"object","properties":{"kind":{"const":"cat"}}},"OtherPet":{"type":"object","required":["display_name"],"properties":{"display_name":{"type":"string"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response(JSON.stringify({ display_name: "mystery" }), { status: 200, headers: { "content-type": "application/json" } }) });
const pet = await api.$operations.getPet();
if (pet.displayName !== "mystery" || "display_name" in pet) throw new Error("default discriminator mapping did not select OtherPet");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript default-discriminator runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeDecodesWildcardResponseMediaTypes(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Wildcard","version":"1"},"paths":{"/problem":{"get":{"operationId":"getProblem","responses":{"200":{"description":"OK","content":{"application/*+json":{"schema":{"type":"object","required":["code"],"properties":{"code":{"type":"string"}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response(JSON.stringify({ code: "problem" }), { status: 200, headers: { "content-type": "application/problem+json" } }) });
const problem = await api.$operations.getProblem();
if (problem.code !== "problem") throw new Error("wildcard response media type was not decoded");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript wildcard-response runtime test: %v\n%s", err, output)
	}
}

func TestVariantRuntimeRejectsValuesMatchingNoBranch(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Variant","version":"1"},"paths":{"/variant":{"post":{"operationId":"createVariant","requestBody":{"required":true,"content":{"application/json":{"schema":{"oneOf":[{"type":"string"},{"type":"integer"}]}}}},"responses":{"204":{"description":"No Content"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let fetched = false;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { fetched = true; throw new Error("fetch must not run"); } });
try { await api.$operations.createVariant({ body: true }); throw new Error("invalid variant accepted"); }
catch (error) {
  if (!String(error).includes("oneOf requires exactly one matching schema") && !String(error.cause).includes("oneOf requires exactly one matching schema")) throw error;
  if (fetched) throw new Error("fetch ran before variant validation");
}
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript variant runtime test: %v\n%s", err, output)
	}
}

func TestSchemaRuntimeRejectsNumericBoundsBeforeFetch(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Bounds","version":"1"},"paths":{"/bounds":{"post":{"operationId":"createBound","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"integer","minimum":2,"maximum":4,"multipleOf":2}}}},"responses":{"204":{"description":"No Content"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let fetched = false;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { fetched = true; throw new Error("fetch must not run"); } });
try { await api.$operations.createBound({ body: 0 }); throw new Error("invalid numeric bound accepted"); }
catch (error) {
  if (!String(error).includes("must be >= 2") && !String(error.cause).includes("must be >= 2")) throw error;
  if (fetched) throw new Error("fetch ran before numeric validation");
}
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript numeric-bound runtime test: %v\n%s", err, output)
	}
}

func TestRuntimePreservesReservedQueryCharactersWhenAllowed(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Reserved","version":"1"},"paths":{"/search":{"get":{"operationId":"searchItems","parameters":[{"name":"query","in":"query","allowReserved":true,"schema":{"type":"string"}}],"responses":{"204":{"description":"No Content"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let requestURL = "";
const api = createClient({ baseURL: "https://api.example.test", fetch: async (url) => { requestURL = String(url); return new Response(null, { status: 204 }); } });
await api.$operations.searchItems({ query: { query: "/a:b?c=d&e" } });
if (!requestURL.includes("query=/a:b?c=d%26e")) throw new Error("reserved query serialization mismatch: " + requestURL);
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript allowReserved runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeSerializesDelimitedObjectQueryParameters(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Delimited","version":"1"},"paths":{"/search":{"get":{"operationId":"searchDelimited","parameters":[{"name":"filter","in":"query","style":"pipeDelimited","explode":false,"schema":{"type":"object","additionalProperties":{"type":"string"}}}],"responses":{"204":{"description":"No Content"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let requestURL = "";
const api = createClient({ baseURL: "https://api.example.test", fetch: async (url) => { requestURL = String(url); return new Response(null, { status: 204 }); } });
await api.$operations.searchDelimited({ query: { filter: { name: "widget", state: "active" } } });
if (!requestURL.includes("filter=name%7Cwidget%7Cstate%7Cactive")) throw new Error("delimited object mismatch: " + requestURL);
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript delimited-object runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeSerializesOpenAPI32QuerystringFormContent(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.2.0","info":{"title":"Querystring","version":"1"},"paths":{"/search":{"get":{"operationId":"searchWholeQuery","parameters":[{"name":"form","in":"querystring","content":{"application/x-www-form-urlencoded":{"schema":{"type":"object","properties":{"term":{"type":"string"},"page":{"type":"integer"}}}}}}],"responses":{"204":{"description":"No Content"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let requestURL = "";
const api = createClient({ baseURL: "https://api.example.test", fetch: async (url) => { requestURL = String(url); return new Response(null, { status: 204 }); } });
await api.$operations.searchWholeQuery({ query: { form: { term: "widgets", page: 2 } } });
if (!requestURL.includes("term=widgets&page=2")) throw new Error("whole query form mismatch: " + requestURL);
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript querystring-form runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeSerializesOpenAPI32CookieStyleWithoutPercentEncoding(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.2.0","info":{"title":"Cookie style","version":"1"},"paths":{"/preferences":{"get":{"operationId":"getPreferences","parameters":[{"name":"prefs","in":"cookie","style":"cookie","explode":true,"schema":{"type":"object","required":["theme","event_id"],"properties":{"theme":{"type":"string"},"event_id":{"type":"string"}}}}],"responses":{"204":{"description":"No Content"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", transport: { capabilities: { cookieJar: true }, fetch: async (_input, init) => {
  if (new Headers(init.headers).get("cookie") !== "theme=dark; event_id=a%2Fb") throw new Error("cookie style changed raw cookie text: " + new Headers(init.headers).get("cookie"));
  return new Response(null, { status: 204 });
} } });
await api.$operations.getPreferences({ cookieParams: { prefs: { theme: "dark", eventID: "a%2Fb" } } });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript cookie-style runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeSelectsAndExpandsOpenAPIServerVariables(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Servers","version":"1"},"servers":[{"url":"/api/{region}","variables":{"region":{"default":"kr","enum":["kr","us"]}}}],"paths":{"/status":{"get":{"operationId":"getStatus","responses":{"204":{"description":"OK"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let url = "";
const api = createClient({ origin: "https://gateway.example.test", server: { id: "#/servers/0", variables: { region: "us" } }, fetch: async (requestURL) => { url = String(requestURL); return new Response(null, { status: 204 }); } });
await api.$operations.getStatus();
if (url !== "https://gateway.example.test/api/us/status") throw new Error("server expansion mismatch: " + url);
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript server-selection runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeSelectsOperationScopedServerAlternatives(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Scoped servers","version":"1"},"paths":{"/status":{"get":{"operationId":"getStatus","servers":[{"url":"https://one.example.test/v1"},{"url":"https://two.example.test/v2"}],"responses":{"204":{"description":"OK"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let url = "";
const api = createClient({ server: { id: "#/paths/~1status/get/servers/1" }, fetch: async (requestURL) => { url = String(requestURL); return new Response(null, { status: 204 }); } });
await api.$operations.getStatus();
if (url !== "https://two.example.test/v2/status") throw new Error("scoped server selection mismatch: " + url);
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript scoped-server runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeDecodesDeclaredResponseHeaders(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Headers","version":"1"},"components":{"headers":{"RateLimit":{"required":true,"schema":{"type":"integer"}}}},"paths":{"/limit":{"get":{"operationId":"getLimit","responses":{"default":{"description":"Fallback","headers":{"X-Fallback":{"required":true,"schema":{"type":"boolean"}}}},"200":{"description":"OK","headers":{"X-Rate-Limit":{"$ref":"#/components/headers/RateLimit"},"X-Context":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["region"],"properties":{"region":{"type":"string"}}}}}},"X-Event":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}}},"X-Custom":{"required":true,"content":{"application/vnd.example.header":{"schema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}}},"X-Object":{"required":true,"style":"simple","explode":true,"schema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}},"content":{"application/json":{"schema":{"type":"object"}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", codecs: { "application/vnd.example.header": { decodeParameter: (value) => ({ event_id: value.replace("custom:", "") }) } }, fetch: async () => new Response("{}", { status: 200, headers: { "content-type": "application/json", "x-rate-limit": "42", "x-context": '{"region":"kr"}', "x-event": '{"event_id":"event"}', "x-custom": "custom:client", "x-object": "event_id=object" } }) });
const response = await api.$operations.getLimit.raw();
if (response.headers.xRateLimit !== 42 || response.headers.xContext.region !== "kr" || response.headers.xEvent.eventID !== "event" || response.headers.xCustom.eventID !== "client" || response.headers.xObject.eventID !== "object" || response.response.headers.get("x-rate-limit") !== "42") throw new Error("typed response header mismatch");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript response-header runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeRejectsMalformedDeclaredResponseHeaders(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Headers","version":"1"},"paths":{"/limit":{"get":{"operationId":"getLimit","responses":{"200":{"description":"OK","headers":{"X-Rate-Limit":{"required":true,"schema":{"type":"integer"}},"X-Enabled":{"required":true,"schema":{"type":"boolean"}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response(null, { status: 200, headers: { "x-rate-limit": "42widgets", "x-enabled": "not-a-boolean" } }) });
await api.$operations.getLimit.raw().then(() => { throw new Error("malformed headers were accepted"); }, (error) => { if (error.code !== "RESPONSE_DECODE_FAILED" || !String(error.cause).includes("not a boolean")) throw error; });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript malformed-response-header runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeRequiresDeclaredCapabilityForSetCookieResponseHeaders(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Cookies","version":"1"},"paths":{"/session":{"get":{"operationId":"getSession","responses":{"204":{"description":"OK","headers":{"Set-Cookie":{"required":true,"schema":{"type":"string"}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const fetch = async () => new Response(null, { status: 204, headers: { "set-cookie": "session=abc" } });
await createClient({ baseURL: "https://api.example.test", fetch }).$operations.getSession.raw().then(() => { throw new Error("unreadable Set-Cookie was accepted"); }, (error) => { if (error.code !== "TRANSPORT_CAPABILITY_REQUIRED") throw error; });
const api = createClient({ baseURL: "https://api.example.test", transport: { capabilities: { readableResponseHeaders: ["set-cookie"] }, fetch } });
const response = await api.$operations.getSession.raw();
if (response.headers.setCookie !== "session=abc") throw new Error("capable Set-Cookie transport did not decode header");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript Set-Cookie capability runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeUsesRegisteredMediaCodecsForDeclaredCustomMedia(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Codec","version":"1"},"paths":{"/widget":{"post":{"operationId":"createWidget","requestBody":{"required":true,"content":{"application/vnd.acme.widget":{"schema":{"type":"string"}}}},"responses":{"200":{"description":"OK","content":{"application/vnd.acme.widget":{"schema":{"type":"string"}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let encoded = "";
const api = createClient({ baseURL: "https://api.example.test", codecs: { "application/vnd.acme.widget": { encode: (value) => { encoded = "wire:" + value; return encoded; }, decode: async (response) => "decoded:" + await response.text() } }, fetch: async (_url, init) => { if (String(init.body) !== "wire:input") throw new Error("custom encoder was skipped"); return new Response("output", { status: 200, headers: { "content-type": "application/vnd.acme.widget" } }); } });
const value = await api.$operations.createWidget({ body: "input" });
if (encoded !== "wire:input" || value !== "decoded:output") throw new Error("custom codec result mismatch");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript custom-codec runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeResolvesReusableOpenAPI32MediaTypes(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.2.0","info":{"title":"Reusable media","version":"1"},"components":{"mediaTypes":{"Widget":{"schema":{"type":"object","required":["widget_id"],"properties":{"widget_id":{"type":"string"}}}}}},"paths":{"/widget":{"post":{"operationId":"createWidget","requestBody":{"required":true,"content":{"application/json":{"$ref":"#/components/mediaTypes/Widget"}}},"responses":{"200":{"description":"OK","content":{"application/json":{"$ref":"#/components/mediaTypes/Widget"}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async (_url, init) => {
  if (init.body !== '{"widget_id":"input"}') throw new Error("reusable request media type was not encoded");
  return new Response('{"widget_id":"output"}', { status: 200, headers: { "content-type": "application/json" } });
} });
const widget = await api.$operations.createWidget({ body: { widgetID: "input" } });
if (widget.widgetID !== "output") throw new Error("reusable response media type was not decoded");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript reusable-media runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeAppliesOpenAPISecurityAlternativesAndOperationOverride(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.2.0","info":{"title":"Security","version":"1"},"security":[{"ApiKey":[]},{"Bearer":[]}],"components":{"securitySchemes":{"ApiKey":{"type":"apiKey","in":"header","name":"X-API-Key"},"Bearer":{"type":"http","scheme":"bearer"}}},"paths":{"/protected":{"get":{"operationId":"readSecure","responses":{"204":{"description":"OK"}}}},"/public":{"get":{"operationId":"getPublic","security":[],"responses":{"204":{"description":"OK"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let calls = 0;
let credentialCalls = 0;
const credentials = ({ alternatives, origin }) => {
  credentialCalls++;
  if (origin !== "https://api.example.test") throw new Error("credential origin mismatch");
  return { alternative: alternatives.ApiKey, values: { ApiKey: { kind: "api-key", value: "secret" } } };
};
const api = createClient({ baseURL: "https://api.example.test", credentials, fetch: async (_url, init) => {
  calls++;
  if (new Headers(init.headers).get("x-api-key") !== "secret") throw new Error("API key not applied");
  return new Response(null, { status: 204 });
} });
await api.$operations.readSecure();
if (credentialCalls !== 1 || calls !== 1) throw new Error("protected request selection mismatch");
const publicAPI = createClient({ baseURL: "https://api.example.test", credentials, fetch: async (_url, init) => {
  if (new Headers(init.headers).has("x-api-key")) throw new Error("operation security override was ignored");
  return new Response(null, { status: 204 });
} });
await publicAPI.$operations.getPublic();
if (credentialCalls !== 1) throw new Error("public operation requested credentials");
await createClient({ baseURL: "https://api.example.test", credentials, headers: { "x-api-key": "caller" }, fetch: async () => { throw new Error("fetch must not run after credential collision"); } }).$operations.readSecure().then(() => { throw new Error("credential collision was accepted"); }, (error) => { if (error.code !== "SECURITY_CREDENTIALS_INVALID") throw error; });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript security runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeAppliesEveryHostManagedSecurityCredentialShape(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.2.0","info":{"title":"Security shapes","version":"1"},"components":{"securitySchemes":{"Basic":{"type":"http","scheme":"basic"},"Bearer":{"type":"http","scheme":"bearer","bearerFormat":"JWT"},"Digest":{"type":"http","scheme":"digest"},"QueryKey":{"type":"apiKey","in":"query","name":"api_key"},"CookieKey":{"type":"apiKey","in":"cookie","name":"session"},"OAuth":{"type":"oauth2","oauth2MetadataUrl":"https://auth.example.test/metadata","flows":{"authorizationCode":{"authorizationUrl":"https://auth.example.test/authorize","tokenUrl":"https://auth.example.test/token","refreshUrl":"https://auth.example.test/refresh","scopes":{}},"clientCredentials":{"tokenUrl":"https://auth.example.test/token","scopes":{"widgets:read":"Read widgets"}},"deviceAuthorization":{"deviceAuthorizationUrl":"https://auth.example.test/device","tokenUrl":"https://auth.example.test/token","scopes":{}}}},"OpenID":{"type":"openIdConnect","openIdConnectUrl":"https://auth.example.test/openid"},"Mutual":{"type":"mutualTLS","deprecated":true}}},"paths":{"/basic":{"get":{"operationId":"getBasic","security":[{"Basic":[]}],"responses":{"204":{"description":"OK"}}}},"/bearer":{"get":{"operationId":"getBearer","security":[{"Bearer":[]}],"responses":{"204":{"description":"OK"}}}},"/digest":{"get":{"operationId":"getDigest","security":[{"Digest":[]}],"responses":{"204":{"description":"OK"}}}},"/query":{"get":{"operationId":"getQuery","security":[{"QueryKey":[]}],"responses":{"204":{"description":"OK"}}}},"/oauth":{"get":{"operationId":"getOAuth","security":[{"OAuth":["widgets:read"]}],"responses":{"204":{"description":"OK"}}}},"/openid":{"get":{"operationId":"getOpenID","security":[{"OpenID":[]}],"responses":{"204":{"description":"OK"}}}},"/cookie":{"get":{"operationId":"getCookie","security":[{"CookieKey":[]}],"responses":{"204":{"description":"OK"}}}},"/mtls":{"get":{"operationId":"getMTLS","security":[{"Mutual":[]}],"responses":{"204":{"description":"OK"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const credentials = ({ alternatives }) => {
  const alternative = Object.values(alternatives)[0];
  const scheme = alternative.schemes[0];
  if (scheme.name === "Bearer" && scheme.bearerFormat !== "JWT") throw new Error("bearer format metadata missing");
  if (scheme.name === "OAuth" && (scheme.oauth2MetadataUrl !== "https://auth.example.test/metadata" || scheme.flows.authorizationCode.refreshUrl !== "https://auth.example.test/refresh" || scheme.flows.deviceAuthorization.deviceAuthorizationUrl !== "https://auth.example.test/device" || scheme.scopes[0] !== "widgets:read")) throw new Error("OAuth metadata missing");
  if (scheme.name === "OpenID" && scheme.openIdConnectUrl !== "https://auth.example.test/openid") throw new Error("OpenID metadata missing");
  if (scheme.name === "Mutual" && !scheme.deprecated) throw new Error("mTLS deprecation metadata missing");
  const values = {
    Basic: { kind: "http-basic", username: "a", password: "b" },
    Bearer: { kind: "http-bearer", token: "bearer-token" },
    Digest: { kind: "http", value: "digest-value" },
    QueryKey: { kind: "api-key", value: "query-value" },
    CookieKey: { kind: "api-key", value: "cookie-value" },
    OAuth: { kind: "oauth2", token: "oauth-token" },
    OpenID: { kind: "openIdConnect", token: "openid-token" },
    Mutual: { kind: "mutual-tls" },
  };
  return { alternative, values: { [scheme.name]: values[scheme.name] } };
};
const expected = {
  "/basic": "Basic YTpi",
  "/bearer": "Bearer bearer-token",
  "/digest": "digest digest-value",
  "/oauth": "Bearer oauth-token",
  "/openid": "Bearer openid-token",
};
const api = createClient({ baseURL: "https://api.example.test", credentials, fetch: async (input, init) => {
  const url = new URL(String(input));
  if (url.pathname === "/query") {
    if (url.searchParams.get("api_key") !== "query-value") throw new Error("query API key mismatch");
  } else if (new Headers(init.headers).get("authorization") !== expected[url.pathname]) {
    throw new Error("authorization mismatch for " + url.pathname);
  }
  return new Response(null, { status: 204 });
} });
await api.$operations.getBasic();
await api.$operations.getBearer();
await api.$operations.getDigest();
await api.$operations.getQuery();
await api.$operations.getOAuth();
await api.$operations.getOpenID();
await api.$operations.getCookie().then(() => { throw new Error("cookie security unexpectedly dispatched"); }, (error) => { if (error.code !== "TRANSPORT_CAPABILITY_REQUIRED") throw error; });
await api.$operations.getMtls().then(() => { throw new Error("mTLS security unexpectedly dispatched"); }, (error) => { if (error.code !== "TRANSPORT_CAPABILITY_REQUIRED") throw error; });
const capable = createClient({ baseURL: "https://api.example.test", credentials, transport: { capabilities: { cookieJar: true, mutualTLS: true }, fetch: async (input, init) => {
  const url = new URL(String(input));
  if (url.pathname === "/cookie" && new Headers(init.headers).get("cookie") !== "session=cookie-value") throw new Error("cookie capability was not used");
  return new Response(null, { status: 204 });
} } });
await capable.$operations.getCookie();
await capable.$operations.getMtls();
const invalid = createClient({ baseURL: "https://api.example.test", credentials: ({ alternatives }) => ({ alternative: alternatives.Bearer, values: { Basic: { kind: "http-basic", username: "a", password: "b" } } }), fetch: async () => { throw new Error("invalid credentials reached fetch"); } });
await invalid.$operations.getBearer().then(() => { throw new Error("invalid credential set was accepted"); }, (error) => { if (error.code !== "SECURITY_CREDENTIALS_INVALID") throw error; });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript security-shape runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeEncodesOpenAPIEncodingObjectMultipartParts(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Multipart encoding", "version": "1"},
  "paths": {
    "/upload": {
      "post": {
        "operationId": "uploadAsset",
        "requestBody": {
          "required": true,
          "content": {
            "multipart/form-data": {
              "schema": {
                "type": "object",
                "required": ["metadata"],
                "properties": {"metadata": {"type": "object", "required": ["title"], "properties": {"title": {"type": "string"}}}}
              },
              "encoding": {
                "metadata": {
	                  "contentType": "application/vnd.example.asset",
	                  "headers": {"X-Part-ID": {"required": true, "schema": {"type": "string"}}, "X-Part-Meta": {"style":"simple","explode":true,"schema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}, "X-Part-Custom":{"content":{"application/vnd.example.part":{"schema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}}}}
                }
              }
            }
          }
        },
        "responses": {"204": {"description": "OK"}}
      }
    }
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", codecs: { "application/vnd.example.asset": { encode: (value) => "asset:" + value.title }, "application/vnd.example.part": { decodeParameter: (value) => ({ event_id: value.replace("custom:", "") }) } }, fetch: async (_url, init) => {
  const contentType = new Headers(init.headers).get("content-type");
  if (!contentType?.startsWith("multipart/form-data; boundary=")) throw new Error("multipart boundary header missing");
  const body = await init.body.text();
  if (!body.includes("Content-Type: application/vnd.example.asset") || !body.includes("x-part-id: asset-42") || !body.includes("x-part-meta: event_id=asset") || !body.includes("x-part-custom: custom:asset") || !body.includes("asset:manual")) throw new Error("Encoding Object part plan mismatch");
  return new Response(null, { status: 204 });
} });
await api.$operations.uploadAsset({ body: { metadata: { title: "manual" } } }, { multipartHeaders: { metadata: { "X-Part-ID": "asset-42", "X-Part-Meta": "event_id=asset", "X-Part-Custom": "custom:asset" } } });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript Encoding Object multipart runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeEncodesAndDecodesOpenAPIXMLObjects(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.2.0",
  "info": {"title": "XML", "version": "1"},
  "paths": {
    "/pet": {
      "post": {
        "operationId": "savePet",
        "requestBody": {"required": true, "content": {"application/xml": {"schema": {
          "type": "object", "xml": {"name": "pet", "namespace": "https://example.test/pets", "prefix": "p"},
          "required": ["pet_id", "name"],
          "properties": {
            "pet_id": {"type": "integer", "xml": {"name": "id", "attribute": true}},
            "name": {"type": "string", "xml": {"name": "pet_name"}},
            "tags": {"type": "array", "xml": {"name": "tags", "wrapped": true}, "items": {"type": "string", "xml": {"name": "tag"}}}
          }
        }}}},
        "responses": {"200": {"description": "OK", "content": {"application/xml": {"schema": {
          "type": "object", "xml": {"name": "pet", "prefix": "p"},
          "properties": {
            "pet_id": {"type": "integer", "xml": {"name": "id", "attribute": true}},
            "name": {"type": "string", "xml": {"name": "pet_name"}},
            "tags": {"type": "array", "xml": {"name": "tags", "wrapped": true}, "items": {"type": "string", "xml": {"name": "tag"}}}
          }
        }}}}}
      }
    }
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async (_url, init) => {
  const body = String(init.body);
  if (!body.includes('<p:pet xmlns:p="https://example.test/pets" id="7">') || !body.includes("<pet_name>Milo &amp; Co</pet_name>") || !body.includes("<tags><tag>one</tag><tag>two</tag></tags>")) throw new Error("XML request encoding mismatch: " + body);
  return new Response('<p:pet id="8"><pet_name>Rex</pet_name><tags><tag>red</tag><tag>blue</tag></tags></p:pet>', { status: 200, headers: { "content-type": "application/xml" } });
} });
const pet = await api.$operations.savePet({ body: { petID: 7, name: "Milo & Co", tags: ["one", "two"] } });
if (pet.petID !== 8 || pet.name !== "Rex" || pet.tags.join(",") !== "red,blue") throw new Error("XML response decoding mismatch");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript XML runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeValidatesJSONSchemaContentSchema(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Content schema","version":"1"},"paths":{"/payload":{"get":{"operationId":"getPayload","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"type":"string","contentEncoding":"base64","contentMediaType":"application/json","contentSchema":{"type":"object","required":["code"],"properties":{"code":{"type":"string"}}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const valid = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response('"eyJjb2RlIjoiT0sifQ=="', { status: 200, headers: { "content-type": "application/json" } }) });
if (await valid.$operations.getPayload() !== "eyJjb2RlIjoiT0sifQ==") throw new Error("content schema changed the outer string");
const invalid = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response('"e30="', { status: 200, headers: { "content-type": "application/json" } }) });
await invalid.$operations.getPayload().then(() => { throw new Error("invalid decoded content was accepted"); }, (error) => { if (error.code !== "RESPONSE_DECODE_FAILED") throw error; });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript contentSchema runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeValidatesXMLSchemaContentSchema(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "XML content schema", "version": "1"},
  "paths": {
    "/payload": {
      "get": {
        "operationId": "getPayload",
        "responses": {
          "200": {
            "description": "OK",
            "content": {
              "application/json": {
                "schema": {
                  "type": "string",
                  "contentMediaType": "application/xml",
                  "contentSchema": {
                    "type": "object",
                    "xml": {"name": "payload"},
                    "required": ["code"],
                    "properties": {"code": {"type": "string", "xml": {"name": "code"}}}
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const valid = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response(JSON.stringify("<payload><code>OK</code></payload>"), { status: 200, headers: { "content-type": "application/json" } }) });
if (await valid.$operations.getPayload() !== "<payload><code>OK</code></payload>") throw new Error("XML content schema changed the outer string");
const invalid = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response(JSON.stringify("<payload></payload>"), { status: 200, headers: { "content-type": "application/json" } }) });
await invalid.$operations.getPayload().then(() => { throw new Error("invalid XML content was accepted"); }, (error) => { if (error.code !== "RESPONSE_DECODE_FAILED") throw error; });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript XML contentSchema runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeUsesComponentNameForXMLReferenceRoot(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "XML component", "version": "1"},
  "components": {"schemas": {"Pet": {"type": "object", "required": ["name"], "properties": {"name": {"type": "string"}}}}},
  "paths": {
    "/pet": {
      "post": {
        "operationId": "savePet",
        "requestBody": {"required": true, "content": {"application/xml": {"schema": {"$ref": "#/components/schemas/Pet"}}}},
        "responses": {"200": {"description": "OK", "content": {"application/xml": {"schema": {"$ref": "#/components/schemas/Pet"}}}}}
      }
    }
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async (_url, init) => {
  if (String(init.body) !== "<Pet><name>Milo</name></Pet>") throw new Error("component XML root mismatch: " + init.body);
  return new Response("<Pet><name>Rex</name></Pet>", { status: 200, headers: { "content-type": "application/xml" } });
} });
const pet = await api.$operations.savePet({ body: { name: "Milo" } });
if (pet.name !== "Rex") throw new Error("component XML response mismatch");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript component XML runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeEncodesStructuredMultipartFieldsAsJSONParts(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Upload","version":"1"},"paths":{"/upload":{"post":{"operationId":"createUpload","requestBody":{"required":true,"content":{"multipart/form-data":{"schema":{"type":"object","required":["metadata"],"properties":{"metadata":{"type":"object","properties":{"id":{"type":"string"}}}}}}}},"responses":{"204":{"description":"OK"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async (_url, init) => { const part = init.body.get("metadata"); if (!(part instanceof Blob) || part.type !== "application/json" || await part.text() !== '{"id":"one"}') throw new Error("structured multipart part mismatch"); return new Response(null, { status: 204 }); } });
await api.$operations.createUpload({ body: { metadata: { id: "one" } } });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript structured-multipart runtime test: %v\n%s", err, output)
	}
}

func runTypeScriptRuntime(t *testing.T, document *ir.Document, operationID, input, method, responseBody string) {
	t.Helper()
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const [path, operationID, inputJSON, method, responseBody] = process.argv.slice(1);
const { createClient } = await import(pathToFileURL(path).href);
const requests = [];
const api = createClient({ baseURL: "https://api.example.test/v1", fetch: async (url, init) => {
  requests.push({ method: init.method, url: String(url) });
  return new Response(responseBody === "null" ? null : responseBody, { status: responseBody === "null" ? 204 : 200, headers: { "content-type": "application/json" } });
}});
const input = JSON.parse(inputJSON);
const output = input === null ? await api.$operations[operationID]() : await api.$operations[operationID](input);
if (requests[0].method !== method) throw new Error("method mismatch: " + requests[0].method);
if (responseBody === "null" ? output !== undefined : JSON.stringify(output) !== responseBody) throw new Error("decoded output mismatch");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js"), operationID, input, method, responseBody).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript runtime test: %v\n%s", err, output)
	}
}

func compileTypeScriptArtifacts(t *testing.T, document *ir.Document) string {
	t.Helper()
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	writeTargetArtifacts(t, source, artifacts)
	if err := os.WriteFile(filepath.Join(source, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "tsconfig.json"), []byte(parityTSConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join("..", "..", "..", "test", "typescript", "node_modules", "typescript", "lib", "tsc.js")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("TypeScript compiler unavailable for runtime test: %v", err)
	}
	if output, err := exec.Command("node", tsc, "--project", filepath.Join(source, "tsconfig.json")).CombinedOutput(); err != nil {
		t.Fatalf("compile generated TypeScript target: %v\n%s", err, output)
	}
	output := filepath.Join(directory, "output")
	if err := os.WriteFile(filepath.Join(output, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return output
}

const parityTSConfig = `{
  "compilerOptions": {"target":"ES2022","module":"NodeNext","moduleResolution":"NodeNext","strict":true,"skipLibCheck":true,"rootDir":".","outDir":"../output"},
  "include": ["**/*.ts"]
}`

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
