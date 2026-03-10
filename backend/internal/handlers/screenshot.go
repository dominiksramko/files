package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ScreenshotHandler struct{}

func NewScreenshotHandler() *ScreenshotHandler {
	return &ScreenshotHandler{}
}

type screenshotRequest struct {
	Path      string  `json:"path"`
	Timestamp float64 `json:"timestamp"`
}

// POST /api/v1/files/screenshot
func (handler *ScreenshotHandler) Take(ctx *gin.Context) {
	root := userRoot(ctx)

	var req screenshotRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	absVideoPath, err := safePath(root, req.Path)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	videoDir := filepath.Dir(absVideoPath)
	videoBase := filepath.Base(absVideoPath)
	videoStem := strings.TrimSuffix(videoBase, filepath.Ext(videoBase))

	totalSeconds := int(req.Timestamp)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	videoTimestamp := fmt.Sprintf("%02dh%02dm%02ds", hours, minutes, seconds)

	capturedAt := time.Now().Format("2006-01-02_15_04_05")
	outputFilename := fmt.Sprintf("%s_%s_%s.png", videoStem, videoTimestamp, capturedAt)
	absOutputPath := filepath.Join(videoDir, outputFilename)

	cmd := exec.Command("ffmpeg",
		"-ss", fmt.Sprintf("%.3f", req.Timestamp),
		"-i", absVideoPath,
		"-frames:v", "1",
		"-y",
		absOutputPath,
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("screenshot: ffmpeg failed for %q: %v\n%s", req.Path, err, output)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "screenshot failed"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"filename": outputFilename})
}
