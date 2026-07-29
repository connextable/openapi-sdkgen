# SDK extension

표준 OpenAPI 문서만으로 TypeScript SDK를 생성할 수 있습니다.
openapi-sdkgen extension, `operationId`, `/v1` 루트 server, operation별
security 선언은 필수가 아닙니다.

Extension은 OpenAPI만으로 표현할 수 없는 SDK 편의 기능을 명시적으로
활성화합니다. 인식하는 extension이 존재하면 openapi-sdkgen은 source를
생성하기 전에 전체 계약을 검증합니다. 잘못된 선언을 무시하거나 추측해서
처리하지 않습니다.

## 기본 동작

- `operationId`가 없는 operation도 `api.$routes["METHOD /path"]`로 호출할 수 있습니다.
- query, header, cookie, path parameter는 표준 Parameter Object와 정확한 이름을 사용합니다.
- `required`, `minimum`, `pattern`, `enum` 같은 request/response 제약은 생성 코드의
  runtime validation 규칙으로 유지됩니다.
- 알 수 없는 third-party `x-*`는 `metadata.ts`에 보존되며 SDK 동작에는 영향을 주지 않습니다.
- `x-filter`, `x-concurrency`, `x-idempotency`는 의미를 부여하지 않는 vendor metadata입니다.
  filter는 query parameter로, `If-Match`와 `Idempotency-Key`는 표준 header parameter로 선언하세요.

생성 코드의 검증은 SDK 호출과 생성된 inbound adapter를 보호합니다. API 제공 서버는
authorization, 상태, 재고 같은 business rule을 포함해 독립적으로 검증해야 합니다.

## `x-envelope`

위치: 일반 Paths Operation Object.

허용 값은 `data`뿐입니다. body가 있는 모든 성공 JSON response는 선언된
`data` property를 가진 object여야 합니다.

```yaml
x-envelope: data
```

일반 호출은 `data` member를 반환합니다. `.raw()`는 metadata를 포함한 전체 decoded body를
유지하고 pagination도 이 전체 body를 읽습니다. Extension이 없으면 일반 호출도 전체 body를 반환합니다.

`x-envelope: none`을 쓰지 말고 extension을 생략하세요.

## `x-pagination`

위치: 일반 Paths Operation Object.

Extension이 없으면 operation은 평범하게 생성되고 `.paginate()` helper가 생기지 않습니다.
Pagination helper를 사용하더라도 선언된 query parameter와 response schema가 기준입니다.

### 문자열 shorthand

허용 값은 `cursor`, `offset`, `both`입니다. Mode에 따라 다음 표준 query 이름이 필요합니다.

- cursor: string `cursor`, positive integer `limit`
- offset: non-negative integer `offset`, positive integer `limit`
- both: 세 control 모두

모든 성공 JSON response는 아래 layout 중 하나를 동일하게 사용해야 합니다.

| Layout | Items | Pagination metadata |
|---|---|---|
| Root collection | `/items` | `/pagination/*` |
| Nested collection | `/data/items` | `/data/pagination/*` |
| Data-array envelope | `/data` | `/meta/pagination/*` |

Cursor metadata의 `nextCursor`는 string 또는 null schema입니다. Offset metadata는
`offset`, `limit`, `total`을 사용할 수 있습니다. Offset과 total은 non-negative integer,
limit은 positive integer여야 하며 schema에 해당 bound가 선언되어야 합니다.

```yaml
x-pagination: cursor
```

불완전하거나 layout이 섞였거나 모호하면 shorthand를 사용할 수 없습니다. 다른 이름이나
response 구조는 object form으로 선언하세요.

### 명시적 object form

Object form은 정확한 query parameter 이름과 전체 decoded response body 기준의
RFC 6901 JSON Pointer를 연결합니다.

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

`mode`는 `cursor`, `offset`, `both` 중 하나입니다. `items`는 항상 필요합니다.
Cursor mode는 request `cursor`와 response `nextCursor`가 필요하고, offset mode는
request `offset`, `limit`이 필요합니다. 선택적인 offset response pointer가 없으면 현재
request 값을 fallback으로 사용합니다. 빈 pointer는 response body root를 가리킵니다.

`both`의 `.paginate({mode: "cursor" | "offset", ...})`에서 top-level `mode`는 helper 전용입니다.
실제 query parameter 이름이 `mode`라면 `query.mode`에 그대로 남고 그대로 전송됩니다.

Iterator는 filter, sort input, request option, 초기 control을 보존합니다. Cursor가 없거나
null, 빈 문자열, 반복 값이면 종료합니다. Offset page가 비었거나 짧거나 total에 도달했거나
반복 또는 진행하지 않으면 종료합니다.

## `x-sort`

위치: 일반 Paths operation에서 변환할 정확한 query Parameter Object. Webhook과
callback parameter는 schema에서 그대로 파생하며 이 client projection을 허용하지 않습니다.

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

Schema는 `field:asc` 또는 `field:desc` 형태의 고유한 string enum array여야 합니다.
생성 input은 연관관계를 보존한 `{field, direction}` union이 됩니다. Runtime은 이를
선언된 enum으로 변환한 다음 표준 schema validation과 Parameter serialization을 적용합니다.
Operation-level 및 inbound-only webhook/callback `x-sort` 선언은 잘못된 선언입니다.

## `x-sdk-visibility`

위치: 일반 Paths Operation Object.

허용 값:

- `internal`: exact route와 operation-ID catalog는 유지하고 path resource tree에서는 제외
- `hidden`: operation과 이에 의존하는 client helper를 모두 제외

Extension이 없으면 public입니다. `x-sdk-visibility: public`을 쓰지 마세요.

## `x-error-category`

위치: operation error response에서 도달 가능한 인식된 outer error-envelope component schema.

인식하는 error shape는 exact `code`를 가진 required outer `error` object입니다.
Nested error object에 `category` property가 없을 때
`x-error-category: value`가 해당 schema의 모든 exact code에 non-empty static category를 제공합니다.

Required nested `category`의 string `const` 또는 single-value `enum`이 wire 기준입니다.
같은 extension은 redundant warning이고, 충돌하면 error입니다. Optional, non-string,
multi-value wire category는 extension으로 override할 수 없습니다.

## Diagnostic과 migration

별도 `validate` command는 없으며 `generate`가 유일한 validation workflow입니다.
발견 가능한 warning과 error를 phase/source별로 한 번 출력합니다. Warning만 있으면
생성하고, error가 있으면 새 output을 만들지 않으며 기존 output도 byte 단위로 보존합니다.

기존 문서를 옮길 때:

- `x-envelope: none`, `x-sdk-visibility: public` 제거
- placeholder `x-concurrency`, `x-idempotency` 필수 선언 제거
- `If-Match`, `Idempotency-Key`를 Header Parameter Object로 선언하고 정확한
  `headerParams` key로 전달
- 기존 `RequestOptions.ifMatch`, `RequestOptions.idempotencyKey` 사용을 header parameter로 교체
- 모든 operation은 `Routes`/`$routes`에서 접근하고, 명시적 operation ID가 있는 경우에만
  `Operations`/`$operations` exact alias 사용
- exported runtime helper를 직접 사용한다면 `createPaginator`에 profile string 대신
  검증된 `PaginationPlan` 전달

Diagnostic과 CI 동작은 [SDK 생성](../guide/generate.md)을 참고하세요.
