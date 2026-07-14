package sdkgen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRemoteReferenceResolverRequiresExactHTTPSOrigins(t *testing.T) {
	for _, origin := range []string{"http://schemas.example.test", "https://schemas.example.test/path", "https://user@schemas.example.test", "https://schemas.example.test?x=1"} {
		t.Run(origin, func(t *testing.T) {
			if _, err := canonicalRemoteOrigin(origin); err == nil {
				t.Fatalf("origin %q accepted", origin)
			}
		})
	}
	if got, err := canonicalRemoteOrigin("https://SCHEMAS.example.test:443"); err != nil || got != "https://schemas.example.test:443" {
		t.Fatalf("canonical origin = %q, %v", got, err)
	}
}

func TestRemoteReferenceResolverRejectsNonPublicDNS(t *testing.T) {
	resolver := &remoteReferenceResolver{
		origins: map[string]struct{}{"https://schemas.example.test": {}},
		lookup: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		},
	}
	if _, err := resolver.validateURL(context.Background(), "https://schemas.example.test/schema.json"); err == nil || !strings.Contains(err.Error(), "not a public address") {
		t.Fatalf("validateURL error = %v", err)
	}
}

func TestRemoteReferenceResolverLocksFetchedContent(t *testing.T) {
	server := httptest.NewTLSServer(httpHandler(`{"Thing":{"type":"string"}}`))
	defer server.Close()
	body := `{"Thing":{"type":"string"}}`
	digest := sha256.Sum256([]byte(body))
	resolver := &remoteReferenceResolver{
		origins: map[string]struct{}{server.URL: {}},
		lock:    &referenceLock{Version: 1, References: map[string]string{}, Extensions: map[string]string{}},
		update:  true,
		cache:   t.TempDir(),
		client:  server.Client(),
		lookup: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		},
	}
	response, err := resolver.fetch(server.URL + "/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	key := server.URL + "/schema.json"
	if got := resolver.lock.References[key]; got != hex.EncodeToString(digest[:]) {
		t.Fatalf("lock digest = %q", got)
	}
	resolver.update = false
	if _, err := resolver.fetch(key); err != nil {
		t.Fatal(err)
	}
	resolver.offline = true
	if _, err := resolver.fetch(key); err != nil {
		t.Fatal(err)
	}
}

func TestProtectedTLSAndClientIdentityDoNotExtendToCrossOriginReferences(t *testing.T) {
	const secret = "credential-sentinel"
	t.Setenv("SDKGEN_HTTP_TOKEN", secret)
	root := httptest.NewTLSServer(http.NotFoundHandler())
	defer root.Close()
	caPath, certPath, keyPath := writeTLSServerCredentials(t, root)
	config, err := configureHTTPInput(CompileOptions{
		HTTPHeaderEnv: []string{"Authorization=SDKGEN_HTTP_TOKEN"},
		TLSClientCert: certPath,
		TLSClientKey:  keyPath,
		TLSCAFile:     caPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	var requests int
	var peerCertificates int
	crossOrigin := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		peerCertificates = len(request.TLS.PeerCertificates)
		if got := request.Header.Get("Authorization"); got != "" {
			t.Errorf("cross-origin Authorization = %q", got)
		}
		_, _ = response.Write([]byte(`{"Thing":{"type":"string"}}`))
	}))
	crossOrigin.TLS = &tls.Config{
		Certificates: append([]tls.Certificate(nil), root.TLS.Certificates...),
		ClientAuth:   tls.RequestClientCert,
	}
	crossOrigin.StartTLS()
	defer crossOrigin.Close()
	trustedBase, err := url.Parse(root.URL + "/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	resolverOptions := CompileOptions{
		RemoteRefAllowlist: []string{crossOrigin.URL},
		remoteReferenceLookup: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		},
	}
	resolver, err := newRemoteReferenceResolver(resolverOptions, &referenceLock{Version: 1, References: map[string]string{}, Extensions: map[string]string{}}, t.TempDir(), trustedBase, config)
	if err != nil {
		t.Fatal(err)
	}
	transport := resolver.client.Transport.(*http.Transport)
	dialer := &net.Dialer{}
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, crossOrigin.Listener.Addr().String())
	}
	key := crossOrigin.URL + "/schema.json"
	if _, err := resolver.fetch(key); err == nil {
		t.Fatal("cross-origin reference trusted the protected input CA")
	}
	if requests != 0 {
		t.Fatalf("untrusted cross-origin requests = %d", requests)
	}
	resolver, err = newRemoteReferenceResolver(resolverOptions, &referenceLock{Version: 1, References: map[string]string{}, Extensions: map[string]string{}}, t.TempDir(), trustedBase, config)
	if err != nil {
		t.Fatal(err)
	}
	transport = resolver.client.Transport.(*http.Transport)
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, crossOrigin.Listener.Addr().String())
	}
	crossTransport := crossOrigin.Client().Transport.(*http.Transport)
	tlsConfig := transport.TLSClientConfig.Clone()
	tlsConfig.RootCAs = crossTransport.TLSClientConfig.RootCAs
	transport.TLSClientConfig = tlsConfig
	resolver.update = true
	response, err := resolver.fetch(key)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if requests != 1 {
		t.Fatalf("trusted test cross-origin requests = %d", requests)
	}
	if peerCertificates != 0 {
		t.Fatalf("cross-origin client certificates = %d", peerCertificates)
	}
}

func TestProtectedReferenceErrorsDoNotReflectHeaderValues(t *testing.T) {
	const secret = "credential-sentinel"
	t.Setenv("SDKGEN_HTTP_TOKEN", secret)
	crossOriginCalled := false
	crossOrigin := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		crossOriginCalled = true
	}))
	defer crossOrigin.Close()
	root := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != secret {
			t.Errorf("Authorization = %q", got)
		}
		switch request.URL.Path {
		case "/redirect.json":
			http.Redirect(response, request, crossOrigin.URL+"/?echo="+secret, http.StatusFound)
		case "/malformed-redirect.json":
			response.Header().Set("Location", "%zz"+secret)
			response.WriteHeader(http.StatusFound)
		case "/status.json":
			connection, buffer, err := response.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			defer connection.Close()
			_, _ = buffer.WriteString("HTTP/1.1 499 " + secret + "\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
			_ = buffer.Flush()
		}
	}))
	defer root.Close()
	config, err := configureHTTPInput(CompileOptions{HTTPHeaderEnv: []string{"Authorization=SDKGEN_HTTP_TOKEN"}})
	if err != nil {
		t.Fatal(err)
	}
	trustedBase, err := url.Parse(root.URL + "/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := newRemoteReferenceResolver(CompileOptions{}, &referenceLock{Version: 1, References: map[string]string{}, Extensions: map[string]string{}}, t.TempDir(), trustedBase, config)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/redirect.json", "/malformed-redirect.json", "/status.json"} {
		if _, err := resolver.fetch(root.URL + path); err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("protected reference %s error = %v", path, err)
		}
	}
	if crossOriginCalled {
		t.Fatal("protected reference redirect reached cross origin")
	}
}

func TestProtectedReferenceCacheUsesPrivatePermissionsAndNarrowsExistingEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows fails protected cache access before persistence")
	}
	cachePath := filepath.Join(t.TempDir(), "cache")
	body := []byte(`{"Thing":{"type":"string"}}`)
	digestBytes := sha256.Sum256(body)
	digest := hex.EncodeToString(digestBytes[:])
	resolver := &remoteReferenceResolver{cache: cachePath}
	if err := resolver.cacheBody(digest, body, true); err != nil {
		t.Fatal(err)
	}
	assertFileMode(t, cachePath, 0o700)
	entryPath := filepath.Join(cachePath, digest)
	assertFileMode(t, entryPath, 0o600)
	if err := os.Chmod(cachePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(entryPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := resolver.cacheBody(digest, body, true); err != nil {
		t.Fatal(err)
	}
	assertFileMode(t, cachePath, 0o700)
	assertFileMode(t, entryPath, 0o600)
}

func TestReferenceCachePublicationDoesNotReplaceProtectedEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows fails protected cache access before persistence")
	}
	cachePath := filepath.Join(t.TempDir(), "cache")
	body := []byte(`{"Thing":{"type":"string"}}`)
	digestBytes := sha256.Sum256(body)
	digest := hex.EncodeToString(digestBytes[:])
	cache, err := openReferenceCache(cachePath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	if err := writeReferenceCacheEntry(cache, digest, body, true); err != nil {
		t.Fatal(err)
	}
	if err := writeReferenceCacheEntry(cache, digest, body, false); err != nil {
		t.Fatal(err)
	}
	assertFileMode(t, filepath.Join(cachePath, digest), 0o600)
	if err := cache.Remove(digest); err != nil {
		t.Fatal(err)
	}
	if err := writeReferenceCacheEntry(cache, digest, body, false); err != nil {
		t.Fatal(err)
	}
	assertFileMode(t, filepath.Join(cachePath, digest), 0o644)
	if err := writeReferenceCacheEntry(cache, digest, body, true); err != nil {
		t.Fatal(err)
	}
	assertFileMode(t, filepath.Join(cachePath, digest), 0o600)
}

func TestReferenceCachePublicationHandlesUnsupportedHardLinks(t *testing.T) {
	body := []byte(`{"Thing":{"type":"string"}}`)
	digestBytes := sha256.Sum256(body)
	digest := hex.EncodeToString(digestBytes[:])
	unsupportedLink := func(string, string) error { return errors.ErrUnsupported }

	unprotectedPath := filepath.Join(t.TempDir(), "unprotected")
	unprotected, err := openReferenceCache(unprotectedPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeReferenceCacheEntryWithLink(unprotected, digest, body, false, unsupportedLink); err != nil {
		unprotected.Close()
		t.Fatal(err)
	}
	if err := unprotected.Close(); err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, filepath.Join(unprotectedPath, digest), body)
	if runtime.GOOS != "windows" {
		assertFileMode(t, filepath.Join(unprotectedPath, digest), 0o644)
	}
	if err := os.Remove(filepath.Join(unprotectedPath, digest)); err != nil {
		t.Fatal(err)
	}
	permissionLink := func(oldName, newName string) error {
		return &os.LinkError{Op: "link", Old: oldName, New: newName, Err: os.ErrPermission}
	}
	unprotected, err = openReferenceCache(unprotectedPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeReferenceCacheEntryWithLink(unprotected, digest, body, false, permissionLink); err != nil {
		unprotected.Close()
		t.Fatal(err)
	}
	if err := unprotected.Close(); err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, filepath.Join(unprotectedPath, digest), body)
	if runtime.GOOS != "windows" {
		assertFileMode(t, filepath.Join(unprotectedPath, digest), 0o644)
	}

	if runtime.GOOS != "windows" {
		protectedPath := filepath.Join(t.TempDir(), "protected")
		protected, err := openReferenceCache(protectedPath, true)
		if err != nil {
			t.Fatal(err)
		}
		err = writeReferenceCacheEntryWithLink(protected, digest, body, true, unsupportedLink)
		if closeErr := protected.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if err == nil || !strings.Contains(err.Error(), "does not support atomic no-replace publication") {
			t.Fatalf("protected unsupported-link error = %v", err)
		}
		if _, statErr := os.Lstat(filepath.Join(protectedPath, digest)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("protected unsupported-link digest entry = %v", statErr)
		}
	}
}

func TestReferenceCachePublicationRejectsTemporaryEntryReplacement(t *testing.T) {
	body := []byte(`{"Thing":{"type":"string"}}`)
	digestBytes := sha256.Sum256(body)
	digest := hex.EncodeToString(digestBytes[:])

	t.Run("hard-link symlink replacement", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows fails protected cache access before publication")
		}
		directory := t.TempDir()
		cachePath := filepath.Join(directory, "cache")
		cache, err := openReferenceCache(cachePath, true)
		if err != nil {
			t.Fatal(err)
		}
		defer cache.Close()
		external := filepath.Join(directory, "external")
		if err := os.WriteFile(external, []byte("outside"), 0o640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(external, 0o640); err != nil {
			t.Fatal(err)
		}
		swapLink := func(oldName, newName string) error {
			if err := cache.Remove(oldName); err != nil {
				return err
			}
			if err := cache.Symlink(external, oldName); err != nil {
				return err
			}
			return cache.Link(oldName, newName)
		}
		if err := writeReferenceCacheEntryWithLink(cache, digest, body, true, swapLink); err == nil {
			t.Fatal("symlink replacement was published")
		}
		if _, err := os.Lstat(filepath.Join(cachePath, digest)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unsafe linked digest entry = %v", err)
		}
		contents, err := os.ReadFile(external)
		if err != nil || string(contents) != "outside" {
			t.Fatalf("external content = %q, %v", contents, err)
		}
		assertFileMode(t, external, 0o640)
	})

	t.Run("rename inode replacement", func(t *testing.T) {
		cachePath := filepath.Join(t.TempDir(), "cache")
		cache, err := openReferenceCache(cachePath, false)
		if err != nil {
			t.Fatal(err)
		}
		defer cache.Close()
		swapThenFallback := func(oldName, _ string) error {
			if err := cache.Remove(oldName); err != nil {
				return err
			}
			replacement, err := cache.OpenFile(oldName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				return err
			}
			if _, err := replacement.Write(body); err != nil {
				replacement.Close()
				return err
			}
			if err := replacement.Close(); err != nil {
				return err
			}
			return errors.ErrUnsupported
		}
		if err := writeReferenceCacheEntryWithLink(cache, digest, body, false, swapThenFallback); err == nil || !strings.Contains(err.Error(), "changed during publication") {
			t.Fatalf("rename replacement error = %v", err)
		}
		if _, err := os.Lstat(filepath.Join(cachePath, digest)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unsafe renamed digest entry = %v", err)
		}
	})
}

func TestUnprotectedReferenceCachePreservesExistingPermissions(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache")
	body := []byte(`{"Thing":{"type":"string"}}`)
	digestBytes := sha256.Sum256(body)
	digest := hex.EncodeToString(digestBytes[:])
	if err := os.Mkdir(cachePath, 0o700); err != nil {
		t.Fatal(err)
	}
	entryPath := filepath.Join(cachePath, digest)
	if err := os.WriteFile(entryPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cachePath, 0o711); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(entryPath, 0o640); err != nil {
		t.Fatal(err)
	}
	directoryInfoBefore, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	entryInfoBefore, err := os.Stat(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	resolver := &remoteReferenceResolver{cache: cachePath}
	if err := resolver.cacheBody(digest, body, false); err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, entryPath, body)
	if runtime.GOOS == "windows" {
		directoryInfoAfter, err := os.Stat(cachePath)
		if err != nil {
			t.Fatal(err)
		}
		entryInfoAfter, err := os.Stat(entryPath)
		if err != nil {
			t.Fatal(err)
		}
		if directoryInfoAfter.Mode().Perm() != directoryInfoBefore.Mode().Perm() || entryInfoAfter.Mode().Perm() != entryInfoBefore.Mode().Perm() {
			t.Fatalf("unprotected Windows cache permissions changed: directory %04o -> %04o, entry %04o -> %04o", directoryInfoBefore.Mode().Perm(), directoryInfoAfter.Mode().Perm(), entryInfoBefore.Mode().Perm(), entryInfoAfter.Mode().Perm())
		}
	} else {
		assertFileMode(t, cachePath, 0o711)
		assertFileMode(t, entryPath, 0o640)
	}
}

func TestReferenceCacheRejectsSymlinkAndNonRegularPaths(t *testing.T) {
	body := []byte(`{"Thing":{"type":"string"}}`)
	digestBytes := sha256.Sum256(body)
	digest := hex.EncodeToString(digestBytes[:])

	t.Run("cache root symlink", func(t *testing.T) {
		directory := t.TempDir()
		external := filepath.Join(directory, "external")
		if err := os.Mkdir(external, 0o700); err != nil {
			t.Fatal(err)
		}
		cachePath := filepath.Join(directory, "cache")
		if err := os.Symlink(external, cachePath); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlinks unavailable: %v", err)
			}
			t.Fatal(err)
		}
		if _, err := openReferenceCache(cachePath, false); err == nil || !strings.Contains(err.Error(), "non-symlink directory") {
			t.Fatalf("cache symlink error = %v", err)
		}
	})

	t.Run("cache root regular file", func(t *testing.T) {
		cachePath := filepath.Join(t.TempDir(), "cache")
		if err := os.WriteFile(cachePath, []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := openReferenceCache(cachePath, false); err == nil || !strings.Contains(err.Error(), "non-symlink directory") {
			t.Fatalf("cache regular-file error = %v", err)
		}
	})

	t.Run("cache entry symlink", func(t *testing.T) {
		directory := t.TempDir()
		cachePath := filepath.Join(directory, "cache")
		if err := os.Mkdir(cachePath, 0o700); err != nil {
			t.Fatal(err)
		}
		outsideEntry := filepath.Join(directory, "outside-entry")
		if err := os.WriteFile(outsideEntry, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsideEntry, filepath.Join(cachePath, digest)); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlinks unavailable: %v", err)
			}
			t.Fatal(err)
		}
		resolver := &remoteReferenceResolver{cache: cachePath}
		if err := resolver.cacheBody(digest, body, false); err == nil || !strings.Contains(err.Error(), "non-symlink regular file") {
			t.Fatalf("entry symlink error = %v", err)
		}
	})

	t.Run("cache entry directory", func(t *testing.T) {
		cachePath := filepath.Join(t.TempDir(), "cache")
		if err := os.Mkdir(cachePath, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(cachePath, digest), 0o700); err != nil {
			t.Fatal(err)
		}
		resolver := &remoteReferenceResolver{cache: cachePath}
		if err := resolver.cacheBody(digest, body, false); err == nil || !strings.Contains(err.Error(), "non-symlink regular file") {
			t.Fatalf("entry directory error = %v", err)
		}
	})
}

func TestOfflineReferenceCacheRejectsUnsafePaths(t *testing.T) {
	body := []byte(`{"Thing":{"type":"string"}}`)
	digestBytes := sha256.Sum256(body)
	digest := hex.EncodeToString(digestBytes[:])
	key := "https://schemas.example.test/thing.json"
	lock := &referenceLock{Version: 1, References: map[string]string{key: digest}, Extensions: map[string]string{}}

	newResolver := func(cachePath string) *remoteReferenceResolver {
		return &remoteReferenceResolver{cache: cachePath, lock: lock}
	}

	t.Run("cache root symlink", func(t *testing.T) {
		directory := t.TempDir()
		external := filepath.Join(directory, "external")
		if err := os.Mkdir(external, 0o700); err != nil {
			t.Fatal(err)
		}
		cachePath := filepath.Join(directory, "cache")
		if err := os.Symlink(external, cachePath); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlinks unavailable: %v", err)
			}
			t.Fatal(err)
		}
		if _, err := newResolver(cachePath).fetchCached(key); err == nil || !strings.Contains(err.Error(), "non-symlink directory") {
			t.Fatalf("offline cache-root symlink error = %v", err)
		}
	})

	t.Run("cache root regular file", func(t *testing.T) {
		cachePath := filepath.Join(t.TempDir(), "cache")
		if err := os.WriteFile(cachePath, []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := newResolver(cachePath).fetchCached(key); err == nil || !strings.Contains(err.Error(), "non-symlink directory") {
			t.Fatalf("offline cache-root regular-file error = %v", err)
		}
	})

	t.Run("cache entry symlink", func(t *testing.T) {
		directory := t.TempDir()
		cachePath := filepath.Join(directory, "cache")
		if err := os.Mkdir(cachePath, 0o700); err != nil {
			t.Fatal(err)
		}
		outsideEntry := filepath.Join(directory, "outside-entry")
		if err := os.WriteFile(outsideEntry, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsideEntry, filepath.Join(cachePath, digest)); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlinks unavailable: %v", err)
			}
			t.Fatal(err)
		}
		if _, err := newResolver(cachePath).fetchCached(key); err == nil || !strings.Contains(err.Error(), "non-symlink regular file") {
			t.Fatalf("offline cache-entry symlink error = %v", err)
		}
	})

	t.Run("cache entry directory", func(t *testing.T) {
		cachePath := filepath.Join(t.TempDir(), "cache")
		if err := os.Mkdir(cachePath, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(cachePath, digest), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := newResolver(cachePath).fetchCached(key); err == nil || !strings.Contains(err.Error(), "non-symlink regular file") {
			t.Fatalf("offline cache-entry directory error = %v", err)
		}
	})
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode %s = %04o, want %04o", path, got, want)
	}
}

func assertFileContents(t *testing.T, path string, want []byte) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, want) {
		t.Fatalf("contents %s = %q, want %q", path, contents, want)
	}
}

func TestReferenceLockFailsClosedAndWritesAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "refs.lock")
	if _, err := loadReferenceLock(path, false); err == nil {
		t.Fatal("missing lock accepted without update mode")
	}
	lock, err := loadReferenceLock(path, true)
	if err != nil {
		t.Fatal(err)
	}
	lock.References["https://schemas.example.test/a.json"] = "abc"
	if err := writeReferenceLock(path, lock); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadReferenceLock(path, false)
	if err != nil || loaded.References["https://schemas.example.test/a.json"] != "abc" {
		t.Fatalf("loaded = %#v, %v", loaded, err)
	}
}

func TestCompileFileWithOptionsUsesLockedOfflineRemoteReference(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "openapi.json")
	url := "https://schemas.example.test/thing.json"
	document := `{"openapi":"3.1.0","info":{"title":"remote","version":"1"},"paths":{"/things":{"get":{"operationId":"listThings","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"` + url + `#/Thing"}}}}}}}}}`
	if err := os.WriteFile(input, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	remote := []byte(`{"Thing":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}`)
	digest := sha256.Sum256(remote)
	lock := &referenceLock{Version: 1, References: map[string]string{url: hex.EncodeToString(digest[:])}, Extensions: map[string]string{}}
	if err := writeReferenceLock(defaultReferenceLockPath(input), lock); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(directory, ".openapi-sdkgen-cache")
	if err := os.Mkdir(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, hex.EncodeToString(digest[:])), remote, 0o600); err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileFileWithOptions(input, CompileOptions{RemoteRefAllowlist: []string{"https://schemas.example.test"}, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Operations) != 1 || compiled.Operations[0].OperationID != "listThings" {
		t.Fatalf("operations = %#v", compiled.Operations)
	}
}

func TestCompileFileWithOptionsRejectsPrivateRemoteReference(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "openapi.json")
	document := `{"openapi":"3.1.0","info":{"title":"private","version":"1"},"paths":{"/things":{"get":{"operationId":"listThings","responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"https://127.0.0.1/schema.json#/Thing"}}}}}}}}}`
	if err := os.WriteFile(input, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := CompileFileWithOptions(input, CompileOptions{RemoteRefAllowlist: []string{"https://127.0.0.1"}, UpdateRefLock: true})
	if err == nil || !strings.Contains(err.Error(), "not a public address") {
		t.Fatalf("CompileFileWithOptions error = %v", err)
	}
}

func TestSchemaExtensionRequiresLockedTrustedExecutable(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "extension")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"vocabularies\":[\"https://example.test/vocab\"]}}'\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(script))
	manifest := filepath.Join(directory, "extension.json")
	manifestData := `{"version":1,"extensions":[{"vocabularies":["https://example.test/vocab"],"command":"extension","sha256":"` + hex.EncodeToString(digest[:]) + `"}]}`
	if err := os.WriteFile(manifest, []byte(manifestData), 0o600); err != nil {
		t.Fatal(err)
	}
	document := []byte(`{"openapi":"3.1.0","info":{"title":"extension","version":"1"},"paths":{},"components":{"schemas":{"Thing":{"$vocabulary":{"https://example.test/vocab":true}}}}}`)
	lock := &referenceLock{Version: 1, References: map[string]string{}, Extensions: map[string]string{}}
	options := CompileOptions{SchemaExtensionManifests: []string{manifest}, UpdateRefLock: true}
	if err := validateSchemaExtensions(document, options, lock); err != nil {
		t.Fatal(err)
	}
	options.UpdateRefLock = false
	if err := validateSchemaExtensions(document, options, lock); err != nil {
		t.Fatal(err)
	}
	if err := validateSchemaExtensions(document, CompileOptions{}, lock); err == nil || !strings.Contains(err.Error(), "no registered") {
		t.Fatalf("missing extension error = %v", err)
	}
}

func TestCompileFileLowersRequiredCustomVocabularyBeforeTargetGeneration(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "extension")
	script := `#!/bin/sh
read request
case "$request" in
  *'"method":"describe"'*) printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"vocabularies":["https://example.test/vocab"]}}' ;;
  *'"method":"lower"'*) printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"schema":{"type":"string","minLength":3}}}' ;;
  *) printf '%s\n' '{"jsonrpc":"2.0","id":1,"error":{"message":"unknown request"}}' ;;
esac
`
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(script))
	manifest := filepath.Join(directory, "extension.json")
	manifestData := `{"version":1,"extensions":[{"vocabularies":["https://example.test/vocab"],"command":"extension","sha256":"` + hex.EncodeToString(digest[:]) + `"}]}`
	if err := os.WriteFile(manifest, []byte(manifestData), 0o600); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(directory, "openapi.json")
	document := `{"openapi":"3.1.0","info":{"title":"extension","version":"1"},"paths":{"/widgets":{"post":{"operationId":"createWidget","requestBody":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Widget"}}}},"responses":{"204":{"description":"No content"}}}}},"components":{"schemas":{"Widget":{"$vocabulary":{"https://example.test/vocab":true},"x-example-assertion":"must-have-three"}}}}`
	if err := os.WriteFile(input, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileFileWithOptions(input, CompileOptions{SchemaExtensionManifests: []string{manifest}, UpdateRefLock: true})
	if err != nil {
		t.Fatal(err)
	}
	widget := compiled.ComponentSchemas["Widget"]
	minimum, minimumOK := widget["minLength"].(int)
	if widget["type"] != "string" || !minimumOK || minimum != 3 {
		t.Fatalf("lowered component schema = %#v", widget)
	}
	if _, exists := widget["$vocabulary"]; exists {
		t.Fatalf("custom vocabulary leaked after lowering: %#v", widget)
	}
}

func httpHandler(body string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(body))
	})
}
