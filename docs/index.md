---
layout: home

hero:
  name: openapi-sdkgen
  text: Turn OpenAPI into application code
  tagline: Generate SDK source that matches your API description and use it directly.
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: Explore the generated client
      link: /guide/client

features:
  - icon: 🧩
    title: Use it directly in your application
    details: Import the generated client and types directly into your project.
  - icon: ✓
    title: Validate requests and responses
    details: Check requests before they are sent and decoded responses before your application uses them.
  - icon: ⚡
    title: Call APIs your way
    details: Use readable resource methods or call an operation by its exact HTTP method, path, or operation ID.
  - icon: ↗
    title: Generate inbound APIs too
    details: Add Webhook and Callback handler types and routers from the same OpenAPI document when needed.
---

## One command, ordinary application source

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api
```

The command writes the client and types to the selected directory. Follow
[Create your first SDK](./guide/getting-started.md) to generate a client and
make your first API call.
