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

## Provide credentials

For a simple Bearer token, set `authorization` when creating the client.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  authorization: "Bearer example-token",
});
```

Use a `credentials` function when the API offers multiple authentication
alternatives or credentials must be loaded for each request.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  credentials: async ({ alternatives }) => {
    const alternative = alternatives.serviceToken;
    return {
      alternative,
      values: {
        serviceToken: { kind: "http-bearer", token: await getToken() },
      },
    };
  },
});
```

Generated clients support API keys, HTTP Basic and Bearer authentication,
OAuth2, OpenID Connect, and mTLS requirements. Your application remains
responsible for login, token refresh, and credential storage.

For an OpenAPI cookie API-key security scheme, browser applications can use
ambient cookies without reading their values:

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  credentials: "include",
});
```

`"include"` satisfies cookie security through the active Fetch implementation.
The SDK neither asks for the cookie value nor creates a `Cookie` header. Fetch
and the browser cookie policy decide whether a cookie is sent. If an AND
security alternative combines a cookie with another scheme, use a credential
provider for the other schemes and pass `{ credentials: "include" }` as the
request option; omit the ambient cookie scheme from the provider's `values`.

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
