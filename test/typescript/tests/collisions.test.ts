import { describe, expect, it, vi } from "vitest";

import {
  Enums,
  createClient,
  isErrorCategory,
  isEnumValue,
  type Client,
  type Components,
  type EnumValue,
  type OperationInput,
  type OperationParameter,
  type Operations,
} from "../fixtures/generated/collisions/index.js";
import {
  Enums as DirectEnums,
  isEnumValue as isDirectEnumValue,
  type EnumValue as DirectEnumValue,
} from "../fixtures/generated/collisions/enums.js";
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
type TodoStatus = EnumValue<"TodoStatus">;
type StatusValue = EnumValue<"Status">;
type DirectTodoStatus = DirectEnumValue<"TodoStatus">;
type Equal<Left, Right> =
  (<Value>() => Value extends Left ? 1 : 2) extends <Value>() => Value extends Right ? 1 : 2
    ? true
    : false;
type Expect<Value extends true> = Value;
type OperationIdentityAssertions = [
  Expect<Equal<OperationInput<"get-pet">, ModernOperation["input"]>>,
  Expect<Equal<OperationInput<Client["$operations"]["get-pet"]>, ModernOperation["input"]>>,
  Expect<Equal<OperationInput<"get_pet">, LegacyOperation["input"]>>,
  Expect<Equal<OperationInput<Client["$operations"]["get_pet"]>, LegacyOperation["input"]>>,
  Expect<Equal<OperationParameter<"get-pet", "query", "foo-bar">, string>>,
  Expect<Equal<OperationParameter<"get-pet", "query", "foo_bar">, string>>,
];
type EnumAssertions = [
  Expect<Equal<typeof Enums.TodoStatus.TODO, "TODO">>,
  Expect<Equal<typeof Enums.TodoStatus.DONE, "DONE">>,
  Expect<Equal<TodoStatus, "TODO" | "DONE">>,
  Expect<Equal<DirectTodoStatus, TodoStatus>>,
  Expect<Equal<typeof DirectEnums, typeof Enums>>,
  Expect<
    Equal<
      StatusValue,
      | "foo-bar"
      | "foo_bar"
      | "__proto__"
      | "constructor"
      | "map"
      | "length"
      | "values"
      | "members"
      | "0"
      | 2
      | true
      | null
      | { readonly __proto__: true; readonly nested: { readonly value: 1 } }
      | readonly ["x", "y"]
    >
  >,
];
// @ts-expect-error Normalized parameter spellings are not accepted in place of exact names.
type NormalizedParameterName = OperationParameter<"get-pet", "query", "fooBar">;
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
const enumString: (typeof Enums.Status)["foo-bar"] = "foo-bar";
const enumObject: Extract<StatusValue, { readonly __proto__: true }> = {
  ["__proto__"]: true,
  nested: { value: 1 },
};
const enumArray: Extract<StatusValue, readonly ["x", "y"]> = ["x", "y"];
// @ts-expect-error exact enum members retain their own literal values
const invalidEnumString: (typeof Enums.Status)["foo-bar"] = "foo_bar";
const invalidEnumObject: Extract<StatusValue, { readonly __proto__: true }> = {
  // @ts-expect-error nested enum object values remain exact
  ["__proto__"]: false,
  nested: { value: 1 },
};
// @ts-expect-error enum arrays retain order
const invalidEnumArray: Extract<StatusValue, readonly ["x", "y"]> = ["y", "x"];
// @ts-expect-error unknown enum component names are rejected
type UnknownEnum = EnumValue<"MissingStatus">;
function assertEnumArrayAPIsAreAbsent(): void {
  // @ts-expect-error generated enum values have no positional index contract
  Enums.TodoStatus[0];
  // @ts-expect-error generated enum values do not expose Array.prototype.map
  Enums.TodoStatus.map((value: TodoStatus) => value);
  // @ts-expect-error generated enum values do not expose Array.prototype.includes
  Enums.TodoStatus.includes("TODO");
  // @ts-expect-error generated enum values are not tuple-indexed types
  type TupleIndexedEnum = (typeof Enums.TodoStatus)[number];
  void (null as unknown as TupleIndexedEnum);
}
function narrowTodoStatus(value: unknown): TodoStatus | undefined {
  if (isEnumValue(Enums.TodoStatus, value)) return value;
  return undefined;
}
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
  null as unknown as OperationIdentityAssertions,
  null as unknown as EnumAssertions,
  null as unknown as NormalizedParameterName,
  modernInput,
  legacyInput,
  invalidLegacyInput,
  enumString,
  enumObject,
  enumArray,
  invalidEnumString,
  invalidEnumObject,
  invalidEnumArray,
  null as unknown as UnknownEnum,
  assertEnumArrayAPIsAreAbsent,
  narrowTodoStatus,
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
  it("keeps root enum exports identical to the direct enum entry", () => {
    expect(DirectEnums).toBe(Enums);
    expect(DirectEnums.TodoStatus).toBe(Enums.TodoStatus);
    expect(isDirectEnumValue).toBe(isEnumValue);
  });

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
        if (value !== "linked" && value !== "direct")
          throw new Error(`legacy input mismatch: ${value}`);
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

    expect(fetch).toHaveBeenCalledTimes(8);
  });

  it("exposes exact enum members through iterable non-array values", () => {
    expect(Enums.TodoStatus.TODO).toBe("TODO");
    expect(Enums.TodoStatus.DONE).toBe("DONE");
    expect(Array.isArray(Enums.TodoStatus)).toBe(false);
    expect([...Enums.TodoStatus]).toEqual(["TODO", "DONE"]);
    expect(Array.from(Enums.TodoStatus)).toEqual(["TODO", "DONE"]);
    expect(Object.getPrototypeOf(Enums.Status)).toBe(null);
    expect(Object.keys(Enums.Status).sort()).toEqual(
      [
        "foo-bar",
        "foo_bar",
        "__proto__",
        "constructor",
        "map",
        "length",
        "values",
        "members",
        "0",
      ].sort(),
    );
    expect(Object.getOwnPropertyDescriptor(Enums.Status, Symbol.iterator)?.enumerable).toBe(false);
    for (const member of [
      "__proto__",
      "constructor",
      "map",
      "length",
      "values",
      "members",
    ] as const) {
      expect(Object.prototype.hasOwnProperty.call(Enums.Status, member)).toBe(true);
      expect(Enums.Status[member]).toBe(member);
    }

    const values = [...Enums.Status];
    expect(values).toEqual([
      "foo-bar",
      "foo_bar",
      "__proto__",
      "constructor",
      "map",
      "length",
      "values",
      "members",
      "0",
      2,
      true,
      null,
      Object.fromEntries([
        ["__proto__", true],
        ["nested", { value: 1 }],
      ]),
      ["x", "y"],
    ]);
    const objectValue = values.at(-2);
    expect(
      typeof objectValue === "object" && objectValue !== null && !Array.isArray(objectValue),
    ).toBe(true);
    expect(Object.prototype.hasOwnProperty.call(objectValue, "__proto__")).toBe(true);
  });

  it("narrows primitive and structured JSON enum values", () => {
    expect(isEnumValue(Enums.TodoStatus, "TODO")).toBe(true);
    expect(isEnumValue(Enums.TodoStatus, "todo")).toBe(false);
    expect(isEnumValue(Enums.Status, 2)).toBe(true);
    expect(isEnumValue(Enums.Status, null)).toBe(true);
    expect(isEnumValue(Enums.Status, ["x", "y"])).toBe(true);
    expect(isEnumValue(Enums.Status, ["y", "x"])).toBe(false);
    expect(
      isEnumValue(
        Enums.Status,
        Object.fromEntries([
          ["nested", { value: 1 }],
          ["__proto__", true],
        ]),
      ),
    ).toBe(true);
    expect(
      isEnumValue(
        Enums.Status,
        Object.fromEntries([
          ["nested", { value: 2 }],
          ["__proto__", true],
        ]),
      ),
    ).toBe(false);

    const sparse = ["x"];
    sparse.length = 2;
    expect(isEnumValue(Enums.Status, sparse)).toBe(false);
    const cyclic: { self?: unknown } = {};
    cyclic.self = cyclic;
    expect(isEnumValue(Enums.Status, cyclic)).toBe(false);
    let getterRead = false;
    const accessor = Object.fromEntries([["nested", { value: 1 }]]);
    Object.defineProperty(accessor, "__proto__", {
      enumerable: true,
      get() {
        getterRead = true;
        return true;
      },
    });
    expect(isEnumValue(Enums.Status, accessor)).toBe(false);
    expect(getterRead).toBe(false);
    expect(
      isEnumValue(
        Enums.Status,
        new Proxy(Object.create(null), {
          ownKeys() {
            throw new Error("unreadable");
          },
        }),
      ),
    ).toBe(false);
  });

  it("keeps webhook and callback maps exact and prototype-safe", async () => {
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
        await callbacks.callbacks["get-pet-secondary"]["component-status"][expression].POST.fetch(
          new Request("https://host.test/reused", { method: "POST" }),
        )
      ).status,
    ).toBe(204);
  });
});
