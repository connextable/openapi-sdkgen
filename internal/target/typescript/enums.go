package typescript

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"openapi-sdkgen/internal/compiler/ir"
)

type enumValuesPlan struct {
	name           string
	valuesBinding  string
	enumBinding    string
	renderedValues string
	valueType      string
	members        []string
	hasJSONRecord  bool
}

func emitEnums(document *ir.Document) ([]byte, error) {
	plans, err := enumValuesPlans(document)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	for _, plan := range plans {
		if plan.hasJSONRecord {
			output.WriteString(`function __sdkgen_createJSONRecord<Value extends object>(
  entries: readonly (readonly [PropertyKey, unknown])[],
): Value {
  return Object.fromEntries(entries) as Value
}

`)
			break
		}
	}
	if len(plans) > 0 {
		output.WriteString(`function __sdkgen_createEnumValues(values: readonly unknown[]): object {
  const enumValues = Object.create(null) as Record<PropertyKey, unknown>
  for (const value of values) {
    if (typeof value !== "string" || Object.hasOwn(enumValues, value)) continue
    Object.defineProperty(enumValues, value, { enumerable: true, value })
  }
  Object.defineProperty(enumValues, Symbol.iterator, { value: () => values[Symbol.iterator]() })
  return Object.freeze(enumValues)
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
	bindings := make([]runtimeProperty, 0, len(plans))
	for _, plan := range plans {
		fmt.Fprintf(&output, "const %s = %s as const\n", plan.valuesBinding, plan.renderedValues)
		fmt.Fprintf(&output, "const %s = /* @__PURE__ */ __sdkgen_createEnumValues(%s)\n", plan.enumBinding, plan.valuesBinding)
		bindings = append(bindings, runtimeProperty{key: plan.name, value: plan.enumBinding})
	}
	output.WriteString("/** Runtime enum values keyed by exact OpenAPI component schema names. */\n")
	fmt.Fprintf(&output, "export const Enums = %s as {\n", runtimeObjectExpression(bindings))
	for _, plan := range plans {
		fmt.Fprintf(&output, "  /** Values declared by OpenAPI component `%s`. */\n", sanitizeComment(plan.name))
		fmt.Fprintf(&output, "  readonly %s: {\n", quoteTS(plan.name))
		for _, member := range plan.members {
			fmt.Fprintf(&output, "    /** Exact string value `%s`. */\n", sanitizeComment(member))
			fmt.Fprintf(&output, "    readonly %s: %s\n", quoteTS(member), quoteTS(member))
		}
		fmt.Fprintf(&output, "    [Symbol.iterator](): IterableIterator<%s>\n", plan.valueType)
		output.WriteString("  }\n")
	}
	output.WriteString("}\n")
	if len(plans) > 0 {
		output.WriteString("/** Literal value union for an exact generated enum component name. */\n")
		output.WriteString("export type EnumValue<Name extends keyof typeof Enums> = (typeof Enums)[Name] extends Iterable<infer Value> ? Value : never\n")
		output.WriteString("/** Return whether a runtime value structurally matches a generated enum value. */\n")
		output.WriteString("export function isEnumValue<EnumValues extends (typeof Enums)[keyof typeof Enums]>(\n")
		output.WriteString("  enumValues: EnumValues,\n")
		output.WriteString("  value: unknown,\n")
		output.WriteString("): value is EnumValues extends Iterable<infer Value> ? Value : never {\n")
		output.WriteString("  try {\n")
		output.WriteString("    for (const candidate of enumValues) {\n")
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

func enumValuesPlans(document *ir.Document) ([]enumValuesPlan, error) {
	reachable := reachableComponentSchemas(document)
	names := make([]string, 0, len(reachable))
	for name := range reachable {
		names = append(names, name)
	}
	sort.Strings(names)
	plans := make([]enumValuesPlan, 0)
	for _, schemaName := range names {
		schema, ok := componentSchemaValue(document, schemaName).(map[string]any)
		if !ok {
			continue
		}
		values, exists := schema["enum"].([]any)
		if !exists {
			continue
		}
		rendered, hasJSONRecord, err := enumRuntimeJSONExpression(values)
		if err != nil {
			return nil, fmt.Errorf("component %s enum: %w", schemaName, err)
		}
		valueType, err := enumValueType(values)
		if err != nil {
			return nil, fmt.Errorf("component %s enum value type: %w", schemaName, err)
		}
		plans = append(plans, enumValuesPlan{
			name:           schemaName,
			valuesBinding:  stablePrivateIdentifier("component-enum-values", schemaName),
			enumBinding:    stablePrivateIdentifier("component-enum", schemaName),
			renderedValues: rendered,
			valueType:      valueType,
			members:        enumStringMembers(values),
			hasJSONRecord:  hasJSONRecord,
		})
	}
	return plans, nil
}

// enumRuntimeJSONExpression keeps enum literals inferable under an outer
// `as const`. Records still use a typed Object.fromEntries helper so exact
// keys, including "__proto__", remain own data properties without widening
// their generated readonly JSON types.
func enumRuntimeJSONExpression(value any) (string, bool, error) {
	switch typed := value.(type) {
	case map[string]any:
		names := make([]string, 0, len(typed))
		for name := range typed {
			names = append(names, name)
		}
		sort.Strings(names)
		entries := make([]string, 0, len(names))
		for _, name := range names {
			rendered, _, err := enumRuntimeJSONExpression(typed[name])
			if err != nil {
				return "", false, fmt.Errorf("JSON property %q: %w", name, err)
			}
			entries = append(entries, "["+quoteTS(name)+", "+rendered+"]")
		}
		valueType, err := readonlyJSONType(typed)
		if err != nil {
			return "", false, err
		}
		return "/* @__PURE__ */ __sdkgen_createJSONRecord<" + valueType + ">([" + strings.Join(entries, ", ") + "])", true, nil
	case map[string]map[string]any:
		values := make(map[string]any, len(typed))
		for key, item := range typed {
			values[key] = item
		}
		return enumRuntimeJSONExpression(values)
	case []any:
		items := make([]string, 0, len(typed))
		hasJSONRecord := false
		for index, item := range typed {
			rendered, itemHasJSONRecord, err := enumRuntimeJSONExpression(item)
			if err != nil {
				return "", false, fmt.Errorf("JSON item %d: %w", index, err)
			}
			items = append(items, rendered)
			hasJSONRecord = hasJSONRecord || itemHasJSONRecord
		}
		return "[" + strings.Join(items, ", ") + "]", hasJSONRecord, nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return "", false, fmt.Errorf("marshal JSON literal: %w", err)
		}
		return string(data), false, nil
	}
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
