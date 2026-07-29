package sdkgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
)

func TestReservedExtensionScanDistinguishesKeywordsFromExactDataNames(t *testing.T) {
	values, err := reservedExtensionDiagnostics([]byte(`{
  "openapi": "3.1.0",
  "x-sdkgen-root": true,
  "info": {"title": "Reserved", "version": "1"},
  "paths": {"/items": {"get": {
    "x-sdkgen-operation": true,
    "responses": {"200": {"description": "OK"}}
  }}},
  "components": {
    "schemas": {
      "x-sdkgen-component": {
        "type": "object",
        "properties": {"x-sdkgen-property": {"type": "string"}}
      }
    },
    "headers": {"x-sdkgen-header": {"schema": {"type": "string"}}}
  }
}`), "contract.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("diagnostics = %#v", values)
	}
	if values[0].Code != "SDKGEN-E160" || values[1].Code != "SDKGEN-E160" {
		t.Fatalf("diagnostics = %#v", values)
	}
}

func TestRemoteReferenceScanRetainsInternalSourceIdentity(t *testing.T) {
	collector := &diagnostic.Collector{}
	resolver := &remoteReferenceResolver{diagnostics: collector}
	err := resolver.scanRemoteSource(
		[]byte("type: object\nx-sdkgen-remote: true\n"),
		"https://user:secret@example.test/schema.yaml?token=value#fragment",
	)
	if err == nil {
		t.Fatal("reserved remote extension was accepted")
	}
	values := collector.Diagnostics()
	if len(values) != 1 || values[0].Location.Source != "https://user:secret@example.test/schema.yaml?token=value#fragment" {
		t.Fatalf("diagnostics = %#v", values)
	}
	displayed := displayDiagnosticSources(values)
	if displayed[0].Location.Source != "https://example.test/schema.yaml" {
		t.Fatalf("displayed diagnostics = %#v", displayed)
	}
	for _, secret := range []string{"user", "secret", "token", "value", "fragment"} {
		if strings.Contains(displayed[0].Location.Source, secret) {
			t.Fatalf("source leaked %q: %s", secret, displayed[0].Location.Source)
		}
	}
}

func TestDiagnosticSourceDisplayDisambiguatesSanitizedCollisions(t *testing.T) {
	values := []diagnostic.Diagnostic{
		{Location: diagnostic.Location{Source: "https://first:secret@example.test/schema.yaml?token=one"}},
		{Location: diagnostic.Location{Source: "https://second:secret@example.test/schema.yaml?token=two"}},
	}
	displayed := displayDiagnosticSources(values)
	if displayed[0].Location.Source == displayed[1].Location.Source {
		t.Fatalf("sanitized source collision was not disambiguated: %#v", displayed)
	}
	for _, value := range displayed {
		if !strings.Contains(value.Location.Source, "https://example.test/schema.yaml [source ") {
			t.Fatalf("unexpected source display: %q", value.Location.Source)
		}
	}
}

func TestLocalReferenceScanRetainsReferencedSourceAndPointer(t *testing.T) {
	directory := t.TempDir()
	root := filepath.Join(directory, "openapi.yaml")
	reference := filepath.Join(directory, "schema.yaml")
	if err := os.WriteFile(root, []byte("openapi: 3.1.0\ninfo: {title: Scan, version: '1'}\npaths: {}\ncomponents:\n  schemas:\n    Thing: {$ref: schema.yaml}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reference, []byte("type: object\nx-sdkgen-private: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := loadFileInput(root)
	if err != nil {
		t.Fatal(err)
	}
	collector := &diagnostic.Collector{}
	if err := scanLocalReferenceDocuments(source, collector); err != nil {
		t.Fatal(err)
	}
	values := collector.Diagnostics()
	resolvedReference, err := filepath.EvalSymlinks(reference)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].Location.Source != resolvedReference || values[0].Location.Pointer != "#/x-sdkgen-private" {
		t.Fatalf("diagnostics = %#v", values)
	}
}
