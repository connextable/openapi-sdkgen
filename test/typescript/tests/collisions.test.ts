import { describe, expect, it, vi } from "vitest";

import {
  Enums,
  createClient,
  type Components,
  type Operations,
} from "../fixtures/generated/collisions/index.js";
import {
  createCallbackHandlers,
  type CallbackHandlers,
} from "../fixtures/generated/collisions/server/callbacks.js";
import {
  createWebhookRouter,
  type WebhookHandlers,
} from "../fixtures/generated/collisions/server/webhooks.js";

type MoneyInput = Components["Money"]["input"];
type MoneyOutput = Components["Money"]["output"];
type LiteralMoneyInput = Components["MoneyInput"]["output"];
type LiteralMoneyOutput = Components["MoneyOutput"]["output"];
type ModernOperation = Operations["get-pet"];
type LegacyOperation = Operations["get_pet"];

const moneyInput: MoneyInput = { amount: 1, source: "wire-only" };
const moneyOutput: MoneyOutput = { amount: 1, receipt: "read-only" };
const literalMoneyInput: LiteralMoneyInput = "literal";
const literalMoneyOutput: LiteralMoneyOutput = 1;
const operationTypes: [ModernOperation["input"], LegacyOperation["output"]] | undefined = undefined;
void [moneyInput, moneyOutput, literalMoneyInput, literalMoneyOutput, operationTypes];

const record = Object.fromEntries([
  ["foo-bar", "modern"],
  ["foo_bar", "legacy"],
  ["__proto__", "prototype"],
  ["constructor", "constructor"],
  ["money", { amount: 1, receipt: "receipt" }],
  ["moneyInput", "literal"],
  ["moneyOutput", 1],
  ["normalizedOne", "one"],
  ["normalizedTwo", 2],
  ["status", "foo-bar"],
]) as Components["CollisionRecord"]["output"];

describe("exact identity collision fixture", () => {
  it("keeps component, operation, property, header, link, stream, and enum identities distinct", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>(async (input) => {
      const path = new URL(String(input)).pathname;
      if (path === "/pets/modern/pet%2Fone") {
        const response = new Response(JSON.stringify(record), {
          status: 200,
          headers: Object.fromEntries([
            ["content-type", "application/json"],
            ["foo-bar", "modern"],
            ["foo_bar", "legacy"],
            ["constructor", "constructor"],
          ]),
        });
        const headers = response.headers;
        Object.defineProperty(response, "headers", {
          value: {
            get(name: string) {
              return name === "__proto__" ? "prototype" : headers.get(name);
            },
          },
        });
        return response;
      }
      if (path === "/pets/legacy") return new Response(null, { status: 204 });
      if (path === "/streams/modern") {
        return new Response(`${JSON.stringify(record)}\n`, {
          status: 200,
          headers: { "content-type": "application/x-ndjson" },
        });
      }
      if (path === "/") return new Response(null, { status: 204 });
      throw new Error(`unexpected path ${path}`);
    });
    const api = createClient({ baseURL: "https://api.example.test", fetch });

    const raw = await api.$operations["get-pet"].raw({
      path: { "foo-bar": "pet/one" },
      query: { "foo-bar": "modern", foo_bar: "legacy" },
    });
    expect(raw.data["foo-bar"]).toBe("modern");
    expect(raw.data.foo_bar).toBe("legacy");
    expect(Object.prototype.hasOwnProperty.call(raw.data, "__proto__")).toBe(true);
    expect(Object.keys(raw.headers).sort()).toEqual([
      "__proto__",
      "constructor",
      "foo-bar",
      "foo_bar",
    ]);
    expect(Object.keys(api.$links["get-pet"]).sort()).toEqual([
      "__proto__",
      "constructor",
      "next-step",
      "next_step",
    ]);
    for (const name of ["next-step", "next_step", "__proto__", "constructor"] as const) {
      await api.$links["get-pet"][name](raw);
    }

    const streamed = [];
    for await (const value of api.$streams["stream-pet"]()) streamed.push(value);
    expect(streamed).toHaveLength(1);
    expect(streamed[0]?.constructor).toBe("constructor");
    await api.$operations["root-index"]();

    expect(Enums.Status).toEqual([
      "foo-bar",
      "foo_bar",
      Object.fromEntries([["__proto__", true]]),
      ["x", "y"],
      null,
    ]);
    expect(fetch).toHaveBeenCalledTimes(7);
  });

  it("keeps webhook and callback catalogs exact and prototype-safe", async () => {
    const webhookNames = ["event-hook", "event_hook", "__proto__", "constructor"] as const;
    const webhookHandlers = Object.fromEntries(
      webhookNames.map((name) => [name, { POST: async () => ({ status: 204 as const }) }]),
    ) as WebhookHandlers;
    const routes = Object.fromEntries(
      webhookNames.map((name) => [name, `/hooks/${encodeURIComponent(name)}`]),
    ) as unknown as Parameters<typeof createWebhookRouter>[1]["routes"];
    const router = createWebhookRouter(webhookHandlers, {
      routes,
      authenticate: () => undefined,
    });
    for (const name of webhookNames) {
      expect(
        (
          await router.fetch(
            new Request(`https://host.test/hooks/${encodeURIComponent(name)}`, {
              method: "POST",
            }),
          )
        ).status,
      ).toBe(204);
    }

    const expression = "{$request.query.callbackURL}";
    const callbackHandlers = {
      callbacks: {
        "get-pet": {
          "status-hook": {
            [expression]: { POST: async () => ({ status: 204 }) },
          },
          status_hook: {
            [expression]: { POST: async () => ({ status: 204 }) },
          },
          "component-status": {
            [expression]: { POST: async () => ({ status: 204 }) },
          },
        },
      },
      componentCallbacks: Object.fromEntries([
        [
          "Reusable-status",
          Object.fromEntries([[expression, { POST: async () => ({ status: 204 }) }]]),
        ],
        [
          "__proto__",
          Object.fromEntries([
            ["{$request.query.prototypeURL}", { POST: async () => ({ status: 204 }) }],
          ]),
        ],
      ]),
    } as unknown as CallbackHandlers;
    const callbacks = createCallbackHandlers(callbackHandlers);
    expect(Object.prototype.hasOwnProperty.call(callbacks.componentCallbacks, "__proto__")).toBe(
      true,
    );
    expect(
      (
        await callbacks.callbacks["get-pet"]["status-hook"][expression].POST.fetch(
          new Request("https://host.test/callback", { method: "POST" }),
        )
      ).status,
    ).toBe(204);
    expect(
      (
        await callbacks.componentCallbacks["__proto__"]["{$request.query.prototypeURL}"].POST.fetch(
          new Request("https://host.test/component", { method: "POST" }),
        )
      ).status,
    ).toBe(204);
  });
});
