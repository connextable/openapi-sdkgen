package typescript

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"openapi-sdkgen/internal/compiler/ir"
)

type plannedResourceNode struct {
	identity string
	path     string
	node     *resourceNode
}

func emitResourceArtifacts(document *ir.Document, manifest Manifest, plan *semanticModulePlan, links []generatedLink, streams []generatedStream) ([]Artifact, error) {
	if plan == nil {
		return nil, fmt.Errorf("internal TypeScript target: prepared plan has no semantic modules")
	}
	tree, err := buildResourceTree(document, manifest, resourceCapabilityMembers(links, streams))
	if err != nil {
		return nil, err
	}
	paths := make(map[string]string, len(plan.resources))
	for _, module := range plan.resources {
		paths[module.identity] = module.path
	}
	nodes := make([]plannedResourceNode, 0, len(paths))
	collectPlannedResourceNodes(tree, "root", nil, nil, paths, &nodes)
	sort.Slice(nodes, func(left, right int) bool { return nodes[left].path < nodes[right].path })
	artifacts := make([]Artifact, 0, len(nodes)+1)
	for _, module := range nodes {
		source, err := emitResourceNodeModule(document, plan, module, paths)
		if err != nil {
			return nil, fmt.Errorf("emit resource module %q: %w", module.identity, err)
		}
		artifacts = append(artifacts, Artifact{Path: module.path, Data: generatedSource(source)})
	}
	indexSource, err := emitResourceIndex(plan, paths["root"])
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, Artifact{Path: plan.fixed["resource-index"], Data: generatedSource(indexSource)})
	return artifacts, nil
}

func collectPlannedResourceNodes(node *resourceNode, identity string, artifactParent, identityParent []string, paths map[string]string, result *[]plannedResourceNode) {
	if path, exists := paths[identity]; exists {
		*result = append(*result, plannedResourceNode{identity: identity, path: path, node: node})
	}
	for _, name := range sortedResourceChildNames(node) {
		source := node.childSources[name]
		segment := safeArtifactStem(source)
		segments := append(append([]string(nil), artifactParent...), segment)
		identitySegments := append(append([]string(nil), identityParent...), source)
		collectPlannedResourceNodes(node.children[name], "literal:"+strings.Join(identitySegments, "/"), segments, identitySegments, paths, result)
	}
	if node.parameterChild != nil {
		name := node.parameterChild.parameter.Name
		segment := "by-" + safeArtifactStem(name)
		segments := append(append([]string(nil), artifactParent...), segment)
		identitySegments := append(append([]string(nil), identityParent...), "{"+name+"}")
		collectPlannedResourceNodes(node.parameterChild, "parameter:"+strings.Join(identitySegments, "/"), segments, identitySegments, paths, result)
	}
}

func emitResourceNodeModule(document *ir.Document, plan *semanticModulePlan, module plannedResourceNode, paths map[string]string) ([]byte, error) {
	registrySpecifier, err := relativeModuleSpecifier(module.path, plan.fixed["client-registry"])
	if err != nil {
		return nil, err
	}
	helperSpecifier, err := relativeModuleSpecifier(module.path, plan.fixed["route-helpers"])
	if err != nil {
		return nil, err
	}
	callablesSpecifier, err := relativeModuleSpecifier(module.path, "internal/runtime/callables.ts")
	if err != nil {
		return nil, err
	}

	childIdentities := resourceChildIdentities(module.identity, module.node)
	needsAssign := module.node.parameterChild != nil
	needsPath := false
	for name, operation := range module.node.operations {
		if len(operation.PathParameterOrder) > 0 {
			needsPath = true
		}
		if module.node.children[name] != nil {
			needsAssign = true
		}
	}

	var output bytes.Buffer
	fmt.Fprintf(&output, "import type { CallableRegistry } from %s\n", quoteTS(registrySpecifier))
	fmt.Fprintf(&output, "import type { PaginateCall, ResourceCall } from %s\n", quoteTS(helperSpecifier))
	if needsAssign || needsPath {
		imports := make([]string, 0, 2)
		if needsAssign {
			imports = append(imports, "assignCallableProperties")
		}
		if needsPath {
			imports = append(imports, "bindPathOperation")
		}
		fmt.Fprintf(&output, "import { %s } from %s\n", strings.Join(imports, ", "), quoteTS(callablesSpecifier))
	}
	for _, identity := range uniqueResourceChildIdentities(childIdentities) {
		path := paths[identity]
		specifier, err := relativeModuleSpecifier(module.path, path)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&output, "import { build as %s } from %s\n", resourceBuilderName(identity), quoteTS(specifier))
	}
	output.WriteByte('\n')

	output.WriteString("/** Resource surface owned by this resolved OpenAPI path node. */\n")
	output.WriteString("export interface Surface {\n")
	for _, name := range sortedResourceMemberNames(module.node) {
		operation, hasOperation := module.node.operations[name]
		child := module.node.children[name]
		typeName := ""
		if hasOperation {
			typeName = "ResourceCall<" + quoteTS(manifestRouteKey(operation)) + ">"
		}
		if child != nil {
			identity := childIdentities["literal:"+name]
			childType, err := resourceChildSurfaceType(document, plan, module.path, identity, child, paths)
			if err != nil {
				return nil, err
			}
			if typeName == "" {
				typeName = childType
			} else {
				typeName += " & " + childType
			}
		}
		emitResourceEntryJSDoc(&output, "  ", name, operation, hasOperation)
		fmt.Fprintf(&output, "  readonly %s: %s\n", name, typeName)
	}
	if paginated, ok := paginatedResourceNodeOperation(module.node); ok {
		fmt.Fprintf(&output, "  /** Lazily iterates every item from exact operation `%s` pagination. */\n", sanitizeComment(paginated.OperationID))
		fmt.Fprintf(&output, "  readonly paginate: PaginateCall<%s>\n", quoteTS(manifestRouteKey(paginated)))
	}
	if module.node.parameterChild != nil {
		parameter := module.node.parameterChild.parameter
		parameterType, err := resourceParameterType(document, plan, module.path, parameter)
		if err != nil {
			return nil, err
		}
		identity := childIdentities["parameter"]
		path := paths[identity]
		specifier, err := relativeModuleSpecifier(module.path, path)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&output, "  /** Selects one resource by the `%s` path parameter. */\n", sanitizeComment(parameter.Name))
		fmt.Fprintf(&output, "  (%s: %s): import(%s).Surface\n", parameter.Binding, parameterType, quoteTS(specifier))
	}
	output.WriteString("}\n\n")

	output.WriteString("/** Builds this resource node from the completed exact-call registry. */\n")
	output.WriteString("export function build(registry: CallableRegistry, bound: readonly unknown[] = []): Surface {\n")
	output.WriteString("  const members = {\n")
	for _, name := range sortedResourceMemberNames(module.node) {
		fmt.Fprintf(&output, "    %s: ", name)
		if err := emitResourceModuleMemberValue(&output, document, plan, module, paths, childIdentities, name); err != nil {
			return nil, err
		}
		output.WriteString(",\n")
	}
	if paginated, ok := paginatedResourceNodeOperation(module.node); ok {
		fmt.Fprintf(&output, "    paginate: registry.routes[%s].paginate,\n", quoteTS(manifestRouteKey(paginated)))
	}
	output.WriteString("  }\n")
	if module.node.parameterChild != nil {
		parameter := module.node.parameterChild.parameter
		parameterType, err := resourceParameterType(document, plan, module.path, parameter)
		if err != nil {
			return nil, err
		}
		identity := childIdentities["parameter"]
		fmt.Fprintf(&output, "  return assignCallableProperties((%s: %s) => %s(registry, [...bound, %s]), members) as Surface\n", parameter.Binding, parameterType, resourceBuilderName(identity), parameter.Binding)
	} else {
		output.WriteString("  return members as Surface\n")
	}
	output.WriteString("}\n")
	return output.Bytes(), nil
}

func resourceChildSurfaceType(document *ir.Document, plan *semanticModulePlan, artifact, identity string, child *resourceNode, paths map[string]string) (string, error) {
	childSpecifier, err := relativeModuleSpecifier(artifact, paths[identity])
	if err != nil {
		return "", err
	}
	childType := "import(" + quoteTS(childSpecifier) + ").Surface"
	if child.parameterChild == nil {
		return childType, nil
	}
	parameter := child.parameterChild.parameter
	parameterType, err := resourceParameterType(document, plan, artifact, parameter)
	if err != nil {
		return "", err
	}
	parameterIdentity := resourceChildIdentities(identity, child)["parameter"]
	parameterSpecifier, err := relativeModuleSpecifier(artifact, paths[parameterIdentity])
	if err != nil {
		return "", err
	}
	callType := "((" + parameter.Binding + ": " + parameterType + ") => import(" + quoteTS(parameterSpecifier) + ").Surface)"
	return callType + " & " + childType, nil
}

func resourceParameterType(document *ir.Document, plan *semanticModulePlan, artifact string, parameter *operationParameter) (string, error) {
	typeName, err := schemaTypeForScope(document, parameter.Schema, projectionInput, typeRenderContract)
	if err != nil {
		return "", err
	}
	for _, schema := range plan.schemas {
		if !schema.publicProjection {
			continue
		}
		needle := "Contract.ComponentInput<" + quoteTS(schema.name) + ">"
		if !strings.Contains(typeName, needle) {
			continue
		}
		specifier, err := relativeModuleSpecifier(artifact, schema.path)
		if err != nil {
			return "", err
		}
		typeName = strings.ReplaceAll(typeName, needle, "import("+quoteTS(specifier)+").Input")
	}
	if strings.Contains(typeName, "Contract.") {
		return "", fmt.Errorf("resource parameter %q retains an unplanned schema registry reference", parameter.Name)
	}
	return typeName, nil
}

func emitResourceModuleMemberValue(output *bytes.Buffer, document *ir.Document, plan *semanticModulePlan, module plannedResourceNode, paths map[string]string, childIdentities map[string]string, name string) error {
	operation, hasOperation := module.node.operations[name]
	child := module.node.children[name]
	if hasOperation && child != nil {
		output.WriteString("assignCallableProperties(")
		if err := emitResourceModuleOperationValue(output, document, plan, module.path, operation); err != nil {
			return err
		}
		fmt.Fprintf(output, ", %s(registry, bound))", resourceBuilderName(childIdentities["literal:"+name]))
		return nil
	}
	if hasOperation {
		return emitResourceModuleOperationValue(output, document, plan, module.path, operation)
	}
	fmt.Fprintf(output, "%s(registry, bound)", resourceBuilderName(childIdentities["literal:"+name]))
	return nil
}

func emitResourceModuleOperationValue(output *bytes.Buffer, document *ir.Document, plan *semanticModulePlan, artifact string, operation ManifestOperation) error {
	route := manifestRouteKey(operation)
	call := "registry.routes[" + quoteTS(route) + "]"
	if len(operation.PathParameterOrder) == 0 {
		output.WriteString(call)
		return nil
	}
	path, exists := plan.operationByRoute[route]
	if !exists {
		return fmt.Errorf("resource operation %q has no operation module", route)
	}
	specifier, err := relativeModuleSpecifier(artifact, path)
	if err != nil {
		return err
	}
	values := make([]string, 0, len(operation.PathParameterOrder))
	for index, parameter := range operation.PathParameterOrder {
		values = append(values, quoteTS(parameter)+": bound["+fmt.Sprint(index)+"]")
	}
	hasInput := len(operation.InputTypes) > 1
	inputOptional := false
	if hasInput {
		required, err := operationInputRequired(document, findOperation(document, route), operation.InputTypes, true)
		if err != nil {
			return err
		}
		inputOptional = !required
	}
	fmt.Fprintf(output, "bindPathOperation<import(%s).Input, import(%s).ResourceInput, import(%s).Output, import(%s).Options, import(%s).RawResponse>(%s, { %s }, %t, %t)", quoteTS(specifier), quoteTS(specifier), quoteTS(specifier), quoteTS(specifier), quoteTS(specifier), call, strings.Join(values, ", "), hasInput, inputOptional)
	return nil
}

func resourceChildIdentities(parentIdentity string, node *resourceNode) map[string]string {
	result := make(map[string]string)
	parentParts := resourceIdentityParts(parentIdentity)
	for _, name := range sortedResourceChildNames(node) {
		parts := append(append([]string(nil), parentParts...), node.childSources[name])
		identity := "literal:" + strings.Join(parts, "/")
		result["literal:"+name] = identity
		result[identity] = identity
	}
	if node.parameterChild != nil {
		parts := append(append([]string(nil), parentParts...), "{"+node.parameterChild.parameter.Name+"}")
		identity := "parameter:" + strings.Join(parts, "/")
		result["parameter"] = identity
		result[identity] = identity
	}
	return result
}

func resourceIdentityParts(identity string) []string {
	if identity == "root" {
		return nil
	}
	_, value, _ := strings.Cut(identity, ":")
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}

func resourceBuilderName(identity string) string {
	return stablePrivateIdentifier("resource-builder", identity)
}

func uniqueResourceChildIdentities(values map[string]string) []string {
	unique := make(map[string]bool)
	for _, identity := range values {
		unique[identity] = true
	}
	return sortedStringKeys(unique)
}

func emitResourceEntryJSDoc(output *bytes.Buffer, indent, name string, operation ManifestOperation, hasOperation bool) {
	if hasOperation {
		emitResourceOperationJSDoc(output, indent, operation)
		return
	}
	fmt.Fprintf(output, "%s/** Nested resource path segment `%s`. */\n", indent, sanitizeComment(name))
}

func emitResourceIndex(plan *semanticModulePlan, rootPath string) ([]byte, error) {
	specifier, err := relativeModuleSpecifier(plan.fixed["resource-index"], rootPath)
	if err != nil {
		return nil, err
	}
	return []byte("export { build } from " + quoteTS(specifier) + "\nexport type { Surface } from " + quoteTS(specifier) + "\n"), nil
}
