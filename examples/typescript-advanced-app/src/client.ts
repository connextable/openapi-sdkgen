import {
  createClient,
  TransportErrorCode,
  isAPIError,
  isValidationFailedError,
} from "./generated/widget-sdk/index.js";

const baseURL = process.env.WIDGET_API_BASE_URL ?? "http://127.0.0.1:18788/v1";
const api = createClient({
  baseURL,
  authorization: "Bearer example-token",
  transport: {
    fetch: globalThis.fetch,
    capabilities: { cookieJar: true },
  },
});

// Cursor pagination: the generated helper follows `pagination.nextCursor`.
const paginatedWidgets = [];
for await (const widget of api.widgets.paginate({ query: { limit: 1 } })) {
  paginatedWidgets.push(widget);
}
const widgetIDs = paginatedWidgets.map((widget) => widget.id);
if (widgetIDs.join(",") !== "widget-1,widget-2") {
  throw new Error(`unexpected paginated widgets: ${JSON.stringify(paginatedWidgets)}`);
}

// Query arrays, explicit headers, cookies, client authorization, and raw metadata.
const created = await api.widgets.create.raw({
  query: { tag: ["example", "raw"] },
  headerParams: { xTraceID: "trace-1" },
  cookieParams: { session: "example-session" },
  body: { name: "Created widget" },
});

if (created.status !== 201 || created.request.id !== "request-1" || created.data.name !== "Created widget") {
  throw new Error(`unexpected raw create response: ${JSON.stringify(created)}`);
}

// Nested path resources preserve each path parameter in the generated API shape.
const byPath = await api.customers("customer-1").widgets("widget-1").get();
if (byPath.id !== "widget-1" || byPath.name !== "Customer widget") {
  throw new Error(`unexpected path resource: ${JSON.stringify(byPath)}`);
}

// Binary bodies use Uint8Array and keep the declared octet-stream media type.
await api.uploads.post({ body: new Uint8Array([1, 2, 3]) });

// Generated guards preserve server-declared validation details.
const validation = await api.widgets
  .create({
    query: {},
    headerParams: { xTraceID: "trace-1" },
    cookieParams: { session: "example-session" },
    body: { name: "" },
  })
  .catch((error: unknown) => error);
if (!isValidationFailedError(validation)) throw validation;
if (validation.details?.field !== "name") {
  throw new Error(`unexpected validation error: ${JSON.stringify(validation)}`);
}

// Per-request timeout options produce a stable transport error code.
const timeout = await api.slow.get({ timeoutMS: 1 }).catch((error: unknown) => error);
if (!isAPIError(timeout) || timeout.code !== TransportErrorCode.REQUEST_TIMEOUT) throw timeout;

console.log(
  JSON.stringify(
    {
      paginatedWidgets,
      created: created.data,
      requestID: created.request.id,
      byPath,
      validation: validation.details,
      timeout: timeout.code,
    },
    null,
    2,
  ),
);
