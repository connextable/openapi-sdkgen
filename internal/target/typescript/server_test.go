package typescript

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sdkgen "github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/generator"
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
	  {"name":"filter","in":"query","style":"deepObject","explode":true,"schema":{"type":"object","required":["kind_name","count"],"properties":{"kind_name":{"type":"string"},"count":{"type":"integer"}}}},
	  {"name":"meta","in":"header","style":"simple","explode":true,"schema":{"type":"object","required":["trace_id","enabled"],"properties":{"trace_id":{"type":"integer"},"enabled":{"type":"boolean"}}}},
	  {"name":"payload","in":"query","content":{"application/xml":{"schema":{"type":"object","required":["event_id"],"properties":{"event_id":{"type":"string","xml":{"name":"event"}}}}}}},
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
const response = await router.fetch(new Request("https://host.test/hooks/orders?page=2&filter[kind_name]=fresh&filter[count]=3&payload=%3Cpayload%3E%3Cevent%3Exml-event%3C%2Fevent%3E%3C%2Fpayload%3E", { method: "POST", headers: { "content-type": "application/json", "x-signature": "sig-1", "x-trace": "trace-1", "meta": "trace_id=4,enabled=true", "custom": "custom-event", "cookie": "session=one; tags=one; tags=two; theme=dark; event_id=a%2Fb" }, body: JSON.stringify({ id: "order-1" }) }));
if (response.status !== 202 || response.headers.get("x-rate") !== "2" || response.headers.get("x-list") !== "1,2" || response.headers.get("x-object") !== "event_id=outbound" || response.headers.get("x-meta") !== '{"event_id":"outbound"}' || response.headers.get("x-custom") !== "custom:custom-outbound" || JSON.stringify(await response.json()) !== JSON.stringify({ accepted: "order-1" })) throw new Error("handler response was not encoded");
const plain = await router.fetch(new Request("https://host.test/hooks/plain", { method: "GET" }));
if (plain.status !== 200 || plain.headers.get("content-type") !== "application/vnd.example.plain" || await plain.text() !== "custom:plain") throw new Error("custom response was not encoded");
const failedNoBodyHandler = await router.fetch(new Request("https://host.test/hooks/plain?fail=1", { method: "GET" }));
if (failedNoBodyHandler.status !== 500 || await failedNoBodyHandler.text() !== "Internal Server Error") throw new Error("no-body handler error leaked");
const binary = await router.fetch(new Request("https://host.test/hooks/binary", { method: "GET" }));
if (binary.status !== 200 || binary.headers.get("content-type") !== "application/pdf" || JSON.stringify([...new Uint8Array(await binary.arrayBuffer())]) !== "[1,2,3]") throw new Error("binary response was not encoded");
const xml = await router.fetch(new Request("https://host.test/hooks/xml", { method: "GET" }));
if (xml.status !== 200 || xml.headers.get("content-type") !== "application/xml" || await xml.text() !== '<receipt id="receipt-1"><message>hello &amp; goodbye</message></receipt>') throw new Error("XML response was not encoded from its schema");
if (JSON.stringify(seen) !== JSON.stringify([{ body: { id: "order-1" }, operationID: "orderCreatedWebhook", method: "POST", params: { path: {}, query: { page: 2, filter: { kind_name: "fresh", count: 3 }, payload: { event_id: "xml-event" } }, querystring: {}, headerParams: { meta: { trace_id: 4, enabled: true }, custom: { event_id: "custom-event" }, "X-Trace": "trace-1" }, cookieParams: { tags: ["one", "two"], prefs: { event_id: "a%2Fb", theme: "dark" }, session: "one" } } }])) throw new Error("handler context mismatch: " + JSON.stringify(seen));
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
	for _, expected := range []string{"createCallbackHandlers", `export interface Callbacks`, `readonly "createOrder"`, `readonly "orderStatus"`, "{$request.body#/callbackURL}", "No route is generated", `as unknown as CallbackEndpoints["callbacks"]`, `as unknown as CallbackEndpoints["componentCallbacks"]`} {
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
const callbacks = codecs.createCallbackHandlers({ callbacks: { createOrder: { orderStatus: { "{$request.body#/callbackURL}": { POST: async ({ body, operationID, method, request }) => {
  seen.push({ body, operationID, method, path: new URL(request.url).pathname });
  return { status: 204 };
} } } } } }, { codecs: { "application/vnd.example.callback": { async decodeInbound(request) { return JSON.parse(await request.text()); } } }, authenticate: ({ security }) => {
  if (JSON.stringify(security) !== JSON.stringify([{ signature: [] }])) throw new Error("callback security metadata mismatch");
} });
const endpoint = callbacks.callbacks.createOrder.orderStatus["{$request.body#/callbackURL}"].POST;
const response = await endpoint.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "application/vnd.example.callback" }, body: JSON.stringify({ id: "order-1" }) }));
if (response.status !== 204) throw new Error("callback response was not encoded");
if (JSON.stringify(seen) !== JSON.stringify([{ body: { id: "order-1" }, operationID: "orderStatusCallback", method: "POST", path: "/callback" }])) throw new Error("callback context mismatch");
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

func TestServerPublicCatalogsPreserveExactAndPrototypeSensitiveKeys(t *testing.T) {
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
		if !strings.Contains(webhooks, `[`+quoteTS(key)+`, Object.fromEntries(`) {
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
			t.Fatalf("callback catalog missing %q:\n%s", expected, callbacks)
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

func TestServerCatalogsCoverAdditionalOperationsRefsExactParamsAndJSONEquality(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.2.0","info":{"title":"Server catalog","version":"1"},
  "paths":{"/source":{"post":{"operationId":"createSource","responses":{"204":{"description":"OK"}},"callbacks":{
    "copied":{"{$request.body#/callback}":{"$ref":"#/components/pathItems/CopyCallback"}}
  }}}},
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
      "type":"object","required":["choice","constant","items"],"properties":{
        "choice":{"enum":[{"a":1,"b":2}]},
        "constant":{"const":{"x":1,"y":2}},
        "items":{"type":"array","uniqueItems":true,"items":{"type":"object"}}
      }
    }}}},"responses":{"204":{"description":"OK"}}}}
  },
  "components":{"pathItems":{
    "PurgeHook":{"additionalOperations":{"PURGE":{"operationId":"purgeHook","responses":{"204":{"description":"OK"}}}}},
    "CopyCallback":{"additionalOperations":{"COPY":{"operationId":"copyCallback","responses":{"204":{"description":"OK"}}}}}
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
	callbacks := string(artifactByPath(t, artifacts, "server/callbacks.ts"))
	for _, expected := range []string{
		`readonly "PURGE": { readonly context:`,
		`readonly input:`,
		`readonly output:`,
		`readonly response:`,
		`readonly handler:`,
		`readonly endpoint:`,
		`readonly params: Readonly<{ readonly path: Readonly<{ readonly "id": string }>`,
	} {
		if !strings.Contains(webhooks, expected) {
			t.Fatalf("webhook catalog missing %q:\n%s", expected, webhooks)
		}
	}
	for _, expected := range []string{`readonly "COPY": { readonly context:`, `readonly handler:`, `readonly endpoint:`} {
		if !strings.Contains(callbacks, expected) {
			t.Fatalf("callback catalog missing %q:\n%s", expected, callbacks)
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
		t.Skipf("TypeScript compiler unavailable for server catalog test: %v", err)
	}
	if output, err := exec.Command("node", tsc, "--project", filepath.Join(source, "tsconfig.json")).CombinedOutput(); err != nil {
		t.Fatalf("compile generated server catalog: %v\n%s", err, output)
	}
	outputDirectory := filepath.Join(directory, "output")
	if err := os.WriteFile(filepath.Join(outputDirectory, "package.json"), []byte(`{"type":"module"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
import { pathToFileURL } from "node:url";
const webhookModule = await import(pathToFileURL(process.argv[1]).href);
const callbackModule = await import(pathToFileURL(process.argv[2]).href);
let second = 0, purge = 0, valid = 0;
const router = webhookModule.createWebhookRouter({
  second: { POST: async () => { second++; return { status: 204 }; } },
  purged: { PURGE: async () => { purge++; return { status: 204 }; } },
  validated: { POST: async () => { valid++; return { status: 204 }; } },
}, { routes: { first: "/same/{id}", second: "/same/{id}", purged: "/purged", validated: "/validated" } });
const same = await router.fetch(new Request("https://host.test/same/value?id=7", { method: "POST", headers: { id: "true", cookie: "id=cookie" } }));
if (same.status !== 204 || second !== 1) throw new Error("unhandled webhook shadowed a handled route");
if ((await router.fetch(new Request("https://host.test/purged", { method: "PURGE" }))).status !== 204 || purge !== 1) throw new Error("referenced additional webhook did not dispatch");
const accepted = { choice: { b: 2, a: 1 }, constant: { y: 2, x: 1 }, items: [{ a: 1, b: 2 }] };
if ((await router.fetch(new Request("https://host.test/validated", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(accepted) }))).status !== 204 || valid !== 1) throw new Error("reordered JSON object was rejected");
const duplicate = { ...accepted, items: [{ a: 1, b: 2 }, { b: 2, a: 1 }] };
if ((await router.fetch(new Request("https://host.test/validated", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(duplicate) }))).status !== 400 || valid !== 1) throw new Error("reordered uniqueItems duplicate was accepted");
const endpoints = callbackModule.createCallbackHandlers({ callbacks: { createSource: { copied: { "{$request.body#/callback}": { COPY: async () => ({ status: 204 }) } } } } });
if ((await endpoints.callbacks.createSource.copied["{$request.body#/callback}"].COPY.fetch(new Request("https://host.test/callback", { method: "COPY" }))).status !== 204) throw new Error("referenced additional callback did not dispatch");
`
	command := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(outputDirectory, "server", "webhooks.js"), filepath.Join(outputDirectory, "server", "callbacks.js"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("execute generated server catalog runtime test: %v\n%s", err, output)
	}
}

const serverTSConfig = `{
  "compilerOptions": {"target":"ES2022","module":"NodeNext","moduleResolution":"NodeNext","strict":true,"skipLibCheck":true,"rootDir":".","outDir":"../output"},
  "include": ["**/*.ts"]
}`
