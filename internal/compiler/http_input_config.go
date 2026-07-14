package sdkgen

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var (
	defaultOpenAPIInputAccept      = "application/json, application/yaml, text/yaml, */*;q=0.1"
	errHTTPInputRedirectLimit      = errors.New("OpenAPI input redirect limit exceeded")
	errProtectedHTTPRedirectOrigin = errors.New("protected HTTP redirect leaves the OpenAPI input origin")
	errHTTPSProxyPrivateTLS        = errors.New("HTTPS proxy cannot be used with --tls-client-cert, --tls-client-key, or --tls-ca-file; configure NO_PROXY for the input origin")
)

type httpInputConfig struct {
	headers           http.Header
	tlsConfig         *tls.Config
	protected         bool
	privateTLS        bool
	warningWriter     io.Writer
	hasHeaderMappings bool
}

func configureHTTPInput(options CompileOptions) (*httpInputConfig, error) {
	headers, err := resolveHTTPHeaders(options.HTTPHeaderEnv)
	if err != nil {
		return nil, err
	}

	hasCertificate := options.TLSClientCert != "" || options.TLSClientKey != ""
	if (options.TLSClientCert == "") != (options.TLSClientKey == "") {
		return nil, errors.New("--tls-client-cert and --tls-client-key must be provided together")
	}

	var tlsConfig *tls.Config
	if hasCertificate || options.TLSCAFile != "" {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	if hasCertificate {
		certificate, err := tls.LoadX509KeyPair(options.TLSClientCert, options.TLSClientKey)
		if err != nil {
			return nil, fmt.Errorf("load TLS client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	if options.TLSCAFile != "" {
		roots, err := loadAdditionalRootCAs(options.TLSCAFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = roots
	}

	return &httpInputConfig{
		headers:           headers,
		tlsConfig:         tlsConfig,
		protected:         len(headers) != 0 || tlsConfig != nil,
		privateTLS:        tlsConfig != nil,
		warningWriter:     options.HTTPWarningWriter,
		hasHeaderMappings: len(options.HTTPHeaderEnv) != 0,
	}, nil
}

func validateNonHTTPInputOptions(options CompileOptions) error {
	if len(options.HTTPHeaderEnv) != 0 || options.TLSClientCert != "" || options.TLSClientKey != "" || options.TLSCAFile != "" {
		return errors.New("--http-header-env, --tls-client-cert, --tls-client-key, and --tls-ca-file are only valid with an HTTP(S) OpenAPI input")
	}
	return nil
}

func resolveHTTPHeaders(mappings []string) (http.Header, error) {
	headers := make(http.Header, len(mappings))
	seen := make(map[string]struct{}, len(mappings))
	for _, mapping := range mappings {
		name, envName, err := parseHTTPHeaderEnv(mapping)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("--http-header-env duplicates header %q", name)
		}
		seen[key] = struct{}{}
		value, ok := os.LookupEnv(envName)
		if !ok || value == "" {
			return nil, fmt.Errorf("--http-header-env %q requires a non-empty environment variable", name)
		}
		if !validHTTPHeaderValue(value) {
			return nil, fmt.Errorf("--http-header-env %q resolves to an invalid header value", name)
		}
		headers.Set(name, value)
	}
	return headers, nil
}

func parseHTTPHeaderEnv(mapping string) (string, string, error) {
	if mapping == "" || strings.TrimSpace(mapping) != mapping || strings.Count(mapping, "=") != 1 {
		return "", "", fmt.Errorf("invalid --http-header-env %q; use Header-Name=ENV_VAR without surrounding whitespace", mapping)
	}
	name, envName, _ := strings.Cut(mapping, "=")
	if !validHTTPHeaderName(name) {
		return "", "", fmt.Errorf("invalid --http-header-env header name %q", name)
	}
	if isUnsafeHTTPHeader(name) {
		return "", "", fmt.Errorf("--http-header-env header %q is not allowed", name)
	}
	if !validEnvironmentVariableName(envName) {
		return "", "", fmt.Errorf("invalid --http-header-env environment variable %q", envName)
	}
	return name, envName, nil
}

func validHTTPHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if !(character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(character))) {
			return false
		}
	}
	return true
}

func isUnsafeHTTPHeader(value string) bool {
	switch strings.ToLower(value) {
	case "host", "content-length", "connection", "proxy-authorization", "proxy-connection", "transfer-encoding", "upgrade", "trailer", "te", "keep-alive", "cookie":
		return true
	default:
		return false
	}
}

func validEnvironmentVariableName(value string) bool {
	if value == "" || !(value[0] == '_' || value[0] >= 'A' && value[0] <= 'Z' || value[0] >= 'a' && value[0] <= 'z') {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if !(character == '_' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}

func validHTTPHeaderValue(value string) bool {
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character == 0 || character == 0x7f || (character < 0x20 && character != '\t') {
			return false
		}
	}
	return true
}

func loadAdditionalRootCAs(path string) (*x509.CertPool, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read --tls-ca-file: %w", err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("load system certificate pool: %w", err)
	}
	if roots == nil {
		roots = x509.NewCertPool()
	}
	remaining := contents
	certificates := 0
	for len(bytes.TrimSpace(remaining)) != 0 {
		remaining = bytes.TrimSpace(remaining)
		if !bytes.HasPrefix(remaining, []byte("-----BEGIN ")) {
			return nil, errors.New("--tls-ca-file must contain only PEM CERTIFICATE blocks")
		}
		block, rest := pem.Decode(remaining)
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, errors.New("--tls-ca-file must contain only PEM CERTIFICATE blocks")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse --tls-ca-file certificate: %w", err)
		}
		roots.AddCert(certificate)
		certificates++
		remaining = rest
	}
	if certificates == 0 {
		return nil, errors.New("--tls-ca-file must contain at least one PEM CERTIFICATE block")
	}
	return roots, nil
}

func (config *httpInputConfig) applyHeaders(request *http.Request) {
	request.Header.Set("Accept", defaultOpenAPIInputAccept)
	for name, values := range config.headers {
		request.Header[name] = append([]string(nil), values...)
	}
}

func (config *httpInputConfig) newClient(inputURL *url.URL) (*http.Client, error) {
	origin, err := inputURLOrigin(inputURL)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: inputTimeout}
	if config.privateTLS {
		baseTransport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, errors.New("default HTTP transport does not support TLS input configuration")
		}
		transport := baseTransport.Clone()
		transport.TLSClientConfig = mergeTLSConfig(transport.TLSClientConfig, config.tlsConfig)
		proxy := transport.Proxy
		transport.Proxy = func(request *http.Request) (*url.URL, error) {
			if proxy == nil {
				return nil, nil
			}
			proxyURL, err := proxy(request)
			if err != nil {
				return nil, err
			}
			if proxyURL != nil && strings.EqualFold(proxyURL.Scheme, "https") {
				return nil, errHTTPSProxyPrivateTLS
			}
			return proxyURL, nil
		}
		client.Transport = transport
	}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= remoteReferenceRedirect {
			return errHTTPInputRedirectLimit
		}
		if config.protected {
			redirectOrigin, err := inputURLOrigin(request.URL)
			if err != nil || redirectOrigin != origin {
				return errProtectedHTTPRedirectOrigin
			}
		}
		return nil
	}
	return client, nil
}

func sanitizeHTTPClientError(err error, config *httpInputConfig) error {
	if errors.Is(err, errProtectedHTTPRedirectOrigin) {
		return errProtectedHTTPRedirectOrigin
	}
	if errors.Is(err, errHTTPInputRedirectLimit) {
		return errHTTPInputRedirectLimit
	}
	if errors.Is(err, errHTTPSProxyPrivateTLS) {
		return errHTTPSProxyPrivateTLS
	}
	if config == nil || !config.protected {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("protected HTTP request timed out")
	}
	var certificateError *tls.CertificateVerificationError
	if errors.As(err, &certificateError) {
		return errors.New("protected HTTPS certificate verification failed")
	}
	return errors.New("protected HTTP request failed")
}

func safeHTTPStatus(code int) string {
	if text := http.StatusText(code); text != "" {
		return fmt.Sprintf("%d %s", code, text)
	}
	return fmt.Sprintf("%d", code)
}

func mergeTLSConfig(base *tls.Config, additional *tls.Config) *tls.Config {
	if additional == nil {
		if base == nil {
			return nil
		}
		return base.Clone()
	}
	var merged *tls.Config
	if base == nil {
		merged = &tls.Config{}
	} else {
		merged = base.Clone()
	}
	if additional.MinVersion > merged.MinVersion {
		merged.MinVersion = additional.MinVersion
	}
	if additional.RootCAs != nil {
		merged.RootCAs = additional.RootCAs
	}
	if len(additional.Certificates) != 0 {
		merged.Certificates = append([]tls.Certificate(nil), additional.Certificates...)
	}
	return merged
}
