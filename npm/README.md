# openapi-sdkgen

Generate source-mode TypeScript SDKs from OpenAPI 3.0.x, 3.1.x, and 3.2.x
documents.

```sh
pnpm dlx openapi-sdkgen generate \
  --input ./openapi.yaml \
  --target typescript \
  --output ./src/generated/api
```

The package contains precompiled executables for macOS, Linux, and Windows on
arm64 and x64. Go is not required by consumers.

For command reference and generated SDK usage, see the
[project documentation](https://github.com/connextable/openapi-sdkgen#readme).
