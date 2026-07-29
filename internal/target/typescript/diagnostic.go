package typescript

import (
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func operationRouteKey(operation ir.Operation) string {
	if operation.RouteKey != "" {
		return operation.RouteKey
	}
	return operation.Method + " " + operation.Path
}

func operationLabel(operation ir.Operation) string {
	if operation.OperationID != "" {
		return operation.OperationID
	}
	return operationRouteKey(operation)
}

func manifestRouteKey(operation ManifestOperation) string {
	if operation.RouteKey != "" {
		return operation.RouteKey
	}
	return operation.Method + " " + operation.Path
}

func openAPIPointer(parts ...string) string {
	pointer := "#"
	for _, part := range parts {
		pointer = appendOpenAPIPointer(pointer, part)
	}
	return pointer
}

func appendOpenAPIPointer(pointer, part string) string {
	part = strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
	return pointer + "/" + part
}
