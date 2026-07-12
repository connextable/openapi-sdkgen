package typescript

import (
	"bytes"
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/generator"
)

func TestPackageArtifactsRejectsCollidingOperationSymbols(t *testing.T) {
	document := &ir.Document{ContractVersion: "1.0.0", Operations: []ir.Operation{
		{OperationID: "get-pet", Method: "GET", Path: "/pets"},
		{OperationID: "get_pet", Method: "GET", Path: "/pets"},
	}}
	_, err := PackageArtifacts(document, Package{Name: "@example/api"})
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

func TestValidatePackageExportSymbolsRejectsCrossModuleCollision(t *testing.T) {
	err := validatePackageExportSymbols(map[string][]byte{
		"types":  []byte("export type ListWidgetsInput = {}\n"),
		"client": []byte("export type ListWidgetsInput = {}\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "generated package export") {
		t.Fatalf("error = %v", err)
	}
}

func TestPackageArtifactsRejectsComponentAndOperationExportCollision(t *testing.T) {
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
	_, err := PackageArtifacts(document, Package{Name: "@example/api"})
	if err == nil || !strings.Contains(err.Error(), "generated package export") {
		t.Fatalf("error = %v", err)
	}
}

func TestGeneratorRejectsInvalidNPMNameAndVersion(t *testing.T) {
	for _, test := range []struct {
		name        string
		packageName string
		version     string
	}{
		{name: "name", packageName: "@Example/client", version: "1.2.3"},
		{name: "version", packageName: "@example/client", version: "release-1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := (Generator{}).Generate(&ir.Document{ContractVersion: test.version}, generator.Options{PackageName: test.packageName})
			if err == nil {
				t.Fatal("Generate succeeded")
			}
		})
	}
}
