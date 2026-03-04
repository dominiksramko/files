package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"files/internal/thumbs"
)

type ThumbnailHandler struct {
	svc *thumbs.Service
}

func NewThumbnailHandler(svc *thumbs.Service) *ThumbnailHandler {
	return &ThumbnailHandler{svc: svc}
}

// GET /api/v1/thumbnails/get?path=<rel>&hash=<content-hash>
// Serves the generated thumbnail JPEG for the given file.
// Both path and hash are required: path resolves the file within the user's
// root (authorization), hash verifies the file hasn't changed since the
// client received the listing (cache correctness). Mismatches return 404.
func (h *ThumbnailHandler) Get(c *gin.Context) {
	if h.svc == nil {
		c.Status(http.StatusNotFound)
		return
	}
	root, rel := userRoot(c), c.Query("path")
	claimedHash := c.Query("hash")

	if claimedHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hash is required"})
		return
	}

	absPath, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fi, err := os.Stat(absPath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	// Verify the file at this path currently has the claimed hash and a ready thumbnail.
	actualHash, ok := h.svc.HashIfReady(absPath, fi.ModTime().Unix())
	if !ok || actualHash != claimedHash {
		c.Status(http.StatusNotFound)
		return
	}

	thumbPath, ok := h.svc.ThumbPathByHash(claimedHash)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}

	// The URL encodes the content hash so it changes whenever the file changes —
	// safe to cache indefinitely.
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.File(thumbPath)
}

// GET /api/v1/thumbnails/stream?path=<folder-rel>
// SSE endpoint: streams JSON events as thumbnails become ready for files inside
// the given folder. Each event is: data: {"path":"<rel>","hash":"<hash>"}\n\n
// The client should connect here immediately after loading a folder listing and
// stay connected until navigating away.
func (h *ThumbnailHandler) Stream(c *gin.Context) {
	if h.svc == nil {
		// Send an empty SSE stream that immediately closes.
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Writer.Flush()
		return
	}
	root, rel := userRoot(c), c.Query("path")

	folderAbs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // Disable nginx proxy buffering

	ch, unsub := h.svc.Hub().Subscribe(folderAbs)
	defer unsub()

	// Flush headers immediately so the browser knows the connection is live.
	c.Writer.Flush()

	// Enqueue thumbnails for all thumbnailable files in this folder (async).
	go h.enqueueFolder(root, rel, folderAbs)

	type eventMsg struct {
		Path string `json:"path"`
		Hash string `json:"hash"`
	}

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(eventMsg{Path: event.RelPath, Hash: event.Hash})
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}

// enqueueFolder scans a folder and enqueues thumbnail generation for all
// thumbnailable files. This runs asynchronously after the SSE stream starts.
func (h *ThumbnailHandler) enqueueFolder(root, rel, folderAbs string) {
	entries, err := os.ReadDir(folderAbs)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		mime := mimeByExt(filepath.Ext(e.Name()))
		if !thumbs.IsThumbnailable(mime) {
			continue
		}

		entryAbs := filepath.Join(folderAbs, e.Name())
		entryRel := filepath.Join(rel, e.Name())
		h.svc.CheckAndEnqueue(entryAbs, entryRel, folderAbs, mime, info.ModTime().Unix())
	}
}

