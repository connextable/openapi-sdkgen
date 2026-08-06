import { describe, expect, it, vi } from "vitest";

import {
  createClient as createOpenAPI30Client,
  type Operations as OpenAPI30Operations,
  type Routes as OpenAPI30Routes,
} from "../fixtures/generated/baseline-oas30/index.js";
import {
  createClient as createOpenAPI31Client,
  type Operations as OpenAPI31Operations,
  type Routes as OpenAPI31Routes,
} from "../fixtures/generated/baseline-oas31/index.js";
import {
  createCallbackHandlers,
  type RouteCallbacks,
} from "../fixtures/generated/baseline-oas31/server/callbacks.js";
import {
  createClient as createOpenAPI32Client,
  type Operations as OpenAPI32Operations,
  type Routes as OpenAPI32Routes,
} from "../fixtures/generated/baseline-oas32/index.js";

type ExpectNever<Value extends never> = Value;
type OpenAPI30HasNoOperationAliases = ExpectNever<keyof OpenAPI30Operations>;
type OpenAPI31HasNoOperationAliases = ExpectNever<keyof OpenAPI31Operations>;
type OpenAPI32HasNoOperationAliases = ExpectNever<keyof OpenAPI32Operations>;

const openAPI30Output: OpenAPI30Routes["GET /"]["output"] = { version: "1.0.0" };
const openAPI31Output: OpenAPI31Routes["GET /source"]["output"] = { id: "source-1" };
const openAPI32Input: OpenAPI32Routes["QUERY /search"]["input"] = {
  query: { term: "sdk" },
};
const callbackHandler: RouteCallbacks["POST /jobs"]["status"]["{$request.body#/callbackURL}"]["POST"]["handler"] =
  async () => ({ status: 204 });
void [
  null as unknown as OpenAPI30HasNoOperationAliases,
  null as unknown as OpenAPI31HasNoOperationAliases,
  null as unknown as OpenAPI32HasNoOperationAliases,
  openAPI30Output,
  openAPI31Output,
  openAPI32Input,
  callbackHandler,
];

describe("extension-free version conformance", () => {
  it("generates and calls the OpenAPI 3.0 root route without an operation ID", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>(async (input, init) => {
      expect(`${init?.method} ${new URL(String(input)).pathname}`).toBe("GET /");
      return Response.json({ version: "3.0" });
    });
    const api = createOpenAPI30Client({ baseURL: "https://api.example.test", fetch });

    await expect(api.$routes["GET /"]()).resolves.toEqual({ version: "3.0" });
    expect(Object.keys(api.$operations)).toEqual([]);
    expect(api.get).toBe(api.$routes["GET /"]);
  });

  it("keeps ID-less OpenAPI 3.1 links and callbacks on exact route surfaces", async () => {
    const calls: string[] = [];
    const api = createOpenAPI31Client({
      baseURL: "https://api.example.test",
      fetch: async (input, init) => {
        const request = `${init?.method} ${new URL(String(input)).pathname}`;
        calls.push(request);
        if (request === "GET /source") return Response.json({ id: "source-1" });
        if (request === "POST /target") return new Response(null, { status: 204 });
        throw new Error(`unexpected request ${request}`);
      },
    });

    const source = await api.$routes["GET /source"].raw();
    await api.$routes["GET /source"].links.follow(source);
    expect(calls).toEqual(["GET /source", "POST /target"]);
    expect(Object.keys(api.$operations)).toEqual([]);

    const callbacks = createCallbackHandlers({
      routeCallbacks: {
        "POST /jobs": {
          status: {
            "{$request.body#/callbackURL}": {
              POST: callbackHandler,
            },
          },
        },
      },
    });
    const response = await callbacks.routeCallbacks["POST /jobs"].status[
      "{$request.body#/callbackURL}"
    ].POST.fetch(new Request("https://host.example.test/callback", { method: "POST" }));
    expect(response.status).toBe(204);
    expect(Object.keys(callbacks.callbacks)).toEqual([]);
  });

  it("keeps OpenAPI 3.2 QUERY and stream operations on exact route surfaces", async () => {
    const api = createOpenAPI32Client({
      baseURL: "https://api.example.test",
      fetch: async (input, init) => {
        const url = new URL(String(input));
        if (url.pathname === "/events") {
          expect(init?.method).toBe("GET");
          return new Response('"first"\n"second"\n', {
            headers: { "content-type": "application/x-ndjson" },
          });
        }
        expect(`${init?.method} ${url.pathname}`).toBe("QUERY /search");
        expect(url.searchParams.get("term")).toBe("sdk");
        return Response.json(["sdk", "generator"]);
      },
    });

    await expect(api.$routes["QUERY /search"]({ query: { term: "sdk" } })).resolves.toEqual([
      "sdk",
      "generator",
    ]);
    const events: string[] = [];
    for await (const event of api.$routes["GET /events"].stream()) events.push(event);
    expect(events).toEqual(["first", "second"]);
    expect(Object.keys(api.$operations)).toEqual([]);
  });
});
