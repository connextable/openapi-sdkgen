package typescript

import "strings"

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
