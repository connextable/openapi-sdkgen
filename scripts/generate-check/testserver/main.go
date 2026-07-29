package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

func main() {
	flags := flag.NewFlagSet("generate-check-testserver", flag.ExitOnError)
	input := flags.String("input", "", "response body file")
	urlOutput := flags.String("url-output", "", "listening URL output")
	countOutput := flags.String("count-output", "", "request count output")
	reference := flags.String("reference", "", "optional relative reference response body file")
	requiredHeader := flags.String("required-header", "", "optional required Header=Value")
	redirect := flags.Bool("redirect", false, "publish a same-origin redirecting root URL")
	flags.Parse(os.Args[1:])
	if *input == "" || *urlOutput == "" || *countOutput == "" {
		panic(errors.New("--input, --url-output, and --count-output are required"))
	}
	body, err := os.ReadFile(*input)
	if err != nil {
		panic(err)
	}
	var referenceBody []byte
	if *reference != "" {
		referenceBody, err = os.ReadFile(*reference)
		if err != nil {
			panic(err)
		}
	}
	headerName, headerValue := "", ""
	if *requiredHeader != "" {
		headerName, headerValue, _ = strings.Cut(*requiredHeader, "=")
		if headerName == "" || headerValue == "" {
			panic(errors.New("--required-header must be Header=Value"))
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	var requests atomic.Int64
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		count := requests.Add(1)
		if err := os.WriteFile(*countOutput, []byte(strconv.FormatInt(count, 10)), 0o600); err != nil {
			http.Error(response, "count failed", http.StatusInternalServerError)
			return
		}
		if headerName != "" && request.Header.Get(headerName) != headerValue {
			http.Error(response, "missing required header", http.StatusUnauthorized)
			return
		}
		if *redirect && request.URL.Path == "/redirect/openapi.json" {
			http.Redirect(response, request, "/contracts/openapi.json", http.StatusFound)
			return
		}
		rootPath := "/openapi.json"
		referencePath := "/schemas/item.json"
		if *redirect {
			rootPath = "/contracts/openapi.json"
			referencePath = "/contracts/schemas/item.json"
		}
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case rootPath:
			_, _ = response.Write(body)
		case referencePath:
			if len(referenceBody) == 0 {
				http.NotFound(response, request)
				return
			}
			_, _ = response.Write(referenceBody)
		default:
			http.NotFound(response, request)
		}
	})
	server := &http.Server{Handler: handler}
	rootPath := "/openapi.json"
	if *redirect {
		rootPath = "/redirect/openapi.json"
	}
	if err := os.WriteFile(*urlOutput, []byte(fmt.Sprintf("http://%s%s", listener.Addr(), rootPath)), 0o600); err != nil {
		panic(err)
	}
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
