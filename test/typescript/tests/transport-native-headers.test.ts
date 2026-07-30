import { describe, expect, it } from "vitest";

import {
  createClient,
  type Operations,
} from "../fixtures/generated/transport-native-headers/index.js";
import { openapi } from "../fixtures/generated/transport-native-headers/metadata.js";
import {
  createCallbackHandlers,
  type Callbacks,
} from "../fixtures/generated/transport-native-headers/server/callbacks.js";
import {
  createWebhookRouter,
  type Webhooks,
} from "../fixtures/generated/transport-native-headers/server/webhooks.js";

type AllHeaderInput = NonNullable<Operations["allEnvironmentHeaders"]["input"]["headerParams"]>;

const omittedHeaders: AllHeaderInput = {};
const explicitHeaders = {
  "Accept-Charset": "forwarded",
  "Accept-Encoding": "forwarded",
  "Access-Control-Request-Headers": "forwarded",
  "Access-Control-Request-Method": "forwarded",
  Connection: "forwarded",
  "Content-Length": "forwarded",
  Cookie: "forwarded",
  Cookie2: "forwarded",
  Date: "forwarded",
  DNT: "forwarded",
  Expect: "forwarded",
  Host: "forwarded",
  "Keep-Alive": "forwarded",
  Origin: "forwarded",
  Referer: "forwarded",
  "Set-Cookie": "forwarded",
  TE: "forwarded",
  Trailer: "forwarded",
  "Transfer-Encoding": "forwarded",
  Upgrade: "forwarded",
  Via: "forwarded",
  "Proxy-Future": "forwarded",
  "Sec-Future": "forwarded",
} satisfies AllHeaderInput;

const mixedInput: Operations["mixedHeaders"]["input"] = {
  headerParams: { "X-Trace": "trace" },
};
const overrideInput: Operations["overrideMethod"]["input"] = {
  headerParams: { "X-HTTP-Method-Override": "PATCH, TRACE" },
};
declare const typedClient: ReturnType<typeof createClient>;
if (false) {
  typedClient.$operations.allEnvironmentHeaders();
  typedClient.$operations.allEnvironmentHeaders.raw();
  typedClient.$operations.allEnvironmentHeaders({ headerParams: explicitHeaders });
  typedClient.$operations.mixedHeaders(mixedInput);
  typedClient.$operations.overrideMethod(overrideInput);
  // @ts-expect-error ordinary required headers still require operation input
  typedClient.$operations.mixedHeaders();
  // @ts-expect-error ordinary required headers remain required inside headerParams
  typedClient.$operations.mixedHeaders({ headerParams: { Origin: "https://caller.example" } });
  // @ts-expect-error method-override headers retain OpenAPI requiredness
  typedClient.$operations.overrideMethod();
}
void [omittedHeaders, mixedInput, overrideInput];

const headerEntries = Object.entries(explicitHeaders);

describe("transport-native request headers", () => {
  it("keeps every fixed and prefix-family header optional and explicitly settable", async () => {
    const seen: Headers[] = [];
    const api = createClient({
      baseURL: "https://api.example.test",
      fetch: async (_input, init) => {
        seen.push(new Headers(init?.headers));
        return new Response(null, { status: 204 });
      },
    });

    await api.$operations.allEnvironmentHeaders();
    await api.$operations.allEnvironmentHeaders.raw();
    await api.$operations.allEnvironmentHeaders({ headerParams: explicitHeaders });

    expect([...seen[0]!.keys()]).toEqual([]);
    expect([...seen[1]!.keys()]).toEqual([]);
    for (const [name, value] of headerEntries) {
      expect(seen[2]!.get(name)).toBe(value);
    }
  });

  it("preserves ordinary requiredness and delegates method-override values", async () => {
    const seen: Headers[] = [];
    const api = createClient({
      baseURL: "https://api.example.test",
      fetch: async (_input, init) => {
        seen.push(new Headers(init?.headers));
        return new Response(null, { status: 204 });
      },
    });

    await api.$operations.mixedHeaders(mixedInput);
    await api.$operations.mixedHeaders({
      headerParams: { Origin: "https://caller.example", "X-Trace": "trace-explicit" },
    });
    await api.$operations.overrideMethod(overrideInput);

    expect(seen[0]!.get("Origin")).toBeNull();
    expect(seen[0]!.get("X-Trace")).toBe("trace");
    expect(seen[1]!.get("Origin")).toBe("https://caller.example");
    expect(seen[1]!.get("X-Trace")).toBe("trace-explicit");
    expect(seen[2]!.get("X-HTTP-Method-Override")).toBe("PATCH, TRACE");
  });

  it("forwards undeclared raw and header API-key values while retaining ownership", async () => {
    const seen: Array<{ path: string; headers: Headers }> = [];
    const api = createClient({
      baseURL: "https://api.example.test",
      credentials: ({ alternatives }) => ({
        alternative: alternatives.OriginKey!,
        values: { OriginKey: { kind: "api-key", value: "https://credential.example" } },
      }),
      fetch: async (input, init) => {
        seen.push({ path: new URL(String(input)).pathname, headers: new Headers(init?.headers) });
        return new Response(null, { status: 204 });
      },
    });

    await api.$operations.rawHeaders({
      headers: {
        Origin: "https://raw.example",
        "Proxy-Future": "raw-proxy",
        "Sec-Future": "raw-sec",
      },
    });
    await api.$operations.securedByOrigin();

    expect(seen[0]!.headers.get("Origin")).toBe("https://raw.example");
    expect(seen[0]!.headers.get("Proxy-Future")).toBe("raw-proxy");
    expect(seen[0]!.headers.get("Sec-Future")).toBe("raw-sec");
    expect(seen[1]!.headers.get("Origin")).toBe("https://credential.example");

    await expect(
      api.$operations.allEnvironmentHeaders(undefined, {
        headers: { Origin: "https://wrong-channel.example" },
      }),
    ).rejects.toMatchObject({ code: "REQUEST_ENCODE_FAILED" });
    expect(seen).toHaveLength(2);
  });

  it("resolves Link sources from caller input and forwards target values", async () => {
    const seen: Array<{ path: string; headers: Headers }> = [];
    const api = createClient({
      baseURL: "https://api.example.test",
      fetch: async (input, init) => {
        seen.push({ path: new URL(String(input)).pathname, headers: new Headers(init?.headers) });
        return new Response(null, { status: 204 });
      },
    });

    const explicit = await api.$operations.linkSource.raw({
      headerParams: { Origin: "https://source.example" },
    });
    await api.$links.linkSource.copy(explicit, {
      sourceInput: { headerParams: { Origin: "https://source.example" } },
    });
    const omitted = await api.$operations.linkSource.raw();
    await api.$links.linkSource.copy(omitted);
    await api.$links.linkSource.literal(omitted);

    expect(seen.map(({ path, headers }) => [path, Object.fromEntries(headers.entries())])).toEqual([
      ["/link-source", { origin: "https://source.example" }],
      ["/link-target", { "x-trace": "https://source.example" }],
      ["/link-source", {}],
      ["/link-target", {}],
      ["/link-target", { origin: "https://linked.example.test" }],
    ]);
  });

  it("preserves inbound callback, webhook, and metadata requiredness", async () => {
    type CallbackHeaderParams =
      Callbacks["callbackSource"]["status"]["{$request.body#/callbackURL}"]["POST"]["context"]["params"]["headerParams"];
    type WebhookHeaderParams = Webhooks["delivery"]["POST"]["context"]["params"]["headerParams"];
    const callbackType: CallbackHeaderParams = { Origin: "https://caller.example" };
    const webhookType: WebhookHeaderParams = { Origin: "https://caller.example" };
    void [callbackType, webhookType];

    const callback = createCallbackHandlers({
      callbacks: {
        callbackSource: {
          status: {
            "{$request.body#/callbackURL}": {
              POST: async () => ({ status: 204 }),
            },
          },
        },
      },
    }).callbacks.callbackSource.status["{$request.body#/callbackURL}"].POST;
    expect(
      (
        await callback.fetch(
          new Request("https://host.test/callback", {
            method: "POST",
            headers: { Origin: "https://caller.example" },
          }),
        )
      ).status,
    ).toBe(204);
    expect(
      (await callback.fetch(new Request("https://host.test/callback", { method: "POST" }))).status,
    ).toBe(400);

    const webhook = createWebhookRouter(
      { delivery: { POST: async () => ({ status: 204 }) } },
      { routes: { delivery: "/hooks/delivery" } },
    );
    expect(
      (
        await webhook.fetch(
          new Request("https://host.test/hooks/delivery", {
            method: "POST",
            headers: { Origin: "https://caller.example" },
          }),
        )
      ).status,
    ).toBe(204);
    expect(
      (
        await webhook.fetch(
          new Request("https://host.test/hooks/delivery", {
            method: "POST",
          }),
        )
      ).status,
    ).toBe(400);

    const operationOrigin = openapi.document.paths["/all-environment"].get.parameters.find(
      (parameter: { name?: string }) => parameter.name === "Origin",
    );
    const callbackOrigin = openapi.document.paths["/callback-source"].post.callbacks.status[
      "{$request.body#/callbackURL}"
    ].post.parameters.find((parameter: { name?: string }) => parameter.name === "Origin");
    const webhookOrigin = openapi.document.webhooks.delivery.post.parameters.find(
      (parameter: { name?: string }) => parameter.name === "Origin",
    );
    expect([operationOrigin?.required, callbackOrigin?.required, webhookOrigin?.required]).toEqual([
      true,
      true,
      true,
    ]);
  });
});
