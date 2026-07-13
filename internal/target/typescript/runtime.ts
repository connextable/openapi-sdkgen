/** Stable error codes for failures produced by the SDK transport layer. */
export const TransportErrorCode = {
  /** The request input could not be serialized before calling `fetch`. */
  REQUEST_ENCODE_FAILED: "REQUEST_ENCODE_FAILED",
  /** `fetch` failed before an HTTP response was received. */
  NETWORK_ERROR: "NETWORK_ERROR",
  /** The caller's {@link RequestOptions.signal} aborted the request. */
  REQUEST_ABORTED: "REQUEST_ABORTED",
  /** The configured request timeout elapsed before the response completed. */
  REQUEST_TIMEOUT: "REQUEST_TIMEOUT",
  /** The HTTP response body could not be decoded as its declared media type. */
  RESPONSE_DECODE_FAILED: "RESPONSE_DECODE_FAILED",
  /** The operation requires credentials but the client did not provide a usable selection. */
  SECURITY_CREDENTIALS_REQUIRED: "SECURITY_CREDENTIALS_REQUIRED",
  /** Credentials conflict with caller-controlled request data or cannot be applied safely. */
  SECURITY_CREDENTIALS_INVALID: "SECURITY_CREDENTIALS_INVALID",
  /** The host transport lacks a declared capability required by this operation. */
  TRANSPORT_CAPABILITY_REQUIRED: "TRANSPORT_CAPABILITY_REQUIRED",
} as const;

/** Union of all stable SDK transport error codes. */
export type TransportErrorCode = (typeof TransportErrorCode)[keyof typeof TransportErrorCode];

/** Metadata that identifies the server-side request associated with a result or error. */
export interface RequestMetadata {
  /** Server request ID, usually read from the `X-Request-Id` response header. */
  readonly id?: string;
}

/** Values used to construct an {@link APIError}. */
export interface APIErrorOptions<Code extends string, Details = unknown> {
  /** Stable server or transport error code. */
  readonly code: Code;
  /** Human-readable error message. Do not branch application logic on this value. */
  readonly message: string;
  /** Metadata for the request that produced the error. */
  readonly request?: RequestMetadata;
  /** HTTP status code when the server returned a response. */
  readonly status?: number;
  /** Structured server validation or domain-error details, when provided. */
  readonly details?: Details;
  /** Legacy structured server validation fields, when provided. */
  readonly fields?: unknown;
  /** Decoded response body, retained so a Link can follow an error response. */
  readonly data?: unknown;
  /** Original Fetch API response, when one was received. */
  readonly response?: Response;
  /** Original exception that caused a transport or decoding failure. */
  readonly cause?: unknown;
}

/**
 * Normalized error thrown for server-declared errors and SDK transport failures.
 *
 * Use generated error guards or {@link isErrorCode} instead of matching messages.
 */
export class APIError<Code extends string = string, Details = unknown> extends Error {
  /** Standard JavaScript error name. */
  readonly name = "APIError";
  /** Stable server or transport error code. */
  readonly code: Code;
  /** Metadata for the request that produced the error. */
  readonly request: RequestMetadata;
  /** HTTP status code, absent when no response was received. */
  readonly status?: number;
  /** Structured server validation or domain-error details. */
  readonly details?: Details;
  /** Legacy structured server validation fields. */
  readonly fields?: unknown;
  /** Decoded response body, when the server returned a response. */
  readonly data?: unknown;
  /** Original Fetch API response, when available. */
  readonly response?: Response;
  /** Original exception, when this error wraps another failure. */
  readonly cause?: unknown;

  /** Creates a normalized API or transport error. */
  constructor(options: APIErrorOptions<Code, Details>) {
    super(options.message);
    this.code = options.code;
    this.request = options.request ?? {};
    if (options.status !== undefined) this.status = options.status;
    if (options.details !== undefined) this.details = options.details;
    if (options.fields !== undefined) this.fields = options.fields;
    if (options.data !== undefined) this.data = options.data;
    if (options.response !== undefined) this.response = options.response;
    if (options.cause !== undefined) this.cause = options.cause;
  }
}

/** Error produced by request encoding, networking, cancellation, timeout, or response decoding. */
export type TransportError = APIError<TransportErrorCode>;

/**
 * Checks whether a value is an {@link APIError} created by this SDK runtime.
 *
 * @param error Value caught from an SDK call.
 * @returns `true` when `error` is an {@link APIError}.
 */
export function isAPIError(error: unknown): error is APIError {
  return error instanceof APIError;
}

/**
 * Narrows an unknown error to an {@link APIError} with an exact code.
 *
 * @param error Value caught from an SDK call.
 * @param code Server or transport code to match.
 * @returns `true` when both the error type and code match.
 */
export function isErrorCode<Code extends string>(
  error: unknown,
  code: Code,
): error is APIError<Code> {
  return isAPIError(error) && error.code === code;
}

/**
 * Reads a stable error code without requiring a type guard branch.
 *
 * @param error Value caught from an SDK call.
 * @returns Error code for an {@link APIError}; otherwise `undefined`.
 */
export function getErrorCode(error: unknown): string | undefined {
  return isAPIError(error) ? error.code : undefined;
}

/**
 * Reads the server request ID attached to an SDK error.
 *
 * @param error Value caught from an SDK call.
 * @returns Server request ID when available; otherwise `undefined`.
 */
export function getRequestID(error: unknown): string | undefined {
  return isAPIError(error) ? error.request.id : undefined;
}

/** Configuration shared by every operation on a generated client. */
export interface ClientOptions {
  /**
   * Absolute deployment URL including the API version base, for example
   * `https://api.example.com/v1`.
   */
  readonly baseURL?: string;
  /** Absolute origin used only to resolve a selected relative OpenAPI Server URL. */
  readonly origin?: string;
  /** Selects one server from each operation's effective OpenAPI Server list. */
  readonly server?: ServerSelection;
  /** Host-provided codecs for declared non-built-in media types. */
  readonly codecs?: Readonly<Record<string, MediaCodec<unknown>>>;
  /** Optional host transport with explicit capabilities beyond ordinary Fetch. */
  readonly transport?: Transport;
  /** Fetch implementation or wrapper. Defaults to `globalThis.fetch`; the SDK adds no retries. */
  readonly fetch?: typeof globalThis.fetch;
  /** Default headers added to every request. Use dedicated options for SDK-managed headers. */
  readonly headers?: HeadersInit;
  /** Complete default `Authorization` header value, including its authentication scheme. */
  readonly authorization?: string;
  /**
   * Either the default Fetch credentials mode or the host-owned provider for
   * OpenAPI Security Requirement Objects. A string is passed to Fetch; a
   * function is called only after the final operation origin is selected.
   */
  readonly credentials?: RequestCredentials | CredentialProvider;
  /** Default positive timeout in milliseconds. Individual requests may override it. */
  readonly timeoutMS?: number;
  /** Maximum byte count a custom streaming codec may request in one read. */
  readonly maxStreamItemBytes?: number;
}

/** Host-owned encoder/decoder for a declared non-built-in media type. */
export interface MediaCodec<Value> {
  readonly encode?: (value: Value, context: { readonly contentType: string }) => BodyInit | Promise<BodyInit>;
  readonly decode?: (response: Response, context: { readonly contentType: string }) => Value | Promise<Value>;
  /** Decodes one non-streaming inbound server request for a declared custom media type. */
  readonly decodeInbound?: (request: Request, context: { readonly contentType: string }) => Value | Promise<Value>;
  /** Serializes one Parameter Object `content` value into its required string representation. */
  readonly encodeParameter?: (value: Value, context: { readonly contentType: string }) => string | Promise<string>;
  /** Decodes a Parameter Object or response Header Object `content` string. */
  readonly decodeParameter?: (value: string, context: { readonly contentType: string }) => Value | Promise<Value>;
  /** Encodes validated items for one declared custom streaming request body. */
  readonly encodeStream?: (items: AsyncIterable<Value>, context: { readonly contentType: string; readonly signal?: AbortSignal | undefined }) => ReadableStream<Uint8Array> | Promise<ReadableStream<Uint8Array>>;
  /** Decodes one declared custom streaming response without exposing the raw Fetch stream. */
  readonly decodeStream?: (reader: MediaStreamReader, context: { readonly contentType: string; readonly maxFrameBytes: number; readonly signal?: AbortSignal | undefined }) => AsyncIterable<Value>;
  /** Decodes one inbound server stream for a declared custom media type. */
  readonly decodeInboundStream?: (reader: MediaStreamReader, context: { readonly contentType: string; readonly maxFrameBytes: number }) => AsyncIterable<Value>;
}

/** Bounded, cancellable reader supplied to a custom response streaming codec. */
export interface MediaStreamReader {
  /** Reads at most `maxBytes`, which must not exceed the generated stream limit. */
  read(maxBytes: number): Promise<Uint8Array | null>;
  /** Cancels the source response body and releases its reader lock. */
  cancel(reason?: unknown): Promise<void>;
}

/** Explicit capabilities a host transport grants to generated SDK code. */
export interface TransportCapabilities {
  /** The transport can serialize caller-provided Cookie headers. */
  readonly cookieJar?: boolean;
  /** The transport selects a client certificate for an mTLS operation. */
  readonly mutualTLS?: boolean;
  /** Headers Fetch normally withholds, such as Set-Cookie, are readable. */
  readonly readableResponseHeaders?: true | readonly string[];
}

/** Host transport adapter. The SDK never infers elevated transport capabilities. */
export interface Transport {
  readonly fetch: typeof globalThis.fetch;
  readonly capabilities?: TransportCapabilities;
}

/** One declared OpenAPI security scheme in a selected requirement alternative. */
export interface SecuritySchemeDefinition {
  readonly name: string;
  readonly type: "apiKey" | "http" | "oauth2" | "openIdConnect" | "mutualTLS";
  readonly location?: "header" | "query" | "cookie";
  readonly parameterName?: string;
  readonly scheme?: string;
  readonly bearerFormat?: string;
  readonly scopes?: readonly string[];
  /** Lossless OAuth flow declaration for host token acquisition. */
  readonly flows?: Readonly<Record<string, unknown>>;
  readonly openIdConnectUrl?: string;
  readonly oauth2MetadataUrl?: string;
  readonly deprecated?: boolean;
}

/** One OR alternative from an OpenAPI Security Requirement Object array. */
export interface SecurityAlternative {
  readonly id: string;
  readonly schemes: readonly SecuritySchemeDefinition[];
}

/** API-key credential supplied by the host. */
export interface APIKeyCredential {
  readonly kind: "api-key";
  readonly value: string;
}

/** HTTP Basic credential supplied by the host. */
export interface HTTPBasicCredential {
  readonly kind: "http-basic";
  readonly username: string;
  readonly password: string;
}

/** HTTP Bearer credential supplied by the host. */
export interface HTTPBearerCredential {
  readonly kind: "http-bearer";
  readonly token: string;
}

/** Credential for a non-Basic/non-Bearer HTTP authentication scheme. */
export interface HTTPCredential {
  readonly kind: "http";
  readonly value: string;
}

/** OAuth2 or OpenID Connect access token supplied by the host. */
export interface OAuthCredential {
  readonly kind: "oauth2" | "openIdConnect";
  readonly token: string;
}

/** Mutual TLS is selected by a capable host transport, never by the SDK. */
export interface MutualTLSCredential {
  readonly kind: "mutual-tls";
}

/** All credential shapes understood by generated security lowering. */
export type SecurityCredential = APIKeyCredential | HTTPBasicCredential | HTTPBearerCredential | HTTPCredential | OAuthCredential | MutualTLSCredential;

/** Selection returned by a host credential provider. */
export interface SecurityCredentialSelection {
  readonly alternative: SecurityAlternative;
  readonly values: Readonly<Record<string, SecurityCredential>>;
}

/** Context supplied to a host credential provider after final server selection. */
export interface CredentialContext {
  readonly operation: Pick<OperationDefinition, "operationID" | "method" | "path">;
  readonly alternatives: Readonly<Record<string, SecurityAlternative>>;
  readonly origin: string;
}

/** Host-owned security selection and credential acquisition hook. */
export type CredentialProvider = (context: CredentialContext) => SecurityCredentialSelection | Promise<SecurityCredentialSelection>;

/** Options applied to one generated operation call. */
export interface RequestOptions {
  /** Explicit absolute base URL. Generated Link helpers use this for a Link Server Object. */
  readonly baseURL?: string;
  /** Caller-owned cancellation signal. Cancellation is reported as `REQUEST_ABORTED`. */
  readonly signal?: AbortSignal;
  /** Positive timeout in milliseconds for this request, overriding the client default. */
  readonly timeoutMS?: number;
  /** Additional request headers. Contract-owned and SDK-managed headers are rejected here. */
  readonly headers?: HeadersInit;
  /** Complete `Authorization` header value, overriding the client default. */
  readonly authorization?: string;
  /** Requested response media type for operations with multiple representations. */
  readonly accept?: string;
  /** Value sent through the `X-CSRF-Token` header. */
  readonly csrfToken?: string;
  /** Caller-provided value sent through the `X-Request-Id` header. */
  readonly requestID?: string;
  /** Idempotency key sent through the `Idempotency-Key` header. */
  readonly idempotencyKey?: string;
  /** Entity tag precondition sent through the `If-Match` header. */
  readonly ifMatch?: string;
  /** Fetch API credentials mode for this request, overriding the client default. */
  readonly credentials?: RequestCredentials;
  /** Declared additional headers for named multipart form-data parts. */
  readonly multipartHeaders?: Readonly<Record<string, HeadersInit>>;
  /** Selected media type for multipart parts keyed by form name or positional index. */
  readonly multipartContentTypes?: Readonly<Record<string, string>>;
  /** Maximum byte count a custom streaming codec may request in one read. */
  readonly maxStreamItemBytes?: number;
}

/** Binary body values supported by generated request encoders. */
export type BinaryBody = Blob | ArrayBuffer | ArrayBufferView;

/** Endpoint-neutral operation metadata emitted by `sdkgen` for the transport runtime. */
export interface OperationDefinition {
  /** OpenAPI operation ID. */
  readonly operationID: string;
  /** Uppercase HTTP method. */
  readonly method: string;
  /** OpenAPI path template, including any `{parameter}` placeholders. */
  readonly path: string;
  /** Response envelope profile used when decoding successful responses. */
  readonly envelope: string;
  /** Default request-body media type. */
  readonly contentType?: string;
  /** Effective OpenAPI Server alternatives for this operation. */
  readonly servers?: readonly ServerDefinition[];
  /** Generated path, query, header, and cookie parameter definitions. */
  readonly parameters?: readonly ParameterDefinition[];
  /** Case-insensitive set of headers owned by the OpenAPI operation. */
  readonly headerNames?: readonly string[];
  /** Input component schemas used to map TypeScript properties to JSON wire names. */
  readonly inputSchemas?: WireSchemas;
  /** Output component schemas used to map JSON wire names to TypeScript properties. */
  readonly outputSchemas?: WireSchemas;
  /** Supported request-body representations. */
  readonly requestBodies?: readonly WireBodyDefinition[];
  /** Whether the OpenAPI Request Body Object requires a body. */
  readonly requestBodyRequired?: boolean;
  /** Successful response representations keyed by status and media type. */
  readonly responses?: readonly WireResponseDefinition[];
  /** Effective OpenAPI security requirement alternatives for this operation. */
  readonly security?: readonly SecurityAlternative[];
}

/** Stable OpenAPI Server selection supplied to {@link ClientOptions.server}. */
export interface ServerSelection {
  readonly id?: string;
  readonly variables?: Readonly<Record<string, string>>;
}

/** One generated OpenAPI Server Object. */
export interface ServerDefinition {
  readonly id: string;
  readonly url: string;
  readonly variables?: readonly ServerVariableDefinition[];
}

/** One generated OpenAPI Server Variable Object. */
export interface ServerVariableDefinition {
  readonly name: string;
  readonly defaultValue: string;
  readonly enumValues?: readonly string[];
}

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
export interface ParameterDefinition {
  /** HTTP request location of the parameter. */
  readonly location: "path" | "query" | "querystring" | "header" | "cookie";
  /** Exact OpenAPI and HTTP wire name. */
  readonly name: string;
  /** Generated TypeScript property name. */
  readonly property: string;
  /** OpenAPI serialization style. */
  readonly style: string;
  /** Whether objects and arrays are exploded into separate values. */
  readonly explode: boolean;
  /** Preserve RFC 3986 reserved characters in a query value. */
  readonly allowReserved?: boolean;
  /** Whether the parameter must be present before the request is sent. Defaults to false. */
  readonly required?: boolean;
  /** Media type for a content-based parameter. */
  readonly contentType?: string;
  /** Schema used for wire-name transformation before serialization. */
  readonly schema?: WireSchema;
}

/** Successful response including decoded data and the underlying Fetch API response. */
export interface RawResponse<Output, HeaderValues = Headers> {
  /** HTTP status code. */
  readonly status: number;
  /** Normalized response media type without parameters. */
  readonly contentType?: string;
  /** Decoded, typed response body. */
  readonly data: Output;
  /** Response headers. */
	readonly headers: HeaderValues;
  /** Request metadata extracted from the response. */
  readonly request: RequestMetadata;
  /** Original Fetch API response. Its body has already been consumed unless streamed. */
  readonly response: Response;
}

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

/** Allows one Link call to override individual parameter sections. */
export type LinkInputOverride<Input> = {
  readonly [Section in keyof Input]?: Input[Section] extends Readonly<Record<string, unknown>>
    ? Partial<Input[Section]>
    : Input[Section];
};

/** Resolves an OpenAPI Link Object into the generated target operation input. */
export function resolveLinkInput<Input>(response: RawResponse<unknown> | APIError, definition: LinkDefinition, sourceInput?: unknown): Input {
  const source = normalizeLinkResponse(response);
  const result: Record<string, unknown> = {};
  for (const assignment of definition.parameters ?? []) {
    const section = (result[assignment.location] ??= {}) as Record<string, unknown>;
    section[assignment.property] = evaluateLinkValue(source, assignment.value, sourceInput);
  }
  if (definition.requestBody !== undefined) result.body = evaluateLinkValue(source, definition.requestBody, sourceInput);
  return result as Input;
}

function normalizeLinkResponse(response: RawResponse<unknown> | APIError): RawResponse<unknown> {
  if (!isAPIError(response)) return response;
  if (response.response === undefined || response.status === undefined) throw new TypeError("Link requires an APIError with an HTTP response");
  return { status: response.status, data: response.data, headers: response.response.headers, request: response.request, response: response.response };
}

/** Merges Link-derived defaults with explicit target input without mutating either value. */
export function mergeLinkInput<Input>(defaults: Input, override: LinkInputOverride<Input> | undefined): Input {
  if (!isRecord(defaults) || !isRecord(override)) return (override ?? defaults) as Input;
  const result: Record<string, unknown> = { ...defaults };
  for (const [section, value] of Object.entries(override)) {
    const existing = result[section];
    result[section] = isRecord(existing) && isRecord(value) ? { ...existing, ...value } : value;
  }
  return result as Input;
}

function evaluateLinkValue(response: RawResponse<unknown>, value: unknown, sourceInput: unknown): unknown {
	if (isRecord(value) && isRecord(value["x-sdkgen-link-request-parameter"])) {
		const parameter = value["x-sdkgen-link-request-parameter"];
		const section = typeof parameter.section === "string" ? parameter.section : undefined;
		const property = typeof parameter.property === "string" ? parameter.property : undefined;
		const pointer = typeof parameter.pointer === "string" ? parameter.pointer : undefined;
		if (section === undefined || property === undefined || pointer === undefined) throw new TypeError("invalid generated Link request parameter expression");
		const input = isRecord(sourceInput) && isRecord(sourceInput[section]) ? sourceInput[section] : undefined;
		const item = input?.[property];
		return pointer === "" ? item : jsonPointerValue(item, pointer);
	}
  if (typeof value !== "string" || !value.startsWith("$")) return value;
  if (value === "$url") return response.response.url;
  if (value === "$statusCode" || value === "$response.statusCode") return response.status;
  const bodyPrefix = "$response.body";
  if (value === bodyPrefix) return response.data;
  if (value.startsWith(bodyPrefix + "#")) return jsonPointerValue(response.data, value.slice(bodyPrefix.length + 1));
  const header = /^\$response\.header\.([A-Za-z0-9!#$%&'*+.^_`|~-]+)$/i.exec(value);
  if (header !== null) return response.response.headers.get(header[1]!);
	const requestBodyPrefix = "$request.body";
	if (value === requestBodyPrefix) return isRecord(sourceInput) ? sourceInput.body : undefined;
	if (value.startsWith(requestBodyPrefix + "#")) return jsonPointerValue(isRecord(sourceInput) ? sourceInput.body : undefined, value.slice(requestBodyPrefix.length + 1));
	const requestParameter = /^\$request\.(path|query|header|cookie)\.([^#]+)(#.*)?$/.exec(value);
	if (requestParameter !== null) {
		const section = requestParameter[1] === "header" ? "headerParams" : requestParameter[1] === "cookie" ? "cookieParams" : requestParameter[1]!;
		const input = isRecord(sourceInput) && isRecord(sourceInput[section]) ? sourceInput[section] : undefined;
		const item = input?.[requestParameter[2]!];
		return requestParameter[3] === undefined ? item : jsonPointerValue(item, requestParameter[3]!.slice(1));
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
      if (!/^(0|[1-9][0-9]*)$/.test(key)) throw new TypeError(`JSON Pointer array token ${key} is invalid`);
      current = current[Number(key)];
      continue;
    }
    if (!isRecord(current) || !(key in current)) return undefined;
    current = current[key];
  }
  return current;
}

/**
 * Raw response narrowed to an operation's exact status, media type, and output type.
 *
 * @template Status HTTP status literal.
 * @template ContentType Response media-type literal.
 * @template Output Decoded response body type.
 */
export type RawResponseFor<Status extends number, ContentType, Output, HeaderValues = Headers> = Omit<
  RawResponse<Output, HeaderValues>,
  "status" | "contentType"
> &
  Readonly<{
    status: Status;
    contentType: ContentType;
  }>;

/** Low-level request executor used by generated operation bindings. */
export interface RequestFunction {
  /**
   * Sends an operation and returns its decoded response body.
   *
   * @param operation Generated operation metadata.
   * @param input Generated path, query, header, cookie, and body input.
   * @param options Per-request transport options.
   */
  <Output>(
    operation: OperationDefinition,
    input?: unknown,
    options?: RequestOptions,
  ): Promise<Output>;
  /**
   * Sends an operation and returns its decoded body with HTTP response metadata.
   *
   * @param operation Generated operation metadata.
   * @param input Generated path, query, header, cookie, and body input.
   * @param options Per-request transport options.
   */
  raw<Output>(
    operation: OperationDefinition,
    input?: unknown,
    options?: RequestOptions,
  ): Promise<RawResponse<Output>>;
  /** Opens one declared streaming response and lazily decodes its items. */
  stream<Item>(
    operation: OperationDefinition,
    input?: unknown,
    options?: RequestOptions,
  ): AsyncIterable<Item>;
}

/** Callable generated operation that requires typed input. */
export interface InputOperationCall<Input, Output, Options extends RequestOptions, Raw> {
  /** Sends the request and returns the decoded response body. */
  (input: Input, options?: Options): Promise<Output>;
  /** Sends the request and returns decoded data with HTTP response metadata. */
  raw(input: Input, options?: Options): Promise<Raw>;
}

/** Callable generated operation with no input object. */
export interface NoInputOperationCall<Output, Options extends RequestOptions, Raw> {
  /** Sends the request and returns the decoded response body. */
  (options?: Options): Promise<Output>;
  /** Sends the request and returns decoded data with HTTP response metadata. */
  raw(options?: Options): Promise<Raw>;
}

/**
 * Callable operation surface selected from whether the operation accepts input.
 *
 * Generated clients specialize this type with operation-specific input, output,
 * options, and raw-response types.
 */
export type OperationCall<
  Input,
  Output,
  Options extends RequestOptions = RequestOptions,
  Raw = RawResponse<Output>,
> = [Input] extends [never]
  ? NoInputOperationCall<Output, Options, Raw>
  : InputOperationCall<Input, Output, Options, Raw>;

/**
 * Binds generated operation metadata to a low-level request executor.
 *
 * Used by generated clients; applications normally call the generated operation instead.
 */
export function bindOperation<
  Input,
  Output,
  Options extends RequestOptions = RequestOptions,
  Raw = RawResponse<Output>,
>(
  request: RequestFunction,
  operation: OperationDefinition,
  hasInput: boolean,
): OperationCall<Input, Output, Options, Raw> {
  const call = hasInput
    ? (input: Input, options?: Options) => request<Output>(operation, input, options)
    : (options?: Options) => request<Output>(operation, undefined, options);
  const raw = hasInput
    ? (input: Input, options?: Options) => request.raw<Output>(operation, input, options)
    : (options?: Options) => request.raw<Output>(operation, undefined, options);
  return Object.assign(call, { raw }) as OperationCall<Input, Output, Options, Raw>;
}

/**
 * Binds resource path parameters to an input operation.
 *
 * Used to implement generated instance builders such as `api.products(productID)`.
 */
export function bindPathOperation<
  FullInput,
  Input,
  Output,
  Options extends RequestOptions = RequestOptions,
  Raw = RawResponse<Output>,
>(
  operation: InputOperationCall<FullInput, Output, Options, Raw>,
  path: Readonly<Record<string, unknown>>,
  hasInput: boolean,
): OperationCall<Input, Output, Options, Raw> {
  const mergeInput = (input: Input | undefined): FullInput =>
    ({
      ...(isRecord(input) ? input : {}),
      path,
    }) as FullInput;
  const call = hasInput
    ? (input: Input, options?: Options) => operation(mergeInput(input), options)
    : (options?: Options) => operation(mergeInput(undefined), options);
  const raw = hasInput
    ? (input: Input, options?: Options) => operation.raw(mergeInput(input), options)
    : (options?: Options) => operation.raw(mergeInput(undefined), options);
  return Object.assign(call, { raw }) as OperationCall<Input, Output, Options, Raw>;
}

/** Pagination strategy declared by an OpenAPI operation. */
export type PaginationProfile = "cursor" | "offset" | "both";

type QueryInput<Input> = Input extends { readonly query: infer Query } ? Query : never;

/**
 * Input accepted by a generated pagination helper.
 *
 * Operations supporting both strategies require `mode`; cursor and offset fields
 * remain mutually exclusive in every profile.
 */
export type PaginateInput<Input, Profile extends PaginationProfile> = Profile extends "both"
  ?
      | (Omit<Input, "query"> & {
          readonly mode: "cursor";
          readonly query: QueryInput<Input> & { readonly offset?: never };
        })
      | (Omit<Input, "query"> & {
          readonly mode: "offset";
          readonly query: QueryInput<Input> & { readonly cursor?: never };
        })
  : Input & { readonly mode?: never };

/** Function that fetches one typed page for a generated pagination helper. */
export type PageRequest<Input, Page, Options extends RequestOptions = RequestOptions> = (
  input: Input,
  options?: Options,
) => Promise<Page>;

/**
 * Creates a lazy async iterator over all items returned by a paginated operation.
 *
 * The iterator preserves the original filters and sort order. Cursor pagination
 * advances only `cursor`; offset pagination advances only `offset`. No request is
 * sent until iteration begins, and iteration stops when the server signals the end.
 *
 * @param requestPage Generated function that fetches one page.
 * @param profile Pagination strategy declared by the operation.
 * @returns Function producing an {@link AsyncIterable} of decoded items.
 */
export function createPaginator<
  Item,
  Input,
  Page,
  Profile extends PaginationProfile = PaginationProfile,
  Options extends RequestOptions = RequestOptions,
>(
  requestPage: PageRequest<Input, Page, Options>,
  profile: Profile,
): (input: PaginateInput<Input, Profile>, options?: Options) => AsyncIterable<Item> {
  return (input, options) => ({
    async *[Symbol.asyncIterator]() {
      const root: Record<string, unknown> = isRecord(input) ? { ...input } : {};
      const requestedMode = root.mode;
      delete root.mode;
      const mode = resolvePaginationMode(profile, requestedMode);
      const query = isRecord(root.query) ? { ...root.query } : {};
      if (mode === "cursor" && query.offset !== undefined) {
        throw new TypeError("cursor pagination does not accept offset");
      }
      if (mode === "offset" && query.cursor !== undefined) {
        throw new TypeError("offset pagination does not accept cursor");
      }
      root.query = query;
      const seenCursors = new Set<string>();
      if (typeof query.cursor === "string") seenCursors.add(query.cursor);
      for (;;) {
        const page = await requestPage({ ...root, query: { ...query } } as Input, options);
        const items = pageItems(page);
        for (const item of items) yield item as Item;
        if (mode === "cursor") {
          const nextCursor = pagePagination(page).nextCursor;
          if (typeof nextCursor !== "string" || nextCursor === "" || seenCursors.has(nextCursor)) {
            return;
          }
          seenCursors.add(nextCursor);
          query.cursor = nextCursor;
          continue;
        }
        const pagination = pagePagination(page);
        const currentOffset = numberValue(pagination.offset, query.offset, 0);
        const limit = numberValue(pagination.limit, query.limit, items.length);
        const total = typeof pagination.total === "number" ? pagination.total : undefined;
        const nextOffset = currentOffset + limit;
        if (
          limit <= 0 ||
          items.length === 0 ||
          items.length < limit ||
          (total !== undefined && nextOffset >= total)
        )
          return;
        query.offset = nextOffset;
      }
    },
  });
}

const reservedHeaders = new Set([
  "accept",
  "authorization",
  "content-type",
  "idempotency-key",
  "if-match",
  "x-csrf-token",
  "x-request-id",
]);

/**
 * Creates the endpoint-neutral Fetch API request executor used by a generated client.
 *
 * The executor resolves paths, serializes OpenAPI parameters and bodies, maps wire
 * names, applies request options, handles cancellation/timeouts, decodes successful
 * responses, and normalizes failures as {@link APIError}. It never retries requests.
 *
 * @param options Client-wide base URL and transport defaults.
 * @returns Low-level request function used by generated operation bindings.
 */
export function createRequest(options: ClientOptions): RequestFunction {
  const baseURL = options.baseURL === undefined ? undefined : normalizeBaseURL(options.baseURL);
  const fetchImplementation = options.transport?.fetch ?? options.fetch ?? globalThis.fetch;
  if (typeof fetchImplementation !== "function") {
    throw new TypeError("fetch is unavailable; pass ClientOptions.fetch");
  }
	const codecs = normalizeCodecs(options.codecs);

  const execute = async <Output>(
    operation: OperationDefinition,
    input?: unknown,
    requestOptions: RequestOptions = {},
    raw = false,
  ): Promise<Output | RawResponse<Output>> => {
    let encoded: EncodedRequest;
    try {
		const pending = encodeRequest(baseURL, options, codecs, operation, input, requestOptions);
		encoded = isPromise(pending) ? await pending : pending;
		const secured = applyOperationSecurity(options, operation, encoded);
		encoded = isPromise(secured) ? await secured : secured;
    } catch (cause) {
      if (isAPIError(cause)) {
        throw cause;
      }
      throw transportError(
        TransportErrorCode.REQUEST_ENCODE_FAILED,
        `Failed to encode ${operation.operationID} request`,
        cause,
      );
    }

    const timeoutMS = requestOptions.timeoutMS ?? options.timeoutMS;
    const abort = createAbortContext(requestOptions.signal, timeoutMS);
    let responseMetadata: { request: RequestMetadata; status: number; response: Response } | undefined;
    try {
      const init: RequestInit = {
        method: operation.method,
        headers: encoded.headers,
      };
      if (encoded.body !== undefined) {
        init.body = encoded.body as BodyInit;
        if (isReadableStream(encoded.body)) (init as RequestInit & { duplex?: "half" }).duplex = "half";
      }
      if (abort.signal !== undefined) init.signal = abort.signal;
      const credentials = requestOptions.credentials ?? fetchCredentials(options.credentials);
      if (credentials !== undefined) init.credentials = credentials;
      if (abort.signal?.aborted) throw abort.signal.reason;
      assertReadableResponseHeaders(options.transport, operation);
      const response = await awaitAbortable(fetchImplementation(encoded.url, init), abort.signal);
      const request = requestMetadata(response);
      responseMetadata = { request, status: response.status, response };
		const responseDefinition = selectResponseDefinition(operation, response, true);
		if (raw && response.ok && responseDefinition?.itemSchema !== undefined) {
			const contentType = responseContentType(response);
			let headerValues: Readonly<Record<string, unknown>>;
			try {
				headerValues = await decodeResponseHeaders(operation, response, codecs);
			} catch (cause) {
				throw transportErrorFromCause(TransportErrorCode.RESPONSE_DECODE_FAILED, "Failed to decode response headers", cause, responseMetadata);
			}
			return { status: response.status, ...(contentType === undefined ? {} : { contentType }), data: undefined as Output, headers: headerValues, request, response };
		}
      let body: unknown;
      try {
        const decodedBody = await awaitAbortable(decodeResponse(operation, response, request, codecs), abort.signal);
        body = decodeResponseWireValue(operation, response, decodedBody);
      } catch (cause) {
        throw transportErrorFromCause(
          TransportErrorCode.RESPONSE_DECODE_FAILED,
          "Failed to decode response body",
          cause,
          responseMetadata,
        );
      }
      if (!response.ok) {
        throw serverError(response, request, body);
      }
      const data =
        operation.envelope === "data" && isRecord(body) && "data" in body
          ? (body.data as Output)
          : (body as Output);
      if (!raw) return data;
      const contentType = responseContentType(response);
      let headerValues: Readonly<Record<string, unknown>>;
      try {
        headerValues = await decodeResponseHeaders(operation, response, codecs);
      } catch (cause) {
        throw transportErrorFromCause(
          TransportErrorCode.RESPONSE_DECODE_FAILED,
          "Failed to decode response headers",
          cause,
          responseMetadata,
        );
      }
      return {
        status: response.status,
        ...(contentType === undefined ? {} : { contentType }),
        data,
		headers: headerValues,
        request,
        response,
      };
    } catch (cause) {
      if (abort.timedOut()) {
        throw transportErrorFromCause(
          TransportErrorCode.REQUEST_TIMEOUT,
          `Request timed out after ${timeoutMS}ms`,
          cause,
          responseMetadata,
        );
      }
      if (abort.aborted()) {
        throw transportErrorFromCause(
          TransportErrorCode.REQUEST_ABORTED,
          "Request was aborted",
          cause,
          responseMetadata,
        );
      }
      if (isAPIError(cause)) throw cause;
      throw transportError(TransportErrorCode.NETWORK_ERROR, "Network request failed", cause);
    } finally {
      abort.cleanup();
    }
  };
  const request = (<Output>(
    operation: OperationDefinition,
    input?: unknown,
    requestOptions?: RequestOptions,
  ) => execute<Output>(operation, input, requestOptions, false)) as RequestFunction;
  request.raw = <Output>(
    operation: OperationDefinition,
    input?: unknown,
    requestOptions?: RequestOptions,
  ) => execute<Output>(operation, input, requestOptions, true) as Promise<RawResponse<Output>>;
	request.stream = <Item>(operation: OperationDefinition, input?: unknown, requestOptions: RequestOptions = {}): AsyncIterable<Item> =>
		streamOperation<Item>(baseURL, options, codecs, fetchImplementation, operation, input, requestOptions);
  return request;
}

async function* streamOperation<Item>(
  baseURL: string | undefined,
  options: ClientOptions,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
  fetchImplementation: typeof globalThis.fetch,
  operation: OperationDefinition,
  input: unknown,
  requestOptions: RequestOptions,
): AsyncIterable<Item> {
  let encoded: EncodedRequest;
  try {
    const pending = encodeRequest(baseURL, options, codecs, operation, input, requestOptions);
    encoded = isPromise(pending) ? await pending : pending;
    const secured = applyOperationSecurity(options, operation, encoded);
    encoded = isPromise(secured) ? await secured : secured;
  } catch (cause) {
    throw transportError(TransportErrorCode.REQUEST_ENCODE_FAILED, `Failed to encode ${operation.operationID} stream request`, cause);
  }
  const timeoutMS = requestOptions.timeoutMS ?? options.timeoutMS;
  const abort = createAbortContext(requestOptions.signal, timeoutMS);
	let receivedResponse = false;
  try {
    const init: RequestInit = { method: operation.method, headers: encoded.headers, signal: abort.signal };
    if (encoded.body !== undefined) {
      init.body = encoded.body as BodyInit;
      if (isReadableStream(encoded.body)) (init as RequestInit & { duplex?: "half" }).duplex = "half";
    }
    const credentials = requestOptions.credentials ?? fetchCredentials(options.credentials);
    if (credentials !== undefined) init.credentials = credentials;
    if (abort.signal?.aborted) throw abort.signal.reason;
    assertReadableResponseHeaders(options.transport, operation);
    const response = await awaitAbortable(fetchImplementation(encoded.url, init), abort.signal);
		receivedResponse = true;
    const request = requestMetadata(response);
    if (!response.ok) {
      const body = await decodeResponse(operation, response, request, codecs);
      throw serverError(response, request, body);
    }
    const definition = selectResponseDefinition(operation, response, true);
    if (definition?.itemSchema === undefined || response.body === null) {
      throw new TypeError(`response for ${operation.operationID} is not a declared stream`);
    }
    const contentType = response.headers.get("content-type") ?? definition.contentType;
		const maxFrameBytes = resolveMaxStreamItemBytes(requestOptions.maxStreamItemBytes ?? options.maxStreamItemBytes);
    if (isGeneratedStreamMediaType(contentType)) {
      for await (const value of decodeStreamItems(response.body, contentType, definition.itemSchema, operation.outputSchemas ?? {}, codecs, definition.itemEncoding, maxFrameBytes)) {
        yield transformWireValue(value, definition.itemSchema, operation.outputSchemas ?? {}, "decode") as Item;
      }
    } else {
      const codec = codecs.get(normalizeMediaType(contentType));
      if (codec?.decodeStream === undefined) throw new TypeError(`missing decodeStream codec for ${contentType}`);
      const reader = createMediaStreamReader(response.body, maxFrameBytes);
      try {
        for await (const value of codec.decodeStream(reader, { contentType, maxFrameBytes, ...(requestOptions.signal === undefined ? {} : { signal: requestOptions.signal }) })) {
          yield transformWireValue(value, definition.itemSchema, operation.outputSchemas ?? {}, "decode") as Item;
        }
      } finally {
        await reader.cancel();
      }
    }
  } catch (cause) {
    if (isAPIError(cause)) throw cause;
    if (abort.timedOut()) throw transportError(TransportErrorCode.REQUEST_TIMEOUT, `Request timed out after ${timeoutMS}ms`, cause);
    if (abort.aborted()) throw transportError(TransportErrorCode.REQUEST_ABORTED, "Request was aborted", cause);
		if (!receivedResponse) throw transportError(TransportErrorCode.NETWORK_ERROR, "Network request failed", cause);
    throw transportError(TransportErrorCode.RESPONSE_DECODE_FAILED, `Failed to decode ${operation.operationID} stream`, cause);
  } finally {
    abort.cleanup();
  }
}

function resolveMaxStreamItemBytes(value: number | undefined): number {
  const resolved = value ?? 1024 * 1024;
  if (!Number.isSafeInteger(resolved) || resolved <= 0) throw new TypeError("maxStreamItemBytes must be a positive safe integer");
  return resolved;
}

function isGeneratedStreamMediaType(contentType: string): boolean {
  const mediaType = normalizeMediaType(contentType);
  return isSequentialStreamMediaType(mediaType) || mediaType.startsWith("multipart/");
}

function createMediaStreamReader(body: ReadableStream<Uint8Array>, maxFrameBytes: number): MediaStreamReader {
  if (!Number.isSafeInteger(maxFrameBytes) || maxFrameBytes <= 0) throw new TypeError("maxStreamItemBytes must be a positive safe integer");
  const reader = body.getReader();
  let pending = new Uint8Array();
  let done = false;
  let released = false;
  const cancel = async (reason?: unknown): Promise<void> => {
    if (released) return;
    released = true;
    try { await reader.cancel(reason); } finally { reader.releaseLock(); }
  };
  return {
    async read(maxBytes: number): Promise<Uint8Array | null> {
      if (!Number.isSafeInteger(maxBytes) || maxBytes <= 0 || maxBytes > maxFrameBytes) throw new TypeError(`stream read size must be a positive safe integer at most ${maxFrameBytes}`);
      if (released) return null;
      while (pending.length === 0 && !done) {
        const next = await reader.read();
        done = next.done;
        if (next.value !== undefined) pending = next.value;
      }
      if (pending.length === 0) {
        if (!released) { released = true; reader.releaseLock(); }
        return null;
      }
      const result = pending.slice(0, maxBytes);
      pending = pending.slice(result.length);
      return result;
    },
    cancel,
  };
}

async function* decodeStreamItems(
  body: ReadableStream<Uint8Array>,
  contentType: string,
  itemSchema: WireSchema,
  schemas: WireSchemas,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
  itemEncoding: WireEncodingDefinition | undefined,
  maxFrameBytes: number,
): AsyncIterable<unknown> {
  const mediaType = contentType.toLowerCase();
  if (normalizeMediaType(mediaType).startsWith("multipart/")) {
    yield* decodeMultipartStreamItems(body, contentType, itemSchema, schemas, codecs, itemEncoding);
    return;
  }
  const decoder = new TextDecoder();
	const encoder = new TextEncoder();
  let pending = "";
  const reader = body.getReader();
  try {
    while (true) {
      const { done, value } = await reader.read();
      pending += decoder.decode(value, { stream: !done });
      if (mediaType.includes("event-stream")) {
        let boundary: number;
        while ((boundary = pending.search(/\r?\n\r?\n/)) >= 0) {
          const event = pending.slice(0, boundary);
          pending = pending.slice(boundary).replace(/^\r?\n\r?\n/, "");
          const data = event.split(/\r?\n/).filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trimStart()).join("\n");
          if (data !== "") yield parseStreamJSON(data);
        }
      } else if (mediaType.includes("json-seq")) {
        const records = pending.split("\u001e");
        pending = records.pop() ?? "";
        for (const record of records) if (record.trim() !== "") yield parseStreamJSON(record.trim());
      } else {
        let newline: number;
        while ((newline = pending.indexOf("\n")) >= 0) {
          const line = pending.slice(0, newline).replace(/\r$/, "");
          pending = pending.slice(newline + 1);
          if (line.trim() !== "") yield parseStreamJSON(line);
        }
      }
		if (encoder.encode(pending).byteLength > maxFrameBytes) throw new TypeError(`stream item exceeds ${maxFrameBytes} bytes`);
      if (done) break;
    }
    if (pending.trim() !== "") {
      if (mediaType.includes("event-stream")) {
        const data = pending.split(/\r?\n/).filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trimStart()).join("\n");
        if (data !== "") yield parseStreamJSON(data);
      } else yield parseStreamJSON(pending.trim().replace(/^\u001e/, ""));
    }
  } finally {
    try {
      await reader.cancel();
    } finally {
      reader.releaseLock();
    }
  }
}

async function* decodeMultipartStreamItems(
  body: ReadableStream<Uint8Array>,
  contentType: string,
  itemSchema: WireSchema,
  schemas: WireSchemas,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
  itemEncoding: WireEncodingDefinition | undefined,
): AsyncIterable<unknown> {
  for await (const part of decodeMultipartStreamParts(body, contentType)) {
    yield decodeMultipartStreamPart(part, itemSchema, schemas, codecs, itemEncoding);
  }
}

interface MultipartStreamPart {
  readonly headers: Headers;
  readonly bytes: Uint8Array;
}

async function* decodeMultipartStreamParts(
  body: ReadableStream<Uint8Array>,
  contentType: string,
): AsyncIterable<MultipartStreamPart> {
  const boundary = /(?:^|;)\s*boundary=(?:"([^"]+)"|([^;\s]+))/i.exec(contentType)?.[1] ?? /(?:^|;)\s*boundary=(?:"([^"]+)"|([^;\s]+))/i.exec(contentType)?.[2];
  if (boundary === undefined || boundary === "") throw new TypeError("multipart response has no boundary parameter");
  const encoder = new TextEncoder();
  const opening = encoder.encode(`--${boundary}`);
  const separator = encoder.encode(`\r\n--${boundary}`);
  const reader = body.getReader();
  let pending = new Uint8Array();
  let started = false;
  let closed = false;
  try {
    while (!closed) {
      const { done, value } = await reader.read();
      if (value !== undefined) pending = appendStreamBytes(pending, value);
      while (!closed) {
        if (!started) {
          const index = findStreamBytes(pending, opening);
          if (index < 0) break;
          const after = index + opening.length;
          if (pending.length < after + 2) break;
          if (pending[after] === 45 && pending[after + 1] === 45) { closed = true; pending = pending.slice(after + 2); continue; }
          if (pending[after] !== 13 || pending[after + 1] !== 10) throw new TypeError("multipart opening boundary is malformed");
          pending = pending.slice(after + 2);
          started = true;
          continue;
        }
        const index = findStreamBytes(pending, separator);
        if (index < 0) break;
        const after = index + separator.length;
        if (pending.length < after + 2) break;
        const closing = pending[after] === 45 && pending[after + 1] === 45;
        if (!closing && (pending[after] !== 13 || pending[after + 1] !== 10)) throw new TypeError("multipart boundary is malformed");
        const part = pending.slice(0, index);
        pending = pending.slice(after + 2);
        yield parseMultipartStreamPart(part);
        if (closing) closed = true;
      }
      if (done) break;
    }
    if (!closed) throw new TypeError("multipart response ended before its closing boundary");
  } finally {
    try { await reader.cancel(); } finally { reader.releaseLock(); }
  }
}

async function decodeMultipartStreamPart(
  part: MultipartStreamPart,
  itemSchema: WireSchema,
  schemas: WireSchemas,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
  itemEncoding: WireEncodingDefinition | undefined,
): Promise<unknown> {
  const { headers, bytes } = part;
  for (const header of itemEncoding?.headers ?? []) {
    const value = headers.get(header.name);
    if (value === null) {
      if (header.required) throw new TypeError(`multipart part is missing required header ${header.name}`);
      continue;
    }
    const decoded = await decodeResponseHeaderValue(header.name, value, header.schema, header.contentType, header.explode, schemas, codecs);
    validateWireValue(decoded, header.schema, schemas, "decode");
  }
  const declared = itemEncoding?.contentType?.split(",", 1)[0]?.trim();
  const rawPartContentType = headers.get("content-type") ?? declared ?? "text/plain";
  const partContentType = normalizeMediaType(rawPartContentType);
  if (partContentType.startsWith("multipart/")) {
    return decodeMultipartResponse(new Blob([bytes]).stream(), rawPartContentType, {
      contentType: rawPartContentType,
      schema: itemSchema,
      encoding: itemEncoding?.encoding,
      prefixEncoding: itemEncoding?.prefixEncoding,
      itemEncoding: itemEncoding?.itemEncoding,
    }, schemas, codecs);
  }
  if (isJSONMediaType(partContentType)) return parseStreamJSON(new TextDecoder().decode(bytes));
  if (isXMLMediaType(partContentType)) return decodeXML(new TextDecoder().decode(bytes), itemSchema, schemas);
  if (partContentType.startsWith("text/")) return new TextDecoder().decode(bytes);
  if (isBinaryMediaType(partContentType) || itemSchema.contentEncoding === "binary") return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
  const codec = codecs.get(normalizeMediaType(partContentType));
  if (codec?.decode === undefined) throw new TypeError(`missing decode codec for multipart item ${partContentType}`);
  return codec.decode(new Response(bytes, { headers: { "content-type": partContentType } }), { contentType: partContentType });
}

function parseMultipartStreamPart(part: Uint8Array): MultipartStreamPart {
  const split = findStreamBytes(part, new Uint8Array([13, 10, 13, 10]));
  if (split < 0) throw new TypeError("multipart part has no header terminator");
  const headers = parseMultipartStreamHeaders(new TextDecoder().decode(part.slice(0, split)));
  return { headers, bytes: part.slice(split + 4) };
}

function appendStreamBytes(left: Uint8Array, right: Uint8Array): Uint8Array {
  const result = new Uint8Array(left.length + right.length);
  result.set(left);
  result.set(right, left.length);
  return result;
}

function findStreamBytes(source: Uint8Array, wanted: Uint8Array): number {
  if (wanted.length === 0) return 0;
  outer: for (let start = 0; start <= source.length - wanted.length; start++) {
    for (let index = 0; index < wanted.length; index++) if (source[start + index] !== wanted[index]) continue outer;
    return start;
  }
  return -1;
}

function parseMultipartStreamHeaders(source: string): Headers {
  const headers = new Headers();
  for (const line of source.split("\r\n")) {
    const separator = line.indexOf(":");
    if (separator <= 0) throw new TypeError("multipart part has a malformed header");
    headers.append(line.slice(0, separator).trim(), line.slice(separator + 1).trim());
  }
  return headers;
}

function parseStreamJSON(value: string): unknown {
  try {
    return JSON.parse(value);
  } catch (cause) {
    throw new TypeError("stream item is not valid JSON", { cause });
  }
}

async function decodeResponseHeaders(operation: OperationDefinition, response: Response, codecs: ReadonlyMap<string, MediaCodec<unknown>>): Promise<Readonly<Record<string, unknown>>> {
  const definition = selectResponseDefinition(operation, response, false);
  const values: Record<string, unknown> = {};
  for (const header of definition?.headers ?? []) {
    const value = response.headers.get(header.name);
    if (value === null) {
      if (header.required) throw new TypeError(`missing required response header ${header.name}`);
      continue;
    }
    const decoded = await decodeResponseHeaderValue(header.name, value, header.schema, header.contentType, header.explode, operation.outputSchemas ?? {}, codecs);
    validateWireValue(decoded, header.schema, operation.outputSchemas ?? {}, "decode");
    values[header.property] = decodeWireValue(decoded, header.schema, operation.outputSchemas ?? {});
  }
  return values;
}

function fetchCredentials(value: ClientOptions["credentials"]): RequestCredentials | undefined {
  return typeof value === "string" ? value : undefined;
}

function applyOperationSecurity(
	options: ClientOptions,
  operation: OperationDefinition,
  encoded: EncodedRequest,
): EncodedRequest | Promise<EncodedRequest> {
  const declared = operation.security;
  if (declared === undefined || declared.length === 0 || declared.some((alternative) => alternative.schemes.length === 0)) return encoded;
	if (typeof options.credentials !== "function") {
    throw transportError(
      TransportErrorCode.SECURITY_CREDENTIALS_REQUIRED,
      `Operation ${operation.operationID} requires OpenAPI security credentials`,
      undefined,
    );
  }
  const alternatives: Record<string, SecurityAlternative> = {};
  for (const alternative of declared) alternatives[alternative.id] = alternative;
  const context: CredentialContext = {
    operation: { operationID: operation.operationID, method: operation.method, path: operation.path },
    alternatives,
    origin: new URL(encoded.url).origin,
  };
	const selection = options.credentials(context);
  const apply = (resolved: SecurityCredentialSelection): EncodedRequest => {
    const selected = alternatives[resolved?.alternative?.id];
    if (selected === undefined || selected !== resolved.alternative) {
      throw transportError(TransportErrorCode.SECURITY_CREDENTIALS_INVALID, "Credential provider selected an unknown security alternative", undefined);
    }
    const expected = [...selected.schemes].map((scheme) => scheme.name).sort();
    const supplied = Object.keys(resolved.values ?? {}).sort();
    if (expected.length !== supplied.length || expected.some((name, index) => name !== supplied[index])) {
      throw transportError(TransportErrorCode.SECURITY_CREDENTIALS_INVALID, "Credential provider values do not exactly match the selected security alternative", undefined);
    }
    const url = new URL(encoded.url);
	for (const scheme of selected.schemes) applySecurityCredential(options.transport, scheme, resolved.values[scheme.name]!, encoded.headers, url);
    return { ...encoded, url: url.href };
  };
  return isPromise(selection) ? selection.then(apply) : apply(selection);
}

function applySecurityCredential(
	transport: Transport | undefined,
	scheme: SecuritySchemeDefinition,
  credential: SecurityCredential,
  headers: Headers,
  url: URL,
): void {
  switch (scheme.type) {
    case "apiKey": {
      if (credential.kind !== "api-key" || credential.value === "") throw securityCredentialError(scheme.name, "api-key value");
      if (scheme.location === "header") {
        if (headers.has(scheme.parameterName!)) throw securityCollision(scheme.name, `header ${scheme.parameterName}`);
        headers.set(scheme.parameterName!, credential.value);
        return;
      }
      if (scheme.location === "query") {
        if (url.searchParams.has(scheme.parameterName!)) throw securityCollision(scheme.name, `query parameter ${scheme.parameterName}`);
        url.searchParams.set(scheme.parameterName!, credential.value);
        return;
      }
		if (!transport?.capabilities?.cookieJar) {
			throw transportError(TransportErrorCode.TRANSPORT_CAPABILITY_REQUIRED, `Security scheme ${scheme.name} requires a cookie-jar transport`, undefined);
		}
		if (headers.has("Cookie")) throw securityCollision(scheme.name, "Cookie header");
		headers.set("Cookie", `${encodeURIComponent(scheme.parameterName!)}=${encodeURIComponent(credential.value)}`);
		return;
    }
    case "http": {
      if (headers.has("Authorization")) throw securityCollision(scheme.name, "Authorization header");
      if (scheme.scheme === "basic") {
        if (credential.kind !== "http-basic") throw securityCredentialError(scheme.name, "http-basic credential");
        headers.set("Authorization", `Basic ${base64(`${credential.username}:${credential.password}`)}`);
        return;
      }
      if (scheme.scheme === "bearer") {
        if (credential.kind !== "http-bearer" || credential.token === "") throw securityCredentialError(scheme.name, "http-bearer token");
        headers.set("Authorization", `Bearer ${credential.token}`);
        return;
      }
      if (credential.kind !== "http" || credential.value === "") throw securityCredentialError(scheme.name, "http credential");
      headers.set("Authorization", `${scheme.scheme} ${credential.value}`);
      return;
    }
    case "oauth2":
    case "openIdConnect":
      if (credential.kind !== scheme.type || credential.token === "") throw securityCredentialError(scheme.name, `${scheme.type} token`);
      if (headers.has("Authorization")) throw securityCollision(scheme.name, "Authorization header");
      headers.set("Authorization", `Bearer ${credential.token}`);
      return;
    case "mutualTLS":
      if (credential.kind !== "mutual-tls") throw securityCredentialError(scheme.name, "mutual-tls credential");
		if (!transport?.capabilities?.mutualTLS) {
			throw transportError(TransportErrorCode.TRANSPORT_CAPABILITY_REQUIRED, `Security scheme ${scheme.name} requires a mutual-TLS transport`, undefined);
		}
		return;
  }
}

function securityCredentialError(scheme: string, expected: string): TransportError {
  return transportError(TransportErrorCode.SECURITY_CREDENTIALS_INVALID, `Security scheme ${scheme} requires ${expected}`, undefined);
}

function securityCollision(scheme: string, location: string): TransportError {
  return transportError(TransportErrorCode.SECURITY_CREDENTIALS_INVALID, `Security scheme ${scheme} conflicts with caller-supplied ${location}`, undefined);
}

function base64(value: string): string {
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function assertReadableResponseHeaders(transport: Transport | undefined, operation: OperationDefinition): void {
  const readable = transport?.capabilities?.readableResponseHeaders;
  for (const response of operation.responses ?? []) {
    for (const header of response.headers ?? []) {
      if (!header.required || header.name.toLowerCase() !== "set-cookie") continue;
      if (readable === true || (Array.isArray(readable) && readable.some((name) => name.toLowerCase() === "set-cookie"))) continue;
      throw transportError(TransportErrorCode.TRANSPORT_CAPABILITY_REQUIRED, "Reading required Set-Cookie response headers requires a capable transport", undefined);
    }
  }
}

async function decodeResponseHeaderValue(name: string, value: string, schema: WireSchema, contentType: string | undefined, explode: boolean | undefined, schemas: WireSchemas, codecs: ReadonlyMap<string, MediaCodec<unknown>>): Promise<unknown> {
  if (contentType !== undefined) {
    const decoded = decodeHeaderContent(name, value, contentType);
    if (isJSONMediaType(contentType) || contentType.toLowerCase() === "application/x-www-form-urlencoded") return decoded;
    if (isXMLMediaType(contentType)) return decodeXML(value, schema, schemas);
    if (!contentType.toLowerCase().startsWith("text/")) {
      const codec = codecs.get(normalizeMediaType(contentType));
      if (codec?.decodeParameter === undefined) throw new TypeError(`missing decodeParameter codec for response header ${name}`);
      return codec.decodeParameter(value, { contentType });
    }
    value = decoded as string;
  }
  const resolved = resolveHeaderSchema(schema, schemas);
  if (resolved.types?.includes("array")) return value.split(",").map((entry) => decodeResponseHeaderScalar(name, entry, resolved.items ?? {}, schemas));
  if (resolved.types?.includes("object") || resolved.properties !== undefined) {
    const result: Record<string, unknown> = {};
    const tokens = value.split(",");
    if (explode) for (const token of tokens) {
      const separator = token.indexOf("=");
      if (separator < 0) continue;
      const propertyName = token.slice(0, separator);
      const property = resolved.properties?.[propertyName];
      result[propertyName] = decodeResponseHeaderScalar(name, token.slice(separator + 1), property?.schema ?? {}, schemas);
    } else for (let index = 0; index + 1 < tokens.length; index += 2) {
      const property = resolved.properties?.[tokens[index]!];
      result[tokens[index]!] = decodeResponseHeaderScalar(name, tokens[index + 1]!, property?.schema ?? {}, schemas);
    }
    return result;
  }
  return decodeResponseHeaderScalar(name, value, resolved, schemas);
}

function decodeResponseHeaderScalar(name: string, value: string, schema: WireSchema, schemas: WireSchemas): unknown {
  const resolved = resolveHeaderSchema(schema, schemas);
  if (resolved.types?.includes("integer")) {
    const parsed = Number(value);
    if (!Number.isInteger(parsed)) throw new TypeError(`response header ${name} is not an integer`);
    return parsed;
  }
  if (resolved.types?.includes("number")) {
    const parsed = Number(value);
    if (!Number.isFinite(parsed)) throw new TypeError(`response header ${name} is not a number`);
    return parsed;
  }
  if (resolved.types?.includes("boolean")) {
    if (value === "true") return true;
    if (value === "false") return false;
    throw new TypeError(`response header ${name} is not a boolean`);
  }
  return value;
}

function resolveHeaderSchema(schema: WireSchema, schemas: WireSchemas): WireSchema {
  const referenced = schema.reference === undefined ? undefined : schemas[schema.reference];
  return referenced === undefined ? schema : resolveHeaderSchema(referenced, schemas);
}

function decodeHeaderContent(name: string, value: string, contentType: string): unknown {
  if (isJSONMediaType(contentType)) {
    try {
      return JSON.parse(value);
    } catch (cause) {
      throw new TypeError(`response header ${name} is not valid ${contentType}`, { cause });
    }
  }
  if (contentType.toLowerCase() === "application/x-www-form-urlencoded") {
    const result: Record<string, string | string[]> = {};
    for (const [key, item] of new URLSearchParams(value)) {
      const previous = result[key];
      result[key] = previous === undefined ? item : Array.isArray(previous) ? [...previous, item] : [previous, item];
    }
    return result;
  }
  return value;
}

interface EncodedRequest {
  readonly url: string;
  readonly headers: Headers;
  readonly body?: BodyInit | ReadableStream<Uint8Array>;
}

function encodeRequest(
  baseURL: string | undefined,
  client: ClientOptions,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
  operation: OperationDefinition,
  input: unknown,
  options: RequestOptions,
): EncodedRequest | Promise<EncodedRequest> {
  return hasCustomParameterInput(operation, input)
    ? encodeRequestAsync(baseURL, client, codecs, operation, input, options)
    : encodeRequestSynchronous(baseURL, client, codecs, operation, input, options);
}

function hasCustomParameterInput(operation: OperationDefinition, input: unknown): boolean {
  const values = isRecord(input) ? input : {};
  for (const parameter of operation.parameters ?? []) {
    if (parameter.contentType === undefined || !requiresParameterCodec(parameter.contentType)) continue;
    const source = parameter.location === "path" ? values.path : parameter.location === "header" ? values.headerParams : parameter.location === "cookie" ? values.cookieParams : values.query;
    if (isRecord(source) && source[parameter.property] !== undefined) return true;
  }
  return false;
}

function requiresParameterCodec(contentType: string): boolean {
  return !isJSONMediaType(contentType) && !isXMLMediaType(contentType) && contentType.toLowerCase() !== "application/x-www-form-urlencoded" && !contentType.toLowerCase().startsWith("text/");
}

function encodeRequestSynchronous(
  baseURL: string | undefined,
  client: ClientOptions,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
  operation: OperationDefinition,
  input: unknown,
  options: RequestOptions,
): EncodedRequest | Promise<EncodedRequest> {
  const values = isRecord(input) ? input : {};
  const pathValues = isRecord(values.path) ? values.path : {};
  rejectUndefinedArrayValues(pathValues);
  const path = operation.path.replaceAll(/\{([^}]+)\}/g, (_, name: string) => {
    const parameter = findParameter(operation, "path", name);
    const property = parameter?.property ?? name;
    const rawValue = pathValues[property];
    if (rawValue === undefined || rawValue === null) throw new TypeError(`Missing path parameter ${name}`);
    return serializePathParameterSync(parameter, name, encodeParameterWireValue(operation, parameter, rawValue), operation.inputSchemas ?? {});
  });
  const url = new URL(resolveOperationBaseURL(options.baseURL ?? baseURL, client.origin, client.server, operation) + (path.startsWith("/") ? path : `/${path}`));
  const queryValues = isRecord(values.query) ? values.query : {};
  rejectUndefinedArrayValues(queryValues);
  const query = appendQuerySync(queryValues, operation);
  if (query.length > 0) url.search = `${url.search}${url.search === "" ? "?" : "&"}${serializeQuery(query)}`;
  const contractHeaderNames = new Set([...(operation.headerNames ?? []), ...(operation.parameters ?? []).filter((parameter) => parameter.location === "header").map((parameter) => parameter.name)].map((name) => name.toLowerCase()));
  const headers = new Headers();
  appendRawHeaders(headers, client.headers, contractHeaderNames);
  appendRawHeaders(headers, options.headers, contractHeaderNames);
  const headerParams = { ...(isRecord(values.headerParams) ? values.headerParams : {}) };
  rejectUndefinedArrayValues(headerParams);
  for (const [property, value] of Object.entries(headerParams)) {
    if (value === undefined) continue;
    const parameter = findParameterByProperty(operation, "header", property);
    headers.set(parameter?.name ?? property, parameter?.contentType === undefined ? serializeSimpleValue(encodeParameterWireValue(operation, parameter, value), parameter?.explode ?? false) : serializeContentParameterSync(encodeParameterWireValue(operation, parameter, value), parameter.contentType, parameter.schema, operation.inputSchemas ?? {}));
  }
  setHeader(headers, "Authorization", options.authorization ?? client.authorization);
  setHeader(headers, "Accept", options.accept);
  setHeader(headers, "X-CSRF-Token", options.csrfToken);
  setHeader(headers, "X-Request-Id", options.requestID);
  setHeader(headers, "Idempotency-Key", options.idempotencyKey);
  setHeader(headers, "If-Match", options.ifMatch);
  const cookieValues = isRecord(values.cookieParams) ? values.cookieParams : {};
  rejectUndefinedArrayValues(cookieValues);
  assertRequiredParameters(operation, pathValues, queryValues, headerParams, cookieValues);
  const cookies = Object.entries(cookieValues).filter((entry): entry is [string, unknown] => entry[1] !== undefined).flatMap(([property, value]) => serializeCookieSync(operation, property, value));
  if (cookies.length > 0) {
    if (!client.transport?.capabilities?.cookieJar) throw transportError(TransportErrorCode.TRANSPORT_CAPABILITY_REQUIRED, "Sending declared cookie parameters requires a cookie-jar transport", undefined);
    headers.set("Cookie", cookies.join("; "));
  }
  if (!("body" in values) || values.body === undefined) {
    if (operation.requestBodyRequired) throw new TypeError("Missing required request body");
    return { url: url.href, headers };
  }
  rejectUndefinedArrayValues(values.body);
  let contentType = operation.contentType ?? "application/json";
  let bodyValue: unknown = values.body;
  const requestBodies = operation.requestBodies;
  const needsSelection = requestBodies !== undefined && (requestBodies.length > 1 || requestBodies.some((body) => body.contentType.includes("*")));
  if (needsSelection) {
    if (!isRecord(values.body) || typeof values.body.contentType !== "string" || !("value" in values.body)) throw new TypeError("request body media range requires { contentType, value }");
    const selected = selectRequestBodyDefinition(requestBodies!, values.body.contentType);
    if (selected === undefined) throw new TypeError(`request body content type ${values.body.contentType} is not declared by this operation`);
    contentType = values.body.contentType;
    bodyValue = values.body.value;
  }
  const definition = requestBodies === undefined ? undefined : selectRequestBodyDefinition(requestBodies, contentType);
  if (definition?.itemSchema !== undefined && isAsyncIterable(bodyValue) && normalizeMediaType(contentType).startsWith("multipart/")) {
    const encoded = encodeStreamingMultipartBody(contentType, bodyValue, definition.itemSchema, operation.inputSchemas ?? {}, definition.itemEncoding, options.multipartHeaders, options.multipartContentTypes, codecs);
    headers.set("Content-Type", encoded.contentType);
    return { url: url.href, headers, body: encoded.body };
  }
	if (definition?.itemSchema !== undefined && isAsyncIterable(bodyValue) && !isGeneratedStreamMediaType(contentType)) {
		const stream = encodeCustomStreamingRequestBody(contentType, bodyValue, definition.itemSchema, operation.inputSchemas ?? {}, codecs, options.signal);
		const finishStream = (body: ReadableStream<Uint8Array>): EncodedRequest => { headers.set("Content-Type", contentType); return { url: url.href, headers, body }; };
		return isPromise(stream) ? stream.then(finishStream) : finishStream(stream);
	}
  const body = definition?.itemSchema !== undefined && isAsyncIterable(bodyValue) ? encodeSequentialRequestBody(contentType, bodyValue, definition.itemSchema, operation.inputSchemas ?? {}) : encodeRequestBody(contentType, encodeRequestWireValue(operation, contentType, bodyValue), codecs, definition?.schema, operation.inputSchemas ?? {}, definition, options.multipartHeaders, options.multipartContentTypes);
  const finish = (resolved: BodyInit | ReadableStream<Uint8Array>): EncodedRequest => {
    if (!(resolved instanceof FormData)) headers.set("Content-Type", normalizeMediaType(contentType).startsWith("multipart/") && resolved instanceof Blob ? resolved.type : contentType);
    return { url: url.href, headers, body: resolved };
  };
  return isPromise(body) ? body.then(finish) : finish(body);
}

async function encodeRequestAsync(
  baseURL: string | undefined,
  client: ClientOptions,
	codecs: ReadonlyMap<string, MediaCodec<unknown>>,
  operation: OperationDefinition,
  input: unknown,
  options: RequestOptions,
): Promise<EncodedRequest> {
  const values = isRecord(input) ? input : {};
  const pathValues = isRecord(values.path) ? values.path : {};
  rejectUndefinedArrayValues(pathValues);
  let path = operation.path;
  for (const match of operation.path.matchAll(/\{([^}]+)\}/g)) {
    const name = match[1]!;
    const parameter = findParameter(operation, "path", name);
    const property = parameter?.property ?? name;
    const rawValue = pathValues[property];
    if (rawValue === undefined || rawValue === null) {
      throw new TypeError(`Missing path parameter ${name}`);
    }
    const value = encodeParameterWireValue(operation, parameter, rawValue);
    path = path.replace(match[0], await serializePathParameter(parameter, name, value, operation.inputSchemas ?? {}, codecs));
  }
  const operationBaseURL = resolveOperationBaseURL(options.baseURL ?? baseURL, client.origin, client.server, operation);
  const url = new URL(operationBaseURL + (path.startsWith("/") ? path : `/${path}`));
  const queryValues = isRecord(values.query) ? values.query : {};
  rejectUndefinedArrayValues(queryValues);
  const query = await appendQuery(queryValues, operation, codecs);
  if (query.length > 0) url.search = `${url.search}${url.search === "" ? "?" : "&"}${serializeQuery(query)}`;

  const contractHeaderNames = new Set(
    [
      ...(operation.headerNames ?? []),
      ...(operation.parameters ?? [])
        .filter((parameter) => parameter.location === "header")
        .map((parameter) => parameter.name),
    ].map((name) => name.toLowerCase()),
  );
  const headers = new Headers();
  appendRawHeaders(headers, client.headers, contractHeaderNames);
  appendRawHeaders(headers, options.headers, contractHeaderNames);

  const headerParams = {
    ...(isRecord(values.headerParams) ? values.headerParams : {}),
  };
  rejectUndefinedArrayValues(headerParams);
  for (const [property, value] of Object.entries(headerParams)) {
    if (value === undefined) continue;
    const parameter = findParameterByProperty(operation, "header", property);
    const name = parameter?.name ?? property;
    const encodedValue = encodeParameterWireValue(operation, parameter, value);
    headers.set(
      name,
      parameter?.contentType === undefined
        ? serializeSimpleValue(encodedValue, parameter?.explode ?? false)
        : await serializeContentParameter(encodedValue, parameter.contentType, parameter.schema, operation.inputSchemas ?? {}, codecs),
    );
  }
  setHeader(headers, "Authorization", options.authorization ?? client.authorization);
  setHeader(headers, "Accept", options.accept);
  setHeader(headers, "X-CSRF-Token", options.csrfToken);
  setHeader(headers, "X-Request-Id", options.requestID);
  setHeader(headers, "Idempotency-Key", options.idempotencyKey);
  setHeader(headers, "If-Match", options.ifMatch);

  const cookieValues = isRecord(values.cookieParams) ? values.cookieParams : {};
  rejectUndefinedArrayValues(cookieValues);
  assertRequiredParameters(operation, pathValues, queryValues, headerParams, cookieValues);
  const cookiePromises = Object.entries(cookieValues)
    .filter((entry): entry is [string, unknown] => entry[1] !== undefined)
    .map(async ([property, value]) => serializeCookie(operation, property, value, codecs));
  const cookies = (await Promise.all(cookiePromises)).flat();
  if (cookies.length > 0) {
    if (!client.transport?.capabilities?.cookieJar) {
      throw transportError(TransportErrorCode.TRANSPORT_CAPABILITY_REQUIRED, "Sending declared cookie parameters requires a cookie-jar transport", undefined);
    }
    headers.set("Cookie", cookies.join("; "));
  }

  if (!("body" in values) || values.body === undefined) {
	if (operation.requestBodyRequired) throw new TypeError("Missing required request body");
    return { url: url.href, headers };
  }
  rejectUndefinedArrayValues(values.body);
  let contentType = operation.contentType ?? "application/json";
  let bodyValue: unknown = values.body;
  const requestBodies = operation.requestBodies;
  const needsSelection = requestBodies !== undefined && (requestBodies.length > 1 || requestBodies.some((body) => body.contentType.includes("*")));
  if (needsSelection) {
    if (!isRecord(values.body) || typeof values.body.contentType !== "string" || !("value" in values.body)) throw new TypeError("request body media range requires { contentType, value }");
    const selected = selectRequestBodyDefinition(requestBodies!, values.body.contentType);
    if (selected === undefined) throw new TypeError(`request body content type ${values.body.contentType} is not declared by this operation`);
    contentType = values.body.contentType;
    bodyValue = values.body.value;
  }
	const definition = requestBodies === undefined ? undefined : selectRequestBodyDefinition(requestBodies, contentType);
	if (definition?.itemSchema !== undefined && isAsyncIterable(bodyValue) && normalizeMediaType(contentType).startsWith("multipart/")) {
		const encoded = encodeStreamingMultipartBody(contentType, bodyValue, definition.itemSchema, operation.inputSchemas ?? {}, definition.itemEncoding, options.multipartHeaders, options.multipartContentTypes, codecs);
		headers.set("Content-Type", encoded.contentType);
		return { url: url.href, headers, body: encoded.body };
	}
	if (definition?.itemSchema !== undefined && isAsyncIterable(bodyValue) && !isGeneratedStreamMediaType(contentType)) {
		const stream = await encodeCustomStreamingRequestBody(contentType, bodyValue, definition.itemSchema, operation.inputSchemas ?? {}, codecs, options.signal);
		headers.set("Content-Type", contentType);
		return { url: url.href, headers, body: stream };
	}
	const body = definition?.itemSchema !== undefined && isAsyncIterable(bodyValue)
		? encodeSequentialRequestBody(contentType, bodyValue, definition.itemSchema, operation.inputSchemas ?? {})
		: encodeRequestBody(contentType, encodeRequestWireValue(operation, contentType, bodyValue), codecs, definition?.schema, operation.inputSchemas ?? {}, definition, options.multipartHeaders, options.multipartContentTypes);
	const finish = (resolved: BodyInit | ReadableStream<Uint8Array>): EncodedRequest => {
		if (!(resolved instanceof FormData)) {
			const resolvedContentType = normalizeMediaType(contentType).startsWith("multipart/") && resolved instanceof Blob ? resolved.type : contentType;
			headers.set("Content-Type", resolvedContentType);
		}
		return { url: url.href, headers, body: resolved };
	};
	return finish(await body);
}

function assertRequiredParameters(
  operation: OperationDefinition,
  pathValues: Record<string, unknown>,
  queryValues: Record<string, unknown>,
  headerValues: Record<string, unknown>,
  cookieValues: Record<string, unknown>,
): void {
  for (const parameter of operation.parameters ?? []) {
    if (!parameter.required) continue;
    const values = parameter.location === "path" ? pathValues
      : parameter.location === "query" || parameter.location === "querystring" ? queryValues
      : parameter.location === "header" ? headerValues
      : cookieValues;
    if (values[parameter.property] === undefined || values[parameter.property] === null) {
      throw new TypeError(`Missing required ${parameter.location} parameter ${parameter.name}`);
    }
  }
}

function encodeRequestBody(
  contentType: string,
  value: unknown,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
  schema: WireSchema | undefined,
  schemas: WireSchemas,
  definition: WireBodyDefinition | undefined,
  multipartHeaders: Readonly<Record<string, HeadersInit>> | undefined,
	multipartContentTypes: Readonly<Record<string, string>> | undefined,
): BodyInit | Promise<BodyInit> {
  const normalizedContentType = contentType.toLowerCase();
	if (isJSONMediaType(normalizedContentType)) return JSON.stringify(value);
  if (normalizedContentType === "application/x-www-form-urlencoded") {
    if (!isRecord(value)) throw new TypeError("form body must be an object");
    const form = new URLSearchParams();
    for (const [name, item] of formEntries(value, definition?.encoding)) form.append(name, item);
    return form;
  }
	if (normalizedContentType.startsWith("multipart/")) {
		if (definition?.prefixEncoding !== undefined || definition?.itemEncoding !== undefined) {
			if (!Array.isArray(value)) throw new TypeError("positional multipart body must be an array");
      return encodePositionalMultipartBody(contentType, value, definition.prefixEncoding, definition.itemEncoding, multipartHeaders, multipartContentTypes, schema, schemas, codecs);
		}
		if (normalizedContentType !== "multipart/form-data") throw new TypeError(`named multipart encoding requires multipart/form-data, got ${contentType}`);
    if (!isRecord(value)) throw new TypeError("multipart body must be an object");
	if (definition?.encoding?.some((entry) => (entry.headers?.length ?? 0) > 0) || multipartHeaders !== undefined) {
		return encodeMultipartBody(value, definition?.encoding, multipartHeaders, schema, schemas, codecs);
	}
    const form = new FormData();
    const append = (name: string, item: unknown, definition: WireEncodingDefinition | undefined): void => {
      if (item instanceof Blob) form.append(name, item);
      else if (item instanceof ArrayBuffer) form.append(name, new Blob([item]));
      else if (ArrayBuffer.isView(item)) {
        const bytes = new Uint8Array(item.byteLength);
        bytes.set(new Uint8Array(item.buffer, item.byteOffset, item.byteLength));
        form.append(name, new Blob([bytes.buffer]));
      } else if (isRecord(item) || Array.isArray(item)) {
		form.append(name, new Blob([JSON.stringify(item)], { type: definition?.contentType ?? "application/json" }));
      } else if (definition?.contentType !== undefined) form.append(name, new Blob([String(item)], { type: definition.contentType }));
      else form.append(name, String(item));
    };
    for (const [name, item] of Object.entries(value)) {
      if (item === undefined) continue;
		const encoding = definition?.encoding?.find((entry) => entry.name === name);
		if (Array.isArray(item) && encoding?.explode !== false) for (const entry of item) append(name, entry, encoding);
		else append(name, item, encoding);
    }
    return form;
  }
  if (isXMLMediaType(normalizedContentType)) return encodeXML(value, schema ?? {}, schemas);
  if (normalizedContentType.startsWith("text/")) return String(value);
  if (value instanceof Blob || value instanceof ArrayBuffer || ArrayBuffer.isView(value)) {
    return value as BodyInit;
  }
	const codec = codecs.get(normalizeMediaType(contentType));
	if (codec?.encode === undefined) throw new TypeError(`missing encode codec for ${contentType}`);
	return codec.encode(value, { contentType });
}

async function encodePositionalMultipartBody(
  contentType: string,
  values: readonly unknown[],
  prefixEncoding: readonly WireEncodingDefinition[] | undefined,
  itemEncoding: WireEncodingDefinition | undefined,
  suppliedHeaders: Readonly<Record<string, HeadersInit>> | undefined,
  suppliedContentTypes: Readonly<Record<string, string>> | undefined,
	schema: WireSchema | undefined,
	schemas: WireSchemas,
	codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<Blob> {
  const boundary = `----openapi-sdkgen-${multipartBoundaryToken()}`;
  const chunks: BlobPart[] = [];
  for (const [index, value] of values.entries()) {
    const definition = prefixEncoding?.[index] ?? itemEncoding;
    const itemSchema = schema?.prefixItems?.[index] ?? schema?.items ?? {};
    const selectedContentType = resolveMultipartContentType(definition?.contentType, suppliedContentTypes?.[String(index)], defaultMultipartContentType(itemSchema), value);
		const { body, contentType: partContentType } = await multipartPartValue(value, selectedContentType, itemSchema, definition, schemas, codecs);
    const headers = await multipartPartHeaders(undefined, definition, suppliedHeaders?.[String(index)], partContentType, undefined, schemas, codecs);
    chunks.push(`--${boundary}\r\n${headers}\r\n\r\n`, body, "\r\n");
  }
  chunks.push(`--${boundary}--\r\n`);
  return new Blob(chunks, { type: `${contentType}; boundary=${boundary}` });
}

function encodeSequentialRequestBody(
  contentType: string,
  values: AsyncIterable<unknown>,
  itemSchema: WireSchema,
  schemas: WireSchemas,
): ReadableStream<Uint8Array> {
  const mediaType = normalizeMediaType(contentType);
  if (!isSequentialStreamMediaType(mediaType)) throw new TypeError(`unsupported streaming request media type ${contentType}`);
  const iterator = values[Symbol.asyncIterator]();
  const encoder = new TextEncoder();
  return new ReadableStream<Uint8Array>({
    async pull(controller): Promise<void> {
      try {
        const next = await iterator.next();
        if (next.done) {
          controller.close();
          return;
        }
        const value = transformWireValue(next.value, itemSchema, schemas, "encode");
        const json = JSON.stringify(value);
        const record = mediaType.includes("event-stream") ? `data: ${json}\n\n`
          : mediaType.includes("json-seq") ? `\u001e${json}\n`
          : `${json}\n`;
        controller.enqueue(encoder.encode(record));
      } catch (cause) {
        controller.error(cause);
        try { await iterator.return?.(); } catch { /* original error wins */ }
      }
    },
    async cancel(reason): Promise<void> {
      await iterator.return?.(reason);
    },
  });
}

function encodeCustomStreamingRequestBody(
  contentType: string,
  values: AsyncIterable<unknown>,
  itemSchema: WireSchema,
  schemas: WireSchemas,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
  signal: AbortSignal | undefined,
): ReadableStream<Uint8Array> | Promise<ReadableStream<Uint8Array>> {
  const codec = codecs.get(normalizeMediaType(contentType));
  if (codec?.encodeStream === undefined) throw new TypeError(`missing encodeStream codec for ${contentType}`);
  return codec.encodeStream(transformStreamingRequestItems(values, itemSchema, schemas), { contentType, ...(signal === undefined ? {} : { signal }) });
}

async function* transformStreamingRequestItems(values: AsyncIterable<unknown>, itemSchema: WireSchema, schemas: WireSchemas): AsyncIterable<unknown> {
  for await (const value of values) yield transformWireValue(value, itemSchema, schemas, "encode");
}

function encodeStreamingMultipartBody(
  contentType: string,
  values: AsyncIterable<unknown>,
  itemSchema: WireSchema,
  schemas: WireSchemas,
	itemEncoding: WireEncodingDefinition | undefined,
  suppliedHeaders: Readonly<Record<string, HeadersInit>> | undefined,
	suppliedContentTypes: Readonly<Record<string, string>> | undefined,
	codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): { readonly body: ReadableStream<Uint8Array>; readonly contentType: string } {
  const boundary = `----openapi-sdkgen-${multipartBoundaryToken()}`;
  const iterator = values[Symbol.asyncIterator]();
  const encoder = new TextEncoder();
  let index = 0;
  const body = new ReadableStream<Uint8Array>({
    async pull(controller): Promise<void> {
      try {
        const next = await iterator.next();
        if (next.done) {
          controller.enqueue(encoder.encode(`--${boundary}--\r\n`));
          controller.close();
          return;
        }
        const value = transformWireValue(next.value, itemSchema, schemas, "encode");
        const selectedContentType = resolveMultipartContentType(itemEncoding?.contentType, suppliedContentTypes?.[String(index)], defaultMultipartContentType(itemSchema), value);
		const part = await multipartPartValue(value, selectedContentType, itemSchema, itemEncoding, schemas, codecs);
        const headers = await multipartPartHeaders(undefined, itemEncoding, suppliedHeaders?.[String(index)], part.contentType, undefined, schemas, codecs);
        index++;
        const bytes = await new Blob([`--${boundary}\r\n${headers}\r\n\r\n`, part.body, "\r\n"]).arrayBuffer();
        controller.enqueue(new Uint8Array(bytes));
      } catch (cause) {
        controller.error(cause);
        try { await iterator.return?.(); } catch { /* original error wins */ }
      }
    },
    async cancel(reason): Promise<void> { await iterator.return?.(reason); },
  });
  return { body, contentType: `${contentType}; boundary=${boundary}` };
}

function defaultMultipartContentType(schema: WireSchema): string {
  const types = schema.types ?? [];
  if (types.includes("object") || types.includes("array")) return "application/json";
  if (types.includes("string")) return schema.contentEncoding === undefined ? "text/plain" : "application/octet-stream";
  if (types.includes("number") || types.includes("integer") || types.includes("boolean")) return "text/plain";
  return "application/octet-stream";
}

function resolveMultipartContentType(declared: string | undefined, selected: string | undefined, fallback: string, value: unknown): string {
  const candidate = normalizeMediaType(selected ?? (value instanceof Blob && value.type !== "" ? value.type : fallback));
  if (declared === undefined || declared.trim() === "") return candidate;
  const allowed = declared.split(",").map(normalizeMediaType).filter((item) => item !== "");
  if (allowed.some((item) => mediaRangeMatches(item, candidate))) return candidate;
  const exact = allowed.filter((item) => !item.includes("*"));
  if (selected === undefined && exact.length === 1) return exact[0]!;
  throw new TypeError(`multipart part content type ${candidate} is not permitted by ${declared}; select one with RequestOptions.multipartContentTypes`);
}

function mediaRangeMatches(range: string, value: string): boolean {
  const [rangeType, rangeSubtype] = range.split("/", 2);
  const [valueType, valueSubtype] = value.split("/", 2);
  return (rangeType === "*" || rangeType === valueType) && (rangeSubtype === "*" || rangeSubtype === valueSubtype);
}

async function encodeMultipartBody(
  fields: Record<string, unknown>,
  encoding: readonly WireEncodingDefinition[] | undefined,
  suppliedHeaders: Readonly<Record<string, HeadersInit>> | undefined,
  schema: WireSchema | undefined,
	schemas: WireSchemas,
	codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<Blob> {
  const boundary = `----openapi-sdkgen-${multipartBoundaryToken()}`;
  const chunks: BlobPart[] = [];
  const definitions = new Map((encoding ?? []).map((definition) => [definition.name, definition]));
  for (const [name, fieldValue] of Object.entries(fields)) {
    if (fieldValue === undefined) continue;
    const definition = definitions.get(name);
    const values = Array.isArray(fieldValue) && definition?.explode !== false ? fieldValue : [fieldValue];
    for (const item of values) {
			const propertySchema = schema?.properties?.[name]?.schema ?? {};
      const { body, contentType, filename } = await multipartPartValue(item, definition?.contentType, propertySchema, definition, schemas, codecs);
      const headers = await multipartPartHeaders(name, definition, suppliedHeaders?.[name], contentType, filename, schemas, codecs);
      chunks.push(`--${boundary}\r\n${headers}\r\n\r\n`, body, "\r\n");
    }
  }
  chunks.push(`--${boundary}--\r\n`);
  return new Blob(chunks, { type: `multipart/form-data; boundary=${boundary}` });
}

async function multipartPartValue(value: unknown, declaredContentType: string | undefined, schema: WireSchema = {}, definition: WireEncodingDefinition | undefined = undefined, schemas: WireSchemas = {}, codecs: ReadonlyMap<string, MediaCodec<unknown>> = new Map()): Promise<{ body: BlobPart; contentType?: string; filename?: string }> {
	if (declaredContentType !== undefined && normalizeMediaType(declaredContentType).startsWith("multipart/")) {
		if (!Array.isArray(value)) throw new TypeError("nested multipart part must be an array");
		const nested = await encodePositionalMultipartBody(declaredContentType, value, definition?.prefixEncoding, definition?.itemEncoding, undefined, undefined, schema, schemas, codecs);
		return { body: nested, contentType: nested.type };
	}
  if (value instanceof Blob) {
    const file = typeof File !== "undefined" && value instanceof File ? value : undefined;
    return { body: value, contentType: (declaredContentType ?? value.type) || undefined, filename: file?.name };
  }
  if (value instanceof ArrayBuffer) return { body: value, contentType: declaredContentType ?? "application/octet-stream" };
  if (ArrayBuffer.isView(value)) {
    const bytes = new Uint8Array(value.byteLength);
    bytes.set(new Uint8Array(value.buffer, value.byteOffset, value.byteLength));
    return { body: bytes.buffer, contentType: declaredContentType ?? "application/octet-stream" };
  }
	if (declaredContentType !== undefined && requiresMultipartPartCodec(declaredContentType)) {
		const codec = codecs.get(normalizeMediaType(declaredContentType));
		if (codec?.encode === undefined) throw new TypeError(`missing encode codec for multipart part ${declaredContentType}`);
		return { body: multipartCodecBody(await codec.encode(value, { contentType: declaredContentType })), contentType: declaredContentType };
	}
	if (declaredContentType !== undefined && isXMLMediaType(declaredContentType)) {
		return { body: encodeXML(value, schema, schemas), contentType: declaredContentType };
	}
  if (isRecord(value) || Array.isArray(value)) {
    return { body: JSON.stringify(value), contentType: declaredContentType ?? "application/json" };
  }
  return { body: String(value), contentType: declaredContentType };
}

function requiresMultipartPartCodec(contentType: string): boolean {
	const normalized = normalizeMediaType(contentType);
	return !isJSONMediaType(normalized) && !isXMLMediaType(normalized) && !normalized.startsWith("text/") && normalized !== "application/x-www-form-urlencoded" && !isBinaryMediaType(normalized);
}

function multipartCodecBody(value: BodyInit): BlobPart {
	if (typeof value === "string" || value instanceof Blob || value instanceof ArrayBuffer) return value;
	if (ArrayBuffer.isView(value)) return value;
	if (value instanceof URLSearchParams) return value.toString();
	throw new TypeError("multipart part codec must return a string, Blob, ArrayBuffer, ArrayBufferView, or URLSearchParams");
}

async function multipartPartHeaders(
  name: string | undefined,
  definition: WireEncodingDefinition | undefined,
  supplied: HeadersInit | undefined,
  contentType: string | undefined,
  filename: string | undefined,
	schemas: WireSchemas,
	codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<string> {
  const headers = new Headers(supplied);
  const declared = new Map((definition?.headers ?? []).map((header) => [header.name.toLowerCase(), header]));
  for (const header of definition?.headers ?? []) {
    const value = headers.get(header.name);
    if (value === null && header.required) throw new TypeError(`missing required multipart header ${name}.${header.name}`);
    if (value !== null) await validateMultipartHeaderValue(name, header, value, schemas, codecs);
  }
  for (const [headerName] of headers) {
    const normalized = headerName.toLowerCase();
    if (normalized === "content-type" || (name !== undefined && normalized === "content-disposition") || !declared.has(normalized)) {
	  throw new TypeError(`multipart header ${name ?? "position"}.${headerName} is not declared by the Encoding Object`);
    }
  }
	const lines = name === undefined ? [] : [`Content-Disposition: form-data; name="${escapeMultipartToken(name)}"${filename === undefined ? "" : `; filename="${escapeMultipartToken(filename)}"`}`];
  if (contentType !== undefined && contentType !== "") lines.push(`Content-Type: ${contentType}`);
  for (const [headerName, value] of headers) lines.push(`${headerName}: ${value}`);
  return lines.join("\r\n");
}

async function validateMultipartHeaderValue(part: string | undefined, header: WireMultipartHeaderDefinition, value: string, schemas: WireSchemas, codecs: ReadonlyMap<string, MediaCodec<unknown>>): Promise<void> {
  const decoded = await decodeMultipartHeaderValue(`${part ?? "position"}.${header.name}`, value, header.schema, header.contentType, header.explode, schemas, codecs);
  validateWireValue(decoded, header.schema, schemas, "decode");
}

async function decodeMultipartHeaderValue(name: string, value: string, schema: WireSchema, contentType: string | undefined, explode: boolean | undefined, schemas: WireSchemas, codecs: ReadonlyMap<string, MediaCodec<unknown>>): Promise<unknown> {
  if (contentType !== undefined) {
    const decoded = decodeHeaderContent(name, value, contentType);
    if (isJSONMediaType(contentType) || contentType.toLowerCase() === "application/x-www-form-urlencoded") return decoded;
    if (isXMLMediaType(contentType)) return decodeXML(value, schema, schemas);
    if (!contentType.toLowerCase().startsWith("text/")) {
      const codec = codecs.get(normalizeMediaType(contentType));
      if (codec?.decodeParameter === undefined) throw new TypeError(`missing decodeParameter codec for multipart header ${name}`);
      return codec.decodeParameter(value, { contentType });
    }
    value = decoded as string;
  }
  if (schema.types?.includes("array")) return value.split(",").map((item) => decodeMultipartHeaderScalar(name, item, schema.items ?? {}));
  if (schema.types?.includes("object") || schema.properties !== undefined) {
    const result: Record<string, unknown> = {};
    const tokens = value.split(",");
    if (explode) for (const token of tokens) {
      const separator = token.indexOf("=");
      if (separator < 0) continue;
      const property = token.slice(0, separator);
      result[property] = decodeMultipartHeaderScalar(name, token.slice(separator + 1), schema.properties?.[property]?.schema ?? {});
    } else for (let index = 0; index + 1 < tokens.length; index += 2) result[tokens[index]!] = decodeMultipartHeaderScalar(name, tokens[index + 1]!, schema.properties?.[tokens[index]!]?.schema ?? {});
    return result;
  }
  return decodeMultipartHeaderScalar(name, value, schema);
}

function decodeMultipartHeaderScalar(name: string, value: string, schema: WireSchema): unknown {
  if (schema.types?.includes("integer")) { const parsed = Number(value); if (!Number.isInteger(parsed)) throw new TypeError(`multipart header ${name} is not an integer`); return parsed; }
  if (schema.types?.includes("number")) { const parsed = Number(value); if (!Number.isFinite(parsed)) throw new TypeError(`multipart header ${name} is not a number`); return parsed; }
  if (schema.types?.includes("boolean")) { if (value === "true") return true; if (value === "false") return false; throw new TypeError(`multipart header ${name} is not a boolean`); }
  return value;
}

function escapeMultipartToken(value: string): string {
  if (/\r|\n/.test(value)) throw new TypeError("multipart names and filenames cannot contain line breaks");
  return value.replaceAll("\\", "\\\\").replaceAll('"', '\\"');
}

function multipartBoundaryToken(): string {
  const random = globalThis.crypto?.randomUUID?.();
  return random === undefined ? `${Date.now()}-${Math.random().toString(16).slice(2)}` : random;
}

function formEntries(value: Record<string, unknown>, encoding: readonly WireEncodingDefinition[] | undefined): readonly [string, string][] {
  const result: [string, string][] = [];
  for (const [name, item] of Object.entries(value)) {
    if (item === undefined) continue;
    const definition = encoding?.find((entry) => entry.name === name);
    if (definition?.contentType !== undefined && isJSONMediaType(definition.contentType)) {
      result.push([name, JSON.stringify(item)]);
      continue;
    }
    const explode = definition?.explode ?? true;
    if (Array.isArray(item)) {
      if (explode) for (const entry of item) result.push([name, String(entry)]);
      else result.push([name, item.map(String).join(",")]);
      continue;
    }
    if (isRecord(item)) {
      const entries = Object.entries(item).filter((entry): entry is [string, unknown] => entry[1] !== undefined);
      if (explode) for (const [key, entry] of entries) result.push([key, String(entry)]);
      else result.push([name, entries.flatMap(([key, entry]) => [key, String(entry)]).join(",")]);
      continue;
    }
    result.push([name, String(item)]);
  }
  return result;
}

/** Encodes a validated wire value using the OpenAPI XML Object rules. */
export function encodeXML(value: unknown, schema: WireSchema, schemas: WireSchemas): string {
  const rootName = schema.reference ?? schema.xml?.name ?? "root";
  return encodeXMLElement(value, schema, schemas, rootName, true, []);
}

function encodeXMLElement(value: unknown, schema: WireSchema, schemas: WireSchemas, fallbackName: string, root: boolean, dynamicScope: DynamicScope): string {
	const scope = extendDynamicScope(dynamicScope, schema);
	const dynamicTarget = resolveDynamicReference(schema, scope);
	if (dynamicTarget !== undefined) return encodeXMLElement(value, dynamicTarget, schemas, fallbackName, root, scope);
  if (schema.reference !== undefined) {
    const referenced = schemas[schema.reference];
    if (referenced === undefined) throw new TypeError(`XML schema references missing component ${schema.reference}`);
    return encodeXMLElement(value, referenced, schemas, fallbackName, root, scope);
  }
  const xml = schema.xml;
  if (xml?.nodeType === "none") return "";
  if (xml?.nodeType === "text") return escapeXMLText(xmlScalar(value));
  if (xml?.nodeType === "cdata") return `<![CDATA[${xmlScalar(value).replaceAll("]]>", "]]]]><![CDATA[>")}]]>`;
  const name = xmlName(xml, fallbackName);
  if (Array.isArray(value)) {
    const itemSchema = schema.items ?? {};
    const itemName = itemSchema.xml?.name ?? (xml?.wrapped ? itemSchema.xml?.name ?? fallbackName : name);
    const values = value.map((item) => encodeXMLElement(item, itemSchema, schemas, itemName, false, scope)).join("");
    return xml?.wrapped ? wrapXML(name, namespaceAttributes(xml, root), values) : values;
  }
  if (!isRecord(value)) return wrapXML(name, namespaceAttributes(xml, root), escapeXMLText(xmlScalar(value)));
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
  return wrapXML(name, [...namespaceAttributes(xml, root), ...attributes], text + children.join(""));
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
  return [xml.prefix === undefined || xml.prefix === "" ? `xmlns="${escapeXMLAttribute(xml.namespace)}"` : `xmlns:${xml.prefix}="${escapeXMLAttribute(xml.namespace)}"`];
}

function xmlScalar(value: unknown): string {
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") return String(value);
  if (value === null || value === undefined) return "";
  throw new TypeError("XML scalar node requires a string, number, boolean, bigint, null, or undefined value");
}

function escapeXMLText(value: string): string {
  return value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
}

function escapeXMLAttribute(value: string): string {
  return escapeXMLText(value).replaceAll('"', "&quot;").replaceAll("'", "&apos;");
}

function encodeRequestWireValue(
  operation: OperationDefinition,
  contentType: string,
  value: unknown,
): unknown {
  const definition = operation.requestBodies?.find((item) => item.contentType === contentType);
  return definition === undefined
    ? value
    : transformWireValue(value, definition.schema, operation.inputSchemas ?? {}, "encode");
}

function encodeParameterWireValue(
  operation: OperationDefinition,
  parameter: ParameterDefinition | undefined,
  value: unknown,
): unknown {
  return parameter?.schema === undefined
    ? value
    : transformWireValue(value, parameter.schema, operation.inputSchemas ?? {}, "encode");
}

function decodeResponseWireValue(
  operation: OperationDefinition,
  response: Response,
  value: unknown,
): unknown {
  const definition = selectResponseDefinition(operation, response, true);
  if (definition !== undefined && isXMLMediaType(definition.contentType) && typeof value === "string") {
    value = decodeXML(value, definition.schema, operation.outputSchemas ?? {});
  }
  return definition === undefined
    ? value
    : transformWireValue(value, definition.schema, operation.outputSchemas ?? {}, "decode");
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
  const tokens = source.match(/<!\[CDATA\[[\s\S]*?\]\]>|<!--[\s\S]*?-->|<\?[^]*?\?>|<[^>]+>|[^<]+/g) ?? [];
  const roots: XMLNode[] = [];
  const stack: XMLNode[] = [];
  for (const token of tokens) {
    if (token.startsWith("<!--") || token.startsWith("<?")) continue;
    if (token.startsWith("<![CDATA[")) {
      if (stack.length === 0) throw new TypeError("XML character data appears outside the document element");
      stack[stack.length - 1]!.text += token.slice(9, -3);
      continue;
    }
    if (token.startsWith("<!")) throw new TypeError("XML declarations other than comments and CDATA are unsupported");
    if (token.startsWith("</")) {
      const name = token.slice(2, -1).trim();
      const current = stack.pop();
      if (current === undefined || current.name !== name) throw new TypeError(`XML closing tag ${name} does not match the open element`);
      continue;
    }
    if (token.startsWith("<")) {
      const selfClosing = /\/>$/.test(token);
      const body = token.slice(1, selfClosing ? -2 : -1).trim();
      const match = /^([^\s/>]+)([\s\S]*)$/.exec(body);
      if (match === null) throw new TypeError("XML element has no name");
      const node: XMLNode = { name: match[1]!, attributes: parseXMLAttributes(match[2] ?? ""), children: [], text: "" };
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
  if (stack.length !== 0 || roots.length !== 1) throw new TypeError("XML document must contain one balanced root element");
  return roots[0]!;
}

function parseXMLAttributes(source: string): Readonly<Record<string, string>> {
  const result: Record<string, string> = {};
  const expression = /([^\s=]+)\s*=\s*("[^"]*"|'[^']*')/g;
  let match: RegExpExecArray | null;
  while ((match = expression.exec(source)) !== null) result[match[1]!] = unescapeXML(match[2]!.slice(1, -1));
  if (source.replace(expression, "").trim() !== "") throw new TypeError("XML attribute syntax is invalid");
  return result;
}

function decodeXMLNode(node: XMLNode, schema: WireSchema, components: WireSchemas, dynamicScope: DynamicScope): unknown {
	const scope = extendDynamicScope(dynamicScope, schema);
	const dynamicTarget = resolveDynamicReference(schema, scope);
	if (dynamicTarget !== undefined) return decodeXMLNode(node, dynamicTarget, components, scope);
  if (schema.reference !== undefined) {
    const referenced = components[schema.reference];
    if (referenced === undefined) throw new TypeError(`XML schema references missing component ${schema.reference}`);
    return decodeXMLNode(node, referenced, components, scope);
  }
  if (schema.types?.includes("array")) {
    const itemSchema = schema.items ?? {};
    return node.children.map((child) => decodeXMLNode(child, itemSchema, components, scope));
  }
  if (schema.types?.includes("object") || schema.properties !== undefined) {
    const result: Record<string, unknown> = {};
    for (const [wireName, property] of Object.entries(schema.properties ?? {})) {
      const xml = property.schema.xml;
      const name = xmlName(xml, wireName);
      if (xml?.attribute || xml?.nodeType === "attribute") {
        const value = node.attributes[name];
        if (value !== undefined) result[wireName] = decodeXMLScalar(value, property.schema);
        continue;
      }
      if (property.schema.types?.includes("array")) {
        const itemSchema = property.schema.items ?? {};
        const container = xml?.wrapped ? node.children.find((child) => child.name === name) : node;
        if (container !== undefined) {
          const itemName = xmlName(itemSchema.xml, itemSchema.xml?.name ?? wireName);
          result[wireName] = container.children.filter((child) => child.name === itemName).map((child) => decodeXMLNode(child, itemSchema, components, scope));
        }
        continue;
      }
      const child = node.children.find((entry) => entry.name === name);
      if (child !== undefined) result[wireName] = decodeXMLNode(child, property.schema, components, scope);
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
  return value.replaceAll("&lt;", "<").replaceAll("&gt;", ">").replaceAll("&quot;", '"').replaceAll("&apos;", "'").replaceAll("&amp;", "&");
}

function selectResponseDefinition(
  operation: OperationDefinition,
  response: Response,
  requireMediaMatch: boolean,
): WireResponseDefinition | undefined {
  const contentType = responseContentType(response);
  return operation.responses
    ?.filter((item) => {
      if (!statusMatches(item.status, response.status)) return false;
      if (!requireMediaMatch) return true;
      return contentType === undefined ? item.contentType === "" : mediaTypeMatches(item.contentType, contentType);
    })
    .sort((left, right) => {
      const statusDifference = statusMatchScore(right.status, response.status) - statusMatchScore(left.status, response.status);
      if (statusDifference !== 0) return statusDifference;
      return mediaTypeMatchScore(right.contentType, contentType) - mediaTypeMatchScore(left.contentType, contentType);
    })[0];
}

function selectRequestBodyDefinition(bodies: readonly WireBodyDefinition[], contentType: string): WireBodyDefinition | undefined {
  return bodies
    .filter((body) => mediaTypeMatches(body.contentType, contentType))
    .sort((left, right) => mediaTypeMatchScore(right.contentType, contentType) - mediaTypeMatchScore(left.contentType, contentType))[0];
}

function statusMatches(pattern: string, status: number): boolean {
  if (pattern === String(status) || pattern === "default") return true;
  return /^\dXX$/i.test(pattern) && Number(pattern[0]) === Math.floor(status / 100);
}

function statusMatchScore(pattern: string, status: number): number {
  if (pattern === String(status)) return 3;
  if (/^\dXX$/i.test(pattern) && Number(pattern[0]) === Math.floor(status / 100)) return 2;
  return pattern === "default" ? 1 : 0;
}

function mediaTypeMatches(pattern: string, actual: string): boolean {
	const expected = pattern.split(";", 1)[0]?.trim().toLowerCase() ?? "";
	const received = actual.split(";", 1)[0]?.trim().toLowerCase() ?? "";
	if (expected === received || expected === "*/*") return true;
	const [expectedType, expectedSubtype] = expected.split("/", 2);
	const [receivedType, receivedSubtype] = received.split("/", 2);
	if (expectedType === undefined || expectedSubtype === undefined || receivedType === undefined || receivedSubtype === undefined) return false;
	if (expectedType !== "*" && expectedType !== receivedType) return false;
	if (expectedSubtype === "*") return true;
	if (expectedSubtype.startsWith("*+")) return receivedSubtype.endsWith(expectedSubtype.slice(1));
	return false;
}

function mediaTypeMatchScore(pattern: string, actual: string | undefined): number {
	if (actual === undefined) return 0;
	const normalized = pattern.toLowerCase();
	if (normalized === actual) return 3;
	if (normalized.includes("*+")) return 2;
	if (normalized.includes("*")) return 1;
	return 0;
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
  return scope.find((candidate) => candidate.dynamicAnchor === reference.anchor) ?? reference.fallback;
}

function transformWireValue(
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
  if (dynamicTarget !== undefined) return transformWireValue(value, dynamicTarget, components, direction, scope);
  if (schema.reference !== undefined) {
    const referenced = components[schema.reference];
    return referenced === undefined
      ? value
      : transformWireValue(value, referenced, components, direction, scope);
  }
  if (Array.isArray(value)) {
    return value.map((item, index) => {
      const itemSchema = schema.prefixItems?.[index] ?? schema.items;
      return itemSchema === undefined
        ? item
        : transformWireValue(item, itemSchema, components, direction, scope);
    });
  }
  let transformed: unknown = value;
  if (
    isRecord(transformed) &&
    (schema.properties !== undefined || schema.additionalProperties !== undefined)
  ) {
    const source = transformed;
    const result: Record<string, unknown> = { ...source };
    const known = new Set<string>();
    for (const [wireName, propertyDefinition] of Object.entries(schema.properties ?? {})) {
      const sourceName = direction === "encode" ? propertyDefinition.property : wireName;
      const targetName = direction === "encode" ? wireName : propertyDefinition.property;
      known.add(sourceName);
      known.add(targetName);
      if (!(sourceName in source)) continue;
      if (sourceName !== targetName) delete result[sourceName];
      result[targetName] = transformWireValue(
        source[sourceName],
        propertyDefinition.schema,
        components,
        direction,
        scope,
      );
    }
    if (schema.additionalProperties !== undefined && schema.additionalProperties !== false) {
      for (const [key, item] of Object.entries(result)) {
        if (!known.has(key)) {
          result[key] = transformWireValue(
            item,
            schema.additionalProperties,
            components,
            direction,
            scope,
          );
        }
      }
    }
    transformed = result;
  }
  for (const variant of schema.allOf ?? []) {
    transformed = transformWireValue(transformed, variant, components, direction, scope);
  }
	if (schema.if !== undefined) {
		const branch = schemaMatches(transformed, schema.if, components, direction, scope) ? schema.then : schema.else;
		if (branch !== undefined) transformed = transformWireValue(transformed, branch, components, direction, scope);
	}
	for (const variants of [schema.oneOf, schema.anyOf]) {
    if (variants === undefined) continue;
		const selected = schema.discriminator !== undefined
			? discriminatorVariant(transformed, schema, components, direction) ?? variants.find((variant) => schemaMatches(transformed, variant, components, direction, scope))
			: variants.find((variant) => schemaMatches(transformed, variant, components, direction, scope));
    if (selected !== undefined) transformed = transformWireValue(transformed, selected, components, direction, scope);
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
  const scope = extendDynamicScope(dynamicScope, schema);
  if (schema.boolean === false) throw new TypeError("schema is false");
  if (value === undefined) return;
  const dynamicTarget = resolveDynamicReference(schema, scope);
  if (dynamicTarget !== undefined) {
    validateWireValue(value, dynamicTarget, components, direction, scope);
    return;
  }
  if (schema.reference !== undefined) {
    const referenced = components[schema.reference];
    if (referenced !== undefined) validateWireValue(value, referenced, components, direction, scope);
    return;
  }
  if (schema.types !== undefined && !schema.types.some((type) => valueMatchesType(value, type))) {
    throw new TypeError(`expected ${schema.types.join(" | ")}`);
  }
  if (schema.constValue !== undefined && !wireValueEquals(value, schema.constValue)) {
    throw new TypeError("value does not match const");
  }
  if (schema.enumValues !== undefined && !schema.enumValues.some((item) => wireValueEquals(value, item))) {
    throw new TypeError("value is not in enum");
  }
  if (typeof value === "number") {
    if (schema.multipleOf !== undefined && !isMultipleOf(value, schema.multipleOf)) throw new TypeError(`must be a multiple of ${schema.multipleOf}`);
    if (schema.maximum !== undefined && value > schema.maximum) throw new TypeError(`must be <= ${schema.maximum}`);
    if (schema.exclusiveMaximum !== undefined && value >= schema.exclusiveMaximum) throw new TypeError(`must be < ${schema.exclusiveMaximum}`);
    if (schema.minimum !== undefined && value < schema.minimum) throw new TypeError(`must be >= ${schema.minimum}`);
    if (schema.exclusiveMinimum !== undefined && value <= schema.exclusiveMinimum) throw new TypeError(`must be > ${schema.exclusiveMinimum}`);
  }
  if (typeof value === "string") {
    if (schema.minLength !== undefined && [...value].length < schema.minLength) throw new TypeError(`must have length >= ${schema.minLength}`);
    if (schema.maxLength !== undefined && [...value].length > schema.maxLength) throw new TypeError(`must have length <= ${schema.maxLength}`);
    if (schema.pattern !== undefined && !(new RegExp(schema.pattern, "u")).test(value)) throw new TypeError(`must match pattern ${schema.pattern}`);
    if (schema.formatAssertion && schema.format !== undefined && !matchesWireFormat(value, schema.format)) throw new TypeError(`must match format ${schema.format}`);
  }
  if (schema.oneOf !== undefined) {
    const matches = schema.oneOf.filter((item) => schemaMatches(value, item, components, direction, scope));
    if (matches.length !== 1) throw new TypeError(`oneOf requires exactly one matching schema, got ${matches.length}`);
  }
  if (schema.anyOf !== undefined && !schema.anyOf.some((item) => schemaMatches(value, item, components, direction, scope))) {
    throw new TypeError("anyOf requires at least one matching schema");
  }
  if (schema.not !== undefined && schemaMatches(value, schema.not, components, direction, scope)) {
    throw new TypeError("must not match negated schema");
  }
	if (schema.if !== undefined) {
		const branch = schemaMatches(value, schema.if, components, direction, scope) ? schema.then : schema.else;
		if (branch !== undefined) validateWireValue(value, branch, components, direction, scope);
	}
	for (const branch of schema.allOf ?? []) validateWireValue(value, branch, components, direction, scope);
	if (schema.contentSchema !== undefined && typeof value === "string") {
		validateWireValue(decodeSchemaContent(value, schema, components), schema.contentSchema, components, direction, scope);
	}
  if (Array.isArray(value)) {
    if (schema.minItems !== undefined && value.length < schema.minItems) throw new TypeError(`must contain at least ${schema.minItems} items`);
    if (schema.maxItems !== undefined && value.length > schema.maxItems) throw new TypeError(`must contain at most ${schema.maxItems} items`);
    if (schema.uniqueItems && !hasUniqueWireValues(value)) throw new TypeError("must contain unique items");
		if (schema.contains !== undefined) {
			const matches = value.filter((item) => schemaMatches(item, schema.contains!, components, direction, scope)).length;
			const minimum = schema.minContains ?? 1;
			if (matches < minimum) throw new TypeError(`must contain at least ${minimum} matching items`);
			if (schema.maxContains !== undefined && matches > schema.maxContains) throw new TypeError(`must contain at most ${schema.maxContains} matching items`);
		}
    value.forEach((item, index) => {
      const itemSchema = schema.prefixItems?.[index] ?? schema.items;
      if (itemSchema !== undefined) validateWireValue(item, itemSchema, components, direction, scope);
    });
		if (schema.unevaluatedItems !== undefined) {
			const evaluated = evaluatedArrayIndexes(value, schema, components, direction, scope);
			for (const [index, item] of value.entries()) {
				if (evaluated.has(index)) continue;
				if (schema.unevaluatedItems === false) throw new TypeError(`unexpected unevaluated item ${index}`);
				validateWireValue(item, schema.unevaluatedItems, components, direction, scope);
			}
		}
    return;
  }
  if (!isRecord(value)) return;
	if (schema.minProperties !== undefined && Object.keys(value).length < schema.minProperties) throw new TypeError(`must contain at least ${schema.minProperties} properties`);
	if (schema.maxProperties !== undefined && Object.keys(value).length > schema.maxProperties) throw new TypeError(`must contain at most ${schema.maxProperties} properties`);
  const properties = schema.properties ?? {};
  const allowed = new Set<string>();
  for (const [wireName, definition] of Object.entries(properties)) {
    const sourceName = direction === "encode" ? definition.property : wireName;
    allowed.add(sourceName);
    if (sourceName in value) validateWireValue(value[sourceName], definition.schema, components, direction, scope);
  }
  for (const required of schema.required ?? []) {
    const definition = properties[required];
    const sourceName = direction === "encode" && definition !== undefined ? definition.property : required;
    if (!(sourceName in value) || value[sourceName] === undefined) {
      throw new TypeError(`missing required property ${required}`);
    }
  }
	for (const [property, required] of Object.entries(schema.dependentRequired ?? {})) {
		const sourceProperty = direction === "encode" && properties[property] !== undefined ? properties[property].property : property;
		if (!(sourceProperty in value) || value[sourceProperty] === undefined) continue;
		for (const dependency of required) {
			const sourceDependency = direction === "encode" && properties[dependency] !== undefined ? properties[dependency].property : dependency;
			if (!(sourceDependency in value) || value[sourceDependency] === undefined) {
				throw new TypeError(`property ${property} requires property ${dependency}`);
			}
		}
	}
	for (const [property, dependency] of Object.entries(schema.dependentSchemas ?? {})) {
		const sourceProperty = direction === "encode" && properties[property] !== undefined ? properties[property].property : property;
		if (sourceProperty in value && value[sourceProperty] !== undefined) validateWireValue(value, dependency, components, direction, scope);
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
		for (const key of Object.keys(value)) validateWireValue(key, schema.propertyNames, components, direction, scope);
	}
  if (schema.additionalProperties === false) {
    for (const key of Object.keys(value)) {
      if (!allowed.has(key)) throw new TypeError(`unexpected property ${key}`);
    }
  } else if (schema.additionalProperties !== undefined) {
    for (const [key, item] of Object.entries(value)) {
		if (!allowed.has(key)) validateWireValue(item, schema.additionalProperties, components, direction, scope);
    }
  }
	if (schema.unevaluatedProperties !== undefined) {
		const evaluated = evaluatedPropertyNames(value, schema, components, direction, scope);
		for (const [key, item] of Object.entries(value)) {
			if (evaluated.has(key)) continue;
			if (schema.unevaluatedProperties === false) throw new TypeError(`unexpected unevaluated property ${key}`);
			validateWireValue(item, schema.unevaluatedProperties, components, direction, scope);
		}
	}
}

/** Implements the standard JSON Schema 2020-12 format-assertion registry. Unknown formats remain application-defined annotations. */
function matchesWireFormat(value: string, format: string): boolean {
  switch (format.toLowerCase()) {
    case "date-time": return matchesWireDateTime(value);
    case "date": return matchesWireDate(value);
    case "time": return matchesWireTime(value);
    case "duration": return /^P(?!$)(?:\d+Y)?(?:\d+M)?(?:\d+D)?(?:T(?=\d)(?:\d+H)?(?:\d+M)?(?:\d+(?:\.\d+)?S)?)?$/i.test(value);
    case "email": return /^[^\s@]+@[^\s@]+\.[^\s@]+$/u.test(value);
    case "idn-email": return /^[^\s@]+@[^\s@]+$/u.test(value);
    case "hostname": return matchesWireHostname(value);
    case "idn-hostname": return matchesWireIDNHostname(value);
    case "ipv4": return matchesWireIPv4(value);
    case "ipv6": return matchesWireIPv6(value);
    case "uri": return matchesWireURI(value, true, false);
    case "uri-reference": return matchesWireURI(value, false, false);
    case "iri": return matchesWireURI(value, true, true);
    case "iri-reference": return matchesWireURI(value, false, true);
    case "uuid": return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value);
    case "uri-template": return matchesWireURITemplate(value);
    case "json-pointer": return /^(?:\/(?:[^~/]|~[01])*)*$/u.test(value);
    case "relative-json-pointer": return /^(?:0|[1-9][0-9]*)(?:#|(?:\/(?:[^~/]|~[01])*)*)$/u.test(value);
    case "regex": try { new RegExp(value, "u"); return true; } catch { return false; }
    default: return true;
  }
}

function matchesWireDate(value: string): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/u.exec(value);
  if (match === null) return false;
  const year = Number(match[1]); const month = Number(match[2]); const day = Number(match[3]);
  const date = new Date(Date.UTC(year, month - 1, day));
  return date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day;
}

function matchesWireTime(value: string): boolean {
  const match = /^(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/iu.exec(value);
  if (match === null) return false;
  const hour = Number(match[1]); const minute = Number(match[2]); const second = Number(match[3]);
  return hour <= 23 && minute <= 59 && second <= 60;
}

function matchesWireDateTime(value: string): boolean {
  const split = value.indexOf("T") >= 0 ? value.split("T", 2) : value.split("t", 2);
  return split.length === 2 && matchesWireDate(split[0]!) && matchesWireTime(split[1]!);
}

function matchesWireHostname(value: string): boolean {
  if (value.length === 0 || value.length > 253 || /[^\x00-\x7f]/u.test(value)) return false;
  const normalized = value.endsWith(".") ? value.slice(0, -1) : value;
  return normalized.length > 0 && normalized.split(".").every((label) => /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/iu.test(label));
}

function matchesWireIDNHostname(value: string): boolean {
  if (/\s/u.test(value) || value.length === 0) return false;
  try { return matchesWireHostname(new URL("http://" + value).hostname); } catch { return false; }
}

function matchesWireIPv4(value: string): boolean {
  const segments = value.split(".");
  return segments.length === 4 && segments.every((segment) => /^(?:0|[1-9][0-9]{0,2})$/u.test(segment) && Number(segment) <= 255);
}

function matchesWireIPv6(value: string): boolean {
  if (!value.includes(":")) return false;
  try { return new URL("http://[" + value + "]").hostname.length > 0; } catch { return false; }
}

function matchesWireURI(value: string, absolute: boolean, allowUnicode: boolean): boolean {
  if (/[\u0000-\u001f\u007f\s]/u.test(value) || (!allowUnicode && /[^\x00-\x7f]/u.test(value))) return false;
  try {
    const parsed = new URL(value, "https://format.invalid/");
    return !absolute || /^[a-z][a-z0-9+.-]*:/iu.test(value) && parsed.protocol !== "";
  } catch { return false; }
}

function matchesWireURITemplate(value: string): boolean {
  if (/[\u0000-\u001f\u007f\s]/u.test(value)) return false;
  let depth = 0;
  for (const character of value) {
    if (character === "{") depth++;
    else if (character === "}") { depth--; if (depth < 0) return false; }
  }
  return depth === 0;
}

function decodeSchemaContent(value: string, schema: WireSchema, components: WireSchemas): unknown {
  let decoded = value;
  const encoding = schema.contentEncoding?.toLowerCase();
  if (encoding === "base64" || encoding === "base64url") {
    try {
      const normalized = encoding === "base64url" ? value.replaceAll("-", "+").replaceAll("_", "/") : value;
      decoded = new TextDecoder().decode(Uint8Array.from(atob(normalized), (character) => character.charCodeAt(0)));
    } catch (cause) {
      throw new TypeError(`contentEncoding ${schema.contentEncoding} cannot decode the value`, { cause });
    }
  } else if (encoding !== undefined && encoding !== "7bit" && encoding !== "8bit" && encoding !== "binary") {
    throw new TypeError(`unsupported contentEncoding ${schema.contentEncoding}`);
  }
  const mediaType = schema.contentMediaType;
  if (mediaType === undefined || mediaType === "" || mediaType.toLowerCase().startsWith("text/")) return decoded;
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
	if (dynamicTarget !== undefined) return evaluatedArrayIndexes(value, dynamicTarget, components, direction, scope, seen);
	if (schema.reference !== undefined) {
		const referenced = components[schema.reference];
		if (referenced !== undefined) return evaluatedArrayIndexes(value, referenced, components, direction, scope, seen);
	}
	for (let index = 0; index < Math.min(value.length, schema.prefixItems?.length ?? 0); index++) result.add(index);
	if (schema.items !== undefined || schema.additionalProperties !== undefined) {
		for (let index = schema.prefixItems?.length ?? 0; index < value.length; index++) result.add(index);
	}
	if (schema.contains !== undefined) {
		value.forEach((item, index) => { if (schemaMatches(item, schema.contains!, components, direction, scope)) result.add(index); });
	}
	for (const child of schema.allOf ?? []) mergeIndexes(result, evaluatedArrayIndexes(value, child, components, direction, scope, seen));
	for (const variants of [schema.oneOf, schema.anyOf]) {
		for (const child of variants ?? []) if (schemaMatches(value, child, components, direction, scope)) mergeIndexes(result, evaluatedArrayIndexes(value, child, components, direction, scope, seen));
	}
	if (schema.if !== undefined) {
		const child = schemaMatches(value, schema.if, components, direction, scope) ? schema.then : schema.else;
		if (child !== undefined) mergeIndexes(result, evaluatedArrayIndexes(value, child, components, direction, scope, seen));
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
	if (dynamicTarget !== undefined) return evaluatedPropertyNames(value, dynamicTarget, components, direction, scope, seen);
	if (schema.reference !== undefined) {
		const referenced = components[schema.reference];
		if (referenced !== undefined) return evaluatedPropertyNames(value, referenced, components, direction, scope, seen);
	}
	for (const [wireName, definition] of Object.entries(schema.properties ?? {})) {
		const name = direction === "encode" ? definition.property : wireName;
		if (name in value) result.add(name);
	}
	for (const pattern of Object.keys(schema.patternProperties ?? {})) {
		const expression = new RegExp(pattern, "u");
		for (const key of Object.keys(value)) if (expression.test(key)) result.add(key);
	}
	if (schema.additionalProperties !== undefined) for (const key of Object.keys(value)) result.add(key);
	for (const child of schema.allOf ?? []) mergeProperties(result, evaluatedPropertyNames(value, child, components, direction, scope, seen));
	for (const variants of [schema.oneOf, schema.anyOf]) {
		for (const child of variants ?? []) if (schemaMatches(value, child, components, direction, scope)) mergeProperties(result, evaluatedPropertyNames(value, child, components, direction, scope, seen));
	}
	if (schema.if !== undefined) {
		const child = schemaMatches(value, schema.if, components, direction, scope) ? schema.then : schema.else;
		if (child !== undefined) mergeProperties(result, evaluatedPropertyNames(value, child, components, direction, scope, seen));
	}
	return result;
}

function mergeIndexes(target: Set<number>, source: ReadonlySet<number>): void { for (const value of source) target.add(value); }

function mergeProperties(target: Set<string>, source: ReadonlySet<string>): void { for (const value of source) target.add(value); }

function valueMatchesType(value: unknown, type: string): boolean {
  switch (type) {
    case "null": return value === null;
    case "boolean": return typeof value === "boolean";
    case "string": return typeof value === "string";
    case "number": return typeof value === "number" && Number.isFinite(value);
    case "integer": return typeof value === "number" && Number.isInteger(value);
    case "array": return Array.isArray(value);
    case "object": return isRecord(value);
    default: return true;
  }
}

function wireValueEquals(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true;
  if (Array.isArray(left) && Array.isArray(right)) {
    return left.length === right.length && left.every((item, index) => wireValueEquals(item, right[index]));
  }
  if (isRecord(left) && isRecord(right)) {
    const leftKeys = Object.keys(left).sort();
    const rightKeys = Object.keys(right).sort();
    return leftKeys.length === rightKeys.length && leftKeys.every((key, index) => key === rightKeys[index] && wireValueEquals(left[key], right[key]));
  }
  return false;
}

function hasUniqueWireValues(values: readonly unknown[]): boolean {
  return values.every((value, index) => !values.slice(0, index).some((previous) => wireValueEquals(previous, value)));
}

function isMultipleOf(value: number, divisor: number): boolean {
  if (!Number.isFinite(divisor) || divisor <= 0) return false;
  const quotient = value / divisor;
  return Math.abs(quotient - Math.round(quotient)) <= Number.EPSILON * Math.max(1, Math.abs(quotient));
}

function schemaMatches(value: unknown, schema: WireSchema, components: WireSchemas, direction: "encode" | "decode", dynamicScope: DynamicScope = []): boolean {
  try {
    validateWireValue(value, schema, components, direction, dynamicScope);
    return true;
  } catch {
    return false;
  }
}

interface QueryPart {
  readonly name?: string;
  readonly value?: string;
  readonly allowReserved?: boolean;
  readonly raw?: string;
}

async function appendQuery(
  query: Readonly<Record<string, unknown>>,
  operation: OperationDefinition,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<QueryPart[]> {
	const result: QueryPart[] = [];
  for (const [property, value] of Object.entries(query)) {
    if (value === undefined) continue;
    const parameter = findParameterByProperty(operation, "query", property) ?? findParameterByProperty(operation, "querystring", property);
		if (parameter?.location === "querystring") {
			await appendQuerystring(result, value, parameter, operation.inputSchemas ?? {}, codecs);
			continue;
		}
		await appendQueryParameter(
		result,
      parameter?.name ?? property,
      encodeParameterWireValue(operation, parameter, value),
      parameter,
			operation.inputSchemas ?? {}, codecs,
    );
  }
	return result;
}

async function appendQueryParameter(
  query: QueryPart[],
  name: string,
  value: unknown,
  parameter: ParameterDefinition | undefined,
	components: WireSchemas,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<void> {
  if (parameter?.contentType !== undefined) {
		appendQueryValue(query, name, await serializeContentParameter(value, parameter.contentType, parameter.schema, components, codecs), parameter?.allowReserved ?? false);
    return;
  }
  const style = parameter?.style ?? "form";
  const explode = parameter?.explode ?? true;
  if (style === "deepObject" && isRecord(value)) {
    for (const [key, item] of Object.entries(value)) {
			if (item !== undefined) appendQueryValue(query, `${name}[${key}]`, item, parameter?.allowReserved ?? false);
    }
    return;
  }
  if (Array.isArray(value)) {
		if (style === "spaceDelimited") appendQueryValue(query, name, value.map(String).join(" "), parameter?.allowReserved ?? false);
		else if (style === "pipeDelimited") appendQueryValue(query, name, value.map(String).join("|"), parameter?.allowReserved ?? false);
		else if (explode) for (const item of value) appendQueryValue(query, name, item, parameter?.allowReserved ?? false);
		else appendQueryValue(query, name, value.map(String).join(","), parameter?.allowReserved ?? false);
    return;
  }
  if (isRecord(value) && style === "form") {
    const entries = Object.entries(value).filter((entry) => entry[1] !== undefined);
		if (explode) for (const [key, item] of entries) appendQueryValue(query, key, item, parameter?.allowReserved ?? false);
		else appendQueryValue(query, name, entries.flatMap(([key, item]) => [key, String(item)]).join(","), parameter?.allowReserved ?? false);
    return;
  }
	if (isRecord(value) && (style === "spaceDelimited" || style === "pipeDelimited")) {
		const separator = style === "spaceDelimited" ? " " : "|";
		const entries = Object.entries(value).filter((entry) => entry[1] !== undefined);
		const serialized = explode
			? entries.map(([key, item]) => `${key}=${String(item)}`).join(separator)
			: entries.flatMap(([key, item]) => [key, String(item)]).join(separator);
		appendQueryValue(query, name, serialized, parameter?.allowReserved ?? false);
		return;
	}
  appendQueryValue(query, name, value, parameter?.allowReserved ?? false);
}

function appendQuerySync(query: Readonly<Record<string, unknown>>, operation: OperationDefinition): QueryPart[] {
  const result: QueryPart[] = [];
  for (const [property, rawValue] of Object.entries(query)) {
    if (rawValue === undefined) continue;
    const parameter = findParameterByProperty(operation, "query", property) ?? findParameterByProperty(operation, "querystring", property);
    const value = encodeParameterWireValue(operation, parameter, rawValue);
    if (parameter?.location === "querystring") {
      appendQuerystringSync(result, value, parameter, operation.inputSchemas ?? {});
      continue;
    }
    const name = parameter?.name ?? property;
    if (parameter?.contentType !== undefined) { appendQueryValue(result, name, serializeContentParameterSync(value, parameter.contentType, parameter.schema, operation.inputSchemas ?? {}), parameter.allowReserved ?? false); continue; }
    const style = parameter?.style ?? "form";
    const explode = parameter?.explode ?? true;
    if (style === "deepObject" && isRecord(value)) { for (const [key, item] of Object.entries(value)) if (item !== undefined) appendQueryValue(result, `${name}[${key}]`, item, parameter?.allowReserved ?? false); continue; }
    if (Array.isArray(value)) {
      if (style === "spaceDelimited") appendQueryValue(result, name, value.map(String).join(" "), parameter?.allowReserved ?? false);
      else if (style === "pipeDelimited") appendQueryValue(result, name, value.map(String).join("|"), parameter?.allowReserved ?? false);
      else if (explode) for (const item of value) appendQueryValue(result, name, item, parameter?.allowReserved ?? false);
      else appendQueryValue(result, name, value.map(String).join(","), parameter?.allowReserved ?? false);
      continue;
    }
    if (isRecord(value) && style === "form") {
      const entries = Object.entries(value).filter((entry) => entry[1] !== undefined);
      if (explode) for (const [key, item] of entries) appendQueryValue(result, key, item, parameter?.allowReserved ?? false);
      else appendQueryValue(result, name, entries.flatMap(([key, item]) => [key, String(item)]).join(","), parameter?.allowReserved ?? false);
      continue;
    }
    if (isRecord(value) && (style === "spaceDelimited" || style === "pipeDelimited")) {
      const separator = style === "spaceDelimited" ? " " : "|";
      const entries = Object.entries(value).filter((entry) => entry[1] !== undefined);
      appendQueryValue(result, name, explode ? entries.map(([key, item]) => `${key}=${String(item)}`).join(separator) : entries.flatMap(([key, item]) => [key, String(item)]).join(separator), parameter?.allowReserved ?? false);
      continue;
    }
    appendQueryValue(result, name, value, parameter?.allowReserved ?? false);
  }
  return result;
}

function appendQuerystringSync(query: QueryPart[], value: unknown, parameter: ParameterDefinition, components: WireSchemas): void {
  const contentType = parameter.contentType?.toLowerCase();
  if (contentType === "application/x-www-form-urlencoded") {
    if (!isRecord(value)) throw new TypeError("querystring form content must be an object");
    for (const [name, item] of Object.entries(value)) { if (item === undefined) continue; if (Array.isArray(item)) for (const entry of item) query.push({ name, value: String(entry) }); else query.push({ name, value: String(item) }); }
    return;
  }
  if (contentType === "application/json") { query.push({ raw: encodeURIComponent(JSON.stringify(value)) }); return; }
  query.push({ raw: encodeURIComponent(serializeContentParameterSync(value, parameter.contentType ?? "text/plain", parameter.schema, components)) });
}

function appendQueryValue(query: QueryPart[], name: string, value: unknown, allowReserved: boolean): void {
  if (isRecord(value) && typeof value.field === "string" && typeof value.direction === "string") {
		query.push({ name, value: `${value.field}:${value.direction}`, allowReserved });
    return;
  }
  if (typeof value === "object" && value !== null) {
		query.push({ name, value: JSON.stringify(value), allowReserved });
    return;
  }
  query.push({ name, value: String(value), allowReserved });
}

function serializeQuery(query: readonly QueryPart[]): string {
	return query.map((part) => part.raw ?? `${encodeURIComponent(part.name ?? "")}=${part.allowReserved ? encodeReservedQueryValue(part.value ?? "") : encodeURIComponent(part.value ?? "")}`).join("&");
}

async function appendQuerystring(query: QueryPart[], value: unknown, parameter: ParameterDefinition, components: WireSchemas, codecs: ReadonlyMap<string, MediaCodec<unknown>>): Promise<void> {
	const contentType = parameter.contentType?.toLowerCase();
	if (contentType === "application/x-www-form-urlencoded") {
		if (!isRecord(value)) throw new TypeError("querystring form content must be an object");
		for (const [name, item] of Object.entries(value)) {
			if (item === undefined) continue;
			if (Array.isArray(item)) for (const entry of item) query.push({ name, value: String(entry) });
			else query.push({ name, value: String(item) });
		}
		return;
	}
	if (contentType === "application/json") {
		query.push({ raw: encodeURIComponent(JSON.stringify(value)) });
		return;
	}
	query.push({ raw: encodeURIComponent(await serializeContentParameter(value, parameter.contentType ?? "text/plain", parameter.schema, components, codecs)) });
}

function encodeReservedQueryValue(value: string): string {
	return encodeURIComponent(value)
		.replace(/%25([0-9a-f]{2})/gi, "%$1")
		.replace(/%3A|%2F|%3F|%40|%21|%24|%27|%28|%29|%2A|%2C|%3B|%3D/gi, (encoded) => decodeURIComponent(encoded));
}

function findParameter(
  operation: OperationDefinition,
  location: ParameterDefinition["location"],
  name: string,
): ParameterDefinition | undefined {
  return operation.parameters?.find(
    (parameter) => parameter.location === location && parameter.name === name,
  );
}

function findParameterByProperty(
  operation: OperationDefinition,
  location: ParameterDefinition["location"],
  property: string,
): ParameterDefinition | undefined {
  return operation.parameters?.find(
    (parameter) => parameter.location === location && parameter.property === property,
  );
}

async function serializePathParameter(
  parameter: ParameterDefinition | undefined,
  name: string,
  value: unknown,
	components: WireSchemas,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<string> {
  if (parameter?.contentType !== undefined) {
		return encodeURIComponent(await serializeContentParameter(value, parameter.contentType, parameter.schema, components, codecs));
  }
  const style = parameter?.style ?? "simple";
  const explode = parameter?.explode ?? false;
  const encoded = serializePathValue(value, explode, style === "label" ? "." : ",");
  if (style === "label") return `.${encoded}`;
  if (style !== "matrix") return encoded;
  if (Array.isArray(value) && explode) {
    return value
      .map((item) => `;${encodeURIComponent(name)}=${encodeURIComponent(String(item))}`)
      .join("");
  }
  if (isRecord(value) && explode) {
    return Object.entries(value)
      .map(([key, item]) => `;${encodeURIComponent(key)}=${encodeURIComponent(String(item))}`)
      .join("");
  }
  return `;${encodeURIComponent(name)}=${encoded}`;
}

function serializePathParameterSync(parameter: ParameterDefinition | undefined, name: string, value: unknown, components: WireSchemas): string {
  if (parameter?.contentType !== undefined) return encodeURIComponent(serializeContentParameterSync(value, parameter.contentType, parameter.schema, components));
  const style = parameter?.style ?? "simple";
  const explode = parameter?.explode ?? false;
  const encoded = serializePathValue(value, explode, style === "label" ? "." : ",");
  if (style === "label") return `.${encoded}`;
  if (style !== "matrix") return encoded;
  if (Array.isArray(value) && explode) return value.map((item) => `;${encodeURIComponent(name)}=${encodeURIComponent(String(item))}`).join("");
  if (isRecord(value) && explode) return Object.entries(value).map(([key, item]) => `;${encodeURIComponent(key)}=${encodeURIComponent(String(item))}`).join("");
  return `;${encodeURIComponent(name)}=${encoded}`;
}

function serializePathValue(value: unknown, explode: boolean, arraySeparator: string): string {
  if (Array.isArray(value))
    return value
      .map((item) => encodeURIComponent(String(item)))
      .join(explode ? arraySeparator : ",");
  if (isRecord(value)) {
    return Object.entries(value)
      .flatMap(([key, item]) =>
        explode
          ? `${encodeURIComponent(key)}=${encodeURIComponent(String(item))}`
          : [encodeURIComponent(key), encodeURIComponent(String(item))],
      )
      .join(explode ? arraySeparator : ",");
  }
  return encodeURIComponent(String(value));
}

function serializeSimpleValue(value: unknown, explode: boolean): string {
  if (Array.isArray(value)) return value.map(String).join(",");
  if (isRecord(value)) {
    return Object.entries(value)
      .flatMap(([key, item]) => (explode ? `${key}=${String(item)}` : [key, String(item)]))
      .join(",");
  }
  return String(value);
}

async function serializeContentParameter(value: unknown, contentType: string, schema: WireSchema | undefined = undefined, components: WireSchemas = {}, codecs: ReadonlyMap<string, MediaCodec<unknown>> = new Map()): Promise<string> {
	if (isJSONMediaType(contentType)) return JSON.stringify(value);
	if (isXMLMediaType(contentType)) return encodeXML(value, schema ?? {}, components);
	if (contentType.toLowerCase() === "application/x-www-form-urlencoded") {
		if (!isRecord(value)) return String(value);
		const form = new URLSearchParams();
		for (const [name, item] of Object.entries(value)) {
			if (item === undefined) continue;
			if (Array.isArray(item)) for (const entry of item) form.append(name, String(entry));
			else form.append(name, String(item));
		}
		return form.toString();
	}
	const codec = codecs.get(normalizeMediaType(contentType));
	if (codec?.encodeParameter === undefined) throw new TypeError(`missing parameter encode codec for ${contentType}`);
	return await codec.encodeParameter(value, { contentType });
}

function serializeContentParameterSync(value: unknown, contentType: string, schema: WireSchema | undefined = undefined, components: WireSchemas = {}): string {
  if (isJSONMediaType(contentType)) return JSON.stringify(value);
  if (isXMLMediaType(contentType)) return encodeXML(value, schema ?? {}, components);
  if (contentType.toLowerCase() === "application/x-www-form-urlencoded") {
    if (!isRecord(value)) return String(value);
    const form = new URLSearchParams();
    for (const [name, item] of Object.entries(value)) { if (item === undefined) continue; if (Array.isArray(item)) for (const entry of item) form.append(name, String(entry)); else form.append(name, String(item)); }
    return form.toString();
  }
  if (contentType.toLowerCase().startsWith("text/")) return String(value);
  throw new TypeError(`missing parameter encode codec for ${contentType}`);
}

async function serializeCookie(
  operation: OperationDefinition,
  property: string,
	value: unknown,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<string[]> {
  const parameter = findParameterByProperty(operation, "cookie", property);
  const name = parameter?.name ?? property;
	const preserve = parameter?.style === "cookie";
  const pair = (key: string, item: unknown): string => `${preserve ? key : encodeURIComponent(key)}=${preserve ? String(item ?? "") : encodeURIComponent(String(item ?? ""))}`;
  value = encodeParameterWireValue(operation, parameter, value);
  if (parameter?.contentType !== undefined) {
    return [pair(name, await serializeContentParameter(value, parameter.contentType, parameter.schema, operation.inputSchemas ?? {}, codecs))];
  }
  if (Array.isArray(value)) {
    if (parameter?.explode ?? true) {
      return value.map((item) => pair(name, item));
    }
    return [pair(name, value.map(String).join(","))];
  }
  if (isRecord(value) && (parameter?.explode ?? true)) {
    return Object.entries(value).map(([key, item]) => pair(key, item));
  }
  return [pair(name, serializeSimpleValue(value, false))];
}

function serializeCookieSync(operation: OperationDefinition, property: string, value: unknown): string[] {
  const parameter = findParameterByProperty(operation, "cookie", property);
  const name = parameter?.name ?? property;
	const preserve = parameter?.style === "cookie";
  const pair = (key: string, item: unknown): string => `${preserve ? key : encodeURIComponent(key)}=${preserve ? String(item ?? "") : encodeURIComponent(String(item ?? ""))}`;
  value = encodeParameterWireValue(operation, parameter, value);
  if (parameter?.contentType !== undefined) return [pair(name, serializeContentParameterSync(value, parameter.contentType, parameter.schema, operation.inputSchemas ?? {}))];
  if (Array.isArray(value)) return parameter?.explode ?? true ? value.map((item) => pair(name, item)) : [pair(name, value.map(String).join(","))];
  if (isRecord(value) && (parameter?.explode ?? true)) return Object.entries(value).map(([key, item]) => pair(key, item));
  return [pair(name, serializeSimpleValue(value, false))];
}

function resolveOperationBaseURL(
  baseURL: string | undefined,
  origin: string | undefined,
  selection: ServerSelection | undefined,
  operation: OperationDefinition,
): string {
  if (baseURL !== undefined) return baseURL;
  const servers = operation.servers ?? [{ id: "#", url: "/" }];
  const server = selection?.id === undefined ? servers[0] : servers.find((item) => item.id === selection.id);
  if (server === undefined) throw new TypeError(`Unknown server ${selection?.id} for operation ${operation.operationID}`);
  const variables = selection?.variables ?? {};
  const expanded = server.url.replace(/\{([^}]+)\}/g, (_, name: string) => {
    const definition = server.variables?.find((item) => item.name === name);
    if (definition === undefined) throw new TypeError(`Server ${server.id} has no variable ${name}`);
    const value = variables[name] ?? definition.defaultValue;
    if (definition.enumValues !== undefined && !definition.enumValues.includes(value)) {
      throw new TypeError(`Server variable ${name} must be one of ${definition.enumValues.join(", ")}`);
    }
    return encodeURIComponent(value);
  });
  try {
    return normalizeBaseURL(expanded);
  } catch {
    if (origin === undefined) throw new TypeError(`Server ${server.id} is relative; pass ClientOptions.origin or baseURL`);
    const absoluteOrigin = normalizeOrigin(origin);
    return normalizeBaseURL(new URL(expanded, absoluteOrigin).href);
  }
}

function normalizeOrigin(value: string): string {
  const url = new URL(value);
  if ((url.protocol !== "http:" && url.protocol !== "https:") || url.pathname !== "/" || url.search || url.hash) {
    throw new TypeError("origin must be an absolute http(s) origin without path, query, or fragment");
  }
  return url.origin;
}

function appendRawHeaders(
  target: Headers,
  source: HeadersInit | undefined,
  contractNames: ReadonlySet<string>,
): void {
  if (source === undefined) return;
  const incoming = new Headers(source);
  incoming.forEach((value, name) => {
    const lower = name.toLowerCase();
    if (reservedHeaders.has(lower) || contractNames.has(lower)) {
      throw new TypeError(`Raw header ${name} must use its typed option`);
    }
    target.set(name, value);
  });
}

function setHeader(headers: Headers, name: string, value: string | undefined): void {
  if (value !== undefined) headers.set(name, value);
}

function rejectUndefinedArrayValues(value: unknown): void {
  if (Array.isArray(value)) {
    for (const item of value) {
      if (item === undefined) {
        throw new TypeError("Request arrays cannot contain undefined");
      }
      rejectUndefinedArrayValues(item);
    }
    return;
  }
  if (isRecord(value)) {
    for (const item of Object.values(value)) rejectUndefinedArrayValues(item);
  }
}

function normalizeBaseURL(value: string): string {
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new TypeError("baseURL must be an absolute URL");
  }
  if ((url.protocol !== "http:" && url.protocol !== "https:") || url.search || url.hash) {
    throw new TypeError("baseURL must be an absolute http(s) URL without query or fragment");
  }
  url.pathname = url.pathname.replace(/\/+$/, "");
  return url.href.replace(/\/$/, "");
}

interface AbortContext {
  readonly signal: AbortSignal | undefined;
  readonly timedOut: () => boolean;
  readonly aborted: () => boolean;
  readonly cleanup: () => void;
}

function createAbortContext(
  signal: AbortSignal | undefined,
  timeoutMS: number | undefined,
): AbortContext {
  if (timeoutMS !== undefined && (!Number.isFinite(timeoutMS) || timeoutMS <= 0)) {
    throw new TypeError("timeoutMS must be a positive finite number");
  }
  if (signal === undefined && timeoutMS === undefined) {
    return {
      signal: undefined,
      timedOut: () => false,
      aborted: () => false,
      cleanup: () => undefined,
    };
  }
  const controller = new AbortController();
  let timeoutReached = false;
  const forwardAbort = (): void => controller.abort(signal?.reason);
  if (signal?.aborted) forwardAbort();
  else signal?.addEventListener("abort", forwardAbort, { once: true });
  const timer =
    timeoutMS === undefined
      ? undefined
      : setTimeout(() => {
          timeoutReached = true;
          controller.abort();
        }, timeoutMS);
  return {
    signal: controller.signal,
    timedOut: () => timeoutReached,
    aborted: () => signal?.aborted === true,
    cleanup: () => {
      if (timer !== undefined) clearTimeout(timer);
      signal?.removeEventListener("abort", forwardAbort);
    },
  };
}

async function decodeMultipartResponse(
  body: ReadableStream<Uint8Array>,
  contentType: string,
  definition: WireBodyDefinition,
  schemas: WireSchemas,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<unknown[]> {
  const result: unknown[] = [];
  for await (const part of decodeMultipartStreamParts(body, contentType)) {
    const index = result.length;
    const schema = definition.schema.prefixItems?.[index] ?? definition.schema.items ?? {};
    const encoding = definition.prefixEncoding?.[index] ?? definition.itemEncoding;
    result.push(await decodeMultipartStreamPart(part, schema, schemas, codecs, encoding));
  }
  return result;
}

async function decodeResponse(operation: OperationDefinition, response: Response, request: RequestMetadata, codecs: ReadonlyMap<string, MediaCodec<unknown>>): Promise<unknown> {
  if (response.status === 204 || response.status === 205) return undefined;
  const contentType = responseContentType(response);
  if (contentType === undefined || response.body === null) return undefined;
  try {
		const definition = selectResponseDefinition(operation, response, true);
		if (normalizeMediaType(contentType).startsWith("multipart/") && definition !== undefined) {
			return decodeMultipartResponse(response.body, response.headers.get("content-type") ?? contentType, definition, operation.outputSchemas ?? {}, codecs);
		}
	if (isJSONMediaType(contentType)) {
      return await response.json();
    }
    if (contentType.startsWith("text/") || contentType.includes("xml")) {
      return await response.text();
    }
		if (isBinaryMediaType(contentType)) return response.body;
		const codec = codecs.get(contentType);
		if (codec?.decode === undefined) throw new TypeError(`missing decode codec for ${contentType}`);
		return await codec.decode(response, { contentType });
  } catch (cause) {
    throw new APIError({
      code: TransportErrorCode.RESPONSE_DECODE_FAILED,
      message: "Failed to decode response body",
      request,
      status: response.status,
      response,
      cause,
    });
  }
}

function isJSONMediaType(contentType: string): boolean {
	const mediaType = contentType.split(";", 1)[0]?.trim().toLowerCase() ?? "";
	return mediaType === "application/json" || mediaType.endsWith("+json");
}

function isXMLMediaType(contentType: string): boolean {
  const mediaType = contentType.split(";", 1)[0]?.trim().toLowerCase() ?? "";
  return mediaType === "application/xml" || mediaType === "text/xml" || mediaType.endsWith("+xml");
}

function responseContentType(response: Response): string | undefined {
  return response.headers.get("content-type")?.split(";", 1)[0]?.trim().toLowerCase();
}

function normalizeCodecs(codecs: Readonly<Record<string, MediaCodec<unknown>>> | undefined): ReadonlyMap<string, MediaCodec<unknown>> {
	const result = new Map<string, MediaCodec<unknown>>();
	for (const [contentType, codec] of Object.entries(codecs ?? {})) {
		const normalized = normalizeMediaType(contentType);
		if (normalized === "" || result.has(normalized)) throw new TypeError(`duplicate or invalid media codec ${contentType}`);
		result.set(normalized, codec);
	}
	return result;
}

function normalizeMediaType(contentType: string): string {
	return contentType.split(";", 1)[0]?.trim().toLowerCase() ?? "";
}

function isBinaryMediaType(contentType: string): boolean {
	return contentType === "application/octet-stream" || contentType.startsWith("image/") || contentType.startsWith("audio/") || contentType.startsWith("video/");
}

function isSequentialStreamMediaType(contentType: string): boolean {
	return contentType.includes("event-stream") || contentType.includes("json-seq") || contentType.includes("ndjson") || contentType.includes("jsonl");
}

function isPromise<Value>(value: Value | Promise<Value>): value is Promise<Value> {
	return typeof (value as Promise<Value>)?.then === "function";
}

function isAsyncIterable(value: unknown): value is AsyncIterable<unknown> {
  return value !== null && typeof value === "object" && typeof (value as AsyncIterable<unknown>)[Symbol.asyncIterator] === "function";
}

function isReadableStream(value: unknown): value is ReadableStream<Uint8Array> {
  return value !== null && typeof value === "object" && typeof (value as ReadableStream<Uint8Array>).getReader === "function";
}

function serverError(response: Response, request: RequestMetadata, body: unknown): APIError {
  const envelope = isRecord(body) && isRecord(body.error) ? body.error : body;
  const error = isRecord(envelope) ? envelope : {};
  const code = typeof error.code === "string" ? error.code : `HTTP_${response.status}`;
  const message =
    typeof error.message === "string"
      ? error.message
      : typeof body === "string" && body.trim() !== ""
        ? body
      : `HTTP request failed with status ${response.status}`;
  return new APIError({
    code,
    message,
    request,
    status: response.status,
    details: error.details ?? error.fields,
    fields: error.fields,
    data: body,
    response,
  });
}

function requestMetadata(response: Response): RequestMetadata {
  const id = response.headers.get("x-request-id");
  return id === null ? {} : { id };
}

function resolvePaginationMode(
  profile: PaginationProfile,
  requested: unknown,
): "cursor" | "offset" {
  if (profile === "both") {
    if (requested !== "cursor" && requested !== "offset") {
      throw new TypeError('Pagination profile "both" requires mode "cursor" or "offset"');
    }
    return requested;
  }
  if (requested !== undefined && requested !== profile) {
    throw new TypeError(`Pagination mode ${String(requested)} does not match ${profile}`);
  }
  return profile;
}

function pageItems(page: unknown): readonly unknown[] {
  if (!isRecord(page)) return [];
  if (Array.isArray(page.items)) return page.items;
  if (isRecord(page.data) && Array.isArray(page.data.items)) return page.data.items;
  return [];
}

function pagePagination(page: unknown): Record<string, unknown> {
  if (!isRecord(page)) return {};
  if (isRecord(page.pagination)) return page.pagination;
  if (isRecord(page.meta) && isRecord(page.meta.pagination)) return page.meta.pagination;
  if (isRecord(page.data)) {
    if (isRecord(page.data.pagination)) return page.data.pagination;
    if (isRecord(page.data.meta) && isRecord(page.data.meta.pagination)) {
      return page.data.meta.pagination;
    }
  }
  return {};
}

function numberValue(primary: unknown, secondary: unknown, fallback: number): number {
  if (typeof primary === "number" && Number.isFinite(primary)) return primary;
  if (typeof secondary === "number" && Number.isFinite(secondary)) return secondary;
  return fallback;
}

function transportError(code: TransportErrorCode, message: string, cause: unknown): TransportError {
  return new APIError({ code, message, cause });
}

function transportErrorFromCause(
  code: TransportErrorCode,
  message: string,
  cause: unknown,
  responseMetadata?: { request: RequestMetadata; status: number; response: Response },
): TransportError {
  if (isAPIError(cause)) {
    return new APIError({
      code,
      message,
      cause,
      request: cause.request,
      status: cause.status,
      response: cause.response,
    });
  }
  if (responseMetadata !== undefined) {
    return new APIError({ code, message, cause, ...responseMetadata });
  }
  return transportError(code, message, cause);
}

function awaitAbortable<Value>(value: Promise<Value>, signal: AbortSignal | undefined): Promise<Value> {
  if (signal === undefined) return value;
  if (signal.aborted) {
    void value.catch(() => undefined);
    return Promise.reject(signal.reason);
  }
  return new Promise((resolve, reject) => {
    const onAbort = (): void => reject(signal.reason);
    signal.addEventListener("abort", onAbort, { once: true });
    value.then(
      (result) => {
        signal.removeEventListener("abort", onAbort);
        resolve(result);
      },
      (cause) => {
        signal.removeEventListener("abort", onAbort);
        reject(cause);
      },
    );
  });
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
