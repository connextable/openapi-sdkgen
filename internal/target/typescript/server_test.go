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
  binary: async () => ({ status: 200, contentType: "application/pdf", body: new Uint8Array([1, 2, 3]) }),
  plain: async ({ request }) => { if (new URL(request.url).searchParams.has("fail")) throw new Error("private no-body handler detail"); return { status: 200, contentType: "application/vnd.example.plain", body: "plain" }; },
  xml: async () => ({ status: 200, contentType: "application/xml", body: { id: "receipt-1", note: "hello & goodbye" } }),
  selectors: async ({ params }) => { selectorParams = params; return { status: 204 }; },
	orderCreated: async ({ body, operationID, request, params }) => {
	if (body.id === "explode") throw new Error("private handler detail");
	if (body.id === "missing-header") return { status: 202, body: { accepted: body.id } };
	if (body.id === "invalid-header") return { status: 202, headers: { "x-rate": "nope" }, body: { accepted: body.id } };
	if (body.id === "raw-list") return { status: 202, headers: { "x-rate": "2", "x-list": "1,2" }, body: { accepted: body.id } };
	if (body.id === "raw-custom") return { status: 202, headers: { "x-rate": "2", "x-custom": "custom:raw-outbound" }, body: { accepted: body.id } };
	seen.push({ body, operationID, method: request.method, params });
    return { status: 202, headerValues: { xRate: 2, xList: [1, 2], xObject: { eventID: "outbound" }, xMeta: { eventID: "outbound" }, xCustom: { eventID: "custom-outbound" } }, body: { accepted: body.id } };
  },
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
if (JSON.stringify(seen) !== JSON.stringify([{ body: { id: "order-1" }, operationID: "orderCreatedWebhook", method: "POST", params: { page: 2, filter: { count: 3, kindName: "fresh" }, meta: { enabled: true, traceID: 4 }, payload: { eventID: "xml-event" }, custom: { eventID: "custom-event" }, xTrace: "trace-1", tags: ["one", "two"], prefs: { theme: "dark", eventID: "a%2Fb" }, session: "one" } }])) throw new Error("handler context mismatch");
const selectorResponse = await router.fetch(new Request("https://host.test/hooks/selectors/.role,admin,enabled,true/;matrix=role,owner,enabled,false", { method: "GET" }));
if (selectorResponse.status !== 204 || JSON.stringify(selectorParams) !== JSON.stringify({ label: { role: "admin", enabled: true }, matrix: { role: "owner", enabled: false } })) throw new Error("label/matrix path objects were not decoded");
const denied = createWebhookRouter({ orderCreated: async () => ({ status: 202 }) }, { routes: { orderCreated: "/hooks/orders" }, authenticate: () => new Response("no", { status: 401 }) });
if ((await denied.fetch(new Request("https://host.test/hooks/orders?page=2", { method: "POST", headers: { "content-type": "application/json", "x-trace": "trace-1", "cookie": "session=one" }, body: "{}" }))).status !== 401) throw new Error("authentication response was ignored");
const defaultDenied = createWebhookRouter({ orderCreated: async () => ({ status: 202 }) }, { routes: { orderCreated: "/hooks/orders" } });
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
  try { createWebhookRouter({ orderCreated: async () => ({ status: 202 }) }, { routes }); throw new Error("invalid route was accepted"); }
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
	for _, expected := range []string{"createCallbackHandlers", "OrderStatusCallbackContext", "{$request.body#/callbackURL}", "No route is generated"} {
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
const callbacks = codecs.createCallbackHandlers({ orderStatus: async ({ body, operationID, method, request }) => {
  seen.push({ body, operationID, method, path: new URL(request.url).pathname });
  return { status: 204 };
} }, { codecs: { "application/vnd.example.callback": { async decodeInbound(request) { return JSON.parse(await request.text()); } } }, authenticate: ({ security }) => {
  if (JSON.stringify(security) !== JSON.stringify([{ signature: [] }])) throw new Error("callback security metadata mismatch");
} });
const response = await callbacks.orderStatus.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "application/vnd.example.callback" }, body: JSON.stringify({ id: "order-1" }) }));
if (response.status !== 204) throw new Error("callback response was not encoded");
if (JSON.stringify(seen) !== JSON.stringify([{ body: { id: "order-1" }, operationID: "orderStatusCallback", method: "POST", path: "/callback" }])) throw new Error("callback context mismatch");
if ((await callbacks.orderStatus.fetch(new Request("https://host.test/callback", { method: "GET" }))).status !== 405) throw new Error("wrong callback method was accepted");
if ((await callbacks.orderStatus.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "text/plain" }, body: "bad" }))).status !== 415) throw new Error("bad callback media type was accepted");
if ((await callbacks.orderStatus.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "application/vnd.example.callback" }, body: "{}" }))).status !== 400) throw new Error("schema-invalid callback was accepted");
if ((await codecs.createCallbackHandlers({}).orderStatus.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "application/vnd.example.callback" }, body: "{}" }))).status !== 404) throw new Error("missing callback handler was accepted");
const denied = codecs.createCallbackHandlers({ orderStatus: async () => ({ status: 204 }) }, { authenticate: () => new Response("Unauthorized", { status: 401 }) });
if ((await denied.orderStatus.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "application/vnd.example.callback" }, body: JSON.stringify({ id: "order-1" }) }))).status !== 401) throw new Error("callback authentication response was ignored");
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
  formReceived: async ({ body }) => { if (body.count !== 2 || body.enabled !== true || body.tags.join(",") !== "one,two" || body.meta.source !== "form") throw new Error("form values were not typed"); seen.push(body); return { status: 204 }; },
  textReceived: async ({ body }) => { seen.push(body); return { status: 204 }; },
  xmlReceived: async ({ body }) => { seen.push(body); return { status: 204 }; },
  multipartReceived: async ({ body }) => { if (body.meta.source !== "multipart" || body.custom.source !== "custom") throw new Error("multipart fields were not decoded"); seen.push(body); return { status: 204 }; },
  binaryReceived: async ({ body }) => { seen.push({ bytes: body.byteLength }); return { status: 204 }; },
  multiReceived: async ({ body }) => { seen.push(body.contentType === "application/json" ? body.value.eventID : body.value); return { status: 204 }; },
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
const router = createWebhookRouter({ events: async ({ body }) => { for await (const item of body) seen.push(item.eventID); return { status: 204 }; }, frames: async ({ body }) => { for await (const item of body) seen.push(item.frameID); return { status: 204 }; }, custom: async ({ body }) => { seen.push(body.eventID); return { status: 204 }; }, customStream: async ({ body }) => { for await (const item of body) seen.push(item.eventID); return { status: 204 }; }, denied: async () => ({ status: 204 }) }, { routes: { events: "/events", frames: "/frames", custom: "/custom", customStream: "/custom-stream", denied: "/denied" }, codecs, maxStreamItemBytes: 1024 });
const encoder = new TextEncoder();
const valid = new ReadableStream({ start(controller) { controller.enqueue(encoder.encode('{"event_id":"one"}\n{"ev')); controller.enqueue(encoder.encode('ent_id":"two"}\n')); controller.close(); } });
const validResponse = await router.fetch(new Request("https://host.test/events", { method: "POST", headers: { "content-type": "application/x-ndjson" }, body: valid, duplex: "half" }));
if (validResponse.status !== 204 || seen.join(",") !== "one,two") throw new Error("inbound NDJSON stream was not decoded");
const invalid = new ReadableStream({ start(controller) { controller.enqueue(encoder.encode('{"wrong":true}\n')); controller.close(); } });
const invalidResponse = await router.fetch(new Request("https://host.test/events", { method: "POST", headers: { "content-type": "application/x-ndjson" }, body: invalid, duplex: "half" }));
if (invalidResponse.status !== 400) throw new Error("invalid inbound stream item was accepted");
const bounded = createWebhookRouter({ events: async ({ body }) => { for await (const _ of body) { } return { status: 204 }; } }, { routes: { events: "/events" }, maxStreamItemBytes: 4 });
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
	for _, expected := range []string{"EventGetWebhookContext", "EventPostWebhookContext", "readonly event?: (context: EventGetWebhookContext | EventPostWebhookContext)"} {
		if !strings.Contains(webhooks, expected) {
			t.Fatalf("multi-method webhook source missing %q:\n%s", expected, webhooks)
		}
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
	if strings.Count(callbacks, "export interface OrderStatusCallbackContext") != 1 {
		t.Fatalf("component callback was emitted more than once:\n%s", callbacks)
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
	for _, expected := range []string{"decodeInboundParameters", `location: "header"`, `name: "X-Signature"`, `property: "xSignature"`} {
		if !strings.Contains(webhooks, expected) {
			t.Fatalf("webhook parameter metadata missing %q:\n%s", expected, webhooks)
		}
	}
}

const serverTSConfig = `{
  "compilerOptions": {"target":"ES2022","module":"NodeNext","moduleResolution":"NodeNext","strict":true,"skipLibCheck":true,"rootDir":".","outDir":"../output"},
  "include": ["**/*.ts"]
}`
