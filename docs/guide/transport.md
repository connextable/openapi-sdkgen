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

## Caller-owned and host-managed headers

Generated request inputs include headers the application is allowed to set,
such as `If-Match` and `Idempotency-Key`. Headers reserved by the
[Fetch Standard](https://fetch.spec.whatwg.org/#forbidden-request-header) stay
under Fetch or host-environment control. This includes `Origin`, `Host`,
`Cookie`, `Content-Length`, `Accept-Encoding`, and every `Proxy-*` or `Sec-*`
header.

An OpenAPI declaration can still mark one of these headers as required on the
wire. The generated browser call does not ask the application to provide it:

```ts
await api.auth.oauth(provider).post({
  body: {
    intent: "login",
    returnTo,
  },
});
```

Browser Fetch supplies, omits, or rewrites the header according to the browser
security context. The API server remains responsible for validating the
header. The original declaration is still available in `metadata.js`, and
generated Webhook or Callback handlers still receive and validate the full
inbound header contract.

Neither `headerParams` nor the raw `headers` option can set a host-managed
header. Runtime checks also cover JavaScript or `as any` escape hatches before
the transport runs. `X-HTTP-Method`, `X-HTTP-Method-Override`, and
`X-Method-Override` remain normal inputs, but a parsed comma-separated method
item that equals `CONNECT`, `TRACE`, or `TRACK` case-insensitively is rejected.
Header-located OpenAPI API-key credentials pass through the same policy before
the transport runs.

After regenerating an existing SDK, remove previously supplied values such as
`headerParams.Origin`. This is an intentional source-level breaking change; no
compatibility property is generated.

### Inject a header in a non-browser transport

A trusted Node or other non-browser transport can add a host-managed header
after the generated request has been encoded. Keep the value outside public
operation input and raw header options:

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

Use this pattern only when that transport is the trusted owner of the value.
It does not bypass browser Fetch rules or turn a Fetch-managed header into a
caller-provided API-key credential.

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
