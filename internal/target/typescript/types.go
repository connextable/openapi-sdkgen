package typescript

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/compiler/naming"
)

type projection string

const (
	projectionNeutral projection = "neutral"
	projectionInput   projection = "input"
	projectionOutput  projection = "output"
)

func emitTypes(document *ir.Document) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("/** Sort directions accepted by generated list operations. */\n")
	output.WriteString("export const SortDirection = {\n")
	output.WriteString("  /** Ascending sort order. */\n  ASC: \"asc\",\n")
	output.WriteString("  /** Descending sort order. */\n  DESC: \"desc\",\n")
	output.WriteString("} as const\n")
	output.WriteString("/** Sort direction value accepted by generated sort inputs. */\n")
	output.WriteString("export type SortDirection = (typeof SortDirection)[keyof typeof SortDirection]\n\n")
	output.WriteString("/** Query controls for cursor-based pagination. */\n")
	output.WriteString("export type CursorPaginationInput = {\n")
	output.WriteString("  /** Opaque cursor returned by the previous page. Omit for the first page. */\n  readonly cursor?: string | undefined\n")
	output.WriteString("  /** Maximum number of items requested for one page. */\n  readonly limit?: number | undefined\n")
	output.WriteString("  /** Offset pagination is unavailable in cursor mode. */\n  readonly offset?: never\n")
	output.WriteString("}\n")
	output.WriteString("/** Query controls for offset-based pagination. */\n")
	output.WriteString("export type OffsetPaginationInput = {\n")
	output.WriteString("  /** Zero-based index of the first requested item. */\n  readonly offset?: number | undefined\n")
	output.WriteString("  /** Maximum number of items requested for one page. */\n  readonly limit?: number | undefined\n")
	output.WriteString("  /** Cursor pagination is unavailable in offset mode. */\n  readonly cursor?: never\n")
	output.WriteString("}\n")
	output.WriteString("/** Query controls for an operation supporting either cursor or offset pagination. */\n")
	output.WriteString("export type BothPaginationInput = CursorPaginationInput | OffsetPaginationInput\n\n")

	reachable := reachableComponentSchemas(document)
	names := make([]string, 0, len(reachable))
	for name := range reachable {
		names = append(names, name)
	}
	sort.Strings(names)
	if err := validateComponentSymbols(document, names); err != nil {
		return nil, err
	}

	var exported []string
	for _, schemaName := range names {
		schema := document.ComponentSchemas[schemaName]
		if isErrorSchema(document, schema) {
			continue
		}
		declarations := componentDeclarations(schemaName, schema)
		for _, declaration := range declarations {
			typeName, err := naming.Public(declaration.name)
			if err != nil {
				return nil, fmt.Errorf("component %s: %w", schemaName, err)
			}
			typeSource, err := schemaType(document, schema, declaration.projection)
			if err != nil {
				return nil, fmt.Errorf("component %s: %w", schemaName, err)
			}
			emitSchemaJSDoc(&output, schemaName, schema, declaration.projection)
			fmt.Fprintf(&output, "export type %s = %s\n\n", typeName, typeSource)
			if enumValues, ok := stringEnum(schema); ok {
				fmt.Fprintf(&output, "/** Runtime values for {@link %s}. */\n", typeName)
				fmt.Fprintf(&output, "export const %s = {\n", typeName)
				enumKeys := make(map[string]string, len(enumValues))
				for _, value := range enumValues {
					key, err := naming.Public(value)
					if err != nil {
						return nil, fmt.Errorf("enum %s value %q: %w", schemaName, value, err)
					}
					if previous, exists := enumKeys[key]; exists {
						return nil, fmt.Errorf("enum %s values %q and %q both generate TypeScript key %q", schemaName, previous, value, key)
					}
					enumKeys[key] = value
					fmt.Fprintf(&output, "  /** OpenAPI enum value `%s`. */\n", sanitizeComment(value))
					fmt.Fprintf(&output, "  %s: %s,\n", key, quoteTS(value))
				}
				output.WriteString("} as const\n\n")
			}
			exported = append(exported, typeName)
		}
	}

	output.WriteString("/** Map of every public OpenAPI component name to its generated TypeScript type. */\n")
	output.WriteString("export interface Components {\n")
	for _, name := range exported {
		fmt.Fprintf(&output, "  /** {@link %s} */\n", name)
		fmt.Fprintf(&output, "  readonly %s: %s\n", name, name)
	}
	output.WriteString("}\n")
	return output.Bytes(), nil
}

func validateComponentSymbols(document *ir.Document, names []string) error {
	symbols := map[string]string{
		"SortDirection":         "built-in SortDirection",
		"CursorPaginationInput": "built-in CursorPaginationInput",
		"OffsetPaginationInput": "built-in OffsetPaginationInput",
		"BothPaginationInput":   "built-in BothPaginationInput",
		"Components":            "built-in Components",
	}
	for _, schemaName := range names {
		schema := document.ComponentSchemas[schemaName]
		if isErrorSchema(document, schema) {
			continue
		}
		for _, declaration := range componentDeclarations(schemaName, schema) {
			symbol, err := naming.Public(declaration.name)
			if err != nil {
				return fmt.Errorf("component %s: %w", schemaName, err)
			}
			if previous, exists := symbols[symbol]; exists {
				return fmt.Errorf("component %q generates TypeScript symbol %q already used by %s", schemaName, symbol, previous)
			}
			symbols[symbol] = "component " + fmt.Sprintf("%q", schemaName)
		}
	}
	return nil
}

func reachableComponentSchemas(document *ir.Document) map[string]bool {
	visible := make(map[string]bool)
	hidden := make(map[string]bool)
	var visit func(any, map[string]bool)
	visit = func(value any, found map[string]bool) {
		switch typed := value.(type) {
		case map[string]any:
			if reference, _ := typed["$ref"].(string); reference != "" {
				name, err := componentSchemaReferenceName(reference)
				if err != nil {
					break
				}
				if !found[name] {
					found[name] = true
					visit(document.ComponentSchemas[name], found)
				}
			}
			for key, item := range typed {
				if key != "$ref" {
					visit(item, found)
				}
			}
		case []any:
			for _, item := range typed {
				visit(item, found)
			}
		}
	}
	for _, operation := range document.Operations {
		found := visible
		if operation.Visibility == "hidden" {
			found = hidden
		}
		visit(operation.Raw, found)
		visit(operation.PathItemRaw["parameters"], found)
	}
	visit(document.Raw["webhooks"], visible)
	components, _ := document.Raw["components"].(map[string]any)
	visit(components["callbacks"], visible)
	result := make(map[string]bool, len(document.ComponentSchemas))
	for name := range document.ComponentSchemas {
		if !hidden[name] || visible[name] {
			result[name] = true
		}
	}
	return result
}

func emitSchemaJSDoc(output *bytes.Buffer, schemaName string, schema map[string]any, direction projection) {
	fallback := "OpenAPI component `" + sanitizeComment(schemaName) + "`."
	if direction == projectionInput {
		fallback = "Input representation of OpenAPI component `" + sanitizeComment(schemaName) + "`."
	} else if direction == projectionOutput {
		fallback = "Output representation of OpenAPI component `" + sanitizeComment(schemaName) + "`."
	}
	emitSchemaValueJSDoc(output, "", schema, fallback)
}

func emitSchemaValueJSDoc(output *bytes.Buffer, indent string, schema map[string]any, fallback string) {
	description, _ := schema["description"].(string)
	format, _ := schema["format"].(string)
	defaultValue, hasDefault := schema["default"]
	deprecated, _ := schema["deprecated"].(bool)
	fmt.Fprintf(output, "%s/**\n", indent)
	if description != "" {
		fmt.Fprintf(output, "%s * %s\n", indent, sanitizeComment(description))
	} else {
		fmt.Fprintf(output, "%s * %s\n", indent, fallback)
	}
	if format != "" {
		fmt.Fprintf(output, "%s * Format: `%s`.\n", indent, sanitizeComment(format))
	}
	if boolValue(schema, "readOnly") {
		fmt.Fprintf(output, "%s * Present in responses; omitted from generated input projections.\n", indent)
	}
	if boolValue(schema, "writeOnly") {
		fmt.Fprintf(output, "%s * Accepted in requests; omitted from generated output projections.\n", indent)
	}
	if hasDefault {
		fmt.Fprintf(output, "%s * @default %s\n", indent, literalTS(defaultValue))
	}
	if deprecated {
		fmt.Fprintf(output, "%s * @deprecated This OpenAPI value is deprecated.\n", indent)
	}
	if constraints := schemaConstraintSummary(schema); constraints != "" {
		fmt.Fprintf(output, "%s * Constraints: %s.\n", indent, constraints)
	}
	fmt.Fprintf(output, "%s */\n", indent)
}

// schemaConstraintSummary keeps validation-only Schema Object keywords visible
// in generated source. TypeScript cannot encode every JSON Schema predicate in
// a static type, so the generated declaration records the contract rather than
// silently discarding it.
func schemaConstraintSummary(schema map[string]any) string {
	keys := []string{
		"multipleOf", "maximum", "exclusiveMaximum", "minimum", "exclusiveMinimum",
		"maxLength", "minLength", "pattern", "maxItems", "minItems", "uniqueItems",
		"contains", "minContains", "maxContains", "maxProperties", "minProperties",
		"dependentRequired", "propertyNames", "unevaluatedItems", "unevaluatedProperties",
		"contentEncoding", "contentMediaType", "contentSchema",
	}
	items := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, ok := schema[key]; ok {
			items = append(items, "`"+key+"="+sanitizeComment(literalTS(value))+"`")
		}
	}
	return strings.Join(items, ", ")
}

type componentDeclaration struct {
	name       string
	projection projection
}

func componentDeclarations(name string, schema map[string]any) []componentDeclaration {
	if !isStructuralSchema(schema) || isDirectionNeutral(name) {
		return []componentDeclaration{{name: name, projection: projectionNeutral}}
	}
	if strings.HasSuffix(name, "Input") {
		return []componentDeclaration{{name: name, projection: projectionInput}}
	}
	if strings.HasSuffix(name, "Output") {
		return []componentDeclaration{{name: name, projection: projectionOutput}}
	}
	return []componentDeclaration{
		{name: name + "Input", projection: projectionInput},
		{name: name + "Output", projection: projectionOutput},
	}
}

func schemaType(document *ir.Document, value any, direction projection) (string, error) {
	if boolean, ok := value.(bool); ok {
		if boolean {
			return "unknown", nil
		}
		return "never", nil
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return "unknown", nil
	}
	// OpenAPI 3.0 expresses nullability independently of `type`, unlike the
	// JSON Schema type array used by OpenAPI 3.1 and 3.2.
	if document.OpenAPIVersionLine != "3.1" && document.OpenAPIVersionLine != "3.2" && boolValue(schema, "nullable") {
		withoutNullable := make(map[string]any, len(schema)-1)
		for key, value := range schema {
			if key != "nullable" {
				withoutNullable[key] = value
			}
		}
		value, err := schemaType(document, withoutNullable, direction)
		if err != nil {
			return "", err
		}
		return addUnionMember(value, "null"), nil
	}
	if dynamicReference, ok := schema["x-sdkgen-dynamic-reference"].(map[string]any); ok {
		reference, _ := dynamicReference["reference"].(string)
		name, err := componentSchemaReferenceName(reference)
		if err != nil {
			return "", err
		}
		referenced, err := referencedType(document, name, direction)
		if err != nil || len(schema) == 1 {
			return referenced, err
		}
		siblings := make(map[string]any, len(schema)-1)
		for key, value := range schema {
			if key != "x-sdkgen-dynamic-reference" && isTypeAffectingSchemaKeyword(key) {
				siblings[key] = value
			}
		}
		if len(siblings) == 0 {
			return referenced, nil
		}
		siblingType, err := schemaType(document, siblings, direction)
		if err != nil {
			return "", err
		}
		return "(" + referenced + ") & (" + siblingType + ")", nil
	}
	if reference, _ := schema["$ref"].(string); reference != "" {
		name, err := componentSchemaReferenceName(reference)
		if err != nil {
			return "", err
		}
		referenced, err := referencedType(document, name, direction)
		if err != nil || len(schema) == 1 {
			return referenced, err
		}
		siblings := make(map[string]any, len(schema)-1)
		for key, value := range schema {
			if key != "$ref" && isTypeAffectingSchemaKeyword(key) {
				siblings[key] = value
			}
		}
		if len(siblings) == 0 {
			return referenced, nil
		}
		siblingType, err := schemaType(document, siblings, direction)
		if err != nil {
			return "", err
		}
		return "(" + referenced + ") & (" + siblingType + ")", nil
	}
	if value, exists := schema["const"]; exists {
		return literalTS(value), nil
	}
	if values, ok := schema["enum"].([]any); ok && len(values) > 0 {
		quoted := make([]string, 0, len(values))
		for _, value := range values {
			quoted = append(quoted, literalTS(value))
		}
		return strings.Join(quoted, " | "), nil
	}
	if variants, ok := schema["oneOf"].([]any); ok {
		return unionType(document, variants, direction)
	}
	if variants, ok := schema["anyOf"].([]any); ok {
		return unionType(document, variants, direction)
	}
	if variants, ok := schema["allOf"].([]any); ok {
		parts, err := schemaListTypes(document, variants, direction)
		if err != nil {
			return "", err
		}
		return strings.Join(parts, " & "), nil
	}

	types := schemaTypes(schema["type"])
	if len(types) > 1 {
		parts := make([]string, 0, len(types))
		for _, value := range types {
			part, err := scalarOrCompositeType(document, value, schema, direction)
			if err != nil {
				return "", err
			}
			parts = append(parts, part)
		}
		return strings.Join(uniqueStrings(parts), " | "), nil
	}
	if len(types) == 0 {
		if _, exists := schema["properties"]; exists {
			return objectType(document, schema, direction)
		}
		if _, exists := schema["additionalProperties"]; exists {
			return objectType(document, schema, direction)
		}
		if _, exists := schema["prefixItems"]; exists {
			return arrayType(document, schema, direction)
		}
		return "unknown", nil
	}
	return scalarOrCompositeType(document, types[0], schema, direction)
}

func isTypeAffectingSchemaKeyword(key string) bool {
	switch key {
	case "type", "const", "enum", "oneOf", "anyOf", "allOf", "properties", "required", "additionalProperties", "items", "prefixItems":
		return true
	default:
		return false
	}
}

func scalarOrCompositeType(document *ir.Document, kind string, schema map[string]any, direction projection) (string, error) {
	switch kind {
	case "string":
		return "string", nil
	case "integer", "number":
		return "number", nil
	case "boolean":
		return "boolean", nil
	case "null":
		return "null", nil
	case "array":
		return arrayType(document, schema, direction)
	case "object":
		return objectType(document, schema, direction)
	default:
		return "unknown", nil
	}
}

func arrayType(document *ir.Document, schema map[string]any, direction projection) (string, error) {
	if prefixItems, ok := schema["prefixItems"].([]any); ok && len(prefixItems) > 0 {
		parts, err := schemaListTypes(document, prefixItems, direction)
		if err != nil {
			return "", err
		}
		if items, exists := schema["items"]; exists {
			itemType, err := schemaType(document, items, direction)
			if err != nil {
				return "", err
			}
			parts = append(parts, "...("+itemType+")[]")
		} else {
			// JSON Schema allows unconstrained trailing items when `items` is
			// absent. Preserve that openness instead of producing a closed tuple.
			parts = append(parts, "...unknown[]")
		}
		return "readonly [" + strings.Join(parts, ", ") + "]", nil
	}
	items, exists := schema["items"]
	if !exists {
		return "readonly unknown[]", nil
	}
	itemType, err := schemaType(document, items, direction)
	if err != nil {
		return "", err
	}
	return "readonly (" + itemType + ")[]", nil
}

func objectType(document *ir.Document, schema map[string]any, direction projection) (string, error) {
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) == 0 {
		if additional, exists := schema["additionalProperties"]; exists {
			valueType, err := schemaType(document, additional, direction)
			if err != nil {
				return "", err
			}
			return "Readonly<Record<string, " + valueType + ">>", nil
		}
		if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
			return "Readonly<Record<string, never>>", nil
		}
		return "Readonly<Record<string, unknown>>", nil
	}
	required := make(map[string]bool)
	if values, ok := schema["required"].([]any); ok {
		for _, value := range values {
			if name, ok := value.(string); ok {
				required[name] = true
			}
		}
	}
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var output bytes.Buffer
	output.WriteString("{\n")
	propertySources := make(map[string]string, len(keys))
	for _, wireName := range keys {
		propertyValue := properties[wireName]
		propertySchema, _ := propertyValue.(map[string]any)
		if direction == projectionInput && boolValue(propertySchema, "readOnly") {
			continue
		}
		if direction == projectionOutput && boolValue(propertySchema, "writeOnly") {
			continue
		}
		propertyType, err := schemaType(document, propertyValue, direction)
		if err != nil {
			return "", err
		}
		propertyName, err := naming.Property(wireName)
		if err != nil {
			propertyName = quoteTS(wireName)
		}
		if previous, exists := propertySources[propertyName]; exists {
			return "", fmt.Errorf("object properties %q and %q both generate TypeScript property %s", previous, wireName, propertyName)
		}
		propertySources[propertyName] = wireName
		optional := ""
		if !required[wireName] {
			optional = "?"
			if direction == projectionInput {
				propertyType += " | undefined"
			}
		}
		emitSchemaValueJSDoc(&output, "  ", propertySchema, "OpenAPI property `"+sanitizeComment(wireName)+"`.")
		fmt.Fprintf(&output, "  readonly %s%s: %s\n", propertyName, optional, propertyType)
	}
	output.WriteString("}")
	additional, err := objectAdditionalType(document, schema, direction)
	if err != nil {
		return "", err
	}
	if additional == "" {
		return output.String(), nil
	}
	return "(" + output.String() + ") & (Readonly<Record<string, " + additional + ">>)", nil
}

// objectAdditionalType is intentionally conservative for patternProperties:
// TypeScript has no regex-key type, so every additional key is represented by
// the union of the applicable pattern schemas. The exact patterns stay in the
// generated documentation/contract metadata.
func objectAdditionalType(document *ir.Document, schema map[string]any, direction projection) (string, error) {
	var values []string
	if additional, exists := schema["additionalProperties"]; exists {
		if boolean, ok := additional.(bool); ok && !boolean {
			return "", nil
		}
		value, err := schemaType(document, additional, direction)
		if err != nil {
			return "", err
		}
		values = append(values, value)
	}
	patterns, _ := schema["patternProperties"].(map[string]any)
	patternNames := make([]string, 0, len(patterns))
	for pattern := range patterns {
		patternNames = append(patternNames, pattern)
	}
	sort.Strings(patternNames)
	for _, pattern := range patternNames {
		typeValue, err := schemaType(document, patterns[pattern], direction)
		if err != nil {
			return "", err
		}
		values = append(values, typeValue)
	}
	return strings.Join(uniqueStrings(values), " | "), nil
}

func referencedType(document *ir.Document, name string, direction projection) (string, error) {
	schema, exists := document.ComponentSchemas[name]
	if !exists {
		if compiled, ok := document.Schemas[name]; ok {
			if boolean, ok := compiled.Value.(bool); ok {
				if boolean {
					return "unknown", nil
				}
				return "never", nil
			}
		}
		return naming.Public(name)
	}
	declarations := componentDeclarations(name, schema)
	selected := declarations[0]
	if len(declarations) > 1 {
		if direction == projectionInput {
			selected = declarations[0]
		} else {
			selected = declarations[1]
		}
	}
	return naming.Public(selected.name)
}

func isSuccessResponseStatus(status string) bool {
	return status == "default" || strings.HasPrefix(status, "2")
}

func operationOutputType(document *ir.Document, operation ir.Operation) (string, error) {
	responses, _ := operation.Raw["responses"].(map[string]any)
	statusCodes := make([]string, 0, len(responses))
	for status := range responses {
		if isSuccessResponseStatus(status) {
			statusCodes = append(statusCodes, status)
		}
	}
	sort.Strings(statusCodes)
	var result []string
	for _, status := range statusCodes {
		response, _ := responses[status].(map[string]any)
		response, err := resolveComponentObject(document, response, "responses")
		if err != nil {
			return "", err
		}
		content, _ := response["content"].(map[string]any)
		if len(content) == 0 {
			result = append(result, "void")
			continue
		}
		mediaTypes := make([]string, 0, len(content))
		for mediaType := range content {
			mediaTypes = append(mediaTypes, mediaType)
		}
		sort.Strings(mediaTypes)
		for _, mediaType := range mediaTypes {
			media, _ := content[mediaType].(map[string]any)
			media, err := resolveMediaTypeObject(document, media)
			if err != nil {
				return "", err
			}
			schema, hasSchema := media["schema"].(map[string]any)
			if !hasSchema {
				// A Media Type Object without a Schema Object still has a body.
				// Its shape is unconstrained, not absent.
				result = append(result, "unknown")
				continue
			}
			if isBinaryMedia(mediaType, schema) {
				result = append(result, "ReadableStream<Uint8Array>")
				continue
			}
			if isTextMedia(mediaType) {
				result = append(result, "string")
				continue
			}
			if operation.Envelope == "data" {
				if dataSchema := envelopeDataSchema(document, schema, make(map[string]bool)); len(dataSchema) > 0 {
					schema = dataSchema
				}
			}
			valueType, err := schemaType(document, schema, projectionOutput)
			if err != nil {
				return "", err
			}
			result = append(result, valueType)
		}
	}
	if len(result) == 0 {
		return "void", nil
	}
	return strings.Join(uniqueStrings(result), " | "), nil
}

func operationRawResponseType(document *ir.Document, operation ir.Operation) (string, error) {
	responses, _ := operation.Raw["responses"].(map[string]any)
	statusCodes := make([]string, 0, len(responses))
	for status := range responses {
		if isSuccessResponseStatus(status) {
			statusCodes = append(statusCodes, status)
		}
	}
	sort.Strings(statusCodes)
	var result []string
	for _, status := range statusCodes {
		response, _ := responses[status].(map[string]any)
		response, err := resolveComponentObject(document, response, "responses")
		if err != nil {
			return "", err
		}
		statusType := "number"
		if len(status) == 3 && status[0] >= '0' && status[0] <= '9' && status[1] >= '0' && status[1] <= '9' && status[2] >= '0' && status[2] <= '9' {
			statusType = status
		}
		content, _ := response["content"].(map[string]any)
		headerType, err := responseHeaderType(document, response)
		if err != nil {
			return "", err
		}
		if len(content) == 0 {
			result = append(result, "RawResponseFor<"+statusType+", undefined, void, "+headerType+">")
			continue
		}
		mediaTypes := make([]string, 0, len(content))
		for mediaType := range content {
			mediaTypes = append(mediaTypes, mediaType)
		}
		sort.Strings(mediaTypes)
		for _, mediaType := range mediaTypes {
			media, _ := content[mediaType].(map[string]any)
			media, err := resolveMediaTypeObject(document, media)
			if err != nil {
				return "", err
			}
			schema := media["schema"]
			schemaObject, _ := schema.(map[string]any)
			valueType := "void"
			if schema != nil {
				if isBinaryMedia(mediaType, schemaObject) {
					valueType = "ReadableStream<Uint8Array>"
				} else if isTextMedia(mediaType) {
					valueType = "string"
				} else {
					if operation.Envelope == "data" {
						if dataSchema := envelopeDataSchema(document, schemaObject, make(map[string]bool)); len(dataSchema) > 0 {
							schema = dataSchema
						}
					}
					valueType, err = schemaType(document, schema, projectionOutput)
					if err != nil {
						return "", err
					}
				}
			}
			result = append(result, "RawResponseFor<"+statusType+", "+quoteTS(mediaType)+", "+valueType+", "+headerType+">")
		}
	}
	if len(result) == 0 {
		return "RawResponseFor<number, string | undefined, void>", nil
	}
	return strings.Join(uniqueStrings(result), " | "), nil
}

func responseHeaderType(document *ir.Document, response map[string]any) (string, error) {
	headers, _ := response["headers"].(map[string]any)
	if len(headers) == 0 {
		return "Readonly<Record<string, never>>", nil
	}
	names := sortedAnyKeys(headers)
	fields := make([]string, 0, len(names))
	for _, name := range names {
		header, _ := headers[name].(map[string]any)
		resolved, err := resolveComponentObject(document, header, "headers")
		if err != nil {
			return "", err
		}
		schema, _, err := responseHeaderSchema(document, resolved)
		if err != nil {
			return "", err
		}
		valueType, err := schemaType(document, schema, projectionOutput)
		if err != nil {
			return "", err
		}
		property, err := naming.Property(name)
		if err != nil {
			property = quoteTS(name)
		}
		optional := "?"
		if boolValue(resolved, "required") {
			optional = ""
		}
		fields = append(fields, "readonly "+property+optional+": "+valueType)
	}
	return "{ " + strings.Join(fields, "; ") + " }", nil
}

func responseHeaderSchema(document *ir.Document, header map[string]any) (any, string, error) {
	content, _ := header["content"].(map[string]any)
	if len(content) == 0 {
		return header["schema"], "", nil
	}
	if len(content) != 1 {
		return nil, "", fmt.Errorf("Header Object content must define exactly one media type")
	}
	mediaType := sortedAnyKeys(content)[0]
	media, _ := content[mediaType].(map[string]any)
	media, err := resolveMediaTypeObject(document, media)
	if err != nil {
		return nil, "", err
	}
	return media["schema"], mediaType, nil
}

func operationMediaOutputTypes(document *ir.Document, operation ir.Operation) (map[string]string, error) {
	responses, _ := operation.Raw["responses"].(map[string]any)
	statusCodes := make([]string, 0, len(responses))
	for status := range responses {
		if isSuccessResponseStatus(status) {
			statusCodes = append(statusCodes, status)
		}
	}
	sort.Strings(statusCodes)
	byMedia := make(map[string][]string)
	for _, status := range statusCodes {
		response, _ := responses[status].(map[string]any)
		response, err := resolveComponentObject(document, response, "responses")
		if err != nil {
			return nil, err
		}
		content, _ := response["content"].(map[string]any)
		for mediaType, value := range content {
			media, _ := value.(map[string]any)
			media, err := resolveMediaTypeObject(document, media)
			if err != nil {
				return nil, err
			}
			schema := media["schema"]
			schemaObject, _ := schema.(map[string]any)
			valueType := "void"
			if schema != nil {
				if isBinaryMedia(mediaType, schemaObject) {
					valueType = "ReadableStream<Uint8Array>"
				} else if isTextMedia(mediaType) {
					valueType = "string"
				} else {
					if operation.Envelope == "data" {
						if dataSchema := envelopeDataSchema(document, schemaObject, make(map[string]bool)); len(dataSchema) > 0 {
							schema = dataSchema
						}
					}
					valueType, err = schemaType(document, schema, projectionOutput)
					if err != nil {
						return nil, err
					}
				}
			}
			byMedia[mediaType] = append(byMedia[mediaType], valueType)
		}
	}
	result := make(map[string]string, len(byMedia))
	for mediaType, values := range byMedia {
		result[mediaType] = strings.Join(uniqueStrings(values), " | ")
	}
	return result, nil
}

func isBinaryMedia(mediaType string, schema map[string]any) bool {
	mediaType = strings.ToLower(mediaType)
	if strings.HasPrefix(mediaType, "text/") || isJSONMediaType(mediaType) || strings.Contains(mediaType, "xml") {
		return false
	}
	format, _ := schema["format"].(string)
	contentEncoding, _ := schema["contentEncoding"].(string)
	return format == "binary" || contentEncoding == "binary" || mediaType == "application/octet-stream"
}

func isTextMedia(mediaType string) bool {
	mediaType = strings.ToLower(mediaType)
	return strings.HasPrefix(mediaType, "text/") && !strings.Contains(mediaType, "xml")
}

func isJSONMediaType(mediaType string) bool {
	mediaType = strings.TrimSpace(strings.SplitN(strings.ToLower(mediaType), ";", 2)[0])
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func envelopeDataSchema(document *ir.Document, schema map[string]any, seen map[string]bool) map[string]any {
	if reference, _ := schema["$ref"].(string); reference != "" {
		name, err := componentSchemaReferenceName(reference)
		if err != nil {
			return nil
		}
		if seen[name] {
			return nil
		}
		seen[name] = true
		return envelopeDataSchema(document, document.ComponentSchemas[name], seen)
	}
	properties, _ := schema["properties"].(map[string]any)
	data, _ := properties["data"].(map[string]any)
	return data
}

func operationSuccessSchema(document *ir.Document, operation ir.Operation) (map[string]any, bool, error) {
	responses, _ := operation.Raw["responses"].(map[string]any)
	statusCodes := make([]string, 0, len(responses))
	for status := range responses {
		if isSuccessResponseStatus(status) {
			statusCodes = append(statusCodes, status)
		}
	}
	sort.Strings(statusCodes)
	for _, status := range statusCodes {
		response, _ := responses[status].(map[string]any)
		response, err := resolveComponentObject(document, response, "responses")
		if err != nil {
			return nil, false, err
		}
		content, _ := response["content"].(map[string]any)
		mediaTypes := make([]string, 0, len(content))
		for mediaType := range content {
			mediaTypes = append(mediaTypes, mediaType)
		}
		sort.Strings(mediaTypes)
		for _, mediaType := range mediaTypes {
			media, _ := content[mediaType].(map[string]any)
			media, err := resolveMediaTypeObject(document, media)
			if err != nil {
				return nil, false, err
			}
			schema, _ := media["schema"].(map[string]any)
			if len(schema) == 0 {
				continue
			}
			return schema, true, nil
		}
		return nil, false, nil
	}
	return nil, false, nil
}

func operationItemType(document *ir.Document, operation ir.Operation) (string, error) {
	schema, found, err := operationSuccessSchema(document, operation)
	if err != nil {
		return "", err
	}
	if !found {
		return "unknown", nil
	}
	items := findItemsSchema(document, schema, make(map[string]bool))
	if len(items) == 0 {
		return "unknown", nil
	}
	return schemaType(document, items, projectionOutput)
}

func findItemsSchema(document *ir.Document, schema map[string]any, seen map[string]bool) map[string]any {
	if reference, _ := schema["$ref"].(string); reference != "" {
		name, err := componentSchemaReferenceName(reference)
		if err != nil {
			return nil
		}
		if seen[name] {
			return nil
		}
		seen[name] = true
		return findItemsSchema(document, document.ComponentSchemas[name], seen)
	}
	properties, _ := schema["properties"].(map[string]any)
	if itemsProperty, _ := properties["items"].(map[string]any); len(itemsProperty) > 0 {
		if items, _ := itemsProperty["items"].(map[string]any); len(items) > 0 {
			return items
		}
	}
	for _, key := range []string{"data", "result"} {
		if nested, _ := properties[key].(map[string]any); len(nested) > 0 {
			if result := findItemsSchema(document, nested, seen); len(result) > 0 {
				return result
			}
		}
	}
	return nil
}

func operationInputTypes(document *ir.Document, operation ir.Operation) ([]string, error) {
	var result []string
	name := operationTypeName(operation.OperationID)
	if parameters, err := parametersIn(document, operation, "path"); err != nil {
		return nil, err
	} else if len(parameters) > 0 {
		result = append(result, name+"PathInput")
	}
	if parameters, err := queryParameters(document, operation); err != nil {
		return nil, err
	} else if len(parameters) > 0 || operation.Pagination != "" || operation.Raw["x-sort"] != nil {
		result = append(result, name+"QueryInput")
	}
	if parameters, err := parametersIn(document, operation, "header"); err != nil {
		return nil, err
	} else if len(parameters) > 0 {
		result = append(result, name+"HeaderInput")
	}
	if parameters, err := parametersIn(document, operation, "cookie"); err != nil {
		return nil, err
	} else if len(parameters) > 0 {
		result = append(result, name+"CookieInput")
	}
	if body, ok := operation.Raw["requestBody"].(map[string]any); ok {
		if _, err := resolveComponentObject(document, body, "requestBodies"); err != nil {
			return nil, err
		}
		result = append(result, name+"BodyInput")
	}
	return result, nil
}

func isStructuralSchema(schema map[string]any) bool {
	for _, value := range schemaTypes(schema["type"]) {
		if value == "object" || value == "array" {
			return true
		}
	}
	_, hasProperties := schema["properties"]
	_, hasOneOf := schema["oneOf"]
	_, hasAllOf := schema["allOf"]
	return hasProperties || hasOneOf || hasAllOf
}

func isDirectionNeutral(name string) bool {
	return strings.HasSuffix(name, "Error") || strings.HasSuffix(name, "Code") || name == "Pagination"
}

func schemaTypes(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		result := make([]string, 0, len(typed))
		for _, value := range typed {
			if text, ok := value.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func addUnionMember(value, member string) string {
	for _, item := range strings.Split(value, " | ") {
		if item == member {
			return value
		}
	}
	return value + " | " + member
}

func schemaEnum(schema map[string]any) []string {
	values, _ := schema["enum"].([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func stringEnum(schema map[string]any) ([]string, bool) {
	values, exists := schema["enum"].([]any)
	if !exists || len(values) == 0 {
		return nil, false
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, false
		}
		result = append(result, text)
	}
	return result, true
}

func unionType(document *ir.Document, variants []any, direction projection) (string, error) {
	parts, err := schemaListTypes(document, variants, direction)
	if err != nil {
		return "", err
	}
	return strings.Join(uniqueStrings(parts), " | "), nil
}

func schemaListTypes(document *ir.Document, variants []any, direction projection) ([]string, error) {
	parts := make([]string, 0, len(variants))
	for _, variant := range variants {
		schema, _ := variant.(map[string]any)
		part, err := schemaType(document, schema, direction)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func literalTS(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "unknown"
	}
	return string(data)
}

func quoteTS(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func sanitizeComment(value string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(value), " "), "*/", "* /")
}
