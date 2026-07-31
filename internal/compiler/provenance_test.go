package sdkgen

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openapi-sdkgen/internal/compiler/ir"
)

func TestCompileFileCarriesRootAndReferenceProvenance(t *testing.T) {
	directory := t.TempDir()
	root := filepath.Join(directory, "openapi.yaml")
	schema := filepath.Join(directory, "schema.yaml")
	if err := os.WriteFile(root, []byte(`openapi: 3.1.0
info: {title: Provenance, version: "1"}
paths:
  /things:
    get:
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema: {$ref: schema.yaml#/Thing}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(schema, []byte("Thing: {type: string}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := CompileFile(root)
	if err != nil {
		t.Fatal(err)
	}
	rootLocation := document.Provenance["#/paths/~1things/get"]
	if rootLocation.Primary.Source != root {
		t.Fatalf("root provenance = %#v", rootLocation)
	}
	reference := document.Provenance["#/paths/~1things/get/responses/200/content/application~1json/schema"]
	resolvedSchema, err := filepath.EvalSymlinks(schema)
	if err != nil {
		t.Fatal(err)
	}
	if reference.Primary.Source != resolvedSchema || reference.Primary.Pointer != "#/Thing" || len(reference.Related) != 1 {
		t.Fatalf("reference provenance = %#v", reference)
	}
}

func TestCompileFileCarriesReferenceProvenanceThroughDescendants(t *testing.T) {
	directory := t.TempDir()
	root := filepath.Join(directory, "openapi.yaml")
	schema := filepath.Join(directory, "schema.yaml")
	if err := os.WriteFile(root, []byte(`openapi: 3.1.0
info: {title: Provenance, version: "1"}
paths:
  /things:
    get:
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema: {$ref: schema.yaml#/Thing}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(schema, []byte(`Thing:
  type: object
  properties:
    value:
      type: string
      x-envelope: data
`), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := CompileFile(root)
	if err != nil {
		t.Fatal(err)
	}
	pointer := "#/components/schemas/Thing/properties/value/x-envelope"
	value := document.Provenance[pointer]
	resolvedSchema, err := filepath.EvalSymlinks(schema)
	if err != nil {
		t.Fatal(err)
	}
	if value.Primary.Source != resolvedSchema || value.Primary.Pointer != "#/Thing/properties/value/x-envelope" {
		t.Fatalf("descendant provenance = %#v", value)
	}
	if len(value.Related) != 1 || value.Related[0].Source != root {
		t.Fatalf("descendant related provenance = %#v", value.Related)
	}
}

func TestCompileFileCarriesNestedReferenceProvenance(t *testing.T) {
	directory := t.TempDir()
	root := filepath.Join(directory, "openapi.yaml")
	first := filepath.Join(directory, "first.yaml")
	second := filepath.Join(directory, "second.yaml")
	if err := os.WriteFile(root, []byte(`openapi: 3.1.0
info: {title: Nested provenance, version: "1"}
paths:
  /things:
    get:
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema: {$ref: first.yaml#/Thing}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte(`Thing:
  type: object
  properties:
    detail: {$ref: second.yaml#/Detail}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(`Detail:
  type: string
  x-envelope: data
`), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := CompileFile(root)
	if err != nil {
		t.Fatal(err)
	}
	resolvedSecond, err := filepath.EvalSymlinks(second)
	if err != nil {
		t.Fatal(err)
	}
	var found ir.Provenance
	for pointer, value := range document.Provenance {
		if strings.HasSuffix(pointer, "/x-envelope") && value.Primary.Source == resolvedSecond {
			found = value
			break
		}
	}
	if found.Primary.Source != resolvedSecond || found.Primary.Pointer != "#/Detail/x-envelope" {
		t.Fatalf("nested provenance = %#v", found)
	}
	resolvedFirst, err := filepath.EvalSymlinks(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Related) != 1 || found.Related[0].Source != resolvedFirst || found.Related[0].Pointer != "#/Thing/properties/detail/$ref" {
		t.Fatalf("nested related provenance = %#v", found.Related)
	}
}

func TestReferenceProvenanceTraversesNestedRemoteSources(t *testing.T) {
	const first = "https://schemas.example.test/first.yaml?revision=1"
	const second = "https://schemas.example.test/second.yaml?revision=2"
	root := []byte(`openapi: 3.1.0
info: {title: Remote provenance, version: "1"}
paths: {}
components:
  schemas:
    Root: {$ref: "` + first + `#/Thing"}
`)
	sources := map[string][]byte{
		first: []byte(`Thing:
  type: object
  properties:
    detail: {$ref: "` + second + `#/Detail"}
`),
		second: []byte(`Detail:
  type: string
  x-envelope: data
`),
	}
	values := referenceProvenance(root, "https://api.example.test/openapi.yaml", "https://api.example.test/openapi.yaml", "", sources)
	pointer := "#/components/schemas/Root/properties/detail"
	value := values[pointer]
	if value.Primary.Source != second || value.Primary.Pointer != "#/Detail" {
		t.Fatalf("remote nested provenance = %#v", value)
	}
	if len(value.Related) != 1 || value.Related[0].Source != first || value.Related[0].Pointer != "#/Thing/properties/detail/$ref" {
		t.Fatalf("remote nested related provenance = %#v", value.Related)
	}
}

func TestReferenceProvenanceUsesRemoteInputBaseForStandardInput(t *testing.T) {
	const base = "https://api.example.test/contracts/openapi.yaml"
	const first = "https://api.example.test/contracts/schemas/first.yaml"
	root := []byte(`components:
  schemas:
    Root: {$ref: "schemas/first.yaml#/Thing"}
`)
	sources := map[string][]byte{
		first: []byte(`Thing: {type: string}`),
	}
	values := referenceProvenance(root, "standard input", base, "", sources)
	value := values["#/components/schemas/Root"]
	if value.Primary.Source != first || value.Primary.Pointer != "#/Thing" {
		t.Fatalf("stdin remote provenance = %#v", value)
	}
	if len(value.Related) != 1 || value.Related[0].Source != "standard input" {
		t.Fatalf("stdin related provenance = %#v", value.Related)
	}
}

func TestReferenceProvenanceResolvesRelativeRefsFromMixedCaseHTTPS(t *testing.T) {
	base, err := url.Parse("HTTPS://schemas.example.test/contracts/first.yaml")
	if err != nil {
		t.Fatal(err)
	}
	target := base.ResolveReference(&url.URL{Path: "second.yaml"}).String()
	root := []byte(`Root: {$ref: "second.yaml#/Thing"}`)
	sources := map[string][]byte{target: []byte(`Thing: {type: string}`)}
	values := referenceProvenance(root, base.String(), base.String(), "", sources)
	value := values["#/Root"]
	if value.Primary.Source != target || value.Primary.Pointer != "#/Thing" {
		t.Fatalf("mixed-case remote provenance = %#v", value)
	}
}
