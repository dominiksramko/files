package thumbs

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const maxConcurrent = 4
const ThumbMaxDim = 320

// IsThumbnailable reports whether the given MIME type can be thumbnailed.
func IsThumbnailable(mime string) bool {
	if strings.HasPrefix(mime, "video/") {
		return true
	}
	switch mime {
	case "image/jpeg", "image/png", "image/gif",
		"image/webp", "image/bmp", "image/tiff",
		"image/svg+xml":
		return true
	}
	return isMagickThumbable(mime)
}

// isMagickThumbable reports whether the MIME type requires ImageMagick for thumbnailing.
func isMagickThumbable(mime string) bool {
	switch mime {
	case "image/avif", "image/heic", "image/heif", "image/jxl",
		"image/x-icon", "image/vnd.adobe.photoshop", "image/x-tga",
		"image/x-xcf", "image/vnd.radiance", "image/x-exr", "image/vnd.ms-dds":
		return true
	}
	return false
}

type task struct {
	absPath     string
	relPath     string // relative to user root — matches FileEntry.path
	folderAbs   string // for SSE broadcast key
	contentHash string // DB primary key
	thumbFile   string // contentHash + ".jpg", stored under cacheDir
	mime        string
}

// hashCacheEntry caches a content hash alongside the mtime it was computed for.
// If the file's mtime hasn't changed we can skip re-reading it.
type hashCacheEntry struct {
	mtime int64
	hash  string
}

// Service manages thumbnail generation, caching, and SSE broadcasting.
type Service struct {
	db        *sql.DB
	cacheDir  string
	hub       *hub
	sem       chan struct{}
	hashCache sync.Map // absPath → hashCacheEntry
}

// New creates a Service, ensures the cache directory exists, and resets any
// thumbnails that were left in a 'pending' state from a previous run.
func New(db *sql.DB, cacheDir string) (*Service, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create thumb cache dir: %w", err)
	}
	// Pending entries from a previous server run will never complete — reset them
	// so the next folder visit re-queues them.
	if _, err := db.Exec(`DELETE FROM thumbnails WHERE status = 'pending'`); err != nil {
		return nil, fmt.Errorf("reset pending thumbs: %w", err)
	}
	return &Service{
		db:       db,
		cacheDir: cacheDir,
		hub:      newHub(),
		sem:      make(chan struct{}, maxConcurrent),
	}, nil
}

// Hub exposes the internal hub so the HTTP handler can subscribe to events.
func (s *Service) Hub() *hub { return s.hub }

// cachedHash returns the content hash for absPath, using a sync.Map cache keyed
// by (absPath, mtime) to avoid re-reading files that haven't changed.
func (s *Service) cachedHash(absPath string, mtime int64) (string, error) {
	if v, ok := s.hashCache.Load(absPath); ok {
		if e := v.(hashCacheEntry); e.mtime == mtime {
			return e.hash, nil
		}
	}
	hash, err := FileContentHash(absPath)
	if err != nil {
		return "", err
	}
	s.hashCache.Store(absPath, hashCacheEntry{mtime: mtime, hash: hash})
	return hash, nil
}

// HashIfReady returns the content hash for the file at absPath if a finished
// thumbnail exists for it, otherwise returns ("", false). It is a pure read —
// it never enqueues generation. mtime is used only as a hint to skip
// re-hashing unchanged files.
func (s *Service) HashIfReady(absPath string, mtime int64) (string, bool) {
	hash, err := s.cachedHash(absPath, mtime)
	if err != nil {
		return "", false
	}
	var status string
	err = s.db.QueryRow(
		`SELECT status FROM thumbnails WHERE content_hash = ?`, hash,
	).Scan(&status)
	if err != nil || status != "done" {
		return "", false
	}
	return hash, true
}

// CheckAndEnqueue ensures a thumbnail is queued for background generation when
// one is missing or the file's content has changed. Already-done or in-progress
// thumbnails are left untouched.
// mtime is used only as a hint to skip re-hashing unchanged files.
func (s *Service) CheckAndEnqueue(absPath, relPath, folderAbs, mime string, mtime int64) {
	hash, err := s.cachedHash(absPath, mtime)
	if err != nil {
		return
	}
	thumbFile := hash + ".jpg"

	var status string
	err = s.db.QueryRow(
		`SELECT status FROM thumbnails WHERE content_hash = ?`, hash,
	).Scan(&status)

	switch {
	case err == sql.ErrNoRows:
		s.db.Exec(
			`INSERT INTO thumbnails (content_hash, thumb_file, status) VALUES (?, ?, 'pending')`,
			hash, thumbFile,
		)
		s.enqueue(task{absPath, relPath, folderAbs, hash, thumbFile, mime})
	case err != nil:
		// DB error — skip silently.
	case status == "error":
		// Previous attempt failed; retry.
		s.db.Exec(`UPDATE thumbnails SET status = 'pending' WHERE content_hash = ?`, hash)
		s.enqueue(task{absPath, relPath, folderAbs, hash, thumbFile, mime})
	// status == "done" or "pending": nothing to do.
	}
}

// ThumbPathByHash returns the absolute filesystem path to a ready thumbnail
// looked up directly by its content hash. Returns ("", false) if not ready.
func (s *Service) ThumbPathByHash(hash string) (string, bool) {
	var thumbFile, status string
	err := s.db.QueryRow(
		`SELECT thumb_file, status FROM thumbnails WHERE content_hash = ?`, hash,
	).Scan(&thumbFile, &status)
	if err != nil || status != "done" {
		return "", false
	}
	return filepath.Join(s.cacheDir, thumbFile), true
}

func (s *Service) enqueue(t task) {
	go func() {
		s.sem <- struct{}{} // acquire slot
		defer func() { <-s.sem }()
		s.process(t)
	}()
}

func (s *Service) process(t task) {
	thumbPath := filepath.Join(s.cacheDir, t.thumbFile)
	var err error
	switch {
	case strings.HasPrefix(t.mime, "video/"):
		err = generateVideoThumb(t.absPath, thumbPath, ThumbMaxDim)
	case t.mime == "image/svg+xml":
		err = generateSVGThumb(t.absPath, thumbPath, ThumbMaxDim)
	case isMagickThumbable(t.mime):
		err = generateMagickThumb(t.absPath, thumbPath, ThumbMaxDim)
	default:
		err = generateThumb(t.absPath, thumbPath, ThumbMaxDim)
	}

	status := "done"
	if err != nil {
		status = "error"
		log.Printf("thumbs: %s: %v", t.absPath, err)
	}

	s.db.Exec(
		`UPDATE thumbnails SET status = ? WHERE content_hash = ?`,
		status, t.contentHash,
	)

	if status == "done" {
		s.hub.Broadcast(t.folderAbs, t.relPath, t.contentHash)
	}
}

// ClearAll deletes all thumbnail records from the database and removes all
// physical thumbnail files from the cache directory.
func (s *Service) ClearAll() error {
	rows, err := s.db.Query(`SELECT thumb_file FROM thumbnails`)
	if err != nil {
		return fmt.Errorf("query thumbnails: %w", err)
	}
	defer rows.Close()

	var thumbFiles []string
	for rows.Next() {
		var thumbFile string
		if err := rows.Scan(&thumbFile); err != nil {
			continue
		}
		thumbFiles = append(thumbFiles, thumbFile)
	}

	if _, err := s.db.Exec(`DELETE FROM thumbnails`); err != nil {
		return fmt.Errorf("delete thumbnail records: %w", err)
	}

	deleted := 0
	for _, thumbFile := range thumbFiles {
		thumbPath := filepath.Join(s.cacheDir, thumbFile)
		if err := os.Remove(thumbPath); err != nil {
			if !os.IsNotExist(err) {
				log.Printf("thumbs: failed to delete %s: %v", thumbPath, err)
			}
		} else {
			deleted++
		}
	}

	log.Printf("thumbs: cleared %d records, deleted %d files", len(thumbFiles), deleted)
	return nil
}
