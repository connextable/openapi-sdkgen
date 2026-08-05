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

Headers declared in OpenAPI are available through `headerParams`. See
[Request headers](../guide/transport.md#request-headers).

## Servers and authentication

OpenAPI Server Objects, security schemes, and operation-specific security
requirements are supported. The SDK selects a sole effective requirement.
Generated operation options expose stable Security Requirement Object IDs
through a required `securityRequirement` union only when alternatives exist.
Your application provides tokens and certificates directly or through a
`securityProvider` that receives the selected requirement.

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

The source OpenAPI document is available through the generated metadata entry point.
