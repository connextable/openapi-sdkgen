import { describe, expect, it, vi } from "vitest";

import {
  TransportErrorCode,
  createClient,
  getRequestID,
  isAPIError,
  isErrorCategory,
  isErrorCode,
} from "../fixtures/generated/client/index.js";
import type {
  Client,
  LinkCalls,
  OperationBody,
  OperationContract,
  OperationHeaders,
  OperationInput,
  OperationOutput,
  OperationParameter,
  OperationPath,
  OperationQuery,
  Operations,
  PaginateCall,
  RawCall,
  ResourceCall,
  RouteBody,
  RouteContract,
  RouteHeaders,
  RouteInput,
  RouteOutput,
  RouteParameter,
  RoutePath,
  StreamCall,
} from "../fixtures/generated/client/index.js";
import { openapi } from "../fixtures/generated/client/metadata.js";

type Expect<Value extends true> = Value;
type Equal<Left, Right> =
  (<Value>() => Value extends Left ? 1 : 2) extends <Value>() => Value extends Right ? 1 : 2
    ? true
    : false;
type RootHidesMetadata = Expect<
  "openapi" extends keyof typeof import("../fixtures/generated/client/index.js") ? false : true
>;
type CreateTask = RouteContract<"POST /projects/{projectID}/tasks">;
type CreateTaskSlots = [
  CreateTask["input"],
  CreateTask["resourceInput"],
  CreateTask["options"],
  CreateTask["output"],
  CreateTask["error"],
  CreateTask["rawResponse"],
  CreateTask["call"],
  CreateTask["resourceCall"],
  CreateTask["pagination"],
  CreateTask["links"],
  CreateTask["stream"],
];
type CreateTaskCall = ResourceCall<"POST /projects/{projectID}/tasks">;
type CreateTaskRawCall = RawCall<"POST /projects/{projectID}/tasks">;
type TaskPagination = PaginateCall<"GET /tasks">;
type NoStream = StreamCall<"POST /projects/{projectID}/tasks">;
type NoLinks = LinkCalls<"POST /projects/{projectID}/tasks">;
type CreateTaskExactMethod = Client["$operations"]["createTask"];
type CreateTaskRouteMethod = Client["$routes"]["POST /projects/{projectID}/tasks"];
type CreateTaskResourceMethod = ReturnType<Client["projects"]>["tasks"]["create"];
type CreateTaskInputByID = OperationInput<"createTask">;
type CreateTaskInputByMethod = OperationInput<CreateTaskExactMethod>;
type CreateTaskInputByRouteMethod = OperationInput<CreateTaskRouteMethod>;
type CreateTaskResourceInput = OperationInput<CreateTaskResourceMethod>;
type CreateTaskBody = OperationBody<"createTask">;
type ListTasksQuery = OperationQuery<"listTasks">;
type ListTasksState = OperationParameter<"listTasks", "query", "state">;
type CreateWidgetHeaders = OperationHeaders<Client["widgets"]["create"]>;
type CreateWidgetTraceID = OperationParameter<Client["widgets"]["create"], "header", "X-Trace-Id">;
type HealthOutput = OperationOutput<Client["$routes"]["GET /health"]>;
type OperationHelperAssertions = [
  Expect<Equal<OperationContract<"createTask">, OperationContract<CreateTaskExactMethod>>>,
  Expect<Equal<CreateTaskInputByID, CreateTask["input"]>>,
  Expect<Equal<CreateTaskInputByMethod, CreateTask["input"]>>,
  Expect<Equal<CreateTaskInputByRouteMethod, CreateTask["input"]>>,
  Expect<Equal<CreateTaskResourceInput, CreateTask["resourceInput"]>>,
  Expect<Equal<CreateTaskBody, RouteBody<"POST /projects/{projectID}/tasks">>>,
  Expect<Equal<OperationOutput<CreateTaskResourceMethod>, CreateTask["output"]>>,
  Expect<Equal<RouteInput<"POST /projects/{projectID}/tasks">, CreateTask["input"]>>,
  Expect<Equal<RouteOutput<"POST /projects/{projectID}/tasks">, CreateTask["output"]>>,
  Expect<Equal<RoutePath<"POST /projects/{projectID}/tasks">, { readonly projectID: string }>>,
  Expect<Equal<RouteParameter<"POST /projects/{projectID}/tasks", "path", "projectID">, string>>,
  Expect<Equal<ListTasksState, string | null>>,
  Expect<Equal<ListTasksQuery["state"], string | null | undefined>>,
  Expect<Equal<CreateWidgetHeaders, RouteHeaders<"POST /widgets">>>,
  Expect<Equal<CreateWidgetTraceID, string>>,
  Expect<Equal<HealthOutput, void>>,
];
// @ts-expect-error Unknown route keys are rejected by the public helper.
type UnknownRoute = RouteContract<"GET /missing">;
// @ts-expect-error Unknown operation IDs are rejected.
type UnknownOperation = OperationInput<"missingOperation">;
// @ts-expect-error Route strings use Route* helpers rather than Operation* helpers.
type RouteStringAsOperation = OperationInput<"POST /projects/{projectID}/tasks">;
// @ts-expect-error Arbitrary functions are not generated operation sources.
type ArbitraryFunctionOperation = OperationInput<(input: unknown) => Promise<unknown>>;
// @ts-expect-error Resource-tree parent nodes are not operation methods.
type ResourceParentOperation = OperationInput<ReturnType<Client["projects"]>["tasks"]>;
// @ts-expect-error Raw methods are not standalone operation sources.
type RawMethodOperation = OperationInput<CreateTaskExactMethod["raw"]>;
// @ts-expect-error Pagination methods are not standalone operation sources.
type PaginationMethodOperation = OperationInput<Client["tasks"]["paginate"]>;
// @ts-expect-error Bound resource methods do not retain a path input section.
type BoundResourcePath = OperationPath<CreateTaskResourceMethod>;
// @ts-expect-error Query-only operations have no request body section.
type MissingBody = OperationBody<"listTasks">;
// @ts-expect-error Body-only operations have no query section.
type MissingQuery = OperationQuery<"createTask">;
// @ts-expect-error Body is not an OpenAPI parameter location.
type BodyParameter = OperationParameter<"createTask", "body", "title">;
// @ts-expect-error Unknown parameter names are rejected.
type UnknownParameter = OperationParameter<"listTasks", "query", "missing">;
// @ts-expect-error Individual optional parameter helpers exclude omission undefined.
const undefinedState: ListTasksState = undefined;
const nullableState: ListTasksState = null;

const rootHidesMetadata: RootHidesMetadata = true;
void [
  rootHidesMetadata,
  null as unknown as CreateTaskSlots,
  null as unknown as CreateTaskCall,
  null as unknown as CreateTaskRawCall,
  null as unknown as TaskPagination,
  null as unknown as NoStream,
  null as unknown as NoLinks,
  null as unknown as OperationHelperAssertions,
  nullableState,
  null as unknown as UnknownRoute,
  null as unknown as UnknownOperation,
  null as unknown as RouteStringAsOperation,
  null as unknown as ArbitraryFunctionOperation,
  null as unknown as ResourceParentOperation,
  null as unknown as RawMethodOperation,
  null as unknown as PaginationMethodOperation,
  null as unknown as BoundResourcePath,
  null as unknown as MissingBody,
  null as unknown as MissingQuery,
  null as unknown as BodyParameter,
  null as unknown as UnknownParameter,
  undefinedState,
];

describe("generated TypeScript source", () => {
  it("keeps lossless OpenAPI metadata behind its explicit entry", () => {
    expect(openapi.version).toBe("3.2.0");
    expect(openapi.versionLine).toBe("3.2");
    expect(openapi.document.openapi).toBe("3.2.0");
  });

  it("accepts binary request values exposed by generated body types", () => {
    const body: Operations["uploadWidget"]["input"]["body"] = new Uint8Array([1, 2, 3]);
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
        headerParams: { "X-Trace-Id": "trace-1" },
        body: { name: "first", requestId: "request-1" },
      }),
    ).resolves.toEqual({ id: "widget-1", name: "first", requestId: "request-2" });
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

  it("preserves bound path input through a concise resource call", async () => {
    const api = createClient({
      baseURL: "https://api.example.test/api",
      fetch: async (input, init) => {
        expect(`${init?.method} ${new URL(String(input)).pathname}`).toBe(
          "POST /api/projects/project%2F1/tasks",
        );
        return Response.json(
          {
            id: "task-1",
            projectID: "project/1",
            title: "Write documentation",
            priority: "HIGH",
          },
          { status: 201 },
        );
      },
    });

    await expect(
      api.projects("project/1").tasks.create({
        body: { title: "Write documentation", priority: "HIGH" },
      }),
    ).resolves.toMatchObject({ id: "task-1", projectID: "project/1" });
  });

  it("exposes raw responses through the generated operation call", async () => {
    const api = createClient({
      baseURL: "https://api.example.test/api",
      fetch: async () =>
        new Response('{"data":{"id":"widget-1","name":"raw"}}', {
          status: 201,
          headers: { "content-type": "application/json", "x-request-id": "raw-request" },
        }),
    });
    await expect(
      api.widgets.create.raw({
        query: {},
        headerParams: { "X-Trace-Id": "trace-raw" },
        body: { name: "raw" },
      }),
    ).resolves.toMatchObject({
      status: 201,
      contentType: "application/json",
      data: { data: { id: "widget-1", name: "raw" } },
      request: { id: "raw-request" },
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
      .create({ query: {}, headerParams: { "X-Trace-Id": "trace-2" }, body: { name: "invalid" } })
      .catch((cause: unknown) => cause);
    expect(isErrorCode(error, "validation_failed")).toBe(true);
    expect(isErrorCategory(error, "validation")).toBe(true);
    expect(getRequestID(error)).toBe("request-error");
    if (!isErrorCategory(error, "validation")) throw new Error("expected validation error");
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
        { query: {}, headerParams: { "X-Trace-Id": "trace-3" }, body: { name: "slow" } },
        { timeoutMS: 1 },
      )
      .catch((cause: unknown) => cause);
    expect(isAPIError(error)).toBe(true);
    if (!isAPIError(error)) throw new Error("expected API error");
    expect(error.code).toBe(TransportErrorCode.REQUEST_TIMEOUT);
  });
});
