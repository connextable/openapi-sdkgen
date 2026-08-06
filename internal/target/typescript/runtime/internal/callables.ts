import { isRecord } from "./objects.js";
import type { OperationDefinition } from "./operation.js";
import type { RawResponse, RequestOptions } from "./request.js";

/** Low-level request executor used by generated operation bindings. */
export interface RequestFunction {
  /**
   * Sends an operation and returns its decoded response body.
   *
   * @param operation Generated operation metadata.
   * @param input Generated path, query, header, cookie, and body input.
   * @param options Per-request transport options.
   */
  <Output>(
    operation: OperationDefinition,
    input?: unknown,
    options?: RequestOptions,
  ): Promise<Output>;
  /**
   * Sends an operation and returns its decoded body with HTTP response metadata.
   *
   * @param operation Generated operation metadata.
   * @param input Generated path, query, header, cookie, and body input.
   * @param options Per-request transport options.
   */
  raw<Output>(
    operation: OperationDefinition,
    input?: unknown,
    options?: RequestOptions,
  ): Promise<RawResponse<Output>>;
  /** Opens one declared streaming response and lazily decodes its items. */
  stream<Item>(
    operation: OperationDefinition,
    input?: unknown,
    options?: RequestOptions,
  ): AsyncIterable<Item>;
}

type RequiredKeys<Value> = {
  [Key in keyof Value]-?: Record<never, never> extends Pick<Value, Key> ? never : Key;
}[keyof Value];

type OperationOptionsArguments<Options extends RequestOptions> = [RequiredKeys<Options>] extends [
  never,
]
  ? [options?: Options]
  : [options: Options];

const generatedOperationInputKeys = [
  "body",
  "path",
  "query",
  "querystring",
  "headerParams",
  "cookieParams",
] as const;

function isGeneratedOperationInput(value: unknown): boolean {
  return isRecord(value) && generatedOperationInputKeys.some((key) => Object.hasOwn(value, key));
}

function splitOptionalOperationArguments<Input, Options extends RequestOptions>(
  args: readonly unknown[],
): readonly [Input | undefined, Options | undefined] {
  const [first, second] = args;
  if (args.length > 1 || isGeneratedOperationInput(first)) {
    return [first as Input | undefined, second as Options | undefined];
  }
  return [undefined, first as Options | undefined];
}

function operationOptionsArguments<Options extends RequestOptions>(
  options: Options | undefined,
): OperationOptionsArguments<Options> {
  return (options === undefined ? [] : [options]) as OperationOptionsArguments<Options>;
}

/** Callable generated operation that requires typed input. */
export interface InputOperationCall<Input, Output, Options extends RequestOptions, Raw> {
  /** Sends the request and returns the decoded response body. */
  (input: Input, ...options: OperationOptionsArguments<Options>): Promise<Output>;
  /** Sends the request and returns decoded data with HTTP response metadata. */
  raw(input: Input, ...options: OperationOptionsArguments<Options>): Promise<Raw>;
}

/** Callable generated operation with no input object. */
export interface NoInputOperationCall<Output, Options extends RequestOptions, Raw> {
  /** Sends the request and returns the decoded response body. */
  (...options: OperationOptionsArguments<Options>): Promise<Output>;
  /** Sends the request and returns decoded data with HTTP response metadata. */
  raw(...options: OperationOptionsArguments<Options>): Promise<Raw>;
}

/**
 * Callable operation surface selected from whether the operation accepts input.
 *
 * Generated clients specialize this type with operation-specific input, output,
 * options, and raw-response types.
 */
export type OperationCall<
  Input,
  Output,
  Options extends RequestOptions = RequestOptions,
  Raw = RawResponse<Output>,
> = [Input] extends [never]
  ? NoInputOperationCall<Output, Options, Raw>
  : InputOperationCall<Input, Output, Options, Raw>;

/**
 * Binds generated operation metadata to a low-level request executor.
 *
 * Used by generated clients; applications normally call the generated operation instead.
 */
export function bindOperation<
  Input,
  Output,
  Options extends RequestOptions = RequestOptions,
  Raw = RawResponse<Output>,
>(
  request: RequestFunction,
  operation: OperationDefinition,
  hasInput: boolean,
  inputOptional = false,
): OperationCall<Input, Output, Options, Raw> {
  const call = hasInput
    ? inputOptional
      ? (...args: readonly unknown[]) => {
          const [input, options] = splitOptionalOperationArguments<Input, Options>(args);
          return request<Output>(operation, input, options);
        }
      : (input: Input, ...options: OperationOptionsArguments<Options>) =>
          request<Output>(operation, input, options[0])
    : (...options: OperationOptionsArguments<Options>) =>
        request<Output>(operation, undefined, options[0]);
  const raw = hasInput
    ? inputOptional
      ? (...args: readonly unknown[]) => {
          const [input, options] = splitOptionalOperationArguments<Input, Options>(args);
          return request.raw<Output>(operation, input, options);
        }
      : (input: Input, ...options: OperationOptionsArguments<Options>) =>
          request.raw<Output>(operation, input, options[0])
    : (...options: OperationOptionsArguments<Options>) =>
        request.raw<Output>(operation, undefined, options[0]);
  return Object.assign(call, { raw }) as OperationCall<Input, Output, Options, Raw>;
}

/** Binds generated streaming operation metadata with the same input and options dispatch as decoded calls. */
export function bindStreamOperation<Input, Item, Options extends RequestOptions = RequestOptions>(
  request: RequestFunction,
  operation: OperationDefinition,
  hasInput: boolean,
  inputOptional = false,
): (...args: readonly unknown[]) => AsyncIterable<Item> {
  return (...args: readonly unknown[]) => {
    if (!hasInput)
      return request.stream<Item>(operation, undefined, args[0] as Options | undefined);
    if (!inputOptional)
      return request.stream<Item>(operation, args[0] as Input, args[1] as Options | undefined);
    const [input, options] = splitOptionalOperationArguments<Input, Options>(args);
    return request.stream<Item>(operation, input, options);
  };
}

/**
 * Binds resource path parameters to an input operation.
 *
 * Used to implement generated instance builders such as `api.products(productID)`.
 */
export function bindPathOperation<
  FullInput,
  Input,
  Output,
  Options extends RequestOptions = RequestOptions,
  Raw = RawResponse<Output>,
>(
  operation: InputOperationCall<FullInput, Output, Options, Raw>,
  path: Readonly<Record<string, unknown>>,
  hasInput: boolean,
  inputOptional = false,
): OperationCall<Input, Output, Options, Raw> {
  const mergeInput = (input: Input | undefined): FullInput =>
    ({
      ...(isRecord(input) ? input : {}),
      path,
    }) as FullInput;
  const call = hasInput
    ? inputOptional
      ? (...args: readonly unknown[]) => {
          const [input, options] = splitOptionalOperationArguments<Input, Options>(args);
          return operation(mergeInput(input), ...operationOptionsArguments(options));
        }
      : (input: Input, ...options: OperationOptionsArguments<Options>) =>
          operation(mergeInput(input), ...options)
    : (...options: OperationOptionsArguments<Options>) =>
        operation(mergeInput(undefined), ...options);
  const raw = hasInput
    ? inputOptional
      ? (...args: readonly unknown[]) => {
          const [input, options] = splitOptionalOperationArguments<Input, Options>(args);
          return operation.raw(mergeInput(input), ...operationOptionsArguments(options));
        }
      : (input: Input, ...options: OperationOptionsArguments<Options>) =>
          operation.raw(mergeInput(input), ...options)
    : (...options: OperationOptionsArguments<Options>) =>
        operation.raw(mergeInput(undefined), ...options);
  return Object.assign(call, { raw }) as OperationCall<Input, Output, Options, Raw>;
}

/** Adds namespace members to a callable without colliding with Function prototype properties. */
export function assignCallableProperties<
  Call extends (...args: never[]) => unknown,
  Members extends object,
>(call: Call, members: Members): Call & Members {
  for (const [key, value] of Object.entries(members)) {
    Object.defineProperty(call, key, {
      value,
      enumerable: true,
      configurable: true,
      writable: true,
    });
  }
  return call as Call & Members;
}
