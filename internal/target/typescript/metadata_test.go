package typescript

import (
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func TestEmitMetadataPreservesDocumentationExamplesAndExtensions(t *testing.T) {
	document := &ir.Document{
		OpenAPIVersion:     "3.2.0",
		OpenAPIVersionLine: "3.2",
		Raw: map[string]any{
			"openapi": "3.2.0",
			"info": map[string]any{
				"title":   "Metadata",
				"summary": "Document summary",
			},
			"servers":      []any{map[string]any{"url": "https://api.example.test", "name": "production"}},
			"tags":         []any{map[string]any{"name": "widgets", "summary": "Widgets", "parent": "api", "kind": "navigation"}},
			"externalDocs": map[string]any{"url": "https://example.test/docs"},
			"x-root":       map[string]any{"preserved": true},
			"paths": map[string]any{
				"/widgets": map[string]any{
					"x-path": map[string]any{"preserved": true},
					"get": map[string]any{
						"tags":         []any{"widgets"},
						"summary":      "List widgets",
						"description":  "Lists every widget.",
						"externalDocs": map[string]any{"url": "https://example.test/widgets"},
						"deprecated":   true,
						"x-operation":  map[string]any{"preserved": true},
						"parameters":   []any{map[string]any{"name": "id", "in": "query", "description": "Widget filter", "deprecated": true, "x-parameter": map[string]any{"preserved": true}}},
						"responses": map[string]any{"200": map[string]any{
							"summary":     "Widget response",
							"description": "Widget response",
							"content": map[string]any{"application/json": map[string]any{
								"examples": map[string]any{"widget": map[string]any{"value": map[string]any{"id": "1"}}},
							}},
						}},
					},
				},
			},
			"components": map[string]any{
				"examples": map[string]any{"widget": map[string]any{"value": map[string]any{"id": "1"}, "dataValue": map[string]any{"id": "1"}, "serializedValue": "{\"id\":\"1\"}"}},
				"schemas":  map[string]any{"Widget": map[string]any{"type": "object", "x-schema": map[string]any{"preserved": true}}},
			},
		},
	}
	source, err := emitMetadata(document, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"summary":"Document summary"`,
		`"name":"production"`,
		`"parent":"api"`,
		`"kind":"navigation"`,
		`"externalDocs":{"url":"https://example.test/docs"}`,
		`"x-root":{"preserved":true}`,
		`"x-path":{"preserved":true}`,
		`"x-operation":{"preserved":true}`,
		`"x-parameter":{"preserved":true}`,
		`"x-schema":{"preserved":true}`,
		`"summary":"List widgets"`,
		`"description":"Lists every widget."`,
		`"url":"https://example.test/widgets"`,
		`"description":"Widget filter"`,
		`"description":"Widget response"`,
		`"summary":"Widget response"`,
		`"dataValue":{"id":"1"}`,
		`"serializedValue":"{\"id\":\"1\"}"`,
		`"examples":{"widget":{"value":{"id":"1"}}}`,
		`export const openapiVersionLine = "3.2"`,
	} {
		if !strings.Contains(string(source), expected) {
			t.Fatalf("metadata missing %q:\n%s", expected, source)
		}
	}
}
