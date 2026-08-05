package typescript

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"openapi-sdkgen/internal/compiler/ir"
)

type enumCatalogPlan struct {
	name           string
	valuesBinding  string
	catalogBinding string
	renderedValues string
	tupleType      string
	valueType      string
	members        []string
}

func emitEnums(document *ir.Document) ([]byte, error) {
	catalogs, err := enumCatalogPlans(document)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if len(catalogs) > 0 {
		output.WriteString(`function __sdkgen_createEnumCatalog(values: readonly unknown[]): object {
  const catalog = Object.create(null) as Record<PropertyKey, unknown>
  for (const value of values) {
    if (typeof value !== "string" || Object.hasOwn(catalog, value)) continue
    Object.defineProperty(catalog, value, { enumerable: true, value })
  }
  Object.defineProperty(catalog, Symbol.iterator, { value: () => values[Symbol.iterator]() })
  return Object.freeze(catalog)
}

function __sdkgen_enumValueEquals(left: unknown, right: unknown, seen = new WeakMap<object, WeakSet<object>>()): boolean {
  if (typeof left === "number" && typeof right === "number") return left === right
  if (typeof left !== "object" || left === null || typeof right !== "object" || right === null) return Object.is(left, right)
  const leftArray = Array.isArray(left)
  const rightArray = Array.isArray(right)
  if (leftArray !== rightArray) return false
  if (!leftArray) {
    const leftPrototype = Object.getPrototypeOf(left)
    const rightPrototype = Object.getPrototypeOf(right)
    if ((leftPrototype !== Object.prototype && leftPrototype !== null) || (rightPrototype !== Object.prototype && rightPrototype !== null)) return false
  }
  let compared = seen.get(left)
  if (compared?.has(right)) return false
  if (compared === undefined) {
    compared = new WeakSet<object>()
    seen.set(left, compared)
  }
  compared.add(right)
  if (leftArray && rightArray) {
    if (left.length !== right.length || Object.keys(left).length !== left.length || Object.keys(right).length !== right.length) return false
    for (let index = 0; index < left.length; index++) {
      const leftItem = Object.getOwnPropertyDescriptor(left, index)
      const rightItem = Object.getOwnPropertyDescriptor(right, index)
      if (leftItem === undefined || rightItem === undefined || !("value" in leftItem) || !("value" in rightItem) || !__sdkgen_enumValueEquals(leftItem.value, rightItem.value, seen)) return false
    }
    return true
  }
  const leftKeys = Object.keys(left).sort()
  const rightKeys = Object.keys(right).sort()
  if (leftKeys.length !== rightKeys.length) return false
  for (let index = 0; index < leftKeys.length; index++) {
    const key = leftKeys[index]
    if (key !== rightKeys[index]) return false
    const leftItem = Object.getOwnPropertyDescriptor(left, key)
    const rightItem = Object.getOwnPropertyDescriptor(right, key)
    if (leftItem === undefined || rightItem === undefined || !("value" in leftItem) || !("value" in rightItem) || !__sdkgen_enumValueEquals(leftItem.value, rightItem.value, seen)) return false
  }
  return true
}

`)
	}
	bindings := make([]runtimeProperty, 0, len(catalogs))
	for _, catalog := range catalogs {
		fmt.Fprintf(&output, "const %s = %s as unknown as %s\n", catalog.valuesBinding, catalog.renderedValues, catalog.tupleType)
		fmt.Fprintf(&output, "const %s = __sdkgen_createEnumCatalog(%s)\n", catalog.catalogBinding, catalog.valuesBinding)
		bindings = append(bindings, runtimeProperty{key: catalog.name, value: catalog.catalogBinding})
	}
	output.WriteString("/** Runtime enum values keyed by exact OpenAPI component schema names. */\n")
	fmt.Fprintf(&output, "export const Enums = %s as {\n", runtimeObjectExpression(bindings))
	for _, catalog := range catalogs {
		fmt.Fprintf(&output, "  /** Values declared by OpenAPI component `%s`. */\n", sanitizeComment(catalog.name))
		fmt.Fprintf(&output, "  readonly %s: {\n", quoteTS(catalog.name))
		for _, member := range catalog.members {
			fmt.Fprintf(&output, "    /** Exact string value `%s`. */\n", sanitizeComment(member))
			fmt.Fprintf(&output, "    readonly %s: %s\n", quoteTS(member), quoteTS(member))
		}
		fmt.Fprintf(&output, "    [Symbol.iterator](): IterableIterator<%s>\n", catalog.valueType)
		output.WriteString("  }\n")
	}
	output.WriteString("}\n")
	if len(catalogs) > 0 {
		output.WriteString("/** Literal value union for an exact generated enum component name. */\n")
		output.WriteString("export type EnumValue<Name extends keyof typeof Enums> = (typeof Enums)[Name] extends Iterable<infer Value> ? Value : never\n")
		output.WriteString("/** Return whether a runtime value structurally matches a generated enum value. */\n")
		output.WriteString("export function isEnumValue<Catalog extends (typeof Enums)[keyof typeof Enums]>(\n")
		output.WriteString("  catalog: Catalog,\n")
		output.WriteString("  value: unknown,\n")
		output.WriteString("): value is Catalog extends Iterable<infer Value> ? Value : never {\n")
		output.WriteString("  try {\n")
		output.WriteString("    for (const candidate of catalog) {\n")
		output.WriteString("      if (__sdkgen_enumValueEquals(candidate, value)) return true\n")
		output.WriteString("    }\n")
		output.WriteString("  } catch {\n")
		output.WriteString("    return false\n")
		output.WriteString("  }\n")
		output.WriteString("  return false\n")
		output.WriteString("}\n")
	}
	return output.Bytes(), nil
}

func enumCatalogPlans(document *ir.Document) ([]enumCatalogPlan, error) {
	reachable := reachableComponentSchemas(document)
	names := make([]string, 0, len(reachable))
	for name := range reachable {
		names = append(names, name)
	}
	sort.Strings(names)
	catalogs := make([]enumCatalogPlan, 0)
	for _, schemaName := range names {
		schema, ok := componentSchemaValue(document, schemaName).(map[string]any)
		if !ok {
			continue
		}
		values, exists := schema["enum"].([]any)
		if !exists {
			continue
		}
		rendered, err := runtimeJSONExpression(values)
		if err != nil {
			return nil, fmt.Errorf("component %s enum: %w", schemaName, err)
		}
		tupleType, err := readonlyJSONType(values)
		if err != nil {
			return nil, fmt.Errorf("component %s enum type: %w", schemaName, err)
		}
		valueType, err := enumValueType(values)
		if err != nil {
			return nil, fmt.Errorf("component %s enum value type: %w", schemaName, err)
		}
		catalogs = append(catalogs, enumCatalogPlan{
			name:           schemaName,
			valuesBinding:  stablePrivateIdentifier("component-enum-values", schemaName),
			catalogBinding: stablePrivateIdentifier("component-enum-catalog", schemaName),
			renderedValues: rendered,
			tupleType:      tupleType,
			valueType:      valueType,
			members:        enumStringMembers(values),
		})
	}
	return catalogs, nil
}

func enumValueType(values []any) (string, error) {
	if len(values) == 0 {
		return "never", nil
	}
	types := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for index, value := range values {
		rendered, err := readonlyJSONType(value)
		if err != nil {
			return "", fmt.Errorf("JSON item %d: %w", index, err)
		}
		if seen[rendered] {
			continue
		}
		seen[rendered] = true
		types = append(types, rendered)
	}
	return strings.Join(types, " | "), nil
}

func enumStringMembers(values []any) []string {
	members := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		member, ok := value.(string)
		if !ok || seen[member] {
			continue
		}
		seen[member] = true
		members = append(members, member)
	}
	return members
}
