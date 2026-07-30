# SDK 확장 기능

표준 OpenAPI만으로 SDK를 생성할 수 있습니다. 이 페이지의 `x-*` 필드는
OpenAPI만으로 표현하기 어려운 편의 기능이 필요할 때만 사용하세요.

openapi-sdkgen이 지원하는 확장 필드가 있으면 선언 내용을 먼저 검사합니다.
잘못된 설정을 무시하거나 임의로 해석하지 않으며, 오류가 있으면 SDK를 생성하지
않습니다.

## 확장 필드 없이 사용할 수 있는 기능

- `operationId`가 없어도 `api.$routes["METHOD /path"]`로 API를 호출할 수
  있습니다.
- query, header, cookie, path 매개변수는 OpenAPI에 선언된 이름을 그대로
  사용합니다.
- `required`, `minimum`, `pattern`, `enum` 같은 스키마 제약은 생성된 코드의
  요청·응답 검사에 반영됩니다.
- openapi-sdkgen이 알지 못하는 `x-*` 필드는 메타데이터에 보존되지만 SDK 동작을
  바꾸지는 않습니다.

필터는 query 매개변수로, `If-Match`와 `Idempotency-Key`는 header 매개변수로
선언하세요. `x-filter`, `x-concurrency`, `x-idempotency`에는 별도 동작을
부여하지 않습니다.

## `x-envelope`

성공 응답의 `data` 속성만 일반 호출의 반환값으로 사용합니다.

```yaml
x-envelope: data
```

이 확장을 사용하려면 본문이 있는 모든 성공 JSON 응답이 `data` 속성을 가진
object여야 합니다. `.raw()`는 나머지 메타데이터를 포함한 전체 응답 본문을
반환합니다.

전체 응답을 그대로 받으려면 `x-envelope`를 생략하세요.

## `x-pagination`

페이지 순회를 돕는 `.paginate()` 메서드를 생성합니다. 확장을 생략하면 API는
그대로 생성되지만 `.paginate()`는 추가되지 않습니다.

### 기본 형식

값은 `cursor`, `offset`, `both` 중 하나입니다.

```yaml
x-pagination: cursor
```

각 방식에 필요한 query 매개변수는 다음과 같습니다.

| 방식 | 필요한 query 매개변수 |
| --- | --- |
| `cursor` | 문자열 `cursor`, 양의 정수 `limit` |
| `offset` | 0 이상의 정수 `offset`, 양의 정수 `limit` |
| `both` | `cursor`, `offset`, `limit` |

성공 JSON 응답은 다음 구조 중 하나를 사용해야 합니다.

| 응답 구조 | 항목 | 페이지 정보 |
| --- | --- | --- |
| 루트 목록 | `/items` | `/pagination/*` |
| 중첩 목록 | `/data/items` | `/data/pagination/*` |
| `data` 배열 | `/data` | `/meta/pagination/*` |

cursor 방식의 `nextCursor`는 문자열 또는 `null`이어야 합니다. offset 방식은
`offset`, `limit`, `total`을 사용할 수 있으며 스키마에도 각 값의 범위를
선언해야 합니다.

### 매개변수와 응답 경로 직접 지정

다른 이름이나 응답 구조를 사용한다면 query 매개변수 이름과 응답 본문의 JSON
Pointer를 직접 연결합니다.

```yaml
x-pagination:
  mode: both
  request:
    cursor: cursorToken
    offset: pageOffset
    limit: pageSize
  response:
    items: /payload/rows
    nextCursor: /payload/page/next
    offset: /payload/page/offset
    limit: /payload/page/limit
    total: /payload/page/total
```

`mode`와 `items`는 필수입니다. cursor 방식은 요청의 `cursor`와 응답의
`nextCursor`가 필요합니다. offset 방식은 요청의 `offset`과 `limit`이
필요합니다.

`.paginate()`는 다음 cursor가 없거나 같은 값이 반복될 때, 또는 offset
페이지가 비었거나 마지막 항목에 도달했을 때 순회를 끝냅니다.

## `x-sort`

정렬에 사용하는 query 매개변수에 선언합니다.

```yaml
- name: sort
  in: query
  schema:
    type: array
    items:
      type: string
      enum: [name:asc, name:desc, createdAt:asc, createdAt:desc]
  x-sort:
    format: field-direction
```

스키마는 `field:asc` 또는 `field:desc` 형식의 고유한 문자열 enum 배열이어야
합니다. 생성된 클라이언트에서는 다음과 같은 값으로 전달할 수 있습니다.

```ts
{ field: "createdAt", direction: "desc" }
```

Webhook과 Callback에는 `x-sort`를 사용할 수 없습니다.

## `x-sdk-visibility`

클라이언트에서 API를 노출할 방법을 지정합니다.

```yaml
x-sdk-visibility: internal
```

- `internal`: `$routes`와 `$operations`에서는 호출할 수 있지만 리소스
  메서드에서는 제외합니다.
- `hidden`: API와 관련 클라이언트 메서드를 생성하지 않습니다.

확장을 생략하면 일반 공개 API로 생성됩니다.

## `x-error-category`

오류 응답의 `error` object에 정확한 `code`가 있고 `category`가 없을 때 정적인
오류 범주를 추가합니다.

```yaml
x-error-category: validation
```

스키마에 이미 필수 `category`가 선언되어 있다면 그 값이 우선합니다. 서로 다른
값을 중복으로 선언하면 오류가 발생합니다.

## 이전 설정 정리

이전 버전의 설정을 사용하고 있다면 다음 항목을 확인하세요.

- `x-envelope: none`, `x-sdk-visibility: public`은 제거합니다.
- `x-concurrency`, `x-idempotency` 대신 `If-Match`,
  `Idempotency-Key` header 매개변수를 선언합니다.
- 모든 API는 `Routes`와 `$routes`에 생성됩니다. `operationId`가 있는
  경우에만 `Operations`와 `$operations`에도 생성됩니다.

생성 오류와 CI 사용법은 [SDK 생성](../guide/generate.md)을 참고하세요.
