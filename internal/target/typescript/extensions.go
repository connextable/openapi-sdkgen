package typescript

import (
	"fmt"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
	"github.com/connextable/openapi-sdkgen/internal/openapiwalk"
)

var recognizedExtensionNames = map[string]bool{
	"x-envelope":       true,
	"x-error-category": true,
	"x-pagination":     true,
	"x-sdk-visibility": true,
	"x-sort":           true,
}

type extensionOccurrence struct {
	Name    string
	Pointer string
	Object  map[string]any
}

func prepareKnownExtensions(document *ir.Document) (*ir.Document, []diagnostic.Diagnostic, error) {
	prepared := cloneDocumentForPreparation(document)
	consumed := make(map[string]bool)
	var diagnostics []diagnostic.Diagnostic
	occurrences := collectRecognizedExtensionOccurrences(prepared.Raw)

	for index := range prepared.Operations {
		operation := &prepared.Operations[index]
		findings, err := prepareOperationExtensions(prepared, operation, consumed)
		if err != nil {
			return nil, nil, err
		}
		diagnostics = append(diagnostics, findings...)
		values, err := operationParameters(prepared, *operation)
		if err != nil {
			return nil, nil, err
		}
		for _, parameter := range values {
			if _, present := parameter.Raw["x-sort"]; !present {
				continue
			}
			pointer := parameter.Pointer + "/x-sort"
			consumed[pointer] = true
			plan, findings := validateSortExtension(prepared, *operation, parameter)
			diagnostics = append(diagnostics, findings...)
			if plan != nil {
				if operation.SortParameters == nil {
					operation.SortParameters = make(map[string]ir.SortParameterPlan)
				}
				operation.SortParameters[parameter.Location+"\x00"+parameter.Name] = *plan
			}
		}
	}

	categoryDiagnostics := prepareErrorCategories(prepared, consumed)
	diagnostics = append(diagnostics, categoryDiagnostics...)
	diagnostics = append(diagnostics, validateVisibilityDependencies(prepared)...)

	for _, occurrence := range occurrences {
		if consumed[occurrence.Pointer] {
			continue
		}
		location, related := extensionDiagnosticLocation(prepared, occurrence.Pointer)
		diagnostics = append(diagnostics, diagnostic.Diagnostic{
			Severity: diagnostic.SeverityError,
			Code:     "SDKGEN-E600",
			Phase:    diagnostic.PhaseTarget,
			Location: location,
			Related:  related,
			Target:   "typescript",
			Message:  fmt.Sprintf("Recognized extension %q is declared at an unsupported location.", occurrence.Name),
			Hint:     recognizedExtensionLocationHint(occurrence.Name),
		})
	}
	return prepared, diagnostic.Sort(diagnostics), nil
}

func validateVisibilityDependencies(document *ir.Document) []diagnostic.Diagnostic {
	byID := make(map[string]ir.Operation)
	byRoute := make(map[string]ir.Operation)
	for _, operation := range document.Operations {
		if operation.OperationID != "" {
			byID[operation.OperationID] = operation
		}
		byRoute[operationRouteKey(operation)] = operation
	}
	var result []diagnostic.Diagnostic
	for _, source := range document.Operations {
		if source.Visibility == "hidden" {
			continue
		}
		responses, _ := source.Raw["responses"].(map[string]any)
		for _, status := range sortedAnyKeys(responses) {
			response, _ := responses[status].(map[string]any)
			resolved, err := resolveComponentObject(document, response, "responses")
			if err != nil {
				continue
			}
			links, _ := resolved["links"].(map[string]any)
			for _, name := range sortedAnyKeys(links) {
				link, _ := links[name].(map[string]any)
				link, err = resolveComponentObject(document, link, "links")
				if err != nil {
					continue
				}
				target, exists := visibilityLinkTarget(document, byID, byRoute, link)
				if !exists || target.Visibility != "hidden" {
					continue
				}
				pointer := source.Pointer + "/responses/" + escapePointerToken(status) + "/links/" + escapePointerToken(name)
				location, related := extensionDiagnosticLocation(document, pointer)
				result = append(result, diagnostic.Diagnostic{
					Severity: diagnostic.SeverityError, Code: "SDKGEN-E621", Phase: diagnostic.PhaseTarget,
					Location: location, Related: related, Target: "typescript",
					Route: operationRouteKey(source), Operation: source.OperationID,
					Message: fmt.Sprintf("Visible response link %q targets hidden operation %s.", name, operationLabel(target)),
					Hint:    "Make the target internal/public, hide the source, or remove the link.",
				})
			}
		}
	}
	return result
}

func visibilityLinkTarget(document *ir.Document, byID, byRoute map[string]ir.Operation, link map[string]any) (ir.Operation, bool) {
	if operationID, _ := link["operationId"].(string); operationID != "" {
		target, exists := byID[operationID]
		return target, exists
	}
	reference, _ := link["operationRef"].(string)
	if reference == "" {
		return ir.Operation{}, false
	}
	target, err := linkTargetOperation(document, byID, link)
	if err != nil {
		return ir.Operation{}, false
	}
	if exact, exists := byRoute[operationRouteKey(target)]; exists {
		return exact, true
	}
	return target, true
}

func extensionStringValue(object map[string]any, name string) string {
	value, _ := object[name].(string)
	return value
}

func cloneDocumentForPreparation(document *ir.Document) *ir.Document {
	prepared := *document
	prepared.Operations = append([]ir.Operation(nil), document.Operations...)
	prepared.ErrorCategories = make(map[string]string)
	prepared.ParameterSortPlans = make(map[string]ir.SortParameterPlan)
	for index := range prepared.Operations {
		operation := &prepared.Operations[index]
		operation.SortParameters = nil
		operation.Envelope = ""
		operation.Pagination = ""
		operation.PaginationPlan = nil
		operation.Visibility = ""
	}
	return &prepared
}

func prepareOperationExtensions(document *ir.Document, operation *ir.Operation, consumed map[string]bool) ([]diagnostic.Diagnostic, error) {
	var result []diagnostic.Diagnostic
	envelope := operationStringExtension(*operation, "x-envelope", operation.Extensions.Envelope)
	if envelope.Present {
		consumed[envelope.Pointer] = true
		switch {
		case !envelope.Valid:
			result = append(result, operationExtensionDiagnostic(document, *operation, envelope.Pointer, "SDKGEN-E610", "x-envelope must be the string \"data\".", "Use x-envelope: data, or omit the extension for the complete response body."))
		case envelope.Value != "data":
			result = append(result, operationExtensionDiagnostic(document, *operation, envelope.Pointer, "SDKGEN-E611", fmt.Sprintf("x-envelope value %q is not supported.", envelope.Value), "Use x-envelope: data, or omit the extension for baseline behavior."))
		default:
			findings := validateEnvelopeRepresentations(document, *operation, envelope.Pointer)
			result = append(result, findings...)
			if len(findings) == 0 {
				operation.Envelope = "data"
			}
		}
	}

	visibility := operationStringExtension(*operation, "x-sdk-visibility", operation.Extensions.Visibility)
	if visibility.Present {
		consumed[visibility.Pointer] = true
		switch {
		case !visibility.Valid:
			result = append(result, operationExtensionDiagnostic(document, *operation, visibility.Pointer, "SDKGEN-E620", "x-sdk-visibility must be the string \"internal\" or \"hidden\".", "Use internal or hidden, or omit the extension for public visibility."))
		case visibility.Value == "public":
			location, related := extensionDiagnosticLocation(document, visibility.Pointer)
			result = append(result, diagnostic.Diagnostic{
				Severity:  diagnostic.SeverityWarning,
				Code:      "SDKGEN-W620",
				Phase:     diagnostic.PhaseTarget,
				Location:  location,
				Related:   related,
				Target:    "typescript",
				Route:     operationRouteKey(*operation),
				Operation: operation.OperationID,
				Message:   "x-sdk-visibility: public is redundant.",
				Hint:      "Omit x-sdk-visibility; public is the default.",
			})
		case visibility.Value == "internal", visibility.Value == "hidden":
			operation.Visibility = visibility.Value
		default:
			result = append(result, operationExtensionDiagnostic(document, *operation, visibility.Pointer, "SDKGEN-E620", fmt.Sprintf("x-sdk-visibility value %q is not supported.", visibility.Value), "Use internal or hidden, or omit the extension for public visibility."))
		}
	}

	pagination := operationValueExtension(*operation, "x-pagination", operation.Extensions.Pagination)
	if pagination.Present {
		consumed[pagination.Pointer] = true
		plan, findings, err := preparePaginationExtension(document, *operation, pagination)
		if err != nil {
			return nil, err
		}
		result = append(result, findings...)
		if plan != nil {
			operation.Pagination = plan.Mode
			operation.PaginationPlan = plan
		}
	}
	return result, nil
}

func operationStringExtension(operation ir.Operation, name string, compiled ir.StringExtension) ir.StringExtension {
	if compiled.Present {
		return compiled
	}
	raw, present := operation.Raw[name]
	value, valid := raw.(string)
	return ir.StringExtension{
		Present: present,
		Valid:   present && valid,
		Value:   value,
		Raw:     raw,
		Pointer: operationExtensionPointer(operation, name),
	}
}

func operationValueExtension(operation ir.Operation, name string, compiled ir.ValueExtension) ir.ValueExtension {
	if compiled.Present {
		return compiled
	}
	raw, present := operation.Raw[name]
	return ir.ValueExtension{Present: present, Raw: raw, Pointer: operationExtensionPointer(operation, name)}
}

func operationExtensionPointer(operation ir.Operation, name string) string {
	pointer := operation.Pointer
	if pointer == "" {
		pointer = "#/paths/" + escapePointerToken(operation.Path) + "/" + strings.ToLower(operation.Method)
	}
	return pointer + "/" + name
}

func operationExtensionDiagnostic(document *ir.Document, operation ir.Operation, pointer, code, message, hint string) diagnostic.Diagnostic {
	location, related := extensionDiagnosticLocation(document, pointer)
	return diagnostic.Diagnostic{
		Severity:  diagnostic.SeverityError,
		Code:      code,
		Phase:     diagnostic.PhaseTarget,
		Location:  location,
		Related:   related,
		Target:    "typescript",
		Route:     operationRouteKey(operation),
		Operation: operation.OperationID,
		Message:   message,
		Hint:      hint,
	}
}

func validateEnvelopeRepresentations(document *ir.Document, operation ir.Operation, extensionPointer string) []diagnostic.Diagnostic {
	responses, _ := operation.Raw["responses"].(map[string]any)
	statuses := sortedAnyKeys(responses)
	bodyRepresentations := 0
	var incompatible []string
	for _, status := range statuses {
		if !isSuccessResponseStatus(status) {
			continue
		}
		response, _ := responses[status].(map[string]any)
		resolved, err := resolveComponentObject(document, response, "responses")
		if err != nil {
			incompatible = append(incompatible, status+" (unresolved response)")
			continue
		}
		content, _ := resolved["content"].(map[string]any)
		for _, mediaType := range sortedAnyKeys(content) {
			bodyRepresentations++
			media, _ := content[mediaType].(map[string]any)
			media, err = resolveMediaTypeObject(document, media)
			if err != nil {
				incompatible = append(incompatible, status+" "+mediaType+" (unresolved media type)")
				continue
			}
			schemaValue, exists := media["schema"]
			schema, schemaIsObject := schemaValue.(map[string]any)
			resolvedSchema := resolveSchemaReference(document, schema, make(map[string]bool))
			if !exists || !schemaIsObject || schemaValue == false || !schemaCanDescribeObject(resolvedSchema) || isBinaryMedia(mediaType, schema) || isTextMedia(mediaType) || len(envelopeDataSchema(document, schema, make(map[string]bool))) == 0 {
				incompatible = append(incompatible, status+" "+mediaType)
			}
		}
	}
	if bodyRepresentations == 0 {
		return []diagnostic.Diagnostic{operationExtensionDiagnostic(document, operation, extensionPointer, "SDKGEN-E612", "x-envelope: data requires at least one body-bearing successful response.", "Declare a successful object response with a data property, or omit x-envelope.")}
	}
	if len(incompatible) == 0 {
		return nil
	}
	sort.Strings(incompatible)
	return []diagnostic.Diagnostic{operationExtensionDiagnostic(document, operation, extensionPointer, "SDKGEN-E612", "x-envelope: data is incompatible with successful representations: "+strings.Join(incompatible, ", ")+".", "Make every body-bearing successful representation an object with a declared data property, or omit x-envelope.")}
}

func schemaCanDescribeObject(schema map[string]any) bool {
	if schemaHasType(schema, "object") {
		return true
	}
	if _, exists := schema["properties"]; exists && schema["type"] == nil {
		return true
	}
	for _, keyword := range []string{"allOf", "oneOf", "anyOf"} {
		if values, ok := schema[keyword].([]any); ok && len(values) > 0 {
			return true
		}
	}
	return false
}

func validateSortExtension(document *ir.Document, operation ir.Operation, parameter operationParameter) (*ir.SortParameterPlan, []diagnostic.Diagnostic) {
	pointer := parameter.Pointer + "/x-sort"
	fail := func(message, hint string) (*ir.SortParameterPlan, []diagnostic.Diagnostic) {
		return nil, []diagnostic.Diagnostic{operationExtensionDiagnostic(document, operation, pointer, "SDKGEN-E630", message, hint)}
	}
	if parameter.Location != "query" {
		return fail("x-sort is only valid on a query Parameter Object.", "Move x-sort to the query array parameter it transforms.")
	}
	declaration, ok := parameter.Raw["x-sort"].(map[string]any)
	if !ok || len(declaration) != 1 || declaration["format"] != "field-direction" {
		return fail("x-sort must be exactly {\"format\":\"field-direction\"}.", "Use the documented field-direction declaration.")
	}
	schema, ok := parameter.Schema.(map[string]any)
	if !ok {
		return fail("x-sort requires a schema-based query parameter.", "Declare an array schema with string enum items.")
	}
	schema = resolveSchemaReference(document, schema, make(map[string]bool))
	if !schemaHasType(schema, "array") {
		return fail("x-sort requires an array parameter schema.", "Set type: array and declare string enum items.")
	}
	items, _ := schema["items"].(map[string]any)
	items = resolveSchemaReference(document, items, make(map[string]bool))
	if !schemaHasType(items, "string") {
		return fail("x-sort array items must have string type.", "Declare string items with exact field:direction enum values.")
	}
	values, ok := items["enum"].([]any)
	if !ok || len(values) == 0 {
		return fail("x-sort array items require a non-empty enum.", "Declare exact field:asc and field:desc wire values.")
	}
	seenWire := make(map[string]bool)
	plan := &ir.SortParameterPlan{}
	for _, raw := range values {
		wire, ok := raw.(string)
		if !ok || seenWire[wire] {
			return fail("x-sort enum values must be unique strings.", "Remove duplicate or non-string enum values.")
		}
		seenWire[wire] = true
		field, direction, found := strings.Cut(wire, ":")
		if !found || field == "" || strings.Contains(direction, ":") || (direction != "asc" && direction != "desc") {
			return fail(fmt.Sprintf("x-sort enum value %q is not field:asc or field:desc.", wire), "Use non-empty field names followed by :asc or :desc.")
		}
		plan.Values = append(plan.Values, ir.SortValue{Wire: wire, Field: field, Direction: direction})
	}
	return plan, nil
}

func schemaHasType(schema map[string]any, expected string) bool {
	switch value := schema["type"].(type) {
	case string:
		return value == expected
	case []any:
		for _, item := range value {
			if item == expected {
				return true
			}
		}
	}
	return false
}

func prepareErrorCategories(document *ir.Document, consumed map[string]bool) []diagnostic.Diagnostic {
	var result []diagnostic.Diagnostic
	codeCategories := make(map[string]map[string][]string)
	reachable := reachableErrorComponentSchemas(document)
	for _, schemaName := range sortedAnyKeys(mapStringAny(document.ComponentSchemas)) {
		if !reachable[schemaName] {
			continue
		}
		schema := document.ComponentSchemas[schemaName]
		codes, errorSchema, recognized := recognizedErrorEnvelope(document, schema)
		if !recognized {
			continue
		}
		pointer := "#/components/schemas/" + escapePointerToken(schemaName) + "/x-error-category"
		rawCategory, extensionPresent := schema["x-error-category"]
		if extensionPresent {
			consumed[pointer] = true
		}
		category, categoryPresent, categoryExact := nestedWireCategory(document, errorSchema)
		selected := ""
		if categoryPresent && categoryExact {
			selected = category
		}
		if extensionPresent {
			static, valid := rawCategory.(string)
			if !valid || static == "" {
				result = append(result, schemaExtensionDiagnostic(document, pointer, "SDKGEN-E640", "x-error-category must be a non-empty string.", "Use a non-empty static category, or omit the extension."))
			} else if categoryPresent && !categoryExact {
				result = append(result, schemaExtensionDiagnostic(document, pointer, "SDKGEN-E641", "x-error-category cannot override an optional or non-exact wire error.category property.", "Make error.category required with a string const or one-value enum, or remove one declaration."))
			} else if categoryExact && static != category {
				result = append(result, schemaExtensionDiagnostic(document, pointer, "SDKGEN-E642", fmt.Sprintf("x-error-category %q conflicts with wire category %q.", static, category), "Remove x-error-category and use the required wire category."))
			} else if categoryExact {
				location, related := extensionDiagnosticLocation(document, pointer)
				result = append(result, diagnostic.Diagnostic{
					Severity: diagnostic.SeverityWarning, Code: "SDKGEN-W641", Phase: diagnostic.PhaseTarget,
					Location: location, Related: related, Target: "typescript",
					Message: "x-error-category repeats the exact required wire error.category value.",
					Hint:    "Remove x-error-category; the wire schema is authoritative.",
				})
			} else {
				selected = static
			}
		}
		if selected == "" {
			continue
		}
		document.ErrorCategories[schemaName] = selected
		for _, code := range codes {
			if codeCategories[code] == nil {
				codeCategories[code] = make(map[string][]string)
			}
			codeCategories[code][selected] = append(codeCategories[code][selected], schemaName)
		}
	}
	for code, categories := range codeCategories {
		if len(categories) < 2 {
			continue
		}
		names := sortedAnyKeys(mapStringAny(categories))
		result = append(result, diagnostic.Diagnostic{
			Severity: diagnostic.SeverityError, Code: "SDKGEN-E643", Phase: diagnostic.PhaseTarget,
			Location: diagnostic.Location{Source: extensionRootSource(document), Pointer: "#/components/schemas"},
			Target:   "typescript",
			Message:  fmt.Sprintf("Error code %q has conflicting exact categories: %s.", code, strings.Join(names, ", ")),
			Hint:     "Assign one exact category to each runtime error code.",
		})
	}
	return result
}

func recognizedErrorEnvelope(document *ir.Document, schema map[string]any) ([]string, map[string]any, bool) {
	schema = effectiveErrorObjectSchema(document, schema, make(map[string]bool))
	if !stringListContains(schema["required"], "error") {
		return nil, nil, false
	}
	properties, _ := schema["properties"].(map[string]any)
	errorSchema, _ := properties["error"].(map[string]any)
	errorSchema = effectiveErrorObjectSchema(document, errorSchema, make(map[string]bool))
	if !stringListContains(errorSchema["required"], "code") {
		return nil, nil, false
	}
	errorProperties, _ := errorSchema["properties"].(map[string]any)
	codeSchema, _ := errorProperties["code"].(map[string]any)
	codeSchema = resolveSchemaReference(document, codeSchema, make(map[string]bool))
	codes := exactStringValues(codeSchema)
	return codes, errorSchema, len(codes) > 0
}

func effectiveErrorObjectSchema(document *ir.Document, schema map[string]any, seen map[string]bool) map[string]any {
	result := make(map[string]any)
	if reference, _ := schema["$ref"].(string); reference != "" {
		if name, err := componentSchemaReferenceName(reference); err == nil && !seen[name] {
			seen[name] = true
			mergeErrorObjectSchema(result, effectiveErrorObjectSchema(document, document.ComponentSchemas[name], seen))
		}
	}
	if variants, _ := schema["allOf"].([]any); len(variants) > 0 {
		for _, variant := range variants {
			item, _ := variant.(map[string]any)
			mergeErrorObjectSchema(result, effectiveErrorObjectSchema(document, item, copyStringBoolMap(seen)))
		}
	}
	mergeErrorObjectSchema(result, schema)
	return result
}

func mergeErrorObjectSchema(target, source map[string]any) {
	required := make(map[string]bool)
	for _, current := range []any{target["required"], source["required"]} {
		values, _ := current.([]any)
		for _, value := range values {
			if text, ok := value.(string); ok {
				required[text] = true
			}
		}
	}
	if len(required) > 0 {
		values := make([]any, 0, len(required))
		for _, name := range sortedAnyKeys(mapStringAny(required)) {
			values = append(values, name)
		}
		target["required"] = values
	}
	properties := make(map[string]any)
	if current, ok := target["properties"].(map[string]any); ok {
		for name, value := range current {
			properties[name] = value
		}
	}
	if current, ok := source["properties"].(map[string]any); ok {
		for name, value := range current {
			properties[name] = value
		}
	}
	if len(properties) > 0 {
		target["properties"] = properties
	}
	if value, exists := source["type"]; exists {
		target["type"] = value
	}
}

func nestedWireCategory(document *ir.Document, errorSchema map[string]any) (string, bool, bool) {
	properties, _ := errorSchema["properties"].(map[string]any)
	categorySchema, present := properties["category"].(map[string]any)
	if !present {
		return "", false, false
	}
	categorySchema = resolveSchemaReference(document, categorySchema, make(map[string]bool))
	values := exactStringValues(categorySchema)
	required := stringListContains(errorSchema["required"], "category")
	if required && len(values) == 1 && values[0] != "" {
		return values[0], true, true
	}
	return "", true, false
}

func exactStringValues(schema map[string]any) []string {
	if value, ok := schema["const"].(string); ok && value != "" {
		return []string{value}
	}
	values, ok := schema["enum"].([]any)
	if !ok || len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok || text == "" {
			return nil
		}
		result = append(result, text)
	}
	return result
}

func stringListContains(value any, expected string) bool {
	values, _ := value.([]any)
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func schemaExtensionDiagnostic(document *ir.Document, pointer, code, message, hint string) diagnostic.Diagnostic {
	location, related := extensionDiagnosticLocation(document, pointer)
	return diagnostic.Diagnostic{
		Severity: diagnostic.SeverityError, Code: code, Phase: diagnostic.PhaseTarget,
		Location: location, Related: related, Target: "typescript", Message: message, Hint: hint,
	}
}

func collectRecognizedExtensionOccurrences(root map[string]any) []extensionOccurrence {
	var result []extensionOccurrence
	var visit func(any, []string)
	visit = func(value any, path []string) {
		switch typed := value.(type) {
		case map[string]any:
			namedMap := openapiwalk.IsNamedMap(path)
			for _, name := range sortedAnyKeys(typed) {
				child := typed[name]
				extensionKey := openapiwalk.IsExtensionKey(path, name)
				if extensionKey && recognizedExtensionNames[name] {
					result = append(result, extensionOccurrence{Name: name, Pointer: pointerFromParts(append(path, name)), Object: typed})
					continue
				}
				if extensionKey {
					continue
				}
				if !namedMap && openapiwalk.IsOpaqueDataField(name, child) {
					continue
				}
				visit(child, append(path, name))
			}
		case []any:
			for index, child := range typed {
				visit(child, append(path, fmt.Sprintf("%d", index)))
			}
		}
	}
	visit(root, nil)
	sort.Slice(result, func(left, right int) bool {
		if result[left].Pointer == result[right].Pointer {
			return result[left].Name < result[right].Name
		}
		return result[left].Pointer < result[right].Pointer
	})
	return result
}

func pointerFromParts(parts []string) string {
	pointer := "#"
	for _, part := range parts {
		pointer += "/" + escapePointerToken(part)
	}
	return pointer
}

func escapePointerToken(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func extensionDiagnosticLocation(document *ir.Document, pointer string) (diagnostic.Location, []diagnostic.Location) {
	if provenance, exists := document.Provenance[pointer]; exists {
		related := make([]diagnostic.Location, 0, len(provenance.Related))
		for _, value := range provenance.Related {
			related = append(related, diagnostic.Location{Source: value.Source, Pointer: value.Pointer})
		}
		return diagnostic.Location{Source: provenance.Primary.Source, Pointer: provenance.Primary.Pointer}, related
	}
	return diagnostic.Location{Source: extensionRootSource(document), Pointer: pointer}, nil
}

func extensionRootSource(document *ir.Document) string {
	if provenance, exists := document.Provenance["#"]; exists && provenance.Primary.Source != "" {
		return provenance.Primary.Source
	}
	return "OpenAPI document"
}

func recognizedExtensionLocationHint(name string) string {
	switch name {
	case "x-envelope", "x-pagination", "x-sdk-visibility":
		return "Declare it only on an ordinary Paths operation."
	case "x-sort":
		return "Declare it only on the query Parameter Object it transforms."
	case "x-error-category":
		return "Declare it only on a recognized outer error-envelope schema."
	default:
		return "Move or remove the extension declaration."
	}
}
