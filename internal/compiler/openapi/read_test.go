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

func TestReadBuildsOpenAPI32Model(t *testing.T) {
	document, err := Read([]byte(minimalDocument))
	if err != nil {
		t.Fatal(err)
	}
	if document.Raw["openapi"] != SupportedVersion {
		t.Fatalf("openapi = %v", document.Raw["openapi"])
	}
}

func TestReadRequiresExactOpenAPI320Version(t *testing.T) {
	for _, version := range []string{"2.0", "3.0.4", "3.1.2", "3.2.1", "3.2"} {
		t.Run(version, func(t *testing.T) {
			input := strings.Replace(minimalDocument, "3.2.0", version, 1)
			if _, err := Read([]byte(input)); err == nil {
				t.Fatalf("version %s accepted", version)
			}
		})
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
