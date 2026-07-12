package typescript

import (
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

func TestMethodTerminalMapsSupportedHTTPMethods(t *testing.T) {
	for method, want := range map[string]string{
		"GET":     "get",
		"POST":    "post",
		"PUT":     "replace",
		"PATCH":   "patch",
		"DELETE":  "delete",
		"QUERY":   "query",
		"OPTIONS": "options",
	} {
		value, err := methodTerminal(method)
		if err != nil || value != want {
			t.Errorf("methodTerminal(%q) = %q, %v; want %q", method, value, err, want)
		}
	}
}

func TestResourceTerminalKeepsLiteralNamespacesAndOperationVerbs(t *testing.T) {
	for _, test := range []struct {
		operation ir.Operation
		parts     []string
		want      string
	}{
		{operation: ir.Operation{OperationID: "listWidgets", Method: "GET", Path: "/widgets"}, parts: []string{"widgets"}, want: "list"},
		{operation: ir.Operation{OperationID: "getWidget", Method: "GET", Path: "/widgets/{widgetID}"}, parts: []string{"widgets", "{widgetID}"}, want: "get"},
		{operation: ir.Operation{OperationID: "createWidget", Method: "POST", Path: "/widgets"}, parts: []string{"widgets"}, want: "create"},
		{operation: ir.Operation{OperationID: "searchWidgets", Method: "POST", Path: "/widgets/search"}, parts: []string{"widgets", "search"}, want: "post"},
		{operation: ir.Operation{OperationID: "runSearch", Method: "POST", Path: "/widgets/search"}, parts: []string{"widgets", "search"}, want: "post"},
	} {
		value, err := resourceTerminalName(test.operation, test.parts)
		if err != nil || value != test.want {
			t.Errorf("resourceTerminalName(%s %s) = %q, %v; want %q", test.operation.Method, test.operation.Path, value, err, test.want)
		}
	}
}
