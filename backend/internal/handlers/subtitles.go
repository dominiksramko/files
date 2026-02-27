package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// textSubtitleCodecs are codecs ffmpeg can losslessly extract to WebVTT.
// Bitmap-based codecs (hdmv_pgs_subtitle, dvd_subtitle, dvbsub) are excluded.
var textSubtitleCodecs = map[string]bool{
	"subrip":   true,
	"ass":      true,
	"ssa":      true,
	"webvtt":   true,
	"mov_text": true,
	"tx3g":     true,
	"text":     true,
}

var subtitleFileExts = map[string]bool{
	".srt": true,
	".vtt": true,
	".ass": true,
	".ssa": true,
}

type SubtitlesHandler struct{}

func NewSubtitlesHandler() *SubtitlesHandler {
	return &SubtitlesHandler{}
}

// SubtitleTrack is the shape returned by the List endpoint.
// url is a /api/v1/files/subtitle?… path without a token; the caller appends &token=.
type SubtitleTrack struct {
	Label   string `json:"label"`
	Srclang string `json:"srclang"`
	URL     string `json:"url"`
}

// GET /api/v1/files/subtitles?path=<rel>
//
// Discovers all subtitle tracks for the given video file:
//  1. Sibling subtitle files whose name starts with the video's base name.
//  2. Text-based streams embedded in the file (requires ffprobe; skipped gracefully if absent).
func (h *SubtitlesHandler) List(c *gin.Context) {
	root, rel := userRoot(c), c.Query("path")
	abs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var tracks []SubtitleTrack

	// ── File-based subtitles ──────────────────────────────────────────────────
	dir := filepath.Dir(abs)
	base := strings.TrimSuffix(filepath.Base(abs), filepath.Ext(filepath.Base(abs)))
	baseLower := strings.ToLower(base)

	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			nameLower := strings.ToLower(name)
			ext := strings.ToLower(filepath.Ext(name))
			if !subtitleFileExts[ext] {
				continue
			}
			// Must be <base>.<anything>.<ext>  or  <base>.<ext>
			if !strings.HasPrefix(nameLower, baseLower+".") {
				continue
			}
			// Language tag: text between the base dot and the final extension.
			// e.g. "movie.en.srt" → "en", "movie.srt" → ""
			inner := name[len(base)+1 : len(name)-len(ext)]

			label, srclang := subtitleLangInfo(inner)

			subAbs := filepath.Join(dir, name)
			subRel, err := filepath.Rel(root, subAbs)
			if err != nil {
				continue
			}
			subRel = filepath.ToSlash(subRel)

			tracks = append(tracks, SubtitleTrack{
				Label:   label,
				Srclang: srclang,
				URL:     "/api/v1/files/subtitle?path=" + url.QueryEscape(subRel),
			})
		}
	}

	// ── Embedded subtitle streams (ffprobe) ───────────────────────────────────
	if embedded, err := probeSubtitles(abs); err == nil {
		for _, s := range embedded {
			label, srclang := subtitleLangInfo(s.language)
			if s.title != "" && !strings.EqualFold(s.title, s.language) {
				label = s.title
			}
			trackURL := fmt.Sprintf(
				"/api/v1/files/subtitle?path=%s&index=%d",
				url.QueryEscape(rel), s.index,
			)
			tracks = append(tracks, SubtitleTrack{
				Label:   label + " [embedded]",
				Srclang: srclang,
				URL:     trackURL,
			})
		}
	}

	if tracks == nil {
		tracks = []SubtitleTrack{}
	}
	c.JSON(http.StatusOK, tracks)
}

// GET /api/v1/files/subtitle?path=<rel>[&index=<n>]
//
// Streams a single subtitle track as WebVTT:
//   - ?path=sub.srt          → reads the .srt file, converts to VTT in memory
//   - ?path=sub.vtt          → serves the .vtt file directly
//   - ?path=sub.ass          → ffmpeg converts ASS → VTT
//   - ?path=file.mkv&index=2 → ffmpeg extracts embedded stream 2 → VTT
func (h *SubtitlesHandler) Stream(c *gin.Context) {
	root, rel := userRoot(c), c.Query("path")
	abs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if idxStr := c.Query("index"); idxStr != "" {
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid index"})
			return
		}
		out, err := ffmpegToVTT(abs, idx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ffmpeg extraction failed"})
			return
		}
		c.Data(http.StatusOK, "text/vtt; charset=utf-8", out)
		return
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	switch strings.ToLower(filepath.Ext(abs)) {
	case ".vtt":
		c.Data(http.StatusOK, "text/vtt; charset=utf-8", data)
	case ".srt":
		c.Data(http.StatusOK, "text/vtt; charset=utf-8", srtToVTT(data))
	default:
		// ASS / SSA / anything else — let ffmpeg convert from the file
		cmd := exec.Command("ffmpeg", "-v", "quiet", "-i", abs, "-f", "webvtt", "pipe:1")
		out, err := cmd.Output()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "subtitle conversion failed"})
			return
		}
		c.Data(http.StatusOK, "text/vtt; charset=utf-8", out)
	}
}

// ── Internal helpers ──────────────────────────────────────────────────────────

type embeddedSub struct {
	index    int
	language string // BCP-47 tag
	title    string
}

func probeSubtitles(path string) ([]embeddedSub, error) {
	cmd := exec.Command("ffprobe", "-v", "quiet",
		"-print_format", "json", "-show_streams", "-select_streams", "s", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var probe struct {
		Streams []struct {
			Index     int               `json:"index"`
			CodecName string            `json:"codec_name"`
			Tags      map[string]string `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, err
	}

	var results []embeddedSub
	for _, s := range probe.Streams {
		if !textSubtitleCodecs[s.CodecName] {
			continue
		}
		lang := s.Tags["language"]
		if lang == "" {
			lang = "und"
		}
		results = append(results, embeddedSub{
			index:    s.Index,
			language: iso6392toBCP47(lang),
			title:    s.Tags["title"],
		})
	}
	return results, nil
}

// ffmpegToVTT extracts stream at streamIndex from path and returns WebVTT bytes.
func ffmpegToVTT(path string, streamIndex int) ([]byte, error) {
	return exec.Command(
		"ffmpeg", "-v", "quiet",
		"-i", path,
		"-map", fmt.Sprintf("0:%d", streamIndex),
		"-f", "webvtt", "pipe:1",
	).Output()
}

// srtToVTT converts a SubRip (.srt) payload to WebVTT in memory.
var srtTimestamp = regexp.MustCompile(`(\d{2}:\d{2}:\d{2}),(\d{3})`)

func srtToVTT(srt []byte) []byte {
	s := string(srt)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = srtTimestamp.ReplaceAllString(strings.TrimSpace(s), "$1.$2")
	return []byte("WEBVTT\n\n" + s + "\n")
}

// subtitleLangInfo maps a language tag from a filename segment or stream tag
// to a human-readable label and a BCP-47 srclang value.
func subtitleLangInfo(tag string) (label, srclang string) {
	if tag == "" {
		return "Default", ""
	}
	type info struct{ label, code string }
	table := map[string]info{
		"en": {"English", "en"}, "eng": {"English", "en"},
		"fr": {"French", "fr"}, "fra": {"French", "fr"}, "fre": {"French", "fr"},
		"de": {"German", "de"}, "deu": {"German", "de"}, "ger": {"German", "de"},
		"es": {"Spanish", "es"}, "spa": {"Spanish", "es"},
		"it": {"Italian", "it"}, "ita": {"Italian", "it"},
		"pt": {"Portuguese", "pt"}, "por": {"Portuguese", "pt"},
		"ru": {"Russian", "ru"}, "rus": {"Russian", "ru"},
		"ja": {"Japanese", "ja"}, "jpn": {"Japanese", "ja"},
		"ko": {"Korean", "ko"}, "kor": {"Korean", "ko"},
		"zh": {"Chinese", "zh"}, "zho": {"Chinese", "zh"}, "chi": {"Chinese", "zh"},
		"ar": {"Arabic", "ar"}, "ara": {"Arabic", "ar"},
		"nl": {"Dutch", "nl"}, "nld": {"Dutch", "nl"},
		"pl": {"Polish", "pl"}, "pol": {"Polish", "pl"},
		"sv": {"Swedish", "sv"}, "swe": {"Swedish", "sv"},
		"no": {"Norwegian", "no"}, "nor": {"Norwegian", "no"},
		"da": {"Danish", "da"}, "dan": {"Danish", "da"},
		"fi": {"Finnish", "fi"}, "fin": {"Finnish", "fi"},
		"tr": {"Turkish", "tr"}, "tur": {"Turkish", "tr"},
		"cs": {"Czech", "cs"}, "ces": {"Czech", "cs"}, "cze": {"Czech", "cs"},
		"hu": {"Hungarian", "hu"}, "hun": {"Hungarian", "hu"},
		"hi": {"Hindi", "hi"}, "hin": {"Hindi", "hi"},
		"uk": {"Ukrainian", "uk"}, "ukr": {"Ukrainian", "uk"},
		"he": {"Hebrew", "he"}, "heb": {"Hebrew", "he"},
		"el": {"Greek", "el"}, "ell": {"Greek", "el"},
	}
	if v, ok := table[strings.ToLower(tag)]; ok {
		return v.label, v.code
	}
	sc := strings.ToLower(tag)
	if len(sc) > 2 {
		sc = sc[:2]
	}
	return tag, sc
}

func iso6392toBCP47(code string) string {
	m := map[string]string{
		"eng": "en", "deu": "de", "fra": "fr", "spa": "es",
		"ita": "it", "por": "pt", "rus": "ru", "jpn": "ja",
		"zho": "zh", "kor": "ko", "ara": "ar", "nld": "nl",
		"pol": "pl", "swe": "sv", "nor": "no", "dan": "da",
		"fin": "fi", "hun": "hu", "ces": "cs", "slk": "sk",
		"ron": "ro", "bul": "bg", "hrv": "hr", "srp": "sr",
		"tur": "tr", "heb": "he", "ell": "el", "tha": "th",
		"vie": "vi", "ind": "id", "msa": "ms", "ukr": "uk",
	}
	if v, ok := m[strings.ToLower(code)]; ok {
		return v
	}
	return code
}
