package typescript

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"openapi-sdkgen/internal/compiler/ir"
)

type renderedSchemaProjections struct {
	imports []string
	input   string
	output  string
}

func emitSchemaArtifacts(document *ir.Document, plan *semanticModulePlan) ([]Artifact, []byte, error) {
	if plan == nil {
		return nil, nil, fmt.Errorf("internal TypeScript target: prepared plan has no semantic modules")
	}
	artifacts := make([]Artifact, 0, len(plan.schemas)+2)
	for _, schema := range plan.schemas {
		source, err := emitSchemaLeaf(document, plan, schema)
		if err != nil {
			return nil, nil, err
		}
		artifacts = append(artifacts, Artifact{Path: schema.path, Data: generatedSource(source)})
	}
	indexSource, err := emitSchemaIndex(document, plan)
	if err != nil {
		return nil, nil, err
	}
	wireSource, err := emitSchemaWireRegistry(plan)
	if err != nil {
		return nil, nil, err
	}
	artifacts = append(artifacts,
		Artifact{Path: plan.fixed["schema-index"], Data: generatedSource(indexSource)},
		Artifact{Path: plan.fixed["schema-wire"], Data: generatedSource(wireSource)},
	)
	return artifacts, indexSource, nil
}

func emitSchemaLeaf(document *ir.Document, plan *semanticModulePlan, schema schemaModulePlan) ([]byte, error) {
	value := componentSchemaValue(document, schema.name)
	var projections renderedSchemaProjections
	var err error
	if schema.publicProjection {
		projections, err = renderSchemaProjections(document, plan, schema, value)
		if err != nil {
			return nil, fmt.Errorf("component %s projections: %w", schema.name, err)
		}
	}
	inputDescriptor := ""
	if schema.inputWire {
		inputDescriptor, err = wireSchemaDescriptorForDocument(document, value, projectionInput)
		if err != nil {
			return nil, fmt.Errorf("component %s input wire schema: %w", schema.name, err)
		}
	}
	outputDescriptor := ""
	if schema.outputWire {
		outputDescriptor, err = wireSchemaDescriptorForDocument(document, value, projectionOutput)
		if err != nil {
			return nil, fmt.Errorf("component %s output wire schema: %w", schema.name, err)
		}
	}

	var output bytes.Buffer
	if schema.inputWire || schema.outputWire {
		output.WriteString("import type { WireSchema } from \"../runtime/codecs.js\"\n")
	}
	for _, declaration := range projections.imports {
		output.WriteString(declaration)
		output.WriteByte('\n')
	}
	if output.Len() > 0 {
		output.WriteByte('\n')
	}
	if schema.publicProjection {
		output.WriteString("/** Request/input projection. */\n")
		fmt.Fprintf(&output, "export type Input = %s\n\n", projections.input)
		output.WriteString("/** Response/output projection. */\n")
		fmt.Fprintf(&output, "export type Output = %s\n", projections.output)
		if schema.inputWire || schema.outputWire {
			output.WriteByte('\n')
		}
	}
	if schema.inputWire {
		output.WriteString("/** Runtime wire descriptor for the input projection. */\n")
		fmt.Fprintf(&output, "export const inputWireSchema: WireSchema = %s\n", inputDescriptor)
		if schema.outputWire {
			output.WriteByte('\n')
		}
	}
	if schema.outputWire {
		output.WriteString("/** Runtime wire descriptor for the output projection. */\n")
		fmt.Fprintf(&output, "export const outputWireSchema: WireSchema = %s\n", outputDescriptor)
	}
	return output.Bytes(), nil
}

func renderSchemaProjections(document *ir.Document, plan *semanticModulePlan, schema schemaModulePlan, value any) (renderedSchemaProjections, error) {
	uses := make([]typeReferenceUse, 0)
	sequence := 0
	var referenceErr error
	countReference := func(name string, direction projection) string {
		if name == schema.name {
			if direction == projectionInput {
				return "Input"
			}
			return "Output"
		}
		path, exists := plan.schemaByName[name]
		if !exists {
			if referenceErr == nil {
				referenceErr = fmt.Errorf("component reference %q has no schema owner", name)
			}
			return "unknown"
		}
		exportName := "Output"
		if direction == projectionInput {
			exportName = "Input"
		}
		key := fmt.Sprintf("%08d", sequence)
		sequence++
		uses = append(uses, typeReferenceUse{key: key, modulePath: path, exportName: exportName})
		return "unknown"
	}
	countScope := typeRenderModule(countReference)
	if _, err := schemaTypeForScope(document, value, projectionInput, countScope); err != nil {
		return renderedSchemaProjections{}, err
	}
	if _, err := schemaTypeForScope(document, value, projectionOutput, countScope); err != nil {
		return renderedSchemaProjections{}, err
	}
	if referenceErr != nil {
		return renderedSchemaProjections{}, referenceErr
	}
	planned, err := planTypeReferences(schema.path, uses)
	if err != nil {
		return renderedSchemaProjections{}, err
	}
	byKey := make(map[string]plannedTypeReference, len(planned))
	for _, reference := range planned {
		byKey[reference.key] = reference
	}

	sequence = 0
	referenceErr = nil
	renderReference := func(name string, direction projection) string {
		if name == schema.name {
			if direction == projectionInput {
				return "Input"
			}
			return "Output"
		}
		key := fmt.Sprintf("%08d", sequence)
		sequence++
		reference, exists := byKey[key]
		if !exists {
			if referenceErr == nil {
				referenceErr = fmt.Errorf("component reference %q was not planned", name)
			}
			return "unknown"
		}
		if reference.inline {
			return "import(" + quoteTS(reference.specifier) + ")." + reference.exportName
		}
		return reference.alias
	}
	renderScope := typeRenderModule(renderReference)
	input, err := schemaTypeForScope(document, value, projectionInput, renderScope)
	if err != nil {
		return renderedSchemaProjections{}, err
	}
	output, err := schemaTypeForScope(document, value, projectionOutput, renderScope)
	if err != nil {
		return renderedSchemaProjections{}, err
	}
	if referenceErr != nil {
		return renderedSchemaProjections{}, referenceErr
	}
	if sequence != len(uses) {
		return renderedSchemaProjections{}, fmt.Errorf("rendered %d component references, planned %d", sequence, len(uses))
	}

	importsByAlias := make(map[string]plannedTypeReference)
	for _, reference := range planned {
		if !reference.inline {
			importsByAlias[reference.alias] = reference
		}
	}
	aliases := make([]string, 0, len(importsByAlias))
	for alias := range importsByAlias {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	imports := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		reference := importsByAlias[alias]
		imports = append(imports, "import type { "+reference.exportName+" as "+alias+" } from "+quoteTS(reference.specifier))
	}
	return renderedSchemaProjections{imports: imports, input: input, output: output}, nil
}

func emitSchemaIndex(document *ir.Document, plan *semanticModulePlan) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("/** Component types keyed by exact OpenAPI schema names. */\n")
	output.WriteString("export interface Components {\n")
	for _, schema := range plan.schemas {
		if !schema.publicProjection {
			continue
		}
		value := componentSchemaValue(document, schema.name)
		if object, ok := value.(map[string]any); ok {
			emitSchemaValueJSDoc(&output, "  ", object, "OpenAPI component `"+sanitizeComment(schema.name)+"`.")
		} else {
			fmt.Fprintf(&output, "  /** OpenAPI component `%s`. */\n", sanitizeComment(schema.name))
		}
		specifier, err := relativeModuleSpecifier(plan.fixed["schema-index"], schema.path)
		if err != nil {
			return nil, fmt.Errorf("component %s registry reference: %w", schema.name, err)
		}
		fmt.Fprintf(&output, "  readonly %s: {\n", quoteTS(schema.name))
		output.WriteString("    /** Request/input projection. */\n")
		fmt.Fprintf(&output, "    readonly input: import(%s).Input\n", quoteTS(specifier))
		output.WriteString("    /** Response/output projection. */\n")
		fmt.Fprintf(&output, "    readonly output: import(%s).Output\n", quoteTS(specifier))
		output.WriteString("  }\n")
	}
	output.WriteString("}\n\n")
	output.WriteString("/** Input projection for an exact OpenAPI component schema name. */\n")
	output.WriteString("export type ComponentInput<Name extends keyof Components> = Components[Name][\"input\"]\n")
	output.WriteString("/** Output projection for an exact OpenAPI component schema name. */\n")
	output.WriteString("export type ComponentOutput<Name extends keyof Components> = Components[Name][\"output\"]\n")
	return output.Bytes(), nil
}

func emitSchemaWireRegistry(plan *semanticModulePlan) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("import type { WireSchemas } from \"../runtime/codecs.js\"\n")
	inputProperties := make([]runtimeProperty, 0, len(plan.schemas))
	outputProperties := make([]runtimeProperty, 0, len(plan.schemas))
	for _, schema := range plan.schemas {
		if !schema.inputWire && !schema.outputWire {
			continue
		}
		specifier, err := relativeModuleSpecifier(plan.fixed["schema-wire"], schema.path)
		if err != nil {
			return nil, fmt.Errorf("component %s wire registry reference: %w", schema.name, err)
		}
		imports := make([]string, 0, 2)
		if schema.inputWire {
			identifier := stablePrivateIdentifier("schema-input-wire", schema.name)
			imports = append(imports, "inputWireSchema as "+identifier)
			inputProperties = append(inputProperties, runtimeProperty{key: schema.name, value: identifier})
		}
		if schema.outputWire {
			identifier := stablePrivateIdentifier("schema-output-wire", schema.name)
			imports = append(imports, "outputWireSchema as "+identifier)
			outputProperties = append(outputProperties, runtimeProperty{key: schema.name, value: identifier})
		}
		fmt.Fprintf(&output, "import { %s } from %s\n", strings.Join(imports, ", "), quoteTS(specifier))
	}
	output.WriteByte('\n')
	output.WriteString("/** Input component wire schemas keyed by exact OpenAPI names. */\n")
	fmt.Fprintf(&output, "export const inputSchemas: WireSchemas = %s\n\n", runtimeObjectExpression(inputProperties))
	output.WriteString("/** Output component wire schemas keyed by exact OpenAPI names. */\n")
	fmt.Fprintf(&output, "export const outputSchemas: WireSchemas = %s\n", runtimeObjectExpression(outputProperties))
	return output.Bytes(), nil
}
