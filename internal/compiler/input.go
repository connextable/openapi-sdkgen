package sdkgen

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	inputMaxBytes = 64 << 20
	inputTimeout  = 30 * time.Second
)

type inputSource struct {
	data       []byte
	display    string
	filePath   string
	fileBase   string
	remoteBase *url.URL
	stdin      bool
}

func loadInputSource(input string, options CompileOptions) (inputSource, error) {
	if input == "" {
		return inputSource{}, errors.New("OpenAPI input is empty")
	}
	if input == "-" {
		reader := options.InputReader
		if reader == nil {
			reader = os.Stdin
		}
		data, err := readInput(reader, "standard input")
		if err != nil {
			return inputSource{}, err
		}
		source := inputSource{data: data, display: "standard input", stdin: true}
		if options.InputBase == "" {
			return source, nil
		}
		base, err := parseInputBase(options.InputBase)
		if err != nil {
			return inputSource{}, err
		}
		source.fileBase = base.fileBase
		source.remoteBase = base.remoteBase
		return source, nil
	}
	if options.InputBase != "" {
		return inputSource{}, errors.New("--input-base is only valid with --input -")
	}
	if isURLInput(input) {
		parsed, err := url.Parse(input)
		if err != nil || parsed.Scheme == "" {
			return inputSource{}, fmt.Errorf("parse OpenAPI input URL: %w", err)
		}
		switch strings.ToLower(parsed.Scheme) {
		case "file":
			return loadFileURLInput(parsed)
		case "http", "https":
			return loadHTTPInput(parsed, options.Offline)
		default:
			return inputSource{}, fmt.Errorf("unsupported OpenAPI input scheme %q; use a path, file URL, HTTP(S) URL, or -", parsed.Scheme)
		}
	}
	return loadFileInput(input)
}

func isURLInput(value string) bool {
	return (len(value) >= len("file:") && strings.EqualFold(value[:len("file:")], "file:")) || strings.Contains(value, "://")
}

func loadFileInput(value string) (inputSource, error) {
	path, err := filepath.Abs(value)
	if err != nil {
		return inputSource{}, fmt.Errorf("resolve OpenAPI document path: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return inputSource{}, fmt.Errorf("read OpenAPI document: %w", err)
	}
	if len(data) == 0 {
		return inputSource{}, fmt.Errorf("OpenAPI document %s is empty", path)
	}
	return inputSource{data: data, display: path, filePath: path, fileBase: filepath.Dir(path)}, nil
}

func loadFileURLInput(value *url.URL) (inputSource, error) {
	if value.Host != "" && !strings.EqualFold(value.Host, "localhost") {
		return inputSource{}, fmt.Errorf("file URL host %q is not local", value.Host)
	}
	if value.RawQuery != "" || value.Fragment != "" {
		return inputSource{}, errors.New("file URL must not contain a query or fragment")
	}
	if !filepath.IsAbs(filepath.FromSlash(value.Path)) {
		return inputSource{}, errors.New("file URL must contain an absolute path")
	}
	return loadFileInput(filepath.FromSlash(value.Path))
}

func loadHTTPInput(value *url.URL, offline bool) (inputSource, error) {
	if offline {
		return inputSource{}, errors.New("--offline cannot fetch an HTTP(S) OpenAPI input; provide a local file or stdin")
	}
	if value.User != nil || value.Fragment != "" {
		return inputSource{}, errors.New("HTTP(S) OpenAPI input must not contain credentials or a fragment")
	}
	client := &http.Client{
		Timeout: inputTimeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= remoteReferenceRedirect {
				return errors.New("OpenAPI input redirect limit exceeded")
			}
			return nil
		},
	}
	request, err := http.NewRequest(http.MethodGet, value.String(), nil)
	if err != nil {
		return inputSource{}, fmt.Errorf("create OpenAPI input request: %w", err)
	}
	request.Header.Set("Accept", "application/json, application/yaml, text/yaml, */*;q=0.1")
	response, err := client.Do(request)
	if err != nil {
		return inputSource{}, fmt.Errorf("fetch OpenAPI input %s: %w", value, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return inputSource{}, fmt.Errorf("fetch OpenAPI input %s: unexpected HTTP status %s", value, response.Status)
	}
	data, err := readInput(response.Body, "HTTP OpenAPI input")
	if err != nil {
		return inputSource{}, err
	}
	final := *response.Request.URL
	return inputSource{data: data, display: final.String(), remoteBase: documentURLBase(&final)}, nil
}

func parseInputBase(value string) (inputSource, error) {
	if isURLInput(value) {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" {
			return inputSource{}, fmt.Errorf("parse --input-base URL: %w", err)
		}
		switch strings.ToLower(parsed.Scheme) {
		case "file":
			if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
				return inputSource{}, fmt.Errorf("file URL host %q is not local", parsed.Host)
			}
			if parsed.RawQuery != "" || parsed.Fragment != "" || !filepath.IsAbs(filepath.FromSlash(parsed.Path)) {
				return inputSource{}, errors.New("--input-base file URL must contain a local absolute path without query or fragment")
			}
			return inputSource{fileBase: filepath.Dir(filepath.FromSlash(parsed.Path))}, nil
		case "http", "https":
			if parsed.User != nil || parsed.Fragment != "" {
				return inputSource{}, errors.New("--input-base HTTP(S) URL must not contain credentials or a fragment")
			}
			return inputSource{remoteBase: documentURLBase(parsed)}, nil
		default:
			return inputSource{}, fmt.Errorf("unsupported --input-base scheme %q", parsed.Scheme)
		}
	}
	base, err := filepath.Abs(value)
	if err != nil {
		return inputSource{}, fmt.Errorf("resolve --input-base path: %w", err)
	}
	return inputSource{fileBase: filepath.Dir(base)}, nil
}

func documentURLBase(value *url.URL) *url.URL {
	base := *value
	base.RawQuery = ""
	base.Fragment = ""
	base.Path = path.Dir(base.Path)
	if base.Path == "." {
		base.Path = "/"
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	base.RawPath = ""
	return &base
}

func readInput(reader io.Reader, label string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, inputMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s is empty", label)
	}
	if len(data) > inputMaxBytes {
		return nil, fmt.Errorf("%s exceeds %d byte limit", label, inputMaxBytes)
	}
	return data, nil
}
