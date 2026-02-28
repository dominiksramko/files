package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type HLSHandler struct {
	cacheDir   string
	mu         sync.Mutex
	activeCmds map[string]*exec.Cmd
}

func NewHLSHandler(cacheDir string) *HLSHandler {
	h := &HLSHandler{
		cacheDir:   cacheDir,
		activeCmds: make(map[string]*exec.Cmd),
	}
	go h.cleanupRoutine()
	return h
}

// GET /api/v1/files/hls/playlist?path=
func (h *HLSHandler) Playlist(c *gin.Context) {
	root, rel := userRoot(c), c.Query("path")
	abs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	// Update last accessed time for the cache dir if it exists
	hashStr := fmt.Sprintf("%s|%d|%d", abs, info.Size(), info.ModTime().Unix())
	hash := sha256.Sum256([]byte(hashStr))
	hashHex := hex.EncodeToString(hash[:])
	cacheDir := filepath.Join(h.cacheDir, hashHex)
	_ = os.Chtimes(filepath.Join(cacheDir, "last_accessed"), time.Now(), time.Now())

	cmd := exec.CommandContext(c.Request.Context(), "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		abs,
	)
	out, err := cmd.Output()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to probe video"})
		return
	}
	durationStr := strings.TrimSpace(string(out))
	durationStr = strings.Split(durationStr, "\n")[0] // Just in case multiple durations are printed
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil || duration <= 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse video duration"})
		return
	}

	encodedPath := url.QueryEscape(rel)
	token := c.Query("token")

	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)

	fmt.Fprintln(c.Writer, "#EXTM3U")
	fmt.Fprintln(c.Writer, "#EXT-X-VERSION:3")
	fmt.Fprintln(c.Writer, "#EXT-X-TARGETDURATION:10")
	fmt.Fprintln(c.Writer, "#EXT-X-MEDIA-SEQUENCE:0")
	fmt.Fprintln(c.Writer, "#EXT-X-PLAYLIST-TYPE:VOD")

	seq := 0
	for duration > 0 {
		chunkDur := 10.0
		if duration < 10.0 {
			chunkDur = duration
		}
		fmt.Fprintf(c.Writer, "#EXTINF:%.3f,\n", chunkDur)
		fmt.Fprintf(c.Writer, "segment?path=%s&seq=%d&token=%s\n", encodedPath, seq, token)
		duration -= 10.0
		seq++
	}
	fmt.Fprintln(c.Writer, "#EXT-X-ENDLIST")
}

// GET /api/v1/files/hls/segment?path=...&seq=0
func (h *HLSHandler) Segment(c *gin.Context) {
	root, rel := userRoot(c), c.Query("path")
	abs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	seqStr := c.Query("seq")
	seq, err := strconv.Atoi(seqStr)
	if err != nil || seq < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sequence"})
		return
	}

	hashStr := fmt.Sprintf("%s|%d|%d", abs, info.Size(), info.ModTime().Unix())
	hash := sha256.Sum256([]byte(hashStr))
	hashHex := hex.EncodeToString(hash[:])

	cacheDir := filepath.Join(h.cacheDir, hashHex)
	chunkName := fmt.Sprintf("chunk_%03d.ts", seq)
	chunkPath := filepath.Join(cacheDir, chunkName)

	if !fileExists(chunkPath) {
		err := h.ensureGenerating(abs, cacheDir, hashHex, seq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start generator"})
			return
		}

		ready := false
		for i := 0; i < 150; i++ {
			if fileExists(chunkPath) {
				ready = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !ready {
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "chunk generation timeout"})
			return
		}
	}

	c.Header("Cache-Control", "public, max-age=604800") // 1 week
	c.File(chunkPath)
}

func (h *HLSHandler) ensureGenerating(absPath, cacheDir, hash string, seq int) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if fileExists(filepath.Join(cacheDir, fmt.Sprintf("chunk_%03d.ts", seq))) {
		return nil
	}

	if oldCmd, exists := h.activeCmds[hash]; exists {
		_ = oldCmd.Process.Kill()
		delete(h.activeCmds, hash)
	}

	err := os.MkdirAll(cacheDir, 0755)
	if err != nil {
		return err
	}

	accessFile := filepath.Join(cacheDir, "last_accessed")
	if !fileExists(accessFile) {
		_ = os.WriteFile(accessFile, []byte(""), 0644)
	} else {
		_ = os.Chtimes(accessFile, time.Now(), time.Now())
	}

	startAt := float64(seq) * 10.0
	cmd := exec.Command("ffmpeg",
		"-y",
		"-ss", fmt.Sprintf("%.3f", startAt),
		"-i", absPath,
		"-output_ts_offset", fmt.Sprintf("%.3f", startAt),
		"-c:v", "copy",
		"-c:a", "aac",
		"-f", "hls",
		"-hls_time", "10",
		"-hls_list_size", "0",
		"-start_number", strconv.Itoa(seq),
		"-hls_flags", "temp_file",
		"-hls_segment_filename", filepath.Join(cacheDir, "chunk_%03d.ts"),
		filepath.Join(cacheDir, "playlist.m3u8"),
	)

	if err := cmd.Start(); err != nil {
		return err
	}

	h.activeCmds[hash] = cmd

	go func() {
		_ = cmd.Wait()
		h.mu.Lock()
		if h.activeCmds[hash] == cmd {
			delete(h.activeCmds, hash)
		}
		h.mu.Unlock()
	}()

	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (h *HLSHandler) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour)
	for range ticker.C {
		h.cleanupOldCache()
	}
}

func (h *HLSHandler) cleanupOldCache() {
	if h.cacheDir == "" {
		return
	}
	entries, err := os.ReadDir(h.cacheDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			dirPath := filepath.Join(h.cacheDir, e.Name())
			accessFile := filepath.Join(dirPath, "last_accessed")
			info, err := os.Stat(accessFile)
			if err == nil {
				if time.Since(info.ModTime()) > 7*24*time.Hour {
					_ = os.RemoveAll(dirPath)
				}
			}
		}
	}
}
