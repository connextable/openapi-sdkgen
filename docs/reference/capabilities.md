# OpenAPI support

openapi-sdkgen supports OpenAPI 3.0.x, 3.1.x, and 3.2.x files. Each file is
interpreted according to its declared OpenAPI version.

## Requests and responses

The TypeScript target generates types and client code for:

- API paths, HTTP methods, and parameters
- JSON, text, binary, form, and multipart request bodies
- status-specific responses and response headers
- request and response validation based on OpenAPI schemas

Call an API through a resource method, its exact HTTP route, or its
`operationId`.

For request headers, generated clients distinguish caller requiredness from
wire requiredness. Names controlled by Fetch remain explicitly settable but are
optional caller inputs. Typed values, undeclared raw headers, header API keys,
and method-override values are forwarded to Fetch without SDK policy blocking.
Metadata and generated Webhook and Callback contracts preserve the original
OpenAPI requiredness. See
[Caller inputs and environment-controlled headers](../guide/transport.md#caller-inputs-and-environment-controlled-headers).

## Servers and authentication

OpenAPI Server Objects, security schemes, and operation-specific security
requirements are supported. Generated operation options expose stable Security
Requirement Object IDs through `securityRequirement`; your application provides
tokens and certificates directly or through `securityProvider`.

See [transport, authentication, and streams](../guide/transport.md) for
configuration examples.

## Links and streams

OpenAPI Links become typed follow-up request helpers. Streaming responses are
available as `AsyncIterable` values.

## Webhooks and Callbacks

Add `--with server` to generate types and Fetch-based routers for receiving
Webhooks and Callbacks.

## Unsupported features

When the selected target cannot generate an OpenAPI feature, generation stops
with an error that identifies the relevant location. Unsupported features are
not silently skipped.

OpenAPI information that does not map to the client API remains available from
`metadata.js`.
