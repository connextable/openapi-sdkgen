import type { RequestFunction } from "./callables.js";
import {
  decodeWireValue,
  decodeXML,
  encodeXML,
  isJSONMediaType,
  isXMLMediaType,
  transformWireValue,
  validateWireValue,
  type MediaCodec,
  type MediaStreamReader,
  type WireBodyDefinition,
  type WireEncodingDefinition,
  type WireMultipartHeaderDefinition,
  type WireResponseDefinition,
  type WireSchema,
  type WireSchemas,
} from "./codecs.js";
import type { ClientOptions, SecurityCredentialContext } from "./configuration.js";
import { APIError, TransportErrorCode, isAPIError, type TransportError } from "./errors.js";
import { defineOwnDataProperty, isRecord } from "./objects.js";
import {
  operationDiagnosticName,
  type OperationDefinition,
  type ParameterDefinition,
  type ServerSelection,
} from "./operation.js";
import type { RawResponse, RequestMetadata, RequestOptions } from "./request.js";
import type {
  APIKeyCredential,
  HTTPBasicCredential,
  HTTPBearerCredential,
  HTTPCredential,
  OAuthCredential,
  SecurityCredential,
  SecurityCredentials,
  SecurityRequirementDefinition,
  SecuritySchemeDefinition,
} from "./security.js";
import type { Transport } from "./transport.js";

/** Internal operation options after generated security selection is attached. */
interface OperationRequestOptions extends RequestOptions {
  readonly securityRequirement?: string;
}

const reservedHeaders = /* @__PURE__ */ new Set([
  "accept",
  "authorization",
  "content-type",
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
    const credentials = requestOptions.credentials ?? options.credentials;
    let encoded: EncodedRequest;
    try {
      const pending = encodeRequest(baseURL, options, codecs, operation, input, requestOptions);
      encoded = isPromise(pending) ? await pending : pending;
      const secured = applyOperationSecurity(
        options,
        operation,
        encoded,
        requestOptions,
        credentials,
      );
      encoded = isPromise(secured) ? await secured : secured;
    } catch (cause) {
      if (isAPIError(cause)) {
        throw cause;
      }
      throw transportError(
        TransportErrorCode.REQUEST_ENCODE_FAILED,
        `Failed to encode ${operationDiagnosticName(operation)} request`,
        cause,
      );
    }

    const timeoutMS = requestOptions.timeoutMS ?? options.timeoutMS;
    const abort = createAbortContext(requestOptions.signal, timeoutMS);
    let responseMetadata:
      | { request: RequestMetadata; status: number; response: Response }
      | undefined;
    try {
      const init: RequestInit = {
        method: operation.method,
        headers: encoded.headers,
      };
      if (encoded.body !== undefined) {
        init.body = encoded.body as BodyInit;
        if (isReadableStream(encoded.body))
          (init as RequestInit & { duplex?: "half" }).duplex = "half";
      }
      if (abort.signal !== undefined) init.signal = abort.signal;
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
          await response.body?.cancel().catch(() => undefined);
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
          data: undefined as Output,
          headers: headerValues,
          request,
          response,
        };
      }
      let body: unknown;
      try {
        const decodedBody = await awaitAbortable(
          decodeResponse(operation, response, request, codecs),
          abort.signal,
        );
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
        operation.envelope === "data" && isRecord(body) && Object.hasOwn(body, "data")
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
        data: body as Output,
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
  request.stream = <Item>(
    operation: OperationDefinition,
    input?: unknown,
    requestOptions: RequestOptions = {},
  ): AsyncIterable<Item> =>
    streamOperation<Item>(
      baseURL,
      options,
      codecs,
      fetchImplementation,
      operation,
      input,
      requestOptions,
    );
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
  const credentials = requestOptions.credentials ?? options.credentials;
  let encoded: EncodedRequest;
  try {
    const pending = encodeRequest(baseURL, options, codecs, operation, input, requestOptions);
    encoded = isPromise(pending) ? await pending : pending;
    const secured = applyOperationSecurity(
      options,
      operation,
      encoded,
      requestOptions,
      credentials,
    );
    encoded = isPromise(secured) ? await secured : secured;
  } catch (cause) {
    if (isAPIError(cause)) throw cause;
    throw transportError(
      TransportErrorCode.REQUEST_ENCODE_FAILED,
      `Failed to encode ${operationDiagnosticName(operation)} stream request`,
      cause,
    );
  }
  const timeoutMS = requestOptions.timeoutMS ?? options.timeoutMS;
  const abort = createAbortContext(requestOptions.signal, timeoutMS);
  let receivedResponse = false;
  try {
    const init: RequestInit = {
      method: operation.method,
      headers: encoded.headers,
      ...(abort.signal === undefined ? {} : { signal: abort.signal }),
    };
    if (encoded.body !== undefined) {
      init.body = encoded.body as BodyInit;
      if (isReadableStream(encoded.body))
        (init as RequestInit & { duplex?: "half" }).duplex = "half";
    }
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
      throw new TypeError(
        `response for ${operationDiagnosticName(operation)} is not a declared stream`,
      );
    }
    const contentType = response.headers.get("content-type") ?? definition.contentType;
    const maxFrameBytes = resolveMaxStreamItemBytes(
      requestOptions.maxStreamItemBytes ?? options.maxStreamItemBytes,
    );
    if (isGeneratedStreamMediaType(contentType)) {
      for await (const value of decodeStreamItems(
        response.body,
        contentType,
        definition.itemSchema,
        operation.outputSchemas ?? {},
        codecs,
        definition.itemEncoding,
        maxFrameBytes,
      )) {
        yield transformWireValue(
          value,
          definition.itemSchema,
          operation.outputSchemas ?? {},
          "decode",
        ) as Item;
      }
    } else {
      const codec = codecs.get(normalizeMediaType(contentType));
      if (codec?.decodeStream === undefined)
        throw new TypeError(`missing decodeStream codec for ${contentType}`);
      const reader = createMediaStreamReader(response.body, maxFrameBytes);
      try {
        for await (const value of codec.decodeStream(reader, {
          contentType,
          maxFrameBytes,
          ...(requestOptions.signal === undefined ? {} : { signal: requestOptions.signal }),
        })) {
          yield transformWireValue(
            value,
            definition.itemSchema,
            operation.outputSchemas ?? {},
            "decode",
          ) as Item;
        }
      } finally {
        await reader.cancel();
      }
    }
  } catch (cause) {
    if (isAPIError(cause)) throw cause;
    if (abort.timedOut())
      throw transportError(
        TransportErrorCode.REQUEST_TIMEOUT,
        `Request timed out after ${timeoutMS}ms`,
        cause,
      );
    if (abort.aborted())
      throw transportError(TransportErrorCode.REQUEST_ABORTED, "Request was aborted", cause);
    if (!receivedResponse)
      throw transportError(TransportErrorCode.NETWORK_ERROR, "Network request failed", cause);
    throw transportError(
      TransportErrorCode.RESPONSE_DECODE_FAILED,
      `Failed to decode ${operationDiagnosticName(operation)} stream`,
      cause,
    );
  } finally {
    abort.cleanup();
  }
}

function resolveMaxStreamItemBytes(value: number | undefined): number {
  const resolved = value ?? 1024 * 1024;
  if (!Number.isSafeInteger(resolved) || resolved <= 0)
    throw new TypeError("maxStreamItemBytes must be a positive safe integer");
  return resolved;
}

function isGeneratedStreamMediaType(contentType: string): boolean {
  const mediaType = normalizeMediaType(contentType);
  return isSequentialStreamMediaType(mediaType) || mediaType.startsWith("multipart/");
}

function createMediaStreamReader(
  body: ReadableStream<Uint8Array>,
  maxFrameBytes: number,
): MediaStreamReader {
  if (!Number.isSafeInteger(maxFrameBytes) || maxFrameBytes <= 0)
    throw new TypeError("maxStreamItemBytes must be a positive safe integer");
  const reader = body.getReader();
  let pending: Uint8Array<ArrayBufferLike> = new Uint8Array();
  let done = false;
  let released = false;
  const cancel = async (reason?: unknown): Promise<void> => {
    if (released) return;
    released = true;
    try {
      await reader.cancel(reason);
    } finally {
      reader.releaseLock();
    }
  };
  return {
    async read(maxBytes: number): Promise<Uint8Array | null> {
      if (!Number.isSafeInteger(maxBytes) || maxBytes <= 0 || maxBytes > maxFrameBytes)
        throw new TypeError(
          `stream read size must be a positive safe integer at most ${maxFrameBytes}`,
        );
      if (released) return null;
      while (pending.length === 0 && !done) {
        const next = await reader.read();
        done = next.done;
        if (next.value !== undefined) pending = next.value;
      }
      if (pending.length === 0) {
        if (!released) {
          released = true;
          reader.releaseLock();
        }
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
          const data = event
            .split(/\r?\n/)
            .filter((line) => line.startsWith("data:"))
            .map((line) => line.slice(5).trimStart())
            .join("\n");
          if (data !== "") yield parseStreamJSON(data);
        }
      } else if (mediaType.includes("json-seq")) {
        const records = pending.split("\u001e");
        pending = records.pop() ?? "";
        for (const record of records)
          if (record.trim() !== "") yield parseStreamJSON(record.trim());
      } else {
        let newline: number;
        while ((newline = pending.indexOf("\n")) >= 0) {
          const line = pending.slice(0, newline).replace(/\r$/, "");
          pending = pending.slice(newline + 1);
          if (line.trim() !== "") yield parseStreamJSON(line);
        }
      }
      if (encoder.encode(pending).byteLength > maxFrameBytes)
        throw new TypeError(`stream item exceeds ${maxFrameBytes} bytes`);
      if (done) break;
    }
    if (pending.trim() !== "") {
      if (mediaType.includes("event-stream")) {
        const data = pending
          .split(/\r?\n/)
          .filter((line) => line.startsWith("data:"))
          .map((line) => line.slice(5).trimStart())
          .join("\n");
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
  const boundary =
    /(?:^|;)\s*boundary=(?:"([^"]+)"|([^;\s]+))/i.exec(contentType)?.[1] ??
    /(?:^|;)\s*boundary=(?:"([^"]+)"|([^;\s]+))/i.exec(contentType)?.[2];
  if (boundary === undefined || boundary === "")
    throw new TypeError("multipart response has no boundary parameter");
  const encoder = new TextEncoder();
  const opening = encoder.encode(`--${boundary}`);
  const separator = encoder.encode(`\r\n--${boundary}`);
  const reader = body.getReader();
  let pending: Uint8Array<ArrayBufferLike> = new Uint8Array();
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
          if (pending[after] === 45 && pending[after + 1] === 45) {
            closed = true;
            pending = pending.slice(after + 2);
            continue;
          }
          if (pending[after] !== 13 || pending[after + 1] !== 10)
            throw new TypeError("multipart opening boundary is malformed");
          pending = pending.slice(after + 2);
          started = true;
          continue;
        }
        const index = findStreamBytes(pending, separator);
        if (index < 0) break;
        const after = index + separator.length;
        if (pending.length < after + 2) break;
        const closing = pending[after] === 45 && pending[after + 1] === 45;
        if (!closing && (pending[after] !== 13 || pending[after + 1] !== 10))
          throw new TypeError("multipart boundary is malformed");
        const part = pending.slice(0, index);
        pending = pending.slice(after + 2);
        yield parseMultipartStreamPart(part);
        if (closing) closed = true;
      }
      if (done) break;
    }
    if (!closed) throw new TypeError("multipart response ended before its closing boundary");
  } finally {
    try {
      await reader.cancel();
    } finally {
      reader.releaseLock();
    }
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
      if (header.required)
        throw new TypeError(`multipart part is missing required header ${header.name}`);
      continue;
    }
    const decoded = await decodeResponseHeaderValue(
      header.name,
      value,
      header.schema,
      header.contentType,
      header.explode,
      schemas,
      codecs,
    );
    validateWireValue(decoded, header.schema, schemas, "decode");
  }
  const declared = itemEncoding?.contentType?.split(",", 1)[0]?.trim();
  const rawPartContentType = headers.get("content-type") ?? declared ?? "text/plain";
  const partContentType = normalizeMediaType(rawPartContentType);
  if (partContentType.startsWith("multipart/")) {
    return decodeMultipartResponse(
      new Blob([ownedArrayBuffer(bytes)]).stream(),
      rawPartContentType,
      {
        contentType: rawPartContentType,
        schema: itemSchema,
        ...(itemEncoding?.encoding === undefined ? {} : { encoding: itemEncoding.encoding }),
        ...(itemEncoding?.prefixEncoding === undefined
          ? {}
          : { prefixEncoding: itemEncoding.prefixEncoding }),
        ...(itemEncoding?.itemEncoding === undefined
          ? {}
          : { itemEncoding: itemEncoding.itemEncoding }),
      },
      schemas,
      codecs,
    );
  }
  if (isJSONMediaType(partContentType)) return parseStreamJSON(new TextDecoder().decode(bytes));
  if (isXMLMediaType(partContentType))
    return decodeXML(new TextDecoder().decode(bytes), itemSchema, schemas);
  if (partContentType.startsWith("text/")) return new TextDecoder().decode(bytes);
  if (isBinaryMediaType(partContentType) || itemSchema.contentEncoding === "binary")
    return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
  const codec = codecs.get(normalizeMediaType(partContentType));
  if (codec?.decode === undefined)
    throw new TypeError(`missing decode codec for multipart item ${partContentType}`);
  return codec.decode(
    new Response(ownedArrayBuffer(bytes), { headers: { "content-type": partContentType } }),
    { contentType: partContentType },
  );
}

function ownedArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const copy = new Uint8Array(bytes.byteLength);
  copy.set(bytes);
  return copy.buffer;
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
    for (let index = 0; index < wanted.length; index++)
      if (source[start + index] !== wanted[index]) continue outer;
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

async function decodeResponseHeaders(
  operation: OperationDefinition,
  response: Response,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<Readonly<Record<string, unknown>>> {
  const definition = selectResponseDefinition(operation, response, false);
  const values = Object.create(null) as Record<string, unknown>;
  for (const header of definition?.headers ?? []) {
    const value = response.headers.get(header.name);
    if (value === null) {
      if (header.required) throw new TypeError(`missing required response header ${header.name}`);
      continue;
    }
    const decoded = await decodeResponseHeaderValue(
      header.name,
      value,
      header.schema,
      header.contentType,
      header.explode,
      operation.outputSchemas ?? {},
      codecs,
    );
    validateWireValue(decoded, header.schema, operation.outputSchemas ?? {}, "decode");
    defineOwnDataProperty(
      values,
      header.property,
      decodeWireValue(decoded, header.schema, operation.outputSchemas ?? {}),
    );
  }
  return values;
}

function applyOperationSecurity(
  options: ClientOptions,
  operation: OperationDefinition,
  encoded: EncodedRequest,
  requestOptions: OperationRequestOptions,
  credentials: RequestCredentials | undefined,
): EncodedRequest | Promise<EncodedRequest> {
  const declared = operation.security;
  const requestedID = requestOptions.securityRequirement;
  if (declared === undefined || declared.length === 0) {
    if (requestedID !== undefined)
      throw securityRequirementInvalid(
        "The operation does not declare an OpenAPI security requirement",
      );
    return encoded;
  }
  const requirements = Object.create(null) as Record<string, SecurityRequirementDefinition>;
  for (const requirement of declared)
    defineOwnDataProperty(requirements, requirement.id, requirement);
  let selected: SecurityRequirementDefinition;
  if (declared.length === 1) {
    if (requestedID !== undefined) {
      throw securityRequirementInvalid(
        `Operation ${operationDiagnosticName(operation)} has one SDK-selected security requirement and does not accept an explicit selection`,
      );
    }
    selected = declared[0]!;
  } else {
    if (requestedID === undefined) {
      throw transportError(
        TransportErrorCode.SECURITY_REQUIREMENT_REQUIRED,
        `Operation ${operationDiagnosticName(operation)} requires an explicit OpenAPI security requirement`,
        undefined,
      );
    }
    const requested = requirements[requestedID];
    if (requested === undefined) {
      throw securityRequirementInvalid(
        `Operation ${operationDiagnosticName(operation)} does not declare security requirement ${requestedID}`,
      );
    }
    selected = requested;
  }
  if (
    securityRequirementIsSatisfied(options, requestOptions, encoded, credentials, selected, true)
  ) {
    return applySelectedSecurityRequirement(
      options,
      requestOptions,
      encoded,
      credentials,
      selected,
      {},
      false,
    );
  }
  if (typeof options.securityProvider !== "function") {
    return applySelectedSecurityRequirement(
      options,
      requestOptions,
      encoded,
      credentials,
      selected,
      {},
      false,
    );
  }
  const context: SecurityCredentialContext = {
    operation: {
      route: operation.route,
      ...(operation.operationID === undefined ? {} : { operationID: operation.operationID }),
      method: operation.method,
      path: operation.path,
    },
    requirement: selected,
    origin: new URL(encoded.url).origin,
  };
  const suppliedCredentials = options.securityProvider(context);
  const apply = (resolved: SecurityCredentials): EncodedRequest =>
    applySelectedSecurityRequirement(
      options,
      requestOptions,
      encoded,
      credentials,
      selected,
      resolved,
      true,
    );
  return isPromise(suppliedCredentials)
    ? suppliedCredentials.then(apply)
    : apply(suppliedCredentials);
}

type SDKSecuritySource =
  | { readonly state: "none" }
  | { readonly state: "conflict"; readonly location: string }
  | {
      readonly state: "satisfied";
      readonly kind: "header";
      readonly name: string;
      readonly value: string;
    }
  | { readonly state: "satisfied"; readonly kind: "cookie" | "mutualTLS" };

function securityRequirementIsSatisfied(
  options: ClientOptions,
  requestOptions: RequestOptions,
  encoded: EncodedRequest,
  credentials: RequestCredentials | undefined,
  requirement: SecurityRequirementDefinition,
  allowMutualTLS: boolean,
): boolean {
  return requirement.schemes.every(
    (scheme) =>
      securitySourceForScheme(options, requestOptions, encoded, credentials, scheme, allowMutualTLS)
        .state === "satisfied",
  );
}

function securitySourceForScheme(
  options: ClientOptions,
  requestOptions: RequestOptions,
  encoded: EncodedRequest,
  credentials: RequestCredentials | undefined,
  scheme: SecuritySchemeDefinition,
  allowMutualTLS: boolean,
): SDKSecuritySource {
  if (usesAuthorizationHeader(scheme)) {
    if (requestOptions.authorization === undefined && options.authorization === undefined)
      return { state: "none" };
    const value = encoded.headers.get("Authorization") ?? "";
    return matchesAuthorizationScheme(scheme, value)
      ? { state: "satisfied", kind: "header", name: "Authorization", value }
      : { state: "conflict", location: "Authorization header" };
  }
  if (isCSRFHeaderScheme(scheme)) {
    if (requestOptions.csrfToken === undefined) return { state: "none" };
    const value = encoded.headers.get("X-CSRF-Token") ?? "";
    return value === ""
      ? { state: "conflict", location: "X-CSRF-Token header" }
      : { state: "satisfied", kind: "header", name: "X-CSRF-Token", value };
  }
  if (scheme.type === "apiKey" && scheme.location === "cookie" && credentials === "include") {
    return { state: "satisfied", kind: "cookie" };
  }
  if (scheme.type === "mutualTLS" && allowMutualTLS && options.transport?.capabilities?.mutualTLS) {
    return { state: "satisfied", kind: "mutualTLS" };
  }
  return { state: "none" };
}

function usesAuthorizationHeader(scheme: SecuritySchemeDefinition): boolean {
  return (
    scheme.type === "http" ||
    scheme.type === "oauth2" ||
    scheme.type === "openIdConnect" ||
    (scheme.type === "apiKey" &&
      scheme.location === "header" &&
      scheme.parameterName?.toLowerCase() === "authorization")
  );
}

function isCSRFHeaderScheme(scheme: SecuritySchemeDefinition): boolean {
  return (
    scheme.type === "apiKey" &&
    scheme.location === "header" &&
    scheme.parameterName?.toLowerCase() === "x-csrf-token"
  );
}

function matchesAuthorizationScheme(scheme: SecuritySchemeDefinition, value: string): boolean {
  if (value === "") return false;
  if (scheme.type === "apiKey") return true;
  const separator = value.indexOf(" ");
  if (separator <= 0 || value.slice(separator + 1).trim() === "") return false;
  const protocol = value.slice(0, separator).toLowerCase();
  if (scheme.type === "oauth2" || scheme.type === "openIdConnect") return protocol === "bearer";
  return protocol === scheme.scheme?.toLowerCase();
}

function applySelectedSecurityRequirement(
  options: ClientOptions,
  requestOptions: RequestOptions,
  encoded: EncodedRequest,
  credentialsMode: RequestCredentials | undefined,
  requirement: SecurityRequirementDefinition,
  suppliedCredentials: unknown,
  providerReturned: boolean,
): EncodedRequest {
  if (!isRecord(suppliedCredentials)) {
    throw transportError(
      TransportErrorCode.SECURITY_CREDENTIALS_INVALID,
      "Security credential provider returned an invalid credentials object",
      undefined,
    );
  }
  const declaredNames = new Set(requirement.schemes.map((scheme) => scheme.name));
  if (Object.keys(suppliedCredentials).some((name) => !declaredNames.has(name))) {
    throw transportError(
      TransportErrorCode.SECURITY_CREDENTIALS_INVALID,
      "Security credential provider returned credentials outside the selected requirement",
      undefined,
    );
  }
  const url = new URL(encoded.url);
  for (const scheme of requirement.schemes) {
    const source = securitySourceForScheme(
      options,
      requestOptions,
      encoded,
      credentialsMode,
      scheme,
      true,
    );
    const credential = suppliedCredentials[scheme.name] as SecurityCredential | undefined;
    if (source.state === "conflict") throw securityCollision(scheme.name, source.location);
    if (source.state === "satisfied") {
      if (credential === undefined) continue;
      if (source.kind === "header") {
        const header = securityCredentialHeader(scheme, credential);
        if (
          header !== undefined &&
          header.name.toLowerCase() === source.name.toLowerCase() &&
          normalizeHeaderValue(header.name, header.value) === source.value
        )
          continue;
        throw securityCollision(scheme.name, source.name);
      }
      if (source.kind === "mutualTLS") {
        assertSecurityCredentialShape(scheme, credential);
        continue;
      }
      assertSecurityCredentialShape(scheme, credential);
      throw securityCollision(scheme.name, "ambient Cookie credentials");
    }
    if (credential === undefined) {
      if (providerReturned) {
        throw transportError(
          TransportErrorCode.SECURITY_CREDENTIALS_INVALID,
          `Security credential provider omitted scheme ${scheme.name}`,
          undefined,
        );
      }
      throw transportError(
        TransportErrorCode.SECURITY_CREDENTIALS_REQUIRED,
        `Security requirement ${requirement.id} requires credentials for scheme ${scheme.name}`,
        undefined,
      );
    }
    applySecurityCredential(options.transport, scheme, credential, encoded.headers, url);
  }
  return { ...encoded, url: url.href };
}

function applySecurityCredential(
  transport: Transport | undefined,
  scheme: SecuritySchemeDefinition,
  credential: SecurityCredential,
  headers: Headers,
  url: URL,
): void {
  const header = securityCredentialHeader(scheme, credential);
  if (header !== undefined) {
    if (headers.has(header.name)) throw securityCollision(scheme.name, `header ${header.name}`);
    headers.set(header.name, header.value);
    return;
  }
  switch (scheme.type) {
    case "apiKey": {
      const apiKey = credential as APIKeyCredential;
      if (scheme.location === "query") {
        if (url.searchParams.has(scheme.parameterName!))
          throw securityCollision(scheme.name, `query parameter ${scheme.parameterName}`);
        url.searchParams.set(scheme.parameterName!, apiKey.value);
        return;
      }
      if (!transport?.capabilities?.cookieJar) {
        throw transportError(
          TransportErrorCode.TRANSPORT_CAPABILITY_REQUIRED,
          `Security scheme ${scheme.name} requires a cookie-jar transport`,
          undefined,
        );
      }
      if (headers.has("Cookie")) throw securityCollision(scheme.name, "Cookie header");
      headers.set(
        "Cookie",
        `${encodeURIComponent(scheme.parameterName!)}=${encodeURIComponent(apiKey.value)}`,
      );
      return;
    }
    case "http":
    case "oauth2":
    case "openIdConnect":
      return;
    case "mutualTLS":
      if (!transport?.capabilities?.mutualTLS) {
        throw transportError(
          TransportErrorCode.TRANSPORT_CAPABILITY_REQUIRED,
          `Security scheme ${scheme.name} requires a mutual-TLS transport`,
          undefined,
        );
      }
      return;
  }
}

function securityCredentialHeader(
  scheme: SecuritySchemeDefinition,
  credential: SecurityCredential,
): { readonly name: string; readonly value: string } | undefined {
  assertSecurityCredentialShape(scheme, credential);
  if (scheme.type === "apiKey") {
    const apiKey = credential as APIKeyCredential;
    return scheme.location === "header"
      ? { name: scheme.parameterName!, value: apiKey.value }
      : undefined;
  }
  if (scheme.type === "http") {
    if (scheme.scheme === "basic") {
      const basic = credential as HTTPBasicCredential;
      return {
        name: "Authorization",
        value: `Basic ${base64(`${basic.username}:${basic.password}`)}`,
      };
    }
    if (scheme.scheme === "bearer")
      return {
        name: "Authorization",
        value: `Bearer ${(credential as HTTPBearerCredential).token}`,
      };
    return {
      name: "Authorization",
      value: `${scheme.scheme} ${(credential as HTTPCredential).value}`,
    };
  }
  if (scheme.type === "oauth2" || scheme.type === "openIdConnect") {
    return { name: "Authorization", value: `Bearer ${(credential as OAuthCredential).token}` };
  }
  return undefined;
}

function assertSecurityCredentialShape(
  scheme: SecuritySchemeDefinition,
  credential: SecurityCredential,
): void {
  if (scheme.type === "apiKey") {
    if (
      credential?.kind !== "api-key" ||
      typeof credential.value !== "string" ||
      credential.value === ""
    )
      throw securityCredentialError(scheme.name, "api-key value");
    return;
  }
  if (scheme.type === "http") {
    if (scheme.scheme === "basic") {
      if (
        credential?.kind !== "http-basic" ||
        typeof credential.username !== "string" ||
        typeof credential.password !== "string"
      )
        throw securityCredentialError(scheme.name, "http-basic credential");
      return;
    }
    if (scheme.scheme === "bearer") {
      if (
        credential?.kind !== "http-bearer" ||
        typeof credential.token !== "string" ||
        credential.token === ""
      )
        throw securityCredentialError(scheme.name, "http-bearer token");
      return;
    }
    if (
      credential?.kind !== "http" ||
      typeof credential.value !== "string" ||
      credential.value === ""
    )
      throw securityCredentialError(scheme.name, "http credential");
    return;
  }
  if (scheme.type === "oauth2" || scheme.type === "openIdConnect") {
    if (
      credential?.kind !== scheme.type ||
      typeof credential.token !== "string" ||
      credential.token === ""
    )
      throw securityCredentialError(scheme.name, `${scheme.type} token`);
    return;
  }
  if (credential?.kind !== "mutual-tls")
    throw securityCredentialError(scheme.name, "mutual-tls credential");
}

function normalizeHeaderValue(name: string, value: string): string {
  const headers = new Headers();
  headers.set(name, value);
  return headers.get(name)!;
}

function securityRequirementInvalid(message: string): TransportError {
  return transportError(TransportErrorCode.SECURITY_REQUIREMENT_INVALID, message, undefined);
}

function securityCredentialError(scheme: string, expected: string): TransportError {
  return transportError(
    TransportErrorCode.SECURITY_CREDENTIALS_INVALID,
    `Security scheme ${scheme} requires ${expected}`,
    undefined,
  );
}

function securityCollision(scheme: string, location: string): TransportError {
  return transportError(
    TransportErrorCode.SECURITY_CREDENTIALS_INVALID,
    `Security scheme ${scheme} conflicts with caller-supplied ${location}`,
    undefined,
  );
}

function base64(value: string): string {
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function assertReadableResponseHeaders(
  transport: Transport | undefined,
  operation: OperationDefinition,
): void {
  const readable = transport?.capabilities?.readableResponseHeaders;
  for (const response of operation.responses ?? []) {
    for (const header of response.headers ?? []) {
      if (!header.required || header.name.toLowerCase() !== "set-cookie") continue;
      if (
        readable === true ||
        (Array.isArray(readable) && readable.some((name) => name.toLowerCase() === "set-cookie"))
      )
        continue;
      throw transportError(
        TransportErrorCode.TRANSPORT_CAPABILITY_REQUIRED,
        "Reading required Set-Cookie response headers requires a capable transport",
        undefined,
      );
    }
  }
}

async function decodeResponseHeaderValue(
  name: string,
  value: string,
  schema: WireSchema,
  contentType: string | undefined,
  explode: boolean | undefined,
  schemas: WireSchemas,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<unknown> {
  if (contentType !== undefined) {
    const decoded = decodeHeaderContent(name, value, contentType);
    if (
      isJSONMediaType(contentType) ||
      contentType.toLowerCase() === "application/x-www-form-urlencoded"
    )
      return decoded;
    if (isXMLMediaType(contentType)) return decodeXML(value, schema, schemas);
    if (!contentType.toLowerCase().startsWith("text/")) {
      const codec = codecs.get(normalizeMediaType(contentType));
      if (codec?.decodeParameter === undefined)
        throw new TypeError(`missing decodeParameter codec for response header ${name}`);
      return codec.decodeParameter(value, { contentType });
    }
    value = decoded as string;
  }
  const resolved = resolveHeaderSchema(schema, schemas);
  if (resolved.types?.includes("array"))
    return value
      .split(",")
      .map((entry) => decodeResponseHeaderScalar(name, entry, resolved.items ?? {}, schemas));
  if (resolved.types?.includes("object") || resolved.properties !== undefined) {
    const result = Object.create(null) as Record<string, unknown>;
    const tokens = value.split(",");
    if (explode)
      for (const token of tokens) {
        const separator = token.indexOf("=");
        if (separator < 0) continue;
        const propertyName = token.slice(0, separator);
        const property = resolved.properties?.[propertyName];
        defineOwnDataProperty(
          result,
          propertyName,
          decodeResponseHeaderScalar(
            name,
            token.slice(separator + 1),
            property?.schema ?? {},
            schemas,
          ),
        );
      }
    else
      for (let index = 0; index + 1 < tokens.length; index += 2) {
        const property = resolved.properties?.[tokens[index]!];
        defineOwnDataProperty(
          result,
          tokens[index]!,
          decodeResponseHeaderScalar(name, tokens[index + 1]!, property?.schema ?? {}, schemas),
        );
      }
    return result;
  }
  return decodeResponseHeaderScalar(name, value, resolved, schemas);
}

function decodeResponseHeaderScalar(
  name: string,
  value: string,
  schema: WireSchema,
  schemas: WireSchemas,
): unknown {
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
    const result = Object.create(null) as Record<string, string | string[]>;
    for (const [key, item] of new URLSearchParams(value)) {
      const previous = result[key];
      defineOwnDataProperty(
        result,
        key,
        previous === undefined
          ? item
          : Array.isArray(previous)
            ? [...previous, item]
            : [previous, item],
      );
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
    if (parameter.contentType === undefined || !requiresParameterCodec(parameter.contentType))
      continue;
    const source =
      parameter.location === "path"
        ? values.path
        : parameter.location === "header"
          ? values.headerParams
          : parameter.location === "cookie"
            ? values.cookieParams
            : parameter.location === "querystring"
              ? values.querystring
              : values.query;
    if (isRecord(source) && source[parameter.property] !== undefined) return true;
  }
  return false;
}

function requiresParameterCodec(contentType: string): boolean {
  return (
    !isJSONMediaType(contentType) &&
    !isXMLMediaType(contentType) &&
    contentType.toLowerCase() !== "application/x-www-form-urlencoded" &&
    !contentType.toLowerCase().startsWith("text/")
  );
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
    if (rawValue === undefined || rawValue === null)
      throw new TypeError(`Missing path parameter ${name}`);
    return serializePathParameterSync(
      parameter,
      name,
      encodeParameterWireValue(operation, parameter, rawValue),
      operation.inputSchemas ?? {},
    );
  });
  const url = new URL(
    resolveOperationBaseURL(options.baseURL ?? baseURL, client.origin, client.server, operation) +
      (path.startsWith("/") ? path : `/${path}`),
  );
  const queryValues = isRecord(values.query) ? values.query : {};
  const querystringValues = isRecord(values.querystring) ? values.querystring : {};
  rejectUndefinedArrayValues(queryValues);
  rejectUndefinedArrayValues(querystringValues);
  const query = [
    ...appendQuerySync(queryValues, operation, "query"),
    ...appendQuerySync(querystringValues, operation, "querystring"),
  ];
  if (query.length > 0)
    url.search = `${url.search}${url.search === "" ? "?" : "&"}${serializeQuery(query)}`;
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
  const headerParams = { ...(isRecord(values.headerParams) ? values.headerParams : {}) };
  rejectUndefinedArrayValues(headerParams);
  for (const [property, value] of Object.entries(headerParams)) {
    if (value === undefined) continue;
    const parameter = findParameterByProperty(operation, "header", property);
    const name = parameter?.name ?? property;
    const serialized =
      parameter?.contentType === undefined
        ? serializeSimpleValue(
            encodeParameterWireValue(operation, parameter, value),
            parameter?.explode ?? false,
          )
        : serializeContentParameterSync(
            encodeParameterWireValue(operation, parameter, value),
            parameter.contentType,
            parameter.schema,
            operation.inputSchemas ?? {},
          );
    headers.set(name, serialized);
  }
  setHeader(headers, "Authorization", options.authorization ?? client.authorization);
  setHeader(headers, "Accept", options.accept);
  setHeader(headers, "X-CSRF-Token", options.csrfToken);
  setHeader(headers, "X-Request-Id", options.requestID);
  const cookieValues = isRecord(values.cookieParams) ? values.cookieParams : {};
  rejectUndefinedArrayValues(cookieValues);
  assertRequiredParameters(
    operation,
    pathValues,
    queryValues,
    querystringValues,
    headerParams,
    cookieValues,
  );
  const cookies = Object.entries(cookieValues)
    .filter((entry): entry is [string, unknown] => entry[1] !== undefined)
    .flatMap(([property, value]) => serializeCookieSync(operation, property, value));
  if (cookies.length > 0) {
    if (!client.transport?.capabilities?.cookieJar)
      throw transportError(
        TransportErrorCode.TRANSPORT_CAPABILITY_REQUIRED,
        "Sending declared cookie parameters requires a cookie-jar transport",
        undefined,
      );
    headers.set("Cookie", cookies.join("; "));
  }
  if (!Object.hasOwn(values, "body") || values.body === undefined) {
    if (operation.requestBodyRequired) throw new TypeError("Missing required request body");
    return { url: url.href, headers };
  }
  rejectUndefinedArrayValues(values.body);
  let contentType = operation.contentType ?? "application/json";
  let bodyValue: unknown = values.body;
  const requestBodies = operation.requestBodies;
  const needsSelection =
    requestBodies !== undefined &&
    (requestBodies.length > 1 || requestBodies.some((body) => body.contentType.includes("*")));
  if (needsSelection) {
    if (
      !isRecord(values.body) ||
      typeof values.body.contentType !== "string" ||
      !Object.hasOwn(values.body, "value")
    )
      throw new TypeError("request body media range requires { contentType, value }");
    const selected = selectRequestBodyDefinition(requestBodies!, values.body.contentType);
    if (selected === undefined)
      throw new TypeError(
        `request body content type ${values.body.contentType} is not declared by this operation`,
      );
    contentType = values.body.contentType;
    bodyValue = values.body.value;
  }
  const definition =
    requestBodies === undefined
      ? undefined
      : selectRequestBodyDefinition(requestBodies, contentType);
  if (
    definition?.itemSchema !== undefined &&
    isAsyncIterable(bodyValue) &&
    normalizeMediaType(contentType).startsWith("multipart/")
  ) {
    const encoded = encodeStreamingMultipartBody(
      contentType,
      bodyValue,
      definition.itemSchema,
      operation.inputSchemas ?? {},
      definition.itemEncoding,
      options.multipartHeaders,
      options.multipartContentTypes,
      codecs,
    );
    headers.set("Content-Type", encoded.contentType);
    return { url: url.href, headers, body: encoded.body };
  }
  if (
    definition?.itemSchema !== undefined &&
    isAsyncIterable(bodyValue) &&
    !isGeneratedStreamMediaType(contentType)
  ) {
    const stream = encodeCustomStreamingRequestBody(
      contentType,
      bodyValue,
      definition.itemSchema,
      operation.inputSchemas ?? {},
      codecs,
      options.signal,
    );
    const finishStream = (body: ReadableStream<Uint8Array>): EncodedRequest => {
      headers.set("Content-Type", contentType);
      return { url: url.href, headers, body };
    };
    return isPromise(stream) ? stream.then(finishStream) : finishStream(stream);
  }
  const body =
    definition?.itemSchema !== undefined && isAsyncIterable(bodyValue)
      ? encodeSequentialRequestBody(
          contentType,
          bodyValue,
          definition.itemSchema,
          operation.inputSchemas ?? {},
        )
      : encodeRequestBody(
          contentType,
          encodeRequestWireValue(operation, contentType, bodyValue),
          codecs,
          definition?.schema,
          operation.inputSchemas ?? {},
          definition,
          options.multipartHeaders,
          options.multipartContentTypes,
        );
  const finish = (resolved: BodyInit | ReadableStream<Uint8Array>): EncodedRequest => {
    if (!(resolved instanceof FormData))
      headers.set(
        "Content-Type",
        normalizeMediaType(contentType).startsWith("multipart/") && resolved instanceof Blob
          ? resolved.type
          : contentType,
      );
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
    path = path.replace(
      match[0],
      await serializePathParameter(parameter, name, value, operation.inputSchemas ?? {}, codecs),
    );
  }
  const operationBaseURL = resolveOperationBaseURL(
    options.baseURL ?? baseURL,
    client.origin,
    client.server,
    operation,
  );
  const url = new URL(operationBaseURL + (path.startsWith("/") ? path : `/${path}`));
  const queryValues = isRecord(values.query) ? values.query : {};
  const querystringValues = isRecord(values.querystring) ? values.querystring : {};
  rejectUndefinedArrayValues(queryValues);
  rejectUndefinedArrayValues(querystringValues);
  const query = [
    ...(await appendQuery(queryValues, operation, codecs, "query")),
    ...(await appendQuery(querystringValues, operation, codecs, "querystring")),
  ];
  if (query.length > 0)
    url.search = `${url.search}${url.search === "" ? "?" : "&"}${serializeQuery(query)}`;

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
    const serialized =
      parameter?.contentType === undefined
        ? serializeSimpleValue(encodedValue, parameter?.explode ?? false)
        : await serializeContentParameter(
            encodedValue,
            parameter.contentType,
            parameter.schema,
            operation.inputSchemas ?? {},
            codecs,
          );
    headers.set(name, serialized);
  }
  setHeader(headers, "Authorization", options.authorization ?? client.authorization);
  setHeader(headers, "Accept", options.accept);
  setHeader(headers, "X-CSRF-Token", options.csrfToken);
  setHeader(headers, "X-Request-Id", options.requestID);

  const cookieValues = isRecord(values.cookieParams) ? values.cookieParams : {};
  rejectUndefinedArrayValues(cookieValues);
  assertRequiredParameters(
    operation,
    pathValues,
    queryValues,
    querystringValues,
    headerParams,
    cookieValues,
  );
  const cookiePromises = Object.entries(cookieValues)
    .filter((entry): entry is [string, unknown] => entry[1] !== undefined)
    .map(async ([property, value]) => serializeCookie(operation, property, value, codecs));
  const cookies = (await Promise.all(cookiePromises)).flat();
  if (cookies.length > 0) {
    if (!client.transport?.capabilities?.cookieJar) {
      throw transportError(
        TransportErrorCode.TRANSPORT_CAPABILITY_REQUIRED,
        "Sending declared cookie parameters requires a cookie-jar transport",
        undefined,
      );
    }
    headers.set("Cookie", cookies.join("; "));
  }

  if (!Object.hasOwn(values, "body") || values.body === undefined) {
    if (operation.requestBodyRequired) throw new TypeError("Missing required request body");
    return { url: url.href, headers };
  }
  rejectUndefinedArrayValues(values.body);
  let contentType = operation.contentType ?? "application/json";
  let bodyValue: unknown = values.body;
  const requestBodies = operation.requestBodies;
  const needsSelection =
    requestBodies !== undefined &&
    (requestBodies.length > 1 || requestBodies.some((body) => body.contentType.includes("*")));
  if (needsSelection) {
    if (
      !isRecord(values.body) ||
      typeof values.body.contentType !== "string" ||
      !Object.hasOwn(values.body, "value")
    )
      throw new TypeError("request body media range requires { contentType, value }");
    const selected = selectRequestBodyDefinition(requestBodies!, values.body.contentType);
    if (selected === undefined)
      throw new TypeError(
        `request body content type ${values.body.contentType} is not declared by this operation`,
      );
    contentType = values.body.contentType;
    bodyValue = values.body.value;
  }
  const definition =
    requestBodies === undefined
      ? undefined
      : selectRequestBodyDefinition(requestBodies, contentType);
  if (
    definition?.itemSchema !== undefined &&
    isAsyncIterable(bodyValue) &&
    normalizeMediaType(contentType).startsWith("multipart/")
  ) {
    const encoded = encodeStreamingMultipartBody(
      contentType,
      bodyValue,
      definition.itemSchema,
      operation.inputSchemas ?? {},
      definition.itemEncoding,
      options.multipartHeaders,
      options.multipartContentTypes,
      codecs,
    );
    headers.set("Content-Type", encoded.contentType);
    return { url: url.href, headers, body: encoded.body };
  }
  if (
    definition?.itemSchema !== undefined &&
    isAsyncIterable(bodyValue) &&
    !isGeneratedStreamMediaType(contentType)
  ) {
    const stream = await encodeCustomStreamingRequestBody(
      contentType,
      bodyValue,
      definition.itemSchema,
      operation.inputSchemas ?? {},
      codecs,
      options.signal,
    );
    headers.set("Content-Type", contentType);
    return { url: url.href, headers, body: stream };
  }
  const body =
    definition?.itemSchema !== undefined && isAsyncIterable(bodyValue)
      ? encodeSequentialRequestBody(
          contentType,
          bodyValue,
          definition.itemSchema,
          operation.inputSchemas ?? {},
        )
      : encodeRequestBody(
          contentType,
          encodeRequestWireValue(operation, contentType, bodyValue),
          codecs,
          definition?.schema,
          operation.inputSchemas ?? {},
          definition,
          options.multipartHeaders,
          options.multipartContentTypes,
        );
  const finish = (resolved: BodyInit | ReadableStream<Uint8Array>): EncodedRequest => {
    if (!(resolved instanceof FormData)) {
      const resolvedContentType =
        normalizeMediaType(contentType).startsWith("multipart/") && resolved instanceof Blob
          ? resolved.type
          : contentType;
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
  querystringValues: Record<string, unknown>,
  headerValues: Record<string, unknown>,
  cookieValues: Record<string, unknown>,
): void {
  for (const parameter of operation.parameters ?? []) {
    if (!parameter.required) continue;
    const values =
      parameter.location === "path"
        ? pathValues
        : parameter.location === "query"
          ? queryValues
          : parameter.location === "querystring"
            ? querystringValues
            : parameter.location === "header"
              ? headerValues
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
      return encodePositionalMultipartBody(
        contentType,
        value,
        definition.prefixEncoding,
        definition.itemEncoding,
        multipartHeaders,
        multipartContentTypes,
        schema,
        schemas,
        codecs,
      );
    }
    if (normalizedContentType !== "multipart/form-data")
      throw new TypeError(
        `named multipart encoding requires multipart/form-data, got ${contentType}`,
      );
    if (!isRecord(value)) throw new TypeError("multipart body must be an object");
    if (
      definition?.encoding?.some((entry) => (entry.headers?.length ?? 0) > 0) ||
      multipartHeaders !== undefined
    ) {
      return encodeMultipartBody(
        value,
        definition?.encoding,
        multipartHeaders,
        schema,
        schemas,
        codecs,
      );
    }
    const form = new FormData();
    const append = (
      name: string,
      item: unknown,
      definition: WireEncodingDefinition | undefined,
    ): void => {
      if (item instanceof Blob) form.append(name, item);
      else if (item instanceof ArrayBuffer) form.append(name, new Blob([item]));
      else if (ArrayBuffer.isView(item)) {
        const bytes = new Uint8Array(item.byteLength);
        bytes.set(new Uint8Array(item.buffer, item.byteOffset, item.byteLength));
        form.append(name, new Blob([bytes.buffer]));
      } else if (isRecord(item) || Array.isArray(item)) {
        form.append(
          name,
          new Blob([JSON.stringify(item)], { type: definition?.contentType ?? "application/json" }),
        );
      } else if (definition?.contentType !== undefined)
        form.append(name, new Blob([String(item)], { type: definition.contentType }));
      else form.append(name, String(item));
    };
    for (const [name, item] of Object.entries(value)) {
      if (item === undefined) continue;
      const encoding = definition?.encoding?.find((entry) => entry.name === name);
      if (Array.isArray(item) && encoding?.explode !== false)
        for (const entry of item) append(name, entry, encoding);
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
    const selectedContentType = resolveMultipartContentType(
      definition?.contentType,
      suppliedContentTypes?.[String(index)],
      defaultMultipartContentType(itemSchema),
      value,
    );
    const { body, contentType: partContentType } = await multipartPartValue(
      value,
      selectedContentType,
      itemSchema,
      definition,
      schemas,
      codecs,
    );
    const headers = await multipartPartHeaders(
      undefined,
      definition,
      suppliedHeaders?.[String(index)],
      partContentType,
      undefined,
      schemas,
      codecs,
    );
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
  if (!isSequentialStreamMediaType(mediaType))
    throw new TypeError(`unsupported streaming request media type ${contentType}`);
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
        const record = mediaType.includes("event-stream")
          ? `data: ${json}\n\n`
          : mediaType.includes("json-seq")
            ? `\u001e${json}\n`
            : `${json}\n`;
        controller.enqueue(encoder.encode(record));
      } catch (cause) {
        controller.error(cause);
        try {
          await iterator.return?.();
        } catch {
          /* original error wins */
        }
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
  if (codec?.encodeStream === undefined)
    throw new TypeError(`missing encodeStream codec for ${contentType}`);
  return codec.encodeStream(transformStreamingRequestItems(values, itemSchema, schemas), {
    contentType,
    ...(signal === undefined ? {} : { signal }),
  });
}

async function* transformStreamingRequestItems(
  values: AsyncIterable<unknown>,
  itemSchema: WireSchema,
  schemas: WireSchemas,
): AsyncIterable<unknown> {
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
        const selectedContentType = resolveMultipartContentType(
          itemEncoding?.contentType,
          suppliedContentTypes?.[String(index)],
          defaultMultipartContentType(itemSchema),
          value,
        );
        const part = await multipartPartValue(
          value,
          selectedContentType,
          itemSchema,
          itemEncoding,
          schemas,
          codecs,
        );
        const headers = await multipartPartHeaders(
          undefined,
          itemEncoding,
          suppliedHeaders?.[String(index)],
          part.contentType,
          undefined,
          schemas,
          codecs,
        );
        index++;
        const bytes = await new Blob([
          `--${boundary}\r\n${headers}\r\n\r\n`,
          part.body,
          "\r\n",
        ]).arrayBuffer();
        controller.enqueue(new Uint8Array(bytes));
      } catch (cause) {
        controller.error(cause);
        try {
          await iterator.return?.();
        } catch {
          /* original error wins */
        }
      }
    },
    async cancel(reason): Promise<void> {
      await iterator.return?.(reason);
    },
  });
  return { body, contentType: `${contentType}; boundary=${boundary}` };
}

function defaultMultipartContentType(schema: WireSchema): string {
  const types = schema.types ?? [];
  if (types.includes("object") || types.includes("array")) return "application/json";
  if (types.includes("string"))
    return schema.contentEncoding === undefined ? "text/plain" : "application/octet-stream";
  if (types.includes("number") || types.includes("integer") || types.includes("boolean"))
    return "text/plain";
  return "application/octet-stream";
}

function resolveMultipartContentType(
  declared: string | undefined,
  selected: string | undefined,
  fallback: string,
  value: unknown,
): string {
  const candidate = normalizeMediaType(
    selected ?? (value instanceof Blob && value.type !== "" ? value.type : fallback),
  );
  if (declared === undefined || declared.trim() === "") return candidate;
  const allowed = declared
    .split(",")
    .map(normalizeMediaType)
    .filter((item) => item !== "");
  if (allowed.some((item) => mediaRangeMatches(item, candidate))) return candidate;
  const exact = allowed.filter((item) => !item.includes("*"));
  if (selected === undefined && exact.length === 1) return exact[0]!;
  throw new TypeError(
    `multipart part content type ${candidate} is not permitted by ${declared}; select one with RequestOptions.multipartContentTypes`,
  );
}

function mediaRangeMatches(range: string, value: string): boolean {
  const [rangeType, rangeSubtype] = range.split("/", 2);
  const [valueType, valueSubtype] = value.split("/", 2);
  return (
    (rangeType === "*" || rangeType === valueType) &&
    (rangeSubtype === "*" || rangeSubtype === valueSubtype)
  );
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
    const values =
      Array.isArray(fieldValue) && definition?.explode !== false ? fieldValue : [fieldValue];
    for (const item of values) {
      const propertySchema = schema?.properties?.[name]?.schema ?? {};
      const { body, contentType, filename } = await multipartPartValue(
        item,
        definition?.contentType,
        propertySchema,
        definition,
        schemas,
        codecs,
      );
      const headers = await multipartPartHeaders(
        name,
        definition,
        suppliedHeaders?.[name],
        contentType,
        filename,
        schemas,
        codecs,
      );
      chunks.push(`--${boundary}\r\n${headers}\r\n\r\n`, body, "\r\n");
    }
  }
  chunks.push(`--${boundary}--\r\n`);
  return new Blob(chunks, { type: `multipart/form-data; boundary=${boundary}` });
}

async function multipartPartValue(
  value: unknown,
  declaredContentType: string | undefined,
  schema: WireSchema = {},
  definition: WireEncodingDefinition | undefined = undefined,
  schemas: WireSchemas = {},
  codecs: ReadonlyMap<string, MediaCodec<unknown>> = new Map(),
): Promise<{ body: BlobPart; contentType?: string; filename?: string }> {
  if (
    declaredContentType !== undefined &&
    normalizeMediaType(declaredContentType).startsWith("multipart/")
  ) {
    if (!Array.isArray(value)) throw new TypeError("nested multipart part must be an array");
    const nested = await encodePositionalMultipartBody(
      declaredContentType,
      value,
      definition?.prefixEncoding,
      definition?.itemEncoding,
      undefined,
      undefined,
      schema,
      schemas,
      codecs,
    );
    return { body: nested, contentType: nested.type };
  }
  if (value instanceof Blob) {
    const file = typeof File !== "undefined" && value instanceof File ? value : undefined;
    const contentType = (declaredContentType ?? value.type) || undefined;
    return {
      body: value,
      ...(contentType === undefined ? {} : { contentType }),
      ...(file === undefined ? {} : { filename: file.name }),
    };
  }
  if (value instanceof ArrayBuffer)
    return { body: value, contentType: declaredContentType ?? "application/octet-stream" };
  if (ArrayBuffer.isView(value)) {
    const bytes = new Uint8Array(value.byteLength);
    bytes.set(new Uint8Array(value.buffer, value.byteOffset, value.byteLength));
    return { body: bytes.buffer, contentType: declaredContentType ?? "application/octet-stream" };
  }
  if (declaredContentType !== undefined && requiresMultipartPartCodec(declaredContentType)) {
    const codec = codecs.get(normalizeMediaType(declaredContentType));
    if (codec?.encode === undefined)
      throw new TypeError(`missing encode codec for multipart part ${declaredContentType}`);
    return {
      body: multipartCodecBody(await codec.encode(value, { contentType: declaredContentType })),
      contentType: declaredContentType,
    };
  }
  if (declaredContentType !== undefined && isXMLMediaType(declaredContentType)) {
    return { body: encodeXML(value, schema, schemas), contentType: declaredContentType };
  }
  if (isRecord(value) || Array.isArray(value)) {
    return { body: JSON.stringify(value), contentType: declaredContentType ?? "application/json" };
  }
  return {
    body: String(value),
    ...(declaredContentType === undefined ? {} : { contentType: declaredContentType }),
  };
}

function requiresMultipartPartCodec(contentType: string): boolean {
  const normalized = normalizeMediaType(contentType);
  return (
    !isJSONMediaType(normalized) &&
    !isXMLMediaType(normalized) &&
    !normalized.startsWith("text/") &&
    normalized !== "application/x-www-form-urlencoded" &&
    !isBinaryMediaType(normalized)
  );
}

function multipartCodecBody(value: BodyInit): BlobPart {
  if (typeof value === "string" || value instanceof Blob || value instanceof ArrayBuffer)
    return value;
  if (ArrayBuffer.isView(value)) return value;
  if (value instanceof URLSearchParams) return value.toString();
  throw new TypeError(
    "multipart part codec must return a string, Blob, ArrayBuffer, ArrayBufferView, or URLSearchParams",
  );
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
  const declared = new Map(
    (definition?.headers ?? []).map((header) => [header.name.toLowerCase(), header]),
  );
  for (const header of definition?.headers ?? []) {
    const value = headers.get(header.name);
    if (value === null && header.required)
      throw new TypeError(`missing required multipart header ${name}.${header.name}`);
    if (value !== null) await validateMultipartHeaderValue(name, header, value, schemas, codecs);
  }
  for (const [headerName] of headers) {
    const normalized = headerName.toLowerCase();
    if (
      normalized === "content-type" ||
      (name !== undefined && normalized === "content-disposition") ||
      !declared.has(normalized)
    ) {
      throw new TypeError(
        `multipart header ${name ?? "position"}.${headerName} is not declared by the Encoding Object`,
      );
    }
  }
  const lines =
    name === undefined
      ? []
      : [
          `Content-Disposition: form-data; name="${escapeMultipartToken(name)}"${filename === undefined ? "" : `; filename="${escapeMultipartToken(filename)}"`}`,
        ];
  if (contentType !== undefined && contentType !== "") lines.push(`Content-Type: ${contentType}`);
  for (const [headerName, value] of headers) lines.push(`${headerName}: ${value}`);
  return lines.join("\r\n");
}

async function validateMultipartHeaderValue(
  part: string | undefined,
  header: WireMultipartHeaderDefinition,
  value: string,
  schemas: WireSchemas,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<void> {
  const decoded = await decodeMultipartHeaderValue(
    `${part ?? "position"}.${header.name}`,
    value,
    header.schema,
    header.contentType,
    header.explode,
    schemas,
    codecs,
  );
  validateWireValue(decoded, header.schema, schemas, "decode");
}

async function decodeMultipartHeaderValue(
  name: string,
  value: string,
  schema: WireSchema,
  contentType: string | undefined,
  explode: boolean | undefined,
  schemas: WireSchemas,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<unknown> {
  if (contentType !== undefined) {
    const decoded = decodeHeaderContent(name, value, contentType);
    if (
      isJSONMediaType(contentType) ||
      contentType.toLowerCase() === "application/x-www-form-urlencoded"
    )
      return decoded;
    if (isXMLMediaType(contentType)) return decodeXML(value, schema, schemas);
    if (!contentType.toLowerCase().startsWith("text/")) {
      const codec = codecs.get(normalizeMediaType(contentType));
      if (codec?.decodeParameter === undefined)
        throw new TypeError(`missing decodeParameter codec for multipart header ${name}`);
      return codec.decodeParameter(value, { contentType });
    }
    value = decoded as string;
  }
  if (schema.types?.includes("array"))
    return value
      .split(",")
      .map((item) => decodeMultipartHeaderScalar(name, item, schema.items ?? {}));
  if (schema.types?.includes("object") || schema.properties !== undefined) {
    const result = Object.create(null) as Record<string, unknown>;
    const tokens = value.split(",");
    if (explode)
      for (const token of tokens) {
        const separator = token.indexOf("=");
        if (separator < 0) continue;
        const property = token.slice(0, separator);
        defineOwnDataProperty(
          result,
          property,
          decodeMultipartHeaderScalar(
            name,
            token.slice(separator + 1),
            schema.properties?.[property]?.schema ?? {},
          ),
        );
      }
    else
      for (let index = 0; index + 1 < tokens.length; index += 2)
        defineOwnDataProperty(
          result,
          tokens[index]!,
          decodeMultipartHeaderScalar(
            name,
            tokens[index + 1]!,
            schema.properties?.[tokens[index]!]?.schema ?? {},
          ),
        );
    return result;
  }
  return decodeMultipartHeaderScalar(name, value, schema);
}

function decodeMultipartHeaderScalar(name: string, value: string, schema: WireSchema): unknown {
  if (schema.types?.includes("integer")) {
    const parsed = Number(value);
    if (!Number.isInteger(parsed))
      throw new TypeError(`multipart header ${name} is not an integer`);
    return parsed;
  }
  if (schema.types?.includes("number")) {
    const parsed = Number(value);
    if (!Number.isFinite(parsed)) throw new TypeError(`multipart header ${name} is not a number`);
    return parsed;
  }
  if (schema.types?.includes("boolean")) {
    if (value === "true") return true;
    if (value === "false") return false;
    throw new TypeError(`multipart header ${name} is not a boolean`);
  }
  return value;
}

function escapeMultipartToken(value: string): string {
  if (/\r|\n/.test(value))
    throw new TypeError("multipart names and filenames cannot contain line breaks");
  return value.replaceAll("\\", "\\\\").replaceAll('"', '\\"');
}

function multipartBoundaryToken(): string {
  const random = globalThis.crypto?.randomUUID?.();
  return random === undefined ? `${Date.now()}-${Math.random().toString(16).slice(2)}` : random;
}

function formEntries(
  value: Record<string, unknown>,
  encoding: readonly WireEncodingDefinition[] | undefined,
): readonly [string, string][] {
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
      const entries = Object.entries(item).filter(
        (entry): entry is [string, unknown] => entry[1] !== undefined,
      );
      if (explode) for (const [key, entry] of entries) result.push([key, String(entry)]);
      else result.push([name, entries.flatMap(([key, entry]) => [key, String(entry)]).join(",")]);
      continue;
    }
    result.push([name, String(item)]);
  }
  return result;
}

/** Encodes a validated wire value using the OpenAPI XML Object rules. */
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
  if (parameter?.sort !== undefined && Array.isArray(value)) {
    value = value.map((entry) => {
      if (
        !isRecord(entry) ||
        typeof entry.field !== "string" ||
        typeof entry.direction !== "string"
      ) {
        throw new TypeError(`Invalid structured sort value for ${parameter.name}`);
      }
      const wire = parameter.sort?.[`${entry.field}\u0000${entry.direction}`];
      if (wire === undefined)
        throw new TypeError(`Invalid structured sort value for ${parameter.name}`);
      return wire;
    });
  }
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
  if (
    definition !== undefined &&
    isXMLMediaType(definition.contentType) &&
    typeof value === "string"
  ) {
    value = decodeXML(value, definition.schema, operation.outputSchemas ?? {});
  }
  return definition === undefined
    ? value
    : transformWireValue(value, definition.schema, operation.outputSchemas ?? {}, "decode");
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
      return contentType === undefined
        ? item.contentType === ""
        : mediaTypeMatches(item.contentType, contentType);
    })
    .sort((left, right) => {
      const statusDifference =
        statusMatchScore(right.status, response.status) -
        statusMatchScore(left.status, response.status);
      if (statusDifference !== 0) return statusDifference;
      return (
        mediaTypeMatchScore(right.contentType, contentType) -
        mediaTypeMatchScore(left.contentType, contentType)
      );
    })[0];
}

function selectRequestBodyDefinition(
  bodies: readonly WireBodyDefinition[],
  contentType: string,
): WireBodyDefinition | undefined {
  return bodies
    .filter((body) => mediaTypeMatches(body.contentType, contentType))
    .sort(
      (left, right) =>
        mediaTypeMatchScore(right.contentType, contentType) -
        mediaTypeMatchScore(left.contentType, contentType),
    )[0];
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
  if (
    expectedType === undefined ||
    expectedSubtype === undefined ||
    receivedType === undefined ||
    receivedSubtype === undefined
  )
    return false;
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
  location: "query" | "querystring",
): Promise<QueryPart[]> {
  const result: QueryPart[] = [];
  for (const [property, value] of Object.entries(query)) {
    if (value === undefined) continue;
    const parameter = findParameterByProperty(operation, location, property);
    if (parameter?.location === "querystring") {
      await appendQuerystring(
        result,
        encodeParameterWireValue(operation, parameter, value),
        parameter,
        operation.inputSchemas ?? {},
        codecs,
      );
      continue;
    }
    await appendQueryParameter(
      result,
      parameter?.name ?? property,
      encodeParameterWireValue(operation, parameter, value),
      parameter,
      operation.inputSchemas ?? {},
      codecs,
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
    appendQueryValue(
      query,
      name,
      await serializeContentParameter(
        value,
        parameter.contentType,
        parameter.schema,
        components,
        codecs,
      ),
      parameter?.allowReserved ?? false,
    );
    return;
  }
  const style = parameter?.style ?? "form";
  const explode = parameter?.explode ?? true;
  if (style === "deepObject" && isRecord(value)) {
    for (const [key, item] of Object.entries(value)) {
      if (item !== undefined)
        appendQueryValue(query, `${name}[${key}]`, item, parameter?.allowReserved ?? false);
    }
    return;
  }
  if (Array.isArray(value)) {
    if (style === "spaceDelimited")
      appendQueryValue(query, name, value.map(String).join(" "), parameter?.allowReserved ?? false);
    else if (style === "pipeDelimited")
      appendQueryValue(query, name, value.map(String).join("|"), parameter?.allowReserved ?? false);
    else if (explode)
      for (const item of value)
        appendQueryValue(query, name, item, parameter?.allowReserved ?? false);
    else
      appendQueryValue(query, name, value.map(String).join(","), parameter?.allowReserved ?? false);
    return;
  }
  if (isRecord(value) && style === "form") {
    const entries = Object.entries(value).filter((entry) => entry[1] !== undefined);
    if (explode)
      for (const [key, item] of entries)
        appendQueryValue(query, key, item, parameter?.allowReserved ?? false);
    else
      appendQueryValue(
        query,
        name,
        entries.flatMap(([key, item]) => [key, String(item)]).join(","),
        parameter?.allowReserved ?? false,
      );
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

function appendQuerySync(
  query: Readonly<Record<string, unknown>>,
  operation: OperationDefinition,
  location: "query" | "querystring",
): QueryPart[] {
  const result: QueryPart[] = [];
  for (const [property, rawValue] of Object.entries(query)) {
    if (rawValue === undefined) continue;
    const parameter = findParameterByProperty(operation, location, property);
    const value = encodeParameterWireValue(operation, parameter, rawValue);
    if (parameter?.location === "querystring") {
      appendQuerystringSync(result, value, parameter, operation.inputSchemas ?? {});
      continue;
    }
    const name = parameter?.name ?? property;
    if (parameter?.contentType !== undefined) {
      appendQueryValue(
        result,
        name,
        serializeContentParameterSync(
          value,
          parameter.contentType,
          parameter.schema,
          operation.inputSchemas ?? {},
        ),
        parameter.allowReserved ?? false,
      );
      continue;
    }
    const style = parameter?.style ?? "form";
    const explode = parameter?.explode ?? true;
    if (style === "deepObject" && isRecord(value)) {
      for (const [key, item] of Object.entries(value))
        if (item !== undefined)
          appendQueryValue(result, `${name}[${key}]`, item, parameter?.allowReserved ?? false);
      continue;
    }
    if (Array.isArray(value)) {
      if (style === "spaceDelimited")
        appendQueryValue(
          result,
          name,
          value.map(String).join(" "),
          parameter?.allowReserved ?? false,
        );
      else if (style === "pipeDelimited")
        appendQueryValue(
          result,
          name,
          value.map(String).join("|"),
          parameter?.allowReserved ?? false,
        );
      else if (explode)
        for (const item of value)
          appendQueryValue(result, name, item, parameter?.allowReserved ?? false);
      else
        appendQueryValue(
          result,
          name,
          value.map(String).join(","),
          parameter?.allowReserved ?? false,
        );
      continue;
    }
    if (isRecord(value) && style === "form") {
      const entries = Object.entries(value).filter((entry) => entry[1] !== undefined);
      if (explode)
        for (const [key, item] of entries)
          appendQueryValue(result, key, item, parameter?.allowReserved ?? false);
      else
        appendQueryValue(
          result,
          name,
          entries.flatMap(([key, item]) => [key, String(item)]).join(","),
          parameter?.allowReserved ?? false,
        );
      continue;
    }
    if (isRecord(value) && (style === "spaceDelimited" || style === "pipeDelimited")) {
      const separator = style === "spaceDelimited" ? " " : "|";
      const entries = Object.entries(value).filter((entry) => entry[1] !== undefined);
      appendQueryValue(
        result,
        name,
        explode
          ? entries.map(([key, item]) => `${key}=${String(item)}`).join(separator)
          : entries.flatMap(([key, item]) => [key, String(item)]).join(separator),
        parameter?.allowReserved ?? false,
      );
      continue;
    }
    appendQueryValue(result, name, value, parameter?.allowReserved ?? false);
  }
  return result;
}

function appendQuerystringSync(
  query: QueryPart[],
  value: unknown,
  parameter: ParameterDefinition,
  components: WireSchemas,
): void {
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
  query.push({
    raw: encodeURIComponent(
      serializeContentParameterSync(
        value,
        parameter.contentType ?? "text/plain",
        parameter.schema,
        components,
      ),
    ),
  });
}

function appendQueryValue(
  query: QueryPart[],
  name: string,
  value: unknown,
  allowReserved: boolean,
): void {
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
  return query
    .map(
      (part) =>
        part.raw ??
        `${encodeURIComponent(part.name ?? "")}=${part.allowReserved ? encodeReservedQueryValue(part.value ?? "") : encodeURIComponent(part.value ?? "")}`,
    )
    .join("&");
}

async function appendQuerystring(
  query: QueryPart[],
  value: unknown,
  parameter: ParameterDefinition,
  components: WireSchemas,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<void> {
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
  query.push({
    raw: encodeURIComponent(
      await serializeContentParameter(
        value,
        parameter.contentType ?? "text/plain",
        parameter.schema,
        components,
        codecs,
      ),
    ),
  });
}

function encodeReservedQueryValue(value: string): string {
  return encodeURIComponent(value)
    .replace(/%25([0-9a-f]{2})/gi, "%$1")
    .replace(/%3A|%2F|%3F|%40|%21|%24|%27|%28|%29|%2A|%2C|%3B|%3D/gi, (encoded) =>
      decodeURIComponent(encoded),
    );
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
    return encodeURIComponent(
      await serializeContentParameter(
        value,
        parameter.contentType,
        parameter.schema,
        components,
        codecs,
      ),
    );
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
      .filter((entry) => entry[1] !== undefined)
      .map(([key, item]) => `;${encodeURIComponent(key)}=${encodeURIComponent(String(item))}`)
      .join("");
  }
  return `;${encodeURIComponent(name)}=${encoded}`;
}

function serializePathParameterSync(
  parameter: ParameterDefinition | undefined,
  name: string,
  value: unknown,
  components: WireSchemas,
): string {
  if (parameter?.contentType !== undefined)
    return encodeURIComponent(
      serializeContentParameterSync(value, parameter.contentType, parameter.schema, components),
    );
  const style = parameter?.style ?? "simple";
  const explode = parameter?.explode ?? false;
  const encoded = serializePathValue(value, explode, style === "label" ? "." : ",");
  if (style === "label") return `.${encoded}`;
  if (style !== "matrix") return encoded;
  if (Array.isArray(value) && explode)
    return value
      .map((item) => `;${encodeURIComponent(name)}=${encodeURIComponent(String(item))}`)
      .join("");
  if (isRecord(value) && explode)
    return Object.entries(value)
      .filter((entry) => entry[1] !== undefined)
      .map(([key, item]) => `;${encodeURIComponent(key)}=${encodeURIComponent(String(item))}`)
      .join("");
  return `;${encodeURIComponent(name)}=${encoded}`;
}

function serializePathValue(value: unknown, explode: boolean, arraySeparator: string): string {
  if (Array.isArray(value))
    return value
      .map((item) => encodeURIComponent(String(item)))
      .join(explode ? arraySeparator : ",");
  if (isRecord(value)) {
    return Object.entries(value)
      .filter((entry) => entry[1] !== undefined)
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
      .filter((entry) => entry[1] !== undefined)
      .flatMap(([key, item]) => (explode ? `${key}=${String(item)}` : [key, String(item)]))
      .join(",");
  }
  return String(value);
}

async function serializeContentParameter(
  value: unknown,
  contentType: string,
  schema: WireSchema | undefined = undefined,
  components: WireSchemas = {},
  codecs: ReadonlyMap<string, MediaCodec<unknown>> = new Map(),
): Promise<string> {
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
  if (codec?.encodeParameter === undefined)
    throw new TypeError(`missing parameter encode codec for ${contentType}`);
  return await codec.encodeParameter(value, { contentType });
}

function serializeContentParameterSync(
  value: unknown,
  contentType: string,
  schema: WireSchema | undefined = undefined,
  components: WireSchemas = {},
): string {
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
  const pair = (key: string, item: unknown): string =>
    `${preserve ? key : encodeURIComponent(key)}=${preserve ? String(item ?? "") : encodeURIComponent(String(item ?? ""))}`;
  value = encodeParameterWireValue(operation, parameter, value);
  if (parameter?.contentType !== undefined) {
    return [
      pair(
        name,
        await serializeContentParameter(
          value,
          parameter.contentType,
          parameter.schema,
          operation.inputSchemas ?? {},
          codecs,
        ),
      ),
    ];
  }
  if (Array.isArray(value)) {
    if (parameter?.explode ?? true) {
      return value.map((item) => pair(name, item));
    }
    return [pair(name, value.map(String).join(","))];
  }
  if (isRecord(value) && (parameter?.explode ?? true)) {
    return Object.entries(value)
      .filter((entry) => entry[1] !== undefined)
      .map(([key, item]) => pair(key, item));
  }
  return [pair(name, serializeSimpleValue(value, false))];
}

function serializeCookieSync(
  operation: OperationDefinition,
  property: string,
  value: unknown,
): string[] {
  const parameter = findParameterByProperty(operation, "cookie", property);
  const name = parameter?.name ?? property;
  const preserve = parameter?.style === "cookie";
  const pair = (key: string, item: unknown): string =>
    `${preserve ? key : encodeURIComponent(key)}=${preserve ? String(item ?? "") : encodeURIComponent(String(item ?? ""))}`;
  value = encodeParameterWireValue(operation, parameter, value);
  if (parameter?.contentType !== undefined)
    return [
      pair(
        name,
        serializeContentParameterSync(
          value,
          parameter.contentType,
          parameter.schema,
          operation.inputSchemas ?? {},
        ),
      ),
    ];
  if (Array.isArray(value))
    return (parameter?.explode ?? true)
      ? value.map((item) => pair(name, item))
      : [pair(name, value.map(String).join(","))];
  if (isRecord(value) && (parameter?.explode ?? true))
    return Object.entries(value)
      .filter((entry) => entry[1] !== undefined)
      .map(([key, item]) => pair(key, item));
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
  const server =
    selection?.id === undefined ? servers[0] : servers.find((item) => item.id === selection.id);
  if (server === undefined)
    throw new TypeError(
      `Unknown server ${selection?.id} for operation ${operationDiagnosticName(operation)}`,
    );
  const variables = selection?.variables ?? {};
  const expanded = server.url.replace(/\{([^}]+)\}/g, (_, name: string) => {
    const definition = server.variables?.find((item) => item.name === name);
    if (definition === undefined)
      throw new TypeError(`Server ${server.id} has no variable ${name}`);
    const value = variables[name] ?? definition.defaultValue;
    if (definition.enumValues !== undefined && !definition.enumValues.includes(value)) {
      throw new TypeError(
        `Server variable ${name} must be one of ${definition.enumValues.join(", ")}`,
      );
    }
    return encodeURIComponent(value);
  });
  try {
    return normalizeBaseURL(expanded);
  } catch {
    if (origin === undefined)
      throw new TypeError(`Server ${server.id} is relative; pass ClientOptions.origin or baseURL`);
    const absoluteOrigin = normalizeOrigin(origin);
    return normalizeBaseURL(new URL(expanded, absoluteOrigin).href);
  }
}

function normalizeOrigin(value: string): string {
  const url = new URL(value);
  if (
    (url.protocol !== "http:" && url.protocol !== "https:") ||
    url.pathname !== "/" ||
    url.search ||
    url.hash
  ) {
    throw new TypeError(
      "origin must be an absolute http(s) origin without path, query, or fragment",
    );
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

async function decodeResponse(
  operation: OperationDefinition,
  response: Response,
  request: RequestMetadata,
  codecs: ReadonlyMap<string, MediaCodec<unknown>>,
): Promise<unknown> {
  if (response.status === 204 || response.status === 205) return undefined;
  const contentType = responseContentType(response);
  if (contentType === undefined || response.body === null) return undefined;
  try {
    const definition = selectResponseDefinition(operation, response, true);
    if (normalizeMediaType(contentType).startsWith("multipart/") && definition !== undefined) {
      return decodeMultipartResponse(
        response.body,
        response.headers.get("content-type") ?? contentType,
        definition,
        operation.outputSchemas ?? {},
        codecs,
      );
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

function responseContentType(response: Response): string | undefined {
  return response.headers.get("content-type")?.split(";", 1)[0]?.trim().toLowerCase();
}

function normalizeCodecs(
  codecs: Readonly<Record<string, MediaCodec<unknown>>> | undefined,
): ReadonlyMap<string, MediaCodec<unknown>> {
  const result = new Map<string, MediaCodec<unknown>>();
  for (const [contentType, codec] of Object.entries(codecs ?? {})) {
    const normalized = normalizeMediaType(contentType);
    if (normalized === "" || result.has(normalized))
      throw new TypeError(`duplicate or invalid media codec ${contentType}`);
    result.set(normalized, codec);
  }
  return result;
}

function normalizeMediaType(contentType: string): string {
  return contentType.split(";", 1)[0]?.trim().toLowerCase() ?? "";
}

function isBinaryMediaType(contentType: string): boolean {
  return (
    contentType === "application/octet-stream" ||
    contentType.startsWith("image/") ||
    contentType.startsWith("audio/") ||
    contentType.startsWith("video/")
  );
}

function isSequentialStreamMediaType(contentType: string): boolean {
  return (
    contentType.includes("event-stream") ||
    contentType.includes("json-seq") ||
    contentType.includes("ndjson") ||
    contentType.includes("jsonl")
  );
}

function isPromise<Value>(value: Value | Promise<Value>): value is Promise<Value> {
  return typeof (value as Promise<Value>)?.then === "function";
}

function isAsyncIterable(value: unknown): value is AsyncIterable<unknown> {
  return (
    value !== null &&
    typeof value === "object" &&
    typeof (value as AsyncIterable<unknown>)[Symbol.asyncIterator] === "function"
  );
}

function isReadableStream(value: unknown): value is ReadableStream<Uint8Array> {
  return (
    value !== null &&
    typeof value === "object" &&
    typeof (value as ReadableStream<Uint8Array>).getReader === "function"
  );
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
      ...(cause.status === undefined ? {} : { status: cause.status }),
      ...(cause.response === undefined ? {} : { response: cause.response }),
    });
  }
  if (responseMetadata !== undefined) {
    return new APIError({ code, message, cause, ...responseMetadata });
  }
  return transportError(code, message, cause);
}

function awaitAbortable<Value>(
  value: Promise<Value>,
  signal: AbortSignal | undefined,
): Promise<Value> {
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
