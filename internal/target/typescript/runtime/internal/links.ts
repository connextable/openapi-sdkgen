import { isAPIError, type APIError } from "./errors.js";
import { defineOwnDataProperty, isRecord } from "./objects.js";
import type { RawResponse } from "./request.js";

/** One generated OpenAPI Link parameter assignment. */
export interface LinkParameterDefinition {
  /** Target operation input section. */
  readonly location: "path" | "query" | "headerParams" | "cookieParams";
  /** Generated TypeScript property name inside that section. */
  readonly property: string;
  /** Literal value or OpenAPI Runtime Expression. */
  readonly value: unknown;
}

/** One generated OpenAPI Link Object lowered for a source raw response. */
export interface LinkDefinition {
  readonly parameters?: readonly LinkParameterDefinition[];
  /** Literal value or OpenAPI Runtime Expression used as target request body. */
  readonly requestBody?: unknown;
}

/** Per-link invocation values. Explicit input wins over Link-derived defaults. */
export interface LinkInvocation<Input, Options, SourceInput = unknown> {
  /** Source operation input used by `$request` runtime expressions. */
  readonly sourceInput?: SourceInput;
  /** Partial target input that overrides values derived by the Link Object. */
  readonly input?: LinkInputOverride<Input>;
  /** Options applied only to the followed target operation. */
  readonly options?: Options;
}

/** Link invocation whose target operation requires per-request options. */
export type RequiredLinkInvocation<Input, Options, SourceInput = unknown> = Omit<
  LinkInvocation<Input, Options, SourceInput>,
  "options"
> & {
  readonly options: Options;
};

/** Allows one Link call to override individual parameter sections. */
export type LinkInputOverride<Input> = {
  readonly [Section in keyof Input]?: Input[Section] extends Readonly<Record<string, unknown>>
    ? Partial<Input[Section]>
    : Input[Section];
};

/** Resolves an OpenAPI Link Object into the generated target operation input. */
export function resolveLinkInput<Input>(
  response: RawResponse<unknown> | APIError,
  definition: LinkDefinition,
  sourceInput?: unknown,
): Input {
  const source = normalizeLinkResponse(response);
  const result = Object.create(null) as Record<string, unknown>;
  for (const assignment of definition.parameters ?? []) {
    let section = result[assignment.location] as Record<string, unknown> | undefined;
    if (section === undefined) {
      section = Object.create(null) as Record<string, unknown>;
      defineOwnDataProperty(result, assignment.location, section);
    }
    defineOwnDataProperty(
      section,
      assignment.property,
      evaluateLinkValue(source, assignment.value, sourceInput),
    );
  }
  if (definition.requestBody !== undefined)
    defineOwnDataProperty(
      result,
      "body",
      evaluateLinkValue(source, definition.requestBody, sourceInput),
    );
  return result as Input;
}

function normalizeLinkResponse(response: RawResponse<unknown> | APIError): RawResponse<unknown> {
  if (!isAPIError(response)) return response;
  if (response.response === undefined || response.status === undefined)
    throw new TypeError("Link requires an APIError with an HTTP response");
  return {
    status: response.status,
    data: response.data,
    headers: Object.fromEntries(response.response.headers.entries()),
    request: response.request,
    response: response.response,
  };
}

/** Merges Link-derived defaults with explicit target input without mutating either value. */
export function mergeLinkInput<Input>(
  defaults: Input,
  override: LinkInputOverride<Input> | undefined,
): Input {
  if (!isRecord(defaults) || !isRecord(override)) return (override ?? defaults) as Input;
  const result = Object.create(null) as Record<string, unknown>;
  for (const [section, value] of Object.entries(defaults))
    defineOwnDataProperty(result, section, value);
  for (const [section, value] of Object.entries(override)) {
    const existing = result[section];
    if (isRecord(existing) && isRecord(value)) {
      const merged = Object.create(null) as Record<string, unknown>;
      for (const [key, item] of Object.entries(existing)) defineOwnDataProperty(merged, key, item);
      for (const [key, item] of Object.entries(value)) defineOwnDataProperty(merged, key, item);
      defineOwnDataProperty(result, section, merged);
    } else {
      defineOwnDataProperty(result, section, value);
    }
  }
  return result as Input;
}

function evaluateLinkValue(
  response: RawResponse<unknown>,
  value: unknown,
  sourceInput: unknown,
): unknown {
  if (isRecord(value) && isRecord(value["x-sdkgen-link-request-parameter"])) {
    const parameter = value["x-sdkgen-link-request-parameter"];
    const section = typeof parameter.section === "string" ? parameter.section : undefined;
    const property = typeof parameter.property === "string" ? parameter.property : undefined;
    const pointer = typeof parameter.pointer === "string" ? parameter.pointer : undefined;
    if (section === undefined || property === undefined || pointer === undefined)
      throw new TypeError("invalid generated Link request parameter expression");
    const input =
      isRecord(sourceInput) && isRecord(sourceInput[section]) ? sourceInput[section] : undefined;
    const item = input?.[property];
    return pointer === "" ? item : jsonPointerValue(item, pointer);
  }
  if (typeof value !== "string" || !value.startsWith("$")) return value;
  if (value === "$url") return response.response.url;
  if (value === "$statusCode" || value === "$response.statusCode") return response.status;
  const bodyPrefix = "$response.body";
  if (value === bodyPrefix) return response.data;
  if (value.startsWith(bodyPrefix + "#"))
    return jsonPointerValue(response.data, value.slice(bodyPrefix.length + 1));
  const header = /^\$response\.header\.([A-Za-z0-9!#$%&'*+.^_`|~-]+)$/i.exec(value);
  if (header !== null) return response.response.headers.get(header[1]!);
  const requestBodyPrefix = "$request.body";
  if (value === requestBodyPrefix) return isRecord(sourceInput) ? sourceInput.body : undefined;
  if (value.startsWith(requestBodyPrefix + "#"))
    return jsonPointerValue(
      isRecord(sourceInput) ? sourceInput.body : undefined,
      value.slice(requestBodyPrefix.length + 1),
    );
  const requestParameter = /^\$request\.(path|query|header|cookie)\.([^#]+)(#.*)?$/.exec(value);
  if (requestParameter !== null) {
    const section =
      requestParameter[1] === "header"
        ? "headerParams"
        : requestParameter[1] === "cookie"
          ? "cookieParams"
          : requestParameter[1]!;
    const input =
      isRecord(sourceInput) && isRecord(sourceInput[section]) ? sourceInput[section] : undefined;
    const item = input?.[requestParameter[2]!];
    return requestParameter[3] === undefined
      ? item
      : jsonPointerValue(item, requestParameter[3]!.slice(1));
  }
  throw new TypeError(`unsupported OpenAPI Link runtime expression ${value}`);
}

function jsonPointerValue(value: unknown, pointer: string): unknown {
  if (pointer === "") return value;
  if (!pointer.startsWith("/")) throw new TypeError(`invalid JSON Pointer ${pointer}`);
  let current: unknown = value;
  for (const token of pointer.slice(1).split("/")) {
    const key = token.replaceAll("~1", "/").replaceAll("~0", "~");
    if (Array.isArray(current)) {
      if (!/^(0|[1-9][0-9]*)$/.test(key))
        throw new TypeError(`JSON Pointer array token ${key} is invalid`);
      current = current[Number(key)];
      continue;
    }
    if (!isRecord(current) || !Object.hasOwn(current, key)) return undefined;
    current = current[key];
  }
  return current;
}
