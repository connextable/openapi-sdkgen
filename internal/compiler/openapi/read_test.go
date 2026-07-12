package openapi

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

const minimalDocument = `{
  "openapi": "3.2.0",
  "info": {"title": "Example", "version": "0.1.0"},
  "servers": [{"url": "/v1"}],
  "paths": {}
}`

func TestReadBuildsSupportedOpenAPI3Models(t *testing.T) {
	for version, wantLine := range map[string]VersionLine{
		"3.0.0":              Version30,
		"3.0.4":              Version30,
		"3.1.0":              Version31,
		"3.1.2":              Version31,
		"3.1.3-rc.1+build.7": Version31,
		"3.2.0":              Version32,
		"3.2.1":              Version32,
	} {
		t.Run(version, func(t *testing.T) {
			input := strings.Replace(minimalDocument, "3.2.0", version, 1)
			document, err := Read([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			if got := document.Raw["openapi"]; got != version {
				t.Fatalf("openapi = %v, want %s", got, version)
			}
			if document.Version != wantLine {
				t.Fatalf("version line = %q, want %q", document.Version, wantLine)
			}
		})
	}
}

func TestReadAcceptsYAMLForEverySupportedVersionLine(t *testing.T) {
	for _, version := range []string{"3.0.4", "3.1.2", "3.2.0"} {
		t.Run(version, func(t *testing.T) {
			input := "openapi: \"" + version + "\"\ninfo:\n  title: YAML\n  version: \"1\"\npaths: {}\n"
			document, err := Read([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			if document.Raw["openapi"] != version {
				t.Fatalf("openapi = %#v", document.Raw["openapi"])
			}
		})
	}
}

func TestReadAcceptsYAMLFlowMappingsThatStartWithABrace(t *testing.T) {
	for _, input := range []string{
		`{openapi: "3.1.1", info: {title: YAML, version: "1"}, paths: {}}`,
		`{"openapi": 3.1.1, info: {title: YAML, version: "1"}, paths: {}}`,
		`{"openapi":"3.1.1","info":{"title":"YAML","version":"1"},"paths":{}} # YAML comment`,
	} {
		document, err := Read([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		if document.Version != Version31 {
			t.Fatalf("version = %q", document.Version)
		}
	}
}

func TestReadRejectsUnsupportedOpenAPIVersionLines(t *testing.T) {
	for _, version := range []string{"2.0", "3.2", "3.3.0", "4.0.0", "3.01.0", "3.1.00", "3.1.1-01", "3.1.1-rc.01"} {
		t.Run(version, func(t *testing.T) {
			input := strings.Replace(minimalDocument, "3.2.0", version, 1)
			if _, err := Read([]byte(input)); err == nil {
				t.Fatalf("version %s accepted", version)
			}
		})
	}
}

func TestReadRejectsLaterMinorFeaturesAtJSONPointers(t *testing.T) {
	for _, test := range []struct {
		name    string
		version string
		insert  string
		pointer string
	}{
		{"3.0 type array", "3.0.3", `,"components":{"schemas":{"Item":{"type":["string","null"]}}}`, "#/components/schemas/Item/type"},
		{"3.0 nested prefix items", "3.0.3", `,"components":{"schemas":{"Item":{"properties":{"tuple":{"prefixItems":[]}}}}}`, "#/components/schemas/Item/properties/tuple/prefixItems"},
		{"3.0 info summary", "3.0.3", `,"info":{"title":"Example","version":"0.1.0","summary":"later"}`, "#/info/summary"},
		{"3.0 license identifier", "3.0.3", `,"info":{"title":"Example","version":"0.1.0","license":{"name":"MIT","identifier":"MIT"}}`, "#/info/license/identifier"},
		{"3.0 mutual TLS", "3.0.3", `,"components":{"securitySchemes":{"mtls":{"type":"mutualTLS"}}}`, "#/components/securitySchemes/mtls/type"},
		{"3.0 component path item", "3.0.3", `,"components":{"pathItems":{"Shared":{"query":{}}}}`, "#/components/pathItems"},
		{"3.0 numeric exclusive bound", "3.0.3", `,"components":{"schemas":{"Limit":{"type":"number","exclusiveMaximum":5}}}`, "#/components/schemas/Limit/exclusiveMaximum"},
		{"3.0 response reference sibling", "3.0.3", `,"components":{"responses":{"Base":{"description":"OK"}},"schemas":{}},"paths":{"/items":{"get":{"responses":{"200":{"$ref":"#/components/responses/Base","description":"later"}}}}}`, "#/paths/~1items/get/responses/200/description"},
		{"3.0 callback response reference sibling", "3.0.3", `,"components":{"responses":{"Base":{"description":"OK"}}},"paths":{"/subscriptions":{"post":{"callbacks":{"event":{"{$request.body#/callbackUrl}":{"post":{"responses":{"200":{"$ref":"#/components/responses/Base","description":"later"}}}}}}}}}`, "#/paths/~1subscriptions/post/callbacks/event/{$request.body#~1callbackUrl}/post/responses/200/description"},
		{"3.0 example reference sibling", "3.0.3", `,"components":{"examples":{"Base":{"value":"one"},"Alias":{"$ref":"#/components/examples/Base","description":"later"}}}`, "#/components/examples/Alias/description"},
		{"3.1 reusable querystring", "3.1.1", `,"components":{"parameters":{"Query":{"name":"query","in":"querystring"}}}`, "#/components/parameters/Query/in"},
		{"3.1 querystring", "3.1.1", `,"paths":{"/items":{"get":{"parameters":[{"name":"query","in":"querystring"}]}}}`, "#/paths/~1items/get/parameters/0/in"},
		{"3.1 self", "3.1.1", `,"$self":"https://example.test/openapi.json"`, "#/$self"},
		{"3.1 streaming item schema", "3.1.1", `,"paths":{"/items":{"get":{"responses":{"200":{"description":"OK","content":{"application/json":{"itemSchema":{"type":"object"}}}}}}}}`, "#/paths/~1items/get/responses/200/content/application~1json/itemSchema"},
		{"3.1 server name", "3.1.1", `,"servers":[{"url":"/","name":"production"}]`, "#/servers/0/name"},
		{"3.1 tag parent", "3.1.1", `,"tags":[{"name":"widgets","parent":"api"}]`, "#/tags/0/parent"},
		{"3.1 example data value", "3.1.1", `,"components":{"examples":{"Widget":{"dataValue":{"id":"1"}}}}`, "#/components/examples/Widget/dataValue"},
		{"3.1 cookie style", "3.1.1", `,"paths":{"/items":{"get":{"parameters":[{"name":"session","in":"cookie","style":"cookie"}]}}}`, "#/paths/~1items/get/parameters/0/style"},
		{"3.1 webhook query operation", "3.1.1", `,"webhooks":{"hook":{"query":{"operationId":"bad","responses":{"200":{"description":"OK"}}}}}`, "#/webhooks/hook/query"},
		{"3.1 oauth metadata", "3.1.1", `,"components":{"securitySchemes":{"oauth":{"type":"oauth2","oauth2MetadataUrl":"https://auth.example.test/metadata"}}}`, "#/components/securitySchemes/oauth/oauth2MetadataUrl"},
		{"3.1 device authorization flow", "3.1.1", `,"components":{"securitySchemes":{"oauth":{"type":"oauth2","flows":{"deviceAuthorization":{"deviceAuthorizationUrl":"https://auth.example.test/device"}}}}}`, "#/components/securitySchemes/oauth/flows/deviceAuthorization"},
		{"3.1 response summary", "3.1.1", `,"paths":{"/items":{"get":{"responses":{"200":{"description":"OK","summary":"Items"}}}}}`, "#/paths/~1items/get/responses/200/summary"},
		{"3.1 discriminator default mapping", "3.1.1", `,"components":{"schemas":{"Pet":{"oneOf":[],"discriminator":{"propertyName":"kind","defaultMapping":"Other"}}}}`, "#/components/schemas/Pet/discriminator/defaultMapping"},
		{"3.1 XML node type", "3.1.1", `,"components":{"schemas":{"Pet":{"type":"object","xml":{"nodeType":"element"}}}}`, "#/components/schemas/Pet/xml/nodeType"},
		{"3.1 defs XML node type", "3.1.1", `,"components":{"schemas":{"Pet":{"$defs":{"Child":{"type":"object","xml":{"nodeType":"element"}}}}}}`, "#/components/schemas/Pet/$defs/Child/xml/nodeType"},
		{"3.1 link server name", "3.1.1", `,"paths":{"/source":{"get":{"operationId":"source","responses":{"200":{"description":"OK","links":{"next":{"operationId":"target","server":{"url":"https://api.example.test","name":"prod"}}}}}}},"/target":{"get":{"operationId":"target","responses":{"200":{"description":"OK"}}}}}`, "#/paths/~1source/get/responses/200/links/next/server/name"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := `{"openapi":"` + test.version + `","info":{"title":"Example","version":"0.1.0"},"paths":{}}`
			input = strings.TrimSuffix(input, `}`) + test.insert + `}`
			if _, err := Read([]byte(input)); err == nil || !strings.Contains(err.Error(), test.pointer) {
				t.Fatalf("Read error = %v, want pointer %s", err, test.pointer)
			}
		})
	}
}

func TestReadIgnoresVersionLikeVendorExtensionPayloads(t *testing.T) {
	input := `{"openapi":"3.0.3","info":{"title":"Example","version":"1"},"paths":{},"x-custom":{"schema":{"type":["string","null"]}}}`
	if _, err := Read([]byte(input)); err != nil {
		t.Fatal(err)
	}
}

func TestReadAllowsOpenAPI30PathItemReferenceSiblings(t *testing.T) {
	input := `{"openapi":"3.0.3","info":{"title":"Path Item reference","version":"1"},"paths":{"/common":{"get":{"responses":{"200":{"description":"OK"}}}},"/pets":{"$ref":"#/paths/~1common","summary":"Pets alias"}}}`
	if _, err := Read([]byte(input)); err != nil {
		t.Fatal(err)
	}
}

func TestPathItemReferencePathRecognizesNestedCallbackPathItems(t *testing.T) {
	path := "#/paths/~1subscriptions/post/callbacks/outer/{$request.body#~1url}/post/callbacks/inner/{$request.body#~1callbackUrl}"
	if !isPathItemReferencePath(path) {
		t.Fatalf("nested callback path item = %q", path)
	}
	if isPathItemReferencePath(path + "/post/responses/200") {
		t.Fatal("nested callback response was mistaken for a Path Item")
	}
}

func TestReadDoesNotInterpretExampleInstancesAsOpenAPIOrSchemaSyntax(t *testing.T) {
	input := `{"openapi":"3.0.3","info":{"title":"Example","version":"1"},"paths":{"/items":{"get":{"responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"type":"object"},"examples":{"literal":{"value":{"schema":{"type":["string","null"]},"$ref":"literal","value":1}}}}}}}}}}}`
	if _, err := Read([]byte(input)); err != nil {
		t.Fatal(err)
	}
}

func TestReadDoesNotTreatJSONSchemaVocabularyAsOpenAPI32MediaSyntax(t *testing.T) {
	input := `{"openapi":"3.1.1","info":{"title":"Schema vocabulary","version":"1"},"paths":{},"components":{"schemas":{"Widget":{"itemSchema":{"type":"string"}}}}}`
	if _, err := Read([]byte(input)); err != nil {
		t.Fatal(err)
	}
}

func TestReadAcceptsVersionCorrectExclusiveBounds(t *testing.T) {
	for _, input := range []string{
		`{"openapi":"3.0.3","info":{"title":"Example","version":"1"},"paths":{},"components":{"schemas":{"Limit":{"type":"number","maximum":5,"exclusiveMaximum":true}}}}`,
		`{"openapi":"3.1.1","info":{"title":"Example","version":"1"},"paths":{},"components":{"schemas":{"Limit":{"type":"number","exclusiveMaximum":5}}}}`,
	} {
		if _, err := Read([]byte(input)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadAcceptsOpenAPI32OnlyFields(t *testing.T) {
	input := `{
  "openapi":"3.2.0",
  "info":{"title":"OAS 3.2","version":"1"},
  "servers":[{"url":"/","name":"production"}],
  "tags":[{"name":"widgets","summary":"Widgets","parent":"api","kind":"nav"}],
  "components":{"examples":{"Widget":{"dataValue":{"id":"1"},"serializedValue":"{\"id\":\"1\"}"}}},
  "paths":{"/widgets":{"get":{"parameters":[{"name":"session","in":"cookie","style":"cookie","schema":{"type":"string"}}],"responses":{"200":{"description":"OK","summary":"Widgets"}}}}}
}`
	if _, err := Read([]byte(input)); err != nil {
		t.Fatal(err)
	}
}

func TestReadRejectsMissingOrNonStringVersion(t *testing.T) {
	for name, input := range map[string]string{
		"missing":    strings.Replace(minimalDocument, `"openapi": "3.2.0",`, "", 1),
		"non-string": strings.Replace(minimalDocument, `"3.2.0"`, "3.2", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Read([]byte(input)); err == nil {
				t.Fatal("invalid OpenAPI version accepted")
			}
		})
	}
}

func TestReadPreservesOpenAPI32DocumentLosslessly(t *testing.T) {
	data, err := os.ReadFile("testdata/oas32-conformance.json")
	if err != nil {
		t.Fatal(err)
	}
	document, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}

	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var want map[string]any
	if err := decoder.Decode(&want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(document.Raw, want) {
		t.Fatal("parsed raw document differs from source JSON")
	}

	rootExtension := document.Raw["x-root-extension"].(map[string]any)
	nested := rootExtension["nested"].(map[string]any)
	if got := nested["count"]; got != json.Number("9007199254740993") {
		t.Fatalf("large extension number = %#v", got)
	}
	item := document.Raw["components"].(map[string]any)["schemas"].(map[string]any)["Item"].(map[string]any)
	for _, keyword := range []string{"$schema", "$id", "dependentRequired", "unevaluatedProperties", "exampleVocabularyKeyword", "x-schema-extension"} {
		if _, ok := item[keyword]; !ok {
			t.Errorf("schema keyword %q was not preserved", keyword)
		}
	}
	coordinates := item["properties"].(map[string]any)["coordinates"].(map[string]any)
	if _, ok := coordinates["prefixItems"]; !ok {
		t.Error("schema keyword \"prefixItems\" was not preserved")
	}
}

func TestReadRejectsTrailingJSON(t *testing.T) {
	if _, err := Read([]byte(minimalDocument + `{}`)); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}
