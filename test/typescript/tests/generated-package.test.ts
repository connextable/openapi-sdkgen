import { readFile } from "node:fs/promises";

import { describe, expect, it, vi } from "vitest";

import {
  TransportErrorCode,
  createClient,
  getRequestID,
  isAPIError,
  isValidationFailedError,
} from "@example/conformance-client";
import type { UploadWidgetBodyInput } from "@example/conformance-client";

describe("generated TypeScript package", () => {
  it("accepts binary request values exposed by generated body types", () => {
    const body: UploadWidgetBodyInput = new Uint8Array([1, 2, 3]);
    expect(body).toBeInstanceOf(Uint8Array);
  });

  it("exports a nested resource client that serializes request inputs", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>(async (input, init) => {
      const url = new URL(String(input));
      expect(`${init?.method} ${url.pathname}${url.search}`).toBe(
        "POST /api/widgets?tag=one&tag=two",
      );
      expect(new Headers(init?.headers).get("x-trace-id")).toBe("trace-1");
      expect(init?.body).toBe('{"name":"first","requestId":"request-1"}');
      return new Response('{"data":{"id":"widget-1","name":"first","requestId":"request-2"}}', {
        status: 201,
        headers: { "content-type": "Application/JSON" },
      });
    });
    const api = createClient({ baseURL: "https://api.example.test/api", fetch });

    await expect(
      api.widgets.create({
        query: { tag: ["one", "two"] },
        headerParams: { xTraceID: "trace-1" },
        body: { name: "first", requestID: "request-1" },
      }),
    ).resolves.toEqual({ id: "widget-1", name: "first", requestID: "request-2" });
    expect(api.widgets.create).toBe(api.$operations.createWidget);
  });

  it("preserves path parameters through the nested resource tree", async () => {
    const api = createClient({
      baseURL: "https://api.example.test/api",
      fetch: async (input, init) => {
        expect(`${init?.method} ${new URL(String(input)).pathname}`).toBe(
          "GET /api/customers/customer%2F1/widgets/widget%2F2",
        );
        return new Response('{"data":{"id":"widget/2","name":"nested"}}', {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      },
    });

    await expect(api.customers("customer/1").widgets("widget/2").get()).resolves.toEqual({
      id: "widget/2",
      name: "nested",
    });
  });

  it("exports generated error guards with documented error details", async () => {
    const api = createClient({
      baseURL: "https://api.example.test/api",
      fetch: async () =>
        new Response(
          '{"error":{"code":"validation_failed","message":"invalid","details":{"field":"name"}}}',
          {
            status: 400,
            headers: { "content-type": "application/json", "x-request-id": "request-error" },
          },
        ),
    });

    const error = await api.widgets
      .create({ query: {}, headerParams: { xTraceID: "trace-2" }, body: { name: "invalid" } })
      .catch((cause: unknown) => cause);
    expect(isValidationFailedError(error)).toBe(true);
    expect(getRequestID(error)).toBe("request-error");
    if (!isValidationFailedError(error)) throw new Error("expected validation error");
    expect(error.details).toEqual({ field: "name" });
  });

  it("keeps timeout active while decoding a response body", async () => {
    const api = createClient({
      baseURL: "https://api.example.test/api",
      fetch: async (_input, init) =>
        ({
          body: {},
          headers: new Headers({ "content-type": "application/json" }),
          json: () =>
            new Promise((_, reject) => {
              init?.signal?.addEventListener("abort", () => reject(init.signal?.reason), {
                once: true,
              });
            }),
          ok: true,
          status: 201,
        }) as Response,
    });

    const error = await api.widgets
      .create(
        { query: {}, headerParams: { xTraceID: "trace-3" }, body: { name: "slow" } },
        { timeoutMS: 1 },
      )
      .catch((cause: unknown) => cause);
    expect(isAPIError(error)).toBe(true);
    if (!isAPIError(error)) throw new Error("expected API error");
    expect(error.code).toBe(TransportErrorCode.REQUEST_TIMEOUT);
  });

  it("keeps generated package metadata independent from the conformance harness", async () => {
    const packageJSON = JSON.parse(
      await readFile(new URL("../fixtures/generated/client/package.json", import.meta.url), "utf8"),
    ) as Record<string, unknown>;
    expect(packageJSON.name).toBe("@example/conformance-client");
    expect(packageJSON.dependencies).toBeUndefined();
    expect(packageJSON.private).toBeUndefined();
    expect(packageJSON.files).toEqual(["dist", "README.md", "manifest.json"]);
    expect(packageJSON.exports).toEqual({
      ".": { import: "./dist/src/index.js", types: "./dist/src/index.d.ts" },
    });
    await expect(
      readFile(new URL("../fixtures/generated/client/dist/src/index.js", import.meta.url), "utf8"),
    ).resolves.toContain("generated/index.js");
  });
});
