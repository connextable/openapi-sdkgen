import type {
  WireBodyDefinition,
  WireResponseDefinition,
  WireSchema,
  WireSchemas,
} from "./codecs.js";
import type { SecurityRequirementDefinition } from "./security.js";

/** Endpoint-neutral operation metadata emitted by `sdkgen` for the transport runtime. */
export interface OperationDefinition {
  /** Canonical method-and-path route identity. */
  readonly route: string;
  /** Exact OpenAPI operation ID when explicitly declared. */
  readonly operationID?: string;
  /** Uppercase HTTP method. */
  readonly method: string;
  /** OpenAPI path template, including any `{parameter}` placeholders. */
  readonly path: string;
  /** Response envelope profile used when decoding successful responses. */
  readonly envelope: string;
  /** Default request-body media type. */
  readonly contentType?: string;
  /** Effective OpenAPI Server alternatives for this operation. */
  readonly servers?: readonly ServerDefinition[];
  /** Generated path, query, header, and cookie parameter definitions. */
  readonly parameters?: readonly ParameterDefinition[];
  /** Case-insensitive set of headers owned by the OpenAPI operation. */
  readonly headerNames?: readonly string[];
  /** Input component schemas used to map TypeScript properties to JSON wire names. */
  readonly inputSchemas?: WireSchemas;
  /** Output component schemas used to map JSON wire names to TypeScript properties. */
  readonly outputSchemas?: WireSchemas;
  /** Supported request-body representations. */
  readonly requestBodies?: readonly WireBodyDefinition[];
  /** Whether the OpenAPI Request Body Object requires a body. */
  readonly requestBodyRequired?: boolean;
  /** Successful response representations keyed by status and media type. */
  readonly responses?: readonly WireResponseDefinition[];
  /** Effective OpenAPI Security Requirement Objects for this operation. */
  readonly security?: readonly SecurityRequirementDefinition[];
}

/** Returns the stable operation name used in runtime diagnostics. */
export function operationDiagnosticName(operation: OperationDefinition): string {
  return operation.operationID ?? operation.route;
}

/** Stable OpenAPI Server selection supplied to {@link ClientOptions.server}. */
export interface ServerSelection {
  readonly id?: string;
  readonly variables?: Readonly<Record<string, string>>;
}

/** One generated OpenAPI Server Object. */
export interface ServerDefinition {
  readonly id: string;
  readonly url: string;
  readonly variables?: readonly ServerVariableDefinition[];
}

/** One generated OpenAPI Server Variable Object. */
export interface ServerVariableDefinition {
  readonly name: string;
  readonly defaultValue: string;
  readonly enumValues?: readonly string[];
}

/** OpenAPI parameter serialization metadata emitted by `sdkgen`. */
export interface ParameterDefinition {
  /** HTTP request location of the parameter. */
  readonly location: "path" | "query" | "querystring" | "header" | "cookie";
  /** Exact OpenAPI and HTTP wire name. */
  readonly name: string;
  /** Generated TypeScript property name. */
  readonly property: string;
  /** OpenAPI serialization style. */
  readonly style: string;
  /** Whether objects and arrays are exploded into separate values. */
  readonly explode: boolean;
  /** Preserve RFC 3986 reserved characters in a query value. */
  readonly allowReserved?: boolean;
  /** Whether the parameter must be present before the request is sent. Defaults to false. */
  readonly required?: boolean;
  /** Media type for a content-based parameter. */
  readonly contentType?: string;
  /** Schema used for wire-name transformation before serialization. */
  readonly schema?: WireSchema;
  /** Structured sort member keys mapped to exact OpenAPI enum wire values. */
  readonly sort?: Readonly<Record<string, string>>;
}
