# OpenAPI SDK Implementation Roadmap

This document records the planned path from the current capability matrix to
full OpenAPI 3.0.x, 3.1.x, and 3.2.x SDK generation. It complements the
[capability matrix](openapi-feature-matrix.md): the matrix is the current
field-level contract, while this document defines the implementation order and
the runtime APIs needed to turn current feature-path diagnostics into generated
behavior.

## Goal

Parse every valid OpenAPI 3.x document according to the semantics of its
declared version and generate a correct TypeScript SDK surface.
Generated code must not silently drop a valid construct. A feature remains an
explicit diagnostic until its parser, type lowering, wire codec, runtime
behavior, and tests are all implemented.

The active and sole output target is TypeScript source mode, including the
optional TypeScript inbound-server add-on. A future language target may consume
the same normalized IR only after the TypeScript contract and conformance suite
are complete; it must not drive or weaken the TypeScript design.

## Current Boundary

The current generator already supports fixed servers, standard operations,
resource and operation-ID APIs, the supported parameter styles, JSON/text/
binary/basic form bodies, status-aware responses, source-mode metadata, and
TypeScript inbound Webhook/Callback contracts. The exact present state is
authoritative in the [feature manifest](openapi-feature-manifest.json).

The work below is intentionally ordered by shared dependencies. Implementing a
surface feature before its resolver, schema validator, or wire codec would
create target-specific shortcuts and inconsistent behavior between OpenAPI
versions.

## Architecture Direction

```text
OpenAPI 3.0 / 3.1 / 3.2 document
        │
        ▼
version-aware resolver ──► normalized OpenAPI IR
        │                         │
        │                         ├── compiled schema IR
        │                         ├── parameter/media codec IR
        │                         ├── security requirement IR
        │                         └── response/link/stream IR
        ▼
TypeScript client target / TypeScript inbound-server add-on
```

The resolver and intermediate representations must be target-neutral even
while TypeScript is the only output target. This keeps a later target from
reimplementing OpenAPI semantics, while the public API is optimized for
TypeScript users now.

## Settled Contract Rules

- Generated clients validate values before encoding a request and validate a
  response after decoding it. A validation error contains the OpenAPI location,
  JSON Pointer, and relevant composition branch; invalid values are never
  silently coerced.
- `oneOf` succeeds only when exactly one branch validates. `anyOf` succeeds
  when one or more branches validate. `not`, conditionals, containment, and
  unevaluated-property rules are executable runtime semantics, not type-only
  annotations.
- An explicit `baseURL` always overrides OpenAPI Servers. Without `baseURL`,
  an operation server wins over a path server, which wins over a root server.
  Server variables are validated before URL expansion.
- Standard JSON, `+json`, text, binary, form, multipart, and XML codecs are
  generated. An unrecognized media type requires a caller-registered codec;
  it is never guessed as JSON or text.
- Streaming operations expose typed `AsyncIterable` values and accept
  `AbortSignal`. Raw Fetch streams stay available from the raw-response API.
  The SDK preserves Fetch pull-based backpressure, reports decode failures from
  iteration, and does not reconnect SSE automatically.
- Link-derived inputs are defaults. Explicit call inputs override them; an
  unresolved required value produces an error instead of an invented request.
- Credential acquisition, token storage, login, refresh, and certificate
  selection are host responsibilities. Generated code applies values to the
  declared security schemes and wire locations.
- Generated inbound endpoints use Fetch `Request` and `Response` only.
  Framework adapters remain outside generated output.
- Remote references are opt-in only. Default compilation resolves document and
  contained local-file references without network access.
- A declared custom JSON Schema vocabulary is compiled through an explicitly
  registered, trusted local extension. The generated SDK contains its compiled
  output and never invokes an extension at application runtime.

## Implementation Phases

### Phase 1: Reference and Schema Resource Resolution

Implement version-aware resolution for currently rejected reference features:

- JSON Schema `$id`, `$schema`, `$anchor`, `$dynamicAnchor`, `$dynamicRef`,
  `$defs`, `$vocabulary`, `$comment`, nested pointers, and boolean schemas.
- OpenAPI 3.2 `$self` and relative-reference base URI behavior.
- Schema reference siblings with wire semantics.
- External Path Item and reusable-object references, including deterministic
  handling for cycles outside recursive component schemas.

Required result:

- Every resolved schema has a stable resource URI, pointer, dialect, and
  reference target in the normalized IR.
- Cycles are either represented by the schema IR or rejected with one precise
  cycle diagnostic.
- Contained local-file references resolve from the canonical real path of the
  referring resource URI; the input root is a containment boundary, not the
  relative-resolution base. A `file:` URI, `..` traversal, or symlink may not
  resolve outside that canonical root.
- The resolver recognizes standard vocabularies and their required/optional
  flags. Unknown required vocabularies require a registered compiler extension;
  unknown optional vocabularies are preserved as annotations. `$comment` is
  preserved as metadata and never changes validation.
- The compiler never performs implicit network fetches.

Validation:

- 3.0, 3.1, and 3.2 fixture families for local, contained-file, anchor, and
  dynamic references.
- Valid/invalid URI-scope tests and deterministic artifact snapshots.

### Extension Protocol

Custom vocabularies are a first-class compile-time extension point. The CLI
accepts repeatable `--schema-extension <manifest>` options. Each checked-in
extension manifest maps one or more vocabulary URIs to a trusted local
executable and argument array; command strings are never interpreted by a
shell. The executable speaks versioned JSON-RPC over standard input/output.

An extension first declares its vocabulary URIs and a stable executable digest.
For each schema using that vocabulary, it receives a versioned JSON-RPC `lower`
request with the resolved schema fragment, JSON Pointer location, vocabulary
URI, and `typescript` target. It returns one replacement JSON Schema object or
boolean; the normal compiler pipeline validates and lowers that replacement
into generated source. The generator records the extension URI and digest in
the generation lockfile. It never downloads an extension, runs an extension at
application runtime, or silently treats a required vocabulary as an annotation.

`lower` does not accept executable source, TypeScript snippets, codecs, or
runtime callbacks. Custom semantics must be represented by the returned JSON
Schema fragment. Target-specific non-schema lowering remains a separate future
protocol revision, rather than an unvalidated code-injection surface.

An unavailable extension, URI/digest mismatch, unsupported target/context, or
invalid extension output is a feature-path diagnostic. An unknown optional
vocabulary remains preserved annotation metadata without running an extension.

Validation:

- Standard vocabulary, optional custom vocabulary, required custom vocabulary,
  missing extension, digest mismatch, and invalid-extension-output fixtures.
- A trusted fixture extension proving request, response, and inbound validation
  plus codec lowering; generated source must run without the extension process.

### Phase 2: Compiled JSON Schema Semantics

Add one target-neutral compiled-schema representation used for TypeScript type
lowering, outbound codecs, and inbound validation.

Coverage:

- The complete OAS 3.0 Schema Object: numeric, string, array, and object
  constraints; `nullable`; `readOnly`/`writeOnly` request and response
  projection; composition; discriminator; XML annotations; and examples.
- The complete JSON Schema 2020-12 Core, Applicator, Validation, Content, and
  Annotation vocabularies used by OAS 3.1/3.2, including `$vocabulary`,
  `$comment`, `const`, every numeric/string/array/object bound, `uniqueItems`,
  and `format`.
- `oneOf`, `anyOf`, `not`, `if`, `then`, `else`.
- `contains`, `minContains`, `maxContains`, `prefixItems`, and tuple semantics.
- `patternProperties`, `propertyNames`, `additionalProperties: false`,
  `unevaluatedItems`, `unevaluatedProperties`.
- `dependentSchemas`, `dependentRequired`, `contentSchema`, and boolean schema
  values.
- OpenAPI discriminator mappings, including version-appropriate default
  mappings.

`format` is annotation-only unless the active dialect declares the standard
format-assertion vocabulary. In assertion mode, each supported standard format
is validated; an unknown required assertion vocabulary is a feature-path
diagnostic rather than a guessed validation rule.

Required result:

- Generated types retain useful discriminated unions where the contract permits
  one.
- Runtime validation chooses and validates the same branch as the generated
  wire codec: `oneOf` requires exactly one match and `anyOf` requires at least
  one match.
- Validation failures include the OpenAPI/JSON Pointer path and branch context.

Validation:

- A conformance corpus with success and failure vectors for each keyword.
- A keyword ledger with request, response, and inbound vectors for every
  manifest schema feature and applicable OpenAPI version.
- Type-level assertions plus runtime request encode, response decode, and
  inbound validation parity tests for TypeScript output.

### Phase 3: Server Selection and Parameter Serialization

Extend the transport IR and client configuration for:

- Server URL variables and root/path/operation server alternatives.
- OpenAPI 3.2 `querystring` parameters and querystring-format extensions.
- `allowReserved`, `allowEmptyValue`, cookie style, delimited object values,
  and multi-media parameter `content`.
- Transport capability checks for ordinary cookie parameters and response
  headers that Fetch environments cannot expose, including `Set-Cookie`.

Required result:

- A generated operation describes its available servers and variable schema.
- The client validates selected variable values before issuing a request.
- Parameter serialization has one tested implementation per OpenAPI style and
  media-type rule.

Public contract:

```ts
const api = createClient({
  origin: "https://api.example.test",
  server: { id: "#/servers/1", variables: { region: "kr" } },
});
```

`baseURL` is an absolute explicit override and wins over every OpenAPI Server.
When absent, the selected operation server, then path server, then root server
supplies the base URL. A named server is selected through a generated stable
`server.id`, which is the source pointer and therefore exists for every OpenAPI
version; a 3.2 `name` is descriptive metadata only. Without an ID, the first
server in the applicable scope is selected. An omitted `servers` field inherits
its broader scope; an explicit empty array selects the OpenAPI default `/` at
that scope and does not inherit. Its variables are checked before expansion.
The applicable server array is selected first by operation, path, then root
scope; `server.id` must identify a member of that one array. A cross-scope or
unknown ID fails before credential lookup rather than overriding OpenAPI server
precedence. `origin` is an absolute origin used only to resolve a selected
relative server, including `/`; it never overrides a selected absolute server. A relative server
without `origin`, or a malformed `baseURL`/`origin`, fails before credential
lookup or Fetch.

`createClient` accepts a single optional transport adapter with a Fetch method
and declared capabilities for cookie-jar access, readable response headers, and
mutual TLS. The default adapter is the host Fetch implementation. A caller-
supplied ordinary cookie parameter always requires a cookie-jar adapter because
browser Fetch cannot serialize that value; existing same-origin browser cookies
can satisfy cookie *security* only. Generated response headers expose only
headers the active transport can read. An operation that requires `Set-Cookie`
or any other unreadable header fails with a typed transport-capability error
rather than producing `undefined`; a Node or custom adapter must explicitly
declare that capability.

### Phase 4: Media Types, Multipart, and XML

Build a media codec registry and multipart part plan for:

- Media Type and Encoding Objects, structured multipart fields, part headers,
  per-part media types, ordered/unnamed multipart, and 3.2 positional media.
- XML Object behavior, including 3.2 XML node/array/text/null rules.
- Reusable Component Media Types and custom runtime media codecs.

Required result:

- A generated request body identifies its exact codec and media selection API.
- Unknown media types require an explicitly registered host codec instead of
  being coerced into JSON or text.

The client accepts registered codecs by media type. Generated code owns the
OpenAPI media selection and calls the codec only after selecting the declared
media type. A codec cannot bypass schema validation or alter an unrelated
content type. Content-type matching is case-insensitive and ignores parameters;
generated exact media types win over structured `+json` handling and declared
wildcards. If the actual response content type has no declared or registered
codec, decoding fails. Codec registration cannot override a generated exact
codec.

Public contract:

```ts
interface MediaCodec<Value> {
  encode?(value: Value, context: { contentType: string }): BodyInit | Promise<BodyInit>;
  decode?(response: Response, context: { contentType: string }): Value | Promise<Value>;
  decodeInbound?(request: Request, context: { contentType: string }): Value | Promise<Value>;
  encodeParameter?(value: Value, context: { contentType: string }): string | Promise<string>;
  encodeStream?(
    items: AsyncIterable<Value>,
    context: { contentType: string; signal?: AbortSignal },
  ): ReadableStream<Uint8Array> | Promise<ReadableStream<Uint8Array>>;
  decodeStream?(
    reader: MediaStreamReader,
    context: { contentType: string; maxFrameBytes: number; signal?: AbortSignal },
  ): AsyncIterable<Value>;
  decodeInboundStream?(
    reader: MediaStreamReader,
    context: { contentType: string; maxFrameBytes: number },
  ): AsyncIterable<Value>;
}

interface MediaStreamReader {
  read(maxBytes: number): Promise<Uint8Array | null>;
  cancel(reason?: unknown): Promise<void>;
}

const api = createClient({
  codecs: {
    "application/vnd.acme.widget": widgetCodec,
  },
});

await api.widgets.create({
  body: { contentType: "application/vnd.acme.widget", value: widget },
});
```

`ClientOptions.codecs` keys normalize to lower-case media types without
parameters; duplicate normalized keys fail at client creation. A generated
multi-media request requires its `body.contentType` discriminant, and a
multi-media response accepts an explicit `accept` selection. Input schemas are
validated before `encode`; decoded values are validated after `decode` or each
`decodeStream` item. A custom codec cannot claim a generated exact media type.
The selected request codec must provide `encode`; a selected non-stream response
codec must provide `decode`; and a selected stream response codec must provide
`decodeStream`. A missing request capability fails before Fetch. A missing
response capability fails after headers select the media type but before body
acquisition.

The SDK owns `MediaStreamReader`, bounds every read, and cancels it when the
returned iterator ends early, aborts, or throws. A stream codec must parse with
that reader, enforce `maxFrameBytes` for its own record format, and cannot
bypass cancellation or the configured size bound by retaining a raw Fetch body.

### Phase 5: Response Contracts, Links, and Streams

Extend response lowering for:

- Typed response headers and exact/default/range status response unions.
- Response media wildcards and `Accept` negotiation.
- Link Objects, `operationId`/`operationRef`, parameters, request bodies,
  server overrides, and runtime expressions.
- JSON sequence, binary sequence, NDJSON response streams, and Server-Sent
  Events.

Required result:

```ts
const response = await api.orders.get.raw({ path: { orderID } });
response.headers.rateLimitRemaining;

for await (const event of api.events.stream({ path: { subscriptionID } }, { signal })) {
  // typed stream item or SSE event
}

await response.links.next({ query: { pageSize: 100 } }, { signal });
```

Stream APIs use typed `AsyncIterable`, accept `AbortSignal`, preserve Fetch
pull-based backpressure, and report decode failures while iterating. Automatic
SSE reconnect is intentionally absent because replay, cursor, authentication,
and duplicate-event policy belong to the host application. Links preserve the
originating response context. Link-derived values are defaults, explicit call
values override them, and unresolved required expressions fail before sending
a request. For non-streaming operations, `raw` returns the typed status,
decoded headers, selected content type, decoded value, and the original
`Response` after its body has been consumed. For streaming operations,
`stream()` is the sole owner of the response body and returns the typed
iterator; `raw()` returns the untouched `Response` without a decoded stream or
value. A link inherits its client
transport, codecs, and credential provider, but takes its own request options
and never reuses the originating operation's `AbortSignal`.

Streaming request codecs are completed here with the common stream transport.
Every reader has a configurable maximum record/frame size; malformed or
oversized input fails and cancels the underlying reader. Iterator `return()`
and abort both cancel the reader and release its lock.

### Phase 6: Outbound Security

Implement security requirement planning for API key, HTTP, OAuth2, OpenID
Connect, mutual TLS, alternatives, optional requirements, and operation
overrides.

Required result:

- Generated code identifies declared requirement alternatives, asks the host
  to select one alternative and provide the complete credential set for it,
  then applies each credential at its declared header, query, or cookie
  location.
- Credential acquisition remains host-owned. The SDK does not embed token
  storage, login UX, refresh logic, or secrets.
- Generated security metadata preserves every declared OAuth2 flow URL and
  scope for 3.0/3.1/3.2, the 3.2 `deviceAuthorization` flow,
  `oauth2MetadataUrl`, OpenID Connect URL, and 3.2 security-scheme
  `deprecated` state. Discovery, device-code interaction, token acquisition,
  and refresh remain host-owned; the credential provider receives the typed
  declaration and selected scopes.

Public contract:

```ts
const api = createClient({
  baseURL,
  credentials: async ({ operation, alternatives, origin }) => ({
    alternative: alternatives.bearerAuth,
    values: {
      bearerAuth: { kind: "http-bearer", token: await tokenFor(operation, origin) },
    },
  }),
});
```

The generated credential type is a discriminated union keyed by the document's
security-scheme names and alternatives. Dispatch fails before Fetch unless the
selected alternative has every required credential. Credentials are requested
for the final normalized origin; they are never reused for a Link or server
override at another origin unless the host explicitly returns them for that
origin. `values` permits exactly the schemes in the selected alternative:
missing or extra values fail before Fetch. Security-owned header/query/cookie
locations cannot be supplied by ordinary call options; a collision fails rather
than overwriting a credential. API keys in cookies are transport-capability
gated. Browser Fetch uses only an existing same-origin browser cookie under the
configured credential mode and never synthesizes a `Cookie` header; a
cross-origin browser call or a transport without a cookie jar fails before
dispatch. Other environments require an explicit cookie-jar transport adapter.
Mutual TLS requires a compatible host transport adapter. Its capability is
checked before dispatch and an mTLS operation never falls back to default
Fetch.

Validation:

- 3.0, 3.1, and 3.2 fixtures for every OAuth2 flow URL, scope, alternative,
  and operation override; 3.2 fixtures additionally cover
  `deviceAuthorization`, `oauth2MetadataUrl`, and deprecated schemes.
- Generated-provider typechecks, runtime alternative dispatch, metadata
  assertions, missing-credential failures, and transport-capability failures.

### Phase 7: Inbound Server Parity

Apply the schema, parameter, media, stream, and security engines to generated
Webhook and Callback endpoints.

Coverage:

- Inbound path/query/header/cookie parameters.
- Non-JSON bodies, multipart, XML, and streams.
- Declared security schemes and requirement alternatives.
- The TypeScript `--with server` output.

For an endpoint with non-empty effective security, router construction requires
an authenticator or installs a default 401 rejection. The generated
authentication context contains the declared alternatives and typed candidates
for header, query, and cookie credentials; the host decides identity and
authorization. Authentication runs after route selection but before body
consumption and handler invocation. `security: []` explicitly disables inherited
security. A host rejection becomes its chosen safe `Response`; malformed input
maps to generated 400/415/422 responses, and an unhandled handler exception
maps to a detail-free 500 response.

The existing Fetch-native public boundary remains the model:

```ts
createWebhookRouter(handlers, options)
createCallbackHandlers(handlers, options).deliveryStatus.fetch(request)
```

`WebhookRouterOptions` and `CallbackHandlerOptions` accept the same
media-type-keyed codec registry as the client. `decodeInbound` handles a
declared custom non-streaming request body; `decodeInboundStream` handles its
declared streaming form through the bounded `MediaStreamReader`. Both results
are validated as wire values and transformed to generated TypeScript property
names before a handler receives them. `encode` also serializes a declared
custom handler response media type after output validation. A missing inbound
codec returns 415; malformed codec output returns 400. An unhandled handler
exception returns a detail-free 500 response.

For one declared inbound media type, a generated handler receives the decoded
body value directly. For multiple declared media types, it receives a
discriminated body value: `{ contentType, value }`. `contentType` is the
selected declared media type, and `value` has that representation's generated
input type. This keeps runtime media selection visible and type-safe rather
than guessing from an overlapping value shape.

### Phase 8: Reusable OpenAPI Components

Finish reusable Component Objects that depend on earlier phases:

- Component Headers, Links, Media Types, Security Schemes, and Callback
  variants.
- Reusable response header projection and Link execution.
- Complete reference handling for every allowed component context.

## Cross-cutting Requirements

- Preserve source-mode output: no generated package metadata, hidden build
  step, or mandatory runtime dependency.
- Keep the normalized IR, validator, codec, transport, and security layers
  independent of TypeScript source formatting. TypeScript is the only active
  target until this roadmap is complete.
- Keep public client, metadata, and inbound server entrypoints separate.
- Every newly supported construct updates the feature manifest from `error` to
  `generated` or `metadata`, with executable evidence. A construct generated
  only by an optional add-on keeps the base client target in `error` and records
  its executable conditional state (for example `typescript --with server`)
  instead; completion evaluates that selected target/add-on condition.
- Keep unsupported input as a feature-path diagnostic until all required layers
  are complete.

## Validation Strategy

Every phase requires all of the following before a capability status changes:

1. Version fixtures for every version where the feature exists.
2. Parser/resolver tests for valid and invalid input.
3. Generated TypeScript typecheck consumers.
4. Runtime request/response tests, including an invalid edge case.
5. Source-mode application example coverage where the feature changes public
   usage.
6. Capability manifest, inventory, and matrix evidence updates.
7. `just agent check` and a scoped review of parser, runtime, public API, and
   documentation changes.

## Remote Reference Policy

Remote references are intentionally not part of the default compiler path.
Remote mode uses repeatable `--allow-remote-ref <origin>` allowlists,
`--ref-lock <path>`, and explicit `--update-ref-lock`. The default lockfile is
`<input>.openapi-sdkgen.lock`; allowing a remote origin never creates or updates
it implicitly. The same lockfile records trusted schema-extension executable
digests.
The optional remote-reference mode must require HTTPS, explicit origin
allowlists, bounded redirects and response sizes, timeouts, a content-addressed
cache, integrity recording in a lockfile, and an offline/reproducible mode.
Each redirect hop resolves and pins DNS before connection, rejects loopback,
private, link-local, and otherwise non-public addresses, and is revalidated on
redirect to prevent DNS rebinding. Remote fetches are unauthenticated. A missing
or mismatched lockfile digest fails closed unless an explicit lock-update mode
is selected. No compiler invocation may make an implicit network request. Until
this mode exists, contained local-file references remain the supported
external-reference model.

## Extension Boundary

The standard OpenAPI and JSON Schema vocabularies for each supported OpenAPI
version are implemented by this roadmap. An arbitrary custom JSON Schema
vocabulary has semantics defined outside OpenAPI, so no generator can infer its
validator or wire behavior. Unknown optional vocabularies are preserved as
annotations. A document that requires an unknown vocabulary must provide an
explicitly registered compiler extension; otherwise compilation fails with a
feature-path diagnostic. This preserves correctness without silently discarding
extension semantics.

## Completion Definition

The roadmap is complete only when the feature manifest has no ordinary
OpenAPI/JSON Schema feature left in `error` state for the selected target and
environment, every exception has an explicit environment boundary, and the
versioned conformance suite proves generated behavior rather than parser
acceptance alone.
