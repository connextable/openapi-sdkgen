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
  readonly baseURL: string;
  /** Fetch implementation or wrapper. Defaults to `globalThis.fetch`; the SDK adds no retries. */
  readonly fetch?: typeof globalThis.fetch;
  /** Default headers added to every request. Use dedicated options for SDK-managed headers. */
  readonly headers?: HeadersInit;
  /** Complete default `Authorization` header value, including its authentication scheme. */
  readonly authorization?: string;
  /** Default Fetch API credentials mode. */
  readonly credentials?: RequestCredentials;
  /** Default positive timeout in milliseconds. Individual requests may override it. */
  readonly timeoutMS?: number;
}

/** Options applied to one generated operation call. */
export interface RequestOptions {
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
  /** Operation-specific server URL when it differs from the client base URL. */
  readonly serverURL?: string;
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
  /** Successful response representations keyed by status and media type. */
  readonly responses?: readonly WireResponseDefinition[];
}

/** Minimal recursive schema used for runtime wire-name transformation. */
export interface WireSchema {
  /** Referenced component name. */
  readonly reference?: string;
  /** Object properties keyed by their JSON wire names. */
  readonly properties?: Readonly<Record<string, WireProperty>>;
  /** Homogeneous array item schema. */
  readonly items?: WireSchema;
  /** Tuple item schemas in positional order. */
  readonly prefixItems?: readonly WireSchema[];
  /** Schema for additional object properties. */
  readonly additionalProperties?: WireSchema;
  /** Schemas whose transformations are applied cumulatively. */
  readonly allOf?: readonly WireSchema[];
  /** Alternative schemas considered when transforming a value. */
  readonly oneOf?: readonly WireSchema[];
  /** Alternative schemas considered when transforming a value. */
  readonly anyOf?: readonly WireSchema[];
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
}

/** Successful response representation understood by the runtime. */
export interface WireResponseDefinition extends WireBodyDefinition {
  /** Exact status code, `default`, or wildcard status such as `2XX`. */
  readonly status: string;
}

/** OpenAPI parameter serialization metadata emitted by `sdkgen`. */
export interface ParameterDefinition {
  /** HTTP request location of the parameter. */
  readonly location: "path" | "query" | "header" | "cookie";
  /** Exact OpenAPI and HTTP wire name. */
  readonly name: string;
  /** Generated TypeScript property name. */
  readonly property: string;
  /** OpenAPI serialization style. */
  readonly style: string;
  /** Whether objects and arrays are exploded into separate values. */
  readonly explode: boolean;
  /** Media type for a content-based parameter. */
  readonly contentType?: string;
  /** Schema used for wire-name transformation before serialization. */
  readonly schema?: WireSchema;
}

/** Successful response including decoded data and the underlying Fetch API response. */
export interface RawResponse<Output> {
  /** HTTP status code. */
  readonly status: number;
  /** Normalized response media type without parameters. */
  readonly contentType?: string;
  /** Decoded, typed response body. */
  readonly data: Output;
  /** Response headers. */
  readonly headers: Headers;
  /** Request metadata extracted from the response. */
  readonly request: RequestMetadata;
  /** Original Fetch API response. Its body has already been consumed unless streamed. */
  readonly response: Response;
}

/**
 * Raw response narrowed to an operation's exact status, media type, and output type.
 *
 * @template Status HTTP status literal.
 * @template ContentType Response media-type literal.
 * @template Output Decoded response body type.
 */
export type RawResponseFor<Status extends number, ContentType, Output> = Omit<
  RawResponse<Output>,
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
      for (;;) {
        const page = await requestPage({ ...root, query: { ...query } } as Input, options);
        const items = pageItems(page);
        for (const item of items) yield item as Item;
        if (mode === "cursor") {
          const nextCursor = pagePagination(page).nextCursor;
          if (typeof nextCursor !== "string" || nextCursor === "" || nextCursor === query.cursor) {
            return;
          }
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
  const baseURL = normalizeBaseURL(options.baseURL);
  const fetchImplementation = options.fetch ?? globalThis.fetch;
  if (typeof fetchImplementation !== "function") {
    throw new TypeError("fetch is unavailable; pass ClientOptions.fetch");
  }

  const execute = async <Output>(
    operation: OperationDefinition,
    input?: unknown,
    requestOptions: RequestOptions = {},
    raw = false,
  ): Promise<Output | RawResponse<Output>> => {
    let encoded: EncodedRequest;
    try {
      encoded = encodeRequest(baseURL, options, operation, input, requestOptions);
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
    try {
      const init: RequestInit = {
        method: operation.method,
        headers: encoded.headers,
      };
      if (encoded.body !== undefined) init.body = encoded.body;
      if (abort.signal !== undefined) init.signal = abort.signal;
      const credentials = requestOptions.credentials ?? options.credentials;
      if (credentials !== undefined) init.credentials = credentials;
      const response = await fetchImplementation(encoded.url, init);
      const request = requestMetadata(response);
      const decodedBody = await decodeResponse(response, request);
      const body = decodeResponseWireValue(operation, response, decodedBody);
      if (!response.ok) {
        throw serverError(response, request, body);
      }
      const data =
        operation.envelope === "data" && isRecord(body) && "data" in body
          ? (body.data as Output)
          : (body as Output);
      if (!raw) return data;
      const contentType = responseContentType(response);
      return {
        status: response.status,
        ...(contentType === undefined ? {} : { contentType }),
        data,
        headers: response.headers,
        request,
        response,
      };
    } catch (cause) {
      if (abort.timedOut()) {
        throw transportError(
          TransportErrorCode.REQUEST_TIMEOUT,
          `Request timed out after ${timeoutMS}ms`,
          cause,
        );
      }
      if (abort.aborted()) {
        throw transportError(TransportErrorCode.REQUEST_ABORTED, "Request was aborted", cause);
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
  return request;
}

interface EncodedRequest {
  readonly url: string;
  readonly headers: Headers;
  readonly body?: BodyInit;
}

function encodeRequest(
  baseURL: string,
  client: ClientOptions,
  operation: OperationDefinition,
  input: unknown,
  options: RequestOptions,
): EncodedRequest {
  const values = isRecord(input) ? input : {};
  const pathValues = isRecord(values.path) ? values.path : {};
  const path = operation.path.replaceAll(/\{([^}]+)\}/g, (_, name: string) => {
    const parameter = findParameter(operation, "path", name);
    const property = parameter?.property ?? name;
    const rawValue = pathValues[property];
    if (rawValue === undefined || rawValue === null) {
      throw new TypeError(`Missing path parameter ${name}`);
    }
    const value = encodeParameterWireValue(operation, parameter, rawValue);
    return serializePathParameter(parameter, name, value);
  });
  const operationBaseURL = resolveOperationBaseURL(baseURL, operation.serverURL);
  const url = new URL(operationBaseURL + (path.startsWith("/") ? path : `/${path}`));
  appendQuery(url.searchParams, isRecord(values.query) ? values.query : {}, operation);

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
  for (const [property, value] of Object.entries(headerParams)) {
    if (value === undefined) continue;
    const parameter = findParameterByProperty(operation, "header", property);
    const name = parameter?.name ?? property;
    const encodedValue = encodeParameterWireValue(operation, parameter, value);
    headers.set(
      name,
      parameter?.contentType === undefined
        ? serializeSimpleValue(encodedValue, parameter?.explode ?? false)
        : serializeContentParameter(encodedValue, parameter.contentType),
    );
  }
  setHeader(headers, "Authorization", options.authorization ?? client.authorization);
  setHeader(headers, "Accept", options.accept);
  setHeader(headers, "X-CSRF-Token", options.csrfToken);
  setHeader(headers, "X-Request-Id", options.requestID);
  setHeader(headers, "Idempotency-Key", options.idempotencyKey);
  setHeader(headers, "If-Match", options.ifMatch);

  const cookieValues = isRecord(values.cookieParams) ? values.cookieParams : {};
  const cookies = Object.entries(cookieValues)
    .filter((entry): entry is [string, unknown] => entry[1] !== undefined)
    .flatMap(([property, value]) => serializeCookie(operation, property, value));
  if (cookies.length > 0) headers.set("Cookie", cookies.join("; "));

  if (!("body" in values) || values.body === undefined) {
    return { url: url.href, headers };
  }
  rejectUndefinedArrayValues(values.body);
  let contentType = operation.contentType ?? "application/json";
  let bodyValue: unknown = values.body;
  if (
    isRecord(values.body) &&
    typeof values.body.contentType === "string" &&
    "value" in values.body
  ) {
    contentType = values.body.contentType;
    bodyValue = values.body.value;
  }
  const encodedBody = encodeRequestWireValue(operation, contentType, bodyValue);
  const body = encodeRequestBody(contentType, encodedBody);
  if (!(body instanceof FormData)) headers.set("Content-Type", contentType);
  return { url: url.href, headers, body };
}

function encodeRequestBody(contentType: string, value: unknown): BodyInit {
  if (contentType.includes("json")) return JSON.stringify(value);
  if (contentType === "application/x-www-form-urlencoded") {
    if (!isRecord(value)) throw new TypeError("form body must be an object");
    const form = new URLSearchParams();
    for (const [name, item] of Object.entries(value)) {
      if (item === undefined) continue;
      if (Array.isArray(item)) for (const entry of item) form.append(name, String(entry));
      else form.append(name, String(item));
    }
    return form;
  }
  if (contentType === "multipart/form-data") {
    if (!isRecord(value)) throw new TypeError("multipart body must be an object");
    const form = new FormData();
    const append = (name: string, item: unknown): void => {
      if (item instanceof Blob) form.append(name, item);
      else form.append(name, String(item));
    };
    for (const [name, item] of Object.entries(value)) {
      if (item === undefined) continue;
      if (Array.isArray(item)) for (const entry of item) append(name, entry);
      else append(name, item);
    }
    return form;
  }
  if (contentType.startsWith("text/") || contentType.includes("xml")) return String(value);
  if (value instanceof Blob || value instanceof ArrayBuffer || ArrayBuffer.isView(value)) {
    return value as BodyInit;
  }
  throw new TypeError(`unsupported body value for ${contentType}`);
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
  const contentType = responseContentType(response);
  const definition = operation.responses?.find(
    (item) =>
      statusMatches(item.status, response.status) &&
      (contentType === undefined || item.contentType.toLowerCase() === contentType),
  );
  return definition === undefined
    ? value
    : transformWireValue(value, definition.schema, operation.outputSchemas ?? {}, "decode");
}

function statusMatches(pattern: string, status: number): boolean {
  if (pattern === String(status) || pattern === "default") return true;
  return /^\dXX$/i.test(pattern) && Number(pattern[0]) === Math.floor(status / 100);
}

function transformWireValue(
  value: unknown,
  schema: WireSchema,
  components: WireSchemas,
  direction: "encode" | "decode",
): unknown {
  if (value === null || value === undefined) return value;
  if (schema.reference !== undefined) {
    const referenced = components[schema.reference];
    return referenced === undefined
      ? value
      : transformWireValue(value, referenced, components, direction);
  }
  if (Array.isArray(value)) {
    return value.map((item, index) => {
      const itemSchema = schema.prefixItems?.[index] ?? schema.items;
      return itemSchema === undefined
        ? item
        : transformWireValue(item, itemSchema, components, direction);
    });
  }
  let transformed: unknown = value;
  if (isRecord(transformed) && schema.properties !== undefined) {
    const source = transformed;
    const result: Record<string, unknown> = { ...source };
    const known = new Set<string>();
    for (const [wireName, propertyDefinition] of Object.entries(schema.properties)) {
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
      );
    }
    if (schema.additionalProperties !== undefined) {
      for (const [key, item] of Object.entries(result)) {
        if (!known.has(key)) {
          result[key] = transformWireValue(
            item,
            schema.additionalProperties,
            components,
            direction,
          );
        }
      }
    }
    transformed = result;
  }
  for (const variants of [schema.allOf, schema.oneOf, schema.anyOf]) {
    if (variants === undefined) continue;
    for (const variant of variants) {
      transformed = transformWireValue(transformed, variant, components, direction);
    }
  }
  return transformed;
}

function appendQuery(
  search: URLSearchParams,
  query: Readonly<Record<string, unknown>>,
  operation: OperationDefinition,
): void {
  for (const [property, value] of Object.entries(query)) {
    if (value === undefined) continue;
    const parameter = findParameterByProperty(operation, "query", property);
    appendQueryParameter(
      search,
      parameter?.name ?? property,
      encodeParameterWireValue(operation, parameter, value),
      parameter,
    );
  }
}

function appendQueryParameter(
  search: URLSearchParams,
  name: string,
  value: unknown,
  parameter: ParameterDefinition | undefined,
): void {
  if (parameter?.contentType !== undefined) {
    appendQueryValue(search, name, serializeContentParameter(value, parameter.contentType));
    return;
  }
  const style = parameter?.style ?? "form";
  const explode = parameter?.explode ?? true;
  if (style === "deepObject" && isRecord(value)) {
    for (const [key, item] of Object.entries(value)) {
      if (item !== undefined) appendQueryValue(search, `${name}[${key}]`, item);
    }
    return;
  }
  if (Array.isArray(value)) {
    if (style === "spaceDelimited") search.append(name, value.map(String).join(" "));
    else if (style === "pipeDelimited") search.append(name, value.map(String).join("|"));
    else if (explode) for (const item of value) appendQueryValue(search, name, item);
    else search.append(name, value.map(String).join(","));
    return;
  }
  if (isRecord(value) && style === "form") {
    const entries = Object.entries(value).filter((entry) => entry[1] !== undefined);
    if (explode) for (const [key, item] of entries) appendQueryValue(search, key, item);
    else search.append(name, entries.flatMap(([key, item]) => [key, String(item)]).join(","));
    return;
  }
  appendQueryValue(search, name, value);
}

function appendQueryValue(search: URLSearchParams, name: string, value: unknown): void {
  if (isRecord(value) && typeof value.field === "string" && typeof value.direction === "string") {
    search.append(name, `${value.field}:${value.direction}`);
    return;
  }
  if (typeof value === "object" && value !== null) {
    search.append(name, JSON.stringify(value));
    return;
  }
  search.append(name, String(value));
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

function serializePathParameter(
  parameter: ParameterDefinition | undefined,
  name: string,
  value: unknown,
): string {
  if (parameter?.contentType !== undefined) {
    return encodeURIComponent(serializeContentParameter(value, parameter.contentType));
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

function serializeContentParameter(value: unknown, contentType: string): string {
  if (contentType.includes("json")) return JSON.stringify(value);
  return String(value);
}

function serializeCookie(
  operation: OperationDefinition,
  property: string,
  value: unknown,
): string[] {
  const parameter = findParameterByProperty(operation, "cookie", property);
  const name = parameter?.name ?? property;
  value = encodeParameterWireValue(operation, parameter, value);
  if (parameter?.contentType !== undefined) {
    return [
      `${encodeURIComponent(name)}=${encodeURIComponent(serializeContentParameter(value, parameter.contentType))}`,
    ];
  }
  if (Array.isArray(value)) {
    if (parameter?.explode ?? true) {
      return value.map((item) => `${encodeURIComponent(name)}=${encodeURIComponent(String(item))}`);
    }
    return [`${encodeURIComponent(name)}=${encodeURIComponent(value.map(String).join(","))}`];
  }
  if (isRecord(value) && (parameter?.explode ?? true)) {
    return Object.entries(value).map(
      ([key, item]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(item))}`,
    );
  }
  return [`${encodeURIComponent(name)}=${encodeURIComponent(serializeSimpleValue(value, false))}`];
}

function resolveOperationBaseURL(baseURL: string, serverURL: string | undefined): string {
  if (serverURL === undefined) return baseURL;
  const base = new URL(baseURL);
  const server = new URL(serverURL, base.origin);
  return server.href.replace(/\/$/, "");
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

async function decodeResponse(response: Response, request: RequestMetadata): Promise<unknown> {
  if (response.status === 204 || response.status === 205) return undefined;
  const contentType = responseContentType(response);
  if (contentType === undefined || response.body === null) return undefined;
  try {
    if (contentType === "application/json" || contentType.endsWith("+json")) {
      return await response.json();
    }
    if (contentType.startsWith("text/") || contentType.includes("xml")) {
      return await response.text();
    }
    return response.body;
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

function responseContentType(response: Response): string | undefined {
  return response.headers.get("content-type")?.split(";", 1)[0]?.trim().toLowerCase();
}

function serverError(response: Response, request: RequestMetadata, body: unknown): APIError {
  const envelope = isRecord(body) && isRecord(body.error) ? body.error : body;
  const error = isRecord(envelope) ? envelope : {};
  const code = typeof error.code === "string" ? error.code : `HTTP_${response.status}`;
  const message =
    typeof error.message === "string"
      ? error.message
      : `HTTP request failed with status ${response.status}`;
  return new APIError({
    code,
    message,
    request,
    status: response.status,
    details: error.details ?? error.fields,
    fields: error.fields,
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

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
