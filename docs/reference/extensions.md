# SDK extensions

Standard OpenAPI is the complete baseline contract. A document does not need
any openapi-sdkgen extension, an `operationId`, a root `/v1` server, or an
explicit per-operation security declaration to generate a TypeScript SDK.

Extensions opt into conveniences that OpenAPI cannot describe on its own. When
a recognized extension is present, openapi-sdkgen validates its whole contract
before emitting source. An invalid declaration is an error; it is never ignored
or guessed.

## Baseline behavior

- Operations without `operationId` remain callable through
  `api.$routes["METHOD /path"]`.
- Query, header, cookie, and path parameters use their standard Parameter
  Objects and exact names.
- Request and response constraints such as `required`, `minimum`, `pattern`,
  and `enum` remain runtime validation rules in generated code.
- Unknown third-party `x-*` fields are preserved in `metadata.ts` and otherwise
  have no effect.
- `x-filter`, `x-concurrency`, and `x-idempotency` are ordinary inert vendor
  metadata. Declare filters as query parameters and declare `If-Match` or
  `Idempotency-Key` as standard header parameters.

The API provider must still validate independently. Generated validation
protects SDK calls and generated inbound adapters; it does not replace server
authorization, state, inventory, or other business rules.

## `x-envelope`

Location: an ordinary Paths Operation Object.

The only accepted value is `data`. Every body-bearing successful JSON response
must be an object with a declared `data` property.

```yaml
x-envelope: data
```

The ordinary call returns the projected `data` member. `.raw()` retains the
complete decoded body, including metadata, and pagination reads that complete
body. Without `x-envelope`, the ordinary call returns the complete body.

Do not write `x-envelope: none`; omit the extension.

## `x-pagination`

Location: an ordinary Paths Operation Object.

Without this extension, the operation is generated normally and has no
`.paginate()` helper. Pagination is an SDK convenience only; the declared query
parameters and response schemas remain authoritative.

### String shorthand

Accepted values are `cursor`, `offset`, and `both`. Shorthand requires the
standard query names used by its mode:

- cursor: `cursor` as a string and `limit` as a positive integer
- offset: `offset` as a non-negative integer and `limit` as a positive integer
- both: all three controls

Every successful JSON response must use the same one of these layouts:

| Layout | Items | Pagination metadata |
|---|---|---|
| Root collection | `/items` | `/pagination/*` |
| Nested collection | `/data/items` | `/data/pagination/*` |
| Data-array envelope | `/data` | `/meta/pagination/*` |

Cursor metadata uses `nextCursor` with a string-or-null schema. Offset metadata
may use `offset`, `limit`, and `total`; offset and total are non-negative
integers, and limit is a positive integer. Schema bounds must express those
rules.

```yaml
x-pagination: cursor
```

Shorthand rejects incomplete, mixed, or ambiguous layouts. Use the object form
for any other names or response shape.

### Explicit object form

The object form maps exact query parameter names and RFC 6901 JSON Pointers into
the complete decoded response body:

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

`mode` is `cursor`, `offset`, or `both`. `items` is always required. Cursor
mode requires request `cursor` and response `nextCursor`; offset mode requires
request `offset` and `limit`. Optional offset response pointers fall back to
the current request values. An empty pointer addresses the response-body root.

For `both`, `.paginate({mode: "cursor" | "offset", ...})` uses a helper-only
top-level discriminator. A real query parameter named `mode` remains under
`query.mode` and is sent unchanged.

The iterator preserves filters, sort input, request options, and initial
controls. It stops on absent, null, empty, or repeated cursors and on empty,
short, total-complete, repeated, or non-progressing offset pages.

## `x-sort`

Location: the exact query Parameter Object being transformed on an ordinary
Paths operation. Webhook and callback parameters remain schema-derived and do
not accept this client projection.

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

The schema must be an array of unique string enum values in `field:asc` or
`field:desc` form. Generated input becomes a correlated
`{field, direction}` union. The runtime converts it back to the declared enum,
then applies the standard schema validation and Parameter serialization rules.
Operation-level and inbound-only webhook/callback `x-sort` declarations are
invalid.

## `x-sdk-visibility`

Location: an ordinary Paths Operation Object.

Accepted values:

- `internal`: keep exact route and explicit operation-ID catalogs, but omit the
  path resource tree entry
- `hidden`: omit the operation and its dependent client helpers

Absence means public. Do not write `x-sdk-visibility: public`.

## `x-error-category`

Location: a recognized outer error-envelope component schema reachable from an
operation error response.

The recognized shape has a required outer `error` object with an exact `code`.
When the nested error object has no `category` property,
`x-error-category: value` supplies a static non-empty category for all exact
codes contributed by that schema.

A required nested `category` string `const` or single-value `enum` is the wire
source of truth. An equal extension is redundant and warns; a conflicting
extension errors. Optional, non-string, or multi-valued wire categories cannot
be overridden.

## Diagnostics and migration

`generate` is the only validation workflow; there is no separate `validate`
command. It reports every discoverable warning and error once, grouped by phase
and source. Warnings still emit. Errors leave an absent output absent and a
pre-existing output byte-for-byte unchanged.

When migrating older documents:

- remove `x-envelope: none` and `x-sdk-visibility: public`
- remove mandatory placeholder `x-concurrency` and `x-idempotency` declarations
- declare `If-Match` and `Idempotency-Key` as Header Parameter Objects and pass
  them through exact `headerParams` keys
- replace legacy `RequestOptions.ifMatch` and
  `RequestOptions.idempotencyKey` usage with those header parameters
- use `Routes`/`$routes` for every operation; `Operations`/`$operations` remain
  exact aliases only for explicitly declared operation IDs
- if calling the exported runtime helper directly, pass a validated
  `PaginationPlan` to `createPaginator` instead of a profile string

See [Generate an SDK](../guide/generate.md) for diagnostic and CI behavior.
