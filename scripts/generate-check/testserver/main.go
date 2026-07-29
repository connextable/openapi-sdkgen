package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
)

func main() {
	flags := flag.NewFlagSet("generate-check-testserver", flag.ExitOnError)
	input := flags.String("input", "", "response body file")
	urlOutput := flags.String("url-output", "", "listening URL output")
	countOutput := flags.String("count-output", "", "request count output")
	flags.Parse(os.Args[1:])
	if *input == "" || *urlOutput == "" || *countOutput == "" {
		panic(errors.New("--input, --url-output, and --count-output are required"))
	}
	body, err := os.ReadFile(*input)
	if err != nil {
		panic(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	var requests atomic.Int64
	handler := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		count := requests.Add(1)
		if err := os.WriteFile(*countOutput, []byte(strconv.FormatInt(count, 10)), 0o600); err != nil {
			http.Error(response, "count failed", http.StatusInternalServerError)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write(body)
	})
	server := &http.Server{Handler: handler}
	if err := os.WriteFile(*urlOutput, []byte(fmt.Sprintf("http://%s/openapi.json", listener.Addr())), 0o600); err != nil {
		panic(err)
	}
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
