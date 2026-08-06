package typescript

import (
	"strings"
	"testing"

	sdkgen "openapi-sdkgen/internal/compiler"
)

func TestOperationArtifactsOwnOneNonHiddenRouteEach(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Operation modules", "version": "1"},
  "paths": {
    "/users/{userId}": {"parameters": [{"name": "userId", "in": "path", "required": true, "schema": {"type": "string"}}], "get": {
      "operationId": "getUser",
      "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/User"}}}}}
    }},
    "/users": {"post": {
      "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/User"}}}},
      "responses": {"204": {"description": "Created"}}
    }},
    "/internal": {"get": {"operationId": "getInternal", "x-sdk-visibility": "internal", "responses": {"204": {"description": "OK"}}}},
    "/hidden": {"delete": {"operationId": "deleteHidden", "x-sdk-visibility": "hidden", "responses": {"204": {"description": "OK"}}}}
  },
  "components": {"schemas": {"User": {"type": "object", "required": ["id"], "properties": {"id": {"type": "string"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	byPath := make(map[string]string, len(artifacts))
	for _, artifact := range artifacts {
		byPath[artifact.Path] = string(artifact.Data)
	}

	expected := map[string]string{
		"internal/operations/users/by-user-id/get.ts": `export type RouteKey = "GET /users/{userId}"`,
		"internal/operations/users/post.ts":           `export type RouteKey = "POST /users"`,
		"internal/operations/internal/get.ts":         `export type RouteKey = "GET /internal"`,
	}
	for path, route := range expected {
		source, exists := byPath[path]
		if !exists {
			t.Fatalf("missing operation leaf %s", path)
		}
		for _, fragment := range []string{route, "export interface RequestInputs", "export interface Contract", "export function bindBase"} {
			if !strings.Contains(source, fragment) {
				t.Fatalf("operation leaf %s missing %q:\n%s", path, fragment, source)
			}
		}
		for otherPath, otherRoute := range expected {
			if otherPath != path && strings.Contains(source, otherRoute) {
				t.Fatalf("operation leaf %s contains foreign route %q", path, otherRoute)
			}
		}
	}
	if _, exists := byPath["internal/operations/hidden/delete.ts"]; exists {
		t.Fatal("hidden operation emitted a semantic leaf")
	}
	if source := byPath["internal/operations/internal/get.ts"]; !strings.Contains(source, "export type ResourceCall = never") {
		t.Fatalf("internal operation exposed a resource call:\n%s", source)
	}
	if source := byPath["internal/operations/users/post.ts"]; !strings.Contains(source, "requestBodies:") || strings.Contains(source, "GET /users/{userId}") {
		t.Fatalf("unidentified operation leaf lost its own definition or contains a foreign one:\n%s", source)
	}
	for _, path := range []string{"internal/operations/users/by-user-id/get.ts", "internal/operations/users/post.ts"} {
		source := byPath[path]
		if strings.Contains(source, "schemas/index.js") || !strings.Contains(source, "schemas/user.js") {
			t.Fatalf("operation leaf %s does not import its schema owner directly:\n%s", path, source)
		}
	}
}

func TestOperationLeafEmitsLocalCapabilityFactories(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Operation capabilities", "version": "1"},
  "paths": {
    "/pages": {"get": {
      "operationId": "listPages",
      "x-pagination": {"mode": "cursor", "request": {"cursor": "cursor"}, "response": {"items": "/items", "nextCursor": "/nextCursor"}},
      "parameters": [{"name": "cursor", "in": "query", "schema": {"type": "string"}}],
      "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": {"type": "object", "required": ["items"], "properties": {"items": {"type": "array", "items": {"type": "string"}}, "nextCursor": {"type": "string"}}}}}}}
    }}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	source := string(artifactByPath(t, artifacts, "internal/operations/pages/get.ts"))
	for _, expected := range []string{"export type Pagination =", "export function bindPagination", "createPaginator<"} {
		if !strings.Contains(source, expected) {
			t.Fatalf("pagination leaf missing %q:\n%s", expected, source)
		}
	}
}
