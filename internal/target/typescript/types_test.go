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
		{name: "OpenAPI 3.0 nullable", schema: map[string]any{"type": "string", "nullable": true}, want: "string | null"},
		{name: "map", schema: map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, want: "Readonly<Record<string, number>>"},
		{name: "tuple", schema: map[string]any{"type": "array", "prefixItems": []any{map[string]any{"type": "string"}, map[string]any{"type": "integer"}}, "items": map[string]any{"type": "boolean"}}, want: "readonly [string, number, ...(boolean)[]]"},
		{name: "union", schema: map[string]any{"oneOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "integer"}, map[string]any{"type": "string"}}}, want: "string | number"},
		{name: "intersection", schema: map[string]any{"allOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "null"}}}, want: "string & null"},
		{name: "reference sibling", schema: map[string]any{"$ref": "#/components/schemas/Widget", "type": "object", "additionalProperties": map[string]any{"type": "string"}}, want: "(WidgetInput) & (Readonly<Record<string, string>>)"},
		{name: "pattern properties", schema: map[string]any{"type": "object", "properties": map[string]any{"fixed": map[string]any{"type": "string"}}, "patternProperties": map[string]any{"^x-": map[string]any{"type": "integer"}}}, want: "({\n  /**\n   * OpenAPI property `fixed`.\n   */\n  readonly fixed?: string | undefined\n}) & (Readonly<Record<string, number>>)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := schemaType(document, test.schema, projectionInput)
			if err != nil || value != test.want {
				t.Fatalf("schemaType = %q, %v; want %q", value, err, test.want)
			}
		})
	}
}

func TestSourceArtifactsEmitsClosedObjectRuntimeValidation(t *testing.T) {
	artifacts, err := SourceArtifacts(&ir.Document{ComponentSchemas: map[string]map[string]any{
		"Closed": {"type": "object", "additionalProperties": false},
	}, Operations: []ir.Operation{{
		OperationID: "getClosed", Method: "GET", Path: "/closed",
		Raw: map[string]any{
			"responses": map[string]any{
				"200": map[string]any{
					"content": map[string]any{
						"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Closed"}},
					},
				},
			},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if source := string(artifactByPath(t, artifacts, "generated/client.ts")); !strings.Contains(source, `additionalProperties: false`) {
		t.Fatalf("closed schema descriptor missing:\n%s", source)
	}
}

func TestSchemaConstraintSummaryListsValidationOnlyKeywords(t *testing.T) {
	got := schemaConstraintSummary(map[string]any{
		"multipleOf":            2,
		"maximum":               10,
		"exclusiveMaximum":      true,
		"minimum":               1,
		"exclusiveMinimum":      false,
		"maxLength":             20,
		"minLength":             2,
		"pattern":               "^[a-z]+$",
		"maxItems":              3,
		"minItems":              1,
		"uniqueItems":           true,
		"maxProperties":         4,
		"minProperties":         1,
		"unevaluatedProperties": false,
		"contentMediaType":      "application/json",
	})
	for _, want := range []string{
		"multipleOf=2", "maximum=10", "exclusiveMaximum=true", "minimum=1", "exclusiveMinimum=false",
		"maxLength=20", "minLength=2", "pattern=\"^[a-z]+$\"", "maxItems=3", "minItems=1", "uniqueItems=true",
		"maxProperties=4", "minProperties=1", "unevaluatedProperties=false", "contentMediaType=\"application/json\"",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("constraint summary = %q, missing %q", got, want)
		}
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

func TestOperationOutputTypesIncludeDefaultResponses(t *testing.T) {
	operation := ir.Operation{Raw: map[string]any{
		"responses": map[string]any{
			"default": map[string]any{
				"description": "Fallback",
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{
							"type":       "object",
							"properties": map[string]any{"id": map[string]any{"type": "string"}},
						},
					},
				},
			},
		},
	}}
	document := &ir.Document{}
	output, err := operationOutputType(document, operation)
	if err != nil || !strings.Contains(output, "readonly id?: string") {
		t.Fatalf("output = %q, %v", output, err)
	}
	raw, err := operationRawResponseType(document, operation)
	if err != nil || !strings.Contains(raw, "RawResponseFor<number") || !strings.Contains(raw, "readonly id?: string") {
		t.Fatalf("raw = %q, %v", raw, err)
	}
}

func TestOperationOutputTypesNormalizeMediaTypeCasing(t *testing.T) {
	operation := ir.Operation{Raw: map[string]any{"responses": map[string]any{
		"200": map[string]any{"description": "Stream", "content": map[string]any{
			"APPLICATION/OCTET-STREAM": map[string]any{"schema": map[string]any{"type": "string"}},
		}},
	}}}
	output, err := operationOutputType(&ir.Document{}, operation)
	if err != nil || output != "ReadableStream<Uint8Array>" {
		t.Fatalf("output = %q, %v", output, err)
	}
}

func TestOperationOutputTypeUsesUnknownForSchemaLessMediaBodies(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{{
		OperationID: "getValue",
		Raw: map[string]any{"responses": map[string]any{
			"200": map[string]any{"description": "OK", "content": map[string]any{"application/json": map[string]any{}}},
		}},
	}}}
	output, err := operationOutputType(document, document.Operations[0])
	if err != nil {
		t.Fatal(err)
	}
	if output != "unknown" {
		t.Fatalf("output = %q, want unknown", output)
	}
}

func TestSchemaTypeKeepsPrefixItemsOpenWhenItemsIsAbsent(t *testing.T) {
	value, err := schemaType(&ir.Document{}, map[string]any{
		"type":        "array",
		"prefixItems": []any{map[string]any{"type": "string"}},
	}, projectionOutput)
	if err != nil {
		t.Fatal(err)
	}
	if value != "readonly [string, ...unknown[]]" {
		t.Fatalf("type = %q", value)
	}
}

func TestSourceArtifactsGenerateRecursiveComponentSchemas(t *testing.T) {
	document := &ir.Document{ComponentSchemas: map[string]map[string]any{
		"Node": {"type": "object", "properties": map[string]any{
			"next": map[string]any{"$ref": "#/components/schemas/Node"},
		}},
	}}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	if source := string(artifactByPath(t, artifacts, "generated/types.ts")); !strings.Contains(source, "readonly next?: Node") {
		t.Fatalf("recursive schema missing from generated types:\n%s", source)
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
