package typescript

import (
	"strings"
	"testing"

	sdkgen "openapi-sdkgen/internal/compiler"
)

func TestCallableRegistryOwnsSingleBindingAndCapabilityAssembly(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.2.0",
  "info": {"title": "Callable registry", "version": "1"},
  "paths": {
    "/events": {"get": {
      "operationId": "listEvents",
      "responses": {"200": {"description": "OK", "content": {"application/x-ndjson": {"itemSchema": {"type": "string"}}}}}
    }},
    "/source": {"get": {
      "operationId": "getSource",
      "responses": {"200": {"description": "OK", "links": {"follow": {"operationId": "postTarget"}}}}
    }},
    "/target": {"post": {"operationId": "postTarget", "responses": {"204": {"description": "OK"}}}},
    "/hidden": {"get": {"operationId": "getHidden", "x-sdk-visibility": "hidden", "responses": {"204": {"description": "OK"}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	registry := string(artifactByPath(t, artifacts, "internal/client/registry.ts"))
	client := string(artifactByPath(t, artifacts, "internal/client/factory.ts"))

	if got := strings.Count(registry, "bindBase as "); got != 3 {
		t.Fatalf("base factory imports = %d, want one per non-hidden route:\n%s", got, registry)
	}
	if got := strings.Count(registry, "bindStream as "); got != 1 {
		t.Fatalf("stream factory imports = %d, want one:\n%s", got, registry)
	}
	if got := strings.Count(registry, "bindLinks as "); got != 1 {
		t.Fatalf("link factory imports = %d, want one source factory:\n%s", got, registry)
	}
	for _, expected := range []string{
		`const completed = {} as { [Route in keyof Routes]: Routes[Route]["call"] }`,
		`Object.assign(completed,`,
		`["links", __sdkgen_`,
		`["stream", __sdkgen_`,
		`routes: completed`,
	} {
		if !strings.Contains(registry, expected) {
			t.Fatalf("callable registry missing %q:\n%s", expected, registry)
		}
	}
	if strings.Contains(registry, "GET /hidden") || strings.Contains(registry, "getHidden") {
		t.Fatalf("hidden operation entered callable registry:\n%s", registry)
	}
	if strings.Contains(client, "bindOperation<") || strings.Contains(client, "createPaginator<") || strings.Contains(client, `route: "GET /events"`) {
		t.Fatalf("client composition retained an inline operation binding or definition:\n%s", client)
	}
	if !strings.Contains(client, "const registry = createCallableRegistry(") || !strings.Contains(client, `$routes: registry.routes`) || !strings.Contains(client, `$operations: registry.operations`) {
		t.Fatalf("client does not consume the completed callable registry:\n%s", client)
	}
}
