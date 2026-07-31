package typescript

import (
	"bytes"
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func TestSourceArtifactsPreservesNormalizationEquivalentOperationIDs(t *testing.T) {
	document := &ir.Document{ContractVersion: "1.0.0", Operations: []ir.Operation{
		{OperationID: "get-pet", Method: "GET", Path: "/pets/modern"},
		{OperationID: "get_pet", Method: "GET", Path: "/pets/legacy"},
	}}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	source := string(artifactByPath(t, artifacts, "generated/client.ts"))
	for operationID, routeKey := range map[string]string{"get-pet": "GET /pets/modern", "get_pet": "GET /pets/legacy"} {
		if !strings.Contains(source, "readonly "+quoteTS(operationID)+": Routes["+quoteTS(routeKey)+"]") ||
			!strings.Contains(source, `["`+operationID+`", __sdkgen_`) {
			t.Fatalf("exact operation %q missing:\n%s", operationID, source)
		}
	}
}

func TestSourceArtifactsAllowsMissingOperationIDAndRejectsDuplicateExactIDs(t *testing.T) {
	artifacts, err := SourceArtifacts(&ir.Document{Operations: []ir.Operation{{Method: "GET", Path: "/missing"}}})
	if err != nil {
		t.Fatal(err)
	}
	source := string(artifactByPath(t, artifacts, "generated/client.ts"))
	if !strings.Contains(source, `readonly "GET /missing": Routes["GET /missing"]["call"]`) || strings.Contains(source, `readonly "":`) {
		t.Fatalf("route-only operation surface missing:\n%s", source)
	}
	if strings.Contains(source, "Operation ID: ``") {
		t.Fatalf("missing operation ID rendered as an empty identity:\n%s", source)
	}
	_, err = SourceArtifacts(&ir.Document{Operations: []ir.Operation{
		{OperationID: "same", Method: "GET", Path: "/one"},
		{OperationID: "same", Method: "POST", Path: "/two"},
	}})
	if err == nil || !strings.Contains(err.Error(), "operationId") {
		t.Fatalf("error = %v", err)
	}
}

func TestIDLessOperationParameterBindingsUseRouteIdentity(t *testing.T) {
	first, err := operationParameters(&ir.Document{}, ir.Operation{Method: "GET", Path: "/first", Raw: map[string]any{
		"parameters": []any{map[string]any{"name": "query", "in": "query", "schema": map[string]any{"type": "string"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := operationParameters(&ir.Document{}, ir.Operation{Method: "GET", Path: "/second", Raw: map[string]any{
		"parameters": []any{map[string]any{"name": "query", "in": "query", "schema": map[string]any{"type": "string"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if first[0].Binding == second[0].Binding {
		t.Fatalf("ID-less operation bindings collided: %q", first[0].Binding)
	}
}

func TestSourceArtifactsAllowsMissingOperationIDForEveryVisibility(t *testing.T) {
	if _, err := SourceArtifacts(&ir.Document{Operations: []ir.Operation{
		{Method: "GET", Path: "/hidden", Visibility: "hidden"},
		{OperationID: "visible", Method: "GET", Path: "/visible", Visibility: "public"},
	}}); err != nil {
		t.Fatalf("hidden operation without operationId failed: %v", err)
	}
	_, err := SourceArtifacts(&ir.Document{Operations: []ir.Operation{
		{OperationID: "same", Method: "GET", Path: "/hidden", Visibility: "hidden"},
		{OperationID: "same", Method: "GET", Path: "/visible", Visibility: "public"},
	}})
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate hidden operationId error = %v", err)
	}
}

func TestEmitTypesPreservesNormalizedAndProjectionComponentNameCollisions(t *testing.T) {
	for name, schema := range map[string]map[string]map[string]any{
		"normalized": {
			"foo-bar": map[string]any{"type": "string"},
			"foo_bar": map[string]any{"type": "string"},
		},
		"projection": {
			"Product":      map[string]any{"type": "object", "properties": map[string]any{}},
			"ProductInput": map[string]any{"type": "object", "properties": map[string]any{}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			document := &ir.Document{ComponentSchemas: schema}
			source, err := emitTypes(document)
			if err != nil {
				t.Fatal(err)
			}
			for component := range schema {
				if !strings.Contains(string(source), "readonly "+quoteTS(component)+": {") {
					t.Fatalf("component %q missing from exact-key catalog:\n%s", component, source)
				}
			}
			if strings.Contains(string(source), "export type ProductInput =") || strings.Contains(string(source), "export type FooBar =") {
				t.Fatalf("flat normalized component alias leaked:\n%s", source)
			}
		})
	}
}

func TestObjectTypePreservesNormalizationEquivalentProperties(t *testing.T) {
	value, err := objectType(&ir.Document{}, map[string]any{
		"properties": map[string]any{
			"foo-bar": map[string]any{"type": "string"},
			"foo_bar": map[string]any{"type": "string"},
		},
	}, projectionInput)
	if err != nil || !strings.Contains(value, `readonly "foo-bar"?: string | undefined`) || !strings.Contains(value, `readonly "foo_bar"?: string | undefined`) {
		t.Fatalf("type = %q, error = %v", value, err)
	}
}

func TestOperationParametersPreserveNormalizationEquivalentNames(t *testing.T) {
	parameters, err := operationParameters(&ir.Document{}, ir.Operation{OperationID: "search", Raw: map[string]any{
		"parameters": []any{
			map[string]any{"name": "x-id", "in": "query", "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "x_id", "in": "query", "schema": map[string]any{"type": "string"}},
		},
	}})
	if err != nil || len(parameters) != 2 || parameters[0].Property != parameters[0].Name || parameters[1].Property != parameters[1].Name || parameters[0].Binding == parameters[1].Binding {
		t.Fatalf("parameters = %#v, error = %v", parameters, err)
	}
}

func TestOperationParametersKeepSameExactNameSeparateByLocation(t *testing.T) {
	raw := make([]any, 0, 5)
	for _, location := range []string{"path", "query", "querystring", "header", "cookie"} {
		raw = append(raw, map[string]any{
			"name":     "id",
			"in":       location,
			"required": location == "path",
			"schema":   map[string]any{"type": "string"},
		})
	}
	parameters, err := operationParameters(&ir.Document{}, ir.Operation{
		OperationID:        "getItem",
		PathParameterOrder: []string{"id"},
		Raw:                map[string]any{"parameters": raw},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(parameters) != 5 {
		t.Fatalf("parameter count = %d, want 5", len(parameters))
	}
	bindings := map[string]bool{}
	locations := map[string]bool{}
	for _, parameter := range parameters {
		if parameter.Name != "id" || parameter.Property != "id" {
			t.Fatalf("parameter identity changed: %#v", parameter)
		}
		if bindings[parameter.Binding] {
			t.Fatalf("private binding %q was reused across locations", parameter.Binding)
		}
		bindings[parameter.Binding] = true
		locations[parameter.Location] = true
	}
	if len(locations) != 5 {
		t.Fatalf("locations = %#v", locations)
	}
}

func TestManifestExamplesUsePlannedBindingsForNormalizationEquivalentPathKeys(t *testing.T) {
	operation := ir.Operation{
		OperationID:        "getPair",
		Method:             "GET",
		Path:               "/pairs/{foo-bar}/{foo_bar}",
		PathParameterOrder: []string{"foo-bar", "foo_bar"},
		Raw: map[string]any{"parameters": []any{
			map[string]any{"name": "foo-bar", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "foo_bar", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
		}},
	}
	document := &ir.Document{Operations: []ir.Operation{operation}}
	parameters, err := operationParameters(document, operation)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	call := manifest.Operations[0].CallExpression
	if len(parameters) != 2 || parameters[0].Binding == parameters[1].Binding ||
		!strings.Contains(call, parameters[0].Binding) || !strings.Contains(call, parameters[1].Binding) {
		t.Fatalf("parameters = %#v, call = %q", parameters, call)
	}
}

func TestResourceSelectorsUseReadableLocallyUniquePathBindings(t *testing.T) {
	names := []string{"foo-bar", "foo_bar", "default", "123-id", "한글", "$"}
	parameters := make([]any, 0, len(names))
	for _, name := range names {
		parameters = append(parameters, map[string]any{
			"name": name, "in": "path", "required": true, "schema": map[string]any{"type": "string"},
		})
	}
	operation := ir.Operation{
		OperationID:        "getSelectorTest",
		Method:             "GET",
		Path:               "/selectors/{foo-bar}/{foo_bar}/{default}/{123-id}/{한글}/{$}",
		PathParameterOrder: names,
		Raw:                map[string]any{"parameters": parameters},
	}
	document := &ir.Document{Operations: []ir.Operation{operation}}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	wantCall := "api.selectors(fooBar)(fooBar2)(defaultValue)(value123ID)(한글)(pathParameter).get()"
	if call := manifest.Operations[0].CallExpression; call != wantCall {
		t.Fatalf("call expression = %q, want %q", call, wantCall)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	source := string(artifactByPath(t, artifacts, "generated/client.ts"))
	for _, expected := range []string{
		"(fooBar: string):", "(fooBar2: string):", "(defaultValue: string):",
		"(value123ID: string):", "(한글: string):", "(pathParameter: string):",
		`"foo-bar": fooBar`, `"foo_bar": fooBar2`, `"default": defaultValue`,
		`"123-id": value123ID`, `"한글": 한글`, `"$": pathParameter`,
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("readable selector source missing %q:\n%s", expected, source)
		}
	}
}

func TestOperationParametersFollowURIPathParameterOrder(t *testing.T) {
	parameters, err := operationParameters(&ir.Document{}, ir.Operation{
		PathParameterOrder: []string{"customerID", "widgetID"},
		Raw: map[string]any{"parameters": []any{
			map[string]any{"name": "widgetID", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "customerID", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(parameters) != 2 || parameters[0].Name != "customerID" || parameters[1].Name != "widgetID" {
		t.Fatalf("path parameters = %#v", parameters)
	}
}

func TestRequestBodyTypeUsesRuntimeBinaryBody(t *testing.T) {
	value, err := requestBodyType(&ir.Document{}, map[string]any{"content": map[string]any{
		"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}},
	}})
	if err != nil || value != "BinaryBody" {
		t.Fatalf("request body type = %q, %v", value, err)
	}
}

func TestEmitTypesPreservesCollidingEnumValuesAsLiterals(t *testing.T) {
	source, err := emitTypes(&ir.Document{
		ComponentSchemas: map[string]map[string]any{
			"Status": {"type": "string", "enum": []any{"foo-bar", "foo_bar"}},
		},
		Operations: []ir.Operation{{Raw: map[string]any{
			"responses": map[string]any{
				"200": map[string]any{
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/Status"},
						},
					},
				},
			},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), `"foo-bar" | "foo_bar"`) {
		t.Fatalf("enum values missing:\n%s", source)
	}
	if !strings.Contains(string(source), `["Status", __sdkgen_status_`) ||
		!strings.Contains(string(source), `readonly "Status": typeof __sdkgen_status_`) {
		t.Fatalf("exact enum catalog entry missing:\n%s", source)
	}
	if strings.Contains(string(source), "FOO_BAR:") {
		t.Fatalf("normalized enum member leaked:\n%s", source)
	}
}

func TestBuildResourceTreeComposesOperationAndChildNamespace(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
		{OperationID: "listUsers", Method: "GET", Path: "/users"},
		{OperationID: "getList", Method: "GET", Path: "/users/list"},
	}}
	manifest := Manifest{Operations: []ManifestOperation{
		{OperationID: "listUsers", Method: "GET", Path: "/users", Visibility: "public"},
		{OperationID: "getList", Method: "GET", Path: "/users/list", Visibility: "public"},
	}}
	tree, err := buildResourceTree(document, manifest)
	if err != nil {
		t.Fatal(err)
	}
	users := tree.children["users"]
	if users == nil || users.operations["list"].OperationID != "listUsers" || users.children["list"].operations["get"].OperationID != "getList" {
		t.Fatalf("resource tree did not retain the composable collision: %#v", users)
	}
	built, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	calls := manifestCalls(built)
	if calls["listUsers"] != "api.users.list()" || calls["getList"] != "api.users.list.get()" {
		t.Fatalf("calls = %#v", calls)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	source := string(artifactByPath(t, artifacts, "generated/client.ts"))
	if !strings.Contains(source, `readonly list: ResourceCall<"GET /users"> & {`) || !strings.Contains(source, "list: assignCallableProperties(") {
		t.Fatalf("callable namespace was not emitted:\n%s", source)
	}
}

func TestBuildResourceTreePrunesEmptyBranchesAfterParameterCollision(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
		pathOperation("getProfile", "GET", "/users/{id}/profile", "id", map[string]any{"type": "string"}),
		pathOperation("getSettings", "GET", "/users/{userID}/settings", "userID", map[string]any{"type": "integer"}),
	}}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := buildResourceTree(document, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := tree.children["users"]; exists {
		t.Fatalf("empty users resource was retained: %#v", tree.children["users"])
	}
	for _, operation := range manifest.Operations {
		if operation.ResourceSegments != nil {
			t.Fatalf("operation %q retained resource segments %#v", operation.OperationID, operation.ResourceSegments)
		}
	}
}

func TestTemplatedResourcePathValidationPreservesRawPathShape(t *testing.T) {
	if err := validateTemplatedResourcePaths(&ir.Document{Raw: map[string]any{"paths": map[string]any{
		"/users/{id}":    map[string]any{},
		"/users/{name}/": map[string]any{},
	}}}); err != nil {
		t.Fatalf("trailing-slash-distinct paths were rejected: %v", err)
	}
	err := validateTemplatedResourcePaths(&ir.Document{Raw: map[string]any{"paths": map[string]any{
		"/files/{id}.json":   map[string]any{},
		"/files/{name}.json": map[string]any{},
	}}})
	if err == nil || !strings.Contains(err.Error(), "identical templated shape") {
		t.Fatalf("embedded-template collision error = %v", err)
	}
	if err := validateTemplatedResourcePaths(&ir.Document{Raw: map[string]any{"paths": map[string]any{
		"x-{id}":   map[string]any{},
		"x-{name}": map[string]any{},
	}}}); err != nil {
		t.Fatalf("templated Paths extensions were treated as URL paths: %v", err)
	}
}

func TestEmbeddedPathTemplateFallsBackToExactOperationCatalog(t *testing.T) {
	operation := pathOperation("getJSONFile", "GET", "/files/{id}.json", "id", map[string]any{"type": "string"})
	document := &ir.Document{
		Raw:        map[string]any{"paths": map[string]any{operation.Path: map[string]any{}}},
		Operations: []ir.Operation{operation},
	}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Operations) != 1 || manifest.Operations[0].ResourceSegments != nil ||
		!strings.HasPrefix(manifest.Operations[0].CallExpression, `api.$operations["getJSONFile"]`) {
		t.Fatalf("embedded-template manifest = %#v", manifest.Operations)
	}
	if _, err := SourceArtifacts(document); err != nil {
		t.Fatalf("embedded-template exact fallback failed generation: %v", err)
	}
}

func TestRepeatedPathParameterFallsBackToExactOperationCatalog(t *testing.T) {
	operation := pathOperation("getAlias", "GET", "/users/{id}/aliases/{id}", "id", map[string]any{"type": "string"})
	operation.PathParameterOrder = []string{"id", "id"}
	document := &ir.Document{Operations: []ir.Operation{
		operation,
		{OperationID: "listUsers", Method: "GET", Path: "/users"},
	}}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	calls := manifestCalls(manifest)
	if !strings.HasPrefix(calls["getAlias"], `api.$operations["getAlias"]`) || calls["listUsers"] != "api.users.list()" {
		t.Fatalf("calls = %#v", calls)
	}
	if strings.Count(calls["getAlias"], `"id":`) != 1 {
		t.Fatalf("repeated parameter call contains duplicate input keys: %q", calls["getAlias"])
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	client := string(artifactByPath(t, artifacts, "generated/client.ts"))
	start := strings.Index(client, `readonly "GET /users/{id}/aliases/{id}": {`)
	if start < 0 || !strings.Contains(client[start:], "readonly resourceCall: never") {
		t.Fatalf("repeated path parameter retained a resource call:\n%s", client)
	}
}

func TestBuildResourceTreeOmitsOperationShortcutBeforeCallableParameterChild(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
		{OperationID: "listUsers", Method: "GET", Path: "/users"},
		pathOperation("getListedUser", "GET", "/users/list/{id}", "id", map[string]any{"type": "string"}),
	}}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	calls := manifestCalls(manifest)
	if calls["listUsers"] != `api.$operations["listUsers"]()` || calls["getListedUser"] != "api.users.list(id).get()" {
		t.Fatalf("calls = %#v", calls)
	}
	tree, err := buildResourceTree(document, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := tree.children["users"].operations["list"]; exists || tree.children["users"].children["list"].parameterChild == nil {
		t.Fatalf("callable parameter branch collision was not resolved: %#v", tree.children["users"])
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	source := string(artifactByPath(t, artifacts, "generated/client.ts"))
	start := strings.Index(source, `readonly "GET /users": {`)
	if start < 0 {
		t.Fatalf("listUsers catalog entry missing:\n%s", source)
	}
	entry := source[start:]
	if end := strings.Index(entry, "\n  }"); end >= 0 {
		entry = entry[:end]
	}
	if !strings.Contains(entry, "readonly resourceCall: never") {
		t.Fatalf("omitted resource shortcut remained callable:\n%s", entry)
	}
}

func TestBuildResourceTreeOmitsNormalizationEquivalentLiteralSiblings(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
		{OperationID: "getModern", Method: "GET", Path: "/foo-bar"},
		{OperationID: "getLegacy", Method: "GET", Path: "/foo_bar"},
	}}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(buildResourceSegments(manifest)) != 0 {
		t.Fatalf("normalization-equivalent siblings retained a resource shortcut: %#v", manifest.Operations)
	}
	for id, call := range manifestCalls(manifest) {
		if call != `api.$operations["`+id+`"]()` {
			t.Fatalf("%s call = %q", id, call)
		}
	}
}

func TestBuildResourceTreeOmitsTwoOperationsRequiringOneTerminal(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
		{OperationID: "listUsers", Method: "GET", Path: "/users"},
		{OperationID: "searchUsers", Method: "GET", Path: "/users"},
	}}
	if _, err := SourceArtifacts(document); err == nil || !strings.Contains(err.Error(), "route identity") {
		t.Fatalf("duplicate route error = %v", err)
	}
}

func TestBuildResourceTreePreservesFixedCallCapabilities(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
		{OperationID: "listUsers", Method: "GET", Path: "/users", Pagination: "cursor"},
		{OperationID: "getPaginate", Method: "GET", Path: "/users/paginate"},
		{OperationID: "getRaw", Method: "GET", Path: "/users/list/raw"},
	}}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	calls := manifestCalls(manifest)
	if calls["listUsers"] != "api.users.list({ query })" {
		t.Fatalf("paginated operation call = %q", calls["listUsers"])
	}
	for _, id := range []string{"getPaginate", "getRaw"} {
		if !strings.HasPrefix(calls[id], `api.$operations["`+id+`"]`) {
			t.Fatalf("%s did not fall back: %q", id, calls[id])
		}
	}
	tree, err := buildResourceTree(document, manifest)
	if err != nil {
		t.Fatal(err)
	}
	users := tree.children["users"]
	if _, ok := paginatedResourceNodeOperation(users); !ok || users.children["paginate"] != nil || users.children["list"] != nil {
		t.Fatalf("fixed capability collision was not resolved: %#v", users)
	}
}

func TestBuildResourceTreePreservesLinkAndStreamCapabilities(t *testing.T) {
	source := ir.Operation{OperationID: "listUsers", Method: "GET", Path: "/users"}
	streamChild := ir.Operation{OperationID: "getStreamChild", Method: "GET", Path: "/users/list/stream"}
	linksChild := ir.Operation{OperationID: "getLinksChild", Method: "GET", Path: "/users/list/links"}
	document := &ir.Document{Operations: []ir.Operation{source, streamChild, linksChild}}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	links := []generatedLink{{SourceOperation: source}}
	streams := []generatedStream{{Operation: source}}
	if err := reconcileResourceCapabilities(document, &manifest, links, streams); err != nil {
		t.Fatal(err)
	}
	calls := manifestCalls(manifest)
	for _, operationID := range []string{"getStreamChild", "getLinksChild"} {
		if !strings.HasPrefix(calls[operationID], `api.$operations["`+operationID+`"]`) {
			t.Fatalf("%s capability collision call = %q", operationID, calls[operationID])
		}
	}
	tree, err := buildResourceTree(document, manifest, resourceCapabilityMembers(links, streams))
	if err != nil {
		t.Fatal(err)
	}
	users := tree.children["users"]
	if users == nil || users.children["list"] != nil {
		t.Fatalf("capability collision retained child namespace: %#v", users)
	}
}

func TestBuildResourceTreeKeepsRootOperationAndOrdinaryChild(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
		{OperationID: "getRoot", Method: "GET", Path: "/"},
		{OperationID: "getGet", Method: "GET", Path: "/get"},
	}}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	calls := manifestCalls(manifest)
	if calls["getRoot"] != "api.get()" || calls["getGet"] != "api.getValue.get()" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestBuildResourceTreeFallsBackForRootParameterBranch(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
		pathOperation("getTenant", "GET", "/{tenant}", "tenant", map[string]any{"type": "string"}),
	}}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	if call := manifest.Operations[0].CallExpression; call != `api.$operations["getTenant"]({ path: { "tenant": tenant } })` {
		t.Fatalf("call = %q", call)
	}
}

func TestBuildResourceTreeSharesCompatibleParameterPositionAndRemapsNames(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
		pathOperation("getProfile", "GET", "/users/{id}/profile", "id", map[string]any{"type": "string"}),
		pathOperation("getSettings", "GET", "/users/{userID}/settings", "userID", map[string]any{"type": "string"}),
	}}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := buildResourceTree(document, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if tree.children["users"].parameterChild == nil {
		t.Fatal("compatible structural parameter branch was omitted")
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	source := string(artifactByPath(t, artifacts, "generated/client.ts"))
	if !strings.Contains(source, `{ "id": id`) || !strings.Contains(source, `{ "userID": id`) {
		t.Fatalf("terminal parameter remapping missing:\n%s", source)
	}
}

func TestBuildResourceTreeOmitsIncompatibleSharedParameterPosition(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
		pathOperation("getProfile", "GET", "/users/{id}/profile", "id", map[string]any{"type": "string"}),
		pathOperation("getSettings", "GET", "/users/{userID}/settings", "userID", map[string]any{"type": "integer"}),
	}}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	if tree, err := buildResourceTree(document, manifest); err != nil || tree.children["users"] != nil {
		t.Fatalf("incompatible parameter branch = %#v, %v", tree, err)
	}
	for id, call := range manifestCalls(manifest) {
		if !strings.HasPrefix(call, `api.$operations["`+id+`"]`) {
			t.Fatalf("%s call = %q", id, call)
		}
	}
}

func TestBuildResourceTreeRejectsIdenticalTemplatedPathShapes(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
		pathOperation("getByID", "GET", "/users/{id}", "id", map[string]any{"type": "string"}),
		pathOperation("getByName", "GET", "/users/{name}", "name", map[string]any{"type": "string"}),
	}}
	_, err := buildManifest(document)
	if err == nil || !strings.Contains(err.Error(), "identical templated shape") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildResourceTreeRejectsEmptyConflictingTemplatedPathItem(t *testing.T) {
	document := &ir.Document{
		Raw: map[string]any{"paths": map[string]any{
			"/users/{id}":   map[string]any{"get": map[string]any{"operationId": "getUser"}},
			"/users/{name}": map[string]any{},
		}},
		Operations: []ir.Operation{pathOperation("getUser", "GET", "/users/{id}", "id", map[string]any{"type": "string"})},
	}
	_, err := buildManifest(document)
	if err == nil || !strings.Contains(err.Error(), "identical templated shape") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildResourceTreePreservesLiteralAndTemplateTraversal(t *testing.T) {
	document := &ir.Document{Operations: []ir.Operation{
		{OperationID: "getCurrentUser", Method: "GET", Path: "/users/me"},
		pathOperation("getUser", "GET", "/users/{id}", "id", map[string]any{"type": "string"}),
	}}
	manifest, err := buildManifest(document)
	if err != nil {
		t.Fatal(err)
	}
	calls := manifestCalls(manifest)
	if calls["getCurrentUser"] != "api.users.me.get()" || calls["getUser"] != "api.users(id).get()" {
		t.Fatalf("calls = %#v", calls)
	}
}

func pathOperation(operationID, method, path, parameterName string, schema map[string]any) ir.Operation {
	return ir.Operation{
		OperationID:        operationID,
		Method:             method,
		Path:               path,
		PathParameterOrder: []string{parameterName},
		Raw: map[string]any{"parameters": []any{
			map[string]any{"name": parameterName, "in": "path", "required": true, "schema": schema},
		}},
	}
}

func manifestCalls(manifest Manifest) map[string]string {
	result := make(map[string]string, len(manifest.Operations))
	for _, operation := range manifest.Operations {
		result[operation.OperationID] = operation.CallExpression
	}
	return result
}

func buildResourceSegments(manifest Manifest) map[string]bool {
	result := make(map[string]bool)
	for _, operation := range manifest.Operations {
		for _, segment := range operation.ResourceSegments {
			result[segment] = true
		}
	}
	return result
}

func TestEmitQueryTypesKeepsOrdinaryLimitAndSortParameters(t *testing.T) {
	operation := ir.Operation{
		OperationID: "searchWidgets",
		Method:      "GET",
		Path:        "/widgets",
		Raw: map[string]any{"parameters": []any{
			map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer"}},
			map[string]any{"name": "sort", "in": "query", "schema": map[string]any{"type": "string"}},
		}},
	}
	parameters, err := operationParameters(&ir.Document{}, operation)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := emitQueryTypes(&output, &ir.Document{}, operation, "SearchWidgets", parameters); err != nil {
		t.Fatal(err)
	}
	for _, property := range []string{`readonly "limit"?: number | undefined`, `readonly "sort"?: string | undefined`} {
		if !strings.Contains(output.String(), property) {
			t.Fatalf("query input omitted %q:\n%s", property, output.String())
		}
	}
}

func TestValidateSourceExportSymbolsRejectsCrossModuleCollision(t *testing.T) {
	err := validateSourceExportSymbols(map[string][]byte{
		"types":  []byte("export type ListWidgetsInput = {}\n"),
		"client": []byte("export type ListWidgetsInput = {}\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "generated source export") {
		t.Fatalf("error = %v", err)
	}
}

func TestSourceArtifactsSeparatesComponentAndOperationNamespaces(t *testing.T) {
	document := &ir.Document{
		ContractVersion: "1.0.0",
		ComponentSchemas: map[string]map[string]any{
			"APIError": {"type": "string"},
		},
		Operations: []ir.Operation{{
			OperationID: "listWidgets",
			Method:      "GET",
			Path:        "/widgets",
			Raw: map[string]any{
				"responses": map[string]any{
					"200": map[string]any{
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/APIError"},
							},
						},
					},
				},
			},
		}},
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	types := string(artifactByPath(t, artifacts, "generated/types.ts"))
	client := string(artifactByPath(t, artifacts, "generated/client.ts"))
	if !strings.Contains(types, `readonly "APIError": {`) || !strings.Contains(client, `Contract.ComponentOutput<"APIError">`) {
		t.Fatalf("component namespace was not preserved:\n%s\n%s", types, client)
	}
}

func TestSourceArtifactsDoesNotRequireNPMNameOrSemVer(t *testing.T) {
	artifacts, err := SourceArtifacts(&ir.Document{ContractVersion: "release-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 7 {
		t.Fatalf("source artifact count = %d, want 7", len(artifacts))
	}
}
