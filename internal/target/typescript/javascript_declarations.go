package typescript

import (
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

// emitJavaScriptDeclarationArtifacts supplies adjacent declarations for native
// ESM source. They deliberately share the TypeScript client's public type
// emitter, so resource and $operations calls stay contract-identical.
func emitJavaScriptDeclarationArtifacts(document *ir.Document, manifest Manifest) ([]Artifact, error) {
	typesSource, err := emitTypes(document)
	if err != nil {
		return nil, err
	}
	errorsSource, err := emitErrors(document)
	if err != nil {
		return nil, err
	}
	clientSource, err := emitClient(document, manifest)
	if err != nil {
		return nil, err
	}
	return []Artifact{
		{Path: "index.d.ts", Data: generatedSource([]byte("export * from \"./generated/client.js\"\nexport * from \"./generated/errors.js\"\n"))},
		{Path: "generated/client.d.ts", Data: generatedSource(clientDeclaration(clientSource))},
		{Path: "generated/errors.d.ts", Data: generatedSource(declarationFunctions(errorsSource))},
		{Path: "metadata.d.ts", Data: generatedSource(javascriptMetadataDeclaration(document))},
		{Path: "generated/runtime.d.ts", Data: generatedSource(javascriptRuntimeDeclaration())},
		{Path: "generated/types.d.ts", Data: generatedSource(declarationConstants(typesSource))},
	}, nil
}

func clientDeclaration(source []byte) []byte {
	const marker = "export function createClient(options: ClientOptions): Client {"
	value := string(source)
	index := strings.Index(value, marker)
	if index < 0 {
		return source
	}
	prefix := value[:index]
	if runtimeImportEnd := strings.Index(prefix, "} from \"./runtime.js\"\n"); runtimeImportEnd >= 0 {
		runtimeImportEnd += len("} from \"./runtime.js\"\n")
		prefix = "import type { BinaryBody, ClientOptions, PaginateInput, RawResponseFor, RequestOptions } from \"./runtime.js\"\n" + prefix[runtimeImportEnd:]
	}
	prefix = removeDeclarationRuntimeValue(prefix, "inputSchemas")
	prefix = removeDeclarationRuntimeValue(prefix, "outputSchemas")
	return []byte(prefix + "export declare function createClient(options: ClientOptions): Client\n")
}

func removeDeclarationRuntimeValue(source, name string) string {
	marker := "const " + name + ": WireSchemas = {"
	start := strings.Index(source, marker)
	if start < 0 {
		return source
	}
	brace := strings.Index(source[start:], "{") + start
	end, found := matchingTypeScriptBrace(source, brace)
	if found {
		end++
		for end < len(source) && source[end] == '\n' {
			end++
		}
		return source[:start] + source[end:]
	}
	return source
}

func declarationFunctions(source []byte) []byte {
	value := string(source)
	for {
		start := strings.Index(value, "export function ")
		if start < 0 {
			return []byte(value)
		}
		brace := strings.Index(value[start:], "{")
		if brace < 0 {
			return []byte(value)
		}
		brace += start
		end, found := matchingTypeScriptBrace(value, brace)
		if !found {
			return []byte(value)
		}
		end++
		signature := strings.TrimSpace(value[start+len("export ") : brace])
		value = value[:start] + "export declare " + signature + "\n" + value[end:]
	}
}

func declarationConstants(source []byte) []byte {
	value := string(source)
	value = strings.Replace(value, "export type SortDirection = (typeof SortDirection)[keyof typeof SortDirection]", "export type SortDirection = \"asc\" | \"desc\"", 1)
	for {
		start := strings.Index(value, "export const ")
		if start < 0 {
			return []byte(value)
		}
		brace := strings.Index(value[start:], "{")
		if brace < 0 {
			return []byte(value)
		}
		brace += start
		end, found := matchingTypeScriptBrace(value, brace)
		if !found {
			return []byte(value)
		}
		end++
		if !strings.HasPrefix(value[end:], " as const") {
			return []byte(value)
		}
		end += len(" as const")
		for end < len(value) && value[end] == '\n' {
			end++
		}
		value = value[:start] + value[end:]
	}
}

func matchingTypeScriptBrace(source string, start int) (int, bool) {
	depth := 0
	for index := start; index < len(source); index++ {
		if source[index] == '/' && index+1 < len(source) && source[index+1] == '/' {
			for index < len(source) && source[index] != '\n' {
				index++
			}
			continue
		}
		if source[index] == '/' && index+1 < len(source) && source[index+1] == '*' {
			index += 2
			for index+1 < len(source) && !(source[index] == '*' && source[index+1] == '/') {
				index++
			}
			index++
			continue
		}
		if source[index] == '\'' || source[index] == '"' || source[index] == '`' {
			quote := source[index]
			index++
			for index < len(source) {
				if source[index] == '\\' {
					index += 2
					continue
				}
				if index < len(source) && source[index] == quote {
					break
				}
				index++
			}
			continue
		}
		switch source[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index, true
			}
		}
	}
	return 0, false
}

func javascriptMetadataDeclaration(document *ir.Document) []byte {
	return []byte("/** Lossless OpenAPI metadata, kept separate from the client call surface. */\n" +
		"export declare const openapi: Readonly<{ readonly document: Readonly<Record<string, unknown>>; readonly version: " + quoteTS(document.OpenAPIVersion) + "; readonly versionLine: " + quoteTS(document.OpenAPIVersionLine) + " }>\n")
}

func javascriptRuntimeDeclaration() []byte {
	return []byte(`export declare const TransportErrorCode: {
  readonly REQUEST_ENCODE_FAILED: "REQUEST_ENCODE_FAILED"
  readonly NETWORK_ERROR: "NETWORK_ERROR"
  readonly REQUEST_ABORTED: "REQUEST_ABORTED"
  readonly REQUEST_TIMEOUT: "REQUEST_TIMEOUT"
  readonly RESPONSE_DECODE_FAILED: "RESPONSE_DECODE_FAILED"
}
export type TransportErrorCode = (typeof TransportErrorCode)[keyof typeof TransportErrorCode]
export interface RequestMetadata { readonly id?: string | undefined }
export interface APIErrorOptions<Code extends string, Details = unknown> { readonly code: Code; readonly message: string; readonly request?: RequestMetadata | undefined; readonly status?: number | undefined; readonly details?: Details | undefined; readonly fields?: unknown; readonly response?: Response | undefined; readonly cause?: unknown }
export declare class APIError<Code extends string = string, Details = unknown> extends Error { readonly code: Code; readonly request: RequestMetadata; readonly status?: number | undefined; readonly details?: Details | undefined; readonly fields?: unknown; readonly response?: Response | undefined; readonly cause?: unknown; constructor(options: APIErrorOptions<Code, Details>) }
export type TransportError = APIError<TransportErrorCode>
export declare function isAPIError(error: unknown): error is APIError
export declare function isErrorCode<Code extends string>(error: unknown, code: Code): error is APIError<Code>
export declare function getErrorCode(error: unknown): string | undefined
export declare function getRequestID(error: unknown): string | undefined
export interface ClientOptions { readonly baseURL: string; readonly fetch?: typeof globalThis.fetch | undefined; readonly headers?: HeadersInit | undefined; readonly authorization?: string | undefined; readonly credentials?: RequestCredentials | undefined; readonly timeoutMS?: number | undefined }
export interface RequestOptions { readonly signal?: AbortSignal | undefined; readonly timeoutMS?: number | undefined; readonly headers?: HeadersInit | undefined; readonly authorization?: string | undefined; readonly accept?: string | undefined; readonly csrfToken?: string | undefined; readonly requestID?: string | undefined; readonly idempotencyKey?: string | undefined; readonly ifMatch?: string | undefined; readonly credentials?: RequestCredentials | undefined }
export type BinaryBody = Blob | ArrayBuffer | ArrayBufferView
export interface RawResponse<Output> { readonly status: number; readonly contentType?: string | undefined; readonly data: Output; readonly headers: Headers; readonly request: RequestMetadata; readonly response: Response }
export type RawResponseFor<Status extends number, ContentType, Output> = Omit<RawResponse<Output>, "status" | "contentType"> & Readonly<{ status: Status; contentType: ContentType }>
export interface InputOperationCall<Input, Output, Options extends RequestOptions, Raw> { (input: Input, options?: Options): Promise<Output>; raw(input: Input, options?: Options): Promise<Raw> }
export interface NoInputOperationCall<Output, Options extends RequestOptions, Raw> { (options?: Options): Promise<Output>; raw(options?: Options): Promise<Raw> }
export type OperationCall<Input, Output, Options extends RequestOptions = RequestOptions, Raw = RawResponse<Output>> = [Input] extends [never] ? NoInputOperationCall<Output, Options, Raw> : InputOperationCall<Input, Output, Options, Raw>
export type PaginationProfile = "cursor" | "offset" | "both"
export type PaginateInput<Input, Profile extends PaginationProfile> = Input & (Profile extends "both" ? { readonly mode?: "cursor" | "offset" | undefined } : { readonly mode?: Profile | undefined })
`)
}
