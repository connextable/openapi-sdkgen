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
	origins map[string]struct{}
	lock    *referenceLock
	update  bool
	offline bool
	cache   string
	client  *http.Client
	lookup  hostLookup
	mu      sync.Mutex
	errs    []error
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

func newRemoteReferenceResolver(options CompileOptions, lock *referenceLock, cache string) (*remoteReferenceResolver, error) {
	origins := make(map[string]struct{}, len(options.RemoteRefAllowlist))
	for _, value := range options.RemoteRefAllowlist {
		origin, err := canonicalRemoteOrigin(value)
		if err != nil {
			return nil, fmt.Errorf("invalid --allow-remote-ref %q: %w", value, err)
		}
		origins[origin] = struct{}{}
	}
	if len(origins) == 0 {
		return nil, errors.New("remote references require at least one --allow-remote-ref HTTPS origin")
	}
	lookup := func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return net.DefaultResolver.LookupIPAddr(ctx, host)
	}
	resolver := &remoteReferenceResolver{origins: origins, lock: lock, update: options.UpdateRefLock, offline: options.Offline, cache: cache, lookup: lookup}
	resolver.client = secureRemoteHTTPClient(resolver)
	return resolver, nil
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
	if err != nil || u == nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("remote reference %q must be an unauthenticated HTTPS URL", rawURL)
	}
	origin := u.Scheme + "://" + strings.ToLower(u.Host)
	if _, ok := r.origins[origin]; !ok {
		return nil, fmt.Errorf("remote reference %q origin %q is not allowlisted", rawURL, origin)
	}
	return u, nil
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

func (r *remoteReferenceResolver) fetch(rawURL string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), remoteReferenceTimeout)
	defer cancel()
	u, err := r.validateURLSyntax(rawURL)
	if err != nil {
		return nil, err
	}
	key := u.String()
	if r.offline {
		return r.fetchCached(key)
	}
	if err := r.validateHost(ctx, u.Hostname()); err != nil {
		return nil, fmt.Errorf("remote reference %q host: %w", rawURL, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json, application/yaml, text/yaml, */*;q=0.1")
	response, err := r.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch remote reference %s: %w", u.String(), err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		response.Body.Close()
		return nil, fmt.Errorf("fetch remote reference %s: unexpected HTTP status %s", u.String(), response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, remoteReferenceMaxBytes+1))
	response.Body.Close()
	if err != nil {
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
	if err := r.cacheBody(encoded, body); err != nil {
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
	data, err := os.ReadFile(filepath.Join(r.cache, digest))
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

func (r *remoteReferenceResolver) cacheBody(digest string, body []byte) error {
	if r.cache == "" {
		return errors.New("remote reference cache path is empty")
	}
	if err := os.MkdirAll(r.cache, 0o755); err != nil {
		return fmt.Errorf("create remote reference cache: %w", err)
	}
	path := filepath.Join(r.cache, digest)
	if existing, err := os.ReadFile(path); err == nil {
		actual := sha256.Sum256(existing)
		if hex.EncodeToString(actual[:]) == digest {
			return nil
		}
		return fmt.Errorf("cached remote reference %s has unexpected content", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read remote reference cache %s: %w", path, err)
	}
	temporary, err := os.CreateTemp(r.cache, ".openapi-sdkgen-ref-*")
	if err != nil {
		return fmt.Errorf("create remote reference cache entry: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return fmt.Errorf("write remote reference cache entry: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return fmt.Errorf("set remote reference cache mode: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close remote reference cache entry: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish remote reference cache entry: %w", err)
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
