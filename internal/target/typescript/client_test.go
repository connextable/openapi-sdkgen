package typescript

import (
	"reflect"
	"strings"
	"testing"

	sdkgen "github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func TestOptionalInputCallsEmitOptionsOnlyOverloads(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Optional calls", "version": "1"},
  "paths": {
    "/optional": {"get": {"operationId": "optional", "parameters": [{"name": "force", "in": "query", "schema": {"type": "boolean"}}], "responses": {"204": {"description": "OK"}}}},
    "/required": {"post": {"operationId": "required", "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "string"}}}}, "responses": {"204": {"description": "OK"}}}},
    "/health": {"get": {"operationId": "health", "responses": {"204": {"description": "OK"}}}},
    "/accounts/{accountID}/phone": {"delete": {"operationId": "deletePhone", "parameters": [{"name": "accountID", "in": "path", "required": true, "schema": {"type": "string"}}, {"name": "force", "in": "query", "schema": {"type": "boolean"}}], "responses": {"204": {"description": "OK"}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	client := string(artifactByPath(t, artifacts, "generated/client.ts"))
	interfaceBody := func(name string) string {
		t.Helper()
		start := strings.Index(client, "interface "+name)
		if start < 0 {
			t.Fatalf("missing interface %s:\n%s", name, client)
		}
		end := strings.Index(client[start:], "\n}\n\n")
		if end < 0 {
			t.Fatalf("unterminated interface %s:\n%s", name, client[start:])
		}
		return client[start : start+end]
	}

	optionalName := operationTypeName("GET /optional")
	optionalCall := interfaceBody(optionalName + "Call")
	for _, expected := range []string{
		"(options?: " + optionalName + "Options)",
		"(input?: " + optionalName + "Input, options?: " + optionalName + "Options)",
		"readonly raw: " + optionalName + "RawCall",
	} {
		if !strings.Contains(optionalCall, expected) {
			t.Fatalf("optional call missing %q:\n%s", expected, optionalCall)
		}
	}
	optionalRawCall := interfaceBody(optionalName + "RawCall")
	for _, expected := range []string{
		"(options?: " + optionalName + "Options)",
		"(input?: " + optionalName + "Input, options?: " + optionalName + "Options)",
	} {
		if !strings.Contains(optionalRawCall, expected) {
			t.Fatalf("optional raw call missing %q:\n%s", expected, optionalRawCall)
		}
	}

	requiredName := operationTypeName("POST /required")
	if requiredCall := interfaceBody(requiredName + "Call"); strings.Contains(requiredCall, "(options?: "+requiredName+"Options)") {
		t.Fatalf("required call gained options-only overload:\n%s", requiredCall)
	}
	if requiredRawCall := interfaceBody(requiredName + "RawCall"); strings.Contains(requiredRawCall, "(options?: "+requiredName+"Options)") {
		t.Fatalf("required raw call gained options-only overload:\n%s", requiredRawCall)
	}

	healthName := operationTypeName("GET /health")
	healthCall := interfaceBody(healthName + "Call")
	healthRawCall := interfaceBody(healthName + "RawCall")
	if strings.Count(healthCall, "\n  (options?: "+healthName+"Options)") != 1 || strings.Count(healthRawCall, "\n  (options?: "+healthName+"Options)") != 1 {
		t.Fatalf("no-input call should retain one options-only signature:\n%s", healthCall)
	}

	deleteName := operationTypeName("DELETE /accounts/{accountID}/phone")
	deleteCall := interfaceBody(deleteName + "Call")
	if strings.Contains(deleteCall, "(options?: "+deleteName+"Options)") {
		t.Fatalf("full path call gained options-only overload:\n%s", deleteCall)
	}
	deleteResourceCall := interfaceBody(deleteName + "ResourceCall")
	deleteResourceRawCall := interfaceBody(deleteName + "ResourceRawCall")
	if !strings.Contains(deleteResourceCall, "(options?: "+deleteName+"Options)") || !strings.Contains(deleteResourceRawCall, "(options?: "+deleteName+"Options)") {
		t.Fatalf("optional resource call missing options-only overload:\n%s", deleteResourceCall)
	}
}

func TestJSDocTypeReferenceFlattensInlineObjectComments(t *testing.T) {
	value := jsDocTypeReference("{\n  /** property docs */\n  readonly id: string\n}")
	if strings.Contains(value, "/*") || value != "`{ readonly id: string }`" {
		t.Fatalf("reference = %q", value)
	}
}

func TestOperationServerURLPrefersOperationThenPathOverride(t *testing.T) {
	document := &ir.Document{Raw: map[string]any{"servers": []any{map[string]any{"url": "https://api.example.test/v1"}}}}
	operation := ir.Operation{
		Path:        "/widgets",
		Method:      "GET",
		Raw:         map[string]any{"servers": []any{map[string]any{"url": "https://operations.example.test/v2"}}},
		PathItemRaw: map[string]any{"servers": []any{map[string]any{"url": "https://paths.example.test/v3"}}},
	}
	if value := operationServers(document, operation); !strings.Contains(value, "operations.example.test/v2") || !strings.Contains(value, `#/paths/~1widgets/get/servers/0`) {
		t.Fatalf("operation servers = %q", value)
	}
	delete(operation.Raw, "servers")
	if value := operationServers(document, operation); !strings.Contains(value, "paths.example.test/v3") || !strings.Contains(value, `#/paths/~1widgets/servers/0`) {
		t.Fatalf("path servers = %q", value)
	}
	operation.PathItemRaw = map[string]any{}
	if value := operationServers(document, operation); !strings.Contains(value, "api.example.test/v1") || !strings.Contains(value, `#/servers/0`) {
		t.Fatalf("root servers = %q", value)
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
		{name: "false binary", body: map[string]any{"content": map[string]any{"application/octet-stream": map[string]any{"schema": false}}}, want: "never"},
		{name: "false binary variant", body: map[string]any{"content": map[string]any{
			"application/json":         map[string]any{"schema": map[string]any{"type": "string"}},
			"application/octet-stream": map[string]any{"schema": false},
		}}, want: `{ readonly contentType: "application/json"; readonly value: string } | { readonly contentType: "application/octet-stream"; readonly value: never }`},
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

func TestOperationResponseMediaTypesIncludesDefaultResponses(t *testing.T) {
	document := &ir.Document{}
	operation := ir.Operation{Raw: map[string]any{"responses": map[string]any{
		"default": map[string]any{"content": map[string]any{"application/json": map[string]any{}, "text/plain": map[string]any{}}},
	}}}
	mediaTypes, err := operationResponseMediaTypes(document, operation)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(mediaTypes, []string{"application/json", "text/plain"}) {
		t.Fatalf("media types = %#v", mediaTypes)
	}
}
