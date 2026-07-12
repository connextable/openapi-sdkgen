package typescript

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	sdkgen "github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths(t *testing.T) {
	document := &ir.Document{
		Raw: map[string]any{
			"$self":             "https://api.example.test/openapi.json",
			"jsonSchemaDialect": "https://spec.example.test/dialect",
			"webhooks":          map[string]any{"received": map[string]any{}},
			"components": map[string]any{
				"securitySchemes": map[string]any{"oauth": map[string]any{"type": "oauth2"}},
			},
		},
		Operations: []ir.Operation{{
			Path:   "/events",
			Method: "GET",
			Raw: map[string]any{
				"parameters": []any{map[string]any{"name": "raw", "in": "querystring"}},
				"callbacks":  map[string]any{"onDone": map[string]any{}},
				"responses": map[string]any{
					"200": map[string]any{
						"links":   map[string]any{"next": map[string]any{}},
						"content": map[string]any{"text/event-stream": map[string]any{}},
					},
				},
			},
		}},
	}
	_, err := SourceArtifacts(document)
	if err == nil {
		t.Fatal("unimplemented OpenAPI features accepted")
	}
	for _, expected := range []string{
		"#/$self (base URI resolution)",
		"#/jsonSchemaDialect (dialect selection)",
		"#/webhooks (generated inbound webhook contracts)",
		"#/components/securitySchemes/oauth/type (typed security providers)",
		"#/paths/~1events/get/callbacks (generated callback contracts)",
		"#/paths/~1events/get/parameters/0/in (whole-querystring serialization)",
		"#/paths/~1events/get/responses/200/links (generated link helpers)",
		"#/paths/~1events/get/responses/200/content/text~1event-stream (streaming response API)",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("error = %q, missing %q", err, expected)
		}
	}
}

func TestSourceArtifactsAllowsImplementedOpenAPIHTTPFeatures(t *testing.T) {
	document := &ir.Document{
		Raw: map[string]any{"servers": []any{map[string]any{"url": "https://api.example.test"}}},
		Operations: []ir.Operation{{
			OperationID: "createUpload",
			Path:        "/uploads/{uploadID}",
			Method:      "POST",
			PathItemRaw: map[string]any{
				"parameters": []any{map[string]any{"name": "uploadID", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}},
			},
			Raw: map[string]any{
				"requestBody": map[string]any{"content": map[string]any{"multipart/form-data": map[string]any{"schema": map[string]any{"type": "object"}}}},
				"responses":   map[string]any{"201": map[string]any{"description": "Created", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object"}}}}},
			},
		}},
	}
	if _, err := SourceArtifacts(document); err != nil {
		t.Fatal(err)
	}
}

func TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures(t *testing.T) {
	document := &ir.Document{Raw: map[string]any{"components": map[string]any{
		"headers":    map[string]any{"RateLimit": map[string]any{"required": true, "schema": map[string]any{"type": "integer"}}},
		"parameters": map[string]any{"Search": map[string]any{"name": "search", "in": "query", "allowReserved": true, "allowEmptyValue": true}},
		"responses":  map[string]any{"Events": map[string]any{"content": map[string]any{"text/event-stream": map[string]any{}}}},
	}}}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		if _, err := generate(document); err == nil || !strings.Contains(err.Error(), "allowReserved") || !strings.Contains(err.Error(), "allowEmptyValue") || !strings.Contains(err.Error(), "event-stream") || !strings.Contains(err.Error(), "components/headers") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestSourceArtifactsRejectsOpenAPI32SecurityFieldsAtExactPaths(t *testing.T) {
	document := &ir.Document{Raw: map[string]any{"components": map[string]any{"securitySchemes": map[string]any{
		"oauth": map[string]any{
			"type": "oauth2", "oauth2MetadataUrl": "https://auth.example.test/metadata", "deprecated": true,
			"flows": map[string]any{"deviceAuthorization": map[string]any{"deviceAuthorizationUrl": "https://auth.example.test/device"}},
		},
	}}}}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		_, err := generate(document)
		if err == nil {
			t.Fatal("OpenAPI 3.2 security fields accepted")
		}
		for _, expected := range []string{"/type", "/oauth2MetadataUrl", "/deprecated", "/flows"} {
			if !strings.Contains(err.Error(), "#/components/securitySchemes/oauth"+expected+" (typed security providers)") {
				t.Fatalf("error = %q, missing %q", err, expected)
			}
		}
	}
}

func TestSourceArtifactsRejectsOpenAPI32CookieStyleWithoutEncoder(t *testing.T) {
	document := &ir.Document{
		Operations: []ir.Operation{{
			Path:   "/widgets",
			Method: "GET",
			Raw: map[string]any{"parameters": []any{
				map[string]any{"name": "session", "in": "cookie", "style": "cookie", "schema": map[string]any{"type": "string"}},
			}},
		}},
	}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		_, err := generate(document)
		if err == nil || !strings.Contains(err.Error(), "#/paths/~1widgets/get/parameters/0/style (cookie parameter serialization)") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestSourceArtifactsRejectsMultipleScopedServersWithoutSelectionAPI(t *testing.T) {
	document := &ir.Document{
		Operations: []ir.Operation{{
			Path:   "/widgets",
			Method: "GET",
			Raw: map[string]any{"servers": []any{
				map[string]any{"url": "https://one.example.test"},
				map[string]any{"url": "https://two.example.test"},
			}},
		}},
	}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		_, err := generate(document)
		if err == nil || !strings.Contains(err.Error(), "#/paths/~1widgets/get/servers (scoped server selection)") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestSourceArtifactsRejectsXMLMediaTypesWithoutCodecSemantics(t *testing.T) {
	document := &ir.Document{
		Operations: []ir.Operation{{
			Path:   "/widgets",
			Method: "POST",
			Raw: map[string]any{
				"requestBody": map[string]any{"content": map[string]any{"application/xml": map[string]any{"schema": map[string]any{"type": "object"}}}},
				"responses":   map[string]any{"200": map[string]any{"content": map[string]any{"application/xml": map[string]any{"schema": map[string]any{"type": "object"}}}}},
			},
		}},
	}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		_, err := generate(document)
		if err == nil || !strings.Contains(err.Error(), "#/paths/~1widgets/post/requestBody/content/application~1xml (XML media-type codec)") || !strings.Contains(err.Error(), "#/paths/~1widgets/post/responses/200/content/application~1xml (XML media-type codec)") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestSourceArtifactsHandlesCaseInsensitiveJSONMediaType(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{{
		OperationID: "createWidget",
		Path:        "/widgets",
		Method:      "POST",
		Raw: map[string]any{"requestBody": map[string]any{"content": map[string]any{
			"Application/JSON": map[string]any{"schema": map[string]any{"type": "object"}},
		}}},
	}}}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		if _, err := generate(document); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSourceArtifactsAcceptsStructuredJSONSuffixesAndRejectsJSONLookalikes(t *testing.T) {
	valid := &ir.Document{Operations: []ir.Operation{{
		OperationID: "createProblem", Path: "/problems", Method: "POST", Raw: map[string]any{"requestBody": map[string]any{"content": map[string]any{
			"application/problem+json": map[string]any{"schema": map[string]any{"type": "object"}},
		}}},
	}}}
	invalid := &ir.Document{Operations: []ir.Operation{{
		OperationID: "createNotJSON", Path: "/not-json", Method: "POST", Raw: map[string]any{"requestBody": map[string]any{"content": map[string]any{
			"application/notjson": map[string]any{"schema": map[string]any{"type": "object"}},
		}}},
	}}}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		if _, err := generate(valid); err != nil {
			t.Fatal(err)
		}
		if _, err := generate(invalid); err == nil || !strings.Contains(err.Error(), "/application~1notjson (runtime media-type codec)") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestSourceArtifactsRejectsStructuredMultipartDefaultsAndUnknownMedia(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{{
		Path:   "/uploads",
		Method: "POST",
		Raw: map[string]any{
			"requestBody": map[string]any{"content": map[string]any{
				"multipart/form-data": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{
					"metadata": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}},
				}}},
			}},
			"responses": map[string]any{"200": map[string]any{"content": map[string]any{
				"application/pdf": map[string]any{"schema": map[string]any{"type": "string"}},
			}}},
		},
	}}}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		_, err := generate(document)
		if err == nil || !strings.Contains(err.Error(), "/multipart~1form-data (structured multipart default encoding)") || !strings.Contains(err.Error(), "/application~1pdf (runtime media-type codec)") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestSourceArtifactsRejectsStructuredMultipartComponentSchemas(t *testing.T) {
	document := &ir.Document{
		ComponentSchemas: map[string]map[string]any{
			"Upload": {"type": "object", "properties": map[string]any{
				"metadata": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}},
			}},
		},
		Operations: []ir.Operation{{
			Path: "/uploads", Method: "POST", Raw: map[string]any{"requestBody": map[string]any{"content": map[string]any{
				"multipart/form-data": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Upload"}},
			}}},
		}},
	}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		_, err := generate(document)
		if err == nil || !strings.Contains(err.Error(), "/multipart~1form-data (structured multipart default encoding)") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestSourceArtifactsRejectsResponseHeaderContracts(t *testing.T) {
	document := &ir.Document{
		Operations: []ir.Operation{{
			Path:   "/widgets",
			Method: "GET",
			Raw: map[string]any{"responses": map[string]any{"200": map[string]any{"headers": map[string]any{
				"X-Rate-Limit": map[string]any{"required": true, "schema": map[string]any{"type": "integer"}},
			}}}},
		}},
	}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		_, err := generate(document)
		if err == nil || !strings.Contains(err.Error(), "#/paths/~1widgets/get/responses/200/headers (typed response header contracts)") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestSourceArtifactsRejectsParameterSerializationItCannotRepresent(t *testing.T) {
	document := &ir.Document{
		Operations: []ir.Operation{{
			Path:   "/widgets",
			Method: "GET",
			Raw: map[string]any{"parameters": []any{
				map[string]any{"name": "filter", "in": "query", "style": "pipeDelimited", "schema": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}},
				map[string]any{"name": "xml", "in": "query", "content": map[string]any{"application/xml": map[string]any{"schema": map[string]any{"type": "object"}}}},
			}},
		}},
	}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		_, err := generate(document)
		if err == nil || !strings.Contains(err.Error(), "/style (object serialization for pipeDelimited)") || !strings.Contains(err.Error(), "/application~1xml (structured parameter content serialization)") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestSourceArtifactsRejectsOpenAPI32StreamingAndPositionalMediaFeatures(t *testing.T) {
	document := &ir.Document{Raw: map[string]any{
		"openapi": "3.2.0",
	}, Operations: []ir.Operation{{
		Path: "/logs", Method: "POST", Raw: map[string]any{
			"requestBody": map[string]any{"content": map[string]any{
				"multipart/mixed": map[string]any{
					"prefixEncoding": []any{map[string]any{}},
					"itemEncoding":   map[string]any{"contentType": "application/octet-stream"},
				},
				"application/x-ndjson": map[string]any{"schema": map[string]any{"type": "object"}},
			}},
			"responses": map[string]any{"200": map[string]any{"content": map[string]any{
				"application/json-seq": map[string]any{"itemSchema": map[string]any{"type": "object"}},
			}}},
		},
	}}}
	for _, generate := range []func(*ir.Document) ([]Artifact, error){SourceArtifacts, JavaScriptSourceArtifacts} {
		_, err := generate(document)
		if err == nil {
			t.Fatal("OpenAPI 3.2 streaming/positional media features accepted")
		}
		for _, expected := range []string{"/prefixEncoding", "/itemEncoding", "/itemSchema", "/application~1x-ndjson (streaming request encoder)"} {
			if !strings.Contains(err.Error(), expected) {
				t.Fatalf("error = %q, missing %q", err, expected)
			}
		}
	}
}

func TestSourceArtifactsGenerateOpenAPI32QueryAndAdditionalOperations(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.2.0",
  "info": {"title": "Operations", "version": "1"},
  "paths": {
    "/records": {
      "query": {"operationId": "queryRecords", "responses": {"200": {"description": "OK"}}},
      "additionalOperations": {
        "PURGE": {"operationId": "purgeRecords", "responses": {"204": {"description": "Deleted"}}}
      }
    }
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	client := artifactByPath(t, artifacts, "generated/client.ts")
	for _, expected := range []string{
		`operationID: "queryRecords", method: "QUERY"`,
		`operationID: "purgeRecords", method: "PURGE"`,
	} {
		if !strings.Contains(string(client), expected) {
			t.Fatalf("client source missing %q:\n%s", expected, client)
		}
	}
}

func TestSourceArtifactsGenerateEveryStandardHTTPMethod(t *testing.T) {
	methods := []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}
	paths := make(map[string]any, len(methods))
	for _, method := range methods {
		paths["/"+method] = map[string]any{method: map[string]any{
			"operationId": method + "Operation",
			"responses":   map[string]any{"204": map[string]any{"description": "No Content"}},
		}}
	}
	document, err := sdkgen.Compile([]byte(marshalOpenAPIDocument(t, "3.2.0", paths)))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	client := string(artifactByPath(t, artifacts, "generated/client.ts"))
	for _, method := range methods {
		expected := fmt.Sprintf(`operationID: %q, method: %q`, method+"Operation", strings.ToUpper(method))
		if !strings.Contains(client, expected) {
			t.Fatalf("client source missing %q:\n%s", expected, client)
		}
	}
}

func marshalOpenAPIDocument(t *testing.T, version string, paths map[string]any) string {
	t.Helper()
	contents, err := json.Marshal(map[string]any{
		"openapi": version,
		"info":    map[string]any{"title": "Methods", "version": "1"},
		"paths":   paths,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func TestSourceArtifactsGenerateAcrossSupportedOpenAPIVersionLines(t *testing.T) {
	for _, test := range []struct {
		version string
		schema  string
		want    string
	}{
		{"3.0.3", `{"type":"string","nullable":true}`, "export type Item = string | null"},
		{"3.1.1", `{"type":["string","null"]}`, "export type Item = string | null"},
		{"3.2.0", `{"const":"stable"}`, `export type Item = "stable"`},
	} {
		t.Run(test.version, func(t *testing.T) {
			input := fmt.Sprintf(`{
  "openapi": %q,
  "info": {"title": "Versioned", "version": "1"},
  "paths": {"/item": {"get": {"operationId": "getItem", "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Item"}}}}}}}},
  "components": {"schemas": {"Item": %s}}
}`, test.version, test.schema)
			document, err := sdkgen.Compile([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			artifacts, err := SourceArtifacts(document)
			if err != nil {
				t.Fatal(err)
			}
			if source := string(artifactByPath(t, artifacts, "generated/types.ts")); !strings.Contains(source, test.want) {
				t.Fatalf("types source missing %q:\n%s", test.want, source)
			}
		})
	}
}

func TestSourceArtifactsDoesNotApplyOpenAPI30NullableToOpenAPI31Schemas(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.1", "info":{"title":"Nullable","version":"1"},
  "paths":{"/item":{"get":{"operationId":"getItem","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Item"}}}}}}}},
  "components":{"schemas":{"Item":{"type":"string","nullable":true}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	if source := string(artifactByPath(t, artifacts, "generated/types.ts")); !strings.Contains(source, "export type Item = string") || strings.Contains(source, "export type Item = string | null") {
		t.Fatalf("unexpected nullable 3.1 lowering:\n%s", source)
	}
}

func artifactByPath(t *testing.T, artifacts []Artifact, path string) []byte {
	t.Helper()
	for _, artifact := range artifacts {
		if artifact.Path == path {
			return artifact.Data
		}
	}
	t.Fatalf("missing artifact %s", path)
	return nil
}
