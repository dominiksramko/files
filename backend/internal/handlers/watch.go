package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"

	"files/internal/thumbs"
)

// watchPingInterval keeps idle streams alive through proxies and NAT, lets the
// client notice a dead connection, and lets the server notice a gone client
// (a write to it fails).
const watchPingInterval = 25 * time.Second

// GET /api/v1/files/watch?path=<folder-rel>
// SSE endpoint: streams file system events for the given folder, plus thumbnail
// events from the thumb hub. Named events:
//
//	event: snapshot data: FileEntry[] — the full listing; sent first, and again
//	                                    whenever events may have been lost
//	event: add      data: FileEntry JSON
//	event: remove   data: {"path":"<rel>"}
//	event: update   data: FileEntry JSON
//	event: thumb    data: {"hash":"<hash>"}
//	event: ping     data: {}
func (h *FilesHandler) Watch(c *gin.Context) {
	root, rel := userRoot(c), c.Query("path")

	folderAbs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := os.Stat(folderAbs); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "directory not found"})
		return
	}

	// The watcher starts before the snapshot is read, so nothing can change
	// unseen in between, and before the headers go out, so a failure (e.g. the
	// inotify instance limit) is still reported as an error status.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("watch %q: %v", rel, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cannot watch directory"})
		return
	}
	defer watcher.Close()
	if err := watcher.Add(folderAbs); err != nil {
		log.Printf("watch %q: %v", rel, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cannot watch directory"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Subscribe to thumbnail hub (nil channel when thumbs disabled → never fires in select).
	var thumbCh <-chan thumbs.ThumbEvent
	var thumbUnsub func()
	if h.thumbs != nil {
		thumbCh, thumbUnsub = h.thumbs.Hub().Subscribe(folderAbs)
		defer thumbUnsub()
		go enqueueFolder(h.thumbs, root, rel, folderAbs)
	}

	// A failed write means the client is gone; the handler returns.
	writeEvent := func(event string, payload any) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}

	sendSnapshot := func() error {
		files, err := h.listDir(folderAbs, rel)
		if err != nil {
			return err
		}
		return writeEvent("snapshot", files)
	}

	buildEntry := func(absPath, relPath string) (FileEntry, bool) {
		info, err := os.Lstat(absPath) // Lstat, like the listing's DirEntry.Info
		if err != nil {
			return FileEntry{}, false
		}
		return h.newFileEntry(absPath, relPath, info), true
	}

	type pending struct {
		op   fsnotify.Op
		when time.Time
	}
	debounce := make(map[string]pending)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	pingTicker := time.NewTicker(watchPingInterval)
	defer pingTicker.Stop()

	flush := func() error {
		now := time.Now()
		for name, p := range debounce {
			if now.Sub(p.when) < 80*time.Millisecond {
				continue
			}
			delete(debounce, name)

			entryAbs := filepath.Join(folderAbs, name)
			entryRel := filepath.Join(rel, name)

			if p.op.Has(fsnotify.Create) {
				if fe, ok := buildEntry(entryAbs, entryRel); ok {
					if err := writeEvent("add", fe); err != nil {
						return err
					}
					if h.thumbs != nil && !fe.IsDir && thumbs.IsThumbnailable(fe.Mime) {
						h.thumbs.CheckAndEnqueue(entryAbs, entryRel, folderAbs, fe.Mime, fe.ModTime.Unix())
					}
				}
			} else if p.op.Has(fsnotify.Remove) || p.op.Has(fsnotify.Rename) {
				if err := writeEvent("remove", map[string]string{"path": entryRel}); err != nil {
					return err
				}
			} else if p.op.Has(fsnotify.Write) || p.op.Has(fsnotify.Chmod) {
				if fe, ok := buildEntry(entryAbs, entryRel); ok {
					if err := writeEvent("update", fe); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}

	if err := sendSnapshot(); err != nil {
		return
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			name := filepath.Base(event.Name)
			existing, has := debounce[name]
			op := event.Op
			if has {
				op |= existing.op
			}
			debounce[name] = pending{op: op, when: time.Now()}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			// Events may have been dropped (e.g. fsnotify.ErrEventOverflow during a
			// bulk copy): resend the whole listing so the client can't drift.
			log.Printf("watch %q: %v", rel, err)
			if sendSnapshot() != nil {
				return
			}

		case <-ticker.C:
			if flush() != nil {
				return
			}

		case <-pingTicker.C:
			if writeEvent("ping", struct{}{}) != nil {
				return
			}

		case event, ok := <-thumbCh:
			if !ok {
				thumbCh = nil
				continue
			}
			if writeEvent("thumb", map[string]string{"hash": event.Hash}) != nil {
				return
			}

		case <-c.Request.Context().Done():
			return
		}
	}
}
