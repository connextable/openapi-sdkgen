package typescript

import "testing"

func TestOpenAPIPointerEscapesRFC6901Segments(t *testing.T) {
	if got, want := openAPIPointer("paths", "/widgets/~draft", "get"), "#/paths/~1widgets~1~0draft/get"; got != want {
		t.Fatalf("pointer = %q, want %q", got, want)
	}
}
