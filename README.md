# openapi-sdkgen

`openapi-sdkgen` compiles an OpenAPI 3.2 document into a typed SDK package.

The primary distribution is a precompiled CLI binary published through GitHub Releases, so TypeScript consumers do not need Go installed. Go users can also install the command from the module:

```sh
go install github.com/connextable/openapi-sdkgen/cmd/openapi-sdkgen@latest
```

## Generate a TypeScript package

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./sdk \
  --package-name @example/api-sdk
```

`--package-name` is optional; the output directory name becomes the package name when it is omitted.

The output directory must be fresh. The CLI stages every artifact and publishes it only after generation succeeds, rather than modifying an existing package tree.

The output is an independent package source tree. It includes the generated client, public entrypoint, `package.json`, TypeScript build configuration, README, and contract manifest. It also declares its own `build` script and TypeScript build dependency. Run the emitted package's package-manager install and `build` command, then publish that package; `test/typescript` is never part of the package to publish.

Generate separate packages by invoking the command once per OpenAPI document and output directory.

## Architecture

```txt
OpenAPI 3.2 document
        │
        ▼
parser + validation → language-neutral IR → built-in target registry
                                               └─ typescript
```

Targets are compiled into the binary. Adding a future Kotlin, Swift, or Go target implements the target interface; it does not change OpenAPI parsing or CLI command flow.

## Development

All project operations use the agent-safe `just agent` commands.

```sh
just agent ts-lock      # update test-only TypeScript lockfile intentionally
just agent ts-install
just agent check
```

`just agent conformance` builds the CLI, generates a generic fixture package into an ignored directory, compiles it, typechecks consumer tests, and runs its runtime tests. The test fixture proves generated package behavior without turning the TypeScript harness into a deployment source.
