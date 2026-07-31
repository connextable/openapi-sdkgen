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

func TestRuntimeUsesJSONNumericEqualityForSignedZero(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0","info":{"title":"Signed zero","version":"1"},
  "paths":{"/values":{"post":{"operationId":"setValues","requestBody":{"required":true,"content":{"application/json":{"schema":{
    "type":"object","required":["constant","choice","items","loose"],"properties":{
      "constant":{"const":0},
      "choice":{"enum":[0]},
      "items":{"type":"array","uniqueItems":true,"items":{"type":"number"}},
      "loose":{"type":"array","uniqueItems":true}
    }
  }}}},"responses":{"204":{"description":"OK"}}}}}
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
await api.$operations.setValues({ body: { constant: -0, choice: -0, items: [0, 1], loose: [1, null] } });
try { await api.$operations.setValues({ body: { constant: 0, choice: 0, items: [0, -0], loose: [] } }); throw new Error("signed-zero uniqueItems duplicate was accepted"); }
catch (error) { if (!String(error).includes("unique items") && !String(error.cause).includes("unique items")) throw error; }
const sparse = Array(2); sparse[1] = null;
try { await api.$operations.setValues({ body: { constant: 0, choice: 0, items: sparse, loose: [] } }); throw new Error("sparse array was accepted"); }
catch (error) { if (!String(error).includes("sparse items") && !String(error.cause).includes("sparse items") && !String(error.cause).includes("cannot contain undefined")) throw error; }
try { await api.$operations.setValues({ body: { constant: 0, choice: 0, items: [], loose: [NaN, null] } }); throw new Error("non-finite JSON number was accepted"); }
catch (error) { if (!String(error).includes("finite JSON number") && !String(error.cause).includes("finite JSON number")) throw error; }
if (calls !== 1) throw new Error("signed-zero validation reached fetch unexpectedly: " + calls);
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute signed-zero JSON equality test: %v\n%s", err, output)
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

func TestGeneratedLinksAndStreamsUseStandardHeaderParameters(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0","info":{"title":"Required options","version":"1"},
  "paths":{
    "/source":{"get":{"operationId":"getSource","responses":{
      "200":{"description":"Required","links":{"follow":{"operationId":"followTarget"}}},
      "201":{"description":"Optional","links":{"follow":{"operationId":"optionalTarget"}}}
    }}},
    "/target":{"post":{"operationId":"followTarget","parameters":[
      {"name":"Idempotency-Key","in":"header","required":true,"schema":{"type":"string"}},
      {"name":"If-Match","in":"header","required":true,"schema":{"type":"string"}}
    ],"responses":{"204":{"description":"OK"}}}},
    "/optional":{"post":{"operationId":"optionalTarget","responses":{"204":{"description":"OK"}}}},
    "/events":{"get":{"operationId":"tailEvents","parameters":[
      {"name":"If-Match","in":"header","required":true,"schema":{"type":"string"}}
    ],"responses":{"200":{"description":"OK","content":{"application/x-ndjson":{"itemSchema":{"type":"string"}}}}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	client := string(artifactByPath(t, artifacts, "generated/client.ts"))
	for _, expected := range []string{
		`invocation?: LinkInvocation<Routes["POST /target"]["input"], Routes["POST /target"]["options"]`,
		`readonly "Idempotency-Key": string`,
		`readonly "If-Match": string`,
		`readonly "tailEvents": StreamCall<"GET /events">`,
	} {
		if !strings.Contains(client, expected) {
			t.Fatalf("standard header contract missing %q:\n%s", expected, client)
		}
	}
	probe := `import { createClient, type Routes } from "./index.js"
declare const api: ReturnType<typeof createClient>
declare const response: Routes["GET /source"]["rawResponse"]
api.$links.getSource.follow(response, { input: { headerParams: { "Idempotency-Key": "idem", "If-Match": "v1" } } })
api.$links.getSource.follow.byStatus.status201(response)
// @ts-expect-error removed transport convenience option
api.$operations.followTarget({ headerParams: { "Idempotency-Key": "idem", "If-Match": "v1" } }, { idempotencyKey: "legacy" })
`
	output := compileTypeScriptArtifactsWithProbe(t, document, "required-links.probe.ts", probe)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const seen = [];
const api = createClient({ baseURL: "https://api.example.test", fetch: async (input, init) => {
  const path = new URL(String(input)).pathname;
  const headers = new Headers(init.headers);
  seen.push([path, headers.get("idempotency-key"), headers.get("if-match")]);
  if (path === "/source") return new Response(null, { status: 200 });
  if (path === "/events") return new Response('"one"\n', { status: 200, headers: { "content-type": "application/x-ndjson" } });
  return new Response(null, { status: 204 });
}});
const source = await api.$operations.getSource.raw();
await api.$links.getSource.follow(source, { input: { headerParams: { "Idempotency-Key": "idem", "If-Match": "v1" } } });
for await (const _event of api.$streams.tailEvents({ headerParams: { "If-Match": "v2" } })) { break; }
if (JSON.stringify(seen) !== JSON.stringify([["/source",null,null],["/target","idem","v1"],["/events",null,"v2"]])) throw new Error("declared headers were not forwarded: " + JSON.stringify(seen));
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute required link/stream options test: %v\n%s", err, output)
	}
}

func TestRuntimeDelegatesEnvironmentControlledRequestHeadersToFetch(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.2.0",
  "info": {"title": "Fetch managed headers", "version": "1"},
  "paths": {
    "/oauth": {
      "post": {
        "operationId": "oauth",
        "parameters": [
          {"name": "Origin", "in": "header", "required": true, "schema": {"type": "string"}}
        ],
        "requestBody": {
          "required": true,
          "content": {"application/json": {"schema": {"type": "object"}}}
        },
        "responses": {"204": {"description": "OK"}}
      }
    },
    "/override": {
      "post": {
        "operationId": "override",
        "parameters": [
          {"name": "X-HTTP-Method-Override", "in": "header", "required": true, "schema": {"type": "string"}}
        ],
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
let calls = 0;
const dispatched = [];
const fetch = async (_input, init) => {
  calls++;
  dispatched.push(new Headers(init.headers));
  return new Response(null, { status: 204 });
};

const api = createClient({ baseURL: "https://api.example.test", fetch });
await api.$operations.oauth({ body: {}, headerParams: { Origin: "https://caller.example" } });
if (dispatched.at(-1)?.get("Origin") !== "https://caller.example") throw new Error("typed Origin was not dispatched");
await api.$operations.oauth({ body: {} });
if (dispatched.at(-1)?.has("Origin")) throw new Error("omitted Origin was synthesized by the SDK");

await api.$operations.override({ headerParams: { "X-HTTP-Method-Override": "PATCH" } });
if (dispatched.at(-1)?.get("X-HTTP-Method-Override") !== "PATCH") throw new Error("safe method override was not dispatched");
await api.$operations.override(
  { headerParams: { "X-HTTP-Method-Override": "PATCH, TRACE" } },
  { headers: { Origin: "https://raw.example", "Sec-Fetch-Site": "same-origin" } },
);
if (dispatched.at(-1)?.get("X-HTTP-Method-Override") !== "PATCH, TRACE") throw new Error("conditional method value was not delegated");
if (dispatched.at(-1)?.get("Origin") !== "https://raw.example") throw new Error("raw Origin was not dispatched");
if (dispatched.at(-1)?.get("Sec-Fetch-Site") !== "same-origin") throw new Error("raw Sec header was not dispatched");

const transport = createClient({
  baseURL: "https://api.example.test",
  transport: {
    fetch: async (_input, init) => {
      const headers = new Headers(init.headers);
      if (headers.has("Origin")) throw new Error("SDK synthesized Origin before the transport wrapper");
      headers.set("Origin", "https://transport.example");
      if (headers.get("Origin") !== "https://transport.example") throw new Error("transport did not inject Origin");
      return new Response(null, { status: 204 });
    },
  },
});
await transport.$operations.oauth({ body: {} });
if (calls !== 4) throw new Error("request dispatch count mismatch: " + calls);
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute Fetch-managed request-header runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeEnvelopeProjectionKeepsCompleteRawBody(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0","info":{"title":"Envelope runtime","version":"1"},
  "paths":{"/value":{"get":{
    "operationId":"getValue",
    "x-envelope":"data",
    "responses":{"200":{"description":"OK","content":{"application/json":{"schema":{
      "type":"object","required":["data","meta"],
      "properties":{
        "data":{"type":"string"},
        "meta":{"type":"object","required":["trace"],"properties":{"trace":{"type":"string"}}}
      }
    }}}}}
  }}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	probe := `import { createClient } from "./index.js"
declare const api: ReturnType<typeof createClient>
const value: Promise<string> = api.$operations.getValue()
const raw = await api.$operations.getValue.raw()
const trace: string = raw.data.meta.trace
void value
void trace
`
	output := compileTypeScriptArtifactsWithProbe(t, document, "envelope.probe.ts", probe)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => new Response(JSON.stringify({ data: "value", meta: { trace: "trace-1" } }), { status: 200, headers: { "content-type": "application/json" } }) });
if (await api.$operations.getValue() !== "value") throw new Error("ordinary envelope output was not projected");
const raw = await api.$operations.getValue.raw();
if (raw.data.data !== "value" || raw.data.meta.trace !== "trace-1") throw new Error("raw envelope body was projected");
`
	if outputBytes, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute envelope runtime test: %v\n%s", err, outputBytes)
	}
}

func TestRuntimeSortProjectionUsesParameterEnumBeforeStandardSerialization(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0","info":{"title":"Sort runtime","version":"1"},
  "paths":{"/items":{"get":{
    "operationId":"listItems",
    "x-concurrency":"required",
    "x-idempotency":"required",
    "parameters":[{
      "name":"order","in":"query",
      "x-filter":{"operator":"custom-metadata"},
      "x-sort":{"format":"field-direction"},
      "schema":{"type":"array","items":{"type":"string","enum":["createdAt:asc","createdAt:desc","name:asc"],"pattern":"^[A-Za-z]+:(asc|desc)$"}}
    }],
    "responses":{"204":{"description":"OK"}}
  }}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	probe := `import { createClient } from "./index.js"
declare const api: ReturnType<typeof createClient>
api.$operations.listItems({ query: { order: [{ field: "createdAt", direction: "desc" }] } })
// @ts-expect-error enum correlation rejects undeclared pairs
api.$operations.listItems({ query: { order: [{ field: "name", direction: "desc" }] } })
`
	output := compileTypeScriptArtifactsWithProbe(t, document, "sort.probe.ts", probe)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let calls = 0;
const api = createClient({ baseURL: "https://api.example.test", fetch: async (input) => {
  calls++;
  const values = new URL(String(input)).searchParams.getAll("order");
  if (JSON.stringify(values) !== JSON.stringify(["createdAt:desc"])) throw new Error("sort wire value mismatch: " + JSON.stringify(values));
  return new Response(null, { status: 204 });
} });
await api.$operations.listItems({ query: { order: [{ field: "createdAt", direction: "desc" }] } });
try {
  await api.$operations.listItems({ query: { order: [{ field: "name", direction: "desc" }] } });
  throw new Error("invalid structured sort was accepted");
} catch (error) {
  if (String(error).includes("invalid structured sort was accepted")) throw error;
}
if (calls !== 1) throw new Error("invalid sort reached fetch");
`
	if outputBytes, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute sort runtime test: %v\n%s", err, outputBytes)
	}
}

func TestGeneratedPublicIdentityMapsPreserveExactAndPrototypeSensitiveKeys(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Exact identities","version":"1"},
  "paths":{
    "/source-one":{"get":{"operationId":"source-op","responses":{"200":{"description":"OK","links":{
      "next-step":{"operationId":"getTarget"},
      "next_step":{"operationId":"getTarget"},
      "__proto__":{"operationId":"getTarget"},
      "constructor":{"operationId":"getTarget"}
    }}}}},
    "/source-two":{"get":{"operationId":"source_op","responses":{"200":{"description":"OK","links":{"next-step":{"operationId":"getTarget"}}}}}},
    "/target":{"get":{"operationId":"getTarget","responses":{"204":{"description":"OK"}}}},
    "/stream-one":{"get":{"operationId":"tail-log","responses":{"200":{"description":"OK","content":{"application/x-ndjson":{"itemSchema":{"type":"string"}}}}}}},
    "/stream-two":{"get":{"operationId":"tail_log","responses":{"200":{"description":"OK","content":{"application/x-ndjson":{"itemSchema":{"type":"string"}}}}}}},
    "/stream-prototype":{"get":{"operationId":"__proto__","responses":{"200":{"description":"OK","content":{"application/x-ndjson":{"itemSchema":{"type":"string"}}}}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let targets = 0;
const api = createClient({ baseURL: "https://api.example.test", fetch: async (input) => {
  const path = new URL(String(input)).pathname;
  if (path === "/source-one" || path === "/source-two") return new Response(null, { status: 200 });
  if (path === "/target") { targets++; return new Response(null, { status: 204 }); }
  if (path.startsWith("/stream-")) return new Response('"item"\n', { status: 200, headers: { "content-type": "application/x-ndjson" } });
  throw new Error("unexpected path " + path);
} });
if (JSON.stringify(Object.keys(api.$links).sort()) !== JSON.stringify(["source-op", "source_op"])) throw new Error("link source identities changed");
if (JSON.stringify(Object.keys(api.$links["source-op"]).sort()) !== JSON.stringify(["__proto__", "constructor", "next-step", "next_step"])) throw new Error("link identities changed");
if (!Object.prototype.hasOwnProperty.call(api.$links["source-op"], "__proto__")) throw new Error("prototype-sensitive link is not an own property");
const source = await api.$operations["source-op"].raw();
for (const name of ["next-step", "next_step", "__proto__", "constructor"]) await api.$links["source-op"][name](source);
if (targets !== 4) throw new Error("exact-key links did not dispatch");
if (JSON.stringify(Object.keys(api.$streams).sort()) !== JSON.stringify(["__proto__", "tail-log", "tail_log"])) throw new Error("stream identities changed");
for (const name of ["tail-log", "tail_log", "__proto__"]) {
  const values = [];
  for await (const value of api.$streams[name]()) values.push(value);
  if (values.join(",") !== "item") throw new Error("stream identity did not dispatch: " + name);
}
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute exact public identity runtime test: %v\n%s", err, output)
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
const sourceInput = { query: { page: 3 }, headerParams: { "x-source": "request-trace" }, cookieParams: { session: "session-id" }, body: { id: "source-id" } };
const response = await api.$operations.createSource.raw(sourceInput);
await api.$links.createSource.follow(response, { sourceInput });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript request-expression Link runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedResourceOperationComposesWithChildNamespace(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0",
  "info":{"title":"Resource composition","version":"1"},
  "paths":{
    "/users":{"get":{"operationId":"listUsers","responses":{"204":{"description":"OK"}}}},
    "/users/list":{"get":{"operationId":"getList","responses":{"204":{"description":"OK"}}}},
    "/users/{id}/profile":{"get":{"operationId":"getProfile","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"204":{"description":"OK"}}}},
    "/users/{userID}/settings":{"get":{"operationId":"getSettings","parameters":[{"name":"userID","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"204":{"description":"OK"}}}}
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
  seen.push(new URL(String(input)).pathname);
  return new Response(null, { status: 204 });
} });
await api.users.list();
await api.users.list.get();
await api.users("profile-id").profile.get();
await api.users("settings-id").settings.get();
if (typeof api.users.list.raw !== "function" || seen.join(",") !== "/users,/users/list,/users/profile-id/profile,/users/settings-id/settings") throw new Error("callable resource namespace mismatch");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute callable resource namespace test: %v\n%s", err, output)
	}
}

func TestGeneratedResourceCollisionFallbackMatrixCompilesAndDispatches(t *testing.T) {
	plain := func(operationID, method, path string) ir.Operation {
		return ir.Operation{
			OperationID: operationID,
			Method:      method,
			Path:        path,
			Raw:         map[string]any{"responses": map[string]any{"204": map[string]any{"description": "OK"}}},
		}
	}
	parameterized := func(operationID, path, parameter string, schema map[string]any) ir.Operation {
		operation := pathOperation(operationID, "GET", path, parameter, schema)
		operation.Raw["responses"] = map[string]any{"204": map[string]any{"description": "OK"}}
		return operation
	}
	listWidgets := plain("listWidgets", "GET", "/widgets")
	listWidgets.Raw = map[string]any{
		"x-pagination": "cursor",
		"parameters": []any{
			map[string]any{"name": "cursor", "in": "query", "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "minimum": float64(1)}},
		},
		"responses": map[string]any{"200": map[string]any{
			"description": "OK",
			"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{
				"type":     "object",
				"required": []any{"items", "pagination"},
				"properties": map[string]any{
					"items": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"pagination": map[string]any{"type": "object", "properties": map[string]any{
						"nextCursor": map[string]any{"type": []any{"string", "null"}},
					}},
				},
			}}},
		}},
	}
	document := &ir.Document{Operations: []ir.Operation{
		plain("getModern", "GET", "/foo-bar"),
		plain("getLegacy", "GET", "/foo_bar"),
		plain("listPeople", "GET", "/people"),
		parameterized("getListedPerson", "/people/list/{id}", "id", map[string]any{"type": "string"}),
		plain("getRoot", "GET", "/"),
		plain("getGet", "GET", "/get"),
		parameterized("getTenant", "/{tenant}", "tenant", map[string]any{"type": "string"}),
		func() ir.Operation {
			operation := parameterized("getAlias", "/repeat/{id}/aliases/{id}", "id", map[string]any{"type": "string"})
			operation.PathParameterOrder = []string{"id", "id"}
			return operation
		}(),
		parameterized("getProfile", "/teams/{id}/profile", "id", map[string]any{"type": "string"}),
		parameterized("getSettings", "/teams/{teamID}/settings", "teamID", map[string]any{"type": "integer"}),
		listWidgets,
		plain("getPaginate", "GET", "/widgets/paginate"),
		plain("getRaw", "GET", "/widgets/list/raw"),
	}}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	client := string(artifactByPath(t, artifacts, "generated/client.ts"))
	routesByID := make(map[string]string, len(document.Operations))
	for _, operation := range document.Operations {
		routesByID[operation.OperationID] = operation.Method + " " + operation.Path
	}
	for _, operationID := range []string{
		"getModern", "getLegacy", "listPeople", "getTenant", "getAlias",
		"getProfile", "getSettings", "getPaginate", "getRaw",
	} {
		start := strings.Index(client, `readonly `+quoteTS(routesByID[operationID])+`: {`)
		if start < 0 {
			t.Fatalf("operation %q missing from catalog:\n%s", operationID, client)
		}
		entry := client[start:]
		if end := strings.Index(entry, "\n  }"); end >= 0 {
			entry = entry[:end]
		}
		if !strings.Contains(entry, "readonly resourceCall: never") {
			t.Fatalf("operation %q retained ambiguous resource call:\n%s", operationID, entry)
		}
	}
	probe := `import { createClient } from "./index.js"
declare const api: ReturnType<typeof createClient>
api.$operations.listPeople()
api.people.list("person").get()
api.get()
api.getValue.get()
// @ts-expect-error the colliding list operation shortcut is suppressed in favor of the parameter binder
api.people.list()
`
	output := compileTypeScriptArtifactsWithProbe(t, document, "resource-collisions.probe.ts", probe)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const paths = [];
const api = createClient({ baseURL: "https://api.example.test", fetch: async (input) => {
  paths.push(new URL(String(input)).pathname);
  return new Response(null, { status: 204 });
} });
await api.$operations.getModern();
await api.$operations.getLegacy();
await api.$operations.listPeople();
await api.people.list("person").get();
await api.get();
await api.getValue.get();
await api.$operations.getTenant({ path: { tenant: "acme" } });
await api.$operations.getAlias({ path: { id: "same" } });
await api.$operations.getProfile({ path: { id: "team" } });
await api.$operations.getSettings({ path: { teamID: 7 } });
await api.$operations.getPaginate();
await api.$operations.getRaw();
const expected = ["/foo-bar","/foo_bar","/people","/people/list/person","/","/get","/acme","/repeat/same/aliases/same","/teams/team/profile","/teams/7/settings","/widgets/paginate","/widgets/list/raw"];
if (JSON.stringify(paths) !== JSON.stringify(expected)) throw new Error("resource fallback dispatch mismatch: " + JSON.stringify(paths));
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute resource collision fallback matrix: %v\n%s", err, output)
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

func TestGeneratedResponseLinksDelegateEnvironmentControlledRequestHeaders(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.1", "info":{"title":"Transport-native request header Links","version":"1"},
  "components":{
    "links":{"ManagedOrigin":{"operationId":"getOrigin","parameters":{"Origin":"https://component.example"}}},
    "responses":{"ManagedResponse":{"description":"OK","links":{"follow":{"operationId":"getOrigin","parameters":{"Origin":"https://response.example"}}}}}
  },
  "paths":{
    "/source":{"get":{"operationId":"getSource","parameters":[
      {"name":"Origin","in":"header","required":true,"schema":{"type":"string"}}
    ],"responses":{"204":{"description":"OK","links":{
      "trace":{"operationId":"getTrace","parameters":{"X-Trace":"$request.header.Origin"}},
      "origin":{"operationId":"getOrigin","parameters":{"Origin":"https://literal.example"}}
    }}}}},
    "/component-source":{"get":{"operationId":"getComponentSource","responses":{"204":{"description":"OK","links":{"follow":{"$ref":"#/components/links/ManagedOrigin"}}}}}},
    "/response-source":{"get":{"operationId":"getResponseSource","responses":{"204":{"$ref":"#/components/responses/ManagedResponse"}}}},
    "/trace":{"get":{"operationId":"getTrace","parameters":[
      {"name":"X-Trace","in":"header","schema":{"type":"string"}}
    ],"responses":{"204":{"description":"OK"}}}},
    "/origin":{"get":{"operationId":"getOrigin","parameters":[
      {"name":"Origin","in":"header","required":true,"schema":{"type":"string"}}
    ],"responses":{"204":{"description":"OK"}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, diagnostics, err := prepareSourcePlan(document, false); err != nil {
		t.Fatal(err)
	} else if len(diagnostics) != 0 {
		t.Fatalf("environment-controlled Link diagnostics = %#v", diagnostics)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const seen = [];
const api = createClient({
  baseURL: "https://api.example.test",
  fetch: async (input, init) => {
    seen.push([new URL(String(input)).pathname, Object.fromEntries(new Headers(init.headers).entries())]);
    return new Response(null, { status: 204 });
  },
});
const explicit = await api.$operations.getSource.raw({ headerParams: { Origin: "https://source.example" } });
await api.$links.getSource.trace(explicit, { sourceInput: { headerParams: { Origin: "https://source.example" } } });
await api.$links.getSource.origin(explicit);
const omitted = await api.$operations.getSource.raw();
await api.$links.getSource.trace(omitted);
const component = await api.$operations.getComponentSource.raw();
await api.$links.getComponentSource.follow(component);
const response = await api.$operations.getResponseSource.raw();
await api.$links.getResponseSource.follow(response);
const expected = [
  ["/source", { origin: "https://source.example" }],
  ["/trace", { "x-trace": "https://source.example" }],
  ["/origin", { origin: "https://literal.example" }],
  ["/source", {}],
  ["/trace", {}],
  ["/component-source", {}],
  ["/origin", { origin: "https://component.example" }],
  ["/response-source", {}],
  ["/origin", { origin: "https://response.example" }],
];
if (JSON.stringify(seen) !== JSON.stringify(expected)) throw new Error("environment-controlled Link dispatch mismatch: " + JSON.stringify(seen));
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute environment-controlled Link runtime test: %v\n%s", err, output)
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
await api.$operations.findRecords({ query: { filter: { id: "query" } }, headerParams: { "x-filter": { id: "header" } } });
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
let calls = 0;
const codec = { encodeParameter: async (value) => "cbor:" + value.id };
const api = createClient({ baseURL: "https://api.example.test", codecs: { "application/cbor": codec }, transport: { capabilities: { cookieJar: true }, fetch: async (input, init) => {
  calls++;
  const url = new URL(String(input));
  if (url.pathname !== "/records/cbor%3Apath" || url.searchParams.get("filter") !== "cbor:query" || !url.search.includes("cbor%3Awhole")) throw new Error("custom path/query codec was not applied");
  const headers = new Headers(init.headers);
  if (headers.get("x-filter") !== "cbor:header" || headers.get("cookie") !== "crumb=cbor%3Acookie") throw new Error("custom header/cookie codec was not applied");
  return new Response(null, { status: 204 });
} } });
await api.$operations.getRecord({ path: { record: { id: "path" } }, query: { filter: { id: "query" } }, querystring: { whole: { id: "whole" } }, headerParams: { "X-Filter": { id: "header" } }, cookieParams: { crumb: { id: "cookie" } } });
try { await api.$operations.getRecord({ path: { record: { id: "path" } }, querystring: { whole: {} } }); throw new Error("invalid async querystring value was accepted"); }
catch (error) { if (!String(error).includes("required property id") && !String(error.cause).includes("required property id")) throw error; }
if (calls !== 1) throw new Error("invalid async querystring value reached fetch");
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

func TestObjectParametersOmitUndefinedOptionalProperties(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0","info":{"title":"Undefined object properties","version":"1"},
  "paths":{"/objects/{id}":{"get":{"operationId":"getObject","parameters":[
    {"name":"id","in":"path","required":true,"style":"matrix","explode":true,"schema":{"type":"object","required":["keep"],"properties":{"keep":{"type":"string"},"omit":{"type":"string"}}}},
    {"name":"id","in":"header","required":true,"style":"simple","explode":true,"schema":{"type":"object","required":["keep"],"properties":{"keep":{"type":"string"},"omit":{"type":"string"}}}},
    {"name":"id","in":"cookie","required":true,"style":"form","explode":true,"schema":{"type":"object","required":["keep"],"properties":{"keep":{"type":"string"},"omit":{"type":"string"}}}}
  ],"responses":{"204":{"description":"OK"}}}}}
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
  const headers = new Headers(init.headers);
  if (url.pathname !== "/objects/;keep=path" || url.pathname.includes("undefined")) throw new Error("path serialized undefined: " + url.pathname);
  if (headers.get("id") !== "keep=header" || headers.get("id")?.includes("undefined")) throw new Error("header serialized undefined: " + headers.get("id"));
  if (headers.get("cookie") !== "keep=cookie" || headers.get("cookie")?.includes("undefined")) throw new Error("cookie serialized undefined: " + headers.get("cookie"));
  return new Response(null, { status: 204 });
} } });
await api.$operations.getObject({
  path: { id: { keep: "path", omit: undefined } },
  headerParams: { id: { keep: "header", omit: undefined } },
  cookieParams: { id: { keep: "cookie", omit: undefined } },
});
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute undefined object-parameter test: %v\n%s", err, output)
	}
}

func TestGeneratedClientKeepsSameParameterNameAcrossAllLocations(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0","info":{"title":"Repeated parameter names","version":"1"},
  "paths":{"/items/{id}":{"get":{"operationId":"getItem","parameters":[
    {"name":"id","in":"path","required":true,"schema":{"type":"string"}},
    {"name":"id","in":"query","required":true,"schema":{"type":"integer"}},
    {"name":"id","in":"querystring","required":true,"content":{"text/plain":{"schema":{"type":"string"}}}},
    {"name":"id","in":"header","required":true,"schema":{"type":"boolean"}},
    {"name":"id","in":"cookie","required":true,"schema":{"type":"string"}}
  ],"responses":{"204":{"description":"OK"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	probe := `import { createClient } from "./index.js"
declare const api: ReturnType<typeof createClient>
api.$operations.getItem({
  path: { id: "path-id" },
  query: { id: 2 },
  querystring: { id: "raw=value" },
  headerParams: { id: true },
  cookieParams: { id: "cookie-id" },
})
// @ts-expect-error path id remains a string
api.$operations.getItem({ path: { id: 1 }, query: { id: 2 }, querystring: { id: "raw" }, headerParams: { id: true }, cookieParams: { id: "cookie" } })
// @ts-expect-error query id remains a number
api.$operations.getItem({ path: { id: "path" }, query: { id: "2" }, querystring: { id: "raw" }, headerParams: { id: true }, cookieParams: { id: "cookie" } })
// @ts-expect-error querystring id remains a string
api.$operations.getItem({ path: { id: "path" }, query: { id: 2 }, querystring: { id: 3 }, headerParams: { id: true }, cookieParams: { id: "cookie" } })
// @ts-expect-error header id remains a boolean
api.$operations.getItem({ path: { id: "path" }, query: { id: 2 }, querystring: { id: "raw" }, headerParams: { id: "true" }, cookieParams: { id: "cookie" } })
// @ts-expect-error cookie id remains a string
api.$operations.getItem({ path: { id: "path" }, query: { id: 2 }, querystring: { id: "raw" }, headerParams: { id: true }, cookieParams: { id: false } })
`
	output := compileTypeScriptArtifactsWithProbe(t, document, "repeated-parameters.probe.ts", probe)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const api = createClient({ baseURL: "https://api.example.test", transport: { capabilities: { cookieJar: true }, fetch: async (input, init) => {
  const url = new URL(String(input));
  const headers = new Headers(init.headers);
  if (url.pathname !== "/items/path-id") throw new Error("path parameter collapsed: " + url.pathname);
  if (url.search !== "?id=2&raw%3Dvalue") throw new Error("query parameters collapsed: " + url.search);
  if (headers.get("id") !== "true") throw new Error("header parameter collapsed");
  if (headers.get("cookie") !== "id=cookie-id") throw new Error("cookie parameter collapsed");
  return new Response(null, { status: 204 });
} } });
await api.$operations.getItem({
  path: { id: "path-id" },
  query: { id: 2 },
  querystring: { id: "raw=value" },
  headerParams: { id: true },
  cookieParams: { id: "cookie-id" },
});
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute repeated parameter-name runtime test: %v\n%s", err, output)
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
for await (const event of api.$streams.tailLogs()) events.push(event.event_id);
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
for await (const event of api.$streams.tailJSONLines()) events.push(event.event_id);
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
for await (const event of api.$streams.tailCustomEvents()) events.push(event.event_id);
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
async function* events() { yield { event_id: "one" }; yield { event_id: "two" }; }
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
for await (const frame of api.$streams.tailFrames()) frames.push(frame.frame_id);
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
if (JSON.stringify(bundle) !== JSON.stringify([{ bundle_id: "one" }, "ready"])) throw new Error("complete multipart response did not decode positional parts");
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
const result = await api.$operations.roundTripNested({ body: [[{ top_id: "in" }], [{ inner_id: "in" }, "queued"]] });
if (JSON.stringify(result) !== JSON.stringify([[{ top_id: "out" }], [{ inner_id: "out" }, "ready"]])) throw new Error("nested multipart response was not decoded");
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

func TestBooleanFalseBinarySchemasRejectRequestAndResponseValues(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Never binary","version":"1"},"paths":{"/input":{"post":{"operationId":"createBinary","requestBody":{"required":true,"content":{"application/octet-stream":{"schema":false}}},"responses":{"204":{"description":"No Content"}}}},"/output":{"get":{"operationId":"getBinary","responses":{"200":{"description":"Impossible","content":{"application/octet-stream":{"schema":false}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let fetched = 0;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { fetched++; return new Response(new Uint8Array([1]), { status: 200, headers: { "content-type": "application/octet-stream" } }); } });
try { await api.$operations.createBinary({ body: new Uint8Array([1]) }); throw new Error("false binary request schema accepted a value"); }
catch (error) { if (!String(error).includes("schema is false") && !String(error.cause).includes("schema is false")) throw error; }
if (fetched !== 0) throw new Error("false binary request reached fetch");
try { await api.$operations.getBinary(); throw new Error("false binary response schema accepted a value"); }
catch (error) { if (!String(error).includes("schema is false") && !String(error.cause).includes("schema is false")) throw error; }
if (fetched !== 1) throw new Error("binary response fixture did not run");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript false binary schema test: %v\n%s", err, output)
	}
}

func TestOpenAPI30NullableRequestRuntimeMatchesGeneratedType(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.0.3","info":{"title":"Nullable","version":"1"},"paths":{"/value":{"post":{"operationId":"createValue","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"string","nullable":true}}}},"responses":{"204":{"description":"No Content"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let fetched = 0;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { fetched++; return new Response(null, { status: 204 }); } });
await api.$operations.createValue({ body: null });
try { await api.$operations.createValue({ body: 1 }); throw new Error("invalid nullable value accepted"); }
catch (error) { if (!String(error).includes("expected string | null") && !String(error.cause).includes("expected string | null")) throw error; }
if (fetched !== 1) throw new Error("nullable request validation did not run before fetch");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute OpenAPI 3.0 nullable runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeRejectsUnevaluatedPropertiesAndItemsBeforeFetch(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
	  "openapi":"3.1.0","info":{"title":"Unevaluated","version":"1"},
	  "paths":{
	    "/object":{"post":{"operationId":"createObject","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","allOf":[{"properties":{"id":{"type":"string"}}},{"properties":{"name":{"type":"string"}}}],"unevaluatedProperties":false}}}},"responses":{"204":{"description":"No Content"}}}},
	    "/array":{"post":{"operationId":"createArray","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"array","prefixItems":[{"type":"string"}],"additionalProperties":false,"unevaluatedItems":false}}}},"responses":{"204":{"description":"No Content"}}}},
	    "/referenced":{"post":{"operationId":"createReferenced","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Base","unevaluatedProperties":false}}}},"responses":{"204":{"description":"No Content"}}}},
	    "/dependent":{"post":{"operationId":"createDependent","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","properties":{"enabled":{"type":"boolean"}},"dependentSchemas":{"enabled":{"properties":{"label":{"type":"string"}}}},"unevaluatedProperties":false}}}},"responses":{"204":{"description":"No Content"}}}}
	  },
	  "components":{"schemas":{"Base":{"type":"object","properties":{"id":{"type":"string"}}}}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let fetched = 0;
const api = createClient({ baseURL: "https://api.example.test", fetch: async () => { fetched++; return new Response(null, { status: 204 }); } });
for (const [operation, input, expected] of [["createObject", { body: { id: "one", name: "widget", extra: true } }, "unexpected unevaluated property extra"], ["createArray", { body: ["one", "extra"] }, "unexpected unevaluated item 1"]]) {
  try { await api.$operations[operation](input); throw new Error("unevaluated value accepted"); }
  catch (error) { if (!String(error).includes(expected) && !String(error.cause).includes(expected)) throw error; }
}
await api.$operations.createReferenced({ body: { id: "one" } });
await api.$operations.createDependent({ body: { enabled: true, label: "ok" } });
if (fetched !== 2) throw new Error("valid referenced/dependent annotations were not merged");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript unevaluated runtime test: %v\n%s", err, output)
	}
}

func TestDiscriminatorRuntimeSelectsMappedBranch(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Pets","version":"1"},"paths":{"/pet":{"get":{"operationId":"getPet","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Pet"}}}}}}}},"components":{"schemas":{"Pet":{"oneOf":[{"$ref":"#/components/schemas/Cat"},{"$ref":"#/components/schemas/Dog"}],"discriminator":{"propertyName":"kind","mapping":{"cat":"#%2Fcomponents%2Fschemas%2FCat","dog":"#/components/schemas/Dog"}}},"Cat":{"type":"object","required":["kind","lives"],"properties":{"kind":{"const":"cat"},"lives":{"type":"integer"}}},"Dog":{"type":"object","required":["kind","barks"],"properties":{"kind":{"const":"dog"},"barks":{"type":"boolean"}}}}}}`))
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
if (pet.display_name !== "mystery") throw new Error("default discriminator mapping did not select OtherPet");
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
await api.$operations.searchWholeQuery({ querystring: { form: { term: "widgets", page: 2 } } });
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
await api.$operations.getPreferences({ cookieParams: { prefs: { theme: "dark", event_id: "a%2Fb" } } });
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
if (response.headers["X-Rate-Limit"] !== 42 || response.headers["X-Context"].region !== "kr" || response.headers["X-Event"].event_id !== "event" || response.headers["X-Custom"].event_id !== "client" || response.headers["X-Object"].event_id !== "object" || response.response.headers.get("x-rate-limit") !== "42") throw new Error("typed response header mismatch");
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

func TestRawSequentialResponseKeepsBodyAndCancelsOnHeaderFailure(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0",
  "info":{"title":"Sequential raw response","version":"1"},
  "paths":{"/events":{"get":{
    "operationId":"tailEvents",
    "responses":{"200":{
      "description":"OK",
      "headers":{"X-Stream-ID":{"required":true,"schema":{"type":"string"}}},
      "content":{"application/x-ndjson":{
        "schema":{"type":"array","items":{"type":"string"}},
        "itemSchema":{"type":"string"}
      }}
    }}
  }}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	probe := `import { createClient } from "./index.js"
declare const api: ReturnType<typeof createClient>
const response = await api.$operations.tailEvents.raw()
const data: void = response.data
void data
`
	output := compileTypeScriptArtifactsWithProbe(t, document, "sequential-raw.probe.ts", probe)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let cancelled = 0;
const body = new ReadableStream({
  start(controller) { controller.enqueue(new TextEncoder().encode('"one"\n')); },
  cancel() { cancelled++; }
});
const api = createClient({
  baseURL: "https://api.example.test",
  fetch: async () => new Response(body, { status: 200, headers: { "content-type": "application/x-ndjson" } })
});
await api.$operations.tailEvents.raw().then(
  () => { throw new Error("missing response header was accepted"); },
  (error) => { if (error.code !== "RESPONSE_DECODE_FAILED") throw error; }
);
if (cancelled !== 1) throw new Error("stream was not cancelled exactly once: " + cancelled);
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute raw sequential response cleanup test: %v\n%s", err, output)
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
if (response.headers["Set-Cookie"] !== "session=abc") throw new Error("capable Set-Cookie transport did not decode header");
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
const widget = await api.$operations.createWidget({ body: { widget_id: "input" } });
if (widget.widget_id !== "output") throw new Error("reusable response media type was not decoded");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript reusable-media runtime test: %v\n%s", err, output)
	}
}

func TestGeneratedSecurityRequirementOptionsStayOperationSpecificAcrossCallSurfaces(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0",
  "info":{"title":"Security requirement options","version":"1"},
  "components":{"securitySchemes":{
    "GuestCapability":{"type":"http","scheme":"bearer"},
    "BuyerSessionCookie":{"type":"apiKey","in":"cookie","name":"buyer_session"},
    "BuyerCSRFHeader":{"type":"apiKey","in":"header","name":"X-CSRF-Token"},
    "OperatorKey":{"type":"apiKey","in":"query","name":"operator_key"}
  }},
  "paths":{
    "/checkout":{"post":{"operationId":"mutateCheckout","security":[{"BuyerSessionCookie":[],"BuyerCSRFHeader":[]},{"GuestCapability":[]}],"responses":{"204":{"description":"OK"}}}},
    "/events":{"get":{"operationId":"watchEvents","security":[{"BuyerSessionCookie":[],"BuyerCSRFHeader":[]},{"GuestCapability":[]}],"responses":{"200":{"description":"OK","content":{"application/x-ndjson":{"itemSchema":{"type":"string"}}}}}}},
    "/items":{"get":{"operationId":"listItems","security":[{"BuyerSessionCookie":[],"BuyerCSRFHeader":[]},{"GuestCapability":[]}],"parameters":[{"name":"cursor","in":"query","schema":{"type":"string"}},{"name":"limit","in":"query","schema":{"type":"integer","minimum":1}}],"responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"type":"object","properties":{"items":{"type":"array","items":{"type":"string"}},"pagination":{"type":"object","properties":{"nextCursor":{"type":["string","null"]}}}}}}}}},"x-pagination":"cursor"}},
    "/operator":{"get":{"operationId":"getOperator","security":[{"OperatorKey":[]}],"responses":{"204":{"description":"OK"}}}},
    "/public":{"get":{"operationId":"getPublic","security":[],"responses":{"200":{"description":"OK","links":{"checkout":{"operationId":"mutateCheckout"}}}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	probe := `import { createClient, type Routes } from "./index.js"
declare const api: ReturnType<typeof createClient>
declare const source: Routes["GET /public"]["rawResponse"]
api.$operations.mutateCheckout({ securityRequirement: "GuestCapability", authorization: "Bearer guest" })
api.$operations.mutateCheckout.raw({ securityRequirement: "BuyerCSRFHeader__BuyerSessionCookie", credentials: "include", csrfToken: "csrf" })
api.$routes["POST /checkout"]({ securityRequirement: "GuestCapability", authorization: "Bearer guest" })
api.checkout.post({ securityRequirement: "GuestCapability", authorization: "Bearer guest" })
api.$streams.watchEvents({ securityRequirement: "GuestCapability", authorization: "Bearer guest" })
api.$operations.listItems.paginate({ query: {} }, { securityRequirement: "GuestCapability", authorization: "Bearer guest" })
api.$links.getPublic.checkout(source, { options: { securityRequirement: "GuestCapability", authorization: "Bearer guest" } })
// @ts-expect-error unknown requirement ID
api.$operations.mutateCheckout({ securityRequirement: "Unknown" })
// @ts-expect-error another operation's requirement ID
api.$operations.mutateCheckout({ securityRequirement: "OperatorKey" })
// @ts-expect-error unsecured operations do not expose securityRequirement
api.$operations.getPublic({ securityRequirement: "GuestCapability" })
// @ts-expect-error stream options retain the target operation's requirement IDs
api.$streams.watchEvents({ securityRequirement: "OperatorKey" })
// @ts-expect-error Link target options retain the target operation's requirement IDs
api.$links.getPublic.checkout(source, { options: { securityRequirement: "OperatorKey" } })
`
	compileTypeScriptArtifactsWithProbe(t, document, "security-requirements.probe.ts", probe)
}

func TestRuntimeSelectsAndSatisfiesOpenAPISecurityRequirementsConsistently(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0",
  "info":{"title":"Security requirement planner","version":"1"},
  "components":{"securitySchemes":{
    "GuestCapability":{"type":"http","scheme":"bearer"},
    "BuyerSessionCookie":{"type":"apiKey","in":"cookie","name":"buyer_session"},
    "BuyerCSRFHeader":{"type":"apiKey","in":"header","name":"X-CSRF-Token"}
  }},
  "paths":{
    "/checkout":{"post":{"operationId":"mutateCheckout","security":[{"BuyerSessionCookie":[],"BuyerCSRFHeader":[]},{"GuestCapability":[]}],"responses":{"204":{"description":"OK"}}}},
    "/optional":{"get":{"operationId":"getOptional","security":[{}, {"GuestCapability":[]}],"responses":{"204":{"description":"OK"}}}},
    "/events":{"get":{"operationId":"watchEvents","security":[{"GuestCapability":[]}],"responses":{"200":{"description":"OK","content":{"application/x-ndjson":{"itemSchema":{"type":"string"}}}}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const calls = [];
const fetch = async (input, init) => {
  const headers = new Headers(init.headers);
  calls.push({ path: new URL(String(input)).pathname, authorization: headers.get("authorization"), csrf: headers.get("x-csrf-token"), credentials: init.credentials });
  return new Response(null, { status: 204 });
};

let providerCalls = 0;
const providerSelectedGuest = createClient({
  baseURL: "https://api.example.test",
  credentials: "include",
  securityProvider: ({ requirements, selectedRequirement }) => {
    providerCalls++;
    if (selectedRequirement !== undefined) throw new Error("unexpected explicit requirement");
    return { requirement: requirements.GuestCapability, credentials: { GuestCapability: { kind: "http-bearer", token: "provider" } } };
  },
  fetch,
});
await providerSelectedGuest.$operations.mutateCheckout();
if (providerCalls !== 1) throw new Error("ambient cookies bypassed the security provider");

const explicitGuest = createClient({
  baseURL: "https://api.example.test",
  securityProvider: () => { throw new Error("fully satisfied explicit requirement called provider"); },
  fetch,
});
await explicitGuest.$operations.mutateCheckout({ securityRequirement: "GuestCapability", authorization: "Bearer explicit" });

const explicitBuyer = createClient({
  baseURL: "https://api.example.test",
  securityProvider: () => { throw new Error("fully satisfied cookie requirement called provider"); },
  fetch,
});
await explicitBuyer.$operations.mutateCheckout({
  securityRequirement: "BuyerCSRFHeader__BuyerSessionCookie",
  credentials: "include",
  csrfToken: "csrf",
});

let matchingProviderCalls = 0;
const matchingProvider = createClient({
  baseURL: "https://api.example.test",
  authorization: "Bearer same",
  securityProvider: ({ requirements }) => {
    matchingProviderCalls++;
    return { requirement: requirements.GuestCapability, credentials: { GuestCapability: { kind: "http-bearer", token: "same" } } };
  },
  fetch,
});
await matchingProvider.$operations.mutateCheckout();
if (matchingProviderCalls !== 1) throw new Error("provider was not authoritative without an explicit request selection");

await createClient({ baseURL: "https://api.example.test", fetch }).$operations.getOptional();
let optionalProviderCalls = 0;
await createClient({
  baseURL: "https://api.example.test",
  securityProvider: ({ requirements }) => {
    optionalProviderCalls++;
    return { requirement: requirements.GuestCapability, credentials: { GuestCapability: { kind: "http-bearer", token: "optional-provider" } } };
  },
  fetch,
}).$operations.getOptional();
if (optionalProviderCalls !== 1) throw new Error("anonymous access bypassed the security provider");

const noFetch = async () => { throw new Error("invalid security request reached fetch"); };
const expectCode = async (promise, code) => promise.then(
  () => { throw new Error("expected " + code); },
  (error) => { if (error.code !== code) throw error; },
);

await expectCode(
  createClient({ baseURL: "https://api.example.test", fetch: noFetch }).$operations.mutateCheckout({ securityRequirement: "Unknown" }),
  "SECURITY_REQUIREMENT_INVALID",
);
await expectCode(
  createClient({ baseURL: "https://api.example.test", fetch: noFetch }).$operations.watchEvents.stream({ securityRequirement: "Unknown" })[Symbol.asyncIterator]().next(),
  "SECURITY_REQUIREMENT_INVALID",
);
await expectCode(
  createClient({
    baseURL: "https://api.example.test",
    securityProvider: ({ requirements, selectedRequirement }) => {
      if (selectedRequirement !== requirements.BuyerCSRFHeader__BuyerSessionCookie) throw new Error("explicit requirement missing from provider context");
      return { requirement: requirements.GuestCapability, credentials: {} };
    },
    fetch: noFetch,
  }).$operations.mutateCheckout({ securityRequirement: "BuyerCSRFHeader__BuyerSessionCookie" }),
  "SECURITY_REQUIREMENT_INVALID",
);
await expectCode(
  createClient({ baseURL: "https://api.example.test", credentials: "include", authorization: "Bearer both", fetch: noFetch }).$operations.mutateCheckout({ csrfToken: "csrf" }),
  "SECURITY_REQUIREMENT_REQUIRED",
);
await expectCode(
  createClient({ baseURL: "https://api.example.test", authorization: "Bearer ambiguous", fetch: noFetch }).$operations.getOptional(),
  "SECURITY_REQUIREMENT_REQUIRED",
);
await expectCode(
  createClient({ baseURL: "https://api.example.test", fetch: noFetch }).$operations.mutateCheckout(),
  "SECURITY_CREDENTIALS_REQUIRED",
);
await expectCode(
  createClient({
    baseURL: "https://api.example.test",
    authorization: "Bearer dedicated",
    securityProvider: ({ requirements }) => ({ requirement: requirements.GuestCapability, credentials: { GuestCapability: { kind: "http-bearer", token: "different" } } }),
    fetch: noFetch,
  }).$operations.mutateCheckout(),
  "SECURITY_CREDENTIALS_INVALID",
);
await expectCode(
  createClient({
    baseURL: "https://api.example.test",
    securityProvider: ({ requirements }) => ({ requirement: requirements.GuestCapability, credentials: {} }),
    fetch: noFetch,
  }).$operations.mutateCheckout(),
  "SECURITY_CREDENTIALS_INVALID",
);

if (JSON.stringify(calls) !== JSON.stringify([
  { path: "/checkout", authorization: "Bearer provider", csrf: null, credentials: "include" },
  { path: "/checkout", authorization: "Bearer explicit", csrf: null },
  { path: "/checkout", authorization: null, csrf: "csrf", credentials: "include" },
  { path: "/checkout", authorization: "Bearer same", csrf: null },
  { path: "/optional", authorization: null, csrf: null },
  { path: "/optional", authorization: "Bearer optional-provider", csrf: null },
])) throw new Error("security dispatch mismatch: " + JSON.stringify(calls));
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript security-requirement planner test: %v\n%s", err, output)
	}
}

func TestRuntimeAppliesOpenAPISecurityRequirementsAndOperationOverride(t *testing.T) {
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
const securityProvider = ({ requirements, origin }) => {
  credentialCalls++;
  if (origin !== "https://api.example.test") throw new Error("credential origin mismatch");
  return { requirement: requirements.ApiKey, credentials: { ApiKey: { kind: "api-key", value: "secret" } } };
};
const api = createClient({ baseURL: "https://api.example.test", securityProvider, fetch: async (_url, init) => {
  calls++;
  if (new Headers(init.headers).get("x-api-key") !== "secret") throw new Error("API key not applied");
  return new Response(null, { status: 204 });
} });
await api.$operations.readSecure();
if (credentialCalls !== 1 || calls !== 1) throw new Error("protected request selection mismatch");
const publicAPI = createClient({ baseURL: "https://api.example.test", securityProvider, fetch: async (_url, init) => {
  if (new Headers(init.headers).has("x-api-key")) throw new Error("operation security override was ignored");
  return new Response(null, { status: 204 });
} });
await publicAPI.$operations.getPublic();
if (credentialCalls !== 1) throw new Error("public operation requested credentials");
await createClient({ baseURL: "https://api.example.test", securityProvider, headers: { "x-api-key": "caller" }, fetch: async () => { throw new Error("fetch must not run after credential collision"); } }).$operations.readSecure().then(() => { throw new Error("credential collision was accepted"); }, (error) => { if (error.code !== "SECURITY_CREDENTIALS_INVALID") throw error; });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript security runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeDelegatesEnvironmentControlledHeaderAPIKeysToFetch(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0",
  "info":{"title":"Fetch-managed security headers","version":"1"},
  "components":{"securitySchemes":{
    "OriginKey":{"type":"apiKey","in":"header","name":"Origin"},
    "OverrideKey":{"type":"apiKey","in":"header","name":"X-HTTP-Method-Override"},
    "AgentKey":{"type":"apiKey","in":"header","name":"User-Agent"}
  }},
  "paths":{
    "/origin":{"get":{"operationId":"getOrigin","security":[{"OriginKey":[]}],"responses":{"204":{"description":"OK"}}}},
    "/override":{"get":{"operationId":"getOverride","security":[{"OverrideKey":[]}],"responses":{"204":{"description":"OK"}}}},
    "/agent":{"get":{"operationId":"getAgent","security":[{"AgentKey":[]}],"responses":{"204":{"description":"OK"}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let override = 123;
let calls = 0;
const securityProvider = ({ requirements, operation }) => {
  const requirement = Object.values(requirements)[0];
  const values = {
    getOrigin: "https://caller.example",
    getOverride: override,
    getAgent: "openapi-sdkgen-test",
  };
  return { requirement, credentials: { [requirement.schemes[0].name]: { kind: "api-key", value: values[operation.operationID] } } };
};
const api = createClient({
  baseURL: "https://api.example.test",
  securityProvider,
  fetch: async (input, init) => {
    calls++;
    const url = new URL(String(input));
    const headers = new Headers(init.headers);
    if (url.pathname === "/origin" && headers.get("Origin") !== "https://caller.example") throw new Error("Origin API key missing");
    if (url.pathname === "/override" && headers.get("X-HTTP-Method-Override") !== override) throw new Error("override API key missing");
    if (url.pathname === "/agent" && headers.get("User-Agent") !== "openapi-sdkgen-test") throw new Error("User-Agent API key missing");
    return new Response(null, { status: 204 });
  },
});
const previousCalls = calls;
await api.$operations.getOverride().then(
  () => { throw new Error("non-string API key was accepted"); },
  (error) => {
    if (error.code !== "SECURITY_CREDENTIALS_INVALID") throw error;
    if (calls !== previousCalls) throw new Error("non-string API key reached fetch");
  },
);
override = "TRACE";
await api.$operations.getOrigin();
await api.$operations.getOverride();
override = "PATCH";
await api.$operations.getOverride();
await api.$operations.getAgent();
if (calls !== 4) throw new Error("security header dispatch count mismatch: " + calls);
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute Fetch-managed security-header runtime test: %v\n%s", err, output)
	}
}

func TestRuntimePreservesExactAndPrototypeSensitiveSecuritySchemeNames(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Exact security","version":"1"},
  "components":{"securitySchemes":{
    "api-key":{"type":"apiKey","in":"header","name":"x-api-key-one"},
    "api_key":{"type":"apiKey","in":"header","name":"x-api-key-two"},
    "__proto__":{"type":"apiKey","in":"header","name":"x-api-key-three"},
    "constructor":{"type":"apiKey","in":"header","name":"x-api-key-four"}
  }},
  "paths":{"/protected":{"get":{"operationId":"readSecure","security":[{"api-key":[],"api_key":[],"__proto__":[],"constructor":[]}],"responses":{"204":{"description":"OK"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const expectedNames = ["__proto__", "api-key", "api_key", "constructor"];
const api = createClient({
  baseURL: "https://api.example.test",
  securityProvider: ({ requirements }) => {
    const requirement = Object.values(requirements)[0];
    if (JSON.stringify(requirement.schemes.map(({ name }) => name).sort()) !== JSON.stringify(expectedNames)) throw new Error("security scheme identities changed");
    return {
      requirement,
      credentials: Object.fromEntries(expectedNames.map((name, index) => [name, { kind: "api-key", value: String(index + 1) }])),
    };
  },
  fetch: async (_input, init) => {
    const headers = new Headers(init.headers);
    if (["x-api-key-one", "x-api-key-two", "x-api-key-three", "x-api-key-four"].some((name) => !headers.has(name))) throw new Error("exact security credentials were not applied");
    return new Response(null, { status: 204 });
  },
});
await api.$operations.readSecure();
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute exact security identity runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeUsesAmbientFetchCookiesForCookieSecurity(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0",
  "info":{"title":"Ambient cookie security","version":"1"},
  "components":{"securitySchemes":{
    "Cookie":{"type":"apiKey","in":"cookie","name":"session"},
    "Bearer":{"type":"http","scheme":"bearer"}
  }},
  "paths":{
    "/client":{"get":{"operationId":"getClientCookie","security":[{"Cookie":[]}],"responses":{"204":{"description":"OK"}}}},
    "/per-request":{"get":{"operationId":"getRequestCookie","security":[{"Cookie":[]}],"responses":{"204":{"description":"OK"}}}},
    "/stream":{"get":{"operationId":"streamCookie","security":[{"Cookie":[]}],"responses":{"200":{"description":"OK","content":{"application/x-ndjson":{"itemSchema":{"type":"string"}}}}}}},
    "/mixed":{"get":{"operationId":"getMixedCookie","security":[{"Cookie":[],"Bearer":[]}],"responses":{"204":{"description":"OK"}}}}
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
const fetch = async (input, init) => {
  const headers = new Headers(init.headers);
  const path = new URL(String(input)).pathname;
  seen.push({ path, credentials: init.credentials, cookie: headers.get("cookie"), authorization: headers.get("authorization") });
  if (path === "/stream") return new Response('"event"\n', { headers: { "content-type": "application/x-ndjson" } });
  return new Response(null, { status: 204 });
};

const clientAmbient = createClient({ baseURL: "https://api.example.test", credentials: "include", fetch });
await clientAmbient.$operations.getClientCookie();
for await (const item of clientAmbient.$operations.streamCookie.stream()) {
  if (item !== "event") throw new Error("client ambient stream item mismatch");
}

const requestAmbient = createClient({ baseURL: "https://api.example.test", fetch });
await requestAmbient.$operations.getRequestCookie({ credentials: "include" });
for await (const item of requestAmbient.$operations.streamCookie.stream({ credentials: "include" })) {
  if (item !== "event") throw new Error("request ambient stream item mismatch");
}

let providerCalls = 0;
const mixed = createClient({
  baseURL: "https://api.example.test",
  securityProvider: ({ requirements }) => {
    providerCalls++;
    const requirement = Object.values(requirements)[0];
    return { requirement, credentials: { Bearer: { kind: "http-bearer", token: "token" } } };
  },
  fetch,
});
await mixed.$operations.getMixedCookie({ credentials: "include" });

if (providerCalls !== 1) throw new Error("mixed ambient credential provider call mismatch");
if (JSON.stringify(seen) !== JSON.stringify([
  { path: "/client", credentials: "include", cookie: null, authorization: null },
  { path: "/stream", credentials: "include", cookie: null, authorization: null },
  { path: "/per-request", credentials: "include", cookie: null, authorization: null },
  { path: "/stream", credentials: "include", cookie: null, authorization: null },
  { path: "/mixed", credentials: "include", cookie: null, authorization: "Bearer token" },
])) throw new Error("ambient cookie security dispatch mismatch: " + JSON.stringify(seen));

const missing = createClient({ baseURL: "https://api.example.test", credentials: "omit", fetch: async () => { throw new Error("missing ambient credentials reached fetch"); } });
await missing.$operations.getClientCookie().then(
  () => { throw new Error("cookie security accepted credentials omit"); },
  (error) => { if (error.code !== "SECURITY_CREDENTIALS_REQUIRED") throw error; },
);
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute ambient cookie security runtime test: %v\n%s", err, output)
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
const securityProvider = ({ requirements }) => {
  const requirement = Object.values(requirements)[0];
  const scheme = requirement.schemes[0];
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
  return { requirement, credentials: { [scheme.name]: values[scheme.name] } };
};
const expected = {
  "/basic": "Basic YTpi",
  "/bearer": "Bearer bearer-token",
  "/digest": "digest digest-value",
  "/oauth": "Bearer oauth-token",
  "/openid": "Bearer openid-token",
};
const api = createClient({ baseURL: "https://api.example.test", securityProvider, fetch: async (input, init) => {
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
await api.$operations.getMTLS().then(() => { throw new Error("mTLS security unexpectedly dispatched"); }, (error) => { if (error.code !== "TRANSPORT_CAPABILITY_REQUIRED") throw error; });
const capable = createClient({ baseURL: "https://api.example.test", securityProvider, transport: { capabilities: { cookieJar: true, mutualTLS: true }, fetch: async (input, init) => {
  const url = new URL(String(input));
  if (url.pathname === "/cookie" && new Headers(init.headers).get("cookie") !== "session=cookie-value") throw new Error("cookie capability was not used");
  return new Response(null, { status: 204 });
} } });
await capable.$operations.getCookie();
await capable.$operations.getMTLS();
const invalid = createClient({ baseURL: "https://api.example.test", securityProvider: ({ requirements }) => ({ requirement: requirements.Bearer, credentials: { Basic: { kind: "http-basic", username: "a", password: "b" } } }), fetch: async () => { throw new Error("invalid credentials reached fetch"); } });
await invalid.$operations.getBearer().then(() => { throw new Error("invalid credential set was accepted"); }, (error) => { if (error.code !== "SECURITY_CREDENTIALS_INVALID") throw error; });
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute TypeScript security-shape runtime test: %v\n%s", err, output)
	}
}

func TestRuntimeDispatchesNormalizationEquivalentExactOperationIDs(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Exact operations", "version": "1"},
  "paths": {
    "/modern": {"get": {"operationId": "get-pet", "responses": {"204": {"description": "OK"}}}},
    "/legacy": {"get": {"operationId": "get_pet", "responses": {"204": {"description": "OK"}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const paths = [];
const api = createClient({ baseURL: "https://api.example.test", fetch: async (input) => {
  paths.push(new URL(String(input)).pathname);
  return new Response(null, { status: 204 });
} });
await api.$operations["get-pet"]();
await api.$operations["get_pet"]();
if (paths.join(",") !== "/modern,/legacy") throw new Error("exact operation dispatch mismatch: " + paths.join(","));
`
	if result, runErr := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); runErr != nil {
		t.Fatalf("execute exact-operation runtime test: %v\n%s", runErr, result)
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
const pet = await api.$operations.savePet({ body: { pet_id: 7, name: "Milo & Co", tags: ["one", "two"] } });
if (pet.pet_id !== 8 || pet.name !== "Rex" || pet.tags.join(",") !== "red,blue") throw new Error("XML response decoding mismatch");
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

func TestWireSchemasPreservePrototypeNamedComponentsAndProperties(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.1","info":{"title":"Prototype safety","version":"1"},
  "paths":{"/value":{"post":{"operationId":"createValue","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/__proto__"}}}},"responses":{"204":{"description":"OK"}}}}},
  "components":{"schemas":{"__proto__":{"type":"object","required":["__proto__"],"properties":{"__proto__":{"type":"string"}},"additionalProperties":false}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
let calls = 0;
const api = createClient({ baseURL: "https://api.example.test", fetch: async (_url, init) => {
  calls++;
  const body = JSON.parse(init.body);
  if (!Object.hasOwn(body, "__proto__") || body.__proto__ !== "safe" || Object.getPrototypeOf(body) !== Object.prototype) throw new Error("wire property was not preserved");
  return new Response(null, { status: 204 });
}});
const invalid = Object.fromEntries([["__proto__", 7]]);
try { await api.$operations.createValue({ body: invalid }); throw new Error("invalid prototype property accepted"); }
catch (error) { if (!String(error).includes("expected string") && !String(error.cause).includes("expected string")) throw error; }
if (calls !== 0) throw new Error("invalid value reached fetch");
const valid = Object.fromEntries([["__proto__", "safe"]]);
if (!Object.hasOwn(valid, "__proto__") || Object.getPrototypeOf(valid) !== Object.prototype) throw new Error("test input is not prototype-safe");
await api.$operations.createValue({ body: valid });
if (calls !== 1) throw new Error("valid value did not reach fetch");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute prototype-safe wire schema test: %v\n%s", err, output)
	}
}

func TestVisibleRecursiveComponentCanServeRequestSuccessAndErrorRoles(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.1","info":{"title":"Dual role","version":"1"},
  "paths":{"/node":{"post":{"operationId":"saveNode","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/NodeError"}}}},"responses":{
    "200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"#/components/schemas/NodeError"}}}},
    "400":{"description":"Error","content":{"application/problem+json":{"schema":{"$ref":"#/components/schemas/NodeError"}}}}
  }}}},
  "components":{"schemas":{"NodeError":{
    "type":"object","required":["error"],"properties":{
      "child":{"$ref":"#/components/schemas/NodeError"},
      "requestOnly":{"type":"string","writeOnly":true},
      "responseOnly":{"type":"string","readOnly":true},
      "error":{"type":"object","required":["code","message","details"],"properties":{
        "code":{"const":"node_error"},"message":{"type":"string"},"details":{"$ref":"#/components/schemas/NodeDetails"}
      }}
    }
  },"NodeDetails":{"type":"object","properties":{"reason":{"type":"string"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	types := string(artifactByPath(t, artifacts, "generated/types.ts"))
	errorsSource := string(artifactByPath(t, artifacts, "generated/errors.ts"))
	client := string(artifactByPath(t, artifacts, "generated/client.ts"))
	for _, expected := range []string{
		`readonly "NodeError": {`,
		`readonly "child"?: ComponentInput<"NodeError">`,
		`readonly "child"?: ComponentOutput<"NodeError">`,
	} {
		if !strings.Contains(types, expected) {
			t.Fatalf("dual-role component missing %q:\n%s", expected, types)
		}
	}
	if !strings.Contains(errorsSource, `"node_error"`) || !strings.Contains(client, `readonly error: Errors.ServerError<"node_error"`) {
		t.Fatalf("dual-role error contract missing:\n%s\n%s", errorsSource, client)
	}
	_ = compileTypeScriptArtifacts(t, document)
}

func TestResourceCallableNamespacesSupportFunctionIntrinsicNames(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.1","info":{"title":"Callable namespaces","version":"1"},
  "paths":{
    "/users":{"get":{"operationId":"listUsers","responses":{"204":{"description":"OK"}}}},
    "/users/list/name":{"get":{"operationId":"getName","responses":{"204":{"description":"OK"}}}},
    "/users/list/length":{"get":{"operationId":"getLength","responses":{"204":{"description":"OK"}}}},
    "/users/list/caller":{"get":{"operationId":"getCaller","responses":{"204":{"description":"OK"}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	output := compileTypeScriptArtifacts(t, document)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const paths = [];
const api = createClient({ baseURL: "https://api.example.test", fetch: async (url) => { paths.push(new URL(url).pathname); return new Response(null, { status: 204 }); } });
await api.users.list();
await api.users.list.name.get();
await api.users.list.length.get();
await api.users.list.caller.get();
if (JSON.stringify(paths) !== JSON.stringify(["/users", "/users/list/name", "/users/list/length", "/users/list/caller"])) throw new Error("callable namespace dispatch mismatch: " + JSON.stringify(paths));
for (const key of ["name", "length", "caller"]) if (!Object.hasOwn(api.users.list, key)) throw new Error("missing own callable member " + key);
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute callable namespace intrinsic test: %v\n%s", err, output)
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
	return compileTypeScriptArtifactsWithProbe(t, document, "", "")
}

func compileTypeScriptArtifactsWithProbe(t *testing.T, document *ir.Document, probeName, probe string) string {
	t.Helper()
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	writeTargetArtifacts(t, source, artifacts)
	if probeName != "" {
		if err := os.WriteFile(filepath.Join(source, probeName), []byte(probe), 0o600); err != nil {
			t.Fatal(err)
		}
	}
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
