package typescript

import (
	"strings"
	"testing"
)

func TestJSDocTypeReferenceFlattensInlineObjectComments(t *testing.T) {
	value := jsDocTypeReference("{\n  /** property docs */\n  readonly id: string\n}")
	if strings.Contains(value, "/*") || value != "`{ readonly id: string }`" {
		t.Fatalf("reference = %q", value)
	}
}
