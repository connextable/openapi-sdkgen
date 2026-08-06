package typescript

import (
	"strings"
	"testing"

	sdkgen "openapi-sdkgen/internal/compiler"
)

func TestRouteArtifactsAreThinInlineTypeRegistries(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Route modules", "version": "1"},
  "paths": {
    "/public": {"get": {"operationId": "getPublic", "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Public"}}}}}}},
    "/unnamed": {"post": {"responses": {"204": {"description": "OK"}}}},
    "/internal": {"get": {"operationId": "getInternal", "x-sdk-visibility": "internal", "responses": {"204": {"description": "OK"}}}},
    "/hidden": {"get": {"operationId": "getHidden", "x-sdk-visibility": "hidden", "responses": {"204": {"description": "OK"}}}}
  },
  "components": {"schemas": {
    "Public": {"type": "object", "properties": {"value": {"type": "string"}}},
    "HiddenOnly": {"type": "string"}
  }}
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	index := string(artifactByPath(t, artifacts, "internal/routes/index.ts"))
	inputs := string(artifactByPath(t, artifacts, "internal/routes/inputs.ts"))
	helpers := string(artifactByPath(t, artifacts, "internal/routes/helpers.ts"))

	entries := map[string]string{
		`readonly "GET /public":`:   `import("../operations/public/get.js").Contract`,
		`readonly "POST /unnamed":`: `import("../operations/unnamed/post.js").Contract`,
		`readonly "GET /internal":`: `import("../operations/internal/get.js").Contract`,
	}
	for entry, reference := range entries {
		if strings.Count(index, entry) != 1 || !strings.Contains(index, entry+" "+reference) {
			t.Fatalf("route registry entry %q is not one inline leaf reference:\n%s", entry, index)
		}
	}
	for _, forbidden := range []string{`GET /hidden`, `getHidden`, "import type ", "readonly input:", "readonly output:", "interface Contract"} {
		if strings.Contains(index, forbidden) {
			t.Fatalf("route registry contains %q:\n%s", forbidden, index)
		}
	}
	if !strings.Contains(index, `readonly "getPublic": Routes["GET /public"]`) || !strings.Contains(index, `readonly "getInternal": Routes["GET /internal"]`) {
		t.Fatalf("operation-ID registry membership changed:\n%s", index)
	}
	for entry, reference := range map[string]string{
		`readonly "GET /public":`:   `import("../operations/public/get.js").RequestInputs`,
		`readonly "POST /unnamed":`: `import("../operations/unnamed/post.js").RequestInputs`,
		`readonly "GET /internal":`: `import("../operations/internal/get.js").RequestInputs`,
	} {
		if strings.Count(inputs, entry) != 1 || !strings.Contains(inputs, entry+" "+reference) {
			t.Fatalf("request-input registry entry %q is not one inline leaf reference:\n%s", entry, inputs)
		}
	}
	for _, expected := range []string{
		`from "../runtime/identity.js"`,
		`from "./index.js"`,
		`from "./inputs.js"`,
		"export type RouteInput<",
		"export type OperationInput<",
		"export type RouteParameter<",
	} {
		if !strings.Contains(helpers, expected) {
			t.Fatalf("route helpers missing %q:\n%s", expected, helpers)
		}
	}
}
