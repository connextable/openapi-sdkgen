package typescript

import (
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func TestSchemaTypeMapsCompositeOpenAPISchemas(t *testing.T) {
	document := &ir.Document{ComponentSchemas: map[string]map[string]any{
		"Widget": {"type": "object", "properties": map[string]any{}},
	}}
	for _, test := range []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{name: "nullable", schema: map[string]any{"type": []any{"string", "null"}}, want: "string | null"},
		{name: "map", schema: map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, want: "Readonly<Record<string, number>>"},
		{name: "closed map", schema: map[string]any{"type": "object", "additionalProperties": false}, want: "Readonly<Record<string, never>>"},
		{name: "tuple", schema: map[string]any{"type": "array", "prefixItems": []any{map[string]any{"type": "string"}, map[string]any{"type": "integer"}}, "items": map[string]any{"type": "boolean"}}, want: "readonly [string, number, ...(boolean)[]]"},
		{name: "union", schema: map[string]any{"oneOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "integer"}, map[string]any{"type": "string"}}}, want: "string | number"},
		{name: "intersection", schema: map[string]any{"allOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "null"}}}, want: "string & null"},
		{name: "reference sibling", schema: map[string]any{"$ref": "#/components/schemas/Widget", "type": "object", "additionalProperties": map[string]any{"type": "string"}}, want: "(WidgetInput) & (Readonly<Record<string, string>>)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := schemaType(document, test.schema, projectionInput)
			if err != nil || value != test.want {
				t.Fatalf("schemaType = %q, %v; want %q", value, err, test.want)
			}
		})
	}
}

func TestSchemaTypeProjectsReadAndWriteOnlyProperties(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{"visible", "read", "write"},
		"properties": map[string]any{
			"visible": map[string]any{"type": "string"},
			"read":    map[string]any{"type": "string", "readOnly": true},
			"write":   map[string]any{"type": "string", "writeOnly": true},
		},
	}
	input, err := schemaType(&ir.Document{}, schema, projectionInput)
	if err != nil || !containsAll(input, "readonly visible: string", "readonly write: string") || strings.Contains(input, "readonly read: string") {
		t.Fatalf("input = %q, %v", input, err)
	}
	output, err := schemaType(&ir.Document{}, schema, projectionOutput)
	if err != nil || output == input || !containsAll(output, "readonly visible: string", "readonly read: string") || containsAll(output, "readonly write: string") {
		t.Fatalf("output = %q, %v", output, err)
	}
}

func TestEnvelopeDataSchemaFollowsReferencesAndStopsCycles(t *testing.T) {
	document := &ir.Document{ComponentSchemas: map[string]map[string]any{
		"Envelope": {"properties": map[string]any{"data": map[string]any{"$ref": "#/components/schemas/Result"}}},
		"Result":   {"properties": map[string]any{"data": map[string]any{"type": "string"}}},
		"Cycle":    {"$ref": "#/components/schemas/Cycle"},
	}}
	data := envelopeDataSchema(document, map[string]any{"$ref": "#/components/schemas/Envelope"}, map[string]bool{})
	if reference, _ := data["$ref"].(string); reference != "#/components/schemas/Result" {
		t.Fatalf("envelope data = %#v", data)
	}
	if value := envelopeDataSchema(document, map[string]any{"$ref": "#/components/schemas/Cycle"}, map[string]bool{}); value != nil {
		t.Fatalf("cyclic envelope data = %#v", value)
	}
}

func containsAll(value string, expected ...string) bool {
	for _, item := range expected {
		if !strings.Contains(value, item) {
			return false
		}
	}
	return true
}
