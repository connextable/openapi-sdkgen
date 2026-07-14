package sdkgen

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHTTPHeaderEnvValidation(t *testing.T) {
	for _, mapping := range []string{
		" Authorization=TOKEN",
		"Authorization=TOKEN ",
		"Authorization",
		"Authorization=TOKEN=OTHER",
		"Bad Header=TOKEN",
		"Host=TOKEN",
		"Cookie=TOKEN",
		"Authorization=1TOKEN",
	} {
		t.Run(mapping, func(t *testing.T) {
			if _, _, err := parseHTTPHeaderEnv(mapping); err == nil {
				t.Fatalf("parseHTTPHeaderEnv(%q) succeeded", mapping)
			}
		})
	}
	t.Setenv("SDKGEN_HTTP_TOKEN", "credential-sentinel")
	headers, err := resolveHTTPHeaders([]string{"Authorization=SDKGEN_HTTP_TOKEN", "Accept=SDKGEN_HTTP_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	if got := headers.Get("Authorization"); got != "credential-sentinel" {
		t.Fatalf("Authorization = %q", got)
	}
	if _, err := resolveHTTPHeaders([]string{"Authorization=SDKGEN_HTTP_TOKEN", "authorization=SDKGEN_HTTP_TOKEN"}); err == nil {
		t.Fatal("case-insensitive duplicate headers succeeded")
	}
	if validHTTPHeaderValue("credential\x00sentinel") {
		t.Fatal("control-character header value succeeded")
	}
}

func TestHTTPInputPreflightRejectsInvalidSettingsBeforeDial(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()
	directory := t.TempDir()
	badCA := filepath.Join(directory, "bad-ca.pem")
	if err := os.WriteFile(badCA, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	malformedCert := filepath.Join(directory, "malformed-cert.pem")
	malformedKey := filepath.Join(directory, "malformed-key.pem")
	if err := os.WriteFile(malformedCert, []byte("credential-sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(malformedKey, []byte("credential-sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstTLS := httptest.NewTLSServer(http.NotFoundHandler())
	defer firstTLS.Close()
	_, firstCert, _ := writeTLSServerCredentials(t, firstTLS)
	mismatchedPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	mismatchedKey := filepath.Join(directory, "mismatched-key.pem")
	mismatchedKeyBytes, err := x509.MarshalPKCS8PrivateKey(mismatchedPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mismatchedKey, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: mismatchedKeyBytes}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SDKGEN_EMPTY_TOKEN", "")
	t.Setenv("SDKGEN_INVALID_TOKEN", "credential-sentinel\x7f")
	t.Setenv("SDKGEN_VALID_TOKEN", "credential-sentinel")

	for _, test := range []struct {
		name    string
		options CompileOptions
		want    string
	}{
		{name: "missing environment", options: CompileOptions{HTTPHeaderEnv: []string{"Authorization=SDKGEN_MISSING_TOKEN"}}, want: "non-empty environment"},
		{name: "empty environment", options: CompileOptions{HTTPHeaderEnv: []string{"Authorization=SDKGEN_EMPTY_TOKEN"}}, want: "non-empty environment"},
		{name: "malformed mapping", options: CompileOptions{HTTPHeaderEnv: []string{"Authorization=SDKGEN_VALID_TOKEN=OTHER"}}, want: "invalid --http-header-env"},
		{name: "invalid resolved value", options: CompileOptions{HTTPHeaderEnv: []string{"Authorization=SDKGEN_INVALID_TOKEN"}}, want: "invalid header value"},
		{name: "certificate pair", options: CompileOptions{TLSClientCert: "certificate.pem"}, want: "provided together"},
		{name: "invalid certificate files", options: CompileOptions{TLSClientCert: filepath.Join(directory, "certificate.pem"), TLSClientKey: filepath.Join(directory, "key.pem")}, want: "load TLS client certificate"},
		{name: "malformed certificate PEM", options: CompileOptions{TLSClientCert: malformedCert, TLSClientKey: malformedKey}, want: "load TLS client certificate"},
		{name: "mismatched certificate pair", options: CompileOptions{TLSClientCert: firstCert, TLSClientKey: mismatchedKey}, want: "private key does not match public key"},
		{name: "invalid CA", options: CompileOptions{TLSCAFile: badCA}, want: "CERTIFICATE"},
		{name: "unsafe header", options: CompileOptions{HTTPHeaderEnv: []string{"Cookie=SDKGEN_MISSING_TOKEN"}}, want: "not allowed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := loadInputSource(server.URL, test.options)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
			if strings.Contains(err.Error(), "credential-sentinel") {
				t.Fatalf("preflight error leaked credential or key material: %v", err)
			}
			if called {
				t.Fatal("invalid HTTP input settings opened a network connection")
			}
		})
	}
}

func TestHTTPStatusReasonCannotLeakHeaderValue(t *testing.T) {
	const secret = "credential-sentinel"
	t.Setenv("SDKGEN_HTTP_TOKEN", secret)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverDone := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer connection.Close()
		request, err := http.ReadRequest(bufio.NewReader(connection))
		if err != nil {
			serverDone <- err
			return
		}
		if got := request.Header.Get("Authorization"); got != secret {
			serverDone <- fmt.Errorf("Authorization = %q", got)
			return
		}
		_, err = fmt.Fprintf(connection, "HTTP/1.1 499 %s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", secret)
		serverDone <- err
	}()
	var warnings bytes.Buffer
	_, err = loadInputSource("http://"+listener.Addr().String(), CompileOptions{
		HTTPHeaderEnv:     []string{"Authorization=SDKGEN_HTTP_TOKEN"},
		HTTPWarningWriter: &warnings,
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected HTTP status 499") || strings.Contains(err.Error(), secret) {
		t.Fatalf("HTTP status error = %v", err)
	}
	if serverErr := <-serverDone; serverErr != nil {
		t.Fatal(serverErr)
	}
}

func TestHTTPInputSettingsRejectNonHTTPInputs(t *testing.T) {
	t.Setenv("SDKGEN_HTTP_TOKEN", "credential-sentinel")
	if _, err := loadInputSource("-", CompileOptions{
		InputReader:   strings.NewReader("ignored"),
		HTTPHeaderEnv: []string{"Authorization=SDKGEN_HTTP_TOKEN"},
	}); err == nil || !strings.Contains(err.Error(), "only valid with an HTTP(S)") {
		t.Fatalf("stdin setting error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(path, []byte("openapi: 3.2.0\ninfo: {title: Example, version: '1'}\npaths: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadInputSource(path, CompileOptions{TLSCAFile: path}); err == nil || !strings.Contains(err.Error(), "only valid with an HTTP(S)") {
		t.Fatalf("file setting error = %v", err)
	}
}

func TestHTTPInputHeaderWarningIsSecretFreeAndOptional(t *testing.T) {
	const secret = "credential-sentinel"
	t.Setenv("SDKGEN_HTTP_TOKEN", secret)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != secret {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = response.Write([]byte("openapi: 3.2.0\ninfo: {title: Example, version: '1'}\npaths: {}\n"))
	}))
	defer server.Close()

	var warnings bytes.Buffer
	if _, err := loadInputSource(server.URL, CompileOptions{
		HTTPHeaderEnv:     []string{"Authorization=SDKGEN_HTTP_TOKEN"},
		HTTPWarningWriter: &warnings,
	}); err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(warnings.String(), "warning:"); count != 1 {
		t.Fatalf("warning count = %d: %q", count, warnings.String())
	}
	if strings.Contains(warnings.String(), secret) {
		t.Fatalf("warning leaked secret: %q", warnings.String())
	}
	if _, err := loadInputSource(server.URL, CompileOptions{HTTPHeaderEnv: []string{"Authorization=SDKGEN_HTTP_TOKEN"}}); err != nil {
		t.Fatalf("nil warning writer failed: %v", err)
	}
}

func TestHTTPInputHeaderWarningFailurePreventsRequest(t *testing.T) {
	t.Setenv("SDKGEN_HTTP_TOKEN", "credential-sentinel")
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()
	_, err := loadInputSource(server.URL, CompileOptions{
		HTTPHeaderEnv:     []string{"Authorization=SDKGEN_HTTP_TOKEN"},
		HTTPWarningWriter: failingWriter{},
	})
	if err == nil || !strings.Contains(err.Error(), "write HTTP input security warning") {
		t.Fatalf("warning write error = %v", err)
	}
	if called {
		t.Fatal("request proceeded after warning write failure")
	}
}

func TestAdditionalRootCAsRejectNonCertificatePEM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte("-----BEGIN PRIVATE KEY-----\nZm9v\n-----END PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAdditionalRootCAs(path); err == nil || !strings.Contains(err.Error(), "CERTIFICATE") {
		t.Fatalf("CA error = %v", err)
	}
}

func TestAdditionalRootCAsRejectAllNonWhitespaceJunk(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	caPath, _, _ := writeTLSServerCredentials(t, server)
	validCA, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string][]byte{
		"leading":  append([]byte("junk\n"), validCA...),
		"between":  append(append(validCA, []byte("\njunk\n")...), validCA...),
		"trailing": append(validCA, []byte("\njunk")...),
		"partial":  []byte("-----BEGIN CERTIFICATE-----\npartial"),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ca.pem")
			if err := os.WriteFile(path, contents, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadAdditionalRootCAs(path); err == nil {
				t.Fatal("invalid CA bundle succeeded")
			}
		})
	}
}

func TestHTTPInputTLSSettingsRejectHTTPBeforeDial(t *testing.T) {
	tlsServer := httptest.NewTLSServer(http.NotFoundHandler())
	defer tlsServer.Close()
	caPath, certPath, keyPath := writeTLSServerCredentials(t, tlsServer)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()
	for _, test := range []struct {
		name    string
		options CompileOptions
	}{
		{name: "CA", options: CompileOptions{TLSCAFile: caPath}},
		{name: "client certificate", options: CompileOptions{TLSClientCert: certPath, TLSClientKey: keyPath}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := loadInputSource(server.URL, test.options); err == nil || !strings.Contains(err.Error(), "require an HTTPS") {
				t.Fatalf("HTTP TLS setting error = %v", err)
			}
			if called {
				t.Fatal("HTTP TLS setting opened a request")
			}
		})
	}
}

func TestProtectedHTTPInputRedirectsStayOnOrigin(t *testing.T) {
	const token = "credential-sentinel"
	t.Setenv("SDKGEN_HTTP_TOKEN", token)
	crossOriginCalled := false
	crossOrigin := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		crossOriginCalled = true
	}))
	defer crossOrigin.Close()
	root := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/same-origin":
			http.Redirect(response, request, "/final", http.StatusFound)
		case "/final":
			if got := request.Header.Get("Authorization"); got != token {
				t.Errorf("Authorization = %q", got)
			}
			if got := request.Header.Get("Accept"); got != token {
				t.Errorf("Accept = %q", got)
			}
			_, _ = response.Write([]byte("openapi: 3.2.0\ninfo: {title: Example, version: '1'}\npaths: {}\n"))
		case "/cross-origin":
			http.Redirect(response, request, crossOrigin.URL+"/?echo="+token, http.StatusFound)
		case "/malformed-redirect":
			response.Header().Set("Location", "%zz"+token)
			response.WriteHeader(http.StatusFound)
		}
	}))
	defer root.Close()
	options := CompileOptions{HTTPHeaderEnv: []string{
		"Authorization=SDKGEN_HTTP_TOKEN",
		"Accept=SDKGEN_HTTP_TOKEN",
	}}
	if _, err := loadInputSource(root.URL+"/same-origin", options); err != nil {
		t.Fatalf("same-origin redirect: %v", err)
	}
	if _, err := loadInputSource(root.URL+"/cross-origin", options); err == nil || !strings.Contains(err.Error(), "leaves the OpenAPI input origin") || strings.Contains(err.Error(), token) {
		t.Fatalf("cross-origin redirect error = %v", err)
	}
	if _, err := loadInputSource(root.URL+"/malformed-redirect", options); err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("malformed redirect error = %v", err)
	}
	if crossOriginCalled {
		t.Fatal("protected cross-origin redirect opened a request")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("write failed")
}

func TestUnprotectedHTTPInputCanFollowCrossOriginRedirect(t *testing.T) {
	crossOrigin := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("openapi: 3.2.0\ninfo: {title: Example, version: '1'}\npaths: {}\n"))
	}))
	defer crossOrigin.Close()
	root := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, crossOrigin.URL, http.StatusFound)
	}))
	defer root.Close()
	if _, err := loadInputSource(root.URL, CompileOptions{}); err != nil {
		t.Fatalf("unprotected cross-origin redirect: %v", err)
	}
}

func TestHTTPSInputSupportsAdditionalCAAndClientCertificate(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if len(request.TLS.PeerCertificates) == 0 {
			t.Error("missing client certificate")
		}
		_, _ = response.Write([]byte("openapi: 3.2.0\ninfo: {title: Example, version: '1'}\npaths: {}\n"))
	}))
	server.TLS = &tls.Config{ClientAuth: tls.RequireAnyClientCert}
	server.StartTLS()
	defer server.Close()

	directory := t.TempDir()
	caPath := filepath.Join(directory, "ca.pem")
	certPath := filepath.Join(directory, "client.pem")
	keyPath := filepath.Join(directory, "client-key.pem")
	certificate := server.TLS.Certificates[0]
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0o600); err != nil {
		t.Fatal(err)
	}
	privateKey, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey}), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := loadInputSource(server.URL, CompileOptions{}); err == nil {
		t.Fatal("untrusted server succeeded")
	}
	if _, err := loadInputSource(server.URL, CompileOptions{
		TLSClientCert: certPath,
		TLSClientKey:  keyPath,
		TLSCAFile:     caPath,
	}); err != nil {
		t.Fatalf("custom CA and client certificate: %v", err)
	}
}

func TestHTTPSInputSettingsApplyToSameOriginReferences(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows fails protected same-origin reference caching before persistence")
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if len(request.TLS.PeerCertificates) == 0 {
			t.Error("missing client certificate")
		}
		switch request.URL.Path {
		case "/openapi.yaml":
			_, _ = response.Write([]byte(`openapi: 3.2.0
info: {title: Protected reference, version: "1"}
paths:
  /things:
    get:
      operationId: listThings
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema: {$ref: schemas.yaml#/Thing}
`))
		case "/schemas.yaml":
			_, _ = response.Write([]byte("Thing:\n  type: object\n  properties:\n    id: {type: string}\n"))
		default:
			http.NotFound(response, request)
		}
	}))
	server.TLS = &tls.Config{ClientAuth: tls.RequireAnyClientCert}
	server.StartTLS()
	defer server.Close()
	caPath, certPath, keyPath := writeTLSServerCredentials(t, server)
	compiled, err := CompileInputWithOptions(server.URL+"/openapi.yaml", CompileOptions{
		TLSClientCert: certPath,
		TLSClientKey:  keyPath,
		TLSCAFile:     caPath,
		RefLockPath:   filepath.Join(t.TempDir(), "refs.lock"),
		UpdateRefLock: true,
	})
	if err != nil || len(compiled.Operations) != 1 {
		t.Fatalf("same-origin TLS reference compilation = %#v, %v", compiled, err)
	}
}

func writeTLSServerCredentials(t *testing.T, server *httptest.Server) (string, string, string) {
	t.Helper()
	directory := t.TempDir()
	caPath := filepath.Join(directory, "ca.pem")
	certPath := filepath.Join(directory, "client.pem")
	keyPath := filepath.Join(directory, "client-key.pem")
	certificate := server.TLS.Certificates[0]
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0o600); err != nil {
		t.Fatal(err)
	}
	privateKey, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey}), 0o600); err != nil {
		t.Fatal(err)
	}
	return caPath, certPath, keyPath
}

func TestHTTPSProxyIsRejectedBeforeDialForPrivateTLS(t *testing.T) {
	if os.Getenv("SDKGEN_HTTPS_PROXY_HELPER") == "1" {
		config := &httpInputConfig{privateTLS: true, tlsConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
		client, err := config.newClient(&url.URL{Scheme: "https", Host: "example.test"})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		request, err := http.NewRequest(http.MethodGet, "https://example.test/openapi.yaml", nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_, err = client.Do(request)
		if err == nil || !strings.Contains(err.Error(), "HTTPS proxy cannot be used") || !strings.Contains(err.Error(), "NO_PROXY") || strings.Contains(err.Error(), "credential-sentinel") {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestHTTPSProxyIsRejectedBeforeDialForPrivateTLS$")
	command.Env = append(os.Environ(),
		"SDKGEN_HTTPS_PROXY_HELPER=1",
		"HTTPS_PROXY=https://user:credential-sentinel@127.0.0.1:1",
		"https_proxy=https://user:credential-sentinel@127.0.0.1:1",
		"NO_PROXY=",
		"no_proxy=",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("HTTPS proxy helper: %v\n%s", err, output)
	}
}

func TestPrivateTLSProxyPolicyPreservesSupportedSelections(t *testing.T) {
	if proxyCase := os.Getenv("SDKGEN_PROXY_POLICY_HELPER"); proxyCase != "" {
		config := &httpInputConfig{privateTLS: true, tlsConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
		client, err := config.newClient(&url.URL{Scheme: "https", Host: "example.test"})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		transport, ok := client.Transport.(*http.Transport)
		if !ok {
			fmt.Fprintln(os.Stderr, "private TLS client did not clone the default transport")
			os.Exit(1)
		}
		request, err := http.NewRequest(http.MethodGet, "https://example.test/openapi.yaml", nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		proxyURL, err := transport.Proxy(request)
		switch proxyCase {
		case "http":
			if err != nil || proxyURL == nil || proxyURL.Scheme != "http" {
				fmt.Fprintf(os.Stderr, "HTTP proxy selection = %v, %v\n", proxyURL, err)
				os.Exit(1)
			}
		case "socks":
			if err != nil || proxyURL == nil || proxyURL.Scheme != "socks5" {
				fmt.Fprintf(os.Stderr, "SOCKS proxy selection = %v, %v\n", proxyURL, err)
				os.Exit(1)
			}
		case "no-proxy":
			if err != nil || proxyURL != nil {
				fmt.Fprintf(os.Stderr, "NO_PROXY selection = %v, %v\n", proxyURL, err)
				os.Exit(1)
			}
		default:
			fmt.Fprintln(os.Stderr, "unknown proxy helper case")
			os.Exit(1)
		}
		return
	}
	for _, test := range []struct {
		name      string
		proxyCase string
		proxyURL  string
		noProxy   string
	}{
		{name: "HTTP proxy", proxyCase: "http", proxyURL: "http://127.0.0.1:1"},
		{name: "SOCKS proxy", proxyCase: "socks", proxyURL: "socks5://127.0.0.1:1"},
		{name: "NO_PROXY bypass", proxyCase: "no-proxy", proxyURL: "https://user:credential-sentinel@127.0.0.1:1", noProxy: "example.test"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestPrivateTLSProxyPolicyPreservesSupportedSelections$")
			command.Env = append(os.Environ(),
				"SDKGEN_PROXY_POLICY_HELPER="+test.proxyCase,
				"HTTPS_PROXY="+test.proxyURL,
				"https_proxy="+test.proxyURL,
				"NO_PROXY="+test.noProxy,
				"no_proxy="+test.noProxy,
			)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("proxy helper: %v\n%s", err, output)
			}
		})
	}
}

func TestHeaderOnlyInputUsesDefaultTransportProxyBehavior(t *testing.T) {
	config := &httpInputConfig{protected: true, hasHeaderMappings: true}
	client, err := config.newClient(&url.URL{Scheme: "https", Host: "example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if client.Transport != nil {
		t.Fatalf("header-only client transport = %#v, want default transport", client.Transport)
	}
}
