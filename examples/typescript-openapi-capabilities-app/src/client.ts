import { request as httpRequest } from "node:http";

import {
  createClient,
  isValidationFailedError,
} from "./generated/capabilities-sdk/index.js";
import { openapi } from "./generated/capabilities-sdk/metadata.js";

const apiBaseURL = process.env.CAPABILITIES_API_BASE_URL ?? "http://127.0.0.1:18790/v1";
const webhookBaseURL = process.env.CAPABILITIES_WEBHOOK_BASE_URL ?? "http://127.0.0.1:18791";
const callbackBaseURL = process.env.CAPABILITIES_CALLBACK_BASE_URL ?? webhookBaseURL;
const api = createClient({
  baseURL: apiBaseURL,
  transport: {
    fetch: globalThis.fetch,
    capabilities: { cookieJar: true },
  },
});

const assert: (condition: unknown, message: string) => asserts condition = (condition, message) => {
  if (!condition) throw new Error(message);
};

assert(openapi.version === "3.2.0" && openapi.versionLine === "3.2", "OpenAPI version metadata changed");
assert(openapi.document.info.title === "Capability Showcase API", "OpenAPI document metadata changed");

// The exact-operation surface is useful for tooling and non-resource-shaped APIs.
// This call uses every supported parameter location and serialization style.
const inspected = await api.$operations.inspectRecord({
  path: { label: ["one", "two"], matrix: { left: "1", right: "two" } },
  query: {
    tag: ["one", "two"],
    filter: { name: "widget", state: "active" },
    spaces: ["one", "two"],
    pipes: ["one", "two"],
    query: { scope: "all" },
  },
  headerParams: { xTraceID: "trace-1" },
  cookieParams: { session: "example-session" },
});
assert(inspected.id === "item-record", "parameter serialization did not reach the server");

// Resource calls are the ergonomic default. The paginator follows the generated
// `pagination.nextCursor` contract without application-owned cursor plumbing.
const paginatedIDs: string[] = [];
for await (const item of api.items.paginate({ query: { limit: 1 } })) paginatedIDs.push(item.id);
assert(paginatedIDs.join(",") === "item-1,item-2", "cursor pagination did not traverse both pages");

// Input/output projections remove `id` from input and `secret` from output.
const created = await api.items.create.raw({
  body: {
    name: "Created item",
    status: "draft",
    secret: "request-only value",
    callbackURL: `${callbackBaseURL}/callbacks/delivery`,
  },
});
assert(
  created.status === 201 && created.request.id === "create-item-1" && created.data !== undefined,
  "raw response metadata changed",
);
assert(created.data.id === "item-3" && !("secret" in created.data), "item projection changed");

// Generated error guards expose the server's declared error code and details.
const invalid = await api.items
  .create({ body: { name: "server-rejected", status: "draft", callbackURL: `${callbackBaseURL}/callbacks/delivery` } })
  .catch((error: unknown) => error);
assert(isValidationFailedError(invalid), "typed validation error was not preserved");
assert(invalid.details?.name === "must not be empty", "validation details changed");

// Each generated request-body codec travels over the same ordinary client API.
await api.$operations.submitForm({ body: { name: "form item", tag: ["one", "two"] } });
await api.$operations.uploadAttachment({
  body: { name: "attachment.txt", file: "file contents" },
});
const text = await api.$operations.echoText({ body: "hello" });
assert(text === "echo:hello", "text response decoder changed");
const binary = await api.$operations.echoBinary({ body: new Uint8Array([4, 5, 6]) });
const binaryBytes = new Uint8Array(await new Response(binary).arrayBuffer());
assert(binaryBytes.join(",") === "4,5,6", "binary response decoder changed");

// Component Path Item references become the same resource-oriented API surface.
await api.status.get();

// OpenAPI 3.2 adds QUERY and arbitrary additional operations. All methods except
// TRACE run through standard Fetch. Browsers forbid TRACE, so consumers that need
// it provide an explicit transport implementation for that one operation.
await api.$operations.getVerb();
await api.$operations.putVerb();
await api.$operations.postVerb();
await api.$operations.deleteVerb();
await api.$operations.optionsVerb();
await api.$operations.headVerb();
await api.$operations.patchVerb();
await api.$operations.queryVerb();
await api.$operations.purgeVerb();

const traceFetch = (async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
  const url = new URL(input instanceof Request ? input.url : input.toString());
  const method = init?.method ?? (input instanceof Request ? input.method : "GET");
  assert(method === "TRACE", "trace transport must only serve TRACE calls");
  return await new Promise<Response>((resolve, reject) => {
    const request = httpRequest(
      url,
      { method, headers: Object.fromEntries(new Headers(init?.headers)) },
      (response) => {
        const chunks: Buffer[] = [];
        response.on("data", (chunk: Buffer) => chunks.push(Buffer.from(chunk)));
        response.on("end", () =>
          resolve(
            new Response(response.statusCode === 204 || response.statusCode === 205 || response.statusCode === 304 ? null : Buffer.concat(chunks), {
              status: response.statusCode ?? 500,
              headers: response.headers as HeadersInit,
            }),
          ),
        );
      },
    );
    request.once("error", reject);
    request.end();
  });
}) as typeof fetch;
await createClient({ baseURL: apiBaseURL, fetch: traceFetch }).$operations.traceVerb();

// Root Webhook names are not URLs. The host supplies its route and authentication
// policy; the generated Fetch router validates the JSON contract and calls its handler.
const webhookResponse = await fetch(`${webhookBaseURL}/hooks/items`, {
  method: "POST",
  headers: { "content-type": "application/json", "x-webhook-signature": "example-signature" },
  body: JSON.stringify({ id: "item-3", kind: "changed", attempt: 1 }),
});
assert(webhookResponse.status === 202, "webhook router rejected a valid delivery");
assert((await webhookResponse.json() as { accepted: string }).accepted === "item-3", "webhook handler output changed");

console.log(
  JSON.stringify(
    {
      inspected: inspected.id,
      paginatedIDs,
      created: created.data.id,
      callback: "delivered",
      webhook: "accepted",
    },
    null,
    2,
  ),
);
