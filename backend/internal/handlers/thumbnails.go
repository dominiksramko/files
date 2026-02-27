package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"files/internal/thumbs"
)

type ThumbnailHandler struct {
	svc *thumbs.Service
}

func NewThumbnailHandler(svc *thumbs.Service) *ThumbnailHandler {
	return &ThumbnailHandler{svc: svc}
}

// GET /api/v1/thumbnails/get?path=<rel>
// Serves the generated thumbnail JPEG for the given file.
func (h *ThumbnailHandler) Get(c *gin.Context) {
	if h.svc == nil {
		c.Status(http.StatusNotFound)
		return
	}
	root, rel := userRoot(c), c.Query("path")

	absPath, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	thumbPath, ok := h.svc.ThumbPath(absPath)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}

	// Thumbnails are content-addressed (filename includes mtime hash), so they
	// can be cached indefinitely.
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.File(thumbPath)
}

// GET /api/v1/thumbnails/stream?path=<folder-rel>
// SSE endpoint: streams "data: <rel-path>\n\n" events as thumbnails become
// ready for files inside the given folder. The client should connect here
// immediately after loading a folder listing and stay connected until
// navigating away.
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

	for {
		select {
		case relPath, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(c.Writer, "data: %s\n\n", relPath)
			c.Writer.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}
