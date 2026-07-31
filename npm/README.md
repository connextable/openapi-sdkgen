# openapi-sdkgen

Generate application SDK source from OpenAPI 3.0, 3.1, and 3.2 documents.
The current release includes the `typescript` target.

```sh
pnpm dlx openapi-sdkgen generate \
  --input ./openapi.yaml \
  --target typescript \
  --output ./src/generated/api
```

The package contains precompiled executables for macOS, Linux, and Windows on
arm64 and x64. Go is not required by consumers.

For command reference and generated SDK usage, see the
[project documentation](https://jinyongp.github.io/openapi-sdkgen/).
