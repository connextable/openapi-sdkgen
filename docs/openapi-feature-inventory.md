# OpenAPI 3.x Atomic SDK Capability Inventory

This is the grouped, human-readable companion to
[`openapi-feature-matrix.md`](openapi-feature-matrix.md). Every listed surface
has one target state only: `generated`, `metadata`, or `error`. `error` means
generation stops before output for that target and includes the JSON Pointer of
the feature. `metadata` means the lossless `openapiDocument` artifact exposes
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
| scoped-server-alternatives | multiple path/operation Server Objects | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsMultipleScopedServersWithoutSelectionAPI` |
| server-variables | server-variable `enum`, `default`, `description` | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
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
| local-link-security-references | Link `operationRef` and Security Requirement implicit references | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| component-schema-reference-escapes | RFC 6901 `~0`/`~1` component schema names | all | generated | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsDecodesEscapedComponentSchemaReferences` |
| nested-schema-pointers | non-component Schema Object JSON Pointers | all | error | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsNestedSchemaPointers` |
| contained-external-references | file references inside the input root | all | generated | `internal/compiler/compiler_test.go::TestCompileFileBundlesInDirectoryReferencesForEverySupportedVersionLine` |
| remote-or-escaping-references | remote and input-root escaping references | all | error | `internal/compiler/compiler_test.go::TestCompileFileRejectsReferenceOutsideInputDirectory` |
| reference-cycles | cyclic reusable schema references | all | generated | `internal/target/typescript/types_test.go::TestSourceArtifactsGenerateRecursiveComponentSchemas` |
| schema-components | `components.schemas` object schemas | all | generated | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| reusable-responses | `components.responses` | all | generated | `internal/target/typescript/emit_test.go::TestSourceArtifactsStayConsistentAndDeterministic` |
| reusable-parameters | `components.parameters` | all | generated | `internal/target/typescript/emit_test.go::TestSourceArtifactsStayConsistentAndDeterministic` |
| reusable-request-bodies | `components.requestBodies` | all | generated | `internal/target/typescript/emit_test.go::TestSourceArtifactsStayConsistentAndDeterministic` |
| reusable-headers | `components.headers` | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| reusable-examples | `components.examples` | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| reusable-links-callbacks-security | `components.links`, `callbacks`, `securitySchemes` | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| reusable-path-items | `components.pathItems` | 3.1+ | generated | `internal/compiler/ir/build_test.go::TestBuildResolvesLocalPathItemReferences` |
| reusable-media-types | `components.mediaTypes` | 3.2 | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |

## HTTP Calls, Parameters, and Bodies

| ID | Fields | Versions | State | Evidence |
| --- | --- | --- | --- | --- |
| standard-methods | `get`, `put`, `post`, `delete`, `options`, `head`, `patch`, `trace` | all | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateEveryStandardHTTPMethod` |
| query-method | `query` | 3.2 | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateOpenAPI32QueryAndAdditionalOperations` |
| additional-operations | `additionalOperations` arbitrary methods | 3.2 | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateOpenAPI32QueryAndAdditionalOperations` |
| parameter-locations | `path`, `query`, `header`, `cookie` | all | generated | `internal/target/typescript/runtime_parity_test.go::TestTypeScriptAndJavaScriptTargetsHaveEquivalentRuntimeTransport` |
| parameter-serialization | scalar/array `simple`, `label`, `matrix`, `form`, `spaceDelimited`, `pipeDelimited`, `deepObject`, `explode` | all | generated | `test/typescript/tests/runtime.test.ts::serializes paths, query styles, headers, cookies, and wire names` |
| parameter-delimited-object | `spaceDelimited` and `pipeDelimited` object parameters | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsParameterSerializationItCannotRepresent` |
| parameter-schema-content | parameter `schema`, single-media `content` | all | generated | `internal/target/typescript/emit_test.go::TestSourceArtifactsStayConsistentAndDeterministic` |
| parameter-docs | parameter `description`, `deprecated`, examples | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| required-inputs | required path/query/header/cookie parameters and request body | all | generated | `internal/target/typescript/runtime_parity_test.go::TestTargetsRejectMissingRequiredRuntimeInputsBeforeFetch` |
| parameter-structured-non-json-content | structured single-media Parameter Object `content` outside JSON | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsParameterSerializationItCannotRepresent` |
| parameter-allow-reserved | `allowReserved: true` | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| parameter-allow-empty-value | `allowEmptyValue: true` | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| parameter-querystring | `in: querystring` and querystring format extensions | 3.2 | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| request-basic-media | JSON, text, binary, `application/x-www-form-urlencoded`, basic multipart | all | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsAllowsImplementedOpenAPIHTTPFeatures` |
| xml-media | `application/xml` and XML-suffixed media types | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsXMLMediaTypesWithoutCodecSemantics` |
| request-encoding | Media Type `encoding`, nested/ordered/unnamed multipart, per-part headers and transfer encoding | all; expanded 3.2 | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| media-item-schema-and-positional-encoding | Media Type `itemSchema`, `prefixEncoding`, `itemEncoding` | 3.2 | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsOpenAPI32StreamingAndPositionalMediaFeatures` |
| request-description-examples | Request Body description and Media Type examples | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |

## Responses, Asynchronous Features, and Security

| ID | Fields | Versions | State | Evidence |
| --- | --- | --- | --- | --- |
| response-status-selection | exact/default/range status keys and no-content | all | generated | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| default-response-media-negotiation | `default` Response Object media types in generated `accept` options | all | generated | `internal/target/typescript/client_test.go::TestOperationResponseMediaTypesIncludesDefaultResponses` |
| response-body-media | JSON, text, and binary response bodies | all | generated | `internal/target/typescript/types_test.go::TestOperationOutputTypesIncludeDefaultResponses` |
| response-docs | Response `description` and examples | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| response-headers | Response Object and Header Object declarations | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsResponseHeaderContracts` |
| response-media-wildcards | wildcard media negotiation | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnsupportedReusableComponentFeatures` |
| response-streams | sequential JSON, binary streams, Server-Sent Events | 3.2 | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| response-links | Link Object, runtime expressions, `operationId`, `operationRef`, parameters, requestBody, server | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| callbacks | Callback Object, key expressions, callback Path Items | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| webhooks | root Webhook Object | 3.1+ | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| examples | Example `summary`, `description`, `value`, `externalValue` | all | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| security-requirements | root/operation requirement alternatives, optional requirements, override | all | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| security-schemes | API key, HTTP, OAuth2, OpenID Connect, mutual TLS | all; mutual TLS 3.1+ | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |

## Schema Object: OAS 3.0.x

| ID | Fields | Versions | State | Evidence |
| --- | --- | --- | --- | --- |
| oas30-scalar-types | `type`, `enum` | 3.0 | generated | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| oas30-scalar-constraints | `multipleOf`, bounds, string constraints, `format`, `default`, `example` | 3.0 | metadata | `internal/target/typescript/types_test.go::TestSchemaConstraintSummaryListsValidationOnlyKeywords` |
| oas30-arrays-objects | `items`, `properties`, `required`, schema-valued `additionalProperties` | 3.0 | generated | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| oas30-closed-objects | `additionalProperties: false` | 3.0 | error | `internal/target/typescript/types_test.go::TestSourceArtifactsRejectsClosedObjectSchemasWithoutRuntimeValidation` |
| oas30-array-constraints | `maxItems`, `minItems`, `uniqueItems` | 3.0 | metadata | `internal/target/typescript/types_test.go::TestSchemaConstraintSummaryListsValidationOnlyKeywords` |
| oas30-object-constraints | `maxProperties`, `minProperties` | 3.0 | metadata | `internal/target/typescript/types_test.go::TestSchemaConstraintSummaryListsValidationOnlyKeywords` |
| oas30-all-of | `allOf` | 3.0 | generated | `internal/target/typescript/types_test.go::TestSchemaTypeMapsCompositeOpenAPISchemas` |
| oas30-variant-composition | `oneOf`, `anyOf` | 3.0 | error | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsVariantSchemasWithoutWireBranchSelection` |
| oas30-negation | `not` | 3.0 | error | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsUnimplementedSchemaFeaturesWithPaths` |
| oas30-nullability-projection | `nullable`, `readOnly`, `writeOnly` | 3.0 | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateAcrossSupportedOpenAPIVersionLines` |
| oas30-schema-deprecated | Schema `deprecated` | 3.0 | metadata | `internal/target/typescript/emit_test.go::TestSourceArtifactsStayConsistentAndDeterministic` |
| oas30-discriminator-xml | `discriminator`, `xml` | 3.0 | error | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsUnimplementedSchemaFeaturesWithPaths` |
| oas30-schema-external-docs | Schema `externalDocs` | 3.0 | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |

## Schema Object: JSON Schema 2020-12

| ID | Fields | Versions | State | Evidence |
| --- | --- | --- | --- | --- |
| jsonschema-structural | `$ref`, `type` including arrays and `null`, `const`, `enum`, `allOf`, `prefixItems`, `items`, `properties`, schema-valued `additionalProperties` | 3.1+ | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateAcrossSupportedOpenAPIVersionLines` |
| jsonschema-variant-composition | `oneOf`, `anyOf` | 3.1+ | error | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsVariantSchemasWithoutWireBranchSelection` |
| jsonschema-closed-objects | `additionalProperties: false` | 3.1+ | error | `internal/target/typescript/types_test.go::TestSourceArtifactsRejectsClosedObjectSchemasWithoutRuntimeValidation` |
| jsonschema-pattern-properties | `patternProperties` | 3.1+ | error | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsSchemaVocabularyWithoutWireSemantics` |
| jsonschema-doc-annotations | `title`, `description`, `default`, `deprecated`, `examples`, `format`, `contentEncoding`, `contentMediaType` | 3.1+ | metadata | `internal/target/typescript/types_test.go::TestSchemaConstraintSummaryListsValidationOnlyKeywords` |
| jsonschema-numeric-string-array-object-constraints | `multipleOf`, bounds, string bounds, array bounds, object bounds | 3.1+ | metadata | `internal/target/typescript/types_test.go::TestSchemaConstraintSummaryListsValidationOnlyKeywords` |
| jsonschema-directional-annotations | `readOnly`, `writeOnly` | 3.1+ | generated | `internal/target/typescript/types_test.go::TestSchemaTypeProjectsReadAndWriteOnlyProperties` |
| jsonschema-resource-dialect | `$id`, `$schema`, `$anchor`, `$dynamicAnchor`, `$dynamicRef`, `$vocabulary`, `$comment`, `$defs` | 3.1+ | error | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsUnimplementedSchemaFeaturesWithPaths` |
| jsonschema-boolean | boolean schemas | 3.1+ | error | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsBooleanSchemaFromAnOpenAPI31Document` |
| jsonschema-applicator-assertions | `not`, `if`, `then`, `else`, `dependentSchemas`, `contains`, `minContains`, `maxContains`, `propertyNames`, `unevaluatedItems`, `unevaluatedProperties` | 3.1+ | error | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsUnimplementedSchemaFeaturesWithPaths` |
| jsonschema-validation-assertions | `dependentRequired`, `contentSchema` | 3.1+ | error | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsUnimplementedSchemaFeaturesWithPaths` |
| jsonschema-openapi-annotations | `discriminator`, `xml` | 3.1+ | error | `internal/target/typescript/schema_support_test.go::TestSourceArtifactsRejectsUnimplementedSchemaFeaturesWithPaths` |
| jsonschema-schema-external-docs | Schema `externalDocs` | 3.1+ | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |

## Version-only Syntax and Semantics

| ID | Fields | Versions | State | Evidence |
| --- | --- | --- | --- | --- |
| oas30-nullable | `nullable` | 3.0 | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateAcrossSupportedOpenAPIVersionLines` |
| oas30-bound-syntax | boolean `exclusiveMaximum` and `exclusiveMinimum` | 3.0 | metadata | `internal/compiler/openapi/read_test.go::TestReadAcceptsVersionCorrectExclusiveBounds` |
| oas30-schema-version-gate | type arrays, numeric exclusive bounds, and 3.1 JSON Schema keywords | 3.0 | error | `internal/compiler/openapi/read_test.go::TestReadRejectsLaterMinorFeaturesAtJSONPointers` |
| oas31-documentation | `info.summary`, `license.identifier` | 3.1+ | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| oas31-numeric-bounds | numeric `exclusiveMaximum`, numeric `exclusiveMinimum` | 3.1+ | metadata | `internal/compiler/openapi/read_test.go::TestReadAcceptsVersionCorrectExclusiveBounds` |
| oas31-legacy-nullable | legacy `nullable` | 3.1+ | metadata | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsDoesNotApplyOpenAPI30NullableToOpenAPI31Schemas` |
| oas31-schema-dialect | `jsonSchemaDialect` | 3.1+ | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| oas32-base-uri | `$self` and 3.2 base URI semantics | 3.2 | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| oas32-server-name | Server Object `name` | 3.2 | metadata | `internal/compiler/openapi/read_test.go::TestReadAcceptsOpenAPI32OnlyFields` |
| oas32-tag-hierarchy | Tag Object `summary`, `parent`, `kind` | 3.2 | metadata | `internal/compiler/openapi/read_test.go::TestReadAcceptsOpenAPI32OnlyFields` |
| oas32-example-data-values | Example Object `dataValue`, `serializedValue` | 3.2 | metadata | `internal/compiler/openapi/read_test.go::TestReadAcceptsOpenAPI32OnlyFields` |
| oas32-cookie-style | Parameter `style: cookie` | 3.2 | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsOpenAPI32CookieStyleWithoutEncoder` |
| oas32-security-oauth2-metadata-url | Security Scheme `oauth2MetadataUrl` | 3.2 | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| oas32-security-deprecated | Security Scheme `deprecated` | 3.2 | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| oas32-device-authorization-flow | OAuth `deviceAuthorization` flow | 3.2 | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| oas32-response-summary | Response `summary` | 3.2 | metadata | `internal/target/typescript/metadata_test.go::TestEmitMetadataPreservesDocumentationExamplesAndExtensions` |
| oas32-optional-paths | optional `paths` when another API surface is present | 3.2 | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| oas32-operations | `query`, `additionalOperations` | 3.2 | generated | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsGenerateOpenAPI32QueryAndAdditionalOperations` |
| oas32-unsupported-surface | `querystring`, `components.mediaTypes` | 3.2 | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
| oas32-streaming-media | streaming/sequential/SSE media and expanded multipart rules | 3.2 | error | `internal/target/typescript/openapi_support_test.go::TestSourceArtifactsRejectsUnimplementedOpenAPIFeaturesWithPaths` |
