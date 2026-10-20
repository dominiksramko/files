package handlers

import (
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"files/internal/thumbs"
	"files/internal/userfs"
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
	claimedHash := c.Query("hash")
	if claimedHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hash is required"})
		return
	}

	fsys, name, ok := requestFolderName(c, c.Query("path"))
	if !ok {
		return
	}
	fi, err := fsys.Stat(name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	absPath, err := fsys.RealPath(name)
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

// enqueueFolder enqueues thumbnail generation for every thumbnailable file in
// the folder (folderAbs is its real location). Links are resolved through
// fsys: one that can't be followed inside the user's folder gets no
// thumbnail. Called asynchronously when the watch stream connects.
func enqueueFolder(svc *thumbs.Service, fsys *userfs.FS, folder, folderAbs string) {
	entries, err := fsys.ReadDir(folder)
	if err != nil {
		return
	}

	for _, e := range entries {
		name := filepath.Join(folder, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		entryAbs := entryRealPath(fsys, folderAbs, name, info)
		if entryAbs == "" {
			continue
		}
		if info, err = fsys.Stat(name); err != nil || info.IsDir() {
			continue
		}
		mime := mimeByExt(filepath.Ext(e.Name()))
		if !thumbs.IsThumbnailable(mime) {
			continue
		}
		svc.CheckAndEnqueue(entryAbs, filepath.ToSlash(name), folderAbs, mime, info.ModTime().Unix())
	}
}
