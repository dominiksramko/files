package handlers

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"files/internal/diskusage"
	"files/internal/userfs"

	"github.com/gin-gonic/gin"
)

// The disk usage page: a scan of a folder, kept in memory (one per user) and
// browsed a folder at a time. The scan keeps folders only; the files of the
// folder being looked at are read again each time.

const (
	usageScanKept       = 30 * time.Minute // a scan not looked at for this long is dropped
	usageScansAtOnce    = 2                // scans running at once, server-wide
	usageSessionRecheck = 5 * time.Second  // how often a running scan checks the session
)

type userScan struct {
	mu       sync.Mutex // held while the scan is read or changed
	homeDir  string     // the user's home folder it was made in
	result   *diskusage.Result
	lastUsed time.Time
}

// usageStore holds each user's latest scan, and the user's running one.
type usageStore struct {
	mu      sync.Mutex
	scans   map[int64]*userScan
	running map[int64]*runningScan
	slots   chan struct{}
}

type runningScan struct{ cancel context.CancelFunc }

func newUsageStore() *usageStore {
	return &usageStore{
		scans:   map[int64]*userScan{},
		running: map[int64]*runningScan{},
		slots:   make(chan struct{}, usageScansAtOnce),
	}
}

// start registers a new scan for userID, stopping the one it has running.
// finish must be called when it ends.
func (s *usageStore) start(userID int64, cancel context.CancelFunc) (finish func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous := s.running[userID]; previous != nil {
		previous.cancel()
	}
	scan := &runningScan{cancel: cancel}
	s.running[userID] = scan
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.running[userID] == scan {
			delete(s.running, userID)
		}
	}
}

func (s *usageStore) save(userID int64, homeDir string, result *diskusage.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scans[userID] = &userScan{homeDir: homeDir, result: result, lastUsed: time.Now()}
}

// use runs fn with the user's scan of the folder name, if there is one; fn
// may change the scan. It reports whether there was. Scans not used for
// usageScanKept are dropped on the way.
func (s *usageStore) use(userID int64, homeDir, name string, fn func(*diskusage.Result)) bool {
	s.mu.Lock()
	now := time.Now()
	for id, scan := range s.scans {
		if now.Sub(scan.lastUsed) > usageScanKept {
			delete(s.scans, id)
		}
	}
	scan := s.scans[userID]
	if scan == nil || scan.homeDir != homeDir || scan.result.Name != name {
		s.mu.Unlock()
		return false
	}
	scan.lastUsed = now
	s.mu.Unlock()

	// Only this scan waits while fn runs (a large delete takes a while).
	scan.mu.Lock()
	defer scan.mu.Unlock()
	fn(scan.result)
	return true
}

// UsageEntry is an item of the folder looked at, with what it takes. A folder
// the scan doesn't have (made since, or a link) isn't Scanned: its sizes are
// its own only. HardLinks is the number of names a file has when more than
// one: they share the space, which the scan's totals count once.
type UsageEntry struct {
	FileEntry
	diskusage.Totals
	Link      bool `json:"link,omitempty"`
	HardLinks int  `json:"hard_links,omitempty"`
	Scanned   bool `json:"scanned"`
}

// hardLinks is info's number of names when it has more than one.
func hardLinks(info os.FileInfo) int {
	if links := diskusage.HardLinks(info); links > 1 {
		return links
	}
	return 0
}

type usageListing struct {
	Root       string           `json:"root"`
	At         string           `json:"at"`
	ScannedAt  time.Time        `json:"scanned_at"`
	Unreadable int              `json:"unreadable"`
	RootTotal  diskusage.Totals `json:"root_total"`
	Total      diskusage.Totals `json:"total"`
	Entries    []UsageEntry     `json:"entries"`
}

func usageUserID(c *gin.Context) int64 { return c.GetInt64("userID") }

// usageScanContext ends with the request, when the server shuts down, or when
// cancel is called.
func (h *FilesHandler) usageScanContext(c *gin.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(c.Request.Context())
	select {
	case <-h.streamsEnd:
		cancel()
	default:
		go func() {
			select {
			case <-h.streamsEnd:
				cancel()
			case <-ctx.Done():
			}
		}()
	}
	return ctx, cancel
}

// GET /api/v1/files/usage/scan?path=
//
// Scans the folder and keeps the result for the other usage endpoints.
// Streams SSE events:
//
//	event: queued   data: {}                                   — waiting for other scans
//	event: progress data: {"files","dirs","disk","apparent"}   — running totals
//	event: done     data: {"scanned_at"}
//	event: failed   data: {"error"}   — named apart from EventSource's own error event
//
// It stops when the client leaves, the session ends or the server shuts down,
// and when the same user starts another scan.
func (h *FilesHandler) UsageScan(c *gin.Context) {
	fsys, name, ok := requestFolderName(c, c.Query("path"))
	if !ok {
		return
	}
	userID := usageUserID(c)
	ctx, cancel := h.usageScanContext(c)
	defer cancel()
	finish := h.usage.start(userID, cancel)
	defer finish()

	startEventStream(c)

	select {
	case h.usage.slots <- struct{}{}:
	default:
		if writeEvent(c, "queued", struct{}{}) != nil {
			return
		}
		select {
		case h.usage.slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
	}
	defer func() { <-h.usage.slots }()

	lastCheck := time.Now()
	result, err := diskusage.Scan(ctx, fsys, name, func(totals diskusage.Totals) {
		if time.Since(lastCheck) >= usageSessionRecheck {
			lastCheck = time.Now()
			if !sessionStillValid(c) {
				cancel()
				return
			}
		}
		if writeEvent(c, "progress", totals) != nil {
			cancel()
		}
	})
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		message := "cannot scan this folder"
		if errors.Is(err, os.ErrNotExist) {
			message = "folder not found"
		} else if errors.Is(err, diskusage.ErrNotFolder) {
			message = "not a folder"
		}
		writeEvent(c, "failed", gin.H{"error": message})
		return
	}
	// The session was checked when the stream opened; keep nothing for a
	// session that has ended since.
	if !sessionStillValid(c) {
		return
	}
	h.usage.save(userID, userRoot(c), result)
	writeEvent(c, "progress", result.Root.Totals)
	writeEvent(c, "done", gin.H{"scanned_at": result.ScannedAt})
}

// requestUsage checks the scanned folder path (and answers 404 with
// scan_needed when the user has no scan of it) and runs fn with the scan.
func (h *FilesHandler) requestUsage(c *gin.Context, path string, fn func(*userfs.FS, *diskusage.Result)) {
	fsys, root, ok := requestFolderName(c, path)
	if !ok {
		return
	}
	if !h.usage.use(usageUserID(c), userRoot(c), root, func(result *diskusage.Result) { fn(fsys, result) }) {
		c.JSON(http.StatusNotFound, gin.H{"error": "this folder hasn't been scanned", "scan_needed": true})
	}
}

// GET /api/v1/files/usage?path=<scanned folder>&at=<folder inside it>
//
// The folder at (default: the scanned one) with what each item takes,
// largest on disk first.
func (h *FilesHandler) Usage(c *gin.Context) {
	path := c.Query("path")
	atPath := c.Query("at")
	if atPath == "" {
		atPath = path
	}
	at, ok := requestName(c, atPath)
	if !ok {
		return
	}
	h.requestUsage(c, path, func(fsys *userfs.FS, result *diskusage.Result) {
		if !result.Contains(at) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "not inside the scanned folder"})
			return
		}
		files, err := h.listDir(fsys, at)
		if err != nil {
			if os.IsNotExist(err) {
				c.JSON(http.StatusNotFound, gin.H{"error": "this folder no longer exists"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			}
			return
		}
		listing := usageListing{
			Root:       userfs.RequestPath(result.Name),
			At:         userfs.RequestPath(at),
			ScannedAt:  result.ScannedAt,
			Unreadable: result.Unreadable,
			RootTotal:  result.Root.Totals,
			Entries:    make([]UsageEntry, 0, len(files)),
		}
		folder := result.Find(at)
		if folder != nil {
			listing.Total = folder.Totals
		}
		for _, file := range files {
			name := filepath.FromSlash(file.Path)
			info, err := fsys.Lstat(name)
			if err != nil {
				continue // removed since it was listed
			}
			entry := UsageEntry{FileEntry: file, Link: info.Mode()&os.ModeSymlink != 0, HardLinks: hardLinks(info)}
			var scanned *diskusage.Dir
			if folder != nil && info.IsDir() {
				scanned = folder.Children[info.Name()]
			}
			if scanned != nil {
				entry.Totals = scanned.Totals
				entry.Scanned = true
			} else {
				entry.Disk = diskusage.Allocated(info)
				if !info.IsDir() {
					entry.Apparent = info.Size()
				}
			}
			listing.Entries = append(listing.Entries, entry)
		}
		sort.SliceStable(listing.Entries, func(i, j int) bool {
			return listing.Entries[i].Disk > listing.Entries[j].Disk
		})
		c.JSON(http.StatusOK, listing)
	})
}

// GET /api/v1/files/usage/largest?path=<scanned folder>&size=disk|apparent
//
// The largest files of the scan that are still there, largest first.
func (h *FilesHandler) UsageLargest(c *gin.Context) {
	measure := diskusage.ByDisk
	if c.Query("size") == "apparent" {
		measure = diskusage.ByApparent
	}
	h.requestUsage(c, c.Query("path"), func(fsys *userfs.FS, result *diskusage.Result) {
		entries := []UsageEntry{}
		for _, file := range result.Largest(measure) {
			info, err := fsys.Lstat(file.Path)
			if err != nil || info.IsDir() {
				continue // deleted or replaced since the scan
			}
			entries = append(entries, UsageEntry{
				FileEntry: h.newFileEntry(fsys, "", file.Path, info),
				Totals:    diskusage.Totals{Disk: file.Disk, Apparent: file.Apparent},
				Link:      info.Mode()&os.ModeSymlink != 0,
				HardLinks: hardLinks(info),
				Scanned:   true,
			})
		}
		c.JSON(http.StatusOK, entries)
	})
}

// DELETE /api/v1/files/usage   body: { "path": "<scanned folder>", "paths": [...] }
//
// Deletes items inside the scanned folder for good, like DELETE /files, and
// takes them out of the scan. Answers with what was freed and the new totals.
func (h *FilesHandler) UsageDelete(c *gin.Context) {
	var body struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths required"})
		return
	}
	h.requestUsage(c, body.Path, func(fsys *userfs.FS, result *diskusage.Result) {
		_, names, ok := requestChildNames(c, body.Paths)
		if !ok {
			return
		}
		infos := make([]os.FileInfo, len(names))
		var outside []string
		for i, name := range names {
			if name == result.Name || !result.Contains(name) {
				outside = append(outside, userfs.RequestPath(name))
				continue
			}
			infos[i], _ = fsys.Lstat(name) // a missing one is reported by removeEntries
		}
		if len(outside) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "not inside the scanned folder: " + strings.Join(outside, ", ")})
			return
		}
		if !removeEntries(c, fsys, names) {
			return
		}
		var freed diskusage.Totals
		for i, name := range names {
			removed := result.Remove(name, infos[i])
			freed.Files += removed.Files
			freed.Dirs += removed.Dirs
			freed.Disk += removed.Disk
			freed.Apparent += removed.Apparent
		}
		c.JSON(http.StatusOK, gin.H{"freed": freed, "root_total": result.Root.Totals})
	})
}
