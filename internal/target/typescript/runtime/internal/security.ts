/** One declared OpenAPI security scheme in a selected requirement. */
export interface SecuritySchemeDefinition {
  readonly name: string;
  readonly type: "apiKey" | "http" | "oauth2" | "openIdConnect" | "mutualTLS";
  readonly location?: "header" | "query" | "cookie";
  readonly parameterName?: string;
  readonly scheme?: string;
  readonly bearerFormat?: string;
  readonly scopes?: readonly string[];
  /** Lossless OAuth flow declaration for host token acquisition. */
  readonly flows?: Readonly<Record<string, unknown>>;
  readonly openIdConnectUrl?: string;
  readonly oauth2MetadataUrl?: string;
  readonly deprecated?: boolean;
}

/** Normalized OpenAPI Security Requirement Object available to an operation. */
export interface SecurityRequirementDefinition {
  readonly id: string;
  readonly schemes: readonly SecuritySchemeDefinition[];
}

/** API-key credential supplied by the host. */
export interface APIKeyCredential {
  readonly kind: "api-key";
  readonly value: string;
}

/** HTTP Basic credential supplied by the host. */
export interface HTTPBasicCredential {
  readonly kind: "http-basic";
  readonly username: string;
  readonly password: string;
}

/** HTTP Bearer credential supplied by the host. */
export interface HTTPBearerCredential {
  readonly kind: "http-bearer";
  readonly token: string;
}

/** Credential for a non-Basic/non-Bearer HTTP authentication scheme. */
export interface HTTPCredential {
  readonly kind: "http";
  readonly value: string;
}

/** OAuth2 or OpenID Connect access token supplied by the host. */
export interface OAuthCredential {
  readonly kind: "oauth2" | "openIdConnect";
  readonly token: string;
}

/** Mutual TLS is selected by a capable host transport, never by the SDK. */
export interface MutualTLSCredential {
  readonly kind: "mutual-tls";
}

/** All credential shapes understood by generated security lowering. */
export type SecurityCredential =
  | APIKeyCredential
  | HTTPBasicCredential
  | HTTPBearerCredential
  | HTTPCredential
  | OAuthCredential
  | MutualTLSCredential;

/** Credentials keyed by scheme name for one selected OpenAPI security requirement. */
export type SecurityCredentials = Readonly<Record<string, SecurityCredential>>;

/** Context supplied to a security credential provider after final server selection. */
