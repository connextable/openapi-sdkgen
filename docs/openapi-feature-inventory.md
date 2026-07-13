# OpenAPI 3.x Atomic SDK Capability Inventory

This is the grouped, human-readable companion to
[`openapi-feature-matrix.md`](openapi-feature-matrix.md). Every listed surface
has one base-client target state only: `generated`, `metadata`, or `error`.
An optional add-on may override that state through its documented manifest
condition; for example Webhooks remain a client-only `error` but are generated
by `typescript --with server`. `error` means generation stops before output for
that selected target and includes the JSON Pointer of the feature. `metadata` means the lossless `metadata.js` `openapi.document` export exposes
the source value, but does not invent a runtime API.

The canonical field-level source is
[`openapi-feature-manifest.json`](openapi-feature-manifest.json). It uses one
ID, state, version scope, and executable evidence reference per field. This
grouped view covers the common OpenAPI 3.x surface once; a field appears in a
version-only table only when its availability or semantics differ by minor
line.

## Common Document and Discovery Surface

| ID | Fields | Versions | State | Evidence |
| --- | --- | --- | --- | --- |
| document-version | `openapi` SemVer | all | generated | `internal/compiler/openapi/read_test.go::TestReadBuildsSupportedOpenAPI3Models` |
| info-documentation | `info.title`, `description`, `termsOfService`, `contact`, `license`, `version` | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| document-discovery | root `tags`, `externalDocs` | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| extensions | every `x-*` patterned field | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| fixed-server | server URL without variables; root/path/operation override | all | generated | `internal/target/typescript/client_test.go::TestOperationServerURLPrefersOperationThenPathOverride` |
| scoped-server-alternatives | multiple root/path/operation Server Objects selected by stable ID | all | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeSelectsOperationScopedServerAlternatives` |
| server-variables | server-variable `enum`, `default` | all | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeSelectsAndExpandsOpenAPIServerVariables` |
| server-variable-description | server-variable `description` | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| paths | root Paths Object and path template matching | all | generated | `internal/compiler/ir/build_test.go::TestBuildExtractsOperationsDeterministically` |
| path-item-docs | Path Item `summary`, `description`, extensions | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| operation-identity | Operation `operationId` | all | generated | `internal/target/typescript/emit_test.go::TestSourceArtifactsStayConsistentAndDeterministic` |
| operation-docs | Operation `tags`, `summary`, `description`, `externalDocs`, `deprecated`, extensions | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |

## Reuse and Components

| ID | Fields | Versions | State | Evidence |
| --- | --- | --- | --- | --- |
| local-path-item-references | local JSON Pointer Path Item `$ref` from `paths` and `components.pathItems` | all; `components.pathItems` 3.1+ | generated | `internal/compiler/ir/build_test.go::TestBuildResolvesLocalPathsPathItemReferences` |
| local-component-response-references | local `components.responses` `$ref` | all | generated | `internal/target/typescript/references_test.go::TestOperationWireBodiesResolveReusableComponents` |
| local-component-parameter-references | local `components.parameters` `$ref` | all | generated | `internal/target/typescript/references_test.go::TestResolveComponentObjectDecodesEscapedComponentNames` |
| local-component-request-body-references | local `components.requestBodies` `$ref` | all | generated | `internal/target/typescript/references_test.go::TestOperationWireBodiesResolveReusableComponents` |
| local-component-schema-references | direct local `components.schemas` `$ref` | all | generated | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsDecodesEscapedComponentSchemaReferences` |
| local-link-security-references | Link local `operationRef` and Security Requirement implicit references | all | generated | `internal/target/typescript/runtime_parity_test.go::TestGeneratedResponseLinksFollowTypedTargetOperations` |
| component-schema-reference-escapes | RFC 6901 `~0`/`~1` component schema names | all | generated | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsDecodesEscapedComponentSchemaReferences` |
| nested-schema-pointers | component Schema Object JSON Pointers | all | generated | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsAcceptsNestedComponentSchemaReferencesAfterCompilation` |
| contained-external-references | file references inside the input root | all | generated | `internal/compiler/compiler_test.go::TestCompileFileBundlesInDirectoryReferencesForEverySupportedVersionLine` |
| remote-references | allowlisted HTTPS `$ref` with lockfile, content-addressed cache, DNS policy, and offline mode | all | generated | `internal/compiler/references_test.go::TestCompileFileWithOptionsUsesLockedOfflineRemoteReference` |
| escaping-references | file `$ref` outside the input root | all | error | `internal/compiler/compiler_test.go::TestCompileFileRejectsReferenceOutsideInputDirectory` |
| reference-cycles | cyclic reusable schema references | all | generated | `internal/target/typescript/types_test.go::TestSourceArtifactsGenerateRecursiveComponentSchemas` |
| schema-components | `components.schemas` object schemas | all | generated | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| reusable-responses | `components.responses` | all | generated | `internal/target/typescript/emit_test.go::TestSourceArtifactsStayConsistentAndDeterministic` |
| reusable-parameters | `components.parameters` | all | generated | `internal/target/typescript/emit_test.go::TestSourceArtifactsStayConsistentAndDeterministic` |
| reusable-request-bodies | `components.requestBodies` | all | generated | `internal/target/typescript/emit_test.go::TestSourceArtifactsStayConsistentAndDeterministic` |
| reusable-headers | `components.headers` | all | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeDecodesDeclaredResponseHeaders` |
| reusable-examples | `components.examples` | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| reusable-links | `components.links` referenced by Response Objects | all | generated | `internal/target/typescript/runtime_parity_test.go::TestGeneratedResponseLinksFollowTypedTargetOperations` |
| reusable-security-schemes | `components.securitySchemes` | all | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesEveryHostManagedSecurityCredentialShape` |
| reusable-path-items | `components.pathItems` | 3.1+ | generated | `internal/compiler/ir/build_test.go::TestBuildResolvesLocalPathItemReferences` |
| reusable-media-types | `components.mediaTypes` | 3.2 | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeResolvesReusableOpenAPI32MediaTypes` |

## HTTP Calls, Parameters, and Bodies

| ID | Fields | Versions | State | Evidence |
| --- | --- | --- | --- | --- |
| standard-methods | `get`, `put`, `post`, `delete`, `options`, `head`, `patch`, `trace` | all | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateEveryStandardHTTPMethod` |
| query-method | `query` | 3.2 | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateOpenAPI32QueryAndAdditionalOperations` |
| additional-operations | `additionalOperations` arbitrary methods | 3.2 | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateOpenAPI32QueryAndAdditionalOperations` |
| parameter-locations | `path`, `query`, `header`, `cookie` | all | generated | `internal/target/typescript/runtime_parity_test.go::TestVersionedTypeScriptRuntime` |
| parameter-serialization | scalar/array `simple`, `label`, `matrix`, `form`, `spaceDelimited`, `pipeDelimited`, `deepObject`, `explode` | all | generated | `test/typescript/tests/runtime.test.ts::serializes paths, query styles, headers, cookies, and wire names` |
| parameter-delimited-object | `spaceDelimited` and `pipeDelimited` object parameters | all | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeSerializesDelimitedObjectQueryParameters` |
| parameter-schema-content | parameter `schema`, single-media `content` | all | generated | `internal/target/typescript/emit_test.go::TestSourceArtifactsStayConsistentAndDeterministic` |
| parameter-docs | parameter `description`, `deprecated`, examples | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| required-inputs | required path/query/header/cookie parameters and request body | all | generated | `internal/target/typescript/runtime_parity_test.go::TestTargetsRejectMissingRequiredRuntimeInputsBeforeFetch` |
| parameter-structured-non-json-content | structured single-media Parameter Object `content`, including caller-registered custom media codecs | all | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeUsesAsyncCustomCodecsForParameterContent` |
| parameter-allow-reserved | `allowReserved: true` | all | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimePreservesReservedQueryCharactersWhenAllowed` |
| parameter-allow-empty-value | `allowEmptyValue: true` | all | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsSupportsAllowEmptyValue` |
| parameter-querystring | `in: querystring` content serialization | 3.2 | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeSerializesOpenAPI32QuerystringFormContent` |
| request-basic-media | JSON, text, binary, `application/x-www-form-urlencoded`, basic multipart | all | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsAllowsImplementedOpenAPIHTTPFeatures` |
| xml-media | `application/xml` and XML-suffixed media types | all | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeEncodesAndDecodesOpenAPIXMLObjects` |
| request-encoding | Media Type `encoding`, per-part media types and declared headers | all | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeEncodesOpenAPIEncodingObjectMultipartParts` |
| media-item-schema-and-positional-encoding | Media Type `itemSchema`; NDJSON/JSON Lines/JSON-sequence/SSE, multipart, and registered custom-media request/response streams; ordered/unnamed positional multipart with `prefixEncoding`/`itemEncoding` | 3.2 | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeUsesRegisteredCustomRequestStreamCodec` |
| request-description-examples | Request Body description and Media Type examples | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |

## Responses, Asynchronous Features, and Security

| ID | Fields | Versions | State | Evidence |
| --- | --- | --- | --- | --- |
| response-status-selection | exact/default/range status keys and no-content | all | generated | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| default-response-media-negotiation | `default` Response Object media types in generated `accept` options | all | generated | `internal/target/typescript/client_test.go::TestOperationResponseMediaTypesIncludesDefaultResponses` |
| response-body-media | JSON, text, and binary response bodies | all | generated | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| response-docs | Response `description` and examples | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| response-headers | Response Object and Header Object declarations | all | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeDecodesDeclaredResponseHeaders` |
| response-media-wildcards | exact, type, and structured-suffix wildcard response media matching | all | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeDecodesWildcardResponseMediaTypes` |
| response-streams | NDJSON/JSON Lines, JSON-seq, Server-Sent Event, multipart, and registered custom-media item streams | 3.2 | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeUsesRegisteredCustomResponseStreamCodec` |
| response-links | Link Object `operationId`/local `operationRef`, response/request body/header/status expressions, parameters, requestBody, and same-name status dispatch | all | generated | `internal/target/typescript/runtime_parity_test.go::TestGeneratedResponseLinksDispatchSameNameByStatus` |
| callbacks | Callback Object, key expressions, callback Path Items; generated only by `typescript --with server` | all | error | `internal/target/typescript/server_test.go::TestGeneratedCallbackEndpointsAreHostBoundAndRoundTripJSON` |
| webhooks | root Webhook Object; generated only by `typescript --with server` | 3.1+ | error | `internal/target/typescript/server_test.go::TestGeneratedWebhookRouterExecutesThroughFetch` |
| examples | Example `summary`, `description`, `value`, `externalValue` | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| security-requirements | root/operation requirements, alternatives, and explicit overrides | all | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesOpenAPISecurityAlternativesAndOperationOverride` |
| security-schemes | API key, HTTP, OAuth2, OpenID Connect, and mTLS host providers with capability gates | all; mutual TLS 3.1+ | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesEveryHostManagedSecurityCredentialShape` |

## Schema Object: OAS 3.0.x

| ID | Fields | Versions | State | Evidence |
| --- | --- | --- | --- | --- |
| oas30-scalar-types | `type`, `enum` | 3.0 | generated | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| oas30-scalar-constraints | `multipleOf`, bounds, string constraints, `format`, `default`, `example` | 3.0 | metadata | `internal/target/typescript/types_test.go::TestSchemaConstraintSummaryListsValidationOnlyKeywords` |
| oas30-arrays-objects | `items`, `properties`, `required`, schema-valued `additionalProperties` | 3.0 | generated | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| oas30-closed-objects | `additionalProperties: false` | 3.0 | generated | `internal/target/typescript/types_test.go::TestSourceArtifactsEmitsClosedObjectRuntimeValidation` |
| oas30-array-constraints | `maxItems`, `minItems`, `uniqueItems` | 3.0 | metadata | `internal/target/typescript/types_test.go::TestSchemaConstraintSummaryListsValidationOnlyKeywords` |
| oas30-object-constraints | `maxProperties`, `minProperties` | 3.0 | metadata | `internal/target/typescript/types_test.go::TestSchemaConstraintSummaryListsValidationOnlyKeywords` |
| oas30-all-of | `allOf` | 3.0 | generated | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| oas30-variant-composition | `oneOf`, `anyOf` | 3.0 | generated | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsEmitsVariantWireBranchSelection` |
| oas30-negation | `not` | 3.0 | generated | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsEmitsNegatedSchemaAssertion` |
| oas30-nullability-projection | `nullable`, `readOnly`, `writeOnly` | 3.0 | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateAcrossSupportedOpenAPIVersionLines` |
| oas30-schema-deprecated | Schema `deprecated` | 3.0 | metadata | `internal/target/typescript/emit_test.go::TestSourceArtifactsStayConsistentAndDeterministic` |
| oas30-discriminator | `discriminator` mappings over `oneOf` variants; 3.2 `defaultMapping` over `oneOf`/`anyOf` | 3.0; default mapping 3.2 | generated | `internal/target/typescript/runtime_parity_test.go::TestOpenAPI32DiscriminatorDefaultMappingSelectsAnyOfTransform` |
| oas30-xml | `xml` | 3.0 | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeEncodesAndDecodesOpenAPIXMLObjects` |
| oas30-schema-external-docs | Schema `externalDocs` | 3.0 | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |

## Schema Object: JSON Schema 2020-12

| ID | Fields | Versions | State | Evidence |
| --- | --- | --- | --- | --- |
| jsonschema-structural | `$ref`, `type` including arrays and `null`, `const`, `enum`, `allOf`, `prefixItems`, `items`, `properties`, schema-valued `additionalProperties` | 3.1+ | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateAcrossSupportedOpenAPIVersionLines` |
| jsonschema-variant-composition | `oneOf`, `anyOf` | 3.1+ | generated | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsEmitsVariantWireBranchSelection` |
| jsonschema-closed-objects | `additionalProperties: false` | 3.1+ | generated | `internal/target/typescript/types_test.go::TestSourceArtifactsEmitsClosedObjectRuntimeValidation` |
| jsonschema-pattern-properties | `patternProperties` | 3.1+ | generated | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsEmitsPatternPropertyWireSemantics` |
| jsonschema-doc-annotations | `title`, `description`, `default`, `deprecated`, `examples`, `contentEncoding`, `contentMediaType` | 3.1+ | metadata | `internal/target/typescript/types_test.go::TestSchemaConstraintSummaryListsValidationOnlyKeywords` |
| jsonschema-format | `format` annotation; standard format assertion when its vocabulary is required | 3.1+ | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeSupportsStandardFormatAssertionRegistry` |
| jsonschema-numeric-string-array-object-constraints | `multipleOf`, bounds, string bounds, array bounds, object bounds | 3.1+ | metadata | `internal/target/typescript/types_test.go::TestSchemaConstraintSummaryListsValidationOnlyKeywords` |
| jsonschema-directional-annotations | `readOnly`, `writeOnly` | 3.1+ | generated | `internal/target/typescript/types_test.go::TestSchemaTypeProjectsReadAndWriteOnlyProperties` |
| jsonschema-resource-dialect | `$id`, `$schema`, `$anchor`, `$dynamicAnchor`, `$dynamicRef`, `$comment` | 3.1+ | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeResolvesDynamicReferencesAcrossLockedRemoteSchemaResources` |
| jsonschema-vocabulary | `$vocabulary` | 3.1+ | generated | `internal/compiler/references_test.go::TestCompileFileLowersRequiredCustomVocabularyBeforeTargetGeneration` |
| jsonschema-definitions | `$defs` with local-pointer references | 3.1+ | generated | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsLowersLocalDefinitionsBeforeTypeScriptEmission` |
| jsonschema-boolean | boolean schemas | 3.1+ | generated | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsEmitsBooleanSchemaFromAnOpenAPI31Document` |
| jsonschema-applicator-assertions | `not`, `if`, `then`, `else`, `dependentSchemas`, `contains`, `minContains`, `maxContains`, `propertyNames` | 3.1+ | generated | `internal/target/typescript/runtime_parity_test.go::TestVariantRuntimeRejectsValuesMatchingNoBranch` |
| jsonschema-unevaluated | `unevaluatedItems`, `unevaluatedProperties`, including `allOf` evaluated members | 3.1+ | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeRejectsUnevaluatedPropertiesAndItemsBeforeFetch` |
| jsonschema-validation-assertions | `dependentRequired`, `contentSchema` | 3.1+ | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeValidatesJSONSchemaContentSchema` |
| jsonschema-discriminator | `discriminator` mappings over `oneOf` variants | 3.1+ | generated | `internal/target/typescript/runtime_parity_test.go::TestDiscriminatorRuntimeSelectsMappedBranch` |
| jsonschema-xml-annotation | `xml` | 3.1+ | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeEncodesAndDecodesOpenAPIXMLObjects` |
| jsonschema-schema-external-docs | Schema `externalDocs` | 3.1+ | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |

## Version-only Syntax and Semantics

| ID | Fields | Versions | State | Evidence |
| --- | --- | --- | --- | --- |
| oas30-nullable | `nullable` | 3.0 | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateAcrossSupportedOpenAPIVersionLines` |
| oas30-bound-syntax | boolean `exclusiveMaximum` and `exclusiveMinimum` | 3.0 | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesOpenAPI30ExclusiveBoundsAndRejectsNonNullableNull` |
| oas30-schema-version-gate | type arrays, numeric exclusive bounds, and 3.1 JSON Schema keywords | 3.0 | error | `internal/compiler/openapi/read_test.go::TestReadRejectsLaterMinorFeaturesAtJSONPointers` |
| oas31-documentation | `info.summary`, `license.identifier` | 3.1+ | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| oas31-numeric-bounds | numeric `exclusiveMaximum`, numeric `exclusiveMinimum` | 3.1+ | generated | `internal/target/typescript/runtime_parity_test.go::TestSchemaRuntimeRejectsNumericBoundsBeforeFetch` |
| oas31-legacy-nullable | legacy `nullable` | 3.1+ | metadata | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsDoesNotApplyOpenAPI30NullableToOpenAPI31Schemas` |
| oas31-schema-dialect | `jsonSchemaDialect` | 3.1+ | metadata | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsAcceptsJSONSchemaResourceScopeMetadata` |
| oas32-base-uri | `$self` and 3.2 base URI semantics | 3.2 | generated | `internal/compiler/compiler_test.go::TestCompileUsesOpenAPI32SelfAsSchemaResourceBase` |
| oas32-server-name | Server Object `name` | 3.2 | metadata | `internal/compiler/openapi/read_test.go::TestReadAcceptsOpenAPI32OnlyFields` |
| oas32-tag-hierarchy | Tag Object `summary`, `parent`, `kind` | 3.2 | metadata | `internal/compiler/openapi/read_test.go::TestReadAcceptsOpenAPI32OnlyFields` |
| oas32-example-data-values | Example Object `dataValue`, `serializedValue` | 3.2 | metadata | `internal/compiler/openapi/read_test.go::TestReadAcceptsOpenAPI32OnlyFields` |
| oas32-cookie-style | Parameter `style: cookie`, including raw cookie text preservation and inbound parsing | 3.2 | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeSerializesOpenAPI32CookieStyleWithoutPercentEncoding` |
| oas32-security-oauth2-metadata-url | Security Scheme `oauth2MetadataUrl` | 3.2 | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesEveryHostManagedSecurityCredentialShape` |
| oas32-security-deprecated | Security Scheme `deprecated` | 3.2 | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesEveryHostManagedSecurityCredentialShape` |
| oas32-device-authorization-flow | OAuth `deviceAuthorization` flow | 3.2 | generated | `internal/target/typescript/runtime_parity_test.go::TestRuntimeAppliesEveryHostManagedSecurityCredentialShape` |
| oas32-response-summary | Response `summary` | 3.2 | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| oas32-optional-paths | optional `paths` when another API surface is present | 3.2 | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsAllowsOpenAPI32OptionalPaths` |
| oas32-operations | `query`, `additionalOperations` | 3.2 | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateOpenAPI32QueryAndAdditionalOperations` |
| oas32-streaming-media | streaming/sequential/SSE media and expanded multipart rules | 3.2 | generated | `internal/target/typescript/runtime_parity_test.go::TestGeneratedNestedMultipartRequestAndResponseRoundTrip` |
