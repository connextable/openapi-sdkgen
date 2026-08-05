# 생성된 TypeScript 타입

생성된 SDK의 기본 진입점은 component, route, operation을 기준으로 타입을
제공합니다. Feature 코드는 요청과 응답 타입을 다시 선언하지 않고 런타임에
호출하는 생성 메서드에서 같은 타입을 추출할 수 있습니다.

런타임 호출, 인증, 오류, Link, 스트림은
[생성된 클라이언트 API](./client-api.md)에서 확인할 수 있습니다.

## 타입 기준 선택

소비자 코드가 이미 가지고 있는 식별 기준에 맞춰 helper를 선택합니다.

| 사용할 수 있는 기준 | Helper 종류 | 주 용도 |
| --- | --- | --- |
| 생성된 exact 또는 resource 메서드 | `Operation*<typeof method>` | 생성된 클라이언트 메서드를 직접 사용하는 feature 코드 |
| 정확한 OpenAPI `operationId` | `Operation*<"operationId">` | 안정적인 operation ID로 공유하는 타입 |
| 정확한 `"METHOD /path"` | `Route*<"METHOD /path">` | `operationId`가 없는 route 또는 route 중심 인프라 |
| component 스키마 이름 | `ComponentInput<Name>` / `ComponentOutput<Name>` | 여러 operation이 공유하는 도메인 타입 |
| 전체 생성 catalog | `Components`, `Routes`, `Operations` | 모든 계약 슬롯을 다루는 범용 도구 |

일반적인 feature 코드에서는 catalog를 직접 인덱싱하고 TypeScript utility type으로
선택 값을 정규화하는 것보다 용도별 helper가 읽기 쉽습니다.

## 생성된 메서드에서 타입 추출

생성된 메서드에 `typeof`를 적용하고 그 메서드 타입을 `Operation*` helper에
전달합니다. 세 가지 생성 클라이언트 표면의 메서드를 모두 사용할 수 있습니다.

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

타입 추출은 컴파일 타임에만 동작합니다. 메서드에 wrapper나 런타임 메타데이터를
추가하지 않습니다.

### Exact 메서드와 bound 메서드

`$operations`와 `$routes`는 exact 메서드를 제공합니다. 리소스 트리의 leaf는 상위
selector에서 path 파라미터가 이미 바인딩되었을 수 있습니다.

| 메서드 값 | `OperationInput` 결과 |
| --- | --- |
| `api.$operations.updateTodo` | `path`와 `body`를 포함한 전체 입력 |
| `api.$routes["PATCH /todos/{todoID}"]` | `path`와 `body`를 포함한 전체 입력 |
| `api.todos("todo-1").update` | `path` 없이 `body`만 포함한 bound 입력 |

선택된 호출 표면은 `OperationContract<Source>`에도 적용됩니다. `input`과 `call`
슬롯은 `Source`로 전달한 exact 메서드 또는 bound resource 메서드를 나타냅니다.

```ts
import type { OperationContract } from "./generated/api";

type ExactContract = OperationContract<typeof exactUpdate>;
type BoundContract = OperationContract<typeof updateTodo>;
```

리소스 메서드에는 남은 path 입력이 없으므로
`OperationPath<typeof updateTodo>`는 거부됩니다. Raw, pagination, Link,
stream 함수는 독립 operation source가 아니라 기능 호출입니다. 이 타입이
필요하다면 route 기반 호출 타입을 사용합니다.

### Feature 코드에서 사용

추출한 타입은 OpenAPI 구조를 반복하지 않고 상태, 함수 인자, `satisfies` 표현식에
사용할 수 있습니다.

```ts
const filters = { completed: false } satisfies TodoFilters;

async function update(body: OperationBody<typeof updateTodo>) {
  return updateTodo({ body });
}
```

클라이언트 값 없이 `Client` 타입만 사용할 수 있다면 메서드 타입을 직접
인덱싱합니다.

```ts
import type { Client, OperationInput } from "./generated/api";

type UpdateInput = OperationInput<Client["$operations"]["updateTodo"]>;
```

## Operation ID로 추출

명시적인 `operationId`가 있는 모든 non-hidden operation은 `Operation*`의 문자열
source로 사용할 수 있습니다.

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

문자열은 정확한 operation ID여야 합니다. Route 문자열은 operation ID가 아니므로
`Operation*`에 전달할 수 없습니다. Route 문자열에는 `Route*` helper를 사용합니다.

## Route로 추출

Route helper는 정확한 `"METHOD /openapi/path"` 문자열을 식별자로 사용합니다.
OpenAPI에 `operationId`가 있는지와 관계없이 생성된 route를 다룹니다.

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

### Route 계약 슬롯

`RouteContract<Route>`는 route의 전체 계약을 공개 타입 이름으로 제공합니다.

| 슬롯 | 의미 | 용도별 helper |
| --- | --- | --- |
| `input` | exact 호출의 전체 입력 | `RouteInput<Route>` |
| `resourceInput` | 리소스 path 바인딩 후 입력 | `RouteResourceInput<Route>` |
| `options` | 요청별 전송 옵션 | `RouteOptions<Route>` |
| `output` | 디코딩된 성공 출력 | `RouteOutput<Route>` |
| `error` | 생성된 오류 응답 유니언 | `RouteContract<Route>["error"]` |
| `rawResponse` | 성공한 raw response 유니언 | `RouteRawResponse<Route>` |
| `call` | exact operation 메서드 | `OperationMethod<Route>` |
| `resourceCall` | resource 중심 메서드 | `ResourceCall<Route>` |
| `pagination` | 지원되는 경우 pagination iterator | `PaginateCall<Route>` |
| `links` | 지원되는 경우 response Link 호출 | `LinkCalls<Route>` |
| `stream` | 지원되는 경우 stream 호출 | `StreamCall<Route>` |

여러 슬롯을 같은 route에 묶어야 한다면 전체 계약을 사용합니다. 슬롯 하나만
필요하다면 용도별 helper를 사용합니다. Route가 기능을 선언하지 않았다면 해당
기능 슬롯은 `never`가 됩니다.

## 요청 영역과 파라미터

영역 helper는 호출자가 사용하는 요청 영역 전체를 반환합니다. 파라미터 helper는
path, query, query-string, header, cookie 영역에서 값 하나를 선택합니다.

| 입력 영역 | Operation helper | Route helper | Parameter location |
| --- | --- | --- | --- |
| request body | `OperationBody` | `RouteBody` | 해당 없음 |
| path 파라미터 | `OperationPath` | `RoutePath` | `"path"` |
| query 파라미터 | `OperationQuery` | `RouteQuery` | `"query"` |
| query-string 파라미터 | `OperationQuerystring` | `RouteQuerystring` | `"querystring"` |
| header 파라미터 | `OperationHeaders` | `RouteHeaders` | `"header"` |
| cookie 파라미터 | `OperationCookies` | `RouteCookies` | `"cookie"` |

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

Generic constraint는 요청한 영역이 없는 source, 유효하지 않은 location, 존재하지
않는 파라미터 이름을 거부합니다. Body 필드는 OpenAPI Parameter Object가 아니라
스키마 속성이므로 `"body"`는 파라미터 location이 아닙니다.

### Optional과 nullable

영역 helper는 aggregate 입력 필드가 선택 사항이어서 생긴 바깥쪽 `undefined`만
제거합니다. 영역 내부의 선택 속성은 그대로 선택 사항입니다.

개별 파라미터 helper는 생략을 뜻하는 `undefined`만 제거하고 스키마에 선언된
`null`은 보존합니다.

```ts
type Query = OperationQuery<"listTodos">;
// 선택 속성이라면 Query["search"]는 string | null | undefined입니다.

type Search = OperationParameter<"listTodos", "query", "search">;
// Search는 string | null입니다.
```

전체 호출에서 영역을 생략할 수 있는지는 `OperationInput`이나 `RouteInput`을
기준으로 판단합니다.

## Component 입출력 타입

Component helper는 `components.schemas`의 정확한 이름을 사용하며 입력과 출력
projection을 나눠 제공합니다.

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

`readOnly` 필드는 입력 projection에서 제외되고 `writeOnly` 필드는 출력
projection에서 제외됩니다. 필수, 선택, nullable, literal, union, 배열, 객체
구조는 OpenAPI 스키마를 따릅니다.

Inline operation 스키마는 `Components` 항목을 만들지 않습니다. 대응하는 operation
또는 route의 input, output, body, 영역 helper를 통해 추출합니다.

## Enum 값과 타입

최상위 스키마에 `enum`을 선언한 생성 component 스키마는 타입과 런타임 값 목록을
함께 제공합니다.

다음 OpenAPI component가 있다면:

```yaml
components:
  schemas:
    TodoStatus:
      type: string
      enum: [TODO, DONE]
```

컴파일 타임과 런타임에서 정확한 값을 사용할 수 있습니다.

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

`Enums.TodoStatus`는 readonly tuple 타입을 가진 런타임 배열입니다. 값의 OpenAPI
순서와 정확한 JSON literal을 보존합니다. TypeScript enum 멤버 이름으로
정규화하지 않으므로 `Enums.TodoStatus.DONE`이 아니라 값 자체를 사용합니다.

Component 이름도 그대로 보존합니다. 필요한 경우 대괄호 표기법을 사용합니다.

```ts
type DashedTodoStatus = ComponentOutput<"todo-status">;
type DashedTodoStatusValue = (typeof Enums["todo-status"])[number];
```

런타임 catalog는 문자열, 숫자, boolean, `null`, 배열, 객체를 포함한 JSON enum
값을 지원합니다. 생성된 tuple 타입은 literal 값, 객체 속성, 배열 순서를
보존합니다.

Component 최상위 스키마에 `enum`이 있는 경우에만 `Enums` 항목이 생성됩니다.
중첩 속성이나 inline enum도 포함 타입에서는 literal union으로 생성되지만 별도의
런타임 catalog 항목은 생기지 않습니다. Feature 코드에 재사용할 런타임 선택지가
필요하다면 enum을 component 스키마로 올립니다.

## 기능 및 유틸리티 타입

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

| 타입 또는 값 | 용도 |
| --- | --- |
| `OperationMethod<Route>` | `$operations`와 `$routes`에서 사용하는 exact 생성 메서드 타입 |
| `OperationRawCall<Route>` | exact raw 호출 메서드 |
| `ResourceCall<Route>` | 적용 가능한 path를 바인딩한 resource-tree 메서드 |
| `RawCall<Route>` | resource 중심 raw 호출 |
| `PaginateCall<Route>` | pagination iterator 호출 |
| `LinkCalls<Route>` | response Link 호출 |
| `StreamCall<Route>` | 스트리밍 응답 호출 |
| `CursorPaginationInput` | cursor와 limit 제어 값 |
| `OffsetPaginationInput` | offset과 limit 제어 값 |
| `BothPaginationInput` | cursor와 offset 방식을 섞지 않는 제어 값 |
| `SortDirection` | `"asc" | "desc"` 타입과 대응 런타임 상수 |

```ts
type UpdateCall = ResourceCall<"PATCH /todos/{todoID}">;
type UpdateRaw = RawCall<"PATCH /todos/{todoID}">;
type ListPages = PaginateCall<"GET /todos">;

const direction: SortDirection = SortDirection.DESC;
```

## 이름과 에디터 표시

생성된 operation ID, route, component 이름, 파라미터 이름은 OpenAPI의 철자와
대소문자를 그대로 보존합니다. TypeScript 식별자로 사용할 수 없는 이름에는
대괄호 표기법을 사용합니다.

공개 helper는 생성 구현 alias와 메서드 식별 brand를 공개 타입 이름 뒤에
숨깁니다. VS Code hover와 signature help에는 내부 `__sdkgen_*` 선언이 아니라
`OperationInput`, `RouteBody`, `ResourceCall` 같은 타입이 표시되어야 합니다.
