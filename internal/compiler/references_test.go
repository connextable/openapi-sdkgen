package sdkgen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
