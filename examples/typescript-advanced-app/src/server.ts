import { createServer, type IncomingMessage, type ServerResponse } from "node:http";

const port = Number(process.env.WIDGET_API_PORT ?? "18788");
const widgets = [
  { id: "widget-1", name: "First widget" },
  { id: "widget-2", name: "Second widget" },
];

const readBody = async (request: IncomingMessage): Promise<Buffer> => {
  const chunks: Buffer[] = [];
  for await (const chunk of request) chunks.push(Buffer.from(chunk));
  return Buffer.concat(chunks);
};

const json = (response: ServerResponse, status: number, body: unknown) => {
  response.writeHead(status, { "content-type": "application/json" }).end(JSON.stringify(body));
};

const server = createServer(async (request, response) => {
  const url = new URL(request.url ?? "/", `http://${request.headers.host}`);

  // Pagination fixture: one item per page, then a terminal null cursor.
  if (request.method === "GET" && url.pathname === "/v1/widgets") {
    const start = url.searchParams.get("cursor") === "next" ? 1 : 0;
    if (url.searchParams.get("limit") !== "1") {
      json(response, 400, { error: "expected limit=1" });
      return;
    }
    json(response, 200, {
      items: widgets.slice(start, start + 1),
      pagination: { nextCursor: start === 0 ? "next" : null },
    });
    return;
  }

  // Creation fixture validates every transport feature demonstrated by the client.
  if (request.method === "POST" && url.pathname === "/v1/widgets") {
    const body = JSON.parse((await readBody(request)).toString("utf8")) as { name: string };
    if (body.name === "") {
      json(response, 422, {
        error: { code: "validation_failed", message: "name is required", details: { field: "name" } },
      });
      return;
    }
    if (
      request.headers.authorization !== "Bearer example-token" ||
      request.headers["x-trace-id"] !== "trace-1" ||
      !request.headers.cookie?.includes("session=example-session") ||
      url.searchParams.getAll("tag").join(",") !== "example,raw"
    ) {
      json(response, 401, { error: "missing example request metadata" });
      return;
    }
    const widget = { id: `widget-${widgets.length + 1}`, name: body.name };
    widgets.push(widget);
    response
      .writeHead(201, { "content-type": "application/json", "x-request-id": "request-1" })
      .end(JSON.stringify(widget));
    return;
  }

  // A nested resource fixture for two path parameters.
  if (request.method === "GET" && url.pathname === "/v1/customers/customer-1/widgets/widget-1") {
    json(response, 200, {
      id: decodeURIComponent(url.pathname.split("/").at(-1) ?? ""),
      name: "Customer widget",
    });
    return;
  }

  // Binary upload fixture.
  if (request.method === "POST" && url.pathname === "/v1/uploads") {
    const body = await readBody(request);
    if (
      request.headers["content-type"] !== "application/octet-stream" ||
      !body.equals(Buffer.from([1, 2, 3]))
    ) {
      json(response, 400, { error: "expected binary widget payload" });
      return;
    }
    response.writeHead(204).end();
    return;
  }

  // Deliberately slower than the client's timeout.
  if (request.method === "GET" && url.pathname === "/v1/slow") {
    setTimeout(() => json(response, 200, { ready: true }), 50);
    return;
  }
  json(response, 404, { error: "not found" });
});

server.listen(port, "127.0.0.1", () => {
  console.log(`Widget API listening at http://127.0.0.1:${port}/v1`);
});
