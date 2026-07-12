import { describe, expect, it, vi } from "vitest";

import {
  TransportErrorCode,
  bindOperation,
  bindPathOperation,
  createPaginator,
  createRequest,
  isAPIError,
  isErrorCode,
} from "../fixtures/generated/client/dist/generated/runtime.js";
import type {
  OperationDefinition,
  RawResponse,
  RequestFunction,
} from "../fixtures/generated/client/dist/generated/runtime.js";

const operation = (overrides: Partial<OperationDefinition> = {}): OperationDefinition => ({
  operationID: "runtimeTest",
  method: "POST",
  path: "/items/{itemID}",
  envelope: "none",
  ...overrides,
});

const jsonResponse = (body: unknown, status = 200, headers: HeadersInit = {}): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json", ...headers },
  });

const collect = async <Item>(items: AsyncIterable<Item>): Promise<Item[]> => {
  const result: Item[] = [];
  for await (const item of items) result.push(item);
  return result;
};

describe("generated runtime", () => {
  it("serializes paths, query styles, headers, cookies, and wire names", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>(async (input, init) => {
      const url = new URL(String(input));
      expect(url.pathname).toBe("/v1/items/;itemID=one%2Ftwo");
      expect(url.searchParams.getAll("tags")).toEqual(["one", "two"]);
      expect(url.searchParams.get("filter[name]")).toBe("widget");
      expect(url.searchParams.get("filter[active]")).toBe("true");
      expect(url.searchParams.get("spaces")).toBe("one two");
      expect(url.searchParams.get("pipes")).toBe("one|two");
      expect(url.searchParams.get("query")).toBe('{"scope":"all"}');
      const headers = new Headers(init?.headers);
      expect(headers.get("x-trace")).toBe("trace-1");
      expect(headers.get("cookie")).toBe("session=one; session=two");
      expect(headers.get("authorization")).toBe("Bearer client");
      expect(headers.get("x-request-id")).toBe("request-1");
      expect(init?.body).toBe('{"wire_name":"widget"}');
      return jsonResponse({ wire_name: "response" }, 200, { "x-request-id": "server-1" });
    });
    const request = createRequest({
      baseURL: "https://api.example.test/v1/",
      fetch,
      authorization: "Bearer client",
    });

    const result = await request<{ displayName: string }>(
      operation({
        parameters: [
          { location: "path", name: "itemID", property: "itemID", style: "matrix", explode: false },
          { location: "query", name: "tags", property: "tags", style: "form", explode: true },
          {
            location: "query",
            name: "filter",
            property: "filter",
            style: "deepObject",
            explode: true,
          },
          {
            location: "query",
            name: "spaces",
            property: "spaces",
            style: "spaceDelimited",
            explode: false,
          },
          {
            location: "query",
            name: "pipes",
            property: "pipes",
            style: "pipeDelimited",
            explode: false,
          },
          {
            location: "query",
            name: "query",
            property: "query",
            style: "form",
            explode: true,
            contentType: "application/json",
          },
          {
            location: "header",
            name: "X-Trace",
            property: "trace",
            style: "simple",
            explode: false,
          },
          {
            location: "cookie",
            name: "session",
            property: "session",
            style: "form",
            explode: true,
          },
        ],
        inputSchemas: {
          Body: { properties: { wire_name: { property: "displayName", schema: {} } } },
        },
        requestBodies: [{ contentType: "application/json", schema: { reference: "Body" } }],
        responses: [
          {
            status: "2XX",
            contentType: "application/json",
            schema: { properties: { wire_name: { property: "displayName", schema: {} } } },
          },
        ],
        outputSchemas: {},
      }),
      {
        path: { itemID: "one/two" },
        query: {
          tags: ["one", "two"],
          filter: { name: "widget", active: true },
          spaces: ["one", "two"],
          pipes: ["one", "two"],
          query: { scope: "all" },
        },
        headerParams: { trace: "trace-1" },
        cookieParams: { session: ["one", "two"] },
        body: { displayName: "widget" },
      },
      { requestID: "request-1" },
    );
    expect(result).toEqual({ displayName: "response" });
  });

  it("encodes form, multipart, text, and binary bodies", async () => {
    const requests: RequestInit[] = [];
    const request = createRequest({
      baseURL: "https://api.example.test",
      fetch: async (_input, init) => {
        requests.push(init ?? {});
        return new Response(null, { status: 204 });
      },
    });
    for (const [contentType, body] of [
      ["application/x-www-form-urlencoded", { tag: ["one", "two"], name: "widget" }],
      ["multipart/form-data", { name: "widget", file: new Blob(["body"]) }],
      ["text/plain", "plain text"],
      ["application/octet-stream", new Uint8Array([1, 2])],
    ] as const) {
      await request(operation({ path: "/uploads", contentType }), { body });
    }
    expect(String(requests[0]?.body)).toBe("tag=one&tag=two&name=widget");
    expect(requests[1]?.body).toBeInstanceOf(FormData);
    expect((requests[1]?.body as FormData).get("name")).toBe("widget");
    expect(requests[2]?.body).toBe("plain text");
    expect(requests[3]?.body).toBeInstanceOf(Uint8Array);
  });

  it("normalizes raw output and transport failures", async () => {
    const raw = createRequest({
      baseURL: "https://api.example.test",
      fetch: async () =>
        jsonResponse({ data: { id: "widget-1" } }, 201, { "x-request-id": "server-1" }),
    });
    await expect(
      raw.raw<{ id: string }>(operation({ path: "/health", envelope: "data" })),
    ).resolves.toMatchObject({
      status: 201,
      contentType: "application/json",
      data: { id: "widget-1" },
      request: { id: "server-1" },
    });

    const network = createRequest({
      baseURL: "https://api.example.test",
      fetch: async () => {
        throw new Error("offline");
      },
    });
    const error = await network(operation({ path: "/health" })).catch((cause: unknown) => cause);
    expect(isErrorCode(error, TransportErrorCode.NETWORK_ERROR)).toBe(true);

    const decode = createRequest({
      baseURL: "https://api.example.test",
      fetch: async () =>
        new Response("not json", { status: 200, headers: { "content-type": "application/json" } }),
    });
    const decodeError = await decode(operation({ path: "/health" })).catch(
      (cause: unknown) => cause,
    );
    expect(isErrorCode(decodeError, TransportErrorCode.RESPONSE_DECODE_FAILED)).toBe(true);

    const server = createRequest({
      baseURL: "https://api.example.test",
      fetch: async () =>
        new Response("unavailable", { status: 503, headers: { "content-type": "text/plain" } }),
    });
    const serverError = await server(operation({ path: "/health" })).catch(
      (cause: unknown) => cause,
    );
    expect(isAPIError(serverError)).toBe(true);
    if (!isAPIError(serverError)) throw new Error("expected API error");
    expect(serverError.code).toBe("HTTP_503");
    expect(isErrorCode(serverError, TransportErrorCode.NETWORK_ERROR)).toBe(false);
    expect(isAPIError(new Error("plain"))).toBe(false);

    const controller = new AbortController();
    controller.abort("stop");
    const aborted = createRequest({
      baseURL: "https://api.example.test",
      fetch: async () => {
        throw new Error("aborted");
      },
    });
    const abortedError = await aborted(operation({ path: "/health" }), undefined, {
      signal: controller.signal,
    }).catch((cause: unknown) => cause);
    expect(isErrorCode(abortedError, TransportErrorCode.REQUEST_ABORTED)).toBe(true);
  });

  it("rejects missing input and raw headers owned by the contract", async () => {
    const request = createRequest({
      baseURL: "https://api.example.test",
      fetch: async () => jsonResponse({}),
    });
    const missing = await request(operation(), {}).catch((cause: unknown) => cause);
    expect(isErrorCode(missing, TransportErrorCode.REQUEST_ENCODE_FAILED)).toBe(true);

    const reserved = await request(operation({ path: "/health" }), undefined, {
      headers: { "content-type": "application/json" },
    }).catch((cause: unknown) => cause);
    expect(isErrorCode(reserved, TransportErrorCode.REQUEST_ENCODE_FAILED)).toBe(true);
  });

  it("binds operations and path input without mutating caller input", async () => {
    const calls: unknown[][] = [];
    const request = (async <Output>(...args: unknown[]) => {
      calls.push(args);
      return { id: "widget-1" } as Output;
    }) as unknown as RequestFunction;
    request.raw = async <Output>() =>
      ({
        status: 200,
        contentType: "application/json",
        data: { id: "widget-1" } as Output,
        headers: new Headers(),
        request: {},
        response: new Response(),
      }) as RawResponse<Output>;
    const call = bindOperation<{ body: { name: string } }, { id: string }>(
      request,
      operation(),
      true,
    );
    const bound = bindPathOperation(call, { itemID: "widget-1" }, true);
    const input = { body: { name: "first" } };
    await expect(bound(input)).resolves.toEqual({ id: "widget-1" });
    expect(input).toEqual({ body: { name: "first" } });
    expect(calls).toEqual([
      [operation(), { body: { name: "first" }, path: { itemID: "widget-1" } }, undefined],
    ]);

    const noInput = bindOperation<never, { id: string }>(
      request,
      operation({ path: "/health" }),
      false,
    );
    await expect(noInput()).resolves.toEqual({ id: "widget-1" });
    await expect(noInput.raw()).resolves.toMatchObject({ data: { id: "widget-1" } });
  });

  it("paginates cursor and offset profiles and rejects invalid modes", async () => {
    const cursorInputs: Array<{ query: { cursor?: string } }> = [];
    const cursor = createPaginator<string, { query: { cursor?: string } }, unknown>(
      async (input) => {
        cursorInputs.push(input);
        return input.query.cursor === undefined
          ? { items: ["one"], pagination: { nextCursor: "next" } }
          : { items: ["two"], pagination: { nextCursor: "" } };
      },
      "cursor",
    );
    await expect(collect(cursor({ query: {} }))).resolves.toEqual(["one", "two"]);
    expect(cursorInputs).toEqual([{ query: {} }, { query: { cursor: "next" } }]);

    const offset = createPaginator<string, { query: { offset?: number; limit?: number } }, unknown>(
      async (input) => ({
        items: input.query.offset === 0 ? ["one", "two"] : ["three"],
        pagination: { offset: input.query.offset, limit: 2, total: 3 },
      }),
      "offset",
    );
    await expect(collect(offset({ query: { offset: 0, limit: 2 } }))).resolves.toEqual([
      "one",
      "two",
      "three",
    ]);

    const both = createPaginator<string, { query: {} }, unknown>(
      async () => ({ data: { items: ["nested"], pagination: { nextCursor: "" } } }),
      "both",
    );
    await expect(collect(both({ mode: "cursor", query: {} } as never))).resolves.toEqual([
      "nested",
    ]);
    const error = await collect(both({ query: {} } as never)).catch((cause: unknown) => cause);
    expect(error).toBeInstanceOf(TypeError);
    expect(isAPIError(error)).toBe(false);

    const invalidCursor = await collect(cursor({ query: { offset: 1 } } as never)).catch(
      (cause: unknown) => cause,
    );
    expect(invalidCursor).toBeInstanceOf(TypeError);
  });
});
