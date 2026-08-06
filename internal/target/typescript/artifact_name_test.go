package typescript

import (
	"strings"
	"testing"
)

func TestSafeArtifactStemUsesPortableFallbacks(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"UserProfile": "user-profile",
		"foo.bar":     "foo-bar",
		"CON":         "con-",
		"é":           "value-",
		"":            "value-",
	}
	for source, prefix := range tests {
		source, prefix := source, prefix
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			result := safeArtifactStem(source)
			if !strings.HasPrefix(result, prefix) {
				t.Fatalf("safeArtifactStem(%q) = %q, want prefix %q", source, result, prefix)
			}
			if len(result) > maxArtifactSegmentBytes-len(".ts") {
				t.Fatalf("safeArtifactStem(%q) length = %d", source, len(result))
			}
		})
	}
}

func TestAllocateArtifactPathsResolvesPortableCollisionsForEveryMember(t *testing.T) {
	t.Parallel()
	candidates := []artifactPathCandidate{
		{identity: "Foo", base: "internal/schemas/foo.ts"},
		{identity: "foo", base: "internal/schemas/foo.ts"},
		{identity: "index", base: "internal/schemas/index.ts"},
	}
	first, err := allocateArtifactPaths(candidates, "internal/schemas/index.ts")
	if err != nil {
		t.Fatal(err)
	}
	second, err := allocateArtifactPaths([]artifactPathCandidate{candidates[2], candidates[0], candidates[1]}, "internal/schemas/index.ts")
	if err != nil {
		t.Fatal(err)
	}
	for identity, path := range first {
		if second[identity] != path {
			t.Fatalf("path for %q changed with input order: %q -> %q", identity, path, second[identity])
		}
		if path == "internal/schemas/foo.ts" || path == "internal/schemas/index.ts" {
			t.Fatalf("colliding path for %q was not suffixed: %q", identity, path)
		}
	}
	if portableArtifactPathKey(first["Foo"]) == portableArtifactPathKey(first["foo"]) {
		t.Fatalf("case-fold collision remains: %#v", first)
	}
}

func TestNormalizationEquivalentUnicodeStemsRemainDistinct(t *testing.T) {
	t.Parallel()
	composed := safeArtifactStem("é")
	decomposed := safeArtifactStem("e\u0301")
	if composed == decomposed {
		t.Fatalf("normalization-equivalent source names collided at %q", composed)
	}
}

func TestOperationArtifactBaseUsesRouteStructureAndMethod(t *testing.T) {
	t.Parallel()
	if got, want := operationArtifactBase("/users/{userId}", "GET"), "internal/operations/users/by-user-id/get.ts"; got != want {
		t.Fatalf("operation path = %q, want %q", got, want)
	}
	if got, want := operationArtifactBase("/", "QUERY"), "internal/operations/root/query.ts"; got != want {
		t.Fatalf("root operation path = %q, want %q", got, want)
	}
	long := "/" + strings.Repeat("long-segment/", 30) + "tail"
	got := operationArtifactBase(long, "GET")
	if len(got) > maxArtifactPathBytes || !strings.HasPrefix(got, "internal/operations/route-") {
		t.Fatalf("long operation path did not use bounded fallback: %q", got)
	}
}

func TestRelativeModuleSpecifierUsesNodeNextJSExtension(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		from string
		to   string
	}{
		"root":    {from: "index.ts", to: "internal/index.ts"},
		"sibling": {from: "internal/routes/index.ts", to: "internal/routes/helpers.ts"},
		"nested":  {from: "internal/operations/users/get.ts", to: "internal/schemas/user.ts"},
	}
	wants := map[string]string{
		"root":    "./internal/index.js",
		"sibling": "./helpers.js",
		"nested":  "../../schemas/user.js",
	}
	for name, test := range tests {
		got, err := relativeModuleSpecifier(test.from, test.to)
		if err != nil {
			t.Fatal(err)
		}
		if got != wants[name] {
			t.Fatalf("%s specifier = %q, want %q", name, got, wants[name])
		}
	}
}

func TestValidateArtifactPathRejectsTraversalAndNonPortableSegments(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"../escape.ts", "/absolute.ts", "internal\\escape.ts", "internal/é.ts", "internal/no-extension"} {
		if err := validateArtifactPath(value); err == nil {
			t.Fatalf("validateArtifactPath(%q) succeeded", value)
		}
	}
}
