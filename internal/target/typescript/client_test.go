package typescript

import (
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func TestJSDocTypeReferenceFlattensInlineObjectComments(t *testing.T) {
	value := jsDocTypeReference("{\n  /** property docs */\n  readonly id: string\n}")
	if strings.Contains(value, "/*") || value != "`{ readonly id: string }`" {
		t.Fatalf("reference = %q", value)
	}
}

func TestOperationServerURLPrefersOperationThenPathOverride(t *testing.T) {
	document := &ir.Document{Servers: []ir.Server{{URL: "https://api.example.test/v1"}}}
	operation := ir.Operation{
		Raw:         map[string]any{"servers": []any{map[string]any{"url": "https://operations.example.test/v2"}}},
		PathItemRaw: map[string]any{"servers": []any{map[string]any{"url": "https://paths.example.test/v3"}}},
	}
	if value := operationServerURL(document, operation); value != "https://operations.example.test/v2" {
		t.Fatalf("operation server URL = %q", value)
	}
	delete(operation.Raw, "servers")
	if value := operationServerURL(document, operation); value != "https://paths.example.test/v3" {
		t.Fatalf("path server URL = %q", value)
	}
	operation.PathItemRaw = map[string]any{"servers": []any{map[string]any{"url": "https://api.example.test/v1"}}}
	if value := operationServerURL(document, operation); value != "" {
		t.Fatalf("default server override = %q", value)
	}
}

func TestRequestBodyTypeRepresentsEmptyTextJSONAndMultiMediaBodies(t *testing.T) {
	for _, test := range []struct {
		name string
		body map[string]any
		want string
	}{
		{name: "empty", body: map[string]any{}, want: "unknown"},
		{name: "text", body: map[string]any{"content": map[string]any{"text/plain": map[string]any{"schema": map[string]any{"type": "object"}}}}, want: "string"},
		{name: "json", body: map[string]any{"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "integer"}}}}, want: "number"},
		{name: "multi", body: map[string]any{"content": map[string]any{
			"application/json":         map[string]any{"schema": map[string]any{"type": "string"}},
			"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}},
			"text/plain":               map[string]any{"schema": map[string]any{"type": "string"}},
		}}, want: `{ readonly contentType: "application/json"; readonly value: string } | { readonly contentType: "application/octet-stream"; readonly value: BinaryBody } | { readonly contentType: "text/plain"; readonly value: string }`},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := requestBodyType(&ir.Document{}, test.body)
			if err != nil || value != test.want {
				t.Fatalf("requestBodyType = %q, %v; want %q", value, err, test.want)
			}
		})
	}
}
