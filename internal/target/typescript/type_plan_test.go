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

func TestStablePrivateIdentifierIsInjectiveAcrossRoleAndSourceBoundaries(t *testing.T) {
	values := []string{
		stablePrivateIdentifier("a", "b\x00c"),
		stablePrivateIdentifier("a\x00b", "c"),
		stablePrivateIdentifier("a", "b"),
		stablePrivateIdentifier("a", "B"),
	}
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value] {
			t.Fatalf("private identifier collision: %q", value)
		}
		seen[value] = true
	}
}

func TestReadonlyJSONTypePreservesNestedExactValues(t *testing.T) {
	got, err := readonlyJSONType([]any{
		map[string]any{"__proto__": map[string]any{"value": 1.0}},
		[]any{"x", true, nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `readonly [{ readonly "__proto__": { readonly "value": 1 } }, readonly ["x", true, null]]`
	if got != want {
		t.Fatalf("type = %q, want %q", got, want)
	}
}

func TestEnumValueTypeAndStringMembersPreserveExactValues(t *testing.T) {
	values := []any{
		"ready",
		2.0,
		nil,
		[]any{"x"},
		map[string]any{"__proto__": true},
		"ready",
		"map",
	}
	valueType, err := enumValueType(values)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"ready" | 2 | null | readonly ["x"] | { readonly "__proto__": true } | "map"`; valueType != want {
		t.Fatalf("enum value type = %q, want %q", valueType, want)
	}
	if got, want := strings.Join(enumStringMembers(values), "\x00"), "ready\x00map"; got != want {
		t.Fatalf("enum string members = %q, want %q", got, want)
	}
}

func TestEnumValueTypeHandlesEmptyAndRejectsNonJSONValues(t *testing.T) {
	if got, err := enumValueType(nil); err != nil || got != "never" {
		t.Fatalf("empty enum value type = %q, %v", got, err)
	}
	if _, err := enumValueType([]any{make(chan int)}); err == nil {
		t.Fatal("non-JSON enum value unexpectedly rendered")
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
		`["constructor", ["first", /* @__PURE__ */ Object.fromEntries([["__proto__", true]])]]`,
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
