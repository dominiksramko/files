package thumbs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"files/internal/testutil"
)

// addThumbnail records a thumbnail with the given status; for "done" its file
// is written and last used lastUsed.
func addThumbnail(t *testing.T, service *Service, hash, status string, lastUsed time.Time) string {
	t.Helper()
	thumbPath := filepath.Join(service.cacheDir, hash+".jpg")
	if _, err := service.db.Exec(`INSERT INTO thumbnails (content_hash, thumb_file, status) VALUES (?, ?, ?)`, hash, hash+".jpg", status); err != nil {
		t.Fatal(err)
	}
	if status == "done" {
		if err := os.WriteFile(thumbPath, []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(thumbPath, lastUsed, lastUsed)
	}
	return thumbPath
}

func thumbnailRecorded(t *testing.T, service *Service, hash string) bool {
	t.Helper()
	var count int
	service.db.QueryRow(`SELECT COUNT(*) FROM thumbnails WHERE content_hash = ?`, hash).Scan(&count)
	return count == 1
}

func TestRemoveUnused(t *testing.T) {
	service, err := New(testutil.OpenDB(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	longAgo := time.Now().Add(-unusedThumbAge - time.Hour)
	recent := addThumbnail(t, service, "recent", "done", time.Now())
	unused := addThumbnail(t, service, "unused", "done", longAgo)
	addThumbnail(t, service, "failed", "error", longAgo)
	addThumbnail(t, service, "making", "pending", longAgo)
	addThumbnail(t, service, "lost", "done", time.Now())
	os.Remove(filepath.Join(service.cacheDir, "lost.jpg"))

	if err := service.RemoveUnused(); err != nil {
		t.Fatal(err)
	}
	for hash, kept := range map[string]bool{"recent": true, "making": true, "unused": false, "failed": false, "lost": false} {
		if thumbnailRecorded(t, service, hash) != kept {
			t.Errorf("%s: recorded = %v, want %v", hash, !kept, kept)
		}
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("recent thumbnail deleted: %v", err)
	}
	if _, err := os.Stat(unused); !os.IsNotExist(err) {
		t.Errorf("unused thumbnail kept (stat error: %v)", err)
	}
}

func TestShownThumbnailsAreKept(t *testing.T) {
	service, err := New(testutil.OpenDB(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "photo.jpg")
	os.WriteFile(source, []byte("a photo"), 0o644)
	hash, err := FileContentHash(source)
	if err != nil {
		t.Fatal(err)
	}
	longAgo := time.Now().Add(-unusedThumbAge - time.Hour)
	thumbPath := addThumbnail(t, service, hash, "done", longAgo)

	// Listing the folder the file is in counts as showing its thumbnail.
	if _, ready := service.HashIfReady(source, 1); !ready {
		t.Fatal("thumbnail not ready")
	}
	if info, _ := os.Stat(thumbPath); time.Since(info.ModTime()) > time.Minute {
		t.Errorf("last use not recorded: %v", info.ModTime())
	}

	// Serving it does too, but at most once a day.
	os.Chtimes(thumbPath, longAgo, longAgo)
	service.ThumbPathByHash(hash)
	if info, _ := os.Stat(thumbPath); !info.ModTime().Equal(longAgo) {
		t.Errorf("use recorded again within a day: %v", info.ModTime())
	}
	service.marked.Clear()
	if _, ok := service.ThumbPathByHash(hash); !ok {
		t.Fatal("thumbnail not served")
	}
	if err := service.RemoveUnused(); err != nil {
		t.Fatal(err)
	}
	if !thumbnailRecorded(t, service, hash) {
		t.Error("a thumbnail just served was removed")
	}
}

func TestRemoveUnusedForgetsDeletedFiles(t *testing.T) {
	service, err := New(testutil.OpenDB(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	kept, deleted := filepath.Join(dir, "kept.jpg"), filepath.Join(dir, "deleted.jpg")
	for _, path := range []string{kept, deleted} {
		os.WriteFile(path, []byte(path), 0o644)
		if _, err := service.CachedHash(path, 1); err != nil {
			t.Fatal(err)
		}
	}
	os.Remove(deleted)

	if err := service.RemoveUnused(); err != nil {
		t.Fatal(err)
	}
	if _, ok := service.hashCache.Load(kept); !ok {
		t.Error("hash of an existing file forgotten")
	}
	if _, ok := service.hashCache.Load(deleted); ok {
		t.Error("hash of a deleted file kept")
	}
}
