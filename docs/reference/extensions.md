# SDK extensions

Standard OpenAPI is enough to generate an SDK. Use the `x-*` fields on this
page only when you need a convenience that OpenAPI cannot express directly.

openapi-sdkgen validates every supported extension before writing code. An
invalid declaration stops generation instead of being ignored or guessed.

## Behavior without extensions

- An API without an `operationId` is available through
  `api.$routes["METHOD /path"]`.
- Query, header, cookie, and path parameters keep their OpenAPI names.
- Schema constraints such as `required`, `minimum`, `pattern`, and `enum`
  apply to generated request and response validation.
- Unknown `x-*` fields remain available in metadata but do not change SDK
  behavior.

Declare filters as query parameters. Declare `If-Match` and `Idempotency-Key`
as header parameters. `x-filter`, `x-concurrency`, and `x-idempotency` do not
have special behavior.

## `x-envelope`

Return the `data` property from successful responses.

```yaml
x-envelope: data
```

Every successful JSON response with a body must be an object with a `data`
property. A normal call returns `data`; `.raw()` returns the complete decoded
response.

Omit `x-envelope` when the complete response should be returned.

## `x-pagination`

Generate a `.paginate()` method for an API. Without this extension, the normal
API is still generated but `.paginate()` is not added.

### Default form

The value is `cursor`, `offset`, or `both`.

```yaml
x-pagination: cursor
```

| Mode | Required query parameters |
| --- | --- |
| `cursor` | string `cursor`, positive integer `limit` |
| `offset` | non-negative integer `offset`, positive integer `limit` |
| `both` | `cursor`, `offset`, and `limit` |

Successful JSON responses must use one of these structures:

| Response shape | Items | Pagination data |
| --- | --- | --- |
| Root collection | `/items` | `/pagination/*` |
| Nested collection | `/data/items` | `/data/pagination/*` |
| `data` array | `/data` | `/meta/pagination/*` |

For cursor pagination, `nextCursor` must be a string or `null`. Offset
pagination may use `offset`, `limit`, and `total`, with their valid ranges
declared in the schema.

### Custom parameter names and response paths

Map custom query parameter names and JSON Pointers from the decoded response.

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

`mode` and `items` are required. Cursor mode also requires request `cursor` and
response `nextCursor`. Offset mode requires request `offset` and `limit`.

Pagination ends when the next cursor is absent or repeated, or when an offset
page is empty or reaches the final item.

## `x-sort`

Declare `x-sort` on the query parameter used for sorting.

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

The schema must be an array of unique `field:asc` or `field:desc` enum values.
The generated client accepts values such as:

```ts
{ field: "createdAt", direction: "desc" }
```

`x-sort` is not supported on Webhooks or Callbacks.

## `x-sdk-visibility`

Control how an API appears in the generated client.

```yaml
x-sdk-visibility: internal
```

- `internal`: keep the API in `$routes` and `$operations`, but omit its
  resource method.
- `hidden`: omit the API and related client methods.

Omitting the extension generates a normal public API.

## `x-error-category`

Add a static error category when the outer `error` object has an exact `code`
but no `category`.

```yaml
x-error-category: validation
```

When the schema already declares a required `category`, that value takes
precedence. Conflicting declarations produce an error.

See [Generate an SDK](../guide/generate.md) for generation errors and CI use.
