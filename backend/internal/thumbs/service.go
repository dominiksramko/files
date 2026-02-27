package thumbs

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const maxConcurrent = 4
const ThumbMaxDim = 320

// IsThumbnailable reports whether the given MIME type can be thumbnailed.
// Videos use FFmpeg; images use the Go image package.
// SVG is excluded (needs a full SVG renderer).
func IsThumbnailable(mime string) bool {
	if strings.HasPrefix(mime, "video/") {
		return true
	}
	switch mime {
	case "image/jpeg", "image/png", "image/gif",
		"image/webp", "image/bmp", "image/tiff":
		return true
	}
	return false
}

type task struct {
	absPath   string
	relPath   string // relative to user root — matches FileEntry.path
	folderAbs string // for SSE broadcast key
	mtime     int64
	thumbFile string // filename only (no dir), stored under cacheDir
	mime      string // needed to choose the right generator
}

// Service manages thumbnail generation, caching, and SSE broadcasting.
type Service struct {
	db       *sql.DB
	cacheDir string
	hub      *hub
	sem      chan struct{}
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

// thumbFilename produces a deterministic filename from absPath + mtime.
func (s *Service) thumbFilename(absPath string, mtime int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", absPath, mtime)))
	return fmt.Sprintf("%x", h[:12]) + ".jpg"
}

// CheckAndEnqueue checks the thumbnail status for a file.
// If a thumbnail is missing or stale it is enqueued for background generation.
// Returns true only when a ready thumbnail already exists.
func (s *Service) CheckAndEnqueue(absPath, relPath, folderAbs, mime string, mtime int64) bool {
	var status string
	var storedMtime int64
	var thumbFile string

	err := s.db.QueryRow(
		`SELECT status, file_mtime, thumb_file FROM thumbnails WHERE abs_path = ?`,
		absPath,
	).Scan(&status, &storedMtime, &thumbFile)

	if err == sql.ErrNoRows {
		thumbFile = s.thumbFilename(absPath, mtime)
		s.db.Exec(
			`INSERT INTO thumbnails (abs_path, file_mtime, thumb_file, status) VALUES (?, ?, ?, 'pending')`,
			absPath, mtime, thumbFile,
		)
		s.enqueue(task{absPath, relPath, folderAbs, mtime, thumbFile, mime})
		return false
	}
	if err != nil {
		return false
	}

	// File has changed — invalidate and re-generate.
	if storedMtime != mtime {
		thumbFile = s.thumbFilename(absPath, mtime)
		s.db.Exec(
			`UPDATE thumbnails SET file_mtime = ?, thumb_file = ?, status = 'pending' WHERE abs_path = ?`,
			mtime, thumbFile, absPath,
		)
		s.enqueue(task{absPath, relPath, folderAbs, mtime, thumbFile, mime})
		return false
	}

	return status == "done"
}

// ThumbPath returns the absolute filesystem path to a ready thumbnail.
// Returns ("", false) if the thumbnail is not ready.
func (s *Service) ThumbPath(absPath string) (string, bool) {
	var thumbFile, status string
	err := s.db.QueryRow(
		`SELECT thumb_file, status FROM thumbnails WHERE abs_path = ?`,
		absPath,
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
	if strings.HasPrefix(t.mime, "video/") {
		err = generateVideoThumb(t.absPath, thumbPath, ThumbMaxDim)
	} else {
		err = generateThumb(t.absPath, thumbPath, ThumbMaxDim)
	}

	status := "done"
	if err != nil {
		status = "error"
		log.Printf("thumbs: %s: %v", t.absPath, err)
	}

	// Use mtime guard so a concurrent file change doesn't clobber a new entry.
	s.db.Exec(
		`UPDATE thumbnails SET status = ? WHERE abs_path = ? AND file_mtime = ?`,
		status, t.absPath, t.mtime,
	)

	if status == "done" {
		s.hub.Broadcast(t.folderAbs, t.relPath)
	}
}

// ClearAll deletes all thumbnail records from the database and removes all
// physical thumbnail files from the cache directory.
func (s *Service) ClearAll() error {
	// Get all thumbnail files from DB before deleting records
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

	// Delete all DB records
	if _, err := s.db.Exec(`DELETE FROM thumbnails`); err != nil {
		return fmt.Errorf("delete thumbnail records: %w", err)
	}

	// Delete physical files
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
