---
layout: home

hero:
  name: openapi-sdkgen
  text: Use your OpenAPI contract as TypeScript source
  tagline: Generate a typed client beside your application code and make API changes easier to ship with confidence.
  actions:
    - theme: brand
      text: Start in three minutes
      link: /guide/getting-started
    - theme: alt
      text: Explore the generated client
      link: /guide/client
  image:
    src: /sdk-flow.svg
    alt: OpenAPI document becoming a TypeScript client

features:
  - icon: 🧩
    title: Generate source where you need it
    details: The generated client lives in your source tree. There is no SDK package to publish or build separately.
  - icon: ✓
    title: Keep the contract at the boundary
    details: Validate request data before dispatch and validate decoded responses before application code consumes them.
  - icon: ⚡
    title: Use the API at two levels
    details: Work with readable resources in everyday code, or reach every visible operation through its exact operation ID.
  - icon: ↗
    title: Receive events when your app needs them
    details: Add Fetch-native Webhook and Callback contracts only when you opt into the server add-on.
---

## One command, ordinary application source

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api
```

The output is ordinary TypeScript that you import with a relative ESM path.
Follow [Create your first SDK](./guide/getting-started.md) for the complete
flow, including the first typed request.
