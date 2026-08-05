# Generated TypeScript types

The generated SDK exposes component-, route-, and operation-based types from its
main entry point. Feature code can derive request and response types from the same
generated methods it calls at runtime instead of declaring those shapes again.

See [Generated client API](./client-api.md) for runtime calls, authentication,
errors, Links, and streams.

## Choose a type source

Choose the source that matches the identity already available to the consumer.

| Available identity | Helper family | Typical use |
| --- | --- | --- |
| generated exact or resource method | `Operation*<typeof method>` | Feature code coupled to a generated client method |
| exact OpenAPI `operationId` | `Operation*<"operationId">` | Shared types keyed by a stable operation ID |
| exact `"METHOD /path"` | `Route*<"METHOD /path">` | Routes without an `operationId`, or route-oriented infrastructure |
| component schema name | `ComponentInput<Name>` / `ComponentOutput<Name>` | Domain types shared by several operations |
| complete generated catalogs | `Components`, `Routes`, `Operations` | Generic tooling that needs every contract slot |

The focused helpers are normally easier to read than manually indexing a catalog and
normalizing optional fields with TypeScript utility types.

## Extract types from generated methods

Use `typeof` on a generated method and pass that method type to an `Operation*`
helper. The helper accepts methods from all three generated client surfaces.

```ts
import {
  createClient,
  type OperationBody,
  type OperationInput,
  type OperationQuery,
} from "./generated/api";

const api = createClient({
  baseURL: "https://api.example.test/v1",
});

const listTodos = api.$operations.listTodos;
type TodoFilters = OperationQuery<typeof listTodos>;

const exactUpdate = api.$routes["PATCH /todos/{todoID}"];
type UpdateInput = OperationInput<typeof exactUpdate>;

const updateTodo = api.todos("todo-1").update;
type UpdateBody = OperationBody<typeof updateTodo>;
```

This is compile-time extraction only. It adds no wrapper or runtime metadata to the
method.

### Exact and bound methods

`$operations` and `$routes` expose exact methods. A resource-tree leaf may already
have path parameters bound by parent selectors.

| Method value | `OperationInput` result |
| --- | --- |
| `api.$operations.updateTodo` | full input, including `path` and `body` |
| `api.$routes["PATCH /todos/{todoID}"]` | full input, including `path` and `body` |
| `api.todos("todo-1").update` | bound input containing `body`, without `path` |

The selected surface also applies to `OperationContract<Source>`. Its `input` and
`call` slots describe the exact method or the bound resource method supplied as
`Source`.

```ts
import type { OperationContract } from "./generated/api";

type ExactContract = OperationContract<typeof exactUpdate>;
type BoundContract = OperationContract<typeof updateTodo>;
```

`OperationPath<typeof updateTodo>` is rejected because the resource method has no
path input left. Raw, pagination, Link, and stream functions are capabilities rather
than standalone operation sources; use their route-keyed call types instead.

### Use extracted types in feature code

Extracted types can annotate state, function parameters, or `satisfies` expressions
without repeating the OpenAPI shape.

```ts
const filters = { completed: false } satisfies TodoFilters;

async function update(body: OperationBody<typeof updateTodo>) {
  return updateTodo({ body });
}
```

When only the `Client` type is available, the corresponding method type can be indexed
without creating a client value:

```ts
import type { Client, OperationInput } from "./generated/api";

type UpdateInput = OperationInput<Client["$operations"]["updateTodo"]>;
```

## Extract types by operation ID

Every non-hidden operation with an explicit `operationId` can be used directly as an
`Operation*` string source.

```ts
import type {
  OperationBody,
  OperationContract,
  OperationInput,
  OperationOutput,
  OperationParameter,
  OperationQuery,
  Operations,
} from "./generated/api";

type ListContract = OperationContract<"listTodos">;
type ListInput = OperationInput<"listTodos">;
type ListOutput = OperationOutput<"listTodos">;
type ListQuery = OperationQuery<"listTodos">;
type Limit = OperationParameter<"listTodos", "query", "limit">;
type CreateBody = OperationBody<"createTodo">;
type CatalogInput = Operations["listTodos"]["input"];
```

The string must be an exact operation ID. A route string is not an operation ID and is
therefore not accepted by `Operation*`; use a `Route*` helper for it.

## Extract types by route

Route helpers use the exact `"METHOD /openapi/path"` string as their identity. They
cover generated routes whether or not OpenAPI declares an `operationId`.

```ts
import type {
  RouteBody,
  RouteContract,
  RouteInput,
  RouteOutput,
  RouteParameter,
  Routes,
} from "./generated/api";

type UpdateContract = RouteContract<"PATCH /todos/{todoID}">;
type UpdateInput = RouteInput<"PATCH /todos/{todoID}">;
type UpdateOutput = RouteOutput<"PATCH /todos/{todoID}">;
type UpdateBody = RouteBody<"PATCH /todos/{todoID}">;
type TodoID = RouteParameter<"PATCH /todos/{todoID}", "path", "todoID">;

type HealthOutput = Routes["GET /health"]["output"];
```

### Route contract slots

`RouteContract<Route>` keeps the complete route contract behind public type names.

| Slot | Meaning | Focused helper |
| --- | --- | --- |
| `input` | complete exact-call input | `RouteInput<Route>` |
| `resourceInput` | input after resource path binding | `RouteResourceInput<Route>` |
| `options` | per-request transport options | `RouteOptions<Route>` |
| `output` | decoded successful output | `RouteOutput<Route>` |
| `error` | generated error response union | `RouteContract<Route>["error"]` |
| `rawResponse` | successful raw response union | `RouteRawResponse<Route>` |
| `call` | exact operation method | `OperationMethod<Route>` |
| `resourceCall` | resource-oriented method | `ResourceCall<Route>` |
| `pagination` | pagination iterator when available | `PaginateCall<Route>` |
| `links` | response-link calls when available | `LinkCalls<Route>` |
| `stream` | streaming call when available | `StreamCall<Route>` |

Use the full contract when several slots must stay tied to the same route. Use a
focused helper when only one slot is needed. Capability slots resolve to `never` when
the route does not declare that capability.

## Request sections and parameters

Section helpers return the complete caller-facing section. Parameter helpers select
one value from a path, query, query-string, header, or cookie section.

| Input section | Operation helper | Route helper | Parameter location |
| --- | --- | --- | --- |
| request body | `OperationBody` | `RouteBody` | not applicable |
| path parameters | `OperationPath` | `RoutePath` | `"path"` |
| query parameters | `OperationQuery` | `RouteQuery` | `"query"` |
| query-string parameters | `OperationQuerystring` | `RouteQuerystring` | `"querystring"` |
| header parameters | `OperationHeaders` | `RouteHeaders` | `"header"` |
| cookie parameters | `OperationCookies` | `RouteCookies` | `"cookie"` |

```ts
import type {
  OperationBody,
  OperationParameter,
  OperationQuery,
  RouteParameter,
  RouteQuery,
} from "./generated/api";

type Filters = OperationQuery<typeof listTodos>;
type Limit = OperationParameter<typeof listTodos, "query", "limit">;
type CreateBody = OperationBody<"createTodo">;
type RouteFilters = RouteQuery<"GET /todos">;
type RouteLimit = RouteParameter<"GET /todos", "query", "limit">;
```

The generic constraints reject a source without the requested section, an invalid
location, or an unknown parameter name. Body fields are schema properties rather than
OpenAPI Parameter Objects, so `"body"` is not a parameter location.

### Optional and nullable values

A section helper removes only the outer `undefined` caused by an optional aggregate
input field. Optional properties inside the section remain optional.

An individual parameter helper removes omission `undefined` while preserving
schema-declared `null`:

```ts
type Query = OperationQuery<"listTodos">;
// Query["search"] is string | null | undefined when the property is optional.

type Search = OperationParameter<"listTodos", "query", "search">;
// Search is string | null.
```

`OperationInput` or `RouteInput` remains authoritative for whether the complete call
may omit a section.

## Component input and output types

Component helpers use exact names from `components.schemas` and expose separate input
and output projections.

```ts
import type {
  ComponentInput,
  ComponentOutput,
  Components,
} from "./generated/api";

type TodoInput = ComponentInput<"Todo">;
type TodoOutput = ComponentOutput<"Todo">;

type TodoContract = Components["Todo"];
type EquivalentInput = TodoContract["input"];
type EquivalentOutput = TodoContract["output"];
```

`readOnly` fields are omitted from input projections. `writeOnly` fields are omitted
from output projections. Required, optional, nullable, literal, union, array, and
object shapes continue to follow the OpenAPI schema.

Inline operation schemas do not create a `Components` entry. Extract them through the
corresponding operation or route input, output, body, or section helper.

## Enum values and types

A generated component schema whose top-level schema declares `enum` gets both a type
and a runtime value list.

Given this OpenAPI component:

```yaml
components:
  schemas:
    TodoStatus:
      type: string
      enum: [TODO, DONE]
```

the SDK can use the exact values at both compile time and runtime:

```ts
import { Enums, type ComponentOutput } from "./generated/api";

type TodoStatus = ComponentOutput<"TodoStatus">;
// "TODO" | "DONE"

type TodoStatusFromValues = (typeof Enums.TodoStatus)[number];
// "TODO" | "DONE"

const defaultStatus: TodoStatus = Enums.TodoStatus[0];
const options = Enums.TodoStatus.map((value) => ({
  label: value,
  value,
}));
```

`Enums.TodoStatus` is a runtime array typed as a readonly tuple. Values retain their
OpenAPI order and exact JSON literals. They are not normalized into TypeScript enum
member names, so use the values themselves rather than `Enums.TodoStatus.DONE`.

Component names also remain exact. Use bracket notation when necessary:

```ts
type DashedTodoStatus = ComponentOutput<"todo-status">;
type DashedTodoStatusValue = (typeof Enums["todo-status"])[number];
```

The runtime catalog supports JSON enum values, including strings, numbers, booleans,
`null`, arrays, and objects. Their generated tuple types preserve literal values,
object properties, and array order.

Only a component schema with `enum` on the component's top-level schema receives an
`Enums` entry. A nested-property or inline enum is still emitted as a literal union in
its containing input/output type, but it does not receive a separate runtime catalog
entry. Promote such an enum to a component schema when feature code needs reusable
runtime options.

## Capability and utility types

```ts
import {
  SortDirection,
  type BothPaginationInput,
  type CursorPaginationInput,
  type LinkCalls,
  type OffsetPaginationInput,
  type OperationMethod,
  type OperationRawCall,
  type PaginateCall,
  type RawCall,
  type ResourceCall,
  type StreamCall,
} from "./generated/api";
```

| Type or value | Purpose |
| --- | --- |
| `OperationMethod<Route>` | exact generated method type used by `$operations` and `$routes` |
| `OperationRawCall<Route>` | exact raw-call method |
| `ResourceCall<Route>` | resource-tree method after applicable path binding |
| `RawCall<Route>` | resource-oriented raw call |
| `PaginateCall<Route>` | pagination iterator call |
| `LinkCalls<Route>` | response-link calls |
| `StreamCall<Route>` | streaming response call |
| `CursorPaginationInput` | cursor and limit controls |
| `OffsetPaginationInput` | offset and limit controls |
| `BothPaginationInput` | cursor or offset controls without mixing modes |
| `SortDirection` | `"asc" | "desc"` type and matching runtime constants |

```ts
type UpdateCall = ResourceCall<"PATCH /todos/{todoID}">;
type UpdateRaw = RawCall<"PATCH /todos/{todoID}">;
type ListPages = PaginateCall<"GET /todos">;

const direction: SortDirection = SortDirection.DESC;
```

## Naming and editor output

Generated operation IDs, routes, component names, and parameter names preserve their
OpenAPI spelling and case. Use bracket notation for names that are not valid
TypeScript identifiers.

The public helpers deliberately keep generated implementation aliases and method
identity brands behind public type names. VS Code hover and signature help should show
types such as `OperationInput`, `RouteBody`, and `ResourceCall`, not internal
`__sdkgen_*` declarations.
