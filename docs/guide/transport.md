# Transport, auth, and streams

Most applications need only `baseURL` and, when the API requires it, an
authorization value or credential provider. Configure a custom transport only
when the runtime must offer something ordinary Fetch cannot.

## Default Fetch and custom transports

The client uses host `fetch` by default. Supply a transport only when the host
needs a different Fetch implementation or declares additional capabilities:

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

Capabilities are truthful host declarations, not feature toggles. Cookie
parameters from a
[Parameter Object](https://spec.openapis.org/oas/v3.2.0.html#parameter-object),
unreadable response headers, and mutual TLS fail with a typed
transport-capability error when the active environment cannot provide them.

## Credentials

The application owns token acquisition, refresh, storage, and client
certificate selection. Generated code applies values to the OpenAPI security
[Security Scheme Object](https://spec.openapis.org/oas/v3.2.0.html#security-scheme-object)
and [Security Requirement Object](https://spec.openapis.org/oas/v3.2.0.html#security-requirement-object)
wire locations:

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

The generator supports API keys, HTTP basic/bearer schemes, OAuth2/OpenID
metadata, and mutual-TLS capability requirements. It never implements login or
token refresh flows inside the generated SDK.

## Cancellation, timeouts, and streams

Per-request options accept normal Fetch cancellation controls:

```ts
const controller = new AbortController();

const todos = await api.todos.list(
  { query: { limit: 20 } },
  { signal: controller.signal, timeoutMS: 5_000 },
);
```

Streaming operations return `AsyncIterable` values. Fetch backpressure is
preserved, a consumer can stop iteration naturally, and the generator does not
silently reconnect Server-Sent Events.
