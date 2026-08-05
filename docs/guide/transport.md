# Transport, authentication, and streams

Most applications only need a `baseURL` and API credentials. Configure a custom
transport when the default Fetch behavior is not enough.

## Custom transport

Set `transport` to use another Fetch implementation or declare capabilities such as a
cookie jar, readable response headers, or mTLS.

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

Declare only capabilities provided by the configured transport.

## Request headers

Headers declared in OpenAPI are available through `headerParams`.

```ts
await api.$operations.createTodo({
  headerParams: { "Idempotency-Key": requestID },
  body: { title: "Write documentation" },
});
```

Headers controlled by Fetch, such as `Origin`, `Host`, `Cookie`, and `Sec-*`, are
optional caller inputs. The active Fetch implementation decides whether they are sent.

Add environment-specific headers in a custom transport when needed:

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  transport: {
    async fetch(input, init = {}) {
      const headers = new Headers(init.headers);
      headers.set("Origin", trustedOrigin);
      return fetch(input, { ...init, headers });
    },
  },
});
```

## Authentication

Set a Bearer token directly when one credential is enough.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  authorization: "Bearer example-token",
});
```

When an operation has several OpenAPI security alternatives, select one with
`securityRequirement`. A sole requirement is selected automatically, and an empty
requirement is named `"anonymous"`.

```ts
await api.$operations.updateCheckout({
  securityRequirement: "GuestCapability",
  authorization: "Bearer example-token",
});
```

Use `securityProvider` to load credentials for the selected requirement.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  securityProvider: async ({ operation, requirement, origin }) => ({
    serviceToken: {
      kind: "http-bearer",
      token: await getToken(operation, requirement, origin),
    },
  }),
});
```

Generated clients support API keys, HTTP Basic and Bearer authentication, OAuth2,
OpenID Connect, and mTLS. The application handles login, token refresh, and credential
storage.

For cookie authentication in a browser, configure Fetch credentials:

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  credentials: "include",
});
```

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

Streaming APIs return `AsyncIterable` values. Stopping iteration also stops reading the
response. Server-Sent Events do not reconnect automatically.
