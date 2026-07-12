package validate

import (
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func TestProjectAcceptsDeclaredPathParameters(t *testing.T) {
	document := projectFixture()
	if err := Project(document); err != nil {
		t.Fatal(err)
	}
}

func TestProjectRejectsMissingAndDuplicateOperationIDs(t *testing.T) {
	document := projectFixture()
	document.Operations = append(document.Operations,
		ir.Operation{Method: "GET", Path: "/missing", Raw: validExtensions()},
		ir.Operation{Method: "GET", Path: "/duplicate", OperationID: "getItem", Raw: validExtensions(), Envelope: "data", Concurrency: "none", Idempotency: "unsupported", Visibility: "public"},
	)
	err := Project(document)
	if err == nil {
		t.Fatal("invalid operation IDs accepted")
	}
	if !strings.Contains(err.Error(), "missing operationId") || !strings.Contains(err.Error(), "duplicate operationId") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectRejectsUndeclaredPathParameter(t *testing.T) {
	document := projectFixture()
	paths := document.Raw["paths"].(map[string]any)
	pathItem := paths["/items/{itemID}"].(map[string]any)
	delete(pathItem, "parameters")
	if err := Project(document); err == nil || !strings.Contains(err.Error(), "declared and required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectRequiresPathParameters(t *testing.T) {
	document := projectFixture()
	paths := document.Raw["paths"].(map[string]any)
	pathItem := paths["/items/{itemID}"].(map[string]any)
	parameters := pathItem["parameters"].([]any)
	parameters[0].(map[string]any)["required"] = false
	if err := Project(document); err == nil || !strings.Contains(err.Error(), "declared and required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectResolvesComponentPathParameters(t *testing.T) {
	document := projectFixture()
	paths := document.Raw["paths"].(map[string]any)
	pathItem := paths["/items/{itemID}"].(map[string]any)
	pathItem["parameters"] = []any{map[string]any{"$ref": "#/components/parameters/ItemID"}}
	document.Raw["components"] = map[string]any{"parameters": map[string]any{
		"ItemID": map[string]any{"name": "itemID", "in": "path", "required": true},
	}}
	if err := Project(document); err != nil {
		t.Fatal(err)
	}
	document.Raw["components"].(map[string]any)["parameters"].(map[string]any)["ItemID"].(map[string]any)["required"] = false
	if err := Project(document); err == nil || !strings.Contains(err.Error(), "declared and required") {
		t.Fatalf("error = %v", err)
	}
}

func TestProjectRejectsInvalidPaginationAndFilterProfiles(t *testing.T) {
	document := projectFixture()
	operation := &document.Operations[0]
	operation.Pagination = "page"
	operation.Raw["parameters"] = []any{map[string]any{
		"name": "createdAtContains",
		"in":   "query",
		"x-filter": map[string]any{
			"field":    "createdAt",
			"operator": "contains",
		},
	}}
	err := Project(document)
	if err == nil || !strings.Contains(err.Error(), `invalid x-pagination "page"`) || !strings.Contains(err.Error(), `invalid x-filter operator "contains"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectKeepsSDKVisibilityIndependentFromSecurity(t *testing.T) {
	document := projectFixture()
	document.Operations[0].Raw["security"] = []any{map[string]any{"customer-session": []any{}}}
	if err := Project(document); err != nil {
		t.Fatal(err)
	}
	delete(document.Operations[0].Raw, "security")
	if err := Project(document); err == nil || !strings.Contains(err.Error(), "declare security explicitly") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func projectFixture() *ir.Document {
	extensions := validExtensions()
	return &ir.Document{
		Title:           "Example",
		ContractVersion: "0.1.0",
		OpenAPIVersion:  "3.2.0",
		Servers:         []ir.Server{{URL: "/v1"}},
		Operations: []ir.Operation{{
			OperationID:        "getItem",
			Method:             "GET",
			Path:               "/items/{itemID}",
			PathParameterOrder: []string{"itemID"},
			Envelope:           "data",
			Concurrency:        "none",
			Idempotency:        "unsupported",
			Visibility:         "public",
			Raw:                extensions,
		}},
		Raw: map[string]any{
			"paths": map[string]any{
				"/items/{itemID}": map[string]any{
					"parameters": []any{map[string]any{
						"name":     "itemID",
						"in":       "path",
						"required": true,
					}},
				},
			},
		},
	}
}

func validExtensions() map[string]any {
	return map[string]any{
		"security":         []any{},
		"x-envelope":       "data",
		"x-concurrency":    "none",
		"x-idempotency":    "unsupported",
		"x-sdk-visibility": "public",
	}
}
