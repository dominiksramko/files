package handlers

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
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
	var req screenshotRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	fsys, video, ok := requestFolderName(ctx, req.Path)
	if !ok {
		return
	}
	// ffmpeg reads the real location, checked to be inside the folder, and
	// hands the image back; it's saved through fsys under a new name.
	realVideo, err := fsys.RealPath(video)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
		return
	}

	videoBase := filepath.Base(video)
	videoStem := strings.TrimSuffix(videoBase, filepath.Ext(videoBase))

	totalSeconds := int(req.Timestamp)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	videoTimestamp := fmt.Sprintf("%02dh%02dm%02ds", hours, minutes, seconds)

	capturedAt := time.Now().Format("2006-01-02_15_04_05")
	outputFilename := fmt.Sprintf("%s_%s_%s.png", videoStem, videoTimestamp, capturedAt)

	var stderr bytes.Buffer
	cmd := exec.Command("ffmpeg",
		"-ss", fmt.Sprintf("%.3f", req.Timestamp),
		"-i", realVideo,
		"-frames:v", "1",
		"-f", "image2pipe", "-c:v", "png",
		"pipe:1",
	)
	cmd.Stderr = &stderr
	image, err := cmd.Output()
	if err != nil || len(image) == 0 {
		log.Printf("screenshot: ffmpeg failed for %q: %v\n%s", req.Path, err, stderr.String())
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "screenshot failed"})
		return
	}

	folder := filepath.Dir(video)
	existing, claimed := buildExistingMap(fsys, folder)
	outputFilename = findFreeName(outputFilename, existing, claimed)
	file, err := fsys.OpenFile(filepath.Join(folder, outputFilename), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err == nil {
		_, err = file.Write(image)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"filename": outputFilename})
}
