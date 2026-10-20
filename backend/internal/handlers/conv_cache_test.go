package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConvCacheIsTrimmedToItsSizeLimit(t *testing.T) {
	cache, err := newConvCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cache.maxBytes.Store(150)
	now := time.Now()
	for name, written := range map[string]time.Time{"old.jpg": now.Add(-2 * time.Hour), "new.jpg": now.Add(-time.Hour)} {
		path := filepath.Join(cache.dir, name)
		if err := os.WriteFile(path, []byte(strings.Repeat("x", 100)), 0o644); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(path, written, written)
	}

	cache.cleanup()
	assertMissing(t, filepath.Join(cache.dir, "old.jpg"))
	if _, err := os.Stat(filepath.Join(cache.dir, "new.jpg")); err != nil {
		t.Errorf("newest conversion removed: %v", err)
	}
}

func TestSetConvCacheMaxBytes(t *testing.T) {
	handler := NewFilesHandler(nil, t.TempDir())
	handler.SetConvCacheMaxBytes(1234)
	if got := handler.convCache.maxBytes.Load(); got != 1234 {
		t.Errorf("limit = %d, want 1234", got)
	}
	NewFilesHandler(nil, "").SetConvCacheMaxBytes(1) // no cache: nothing to limit, no crash
}
