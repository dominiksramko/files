package handlers

import (
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSegmentBounds(t *testing.T) {
	near := func(got, want []float64) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if math.Abs(got[i]-want[i]) > 0.001 {
				return false
			}
		}
		return true
	}
	tests := []struct {
		name      string
		duration  float64
		keyframes []float64
		want      []float64
	}{
		{"transcoded: every 10 s", 35, nil, []float64{0, 10, 20, 30}},
		{"transcoded: exact multiple", 30, nil, []float64{0, 10, 20}},
		{"copied: first keyframe at or after each 10 s", 35, []float64{0, 8.333, 16.667, 25, 33.333}, []float64{0, 16.667, 25, 33.333}},
		{"copied: sparse keyframes give fewer segments", 60, []float64{0, 25, 50}, []float64{0, 25, 50}},
		{"copied: one keyframe, one segment", 12, []float64{0}, []float64{0}},
		{"copied: keyframe on a multiple", 25, []float64{0, 10, 20}, []float64{0, 10, 20}},
	}
	for _, test := range tests {
		if got := segmentBounds(test.duration, test.keyframes); !near(got, test.want) {
			t.Errorf("%s: %v, want %v", test.name, got, test.want)
		}
	}
}

// ── With the real tools (the dev container has them) ───────────────────────

func needsFFmpeg(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("needs ffmpeg and ffprobe")
		}
	}
}

// makeVideo encodes a 35 s test video with sound to path, with the given
// ffmpeg output options.
func makeVideo(t *testing.T, path string, options ...string) {
	t.Helper()
	args := append([]string{"-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=35:size=160x90:rate=30",
		"-f", "lavfi", "-i", "sine=duration=35"}, options...)
	if out, err := exec.Command("ffmpeg", append(args, path)...).CombinedOutput(); err != nil {
		t.Fatalf("make %s: %v %s", path, err, out)
	}
}

var extinf = regexp.MustCompile(`#EXTINF:([0-9.]+),`)

func playlistDurations(t *testing.T, handler *HLSHandler, root, video string) []float64 {
	t.Helper()
	response := serve(handler.Playlist, root, httptest.NewRequest(http.MethodGet, "/?path="+video, nil))
	assertStatus(t, response, http.StatusOK)
	var durations []float64
	for _, match := range extinf.FindAllStringSubmatch(response.Body.String(), -1) {
		d, _ := strconv.ParseFloat(match[1], 64)
		durations = append(durations, d)
	}
	return durations
}

// segmentSpan returns the start time and duration of a served segment.
func segmentSpan(t *testing.T, handler *HLSHandler, root, video string, seq int) (float64, float64) {
	t.Helper()
	response := serve(handler.Segment, root, httptest.NewRequest(http.MethodGet, "/?path="+video+"&seq="+strconv.Itoa(seq), nil))
	assertStatus(t, response, http.StatusOK)
	file := filepath.Join(t.TempDir(), "segment.ts")
	os.WriteFile(file, response.Body.Bytes(), 0o644)
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=start_time,duration", "-of", "csv=p=0", file).Output()
	if err != nil {
		t.Fatalf("probe segment %d: %v", seq, err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	start, _ := strconv.ParseFloat(parts[0], 64)
	duration, _ := strconv.ParseFloat(parts[1], 64)
	return start, duration
}

func TestSegmentsMatchThePlaylist(t *testing.T) {
	needsFFmpeg(t)
	// H.264 with B-frames and a keyframe every 8.33 s: copied, cut on keyframes.
	h264 := []string{"-c:v", "libx264", "-bf", "2", "-g", "250", "-keyint_min", "250", "-sc_threshold", "0", "-pix_fmt", "yuv420p"}
	for video, test := range map[string]struct {
		options []string
		want    []float64
	}{
		// Matroska seeks can land a keyframe early; MP4 seeks are exact.
		"copied.mkv": {h264, []float64{16.667, 8.333, 8.333, 1.667}},
		"copied.mp4": {h264, []float64{16.667, 8.333, 8.333, 1.667}},
		// MPEG-4 Part 2: transcoded, keyframes forced every 10 s.
		"transcoded.mkv": {[]string{"-c:v", "mpeg4"}, []float64{10, 10, 10, 5}},
	} {
		name := video
		annRoot, _ := newUserFolders(t)
		makeVideo(t, filepath.Join(annRoot, video), test.options...)
		handler := NewHLSHandler(t.TempDir())

		durations := playlistDurations(t, handler, annRoot, video)
		if len(durations) != len(test.want) {
			t.Fatalf("%s: playlist %v, want %v", name, durations, test.want)
		}
		// Ask for a later segment first (a seek), then all in order.
		order := append([]int{len(durations) - 2}, 0, 1, 2, 3)
		elapsed := map[int]float64{}
		for i := range durations {
			if i > 0 {
				elapsed[i] = elapsed[i-1] + durations[i-1]
			}
		}
		for _, seq := range order {
			start, duration := segmentSpan(t, handler, annRoot, video, seq)
			if math.Abs(duration-test.want[seq]) > 0.1 {
				t.Errorf("%s: segment %d lasts %.3f s, playlist says %.3f", name, seq, duration, durations[seq])
			}
			if math.Abs(start-elapsed[seq]) > 0.15 {
				t.Errorf("%s: segment %d starts at %.3f s, playlist puts it at %.3f", name, seq, start, elapsed[seq])
			}
		}
	}
}

func TestSequentialPlaybackDoesNotRestart(t *testing.T) {
	needsFFmpeg(t)
	annRoot, _ := newUserFolders(t)
	makeVideo(t, filepath.Join(annRoot, "clip.avi"), "-c:v", "mpeg4")
	cacheDir := t.TempDir()
	handler := NewHLSHandler(cacheDir)
	for seq := 0; seq < 4; seq++ {
		segmentSpan(t, handler, annRoot, "clip.avi", seq)
	}
	runs, _ := filepath.Glob(filepath.Join(cacheDir, "*", "run-*"))
	if len(runs) != 1 {
		t.Errorf("%d transcoder runs for sequential playback, want 1: %v", len(runs), runs)
	}
}

func TestIdleTranscodersAreStopped(t *testing.T) {
	needsFFmpeg(t)
	previous := transcoderIdleTimeout
	transcoderIdleTimeout = 100 * time.Millisecond
	t.Cleanup(func() { transcoderIdleTimeout = previous })

	annRoot, _ := newUserFolders(t)
	makeVideo(t, filepath.Join(annRoot, "clip.avi"), "-c:v", "mpeg4")
	handler := NewHLSHandler(t.TempDir())
	segmentSpan(t, handler, annRoot, "clip.avi", 0)

	time.Sleep(200 * time.Millisecond)
	handler.stopIdle()
	handler.mu.Lock()
	running := len(handler.active)
	handler.mu.Unlock()
	if running != 0 {
		t.Errorf("%d transcoders still running after going idle", running)
	}
}

func TestTranscoderLimit(t *testing.T) {
	needsFFmpeg(t)
	previous := maxTranscoders
	maxTranscoders = 1
	t.Cleanup(func() { maxTranscoders = previous })

	annRoot, _ := newUserFolders(t)
	for _, name := range []string{"one.avi", "two.avi"} {
		makeVideo(t, filepath.Join(annRoot, name), "-c:v", "mpeg4")
	}
	handler := NewHLSHandler(t.TempDir())
	segmentSpan(t, handler, annRoot, "one.avi", 0)
	segmentSpan(t, handler, annRoot, "two.avi", 0)
	handler.mu.Lock()
	running := len(handler.active)
	handler.mu.Unlock()
	if running > 1 {
		t.Errorf("%d transcoders running, limit 1", running)
	}
}

func TestOldCacheIsRemoved(t *testing.T) {
	cacheDir := t.TempDir()
	handler := NewHLSHandler(cacheDir)
	longAgo := time.Now().Add(-cacheMaxAge - time.Hour)
	for name, lastPlayed := range map[string]time.Time{"old": longAgo, "recent": time.Now(), "playing": longAgo} {
		writeFiles(t, filepath.Join(cacheDir, name), map[string]string{"last_accessed": ""})
		os.Chtimes(filepath.Join(cacheDir, name, "last_accessed"), lastPlayed, lastPlayed)
	}
	handler.active["playing"] = &transcoder{}

	handler.cleanupOldCache()
	assertMissing(t, filepath.Join(cacheDir, "old"))
	for _, kept := range []string{"recent", "playing"} {
		if _, err := os.Stat(filepath.Join(cacheDir, kept)); err != nil {
			t.Errorf("%s cache removed: %v", kept, err)
		}
	}
}

func TestSegmentsCachedWithoutPlanAreDropped(t *testing.T) {
	needsFFmpeg(t)
	annRoot, _ := newUserFolders(t)
	makeVideo(t, filepath.Join(annRoot, "clip.avi"), "-c:v", "mpeg4")
	cacheDir := t.TempDir()
	handler := NewHLSHandler(cacheDir)
	playlistDurations(t, handler, annRoot, "clip.avi")

	plans, _ := filepath.Glob(filepath.Join(cacheDir, "*", "plan.json"))
	if len(plans) != 1 {
		t.Fatalf("plans: %v", plans)
	}
	videoCache := filepath.Dir(plans[0])
	os.Remove(plans[0])
	writeFiles(t, videoCache, map[string]string{"chunk_000.ts": "from an older version"})

	playlistDurations(t, handler, annRoot, "clip.avi")
	assertMissing(t, filepath.Join(videoCache, "chunk_000.ts"))
	if _, err := os.Stat(plans[0]); err != nil {
		t.Errorf("plan not written again: %v", err)
	}
}

func TestCacheIsTrimmedToItsSizeLimit(t *testing.T) {
	cacheDir := t.TempDir()
	handler := NewHLSHandler(cacheDir)
	handler.SetMaxCacheBytes(250)
	now := time.Now()
	for name, played := range map[string]time.Time{
		"oldest":  now.Add(-3 * time.Hour),
		"older":   now.Add(-2 * time.Hour),
		"recent":  now.Add(-time.Hour),
		"playing": now.Add(-4 * time.Hour),
	} {
		writeFiles(t, filepath.Join(cacheDir, name), map[string]string{
			"last_accessed":      "",
			"run-0/chunk_000.ts": strings.Repeat("x", 100),
		})
		os.Chtimes(filepath.Join(cacheDir, name, "last_accessed"), played, played)
	}
	handler.active["playing"] = &transcoder{}

	handler.trimCache()
	for name, kept := range map[string]bool{"oldest": false, "older": false, "recent": true, "playing": true} {
		if _, err := os.Stat(filepath.Join(cacheDir, name)); (err == nil) != kept {
			t.Errorf("%s: kept = %v, want %v", name, err == nil, kept)
		}
	}
}
