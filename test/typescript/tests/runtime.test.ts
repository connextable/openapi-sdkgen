import { describe, expect, it, vi } from "vitest";

import {
  TransportErrorCode,
  bindOperation,
  bindPathOperation,
  createPaginator,
  createRequest,
  getErrorCode,
  isAPIError,
  isErrorCode,
} from "../fixtures/generated/client/generated/runtime.js";
import type {
  OperationDefinition,
  RawResponse,
  RequestFunction,
} from "../fixtures/generated/client/generated/runtime.js";

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
            contentType: "Application/JSON",
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
      ["multipart/form-data", { name: "widget", file: new Uint8Array([1, 2, 3]) }],
      ["text/plain", "plain text"],
      ["application/octet-stream", new Uint8Array([1, 2])],
    ] as const) {
      await request(operation({ path: "/uploads", contentType }), { body });
    }
    expect(String(requests[0]?.body)).toBe("tag=one&tag=two&name=widget");
    expect(requests[1]?.body).toBeInstanceOf(FormData);
    expect((requests[1]?.body as FormData).get("name")).toBe("widget");
    const file = (requests[1]?.body as FormData).get("file");
    expect(file).toBeInstanceOf(Blob);
    expect([...new Uint8Array(await (file as Blob).arrayBuffer())]).toEqual([1, 2, 3]);
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
    expect(serverError.message).toBe("unavailable");
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

    const undefinedArray = await request(operation({ path: "/health" }), {
      query: { tags: ["valid", undefined] },
    }).catch((cause: unknown) => cause);
    expect(isErrorCode(undefinedArray, TransportErrorCode.REQUEST_ENCODE_FAILED)).toBe(true);
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

  it("stops cursor pagination when a cursor repeats after more than one page", async () => {
    const cursors: string[] = [];
    const paginate = createPaginator<string, { query: { cursor?: string } }, unknown>(
      async (input) => {
        const cursor = input.query.cursor ?? "start";
        cursors.push(cursor);
        return {
          items: [cursor],
          pagination: { nextCursor: cursor === "start" ? "a" : cursor === "a" ? "b" : "a" },
        };
      },
      "cursor",
    );
    await expect(collect(paginate({ query: {} }))).resolves.toEqual(["start", "a", "b"]);
    expect(cursors).toEqual(["start", "a", "b"]);
  });

  it("keeps ordinary bodies with contentType and value fields intact", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>(async (_input, init) => {
      expect(init?.body).toBe('{"contentType":"business","value":"payload"}');
      return jsonResponse({ ok: true });
    });
    const request = createRequest({ baseURL: "https://api.example.test", fetch });
    await expect(
      request(
        operation({
          path: "/single-body",
          requestBodies: [{ contentType: "application/json", schema: {} }],
        }),
        { body: { contentType: "business", value: "payload" } },
      ),
    ).resolves.toEqual({ ok: true });
  });

  it("serializes ordinary limit and sort query parameters", async () => {
    const request = createRequest({
      baseURL: "https://api.example.test",
      fetch: async (input) => {
        const url = new URL(String(input));
        expect(url.searchParams.get("limit")).toBe("25");
        expect(url.searchParams.get("sort")).toBe("createdAt");
        return jsonResponse({ ok: true });
      },
    });
    await expect(
      request(
        operation({
          path: "/search",
          parameters: [
            { location: "query", name: "limit", property: "limit", style: "form", explode: true },
            { location: "query", name: "sort", property: "sort", style: "form", explode: true },
          ],
        }),
        { query: { limit: 25, sort: "createdAt" } },
      ),
    ).resolves.toEqual({ ok: true });
  });

  it("serializes OpenAPI style variants for paths, query objects, headers, and cookies", async () => {
    const request = createRequest({
      baseURL: "https://api.example.test",
      fetch: async (input, init) => {
        const url = new URL(String(input));
        expect(url.pathname).toBe("/styles/.one.two/;first=1;second=two");
        expect(url.searchParams.get("filter")).toBe("first,1,second,two");
        const headers = new Headers(init?.headers);
        expect(headers.get("x-context")).toBe('{"scope":"all"}');
        expect(headers.get("cookie")).toBe("flags=one%2Ctwo");
        return jsonResponse({ ok: true });
      },
    });
    await expect(
      request(
        operation({
          path: "/styles/{label}/{matrix}",
          parameters: [
            { location: "path", name: "label", property: "label", style: "label", explode: true },
            {
              location: "path",
              name: "matrix",
              property: "matrix",
              style: "matrix",
              explode: true,
            },
            {
              location: "query",
              name: "filter",
              property: "filter",
              style: "form",
              explode: false,
            },
            {
              location: "header",
              name: "X-Context",
              property: "context",
              style: "simple",
              explode: false,
              contentType: "application/json",
            },
            { location: "cookie", name: "flags", property: "flags", style: "form", explode: false },
          ],
        }),
        {
          path: { label: ["one", "two"], matrix: { first: 1, second: "two" } },
          query: { filter: { first: 1, second: "two" } },
          headerParams: { context: { scope: "all" } },
          cookieParams: { flags: ["one", "two"] },
        },
      ),
    ).resolves.toEqual({ ok: true });
  });

  it("selects and transforms declared multi-representation request bodies", async () => {
    const request = createRequest({
      baseURL: "https://api.example.test",
      fetch: async (_input, init) => {
        expect(new Headers(init?.headers).get("content-type")).toBe("application/json");
        expect(init?.body).toBe('{"wire_name":"widget"}');
        return new Response(null, { status: 204 });
      },
    });
    await expect(
      request(
        operation({
          path: "/multi-body",
          requestBodies: [
            {
              contentType: "application/json",
              schema: { properties: { wire_name: { property: "displayName", schema: {} } } },
            },
            { contentType: "text/plain", schema: {} },
          ],
          inputSchemas: {},
        }),
        { body: { contentType: "application/json", value: { displayName: "widget" } } },
      ),
    ).resolves.toBeUndefined();
  });

  it("applies reference, tuple, and composed schemas to request and response values", async () => {
    const request = createRequest({
      baseURL: "https://api.example.test",
      fetch: async (_input, init) => {
        expect(init?.body).toBe('[{"wire_name":"request"},{"wire_name":"extra"}]');
        return jsonResponse([{ wire_name: "response" }, { wire_name: "extra-response" }]);
      },
    });
    const itemSchema = { properties: { wire_name: { property: "displayName", schema: {} } } };
    await expect(
      request<Array<{ displayName: string }>>(
        operation({
          path: "/tuple",
          requestBodies: [{ contentType: "application/json", schema: { reference: "Tuple" } }],
          responses: [
            { status: "200", contentType: "application/json", schema: { reference: "Tuple" } },
          ],
          inputSchemas: { Tuple: { prefixItems: [itemSchema], items: itemSchema } },
          outputSchemas: { Tuple: { prefixItems: [itemSchema], items: itemSchema } },
        }),
        { body: [{ displayName: "request" }, { displayName: "extra" }] },
      ),
    ).resolves.toEqual([{ displayName: "response" }, { displayName: "extra-response" }]);
  });

  it("applies every declared composition branch to wire-name transforms", async () => {
    const composed = {
      allOf: [{ properties: { wire_first: { property: "first", schema: {} } } }],
      oneOf: [{ properties: { wire_second: { property: "second", schema: {} } } }],
      anyOf: [{ properties: { wire_third: { property: "third", schema: {} } } }],
    };
    const request = createRequest({
      baseURL: "https://api.example.test",
      fetch: async (_input, init) => {
        expect(init?.body).toBe('{"wire_first":"one","wire_second":"two","wire_third":"three"}');
        return jsonResponse({
          wire_first: "response-one",
          wire_second: "response-two",
          wire_third: "response-three",
        });
      },
    });
    await expect(
      request<{ first: string; second: string; third: string }>(
        operation({
          path: "/composed",
          requestBodies: [{ contentType: "application/json", schema: composed }],
          responses: [{ status: "2XX", contentType: "application/json", schema: composed }],
          inputSchemas: {},
          outputSchemas: {},
        }),
        { body: { first: "one", second: "two", third: "three" } },
      ),
    ).resolves.toEqual({
      first: "response-one",
      second: "response-two",
      third: "response-three",
    });
  });

  it("times out when a fetch implementation ignores its AbortSignal", async () => {
    vi.useFakeTimers();
    try {
      const request = createRequest({
        baseURL: "https://api.example.test",
        fetch: async () => new Promise<Response>(() => undefined),
      });
      const pending = request(operation({ path: "/timeout" }), undefined, { timeoutMS: 5 });
      const result = pending.catch((cause: unknown) => cause);
      await vi.advanceTimersByTimeAsync(5);
      const error = await result;
      expect(isErrorCode(error, TransportErrorCode.REQUEST_TIMEOUT)).toBe(true);
    } finally {
      vi.useRealTimers();
    }
  });

  it("decodes documented text and JSON media types and preserves empty raw responses", async () => {
    const text = createRequest({
      baseURL: "https://api.example.test",
      fetch: async () =>
        new Response("ready", { status: 200, headers: { "content-type": "text/plain" } }),
    });
    await expect(text<string>(operation({ path: "/text" }))).resolves.toBe("ready");

    const problem = createRequest({
      baseURL: "https://api.example.test",
      fetch: async () =>
        new Response('{"title":"invalid"}', {
          status: 200,
          headers: { "content-type": "application/problem+json" },
        }),
    });
    await expect(problem<{ title: string }>(operation({ path: "/problem" }))).resolves.toEqual({
      title: "invalid",
    });

    const empty = createRequest({
      baseURL: "https://api.example.test",
      fetch: async () => new Response(null, { status: 204 }),
    });
    await expect(empty.raw(operation({ path: "/empty" }))).resolves.toMatchObject({
      status: 204,
      data: undefined,
    });
  });

  it("returns binary response streams with raw response metadata", async () => {
    const request = createRequest({
      baseURL: "https://api.example.test",
      fetch: async () =>
        new Response(new Uint8Array([4, 5, 6]), {
          status: 200,
          headers: { "content-type": "application/octet-stream" },
        }),
    });
    const raw = await request.raw<ReadableStream<Uint8Array>>(operation({ path: "/binary" }));
    expect(raw.contentType).toBe("application/octet-stream");
    expect(raw.data).toBeInstanceOf(ReadableStream);
    expect([...new Uint8Array(await new Response(raw.data).arrayBuffer())]).toEqual([4, 5, 6]);
  });

  it("stops offset pagination at zero limit and leaves caller input unchanged", async () => {
    const calls: Array<{ query: { offset?: number; limit?: number } }> = [];
    const paginate = createPaginator<
      string,
      { query: { offset?: number; limit?: number } },
      unknown
    >(async (input) => {
      calls.push(input);
      return { items: ["first"], pagination: { offset: 0, limit: 0, total: 100 } };
    }, "offset");
    const input = { query: { offset: 0, limit: 0 } };
    await expect(collect(paginate(input))).resolves.toEqual(["first"]);
    expect(calls).toEqual([{ query: { offset: 0, limit: 0 } }]);
    expect(input).toEqual({ query: { offset: 0, limit: 0 } });
  });

  it("stops pagination after an empty page returned through meta pagination", async () => {
    const calls: unknown[] = [];
    const paginate = createPaginator<
      string,
      { query: { offset?: number; limit?: number } },
      unknown
    >(async (input) => {
      calls.push(input);
      return { items: [], meta: { pagination: { offset: 10, limit: 5 } } };
    }, "offset");
    await expect(collect(paginate({ query: { offset: 10, limit: 5 } }))).resolves.toEqual([]);
    expect(calls).toHaveLength(1);
  });

  it("rejects invalid base URLs and keeps plain errors unclassified", () => {
    expect(() => createRequest({ baseURL: "/relative" })).toThrow("absolute URL");
    expect(() => createRequest({ baseURL: "ftp://api.example.test" })).toThrow("http(s)");
    expect(getErrorCode(new Error("plain"))).toBeUndefined();
  });

  it("uses operation server overrides and request-level transport options", async () => {
    const request = createRequest({
      baseURL: "https://api.example.test/v1",
      credentials: "include",
      headers: { "x-client-version": "one" },
      fetch: async (input, init) => {
        expect(String(input)).toBe("https://alternate.example.test/v2/health");
        expect(init?.credentials).toBe("omit");
        expect(new Headers(init?.headers).get("x-client-version")).toBe("one");
        return jsonResponse({ ok: true });
      },
    });
    await expect(
      request(
        operation({
          path: "health",
          serverURL: "https://alternate.example.test/v2/",
        }),
        undefined,
        { credentials: "omit" },
      ),
    ).resolves.toEqual({ ok: true });
  });

  it("uses global fetch only when no client transport is supplied", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>(async () => jsonResponse({ ok: true }));
    vi.stubGlobal("fetch", fetch);
    try {
      const request = createRequest({ baseURL: "https://api.example.test" });
      await expect(request(operation({ path: "/global" }))).resolves.toEqual({ ok: true });
      expect(fetch).toHaveBeenCalledOnce();
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("keeps structured server error details and fields", async () => {
    const request = createRequest({
      baseURL: "https://api.example.test",
      fetch: async () =>
        jsonResponse(
          {
            error: {
              code: "invalid",
              message: "bad input",
              details: { reason: "name" },
              fields: { name: "required" },
            },
          },
          422,
        ),
    });
    const error = await request(operation({ path: "/invalid" })).catch((cause: unknown) => cause);
    expect(isAPIError(error)).toBe(true);
    if (!isAPIError(error)) throw new Error("expected API error");
    expect(error.code).toBe("invalid");
    expect(error.details).toEqual({ reason: "name" });
    expect(error.fields).toEqual({ name: "required" });
  });

  it("maps additional-properties values on both request and response", async () => {
    const request = createRequest({
      baseURL: "https://api.example.test",
      fetch: async (_input, init) => {
        expect(init?.body).toBe('{"first":{"wire_name":"request"}}');
        return jsonResponse({ first: { wire_name: "response" } });
      },
    });
    await expect(
      request<{ first: { displayName: string } }>(
        operation({
          path: "/maps",
          requestBodies: [
            {
              contentType: "application/json",
              schema: {
                additionalProperties: {
                  properties: { wire_name: { property: "displayName", schema: {} } },
                },
              },
            },
          ],
          responses: [
            {
              status: "200",
              contentType: "application/json",
              schema: {
                additionalProperties: {
                  properties: { wire_name: { property: "displayName", schema: {} } },
                },
              },
            },
          ],
          inputSchemas: {},
          outputSchemas: {},
        }),
        { body: { first: { displayName: "request" } } },
      ),
    ).resolves.toEqual({ first: { displayName: "response" } });
  });

  it("honors cancellation even when fetch ignores its AbortSignal", async () => {
    let receivedSignal: AbortSignal | undefined;
    const request = createRequest({
      baseURL: "https://api.example.test",
      fetch: async (_input, init) => {
        receivedSignal = init?.signal as AbortSignal | undefined;
        return new Promise<Response>(() => undefined);
      },
    });
    const controller = new AbortController();
    const pending = request(operation({ path: "/slow" }), undefined, { signal: controller.signal });
    controller.abort("cancelled");
    const error = await pending.catch((cause: unknown) => cause);
    expect(receivedSignal).toBeDefined();
    expect(isErrorCode(error, TransportErrorCode.REQUEST_ABORTED)).toBe(true);
  });

  it("preserves response metadata when cancellation interrupts decoding", async () => {
    let release: (() => void) | undefined;
    let resolveFetch: ((response: Response) => void) | undefined;
    const response = jsonResponse({ ok: true }, 200, { "x-request-id": "server-1" });
    vi.spyOn(response, "json").mockImplementation(
      () =>
        new Promise<unknown>((resolve) => {
          release = () => resolve({ ok: true });
        }),
    );
    const request = createRequest({
      baseURL: "https://api.example.test",
      fetch: async () =>
        new Promise<Response>((resolve) => {
          resolveFetch = resolve;
        }),
    });
    const controller = new AbortController();
    const pending = request(operation({ path: "/decode" }), undefined, {
      signal: controller.signal,
    });
    await Promise.resolve();
    resolveFetch?.(response);
    await vi.waitFor(() => expect(release).toBeTypeOf("function"));
    controller.abort();
    const error = await pending.catch((cause: unknown) => cause);
    release?.();
    expect(isAPIError(error)).toBe(true);
    if (!isAPIError(error)) throw new Error("expected API error");
    expect(error.code).toBe(TransportErrorCode.REQUEST_ABORTED);
    expect(error.status).toBe(200);
    expect(error.request).toEqual({ id: "server-1" });
  });
});
