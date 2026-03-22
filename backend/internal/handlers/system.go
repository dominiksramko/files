package handlers

import (
	"net/http"
	"syscall"

	"github.com/gin-gonic/gin"
)

type SystemHandler struct {
	rootPath string
	version  string
}

func NewSystemHandler(rootPath, version string) *SystemHandler {
	return &SystemHandler{rootPath: rootPath, version: version}
}

// GET /api/v1/system/disk
// Returns disk space stats for the filesystem containing rootPath.
func (h *SystemHandler) DiskSpace(c *gin.Context) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(h.rootPath, &stat); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	bsize := uint64(stat.Bsize) //nolint:unconvert // int64 on Linux, int32 on macOS
	total := stat.Blocks * bsize
	free := stat.Bavail * bsize
	used := total - free

	var pct float64
	if total > 0 {
		pct = float64(used) / float64(total) * 100
	}

	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"free":  free,
		"used":  used,
		"pct":   pct,
	})
}

// GET /api/v1/system/version
func (h *SystemHandler) Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"version": h.version})
}
