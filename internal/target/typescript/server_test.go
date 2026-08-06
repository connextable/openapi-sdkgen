package typescript

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sdkgen "openapi-sdkgen/internal/compiler"
	"openapi-sdkgen/internal/generator"
)

func TestGeneratedWebhookRouterExecutesThroughFetch(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
	  "openapi": "3.2.0",
  "info": {"title": "Webhook", "version": "1"},
  "paths": {},
  "webhooks": {"binary": {"get": {"operationId":"binaryWebhook","security":[],"responses":{"200":{"description":"OK","content":{"application/pdf":{"schema":{"type":"string","format":"binary"}}}}}}}, "plain": {"get": {"operationId":"plainWebhook","security":[],"responses":{"200":{"description":"OK","content":{"application/vnd.example.plain":{"schema":{"type":"string"}}}}}}}, "xml": {"get": {"operationId":"xmlWebhook","security":[],"responses":{"200":{"description":"OK","content":{"application/xml":{"schema":{"type":"object","xml":{"name":"receipt"},"required":["id","note"],"properties":{"id":{"type":"string","xml":{"attribute":true}},"note":{"type":"string","xml":{"name":"message"}}}}}}}}}}, "selectors":{"get":{"operationId":"selectorWebhook","security":[],"parameters":[{"name":"label","in":"path","required":true,"style":"label","explode":false,"schema":{"type":"object","required":["role","enabled"],"properties":{"role":{"type":"string"},"enabled":{"type":"boolean"}}}},{"name":"matrix","in":"path","required":true,"style":"matrix","explode":false,"schema":{"type":"object","required":["role","enabled"],"properties":{"role":{"type":"string"},"enabled":{"type":"boolean"}}}}],"responses":{"204":{"description":"OK"}}}}, "orderCreated": {"post": {
    "operationId": "orderCreatedWebhook",
	"parameters": [
	  {"name":"page","in":"query","required":true,"schema":{"type":"integer"}},
	  {"name":"order","in":"query","schema":{"type":"array","items":{"type":"string","enum":["createdAt:asc","createdAt:desc"]}}},
	  {"name":"filter","in":"query","style":"deepObject","explode":true,"schema":{"type":"object","required":["kind_name","count"],"properties":{"kind_name":{"type":"string"},"count":{"type":"integer"}}}},
	  {"name":"meta","in":"header","style":"simple","explode":true,"schema":{"type":"object","required":["trace_id","enabled"],"properties":{"trace_id":{"type":"integer"},"enabled":{"type":"boolean"}}}},
	  {"name":"payload","in":"query","content":{"application/xml":{"schema":{"type":"object","required":["event_id","choice"],"properties":{"event_id":{"type":"string","xml":{"name":"event"}},"choice":{"oneOf":[{"type":"integer"},{"type":"boolean"}]}}}}}},
	  {"name":"custom","in":"header","content":{"application/vnd.example.parameter":{"schema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}}},
	  {"name":"X-Trace","in":"header","required":true,"schema":{"type":"string"}},
	  {"name":"tags","in":"cookie","style":"cookie","explode":true,"schema":{"type":"array","items":{"type":"string"}}},
	  {"name":"prefs","in":"cookie","style":"cookie","explode":true,"schema":{"type":"object","required":["theme","event_id"],"properties":{"theme":{"type":"string"},"event_id":{"type":"string"}}}},
	  {"name":"session","in":"cookie","required":true,"schema":{"type":"string"}}
	],
    "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["id"], "properties": {"id": {"type": "string"}}}}}},
	    "responses": {"202": {"description": "Accepted", "headers":{"X-Rate":{"required":true,"schema":{"type":"integer"}},"X-List":{"schema":{"type":"array","items":{"type":"integer"}}},"X-Object":{"style":"simple","explode":true,"schema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}},"X-Meta":{"content":{"application/json":{"schema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}}},"X-Custom":{"content":{"application/vnd.example.parameter":{"schema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}}}}, "content": {"application/json": {"schema": {"type": "object", "required": ["accepted"], "properties": {"accepted": {"type": "string"}}}}}}}
  }}},
  "security": [{"signature": []}],
  "components": {"securitySchemes": {"signature": {"type": "apiKey", "in": "header", "name": "x-signature"}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	options, err := registry.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := (Generator{}).Generate(document, options)
	if err != nil {
		t.Fatal(err)
	}
	if webhooks := string(artifactByPath(t, artifacts, "server/webhooks.ts")); !strings.Contains(webhooks, `name: "payload"`) || !strings.Contains(webhooks, `contentType: "application/xml"`) {
		t.Fatalf("parameter content plan was not emitted:\n%s", webhooks)
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	writeTargetArtifacts(t, source, artifacts)
	if err := os.WriteFile(filepath.Join(source, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "tsconfig.json"), []byte(serverTSConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join("..", "..", "..", "test", "typescript", "node_modules", "typescript", "lib", "tsc.js")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("TypeScript compiler unavailable for server test: %v", err)
	}
	if output, err := exec.Command("node", tsc, "--project", filepath.Join(source, "tsconfig.json")).CombinedOutput(); err != nil {
		t.Fatalf("compile generated server: %v\n%s", err, output)
	}
	outputDirectory := filepath.Join(directory, "output")
	if err := os.WriteFile(filepath.Join(outputDirectory, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
import { pathToFileURL } from "node:url";
const { createWebhookRouter } = await import(pathToFileURL(process.argv[1]).href);
const seen = [];
let selectorParams;
const router = createWebhookRouter({
  binary: { GET: async () => ({ status: 200, contentType: "application/pdf", body: new Uint8Array([1, 2, 3]) }) },
  plain: { GET: async ({ request }) => { if (new URL(request.url).searchParams.has("fail")) throw new Error("private no-body handler detail"); return { status: 200, contentType: "application/vnd.example.plain", body: "plain" }; } },
  xml: { GET: async () => ({ status: 200, contentType: "application/xml", body: { id: "receipt-1", note: "hello & goodbye" } }) },
  selectors: { GET: async ({ params }) => { selectorParams = params.path; return { status: 204 }; } },
	orderCreated: { POST: async ({ body, operationID, request, params }) => {
	if (body.id === "explode") throw new Error("private handler detail");
	if (body.id === "missing-header") return { status: 202, body: { accepted: body.id } };
	if (body.id === "invalid-header") return { status: 202, headers: { "x-rate": "nope" }, body: { accepted: body.id } };
	if (body.id === "raw-list") return { status: 202, headers: { "x-rate": "2", "x-list": "1,2" }, body: { accepted: body.id } };
	if (body.id === "raw-custom") return { status: 202, headers: { "x-rate": "2", "x-custom": "custom:raw-outbound" }, body: { accepted: body.id } };
	seen.push({ body, operationID, method: request.method, params });
    return { status: 202, headerValues: { "X-Rate": 2, "X-List": [1, 2], "X-Object": { event_id: "outbound" }, "X-Meta": { event_id: "outbound" }, "X-Custom": { event_id: "custom-outbound" } }, body: { accepted: body.id } };
  } },
}, { routes: { binary: "/hooks/binary", plain: "/hooks/plain", xml: "/hooks/xml", selectors: "/hooks/selectors/{label}/{matrix}", orderCreated: "/hooks/orders" }, codecs: { "application/vnd.example.plain": { encode(value) { return "custom:" + value; } }, "application/vnd.example.parameter": { decodeParameter(value) { return { event_id: value.replace("custom:", "") }; }, encodeParameter(value) { return "custom:" + value.event_id; } } }, authenticate: async ({ method, path, security, securityCandidates }) => {
	if (securityCandidates.signature?.value === "boom") throw new Error("private authenticator detail");
	if (method !== "POST" || path !== "/hooks/orders" || JSON.stringify(security) !== JSON.stringify([{ signature: [] }]) || (securityCandidates.signature?.value !== undefined && securityCandidates.signature.value !== "sig-1")) throw new Error("bad auth context");
} });
const response = await router.fetch(new Request("https://host.test/hooks/orders?page=2&order=createdAt%3Adesc&filter[kind_name]=fresh&filter[count]=3&payload=%3Cpayload%3E%3Cevent%3Exml-event%3C%2Fevent%3E%3Cchoice%3E2%3C%2Fchoice%3E%3C%2Fpayload%3E", { method: "POST", headers: { "content-type": "application/json", "x-signature": "sig-1", "x-trace": "trace-1", "meta": "trace_id=4,enabled=true", "custom": "custom-event", "cookie": "session=one; tags=one; tags=two; theme=dark; event_id=a%2Fb" }, body: JSON.stringify({ id: "order-1" }) }));
if (response.status !== 202 || response.headers.get("x-rate") !== "2" || response.headers.get("x-list") !== "1,2" || response.headers.get("x-object") !== "event_id=outbound" || response.headers.get("x-meta") !== '{"event_id":"outbound"}' || response.headers.get("x-custom") !== "custom:custom-outbound" || JSON.stringify(await response.json()) !== JSON.stringify({ accepted: "order-1" })) throw new Error("handler response was not encoded");
const plain = await router.fetch(new Request("https://host.test/hooks/plain", { method: "GET" }));
if (plain.status !== 200 || plain.headers.get("content-type") !== "application/vnd.example.plain" || await plain.text() !== "custom:plain") throw new Error("custom response was not encoded");
const failedNoBodyHandler = await router.fetch(new Request("https://host.test/hooks/plain?fail=1", { method: "GET" }));
if (failedNoBodyHandler.status !== 500 || await failedNoBodyHandler.text() !== "Internal Server Error") throw new Error("no-body handler error leaked");
const binary = await router.fetch(new Request("https://host.test/hooks/binary", { method: "GET" }));
if (binary.status !== 200 || binary.headers.get("content-type") !== "application/pdf" || JSON.stringify([...new Uint8Array(await binary.arrayBuffer())]) !== "[1,2,3]") throw new Error("binary response was not encoded");
const xml = await router.fetch(new Request("https://host.test/hooks/xml", { method: "GET" }));
if (xml.status !== 200 || xml.headers.get("content-type") !== "application/xml" || await xml.text() !== '<receipt id="receipt-1"><message>hello &amp; goodbye</message></receipt>') throw new Error("XML response was not encoded from its schema");
if (JSON.stringify(seen) !== JSON.stringify([{ body: { id: "order-1" }, operationID: "orderCreatedWebhook", method: "POST", params: { path: {}, query: { page: 2, order: ["createdAt:desc"], filter: { kind_name: "fresh", count: 3 }, payload: { choice: 2, event_id: "xml-event" } }, querystring: {}, headerParams: { meta: { trace_id: 4, enabled: true }, custom: { event_id: "custom-event" }, "X-Trace": "trace-1" }, cookieParams: { tags: ["one", "two"], prefs: { event_id: "a%2Fb", theme: "dark" }, session: "one" } } }])) throw new Error("handler context mismatch: " + JSON.stringify(seen));
const selectorResponse = await router.fetch(new Request("https://host.test/hooks/selectors/.role,admin,enabled,true/;matrix=role,owner,enabled,false", { method: "GET" }));
if (selectorResponse.status !== 204 || JSON.stringify(selectorParams) !== JSON.stringify({ label: { role: "admin", enabled: true }, matrix: { role: "owner", enabled: false } })) throw new Error("label/matrix path objects were not decoded");
const denied = createWebhookRouter({ orderCreated: { POST: async () => ({ status: 202 }) } }, { routes: { orderCreated: "/hooks/orders" }, authenticate: () => new Response("no", { status: 401 }) });
if ((await denied.fetch(new Request("https://host.test/hooks/orders?page=2", { method: "POST", headers: { "content-type": "application/json", "x-trace": "trace-1", "cookie": "session=one" }, body: "{}" }))).status !== 401) throw new Error("authentication response was ignored");
const defaultDenied = createWebhookRouter({ orderCreated: { POST: async () => ({ status: 202 }) } }, { routes: { orderCreated: "/hooks/orders" } });
if ((await defaultDenied.fetch(new Request("https://host.test/hooks/orders?page=2", { method: "POST", headers: { "content-type": "application/json", "x-trace": "trace-1", "cookie": "session=one" }, body: JSON.stringify({ id: "order-1" }) }))).status !== 401) throw new Error("protected webhook did not fail closed without an authenticator");
if ((await router.fetch(new Request("https://host.test/hooks/orders?page=2", { method: "POST", headers: { "content-type": "text/plain", "x-trace": "trace-1", "cookie": "session=one" }, body: "bad" }))).status !== 415) throw new Error("bad media type was accepted");
if ((await router.fetch(new Request("https://host.test/hooks/orders?page=2", { method: "POST", headers: { "content-type": "application/json", "x-trace": "trace-1", "cookie": "session=one" }, body: "{}" }))).status !== 400) throw new Error("schema-invalid body was accepted");
const failedHandler = await router.fetch(new Request("https://host.test/hooks/orders?page=2", { method: "POST", headers: { "content-type": "application/json", "x-trace": "trace-1", "cookie": "session=one" }, body: JSON.stringify({ id: "explode" }) }));
if (failedHandler.status !== 500 || await failedHandler.text() !== "Internal Server Error") throw new Error("handler error leaked or did not become a safe 500");
for (const id of ["missing-header", "invalid-header"]) {
  const invalidResponse = await router.fetch(new Request("https://host.test/hooks/orders?page=2", { method: "POST", headers: { "content-type": "application/json", "x-trace": "trace-1", "cookie": "session=one" }, body: JSON.stringify({ id }) }));
  if (invalidResponse.status !== 500 || await invalidResponse.text() !== "Internal Server Error") throw new Error("invalid response header was accepted");
}
if ((await router.fetch(new Request("https://host.test/hooks/orders?page=2", { method: "POST", headers: { "content-type": "application/json", "x-trace": "trace-1", "cookie": "session=one" }, body: JSON.stringify({ id: "raw-list" }) }))).status !== 202) throw new Error("raw array response header was rejected");
if ((await router.fetch(new Request("https://host.test/hooks/orders?page=2", { method: "POST", headers: { "content-type": "application/json", "x-trace": "trace-1", "cookie": "session=one" }, body: JSON.stringify({ id: "raw-custom" }) }))).status !== 202) throw new Error("raw custom response header was rejected");
const failedAuthentication = await router.fetch(new Request("https://host.test/hooks/orders?page=2", { method: "POST", headers: { "content-type": "application/json", "x-signature": "boom", "x-trace": "trace-1", "cookie": "session=one" }, body: JSON.stringify({ id: "order-1" }) }));
if (failedAuthentication.status !== 500 || await failedAuthentication.text() !== "Internal Server Error") throw new Error("authentication error leaked or did not become a safe 500");
for (const routes of [{}, { orderCreated: "hooks/orders" }, { orderCreated: "/hooks/orders?debug=1" }]) {
  try { createWebhookRouter({ orderCreated: { POST: async () => ({ status: 202 }) } }, { routes }); throw new Error("invalid route was accepted"); }
  catch (error) { if (String(error).includes("invalid route was accepted")) throw error; }
}
try { createWebhookRouter({}, { routes: {}, codecs: { "application/vnd.example": {}, "Application/VND.Example": {} } }); throw new Error("duplicate codec was accepted"); }
catch (error) { if (String(error).includes("duplicate codec was accepted")) throw error; }
`
	command := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(outputDirectory, "server", "webhooks.js"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("execute generated webhook router: %v\n%s", err, output)
	}
}

func TestGeneratedCallbackEndpointsAreHostBoundAndRoundTripJSON(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.1",
  "info": {"title": "Callback", "version": "1"},
  "security": [{"signature": []}],
  "paths": {"/orders": {"post": {
    "operationId": "createOrder",
    "security": [],
    "responses": {"202": {"description": "Accepted"}},
    "callbacks": {"orderStatus": {"{$request.body#/callbackURL}": {"post": {
      "operationId": "orderStatusCallback",
      "parameters": [{"name":"order","in":"query","schema":{"type":"array","items":{"type":"string","enum":["name:asc","name:desc"]}}}],
      "requestBody": {"content": {"application/vnd.example.callback": {"schema": {"type": "object", "required": ["id"], "properties": {"id": {"type": "string"}}}}}},
      "responses": {"204": {"description": "Accepted"}}
    }}}}
  }}},
  "components": {"schemas": {}, "securitySchemes": {"signature": {"type": "apiKey", "in": "header", "name": "x-signature"}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	options, err := registry.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := (Generator{}).Generate(document, options)
	if err != nil {
		t.Fatal(err)
	}
	callbacks := string(artifactByPath(t, artifacts, "server/callbacks.ts"))
	for _, expected := range []string{"createCallbackHandlers", `export interface RouteCallbacks`, `readonly "POST /orders"`, `export interface Callbacks`, `readonly "createOrder"`, `readonly "orderStatus"`, "{$request.body#/callbackURL}", "No route is generated", `as unknown as CallbackEndpoints["routeCallbacks"]`, `as unknown as CallbackEndpoints["callbacks"]`, `as unknown as CallbackEndpoints["componentCallbacks"]`} {
		if !strings.Contains(callbacks, expected) {
			t.Fatalf("callback source missing %q:\n%s", expected, callbacks)
		}
	}
	if strings.Contains(callbacks, "createWebhookRouter") || strings.Contains(callbacks, "decodeOrderStatusCallback") || strings.Contains(callbacks, "encodeOrderStatusCallbackResponse") {
		t.Fatalf("callback public surface leaked codecs or a router:\n%s", callbacks)
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	writeTargetArtifacts(t, source, artifacts)
	if err := os.WriteFile(filepath.Join(source, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "tsconfig.json"), []byte(serverTSConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join("..", "..", "..", "test", "typescript", "node_modules", "typescript", "lib", "tsc.js")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("TypeScript compiler unavailable for callback test: %v", err)
	}
	if output, err := exec.Command("node", tsc, "--project", filepath.Join(source, "tsconfig.json")).CombinedOutput(); err != nil {
		t.Fatalf("compile generated callback codecs: %v\n%s", err, output)
	}
	outputDirectory := filepath.Join(directory, "output")
	if err := os.WriteFile(filepath.Join(outputDirectory, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
import { pathToFileURL } from "node:url";
const codecs = await import(pathToFileURL(process.argv[1]).href);
const seen = [];
const callbacks = codecs.createCallbackHandlers({ callbacks: { createOrder: { orderStatus: { "{$request.body#/callbackURL}": { POST: async ({ body, operationID, method, request, params }) => {
  seen.push({ body, operationID, method, path: new URL(request.url).pathname, params });
  return { status: 204 };
} } } } } }, { codecs: { "application/vnd.example.callback": { async decodeInbound(request) { return JSON.parse(await request.text()); } } }, authenticate: ({ security }) => {
  if (JSON.stringify(security) !== JSON.stringify([{ signature: [] }])) throw new Error("callback security metadata mismatch");
} });
const endpoint = callbacks.callbacks.createOrder.orderStatus["{$request.body#/callbackURL}"].POST;
const routeEndpoint = callbacks.routeCallbacks["POST /orders"].orderStatus["{$request.body#/callbackURL}"].POST;
if (routeEndpoint !== endpoint) throw new Error("route and explicit-ID callback endpoints are not referential aliases");
const routeHandler = async () => ({ status: 204 });
const operationHandler = async () => ({ status: 204 });
const duplicateHandlers = codecs.createCallbackHandlers({
  routeCallbacks: { "POST /orders": { orderStatus: { "{$request.body#/callbackURL}": { POST: routeHandler } } } },
  callbacks: { createOrder: { orderStatus: { "{$request.body#/callbackURL}": { POST: operationHandler } } } }
});
try {
  await duplicateHandlers.routeCallbacks["POST /orders"].orderStatus["{$request.body#/callbackURL}"].POST.fetch(new Request("https://host.test/callback", { method: "POST" }));
  throw new Error("duplicate callback handlers were accepted");
} catch (error) {
  if (String(error).includes("duplicate callback handlers were accepted")) throw error;
  if (!String(error).includes("both routeCallbacks and callbacks")) throw error;
}
const sharedHandler = async () => ({ status: 204 });
const duplicatePathParams = codecs.createCallbackHandlers({
  routeCallbacks: { "POST /orders": { orderStatus: { "{$request.body#/callbackURL}": { POST: sharedHandler } } } },
  callbacks: { createOrder: { orderStatus: { "{$request.body#/callbackURL}": { POST: sharedHandler } } } }
}, {
  pathParams: {
    routeCallbacks: { "POST /orders": { orderStatus: { "{$request.body#/callbackURL}": { POST: {} } } } },
    callbacks: { createOrder: { orderStatus: { "{$request.body#/callbackURL}": { POST: {} } } } }
  }
});
try {
  await duplicatePathParams.routeCallbacks["POST /orders"].orderStatus["{$request.body#/callbackURL}"].POST.fetch(new Request("https://host.test/callback", { method: "POST" }));
  throw new Error("duplicate callback path parameters were accepted");
} catch (error) {
  if (String(error).includes("duplicate callback path parameters were accepted")) throw error;
  if (!String(error).includes("both routeCallbacks and callbacks")) throw error;
}
const response = await endpoint.fetch(new Request("https://host.test/callback?order=name%3Aasc", { method: "POST", headers: { "content-type": "application/vnd.example.callback" }, body: JSON.stringify({ id: "order-1" }) }));
if (response.status !== 204) throw new Error("callback response was not encoded");
if (JSON.stringify(seen) !== JSON.stringify([{ body: { id: "order-1" }, operationID: "orderStatusCallback", method: "POST", path: "/callback", params: { path: {}, query: { order: ["name:asc"] }, querystring: {}, headerParams: {}, cookieParams: {} } }])) throw new Error("callback context mismatch");
if ((await endpoint.fetch(new Request("https://host.test/callback", { method: "GET" }))).status !== 405) throw new Error("wrong callback method was accepted");
if ((await endpoint.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "text/plain" }, body: "bad" }))).status !== 415) throw new Error("bad callback media type was accepted");
if ((await endpoint.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "application/vnd.example.callback" }, body: "{}" }))).status !== 400) throw new Error("schema-invalid callback was accepted");
if ((await codecs.createCallbackHandlers({}).callbacks.createOrder.orderStatus["{$request.body#/callbackURL}"].POST.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "application/vnd.example.callback" }, body: "{}" }))).status !== 404) throw new Error("missing callback handler was accepted");
const denied = codecs.createCallbackHandlers({ callbacks: { createOrder: { orderStatus: { "{$request.body#/callbackURL}": { POST: async () => ({ status: 204 }) } } } } }, { authenticate: () => new Response("Unauthorized", { status: 401 }) });
if ((await denied.callbacks.createOrder.orderStatus["{$request.body#/callbackURL}"].POST.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "application/vnd.example.callback" }, body: JSON.stringify({ id: "order-1" }) }))).status !== 401) throw new Error("callback authentication response was ignored");
`
	command := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(outputDirectory, "server", "callbacks.js"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("execute generated callback codecs: %v\n%s", err, output)
	}
}

func TestGeneratedCallbacksSupportIDLessSourceThroughExactRoute(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "ID-less callback source", "version": "1"},
  "paths": {"/jobs": {"post": {
    "responses": {"202": {"description": "Accepted"}},
    "callbacks": {"status": {"{$request.body#/url}": {"post": {
      "responses": {"204": {"description": "Accepted"}}
    }}}}
  }}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	options, err := registry.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := (Generator{}).Generate(document, options)
	if err != nil {
		t.Fatal(err)
	}
	callbacks := string(artifactByPath(t, artifacts, "server/callbacks.ts"))
	for _, expected := range []string{
		`export interface RouteCallbacks`,
		`readonly "POST /jobs"`,
		`routeCallbacks: /* @__PURE__ */ Object.fromEntries([["POST /jobs"`,
		`callbacks: /* @__PURE__ */ Object.fromEntries([])`,
	} {
		if !strings.Contains(callbacks, expected) {
			t.Fatalf("ID-less callback route surface missing %q:\n%s", expected, callbacks)
		}
	}
	if strings.Contains(callbacks, `readonly "":`) {
		t.Fatalf("empty callback source alias leaked:\n%s", callbacks)
	}
}

func TestServerAddOnAcceptsBinaryInboundBodies(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.1",
  "info": {"title": "Webhook", "version": "1"},
  "paths": {},
  "webhooks": {"orderCreated": {"post": {
    "requestBody": {"content": {"application/pdf": {"schema": {"type": "string", "format": "binary"}}}},
    "responses": {"204": {"description": "Accepted"}}
  }}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	options, err := registry.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Generator{}).Generate(document, options); err != nil {
		t.Fatalf("server binary media generation = %v", err)
	}
}

func TestGeneratedWebhookRouterDecodesTextAndFormBodies(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.1", "info":{"title":"Inbound media","version":"1"}, "paths":{},
  "webhooks": {
    "formReceived": {"post":{"requestBody":{"required":true,"content":{"application/x-www-form-urlencoded":{"schema":{"type":"object","required":["name","count","enabled","tags","meta"],"properties":{"name":{"type":"string"},"count":{"type":"integer"},"enabled":{"type":"boolean"},"tags":{"type":"array","items":{"type":"string"}},"meta":{"type":"object","required":["source"],"properties":{"source":{"type":"string"}}}}},"encoding":{"meta":{"contentType":"application/json"}}}}},"responses":{"204":{"description":"OK"}}}},
    "textReceived": {"post":{"requestBody":{"required":true,"content":{"text/plain":{"schema":{"type":"string","minLength":3}}}},"responses":{"204":{"description":"OK"}}}},
    "xmlReceived": {"post":{"requestBody":{"required":true,"content":{"application/xml":{"schema":{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}}}},"responses":{"204":{"description":"OK"}}}},
    "multipartReceived": {"post":{"requestBody":{"required":true,"content":{"multipart/form-data":{"schema":{"type":"object","required":["name","meta","custom"],"properties":{"name":{"type":"string"},"meta":{"type":"object","required":["source"],"properties":{"source":{"type":"string"}}},"custom":{"type":"object","required":["source"],"properties":{"source":{"type":"string"}}}}},"encoding":{"meta":{"contentType":"application/json"},"custom":{"contentType":"application/vnd.example.part"}}}}},"responses":{"204":{"description":"OK"}}}},
    "binaryReceived": {"post":{"requestBody":{"required":true,"content":{"application/pdf":{"schema":{"type":"string","format":"binary"}}}},"responses":{"204":{"description":"OK"}}}},
    "multiReceived": {"post":{"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}},"text/plain":{"schema":{"type":"string"}}}},"responses":{"204":{"description":"OK"}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	options, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	addons, err := options.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := (Generator{}).Generate(document, addons)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	writeTargetArtifacts(t, source, artifacts)
	if err := os.WriteFile(filepath.Join(source, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "tsconfig.json"), []byte(serverTSConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join("..", "..", "..", "test", "typescript", "node_modules", "typescript", "lib", "tsc.js")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("TypeScript compiler unavailable for server test: %v", err)
	}
	if output, err := exec.Command("node", tsc, "--project", filepath.Join(source, "tsconfig.json")).CombinedOutput(); err != nil {
		t.Fatalf("compile generated inbound media server: %v\n%s", err, output)
	}
	outputDirectory := filepath.Join(directory, "output")
	if err := os.WriteFile(filepath.Join(outputDirectory, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
import { pathToFileURL } from "node:url";
const { createWebhookRouter } = await import(pathToFileURL(process.argv[1]).href);
const seen = [];
const router = createWebhookRouter({
  formReceived: { POST: async ({ body }) => { if (body.count !== 2 || body.enabled !== true || body.tags.join(",") !== "one,two" || body.meta.source !== "form") throw new Error("form values were not typed"); seen.push(body); return { status: 204 }; } },
  textReceived: { POST: async ({ body }) => { seen.push(body); return { status: 204 }; } },
  xmlReceived: { POST: async ({ body }) => { seen.push(body); return { status: 204 }; } },
  multipartReceived: { POST: async ({ body }) => { if (body.meta.source !== "multipart" || body.custom.source !== "custom") throw new Error("multipart fields were not decoded"); seen.push(body); return { status: 204 }; } },
  binaryReceived: { POST: async ({ body }) => { seen.push({ bytes: body.byteLength }); return { status: 204 }; } },
  multiReceived: { POST: async ({ body }) => { seen.push(body.contentType === "application/json" ? body.value.event_id : body.value); return { status: 204 }; } },
}, { routes: { formReceived: "/form", textReceived: "/text", xmlReceived: "/xml", multipartReceived: "/multipart", binaryReceived: "/binary", multiReceived: "/multi" }, codecs: { "application/vnd.example.part": { decodeParameter: (value) => JSON.parse(value) } } });
if ((await router.fetch(new Request("https://host.test/form", { method: "POST", headers: { "content-type": "application/x-www-form-urlencoded" }, body: "name=widget&count=2&enabled=true&tags=one&tags=two&meta=%7B%22source%22%3A%22form%22%7D" }))).status !== 204) throw new Error("form body rejected");
if ((await router.fetch(new Request("https://host.test/text", { method: "POST", headers: { "content-type": "text/plain" }, body: "hello" }))).status !== 204) throw new Error("text body rejected");
if ((await router.fetch(new Request("https://host.test/xml", { method: "POST", headers: { "content-type": "application/xml" }, body: "<item><name>widget</name></item>" }))).status !== 204) throw new Error("XML body rejected");
const multipart = new FormData(); multipart.set("name", "widget"); multipart.set("meta", new Blob(['{"source":"multipart"}'], { type: "application/json" })); multipart.set("custom", new Blob(['{"source":"custom"}'], { type: "application/vnd.example.part" }));
if ((await router.fetch(new Request("https://host.test/multipart", { method: "POST", body: multipart }))).status !== 204) throw new Error("multipart body rejected");
if ((await router.fetch(new Request("https://host.test/binary", { method: "POST", headers: { "content-type": "application/pdf" }, body: new Uint8Array([1, 2, 3]) }))).status !== 204) throw new Error("binary body rejected");
if ((await router.fetch(new Request("https://host.test/multi", { method: "POST", headers: { "content-type": "application/json" }, body: '{"event_id":"json"}' }))).status !== 204) throw new Error("JSON multi body rejected");
if ((await router.fetch(new Request("https://host.test/multi", { method: "POST", headers: { "content-type": "text/plain" }, body: "text" }))).status !== 204) throw new Error("text multi body rejected");
if ((await router.fetch(new Request("https://host.test/text", { method: "POST", headers: { "content-type": "text/plain" }, body: "no" }))).status !== 400) throw new Error("invalid text body accepted");
if (JSON.stringify(seen) !== JSON.stringify([{ name: "widget", count: 2, enabled: true, tags: ["one", "two"], meta: { source: "form" } }, "hello", { name: "widget" }, { name: "widget", meta: { source: "multipart" }, custom: { source: "custom" } }, { bytes: 3 }, "json", "text"])) throw new Error("inbound bodies were not decoded");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(outputDirectory, "server", "webhooks.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute generated inbound media server: %v\n%s", err, output)
	}
}

func TestGeneratedWebhookRouterStreamsSequentialBodies(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0", "info":{"title":"Inbound streams","version":"1"}, "paths":{},
  "webhooks":{"events":{"post":{"requestBody":{"required":true,"content":{"application/x-ndjson":{"itemSchema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}}},"responses":{"204":{"description":"OK"}}}},"frames":{"post":{"requestBody":{"required":true,"content":{"multipart/mixed":{"itemSchema":{"type":"object","required":["frame_id"],"properties":{"frame_id":{"type":"string"}}},"itemEncoding":{"contentType":"application/json"}}}},"responses":{"204":{"description":"OK"}}}},"custom":{"post":{"requestBody":{"required":true,"content":{"application/*":{"schema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}}},"responses":{"204":{"description":"OK"}}}},"customStream":{"post":{"requestBody":{"required":true,"content":{"application/vnd.example.events":{"itemSchema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string"}}}}}},"responses":{"204":{"description":"OK"}}}},"denied":{"post":{"requestBody":{"required":true,"content":{"application/json":{"schema":false}}},"responses":{"204":{"description":"OK"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	addons, err := registry.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := (Generator{}).Generate(document, addons)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	writeTargetArtifacts(t, source, artifacts)
	if err := os.WriteFile(filepath.Join(source, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "tsconfig.json"), []byte(serverTSConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join("..", "..", "..", "test", "typescript", "node_modules", "typescript", "lib", "tsc.js")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("TypeScript compiler unavailable for server stream test: %v", err)
	}
	if output, err := exec.Command("node", tsc, "--project", filepath.Join(source, "tsconfig.json")).CombinedOutput(); err != nil {
		t.Fatalf("compile generated inbound stream server: %v\n%s", err, output)
	}
	outputDirectory := filepath.Join(directory, "output")
	if err := os.WriteFile(filepath.Join(outputDirectory, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
import { pathToFileURL } from "node:url";
const { createWebhookRouter } = await import(pathToFileURL(process.argv[1]).href);
const seen = [];
const codecs = {
  "application/vnd.example.event": { async decodeInbound(request) { return JSON.parse(await request.text()); } },
  "application/vnd.example.events": { async *decodeInboundStream(reader) { const decoder = new TextDecoder(); let pending = ""; while (true) { const chunk = await reader.read(1024); if (chunk === null) break; pending += decoder.decode(chunk, { stream: true }); let newline; while ((newline = pending.indexOf("\n")) >= 0) { const line = pending.slice(0, newline); pending = pending.slice(newline + 1); if (line !== "") yield JSON.parse(line); } } if (pending !== "") yield JSON.parse(pending); } },
};
const router = createWebhookRouter({ events: { POST: async ({ body }) => { for await (const item of body) seen.push(item.event_id); return { status: 204 }; } }, frames: { POST: async ({ body }) => { for await (const item of body) seen.push(item.frame_id); return { status: 204 }; } }, custom: { POST: async ({ body }) => { seen.push(body.event_id); return { status: 204 }; } }, customStream: { POST: async ({ body }) => { for await (const item of body) seen.push(item.event_id); return { status: 204 }; } }, denied: { POST: async () => ({ status: 204 }) } }, { routes: { events: "/events", frames: "/frames", custom: "/custom", customStream: "/custom-stream", denied: "/denied" }, codecs, maxStreamItemBytes: 1024 });
const encoder = new TextEncoder();
const valid = new ReadableStream({ start(controller) { controller.enqueue(encoder.encode('{"event_id":"one"}\n{"ev')); controller.enqueue(encoder.encode('ent_id":"two"}\n')); controller.close(); } });
const validResponse = await router.fetch(new Request("https://host.test/events", { method: "POST", headers: { "content-type": "application/x-ndjson" }, body: valid, duplex: "half" }));
if (validResponse.status !== 204 || seen.join(",") !== "one,two") throw new Error("inbound NDJSON stream was not decoded");
const invalid = new ReadableStream({ start(controller) { controller.enqueue(encoder.encode('{"wrong":true}\n')); controller.close(); } });
const invalidResponse = await router.fetch(new Request("https://host.test/events", { method: "POST", headers: { "content-type": "application/x-ndjson" }, body: invalid, duplex: "half" }));
if (invalidResponse.status !== 400) throw new Error("invalid inbound stream item was accepted");
const bounded = createWebhookRouter({ events: { POST: async ({ body }) => { for await (const _ of body) { } return { status: 204 }; } } }, { routes: { events: "/events" }, maxStreamItemBytes: 4 });
if ((await bounded.fetch(new Request("https://host.test/events", { method: "POST", headers: { "content-type": "application/x-ndjson" }, body: '{"event_id":"too-long"}' }))).status !== 400) throw new Error("oversized inbound stream item was accepted");
const multipartBody = "--frames\r\ncontent-type: application/json\r\n\r\n{\"frame_id\":\"one\"}\r\n--frames\r\ncontent-type: application/json\r\n\r\n{\"frame_id\":\"two\"}\r\n--frames--\r\n";
const multipartResponse = await router.fetch(new Request("https://host.test/frames", { method: "POST", headers: { "content-type": "multipart/mixed; boundary=frames" }, body: multipartBody }));
if (multipartResponse.status !== 204 || seen.join(",") !== "one,two,one,two") throw new Error("inbound multipart stream was not decoded");
const customResponse = await router.fetch(new Request("https://host.test/custom", { method: "POST", headers: { "content-type": "application/vnd.example.event" }, body: '{"event_id":"three"}' }));
if (customResponse.status !== 204 || seen.join(",") !== "one,two,one,two,three") throw new Error("custom inbound body was not decoded");
const customStreamResponse = await router.fetch(new Request("https://host.test/custom-stream", { method: "POST", headers: { "content-type": "application/vnd.example.events" }, body: '{"event_id":"four"}\n{"event_id":"five"}\n' }));
if (customStreamResponse.status !== 204 || seen.join(",") !== "one,two,one,two,three,four,five") throw new Error("custom inbound stream was not decoded");
const deniedResponse = await router.fetch(new Request("https://host.test/denied", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" }));
if (deniedResponse.status !== 400) throw new Error("false inbound schema accepted a body");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(outputDirectory, "server", "webhooks.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute generated inbound stream server: %v\n%s", err, output)
	}
}

func TestWebhookWithMultipleMethodsUsesOneUnionHandler(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.1",
  "info": {"title": "Webhook", "version": "1"},
  "paths": {},
  "webhooks": {"event": {
    "get": {"operationId": "eventRead", "responses": {"204": {"description": "Accepted"}}},
    "post": {"operationId": "eventWrite", "responses": {"204": {"description": "Accepted"}}}
  }}
}`))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	options, err := registry.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := (Generator{}).Generate(document, options)
	if err != nil {
		t.Fatal(err)
	}
	webhooks := string(artifactByPath(t, artifacts, "server/webhooks.ts"))
	for _, expected := range []string{`readonly "event": {`, `readonly "GET": { readonly context:`, `readonly "POST": { readonly context:`, `readonly "event"?: {`} {
		if !strings.Contains(webhooks, expected) {
			t.Fatalf("multi-method webhook source missing %q:\n%s", expected, webhooks)
		}
	}
}

func TestServerPublicMapsPreserveExactAndPrototypeSensitiveKeys(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.1", "info":{"title":"Exact server identities","version":"1"},
  "paths":{"/source":{"post":{
    "operationId":"source-op",
    "responses":{"204":{"description":"OK"}},
    "callbacks":{
      "status-hook":{"{$request.body#/callbackURL}":{"post":{"operationId":"callback-one","responses":{"204":{"description":"OK"}}}}},
      "status_hook":{"{$request.body#/callbackURL}":{"post":{"operationId":"callback-two","responses":{"204":{"description":"OK"}}}}},
      "__proto__":{"{$request.body#/callbackURL}":{"post":{"operationId":"callback-three","responses":{"204":{"description":"OK"}}}}},
      "constructor":{"{$request.body#/callbackURL}":{"post":{"operationId":"callback-four","responses":{"204":{"description":"OK"}}}}}
    }
  }}},
  "webhooks":{
    "event-hook":{"post":{"operationId":"webhook-one","responses":{"204":{"description":"OK"}}}},
    "event_hook":{"post":{"operationId":"webhook-two","responses":{"204":{"description":"OK"}}}},
    "__proto__":{"post":{"operationId":"webhook-three","responses":{"204":{"description":"OK"}}}},
    "constructor":{"post":{"operationId":"webhook-four","responses":{"204":{"description":"OK"}}}}
  },
  "components":{
    "callbacks":{
      "component-hook":{"{$request.body#/componentURL}":{"post":{"operationId":"component-one","responses":{"204":{"description":"OK"}}}}},
      "component_hook":{"{$request.body#/componentURL}":{"post":{"operationId":"component-two","responses":{"204":{"description":"OK"}}}}},
      "__proto__":{"{$request.body#/componentURL}":{"post":{"operationId":"component-three","responses":{"204":{"description":"OK"}}}}},
      "constructor":{"{$request.body#/componentURL}":{"post":{"operationId":"component-four","responses":{"204":{"description":"OK"}}}}}
    },
    "securitySchemes":{
      "api-key":{"type":"apiKey","in":"header","name":"x-one"},
      "api_key":{"type":"apiKey","in":"header","name":"x-two"},
      "__proto__":{"type":"apiKey","in":"header","name":"x-three"},
      "constructor":{"type":"apiKey","in":"header","name":"x-four"}
    }
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	options, err := registry.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := (Generator{}).Generate(document, options)
	if err != nil {
		t.Fatal(err)
	}
	webhooks := string(artifactByPath(t, artifacts, "server/webhooks.ts"))
	callbacks := string(artifactByPath(t, artifacts, "server/callbacks.ts"))
	for _, key := range []string{"event-hook", "event_hook", "__proto__", "constructor"} {
		if !strings.Contains(webhooks, `readonly `+quoteTS(key)+`: {`) {
			t.Fatalf("exact webhook %q missing:\n%s", key, webhooks)
		}
	}
	for _, key := range []string{"status-hook", "status_hook", "__proto__", "constructor", "component-hook", "component_hook"} {
		if !strings.Contains(callbacks, `readonly `+quoteTS(key)+`: {`) {
			t.Fatalf("exact callback identity %q missing:\n%s", key, callbacks)
		}
	}
	for _, key := range []string{"api-key", "api_key", "__proto__", "constructor"} {
		if !strings.Contains(webhooks, `[`+quoteTS(key)+`, /* @__PURE__ */ Object.fromEntries(`) {
			t.Fatalf("exact security scheme %q missing from safe runtime map:\n%s", key, webhooks)
		}
	}

	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	writeTargetArtifacts(t, source, artifacts)
	if err := os.WriteFile(filepath.Join(source, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "tsconfig.json"), []byte(serverTSConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join("..", "..", "..", "test", "typescript", "node_modules", "typescript", "lib", "tsc.js")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("TypeScript compiler unavailable for exact server identity test: %v", err)
	}
	if output, err := exec.Command("node", tsc, "--project", filepath.Join(source, "tsconfig.json")).CombinedOutput(); err != nil {
		t.Fatalf("compile exact server identity target: %v\n%s", err, output)
	}
	outputDirectory := filepath.Join(directory, "output")
	if err := os.WriteFile(filepath.Join(outputDirectory, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
import { pathToFileURL } from "node:url";
const webhookModule = await import(pathToFileURL(process.argv[1]).href);
const callbackModule = await import(pathToFileURL(process.argv[2]).href);
const names = ["event-hook", "event_hook", "__proto__", "constructor"];
const handlers = Object.fromEntries(names.map((name) => [name, { POST: async () => ({ status: 204 }) }]));
const routes = Object.fromEntries(names.map((name) => [name, "/" + encodeURIComponent(name)]));
const router = webhookModule.createWebhookRouter(handlers, { routes });
for (const name of names) {
  const response = await router.fetch(new Request("https://host.test/" + encodeURIComponent(name), { method: "POST" }));
  if (response.status !== 204) throw new Error("webhook identity did not dispatch: " + name);
}
const expression = "{$request.body#/callbackURL}";
const callbackNames = ["status-hook", "status_hook", "__proto__", "constructor"];
const callbackHandlers = Object.fromEntries(callbackNames.map((name) => [name, Object.fromEntries([[expression, { POST: async () => ({ status: 204 }) }]])]));
const componentExpression = "{$request.body#/componentURL}";
const componentNames = ["component-hook", "component_hook", "__proto__", "constructor"];
const componentHandlers = Object.fromEntries(componentNames.map((name) => [name, Object.fromEntries([[componentExpression, { POST: async () => ({ status: 204 }) }]])]));
const endpoints = callbackModule.createCallbackHandlers({
  callbacks: Object.fromEntries([["source-op", callbackHandlers]]),
  componentCallbacks: componentHandlers,
});
for (const name of callbackNames) {
  if (!Object.prototype.hasOwnProperty.call(endpoints.callbacks["source-op"], name)) throw new Error("callback is not an own property: " + name);
  if ((await endpoints.callbacks["source-op"][name][expression].POST.fetch(new Request("https://host.test/callback", { method: "POST" }))).status !== 204) throw new Error("callback identity did not dispatch: " + name);
}
for (const name of componentNames) {
  if (!Object.prototype.hasOwnProperty.call(endpoints.componentCallbacks, name)) throw new Error("component callback is not an own property: " + name);
  if ((await endpoints.componentCallbacks[name][componentExpression].POST.fetch(new Request("https://host.test/component", { method: "POST" }))).status !== 204) throw new Error("component callback identity did not dispatch: " + name);
}
`
	command := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(outputDirectory, "server", "webhooks.js"), filepath.Join(outputDirectory, "server", "callbacks.js"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("execute exact server identity runtime test: %v\n%s", err, output)
	}
}

func TestServerAddOnDeduplicatesReferencedComponentCallbacks(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.1",
  "info": {"title": "Callback", "version": "1"},
  "paths": {"/orders": {"post": {"operationId": "createOrder", "responses": {"202": {"description": "Accepted"}}, "callbacks": {"orderStatus": {"$ref": "#/components/callbacks/OrderStatus"}}}}},
  "components": {"callbacks": {"OrderStatus": {"{$request.body#/callbackURL}": {"post": {"operationId": "orderStatusCallback", "responses": {"204": {"description": "Accepted"}}}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	options, err := registry.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := (Generator{}).Generate(document, options)
	if err != nil {
		t.Fatal(err)
	}
	callbacks := string(artifactByPath(t, artifacts, "server/callbacks.ts"))
	for _, expected := range []string{
		`export interface Callbacks`,
		`readonly "createOrder": {`,
		`readonly "orderStatus": {`,
		`export interface ComponentCallbacks`,
		`readonly "OrderStatus": {`,
	} {
		if !strings.Contains(callbacks, expected) {
			t.Fatalf("callback map missing %q:\n%s", expected, callbacks)
		}
	}
}

func TestServerAddOnEmitsInboundParameterDefinitions(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.1",
  "info": {"title": "Webhook", "version": "1"},
  "paths": {},
  "webhooks": {"event": {"post": {"parameters": [{"name": "X-Signature", "in": "header", "required": true, "schema": {"type": "string"}}], "responses": {"204": {"description": "Accepted"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	options, err := registry.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := (Generator{}).Generate(document, options)
	if err != nil {
		t.Fatal(err)
	}
	webhooks := string(artifactByPath(t, artifacts, "server/webhooks.ts"))
	for _, expected := range []string{"decodeInboundParameters", `location: "header"`, `name: "X-Signature"`, `property: "X-Signature"`} {
		if !strings.Contains(webhooks, expected) {
			t.Fatalf("webhook parameter metadata missing %q:\n%s", expected, webhooks)
		}
	}
}

func TestServerAddOnPreservesEnvironmentControlledInboundHeaders(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.2.0",
  "info": {"title": "Environment-controlled inbound headers", "version": "1"},
  "paths": {
    "/managed": {
      "post": {
        "operationId": "managedOperation",
        "security": [],
        "parameters": [
          {"name": "Origin", "in": "header", "required": true, "schema": {"type": "string"}},
          {"name": "Sec-Fetch-Site", "in": "header", "schema": {"type": "string"}}
        ],
        "callbacks": {
          "managedCallback": {
            "{$request.body#/callbackURL}": {
              "post": {
                "operationId": "managedCallbackOperation",
                "security": [],
                "parameters": [
                  {"name": "Origin", "in": "header", "required": true, "schema": {"type": "string"}},
                  {"name": "Sec-Fetch-Site", "in": "header", "schema": {"type": "string"}}
                ],
                "responses": {"204": {"description": "Accepted"}}
              }
            }
          }
        },
        "responses": {"202": {"description": "Accepted"}}
      }
    }
  },
  "webhooks": {
    "managed": {
      "post": {
        "operationId": "managedWebhook",
        "security": [],
        "parameters": [
          {"name": "Origin", "in": "header", "required": true, "schema": {"type": "string"}},
          {"name": "Sec-Fetch-Site", "in": "header", "schema": {"type": "string"}}
        ],
        "responses": {"204": {"description": "Accepted"}}
      }
    }
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	options, err := registry.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := (Generator{}).Generate(document, options)
	if err != nil {
		t.Fatal(err)
	}
	webhooks := string(artifactByPath(t, artifacts, "server/webhooks.ts"))
	callbacks := string(artifactByPath(t, artifacts, "server/callbacks.ts"))
	for _, expected := range []string{
		`readonly "Origin": string`,
		`readonly "Sec-Fetch-Site"?: string`,
		`name: "Origin"`,
		`required: true`,
	} {
		if !strings.Contains(webhooks, expected) {
			t.Fatalf("managed inbound header contract missing %q:\n%s", expected, webhooks)
		}
	}
	for _, expected := range []string{
		`readonly "managedOperation"`,
		`readonly "managedCallback"`,
		`readonly "Origin": string`,
		`readonly "Sec-Fetch-Site"?: string`,
		`name: "Origin"`,
		`required: true`,
	} {
		if !strings.Contains(callbacks, expected) {
			t.Fatalf("callback inbound header contract missing %q:\n%s", expected, callbacks)
		}
	}

	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	writeTargetArtifacts(t, source, artifacts)
	if err := os.WriteFile(filepath.Join(source, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "tsconfig.json"), []byte(serverTSConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := `import type { Callbacks } from "./server/callbacks.js"
import type { Webhooks } from "./server/webhooks.js"
declare const webhookParams: Webhooks["managed"]["POST"]["context"]["params"]["headerParams"]
declare const callbackParams: Callbacks["managedOperation"]["managedCallback"]["{$request.body#/callbackURL}"]["POST"]["context"]["params"]["headerParams"]
const webhookOrigin: string = webhookParams.Origin
const webhookSite: string | undefined = webhookParams["Sec-Fetch-Site"]
const callbackOrigin: string = callbackParams.Origin
const callbackSite: string | undefined = callbackParams["Sec-Fetch-Site"]
void webhookOrigin
void webhookSite
void callbackOrigin
void callbackSite
`
	if err := os.WriteFile(filepath.Join(source, "managed-inbound.probe.ts"), []byte(probe), 0o600); err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join("..", "..", "..", "test", "typescript", "node_modules", "typescript", "lib", "tsc.js")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("TypeScript compiler unavailable for server test: %v", err)
	}
	if output, err := exec.Command("node", tsc, "--project", filepath.Join(source, "tsconfig.json")).CombinedOutput(); err != nil {
		t.Fatalf("compile managed inbound server contract: %v\n%s", err, output)
	}
	outputDirectory := filepath.Join(directory, "output")
	if err := os.WriteFile(filepath.Join(outputDirectory, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
import { pathToFileURL } from "node:url";
const { createWebhookRouter } = await import(pathToFileURL(process.argv[1]).href);
const { createCallbackHandlers } = await import(pathToFileURL(process.argv[2]).href);
const { openapi } = await import(pathToFileURL(process.argv[3]).href);
const seen = [];
const router = createWebhookRouter({
  managed: { POST: async ({ params }) => {
    seen.push({ ...params.headerParams });
    return { status: 204 };
  } },
}, { routes: { managed: "/hooks/managed" } });
const present = await router.fetch(new Request("https://host.test/hooks/managed", {
  method: "POST",
  headers: { Origin: "https://caller.example", "Sec-Fetch-Site": "same-origin" },
}));
if (present.status !== 204) throw new Error("present managed inbound headers were rejected");
if (JSON.stringify(seen) !== JSON.stringify([{ Origin: "https://caller.example", "Sec-Fetch-Site": "same-origin" }])) throw new Error("managed inbound headers were not decoded: " + JSON.stringify(seen));
const missing = await router.fetch(new Request("https://host.test/hooks/managed", { method: "POST" }));
if (missing.status !== 400 || !String(await missing.text()).includes("Origin")) throw new Error("missing required Origin was not rejected");
if (seen.length !== 1) throw new Error("missing Origin reached the handler");
const callbackSeen = [];
const callback = createCallbackHandlers({
  callbacks: { managedOperation: { managedCallback: { "{$request.body#/callbackURL}": {
    POST: async ({ params }) => {
      callbackSeen.push({ ...params.headerParams });
      return { status: 204 };
    },
  } } } },
}).callbacks.managedOperation.managedCallback["{$request.body#/callbackURL}"].POST;
const callbackPresent = await callback.fetch(new Request("https://host.test/callback", {
  method: "POST",
  headers: { Origin: "https://caller.example", "Sec-Fetch-Site": "cross-site" },
}));
if (callbackPresent.status !== 204) throw new Error("present callback headers were rejected");
if (JSON.stringify(callbackSeen) !== JSON.stringify([{ Origin: "https://caller.example", "Sec-Fetch-Site": "cross-site" }])) throw new Error("callback headers were not decoded: " + JSON.stringify(callbackSeen));
const callbackMissing = await callback.fetch(new Request("https://host.test/callback", { method: "POST" }));
if (callbackMissing.status !== 400 || !String(await callbackMissing.text()).includes("Origin")) throw new Error("missing callback Origin was not rejected");
if (callbackSeen.length !== 1) throw new Error("missing callback Origin reached the handler");
const operationHeaders = openapi.document.paths["/managed"].post.parameters;
const callbackHeaders = openapi.document.paths["/managed"].post.callbacks.managedCallback["{$request.body#/callbackURL}"].post.parameters;
const webhookHeaders = openapi.document.webhooks.managed.post.parameters;
for (const [name, parameters] of [["operation", operationHeaders], ["callback", callbackHeaders], ["webhook", webhookHeaders]]) {
  const origin = parameters.find((parameter) => parameter.name === "Origin");
  const site = parameters.find((parameter) => parameter.name === "Sec-Fetch-Site");
  if (origin?.required !== true || Object.hasOwn(site, "required")) throw new Error(name + " metadata changed inbound header requiredness");
}
`
	if output, err := exec.Command(
		"node",
		"--input-type=module",
		"--eval",
		script,
		filepath.Join(outputDirectory, "server", "webhooks.js"),
		filepath.Join(outputDirectory, "server", "callbacks.js"),
		filepath.Join(outputDirectory, "metadata.js"),
	).CombinedOutput(); err != nil {
		t.Fatalf("execute managed inbound header server test: %v\n%s", err, output)
	}
}

func TestServerMapsCoverAdditionalOperationsRefsExactParamsAndJSONEquality(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0","info":{"title":"Server mappings","version":"1"},
  "paths":{
    "/source":{"post":{"operationId":"createSource","responses":{"204":{"description":"OK"}},"callbacks":{
      "copied":{"{$request.body#/callback}":{"$ref":"#/components/pathItems/CopyCallback"}}
    }}},
    "/shared-hook":{"additionalOperations":{"Purge":{"operationId":"purgeShared","responses":{"204":{"description":"OK"}}}}},
    "/shared-callback":{"additionalOperations":{"Copy":{"operationId":"copyShared","responses":{"204":{"description":"OK"}}}}}
  },
  "webhooks":{
    "first":{"post":{"operationId":"firstHook","responses":{"204":{"description":"OK"}}}},
    "second":{"post":{"operationId":"secondHook","parameters":[
      {"name":"id","in":"path","required":true,"schema":{"type":"string"}},
      {"name":"id","in":"query","required":true,"schema":{"type":"integer"}},
      {"name":"id","in":"querystring","required":true,"schema":{"type":"string"}},
      {"name":"id","in":"header","required":true,"schema":{"type":"boolean"}},
      {"name":"id","in":"cookie","required":true,"schema":{"type":"string"}}
    ],"responses":{"204":{"description":"OK"}}}},
    "purged":{"$ref":"#/components/pathItems/PurgeHook"},
    "validated":{"post":{"operationId":"validatedHook","requestBody":{"required":true,"content":{"application/json":{"schema":{
      "type":"object","required":["choice","order","constant","zero","items","glyph","decimal","integerUnion","choiceUnion","exactlyOne","notBad","conditional","containsOne","dependent","noItems"],"properties":{
        "choice":{"enum":[{"a":1,"b":2}]},
        "order":{"enum":[[1,2]]},
        "constant":{"const":{"x":1,"y":2}},
        "zero":{"const":0},
        "items":{"type":"array","uniqueItems":true},
        "glyph":{"type":"string","maxLength":1},
        "decimal":{"type":"number","multipleOf":0.1},
        "integerUnion":{"type":["integer","null"]},
        "choiceUnion":{"anyOf":[{"const":"x"},{"const":"y"}]},
        "exactlyOne":{"oneOf":[{"const":1},{"const":2}]},
        "notBad":{"not":{"const":"bad"}},
        "conditional":{"if":{"const":1},"then":{"minimum":2}},
        "containsOne":{"type":"array","contains":{"const":1}},
        "dependent":{"type":"object","dependentRequired":{"a":["b"]}},
        "noItems":{"type":"array","items":false}
      }
    }}}},"responses":{"204":{"description":"OK"}}}},
    "allowBool":{"post":{"operationId":"allowBool","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Allow"}}}},"responses":{"204":{"description":"OK"}}}},
    "denyBool":{"post":{"operationId":"denyBool","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Deny"}}}},"responses":{"204":{"description":"OK"}}}},
    "refSibling":{"post":{"operationId":"refSibling","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/BaseNumber","minimum":10}}}},"responses":{"204":{"description":"OK"}}}},
    "advanced":{"post":{"operationId":"advanced","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["xcount","closed","tuple"],"propertyNames":{"pattern":"^[a-z]+$"},"patternProperties":{"^x":{"type":"integer"}},"properties":{"closed":{"allOf":[{"type":"object","properties":{"id":{"type":"string"}}}],"unevaluatedProperties":false},"tuple":{"type":"array","prefixItems":[{"type":"string"}],"unevaluatedItems":false}},"additionalProperties":false}}}},"responses":{"204":{"description":"OK"}}}},
    "annotation":{"post":{"operationId":"annotation","requestBody":{"required":true,"content":{"application/json":{"schema":{"x-acme-boolean-schema":false}}}},"responses":{"204":{"description":"OK"}}}},
    "encodedRef":{"post":{"operationId":"encodedRef","requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#%2Fcomponents%2Fschemas%2FFoo%2DBar"}}}},"responses":{"204":{"description":"OK"}}}},
    "parameters":{"get":{"operationId":"parameters","parameters":[{"name":"count","in":"query","required":true,"schema":{"$ref":"#/components/schemas/Integer"}},{"name":"enabled","in":"header","required":true,"schema":{"$ref":"#/components/schemas/Boolean"}},{"name":"values","in":"query","required":true,"schema":{"type":"array","items":{"$ref":"#/components/schemas/Integer","minimum":1}}},{"name":"ambiguousValues","in":"query","required":true,"schema":{"type":"array","items":{"allOf":[{"oneOf":[{"type":"string"},{"type":"integer"}]},{"enum":[1,2]}]}}},{"name":"tuple","in":"query","required":true,"explode":false,"schema":{"type":"array","prefixItems":[{"type":"integer"},{"type":"boolean"}],"items":false}},{"name":"variant","in":"query","required":true,"schema":{"oneOf":[{"type":"integer"},{"type":"boolean"}]}},{"name":"constrained","in":"query","required":true,"schema":{"allOf":[{"oneOf":[{"type":"string"},{"type":"integer"}]},{"enum":[1,2]}]}},{"name":"typeless","in":"query","required":true,"schema":{"enum":[1,2]}},{"name":"typelessBool","in":"query","required":true,"schema":{"const":true}},{"name":"dynamicValue","in":"query","required":true,"schema":{"$dynamicRef":"#value"}},{"name":"scalarArray","in":"query","required":true,"schema":{"oneOf":[{"type":"string"},{"type":"array","minItems":2,"items":{"type":"string"}}]}},{"name":"selected","in":"query","required":true,"style":"deepObject","explode":true,"schema":{"type":"object","oneOf":[{"required":["kind","value"],"properties":{"kind":{"const":"n"},"value":{"type":"integer"}}},{"required":["kind","value"],"properties":{"kind":{"const":"s"},"value":{"type":"string"}}}]}},{"name":"dynamic","in":"query","required":true,"style":"deepObject","explode":true,"schema":{"type":"object","additionalProperties":{"type":"integer"}}},{"name":"flags","in":"query","required":true,"style":"deepObject","explode":true,"schema":{"type":"object","patternProperties":{"^x":{"type":"boolean"}},"additionalProperties":false}},{"name":"composed","in":"query","required":true,"schema":{"allOf":[{"type":"integer"},{"minimum":1}]}},{"name":"xmlChoice","in":"query","required":true,"content":{"application/xml":{"schema":{"oneOf":[{"type":"integer"},{"type":"boolean"}],"xml":{"name":"choice"}}}}}],"responses":{"204":{"description":"OK"}}}},
    "formRef":{"post":{"operationId":"formRef","requestBody":{"required":true,"content":{"application/x-www-form-urlencoded":{"schema":{"$ref":"#/components/schemas/BaseForm","required":["enabled","choice","ambiguous","arrayChoice","typelessArray"],"properties":{"count":{"allOf":[{"minimum":1}]},"choice":{"oneOf":[{"const":"true"},{"const":"false"}]},"enabled":{"type":"boolean"},"ambiguous":{"allOf":[{"oneOf":[{"type":"string"},{"type":"integer"}]},{"enum":[1,2]}]},"arrayChoice":{"oneOf":[{"type":"string"},{"type":"array","items":{"type":"integer"}}]},"typelessArray":{"items":{"type":"integer"}}},"oneOf":[{"required":["mode","branchValue"],"properties":{"mode":{"const":"n"},"branchValue":{"type":"integer"}}},{"required":["mode","branchValue"],"properties":{"mode":{"const":"s"},"branchValue":{"type":"string"}}}]}}}},"responses":{"204":{"description":"OK"}}}},
    "schemaless":{"get":{"operationId":"schemaless","responses":{"200":{"description":"OK","content":{"application/json":{}}}}}},
    "falseResponse":{"get":{"operationId":"falseResponse","responses":{"200":{"description":"OK","content":{"application/json":{"schema":false}}}}}},
    "noSchemaRequest":{"post":{"operationId":"noSchemaRequest","requestBody":{"required":true,"content":{"application/json":{}}},"responses":{"204":{"description":"OK"}}}},
    "nullable":{"post":{"operationId":"nullable","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"string","nullable":true}}}},"responses":{"204":{"description":"OK"}}}},
    "falseBinary":{"get":{"operationId":"falseBinary","responses":{"200":{"description":"OK","content":{"application/octet-stream":{"schema":false}}}}}},
    "falseBinaryInput":{"post":{"operationId":"falseBinaryInput","requestBody":{"required":true,"content":{"application/octet-stream":{"schema":false}}},"responses":{"204":{"description":"OK"}}}},
    "xmlRef":{"post":{"operationId":"xmlRef","requestBody":{"required":true,"content":{"application/xml":{"schema":{"$ref":"#/components/schemas/XMLPayload"}}}},"responses":{"204":{"description":"OK"}}}},
    "rootXML":{"post":{"operationId":"rootXML","requestBody":{"required":true,"content":{"application/xml":{"schema":{"$ref":"#/components/schemas/RootTags"}}}},"responses":{"204":{"description":"OK"}}}},
    "inlineRootXML":{"post":{"operationId":"inlineRootXML","requestBody":{"required":true,"content":{"application/xml":{"schema":{"type":"array","xml":{"wrapped":true,"prefix":"p"},"items":{"type":"array","xml":{"wrapped":true,"prefix":"p"},"items":{"type":"string","xml":{"name":"member","prefix":"p"}}}}}}},"responses":{"204":{"description":"OK"}}}},
    "nestedXML":{"post":{"operationId":"nestedXML","requestBody":{"required":true,"content":{"application/xml":{"schema":{"type":"object","required":["matrix"],"properties":{"matrix":{"type":"array","xml":{"name":"matrix","prefix":"p"},"items":{"type":"array","xml":{"wrapped":true},"items":{"type":"string","xml":{"name":"value"}}}}}}}}},"responses":{"204":{"description":"OK"}}}}
  },
  "components":{
    "pathItems":{
      "PurgeHook":{"$ref":"#/paths/~1shared-hook"},
      "CopyCallback":{"$ref":"#/paths/~1shared-callback"}
    },
    "schemas":{"Allow":true,"Deny":false,"BaseNumber":{"type":"number"},"Foo-Bar":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}},"Integer":{"type":"integer"},"DynamicInteger":{"$dynamicAnchor":"value","type":"integer"},"Boolean":{"type":"boolean"},"RootTags":{"type":"array","xml":{"wrapped":true,"prefix":"p"},"items":{"type":"string","xml":{"name":"tag","prefix":"p"}}},"BaseForm":{"type":"object","required":["count","enabled","choice"],"properties":{"count":{"allOf":[{"type":"integer"}]},"enabled":{"type":"boolean"},"choice":{"oneOf":[{"type":"string"},{"type":"boolean"}]},"mode":{"type":"string"},"branchValue":{"oneOf":[{"type":"string"},{"type":"integer"}]},"arrayChoice":{"oneOf":[{"type":"string"},{"type":"array","items":{"type":"integer"}}]},"typelessArray":{"items":{"type":"integer"}}},"additionalProperties":{"type":"integer"}},"BaseTree":{"$id":"https://schemas.example.test/base-tree","$dynamicAnchor":"node","type":"object","properties":{"child":{"$dynamicRef":"#node"}}},"StrictTree":{"$id":"https://schemas.example.test/strict-tree","$dynamicAnchor":"node","allOf":[{"$ref":"#/components/schemas/BaseTree"},{"type":"object","required":["strict"],"properties":{"strict":{"type":"boolean"}}}]},"XMLPayload":{"type":"object","xml":{"name":"payload"},"required":["count","values","flags","choice","variantObject","tree","textValue","wrappedTags"],"properties":{"count":{"$ref":"#/components/schemas/Integer","xml":{"name":"count","prefix":"p","attribute":true}},"values":{"type":"array","xml":{"name":"value"},"items":{"$ref":"#/components/schemas/Integer"}},"flags":{"type":"array","prefixItems":[{"type":"integer","xml":{"name":"flag"}},{"type":"boolean","xml":{"name":"flag"}}],"items":false},"choice":{"oneOf":[{"type":"integer"},{"type":"boolean"}],"xml":{"name":"choice","prefix":"p"}},"variantObject":{"oneOf":[{"type":"object","required":["kind","value"],"properties":{"kind":{"const":"n"},"value":{"type":"integer"}}},{"type":"object","required":["kind","value"],"properties":{"kind":{"const":"s"},"value":{"type":"string"}}}]},"tree":{"$ref":"#/components/schemas/StrictTree"},"textValue":{"type":"integer","xml":{"nodeType":"text"}},"wrappedTags":{"type":"array","xml":{"name":"tags","wrapped":true},"items":{"type":"string"}}},"additionalProperties":false}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	options, err := registry.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := (Generator{}).Generate(document, options)
	if err != nil {
		t.Fatal(err)
	}
	webhooks := string(artifactByPath(t, artifacts, "server/webhooks.ts"))
	callbacks := string(artifactByPath(t, artifacts, "server/callbacks.ts"))
	for _, expected := range []string{
		`readonly "Purge": { readonly context:`,
		`readonly input:`,
		`readonly output:`,
		`readonly response:`,
		`readonly handler:`,
		`readonly endpoint:`,
		`readonly params: Readonly<{ readonly path: Readonly<{ readonly "id": string }>`,
	} {
		if !strings.Contains(webhooks, expected) {
			t.Fatalf("webhook map missing %q:\n%s", expected, webhooks)
		}
	}
	for _, expected := range []string{`readonly "Copy": { readonly context:`, `readonly handler:`, `readonly endpoint:`} {
		if !strings.Contains(callbacks, expected) {
			t.Fatalf("callback map missing %q:\n%s", expected, callbacks)
		}
	}

	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	writeTargetArtifacts(t, source, artifacts)
	probe := `import type { Webhooks } from "./server/webhooks.js"
type Leaf = Webhooks["second"]["POST"]
const handler: Leaf["handler"] = ({ params }) => {
  const path: string = params.path["id"]
  const query: number = params.query["id"]
  const querystring: string = params.querystring["id"]
  const header: boolean = params.headerParams["id"]
  const cookie: string = params.cookieParams["id"]
  void [path, query, querystring, header, cookie]
  // @ts-expect-error exact query parameter is numeric
  const invalid: string = params.query["id"]
  return { status: 204 }
}
const context: Leaf["input"] = null as never
const output: Leaf["output"] = { status: 204 }
const response: Leaf["response"] = output
const endpoint: Leaf["endpoint"] = null as never
void [handler, context, response, endpoint]
`
	if err := os.WriteFile(filepath.Join(source, "probe.ts"), []byte(probe), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "tsconfig.json"), []byte(serverTSConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join("..", "..", "..", "test", "typescript", "node_modules", "typescript", "lib", "tsc.js")
	if _, err := os.Stat(tsc); err != nil {
		t.Skipf("TypeScript compiler unavailable for server mapping test: %v", err)
	}
	if output, err := exec.Command("node", tsc, "--project", filepath.Join(source, "tsconfig.json")).CombinedOutput(); err != nil {
		t.Fatalf("compile generated server mappings: %v\n%s", err, output)
	}
	outputDirectory := filepath.Join(directory, "output")
	if err := os.WriteFile(filepath.Join(outputDirectory, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
import { pathToFileURL } from "node:url";
const webhookModule = await import(pathToFileURL(process.argv[1]).href);
const callbackModule = await import(pathToFileURL(process.argv[2]).href);
let second = 0, purge = 0, valid = 0, advanced = 0, annotation = 0, encoded = 0, parameters = 0, form = 0, nullable = 0, xml = 0, rootXML = 0, inlineRootXML = 0, nestedXML = 0;
let formBody;
let responseBody = { ok: true };
let responseContentType = "application/json";
const router = webhookModule.createWebhookRouter({
  second: { POST: async () => { second++; return { status: 204 }; } },
  purged: { Purge: async () => { purge++; return { status: 204 }; } },
  validated: { POST: async () => { valid++; return { status: 204 }; } },
  allowBool: { POST: async () => ({ status: 204 }) },
  denyBool: { POST: async () => ({ status: 204 }) },
  refSibling: { POST: async () => ({ status: 204 }) },
  advanced: { POST: async () => { advanced++; return { status: 204 }; } },
  annotation: { POST: async () => { annotation++; return { status: 204 }; } },
  encodedRef: { POST: async () => { encoded++; return { status: 204 }; } },
  parameters: { GET: async ({ params }) => { if (params.query.count !== 2 || params.headerParams.enabled !== true || JSON.stringify(params.query.values) !== "[1,2]" || JSON.stringify(params.query.ambiguousValues) !== "[1,2]" || JSON.stringify(params.query.tuple) !== "[2,true]" || ![3, true].includes(params.query.variant) || params.query.constrained !== 1 || params.query.typeless !== 1 || params.query.typelessBool !== true || params.query.dynamicValue !== 4 || params.query.scalarArray !== "foo" || params.query.selected.kind !== "n" || params.query.selected.value !== 1 || params.query.selected.extra !== "kept" || params.query.dynamic.a !== 2 || params.query.flags.xone !== true || params.query.composed !== 3 || params.query.xmlChoice !== 2) throw new Error("referenced parameter was not coerced"); parameters++; return { status: 204 }; } },
  formRef: { POST: async ({ body }) => { formBody = body; form++; return { status: 204 }; } },
  schemaless: { GET: async () => ({ status: 200, contentType: responseContentType, body: responseBody }) },
  falseResponse: { GET: async () => ({ status: 200, body: "invalid" }) },
  noSchemaRequest: { POST: async () => ({ status: 204 }) },
  nullable: { POST: async ({ body }) => { if (body !== "ok") throw new Error("nullable annotation changed the body"); nullable++; return { status: 204 }; } },
  falseBinary: { GET: async () => ({ status: 200, contentType: "application/octet-stream", body: new Uint8Array([1]) }) },
  falseBinaryInput: { POST: async () => ({ status: 204 }) },
  xmlRef: { POST: async ({ body }) => { if (body.count !== 2 || JSON.stringify(body.values) !== "[3,4]" || JSON.stringify(body.flags) !== "[2,true]" || body.choice !== 2 || body.variantObject.kind !== "n" || body.variantObject.value !== 1 || body.tree.strict !== true || body.tree.child.strict !== false || body.tree.child.child.strict !== true || body.textValue !== 5 || JSON.stringify(body.wrappedTags) !== '["ok"]') throw new Error("referenced XML values were not coerced"); xml++; return { status: 204 }; } },
  rootXML: { POST: async ({ body }) => { if (JSON.stringify(body) !== '["ok"]') throw new Error("root wrapped XML values were not coerced"); rootXML++; return { status: 204 }; } },
  inlineRootXML: { POST: async ({ body }) => { if (JSON.stringify(body) !== '[["ok"]]') throw new Error("inline root wrapped XML values were not coerced"); inlineRootXML++; return { status: 204 }; } },
  nestedXML: { POST: async ({ body }) => { if (JSON.stringify(body.matrix) !== '[["ok"]]') throw new Error("nested wrapped XML values were not coerced"); nestedXML++; return { status: 204 }; } },
}, { routes: { first: "/same/{id}", second: "/same/{id}", purged: "/purged", validated: "/validated", allowBool: "/allow", denyBool: "/deny", refSibling: "/ref-sibling", advanced: "/advanced", annotation: "/annotation", encodedRef: "/encoded", parameters: "/parameters", formRef: "/form", schemaless: "/schemaless", falseResponse: "/false-response", noSchemaRequest: "/no-schema-request", nullable: "/nullable", falseBinary: "/false-binary", falseBinaryInput: "/false-binary-input", xmlRef: "/xml-ref", rootXML: "/root-xml", inlineRootXML: "/inline-root-xml", nestedXML: "/nested-xml" } });
const same = await router.fetch(new Request("https://host.test/same/value?id=7", { method: "POST", headers: { id: "true", cookie: "id=cookie" } }));
if (same.status !== 204 || second !== 1) throw new Error("unhandled webhook shadowed a handled route");
if ((await router.fetch(new Request("https://host.test/purged", { method: "Purge" }))).status !== 204 || purge !== 1) throw new Error("referenced mixed-case additional webhook did not dispatch");
const accepted = '{"choice":{"b":2,"a":1},"order":[1,2],"constant":{"y":2,"x":1},"zero":-0,"items":[{"a":1,"b":2}],"glyph":"😀","decimal":0.3,"integerUnion":2,"choiceUnion":"x","exactlyOne":1,"notBad":"good","conditional":3,"containsOne":[2,1],"dependent":{"a":1,"b":2},"noItems":[]}';
if ((await router.fetch(new Request("https://host.test/validated", { method: "POST", headers: { "content-type": "application/json" }, body: accepted }))).status !== 204 || valid !== 1) throw new Error("valid structural JSON equality was rejected");
const baseline = JSON.parse(accepted);
const changedChoice = { ...baseline, choice: { a: 1, b: 3 } };
if ((await router.fetch(new Request("https://host.test/validated", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(changedChoice) }))).status !== 400 || valid !== 1) throw new Error("changed object enum value was accepted");
const reorderedArray = { ...baseline, order: [2, 1] };
if ((await router.fetch(new Request("https://host.test/validated", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(reorderedArray) }))).status !== 400 || valid !== 1) throw new Error("reordered array enum value was accepted");
const signedZeroDuplicate = JSON.stringify({ ...baseline, items: [] }).replace('"items":[]', '"items":[0,-0]');
if ((await router.fetch(new Request("https://host.test/validated", { method: "POST", headers: { "content-type": "application/json" }, body: signedZeroDuplicate }))).status !== 400 || valid !== 1) throw new Error("signed-zero uniqueItems duplicate was accepted");
for (const [name, patch] of [
  ["unicode length", { glyph: "😀😀" }],
  ["decimal multipleOf", { decimal: 0.31 }],
  ["integer union", { integerUnion: 1.5 }],
  ["anyOf", { choiceUnion: "z" }],
  ["oneOf", { exactlyOne: 3 }],
  ["not", { notBad: "bad" }],
  ["conditional", { conditional: 1 }],
  ["contains", { containsOne: [2] }],
  ["dependentRequired", { dependent: { a: 1 } }],
  ["nested false schema", { noItems: [1] }],
]) {
  const response = await router.fetch(new Request("https://host.test/validated", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ ...baseline, ...patch }) }));
  if (response.status !== 400 || valid !== 1) throw new Error(name + " constraint was ignored");
}
const jsonPost = (path, body) => router.fetch(new Request("https://host.test" + path, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body) }));
if ((await jsonPost("/allow", { any: "value" })).status !== 204) throw new Error("true component schema was not accepted");
if ((await jsonPost("/deny", { any: "value" })).status !== 400) throw new Error("false component schema was accepted");
if ((await jsonPost("/ref-sibling", 5)).status !== 400 || (await jsonPost("/ref-sibling", 10)).status !== 204) throw new Error("$ref assertion sibling was ignored");
if ((await jsonPost("/advanced", { xcount: 1, closed: { id: "ok" }, tuple: ["ok"] })).status !== 204 || advanced !== 1) throw new Error("valid advanced schema was rejected");
for (const [name, body] of [
  ["propertyNames", { Xcount: 1, closed: { id: "ok" }, tuple: ["ok"] }],
  ["patternProperties", { xcount: "bad", closed: { id: "ok" }, tuple: ["ok"] }],
  ["additionalProperties", { xcount: 1, extra: true, closed: { id: "ok" }, tuple: ["ok"] }],
  ["unevaluatedProperties", { xcount: 1, closed: { id: "ok", extra: true }, tuple: ["ok"] }],
  ["unevaluatedItems", { xcount: 1, closed: { id: "ok" }, tuple: ["ok", "extra"] }],
]) if ((await jsonPost("/advanced", body)).status !== 400 || advanced !== 1) throw new Error(name + " constraint was ignored");
if ((await jsonPost("/annotation", "allowed")).status !== 204 || annotation !== 1) throw new Error("annotation collided with boolean schema state");
if ((await jsonPost("/encoded", { id: "ok" })).status !== 204 || (await jsonPost("/encoded", {})).status !== 400 || encoded !== 1) throw new Error("URI-encoded component reference was not resolved");
const parameterURL = (variant, values = "values=1&values=2") => "https://host.test/parameters?count=2&" + values + "&ambiguousValues=1&ambiguousValues=2&tuple=2,true&variant=" + variant + "&constrained=1&typeless=1&typelessBool=true&dynamicValue=4&scalarArray=foo&selected[kind]=n&selected[value]=1&selected[extra]=kept&dynamic[a]=2&flags[xone]=true&composed=3&xmlChoice=%3Cchoice%3E2%3C%2Fchoice%3E";
if ((await router.fetch(new Request(parameterURL("3"), { headers: { enabled: "true" } }))).status !== 204 || (await router.fetch(new Request(parameterURL("true"), { headers: { enabled: "true" } }))).status !== 204 || parameters !== 2) throw new Error("referenced/variant parameters did not dispatch");
if ((await router.fetch(new Request(parameterURL("3", "values=0"), { headers: { enabled: "true" } }))).status !== 400 || parameters !== 2) throw new Error("referenced item assertion sibling was ignored");
if ((await router.fetch(new Request(parameterURL("3") + "&flags[bad]=true", { headers: { enabled: "true" } }))).status !== 400 || parameters !== 2) throw new Error("closed deep-object field was sanitized instead of rejected");
for (const invalidXML of ["%3Cchoice%3E", "%3Cchoice%3Enope%3C%2Fchoice%3E"]) {
  if ((await router.fetch(new Request(parameterURL("3").replace("%3Cchoice%3E2%3C%2Fchoice%3E", invalidXML), { headers: { enabled: "true" } }))).status !== 400 || parameters !== 2) throw new Error("invalid XML parameter was not rejected");
}
const validForm = await router.fetch(new Request("https://host.test/form", { method: "POST", headers: { "content-type": "application/x-www-form-urlencoded" }, body: "count=2&enabled=true&choice=true&ambiguous=1&arrayChoice=1&arrayChoice=2&typelessArray=1&typelessArray=2&mode=n&branchValue=1&extra=3" }));
if (validForm.status !== 204 || form !== 1 || formBody.count !== 2 || formBody.enabled !== true || formBody.choice !== "true" || formBody.ambiguous !== 1 || JSON.stringify(formBody.arrayChoice) !== "[1,2]" || JSON.stringify(formBody.typelessArray) !== "[1,2]" || formBody.mode !== "n" || formBody.branchValue !== 1 || formBody.extra !== 3) throw new Error("referenced form body did not dispatch: " + validForm.status + " " + await validForm.text() + " " + JSON.stringify(formBody));
if ((await router.fetch(new Request("https://host.test/form", { method: "POST", headers: { "content-type": "application/x-www-form-urlencoded" }, body: "count=0&enabled=true&choice=true&ambiguous=1&arrayChoice=1&arrayChoice=2&typelessArray=1&typelessArray=2&mode=n&branchValue=1&extra=3" }))).status !== 400 || form !== 1) throw new Error("overlapping referenced form property constraint was ignored");
if ((await router.fetch(new Request("https://host.test/false-response"))).status !== 500) throw new Error("false response schema accepted a body");
if ((await jsonPost("/no-schema-request", { any: "value" })).status !== 204) throw new Error("schema-less request media type was rejected");
if ((await jsonPost("/nullable", "ok")).status !== 204 || nullable !== 1 || (await jsonPost("/nullable", null)).status !== 400) throw new Error("3.1+ nullable annotation changed validation");
if ((await router.fetch(new Request("https://host.test/false-binary"))).status !== 500) throw new Error("false binary response schema accepted a body");
if ((await router.fetch(new Request("https://host.test/false-binary-input", { method: "POST", headers: { "content-type": "application/octet-stream" }, body: new Uint8Array([1]) }))).status !== 400) throw new Error("false binary request schema accepted a body");
if ((await router.fetch(new Request("https://host.test/xml-ref", { method: "POST", headers: { "content-type": "application/xml" }, body: '<payload xmlns="https://schemas.example.test/payload" xmlns:p="https://schemas.example.test/qualified" p:count="2">5<value>3</value><value>4</value><flag>2</flag><flag>true</flag><p:choice>2</p:choice><variantObject><kind>n</kind><value>1</value></variantObject><tree><strict>true</strict><child><strict>false</strict><child><strict>true</strict></child></child></tree><tags><tags>ok</tags></tags></payload>' }))).status !== 204 || xml !== 1) throw new Error("referenced XML body did not dispatch");
if ((await router.fetch(new Request("https://host.test/xml-ref", { method: "POST", headers: { "content-type": "application/xml" }, body: '<payload xmlns:p="https://schemas.example.test/qualified" p:count="1" p:count="2">5<value>3</value><value>4</value><flag>2</flag><flag>true</flag><p:choice>2</p:choice><variantObject><kind>n</kind><value>1</value></variantObject><tree><strict>true</strict><child><strict>false</strict><child><strict>true</strict></child></child></tree><tags><tags>ok</tags></tags></payload>' }))).status !== 400 || xml !== 1) throw new Error("duplicate XML attribute was accepted");
if ((await router.fetch(new Request("https://host.test/xml-ref", { method: "POST", headers: { "content-type": "application/xml" }, body: '<payload xmlns:p="https://schemas.example.test/qualified" p:count="2">5<value>3</value><value>4</value><flag>2</flag><flag>true</flag><p:choice>2</p:choice><variantObject><kind>n</kind><value>1</value></variantObject><tree><strict>true</strict><child><strict>false</strict><child><strict>true</strict></child></child></tree><tags><tags>ok</tags><extra>bad</extra></tags></payload>' }))).status !== 400 || xml !== 1) throw new Error("wrapped XML field was sanitized instead of rejected");
if ((await router.fetch(new Request("https://host.test/xml-ref", { method: "POST", headers: { "content-type": "application/xml" }, body: '<payload xmlns:p="https://schemas.example.test/qualified" p:count="2">5<value>3</value><value>4</value><flag>2</flag><flag>true</flag><p:choice>2</p:choice><variantObject><kind>n</kind><value>1</value></variantObject><tree><strict>true</strict><child><strict>false</strict><child><strict>true</strict></child></child></tree><tags extra="bad"><tags>ok</tags></tags></payload>' }))).status !== 400 || xml !== 1) throw new Error("wrapped XML attribute was sanitized instead of rejected");
if ((await router.fetch(new Request("https://host.test/xml-ref", { method: "POST", headers: { "content-type": "application/xml" }, body: '<payload xmlns:p="https://schemas.example.test/qualified" p:count="2">5<value>3</value><value>4</value><flag>2</flag><flag>true</flag><p:choice>2</p:choice><variantObject><kind>n</kind><value>1</value></variantObject><tree><strict>true</strict><child><strict>false</strict><child><strict>true</strict></child></child></tree><tags>bad<tags>ok</tags></tags></payload>' }))).status !== 400 || xml !== 1) throw new Error("wrapped XML text was sanitized instead of rejected");
if ((await router.fetch(new Request("https://host.test/root-xml", { method: "POST", headers: { "content-type": "application/xml" }, body: '<p:RootTags xmlns:p="https://schemas.example.test/tags"><p:tag>ok</p:tag></p:RootTags>' }))).status !== 204 || rootXML !== 1) throw new Error("referenced root wrapped XML body did not dispatch");
for (const body of ['<p:wrong><p:tag>ok</p:tag></p:wrong>', '<p:RootTags><p:wrong>ok</p:wrong></p:RootTags>', '<p:RootTags extra="bad"><p:tag>ok</p:tag></p:RootTags>', '<p:RootTags>bad<p:tag>ok</p:tag></p:RootTags>']) {
  if ((await router.fetch(new Request("https://host.test/root-xml", { method: "POST", headers: { "content-type": "application/xml" }, body }))).status !== 400 || rootXML !== 1) throw new Error("invalid root wrapped XML was sanitized instead of rejected: " + body);
}
if ((await router.fetch(new Request("https://host.test/inline-root-xml", { method: "POST", headers: { "content-type": "application/xml" }, body: '<p:root><p:root><p:member>ok</p:member></p:root></p:root>' }))).status !== 204 || inlineRootXML !== 1) throw new Error("inline nested root wrapped XML body did not dispatch");
if ((await router.fetch(new Request("https://host.test/inline-root-xml", { method: "POST", headers: { "content-type": "application/xml" }, body: '<p:wrong><p:root><p:member>ok</p:member></p:root></p:wrong>' }))).status !== 400 || inlineRootXML !== 1) throw new Error("inline root wrapped XML accepted a wrong root");
if ((await router.fetch(new Request("https://host.test/nested-xml", { method: "POST", headers: { "content-type": "application/xml" }, body: '<root xmlns:p="https://schemas.example.test/matrix"><p:matrix><value>ok</value></p:matrix></root>' }))).status !== 204 || nestedXML !== 1) throw new Error("prefixed unwrapped XML array lost its nested fallback");
if ((await router.fetch(new Request("https://host.test/schemaless"))).status !== 200) throw new Error("valid schemaless JSON response was rejected");
responseContentType = "application/json; charset=utf-8";
if ((await router.fetch(new Request("https://host.test/schemaless"))).status !== 200) throw new Error("parameterized JSON response media type was not normalized");
responseContentType = "application/json";
responseBody = [NaN];
if ((await router.fetch(new Request("https://host.test/schemaless"))).status !== 500) throw new Error("non-finite schemaless JSON response was serialized lossily");
responseBody = Array(2); responseBody[1] = null;
if ((await router.fetch(new Request("https://host.test/schemaless"))).status !== 500) throw new Error("sparse schemaless JSON response was serialized lossily");
for (const body of [new Map([["x", 1]]), new Date(0), { toJSON() { return null; } }, new Number(NaN)]) {
  responseBody = body;
  if ((await router.fetch(new Request("https://host.test/schemaless"))).status !== 500) throw new Error("non-JSON object was serialized lossily");
}
responseBody = [1]; responseBody.extra = 2;
if ((await router.fetch(new Request("https://host.test/schemaless"))).status !== 500) throw new Error("array property was serialized lossily");
for (const body of [
  new (class extends Array {}) (1),
  Object.defineProperty([1], "extra", { value: 2 }),
  Object.defineProperty([1], "0", { enumerable: true, get() { return 1; } }),
  Object.defineProperty({}, "value", { enumerable: true, get() { return 1; } }),
]) {
  responseBody = body;
  if ((await router.fetch(new Request("https://host.test/schemaless"))).status !== 500) throw new Error("custom JSON property behavior was serialized lossily");
}
const endpoints = callbackModule.createCallbackHandlers({ callbacks: { createSource: { copied: { "{$request.body#/callback}": { Copy: async () => ({ status: 204 }) } } } } });
if ((await endpoints.callbacks.createSource.copied["{$request.body#/callback}"].Copy.fetch(new Request("https://host.test/callback", { method: "Copy" }))).status !== 204) throw new Error("referenced mixed-case additional callback did not dispatch");
`
	command := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(outputDirectory, "server", "webhooks.js"), filepath.Join(outputDirectory, "server", "callbacks.js"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("execute generated server mapping runtime test: %v\n%s", err, output)
	}
}

const serverTSConfig = `{
  "compilerOptions": {"target":"ES2022","module":"NodeNext","moduleResolution":"NodeNext","strict":true,"skipLibCheck":true,"rootDir":".","outDir":"../output"},
  "include": ["**/*.ts"]
}`
