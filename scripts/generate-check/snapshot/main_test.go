package main

import "testing"

func TestCountOperationsIncludesAdditionalAndReferencedPathItems(t *testing.T) {
	root := map[string]any{
		"paths": map[string]any{
			"/direct": map[string]any{
				"get":   map[string]any{},
				"query": map[string]any{},
				"additionalOperations": map[string]any{
					"PURGE": map[string]any{},
					"COPY":  map[string]any{},
				},
			},
			"/referenced": map[string]any{"$ref": "#/components/pathItems/Shared"},
		},
		"webhooks": map[string]any{
			"event": map[string]any{"post": map[string]any{}},
		},
		"components": map[string]any{
			"pathItems": map[string]any{
				"Shared": map[string]any{"patch": map[string]any{}},
			},
		},
	}
	if got, want := countOperations(root), 6; got != want {
		t.Fatalf("operation count = %d, want %d", got, want)
	}
}
