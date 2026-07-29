package typescript

import (
	"strings"
	"testing"
)

func TestComponentProjectionTypeExpressionRendersBothScopes(t *testing.T) {
	expression := componentProjectionTypeExpression(`Money"Input`, projectionInput)
	if got, want := expression.render(typeRenderLocal), `ComponentInput<"Money\"Input">`; got != want {
		t.Fatalf("local expression = %q, want %q", got, want)
	}
	if got, want := expression.render(typeRenderContract), `Contract.ComponentInput<"Money\"Input">`; got != want {
		t.Fatalf("contract expression = %q, want %q", got, want)
	}

	output := componentProjectionTypeExpression("Money", projectionOutput)
	if got, want := output.render(typeRenderLocal), `ComponentOutput<"Money">`; got != want {
		t.Fatalf("output expression = %q, want %q", got, want)
	}
}

func TestPlannedIdentityPreservesExactKeyAndStablePrivateIdentifier(t *testing.T) {
	first := planIdentity(identityExactPublic, "parameter", "foo-bar")
	second := planIdentity(identityExactPublic, "parameter", "foo_bar")
	repeated := planIdentity(identityExactPublic, "parameter", "foo-bar")

	if got, want := first.typeKey(), `"foo-bar"`; got != want {
		t.Fatalf("type key = %q, want %q", got, want)
	}
	if got, want := first.bracket("Operations"), `Operations["foo-bar"]`; got != want {
		t.Fatalf("bracket access = %q, want %q", got, want)
	}
	if first.privateIdentifier == second.privateIdentifier {
		t.Fatalf("normalization-equivalent identities share private identifier %q", first.privateIdentifier)
	}
	if first.privateIdentifier != repeated.privateIdentifier {
		t.Fatalf("private identifier is not deterministic: %q != %q", first.privateIdentifier, repeated.privateIdentifier)
	}
	if !strings.HasPrefix(first.privateIdentifier, "__sdkgen_fooBar_") {
		t.Fatalf("private identifier = %q", first.privateIdentifier)
	}
}

func TestRuntimeJSONExpressionIsDeterministicAndPrototypeSafe(t *testing.T) {
	left := map[string]any{
		"z": 1,
		"nested": map[string]any{
			"constructor": []any{"first", map[string]any{"__proto__": true}},
		},
		"__proto__": "root",
	}
	right := map[string]any{
		"__proto__": "root",
		"nested": map[string]any{
			"constructor": []any{"first", map[string]any{"__proto__": true}},
		},
		"z": 1,
	}

	first, err := runtimeJSONExpression(left)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtimeJSONExpression(right)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("runtime JSON expression is not deterministic:\n%s\n%s", first, second)
	}
	for _, expected := range []string{
		`Object.fromEntries([["__proto__", "root"]`,
		`["constructor", ["first", Object.fromEntries([["__proto__", true]])]]`,
	} {
		if !strings.Contains(first, expected) {
			t.Fatalf("runtime JSON expression missing %q:\n%s", expected, first)
		}
	}
	if strings.Contains(first, `{"__proto__"`) {
		t.Fatalf("runtime JSON expression used unsafe object literal:\n%s", first)
	}
}

func TestRuntimeJSONExpressionRejectsNonJSONValue(t *testing.T) {
	if _, err := runtimeJSONExpression(make(chan int)); err == nil {
		t.Fatal("non-JSON value unexpectedly rendered")
	}
}
