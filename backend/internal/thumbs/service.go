package thumbs

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxConcurrent = 4

// toolTimeout bounds the external tools making one thumbnail. A variable so
// tests can shorten it.
var toolTimeout = 2 * time.Minute

// Each thumbnailed file gets two: a square for file lists, and the whole
// picture within PreviewMaxDim, which the preview shows while the file itself
// loads. The preview's is stretched to the screen only briefly, so it's
// compressed harder.
const (
	ThumbMaxDim    = 320
	PreviewMaxDim  = 640
	thumbQuality   = 80
	previewQuality = 50
)

// unusedThumbAge: a thumbnail not shown for this long is deleted — its file
// was most likely deleted or changed. It's made again if it's needed.
const unusedThumbAge = 30 * 24 * time.Hour

// usedMarkInterval: a thumbnail's last use is recorded (in its file's
// modification time) at most this often.
const usedMarkInterval = 24 * time.Hour

// IsThumbnailable reports whether the given MIME type can be thumbnailed.
func IsThumbnailable(mime string) bool {
	if strings.HasPrefix(mime, "video/") {
		return true
	}
	switch mime {
	case "image/jpeg", "image/png", "image/gif",
		"image/webp", "image/bmp", "image/tiff",
		"image/svg+xml", "application/pdf":
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
	thumbFile   string // contentHash + ".jpg", stored under cacheDir (the preview's: previewFile(thumbFile))
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
	marked    sync.Map // thumbnail path → time.Time its use was last recorded
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
	service := &Service{
		db:       db,
		cacheDir: cacheDir,
		hub:      newHub(),
		sem:      make(chan struct{}, maxConcurrent),
	}
	go service.periodicCleanup()
	return service, nil
}

// Hub exposes the internal hub so the HTTP handler can subscribe to events.
func (s *Service) Hub() *hub { return s.hub }

// CachedHash returns the content hash for absPath, using a sync.Map cache keyed
// by (absPath, mtime) to avoid re-reading files that haven't changed.
func (s *Service) CachedHash(absPath string, mtime int64) (string, error) {
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
	hash, err := s.CachedHash(absPath, mtime)
	if err != nil {
		return "", false
	}
	if _, ok := s.readyFiles(hash); !ok {
		return "", false
	}
	return hash, true
}

// CheckAndEnqueue ensures a thumbnail is queued for background generation when
// one is missing or the file's content has changed. Duplicate files (same hash,
// different names) share one generation task; the frontend matches thumb events
// by content_hash across all entries.
func (s *Service) CheckAndEnqueue(absPath, relPath, folderAbs, mime string, mtime int64) {
	hash, err := s.CachedHash(absPath, mtime)
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
	case status == "pending":
		// Already in progress — frontend will match by hash when done.
	case status == "error":
		// Previous attempt failed; retry.
		s.db.Exec(`UPDATE thumbnails SET status = 'pending' WHERE content_hash = ?`, hash)
		s.enqueue(task{absPath, relPath, folderAbs, hash, thumbFile, mime})
	}
}

// previewFile is the name of the preview-sized thumbnail made alongside the
// square one named thumbFile.
func previewFile(thumbFile string) string {
	return strings.TrimSuffix(thumbFile, ".jpg") + "-preview.jpg"
}

// cachedFiles returns where both thumbnails of the record thumbFile are kept.
func (s *Service) cachedFiles(thumbFile string) outputs {
	return outputs{
		square:  filepath.Join(s.cacheDir, thumbFile),
		preview: filepath.Join(s.cacheDir, previewFile(thumbFile)),
	}
}

// ThumbPathByHash returns the absolute filesystem path to a ready square
// thumbnail looked up directly by its content hash. Returns ("", false) if
// not ready.
func (s *Service) ThumbPathByHash(hash string) (string, bool) {
	files, ok := s.readyFiles(hash)
	return files.square, ok
}

// PreviewPathByHash is ThumbPathByHash for the preview-sized thumbnail.
func (s *Service) PreviewPathByHash(hash string) (string, bool) {
	files, ok := s.readyFiles(hash)
	return files.preview, ok
}

// readyFiles returns the thumbnails made for hash, if they're ready, and
// records that they're in use (on the square's file, for both).
func (s *Service) readyFiles(hash string) (outputs, bool) {
	var thumbFile, status string
	err := s.db.QueryRow(
		`SELECT thumb_file, status FROM thumbnails WHERE content_hash = ?`, hash,
	).Scan(&thumbFile, &status)
	if err != nil || status != "done" {
		return outputs{}, false
	}
	files := s.cachedFiles(thumbFile)
	s.markUsed(files.square)
	return files, true
}

// markUsed records that the thumbnail was just needed, so RemoveUnused keeps it.
func (s *Service) markUsed(thumbPath string) {
	now := time.Now()
	if last, ok := s.marked.Load(thumbPath); ok && now.Sub(last.(time.Time)) < usedMarkInterval {
		return
	}
	if os.Chtimes(thumbPath, now, now) == nil {
		s.marked.Store(thumbPath, now)
	}
}

// RemoveUnused deletes thumbnails not needed for unusedThumbAge (or with a
// file gone — including those made before previews were, which are then made
// again with one) and the records of failed attempts, which are retried when
// the file is next listed anyway. It also forgets the hashes of files that
// no longer exist.
func (s *Service) RemoveUnused() error {
	rows, err := s.db.Query(`SELECT content_hash, thumb_file, status FROM thumbnails WHERE status != 'pending'`)
	if err != nil {
		return fmt.Errorf("query thumbnails: %w", err)
	}
	var unused []string // content hashes
	for rows.Next() {
		var hash, thumbFile, status string
		if rows.Scan(&hash, &thumbFile, &status) != nil {
			continue
		}
		files := s.cachedFiles(thumbFile)
		if status == "done" && recentlyUsed(files) {
			continue
		}
		unused = append(unused, hash)
		s.removeFiles(files)
	}
	rows.Close()
	for _, hash := range unused {
		if _, err := s.db.Exec(`DELETE FROM thumbnails WHERE content_hash = ? AND status != 'pending'`, hash); err != nil {
			return fmt.Errorf("delete thumbnail record: %w", err)
		}
	}
	if len(unused) > 0 {
		log.Printf("thumbs: removed %d unused thumbnails", len(unused))
	}

	s.hashCache.Range(func(absPath, _ any) bool {
		if _, err := os.Stat(absPath.(string)); os.IsNotExist(err) {
			s.hashCache.Delete(absPath)
		}
		return true
	})
	return nil
}

// recentlyUsed reports whether both thumbnails exist and were used within
// unusedThumbAge (recorded on the square's file).
func recentlyUsed(files outputs) bool {
	info, err := os.Stat(files.square)
	if err != nil || time.Since(info.ModTime()) > unusedThumbAge {
		return false
	}
	_, err = os.Stat(files.preview)
	return err == nil
}

// removeFiles deletes both thumbnails, reporting whether any was there.
func (s *Service) removeFiles(files outputs) (removed bool) {
	for _, path := range []string{files.square, files.preview} {
		if err := os.Remove(path); err == nil {
			removed = true
		} else if !os.IsNotExist(err) {
			log.Printf("thumbs: failed to delete %s: %v", path, err)
		}
	}
	s.marked.Delete(files.square)
	return removed
}

func (s *Service) periodicCleanup() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		if err := s.RemoveUnused(); err != nil {
			log.Printf("thumbs: %v", err)
		}
	}
}

func (s *Service) enqueue(t task) {
	go func() {
		s.sem <- struct{}{} // acquire slot
		defer func() { <-s.sem }()
		s.process(t)
	}()
}

func (s *Service) process(t task) {
	files := s.cachedFiles(t.thumbFile)
	// A file a tool hangs on would otherwise hold one of the few generation
	// slots for good, and a handful of them would stop all thumbnails.
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	err := runRecovered(func() error {
		switch {
		case strings.HasPrefix(t.mime, "video/"):
			return generateVideoThumb(ctx, t.absPath, files)
		case t.mime == "image/svg+xml":
			return generateSVGThumb(ctx, t.absPath, files)
		case t.mime == "application/pdf":
			return generatePDFThumb(ctx, t.absPath, files)
		case isMagickThumbable(t.mime):
			return generateMagickThumb(ctx, t.absPath, t.mime, files)
		default:
			return generateThumb(t.absPath, files)
		}
	})

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
		s.hub.Broadcast(t.folderAbs, t.contentHash)
	}
}

// runRecovered runs work and turns a panic in it into an error. Thumbnails are
// made in the background, outside request handling, where nothing else would
// catch a decoder crashing on a damaged or crafted file — the whole server
// would stop, and crash again whenever the folder is next opened.
func runRecovered(work func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("crashed: %v", recovered)
		}
	}()
	return work()
}

// ClearAll deletes all thumbnail records from the database and removes both
// thumbnail files of each from the cache directory.
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
	s.marked.Clear()

	deleted := 0
	for _, thumbFile := range thumbFiles {
		if s.removeFiles(s.cachedFiles(thumbFile)) {
			deleted++
		}
	}

	log.Printf("thumbs: cleared %d records, deleted the files of %d", len(thumbFiles), deleted)
	return nil
}
