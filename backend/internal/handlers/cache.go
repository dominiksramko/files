package handlers

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"files/internal/thumbs"
)

type CacheHandler struct {
	thumbSvc     *thumbs.Service
	hlsCacheDir  string
	convCacheDir string
}

func NewCacheHandler(thumbSvc *thumbs.Service, hlsCacheDir, convCacheDir string) *CacheHandler {
	return &CacheHandler{thumbSvc: thumbSvc, hlsCacheDir: hlsCacheDir, convCacheDir: convCacheDir}
}

// DELETE /api/v1/cache
// Clears thumbnail DB records + files, HLS segment cache, and conv cache. Admin-only.
func (h *CacheHandler) Clear(c *gin.Context) {
	if h.thumbSvc != nil {
		if err := h.thumbSvc.ClearAll(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// HLS cache: subdirectories per file hash
	if h.hlsCacheDir != "" {
		if entries, err := os.ReadDir(h.hlsCacheDir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					_ = os.RemoveAll(filepath.Join(h.hlsCacheDir, e.Name()))
				}
			}
		}
	}

	// Conv cache: flat converted files (.jpg images, .m4a audio)
	if h.convCacheDir != "" {
		if entries, err := os.ReadDir(h.convCacheDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					_ = os.Remove(filepath.Join(h.convCacheDir, e.Name()))
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "cache cleared"})
}
