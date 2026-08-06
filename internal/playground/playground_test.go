package playground

import (
	"strings"
	"testing"
)

func TestGenerateEmitsTypeScriptFiles(t *testing.T) {
	result, err := Generate([]byte(`{
		"openapi":"3.1.0",
		"info":{"title":"Todo API","version":"1.0.0"},
		"paths":{"/todos":{"get":{"operationId":"listTodos","responses":{"200":{"description":"OK"}}}}}
	}`), "typescript")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Diagnostics != "" {
		t.Fatalf("Generate() diagnostics = %q", result.Diagnostics)
	}
	if len(result.Artifacts) == 0 {
		t.Fatal("Generate() emitted no artifacts")
	}
	if result.Artifacts[0].Path != "internal/client.ts" {
		t.Fatalf("first artifact path = %q", result.Artifacts[0].Path)
	}
	if !strings.Contains(result.Artifacts[0].Content, "listTodos") {
		t.Fatal("generated client does not contain listTodos")
	}
}

func TestGenerateReturnsAuthorDiagnostics(t *testing.T) {
	result, err := Generate([]byte(`{"openapi":"3.1.0","info":{"title":"Broken","version":"1"},"paths":[]}`), "typescript")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Diagnostics == "" {
		t.Fatal("Generate() returned no diagnostics")
	}
	if len(result.Artifacts) != 0 {
		t.Fatalf("Generate() emitted %d artifacts after an author error", len(result.Artifacts))
	}
}

func TestGenerateRejectsUnknownTarget(t *testing.T) {
	if _, err := Generate([]byte(`{}`), "swift"); err == nil || !strings.Contains(err.Error(), "unsupported SDK target") {
		t.Fatalf("Generate() error = %v", err)
	}
}
