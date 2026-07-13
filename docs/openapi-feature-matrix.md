# OpenAPI 3.x SDK Generation Capability Matrix

This matrix is the implementation contract for `openapi-sdkgen`. It covers
every normative OpenAPI 3.x feature family and every Schema Object keyword
family. `generated` means observable generated source/runtime behavior;
`metadata` means exported source metadata/documentation; `error` means a
feature-path diagnostic until the target implements it. No valid feature may be
silently dropped. [The canonical feature manifest](openapi-feature-manifest.json)
is the field-level register for the active TypeScript target; this matrix and
the [capability inventory](openapi-feature-inventory.md) are readable views of it.
JavaScript output is not a supported generator target.

Authoritative specifications: [OAS 3.0.4](https://spec.openapis.org/oas/v3.0.4.html),
[OAS 3.1.1](https://spec.openapis.org/oas/v3.1.1.html), and
[OAS 3.2.0](https://spec.openapis.org/oas/v3.2.0.html).

## Status Legend

| Status | Meaning |
| --- | --- |
| Generated | Current target produces the required source/runtime behavior. |
| Metadata | Current target emits discoverable source metadata only. |
| Error required | Valid input must produce a precise diagnostic until implemented. |

## Evidence Mapping

| Matrix surface | Fixture / assertion | Targets |
| --- | --- | --- |
| Version detection and complete-document references | `internal/compiler/openapi/read_test.go`, `internal/compiler/compiler_test.go` | compiler, TypeScript |
| 3.0 / 3.1 / 3.2 output | `internal/target/typescript/version_matrix_test.go` with `oas30-sdk.json`, `oas31-sdk.json`, `oas32-sdk.json` | TypeScript |
| Schema lowering and unsupported vocabulary | `types_test.go`, `schema_support_test.go`, `diagnostic_test.go` | TypeScript diagnostics |
| HTTP operations, parameters, media, components | `openapi_support_test.go`, `types_test.go` | TypeScript diagnostics |
| ESM import and runtime transport behavior | `runtime_parity_test.go` | TypeScript |
| Metadata/docs/examples/extensions | `metadata_test.go` | TypeScript |
| CLI output and consumer applications | `cmd/openapi-sdkgen/main_test.go`, `just agent example-*` | TypeScript |

## Version Detection and Top-level Objects

| Feature | 3.0.x | 3.1.x | 3.2.x | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- | --- | --- |
| `openapi` minor/patch detection | yes | yes | yes | Generated for SemVer `3.0.x`, `3.1.x`, `3.2.x` lines | detect each supported minor line; reject other minors/majors explicitly | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| `info` (`title`, `summary`, `description`, `termsOfService`, `contact`, `license`, `version`) | common; `summary`/license identifier later | yes | yes | Metadata: lossless `metadata.js` `openapi.document` export | generated API metadata/docs | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| root `servers` and defaults | yes | yes | yes | Generated server selector, variable expansion, and defaults | generated server selector + defaults | `internal/target/typescript/runtime_parity_test.go::TestRuntimeSelectsAndExpandsOpenAPIServerVariables` |
| root `paths` | required | optional with other surface | optional with other surface | Generated when present; optional absence preserved | generated operations | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| root `webhooks` | no | yes | yes | TypeScript `--with server`: generated handler contracts; otherwise error | generated webhook handler contracts | `internal/target/typescript/server_test.go::TestGeneratedWebhookRouterExecutesThroughFetch` |
| `components` | yes | yes | yes | Generated/metadata with feature-path diagnostics for unsupported semantics | reusable generated declarations/codecs | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| root `security`, `tags`, `externalDocs` | yes | yes | yes | Client security alternatives generated through host-owned credentials; tags/docs export through metadata | generated credential plan + docs | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesOpenAPISecurityAlternativesAndOperationOverride` |
| `jsonSchemaDialect` | no | yes | yes | Generated dialect metadata drives schema lowering and vocabulary handling | version/dialect-aware schema lowering | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsAcceptsJSONSchemaResourceScopeMetadata` |
| `$self` | no | no | yes | Generated as the document schema-resource base URI | base URI for references | `internal/compiler/compiler_test.go::TestCompileUsesOpenAPI32SelfAsSchemaResourceBase` |
| `x-*` extensions | yes | yes | yes | Metadata: lossless `metadata.js` `openapi.document` export plus selected project extensions | preserved metadata; documented extensions affect behavior | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |

## Reuse, References, and Servers

| Feature | Versions | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- |
| Local JSON Pointer `$ref` | all | Generated for Path Items, component schemas/responses/parameters/request bodies/Links; Security forms generated | resolve each supported object context | `internal/target/typescript/runtime_parity_test.go::TestGeneratedResponseLinksFollowTypedTargetOperations` |
| External document `$ref` | all | Contained local refs; allowlisted HTTPS refs with lock/cache/offline support; out-of-root file refs error | complete-document resolution under explicit resource policy | `internal/compiler/references_test.go::TestCompileFileWithOptionsUsesLockedOfflineRemoteReference` |
| Reference Object summary/description siblings | 3.1+ | Metadata: lossless `metadata.js` `openapi.document` export | preserve generated docs/metadata | `internal/compiler/compiler_test.go::TestCompileFileBundlesInDirectoryReferencesForEverySupportedVersionLine` |
| Schema `$id`, `$anchor`, `$dynamicAnchor`, `$dynamicRef` | 3.1+ | Generated: canonical pointers and dynamic-scope runtime selection across contained and locked remote schema resources | JSON Schema 2020-12 local and locked remote resource resolution | `internal/target/typescript/runtime_parity_test.go::TestRuntimeResolvesDynamicReferencesAcrossLockedRemoteSchemaResources` |
| `$self` URI and relative references | 3.2 | Generated: `$self` establishes the document schema-resource base for relative `$id`/anchor resolution | 3.2 base URI resolution | `internal/compiler/compiler_test.go::TestCompileUsesOpenAPI32SelfAsSchemaResourceBase` |
| Path Item references | all | Generated local references with sibling merge; external forms error | resolve and merge version-correctly | `internal/compiler/compiler_test.go::TestCompileFileBundlesInDirectoryReferencesForEverySupportedVersionLine` |
| Link local `operationRef`, discriminator URI mapping | 3.1+ | Generated local target operation/schema resolution | resolve target operation/schema | `internal/target/typescript/runtime_parity_test.go::TestGeneratedResponseLinksFollowTypedTargetOperations` |
| Reference cycles | all | Recursive component schemas generated; cyclic reusable non-schema objects error | deterministic recursive type/runtime strategy or diagnostic | `internal/target/typescript/types_test.go::TestSourceArtifactsGenerateRecursiveComponentSchemas` |
| Server URL variables (`enum`, `default`, `description`) | all | Generated enum/default selection and validation; description metadata retained | generated server-variable options and validation | `internal/target/typescript/runtime_parity_test.go::TestRuntimeSelectsAndExpandsOpenAPIServerVariables` |
| Root/path/operation server override and relative URL | all | Generated selected server ID, scoped inheritance, relative origin resolution, and explicit baseURL override | selected base URL at client/operation boundary | `internal/target/typescript/runtime_parity_test.go::TestRuntimeSelectsAndExpandsOpenAPIServerVariables` |

## Paths, Operations, Parameters, and Request Bodies

| Feature | 3.0.x | 3.1.x | 3.2.x | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- | --- | --- |
| Path template and path-level parameters | yes | yes | yes | Generated | typed resource builders and escaped path serialization | `internal/target/typescript/runtime_parity_test.go::TestVersionedTypeScriptRuntime` |
| Standard operations | `get put post delete options head patch trace` | same | plus `query` | Generated; `query` version-gated to 3.2 | generated calls for every allowed method | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateEveryStandardHTTPMethod` |
| `additionalOperations` | no | no | yes | Generated arbitrary methods; version-gated to 3.2 | generated arbitrary-method calls | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateOpenAPI32QueryAndAdditionalOperations` |
| Path Item summary/description/servers | version-dependent | yes | yes | Metadata plus selected server alternatives and variables generated | operation docs/server override | `internal/target/typescript/runtime_parity_test.go::TestRuntimeSelectsOperationScopedServerAlternatives` |
| Operation tags/summary/description/externalDocs/operationId/deprecated | all | all | all | Metadata plus stable generated operation names | stable public names + metadata/docs | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| Operation security/server override | all | all | all | server URL override and security override/alternatives generated | per-call auth/server options | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesOpenAPISecurityAlternativesAndOperationOverride` |
| `path`, `query`, `header`, `cookie` parameters | all | all | all | Generated with style-aware wire serialization, reserved expansion, and cookie transport capability checks | typed input and exact wire serialization | `internal/target/typescript/runtime_parity_test.go::TestVersionedTypeScriptRuntime` |
| `querystring` parameter | no | no | yes | Generated for whole-query form and content serialization | whole-querystring encoder | `internal/target/typescript/runtime_parity_test.go::TestRuntimeSerializesOpenAPI32QuerystringFormContent` |
| Parameter `schema` versus `content` | all | all | all | Generated for one content media type, including custom registered codecs; invalid multi-media content errors | media-type-aware encoder | `internal/target/typescript/runtime_parity_test.go::TestRuntimeUsesAsyncCustomCodecsForParameterContent` |
| Parameter `required`, `deprecated`, `allowEmptyValue`, `allowReserved` | all | all | all | Generated/metadata, required-input validation, and reserved-character escaping | input validation and correct escaping | `internal/target/typescript/runtime_parity_test.go::TestRuntimePreservesReservedQueryCharactersWhenAllowed` |
| Styles: simple, label, matrix, form, spaceDelimited, pipeDelimited, deepObject | all | all | all | Generated | exact style/explode serialization | `test/typescript/tests/runtime.test.ts::serializes paths, query styles, headers, cookies, and wire names` |
| 3.2 querystring-format extensions | no | no | yes | Generated through built-in form/JSON/XML handling and caller-registered custom media codecs | registered format encoder | `internal/target/typescript/runtime_parity_test.go::TestRuntimeUsesAsyncCustomCodecsForParameterContent` |
| Request body `description`, `required`, content map | all | all | all | Generated plus metadata | call input/options + media selection | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsAllowsImplementedOpenAPIHTTPFeatures` |
| JSON, text, binary bodies | all | all | all | Generated | codecs and binary types | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsAllowsImplementedOpenAPIHTTPFeatures` |
| `application/x-www-form-urlencoded` | all | all | all | Generated for scalar/array form bodies | form encoder | `test/typescript/tests/runtime.test.ts::encodes form, multipart, text, and binary bodies` |
| multipart and Encoding Object | all | all | expanded in 3.2 | generated per-property media, serialization, and declared part-header plan | multipart codec including per-part headers/media | `internal/target/typescript/runtime_parity_test.go::TestRuntimeEncodesOpenAPIEncodingObjectMultipartParts` |
| Nested/ordered/unnamed/streaming multipart | no/limited | limited | yes | Generated ordered/unnamed, streaming, and required one-level nested multipart request/response part plans | generated multipart plan | `internal/target/typescript/runtime_parity_test.go::TestGeneratedNestedMultipartRequestAndResponseRoundTrip` |

## Responses, Media, Streams, Links, Callbacks, and Webhooks

| Feature | Versions | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- |
| Exact/default/range response status keys | all | Generated for exact/default/2xx ranges | status-aware raw response union | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| Response description/headers/content/links | all | Body, typed headers, metadata, and typed `$links` helpers generated | decoded body, header projection, link helpers | `internal/target/typescript/runtime_parity_test.go::TestGeneratedResponseLinksFollowTypedTargetOperations` |
| Response media wildcards and negotiation | all | Exact, `type/*`, `*/*`, and `*+suffix` response matching generated; request media ranges use explicit `{ contentType, value }` input selection | content-type decoder/encoder chooses the most specific declaration | `internal/target/typescript/runtime_parity_test.go::TestRuntimeSelectsDeclaredRequestMediaRanges` |
| No-content response | all | Generated `void` branch | `undefined`/void success branch | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| Binary/text/JSON response bodies | all | Generated | typed codecs | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| Sequential JSON and binary streams | 3.2 | Generated typed `AsyncIterable`; NDJSON/JSON Lines/JSON-seq built in, binary/custom sequences through registered codecs | `AsyncIterable`/stream API | `internal/target/typescript/runtime_parity_test.go::TestRuntimeUsesRegisteredCustomResponseStreamCodec` |
| Server-Sent Events | 3.2 | Generated typed `AsyncIterable` with bounded frames and reader cleanup; no automatic reconnect | typed event stream API | `internal/target/typescript/runtime_parity_test.go::TestGeneratedResponseStreamsDecodeNDJSONItemsLazily` |
| Callback Object/key expressions | all | TypeScript `--with server`: host-bound Fetch callback endpoints; otherwise error | callback registration/handler contract | `internal/target/typescript/server_test.go::TestGeneratedCallbackEndpointsAreHostBoundAndRoundTripJSON` |
| Root Webhook Object | 3.1+ | TypeScript `--with server`: Fetch router and typed handler; otherwise error | generated inbound handler contract | `internal/target/typescript/server_test.go::TestGeneratedWebhookRouterExecutesThroughFetch` |
| Link Object (`operationId`/local `operationRef`, parameters, requestBody, response/request body/header/status expressions) | all | Generated typed follow-up helper plus status-dispatch and `byStatus` helpers | follow-up operation helper | `internal/target/typescript/runtime_parity_test.go::TestGeneratedResponseLinksDispatchSameNameByStatus` |
| Example Object (`value`, `externalValue`, summary, description) | all | Metadata: lossless `metadata.js` `openapi.document` export | generated metadata/test vectors | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| `dataValue`/`serializedValue` examples | 3.1+ | Metadata: lossless `metadata.js` `openapi.document` export | serialization test vectors | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |

## Components, Security, XML, and Metadata

| Feature | Versions | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- |
| Component schemas/responses/parameters/examples/request bodies/headers | all | Schemas/responses/parameters/request bodies/headers generated; examples metadata | reusable source declarations/codecs | `internal/target/typescript/runtime_parity_test.go::TestRuntimeDecodesDeclaredResponseHeaders` |
| Component security schemes/links/path items | all | Generated security schemes/path items and response-referenced Links | reusable runtime builders | `internal/target/typescript/runtime_parity_test.go::TestGeneratedResponseLinksFollowTypedTargetOperations` |
| Component callbacks | all | TypeScript `--with server`: host-bound Fetch callback endpoints; otherwise error | reusable callback endpoints | `internal/target/typescript/server_test.go::TestGeneratedCallbackEndpointsAreHostBoundAndRoundTripJSON` |
| Component media types | 3.2 | Generated for request, response, parameter, header, and server media declarations | reusable media codec definitions | `internal/target/typescript/runtime_parity_test.go::TestRuntimeResolvesReusableOpenAPI32MediaTypes` |
| API key in header/query/cookie | all | Generated host credential provider applies every declared API-key wire location; inbound server preserves candidates for host authentication | typed credential provider | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesEveryHostManagedSecurityCredentialShape` |
| HTTP basic/bearer/other schemes | all | Generated host credential provider with collision checks | typed credential provider | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesEveryHostManagedSecurityCredentialShape` |
| OAuth2 flows/scopes and OpenID Connect | all | Generated provider declaration metadata and token application | auth configuration metadata/provider hooks | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesEveryHostManagedSecurityCredentialShape` |
| Mutual TLS | 3.1+ | Generated capability-gated host transport requirement | transport capability metadata/error for unsupported environment | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesEveryHostManagedSecurityCredentialShape` |
| Security requirement alternatives/optional/operation override | all | Generated alternative planner; explicit empty operation override disables inheritance | auth requirement planner | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesOpenAPISecurityAlternativesAndOperationOverride` |
| XML Object (`name`, namespace, prefix, attribute, wrapped) | all | Generated XML request/response codecs | XML codecs | `internal/target/typescript/runtime_parity_test.go::TestRuntimeEncodesAndDecodesOpenAPIXMLObjects` |
| 3.2 XML nodes/arrays/text/null behavior | 3.2 | Generated through the version-aware XML wire codec | version-aware XML codec | `internal/target/typescript/runtime_parity_test.go::TestRuntimeEncodesAndDecodesOpenAPIXMLObjects` |
| Tags/contact/license/docs/terms | all | Metadata: lossless `metadata.js` `openapi.document` export | generated API metadata/docs | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |

## Schema Object Keywords

### OAS 3.0.x Schema Object

| Keyword group | Keywords | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- |
| Numeric | `multipleOf`, `maximum`, `exclusiveMaximum` boolean, `minimum`, `exclusiveMinimum` boolean | Metadata constraint emission | input validator/metadata and correct TS representation | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| String | `maxLength`, `minLength`, `pattern`, `format` | Generated runtime validation; `format` stays annotation-only unless format-assertion is required | validation/format assertion lowering | `internal/target/typescript/runtime_parity_test.go::TestRuntimeSupportsStandardFormatAssertionRegistry` |
| Array | `maxItems`, `minItems`, `uniqueItems`, `items` | Generated arrays/tuples plus constraint metadata | array/tuple type and validator | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| Object | `maxProperties`, `minProperties`, `required`, `properties`, `additionalProperties` | Generated object/maps plus constraint metadata | object/map type and validator | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| Basic/composition | `title`, `type`, `enum`, `allOf`, `oneOf`, `anyOf`, `not` | Generated | generated intersection/codec/validator | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsEmitsNegatedSchemaAssertion` |
| OAS annotations | `description`, `default`, `nullable`, `discriminator`, `readOnly`, `writeOnly`, `xml`, `externalDocs`, `example`, `deprecated` | Generated/metadata | input/output projection, metadata, codec | `internal/target/typescript/runtime_parity_test.go::TestRuntimeEncodesAndDecodesOpenAPIXMLObjects` |

### OAS 3.1.x and 3.2.x JSON Schema Draft 2020-12

| Vocabulary | Keywords | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- |
| Core | `$id`, `$schema`, `$ref`, `$anchor`, `$dynamicRef`, `$dynamicAnchor`, `$vocabulary`, `$comment`, boolean schemas | Local resource references, anchors, dynamic scope, boolean schemas, and trusted compile-time custom-vocabulary lowering generated | dialect-aware resolver/metadata/error | `internal/compiler/references_test.go::TestCompileFileLowersRequiredCustomVocabularyBeforeTargetGeneration` |
| Applicator | `allOf`, `anyOf`, `oneOf`, `not`, `if`, `then`, `else`, `dependentSchemas`, `prefixItems`, `items`, `contains`, `minContains`, `maxContains`, `properties`, `patternProperties`, `additionalProperties`, `propertyNames`, `unevaluatedItems`, `unevaluatedProperties` | Generated executable validation, wire transformation, and TypeScript lowering | type/codec/validator lowering | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsEmitsVariantWireBranchSelection` |
| Validation | `type`, `enum`, `const`, numeric bounds, string bounds, array bounds, object bounds, `required`, `dependentRequired` | Generated types/literals and executable validation | version-correct type and validation lowering | `internal/target/typescript/runtime_parity_test.go::TestSchemaRuntimeRejectsNumericBoundsBeforeFetch` |
| Annotation | `title`, `description`, `default`, `deprecated`, `readOnly`, `writeOnly`, `examples`, `format` | Generated docs/projection/metadata | generated docs and projection | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsUnimplementedSchemaFeaturesWithPaths` |
| Content | `contentEncoding`, `contentMediaType`, `contentSchema` | Generated decoded-content validation without changing the outer string | codec selection/metadata | `internal/target/typescript/runtime_parity_test.go::TestRuntimeValidatesJSONSchemaContentSchema` |
| OpenAPI annotations | `discriminator`, `xml`, `externalDocs` | generated polymorphism/XML; external docs exported as metadata | generated polymorphism/XML/docs | `internal/target/typescript/runtime_parity_test.go::TestRuntimeEncodesAndDecodesOpenAPIXMLObjects` |

## Version-only Deltas

| Delta | 3.0.x | 3.1.x | 3.2.x | Evidence |
| --- | --- | --- | --- | --- |
| Schema base | OAS subset plus `nullable` | JSON Schema 2020-12 base | JSON Schema 2020-12 base plus 3.2 additions; each keyword is generated, metadata, or error in the manifest | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScript` |
| Nullability | `nullable: true` | `type` includes `"null"` | `type` includes `"null"` | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScript` |
| Exclusive bounds | boolean companion fields | numeric exclusive bounds | numeric exclusive bounds | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScript` |
| Examples | `example` | JSON Schema `examples` | `dataValue` and `serializedValue`, with expanded binary/serialization rules | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScript` |
| Root webhooks | absent | present | present | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScript` |
| Schema dialect | absent | `jsonSchemaDialect` | `jsonSchemaDialect` | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScript` |
| Base URI | retrieval/reference rules | JSON Schema base URI rules | `$self` plus JSON Schema base URI rules | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScript` |
| Methods/parameters | standard methods, four locations | same | adds `query`, `querystring`, `additionalOperations` | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScript` |
| Components | standard maps | standard maps | adds `mediaTypes` | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScript` |
| Media | regular/form/multipart | regular/form/multipart | streaming, sequential JSON, SSE, expanded multipart | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScript` |
