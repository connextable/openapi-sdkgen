package ir

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	openapidoc "github.com/connextable/openapi-sdkgen/internal/compiler/openapi"
)

func TestBuildExtractsOperationsDeterministically(t *testing.T) {
	document := &openapidoc.Document{Raw: map[string]any{
		"openapi": "3.2.0",
		"info": map[string]any{
			"title":   "Example",
			"version": "0.1.0",
		},
		"servers": []any{map[string]any{"url": "/v1"}},
		"paths": map[string]any{
			"/items/{itemId}": map[string]any{
				"get":   operationFixture("getItem"),
				"query": operationFixture("queryItem"),
				"additionalOperations": map[string]any{
					"PURGE": operationFixture("purgeItem"),
				},
			},
			"/alpha": map[string]any{
				"get": operationFixture("getAlpha"),
			},
		},
		"components": map[string]any{
			"schemas": map[string]any{
				"Item": map[string]any{"type": "object"},
			},
		},
	}}

	model, err := Build(document)
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := make([]string, 0, len(model.Operations))
	for _, operation := range model.Operations {
		gotIDs = append(gotIDs, operation.OperationID)
	}
	wantIDs := []string{"getAlpha", "getItem", "purgeItem", "queryItem"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("operation order = %v, want %v", gotIDs, wantIDs)
	}
	if got := model.Operations[1].PathParameterOrder; !reflect.DeepEqual(got, []string{"itemId"}) {
		t.Fatalf("path parameters = %v", got)
	}
	if _, ok := model.ComponentSchemas["Item"]; !ok {
		t.Fatal("component schema not preserved")
	}
}

func TestBuildPreservesOpenAPI32SurfaceAndExtractsNewMethods(t *testing.T) {
	data, err := os.ReadFile("../openapi/testdata/oas32-conformance.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := openapidoc.Read(data)
	if err != nil {
		t.Fatal(err)
	}
	model, err := Build(source)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(model.Raw, source.Raw) {
		t.Fatal("IR root raw document differs from parsed source")
	}
	if got, want := model.OpenAPIVersion, "3.2.0"; got != want {
		t.Fatalf("OpenAPI version = %q, want %q", got, want)
	}
	if got, want := model.OpenAPIVersionLine, "3.2"; got != want {
		t.Fatalf("OpenAPI version line = %q, want %q", got, want)
	}
	if got, want := model.Servers, []Server{{URL: "https://{region}.example.test/v1", Description: "Regional API"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("servers = %#v, want %#v", got, want)
	}

	methods := make(map[string]Operation, len(model.Operations))
	for _, operation := range model.Operations {
		methods[operation.Method] = operation
	}
	for method, operationID := range map[string]string{
		"GET":      "getItem",
		"QUERY":    "queryItem",
		"PURGE":    "purgeItem",
		"m-SEARCH": "searchItemByCustomMethod",
	} {
		operation, ok := methods[method]
		if !ok {
			t.Errorf("%s operation was not extracted", method)
			continue
		}
		if operation.OperationID != operationID {
			t.Errorf("%s operation ID = %q, want %q", method, operation.OperationID, operationID)
		}
	}

	get := methods["GET"]
	pathFromRoot := source.Raw["paths"].(map[string]any)["/items/{itemId}"].(map[string]any)
	if !reflect.DeepEqual(get.PathItemRaw, pathFromRoot) {
		t.Fatal("path-level summary, description, parameters, servers, or extensions were not preserved")
	}
	operationFromRoot := pathFromRoot["get"].(map[string]any)
	if !reflect.DeepEqual(get.Raw, operationFromRoot) {
		t.Fatal("operation callbacks, security, externalDocs, or extensions were not preserved")
	}

	for _, field := range []string{"webhooks", "components", "security", "servers", "tags", "externalDocs", "x-root-extension"} {
		if !reflect.DeepEqual(model.Raw[field], source.Raw[field]) {
			t.Errorf("root field %q was not preserved", field)
		}
	}
	item := model.ComponentSchemas["Item"]
	if _, ok := item["exampleVocabularyKeyword"]; !ok {
		t.Error("unknown JSON Schema vocabulary keyword was not preserved")
	}
	rootExtension := model.Raw["x-root-extension"].(map[string]any)
	nested := rootExtension["nested"].(map[string]any)
	if got := nested["count"]; got != json.Number("9007199254740993") {
		t.Fatalf("extension number = %#v", got)
	}
}

func TestBuildPreservesSupportedVersionProvenance(t *testing.T) {
	for version, wantLine := range map[string]string{
		"3.0.3": "3.0",
		"3.1.1": "3.1",
		"3.2.0": "3.2",
	} {
		t.Run(version, func(t *testing.T) {
			schema := `{"type":"object"}`
			if wantLine != "3.0" {
				schema = `{"$id":"https://example.test/schemas/dynamic","$dynamicRef":"#node"}`
			}
			source, err := openapidoc.Read([]byte(`{
  "openapi": "` + version + `",
  "info": {"title": "Versioned", "version": "1"},
  "paths": {},
  "components": {"schemas": {"Dynamic": ` + schema + `}}
}`))
			if err != nil {
				t.Fatal(err)
			}
			model, err := Build(source)
			if err != nil {
				t.Fatal(err)
			}
			if model.OpenAPIVersion != version || model.OpenAPIVersionLine != wantLine {
				t.Fatalf("version provenance = %q (%q), want %q (%q)", model.OpenAPIVersion, model.OpenAPIVersionLine, version, wantLine)
			}
			if wantLine != "3.0" && model.ComponentSchemas["Dynamic"]["$dynamicRef"] != "#node" {
				got := model.ComponentSchemas["Dynamic"]["$dynamicRef"]
				t.Fatalf("schema reference keyword = %#v", got)
			}
		})
	}
}

func TestBuildAcceptsWebhookOnlyOpenAPI32Document(t *testing.T) {
	document := &openapidoc.Document{Raw: map[string]any{
		"openapi": "3.2.0",
		"info": map[string]any{
			"title":   "Webhook-only API",
			"version": "1.0.0",
		},
		"webhooks": map[string]any{
			"event": map[string]any{
				"post": operationFixture("eventWebhook"),
			},
		},
	}}

	model, err := Build(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Operations) != 0 {
		t.Fatalf("path operations = %d, want 0", len(model.Operations))
	}
	if !reflect.DeepEqual(model.Raw["webhooks"], document.Raw["webhooks"]) {
		t.Fatal("webhooks were not preserved")
	}
}

func TestBuildRejectsNonObjectPaths(t *testing.T) {
	document := &openapidoc.Document{Raw: map[string]any{
		"openapi": "3.2.0",
		"info":    map[string]any{"title": "Invalid", "version": "1.0.0"},
		"paths":   []any{},
	}}
	if _, err := Build(document); err == nil {
		t.Fatal("non-object paths accepted")
	}
}

func TestBuildSkipsPathExtensionsAndRejectsOpenAPI32MethodsInEarlierLines(t *testing.T) {
	for _, version := range []string{"3.0.3", "3.1.1"} {
		t.Run(version, func(t *testing.T) {
			document := &openapidoc.Document{Version: openapidoc.VersionLine(version[:3]), Raw: map[string]any{
				"openapi": version,
				"info":    map[string]any{"title": "Versioned", "version": "1"},
				"paths": map[string]any{
					"x-routing": "metadata only",
					"/widgets":  map[string]any{"query": operationFixture("queryWidgets")},
				},
			}}
			if _, err := Build(document); err == nil || !strings.Contains(err.Error(), "/paths/~1widgets/query") {
				t.Fatalf("Build error = %v", err)
			}
		})
	}
	document := &openapidoc.Document{Version: openapidoc.Version32, Raw: map[string]any{
		"openapi": "3.2.0",
		"info":    map[string]any{"title": "Extensions", "version": "1"},
		"paths": map[string]any{
			"x-routing": "metadata only",
			"/widgets":  map[string]any{"query": operationFixture("queryWidgets")},
		},
	}}
	model, err := Build(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Operations) != 1 || model.Operations[0].OperationID != "queryWidgets" {
		t.Fatalf("operations = %#v", model.Operations)
	}
}

func TestBuildResolvesLocalPathItemReferences(t *testing.T) {
	operation := operationFixture("getSharedItem")
	document := &openapidoc.Document{Raw: map[string]any{
		"openapi": "3.2.0",
		"info":    map[string]any{"title": "Referenced", "version": "1.0.0"},
		"paths": map[string]any{
			"/items/{itemID}": map[string]any{"$ref": "#/components/pathItems/Item", "summary": "Override"},
		},
		"components": map[string]any{
			"pathItems": map[string]any{
				"Item": map[string]any{"get": operation, "summary": "Shared"},
			},
		},
	}}
	model, err := Build(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Operations) != 1 || model.Operations[0].OperationID != "getSharedItem" {
		t.Fatalf("operations = %#v", model.Operations)
	}
	if model.Operations[0].PathItemRaw["summary"] != "Override" {
		t.Fatalf("path item siblings not applied: %#v", model.Operations[0].PathItemRaw)
	}
}

func TestBuildResolvesEscapedLocalPathItemReferences(t *testing.T) {
	document := &openapidoc.Document{Raw: map[string]any{
		"openapi": "3.2.0",
		"info":    map[string]any{"title": "Referenced", "version": "1.0.0"},
		"paths": map[string]any{
			"/items": map[string]any{"$ref": "#/components/pathItems/Items~1Shared"},
		},
		"components": map[string]any{
			"pathItems": map[string]any{
				"Items/Shared": map[string]any{"get": operationFixture("getSharedItems")},
			},
		},
	}}
	model, err := Build(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Operations) != 1 || model.Operations[0].OperationID != "getSharedItems" {
		t.Fatalf("operations = %#v", model.Operations)
	}
}

func TestBuildResolvesLocalPathsPathItemReferences(t *testing.T) {
	document := &openapidoc.Document{Raw: map[string]any{
		"openapi": "3.2.0",
		"info":    map[string]any{"title": "Referenced", "version": "1.0.0"},
		"paths": map[string]any{
			"/common": map[string]any{"get": operationFixture("getCommon")},
			"/alias":  map[string]any{"$ref": "#/paths/~1common"},
		},
	}}
	model, err := Build(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Operations) != 2 || model.Operations[1].OperationID != "getCommon" {
		t.Fatalf("operations = %#v", model.Operations)
	}
}

func TestBuildRegistersLosslessSchemaResources(t *testing.T) {
	document := &openapidoc.Document{Raw: map[string]any{
		"openapi":           "3.2.0",
		"$self":             "https://api.example.test/openapi.json",
		"jsonSchemaDialect": "https://example.test/dialect/root",
		"info":              map[string]any{"title": "Schemas", "version": "1"},
		"paths":             map[string]any{},
		"components": map[string]any{"schemas": map[string]any{
			"Never": false,
			"Thing": map[string]any{"$id": "schemas/thing", "$schema": "https://example.test/dialect/thing", "type": "object"},
		}},
	}}
	model, err := Build(document)
	if err != nil {
		t.Fatal(err)
	}
	never, ok := model.Schemas["Never"]
	if !ok || never.Value != false || never.Pointer != "/components/schemas/Never" || never.Dialect != "https://example.test/dialect/root" {
		t.Fatalf("Never schema = %#v", never)
	}
	thing := model.Schemas["Thing"]
	if thing.ResourceURI != "https://api.example.test/schemas/thing" || thing.Dialect != "https://example.test/dialect/thing" {
		t.Fatalf("Thing schema = %#v", thing)
	}
}

func operationFixture(operationID string) map[string]any {
	return map[string]any{
		"operationId":      operationID,
		"x-envelope":       "data",
		"x-concurrency":    "none",
		"x-idempotency":    "unsupported",
		"x-sdk-visibility": "public",
	}
}
