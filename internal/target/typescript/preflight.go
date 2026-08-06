package typescript

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"openapi-sdkgen/internal/compiler/ir"
	"openapi-sdkgen/internal/diagnostic"
)

var pathTemplatePattern = regexp.MustCompile(`\{[^{}]+\}`)

func prepareTargetDiagnostics(plan *sourcePlan) []diagnostic.Diagnostic {
	document := plan.document
	var result []diagnostic.Diagnostic
	for _, feature := range unsupportedSchemasForTarget(document) {
		result = append(result, unsupportedFeatureDiagnostic(
			document,
			feature,
			"SDKGEN-E501",
			"TypeScript cannot represent this Schema Object feature",
			"Remove the unsupported keyword or express the contract with supported OpenAPI and JSON Schema features.",
		))
	}
	if plan.includeServer {
		for _, feature := range unsupportedServerInboundSchemas(document) {
			result = append(result, unsupportedFeatureDiagnostic(
				document,
				feature,
				"SDKGEN-E506",
				"The TypeScript server add-on cannot represent this inbound Schema Object feature",
				"Remove the unsupported keyword or express the inbound contract with supported Schema Object features.",
			))
		}
	}
	for _, feature := range unsupportedOpenAPIFeatures(document, plan.includeServer) {
		pointer, detail := splitUnsupportedFeature(feature)
		if !plan.includeServer && (detail == "generated inbound webhook contracts" || detail == "generated callback contracts") {
			kind := "callback"
			if detail == "generated inbound webhook contracts" {
				kind = "webhook"
			}
			result = append(result, sourceTargetDiagnostic(
				document,
				pointer,
				"SDKGEN-E505",
				fmt.Sprintf("The OpenAPI feature %s requires the TypeScript server add-on for inbound %s contracts.", feature, kind),
				"Generate again with --with server, or remove the inbound contract.",
			))
			continue
		}
		result = append(result, unsupportedFeatureDiagnostic(
			document,
			feature,
			"SDKGEN-E502",
			"TypeScript cannot represent this OpenAPI feature",
			"Remove the unsupported construct or use a supported OpenAPI representation.",
		))
	}
	result = append(result, operationIdentityDiagnostics(document)...)
	result = append(result, templatedResourcePathDiagnostics(document)...)
	result = append(result, securityPreparationDiagnostics(document)...)
	result = append(result, cookieSecurityOwnershipDiagnostics(document)...)
	return diagnostic.Sort(result)
}

func unsupportedFeatureDiagnostic(document *ir.Document, feature, code, message, hint string) diagnostic.Diagnostic {
	pointer, detail := splitUnsupportedFeature(feature)
	if detail != "" {
		message += " at " + feature + "."
	} else {
		message += " at " + pointer + "."
	}
	return sourceTargetDiagnostic(document, pointer, code, message, hint)
}

func splitUnsupportedFeature(feature string) (string, string) {
	pointer := feature
	detail := ""
	if index := strings.Index(feature, " ("); index >= 0 && strings.HasSuffix(feature, ")") {
		pointer = feature[:index]
		detail = strings.TrimSuffix(feature[index+2:], ")")
	}
	return pointer, detail
}

func sourceTargetDiagnostic(document *ir.Document, pointer, code, message, hint string) diagnostic.Diagnostic {
	location, related := extensionDiagnosticLocation(document, pointer)
	value := diagnostic.Diagnostic{
		Severity: diagnostic.SeverityError,
		Code:     code,
		Phase:    diagnostic.PhaseTarget,
		Location: location,
		Related:  related,
		Target:   "typescript",
		Message:  message,
		Hint:     hint,
	}
	if operation, ok := operationAtPointer(document, pointer); ok {
		value.Route = operationRouteKey(operation)
		value.Operation = operation.OperationID
	}
	return value
}

func operationAtPointer(document *ir.Document, pointer string) (ir.Operation, bool) {
	for _, operation := range document.Operations {
		operationPointer := operation.Pointer
		if operationPointer == "" {
			operationPointer = "#/paths/" + escapePointerToken(operation.Path) + "/" + strings.ToLower(operation.Method)
		}
		if pointer == operationPointer || strings.HasPrefix(pointer, operationPointer+"/") {
			return operation, true
		}
	}
	return ir.Operation{}, false
}

type identityOccurrence struct {
	pointer string
}

func operationIdentityDiagnostics(document *ir.Document) []diagnostic.Diagnostic {
	seenRoutes := make(map[string]identityOccurrence, len(document.Operations))
	seenIDs := make(map[string]identityOccurrence, len(document.Operations))
	var result []diagnostic.Diagnostic
	for _, operation := range document.Operations {
		operationPointer := operation.Pointer
		if operationPointer == "" {
			operationPointer = "#/paths/" + escapePointerToken(operation.Path) + "/" + strings.ToLower(operation.Method)
		}
		routeKey := operationRouteKey(operation)
		if previous, exists := seenRoutes[routeKey]; exists {
			value := sourceTargetDiagnostic(
				document,
				operationPointer,
				"SDKGEN-E503",
				fmt.Sprintf("OpenAPI route identity %q is duplicated.", routeKey),
				"Keep exactly one operation for each exact HTTP method and OpenAPI path.",
			)
			previousLocation, _ := extensionDiagnosticLocation(document, previous.pointer)
			value.Related = append(value.Related, previousLocation)
			value.Related = sortTargetLocations(value.Related)
			result = append(result, value)
		} else {
			seenRoutes[routeKey] = identityOccurrence{pointer: operationPointer}
		}
		if operation.OperationID == "" {
			continue
		}
		idPointer := operationPointer + "/operationId"
		if previous, exists := seenIDs[operation.OperationID]; exists {
			value := sourceTargetDiagnostic(
				document,
				idPointer,
				"SDKGEN-E503",
				fmt.Sprintf("operationId %q is duplicated.", operation.OperationID),
				"Give every declared operationId an exact unique value, or omit it and use the exact route key.",
			)
			previousLocation, _ := extensionDiagnosticLocation(document, previous.pointer)
			value.Related = append(value.Related, previousLocation)
			value.Related = sortTargetLocations(value.Related)
			result = append(result, value)
		} else {
			seenIDs[operation.OperationID] = identityOccurrence{pointer: idPointer}
		}
	}
	return result
}

func templatedResourcePathDiagnostics(document *ir.Document) []diagnostic.Diagnostic {
	rawPaths, _ := document.Raw["paths"].(map[string]any)
	paths := sortedAnyKeys(rawPaths)
	if len(paths) == 0 {
		seen := make(map[string]bool)
		for _, operation := range document.Operations {
			if !seen[operation.Path] {
				paths = append(paths, operation.Path)
				seen[operation.Path] = true
			}
		}
	}
	seenShapes := make(map[string]string)
	var result []diagnostic.Diagnostic
	for _, path := range paths {
		if !strings.HasPrefix(path, "/") {
			continue
		}
		shape := pathTemplatePattern.ReplaceAllString(path, "{}")
		if shape == path {
			continue
		}
		previous, exists := seenShapes[shape]
		if !exists {
			seenShapes[shape] = path
			continue
		}
		pointer := "#/paths/" + escapePointerToken(path)
		value := sourceTargetDiagnostic(
			document,
			pointer,
			"SDKGEN-E504",
			fmt.Sprintf("Paths %q and %q have the same templated resource shape %q.", previous, path, shape),
			"Use one resource path shape; changing only path-parameter names does not create a distinct route.",
		)
		previousLocation, _ := extensionDiagnosticLocation(document, "#/paths/"+escapePointerToken(previous))
		value.Related = append(value.Related, previousLocation)
		value.Related = sortTargetLocations(value.Related)
		result = append(result, value)
	}
	return result
}

func securityPreparationDiagnostics(document *ir.Document) []diagnostic.Diagnostic {
	var result []diagnostic.Diagnostic
	for _, operation := range document.Operations {
		if _, _, err := operationSecurityDefinition(document, operation); err != nil {
			pointer := operation.Pointer
			if pointer == "" {
				pointer = "#/paths/" + escapePointerToken(operation.Path) + "/" + strings.ToLower(operation.Method)
			}
			if _, declared := operation.Raw["security"]; declared {
				pointer += "/security"
			} else {
				pointer = "#/security"
			}
			value := sourceTargetDiagnostic(
				document,
				pointer,
				"SDKGEN-E508",
				"Security requirements for this operation are invalid: "+strings.TrimSuffix(err.Error(), ".")+".",
				"Declare valid Security Requirement Objects that reference compatible component security schemes.",
			)
			value.Route = operationRouteKey(operation)
			value.Operation = operation.OperationID
			result = append(result, value)
		}
	}
	return result
}

func cookieSecurityOwnershipDiagnostics(document *ir.Document) []diagnostic.Diagnostic {
	components, _ := document.Raw["components"].(map[string]any)
	schemes, _ := components["securitySchemes"].(map[string]any)
	var result []diagnostic.Diagnostic
	for _, operation := range document.Operations {
		value, exists := operation.Raw["security"]
		if !exists {
			value, exists = document.Raw["security"]
		}
		if !exists {
			continue
		}
		requirements, ok := value.([]any)
		if !ok || len(requirements) == 0 {
			continue
		}
		cookieSchemes := make(map[string][]string)
		for _, item := range requirements {
			requirement, ok := item.(map[string]any)
			if !ok {
				continue
			}
			for _, schemeName := range sortedAnyKeys(requirement) {
				scheme, ok := schemes[schemeName].(map[string]any)
				if !ok {
					continue
				}
				kind, _ := scheme["type"].(string)
				location, _ := scheme["in"].(string)
				if kind != "apiKey" || location != "cookie" {
					continue
				}
				cookieName, _ := scheme["name"].(string)
				if cookieName == "" {
					continue
				}
				pointer := "#/components/securitySchemes/" + escapePointerToken(schemeName) + "/name"
				if !containsString(cookieSchemes[cookieName], pointer) {
					cookieSchemes[cookieName] = append(cookieSchemes[cookieName], pointer)
				}
			}
		}
		if len(cookieSchemes) == 0 {
			continue
		}
		parameters, err := operationParameters(document, operation)
		if err != nil {
			continue
		}
		for _, parameter := range parameters {
			relatedPointers := cookieSchemes[parameter.Name]
			if parameter.Location != "cookie" || len(relatedPointers) == 0 {
				continue
			}
			pointer := parameter.Pointer
			if pointer == "" {
				pointer = operation.Pointer + "/parameters"
			}
			value := sourceTargetDiagnostic(
				document,
				pointer,
				"SDKGEN-E509",
				fmt.Sprintf("Cookie %q is declared as both an operation parameter and security credential.", parameter.Name),
				"Remove the duplicate cookie Parameter Object and let the OpenAPI security scheme own this credential.",
			)
			value.Route = operationRouteKey(operation)
			value.Operation = operation.OperationID
			for _, relatedPointer := range relatedPointers {
				location, _ := extensionDiagnosticLocation(document, relatedPointer)
				value.Related = append(value.Related, location)
			}
			value.Related = sortTargetLocations(value.Related)
			result = append(result, value)
		}
	}
	return result
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func serverPreparationDiagnostic(document *ir.Document, kind string, err error) diagnostic.Diagnostic {
	pointer, message := sourcePointerErrorDetails(err)
	if pointer == "" {
		pointer = "#"
	}
	return sourceTargetDiagnostic(
		document,
		pointer,
		"SDKGEN-E506",
		fmt.Sprintf("The TypeScript server add-on cannot prepare %s: %s.", kind, strings.TrimSuffix(message, ".")),
		"Fix the inbound OpenAPI contract, or generate without --with server if inbound adapters are not required.",
	)
}

func loweringPreparationDiagnostic(document *ir.Document, err error) diagnostic.Diagnostic {
	pointer := "#"
	route := ""
	operationID := ""
	for _, operation := range document.Operations {
		if !strings.Contains(err.Error(), operationLabel(operation)) {
			continue
		}
		pointer = operation.Pointer
		if pointer == "" {
			pointer = "#/paths/" + escapePointerToken(operation.Path) + "/" + strings.ToLower(operation.Method)
		}
		route = operationRouteKey(operation)
		operationID = operation.OperationID
		break
	}
	value := sourceTargetDiagnostic(
		document,
		pointer,
		"SDKGEN-E507",
		"The OpenAPI contract cannot be lowered into a TypeScript operation contract: "+strings.TrimSuffix(err.Error(), ".")+".",
		"Correct the referenced schema, parameter, response, or operation identity shown by this diagnostic.",
	)
	value.Route = route
	value.Operation = operationID
	return value
}

func helperPreparationDiagnostic(document *ir.Document, kind string, code string, err error) diagnostic.Diagnostic {
	pointer, message := sourcePointerErrorDetails(err)
	route := ""
	operationID := ""
	if pointer == "" {
		pointer = "#"
		message = err.Error()
		for _, operation := range document.Operations {
			if !strings.Contains(err.Error(), operationLabel(operation)) {
				continue
			}
			pointer = operation.Pointer
			if pointer == "" {
				pointer = "#/paths/" + escapePointerToken(operation.Path) + "/" + strings.ToLower(operation.Method)
			}
			route = operationRouteKey(operation)
			operationID = operation.OperationID
			break
		}
	}
	value := sourceTargetDiagnostic(
		document,
		pointer,
		code,
		fmt.Sprintf("The TypeScript target cannot prepare %s: %s.", kind, strings.TrimSuffix(message, ".")),
		"Correct the referenced operation, response, or helper definition shown by this diagnostic.",
	)
	if route != "" {
		value.Route = route
		value.Operation = operationID
	}
	return value
}

func splitPointerError(message string) (string, string) {
	if !strings.HasPrefix(message, "#") {
		return "", message
	}
	end := len(message)
	for _, marker := range []string{": ", " must "} {
		if index := strings.Index(message, marker); index >= 0 && index < end {
			end = index
		}
	}
	if end == len(message) {
		return "", message
	}
	pointer := message[:end]
	rest := strings.TrimSpace(strings.TrimPrefix(message[end:], ":"))
	return pointer, rest
}

func sortTargetLocations(values []diagnostic.Location) []diagnostic.Location {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Source == values[j].Source {
			return values[i].Pointer < values[j].Pointer
		}
		return values[i].Source < values[j].Source
	})
	return values
}
