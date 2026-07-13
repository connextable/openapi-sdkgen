package sdkgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompileBuildsValidatedIR(t *testing.T) {
	input := []byte(`{
  "openapi": "3.2.0",
  "info": {"title": "Example", "version": "0.1.0"},
  "servers": [{"url": "/v1"}],
  "paths": {
    "/healthz": {
      "servers": [{"url": "/"}],
      "get": {
        "operationId": "getHealth",
		"security": [],
        "responses": {"200": {"description": "OK"}},
        "x-envelope": "none",
        "x-concurrency": "none",
        "x-idempotency": "unsupported",
        "x-sdk-visibility": "public"
      }
    }
  }
}`)
	document, err := Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Operations) != 1 || document.Operations[0].OperationID != "getHealth" {
		t.Fatalf("operations = %#v", document.Operations)
	}
}

func TestCompileAcceptsGenericOpenAPIWithoutProjectProfile(t *testing.T) {
	input := []byte(`{"openapi":"3.2.0","info":{"title":"Generic","version":"1"},"paths":{}}`)
	if _, err := Compile(input); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileProject(input); err == nil {
		t.Fatal("project profile accepted generic document")
	}
}

func TestCompileFileBundlesInDirectoryReferencesForEverySupportedVersionLine(t *testing.T) {
	for _, version := range []string{"3.0.3", "3.1.1", "3.2.0"} {
		t.Run(version, func(t *testing.T) {
			directory := t.TempDir()
			main := `{
  "openapi": "` + version + `",
  "info": {"title": "External", "version": "1"},
  "paths": {
    "/things": {
      "get": {
        "operationId": "listThings",
        "responses": {
          "200": {
            "description": "OK",
            "content": {"application/json": {"schema": {"$ref": "schemas.json#/Thing"}}}
          }
        }
      }
    }
  }
}`
			mainPath := filepath.Join(directory, "openapi.json")
			if err := os.WriteFile(mainPath, []byte(main), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "schemas.json"), []byte(`{"Thing":{"type":"object","properties":{"id":{"type":"string"}}}}`), 0o600); err != nil {
				t.Fatal(err)
			}

			document, err := CompileFile(mainPath)
			if err != nil {
				t.Fatal(err)
			}
			if document.OpenAPIVersion != version {
				t.Fatalf("version = %q, want %q", document.OpenAPIVersion, version)
			}
			if len(document.Operations) != 1 || document.Operations[0].OperationID != "listThings" {
				t.Fatalf("operations = %#v", document.Operations)
			}
		})
	}
}

func TestCompileProjectFileResolvesExternalReferences(t *testing.T) {
	directory := t.TempDir()
	main := `{"openapi":"3.2.0","info":{"title":"External","version":"1"},"servers":[{"url":"/v1"}],"paths":{"/things":{"get":{"operationId":"listThings","security":[],"responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"schemas.json#/Thing"}}}}},"x-envelope":"none","x-concurrency":"none","x-idempotency":"unsupported","x-sdk-visibility":"public"}}}}`
	external := `{"Thing":{"type":"object","properties":{"requestId":{"type":"string"}}}}`
	mainPath := filepath.Join(directory, "openapi.json")
	if err := os.WriteFile(mainPath, []byte(main), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "schemas.json"), []byte(external), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := CompileFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Operations) != 1 {
		t.Fatalf("operations = %#v", document.Operations)
	}
	if _, err := CompileProjectFile(mainPath); err == nil {
		t.Fatal("project compiler accepted external reference")
	}
}

func TestCompileFileRejectsReferenceOutsideInputDirectory(t *testing.T) {
	root := t.TempDir()
	inputDirectory := filepath.Join(root, "input")
	if err := os.Mkdir(inputDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "outside.json"), []byte(`{"Thing":{"type":"object"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(inputDirectory, "openapi.json")
	input := `{"openapi":"3.2.0","info":{"title":"External","version":"1"},"paths":{"/things":{"get":{"operationId":"listThings","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"../outside.json#/Thing"}}}}}}}}}`
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileFile(path); err == nil || !strings.Contains(err.Error(), "escapes the input directory") {
		t.Fatalf("CompileFile error = %v", err)
	}
}

func TestCompileFileRejectsTransitiveReferenceOutsideInputDirectory(t *testing.T) {
	root := t.TempDir()
	inputDirectory := filepath.Join(root, "input")
	if err := os.MkdirAll(filepath.Join(inputDirectory, "schemas"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "outside.json"), []byte(`{"Thing":{"type":"object"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDirectory, "schemas", "first.json"), []byte(`{"Thing":{"$ref":"../../outside.json#/Thing"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(inputDirectory, "openapi.json")
	input := `{"openapi":"3.2.0","info":{"title":"External","version":"1"},"paths":{"/things":{"get":{"operationId":"listThings","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"schemas/first.json#/Thing"}}}}}}}}}`
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileFile(path); err == nil || !strings.Contains(err.Error(), "escapes the input directory") {
		t.Fatalf("CompileFile error = %v", err)
	}
}

func TestCompileFileAcceptsYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	input := "openapi: 3.2.0\ninfo:\n  title: YAML\n  version: 1\npaths: {}\n"
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileFile(path); err != nil {
		t.Fatal(err)
	}
}

func TestCompileFilePreservesOpenAPI32AdditionalOperations(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "openapi.json")
	if err := os.WriteFile(path, []byte(`{
  "openapi": "3.2.0",
  "info": {"title": "Additional operations", "version": "1"},
  "paths": {
    "/records": {
      "additionalOperations": {
        "PURGE": {"operationId": "purgeRecords", "responses": {"204": {"description": "Deleted"}}}
      }
    }
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := CompileFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Operations) != 1 || document.Operations[0].OperationID != "purgeRecords" || document.Operations[0].Method != "PURGE" {
		t.Fatalf("compiled operations = %#v", document.Operations)
	}
}

func TestCompileNormalizesNestedComponentSchemaReferences(t *testing.T) {
	document, err := Compile([]byte(`{
  "openapi":"3.1.0",
  "info":{"title":"Nested","version":"1"},
  "paths":{"/holder":{"get":{"operationId":"getHolder","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Holder"}}}}}}}},
  "components":{"schemas":{
    "Thing":{"type":"object","properties":{"id":{"type":"string"}}},
    "Holder":{"type":"object","properties":{"id":{"$ref":"#/components/schemas/Thing/properties/id"}}}
  }}
}`))
	if err != nil {
		t.Fatal(err)
	}
	holder := document.ComponentSchemas["Holder"]
	properties, _ := holder["properties"].(map[string]any)
	id, _ := properties["id"].(map[string]any)
	if id["type"] != "string" || id["$ref"] != nil {
		t.Fatalf("normalized nested schema = %#v", id)
	}
}

func TestCompileNormalizesLocalSchemaAnchorReferences(t *testing.T) {
	document, err := Compile([]byte(`{
  "openapi":"3.1.0",
  "info":{"title":"Anchors","version":"1"},
  "paths":{},
  "components":{"schemas":{
    "Node":{
      "$id":"https://schemas.example.test/node",
      "$anchor":"node",
      "type":"object",
      "properties":{"next":{"$ref":"#node"}}
    },
    "Envelope":{
      "type":"object",
      "properties":{"node":{"$ref":"https://schemas.example.test/node#node"}}
    }
  }}
}`))
	if err != nil {
		t.Fatal(err)
	}
	node := document.ComponentSchemas["Node"]
	if _, exists := node["$anchor"]; exists {
		t.Fatalf("anchor was not lowered: %#v", node)
	}
	properties, _ := node["properties"].(map[string]any)
	next, _ := properties["next"].(map[string]any)
	if next["$ref"] != "#/components/schemas/Node" {
		t.Fatalf("anchored reference = %#v", next)
	}
	envelope := document.ComponentSchemas["Envelope"]
	envelopeProperties, _ := envelope["properties"].(map[string]any)
	child, _ := envelopeProperties["node"].(map[string]any)
	if child["$ref"] != "#/components/schemas/Node" {
		t.Fatalf("anchored reference = %#v", child)
	}
}

func TestCompileUsesOpenAPI32SelfAsSchemaResourceBase(t *testing.T) {
	document, err := Compile([]byte(`{
  "openapi":"3.2.0", "$self":"https://schemas.example.test/openapi.json",
  "info":{"title":"Self base","version":"1"}, "paths":{},
  "components":{"schemas":{
    "Node":{"$id":"schemas/node","$anchor":"node","type":"object"},
    "Envelope":{"type":"object","properties":{"node":{"$ref":"https://schemas.example.test/schemas/node#node"}}}
  }}
}`))
	if err != nil {
		t.Fatal(err)
	}
	envelope := document.ComponentSchemas["Envelope"]
	properties, _ := envelope["properties"].(map[string]any)
	node, _ := properties["node"].(map[string]any)
	if node["$ref"] != "#/components/schemas/Node" {
		t.Fatalf("$self-relative anchor reference = %#v", node)
	}
}

func TestCompileLowersDynamicSchemaReferenceMetadata(t *testing.T) {
	document, err := Compile([]byte(`{
  "openapi":"3.1.0", "info":{"title":"Anchors","version":"1"}, "paths":{},
  "components":{"schemas":{"Node":{"$dynamicAnchor":"node","type":"object","properties":{"child":{"$dynamicRef":"#node"}}}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	node := document.ComponentSchemas["Node"]
	if node[dynamicAnchorMetadataKey] != "node" {
		t.Fatalf("dynamic anchor metadata = %#v", node)
	}
	properties, _ := node["properties"].(map[string]any)
	child, _ := properties["child"].(map[string]any)
	dynamic, _ := child[dynamicReferenceMetadataKey].(map[string]any)
	if dynamic["anchor"] != "node" || dynamic["reference"] != "#/components/schemas/Node" {
		t.Fatalf("dynamic reference metadata = %#v", child)
	}
}

func TestCompileDoesNotInterpretExampleReferenceLikeValuesAsSchemas(t *testing.T) {
	document, err := Compile([]byte(`{
  "openapi":"3.1.0", "info":{"title":"Examples","version":"1"}, "paths":{},
  "components":{"schemas":{"Thing":{"type":"object","properties":{"id":{"type":"string"}}},"Example":{"type":"object","examples":[{"$ref":"#/components/schemas/Thing/properties/id"}]}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	example := document.ComponentSchemas["Example"]
	examples, _ := example["examples"].([]any)
	value, _ := examples[0].(map[string]any)
	if value["$ref"] != "#/components/schemas/Thing/properties/id" {
		t.Fatalf("example value changed: %#v", value)
	}
}

func TestCompileProjectValidatesResolvedPathItemParameters(t *testing.T) {
	input := []byte(`{
		"openapi":"3.2.0",
		"info":{"title":"Refs","version":"1"},
		"servers":[{"url":"/v1"}],
		"paths":{"/things/{thingID}":{"$ref":"#/components/pathItems/Thing"}},
		"components":{"pathItems":{"Thing":{
			"parameters":[{"name":"thingID","in":"path","required":true,"schema":{"type":"string"}}],
			"get":{"operationId":"getThing","security":[],"responses":{"204":{"description":"OK"}},"x-envelope":"none","x-concurrency":"none","x-idempotency":"unsupported","x-sdk-visibility":"public"}
		}}}
	}`)
	if _, err := CompileProject(input); err != nil {
		t.Fatal(err)
	}
}
