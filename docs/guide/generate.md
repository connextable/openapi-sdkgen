# Generate an SDK

## Base client

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api
```

The base output has a client entry point, generated declarations, runtime
helpers, and an explicit metadata entry. Generated files contain generated-code
and lint-suppression markers, plus Prettier's `@noprettier` pragma. Enable
`checkIgnorePragma` in Prettier 3.6.0 or later to respect it:

```json
{
  "checkIgnorePragma": true
}
```

For older Prettier versions, ignore the output directory instead, for example
`src/generated/**` in `.prettierignore`.

The CLI reads the document's
[OpenAPI Object](https://spec.openapis.org/oas/v3.2.0.html#openapi-object) and
its reusable [Components Object](https://spec.openapis.org/oas/v3.2.0.html#components-object).

## Inbound server add-on

[Callback Objects](https://spec.openapis.org/oas/v3.2.0.html#callback-object)
and root Webhooks are host-owned endpoints, so they are opt-in:

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --with server \
  --output ./src/generated/api
```

This adds `server/webhooks.ts` and `server/callbacks.ts`. It does not change
the client-only root entry point.

For most applications, generation ends here: rerun the same command whenever
the OpenAPI document changes and commit the generated source with the change.

::: details Advanced: locked remote references

Remote [Reference Object](https://spec.openapis.org/oas/v3.2.0.html#reference-object)
resolution is disabled by default. Permit an exact HTTPS origin
and intentionally write the integrity lock on the first generation:

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api \
  --allow-remote-ref https://schemas.example.test \
  --update-ref-lock
```

Later runs verify the locked response digest. `--offline` resolves only from
the adjacent `.openapi-sdkgen-cache/`; it never opens a network connection.
Remote URLs require HTTPS, an exact allowlisted origin, public DNS addresses,
bounded redirects, and no credentials.
:::

::: details Advanced: custom JSON Schema vocabularies

Required custom vocabularies in a
[Schema Object](https://spec.openapis.org/oas/v3.2.0.html#schema-object) use an
explicit checked-in extension manifest:

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api \
  --schema-extension ./schema-extension.json \
  --update-ref-lock
```

The extension runs only while generating. It exchanges versioned JSON-RPC and
returns a replacement JSON Schema value; generated application code never runs
the extension. See the [CLI reference](../reference/cli.md) for each flag.
:::
