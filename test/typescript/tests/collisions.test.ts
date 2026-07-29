import { describe, expect, it, vi } from "vitest";

import {
  Enums,
  createClient,
  isErrorCategory,
  type Components,
  type Operations,
} from "../fixtures/generated/collisions/index.js";
import {
  createCallbackHandlers,
  type Callbacks,
  type CallbackHandlers,
  type ComponentCallbacks,
} from "../fixtures/generated/collisions/server/callbacks.js";
import {
  createWebhookRouter,
  type WebhookHandlers,
} from "../fixtures/generated/collisions/server/webhooks.js";

type MoneyInput = Components["Money"]["input"];
type MoneyOutput = Components["Money"]["output"];
type LiteralMoneyInput = Components["MoneyInput"]["output"];
type LiteralMoneyOutput = Components["MoneyOutput"]["output"];
type SuffixInputProjection = Components["ProjectionInput"]["input"];
type SuffixOutputProjection = Components["ProjectionInput"]["output"];
type ModernOperation = Operations["get-pet"];
type LegacyOperation = Operations["get_pet"];
type ReusedCallbackA =
  Callbacks["get-pet"]["component-status"]["{$request.query.callbackURL}"]["POST"];
type ReusedCallbackB =
  Callbacks["get-pet-secondary"]["component-status"]["{$request.query.callbackURL}"]["POST"];
type MultiExpressionCallback =
  ComponentCallbacks["Reusable-status"]["{$request.query.backupURL}"]["DELETE"];
type ModernAuthenticationError = Extract<
  ModernOperation["error"],
  { readonly code: "authentication_required" }
>;
type LegacyAuthenticationError = Extract<
  LegacyOperation["error"],
  { readonly code: "authentication_required" }
>;
function assertErrorTypes(
  modernAuthenticationError: ModernAuthenticationError,
  legacyAuthenticationError: LegacyAuthenticationError,
  categoryError: unknown,
): void {
  modernAuthenticationError.details?.reason;
  legacyAuthenticationError.details?.challenge;
  // @ts-expect-error operation-specific error details do not widen to another operation
  modernAuthenticationError.details?.challenge;
  // @ts-expect-error operation-specific error details do not widen to another operation
  legacyAuthenticationError.details?.reason;

  if (isErrorCategory(categoryError, "authentication-required")) {
    if (categoryError.code === "authentication_required") {
      if (categoryError.details && "reason" in categoryError.details) {
        categoryError.details.reason;
      }
      // @ts-expect-error category code narrowing keeps only authentication details
      categoryError.details?.retryAfter;
    } else {
      categoryError.details?.retryAfter;
      // @ts-expect-error category code narrowing keeps only rate-limit details
      categoryError.details?.reason;
    }
  }
}
void assertErrorTypes;

const moneyInput: MoneyInput = { amount: 1, source: "wire-only" };
const moneyOutput: MoneyOutput = { amount: 1, receipt: "read-only" };
const literalMoneyInput: LiteralMoneyInput = "literal";
const literalMoneyOutput: LiteralMoneyOutput = 1;
const suffixInput: SuffixInputProjection = { source: "request" };
const suffixOutput: SuffixOutputProjection = { receipt: "response" };
// @ts-expect-error readOnly field is absent from input projection despite the component suffix
const invalidSuffixInput: SuffixInputProjection = { receipt: "response" };
// @ts-expect-error writeOnly field is absent from output projection despite the component suffix
const invalidSuffixOutput: SuffixOutputProjection = { source: "request" };
const operationTypes: [ModernOperation["input"], LegacyOperation["output"]] | undefined = undefined;
const modernInput: ModernOperation["input"] = {
  path: { "foo-bar": "modern" },
  query: { "foo-bar": "one", foo_bar: "two" },
};
const legacyInput: LegacyOperation["input"] = { query: { "legacy-id": "legacy" } };
// @ts-expect-error normalization-equivalent operation IDs keep distinct required inputs
const invalidLegacyInput: LegacyOperation["input"] = modernInput;
const enumString: (typeof Enums.Status)[0] = "foo-bar";
const enumObject: (typeof Enums.Status)[2] = { ["__proto__"]: true };
const enumArray: (typeof Enums.Status)[3] = ["x", "y"];
// @ts-expect-error enum tuple members retain exact string literals
const invalidEnumString: (typeof Enums.Status)[0] = "foo_bar";
// @ts-expect-error nested enum object values remain exact
const invalidEnumObject: (typeof Enums.Status)[2] = { ["__proto__"]: false };
// @ts-expect-error enum arrays retain order
const invalidEnumArray: (typeof Enums.Status)[3] = ["y", "x"];
void [
  moneyInput,
  moneyOutput,
  literalMoneyInput,
  literalMoneyOutput,
  suffixInput,
  suffixOutput,
  invalidSuffixInput,
  invalidSuffixOutput,
  operationTypes,
  modernInput,
  legacyInput,
  invalidLegacyInput,
  enumString,
  enumObject,
  enumArray,
  invalidEnumString,
  invalidEnumObject,
  invalidEnumArray,
  null as unknown as ReusedCallbackA,
  null as unknown as ReusedCallbackB,
  null as unknown as MultiExpressionCallback,
];

const record = Object.fromEntries([
  ["foo-bar", "modern"],
  ["foo_bar", "legacy"],
  ["__proto__", "prototype"],
  ["constructor", "constructor"],
  ["prototype", "prototype-property"],
  ["toString", "to-string"],
  ['quote"key', "quote"],
  ["back\\slash", "backslash"],
  ["line\nbreak", "control"],
  ["한글", "unicode"],
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
      if (path === "/pets/legacy") {
        const value = new URL(String(input)).searchParams.get("legacy-id");
        if (value !== "linked" && value !== "direct") throw new Error(`legacy input mismatch: ${value}`);
        return new Response(null, { status: 204 });
      }
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
    for (const key of [
      "prototype",
      "toString",
      'quote"key',
      "back\\slash",
      "line\nbreak",
      "한글",
    ]) {
      expect(Object.prototype.hasOwnProperty.call(raw.data, key)).toBe(true);
    }
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
    await api.$operations.get_pet({ query: { "legacy-id": "direct" } });

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
    expect(Object.prototype.hasOwnProperty.call(Enums.Status[2], "__proto__")).toBe(true);
    expect(Object.getPrototypeOf(Enums.Status[2])).toBe(Object.prototype);
    expect(fetch).toHaveBeenCalledTimes(8);
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
        "get-pet-secondary": {
          "component-status": {
            [expression]: { POST: async () => ({ status: 204 }) },
          },
        },
      },
      componentCallbacks: {
        "Reusable-status": {
          [expression]: { POST: async () => ({ status: 204 }) },
        },
        ["__proto__"]: {
          "{$request.query.prototypeURL}": { POST: async () => ({ status: 204 }) },
        },
      },
    } satisfies CallbackHandlers;
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
    expect(
      (
        await callbacks.callbacks["get-pet-secondary"]["component-status"][
          expression
        ].POST.fetch(new Request("https://host.test/reused", { method: "POST" }))
      ).status,
    ).toBe(204);
  });
});
