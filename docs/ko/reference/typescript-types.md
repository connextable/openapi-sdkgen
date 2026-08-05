# 생성된 TypeScript 타입

생성된 SDK의 기본 진입점은 요청, 응답, component, enum 타입을 제공합니다.

생성된 클라이언트의 호출 방법은
[생성된 클라이언트 API](./client-api.md)에서 확인할 수 있습니다.

## 타입 기준 선택

| 기준 | Helper |
| --- | --- |
| 생성된 클라이언트 메서드 | `Operation*<typeof method>` |
| OpenAPI `operationId` | `Operation*<"operationId">` |
| `"METHOD /path"` route | `Route*<"METHOD /path">` |
| `components.schemas` 이름 | `ComponentInput<Name>` / `ComponentOutput<Name>` |

## 생성된 메서드에서 추출

생성된 메서드의 타입을 `Operation*` helper에 전달합니다.

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

const updateTodo = api.todos("todo-1").update;
type UpdateInput = OperationInput<typeof updateTodo>;
type UpdateBody = OperationBody<typeof updateTodo>;
```

`OperationInput`은 메서드에 전달하는 전체 인자입니다. `OperationBody`는 request
body입니다. Resource tree 메서드에서는 selector로 전달한 값이 입력에서
제외됩니다.

```ts
const filters = { completed: false } satisfies TodoFilters;

async function update(body: UpdateBody) {
  return updateTodo({ body });
}
```

## Operation ID 또는 route로 추출

OpenAPI `operationId`에는 `Operation*` helper를 사용하고 `"METHOD /path"`
문자열에는 `Route*` helper를 사용합니다.

```ts
import type {
  OperationBody,
  OperationInput,
  OperationOutput,
  OperationQuery,
  RouteBody,
  RouteInput,
  RouteOutput,
  RouteParameter,
} from "./generated/api";

type ListInput = OperationInput<"listTodos">;
type ListOutput = OperationOutput<"listTodos">;
type ListQuery = OperationQuery<"listTodos">;
type CreateBody = OperationBody<"createTodo">;

type UpdateInput = RouteInput<"PATCH /todos/{todoID}">;
type UpdateOutput = RouteOutput<"PATCH /todos/{todoID}">;
type UpdateBody = RouteBody<"PATCH /todos/{todoID}">;
type TodoID = RouteParameter<
  "PATCH /todos/{todoID}",
  "path",
  "todoID"
>;
```

## 요청 및 응답 helper

| 값 | Operation helper | Route helper |
| --- | --- | --- |
| 전체 호출 입력 | `OperationInput` | `RouteInput` |
| 성공 응답 | `OperationOutput` | `RouteOutput` |
| request body | `OperationBody` | `RouteBody` |
| path 파라미터 | `OperationPath` | `RoutePath` |
| query 파라미터 | `OperationQuery` | `RouteQuery` |
| query-string 파라미터 | `OperationQuerystring` | `RouteQuerystring` |
| header | `OperationHeaders` | `RouteHeaders` |
| cookie | `OperationCookies` | `RouteCookies` |
| 파라미터 하나 | `OperationParameter` | `RouteParameter` |
| 전체 계약 | `OperationContract` | `RouteContract` |

파라미터 helper에는 location과 파라미터 이름을 전달합니다.

```ts
type Limit = OperationParameter<"listTodos", "query", "limit">;
```

선택 속성과 스키마에 선언된 `null`은 보존됩니다. 전체 호출에서 요청 영역을
생략할 수 있는지는 `OperationInput` 또는 `RouteInput`으로 확인합니다.

## Component 타입

`components.schemas`에 선언된 스키마에는 component helper를 사용합니다.

```ts
import type { ComponentInput, ComponentOutput } from "./generated/api";

type TodoInput = ComponentInput<"Todo">;
type TodoOutput = ComponentOutput<"Todo">;
```

입력 타입에서는 `readOnly` 필드가 제외되고 출력 타입에서는 `writeOnly` 필드가
제외됩니다.

## Enum 값과 타입

Component enum은 TypeScript 타입과 런타임 값을 제공합니다.

```yaml
components:
  schemas:
    TodoStatus:
      type: string
      enum: [TODO, DONE]
```

```ts
import { Enums, isEnumValue, type EnumValue } from "./generated/api";

type TodoStatus = EnumValue<"TodoStatus">;
// "TODO" | "DONE"

const defaultStatus: TodoStatus = Enums.TodoStatus.TODO;
const completedStatus = Enums.TodoStatus.DONE;

for (const status of Enums.TodoStatus) {
  console.log(status);
}

const options = Array.from(Enums.TodoStatus);

declare const input: unknown;

if (isEnumValue(Enums.TodoStatus, input)) {
  input satisfies TodoStatus;
}
```

`Enums`에는 component 스키마로 선언된 enum이 포함됩니다. Inline enum과 중첩
enum은 생성된 요청, 응답, component 타입에서 사용할 수 있습니다.

## 추가 제공 타입

생성된 진입점은 raw 호출, resource tree 호출, pagination, Link, stream 타입도
제공합니다.

| 타입 | 용도 |
| --- | --- |
| `OperationMethod<Route>` | 생성된 operation 호출 |
| `OperationRawCall<Route>` | raw operation 호출 |
| `ResourceCall<Route>` | resource tree 호출 |
| `RawCall<Route>` | raw resource tree 호출 |
| `PaginateCall<Route>` | pagination 호출 |
| `LinkCalls<Route>` | OpenAPI Link 호출 |
| `StreamCall<Route>` | stream 호출 |
| `CursorPaginationInput` | cursor pagination 입력 |
| `OffsetPaginationInput` | offset pagination 입력 |
| `BothPaginationInput` | cursor 또는 offset pagination 입력 |
| `SortDirection` | `"asc" | "desc"` 타입과 런타임 상수 |

`Components`, `Operations`, `Routes`는 생성된 전체 타입 map을 제공합니다.
