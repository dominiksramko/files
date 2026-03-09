package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"

	"files/internal/thumbs"
)

// GET /api/v1/files/watch?path=<folder-rel>
// SSE endpoint: streams file system events for the given folder, plus thumbnail
// events from the thumb hub. Named events:
//
//	event: add    data: FileEntry JSON
//	event: remove data: {"path":"<rel>"}
//	event: update data: FileEntry JSON
//	event: thumb  data: {"path":"<rel>","hash":"<hash>"}
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

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()
	if err := watcher.Add(folderAbs); err != nil {
		return
	}

	writeEvent := func(event, data string) {
		fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, data)
		c.Writer.Flush()
	}

	buildEntry := func(absPath, relPath string) (FileEntry, bool) {
		info, err := os.Stat(absPath)
		if err != nil {
			return FileEntry{}, false
		}
		mime := ""
		if !info.IsDir() {
			mime = mimeByExt(filepath.Ext(info.Name()))
		}
		fe := FileEntry{
			Name:       info.Name(),
			Path:       relPath,
			IsDir:      info.IsDir(),
			Size:       info.Size(),
			ModTime:    info.ModTime(),
			CreatedAt:  getBirthTime(absPath),
			Mime:       mime,
			CanExtract: CanExtract(info.Name()),
		}
		if h.thumbs != nil && !info.IsDir() && thumbs.IsThumbnailable(mime) {
			if hash, ok := h.thumbs.HashIfReady(absPath, info.ModTime().Unix()); ok {
				fe.HasThumb = true
				fe.ThumbHash = hash
			}
		}
		return fe, true
	}

	type pending struct {
		op   fsnotify.Op
		when time.Time
	}
	debounce := make(map[string]pending)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	flush := func() {
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
					data, _ := json.Marshal(fe)
					writeEvent("add", string(data))
					if h.thumbs != nil && !fe.IsDir && thumbs.IsThumbnailable(fe.Mime) {
						if info, err := os.Stat(entryAbs); err == nil {
							h.thumbs.CheckAndEnqueue(entryAbs, entryRel, folderAbs, fe.Mime, info.ModTime().Unix())
						}
					}
				}
			} else if p.op.Has(fsnotify.Remove) || p.op.Has(fsnotify.Rename) {
				data, _ := json.Marshal(map[string]string{"path": entryRel})
				writeEvent("remove", string(data))
			} else if p.op.Has(fsnotify.Write) || p.op.Has(fsnotify.Chmod) {
				if fe, ok := buildEntry(entryAbs, entryRel); ok {
					data, _ := json.Marshal(fe)
					writeEvent("update", string(data))
				}
			}
		}
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

		case <-watcher.Errors:
			// non-fatal

		case <-ticker.C:
			flush()

		case event, ok := <-thumbCh:
			if !ok {
				thumbCh = nil
				continue
			}
			data, _ := json.Marshal(map[string]string{"path": event.RelPath, "hash": event.Hash})
			writeEvent("thumb", string(data))

		case <-c.Request.Context().Done():
			return
		}
	}
}
