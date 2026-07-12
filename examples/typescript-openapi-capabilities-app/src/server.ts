import { createServer, type IncomingMessage, type ServerResponse } from "node:http";

import { createCallbackHandlers } from "./generated/capabilities-sdk/server/callbacks.js";
import { createWebhookRouter } from "./generated/capabilities-sdk/server/webhooks.js";

const apiPort = Number(process.env.CAPABILITIES_API_PORT ?? "18790");
const webhookPort = Number(process.env.CAPABILITIES_WEBHOOK_PORT ?? "18791");

const items = [
  { id: "item-1", name: "First item", status: "draft" },
  { id: "item-2", name: "Second item", status: "published" },
];
const expectedVerbMethods = ["GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "QUERY", "PURGE", "TRACE"];
let nextVerbMethod = 0;

const readBody = async (request: IncomingMessage): Promise<Buffer> => {
  const chunks: Buffer[] = [];
  for await (const chunk of request) chunks.push(Buffer.from(chunk));
  return Buffer.concat(chunks);
};

const writeJSON = (response: ServerResponse, status: number, body: unknown, headers: HeadersInit = {}) => {
  response
    .writeHead(status, { "content-type": "application/json", ...Object.fromEntries(new Headers(headers)) })
    .end(JSON.stringify(body));
};

const writeFetchResponse = async (response: ServerResponse, generated: Response) => {
  response.writeHead(generated.status, Object.fromEntries(generated.headers));
  response.end(Buffer.from(await generated.arrayBuffer()));
};

const toFetchRequest = async (request: IncomingMessage): Promise<Request> => {
  const headers = new Headers();
  for (const [name, value] of Object.entries(request.headers)) {
    if (value !== undefined) headers.set(name, Array.isArray(value) ? value.join(", ") : value);
  }
  const body = await readBody(request);
  const method = request.method ?? "GET";
  const url = `http://${request.headers.host ?? `127.0.0.1:${webhookPort}`}${request.url ?? "/"}`;
  return new Request(url, {
    method,
    headers,
    ...(body.length > 0 ? { body: body.toString("utf8"), duplex: "half" as const } : {}),
  });
};

const webhookRouter = createWebhookRouter(
  {
    itemChanged: async ({ body, operationID, security }) => {
      if (operationID !== "itemChangedWebhook") throw new Error("unexpected webhook operation");
      if (JSON.stringify(security) !== JSON.stringify([{ webhookSignature: [] }])) {
        throw new Error("unexpected webhook security metadata");
      }
      return { status: 202, body: { accepted: body.id } };
    },
  },
  {
    routes: { itemChanged: "/hooks/items" },
    authenticate: ({ request }) =>
      request.headers.get("x-webhook-signature") === "example-signature"
        ? undefined
        : new Response("Unauthorized", { status: 401 }),
  },
);

const callbackHandlers = createCallbackHandlers({
  deliveryStatus: async ({ body, operationID }) => {
    if (operationID !== "deliveryStatusCallback" || body?.kind !== "changed") {
      throw new Error("unexpected callback delivery");
    }
    return { status: 204, headers: { "x-callback-delivery": "accepted" } };
  },
});

const apiServer = createServer(async (request, response) => {
  const url = new URL(request.url ?? "/", `http://${request.headers.host}`);

  if (url.pathname === "/v1/verbs") {
	if (request.method !== expectedVerbMethods[nextVerbMethod]) {
		writeJSON(response, 400, {
			error: `expected ${expectedVerbMethods[nextVerbMethod] ?? "no further method"}, received ${request.method}`,
		});
		return;
	}
	nextVerbMethod += 1;
    response.writeHead(204).end();
    return;
  }

  if (request.method === "GET" && url.pathname === "/v1/status") {
    response.writeHead(204).end();
    return;
  }

  if (request.method === "GET" && url.pathname === "/v1/records/.one.two/;left=1;right=two") {
    const valid =
      url.searchParams.getAll("tag").join(",") === "one,two" &&
      url.searchParams.get("filter[name]") === "widget" &&
      url.searchParams.get("filter[state]") === "active" &&
      url.searchParams.get("spaces") === "one two" &&
      url.searchParams.get("pipes") === "one|two" &&
      url.searchParams.get("query") === '{"scope":"all"}' &&
      request.headers["x-trace-id"] === "trace-1" &&
      request.headers.cookie?.includes("session=example-session");
    if (!valid) {
      writeJSON(response, 400, { error: "parameter serialization did not match the OpenAPI contract" });
      return;
    }
    writeJSON(response, 200, { id: "item-record", name: "Serialized record", status: "draft" });
    return;
  }

  if (request.method === "GET" && url.pathname === "/v1/items") {
    const secondPage = url.searchParams.get("cursor") === "next";
    if (url.searchParams.get("limit") !== "1") {
      writeJSON(response, 400, { error: "expected generated limit parameter" });
      return;
    }
    writeJSON(response, 200, {
      items: [items[secondPage ? 1 : 0]],
      pagination: { nextCursor: secondPage ? null : "next" },
    });
    return;
  }

  if (request.method === "POST" && url.pathname === "/v1/items") {
    const body = JSON.parse((await readBody(request)).toString("utf8")) as {
      name?: string;
      status?: string;
      callbackURL?: string;
    };
    if (body.name === "") {
      writeJSON(response, 422, {
        error: {
          code: "validation_failed",
          message: "name is required",
          details: { name: "must not be empty" },
        },
      });
      return;
    }
    const item = { id: `item-${items.length + 1}`, name: body.name ?? "Unnamed", status: body.status ?? "draft" };
    if (body.callbackURL !== undefined) {
      const callbackResponse = await fetch(body.callbackURL, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ id: item.id, kind: "changed", attempt: 1 }),
      });
      if (callbackResponse.status !== 204 || callbackResponse.headers.get("x-callback-delivery") !== "accepted") {
        writeJSON(response, 502, { error: "callback delivery failed" });
        return;
      }
    }
    items.push(item);
    writeJSON(response, 201, item, { "x-request-id": "create-item-1" });
    return;
  }

  if (request.method === "POST" && url.pathname === "/v1/payloads/form") {
    const form = new URLSearchParams((await readBody(request)).toString("utf8"));
    if (form.get("name") !== "form item" || form.getAll("tag").join(",") !== "one,two") {
      writeJSON(response, 400, { error: "unexpected form payload" });
      return;
    }
    response.writeHead(204).end();
    return;
  }

  if (request.method === "POST" && url.pathname === "/v1/payloads/multipart") {
    const contentType = request.headers["content-type"] ?? "";
    const body = (await readBody(request)).toString("utf8");
    if (!contentType.startsWith("multipart/form-data;") || !body.includes("attachment.txt") || !body.includes("file contents")) {
      writeJSON(response, 400, { error: "unexpected multipart payload" });
      return;
    }
    response.writeHead(204).end();
    return;
  }

  if (request.method === "POST" && url.pathname === "/v1/payloads/text") {
    const body = (await readBody(request)).toString("utf8");
    response.writeHead(200, { "content-type": "text/plain" }).end(`echo:${body}`);
    return;
  }

  if (request.method === "POST" && url.pathname === "/v1/payloads/binary") {
    const body = await readBody(request);
    response.writeHead(200, { "content-type": "application/octet-stream" }).end(body);
    return;
  }

  writeJSON(response, 404, { error: "not found" });
});

const webhookServer = createServer(async (request, response) => {
	if (request.method === "POST" && request.url === "/callbacks/delivery") {
		await writeFetchResponse(response, await callbackHandlers.deliveryStatus.fetch(await toFetchRequest(request)));
		return;
	}
  if (request.method === "POST" && request.url === "/hooks/items") {
    await writeFetchResponse(response, await webhookRouter.fetch(await toFetchRequest(request)));
    return;
  }
  response.writeHead(404).end();
});

apiServer.listen(apiPort, "127.0.0.1", () => {
  console.log(`Capability API listening at http://127.0.0.1:${apiPort}/v1`);
});
webhookServer.listen(webhookPort, "127.0.0.1", () => {
  console.log(`Capability inbound host listening at http://127.0.0.1:${webhookPort}`);
});
