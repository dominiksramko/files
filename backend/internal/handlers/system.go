package handlers

import (
	"net/http"
	"syscall"

	"github.com/gin-gonic/gin"
)

type SystemHandler struct {
	version string
}

func NewSystemHandler(version string) *SystemHandler {
	return &SystemHandler{version: version}
}

// GET /api/v1/system/disk
// Returns disk space stats for the disk holding the user's own folder (each
// user's folder can be on its own disk or mount).
func (h *SystemHandler) DiskSpace(c *gin.Context) {
	fsys, folder, ok := requestFolderName(c, "")
	if !ok {
		return
	}
	realFolder, err := fsys.RealPath(folder)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "your folder doesn't exist"})
		return
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(realFolder, &stat); err != nil {
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
