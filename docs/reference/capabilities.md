# OpenAPI support

Use this page when you are deciding whether an OpenAPI document can be generated
as-is, or when a version-specific feature needs a closer look. You do not need
the implementation matrix to create and use a normal SDK.

Guide links point to the current
[OpenAPI 3.2 Object reference](https://spec.openapis.org/oas/v3.2.0.html).
The generator still interprets each document according to its declared 3.0,
3.1, or 3.2 version; use the matching
[3.0](https://spec.openapis.org/oas/v3.0.4.html) or
[3.1](https://spec.openapis.org/oas/v3.1.1.html) specification when a feature
is version-specific.

## What works in an application

### Standard request and response shapes

HTTP operations, parameters, JSON/text/binary/form/multipart bodies, and typed
responses generate a TypeScript client with resource calls and exact
`operationId` calls.

### Servers, security, Links, and streams

OpenAPI Servers, security schemes, response headers, Links, and streams become
generated client behavior. The host still provides transport and credentials
when the platform owns those concerns.

### Callbacks and Webhooks

Add `--with server` to generate Fetch-native inbound handler contracts.

### OpenAPI versions

OpenAPI 3.0.x, 3.1.x, and 3.2.x are parsed and generated according to the
version declared by the document.

## When generation stops

The generator does not silently skip a valid construct. If the selected target
or add-on cannot represent it, generation stops with a feature-path diagnostic.
Some source information is deliberately available only from `metadata.js`,
because exposing it as a fabricated runtime API would be misleading.

## Detailed implementation evidence

The documents below are maintained alongside the compiler and tests. They are
useful for audits, migration work, and exact feature-level compatibility checks.
Large tables scroll horizontally within the page on narrow layouts.

- [Capability inventory](https://github.com/connextable/openapi-sdkgen/blob/main/docs/openapi-feature-inventory.md): grouped feature list.
- [Capability matrix](https://github.com/connextable/openapi-sdkgen/blob/main/docs/openapi-feature-matrix.md): version-specific status and test evidence.
- [Feature manifest](https://github.com/connextable/openapi-sdkgen/blob/main/docs/openapi-feature-manifest.json): machine-readable source of truth.
- [Implementation roadmap](https://github.com/connextable/openapi-sdkgen/blob/main/docs/openapi-sdk-implementation-roadmap.md): architecture and implementation contract.
