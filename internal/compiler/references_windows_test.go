//go:build windows

package sdkgen

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProtectedReferenceCacheFailsBeforePersistenceOnWindows(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache")
	if _, err := openReferenceCache(cachePath, true); err == nil || !strings.Contains(err.Error(), "not supported on Windows") {
		t.Fatalf("protected Windows cache error = %v", err)
	}
}
