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
		"#/components/schemas/Conditional/if (conditional schemas)",
		"#/components/schemas/Conditional/then (conditional schemas)",
		"#/components/schemas/Dynamic/$dynamicRef (dynamic reference resolution)",
		"#/components/schemas/XML/xml (XML serialization)",
		"#/components/schemas/Conditional/if (conditional schemas)",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("error = %q, missing %q", err, expected)
		}
	}
}

func TestSourceArtifactsRejectsBooleanSchemasWithoutErasingTheirAssertions(t *testing.T) {
	document := &ir.Document{Raw: map[string]any{
		"components": map[string]any{
			"schemas": map[string]any{"Never": false},
		},
	}}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		_, err := generate(document)
		if err == nil || !strings.Contains(err.Error(), "#/components/schemas/Never (boolean schemas)") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestSourceArtifactsRejectsBooleanSchemaFromAnOpenAPI31Document(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.1",
  "info": {"title": "Boolean schema", "version": "1"},
  "paths": {"/value": {"get": {"operationId": "getValue", "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": false}}}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		_, err := generate(document)
		if err == nil || !strings.Contains(err.Error(), "#/paths/~1value/get/responses/200/content/application~1json/schema (boolean schemas)") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestSourceArtifactsDoesNotInterpretExampleValuesAsSchemas(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.1","info":{"title":"Example values","version":"1"},"paths":{"/value":{"get":{"operationId":"getValue","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"type":"object"},"examples":{"literal":{"value":{"schema":{"$dynamicRef":"literal"},"$ref":"plain-data"}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
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

func TestSourceArtifactsRejectsNestedSchemaPointers(t *testing.T) {
	_, err := SourceArtifacts(&ir.Document{ComponentSchemas: map[string]map[string]any{
		"Holder": {"$ref": "#/components/schemas/Thing/properties/id"},
	}})
	if err == nil || !strings.Contains(err.Error(), "#/components/schemas/Holder/$ref") || !strings.Contains(err.Error(), "must target one component schema") {
		t.Fatalf("error = %v", err)
	}
}

func TestSourceArtifactsRejectsSchemaReferenceSiblingsWithWireSemantics(t *testing.T) {
	document := &ir.Document{ComponentSchemas: map[string]map[string]any{
		"Widget": {"type": "string"},
	}, Operations: []ir.Operation{{
		Path: "/widgets", Method: "POST", Raw: map[string]any{"requestBody": map[string]any{"content": map[string]any{
			"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Widget", "minLength": 3}},
		}}},
	}}}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		_, err := generate(document)
		if err == nil || !strings.Contains(err.Error(), "#/paths/~1widgets/post/requestBody/content/application~1json/schema/minLength ($ref sibling schema semantics)") {
			t.Fatalf("error = %v", err)
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
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		if _, err := generate(document); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSourceArtifactsRejectsSchemaVocabularyWithoutWireSemantics(t *testing.T) {
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
	if _, err := SourceArtifacts(document); err == nil || !strings.Contains(err.Error(), "patternProperties") {
		t.Fatalf("error = %v", err)
	}
}

func TestSourceArtifactsRejectsVariantSchemasWithoutWireBranchSelection(t *testing.T) {
	_, err := SourceArtifacts(&ir.Document{ComponentSchemas: map[string]map[string]any{
		"Variant": {"oneOf": []any{map[string]any{"type": "object"}, map[string]any{"type": "object"}}},
	}})
	if err == nil || !strings.Contains(err.Error(), "#/components/schemas/Variant/oneOf (one-of wire branch selection)") {
		t.Fatalf("error = %v", err)
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
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		if _, err := generate(document); err == nil || !strings.Contains(err.Error(), "$dynamicRef") || !strings.Contains(err.Error(), "/xml") {
			t.Fatalf("error = %v", err)
		}
	}
}
