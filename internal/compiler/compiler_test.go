package sdkgen

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCompileInputAcceptsPathFileURLHTTPAndStandardInput(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "contract.yaml")
	contents := []byte(`openapi: 3.2.0
info:
  title: Source inputs
  version: "1"
paths: {}
`)
	if err := os.WriteFile(input, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	fileURL := (&url.URL{Scheme: "file", Path: input}).String()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/openapi" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "text/plain")
		_, _ = response.Write(contents)
	}))
	defer server.Close()
	var expected any
	for _, test := range []struct {
		name    string
		input   string
		options CompileOptions
	}{
		{name: "path", input: input},
		{name: "file URL", input: fileURL},
		{name: "HTTP URL", input: server.URL + "/openapi"},
		{name: "standard input", input: "-", options: CompileOptions{InputReader: strings.NewReader(string(contents))}},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := CompileInputWithOptions(test.input, test.options)
			if err != nil {
				t.Fatal(err)
			}
			if document.OpenAPIVersion != "3.2.0" {
				t.Fatalf("version = %q", document.OpenAPIVersion)
			}
			if expected == nil {
				expected = document
				return
			}
			if !reflect.DeepEqual(expected, document) {
				t.Fatal("equivalent input sources produced different compiler documents")
			}
		})
	}
}

func TestInputLocatorDoesNotTreatFilesystemPathsAsURLs(t *testing.T) {
	for _, value := range []string{"C:\\work\\openapi.yaml", "schema:openapi.yaml", "./schema:openapi.yaml"} {
		if isURLInput(value) {
			t.Fatalf("filesystem input %q was classified as a URL", value)
		}
	}
	if _, err := parseInputBase("C:\\work\\openapi.yaml"); err != nil {
		t.Fatalf("Windows input base classification failed: %v", err)
	}
	for _, value := range []string{"file:///workspace/openapi.yaml", "http://localhost:4010/openapi.yaml", "https://api.example.test/openapi.yaml"} {
		if !isURLInput(value) {
			t.Fatalf("URL input %q was not classified as a URL", value)
		}
	}
}

func TestCompileInputResolvesRelativeReferencesFromURLAndStdinBase(t *testing.T) {
	directory := t.TempDir()
	schema := []byte(`Thing:
  type: object
  required: [id]
  properties:
    id: {type: string}
`)
	input := []byte(`openapi: 3.2.0
info: {title: Relative reference, version: "1"}
paths:
  /things:
    get:
      operationId: listThings
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema: {$ref: schemas.yaml#/Thing}
`)
	if err := os.WriteFile(filepath.Join(directory, "schemas.yaml"), schema, 0o600); err != nil {
		t.Fatal(err)
	}
	fileInput := filepath.Join(directory, "openapi.yaml")
	if err := os.WriteFile(fileInput, input, 0o600); err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileInputWithOptions((&url.URL{Scheme: "file", Path: fileInput}).String(), CompileOptions{})
	if err != nil || len(compiled.Operations) != 1 {
		t.Fatalf("file URL compilation = %#v, %v", compiled, err)
	}
	if _, err := CompileInputWithOptions("-", CompileOptions{InputReader: strings.NewReader(string(input))}); err == nil || !strings.Contains(err.Error(), "--input-base") {
		t.Fatalf("stdin relative reference error = %v", err)
	}
	compiled, err = CompileInputWithOptions("-", CompileOptions{
		InputReader: strings.NewReader(string(input)),
		InputBase:   filepath.Join(directory, "openapi.yaml"),
	})
	if err != nil || len(compiled.Operations) != 1 {
		t.Fatalf("stdin base compilation = %#v, %v", compiled, err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/openapi.yaml":
			_, _ = response.Write(input)
		case "/schemas.yaml":
			_, _ = response.Write(schema)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	if _, err := CompileInputWithOptions(server.URL+"/openapi.yaml", CompileOptions{}); err == nil || !strings.Contains(err.Error(), "--ref-lock") {
		t.Fatalf("URL relative reference without lock error = %v", err)
	}
	compiled, err = CompileInputWithOptions(server.URL+"/openapi.yaml", CompileOptions{
		RefLockPath:   filepath.Join(directory, "remote.lock"),
		UpdateRefLock: true,
	})
	if err != nil || len(compiled.Operations) != 1 {
		t.Fatalf("URL base compilation = %#v, %v", compiled, err)
	}
	compiled, err = CompileInputWithOptions(server.URL+"/openapi.yaml", CompileOptions{
		RefLockPath: filepath.Join(directory, "remote.lock"),
	})
	if err != nil || len(compiled.Operations) != 1 {
		t.Fatalf("URL base locked compilation = %#v, %v", compiled, err)
	}

	crossOrigin := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write(schema)
	}))
	defer crossOrigin.Close()
	crossDocument := strings.Replace(string(input), "schemas.yaml", crossOrigin.URL+"/schemas.yaml", 1)
	root := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write([]byte(crossDocument))
	}))
	defer root.Close()
	if _, err := CompileInputWithOptions(root.URL+"/openapi.yaml", CompileOptions{RefLockPath: filepath.Join(directory, "cross.lock"), UpdateRefLock: true}); err == nil {
		t.Fatalf("cross-origin reference error = %v", err)
	}
	compiled, err = CompileInputWithOptions(root.URL+"/openapi.yaml", CompileOptions{
		RemoteRefAllowlist:    []string{crossOrigin.URL},
		RefLockPath:           filepath.Join(directory, "cross.lock"),
		UpdateRefLock:         true,
		remoteReferenceClient: crossOrigin.Client(),
		remoteReferenceLookup: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		},
	})
	if err != nil || len(compiled.Operations) != 1 {
		t.Fatalf("allowlisted cross-origin compilation = %#v, %v", compiled, err)
	}
}

func TestCompileFileIgnoresUnusedSiblingReferenceLock(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "openapi.json")
	if err := os.WriteFile(input, []byte(`{"openapi":"3.2.0","info":{"title":"Input","version":"1"},"paths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input+".openapi-sdkgen.lock", []byte("not JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileFile(input); err != nil {
		t.Fatalf("self-contained file read an unused lock: %v", err)
	}
}

func TestCompileInputRejectsOfflineHTTPAndUnexpectedInputBase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("offline HTTP input opened a request")
	}))
	defer server.Close()
	if _, err := CompileInputWithOptions(server.URL, CompileOptions{Offline: true}); err == nil || !strings.Contains(err.Error(), "--offline") {
		t.Fatalf("offline error = %v", err)
	}
	if _, err := CompileInputWithOptions("-", CompileOptions{InputReader: strings.NewReader("")}); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty stdin error = %v", err)
	}
	directory := t.TempDir()
	input := filepath.Join(directory, "openapi.json")
	if err := os.WriteFile(input, []byte(`{"openapi":"3.2.0","info":{"title":"Input","version":"1"},"paths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileInputWithOptions(input, CompileOptions{InputBase: input}); err == nil || !strings.Contains(err.Error(), "only valid") {
		t.Fatalf("input base error = %v", err)
	}
}

func TestReadInputRejectsOversizedDocument(t *testing.T) {
	if _, err := readInput(strings.NewReader(strings.Repeat("x", inputMaxBytes+1)), "test input"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized input error = %v", err)
	}
}

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
