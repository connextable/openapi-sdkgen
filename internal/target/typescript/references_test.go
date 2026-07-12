package typescript

import (
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func TestResolveComponentObjectResolvesChainsAndAppliesSiblingOverrides(t *testing.T) {
	document := &ir.Document{Raw: map[string]any{"components": map[string]any{
		"requestBodies": map[string]any{
			"Base":  map[string]any{"description": "base", "content": map[string]any{}},
			"Alias": map[string]any{"$ref": "#/components/requestBodies/Base"},
		},
	}}}
	resolved, err := resolveComponentObject(document, map[string]any{
		"$ref":        "#/components/requestBodies/Alias",
		"description": "operation override",
	}, "requestBodies")
	if err != nil {
		t.Fatal(err)
	}
	if resolved["description"] != "operation override" {
		t.Fatalf("resolved description = %#v", resolved["description"])
	}
	if _, ok := resolved["content"].(map[string]any); !ok {
		t.Fatalf("resolved content = %#v", resolved["content"])
	}
}

func TestResolveComponentObjectRejectsExternalUnresolvedAndCyclicReferences(t *testing.T) {
	for _, test := range []struct {
		name      string
		document  *ir.Document
		reference string
		want      string
	}{
		{
			name:      "external",
			document:  &ir.Document{},
			reference: "https://example.test/request.json",
			want:      "external requestBodies reference",
		},
		{
			name: "unresolved",
			document: &ir.Document{Raw: map[string]any{"components": map[string]any{
				"requestBodies": map[string]any{},
			}}},
			reference: "#/components/requestBodies/Missing",
			want:      "unresolved requestBodies reference",
		},
		{
			name: "cycle",
			document: &ir.Document{Raw: map[string]any{"components": map[string]any{
				"requestBodies": map[string]any{
					"A": map[string]any{"$ref": "#/components/requestBodies/B"},
					"B": map[string]any{"$ref": "#/components/requestBodies/A"},
				},
			}}},
			reference: "#/components/requestBodies/A",
			want:      "cyclic requestBodies reference",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolveComponentObject(test.document, map[string]any{"$ref": test.reference}, "requestBodies")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestOperationWireBodiesResolveReusableComponents(t *testing.T) {
	document := &ir.Document{Raw: map[string]any{"components": map[string]any{
		"requestBodies": map[string]any{
			"Widget": map[string]any{"content": map[string]any{
				"application/json": map[string]any{"schema": map[string]any{"properties": map[string]any{"wire_name": map[string]any{"type": "string"}}}},
			}},
		},
		"responses": map[string]any{
			"Widget": map[string]any{"content": map[string]any{
				"application/json": map[string]any{"schema": map[string]any{"properties": map[string]any{"wire_name": map[string]any{"type": "string"}}}},
			}},
		},
	}}}
	operation := ir.Operation{Raw: map[string]any{
		"requestBody": map[string]any{"$ref": "#/components/requestBodies/Widget"},
		"responses":   map[string]any{"200": map[string]any{"$ref": "#/components/responses/Widget"}},
	}}
	requestBodies, hasRequestBodies, err := operationRequestWireBodies(document, operation)
	if err != nil || !hasRequestBodies || !strings.Contains(requestBodies, `contentType: "application/json"`) || !strings.Contains(requestBodies, `"wire_name"`) {
		t.Fatalf("request bodies = %q, %t, %v", requestBodies, hasRequestBodies, err)
	}
	responseBodies, hasResponseBodies, err := operationResponseWireBodies(document, operation)
	if err != nil || !hasResponseBodies || !strings.Contains(responseBodies, `status: "200"`) || !strings.Contains(responseBodies, `"wire_name"`) {
		t.Fatalf("response bodies = %q, %t, %v", responseBodies, hasResponseBodies, err)
	}
}
