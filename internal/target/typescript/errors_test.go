package typescript

import (
	"reflect"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func TestErrorContractsPropagateAndDeduplicateComposedErrorSchemas(t *testing.T) {
	document := &ir.Document{ComponentSchemas: map[string]map[string]any{
		"BaseError": {
			"type": "object",
			"properties": map[string]any{
				"error": map[string]any{"properties": map[string]any{
					"code":    map[string]any{"enum": []any{"invalid_widget", "missing_widget"}},
					"details": map[string]any{"type": "object", "properties": map[string]any{}},
				}},
			},
		},
		"CombinedError": {
			"allOf": []any{
				map[string]any{"$ref": "#/components/schemas/BaseError"},
				map[string]any{"$ref": "#/components/schemas/BaseError"},
			},
		},
	}}
	contracts, bySchema, err := errorContracts(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 2 || len(bySchema["CombinedError"]) != 2 {
		t.Fatalf("contracts = %#v, combined = %#v", contracts, bySchema["CombinedError"])
	}
	operation := ir.Operation{Raw: map[string]any{"responses": map[string]any{
		"400": map[string]any{"content": map[string]any{
			"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/CombinedError"}},
		}},
	}}}
	types, err := operationErrorTypes(document, operation, bySchema)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(types, []string{"InvalidWidgetError", "MissingWidgetError"}) {
		t.Fatalf("operation error types = %#v", types)
	}
}
