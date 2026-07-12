package typescript

import (
	"bytes"
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func TestSourceArtifactsRejectsCollidingOperationSymbols(t *testing.T) {
	document := &ir.Document{ContractVersion: "1.0.0", Operations: []ir.Operation{
		{OperationID: "get-pet", Method: "GET", Path: "/pets"},
		{OperationID: "get_pet", Method: "GET", Path: "/pets"},
	}}
	_, err := SourceArtifacts(document)
	if err == nil || !strings.Contains(err.Error(), "both generate TypeScript") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateComponentSymbolsRejectsNormalizedAndGeneratedNameCollisions(t *testing.T) {
	for name, schema := range map[string]map[string]map[string]any{
		"normalized": {
			"foo-bar": map[string]any{"type": "string"},
			"foo_bar": map[string]any{"type": "string"},
		},
		"projection": {
			"Product":      map[string]any{"type": "object", "properties": map[string]any{}},
			"ProductInput": map[string]any{"type": "object", "properties": map[string]any{}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			document := &ir.Document{ComponentSchemas: schema}
			if err := validateComponentSymbols(document, []string{"Product", "ProductInput", "foo-bar", "foo_bar"}); err == nil || !strings.Contains(err.Error(), "generates TypeScript symbol") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestObjectTypeRejectsCollidingPropertyAliases(t *testing.T) {
	_, err := objectType(&ir.Document{}, map[string]any{
		"properties": map[string]any{
			"foo-bar": map[string]any{"type": "string"},
			"foo_bar": map[string]any{"type": "string"},
		},
	}, projectionInput)
	if err == nil || !strings.Contains(err.Error(), "both generate TypeScript property") {
		t.Fatalf("error = %v", err)
	}
}

func TestOperationParametersRejectCollidingPropertyAliases(t *testing.T) {
	_, err := operationParameters(&ir.Document{}, ir.Operation{Raw: map[string]any{
		"parameters": []any{
			map[string]any{"name": "x-id", "in": "query", "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "x_id", "in": "query", "schema": map[string]any{"type": "string"}},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "both generate TypeScript property") {
		t.Fatalf("error = %v", err)
	}
}

func TestOperationParametersFollowURIPathParameterOrder(t *testing.T) {
	parameters, err := operationParameters(&ir.Document{}, ir.Operation{
		PathParameterOrder: []string{"customerID", "widgetID"},
		Raw: map[string]any{"parameters": []any{
			map[string]any{"name": "widgetID", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "customerID", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(parameters) != 2 || parameters[0].Name != "customerID" || parameters[1].Name != "widgetID" {
		t.Fatalf("path parameters = %#v", parameters)
	}
}

func TestRequestBodyTypeUsesRuntimeBinaryBody(t *testing.T) {
	value, err := requestBodyType(&ir.Document{}, map[string]any{"content": map[string]any{
		"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}},
	}})
	if err != nil || value != "BinaryBody" {
		t.Fatalf("request body type = %q, %v", value, err)
	}
}

func TestEmitTypesRejectsCollidingEnumValueAliases(t *testing.T) {
	_, err := emitTypes(&ir.Document{
		ComponentSchemas: map[string]map[string]any{
			"Status": {"type": "string", "enum": []any{"foo-bar", "foo_bar"}},
		},
		Operations: []ir.Operation{{Raw: map[string]any{
			"responses": map[string]any{
				"200": map[string]any{
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/Status"},
						},
					},
				},
			},
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "both generate TypeScript key") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildResourceTreeRejectsOperationAndChildNameCollision(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
		{OperationID: "listUsers", Method: "GET", Path: "/users"},
		{OperationID: "getList", Method: "GET", Path: "/users/list"},
	}}
	_, err := buildResourceTree(document, Manifest{Operations: []ManifestOperation{
		{OperationID: "listUsers", Method: "GET", Path: "/users", Visibility: "public"},
		{OperationID: "getList", Method: "GET", Path: "/users/list", Visibility: "public"},
	}})
	if err == nil || !strings.Contains(err.Error(), "resource member collision") {
		t.Fatalf("error = %v", err)
	}
}

func TestEmitQueryTypesKeepsOrdinaryLimitAndSortParameters(t *testing.T) {
	operation := ir.Operation{
		OperationID: "searchWidgets",
		Method:      "GET",
		Path:        "/widgets",
		Raw: map[string]any{"parameters": []any{
			map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer"}},
			map[string]any{"name": "sort", "in": "query", "schema": map[string]any{"type": "string"}},
		}},
	}
	parameters, err := operationParameters(&ir.Document{}, operation)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := emitQueryTypes(&output, &ir.Document{}, operation, "SearchWidgets", parameters); err != nil {
		t.Fatal(err)
	}
	for _, property := range []string{"readonly limit?: number | undefined", "readonly sort?: string | undefined"} {
		if !strings.Contains(output.String(), property) {
			t.Fatalf("query input omitted %q:\n%s", property, output.String())
		}
	}
}

func TestValidateSourceExportSymbolsRejectsCrossModuleCollision(t *testing.T) {
	err := validateSourceExportSymbols(map[string][]byte{
		"types":  []byte("export type ListWidgetsInput = {}\n"),
		"client": []byte("export type ListWidgetsInput = {}\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "generated source export") {
		t.Fatalf("error = %v", err)
	}
}

func TestSourceArtifactsRejectsComponentAndOperationExportCollision(t *testing.T) {
	document := &ir.Document{
		ContractVersion: "1.0.0",
		ComponentSchemas: map[string]map[string]any{
			"APIError": {"type": "string"},
		},
		Operations: []ir.Operation{{
			OperationID: "listWidgets",
			Method:      "GET",
			Path:        "/widgets",
			Raw: map[string]any{
				"responses": map[string]any{
					"200": map[string]any{
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/APIError"},
							},
						},
					},
				},
			},
		}},
	}
	_, err := SourceArtifacts(document)
	if err == nil || !strings.Contains(err.Error(), "generated source export") {
		t.Fatalf("error = %v", err)
	}
}

func TestSourceArtifactsDoesNotRequireNPMNameOrSemVer(t *testing.T) {
	artifacts, err := SourceArtifacts(&ir.Document{ContractVersion: "release-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 6 {
		t.Fatalf("source artifact count = %d, want 6", len(artifacts))
	}
}
