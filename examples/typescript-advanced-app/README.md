# TypeScript Advanced App

Full source-mode consumer example. It generates the client into `src/generated/widget-sdk/`, then the app compiles and imports it with a relative path. No generated package install or build step.

Install the `openapi-sdkgen` CLI, Node 22 or newer, and pnpm 10 or newer. The example calls `openapi-sdkgen` from `PATH`, as a normal application repository would.

## Covered behavior

- Client authorization
- Query, header, and cookie parameters
- Cursor pagination
- Typed path resources
- Raw HTTP response metadata
- Typed validation errors
- Binary request bodies
- Request timeout handling

## Run

```sh
./setup.sh

# terminal 1
pnpm run server

# terminal 2
WIDGET_API_BASE_URL=http://127.0.0.1:18788/v1 pnpm run client
```

`src/client.ts` is the complete usage example. It runs as compiled Node ESM, so
it intentionally uses the explicit `.js` file path. In a normal web bundler,
import the generated directory as `./generated/widget-sdk` instead:

```ts
import { createClient } from "./generated/widget-sdk/index.js";
```
