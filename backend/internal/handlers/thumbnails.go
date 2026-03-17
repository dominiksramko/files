package handlers

import (
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

	c.Header("Cache-Control", "no-cache")
	c.File(thumbPath)
}

// enqueueFolder scans a folder and enqueues thumbnail generation for all
// thumbnailable files. Called asynchronously when the watch stream connects.
func enqueueFolder(svc *thumbs.Service, _, rel, folderAbs string) {
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
		svc.CheckAndEnqueue(entryAbs, entryRel, folderAbs, mime, info.ModTime().Unix())
	}
}
