package typescript

import (
	"strings"
	"testing"

	sdkgen "github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func TestSourceArtifactsRejectsUnimplementedSchemaFeaturesWithPaths(t *testing.T) {
	document := &ir.Document{ComponentSchemas: map[string]map[string]any{
		"Conditional": {
			"if":   map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "a"}}},
			"then": map[string]any{"required": []any{"value"}},
		},
		"Dynamic": {"$dynamicRef": "#node"},
		"XML":     {"type": "object", "xml": map[string]any{"name": "xml"}},
	}}
	_, err := SourceArtifacts(document)
	if err == nil {
		t.Fatal("unimplemented schemas accepted")
	}
	for _, expected := range []string{
		"#/components/schemas/Dynamic/$dynamicRef (dynamic reference resolution)",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("error = %q, missing %q", err, expected)
		}
	}
}

func TestSourceArtifactsAcceptsBooleanSchemasWithoutErasingTheirAssertions(t *testing.T) {
	document := &ir.Document{Raw: map[string]any{
		"components": map[string]any{
			"schemas": map[string]any{"Never": false},
		},
	}}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts} {
		if _, err := generate(document); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSourceArtifactsEmitsBooleanSchemaFromAnOpenAPI31Document(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.1",
  "info": {"title": "Boolean schema", "version": "1"},
	"paths": {"/value": {
		"get": {"operationId": "getValue", "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": false}}}}},
		"post": {"operationId": "createValue", "requestBody": {"content": {"application/json": {"schema": true}}}, "responses": {"204": {"description": "No content"}}}
	}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts} {
		artifacts, err := generate(document)
		if err != nil {
			t.Fatal(err)
		}
		source := string(artifactByPath(t, artifacts, "generated/client.ts"))
		if !strings.Contains(source, "boolean: false") {
			t.Fatalf("boolean schema descriptor missing:\n%s", source)
		}
		for _, expected := range []string{"bindOperation<never, unknown", "export type CreateValueBodyInput = unknown"} {
			if !strings.Contains(source, expected) {
				t.Fatalf("boolean schema type missing %q:\n%s", expected, source)
			}
		}
	}
}

func TestSourceArtifactsDoesNotInterpretExampleValuesAsSchemas(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.1","info":{"title":"Example values","version":"1"},"paths":{"/value":{"get":{"operationId":"getValue","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"type":"object"},"examples":{"literal":{"value":{"schema":{"$dynamicRef":"literal"},"$ref":"plain-data"}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts} {
		if _, err := generate(document); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSourceArtifactsDecodesEscapedComponentSchemaReferences(t *testing.T) {
	document := &ir.Document{
		ComponentSchemas: map[string]map[string]any{
			"a/b":    {"type": "object", "properties": map[string]any{"wire_name": map[string]any{"type": "string"}}},
			"Holder": {"$ref": "#/components/schemas/a~1b"},
		},
		Operations: []ir.Operation{{
			OperationID: "getHolder",
			Path:        "/holder",
			Method:      "GET",
			Raw: map[string]any{
				"responses": map[string]any{
					"200": map[string]any{
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/Holder"},
							},
						},
					},
				},
			},
		}},
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	if source := string(artifactByPath(t, artifacts, "generated/client.ts")); !strings.Contains(source, `reference: "a/b"`) {
		t.Fatalf("escaped component reference was not decoded:\n%s", source)
	}
}

func TestSourceArtifactsAcceptsNestedComponentSchemaReferencesAfterCompilation(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0", "info":{"title":"Nested","version":"1"},
  "paths":{"/holder":{"get":{"operationId":"getHolder","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Holder"}}}}}}}},
  "components":{"schemas":{"Thing":{"type":"object","properties":{"id":{"type":"string"}}},"Holder":{"type":"object","properties":{"id":{"$ref":"#/components/schemas/Thing/properties/id"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SourceArtifacts(document); err != nil {
		t.Fatal(err)
	}
}

func TestSourceArtifactsLowersAnchoredSchemaReferences(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0", "info":{"title":"Anchors","version":"1"},
  "paths":{"/node":{"get":{"operationId":"getNode","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Node"}}}}}}}},
  "components":{"schemas":{"Node":{"$anchor":"node","type":"object","properties":{"next":{"$ref":"#node"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	source := string(artifactByPath(t, artifacts, "generated/client.ts"))
	if !strings.Contains(source, `reference: "Node"`) {
		t.Fatalf("anchor reference was not emitted as a component reference:\n%s", source)
	}
}

func TestSourceArtifactsLowersLocalDefinitionsBeforeTypeScriptEmission(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0", "info":{"title":"Definitions","version":"1"},
  "paths":{"/holder":{"get":{"operationId":"getHolder","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Holder"}}}}}}}},
  "components":{"schemas":{"Holder":{"type":"object","$defs":{"Identifier":{"type":"string","minLength":3}},"required":["id"],"properties":{"id":{"$ref":"#/components/schemas/Holder/$defs/Identifier"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	source := string(artifactByPath(t, artifacts, "generated/client.ts"))
	if !strings.Contains(source, "minLength: 3") {
		t.Fatalf("local definition validation was not lowered:\n%s", source)
	}
}

func TestSourceArtifactsRejectsNestedSchemaPointers(t *testing.T) {
	_, err := SourceArtifacts(&ir.Document{ComponentSchemas: map[string]map[string]any{
		"Holder": {"$ref": "#/components/schemas/Thing/properties/id"},
	}})
	if err == nil || !strings.Contains(err.Error(), "#/components/schemas/Holder/$ref") || !strings.Contains(err.Error(), "must target one component schema") {
		t.Fatalf("error = %v", err)
	}
}

func TestSourceArtifactsEmitsSchemaReferenceSiblingsWithWireSemantics(t *testing.T) {
	document := &ir.Document{ComponentSchemas: map[string]map[string]any{
		"Widget": {"type": "string"},
	}, Operations: []ir.Operation{{
		OperationID: "createWidget", Path: "/widgets", Method: "POST", Raw: map[string]any{"requestBody": map[string]any{"content": map[string]any{
			"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Widget", "minLength": 3}},
		}}},
	}}}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts} {
		artifacts, err := generate(document)
		if err != nil {
			t.Fatal(err)
		}
		if source := string(artifactByPath(t, artifacts, "generated/client.ts")); !strings.Contains(source, "allOf:") || !strings.Contains(source, "minLength: 3") {
			t.Fatalf("reference sibling descriptor missing:\n%s", source)
		}
	}
}

func TestSourceArtifactsAllowsSchemaReferenceVendorExtensions(t *testing.T) {
	document := &ir.Document{ComponentSchemas: map[string]map[string]any{
		"Widget": {"type": "string"},
	}, Operations: []ir.Operation{{
		OperationID: "getWidget", Path: "/widgets", Method: "GET", Raw: map[string]any{"responses": map[string]any{"200": map[string]any{"content": map[string]any{
			"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Widget", "x-codegen-name": "widget"}},
		}}}},
	}}}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts} {
		if _, err := generate(document); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSourceArtifactsEmitsPatternPropertyWireSemantics(t *testing.T) {
	document := &ir.Document{ComponentSchemas: map[string]map[string]any{
		"Product": {
			"type":     "object",
			"nullable": true,
			"properties": map[string]any{
				"labels": map[string]any{
					"type": "object",
					"patternProperties": map[string]any{
						"^x-": map[string]any{"type": "string"},
					},
				},
			},
		},
	}}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	if source := string(artifactByPath(t, artifacts, "generated/types.ts")); !strings.Contains(source, "labels") {
		t.Fatalf("pattern property type missing:\n%s", source)
	}
}

func TestSourceArtifactsEmitsVariantWireBranchSelection(t *testing.T) {
	artifacts, err := SourceArtifacts(&ir.Document{ComponentSchemas: map[string]map[string]any{
		"Variant": {"oneOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "integer"}}},
	}, Operations: []ir.Operation{{
		OperationID: "getVariant", Method: "GET", Path: "/variant",
		Raw: map[string]any{
			"responses": map[string]any{
				"200": map[string]any{
					"content": map[string]any{
						"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Variant"}},
					},
				},
			},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if source := string(artifactByPath(t, artifacts, "generated/client.ts")); !strings.Contains(source, "oneOf:") || !strings.Contains(source, `types: ["string"]`) {
		t.Fatalf("variant wire descriptor missing:\n%s", source)
	}
}

func TestSourceArtifactsEmitsNegatedSchemaAssertion(t *testing.T) {
	artifacts, err := SourceArtifacts(&ir.Document{ComponentSchemas: map[string]map[string]any{
		"Allowed": {"not": map[string]any{"const": "forbidden"}},
	}, Operations: []ir.Operation{{
		OperationID: "getAllowed", Method: "GET", Path: "/allowed",
		Raw: map[string]any{
			"responses": map[string]any{
				"200": map[string]any{
					"content": map[string]any{
						"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Allowed"}},
					},
				},
			},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if source := string(artifactByPath(t, artifacts, "generated/client.ts")); !strings.Contains(source, `not: { constValue: "forbidden" }`) {
		t.Fatalf("negated schema descriptor missing:\n%s", source)
	}
}

func TestSourceArtifactsAcceptsConditionalSchemaAssertions(t *testing.T) {
	_, err := SourceArtifacts(&ir.Document{ComponentSchemas: map[string]map[string]any{
		"Conditional": {
			"if":   map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "a"}}},
			"then": map[string]any{"required": []any{"value"}},
			"else": map[string]any{"required": []any{"other"}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSourceArtifactsAcceptsPropertyNameAssertion(t *testing.T) {
	_, err := SourceArtifacts(&ir.Document{ComponentSchemas: map[string]map[string]any{
		"Dictionary": {"type": "object", "propertyNames": map[string]any{"pattern": "^[a-z]+$"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSourceArtifactsAcceptsDependentRequiredAssertion(t *testing.T) {
	_, err := SourceArtifacts(&ir.Document{ComponentSchemas: map[string]map[string]any{
		"Contact": {"type": "object", "dependentRequired": map[string]any{"creditCard": []any{"billingAddress"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSourceArtifactsAcceptsContainsAssertion(t *testing.T) {
	_, err := SourceArtifacts(&ir.Document{ComponentSchemas: map[string]map[string]any{
		"Tags": {"type": "array", "contains": map[string]any{"const": "primary"}, "minContains": 1, "maxContains": 2},
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSourceArtifactsAcceptsDependentSchemaAssertion(t *testing.T) {
	_, err := SourceArtifacts(&ir.Document{ComponentSchemas: map[string]map[string]any{
		"Contact": {"type": "object", "dependentSchemas": map[string]any{"creditCard": map[string]any{"required": []any{"billingAddress"}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSourceArtifactsPreservesJSONSchemaCommentsAsMetadata(t *testing.T) {
	document := &ir.Document{ComponentSchemas: map[string]map[string]any{
		"Commented": {"type": "string", "$comment": "validation note"},
	}, Raw: map[string]any{"components": map[string]any{"schemas": map[string]any{"Commented": map[string]any{"type": "string", "$comment": "validation note"}}}}}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	if source := string(artifactByPath(t, artifacts, "metadata.ts")); !strings.Contains(source, `"$comment":"validation note"`) {
		t.Fatalf("JSON Schema comment missing from metadata:\n%s", source)
	}
}

func TestSourceArtifactsRejectsUnsupportedSchemasInReusableComponents(t *testing.T) {
	document := &ir.Document{Raw: map[string]any{
		"components": map[string]any{
			"requestBodies": map[string]any{
				"Input": map[string]any{
					"content": map[string]any{
						"application/json": map[string]any{"schema": map[string]any{"$dynamicRef": "#input"}},
					},
				},
			},
			"responses": map[string]any{
				"Output": map[string]any{
					"content": map[string]any{
						"application/json": map[string]any{"schema": map[string]any{"xml": map[string]any{"name": "output"}}},
					},
				},
			},
		},
	}}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts} {
		if _, err := generate(document); err == nil || !strings.Contains(err.Error(), "$dynamicRef") {
			t.Fatalf("error = %v", err)
		}
	}
}
