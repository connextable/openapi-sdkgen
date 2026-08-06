package typescript

import (
	"strings"
	"testing"

	sdkgen "openapi-sdkgen/internal/compiler"
)

func TestResourceArtifactsComposeCompletedCallsWithoutRebindingCapabilities(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi":"3.1.0",
  "info":{"title":"Resource modules","version":"1"},
  "paths":{
    "/teams":{"get":{"operationId":"listTeams","responses":{"204":{"description":"OK"}}}},
    "/teams/{teamId}/users":{"parameters":[{"name":"teamId","in":"path","required":true,"schema":{"$ref":"#/components/schemas/TenantID"}}],"get":{"operationId":"listUsers","responses":{"204":{"description":"OK"}}}},
    "/teams/{teamId}/users/{userId}":{"parameters":[{"name":"teamId","in":"path","required":true,"schema":{"$ref":"#/components/schemas/TenantID"}},{"name":"userId","in":"path","required":true,"schema":{"type":"string"}}],"get":{"operationId":"getUser","responses":{"204":{"description":"OK"}}}}
  },
  "components":{"schemas":{"TenantID":{"type":"string"}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	prepared, _, err := prepareSourcePlan(document, false)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	byPath := make(map[string]string, len(artifacts))
	for _, artifact := range artifacts {
		byPath[artifact.Path] = string(artifact.Data)
	}
	for _, module := range prepared.modules.resources {
		source, exists := byPath[module.path]
		if !exists {
			t.Fatalf("missing planned resource module %s (%s)", module.identity, module.path)
		}
		for _, forbidden := range []string{"bindOperation", "bindPagination", "bindLinks", "bindStream", "createPaginator", "createRequest"} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("resource module %s recreates %q:\n%s", module.path, forbidden, source)
			}
		}
	}
	teamSelector := byPath[resourcePathForIdentity(t, prepared.modules, "literal:teams")]
	if !strings.Contains(teamSelector, `schemas/tenant-id.js`) || strings.Contains(teamSelector, "Contract.") {
		t.Fatalf("resource selector does not reference its schema owner directly:\n%s", teamSelector)
	}
	users := byPath[resourcePathForIdentity(t, prepared.modules, "literal:teams/{teamId}/users")]
	if !strings.Contains(users, "bindPathOperation<import(") || !strings.Contains(users, `registry.routes["GET /teams/{teamId}/users"]`) {
		t.Fatalf("path-bound resource does not wrap the completed exact call:\n%s", users)
	}
	if _, exists := byPath["internal/client.ts"]; exists {
		t.Fatal("legacy internal/client.ts was emitted")
	}
}

func TestClientFactoryCreatesOneRegistryThenDelegatesResourceComposition(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{"openapi":"3.1.0","info":{"title":"Client composition","version":"1"},"paths":{"/users":{"get":{"operationId":"listUsers","responses":{"204":{"description":"OK"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	factory := string(artifactByPath(t, artifacts, "internal/client/factory.ts"))
	for _, expected := range []string{
		"const request = createRequest(options)",
		"const registry = createCallableRegistry(request)",
		"const resources = buildResources(registry)",
		"$routes: registry.routes",
		"$operations: registry.operations",
		"...resources",
	} {
		if !strings.Contains(factory, expected) {
			t.Fatalf("client factory missing %q:\n%s", expected, factory)
		}
	}
	if strings.Count(factory, "createCallableRegistry(") != 1 || strings.Contains(factory, "bindOperation") {
		t.Fatalf("client factory recreated operation state:\n%s", factory)
	}
}

func resourcePathForIdentity(t *testing.T, plan *semanticModulePlan, identity string) string {
	t.Helper()
	for _, module := range plan.resources {
		if module.identity == identity {
			return module.path
		}
	}
	t.Fatalf("missing resource identity %s", identity)
	return ""
}
