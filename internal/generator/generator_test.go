package generator

import (
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

type testTarget string

func (target testTarget) Name() string { return string(target) }

func (testTarget) Generate(*ir.Document, Options) ([]Artifact, error) { return nil, nil }

func TestRegistryLooksUpTargetsInStableOrder(t *testing.T) {
	registry, err := NewRegistry(testTarget("typescript"), testTarget("swift"))
	if err != nil {
		t.Fatal(err)
	}
	if names := registry.Names(); strings.Join(names, ",") != "swift,typescript" {
		t.Fatalf("names = %v", names)
	}
	if target, err := registry.Lookup("typescript"); err != nil || target.Name() != "typescript" {
		t.Fatalf("lookup = %v, %v", target, err)
	}
}

func TestRegistryRejectsDuplicateAndUnknownTargets(t *testing.T) {
	if _, err := NewRegistry(testTarget("typescript"), testTarget("typescript")); err == nil {
		t.Fatal("duplicate target was accepted")
	}
	registry, err := NewRegistry(testTarget("typescript"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Lookup("kotlin"); err == nil || !strings.Contains(err.Error(), "available: typescript") {
		t.Fatalf("lookup error = %v", err)
	}
}
