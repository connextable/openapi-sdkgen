# OpenAPI 3.x SDK Generation Capability Matrix

This matrix is the implementation contract for `openapi-sdkgen`. It covers
every normative OpenAPI 3.x feature family and every Schema Object keyword
family. `generated` means observable generated source/runtime behavior;
`metadata` means exported source metadata/documentation; `error` means a
feature-path diagnostic until the target implements it. No valid feature may be
silently dropped. [The canonical feature manifest](openapi-feature-manifest.json)
is the field-level register for both TypeScript and JavaScript; this matrix and
the [capability inventory](openapi-feature-inventory.md) are readable views of it.

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
| Version detection and complete-document references | `internal/compiler/openapi/read_test.go`, `internal/compiler/compiler_test.go` | compiler, TypeScript, JavaScript |
| 3.0 / 3.1 / 3.2 output | `internal/target/typescript/version_matrix_test.go` with `oas30-sdk.json`, `oas31-sdk.json`, `oas32-sdk.json` | TypeScript, JavaScript |
| Schema lowering and unsupported vocabulary | `types_test.go`, `schema_support_test.go`, `diagnostic_test.go` | TypeScript, JavaScript diagnostics |
| HTTP operations, parameters, media, components | `openapi_support_test.go`, `types_test.go` | TypeScript, JavaScript diagnostics |
| ESM import and runtime transport parity | `javascript_test.go`, `runtime_parity_test.go` | TypeScript, JavaScript |
| Metadata/docs/examples/extensions | `metadata_test.go` | TypeScript, JavaScript |
| CLI output and consumer applications | `cmd/openapi-sdkgen/main_test.go`, `just agent example-*` | TypeScript, JavaScript |

## Version Detection and Top-level Objects

| Feature | 3.0.x | 3.1.x | 3.2.x | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- | --- | --- |
| `openapi` minor/patch detection | yes | yes | yes | Generated for SemVer `3.0.x`, `3.1.x`, `3.2.x` lines | detect each supported minor line; reject other minors/majors explicitly | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| `info` (`title`, `summary`, `description`, `termsOfService`, `contact`, `license`, `version`) | common; `summary`/license identifier later | yes | yes | Metadata: lossless `openapiDocument` export | generated API metadata/docs | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| root `servers` and defaults | yes | yes | yes | Generated first fixed URL; variables error | generated server selector + defaults | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| root `paths` | required | optional with other surface | optional with other surface | Generated when present; optional absence preserved | generated operations | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| root `webhooks` | no | yes | yes | Error required: feature-path diagnostic | generated webhook handler contracts | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| `components` | yes | yes | yes | Generated/metadata with feature-path diagnostics for unsupported semantics | reusable generated declarations/codecs | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| root `security`, `tags`, `externalDocs` | yes | yes | yes | Security errors; tags/docs exported through metadata | generated auth metadata + docs | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| `jsonSchemaDialect` | no | yes | yes | Error required: feature-path diagnostic | version/dialect-aware schema lowering | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| `$self` | no | no | yes | Error required: feature-path diagnostic | base URI for references | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| `x-*` extensions | yes | yes | yes | Metadata: lossless `openapiDocument` export plus selected project extensions | preserved metadata; documented extensions affect behavior | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |

## Reuse, References, and Servers

| Feature | Versions | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- |
| Local JSON Pointer `$ref` | all | Generated for Path Items, component schemas/responses/parameters/request bodies; Link/security forms error | resolve each supported object context | `internal/compiler/ir/build_test.go::TestBuildResolvesLocalPathsPathItemReferences` |
| External document `$ref` | all | Generated for contained `CompileFile` references; remote/out-of-root refs error | complete-document resolution under explicit resource policy | `internal/compiler/compiler_test.go::TestCompileFileBundlesInDirectoryReferencesForEverySupportedVersionLine` |
| Reference Object summary/description siblings | 3.1+ | Metadata: lossless `openapiDocument` export | preserve generated docs/metadata | `internal/compiler/compiler_test.go::TestCompileFileBundlesInDirectoryReferencesForEverySupportedVersionLine` |
| Schema `$id`, `$anchor`, `$dynamicAnchor`, `$dynamicRef` | 3.1+ | Error required: schema feature-path diagnostic | JSON Schema 2020-12 resolution or feature-path error | `internal/compiler/compiler_test.go::TestCompileFileBundlesInDirectoryReferencesForEverySupportedVersionLine` |
| `$self` URI and relative references | 3.2 | Error required: schema feature-path diagnostic | 3.2 base URI resolution | `internal/compiler/compiler_test.go::TestCompileFileBundlesInDirectoryReferencesForEverySupportedVersionLine` |
| Path Item references | all | Generated local references with sibling merge; external forms error | resolve and merge version-correctly | `internal/compiler/compiler_test.go::TestCompileFileBundlesInDirectoryReferencesForEverySupportedVersionLine` |
| Link `operationRef`, discriminator URI mapping | 3.1+ | Error required: feature-path diagnostic | resolve target operation/schema | `internal/compiler/compiler_test.go::TestCompileFileBundlesInDirectoryReferencesForEverySupportedVersionLine` |
| Reference cycles | all | Recursive component schemas generated; cyclic reusable non-schema objects error | deterministic recursive type/runtime strategy or diagnostic | `internal/target/typescript/types_test.go::TestSourceArtifactsGenerateRecursiveComponentSchemas` |
| Server URL variables (`enum`, `default`, `description`) | all | Error required: feature-path diagnostic | generated server-variable options and validation | `internal/compiler/compiler_test.go::TestCompileFileBundlesInDirectoryReferencesForEverySupportedVersionLine` |
| Root/path/operation server override and relative URL | all | Generated for fixed overrides; variables error | selected base URL at client/operation boundary | `internal/compiler/compiler_test.go::TestCompileFileBundlesInDirectoryReferencesForEverySupportedVersionLine` |

## Paths, Operations, Parameters, and Request Bodies

| Feature | 3.0.x | 3.1.x | 3.2.x | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- | --- | --- |
| Path template and path-level parameters | yes | yes | yes | Generated | typed resource builders and escaped path serialization | `internal/target/typescript/runtime_parity_test.go::TestTypeScriptAndJavaScriptTargetsHaveEquivalentRuntimeTransport` |
| Standard operations | `get put post delete options head patch trace` | same | plus `query` | Generated; `query` version-gated to 3.2 | generated calls for every allowed method | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateEveryStandardHTTPMethod` |
| `additionalOperations` | no | no | yes | Generated arbitrary methods; version-gated to 3.2 | generated arbitrary-method calls | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateOpenAPI32QueryAndAdditionalOperations` |
| Path Item summary/description/servers | version-dependent | yes | yes | Metadata/fixed server generation; variables error | operation docs/server override | `internal/target/typescript/client_test.go::TestOperationServerURLPrefersOperationThenPathOverride` |
| Operation tags/summary/description/externalDocs/operationId/deprecated | all | all | all | Metadata plus stable generated operation names | stable public names + metadata/docs | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| Operation security/server override | all | all | all | server URL override generated; non-empty security fails with feature-path diagnostic | per-call auth/server options | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| `path`, `query`, `header`, `cookie` parameters | all | all | all | Generated for supported styles; unsupported reserved escaping errors | typed input and exact wire serialization | `internal/target/typescript/runtime_parity_test.go::TestTypeScriptAndJavaScriptTargetsHaveEquivalentRuntimeTransport` |
| `querystring` parameter | no | no | yes | Error required: feature-path diagnostic | whole-querystring encoder | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| Parameter `schema` versus `content` | all | all | all | Generated for one content media type; invalid multi-media content errors | media-type-aware encoder | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsParameterSerializationItCannotRepresent` |
| Parameter `required`, `deprecated`, `allowEmptyValue`, `allowReserved` | all | all | all | Generated/metadata except `allowReserved`, which errors | input validation and correct escaping | `internal/target/typescript/runtime_parity_test.go::TestTargetsRejectMissingRequiredRuntimeInputsBeforeFetch` |
| Styles: simple, label, matrix, form, spaceDelimited, pipeDelimited, deepObject | all | all | all | Generated | exact style/explode serialization | `test/typescript/tests/runtime.test.ts::serializes paths, query styles, headers, cookies, and wire names` |
| 3.2 querystring-format extensions | no | no | yes | Error required: feature-path diagnostic | registered format encoder or explicit error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| Request body `description`, `required`, content map | all | all | all | Generated plus metadata | call input/options + media selection | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsAllowsImplementedOpenAPIHTTPFeatures` |
| JSON, text, binary bodies | all | all | all | Generated | codecs and binary types | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsAllowsImplementedOpenAPIHTTPFeatures` |
| `application/x-www-form-urlencoded` | all | all | all | Generated for scalar/array form bodies | form encoder | `test/typescript/tests/runtime.test.ts::encodes form, multipart, text, and binary bodies` |
| multipart and Encoding Object | all | all | expanded in 3.2 | basic multipart generated; Encoding Object fails with feature-path diagnostic | multipart codec including per-part headers/media | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| Nested/ordered/unnamed/streaming multipart | no/limited | limited | yes | Error required where represented through Encoding Object | generated multipart plan or explicit error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsOpenAPI32StreamingAndPositionalMediaFeatures` |

## Responses, Media, Streams, Links, Callbacks, and Webhooks

| Feature | Versions | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- |
| Exact/default/range response status keys | all | Generated for exact/default/2xx ranges | status-aware raw response union | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| Response description/headers/content/links | all | Body and metadata generated; headers and links error | decoded body, header projection, link helpers | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsResponseHeaderContracts` |
| Response media wildcards and negotiation | all | Error required for media wildcards; exact media types generated | `accept` choice and content-type decoder | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| No-content response | all | Generated `void` branch | `undefined`/void success branch | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| Binary/text/JSON response bodies | all | Generated | typed codecs | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| Sequential JSON and binary streams | 3.2 | Error required: feature-path diagnostic | `AsyncIterable`/stream API | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| Server-Sent Events | 3.2 | Error required: feature-path diagnostic | typed event stream API | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| Callback Object/key expressions | all | Error required: feature-path diagnostic | callback registration/handler contract | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| Root Webhook Object | 3.1+ | Error required: feature-path diagnostic | generated inbound handler contract | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| Link Object (`operationRef`/`operationId`, parameters, requestBody, server, runtime expressions) | all | Error required: feature-path diagnostic | follow-up operation helper | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| Example Object (`value`, `externalValue`, summary, description) | all | Metadata: lossless `openapiDocument` export | generated metadata/test vectors | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| `dataValue`/`serializedValue` examples | 3.1+ | Metadata: lossless `openapiDocument` export | serialization test vectors | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |

## Components, Security, XML, and Metadata

| Feature | Versions | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- |
| Component schemas/responses/parameters/examples/request bodies/headers | all | Schemas/responses/parameters/request bodies generated; examples metadata; headers error | reusable source declarations/codecs | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| Component security schemes/links/callbacks/path items | all | Generated path items; callbacks/links/security error | reusable runtime builders | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| Component media types | 3.2 | Error required: feature-path diagnostic | reusable media codec definitions | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| API key in header/query/cookie | all | Error required through security scheme diagnostic | typed credential provider | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| HTTP basic/bearer/other schemes | all | Error required for schemes; manual `authorization` option remains available | typed credential provider | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| OAuth2 flows/scopes and OpenID Connect | all | Error required through security scheme diagnostic | auth configuration metadata/provider hooks | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| Mutual TLS | 3.1+ | Error required through security scheme diagnostic | transport capability metadata/error for unsupported environment | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| Security requirement alternatives/optional/operation override | all | Error required for non-empty requirement | auth requirement planner | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| XML Object (`name`, namespace, prefix, attribute, wrapped) | all | Error required: schema feature-path diagnostic | XML codecs | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| 3.2 XML nodes/arrays/text/null behavior | 3.2 | Error required: schema feature-path diagnostic | version-aware XML codec | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| Tags/contact/license/docs/terms | all | Metadata: lossless `openapiDocument` export | generated API metadata/docs | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |

## Schema Object Keywords

### OAS 3.0.x Schema Object

| Keyword group | Keywords | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- |
| Numeric | `multipleOf`, `maximum`, `exclusiveMaximum` boolean, `minimum`, `exclusiveMinimum` boolean | Metadata constraint emission | input validator/metadata and correct TS representation | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| String | `maxLength`, `minLength`, `pattern`, `format` | Metadata constraint emission | validation/format metadata | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| Array | `maxItems`, `minItems`, `uniqueItems`, `items` | Generated arrays/tuples plus constraint metadata | array/tuple type and validator | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| Object | `maxProperties`, `minProperties`, `required`, `properties`, `additionalProperties` | Generated object/maps plus constraint metadata | object/map type and validator | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| Basic/composition | `title`, `type`, `enum`, `allOf`, `oneOf`, `anyOf`, `not` | Generated except `not`, `oneOf`, and `anyOf`, which error | generated intersection/codec or feature-path error | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsVariantSchemasWithoutWireBranchSelection` |
| OAS annotations | `description`, `default`, `nullable`, `discriminator`, `readOnly`, `writeOnly`, `xml`, `externalDocs`, `example`, `deprecated` | Generated/metadata except discriminator/XML, which error | input/output projection, metadata, codec | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |

### OAS 3.1.x and 3.2.x JSON Schema Draft 2020-12

| Vocabulary | Keywords | Current | Required SDK result | Evidence |
| --- | --- | --- | --- | --- |
| Core | `$id`, `$schema`, `$ref`, `$anchor`, `$dynamicRef`, `$dynamicAnchor`, `$vocabulary`, `$comment`, boolean schemas | Local component refs generated; base/dynamic/custom dialect and boolean schemas error | dialect-aware resolver/metadata/error | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsBooleanSchemasWithoutErasingTheirAssertions` |
| Applicator | `allOf`, `anyOf`, `oneOf`, `not`, `if`, `then`, `else`, `dependentSchemas`, `prefixItems`, `items`, `contains`, `minContains`, `maxContains`, `properties`, `patternProperties`, `additionalProperties`, `propertyNames`, `unevaluatedItems`, `unevaluatedProperties` | Generated `allOf`/tuples/maps; `oneOf`/`anyOf` and remaining execution semantics error | type/codec/validator lowering | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsVariantSchemasWithoutWireBranchSelection` |
| Validation | `type`, `enum`, `const`, numeric bounds, string bounds, array bounds, object bounds, `required`, `dependentRequired` | Generated types/literals plus constraint metadata; `dependentRequired` errors | version-correct type and validation lowering | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsUnimplementedSchemaFeaturesWithPaths` |
| Annotation | `title`, `description`, `default`, `deprecated`, `readOnly`, `writeOnly`, `examples`, `format` | Generated docs/projection/metadata | generated docs and projection | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsUnimplementedSchemaFeaturesWithPaths` |
| Content | `contentEncoding`, `contentMediaType`, `contentSchema` | Binary/media metadata generated; `contentSchema` errors | codec selection/metadata | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsUnimplementedSchemaFeaturesWithPaths` |
| OpenAPI annotations | `discriminator`, `xml`, `externalDocs` | discriminator/XML fail with schema feature-path diagnostics; external docs exported as metadata | generated polymorphism/XML/docs | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |

## Version-only Deltas

| Delta | 3.0.x | 3.1.x | 3.2.x | Evidence |
| --- | --- | --- | --- | --- |
| Schema base | OAS subset plus `nullable` | JSON Schema 2020-12 base | JSON Schema 2020-12 base plus 3.2 additions; each keyword is generated, metadata, or error in the manifest | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScriptAndJavaScript` |
| Nullability | `nullable: true` | `type` includes `"null"` | `type` includes `"null"` | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScriptAndJavaScript` |
| Exclusive bounds | boolean companion fields | numeric exclusive bounds | numeric exclusive bounds | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScriptAndJavaScript` |
| Examples | `example` | JSON Schema `examples` | `dataValue` and `serializedValue`, with expanded binary/serialization rules | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScriptAndJavaScript` |
| Root webhooks | absent | present | present | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScriptAndJavaScript` |
| Schema dialect | absent | `jsonSchemaDialect` | `jsonSchemaDialect` | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScriptAndJavaScript` |
| Base URI | retrieval/reference rules | JSON Schema base URI rules | `$self` plus JSON Schema base URI rules | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScriptAndJavaScript` |
| Methods/parameters | standard methods, four locations | same | adds `query`, `querystring`, `additionalOperations` | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScriptAndJavaScript` |
| Components | standard maps | standard maps | adds `mediaTypes` | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScriptAndJavaScript` |
| Media | regular/form/multipart | regular/form/multipart | streaming, sequential JSON, SSE, expanded multipart | `internal/target/typescript/version_matrix_test.go::TestVersionFeatureFixturesGenerateForTypeScriptAndJavaScript` |
