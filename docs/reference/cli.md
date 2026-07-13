# CLI reference

## Command

```text
openapi-sdkgen generate --input <document> --target typescript --output <directory>
  [--with <addon> ...]
  [--allow-remote-ref <https-origin> ...]
  [--ref-lock <path>]
  [--update-ref-lock]
  [--offline]
  [--schema-extension <manifest> ...]
```

## Required options

- `--input <document>` — The OpenAPI 3.0.x, 3.1.x, or 3.2.x JSON document to generate from.
- `--target typescript` — The active source-mode target.
- `--output <directory>` — A fresh application-source directory for generated artifacts.

## Optional add-ons

- `--with server` — Adds Fetch-native Callback and Webhook contracts under
  `server/`. Repeat `--with` when combining future add-ons.

## Remote reference policy

- `--allow-remote-ref <origin>` — Permits one exact HTTPS origin for remote
  `$ref` resolution. Repeat the option to permit more than one origin.
- `--ref-lock <path>` — Overrides the remote-reference and extension integrity
  lock path.
- `--update-ref-lock` — Creates or updates the lock only after a successful
  compile.
- `--offline` — Resolves locked remote references only from the local
  content-addressed cache.

Local file references remain contained within the input directory. A reference
outside that canonical root is rejected. Network resolution is off until an
exact origin is supplied.

## Schema extensions

- `--schema-extension <manifest>` — Registers a trusted local compiler
  extension for a required custom JSON Schema vocabulary. Repeat the option
  when registering multiple manifests.

An extension manifest locks its executable digest and vocabulary URIs. The
extension protocol is compile-time JSON-RPC only: it returns a replacement JSON
Schema object or boolean, never executable TypeScript or runtime callbacks.
