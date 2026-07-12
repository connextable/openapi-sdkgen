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
