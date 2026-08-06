/** Explicit capabilities a host transport grants to generated SDK code. */
export interface TransportCapabilities {
  /** The transport can serialize caller-provided Cookie headers. */
  readonly cookieJar?: boolean;
  /** The transport selects a client certificate for an mTLS operation. */
  readonly mutualTLS?: boolean;
  /** Headers Fetch normally withholds, such as Set-Cookie, are readable. */
  readonly readableResponseHeaders?: true | readonly string[];
}

/** Host transport adapter. The SDK never infers elevated transport capabilities. */
export interface Transport {
  readonly fetch: typeof globalThis.fetch;
  readonly capabilities?: TransportCapabilities;
}
