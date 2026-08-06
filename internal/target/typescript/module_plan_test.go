package typescript

import (
	"strings"
	"testing"

	"openapi-sdkgen/internal/compiler/ir"
)

func TestSemanticModulePlanSeparatesSchemaOperationAndResourceOwners(t *testing.T) {
	t.Parallel()
	getUser := pathOperation("getUser", "GET", "/users/{userId}", "userId", map[string]any{"type": "string"})
	getUser.RouteKey = "GET /users/{userId}"
	getUser.Visibility = "public"
	document := &ir.Document{
		ComponentSchemas: map[string]map[string]any{
			"User":  {"type": "object"},
			"index": {"type": "string"},
		},
		Raw: map[string]any{},
		Operations: []ir.Operation{
			getUser,
			{RouteKey: "POST /users", OperationID: "createUser", Method: "POST", Path: "/users", Visibility: "internal"},
			{RouteKey: "DELETE /users/{userId}", OperationID: "deleteUser", Method: "DELETE", Path: "/users/{userId}", Visibility: "hidden"},
		},
	}
	manifest := Manifest{Operations: []ManifestOperation{
		{RouteKey: "GET /users/{userId}", OperationID: "getUser", Method: "GET", Path: "/users/{userId}", Visibility: "public"},
		{RouteKey: "POST /users", OperationID: "createUser", Method: "POST", Path: "/users", Visibility: "internal"},
		{RouteKey: "DELETE /users/{userId}", OperationID: "deleteUser", Method: "DELETE", Path: "/users/{userId}", Visibility: "hidden"},
	}}
	plan, err := buildSemanticModulePlan(document, manifest, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.schemas) != 2 || plan.schemaByName["User"] != "internal/schemas/user.ts" {
		t.Fatalf("schema plan = %#v", plan.schemas)
	}
	if plan.schemaByName["index"] == "internal/schemas/index.ts" {
		t.Fatalf("schema leaf replaced the schema registry: %#v", plan.schemaByName)
	}
	if len(plan.operations) != 2 {
		t.Fatalf("operation plan = %#v, want public and internal only", plan.operations)
	}
	if got := plan.operationByRoute["GET /users/{userId}"]; got != "internal/operations/users/by-user-id/get.ts" {
		t.Fatalf("GET path = %q", got)
	}
	if _, exists := plan.operationByRoute["DELETE /users/{userId}"]; exists {
		t.Fatalf("hidden operation received a module: %#v", plan.operationByRoute)
	}
	hasRoot := false
	for _, resource := range plan.resources {
		hasRoot = hasRoot || resource.identity == "root"
	}
	if !hasRoot {
		t.Fatalf("resource plan = %#v", plan.resources)
	}
}

func TestOperationModulePathDoesNotDependOnOperationID(t *testing.T) {
	t.Parallel()
	plan := &semanticModulePlan{operationByRoute: make(map[string]string)}
	before := Manifest{Operations: []ManifestOperation{{RouteKey: "GET /users", OperationID: "before", Method: "GET", Path: "/users", Visibility: "public"}}}
	if err := plan.planOperations(before); err != nil {
		t.Fatal(err)
	}
	first := plan.operationByRoute["GET /users"]
	plan.operations = nil
	plan.operationByRoute = make(map[string]string)
	after := Manifest{Operations: []ManifestOperation{{RouteKey: "GET /users", OperationID: "after", Method: "GET", Path: "/users", Visibility: "public"}}}
	if err := plan.planOperations(after); err != nil {
		t.Fatal(err)
	}
	if second := plan.operationByRoute["GET /users"]; second != first {
		t.Fatalf("operationId rename moved artifact: %q -> %q", first, second)
	}
}

func TestPlanTypeReferencesUsesInlineThenSharedAlias(t *testing.T) {
	t.Parallel()
	uses := []typeReferenceUse{
		{key: "single", modulePath: "internal/schemas/user.ts", exportName: "Output"},
		{key: "repeat-a", modulePath: "internal/schemas/problem.ts", exportName: "Output"},
		{key: "repeat-b", modulePath: "internal/schemas/problem.ts", exportName: "Output"},
		{key: "bound", modulePath: "internal/schemas/input.ts", exportName: "Input", requiresBinding: true},
	}
	planned, err := planTypeReferences("internal/operations/users/get.ts", uses)
	if err != nil {
		t.Fatal(err)
	}
	byKey := make(map[string]plannedTypeReference, len(planned))
	for _, item := range planned {
		byKey[item.key] = item
	}
	if !byKey["single"].inline || byKey["single"].specifier != "../../schemas/user.js" {
		t.Fatalf("single reference = %#v", byKey["single"])
	}
	if byKey["repeat-a"].inline || byKey["repeat-a"].alias == "" || byKey["repeat-a"].alias != byKey["repeat-b"].alias {
		t.Fatalf("repeated references = %#v %#v", byKey["repeat-a"], byKey["repeat-b"])
	}
	if byKey["bound"].inline || byKey["bound"].alias == "" {
		t.Fatalf("bound reference = %#v", byKey["bound"])
	}
}

func TestValidateSemanticImportDirectionRejectsRuntimeAndOperationBackEdges(t *testing.T) {
	t.Parallel()
	for _, edge := range []moduleImportEdge{
		{from: "internal/runtime/http.ts", to: "internal/schemas/user.ts"},
		{from: "internal/operations/users/get.ts", to: "internal/client/registry.ts"},
	} {
		if err := validateSemanticImportDirection([]moduleImportEdge{edge}); err == nil {
			t.Fatalf("edge was accepted: %#v", edge)
		}
	}
	if err := validateSemanticImportDirection([]moduleImportEdge{{from: "internal/operations/users/get.ts", to: "internal/runtime/http.ts"}}); err != nil {
		t.Fatalf("valid semantic edge rejected: %v", err)
	}
}

func TestValidateGeneratedArtifactsChecksHeaderAndPortableUniqueness(t *testing.T) {
	t.Parallel()
	valid := []Artifact{{Path: "internal/user.ts", Data: generatedSource([]byte("export {}\n"))}}
	if err := validateGeneratedArtifacts(valid); err != nil {
		t.Fatal(err)
	}
	withoutHeader := []Artifact{{Path: "internal/user.ts", Data: []byte("export {}\n")}}
	if err := validateGeneratedArtifacts(withoutHeader); err == nil || !strings.Contains(err.Error(), "header") {
		t.Fatalf("missing-header error = %v", err)
	}
	duplicate := append(valid, Artifact{Path: "INTERNAL/USER.ts", Data: generatedSource(nil)})
	if err := validateGeneratedArtifacts(duplicate); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("duplicate error = %v", err)
	}
}
