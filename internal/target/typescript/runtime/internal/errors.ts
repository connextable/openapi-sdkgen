import type { RequestMetadata } from "./request.js";

/** Stable error codes for failures produced by the SDK transport layer. */
export const TransportErrorCode = {
  /** The request input could not be serialized before calling `fetch`. */
  REQUEST_ENCODE_FAILED: "REQUEST_ENCODE_FAILED",
  /** `fetch` failed before an HTTP response was received. */
  NETWORK_ERROR: "NETWORK_ERROR",
  /** The caller's {@link RequestOptions.signal} aborted the request. */
  REQUEST_ABORTED: "REQUEST_ABORTED",
  /** The configured request timeout elapsed before the response completed. */
  REQUEST_TIMEOUT: "REQUEST_TIMEOUT",
  /** The HTTP response body could not be decoded as its declared media type. */
  RESPONSE_DECODE_FAILED: "RESPONSE_DECODE_FAILED",
  /** The operation has multiple effective security requirements and needs an explicit selection. */
  SECURITY_REQUIREMENT_REQUIRED: "SECURITY_REQUIREMENT_REQUIRED",
  /** The requested or provider-selected security requirement is not valid for the operation. */
  SECURITY_REQUIREMENT_INVALID: "SECURITY_REQUIREMENT_INVALID",
  /** The operation requires credentials but the client did not provide a usable selection. */
  SECURITY_CREDENTIALS_REQUIRED: "SECURITY_CREDENTIALS_REQUIRED",
  /** Credentials conflict with caller-controlled request data or cannot be applied safely. */
  SECURITY_CREDENTIALS_INVALID: "SECURITY_CREDENTIALS_INVALID",
  /** The host transport lacks a declared capability required by this operation. */
  TRANSPORT_CAPABILITY_REQUIRED: "TRANSPORT_CAPABILITY_REQUIRED",
} as const;

/** Union of all stable SDK transport error codes. */
export type TransportErrorCode = (typeof TransportErrorCode)[keyof typeof TransportErrorCode];

/** Values used to construct an {@link APIError}. */
export interface APIErrorOptions<Code extends string, Details = unknown> {
  /** Stable server or transport error code. */
  readonly code: Code;
  /** Human-readable error message. Do not branch application logic on this value. */
  readonly message: string;
  /** Metadata for the request that produced the error. */
  readonly request?: RequestMetadata;
  /** HTTP status code when the server returned a response. */
  readonly status?: number;
  /** Structured server validation or domain-error details, when provided. */
  readonly details?: Details;
  /** Legacy structured server validation fields, when provided. */
  readonly fields?: unknown;
  /** Decoded response body, retained so a Link can follow an error response. */
  readonly data?: unknown;
  /** Original Fetch API response, when one was received. */
  readonly response?: Response;
  /** Original exception that caused a transport or decoding failure. */
  readonly cause?: unknown;
}

/**
 * Normalized error thrown for server-declared errors and SDK transport failures.
 *
 * Use generated error guards or {@link isErrorCode} instead of matching messages.
 */
export class APIError<Code extends string = string, Details = unknown> extends Error {
  /** Standard JavaScript error name. */
  readonly name = "APIError";
  /** Stable server or transport error code. */
  readonly code: Code;
  /** Metadata for the request that produced the error. */
  readonly request: RequestMetadata;
  /** HTTP status code, absent when no response was received. */
  readonly status?: number;
  /** Structured server validation or domain-error details. */
  readonly details?: Details;
  /** Legacy structured server validation fields. */
  readonly fields?: unknown;
  /** Decoded response body, when the server returned a response. */
  readonly data?: unknown;
  /** Original Fetch API response, when available. */
  readonly response?: Response;
  /** Original exception, when this error wraps another failure. */
  readonly cause?: unknown;

  /** Creates a normalized API or transport error. */
  constructor(options: APIErrorOptions<Code, Details>) {
    super(options.message);
    this.code = options.code;
    this.request = options.request ?? {};
    if (options.status !== undefined) this.status = options.status;
    if (options.details !== undefined) this.details = options.details;
    if (options.fields !== undefined) this.fields = options.fields;
    if (options.data !== undefined) this.data = options.data;
    if (options.response !== undefined) this.response = options.response;
    if (options.cause !== undefined) this.cause = options.cause;
  }
}

/** Error produced by request encoding, networking, cancellation, timeout, or response decoding. */
export type TransportError = APIError<TransportErrorCode>;

/**
 * Checks whether a value is an {@link APIError} created by this SDK runtime.
 *
 * @param error Value caught from an SDK call.
 * @returns `true` when `error` is an {@link APIError}.
 */
export function isAPIError(error: unknown): error is APIError {
  return error instanceof APIError;
}

/**
 * Narrows an unknown error to an {@link APIError} with an exact code.
 *
 * @param error Value caught from an SDK call.
 * @param code Server or transport code to match.
 * @returns `true` when both the error type and code match.
 */
export function isErrorCode<Code extends string>(
  error: unknown,
  code: Code,
): error is APIError<Code> {
  return isAPIError(error) && error.code === code;
}

/**
 * Reads a stable error code without requiring a type guard branch.
 *
 * @param error Value caught from an SDK call.
 * @returns Error code for an {@link APIError}; otherwise `undefined`.
 */
export function getErrorCode(error: unknown): string | undefined {
  return isAPIError(error) ? error.code : undefined;
}

/**
 * Reads the server request ID attached to an SDK error.
 *
 * @param error Value caught from an SDK call.
 * @returns Server request ID when available; otherwise `undefined`.
 */
export function getRequestID(error: unknown): string | undefined {
  return isAPIError(error) ? error.request.id : undefined;
}
