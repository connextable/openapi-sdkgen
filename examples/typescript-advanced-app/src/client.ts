import {
  TransportErrorCode,
  isAPIError,
  isValidationFailedError,
  createClient,
} from "./generated/widget-sdk/index.js";

const baseURL = process.env.WIDGET_API_BASE_URL ?? "http://127.0.0.1:18788/v1";
const api = createClient({ baseURL, authorization: "Bearer example-token" });

const pages = [];
for await (const widget of api.widgets.paginate({ query: { limit: 1 } })) pages.push(widget);

const created = await api.widgets.create.raw({
  query: { tag: ["example", "raw"] },
  headerParams: { xTraceID: "trace-1" },
  cookieParams: { session: "example-session" },
  body: { name: "Created widget" },
});
const byPath = await api.customers("customer-1").widgets("widget-1").get();
await api.uploads.post({ body: new Uint8Array([1, 2, 3]) });

const validation = await api.widgets
  .create({
    query: {},
    headerParams: { xTraceID: "trace-1" },
    cookieParams: { session: "example-session" },
    body: { name: "" },
  })
  .catch((error: unknown) => error);
if (!isValidationFailedError(validation)) throw validation;

const timeout = await api.slow.get({ timeoutMS: 1 }).catch((error: unknown) => error);
if (!isAPIError(timeout) || timeout.code !== TransportErrorCode.REQUEST_TIMEOUT) throw timeout;

console.log(
  JSON.stringify(
    {
      pages,
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
