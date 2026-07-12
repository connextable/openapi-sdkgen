package typescript

import (
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func TestPackageArtifactsRejectsCollidingOperationSymbols(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
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
