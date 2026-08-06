import type { MediaCodec } from "./codecs.js";
import type { OperationDefinition, ServerSelection } from "./operation.js";
import type { SecurityCredentials, SecurityRequirementDefinition } from "./security.js";
import type { Transport } from "./transport.js";

/** Configuration shared by every operation on a generated client. */
export interface ClientOptions {
  /**
   * Absolute deployment URL including the API version base, for example
   * `https://api.example.com/v1`.
   */
  readonly baseURL?: string;
  /** Absolute origin used only to resolve a selected relative OpenAPI Server URL. */
  readonly origin?: string;
  /** Selects one server from each operation's effective OpenAPI Server list. */
  readonly server?: ServerSelection;
  /** Host-provided codecs for declared non-built-in media types. */
  readonly codecs?: Readonly<Record<string, MediaCodec<unknown>>>;
  /** Optional host transport with explicit capabilities beyond ordinary Fetch. */
  readonly transport?: Transport;
  /** Fetch implementation or wrapper. Defaults to `globalThis.fetch`; the SDK adds no retries. */
  readonly fetch?: typeof globalThis.fetch;
  /** Default headers added to every request. Use dedicated options for SDK-managed headers. */
  readonly headers?: HeadersInit;
  /** Complete default `Authorization` header value, including its authentication scheme. */
  readonly authorization?: string;
  /**
   * Default Fetch credentials mode passed to the active Fetch implementation.
   * `"include"` satisfies cookie API-key security through ambient Fetch cookies.
   */
  readonly credentials?: RequestCredentials;
  /** Host-owned credential acquisition hook for an already selected OpenAPI security requirement. */
  readonly securityProvider?: SecurityCredentialProvider;
  /** Default positive timeout in milliseconds. Individual requests may override it. */
  readonly timeoutMS?: number;
  /** Maximum byte count a custom streaming codec may request in one read. */
  readonly maxStreamItemBytes?: number;
}

/** Context supplied to a security credential provider after final server selection. */
export interface SecurityCredentialContext {
  readonly operation: Pick<OperationDefinition, "route" | "operationID" | "method" | "path">;
  /** The sole SDK-selected requirement or the caller-selected alternative. */
  readonly requirement: SecurityRequirementDefinition;
  readonly origin: string;
}

/** Host-owned credential acquisition hook for the selected requirement. */
export type SecurityCredentialProvider = (
  context: SecurityCredentialContext,
) => SecurityCredentials | Promise<SecurityCredentials>;
