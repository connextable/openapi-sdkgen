package ir

import (
	"encoding/json"
	"os"
	"reflect"
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

func operationFixture(operationID string) map[string]any {
	return map[string]any{
		"operationId":      operationID,
		"x-envelope":       "data",
		"x-concurrency":    "none",
		"x-idempotency":    "unsupported",
		"x-sdk-visibility": "public",
	}
}
