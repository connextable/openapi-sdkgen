// openapi-sdkgen compiles OpenAPI documents into client SDK packages.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) == 1 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Fprintln(os.Stdout, "usage: openapi-sdkgen generate --input <document> --target <target> --output <directory>")
		return
	}
	fmt.Fprintln(os.Stderr, "openapi-sdkgen: generator command is not configured")
	os.Exit(2)
}
