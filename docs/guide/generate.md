# Generate an SDK

## Base client

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api
```

The TypeScript target generates the client, types, runtime code, and OpenAPI metadata.
Rerun the command whenever the OpenAPI file changes.

## Generate Webhook and Callback code

When the OpenAPI file defines a Webhook or Callback that your application
receives, add `--with server`.

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --with server \
  --output ./src/generated/api
```

This adds `server/webhooks.ts` and `server/callbacks.ts`.

For most applications, generation ends here: rerun the same command whenever
the OpenAPI document changes and commit the generated source with the change.

## Generation errors

`generate` checks the OpenAPI file and selected options before writing code.
There is no separate `validate` command.

Errors include the relevant OpenAPI location when available and explain what
needs to change. Warnings do not stop generation.

If any error occurs, the existing output remains unchanged. This prevents a
failed command from leaving partially generated code.

## CI

Run the same `generate` command in CI and treat its exit status as the gate.
Generate into a temporary directory when CI only needs validation; compare or
copy that directory when generated source is checked in.

```sh
output="$(mktemp -d)/api"
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output "$output"
```

Use the command's exit status as the CI check.

::: details Advanced: locked remote references

Remote `$ref` values are disabled by default. Allow the HTTPS origin and create
the reference lock the first time they are fetched:

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api \
  --allow-remote-ref https://schemas.example.test \
  --update-ref-lock
```

Later runs verify the remote content against the lock. `--offline` uses only
previously cached references and does not open a network connection.
:::

::: details Advanced: custom JSON Schema vocabularies

When the OpenAPI file uses a required custom JSON Schema vocabulary, provide a
local extension manifest:

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api \
  --schema-extension ./schema-extension.json \
  --update-ref-lock
```

The extension runs only while generating and is not included in application
code. See the [CLI reference](../reference/cli.md) for the available options.
:::
