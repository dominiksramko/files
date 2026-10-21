package thumbs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"files/internal/testutil"
)

// addThumbnail records a thumbnail with the given status; for "done" both its
// files are written, last used lastUsed. It returns the square's path.
func addThumbnail(t *testing.T, service *Service, hash, status string, lastUsed time.Time) string {
	t.Helper()
	files := service.cachedFiles(hash + ".jpg")
	if _, err := service.db.Exec(`INSERT INTO thumbnails (content_hash, thumb_file, status) VALUES (?, ?, ?)`, hash, hash+".jpg", status); err != nil {
		t.Fatal(err)
	}
	if status == "done" {
		for _, path := range []string{files.square, files.preview} {
			if err := os.WriteFile(path, []byte("jpeg"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		os.Chtimes(files.square, lastUsed, lastUsed)
	}
	return files.square
}

// assertFilesGone fails unless neither thumbnail file of hash is left.
func assertFilesGone(t *testing.T, service *Service, hash string) {
	t.Helper()
	files := service.cachedFiles(hash + ".jpg")
	for _, path := range []string{files.square, files.preview} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s kept (stat error: %v)", filepath.Base(path), err)
		}
	}
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
	addThumbnail(t, service, "unused", "done", longAgo)
	addThumbnail(t, service, "failed", "error", longAgo)
	addThumbnail(t, service, "making", "pending", longAgo)
	addThumbnail(t, service, "lost", "done", time.Now())
	os.Remove(filepath.Join(service.cacheDir, "lost.jpg"))
	// Made before thumbnails came with a preview: made again when next listed.
	addThumbnail(t, service, "noPreview", "done", time.Now())
	os.Remove(filepath.Join(service.cacheDir, "noPreview-preview.jpg"))

	if err := service.RemoveUnused(); err != nil {
		t.Fatal(err)
	}
	for hash, kept := range map[string]bool{"recent": true, "making": true, "unused": false, "failed": false, "lost": false, "noPreview": false} {
		if thumbnailRecorded(t, service, hash) != kept {
			t.Errorf("%s: recorded = %v, want %v", hash, !kept, kept)
		}
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("recent thumbnail deleted: %v", err)
	}
	assertFilesGone(t, service, "unused")
	assertFilesGone(t, service, "lost")
	assertFilesGone(t, service, "noPreview")
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

func TestServedPreviewsKeepTheirThumbnail(t *testing.T) {
	service, err := New(testutil.OpenDB(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	longAgo := time.Now().Add(-unusedThumbAge - time.Hour)
	squarePath := addThumbnail(t, service, "photo", "done", longAgo)

	previewPath, ok := service.PreviewPathByHash("photo")
	if !ok || previewPath != service.cachedFiles("photo.jpg").preview {
		t.Fatalf("preview = %q, %v", previewPath, ok)
	}
	if info, _ := os.Stat(squarePath); time.Since(info.ModTime()) > time.Minute {
		t.Errorf("serving the preview didn't record the use: %v", info.ModTime())
	}

	addThumbnail(t, service, "making", "pending", time.Now())
	if _, ok := service.PreviewPathByHash("making"); ok {
		t.Error("preview of a thumbnail still being made served")
	}
	if _, ok := service.PreviewPathByHash("unknown"); ok {
		t.Error("preview of an unknown file served")
	}
}

func TestClearAllRemovesBothThumbnails(t *testing.T) {
	service, err := New(testutil.OpenDB(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	addThumbnail(t, service, "photo", "done", time.Now())
	if err := service.ClearAll(); err != nil {
		t.Fatal(err)
	}
	if thumbnailRecorded(t, service, "photo") {
		t.Error("record kept")
	}
	assertFilesGone(t, service, "photo")
}
