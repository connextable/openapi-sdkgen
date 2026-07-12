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
};
/**
 * Normalized error thrown for server-declared errors and SDK transport failures.
 *
 * Use generated error guards or {@link isErrorCode} instead of matching messages.
 */
export class APIError extends Error {
    /** Standard JavaScript error name. */
    name = "APIError";
    /** Stable server or transport error code. */
    code;
    /** Metadata for the request that produced the error. */
    request;
    /** HTTP status code, absent when no response was received. */
    status;
    /** Structured server validation or domain-error details. */
    details;
    /** Legacy structured server validation fields. */
    fields;
    /** Original Fetch API response, when available. */
    response;
    /** Original exception, when this error wraps another failure. */
    cause;
    /** Creates a normalized API or transport error. */
    constructor(options) {
        super(options.message);
        this.code = options.code;
        this.request = options.request ?? {};
        if (options.status !== undefined)
            this.status = options.status;
        if (options.details !== undefined)
            this.details = options.details;
        if (options.fields !== undefined)
            this.fields = options.fields;
        if (options.response !== undefined)
            this.response = options.response;
        if (options.cause !== undefined)
            this.cause = options.cause;
    }
}
/**
 * Checks whether a value is an {@link APIError} created by this SDK runtime.
 *
 * @param error Value caught from an SDK call.
 * @returns `true` when `error` is an {@link APIError}.
 */
export function isAPIError(error) {
    return error instanceof APIError;
}
/**
 * Narrows an unknown error to an {@link APIError} with an exact code.
 *
 * @param error Value caught from an SDK call.
 * @param code Server or transport code to match.
 * @returns `true` when both the error type and code match.
 */
export function isErrorCode(error, code) {
    return isAPIError(error) && error.code === code;
}
/**
 * Reads a stable error code without requiring a type guard branch.
 *
 * @param error Value caught from an SDK call.
 * @returns Error code for an {@link APIError}; otherwise `undefined`.
 */
export function getErrorCode(error) {
    return isAPIError(error) ? error.code : undefined;
}
/**
 * Reads the server request ID attached to an SDK error.
 *
 * @param error Value caught from an SDK call.
 * @returns Server request ID when available; otherwise `undefined`.
 */
export function getRequestID(error) {
    return isAPIError(error) ? error.request.id : undefined;
}
/**
 * Binds generated operation metadata to a low-level request executor.
 *
 * Used by generated clients; applications normally call the generated operation instead.
 */
export function bindOperation(request, operation, hasInput) {
    const call = hasInput
        ? (input, options) => request(operation, input, options)
        : (options) => request(operation, undefined, options);
    const raw = hasInput
        ? (input, options) => request.raw(operation, input, options)
        : (options) => request.raw(operation, undefined, options);
    return Object.assign(call, { raw });
}
/**
 * Binds resource path parameters to an input operation.
 *
 * Used to implement generated instance builders such as `api.products(productID)`.
 */
export function bindPathOperation(operation, path, hasInput) {
    const mergeInput = (input) => ({
        ...(isRecord(input) ? input : {}),
        path,
    });
    const call = hasInput
        ? (input, options) => operation(mergeInput(input), options)
        : (options) => operation(mergeInput(undefined), options);
    const raw = hasInput
        ? (input, options) => operation.raw(mergeInput(input), options)
        : (options) => operation.raw(mergeInput(undefined), options);
    return Object.assign(call, { raw });
}
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
export function createPaginator(requestPage, profile) {
    return (input, options) => ({
        async *[Symbol.asyncIterator]() {
            const root = isRecord(input) ? { ...input } : {};
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
            const seenCursors = new Set();
            if (typeof query.cursor === "string")
                seenCursors.add(query.cursor);
            for (;;) {
                const page = await requestPage({ ...root, query: { ...query } }, options);
                const items = pageItems(page);
                for (const item of items)
                    yield item;
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
                if (limit <= 0 ||
                    items.length === 0 ||
                    items.length < limit ||
                    (total !== undefined && nextOffset >= total))
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
export function createRequest(options) {
    const baseURL = normalizeBaseURL(options.baseURL);
    const fetchImplementation = options.fetch ?? globalThis.fetch;
    if (typeof fetchImplementation !== "function") {
        throw new TypeError("fetch is unavailable; pass ClientOptions.fetch");
    }
    const execute = async (operation, input, requestOptions = {}, raw = false) => {
        let encoded;
        try {
            encoded = encodeRequest(baseURL, options, operation, input, requestOptions);
        }
        catch (cause) {
            if (isAPIError(cause)) {
                throw cause;
            }
            throw transportError(TransportErrorCode.REQUEST_ENCODE_FAILED, `Failed to encode ${operation.operationID} request`, cause);
        }
        const timeoutMS = requestOptions.timeoutMS ?? options.timeoutMS;
        const abort = createAbortContext(requestOptions.signal, timeoutMS);
        let responseMetadata;
        try {
            const init = {
                method: operation.method,
                headers: encoded.headers,
            };
            if (encoded.body !== undefined)
                init.body = encoded.body;
            if (abort.signal !== undefined)
                init.signal = abort.signal;
            const credentials = requestOptions.credentials ?? options.credentials;
            if (credentials !== undefined)
                init.credentials = credentials;
            if (abort.signal?.aborted)
                throw abort.signal.reason;
            const response = await awaitAbortable(fetchImplementation(encoded.url, init), abort.signal);
            const request = requestMetadata(response);
            responseMetadata = { request, status: response.status, response };
            const decodedBody = await awaitAbortable(decodeResponse(response, request), abort.signal);
            const body = decodeResponseWireValue(operation, response, decodedBody);
            if (!response.ok) {
                throw serverError(response, request, body);
            }
            const data = operation.envelope === "data" && isRecord(body) && "data" in body
                ? body.data
                : body;
            if (!raw)
                return data;
            const contentType = responseContentType(response);
            return {
                status: response.status,
                ...(contentType === undefined ? {} : { contentType }),
                data,
                headers: response.headers,
                request,
                response,
            };
        }
        catch (cause) {
            if (abort.timedOut()) {
                throw transportErrorFromCause(TransportErrorCode.REQUEST_TIMEOUT, `Request timed out after ${timeoutMS}ms`, cause, responseMetadata);
            }
            if (abort.aborted()) {
                throw transportErrorFromCause(TransportErrorCode.REQUEST_ABORTED, "Request was aborted", cause, responseMetadata);
            }
            if (isAPIError(cause))
                throw cause;
            throw transportError(TransportErrorCode.NETWORK_ERROR, "Network request failed", cause);
        }
        finally {
            abort.cleanup();
        }
    };
    const request = ((operation, input, requestOptions) => execute(operation, input, requestOptions, false));
    request.raw = (operation, input, requestOptions) => execute(operation, input, requestOptions, true);
    return request;
}
function encodeRequest(baseURL, client, operation, input, options) {
    const values = isRecord(input) ? input : {};
    const pathValues = isRecord(values.path) ? values.path : {};
    rejectUndefinedArrayValues(pathValues);
    const path = operation.path.replaceAll(/\{([^}]+)\}/g, (_, name) => {
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
    const queryValues = isRecord(values.query) ? values.query : {};
    rejectUndefinedArrayValues(queryValues);
    appendQuery(url.searchParams, queryValues, operation);
    const contractHeaderNames = new Set([
        ...(operation.headerNames ?? []),
        ...(operation.parameters ?? [])
            .filter((parameter) => parameter.location === "header")
            .map((parameter) => parameter.name),
    ].map((name) => name.toLowerCase()));
    const headers = new Headers();
    appendRawHeaders(headers, client.headers, contractHeaderNames);
    appendRawHeaders(headers, options.headers, contractHeaderNames);
    const headerParams = {
        ...(isRecord(values.headerParams) ? values.headerParams : {}),
    };
    rejectUndefinedArrayValues(headerParams);
    for (const [property, value] of Object.entries(headerParams)) {
        if (value === undefined)
            continue;
        const parameter = findParameterByProperty(operation, "header", property);
        const name = parameter?.name ?? property;
        const encodedValue = encodeParameterWireValue(operation, parameter, value);
        headers.set(name, parameter?.contentType === undefined
            ? serializeSimpleValue(encodedValue, parameter?.explode ?? false)
            : serializeContentParameter(encodedValue, parameter.contentType));
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
    const cookies = Object.entries(cookieValues)
        .filter((entry) => entry[1] !== undefined)
        .flatMap(([property, value]) => serializeCookie(operation, property, value));
    if (cookies.length > 0)
        headers.set("Cookie", cookies.join("; "));
    if (!("body" in values) || values.body === undefined) {
        if (operation.requestBodyRequired)
            throw new TypeError("Missing required request body");
        return { url: url.href, headers };
    }
    rejectUndefinedArrayValues(values.body);
    let contentType = operation.contentType ?? "application/json";
    let bodyValue = values.body;
    const suppliedBody = values.body;
    if (operation.requestBodies !== undefined &&
        operation.requestBodies.length > 1 &&
        isRecord(suppliedBody) &&
        typeof suppliedBody.contentType === "string" &&
        "value" in suppliedBody &&
        operation.requestBodies.some((body) => body.contentType === suppliedBody.contentType)) {
        contentType = suppliedBody.contentType;
        bodyValue = suppliedBody.value;
    }
    const encodedBody = encodeRequestWireValue(operation, contentType, bodyValue);
    const body = encodeRequestBody(contentType, encodedBody);
    if (!(body instanceof FormData))
        headers.set("Content-Type", contentType);
    return { url: url.href, headers, body };
}
function assertRequiredParameters(operation, pathValues, queryValues, headerValues, cookieValues) {
    for (const parameter of operation.parameters ?? []) {
        if (!parameter.required)
            continue;
        const values = parameter.location === "path" ? pathValues
            : parameter.location === "query" ? queryValues
                : parameter.location === "header" ? headerValues
                    : cookieValues;
        if (values[parameter.property] === undefined || values[parameter.property] === null) {
            throw new TypeError(`Missing required ${parameter.location} parameter ${parameter.name}`);
        }
    }
}
function encodeRequestBody(contentType, value) {
    const normalizedContentType = contentType.toLowerCase();
    if (isJSONMediaType(normalizedContentType))
        return JSON.stringify(value);
    if (normalizedContentType === "application/x-www-form-urlencoded") {
        if (!isRecord(value))
            throw new TypeError("form body must be an object");
        const form = new URLSearchParams();
        for (const [name, item] of Object.entries(value)) {
            if (item === undefined)
                continue;
            if (Array.isArray(item))
                for (const entry of item)
                    form.append(name, String(entry));
            else
                form.append(name, String(item));
        }
        return form;
    }
    if (normalizedContentType === "multipart/form-data") {
        if (!isRecord(value))
            throw new TypeError("multipart body must be an object");
        const form = new FormData();
        const append = (name, item) => {
            if (item instanceof Blob)
                form.append(name, item);
            else if (item instanceof ArrayBuffer)
                form.append(name, new Blob([item]));
            else if (ArrayBuffer.isView(item)) {
                const bytes = new Uint8Array(item.byteLength);
                bytes.set(new Uint8Array(item.buffer, item.byteOffset, item.byteLength));
                form.append(name, new Blob([bytes.buffer]));
            }
            else
                form.append(name, String(item));
        };
        for (const [name, item] of Object.entries(value)) {
            if (item === undefined)
                continue;
            if (Array.isArray(item))
                for (const entry of item)
                    append(name, entry);
            else
                append(name, item);
        }
        return form;
    }
    if (normalizedContentType.startsWith("text/") || normalizedContentType.includes("xml"))
        return String(value);
    if (value instanceof Blob || value instanceof ArrayBuffer || ArrayBuffer.isView(value)) {
        return value;
    }
    throw new TypeError(`unsupported body value for ${contentType}`);
}
function encodeRequestWireValue(operation, contentType, value) {
    const definition = operation.requestBodies?.find((item) => item.contentType === contentType);
    return definition === undefined
        ? value
        : transformWireValue(value, definition.schema, operation.inputSchemas ?? {}, "encode");
}
function encodeParameterWireValue(operation, parameter, value) {
    return parameter?.schema === undefined
        ? value
        : transformWireValue(value, parameter.schema, operation.inputSchemas ?? {}, "encode");
}
function decodeResponseWireValue(operation, response, value) {
    const contentType = responseContentType(response);
    const definition = operation.responses?.find((item) => statusMatches(item.status, response.status) &&
        (contentType === undefined || item.contentType.toLowerCase() === contentType));
    return definition === undefined
        ? value
        : transformWireValue(value, definition.schema, operation.outputSchemas ?? {}, "decode");
}
function statusMatches(pattern, status) {
    if (pattern === String(status) || pattern === "default")
        return true;
    return /^\dXX$/i.test(pattern) && Number(pattern[0]) === Math.floor(status / 100);
}
function transformWireValue(value, schema, components, direction) {
    if (value === null || value === undefined)
        return value;
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
    let transformed = value;
    if (isRecord(transformed) &&
        (schema.properties !== undefined || schema.additionalProperties !== undefined)) {
        const source = transformed;
        const result = { ...source };
        const known = new Set();
        for (const [wireName, propertyDefinition] of Object.entries(schema.properties ?? {})) {
            const sourceName = direction === "encode" ? propertyDefinition.property : wireName;
            const targetName = direction === "encode" ? wireName : propertyDefinition.property;
            known.add(sourceName);
            known.add(targetName);
            if (!(sourceName in source))
                continue;
            if (sourceName !== targetName)
                delete result[sourceName];
            result[targetName] = transformWireValue(source[sourceName], propertyDefinition.schema, components, direction);
        }
        if (schema.additionalProperties !== undefined) {
            for (const [key, item] of Object.entries(result)) {
                if (!known.has(key)) {
                    result[key] = transformWireValue(item, schema.additionalProperties, components, direction);
                }
            }
        }
        transformed = result;
    }
    for (const variants of [schema.allOf, schema.oneOf, schema.anyOf]) {
        if (variants === undefined)
            continue;
        for (const variant of variants) {
            transformed = transformWireValue(transformed, variant, components, direction);
        }
    }
    return transformed;
}
function appendQuery(search, query, operation) {
    for (const [property, value] of Object.entries(query)) {
        if (value === undefined)
            continue;
        const parameter = findParameterByProperty(operation, "query", property);
        appendQueryParameter(search, parameter?.name ?? property, encodeParameterWireValue(operation, parameter, value), parameter);
    }
}
function appendQueryParameter(search, name, value, parameter) {
    if (parameter?.contentType !== undefined) {
        appendQueryValue(search, name, serializeContentParameter(value, parameter.contentType));
        return;
    }
    const style = parameter?.style ?? "form";
    const explode = parameter?.explode ?? true;
    if (style === "deepObject" && isRecord(value)) {
        for (const [key, item] of Object.entries(value)) {
            if (item !== undefined)
                appendQueryValue(search, `${name}[${key}]`, item);
        }
        return;
    }
    if (Array.isArray(value)) {
        if (style === "spaceDelimited")
            search.append(name, value.map(String).join(" "));
        else if (style === "pipeDelimited")
            search.append(name, value.map(String).join("|"));
        else if (explode)
            for (const item of value)
                appendQueryValue(search, name, item);
        else
            search.append(name, value.map(String).join(","));
        return;
    }
    if (isRecord(value) && style === "form") {
        const entries = Object.entries(value).filter((entry) => entry[1] !== undefined);
        if (explode)
            for (const [key, item] of entries)
                appendQueryValue(search, key, item);
        else
            search.append(name, entries.flatMap(([key, item]) => [key, String(item)]).join(","));
        return;
    }
    appendQueryValue(search, name, value);
}
function appendQueryValue(search, name, value) {
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
function findParameter(operation, location, name) {
    return operation.parameters?.find((parameter) => parameter.location === location && parameter.name === name);
}
function findParameterByProperty(operation, location, property) {
    return operation.parameters?.find((parameter) => parameter.location === location && parameter.property === property);
}
function serializePathParameter(parameter, name, value) {
    if (parameter?.contentType !== undefined) {
        return encodeURIComponent(serializeContentParameter(value, parameter.contentType));
    }
    const style = parameter?.style ?? "simple";
    const explode = parameter?.explode ?? false;
    const encoded = serializePathValue(value, explode, style === "label" ? "." : ",");
    if (style === "label")
        return `.${encoded}`;
    if (style !== "matrix")
        return encoded;
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
function serializePathValue(value, explode, arraySeparator) {
    if (Array.isArray(value))
        return value
            .map((item) => encodeURIComponent(String(item)))
            .join(explode ? arraySeparator : ",");
    if (isRecord(value)) {
        return Object.entries(value)
            .flatMap(([key, item]) => explode
            ? `${encodeURIComponent(key)}=${encodeURIComponent(String(item))}`
            : [encodeURIComponent(key), encodeURIComponent(String(item))])
            .join(explode ? arraySeparator : ",");
    }
    return encodeURIComponent(String(value));
}
function serializeSimpleValue(value, explode) {
    if (Array.isArray(value))
        return value.map(String).join(",");
    if (isRecord(value)) {
        return Object.entries(value)
            .flatMap(([key, item]) => (explode ? `${key}=${String(item)}` : [key, String(item)]))
            .join(",");
    }
    return String(value);
}
function serializeContentParameter(value, contentType) {
    if (isJSONMediaType(contentType))
        return JSON.stringify(value);
    return String(value);
}
function serializeCookie(operation, property, value) {
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
        return Object.entries(value).map(([key, item]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(item))}`);
    }
    return [`${encodeURIComponent(name)}=${encodeURIComponent(serializeSimpleValue(value, false))}`];
}
function resolveOperationBaseURL(baseURL, serverURL) {
    if (serverURL === undefined)
        return baseURL;
    const base = new URL(baseURL);
    const server = new URL(serverURL, base.origin);
    return server.href.replace(/\/$/, "");
}
function appendRawHeaders(target, source, contractNames) {
    if (source === undefined)
        return;
    const incoming = new Headers(source);
    incoming.forEach((value, name) => {
        const lower = name.toLowerCase();
        if (reservedHeaders.has(lower) || contractNames.has(lower)) {
            throw new TypeError(`Raw header ${name} must use its typed option`);
        }
        target.set(name, value);
    });
}
function setHeader(headers, name, value) {
    if (value !== undefined)
        headers.set(name, value);
}
function rejectUndefinedArrayValues(value) {
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
        for (const item of Object.values(value))
            rejectUndefinedArrayValues(item);
    }
}
function normalizeBaseURL(value) {
    let url;
    try {
        url = new URL(value);
    }
    catch {
        throw new TypeError("baseURL must be an absolute URL");
    }
    if ((url.protocol !== "http:" && url.protocol !== "https:") || url.search || url.hash) {
        throw new TypeError("baseURL must be an absolute http(s) URL without query or fragment");
    }
    url.pathname = url.pathname.replace(/\/+$/, "");
    return url.href.replace(/\/$/, "");
}
function createAbortContext(signal, timeoutMS) {
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
    const forwardAbort = () => controller.abort(signal?.reason);
    if (signal?.aborted)
        forwardAbort();
    else
        signal?.addEventListener("abort", forwardAbort, { once: true });
    const timer = timeoutMS === undefined
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
            if (timer !== undefined)
                clearTimeout(timer);
            signal?.removeEventListener("abort", forwardAbort);
        },
    };
}
async function decodeResponse(response, request) {
    if (response.status === 204 || response.status === 205)
        return undefined;
    const contentType = responseContentType(response);
    if (contentType === undefined || response.body === null)
        return undefined;
    try {
        if (isJSONMediaType(contentType)) {
            return await response.json();
        }
        if (contentType.startsWith("text/") || contentType.includes("xml")) {
            return await response.text();
        }
        return response.body;
    }
    catch (cause) {
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
function isJSONMediaType(contentType) {
    const mediaType = contentType.split(";", 1)[0]?.trim().toLowerCase() ?? "";
    return mediaType === "application/json" || mediaType.endsWith("+json");
}
function responseContentType(response) {
    return response.headers.get("content-type")?.split(";", 1)[0]?.trim().toLowerCase();
}
function serverError(response, request, body) {
    const envelope = isRecord(body) && isRecord(body.error) ? body.error : body;
    const error = isRecord(envelope) ? envelope : {};
    const code = typeof error.code === "string" ? error.code : `HTTP_${response.status}`;
    const message = typeof error.message === "string"
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
        response,
    });
}
function requestMetadata(response) {
    const id = response.headers.get("x-request-id");
    return id === null ? {} : { id };
}
function resolvePaginationMode(profile, requested) {
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
function pageItems(page) {
    if (!isRecord(page))
        return [];
    if (Array.isArray(page.items))
        return page.items;
    if (isRecord(page.data) && Array.isArray(page.data.items))
        return page.data.items;
    return [];
}
function pagePagination(page) {
    if (!isRecord(page))
        return {};
    if (isRecord(page.pagination))
        return page.pagination;
    if (isRecord(page.meta) && isRecord(page.meta.pagination))
        return page.meta.pagination;
    if (isRecord(page.data)) {
        if (isRecord(page.data.pagination))
            return page.data.pagination;
        if (isRecord(page.data.meta) && isRecord(page.data.meta.pagination)) {
            return page.data.meta.pagination;
        }
    }
    return {};
}
function numberValue(primary, secondary, fallback) {
    if (typeof primary === "number" && Number.isFinite(primary))
        return primary;
    if (typeof secondary === "number" && Number.isFinite(secondary))
        return secondary;
    return fallback;
}
function transportError(code, message, cause) {
    return new APIError({ code, message, cause });
}
function transportErrorFromCause(code, message, cause, responseMetadata) {
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
function awaitAbortable(value, signal) {
    if (signal === undefined)
        return value;
    if (signal.aborted) {
        void value.catch(() => undefined);
        return Promise.reject(signal.reason);
    }
    return new Promise((resolve, reject) => {
        const onAbort = () => reject(signal.reason);
        signal.addEventListener("abort", onAbort, { once: true });
        value.then((result) => {
            signal.removeEventListener("abort", onAbort);
            resolve(result);
        }, (cause) => {
            signal.removeEventListener("abort", onAbort);
            reject(cause);
        });
    });
}
function isRecord(value) {
    return typeof value === "object" && value !== null && !Array.isArray(value);
}
