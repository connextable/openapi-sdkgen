// Package openapiwalk contains structural helpers shared by compiler and target
// preflight traversals.
package openapiwalk

import (
	"strconv"
	"strings"
)

// IsOpaqueDataField reports fields whose descendants are literal payload data,
// not OpenAPI or JSON Schema keyword locations.
func IsOpaqueDataField(name string, value any) bool {
	switch name {
	case "const", "dataValue", "default", "enum", "example", "serializedValue", "value":
		return true
	case "examples":
		_, literal := value.([]any)
		return literal
	default:
		return false
	}
}

// IsNamedMap reports whether the object at path is a map whose keys are
// user-defined names rather than OpenAPI or JSON Schema keywords.
//
// The parent check is essential: a component, response, or schema property may
// itself legally be named "paths", "properties", or another structural token.
// Such a named-map entry is an ordinary object even though its final pointer
// token collides with a structural field name.
func IsNamedMap(path []string) bool {
	if len(path) == 0 {
		return false
	}
	if isSecurityRequirement(path) {
		return true
	}
	parent := path[:len(path)-1]
	if IsNamedMap(parent) {
		// A Callback Object is itself a named map of runtime expressions.
		if isCallbackCollection(parent) {
			return true
		}
		return false
	}
	switch path[len(path)-1] {
	case "paths", "webhooks", "schemas", "parameters", "headers", "requestBodies",
		"responses", "securitySchemes", "links", "callbacks", "pathItems",
		"mediaTypes", "examples", "properties", "patternProperties",
		"dependentSchemas", "dependentRequired", "$defs", "definitions",
		"content", "encoding", "additionalOperations", "variables", "scopes",
		"mapping":
		return true
	default:
		return false
	}
}

// IsExtensionKey reports whether name is an OpenAPI extension at the object
// located at path. Most named maps permit x-* as an ordinary user-defined
// name. Patterned-field maps such as Paths and Callback Objects reserve x-*
// for specification extensions instead.
func IsExtensionKey(path []string, name string) bool {
	if !strings.HasPrefix(name, "x-") {
		return false
	}
	if !IsNamedMap(path) {
		return true
	}
	return isRootPatternedCollection(path) || isCallbackObject(path) || isResponsesObject(path)
}

func isRootPatternedCollection(path []string) bool {
	return len(path) == 1 && (path[0] == "paths" || path[0] == "webhooks")
}

func isCallbackObject(path []string) bool {
	return len(path) > 0 && isCallbackCollection(path[:len(path)-1])
}

func isResponsesObject(path []string) bool {
	if len(path) == 0 || path[len(path)-1] != "responses" {
		return false
	}
	return len(path) != 2 || path[0] != "components"
}

func isCallbackCollection(path []string) bool {
	return len(path) != 0 &&
		path[len(path)-1] == "callbacks" &&
		!IsNamedMap(path[:len(path)-1])
}

func isSecurityRequirement(path []string) bool {
	if len(path) < 2 || path[len(path)-2] != "security" {
		return false
	}
	_, err := strconv.Atoi(path[len(path)-1])
	return err == nil
}
