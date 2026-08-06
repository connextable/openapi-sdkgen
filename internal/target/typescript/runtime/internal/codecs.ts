import { defineOwnDataProperty, isRecord } from "./objects.js";

/** Host-owned encoder/decoder for a declared non-built-in media type. */
export interface MediaCodec<Value> {
  readonly encode?: (
    value: Value,
    context: { readonly contentType: string },
  ) => BodyInit | Promise<BodyInit>;
  readonly decode?: (
    response: Response,
    context: { readonly contentType: string },
  ) => Value | Promise<Value>;
  /** Decodes one non-streaming inbound server request for a declared custom media type. */
  readonly decodeInbound?: (
    request: Request,
    context: { readonly contentType: string },
  ) => Value | Promise<Value>;
  /** Serializes one Parameter Object `content` value into its required string representation. */
  readonly encodeParameter?: (
    value: Value,
    context: { readonly contentType: string },
  ) => string | Promise<string>;
  /** Decodes a Parameter Object or response Header Object `content` string. */
  readonly decodeParameter?: (
    value: string,
    context: { readonly contentType: string },
  ) => Value | Promise<Value>;
  /** Encodes validated items for one declared custom streaming request body. */
  readonly encodeStream?: (
    items: AsyncIterable<Value>,
    context: { readonly contentType: string; readonly signal?: AbortSignal | undefined },
  ) => ReadableStream<Uint8Array> | Promise<ReadableStream<Uint8Array>>;
  /** Decodes one declared custom streaming response without exposing the raw Fetch stream. */
  readonly decodeStream?: (
    reader: MediaStreamReader,
    context: {
      readonly contentType: string;
      readonly maxFrameBytes: number;
      readonly signal?: AbortSignal | undefined;
    },
  ) => AsyncIterable<Value>;
  /** Decodes one inbound server stream for a declared custom media type. */
  readonly decodeInboundStream?: (
    reader: MediaStreamReader,
    context: { readonly contentType: string; readonly maxFrameBytes: number },
  ) => AsyncIterable<Value>;
}

/** Bounded, cancellable reader supplied to a custom response streaming codec. */
export interface MediaStreamReader {
  /** Reads at most `maxBytes`, which must not exceed the generated stream limit. */
  read(maxBytes: number): Promise<Uint8Array | null>;
  /** Cancels the source response body and releases its reader lock. */
  cancel(reason?: unknown): Promise<void>;
}

/** Explicit capabilities a host transport grants to generated SDK code. */
/** Minimal recursive schema used for runtime wire-name transformation. */
export interface WireSchema {
  /** Boolean-schema acceptance. `false` rejects every value. */
  readonly boolean?: boolean;
  /** Referenced component name. */
  readonly reference?: string;
  /** Name this schema contributes to JSON Schema's dynamic scope. */
  readonly dynamicAnchor?: string;
  /** A dynamic reference plus its static fallback target. */
  readonly dynamicReference?: WireDynamicReference;
  /** Allowed JSON Schema primitive or composite types. */
  readonly types?: readonly string[];
  /** Exact permitted literal value. */
  readonly constValue?: unknown;
  /** Permitted literal values. */
  readonly enumValues?: readonly unknown[];
  readonly multipleOf?: number;
  readonly maximum?: number;
  readonly exclusiveMaximum?: number;
  readonly minimum?: number;
  readonly exclusiveMinimum?: number;
  readonly minLength?: number;
  readonly maxLength?: number;
  readonly pattern?: string;
  /** JSON Schema format annotation; asserted only when formatAssertion is true. */
  readonly format?: string;
  /** The active schema dialect requires the standard format-assertion vocabulary. */
  readonly formatAssertion?: boolean;
  readonly minItems?: number;
  readonly maxItems?: number;
  readonly uniqueItems?: boolean;
  readonly contains?: WireSchema;
  readonly minContains?: number;
  readonly maxContains?: number;
  readonly minProperties?: number;
  readonly maxProperties?: number;
  /** Object properties keyed by their JSON wire names. */
  readonly properties?: Readonly<Record<string, WireProperty>>;
  readonly patternProperties?: Readonly<Record<string, WireSchema>>;
  readonly propertyNames?: WireSchema;
  readonly dependentRequired?: Readonly<Record<string, readonly string[]>>;
  readonly dependentSchemas?: Readonly<Record<string, WireSchema>>;
  /** Homogeneous array item schema. */
  readonly items?: WireSchema;
  /** Tuple item schemas in positional order. */
  readonly prefixItems?: readonly WireSchema[];
  /** Schema for additional object properties, or false for a closed object. */
  readonly additionalProperties?: WireSchema | false;
  /** Schema for object properties left unevaluated by sibling applicators, or false to reject them. */
  readonly unevaluatedProperties?: WireSchema | false;
  /** Schema for array items left unevaluated by sibling applicators, or false to reject them. */
  readonly unevaluatedItems?: WireSchema | false;
  /** Required JSON wire property names. */
  readonly required?: readonly string[];
  /** Schemas whose transformations are applied cumulatively. */
  readonly allOf?: readonly WireSchema[];
  /** Alternative schemas considered when transforming a value. */
  readonly oneOf?: readonly WireSchema[];
  /** Alternative schemas considered when transforming a value. */
  readonly anyOf?: readonly WireSchema[];
  /** Schema that must not match. */
  readonly not?: WireSchema;
  readonly if?: WireSchema;
  readonly then?: WireSchema;
  readonly else?: WireSchema;
  /** OpenAPI discriminator dispatch metadata for polymorphic schemas. */
  readonly discriminator?: WireDiscriminator;
  /** OpenAPI XML Object serialization metadata. */
  readonly xml?: WireXML;
  /** JSON Schema content encoding applied before validating contentSchema. */
  readonly contentEncoding?: string;
  /** Media type of string content validated by contentSchema. */
  readonly contentMediaType?: string;
  /** Schema applied to decoded string content without changing the outer value. */
  readonly contentSchema?: WireSchema;
}

/** Runtime representation of one lowered JSON Schema `$dynamicRef`. */
export interface WireDynamicReference {
  readonly anchor: string;
  readonly fallback: WireSchema;
}

/** OpenAPI XML Object metadata attached to a Schema Object. */
export interface WireXML {
  readonly name?: string;
  readonly namespace?: string;
  readonly prefix?: string;
  readonly attribute?: boolean;
  readonly wrapped?: boolean;
  readonly nodeType?: "element" | "attribute" | "text" | "cdata" | "none";
}

/** Maps an OpenAPI discriminator property and values to concrete schema branches. */
export interface WireDiscriminator {
  readonly property: string;
  readonly mapping?: Readonly<Record<string, WireSchema>>;
  readonly defaultMapping?: WireSchema;
}

/** Generated component schema registry keyed by OpenAPI component name. */
export type WireSchemas = Readonly<Record<string, WireSchema>>;

/** Mapping between one JSON wire name and its generated TypeScript property. */
export interface WireProperty {
  /** Generated TypeScript property name. */
  readonly property: string;
  /** Nested transformation schema for the property value. */
  readonly schema: WireSchema;
}

/** Request or response body representation understood by the runtime. */
export interface WireBodyDefinition {
  /** Exact media type, excluding parameters such as charset. */
  readonly contentType: string;
  /** Wire transformation schema for this representation. */
  readonly schema: WireSchema;
  /** OpenAPI 3.2 schema for one streamed response item. */
  readonly itemSchema?: WireSchema;
  /** Per-property Encoding Object declarations for form request bodies. */
  readonly encoding?: readonly WireEncodingDefinition[];
  /** Positional Encoding Objects for the first parts of a multipart body. */
  readonly prefixEncoding?: readonly WireEncodingDefinition[];
  /** Positional Encoding Object applied to the remaining multipart parts. */
  readonly itemEncoding?: WireEncodingDefinition;
}

/** One OpenAPI Encoding Object declaration for a form request-body property. */
export interface WireEncodingDefinition {
  /** Form property name. Omitted for positional multipart encodings. */
  readonly name?: string;
  readonly contentType?: string;
  readonly style?: string;
  readonly explode?: boolean;
  readonly allowReserved?: boolean;
  readonly headers?: readonly WireMultipartHeaderDefinition[];
  /** Nested Encoding Objects for an embedded form or multipart representation. */
  readonly encoding?: readonly WireEncodingDefinition[];
  /** Nested positional Encoding Objects for an embedded multipart representation. */
  readonly prefixEncoding?: readonly WireEncodingDefinition[];
  /** Nested streaming positional Encoding Object for an embedded multipart representation. */
  readonly itemEncoding?: WireEncodingDefinition;
}

/** A Header Object attached to one multipart part by an Encoding Object. */
export interface WireMultipartHeaderDefinition {
  readonly name: string;
  readonly required?: boolean;
  readonly style?: string;
  readonly explode?: boolean;
  readonly contentType?: string;
  readonly schema: WireSchema;
}

/** Successful response representation understood by the runtime. */
export interface WireResponseDefinition extends WireBodyDefinition {
  /** Exact status code, `default`, or wildcard status such as `2XX`. */
  readonly status: string;
  readonly headers?: readonly WireHeaderDefinition[];
}

/** Generated response-header decoding metadata. */
export interface WireHeaderDefinition {
  readonly name: string;
  readonly property: string;
  readonly required?: boolean;
  /** Header serialization style. OpenAPI defaults this to `simple`. */
  readonly style?: string;
  /** Whether an object uses `name=value` entries instead of alternating tokens. */
  readonly explode?: boolean;
  /** The sole Header Object content media type, when content is used instead of schema. */
  readonly contentType?: string;
  readonly schema: WireSchema;
}

/** OpenAPI parameter serialization metadata emitted by `sdkgen`. */
export function encodeXML(value: unknown, schema: WireSchema, schemas: WireSchemas): string {
  const rootName = schema.reference ?? schema.xml?.name ?? "root";
  return encodeXMLElement(value, schema, schemas, rootName, true, []);
}

function encodeXMLElement(
  value: unknown,
  schema: WireSchema,
  schemas: WireSchemas,
  fallbackName: string,
  root: boolean,
  dynamicScope: DynamicScope,
): string {
  const scope = extendDynamicScope(dynamicScope, schema);
  const dynamicTarget = resolveDynamicReference(schema, scope);
  if (dynamicTarget !== undefined)
    return encodeXMLElement(value, dynamicTarget, schemas, fallbackName, root, scope);
  if (schema.reference !== undefined) {
    const referenced = schemas[schema.reference];
    if (referenced === undefined)
      throw new TypeError(`XML schema references missing component ${schema.reference}`);
    return encodeXMLElement(value, referenced, schemas, fallbackName, root, scope);
  }
  const xml = schema.xml;
  if (xml?.nodeType === "none") return "";
  if (xml?.nodeType === "text") return escapeXMLText(xmlScalar(value));
  if (xml?.nodeType === "cdata")
    return `<![CDATA[${xmlScalar(value).replaceAll("]]>", "]]]]><![CDATA[>")}]]>`;
  const name = xmlName(xml, fallbackName);
  if (Array.isArray(value)) {
    const itemSchema = schema.items ?? {};
    const itemName =
      itemSchema.xml?.name ?? (xml?.wrapped ? (itemSchema.xml?.name ?? fallbackName) : name);
    const values = value
      .map((item) => encodeXMLElement(item, itemSchema, schemas, itemName, false, scope))
      .join("");
    return xml?.wrapped ? wrapXML(name, namespaceAttributes(xml, root), values) : values;
  }
  if (!isRecord(value))
    return wrapXML(name, namespaceAttributes(xml, root), escapeXMLText(xmlScalar(value)));
  const attributes: string[] = [];
  const children: string[] = [];
  let text = "";
  for (const [wireName, property] of Object.entries(schema.properties ?? {})) {
    const item = value[wireName];
    if (item === undefined || item === null) continue;
    const childXML = property.schema.xml;
    const childName = childXML?.name ?? wireName;
    if (childXML?.attribute || childXML?.nodeType === "attribute") {
      attributes.push(`${xmlName(childXML, childName)}="${escapeXMLAttribute(xmlScalar(item))}"`);
      continue;
    }
    if (childXML?.nodeType === "text") {
      text += escapeXMLText(xmlScalar(item));
      continue;
    }
    if (childXML?.nodeType === "cdata") {
      text += `<![CDATA[${xmlScalar(item).replaceAll("]]>", "]]]]><![CDATA[>")}]]>`;
      continue;
    }
    children.push(encodeXMLElement(item, property.schema, schemas, childName, false, scope));
  }
  return wrapXML(
    name,
    [...namespaceAttributes(xml, root), ...attributes],
    text + children.join(""),
  );
}

function wrapXML(name: string, attributes: readonly string[], content: string): string {
  const start = `<${name}${attributes.length === 0 ? "" : ` ${attributes.join(" ")}`}>`;
  return `${start}${content}</${name}>`;
}

function xmlName(xml: WireXML | undefined, fallback: string): string {
  const name = xml?.name ?? fallback;
  return xml?.prefix === undefined || xml.prefix === "" ? name : `${xml.prefix}:${name}`;
}

function namespaceAttributes(xml: WireXML | undefined, include: boolean): string[] {
  if (!include || xml?.namespace === undefined || xml.namespace === "") return [];
  return [
    xml.prefix === undefined || xml.prefix === ""
      ? `xmlns="${escapeXMLAttribute(xml.namespace)}"`
      : `xmlns:${xml.prefix}="${escapeXMLAttribute(xml.namespace)}"`,
  ];
}

function xmlScalar(value: unknown): string {
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint")
    return String(value);
  if (value === null || value === undefined) return "";
  throw new TypeError(
    "XML scalar node requires a string, number, boolean, bigint, null, or undefined value",
  );
}

function escapeXMLText(value: string): string {
  return value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
}

function escapeXMLAttribute(value: string): string {
  return escapeXMLText(value).replaceAll('"', "&quot;").replaceAll("'", "&apos;");
}

interface XMLNode {
  readonly name: string;
  readonly attributes: Readonly<Record<string, string>>;
  readonly children: XMLNode[];
  text: string;
}

/** Decodes one XML representation using the generated OpenAPI XML schema metadata. */
export function decodeXML(source: string, schema: WireSchema, components: WireSchemas): unknown {
  return decodeXMLNode(parseXMLDocument(source), schema, components, []);
}

function parseXMLDocument(source: string): XMLNode {
  const tokens =
    source.match(/<!\[CDATA\[[\s\S]*?\]\]>|<!--[\s\S]*?-->|<\?[^]*?\?>|<[^>]+>|[^<]+/g) ?? [];
  const roots: XMLNode[] = [];
  const stack: XMLNode[] = [];
  for (const token of tokens) {
    if (token.startsWith("<!--") || token.startsWith("<?")) continue;
    if (token.startsWith("<![CDATA[")) {
      if (stack.length === 0)
        throw new TypeError("XML character data appears outside the document element");
      stack[stack.length - 1]!.text += token.slice(9, -3);
      continue;
    }
    if (token.startsWith("<!"))
      throw new TypeError("XML declarations other than comments and CDATA are unsupported");
    if (token.startsWith("</")) {
      const name = token.slice(2, -1).trim();
      const current = stack.pop();
      if (current === undefined || current.name !== name)
        throw new TypeError(`XML closing tag ${name} does not match the open element`);
      continue;
    }
    if (token.startsWith("<")) {
      const selfClosing = /\/>$/.test(token);
      const body = token.slice(1, selfClosing ? -2 : -1).trim();
      const match = /^([^\s/>]+)([\s\S]*)$/.exec(body);
      if (match === null) throw new TypeError("XML element has no name");
      const node: XMLNode = {
        name: match[1]!,
        attributes: parseXMLAttributes(match[2] ?? ""),
        children: [],
        text: "",
      };
      if (stack.length === 0) roots.push(node);
      else stack[stack.length - 1]!.children.push(node);
      if (!selfClosing) stack.push(node);
      continue;
    }
    if (stack.length === 0) {
      if (token.trim() !== "") throw new TypeError("XML text appears outside the document element");
      continue;
    }
    stack[stack.length - 1]!.text += unescapeXML(token);
  }
  if (stack.length !== 0 || roots.length !== 1)
    throw new TypeError("XML document must contain one balanced root element");
  return roots[0]!;
}

function parseXMLAttributes(source: string): Readonly<Record<string, string>> {
  const result = Object.create(null) as Record<string, string>;
  const expression = /([^\s=]+)\s*=\s*("[^"]*"|'[^']*')/g;
  let match: RegExpExecArray | null;
  while ((match = expression.exec(source)) !== null)
    defineOwnDataProperty(result, match[1]!, unescapeXML(match[2]!.slice(1, -1)));
  if (source.replace(expression, "").trim() !== "")
    throw new TypeError("XML attribute syntax is invalid");
  return result;
}

function decodeXMLNode(
  node: XMLNode,
  schema: WireSchema,
  components: WireSchemas,
  dynamicScope: DynamicScope,
): unknown {
  const scope = extendDynamicScope(dynamicScope, schema);
  const dynamicTarget = resolveDynamicReference(schema, scope);
  if (dynamicTarget !== undefined) return decodeXMLNode(node, dynamicTarget, components, scope);
  if (schema.reference !== undefined) {
    const referenced = components[schema.reference];
    if (referenced === undefined)
      throw new TypeError(`XML schema references missing component ${schema.reference}`);
    return decodeXMLNode(node, referenced, components, scope);
  }
  if (schema.types?.includes("array")) {
    const itemSchema = schema.items ?? {};
    return node.children.map((child) => decodeXMLNode(child, itemSchema, components, scope));
  }
  if (schema.types?.includes("object") || schema.properties !== undefined) {
    const result = Object.create(null) as Record<string, unknown>;
    for (const [wireName, property] of Object.entries(schema.properties ?? {})) {
      const xml = property.schema.xml;
      const name = xmlName(xml, wireName);
      if (xml?.attribute || xml?.nodeType === "attribute") {
        const value = node.attributes[name];
        if (value !== undefined)
          defineOwnDataProperty(result, wireName, decodeXMLScalar(value, property.schema));
        continue;
      }
      if (property.schema.types?.includes("array")) {
        const itemSchema = property.schema.items ?? {};
        const container = xml?.wrapped ? node.children.find((child) => child.name === name) : node;
        if (container !== undefined) {
          const itemName = xmlName(itemSchema.xml, itemSchema.xml?.name ?? wireName);
          defineOwnDataProperty(
            result,
            wireName,
            container.children
              .filter((child) => child.name === itemName)
              .map((child) => decodeXMLNode(child, itemSchema, components, scope)),
          );
        }
        continue;
      }
      const child = node.children.find((entry) => entry.name === name);
      if (child !== undefined)
        defineOwnDataProperty(
          result,
          wireName,
          decodeXMLNode(child, property.schema, components, scope),
        );
    }
    return result;
  }
  return decodeXMLScalar(node.text, schema);
}

function decodeXMLScalar(value: string, schema: WireSchema): unknown {
  if (schema.types?.includes("integer")) {
    const parsed = Number(value);
    if (!Number.isInteger(parsed)) throw new TypeError("XML value is not an integer");
    return parsed;
  }
  if (schema.types?.includes("number")) {
    const parsed = Number(value);
    if (!Number.isFinite(parsed)) throw new TypeError("XML value is not a number");
    return parsed;
  }
  if (schema.types?.includes("boolean")) {
    if (value === "true") return true;
    if (value === "false") return false;
    throw new TypeError("XML value is not a boolean");
  }
  return value;
}

function unescapeXML(value: string): string {
  return value
    .replaceAll("&lt;", "<")
    .replaceAll("&gt;", ">")
    .replaceAll("&quot;", '"')
    .replaceAll("&apos;", "'")
    .replaceAll("&amp;", "&");
}

type DynamicScope = readonly WireSchema[];

function extendDynamicScope(scope: DynamicScope, schema: WireSchema): DynamicScope {
  return schema.dynamicAnchor === undefined ? scope : [...scope, schema];
}

function resolveDynamicReference(schema: WireSchema, scope: DynamicScope): WireSchema | undefined {
  const reference = schema.dynamicReference;
  if (reference === undefined) return undefined;
  // The outer resource is searched first. This lets a resource that overrides
  // an anchor constrain a base schema reached through a normal `$ref`.
  return (
    scope.find((candidate) => candidate.dynamicAnchor === reference.anchor) ?? reference.fallback
  );
}

/** Recursively maps a value between generated property names and wire names. */
export function transformWireValue(
  value: unknown,
  schema: WireSchema,
  components: WireSchemas,
  direction: "encode" | "decode",
  dynamicScope: DynamicScope = [],
): unknown {
  const scope = extendDynamicScope(dynamicScope, schema);
  validateWireValue(value, schema, components, direction, dynamicScope);
  if (value === null || value === undefined) return value;
  const dynamicTarget = resolveDynamicReference(schema, scope);
  let transformed: unknown = value;
  if (dynamicTarget !== undefined)
    transformed = transformWireValue(transformed, dynamicTarget, components, direction, scope);
  if (schema.reference !== undefined) {
    const referenced = components[schema.reference];
    if (referenced !== undefined)
      transformed = transformWireValue(transformed, referenced, components, direction, scope);
  }
  if (Array.isArray(transformed)) {
    return transformed.map((item, index) => {
      const itemSchema = schema.prefixItems?.[index] ?? schema.items;
      return itemSchema === undefined
        ? item
        : transformWireValue(item, itemSchema, components, direction, scope);
    });
  }
  if (
    isRecord(transformed) &&
    (schema.properties !== undefined || schema.additionalProperties !== undefined)
  ) {
    const source = transformed;
    const result = Object.create(null) as Record<string, unknown>;
    for (const [key, item] of Object.entries(source)) defineOwnDataProperty(result, key, item);
    const known = new Set<string>();
    for (const [wireName, propertyDefinition] of Object.entries(schema.properties ?? {})) {
      const sourceName = direction === "encode" ? propertyDefinition.property : wireName;
      const targetName = direction === "encode" ? wireName : propertyDefinition.property;
      known.add(sourceName);
      known.add(targetName);
      if (!Object.hasOwn(source, sourceName)) continue;
      if (sourceName !== targetName) delete result[sourceName];
      defineOwnDataProperty(
        result,
        targetName,
        transformWireValue(
          source[sourceName],
          propertyDefinition.schema,
          components,
          direction,
          scope,
        ),
      );
    }
    if (schema.additionalProperties !== undefined && schema.additionalProperties !== false) {
      for (const [key, item] of Object.entries(result)) {
        if (!known.has(key)) {
          defineOwnDataProperty(
            result,
            key,
            transformWireValue(item, schema.additionalProperties, components, direction, scope),
          );
        }
      }
    }
    transformed = result;
  }
  for (const branch of schema.allOf ?? []) {
    transformed = transformWireValue(transformed, branch, components, direction, scope);
  }
  if (schema.if !== undefined) {
    const branch = schemaMatches(transformed, schema.if, components, direction, scope)
      ? schema.then
      : schema.else;
    if (branch !== undefined)
      transformed = transformWireValue(transformed, branch, components, direction, scope);
  }
  for (const variants of [schema.oneOf, schema.anyOf]) {
    if (variants === undefined) continue;
    const selected =
      schema.discriminator !== undefined
        ? (discriminatorVariant(transformed, schema, components, direction) ??
          variants.find((variant) =>
            schemaMatches(transformed, variant, components, direction, scope),
          ))
        : variants.find((variant) =>
            schemaMatches(transformed, variant, components, direction, scope),
          );
    if (selected !== undefined)
      transformed = transformWireValue(transformed, selected, components, direction, scope);
  }
  return transformed;
}

/** Converts a validated JSON wire value into generated TypeScript property names. */
export function decodeWireValue(
  value: unknown,
  schema: WireSchema,
  components: WireSchemas,
): unknown {
  return transformWireValue(value, schema, components, "decode");
}

/** Converts generated TypeScript property names into validated JSON wire names. */
export function encodeWireValue(
  value: unknown,
  schema: WireSchema,
  components: WireSchemas,
): unknown {
  return transformWireValue(value, schema, components, "encode");
}

/** Validates a transformed wire value against its generated schema. */
export function validateWireValue(
  value: unknown,
  schema: WireSchema,
  components: WireSchemas,
  direction: "encode" | "decode",
  dynamicScope: DynamicScope = [],
): void {
  assertFiniteJSONNumbers(value);
  const scope = extendDynamicScope(dynamicScope, schema);
  if (schema.boolean === false) throw new TypeError("schema is false");
  if (value === undefined) return;
  const dynamicTarget = resolveDynamicReference(schema, scope);
  if (dynamicTarget !== undefined) {
    validateWireValue(value, dynamicTarget, components, direction, scope);
  }
  if (schema.reference !== undefined) {
    const referenced = components[schema.reference];
    if (referenced !== undefined)
      validateWireValue(value, referenced, components, direction, scope);
  }
  if (schema.types !== undefined && !schema.types.some((type) => valueMatchesType(value, type))) {
    throw new TypeError(`expected ${schema.types.join(" | ")}`);
  }
  if (schema.constValue !== undefined && !wireValueEquals(value, schema.constValue)) {
    throw new TypeError("value does not match const");
  }
  if (
    schema.enumValues !== undefined &&
    !schema.enumValues.some((item) => wireValueEquals(value, item))
  ) {
    throw new TypeError("value is not in enum");
  }
  if (typeof value === "number") {
    if (schema.multipleOf !== undefined && !isMultipleOf(value, schema.multipleOf))
      throw new TypeError(`must be a multiple of ${schema.multipleOf}`);
    if (schema.maximum !== undefined && value > schema.maximum)
      throw new TypeError(`must be <= ${schema.maximum}`);
    if (schema.exclusiveMaximum !== undefined && value >= schema.exclusiveMaximum)
      throw new TypeError(`must be < ${schema.exclusiveMaximum}`);
    if (schema.minimum !== undefined && value < schema.minimum)
      throw new TypeError(`must be >= ${schema.minimum}`);
    if (schema.exclusiveMinimum !== undefined && value <= schema.exclusiveMinimum)
      throw new TypeError(`must be > ${schema.exclusiveMinimum}`);
  }
  if (typeof value === "string") {
    if (schema.minLength !== undefined && [...value].length < schema.minLength)
      throw new TypeError(`must have length >= ${schema.minLength}`);
    if (schema.maxLength !== undefined && [...value].length > schema.maxLength)
      throw new TypeError(`must have length <= ${schema.maxLength}`);
    if (schema.pattern !== undefined && !new RegExp(schema.pattern, "u").test(value))
      throw new TypeError(`must match pattern ${schema.pattern}`);
    if (
      schema.formatAssertion &&
      schema.format !== undefined &&
      !matchesWireFormat(value, schema.format)
    )
      throw new TypeError(`must match format ${schema.format}`);
  }
  if (schema.oneOf !== undefined) {
    const matches = schema.oneOf.filter((item) =>
      schemaMatches(value, item, components, direction, scope),
    );
    if (matches.length !== 1)
      throw new TypeError(`oneOf requires exactly one matching schema, got ${matches.length}`);
  }
  if (
    schema.anyOf !== undefined &&
    !schema.anyOf.some((item) => schemaMatches(value, item, components, direction, scope))
  ) {
    throw new TypeError("anyOf requires at least one matching schema");
  }
  if (schema.not !== undefined && schemaMatches(value, schema.not, components, direction, scope)) {
    throw new TypeError("must not match negated schema");
  }
  if (schema.if !== undefined) {
    const branch = schemaMatches(value, schema.if, components, direction, scope)
      ? schema.then
      : schema.else;
    if (branch !== undefined) validateWireValue(value, branch, components, direction, scope);
  }
  for (const branch of schema.allOf ?? [])
    validateWireValue(value, branch, components, direction, scope);
  if (schema.contentSchema !== undefined && typeof value === "string") {
    validateWireValue(
      decodeSchemaContent(value, schema, components),
      schema.contentSchema,
      components,
      direction,
      scope,
    );
  }
  if (Array.isArray(value)) {
    for (let index = 0; index < value.length; index++) {
      if (!Object.hasOwn(value, index)) throw new TypeError("must not contain sparse items");
    }
    if (schema.minItems !== undefined && value.length < schema.minItems)
      throw new TypeError(`must contain at least ${schema.minItems} items`);
    if (schema.maxItems !== undefined && value.length > schema.maxItems)
      throw new TypeError(`must contain at most ${schema.maxItems} items`);
    if (schema.uniqueItems && !hasUniqueWireValues(value))
      throw new TypeError("must contain unique items");
    if (schema.contains !== undefined) {
      const matches = value.filter((item) =>
        schemaMatches(item, schema.contains!, components, direction, scope),
      ).length;
      const minimum = schema.minContains ?? 1;
      if (matches < minimum) throw new TypeError(`must contain at least ${minimum} matching items`);
      if (schema.maxContains !== undefined && matches > schema.maxContains)
        throw new TypeError(`must contain at most ${schema.maxContains} matching items`);
    }
    for (const [index, item] of value.entries()) {
      const itemSchema = schema.prefixItems?.[index] ?? schema.items;
      if (itemSchema !== undefined)
        validateWireValue(item, itemSchema, components, direction, scope);
    }
    if (schema.unevaluatedItems !== undefined) {
      const evaluated = evaluatedArrayIndexes(value, schema, components, direction, scope);
      for (const [index, item] of value.entries()) {
        if (evaluated.has(index)) continue;
        if (schema.unevaluatedItems === false)
          throw new TypeError(`unexpected unevaluated item ${index}`);
        validateWireValue(item, schema.unevaluatedItems, components, direction, scope);
      }
    }
    return;
  }
  if (!isRecord(value)) return;
  if (schema.minProperties !== undefined && Object.keys(value).length < schema.minProperties)
    throw new TypeError(`must contain at least ${schema.minProperties} properties`);
  if (schema.maxProperties !== undefined && Object.keys(value).length > schema.maxProperties)
    throw new TypeError(`must contain at most ${schema.maxProperties} properties`);
  const properties = schema.properties ?? {};
  const allowed = new Set<string>();
  for (const [wireName, definition] of Object.entries(properties)) {
    const sourceName = direction === "encode" ? definition.property : wireName;
    allowed.add(sourceName);
    if (Object.hasOwn(value, sourceName)) {
      try {
        validateWireValue(value[sourceName], definition.schema, components, direction, scope);
      } catch (cause) {
        throw new TypeError(
          `property ${wireName}: ${cause instanceof Error ? cause.message : "invalid value"}`,
          { cause },
        );
      }
    }
  }
  for (const required of schema.required ?? []) {
    const definition = properties[required];
    const sourceName =
      direction === "encode" && definition !== undefined ? definition.property : required;
    if (!Object.hasOwn(value, sourceName) || value[sourceName] === undefined) {
      throw new TypeError(`missing required property ${required}`);
    }
  }
  for (const [property, required] of Object.entries(schema.dependentRequired ?? {})) {
    const sourceProperty =
      direction === "encode" && properties[property] !== undefined
        ? properties[property].property
        : property;
    if (!Object.hasOwn(value, sourceProperty) || value[sourceProperty] === undefined) continue;
    for (const dependency of required) {
      const sourceDependency =
        direction === "encode" && properties[dependency] !== undefined
          ? properties[dependency].property
          : dependency;
      if (!Object.hasOwn(value, sourceDependency) || value[sourceDependency] === undefined) {
        throw new TypeError(`property ${property} requires property ${dependency}`);
      }
    }
  }
  for (const [property, dependency] of Object.entries(schema.dependentSchemas ?? {})) {
    const sourceProperty =
      direction === "encode" && properties[property] !== undefined
        ? properties[property].property
        : property;
    if (Object.hasOwn(value, sourceProperty) && value[sourceProperty] !== undefined)
      validateWireValue(value, dependency, components, direction, scope);
  }
  for (const [pattern, propertySchema] of Object.entries(schema.patternProperties ?? {})) {
    const expression = new RegExp(pattern, "u");
    for (const [key, item] of Object.entries(value)) {
      if (expression.test(key)) {
        allowed.add(key);
        validateWireValue(item, propertySchema, components, direction, scope);
      }
    }
  }
  if (schema.propertyNames !== undefined) {
    for (const key of Object.keys(value))
      validateWireValue(key, schema.propertyNames, components, direction, scope);
  }
  if (schema.additionalProperties === false) {
    for (const key of Object.keys(value)) {
      if (!allowed.has(key)) throw new TypeError(`unexpected property ${key}`);
    }
  } else if (schema.additionalProperties !== undefined) {
    for (const [key, item] of Object.entries(value)) {
      if (!allowed.has(key))
        validateWireValue(item, schema.additionalProperties, components, direction, scope);
    }
  }
  if (schema.unevaluatedProperties !== undefined) {
    const evaluated = evaluatedPropertyNames(value, schema, components, direction, scope);
    for (const [key, item] of Object.entries(value)) {
      if (evaluated.has(key)) continue;
      if (schema.unevaluatedProperties === false)
        throw new TypeError(`unexpected unevaluated property ${key}`);
      validateWireValue(item, schema.unevaluatedProperties, components, direction, scope);
    }
  }
}

/** Implements the standard JSON Schema 2020-12 format-assertion registry. Unknown formats remain application-defined annotations. */
function matchesWireFormat(value: string, format: string): boolean {
  switch (format.toLowerCase()) {
    case "date-time":
      return matchesWireDateTime(value);
    case "date":
      return matchesWireDate(value);
    case "time":
      return matchesWireTime(value);
    case "duration":
      return /^P(?!$)(?:\d+Y)?(?:\d+M)?(?:\d+D)?(?:T(?=\d)(?:\d+H)?(?:\d+M)?(?:\d+(?:\.\d+)?S)?)?$/i.test(
        value,
      );
    case "email":
      return /^[^\s@]+@[^\s@]+\.[^\s@]+$/u.test(value);
    case "idn-email":
      return /^[^\s@]+@[^\s@]+$/u.test(value);
    case "hostname":
      return matchesWireHostname(value);
    case "idn-hostname":
      return matchesWireIDNHostname(value);
    case "ipv4":
      return matchesWireIPv4(value);
    case "ipv6":
      return matchesWireIPv6(value);
    case "uri":
      return matchesWireURI(value, true, false);
    case "uri-reference":
      return matchesWireURI(value, false, false);
    case "iri":
      return matchesWireURI(value, true, true);
    case "iri-reference":
      return matchesWireURI(value, false, true);
    case "uuid":
      return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value);
    case "uri-template":
      return matchesWireURITemplate(value);
    case "json-pointer":
      return /^(?:\/(?:[^~/]|~[01])*)*$/u.test(value);
    case "relative-json-pointer":
      return /^(?:0|[1-9][0-9]*)(?:#|(?:\/(?:[^~/]|~[01])*)*)$/u.test(value);
    case "regex":
      try {
        new RegExp(value, "u");
        return true;
      } catch {
        return false;
      }
    default:
      return true;
  }
}

function matchesWireDate(value: string): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/u.exec(value);
  if (match === null) return false;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const date = new Date(Date.UTC(year, month - 1, day));
  return (
    date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day
  );
}

function matchesWireTime(value: string): boolean {
  const match = /^(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/iu.exec(value);
  if (match === null) return false;
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  const second = Number(match[3]);
  return hour <= 23 && minute <= 59 && second <= 60;
}

function matchesWireDateTime(value: string): boolean {
  const split = value.indexOf("T") >= 0 ? value.split("T", 2) : value.split("t", 2);
  return split.length === 2 && matchesWireDate(split[0]!) && matchesWireTime(split[1]!);
}

function matchesWireHostname(value: string): boolean {
  if (value.length === 0 || value.length > 253 || /[^\x00-\x7f]/u.test(value)) return false;
  const normalized = value.endsWith(".") ? value.slice(0, -1) : value;
  return (
    normalized.length > 0 &&
    normalized.split(".").every((label) => /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/iu.test(label))
  );
}

function matchesWireIDNHostname(value: string): boolean {
  if (/\s/u.test(value) || value.length === 0) return false;
  try {
    return matchesWireHostname(new URL("http://" + value).hostname);
  } catch {
    return false;
  }
}

function matchesWireIPv4(value: string): boolean {
  const segments = value.split(".");
  return (
    segments.length === 4 &&
    segments.every((segment) => /^(?:0|[1-9][0-9]{0,2})$/u.test(segment) && Number(segment) <= 255)
  );
}

function matchesWireIPv6(value: string): boolean {
  if (!value.includes(":")) return false;
  try {
    return new URL("http://[" + value + "]").hostname.length > 0;
  } catch {
    return false;
  }
}

function matchesWireURI(value: string, absolute: boolean, allowUnicode: boolean): boolean {
  if (/[\u0000-\u001f\u007f\s]/u.test(value) || (!allowUnicode && /[^\x00-\x7f]/u.test(value)))
    return false;
  try {
    const parsed = new URL(value, "https://format.invalid/");
    return !absolute || (/^[a-z][a-z0-9+.-]*:/iu.test(value) && parsed.protocol !== "");
  } catch {
    return false;
  }
}

function matchesWireURITemplate(value: string): boolean {
  if (/[\u0000-\u001f\u007f\s]/u.test(value)) return false;
  let depth = 0;
  for (const character of value) {
    if (character === "{") depth++;
    else if (character === "}") {
      depth--;
      if (depth < 0) return false;
    }
  }
  return depth === 0;
}

function decodeSchemaContent(value: string, schema: WireSchema, components: WireSchemas): unknown {
  let decoded = value;
  const encoding = schema.contentEncoding?.toLowerCase();
  if (encoding === "base64" || encoding === "base64url") {
    try {
      const normalized =
        encoding === "base64url" ? value.replaceAll("-", "+").replaceAll("_", "/") : value;
      decoded = new TextDecoder().decode(
        Uint8Array.from(atob(normalized), (character) => character.charCodeAt(0)),
      );
    } catch (cause) {
      throw new TypeError(`contentEncoding ${schema.contentEncoding} cannot decode the value`, {
        cause,
      });
    }
  } else if (
    encoding !== undefined &&
    encoding !== "7bit" &&
    encoding !== "8bit" &&
    encoding !== "binary"
  ) {
    throw new TypeError(`unsupported contentEncoding ${schema.contentEncoding}`);
  }
  const mediaType = schema.contentMediaType;
  if (mediaType === undefined || mediaType === "" || mediaType.toLowerCase().startsWith("text/"))
    return decoded;
  if (isJSONMediaType(mediaType)) {
    try {
      return JSON.parse(decoded);
    } catch (cause) {
      throw new TypeError(`contentMediaType ${mediaType} cannot decode JSON`, { cause });
    }
  }
  if (isXMLMediaType(mediaType)) return decodeXML(decoded, schema.contentSchema ?? {}, components);
  throw new TypeError(`unsupported contentMediaType ${mediaType}`);
}

function discriminatorVariant(
  value: unknown,
  schema: WireSchema,
  components: WireSchemas,
  direction: "encode" | "decode",
): WireSchema | undefined {
  if (!isRecord(value) || schema.discriminator === undefined) return undefined;
  const property = schema.discriminator.property;
  const candidate = value[property];
  if (typeof candidate !== "string") return schema.discriminator.defaultMapping;
  return schema.discriminator.mapping?.[candidate] ?? schema.discriminator.defaultMapping;
}

function evaluatedArrayIndexes(
  value: readonly unknown[],
  schema: WireSchema,
  components: WireSchemas,
  direction: "encode" | "decode",
  dynamicScope: DynamicScope,
  seen = new Set<WireSchema>(),
): Set<number> {
  if (seen.has(schema)) return new Set();
  seen.add(schema);
  const result = new Set<number>();
  const scope = extendDynamicScope(dynamicScope, schema);
  const dynamicTarget = resolveDynamicReference(schema, scope);
  if (dynamicTarget !== undefined)
    mergeIndexes(
      result,
      evaluatedArrayIndexes(value, dynamicTarget, components, direction, scope, seen),
    );
  if (schema.reference !== undefined) {
    const referenced = components[schema.reference];
    if (referenced !== undefined)
      mergeIndexes(
        result,
        evaluatedArrayIndexes(value, referenced, components, direction, scope, seen),
      );
  }
  for (let index = 0; index < Math.min(value.length, schema.prefixItems?.length ?? 0); index++)
    result.add(index);
  if (schema.items !== undefined) {
    for (let index = schema.prefixItems?.length ?? 0; index < value.length; index++)
      result.add(index);
  }
  if (schema.contains !== undefined) {
    value.forEach((item, index) => {
      if (schemaMatches(item, schema.contains!, components, direction, scope)) result.add(index);
    });
  }
  for (const child of schema.allOf ?? [])
    mergeIndexes(result, evaluatedArrayIndexes(value, child, components, direction, scope, seen));
  for (const variants of [schema.oneOf, schema.anyOf]) {
    for (const child of variants ?? [])
      if (schemaMatches(value, child, components, direction, scope))
        mergeIndexes(
          result,
          evaluatedArrayIndexes(value, child, components, direction, scope, seen),
        );
  }
  if (schema.if !== undefined) {
    const child = schemaMatches(value, schema.if, components, direction, scope)
      ? schema.then
      : schema.else;
    if (child !== undefined)
      mergeIndexes(result, evaluatedArrayIndexes(value, child, components, direction, scope, seen));
  }
  return result;
}

function evaluatedPropertyNames(
  value: Readonly<Record<string, unknown>>,
  schema: WireSchema,
  components: WireSchemas,
  direction: "encode" | "decode",
  dynamicScope: DynamicScope,
  seen = new Set<WireSchema>(),
): Set<string> {
  if (seen.has(schema)) return new Set();
  seen.add(schema);
  const result = new Set<string>();
  const scope = extendDynamicScope(dynamicScope, schema);
  const dynamicTarget = resolveDynamicReference(schema, scope);
  if (dynamicTarget !== undefined)
    mergeProperties(
      result,
      evaluatedPropertyNames(value, dynamicTarget, components, direction, scope, seen),
    );
  if (schema.reference !== undefined) {
    const referenced = components[schema.reference];
    if (referenced !== undefined)
      mergeProperties(
        result,
        evaluatedPropertyNames(value, referenced, components, direction, scope, seen),
      );
  }
  for (const [wireName, definition] of Object.entries(schema.properties ?? {})) {
    const name = direction === "encode" ? definition.property : wireName;
    if (Object.hasOwn(value, name)) result.add(name);
  }
  for (const pattern of Object.keys(schema.patternProperties ?? {})) {
    const expression = new RegExp(pattern, "u");
    for (const key of Object.keys(value)) if (expression.test(key)) result.add(key);
  }
  if (schema.additionalProperties !== undefined)
    for (const key of Object.keys(value)) result.add(key);
  for (const child of schema.allOf ?? [])
    mergeProperties(
      result,
      evaluatedPropertyNames(value, child, components, direction, scope, seen),
    );
  for (const variants of [schema.oneOf, schema.anyOf]) {
    for (const child of variants ?? [])
      if (schemaMatches(value, child, components, direction, scope))
        mergeProperties(
          result,
          evaluatedPropertyNames(value, child, components, direction, scope, seen),
        );
  }
  if (schema.if !== undefined) {
    const child = schemaMatches(value, schema.if, components, direction, scope)
      ? schema.then
      : schema.else;
    if (child !== undefined)
      mergeProperties(
        result,
        evaluatedPropertyNames(value, child, components, direction, scope, seen),
      );
  }
  for (const [property, child] of Object.entries(schema.dependentSchemas ?? {})) {
    if (Object.hasOwn(value, property))
      mergeProperties(
        result,
        evaluatedPropertyNames(value, child, components, direction, scope, seen),
      );
  }
  return result;
}

function mergeIndexes(target: Set<number>, source: ReadonlySet<number>): void {
  for (const value of source) target.add(value);
}

function mergeProperties(target: Set<string>, source: ReadonlySet<string>): void {
  for (const value of source) target.add(value);
}

function valueMatchesType(value: unknown, type: string): boolean {
  switch (type) {
    case "null":
      return value === null;
    case "boolean":
      return typeof value === "boolean";
    case "string":
      return typeof value === "string";
    case "number":
      return typeof value === "number" && Number.isFinite(value);
    case "integer":
      return typeof value === "number" && Number.isInteger(value);
    case "array":
      return Array.isArray(value);
    case "object":
      return isRecord(value);
    default:
      return true;
  }
}

function assertFiniteJSONNumbers(value: unknown, seen = new WeakSet<object>()): void {
  if (typeof value === "number") {
    if (!Number.isFinite(value)) throw new TypeError("must be a finite JSON number");
    return;
  }
  if (typeof value !== "object" || value === null || seen.has(value)) return;
  seen.add(value);
  if (Array.isArray(value)) {
    for (const item of value) assertFiniteJSONNumbers(item, seen);
    return;
  }
  for (const item of Object.values(value)) assertFiniteJSONNumbers(item, seen);
}

function wireValueEquals(left: unknown, right: unknown): boolean {
  if (typeof left === "number" && typeof right === "number") return left === right;
  if (Object.is(left, right)) return true;
  if (Array.isArray(left) && Array.isArray(right)) {
    if (left.length !== right.length) return false;
    for (let index = 0; index < left.length; index++) {
      if (Object.hasOwn(left, index) !== Object.hasOwn(right, index)) return false;
      if (Object.hasOwn(left, index) && !wireValueEquals(left[index], right[index])) return false;
    }
    return true;
  }
  if (isRecord(left) && isRecord(right)) {
    const leftKeys = Object.keys(left).sort();
    const rightKeys = Object.keys(right).sort();
    return (
      leftKeys.length === rightKeys.length &&
      leftKeys.every(
        (key, index) => key === rightKeys[index] && wireValueEquals(left[key], right[key]),
      )
    );
  }
  return false;
}

function hasUniqueWireValues(values: readonly unknown[]): boolean {
  for (let index = 0; index < values.length; index++) {
    for (let previous = 0; previous < index; previous++) {
      if (wireValueEquals(values[previous], values[index])) return false;
    }
  }
  return true;
}

function isMultipleOf(value: number, divisor: number): boolean {
  if (!Number.isFinite(divisor) || divisor <= 0) return false;
  const quotient = value / divisor;
  return (
    Math.abs(quotient - Math.round(quotient)) <= Number.EPSILON * Math.max(1, Math.abs(quotient))
  );
}

function schemaMatches(
  value: unknown,
  schema: WireSchema,
  components: WireSchemas,
  direction: "encode" | "decode",
  dynamicScope: DynamicScope = [],
): boolean {
  try {
    validateWireValue(value, schema, components, direction, dynamicScope);
    return true;
  } catch {
    return false;
  }
}

/** Reports whether a media type uses JSON syntax. */
export function isJSONMediaType(contentType: string): boolean {
  const mediaType = contentType.split(";", 1)[0]?.trim().toLowerCase() ?? "";
  return mediaType === "application/json" || mediaType.endsWith("+json");
}

/** Reports whether a media type uses XML syntax. */
export function isXMLMediaType(contentType: string): boolean {
  const mediaType = contentType.split(";", 1)[0]?.trim().toLowerCase() ?? "";
  return mediaType === "application/xml" || mediaType === "text/xml" || mediaType.endsWith("+xml");
}
