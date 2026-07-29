package sdkgen

import (
	"net/url"
	"strings"
)

// InputSnapshot is one securely acquired OpenAPI root document.
//
// Data is the exact bounded byte sequence read from the input. EffectiveBase is
// the canonical absolute file path or sanitized final HTTP(S) response URL that
// preserves normal relative-reference resolution when Data is later compiled
// through standard input.
type InputSnapshot struct {
	Data          []byte
	Input         string
	Display       string
	EffectiveBase string
}

// AcquireInputSnapshot reads a path, file URL, HTTP(S) URL, or standard input
// exactly once through the compiler's normal secure input path.
func AcquireInputSnapshot(input string, options CompileOptions) (InputSnapshot, error) {
	source, err := loadInputSource(input, options)
	if err != nil {
		return InputSnapshot{}, err
	}
	data := append([]byte(nil), source.data...)
	return InputSnapshot{
		Data:          data,
		Input:         snapshotInputDisplay(input),
		Display:       source.display,
		EffectiveBase: source.effective,
	}, nil
}

func snapshotInputDisplay(input string) string {
	if !isURLInput(input) {
		return input
	}
	parsed, err := url.Parse(input)
	if err != nil || parsed == nil || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return input
	}
	return sanitizedHTTPDocumentURL(parsed)
}
