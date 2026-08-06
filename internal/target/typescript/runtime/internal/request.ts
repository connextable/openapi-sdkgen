/** Metadata that identifies the server-side request associated with a result or error. */
export interface RequestMetadata {
  /** Server request ID, usually read from the `X-Request-Id` response header. */
  readonly id?: string;
}

/** Options applied to one generated operation call. */
export interface RequestOptions {
  /** Explicit absolute base URL. Generated Link helpers use this for a Link Server Object. */
  readonly baseURL?: string;
  /** Caller-owned cancellation signal. Cancellation is reported as `REQUEST_ABORTED`. */
  readonly signal?: AbortSignal;
  /** Positive timeout in milliseconds for this request, overriding the client default. */
  readonly timeoutMS?: number;
  /** Additional request headers. Contract-owned and SDK-managed headers are rejected here. */
  readonly headers?: HeadersInit;
  /** Complete `Authorization` header value, overriding the client default. */
  readonly authorization?: string;
  /** Requested response media type for operations with multiple representations. */
  readonly accept?: string;
  /** Value sent through the `X-CSRF-Token` header. */
  readonly csrfToken?: string;
  /** Caller-provided value sent through the `X-Request-Id` header. */
  readonly requestID?: string;
  /** Fetch credentials mode; `"include"` also satisfies cookie API-key security ambiently. */
  readonly credentials?: RequestCredentials;
  /** Declared additional headers for named multipart form-data parts. */
  readonly multipartHeaders?: Readonly<Record<string, HeadersInit>>;
  /** Selected media type for multipart parts keyed by form name or positional index. */
  readonly multipartContentTypes?: Readonly<Record<string, string>>;
  /** Maximum byte count a custom streaming codec may request in one read. */
  readonly maxStreamItemBytes?: number;
}

/** Binary body values supported by generated request encoders. */
export type BinaryBody = Blob | ArrayBuffer | ArrayBufferView;

/** Successful response including decoded data and the underlying Fetch API response. */
export interface RawResponse<Output, HeaderValues = Readonly<Record<string, unknown>>> {
  /** HTTP status code. */
  readonly status: number;
  /** Normalized response media type without parameters. */
  readonly contentType?: string;
  /** Decoded, typed response body. */
  readonly data: Output;
  /** Response headers. */
  readonly headers: HeaderValues;
  /** Request metadata extracted from the response. */
  readonly request: RequestMetadata;
  /** Original Fetch API response. Its body has already been consumed unless streamed. */
  readonly response: Response;
}

/**
 * Raw response narrowed to an operation's exact status, media type, and output type.
 *
 * @template Status HTTP status literal.
 * @template ContentType Response media-type literal.
 * @template Output Decoded response body type.
 */
export type RawResponseFor<
  Status extends number,
  ContentType,
  Output,
  HeaderValues = Readonly<Record<string, unknown>>,
> = Omit<RawResponse<Output, HeaderValues>, "status" | "contentType"> &
  Readonly<{
    status: Status;
    contentType: ContentType;
  }>;
