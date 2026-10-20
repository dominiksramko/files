package handlers

import (
	"context"
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

	"files/internal/media"

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
	rel := c.Query("path")
	fsys, name, ok := requestFolderName(c, rel)
	if !ok {
		return
	}
	info, err := fsys.Stat(name)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	// ffmpeg gets the real location, checked to be inside the folder.
	abs, err := fsys.RealPath(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	// Update last accessed time for the cache dir if it exists
	hashStr := fmt.Sprintf("%s|%d|%d", abs, info.Size(), info.ModTime().Unix())
	hash := sha256.Sum256([]byte(hashStr))
	hashHex := hex.EncodeToString(hash[:])
	cacheDir := filepath.Join(h.cacheDir, hashHex)
	_ = os.Chtimes(filepath.Join(cacheDir, "last_accessed"), time.Now(), time.Now())

	duration, err := probeDuration(c.Request.Context(), abs)
	if err != nil || duration <= 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to probe video duration"})
		return
	}

	encodedPath := url.QueryEscape(rel)

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
		fmt.Fprintf(c.Writer, "segment?path=%s&seq=%d\n", encodedPath, seq)
		duration -= 10.0
		seq++
	}
	fmt.Fprintln(c.Writer, "#EXT-X-ENDLIST")
}

// GET /api/v1/files/hls/segment?path=...&seq=0
func (h *HLSHandler) Segment(c *gin.Context) {
	rel := c.Query("path")
	fsys, name, ok := requestFolderName(c, rel)
	if !ok {
		return
	}
	info, err := fsys.Stat(name)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	// ffmpeg gets the real location, checked to be inside the folder.
	abs, err := fsys.RealPath(name)
	if err != nil {
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
		if err := h.ensureGenerating(abs, cacheDir, hashHex, seq); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start generator"})
			return
		}

		ready := false
		for i := 0; i < 300; i++ {
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

	// Kill any existing process for this file and restart from seq.
	if oldCmd, exists := h.activeCmds[hash]; exists {
		_ = oldCmd.Process.Kill()
		delete(h.activeCmds, hash)
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	accessFile := filepath.Join(cacheDir, "last_accessed")
	if !fileExists(accessFile) {
		_ = os.WriteFile(accessFile, []byte(""), 0644)
	} else {
		_ = os.Chtimes(accessFile, time.Now(), time.Now())
	}

	// Copy audio only if it's already TS-compatible, otherwise transcode to AAC.
	audioCodec := "aac"
	if probe, err := media.FFprobe(absPath,
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
	).Output(); err == nil {
		switch strings.TrimSpace(string(probe)) {
		case "aac", "mp3":
			audioCodec = "copy"
		}
	}

	// Copy video only if the codec can be muxed into MPEG-TS and decoded by
	// browsers. MPEG-4 Part 2 (DivX/Xvid), MPEG-2 (DVD, .mpg, broadcast .m2ts),
	// WMV and others must be transcoded to H.264.
	videoCodec := "libx264"
	if probe, err := media.FFprobe(absPath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
	).Output(); err == nil {
		switch strings.TrimSpace(string(probe)) {
		case "h264", "hevc":
			videoCodec = "copy"
		}
	}

	startAt := float64(seq) * 10.0

	// nice -n 10: ffmpeg only uses spare CPU, keeping the server responsive.
	// -fflags +genpts: regenerate missing presentation timestamps, required for
	// AVI files where pre-input seeking leaves the first packets without PTS/DTS.
	cmd := media.FFmpeg(context.Background(), absPath,
		[]string{"-y", "-fflags", "+genpts", "-ss", fmt.Sprintf("%.3f", startAt)},
		"-output_ts_offset", fmt.Sprintf("%.3f", startAt),
		"-c:v", videoCodec,
		"-c:a", audioCodec,
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

// probeDuration returns the duration of a video file in seconds.
// It tries the container format first, then falls back to the video stream,
// then the audio stream — necessary for AVI files whose container header
// reports "N/A" but whose streams carry accurate duration metadata.
func probeDuration(ctx context.Context, absPath string) (float64, error) {
	strategies := [][]string{
		{"-show_entries", "format=duration"},
		{"-select_streams", "v:0", "-show_entries", "stream=duration"},
		{"-select_streams", "a:0", "-show_entries", "stream=duration"},
	}
	for _, args := range strategies {
		baseArgs := append([]string{"-v", "error", "-of", "default=noprint_wrappers=1:nokey=1"}, args...)
		out, err := media.FFprobeContext(ctx, absPath, baseArgs...).Output()
		if err != nil {
			continue
		}
		line := strings.TrimSpace(strings.Split(string(out), "\n")[0])
		if line == "" || line == "N/A" {
			continue
		}
		if d, err := strconv.ParseFloat(line, 64); err == nil && d > 0 {
			return d, nil
		}
	}
	return 0, fmt.Errorf("could not determine duration")
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
