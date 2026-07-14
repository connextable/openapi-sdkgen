package sdkgen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	remoteReferenceMaxBytes = 8 << 20
	remoteReferenceTimeout  = 15 * time.Second
	remoteReferenceRedirect = 3
)

// CompileOptions controls explicitly opt-in compiler capabilities. Empty
// options preserve the offline, contained-local-reference default.
type CompileOptions struct {
	// InputBase supplies the document location used to resolve relative $ref
	// values when input is read from standard input. It is ignored for file and
	// URL input, which already provide their own location.
	InputBase string
	// InputReader replaces standard input for --input -. A nil value uses
	// os.Stdin; tests can provide a deterministic reader.
	InputReader io.Reader
	// RemoteRefAllowlist contains exact HTTPS origins permitted for remote $ref
	// resolution, for example "https://schemas.example.test".
	RemoteRefAllowlist []string
	// RefLockPath is the remote-reference and schema-extension integrity lock.
	// When empty, CompileFileWithOptions uses <input>.openapi-sdkgen.lock.
	RefLockPath string
	// UpdateRefLock permits creating or updating RefLockPath after successful
	// compilation. Without it, missing or changed digests fail closed.
	UpdateRefLock bool
	// Offline serves already locked remote references from the local
	// content-addressed cache and never opens a network connection.
	Offline bool
	// SchemaExtensionManifests registers trusted local JSON Schema vocabulary
	// extensions. A manifest is never discovered implicitly.
	SchemaExtensionManifests []string
	// HTTPHeaderEnv maps outbound HTTP request header names to environment
	// variable names. Each value has the form Header-Name=ENV_VAR and is only
	// valid for an HTTP(S) --input URL.
	HTTPHeaderEnv []string
	// TLSClientCert and TLSClientKey provide an optional client certificate for
	// an HTTPS --input URL. They must be supplied together.
	TLSClientCert string
	TLSClientKey  string
	// TLSCAFile contains additional PEM certificate authorities trusted for an
	// HTTPS --input URL.
	TLSCAFile string
	// HTTPWarningWriter receives non-secret diagnostics about HTTP input
	// transport risks. A nil writer disables diagnostics for library callers.
	HTTPWarningWriter io.Writer

	// Tests may replace the remote transport and DNS resolver without exposing
	// those hooks through the CLI contract.
	remoteReferenceClient *http.Client
	remoteReferenceLookup hostLookup
}

type referenceLock struct {
	Version    int               `json:"version"`
	References map[string]string `json:"references,omitempty"`
	Extensions map[string]string `json:"extensions,omitempty"`
}

func defaultReferenceLockPath(input string) string {
	return input + ".openapi-sdkgen.lock"
}

func loadReferenceLock(path string, allowMissing bool) (*referenceLock, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && allowMissing {
		return &referenceLock{Version: 1, References: map[string]string{}, Extensions: map[string]string{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read reference lock %s: %w", path, err)
	}
	var lock referenceLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("decode reference lock %s: %w", path, err)
	}
	if lock.Version != 1 {
		return nil, fmt.Errorf("reference lock %s has unsupported version %d", path, lock.Version)
	}
	if lock.References == nil {
		lock.References = map[string]string{}
	}
	if lock.Extensions == nil {
		lock.Extensions = map[string]string{}
	}
	return &lock, nil
}

func writeReferenceLock(path string, lock *referenceLock) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("encode reference lock: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create reference lock directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".openapi-sdkgen-lock-*")
	if err != nil {
		return fmt.Errorf("create reference lock: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write reference lock: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return fmt.Errorf("set reference lock mode: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close reference lock: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish reference lock: %w", err)
	}
	return nil
}

type hostLookup func(context.Context, string) ([]net.IPAddr, error)

type remoteReferenceResolver struct {
	origins       map[string]struct{}
	trustedOrigin string
	lock          *referenceLock
	update        bool
	offline       bool
	cache         string
	client        *http.Client
	trustedClient *http.Client
	trustedConfig *httpInputConfig
	lookup        hostLookup
	mu            sync.Mutex
	errs          []error
}

func (r *remoteReferenceResolver) handle(rawURL string) (*http.Response, error) {
	response, err := r.fetch(rawURL)
	if err != nil {
		r.mu.Lock()
		r.errs = append(r.errs, err)
		r.mu.Unlock()
	}
	return response, err
}

func (r *remoteReferenceResolver) firstError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.errs) == 0 {
		return nil
	}
	return r.errs[0]
}

func newRemoteReferenceResolver(options CompileOptions, lock *referenceLock, cache string, trustedBase *url.URL, trustedConfig *httpInputConfig) (*remoteReferenceResolver, error) {
	origins := make(map[string]struct{}, len(options.RemoteRefAllowlist))
	for _, value := range options.RemoteRefAllowlist {
		origin, err := canonicalRemoteOrigin(value)
		if err != nil {
			return nil, fmt.Errorf("invalid --allow-remote-ref %q: %w", value, err)
		}
		origins[origin] = struct{}{}
	}
	trustedOrigin := ""
	if trustedBase != nil {
		var err error
		trustedOrigin, err = inputURLOrigin(trustedBase)
		if err != nil {
			return nil, fmt.Errorf("invalid OpenAPI input URL base: %w", err)
		}
	}
	if len(origins) == 0 && trustedOrigin == "" {
		return nil, errors.New("remote references require at least one --allow-remote-ref HTTPS origin")
	}
	lookup := func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return net.DefaultResolver.LookupIPAddr(ctx, host)
	}
	if options.remoteReferenceLookup != nil {
		lookup = options.remoteReferenceLookup
	}
	resolver := &remoteReferenceResolver{origins: origins, trustedOrigin: trustedOrigin, lock: lock, update: options.UpdateRefLock, offline: options.Offline, cache: cache, lookup: lookup}
	resolver.client = secureRemoteHTTPClient(resolver)
	if options.remoteReferenceClient != nil {
		resolver.client = options.remoteReferenceClient
	}
	if trustedOrigin != "" {
		resolver.trustedClient = trustedRemoteHTTPClient(resolver)
		if trustedConfig != nil && trustedConfig.protected {
			client, err := trustedConfig.newClient(trustedBase)
			if err != nil {
				return nil, fmt.Errorf("configure trusted remote reference HTTP client: %w", err)
			}
			resolver.trustedClient = client
			resolver.trustedConfig = trustedConfig
		}
	}
	return resolver, nil
}

func inputURLOrigin(value *url.URL) (string, error) {
	if value == nil || (strings.ToLower(value.Scheme) != "http" && strings.ToLower(value.Scheme) != "https") || value.Host == "" || value.User != nil {
		return "", errors.New("must be an unauthenticated HTTP(S) URL")
	}
	return strings.ToLower(value.Scheme) + "://" + strings.ToLower(value.Host), nil
}

func canonicalRemoteOrigin(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u == nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("must be an exact HTTPS origin without path, credentials, query, or fragment")
	}
	return u.Scheme + "://" + strings.ToLower(u.Host), nil
}

func (r *remoteReferenceResolver) validateURL(ctx context.Context, rawURL string) (*url.URL, error) {
	u, err := r.validateURLSyntax(rawURL)
	if err != nil {
		return nil, err
	}
	if err := r.validateHost(ctx, u.Hostname()); err != nil {
		return nil, fmt.Errorf("remote reference %q host: %w", rawURL, err)
	}
	return u, nil
}

func (r *remoteReferenceResolver) validateURLSyntax(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("remote reference %q must be an unauthenticated URL", rawURL)
	}
	if r.isTrustedURL(u) {
		return u, nil
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("remote reference %q must be an unauthenticated HTTPS URL", rawURL)
	}
	origin := u.Scheme + "://" + strings.ToLower(u.Host)
	if _, ok := r.origins[origin]; !ok {
		return nil, fmt.Errorf("remote reference %q origin %q is not allowlisted", rawURL, origin)
	}
	return u, nil
}

func (r *remoteReferenceResolver) isTrustedURL(value *url.URL) bool {
	origin, err := inputURLOrigin(value)
	return err == nil && origin == r.trustedOrigin
}

func (r *remoteReferenceResolver) validateHost(ctx context.Context, host string) error {
	addresses, err := r.lookup(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve DNS for %s: %w", host, err)
	}
	if len(addresses) == 0 {
		return fmt.Errorf("resolve DNS for %s: no addresses", host)
	}
	for _, address := range addresses {
		if !isPublicRemoteAddress(address.IP) {
			return fmt.Errorf("DNS result %s is not a public address", address.IP)
		}
	}
	return nil
}

func isPublicRemoteAddress(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	return address.IsValid() && address.IsGlobalUnicast() && !address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast() && !address.IsLinkLocalMulticast() && !address.IsMulticast() && !address.IsUnspecified()
}

func secureRemoteHTTPClient(resolver *remoteReferenceResolver) *http.Client {
	dialer := &net.Dialer{Timeout: remoteReferenceTimeout}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := resolver.lookup(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve DNS for %s: %w", host, err)
			}
			for _, candidate := range addresses {
				if !isPublicRemoteAddress(candidate.IP) {
					return nil, fmt.Errorf("DNS result %s is not a public address", candidate.IP)
				}
			}
			var last error
			for _, candidate := range addresses {
				connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
				if err == nil {
					return connection, nil
				}
				last = err
			}
			return nil, last
		},
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: remoteReferenceTimeout,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   remoteReferenceTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= remoteReferenceRedirect {
				return errors.New("remote reference redirect limit exceeded")
			}
			_, err := resolver.validateURL(request.Context(), request.URL.String())
			return err
		},
	}
}

func trustedRemoteHTTPClient(resolver *remoteReferenceResolver) *http.Client {
	return &http.Client{
		Timeout: inputTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= remoteReferenceRedirect {
				return errors.New("remote reference redirect limit exceeded")
			}
			if !resolver.isTrustedURL(request.URL) {
				return fmt.Errorf("remote reference redirect %q leaves the OpenAPI input origin", request.URL)
			}
			return nil
		},
	}
}

func (r *remoteReferenceResolver) fetch(rawURL string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), remoteReferenceTimeout)
	defer cancel()
	u, err := r.validateURLSyntax(rawURL)
	if err != nil {
		return nil, err
	}
	if r.lock == nil {
		return nil, fmt.Errorf("remote reference %s requires --ref-lock for URL or stdin input", rawURL)
	}
	key := u.String()
	if r.offline {
		return r.fetchCached(key)
	}
	client := r.client
	if r.isTrustedURL(u) {
		client = r.trustedClient
	} else if err := r.validateHost(ctx, u.Hostname()); err != nil {
		return nil, fmt.Errorf("remote reference %q host: %w", rawURL, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	var requestConfig *httpInputConfig
	if r.isTrustedURL(u) && r.trustedConfig != nil {
		requestConfig = r.trustedConfig
		requestConfig.applyHeaders(request)
	} else {
		request.Header.Set("Accept", defaultOpenAPIInputAccept)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch remote reference %s: %w", u.String(), sanitizeHTTPClientError(err, requestConfig))
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		response.Body.Close()
		return nil, fmt.Errorf("fetch remote reference %s: unexpected HTTP status %s", u.String(), safeHTTPStatus(response.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, remoteReferenceMaxBytes+1))
	response.Body.Close()
	if err != nil {
		if requestConfig != nil && requestConfig.protected {
			return nil, fmt.Errorf("read protected remote reference %s response failed", u.String())
		}
		return nil, fmt.Errorf("read remote reference %s: %w", u.String(), err)
	}
	if len(body) > remoteReferenceMaxBytes {
		return nil, fmt.Errorf("remote reference %s exceeds %d byte limit", u.String(), remoteReferenceMaxBytes)
	}
	digest := sha256.Sum256(body)
	encoded := hex.EncodeToString(digest[:])
	if previous, ok := r.lock.References[key]; ok && previous != encoded && !r.update {
		return nil, fmt.Errorf("remote reference %s digest changed; run with --update-ref-lock to accept it", key)
	}
	if !r.update {
		if _, ok := r.lock.References[key]; !ok {
			return nil, fmt.Errorf("remote reference %s is missing from the reference lock; run with --update-ref-lock first", key)
		}
	} else {
		r.lock.References[key] = encoded
	}
	protectedCache := r.isTrustedURL(u) && r.trustedConfig != nil && r.trustedConfig.protected
	if err := r.cacheBody(encoded, body, protectedCache); err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	return response, nil
}

func (r *remoteReferenceResolver) fetchCached(key string) (*http.Response, error) {
	digest, ok := r.lock.References[key]
	if !ok {
		return nil, fmt.Errorf("offline remote reference %s is missing from the reference lock", key)
	}
	if len(digest) != sha256.Size*2 {
		return nil, fmt.Errorf("offline remote reference %s has an invalid lock digest", key)
	}
	protectedCache := false
	if u, err := url.Parse(key); err == nil {
		protectedCache = r.isTrustedURL(u) && r.trustedConfig != nil && r.trustedConfig.protected
	}
	cache, err := openReferenceCache(r.cache, protectedCache)
	if err != nil {
		return nil, err
	}
	defer cache.Close()
	data, err := readReferenceCacheEntry(cache, digest, protectedCache)
	if err != nil {
		return nil, fmt.Errorf("read cached remote reference %s: %w", key, err)
	}
	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != digest {
		return nil, fmt.Errorf("cached remote reference %s digest does not match the reference lock", key)
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: int64(len(data)),
	}, nil
}

func (r *remoteReferenceResolver) cacheBody(digest string, body []byte, protected bool) error {
	cache, err := openReferenceCache(r.cache, protected)
	if err != nil {
		return err
	}
	defer cache.Close()
	existing, err := readReferenceCacheEntry(cache, digest, protected)
	if err == nil {
		actual := sha256.Sum256(existing)
		if hex.EncodeToString(actual[:]) == digest {
			return nil
		}
		return fmt.Errorf("cached remote reference %s has unexpected content", digest)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read remote reference cache %s: %w", digest, err)
	}
	return writeReferenceCacheEntry(cache, digest, body, protected)
}

func openReferenceCache(path string, protected bool) (*os.Root, error) {
	if path == "" {
		return nil, errors.New("remote reference cache path is empty")
	}
	if protected && runtime.GOOS == "windows" {
		return nil, errors.New("protected remote reference caching is not supported on Windows because owner-only cache permissions cannot be enforced")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create remote reference cache parent: %w", err)
	}
	parent, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("open remote reference cache parent: %w", err)
	}
	defer parent.Close()
	name := filepath.Base(path)
	info, err := parent.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		mode := os.FileMode(0o755)
		if protected {
			mode = 0o700
		}
		if err := parent.Mkdir(name, mode); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create remote reference cache: %w", err)
		}
		info, err = parent.Lstat(name)
	}
	if err != nil {
		return nil, fmt.Errorf("inspect remote reference cache: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("remote reference cache %s must be a non-symlink directory", path)
	}
	cache, err := parent.OpenRoot(name)
	if err != nil {
		return nil, fmt.Errorf("open remote reference cache: %w", err)
	}
	directory, err := cache.Open(".")
	if err != nil {
		cache.Close()
		return nil, fmt.Errorf("inspect opened remote reference cache: %w", err)
	}
	openedInfo, err := directory.Stat()
	if err != nil || !openedInfo.IsDir() || !os.SameFile(info, openedInfo) {
		directory.Close()
		cache.Close()
		if err != nil {
			return nil, fmt.Errorf("inspect opened remote reference cache: %w", err)
		}
		return nil, fmt.Errorf("remote reference cache %s changed while opening", path)
	}
	if protected {
		err = directory.Chmod(0o700)
		if err != nil {
			directory.Close()
			cache.Close()
			return nil, fmt.Errorf("set protected remote reference cache mode: %w", err)
		}
	}
	if err := directory.Close(); err != nil {
		cache.Close()
		return nil, fmt.Errorf("close remote reference cache: %w", err)
	}
	return cache, nil
}

func readReferenceCacheEntry(cache *os.Root, digest string, protected bool) ([]byte, error) {
	return readVerifiedReferenceCacheEntry(cache, digest, protected, nil)
}

func readVerifiedReferenceCacheEntry(cache *os.Root, digest string, protected bool, expected os.FileInfo) ([]byte, error) {
	info, err := cache.Lstat(digest)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("remote reference cache entry %s must be a non-symlink regular file", digest)
	}
	entry, err := cache.Open(digest)
	if err != nil {
		return nil, err
	}
	defer entry.Close()
	if openedInfo, err := entry.Stat(); err != nil {
		return nil, err
	} else if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, fmt.Errorf("remote reference cache entry %s must be a regular file", digest)
	} else if expected != nil && !os.SameFile(expected, openedInfo) {
		return nil, fmt.Errorf("remote reference cache entry %s changed during publication", digest)
	}
	if protected {
		if err := entry.Chmod(0o600); err != nil {
			return nil, fmt.Errorf("set protected remote reference cache entry mode: %w", err)
		}
	}
	return io.ReadAll(entry)
}

func writeReferenceCacheEntry(cache *os.Root, digest string, body []byte, protected bool) error {
	return writeReferenceCacheEntryWithLink(cache, digest, body, protected, cache.Link)
}

func writeReferenceCacheEntryWithLink(cache *os.Root, digest string, body []byte, protected bool, link func(string, string) error) error {
	mode := os.FileMode(0o644)
	if protected {
		mode = 0o600
	}
	for attempt := 0; attempt < 16; attempt++ {
		temporaryName := fmt.Sprintf(".openapi-sdkgen-ref-%d-%d", os.Getpid(), time.Now().UnixNano())
		temporary, err := cache.OpenFile(temporaryName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("create remote reference cache entry: %w", err)
		}
		committed := false
		defer func() {
			if !committed {
				_ = cache.Remove(temporaryName)
			}
		}()
		if _, err := temporary.Write(body); err != nil {
			temporary.Close()
			return fmt.Errorf("write remote reference cache entry: %w", err)
		}
		if err := temporary.Chmod(mode); err != nil {
			temporary.Close()
			return fmt.Errorf("set remote reference cache mode: %w", err)
		}
		temporaryInfo, err := temporary.Stat()
		if err != nil {
			temporary.Close()
			return fmt.Errorf("inspect remote reference cache temporary entry: %w", err)
		}
		if err := temporary.Close(); err != nil {
			return fmt.Errorf("close remote reference cache entry: %w", err)
		}
		if err := publishReferenceCacheEntry(cache, temporaryName, digest, protected, temporaryInfo, link); err != nil {
			return err
		}
		committed = true
		return nil
	}
	return errors.New("create remote reference cache entry: temporary name collision limit exceeded")
}

func publishReferenceCacheEntry(cache *os.Root, temporaryName, digest string, protected bool, expected os.FileInfo, link func(string, string) error) error {
	linkErr := link(temporaryName, digest)
	switch {
	case linkErr == nil:
		if err := verifyReferenceCacheEntry(cache, digest, protected, expected); err != nil {
			_ = cache.Remove(digest)
			return err
		}
		if err := cache.Remove(temporaryName); err != nil {
			return fmt.Errorf("remove published remote reference cache temporary entry: %w", err)
		}
		return nil
	case errors.Is(linkErr, os.ErrExist):
		if err := verifyReferenceCacheEntry(cache, digest, protected, nil); err != nil {
			return err
		}
		if err := cache.Remove(temporaryName); err != nil {
			return fmt.Errorf("remove redundant remote reference cache temporary entry: %w", err)
		}
		return nil
	case protected:
		if errors.Is(linkErr, errors.ErrUnsupported) || errors.Is(linkErr, os.ErrPermission) {
			return errors.New("protected remote reference cache filesystem does not support atomic no-replace publication")
		}
		return fmt.Errorf("publish protected remote reference cache entry: %w", linkErr)
	default:
		if err := verifyReferenceCacheEntry(cache, digest, false, nil); err == nil {
			if err := cache.Remove(temporaryName); err != nil {
				return fmt.Errorf("remove redundant remote reference cache temporary entry: %w", err)
			}
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := cache.Rename(temporaryName, digest); err != nil {
			return fmt.Errorf("publish unprotected remote reference cache entry: %w", err)
		}
		if err := verifyReferenceCacheEntry(cache, digest, false, expected); err != nil {
			_ = cache.Remove(digest)
			return err
		}
		return nil
	}
}

func verifyReferenceCacheEntry(cache *os.Root, digest string, protected bool, expected os.FileInfo) error {
	existing, err := readVerifiedReferenceCacheEntry(cache, digest, protected, expected)
	if err != nil {
		return fmt.Errorf("read concurrently published remote reference cache entry: %w", err)
	}
	actual := sha256.Sum256(existing)
	if hex.EncodeToString(actual[:]) != digest {
		return fmt.Errorf("concurrently published remote reference cache entry %s has unexpected content", digest)
	}
	return nil
}

func sortedStrings(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
