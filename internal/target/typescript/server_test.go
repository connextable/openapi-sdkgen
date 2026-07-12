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
  "openapi": "3.1.1",
  "info": {"title": "Webhook", "version": "1"},
  "paths": {},
  "webhooks": {"orderCreated": {"post": {
    "operationId": "orderCreatedWebhook",
    "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["id"], "properties": {"id": {"type": "string"}}}}}},
    "responses": {"202": {"description": "Accepted", "content": {"application/json": {"schema": {"type": "object", "required": ["accepted"], "properties": {"accepted": {"type": "string"}}}}}}}
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
const router = createWebhookRouter({
  orderCreated: async ({ body, operationID, request }) => {
    seen.push({ body, operationID, method: request.method });
    return { status: 202, body: { accepted: body.id } };
  },
}, { routes: { orderCreated: "/hooks/orders" }, authenticate: async ({ method, path, security }) => {
  if (method !== "POST" || path !== "/hooks/orders" || JSON.stringify(security) !== JSON.stringify([{ signature: [] }])) throw new Error("bad auth context");
} });
const response = await router.fetch(new Request("https://host.test/hooks/orders", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ id: "order-1" }) }));
if (response.status !== 202 || JSON.stringify(await response.json()) !== JSON.stringify({ accepted: "order-1" })) throw new Error("handler response was not encoded");
if (JSON.stringify(seen) !== JSON.stringify([{ body: { id: "order-1" }, operationID: "orderCreatedWebhook", method: "POST" }])) throw new Error("handler context mismatch");
const denied = createWebhookRouter({ orderCreated: async () => ({ status: 202 }) }, { routes: { orderCreated: "/hooks/orders" }, authenticate: () => new Response("no", { status: 401 }) });
if ((await denied.fetch(new Request("https://host.test/hooks/orders", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" }))).status !== 401) throw new Error("authentication response was ignored");
if ((await router.fetch(new Request("https://host.test/hooks/orders", { method: "POST", headers: { "content-type": "text/plain" }, body: "bad" }))).status !== 415) throw new Error("bad media type was accepted");
if ((await router.fetch(new Request("https://host.test/hooks/orders", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" }))).status !== 400) throw new Error("schema-invalid body was accepted");
for (const routes of [{}, { orderCreated: "hooks/orders" }, { orderCreated: "/hooks/orders?debug=1" }]) {
  try { createWebhookRouter({ orderCreated: async () => ({ status: 202 }) }, { routes }); throw new Error("invalid route was accepted"); }
  catch (error) { if (String(error).includes("invalid route was accepted")) throw error; }
}
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
      "requestBody": {"content": {"application/json": {"schema": {"type": "object", "required": ["id"], "properties": {"id": {"type": "string"}}}}}},
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
} }, { authenticate: ({ security }) => {
  if (JSON.stringify(security) !== JSON.stringify([{ signature: [] }])) throw new Error("callback security metadata mismatch");
} });
const response = await callbacks.orderStatus.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ id: "order-1" }) }));
if (response.status !== 204) throw new Error("callback response was not encoded");
if (JSON.stringify(seen) !== JSON.stringify([{ body: { id: "order-1" }, operationID: "orderStatusCallback", method: "POST", path: "/callback" }])) throw new Error("callback context mismatch");
if ((await callbacks.orderStatus.fetch(new Request("https://host.test/callback", { method: "GET" }))).status !== 405) throw new Error("wrong callback method was accepted");
if ((await callbacks.orderStatus.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "text/plain" }, body: "bad" }))).status !== 415) throw new Error("bad callback media type was accepted");
if ((await callbacks.orderStatus.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" }))).status !== 400) throw new Error("schema-invalid callback was accepted");
if ((await codecs.createCallbackHandlers({}).orderStatus.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" }))).status !== 404) throw new Error("missing callback handler was accepted");
const denied = codecs.createCallbackHandlers({ orderStatus: async () => ({ status: 204 }) }, { authenticate: () => new Response("Unauthorized", { status: 401 }) });
if ((await denied.orderStatus.fetch(new Request("https://host.test/callback", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ id: "order-1" }) }))).status !== 401) throw new Error("callback authentication response was ignored");
`
	command := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(outputDirectory, "server", "callbacks.js"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("execute generated callback codecs: %v\n%s", err, output)
	}
}

func TestServerAddOnRejectsNonJSONInboundBodiesBeforeArtifacts(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.1",
  "info": {"title": "Webhook", "version": "1"},
  "paths": {},
  "webhooks": {"orderCreated": {"post": {
    "requestBody": {"content": {"application/xml": {"schema": {"type": "object"}}}},
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
	if _, err := (Generator{}).Generate(document, options); err == nil || !strings.Contains(err.Error(), "#/webhooks/orderCreated/post/requestBody/content") || !strings.Contains(err.Error(), "exactly one JSON media type") {
		t.Fatalf("server media diagnostic = %v", err)
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

func TestServerAddOnRejectsUndecodedInboundParameters(t *testing.T) {
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
	if _, err := (Generator{}).Generate(document, options); err == nil || !strings.Contains(err.Error(), "#/webhooks/event/post/parameters") || !strings.Contains(err.Error(), "does not yet implement inbound parameter decoding") {
		t.Fatalf("parameter diagnostic = %v", err)
	}
}

const serverTSConfig = `{
  "compilerOptions": {"target":"ES2022","module":"NodeNext","moduleResolution":"NodeNext","strict":true,"skipLibCheck":true,"rootDir":".","outDir":"../output"},
  "include": ["**/*.ts"]
}`
