# Transport, authentication, and streams

Most applications only need a `baseURL` and the credentials required by the
API. Configure a custom transport only when the default Fetch implementation
cannot provide a required feature.

## Custom transport

The client uses the environment's `fetch` by default. Configure `transport`
when you need a different Fetch implementation, a cookie jar, access to
restricted response headers, or mTLS.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  transport: {
    fetch: undiciFetch,
    capabilities: {
      cookieJar: true,
      readableResponseHeaders: ["set-cookie"],
      mutualTLS: true,
    },
  },
});
```

Only declare capabilities that the active environment actually provides. The
client reports an error before sending a request when a required capability is
missing.

## Caller inputs and environment-controlled headers

Every declared request header remains available through `headerParams`.
Names controlled by the active Fetch implementation are optional caller
inputs, even when OpenAPI marks them as required. This set follows the
[Fetch Standard forbidden request-header definition](https://fetch.spec.whatwg.org/#forbidden-request-header)
for fixed names and the `Proxy-*` and `Sec-*` families. It includes `Origin`,
`Host`, `Cookie`, `Content-Length`, and `Accept-Encoding`.

Callers may omit an environment-controlled value:

```ts
await api.auth.oauth(provider).post({
  body: {
    intent: "login",
    returnTo,
  },
});
```

They may also provide it explicitly:

```ts
await api.auth.oauth(provider).post({
  headerParams: { Origin: "https://app.example.test" },
  body: {
    intent: "login",
    returnTo,
  },
});
```

The generated runtime forwards explicit typed values through normal `Headers`
assembly. It also forwards environment-controlled names supplied through raw
headers when the name is not owned by a declared parameter, and forwards
header-located API-key credentials. Existing declared/reserved header collision
checks still apply. `X-HTTP-Method`, `X-HTTP-Method-Override`, and
`X-Method-Override` keep their OpenAPI requiredness, and their values are passed
to Fetch without SDK-side method filtering.

The active Fetch implementation makes the final decision. A browser may ignore,
rewrite, synthesize, or reject a supplied value according to its security
context. An injected Fetch implementation may accept it. Generated input types
therefore describe what the caller may provide, not a guarantee about the final
wire headers.

OpenAPI requiredness remains unchanged in `metadata.js`. Generated Webhook and
Callback handlers also preserve and validate the full inbound header contract.
Link request-header expressions read `invocation.sourceInput`; source input is
not retained automatically with a raw response. Pass it again when following a
Link that uses `$request.header.*`:

```ts
const sourceInput = {
  headerParams: { Origin: "https://app.example.test" },
};
const response = await api.$operations.createSource.raw(sourceInput);

await api.$links.createSource.follow(response, { sourceInput });
```

Without `sourceInput`, the request-header expression resolves to `undefined`,
even when the source call received an explicit value. Header values assigned by
a Link are forwarded to Fetch normally.

This projection is source-compatible with callers that already pass values such
as `headerParams.Origin`; callers may now omit them. It is suitable for a patch
release.

### Normalize a header in a custom transport

A trusted Node or other non-browser transport can still add or normalize a
header after the generated request has been encoded:

```ts
const nodeFetch = globalThis.fetch;

const api = createClient({
  baseURL: "https://api.example.test",
  transport: {
    async fetch(input, init = {}) {
      const headers = new Headers(init.headers);
      headers.set("Origin", trustedOrigin);
      return nodeFetch(input, { ...init, headers });
    },
  },
});
```

Use this pattern when header policy belongs at the transport boundary. It does
not override restrictions imposed by the Fetch implementation that ultimately
sends the request.

## Select a security requirement and provide credentials

For a simple Bearer token, set `authorization` when creating the client.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  authorization: "Bearer example-token",
});
```

OpenAPI calls each object in the `security` array a Security Requirement Object.
The objects are OR choices; all schemes inside one object are required together
(AND). The SDK automatically selects a sole effective requirement. When an
operation has multiple effective requirements, generated operation options
expose their stable IDs as a required, exact `securityRequirement` union and
the operation's options argument is required. An empty requirement uses the
stable ID `"anonymous"` and must still be selected when another requirement is
available:

```ts
await api.$operations.updateCheckout({
  securityRequirement: "GuestCapability",
  authorization: "Bearer example-token",
});

await api.$operations.startOAuth(input, {
  securityRequirement: "anonymous",
});
```

Use `securityProvider` when the host must load credentials for a selected
requirement. The SDK passes that one requirement as `requirement`, whether it
was selected automatically or by the caller. The provider returns only a map
of credentials keyed by scheme name; it never selects or echoes a requirement.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  securityProvider: async ({ operation, requirement, origin }) => {
    if (requirement.id !== "serviceToken") {
      throw new Error(`Unsupported security requirement: ${requirement.id}`);
    }
    return {
      serviceToken: {
        kind: "http-bearer",
        token: await getToken(operation, origin),
      },
    };
  },
});
```

Generated clients support API keys, HTTP Basic and Bearer authentication,
OAuth2, OpenID Connect, and mTLS requirements. Your application remains
responsible for login, token refresh, and credential storage.

Multiple effective requirements always require an explicit
`securityRequirement`. Omission fails with `SECURITY_REQUIREMENT_REQUIRED`
before `securityProvider` or Fetch. With exactly one effective requirement, the
SDK selects it and does not expose a selector. An unsecured operation likewise
does not expose one. Passing a selector to either state from stale JavaScript
fails with `SECURITY_REQUIREMENT_INVALID` before the provider or Fetch.

An anonymous or already satisfied requirement skips the provider. A matching
`authorization` or `csrfToken` satisfies its scheme. When some schemes remain,
the provider may omit an already satisfied scheme or return the same value; a
different value fails before Fetch. Raw `headers` never satisfy a security
scheme.

### Migrate security providers from v3 to v4

Regenerate clients and run the consumer's TypeScript checks after upgrading.

| v3 contract | v4 contract |
|---|---|
| Empty requirement ID `"optional"` | `"anonymous"` |
| Optional selector on a sole requirement | No selector; the SDK selects it |
| `RequestOptions.securityRequirement?: string` | Exact operation-specific selector for alternatives only |
| Provider context `requirements` and `selectedRequirement` | Singular required `requirement` |
| Provider result `{ requirement, credentials }` | Credential map returned directly |

Calls with alternatives must retain an explicit selector, using `"anonymous"`
for anonymous access. Remove redundant selectors from sole-requirement calls.
Update every provider before deploying the regenerated runtime; a stale v3
provider result fails with `SECURITY_CREDENTIALS_INVALID` before Fetch.

For an OpenAPI cookie API-key security scheme, browser applications can use
ambient cookies without reading their values:

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  credentials: "include",
});
```

`credentials` is only the Fetch `RequestCredentials` policy. It can be combined
with `securityProvider`; ambient cookies do not select among multiple
requirements. `"include"` satisfies cookie security through the active Fetch
implementation after the requirement is selected. The SDK neither asks for the
cookie value nor creates a `Cookie` header. Fetch and the browser cookie policy
decide whether a cookie is sent.

For a requirement that combines a session cookie and CSRF header, select the
requirement and satisfy both schemes through dedicated request options:

```ts
await api.$operations.updateCheckout({
  securityRequirement: "BuyerCSRFHeader__BuyerSessionCookie",
  credentials: "include",
  csrfToken: csrf,
});
```

Selection failures use `SECURITY_REQUIREMENT_REQUIRED` or
`SECURITY_REQUIREMENT_INVALID`. Missing, malformed, extra, or conflicting
credentials use `SECURITY_CREDENTIALS_REQUIRED` or
`SECURITY_CREDENTIALS_INVALID`.

Do not declare the same cookie as both an `in: cookie` Parameter Object and an
applied cookie security scheme. Generation rejects that ambiguous ownership.
Ordinary cookie parameters remain explicit caller input and require a capable
cookie-jar transport.

## Cancel a request or set a timeout

Pass an `AbortSignal` or timeout with any request.

```ts
const controller = new AbortController();

const todos = await api.todos.list(
  { query: { limit: 20 } },
  { signal: controller.signal, timeoutMS: 5_000 },
);
```

## Streams

Streaming APIs return `AsyncIterable` values. Stopping iteration also stops
reading the response. Server-Sent Events do not reconnect automatically.
