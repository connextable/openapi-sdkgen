package naming

import (
	"fmt"
	"strings"
	"unicode"
)

var initialisms = map[string]string{
	"api":   "API",
	"url":   "URL",
	"id":    "ID",
	"http":  "HTTP",
	"json":  "JSON",
	"xml":   "XML",
	"oauth": "OAUTH",
	"mfa":   "MFA",
	"csrf":  "CSRF",
	"ssr":   "SSR",
	"eq":    "EQ",
	"gt":    "GT",
	"gte":   "GTE",
	"lt":    "LT",
	"lte":   "LTE",
	"ip":    "IP",
	"uri":   "URI",
	"uuid":  "UUID",
	"tls":   "TLS",
}

var reserved = map[string]struct{}{
	"break": {}, "case": {}, "catch": {}, "class": {}, "const": {},
	"continue": {}, "debugger": {}, "default": {}, "delete": {}, "do": {},
	"else": {}, "enum": {}, "export": {}, "extends": {}, "false": {},
	"finally": {}, "for": {}, "function": {}, "if": {}, "import": {},
	"in": {}, "instanceof": {}, "new": {}, "null": {}, "return": {},
	"super": {}, "switch": {}, "this": {}, "throw": {}, "true": {},
	"try": {}, "typeof": {}, "var": {}, "void": {}, "while": {},
	"with": {}, "yield": {}, "await": {}, "implements": {}, "interface": {},
	"let": {}, "package": {}, "private": {}, "protected": {}, "public": {},
	"static": {}, "abstract": {}, "arguments": {}, "as": {}, "asserts": {},
	"any": {}, "async": {}, "bigint": {}, "constructor": {}, "declare": {},
	"from": {}, "get": {}, "global": {}, "infer": {}, "is": {}, "keyof": {},
	"module": {}, "namespace": {}, "never": {}, "of": {}, "override": {},
	"readonly": {}, "require": {}, "set": {}, "satisfies": {}, "symbol": {},
	"target": {}, "type": {}, "undefined": {}, "unique": {}, "unknown": {},
	"using": {}, "value": {}, "where": {},
}

func Public(value string) (string, error) {
	words := splitWords(value)
	if len(words) == 0 {
		return "", fmt.Errorf("identifier %q has no usable words", value)
	}
	var builder strings.Builder
	for _, word := range words {
		builder.WriteString(publicWord(word))
	}
	result := builder.String()
	if unicode.IsDigit(rune(result[0])) {
		result = "Value" + result
	}
	return result, nil
}

func Property(value string) (string, error) {
	words := splitWords(value)
	if len(words) == 0 {
		return "", fmt.Errorf("identifier %q has no usable words", value)
	}
	first := strings.ToLower(words[0])
	if initialism, ok := initialisms[first]; ok {
		first = strings.ToLower(initialism)
	}
	var builder strings.Builder
	builder.WriteString(first)
	for _, word := range words[1:] {
		builder.WriteString(publicWord(word))
	}
	result := builder.String()
	if unicode.IsDigit(rune(result[0])) || isReserved(result) {
		result += "Value"
	}
	return result, nil
}

func publicWord(word string) string {
	lower := strings.ToLower(word)
	if initialism, ok := initialisms[lower]; ok {
		return initialism
	}
	runes := []rune(lower)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func splitWords(value string) []string {
	var words []string
	var current []rune
	runes := []rune(value)
	flush := func() {
		if len(current) > 0 {
			words = append(words, string(current))
			current = nil
		}
	}
	for index, char := range runes {
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) {
			flush()
			continue
		}
		if len(current) > 0 && unicode.IsUpper(char) {
			previous := runes[index-1]
			nextIsLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])
			if unicode.IsLower(previous) || unicode.IsDigit(previous) || nextIsLower {
				flush()
			}
		}
		current = append(current, char)
	}
	flush()
	return words
}

func isReserved(value string) bool {
	_, ok := reserved[value]
	return ok
}
