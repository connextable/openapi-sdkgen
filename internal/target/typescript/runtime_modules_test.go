package typescript

import (
	"bytes"
	pathpkg "path"
	"regexp"
	"sort"
	"strings"
	"testing"

	"openapi-sdkgen/internal/compiler"
	"openapi-sdkgen/internal/compiler/ir"
)

var runtimeImportPattern = regexp.MustCompile(`\bfrom\s+["']([^"']+)["']`)

func TestRuntimeModulesAreInvariantOwnedAndAcyclic(t *testing.T) {
	firstDocument, err := sdkgen.Compile([]byte(emitterFixture))
	if err != nil {
		t.Fatal(err)
	}
	secondDocument, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.1",
  "info": {"title": "Unrelated runtime fixture", "version": "1"},
  "paths": {
    "/health": {
      "head": {
        "responses": {"204": {"description": "healthy"}}
      }
    }
  }
}`))
	if err != nil {
		t.Fatal(err)
	}

	first := generatedRuntimeArtifactMap(t, firstDocument)
	second := generatedRuntimeArtifactMap(t, secondDocument)
	expected := []string{
		"internal/runtime/callables.ts",
		"internal/runtime/codecs.ts",
		"internal/runtime/configuration.ts",
		"internal/runtime/constants.ts",
		"internal/runtime/errors.ts",
		"internal/runtime/http.ts",
		"internal/runtime/identity.ts",
		"internal/runtime/links.ts",
		"internal/runtime/objects.ts",
		"internal/runtime/operation.ts",
		"internal/runtime/pagination.ts",
		"internal/runtime/request.ts",
		"internal/runtime/security.ts",
		"internal/runtime/transport.ts",
	}
	if paths := sortedRuntimePaths(first); strings.Join(paths, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("runtime artifact paths = %v, want %v", paths, expected)
	}
	if paths := sortedRuntimePaths(second); strings.Join(paths, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("unrelated runtime artifact paths = %v, want %v", paths, expected)
	}
	for _, artifactPath := range expected {
		if !bytes.Equal(first[artifactPath], second[artifactPath]) {
			t.Fatalf("runtime artifact %s varies across unrelated documents", artifactPath)
		}
	}

	owners := make(map[string]string)
	graph := make(map[string][]string, len(first))
	for artifactPath, source := range first {
		for symbol := range exportedSymbols(string(source)) {
			if previous, exists := owners[symbol]; exists {
				t.Fatalf("runtime export %q is owned by both %s and %s", symbol, previous, artifactPath)
			}
			owners[symbol] = artifactPath
		}
		for _, match := range runtimeImportPattern.FindAllSubmatch(source, -1) {
			specifier := string(match[1])
			if !strings.HasPrefix(specifier, "./") {
				t.Fatalf("runtime artifact %s imports non-runtime owner %q", artifactPath, specifier)
			}
			target := pathpkg.Clean(pathpkg.Join(pathpkg.Dir(artifactPath), strings.TrimSuffix(specifier, ".js")+".ts"))
			if _, exists := first[target]; !exists {
				t.Fatalf("runtime artifact %s imports unowned runtime module %q", artifactPath, target)
			}
			graph[artifactPath] = append(graph[artifactPath], target)
		}
	}
	assertAcyclicRuntimeGraph(t, graph)
}

func generatedRuntimeArtifactMap(t *testing.T, document *ir.Document) map[string][]byte {
	t.Helper()
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string][]byte)
	for _, artifact := range artifacts {
		if strings.HasPrefix(artifact.Path, "internal/runtime/") {
			result[artifact.Path] = artifact.Data
		}
		if artifact.Path == "internal/runtime.ts" || artifact.Path == "internal/constants.ts" {
			t.Fatalf("legacy runtime artifact remains: %s", artifact.Path)
		}
		if (artifact.Path == "internal/client.ts" || artifact.Path == "internal/errors.ts") && strings.Contains(string(artifact.Data), `from "./runtime.js"`) {
			t.Fatalf("legacy runtime import remains in %s", artifact.Path)
		}
	}
	return result
}

func sortedRuntimePaths(artifacts map[string][]byte) []string {
	result := make([]string, 0, len(artifacts))
	for artifactPath := range artifacts {
		result = append(result, artifactPath)
	}
	sort.Strings(result)
	return result
}

func assertAcyclicRuntimeGraph(t *testing.T, graph map[string][]string) {
	t.Helper()
	state := make(map[string]uint8, len(graph))
	var visit func(string)
	visit = func(node string) {
		switch state[node] {
		case 1:
			t.Fatalf("runtime import graph contains a cycle at %s", node)
		case 2:
			return
		}
		state[node] = 1
		for _, target := range graph[node] {
			visit(target)
		}
		state[node] = 2
	}
	for node := range graph {
		visit(node)
	}
}
