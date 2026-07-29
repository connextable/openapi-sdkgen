package main

import (
	"strings"
	"testing"
)

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
	got, err := countOperations(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := 6; got != want {
		t.Fatalf("operation count = %d, want %d", got, want)
	}
}

func TestCountOperationsResolvesAnyLocalPathItemPointer(t *testing.T) {
	root := map[string]any{
		"paths": map[string]any{
			"/shared":     map[string]any{"additionalOperations": map[string]any{"Purge": map[string]any{}}},
			"/referenced": map[string]any{"$ref": "#/paths/~1shared"},
			"/encoded":    map[string]any{"$ref": "#%2Fpaths%2F~1shared"},
		},
		"webhooks": map[string]any{
			"event": map[string]any{"$ref": "#/paths/~1shared"},
		},
	}
	got, err := countOperations(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := 4; got != want {
		t.Fatalf("operation count = %d, want %d", got, want)
	}
}

func TestCountOperationsRejectsCyclicPathItems(t *testing.T) {
	for name, pathItems := range map[string]map[string]any{
		"self": {
			"A": map[string]any{"$ref": "#/components/pathItems/A"},
		},
		"mutual": {
			"A": map[string]any{"$ref": "#/components/pathItems/B"},
			"B": map[string]any{"$ref": "#/components/pathItems/A"},
		},
		"encoded": {
			"A": map[string]any{"$ref": "#%2Fcomponents%2FpathItems%2FA"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := map[string]any{
				"paths":      map[string]any{"/cyclic": map[string]any{"$ref": "#/components/pathItems/A"}},
				"components": map[string]any{"pathItems": pathItems},
			}
			if _, err := countOperations(root); err == nil || !strings.Contains(err.Error(), "cyclic path item reference") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCountOperationsDoesNotFailOnExternalPathItemReference(t *testing.T) {
	root := map[string]any{
		"paths": map[string]any{
			"/external": map[string]any{
				"$ref": "./path-items/external.json",
				"post": map[string]any{},
			},
		},
	}
	got, err := countOperations(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := 1; got != want {
		t.Fatalf("operation count = %d, want sibling count %d", got, want)
	}
}
