package cachesize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeEntry writes size bytes at dir/name, last used age ago.
func makeEntry(t *testing.T, dir, name string, size int, age time.Duration) Entry {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o644); err != nil {
		t.Fatal(err)
	}
	return Entry{Path: path, Size: int64(size), LastUsed: time.Now().Add(-age)}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestTrimRemovesLeastRecentlyUsedFirst(t *testing.T) {
	dir := t.TempDir()
	oldest := makeEntry(t, dir, "oldest", 100, 3*time.Hour)
	older := makeEntry(t, dir, "older", 100, 2*time.Hour)
	recent := makeEntry(t, dir, "recent", 100, time.Hour)

	removed := Trim([]Entry{recent, oldest, older}, 150, nil)
	if len(removed) != 2 || exists(oldest.Path) || exists(older.Path) || !exists(recent.Path) {
		t.Errorf("removed %v; want the two oldest", removed)
	}
}

func TestTrimKeepsEntriesInUse(t *testing.T) {
	dir := t.TempDir()
	playing := makeEntry(t, dir, "playing", 100, 3*time.Hour)
	idle := makeEntry(t, dir, "idle", 100, time.Hour)

	Trim([]Entry{playing, idle}, 100, func(entry Entry) bool { return entry.Path == playing.Path })
	if !exists(playing.Path) || exists(idle.Path) {
		t.Error("want the entry in use kept and the idle one removed")
	}
}

func TestTrimUnderTheLimitOrWithoutOne(t *testing.T) {
	dir := t.TempDir()
	entry := makeEntry(t, dir, "small", 100, time.Hour)
	if removed := Trim([]Entry{entry}, 100, nil); len(removed) != 0 {
		t.Errorf("removed %v at exactly the limit", removed)
	}
	if removed := Trim([]Entry{entry}, 0, nil); len(removed) != 0 || !exists(entry.Path) {
		t.Errorf("removed %v with no limit", removed)
	}
}

func TestSize(t *testing.T) {
	dir := t.TempDir()
	makeEntry(t, dir, "a", 10, 0)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	makeEntry(t, filepath.Join(dir, "sub"), "b", 5, 0)
	if got := Size(dir); got != 15 {
		t.Errorf("Size(folder) = %d, want 15", got)
	}
	if got := Size(filepath.Join(dir, "a")); got != 10 {
		t.Errorf("Size(file) = %d, want 10", got)
	}
	if got := Size(filepath.Join(dir, "missing")); got != 0 {
		t.Errorf("Size(missing) = %d, want 0", got)
	}
}
