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
	if !reflect.DeepEqual(types, []string{
		`ServerError<"invalid_widget", Readonly<Record<string, unknown>>>`,
		`ServerError<"missing_widget", Readonly<Record<string, unknown>>>`,
	}) {
		t.Fatalf("operation error types = %#v", types)
	}
}

func TestErrorContractsAggregateCodeDetailsAndNarrowOperations(t *testing.T) {
	document := &ir.Document{ComponentSchemas: map[string]map[string]any{
		"AlphaError": errorEnvelopeSchema("shared-code", "AlphaDetails", ""),
		"BetaError":  errorEnvelopeSchema("shared-code", "BetaDetails", "request-errors"),
		"OtherError": errorEnvelopeSchema("other-code", "OtherDetails", "request-errors"),
		"AlphaDetails": {
			"type": "object", "properties": map[string]any{"alpha": map[string]any{"type": "string"}},
		},
		"BetaDetails": {
			"type": "object", "properties": map[string]any{"beta": map[string]any{"type": "string"}},
		},
		"OtherDetails": {
			"type": "object", "properties": map[string]any{"other": map[string]any{"type": "string"}},
		},
	}}
	contracts, bySchema, err := errorContracts(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 2 {
		t.Fatalf("contracts = %#v", contracts)
	}
	shared := contracts[1]
	if shared.Code != "shared-code" || shared.Category != "request-errors" || !reflect.DeepEqual(shared.Details, []string{
		`Contract.ComponentOutput<"AlphaDetails">`,
		`Contract.ComponentOutput<"BetaDetails">`,
	}) {
		t.Fatalf("shared contract = %#v", shared)
	}
	operation := operationWithErrorSchema("AlphaError")
	types, err := operationErrorTypes(document, operation, bySchema)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(types, []string{`ServerError<"shared-code", Contract.ComponentOutput<"AlphaDetails">>`}) {
		t.Fatalf("operation error types = %#v", types)
	}
	expression, err := operationErrorTypeExpression(document, operation, bySchema)
	if err != nil {
		t.Fatal(err)
	}
	if got := expression.render(typeRenderContract); got != `Errors.ServerError<"shared-code", Contract.ComponentOutput<"AlphaDetails">> | TransportError` {
		t.Fatalf("operation error expression = %q", got)
	}
}

func TestErrorContractsRejectConflictingNonEmptyCategories(t *testing.T) {
	document := &ir.Document{ComponentSchemas: map[string]map[string]any{
		"AlphaError": errorEnvelopeSchema("shared", "AlphaDetails", "alpha"),
		"BetaError":  errorEnvelopeSchema("shared", "BetaDetails", "beta"),
		"AlphaDetails": {
			"type": "string",
		},
		"BetaDetails": {
			"type": "string",
		},
	}}
	_, _, err := errorContracts(document)
	if err == nil || !containsAll(err.Error(), `"alpha"`, `"beta"`, "AlphaError", "BetaError") {
		t.Fatalf("error = %v", err)
	}
}

func TestErrorContractsExcludeHiddenOnlySchemasAndRestoreVisibleReachability(t *testing.T) {
	schema := errorEnvelopeSchema("hidden-code", "HiddenDetails", "")
	document := &ir.Document{
		ComponentSchemas: map[string]map[string]any{
			"HiddenError":   schema,
			"HiddenDetails": {"type": "string"},
		},
		Operations: []ir.Operation{{
			Visibility: "hidden",
			Raw:        operationWithErrorSchema("HiddenError").Raw,
		}},
	}
	contracts, _, err := errorContracts(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 0 {
		t.Fatalf("hidden contracts = %#v", contracts)
	}
	document.Operations = append(document.Operations, ir.Operation{
		Visibility: "public",
		Raw:        operationWithErrorSchema("HiddenError").Raw,
	})
	contracts, _, err = errorContracts(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 1 || contracts[0].Code != "hidden-code" {
		t.Fatalf("visible contracts = %#v", contracts)
	}
}

func errorEnvelopeSchema(code, details, category string) map[string]any {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"error": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"code":    map[string]any{"const": code},
					"details": map[string]any{"$ref": "#/components/schemas/" + details},
				},
			},
		},
	}
	if category != "" {
		schema["x-error-category"] = category
	}
	return schema
}

func operationWithErrorSchema(schema string) ir.Operation {
	return ir.Operation{Raw: map[string]any{"responses": map[string]any{
		"400": map[string]any{"content": map[string]any{
			"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + schema}},
		}},
	}}}
}

func TestErrorContractsResolveEscapedSchemaReferences(t *testing.T) {
	document := &ir.Document{ComponentSchemas: map[string]map[string]any{
		"Base/Error": {
			"type": "object",
			"properties": map[string]any{
				"error": map[string]any{"properties": map[string]any{
					"code": map[string]any{"const": "invalid_widget"},
				}},
			},
		},
	}}
	contracts, bySchema, err := errorContracts(document)
	if err != nil {
		t.Fatal(err)
	}
	operation := ir.Operation{Raw: map[string]any{"responses": map[string]any{
		"400": map[string]any{"content": map[string]any{
			"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Base~1Error"}},
		}},
	}}}
	types, err := operationErrorTypes(document, operation, bySchema)
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 1 || !reflect.DeepEqual(types, []string{`ServerError<"invalid_widget", unknown>`}) {
		t.Fatalf("contracts = %#v, types = %#v", contracts, types)
	}
}
