package typescript

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"openapi-sdkgen/internal/compiler/naming"
)

type typeRenderMode uint8

const (
	typeRenderModeLocal typeRenderMode = iota
	typeRenderModeContract
)

type typeRenderScope struct {
	mode               typeRenderMode
	componentReference func(name string, direction projection) string
}

var (
	typeRenderLocal    = typeRenderScope{mode: typeRenderModeLocal}
	typeRenderContract = typeRenderScope{mode: typeRenderModeContract}
)

func typeRenderModule(reference func(name string, direction projection) string) typeRenderScope {
	return typeRenderScope{mode: typeRenderModeLocal, componentReference: reference}
}

// typeExpression keeps local and cross-module renderings together so emitters
// never have to discover component references by rewriting rendered source.
type typeExpression struct {
	local    string
	contract string
}

func rawTypeExpression(source string) typeExpression {
	return typeExpression{local: source, contract: source}
}

func scopedTypeExpression(local, contract string) typeExpression {
	return typeExpression{local: local, contract: contract}
}

func componentProjectionTypeExpression(name string, direction projection) typeExpression {
	helper := "ComponentOutput"
	if direction == projectionInput {
		helper = "ComponentInput"
	}
	argument := quoteTS(name)
	local := helper + "<" + argument + ">"
	return scopedTypeExpression(local, "Contract."+local)
}

func (expression typeExpression) render(scope typeRenderScope) string {
	if scope.mode == typeRenderModeContract {
		return expression.contract
	}
	return expression.local
}

type identityClass string

const (
	identityExactPublic identityClass = "exact-public"
	identityResource    identityClass = "resource-convenience"
	identityPrivate     identityClass = "private"
)

// plannedIdentity separates an OpenAPI source identity from the private
// identifier used by generated implementation code.
type plannedIdentity struct {
	source            string
	class             identityClass
	privateIdentifier string
}

func planIdentity(class identityClass, role, source string) plannedIdentity {
	return plannedIdentity{
		source:            source,
		class:             class,
		privateIdentifier: stablePrivateIdentifier(role, source),
	}
}

func (identity plannedIdentity) typeKey() string {
	return quoteTS(identity.source)
}

func (identity plannedIdentity) bracket(base string) string {
	return base + "[" + quoteTS(identity.source) + "]"
}

func stablePrivateIdentifier(role, source string) string {
	base, err := naming.Property(source)
	if err != nil || base == "" {
		base = "value"
	}
	// The suffix is a lossless encoding, not a truncated digest. Two distinct
	// role/source pairs therefore cannot produce the same private identifier.
	return "__sdkgen_" + base + "_r" + hex.EncodeToString([]byte(role)) + "_s" + hex.EncodeToString([]byte(source))
}

type runtimeProperty struct {
	key   string
	value string
}

// runtimeObjectExpression constructs externally keyed records through
// Object.fromEntries. Unlike a plain object literal, "__proto__" is installed
// as an ordinary enumerable own data property. The generated call is marked
// pure so bundlers can discard unused runtime concerns, including recursively
// rendered records passed as arguments.
func runtimeObjectExpression(properties []runtimeProperty) string {
	sorted := append([]runtimeProperty(nil), properties...)
	sort.SliceStable(sorted, func(left, right int) bool {
		return sorted[left].key < sorted[right].key
	})
	entries := make([]string, 0, len(sorted))
	for _, property := range sorted {
		entries = append(entries, "["+quoteTS(property.key)+", "+property.value+"]")
	}
	return "/* @__PURE__ */ Object.fromEntries([" + strings.Join(entries, ", ") + "])"
}

// runtimeJSONExpression renders JSON data as deterministic JavaScript source.
// Object members are sorted and recursively constructed without prototype
// setter semantics; array order remains the source order.
func runtimeJSONExpression(value any) (string, error) {
	switch typed := value.(type) {
	case map[string]any:
		properties := make([]runtimeProperty, 0, len(typed))
		for key, item := range typed {
			rendered, err := runtimeJSONExpression(item)
			if err != nil {
				return "", fmt.Errorf("JSON property %q: %w", key, err)
			}
			properties = append(properties, runtimeProperty{key: key, value: rendered})
		}
		return runtimeObjectExpression(properties), nil
	case map[string]map[string]any:
		values := make(map[string]any, len(typed))
		for key, item := range typed {
			values[key] = item
		}
		return runtimeJSONExpression(values)
	case []any:
		items := make([]string, 0, len(typed))
		for index, item := range typed {
			rendered, err := runtimeJSONExpression(item)
			if err != nil {
				return "", fmt.Errorf("JSON item %d: %w", index, err)
			}
			items = append(items, rendered)
		}
		return "[" + strings.Join(items, ", ") + "]", nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("marshal JSON literal: %w", err)
		}
		return string(data), nil
	}
}

func readonlyJSONType(value any) (string, error) {
	switch typed := value.(type) {
	case map[string]any:
		names := make([]string, 0, len(typed))
		for name := range typed {
			names = append(names, name)
		}
		sort.Strings(names)
		fields := make([]string, 0, len(names))
		for _, name := range names {
			rendered, err := readonlyJSONType(typed[name])
			if err != nil {
				return "", fmt.Errorf("JSON property %q: %w", name, err)
			}
			fields = append(fields, "readonly "+quoteTS(name)+": "+rendered)
		}
		return "{ " + strings.Join(fields, "; ") + " }", nil
	case []any:
		items := make([]string, 0, len(typed))
		for index, item := range typed {
			rendered, err := readonlyJSONType(item)
			if err != nil {
				return "", fmt.Errorf("JSON item %d: %w", index, err)
			}
			items = append(items, rendered)
		}
		return "readonly [" + strings.Join(items, ", ") + "]", nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("marshal JSON literal type: %w", err)
		}
		return string(data), nil
	}
}
