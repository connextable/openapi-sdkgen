package sdkgen

import (
	"os"
	"path/filepath"
	"testing"
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
