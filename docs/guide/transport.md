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
