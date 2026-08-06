package typescript

import (
	"fmt"
	pathpkg "path"
	"sort"
	"strings"

	"openapi-sdkgen/internal/compiler/ir"
)

type semanticModulePlan struct {
	schemas          []schemaModulePlan
	operations       []operationModulePlan
	resources        []resourceModulePlan
	fixed            map[string]string
	schemaByName     map[string]string
	operationByRoute map[string]string
}

type schemaModulePlan struct {
	name             string
	path             string
	publicProjection bool
	inputWire        bool
	outputWire       bool
}

type operationModulePlan struct {
	routeKey string
	path     string
}

type resourceModulePlan struct {
	identity string
	path     string
}

func buildSemanticModulePlan(document *ir.Document, manifest Manifest, links []generatedLink, streams []generatedStream) (*semanticModulePlan, error) {
	result := &semanticModulePlan{
		fixed: map[string]string{
			"internal-index":  "internal/index.ts",
			"schema-index":    "internal/schemas/index.ts",
			"schema-wire":     "internal/schemas/wire.ts",
			"route-index":     "internal/routes/index.ts",
			"route-inputs":    "internal/routes/inputs.ts",
			"route-helpers":   "internal/routes/helpers.ts",
			"resource-index":  "internal/resources/index.ts",
			"client-index":    "internal/client/index.ts",
			"client-types":    "internal/client/types.ts",
			"client-registry": "internal/client/registry.ts",
			"client-factory":  "internal/client/factory.ts",
		},
		schemaByName:     make(map[string]string),
		operationByRoute: make(map[string]string),
	}
	if err := result.planSchemas(document); err != nil {
		return nil, err
	}
	if err := result.planOperations(manifest); err != nil {
		return nil, err
	}
	if err := result.planResources(document, manifest, links, streams); err != nil {
		return nil, err
	}
	if err := result.validate(); err != nil {
		return nil, err
	}
	return result, nil
}

func (plan *semanticModulePlan) planSchemas(document *ir.Document) error {
	reachable := reachableComponentSchemas(document)
	inputWire := hasVisibleInputSchemas(document)
	outputWire := hasVisibleResponseBodies(document)
	all := make(map[string]bool, len(document.ComponentSchemas)+len(document.Schemas))
	for name := range document.ComponentSchemas {
		if reachable[name] || inputWire || outputWire {
			all[name] = true
		}
	}
	for name := range document.Schemas {
		if reachable[name] || inputWire || outputWire {
			all[name] = true
		}
	}
	names := sortedStringKeys(all)
	candidates := make([]artifactPathCandidate, 0, len(names))
	for _, name := range names {
		candidates = append(candidates, artifactPathCandidate{identity: name, base: pathpkg.Join("internal", "schemas", safeArtifactStem(name)+".ts")})
	}
	paths, err := allocateArtifactPaths(candidates, plan.fixed["schema-index"], plan.fixed["schema-wire"])
	if err != nil {
		return fmt.Errorf("plan schema artifacts: %w", err)
	}
	for _, name := range names {
		item := schemaModulePlan{name: name, path: paths[name], publicProjection: reachable[name], inputWire: inputWire, outputWire: outputWire}
		plan.schemas = append(plan.schemas, item)
		plan.schemaByName[name] = item.path
	}
	return nil
}

func (plan *semanticModulePlan) planOperations(manifest Manifest) error {
	candidates := make([]artifactPathCandidate, 0, len(manifest.Operations))
	for _, operation := range manifest.Operations {
		if operation.Visibility == "hidden" {
			continue
		}
		routeKey := manifestRouteKey(operation)
		candidates = append(candidates, artifactPathCandidate{identity: routeKey, base: operationArtifactBase(operation.Path, operation.Method)})
	}
	paths, err := allocateArtifactPaths(candidates)
	if err != nil {
		return fmt.Errorf("plan operation artifacts: %w", err)
	}
	routes := make([]string, 0, len(paths))
	for route := range paths {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	for _, route := range routes {
		item := operationModulePlan{routeKey: route, path: paths[route]}
		plan.operations = append(plan.operations, item)
		plan.operationByRoute[route] = item.path
	}
	return nil
}

func (plan *semanticModulePlan) planResources(document *ir.Document, manifest Manifest, links []generatedLink, streams []generatedStream) error {
	tree, err := buildResourceTree(document, manifest, resourceCapabilityMembers(links, streams))
	if err != nil {
		return fmt.Errorf("build resource tree for module plan: %w", err)
	}
	candidates := []artifactPathCandidate{{identity: "root", base: "internal/resources/root.ts"}}
	collectResourceModuleCandidates(tree, nil, nil, &candidates)
	paths, err := allocateArtifactPaths(candidates, plan.fixed["resource-index"])
	if err != nil {
		return fmt.Errorf("plan resource artifacts: %w", err)
	}
	identities := make([]string, 0, len(paths))
	for identity := range paths {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	for _, identity := range identities {
		plan.resources = append(plan.resources, resourceModulePlan{identity: identity, path: paths[identity]})
	}
	return nil
}

func collectResourceModuleCandidates(node *resourceNode, artifactParent, identityParent []string, result *[]artifactPathCandidate) {
	children := sortedResourceChildNames(node)
	for _, name := range children {
		source := node.childSources[name]
		segment := safeArtifactStem(source)
		segments := append(append([]string(nil), artifactParent...), segment)
		identitySegments := append(append([]string(nil), identityParent...), source)
		identity := "literal:" + strings.Join(identitySegments, "/")
		base := resourceArtifactBase(segments, false, identity)
		*result = append(*result, artifactPathCandidate{identity: identity, base: base})
		collectResourceModuleCandidates(node.children[name], segments, identitySegments, result)
	}
	if node.parameterChild != nil {
		name := node.parameterChild.parameter.Name
		segment := "by-" + safeArtifactStem(name)
		segments := append(append([]string(nil), artifactParent...), segment)
		identitySegments := append(append([]string(nil), identityParent...), "{"+name+"}")
		identity := "parameter:" + strings.Join(identitySegments, "/")
		base := resourceArtifactBase(segments, true, identity)
		*result = append(*result, artifactPathCandidate{identity: identity, base: base})
		collectResourceModuleCandidates(node.parameterChild, segments, identitySegments, result)
	}
}

func resourceArtifactBase(segments []string, parameter bool, identity string) string {
	var result string
	if parameter {
		result = pathpkg.Join(append([]string{"internal", "resources"}, segments...)...) + ".ts"
	} else {
		result = pathpkg.Join(append([]string{"internal", "resources"}, append(segments, "index.ts")...)...)
	}
	if len(result) <= maxArtifactPathBytes {
		return result
	}
	return pathpkg.Join("internal", "resources", "node-"+shortArtifactHash(identity)+".ts")
}

func (plan *semanticModulePlan) validate() error {
	owners := make(map[string]string)
	add := func(owner, value string) error {
		if err := validateArtifactPath(value); err != nil {
			return fmt.Errorf("%s path %q: %w", owner, value, err)
		}
		key := portableArtifactPathKey(value)
		if previous, exists := owners[key]; exists {
			return fmt.Errorf("semantic artifact %q is owned by both %s and %s", value, previous, owner)
		}
		owners[key] = owner
		return nil
	}
	fixedNames := make([]string, 0, len(plan.fixed))
	for name := range plan.fixed {
		fixedNames = append(fixedNames, name)
	}
	sort.Strings(fixedNames)
	for _, name := range fixedNames {
		if err := add(name, plan.fixed[name]); err != nil {
			return err
		}
	}
	for _, schema := range plan.schemas {
		if err := add("schema "+schema.name, schema.path); err != nil {
			return err
		}
	}
	for _, operation := range plan.operations {
		if err := add("operation "+operation.routeKey, operation.path); err != nil {
			return err
		}
	}
	for _, resource := range plan.resources {
		if err := add("resource "+resource.identity, resource.path); err != nil {
			return err
		}
	}
	return nil
}

type typeReferenceUse struct {
	key             string
	modulePath      string
	exportName      string
	requiresBinding bool
}

type plannedTypeReference struct {
	key        string
	specifier  string
	exportName string
	alias      string
	inline     bool
}

func planTypeReferences(currentArtifact string, uses []typeReferenceUse) ([]plannedTypeReference, error) {
	counts := make(map[string]int)
	seenKeys := make(map[string]bool, len(uses))
	for _, use := range uses {
		if use.key == "" || use.modulePath == "" || use.exportName == "" {
			return nil, fmt.Errorf("type reference use is incomplete")
		}
		if seenKeys[use.key] {
			return nil, fmt.Errorf("type reference key %q is duplicated", use.key)
		}
		seenKeys[use.key] = true
		counts[use.modulePath+"\x00"+use.exportName]++
	}
	result := make([]plannedTypeReference, 0, len(uses))
	for _, use := range uses {
		specifier, err := relativeModuleSpecifier(currentArtifact, use.modulePath)
		if err != nil {
			return nil, fmt.Errorf("type reference %q: %w", use.key, err)
		}
		identity := use.modulePath + "\x00" + use.exportName
		inline := counts[identity] == 1 && !use.requiresBinding
		alias := ""
		if !inline {
			alias = stablePrivateIdentifier("type-import", identity)
		}
		result = append(result, plannedTypeReference{key: use.key, specifier: specifier, exportName: use.exportName, alias: alias, inline: inline})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].key < result[right].key })
	return result, nil
}

type semanticModuleClass uint8

const (
	moduleUnknown semanticModuleClass = iota
	moduleRuntime
	moduleSchema
	moduleOperation
	moduleRoute
	moduleResource
	moduleClient
)

type moduleImportEdge struct {
	from string
	to   string
}

func validateSemanticImportDirection(edges []moduleImportEdge) error {
	for _, edge := range edges {
		from := classifySemanticModule(edge.from)
		to := classifySemanticModule(edge.to)
		if from == moduleRuntime && to != moduleRuntime {
			return fmt.Errorf("runtime module %q imports semantic module %q", edge.from, edge.to)
		}
		if from == moduleOperation && (to == moduleRoute || to == moduleResource || to == moduleClient) {
			return fmt.Errorf("operation module %q imports aggregate module %q", edge.from, edge.to)
		}
	}
	return nil
}

func classifySemanticModule(value string) semanticModuleClass {
	switch {
	case strings.HasPrefix(value, "internal/runtime/"):
		return moduleRuntime
	case strings.HasPrefix(value, "internal/schemas/"):
		return moduleSchema
	case strings.HasPrefix(value, "internal/operations/"):
		return moduleOperation
	case strings.HasPrefix(value, "internal/routes/"):
		return moduleRoute
	case strings.HasPrefix(value, "internal/resources/"):
		return moduleResource
	case strings.HasPrefix(value, "internal/client/"):
		return moduleClient
	default:
		return moduleUnknown
	}
}

func validateGeneratedArtifacts(artifacts []Artifact) error {
	seen := make(map[string]string, len(artifacts))
	for _, artifact := range artifacts {
		if err := validateArtifactPath(artifact.Path); err != nil {
			return fmt.Errorf("generated artifact %q: %w", artifact.Path, err)
		}
		key := portableArtifactPathKey(artifact.Path)
		if previous, exists := seen[key]; exists {
			return fmt.Errorf("generated artifact %q collides with %q", artifact.Path, previous)
		}
		seen[key] = artifact.Path
		if !strings.HasPrefix(string(artifact.Data), generatedFileHeader) {
			return fmt.Errorf("generated artifact %q is missing the standard header", artifact.Path)
		}
	}
	return nil
}
