import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { once } from "node:events";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { afterAll, beforeAll, describe, expect, it } from "vitest";

const typescriptRoot = fileURLToPath(new URL("..", import.meta.url));
const probeFile = path.join(typescriptRoot, "quickinfo-probe.ts");
const probeURI = pathToFileURL(probeFile).href;
const probeSource = `
import { createClient as createContractClient } from "./fixtures/generated/client/index.js"
import type {
  OperationBody,
  OperationContract,
  OperationInput,
  OperationParameter,
  OperationQuery,
  RouteBody,
  RouteContract,
  RouteInput,
} from "./fixtures/generated/client/index.js"
import { createClient as createOpenAPI31Client } from "./fixtures/generated/baseline-oas31/index.js"
import { createClient as createOpenAPI32Client } from "./fixtures/generated/baseline-oas32/index.js"
import { Enums as collisionEnums, isEnumValue as isCollisionEnumValue } from "./fixtures/generated/collisions/index.js"
import type { EnumValue as CollisionEnumValue } from "./fixtures/generated/collisions/index.js"
import { Enums as directCollisionEnums, isEnumValue as isDirectCollisionEnumValue } from "./fixtures/generated/collisions/enums.js"
import type { EnumValue as DirectCollisionEnumValue } from "./fixtures/generated/collisions/enums.js"

declare const contract: ReturnType<typeof createContractClient>
declare const openAPI31: ReturnType<typeof createOpenAPI31Client>
declare const openAPI32: ReturnType<typeof createOpenAPI32Client>
declare const extractedInput: OperationInput<"createTask">
declare const extractedBody: OperationBody<"createTask">
declare const extractedQuery: OperationQuery<"listTasks">
declare const extractedCursor: OperationParameter<"listTasks", "query", "cursor">
declare const extractedContract: OperationContract<"createTask">
declare const extractedRouteContract: RouteContract<"POST /projects/{projectID}/tasks">
declare const extractedRouteInput: RouteInput<"POST /projects/{projectID}/tasks">
declare const extractedRouteBody: RouteBody<"POST /projects/{projectID}/tasks">
declare const extractedTodoStatus: CollisionEnumValue<"TodoStatus">
declare const directExtractedTodoStatus: DirectCollisionEnumValue<"TodoStatus">

contract.projects("project-1").tasks.create
contract.$operations.createTask
contract.$routes["POST /projects/{projectID}/tasks"]
contract.$operations.createTask({ path: { projectID: "project-1" }, body: { title: "Write docs", priority: "HIGH" } })
contract.projects("project-1").tasks.create({ body: { title: "Write docs", priority: "HIGH" } })
contract.projects("project-1").tasks.create.raw
contract.tasks.paginate
contract.customers
openAPI31.source.get.links
openAPI32.events.get.stream
extractedInput
extractedBody
extractedQuery
extractedCursor
extractedContract
extractedRouteContract
extractedRouteInput
extractedRouteBody
collisionEnums.TodoStatus
collisionEnums.TodoStatus.DONE
extractedTodoStatus
directCollisionEnums.TodoStatus
directExtractedTodoStatus
isCollisionEnumValue(
isDirectCollisionEnumValue(
`;

type PendingRequest = {
  reject(error: Error): void;
  resolve(value: unknown): void;
};

type LSPMessage = {
  error?: { message?: string };
  id?: number;
  method?: string;
  params?: unknown;
  result?: unknown;
};

class LSPClient {
  private buffer = Buffer.alloc(0);
  private nextID = 1;
  private readonly pending = new Map<number, PendingRequest>();
  private stderr = "";

  constructor(private readonly process: ChildProcessWithoutNullStreams) {
    process.stdout.on("data", (chunk: Buffer) => {
      this.buffer = Buffer.concat([this.buffer, chunk]);
      this.readMessages();
    });
    process.stderr.on("data", (chunk: Buffer) => {
      this.stderr += chunk.toString();
    });
    process.on("exit", (code) => {
      if (code === 0) return;
      const error = new Error(`TypeScript LSP exited with ${code}: ${this.stderr}`);
      for (const request of this.pending.values()) request.reject(error);
      this.pending.clear();
    });
  }

  async close() {
    await this.request("shutdown", undefined);
    const exited = once(this.process, "exit");
    this.notify("exit", undefined);
    await exited;
  }

  notify(method: string, params: unknown) {
    this.send({ jsonrpc: "2.0", method, params });
  }

  request<Result>(method: string, params: unknown): Promise<Result> {
    const id = this.nextID++;
    this.send({ id, jsonrpc: "2.0", method, params });
    return new Promise<Result>((resolve, reject) => {
      this.pending.set(id, {
        reject,
        resolve: (value) => resolve(value as Result),
      });
    });
  }

  private readMessages() {
    while (true) {
      const headerEnd = this.buffer.indexOf("\r\n\r\n");
      if (headerEnd < 0) return;
      const header = this.buffer.subarray(0, headerEnd).toString();
      const length = /Content-Length:\s*(\d+)/i.exec(header)?.[1];
      if (length === undefined) throw new Error(`invalid LSP header: ${header}`);
      const bodyLength = Number(length);
      const bodyStart = headerEnd + 4;
      if (this.buffer.length < bodyStart + bodyLength) return;
      const body = this.buffer.subarray(bodyStart, bodyStart + bodyLength).toString();
      this.buffer = this.buffer.subarray(bodyStart + bodyLength);
      this.handleMessage(JSON.parse(body) as LSPMessage);
    }
  }

  private handleMessage(message: LSPMessage) {
    if (message.method !== undefined && message.id !== undefined) {
      const result = message.method === "workspace/configuration" ? [] : null;
      this.send({ id: message.id, jsonrpc: "2.0", result });
      return;
    }
    if (message.id === undefined) return;
    const request = this.pending.get(message.id);
    if (request === undefined) return;
    this.pending.delete(message.id);
    if (message.error !== undefined) {
      request.reject(new Error(message.error.message ?? "TypeScript LSP request failed"));
      return;
    }
    request.resolve(message.result);
  }

  private send(message: object) {
    const body = JSON.stringify(message);
    this.process.stdin.write(`Content-Length: ${Buffer.byteLength(body)}\r\n\r\n${body}`);
  }
}

type Hover = {
  contents:
    | string
    | { kind: string; value: string }
    | readonly (string | { language?: string; value: string })[];
} | null;

type SignatureHelp = {
  signatures: readonly { label: string }[];
} | null;

let client: LSPClient;

beforeAll(async () => {
  const tsc = path.join(typescriptRoot, "node_modules", ".bin", "tsc");
  client = new LSPClient(spawn(tsc, ["--lsp", "--stdio"]));
  await client.request("initialize", {
    capabilities: {
      textDocument: { hover: { contentFormat: ["markdown", "plaintext"] } },
    },
    processId: process.pid,
    rootUri: pathToFileURL(typescriptRoot).href,
    workspaceFolders: [
      { name: "openapi-sdkgen-typescript-conformance", uri: pathToFileURL(typescriptRoot).href },
    ],
  });
  client.notify("initialized", {});
  client.notify("textDocument/didOpen", {
    textDocument: {
      languageId: "typescript",
      text: probeSource,
      uri: probeURI,
      version: 1,
    },
  });
});

afterAll(async () => {
  await client.close();
});

function positionAtEnd(expression: string) {
  const start = probeSource.indexOf(expression);
  if (start < 0) throw new Error(`missing probe expression: ${expression}`);
  const offset = start + expression.length - 1;
  const prefix = probeSource.slice(0, offset);
  const lines = prefix.split("\n");
  return { character: lines.at(-1)?.length ?? 0, line: lines.length - 1 };
}

function positionAfter(expression: string) {
  const start = probeSource.indexOf(expression);
  if (start < 0) throw new Error(`missing probe expression: ${expression}`);
  const prefix = probeSource.slice(0, start + expression.length);
  const lines = prefix.split("\n");
  return { character: lines.at(-1)?.length ?? 0, line: lines.length - 1 };
}

function hoverText(hover: Hover) {
  if (hover === null) throw new Error("missing hover response");
  if (typeof hover.contents === "string") return hover.contents;
  if ("value" in hover.contents) return hover.contents.value;
  return hover.contents
    .map((content) => (typeof content === "string" ? content : content.value))
    .join("\n");
}

function displayedType(hover: string) {
  return /```(?:typescript)?\s*\n([\s\S]*?)```/.exec(hover)?.[1]?.trim() ?? hover;
}

async function quickInfo(expression: string) {
  const hover = await client.request<Hover>("textDocument/hover", {
    position: positionAtEnd(expression),
    textDocument: { uri: probeURI },
  });
  const text = hoverText(hover);
  return { display: displayedType(text), text };
}

async function signatureInfo(expression: string) {
  const result = await client.request<SignatureHelp>("textDocument/signatureHelp", {
    context: { isRetrigger: false, triggerCharacter: "(", triggerKind: 2 },
    position: positionAfter(expression),
    textDocument: { uri: probeURI },
  });
  if (result === null) throw new Error(`missing signature help: ${expression}`);
  return result.signatures.map((signature) => signature.label).join("\n");
}

describe("generated client QuickInfo", () => {
  it("keeps resource operation properties concise and public", async () => {
    const info = await quickInfo('contract.projects("project-1").tasks.create');

    expect(info.display).toContain('ResourceCall<"POST /projects/{projectID}/tasks">');
    expect(info.display.match(/POST \/projects\/\{projectID\}\/tasks/g)).toHaveLength(1);
    expect(info.text).not.toContain("__sdkgen_");
    expect(info.text).toContain("Create a task.");
    expect(info.text).toContain("Operation ID: `createTask`.");
    expect(info.text).toContain("await api.projects(projectID).tasks.create");
    expect(info.text).not.toContain("HTTP:");
  });

  it.each([
    ["contract.$operations.createTask", 'OperationMethod<"POST /projects/{projectID}/tasks">'],
    [
      'contract.$routes["POST /projects/{projectID}/tasks"]',
      'OperationMethod<"POST /projects/{projectID}/tasks">',
    ],
  ])("keeps exact method %s concise and public", async (expression, expected) => {
    const info = await quickInfo(expression);

    expect(info.display).toContain(expected);
    expect(info.text).not.toContain("__sdkgen_");
    expect(info.text).not.toContain("OperationTypeIdentity");
    expect(info.text).not.toContain("operationTypeBrand");
  });

  it.each([
    "extractedInput",
    "extractedBody",
    "extractedQuery",
    "extractedCursor",
    "extractedContract",
    "extractedRouteContract",
    "extractedRouteInput",
    "extractedRouteBody",
  ])("keeps extracted helper %s free of implementation names", async (expression) => {
    const info = await quickInfo(expression);

    expect(info.text).not.toContain("__sdkgen_");
    expect(info.text).not.toContain("RouteRequestSections");
    expect(info.text).not.toContain("OperationRoutes");
    expect(info.text).not.toContain("OperationTypeIdentity");
    expect(info.text).not.toContain("operationTypeBrand");
  });

  it.each([
    ["collisionEnums.TodoStatus", "TodoStatus"],
    ["collisionEnums.TodoStatus.DONE", '"DONE"'],
    ["extractedTodoStatus", '"DONE" | "TODO"'],
    ["directCollisionEnums.TodoStatus", "TodoStatus"],
    ["directExtractedTodoStatus", '"DONE" | "TODO"'],
  ])("keeps enum surface %s exact and public", async (expression, expected) => {
    const info = await quickInfo(expression);

    expect(info.display).toContain(expected);
    expect(info.text).not.toContain("__sdkgen_");
    expect(info.text).not.toContain("createEnumCatalog");
    expect(info.text).not.toContain("enumValueEquals");
  });

  it("keeps enum guard signature public", async () => {
    for (const expression of ["isCollisionEnumValue(", "isDirectCollisionEnumValue("]) {
      const info = await signatureInfo(expression);

      expect(info).toContain(expression.slice(0, -1));
      expect(info).toContain("value: unknown");
      expect(info).toContain("value is");
      expect(info).not.toContain("__sdkgen_");
      expect(info).not.toContain("enumValueEquals");
    }
  });

  it.each([
    ["contract.$operations.createTask(", "RouteInput<"],
    ['contract.projects("project-1").tasks.create(', "RouteResourceInput<"],
  ])("keeps %s signature help public", async (expression, expected) => {
    const info = await signatureInfo(expression);

    expect(info).toContain(expected);
    expect(info).not.toContain("__sdkgen_");
  });

  it("uses readable path selector names", async () => {
    const info = await quickInfo("contract.customers");

    expect(info.display).toContain("(customerID: string)");
    expect(info.text).not.toContain("__sdkgen_");
  });

  it.each([
    [
      'contract.projects("project-1").tasks.create.raw',
      'RawCall<"POST /projects/{projectID}/tasks">',
    ],
    ["contract.tasks.paginate", 'PaginateCall<"GET /tasks">'],
    ["openAPI31.source.get.links", 'LinkCalls<"GET /source">'],
    ["openAPI32.events.get.stream", 'StreamCall<"GET /events">'],
  ])("keeps %s capability concise", async (expression, expected) => {
    const info = await quickInfo(expression);

    expect(info.display).toContain(expected);
    expect(info.text).not.toContain("__sdkgen_");
  });
});
