package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

// segmentTarget is the length the playlist aims each segment at, in seconds.
const segmentTarget = 10.0

// Video transcoding limits, variables so tests can shorten them.
var (
	// maxTranscoders run at once; starting another stops the least recently used.
	maxTranscoders = 2
	// transcoderIdleTimeout stops a transcoder nobody has asked anything of
	// for this long (the viewer left).
	transcoderIdleTimeout = time.Minute
	// segmentLookahead: a segment at most this far past the last one written
	// is waited for rather than restarting the transcoder there.
	segmentLookahead = 3
	// cacheMaxAge: a video's segments are deleted this long after it was last played.
	cacheMaxAge = 7 * 24 * time.Hour
	// segmentWait is how long a segment request waits for its segment.
	segmentWait = 30 * time.Second
)

// hlsPlan is how one video is cut into segments: segment k starts at
// Bounds[k] and ends at Bounds[k+1] (the last one at Duration). It's worked
// out once and kept in the video's cache folder, so the playlist and every
// segment — whichever transcoder run wrote it — agree.
//
// When the video stream is copied, segments can only start on its keyframes,
// so each boundary is the first keyframe at or after a multiple of
// segmentTarget; when it's transcoded, keyframes are forced at exact
// multiples. Either way the playlist lists the real segment lengths, so
// seeking lands where it should.
type hlsPlan struct {
	VideoCodec string    `json:"video_codec"`
	AudioCodec string    `json:"audio_codec"`
	Duration   float64   `json:"duration"`
	Bounds     []float64 `json:"bounds"`
}

// transcoder is a running ffmpeg writing a video's segments from startSeq on,
// into its own run folder, so no run ever rewrites a segment another run
// finished (and that may be being served).
type transcoder struct {
	cmd      *exec.Cmd
	startSeq int
	runDir   string
	lastUsed time.Time
}

type HLSHandler struct {
	cacheDir string
	mu       sync.Mutex
	active   map[string]*transcoder // by video hash
	planMu   sync.Mutex             // one plan worked out at a time
}

func NewHLSHandler(cacheDir string) *HLSHandler {
	h := &HLSHandler{
		cacheDir: cacheDir,
		active:   make(map[string]*transcoder),
	}
	go h.cleanupRoutine()
	return h
}

// hlsVideo is the video a playlist or segment request is about.
type hlsVideo struct {
	absPath  string // real location, checked to be inside the user's folder
	hash     string
	cacheDir string
}

// requestVideo resolves the request's video; on failure it has answered.
func (h *HLSHandler) requestVideo(c *gin.Context) (hlsVideo, bool) {
	fsys, name, ok := requestFolderName(c, c.Query("path"))
	if !ok {
		return hlsVideo{}, false
	}
	info, err := fsys.Stat(name)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return hlsVideo{}, false
	}
	abs, err := fsys.RealPath(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return hlsVideo{}, false
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", abs, info.Size(), info.ModTime().Unix())))
	hash := hex.EncodeToString(sum[:])
	return hlsVideo{absPath: abs, hash: hash, cacheDir: filepath.Join(h.cacheDir, hash)}, true
}

// GET /api/v1/files/hls/playlist?path=
func (h *HLSHandler) Playlist(c *gin.Context) {
	video, ok := h.requestVideo(c)
	if !ok {
		return
	}
	plan, err := h.loadPlan(c.Request.Context(), video)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to probe video"})
		return
	}

	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	writePlaylist(c.Writer, plan, url.QueryEscape(c.Query("path")))
}

// writePlaylist writes the HLS playlist for plan, listing each segment with
// its real length.
func writePlaylist(out io.Writer, plan hlsPlan, encodedPath string) {
	longest := 0.0
	for seq := range plan.Bounds {
		longest = max(longest, plan.segmentDuration(seq))
	}
	fmt.Fprintln(out, "#EXTM3U")
	fmt.Fprintln(out, "#EXT-X-VERSION:3")
	fmt.Fprintf(out, "#EXT-X-TARGETDURATION:%d\n", int(longest+0.999))
	fmt.Fprintln(out, "#EXT-X-MEDIA-SEQUENCE:0")
	fmt.Fprintln(out, "#EXT-X-PLAYLIST-TYPE:VOD")
	for seq := range plan.Bounds {
		fmt.Fprintf(out, "#EXTINF:%.3f,\n", plan.segmentDuration(seq))
		fmt.Fprintf(out, "segment?path=%s&seq=%d\n", encodedPath, seq)
	}
	fmt.Fprintln(out, "#EXT-X-ENDLIST")
}

func (plan hlsPlan) segmentDuration(seq int) float64 {
	if seq+1 < len(plan.Bounds) {
		return plan.Bounds[seq+1] - plan.Bounds[seq]
	}
	return plan.Duration - plan.Bounds[seq]
}

// segmentBounds returns segment start times for a video of the given
// duration: every segmentTarget seconds, or — given the video's keyframe
// times, when it's copied — the first keyframe at or after each multiple.
func segmentBounds(duration float64, keyframes []float64) []float64 {
	bounds := []float64{0}
	for target := segmentTarget; target < duration; target += segmentTarget {
		bound := target
		if keyframes != nil {
			found := false
			for _, keyframe := range keyframes {
				if keyframe >= target-0.0005 {
					bound, found = keyframe, true
					break
				}
			}
			if !found {
				break
			}
		}
		// Skip boundaries that wouldn't start a new segment (two targets
		// reaching the same keyframe) or that sit at the very end.
		if bound > bounds[len(bounds)-1]+0.001 && bound < duration-0.001 {
			bounds = append(bounds, bound)
		}
	}
	return bounds
}

// loadPlan returns the video's plan, working it out (and storing it) the
// first time.
func (h *HLSHandler) loadPlan(ctx context.Context, video hlsVideo) (hlsPlan, error) {
	planPath := filepath.Join(video.cacheDir, "plan.json")
	var plan hlsPlan
	if data, err := os.ReadFile(planPath); err == nil && json.Unmarshal(data, &plan) == nil && len(plan.Bounds) > 0 {
		return plan, nil
	}

	h.planMu.Lock()
	defer h.planMu.Unlock()
	if data, err := os.ReadFile(planPath); err == nil && json.Unmarshal(data, &plan) == nil && len(plan.Bounds) > 0 {
		return plan, nil // worked out meanwhile
	}

	duration, err := probeDuration(ctx, video.absPath)
	if err != nil || duration <= 0 {
		return hlsPlan{}, fmt.Errorf("could not determine duration")
	}
	plan = hlsPlan{
		VideoCodec: videoCodecFor(ctx, video.absPath),
		AudioCodec: audioCodecFor(ctx, video.absPath),
		Duration:   duration,
	}
	var keyframes []float64
	if plan.VideoCodec == "copy" {
		if keyframes, err = probeKeyframes(ctx, video.absPath); err != nil {
			return hlsPlan{}, err
		}
	}
	plan.Bounds = segmentBounds(duration, keyframes)

	// Segments cached without a plan (by older versions) may be cut elsewhere.
	_ = os.RemoveAll(video.cacheDir)
	if err := os.MkdirAll(video.cacheDir, 0o755); err != nil {
		return hlsPlan{}, err
	}
	data, _ := json.Marshal(plan)
	tmp := planPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return hlsPlan{}, err
	}
	return plan, os.Rename(tmp, planPath)
}

// videoCodecFor returns "copy" when the video stream can go into MPEG-TS and
// be decoded by browsers as it is, otherwise the H.264 encoder. MPEG-4 Part 2
// (DivX/Xvid), MPEG-2 (DVD, .mpg, broadcast .m2ts), WMV and others must be
// transcoded.
func videoCodecFor(ctx context.Context, absPath string) string {
	switch probeCodec(ctx, absPath, "v:0") {
	case "h264", "hevc":
		return "copy"
	}
	return "libx264"
}

// audioCodecFor returns "copy" for audio already MPEG-TS compatible, otherwise AAC.
func audioCodecFor(ctx context.Context, absPath string) string {
	switch probeCodec(ctx, absPath, "a:0") {
	case "aac", "mp3":
		return "copy"
	}
	return "aac"
}

func probeCodec(ctx context.Context, absPath, stream string) string {
	out, err := media.FFprobeContext(ctx, absPath,
		"-v", "error",
		"-select_streams", stream,
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
	).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// probeKeyframes returns the times of the video stream's keyframes, in order.
func probeKeyframes(ctx context.Context, absPath string) ([]float64, error) {
	out, err := media.FFprobeContext(ctx, absPath,
		"-v", "error",
		"-select_streams", "v:0",
		"-skip_frame", "nokey",
		"-show_entries", "frame=pts_time",
		"-of", "csv=p=0",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("probing keyframes: %w", err)
	}
	var keyframes []float64
	for _, line := range strings.Split(string(out), "\n") {
		if t, err := strconv.ParseFloat(strings.Trim(strings.TrimSpace(line), ","), 64); err == nil {
			keyframes = append(keyframes, t)
		}
	}
	return keyframes, nil
}

// GET /api/v1/files/hls/segment?path=...&seq=0
func (h *HLSHandler) Segment(c *gin.Context) {
	video, ok := h.requestVideo(c)
	if !ok {
		return
	}
	seq, err := strconv.Atoi(c.Query("seq"))
	if err != nil || seq < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sequence"})
		return
	}
	plan, err := h.loadPlan(c.Request.Context(), video)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to probe video"})
		return
	}
	if seq >= len(plan.Bounds) {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such segment"})
		return
	}
	touchCache(video.cacheDir)

	deadline := time.Now().Add(segmentWait)
	for {
		if chunkPath, ok := findSegment(video.cacheDir, seq); ok {
			h.markUsed(video.hash)
			c.Header("Cache-Control", "public, max-age=604800") // 1 week
			c.File(chunkPath)
			return
		}
		if err := h.ensureGenerating(video, plan, seq); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start generator"})
			return
		}
		if time.Now().After(deadline) || c.Request.Context().Err() != nil {
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "chunk generation timeout"})
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func chunkName(seq int) string { return fmt.Sprintf("chunk_%03d.ts", seq) }

// findSegment returns the file of segment seq if a transcoder run has
// finished it: ffmpeg lists a segment in its run's segment list only once
// it's complete.
func findSegment(cacheDir string, seq int) (string, bool) {
	lists, _ := filepath.Glob(filepath.Join(cacheDir, "run-*", "segments.csv"))
	for _, list := range lists {
		if listed(list, chunkName(seq)) {
			return filepath.Join(filepath.Dir(list), chunkName(seq)), true
		}
	}
	return "", false
}

// segmentList reads a segment list ("chunk_007.ts,70.000,80.000" lines).
func segmentList(path string) [][]string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	records, _ := csv.NewReader(file).ReadAll()
	return records
}

func listed(list, chunk string) bool {
	for _, record := range segmentList(list) {
		if len(record) > 0 && record[0] == chunk {
			return true
		}
	}
	return false
}

// lastSegmentIn returns the highest segment number listed, or -1.
func lastSegmentIn(list string) int {
	last := -1
	for _, record := range segmentList(list) {
		var seq int
		if len(record) == 0 {
			continue
		}
		if _, err := fmt.Sscanf(record[0], "chunk_%d.ts", &seq); err == nil {
			last = max(last, seq)
		}
	}
	return last
}

// markUsed records that the video's transcoder (if any) was just needed.
func (h *HLSHandler) markUsed(hash string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if running, ok := h.active[hash]; ok {
		running.lastUsed = time.Now()
	}
}

// ensureGenerating makes sure segment seq will be written: a running
// transcoder that will reach it soon is left to do so; otherwise one is
// (re)started at seq.
func (h *HLSHandler) ensureGenerating(video hlsVideo, plan hlsPlan, seq int) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if running, ok := h.active[video.hash]; ok {
		written := lastSegmentIn(filepath.Join(running.runDir, "segments.csv"))
		if written < running.startSeq {
			written = running.startSeq - 1
		}
		if seq >= running.startSeq && seq <= written+segmentLookahead {
			running.lastUsed = time.Now()
			return nil
		}
		h.stop(video.hash)
	}
	for len(h.active) >= maxTranscoders {
		h.stop(h.leastRecentlyUsed())
	}

	runDir := filepath.Join(video.cacheDir, fmt.Sprintf("run-%d-%d", seq, time.Now().UnixNano()))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	touchCache(video.cacheDir)

	cmd := media.FFmpeg(context.Background(), video.absPath,
		transcodeInputOptions(plan, seq),
		transcodeOutputArgs(plan, seq, runDir)...)
	if err := cmd.Start(); err != nil {
		return err
	}
	running := &transcoder{cmd: cmd, startSeq: seq, runDir: runDir, lastUsed: time.Now()}
	h.active[video.hash] = running

	go func() {
		_ = cmd.Wait()
		h.mu.Lock()
		if h.active[video.hash] == running {
			delete(h.active, video.hash)
		}
		h.mu.Unlock()
	}()
	return nil
}

// transcodeInputOptions: -copyts keeps the video's own timestamps, so every
// run cuts at the same plan boundaries; -ss starts at segment seq. A copied
// video's seek can land on any earlier keyframe (some formats only index a
// few), so it seeks to just before the boundary keyframe and the output drops
// what comes before it (see transcodeOutputArgs). -fflags +genpts regenerates
// missing timestamps, which AVI files need.
func transcodeInputOptions(plan hlsPlan, seq int) []string {
	options := []string{"-y", "-copyts", "-fflags", "+genpts"}
	if seq > 0 {
		start := plan.Bounds[seq]
		if plan.VideoCodec == "copy" {
			start -= 0.0005
		}
		options = append(options, "-ss", formatSeconds(start))
	}
	return options
}

// transcodeOutputArgs writes segments seq… into runDir with the segment
// muxer, cut just before each later plan boundary (counted from this run's
// first packet), and lists each finished segment in runDir/segments.csv.
// -copypriorss 0 drops copied packets from before the seek point, so a
// copied video's first packet is the keyframe at plan.Bounds[seq].
func transcodeOutputArgs(plan hlsPlan, seq int, runDir string) []string {
	args := []string{"-c:v", plan.VideoCodec, "-c:a", plan.AudioCodec, "-copypriorss", "0"}
	var cuts, keyframes []string
	for _, bound := range plan.Bounds[seq+1:] {
		cuts = append(cuts, formatSeconds(bound-plan.Bounds[seq]-0.001))
		keyframes = append(keyframes, formatSeconds(bound))
	}
	if plan.VideoCodec != "copy" && len(keyframes) > 0 {
		args = append(args, "-force_key_frames", strings.Join(keyframes, ","))
	}
	args = append(args, "-muxdelay", "0", "-f", "segment", "-segment_format", "mpegts")
	if len(cuts) > 0 {
		args = append(args, "-segment_times", strings.Join(cuts, ","))
	} else {
		args = append(args, "-segment_time", formatSeconds(plan.Duration+segmentTarget))
	}
	return append(args,
		"-segment_start_number", strconv.Itoa(seq),
		"-segment_list", filepath.Join(runDir, "segments.csv"), "-segment_list_type", "csv",
		filepath.Join(runDir, "chunk_%03d.ts"),
	)
}

func formatSeconds(seconds float64) string {
	return strconv.FormatFloat(seconds, 'f', 6, 64)
}

// stop ends the video's transcoder; h.mu must be held.
func (h *HLSHandler) stop(hash string) {
	if running, ok := h.active[hash]; ok {
		_ = running.cmd.Process.Kill()
		delete(h.active, hash)
	}
}

// leastRecentlyUsed returns the hash of the transcoder idle longest; h.mu must be held.
func (h *HLSHandler) leastRecentlyUsed() string {
	var oldest string
	var oldestTime time.Time
	for hash, running := range h.active {
		if oldest == "" || running.lastUsed.Before(oldestTime) {
			oldest, oldestTime = hash, running.lastUsed
		}
	}
	return oldest
}

// stopIdle stops transcoders nobody has asked anything of for a while.
func (h *HLSHandler) stopIdle() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for hash, running := range h.active {
		if time.Since(running.lastUsed) > transcoderIdleTimeout {
			h.stop(hash)
		}
	}
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

// touchCache marks the video's cache folder as used (old ones are removed).
func touchCache(cacheDir string) {
	accessFile := filepath.Join(cacheDir, "last_accessed")
	now := time.Now()
	if err := os.Chtimes(accessFile, now, now); err != nil {
		_ = os.WriteFile(accessFile, nil, 0o644)
	}
}

func (h *HLSHandler) cleanupRoutine() {
	idle := time.NewTicker(15 * time.Second)
	old := time.NewTicker(time.Hour)
	for {
		select {
		case <-idle.C:
			h.stopIdle()
		case <-old.C:
			h.cleanupOldCache()
		}
	}
}

// cleanupOldCache removes the cached segments of videos not played for
// cacheMaxAge, except those being transcoded.
func (h *HLSHandler) cleanupOldCache() {
	if h.cacheDir == "" {
		return
	}
	entries, err := os.ReadDir(h.cacheDir)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, entry := range entries {
		if _, running := h.active[entry.Name()]; running || !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(h.cacheDir, entry.Name())
		info, err := os.Stat(filepath.Join(dirPath, "last_accessed"))
		if err == nil && time.Since(info.ModTime()) > cacheMaxAge {
			_ = os.RemoveAll(dirPath)
		}
	}
}
