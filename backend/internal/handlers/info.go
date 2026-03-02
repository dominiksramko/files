package handlers

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

type InfoHandler struct{}

func NewInfoHandler() *InfoHandler { return &InfoHandler{} }

type ImageMeta struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type VideoMeta struct {
	Duration float64 `json:"duration"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Codec    string  `json:"codec,omitempty"`
}

type FileMeta struct {
	Image *ImageMeta `json:"image,omitempty"`
	Video *VideoMeta `json:"video,omitempty"`
}

// GET /api/v1/files/size?paths[]=rel1&paths[]=rel2
//
// Streams SSE events with a running total of files, directories, and bytes
// as it recursively walks all requested paths. Clients should use EventSource.
func (h *InfoHandler) SizeStream(c *gin.Context) {
	root := userRoot(c)
	rels := c.QueryArray("paths[]")

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no") // disable nginx buffering

	type Progress struct {
		Files int64 `json:"files"`
		Dirs  int64 `json:"dirs"`
		Bytes int64 `json:"bytes"`
		Done  bool  `json:"done,omitempty"`
	}

	var p Progress
	n := 0

	flush := func(done bool) {
		p.Done = done
		data, _ := json.Marshal(p)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
	}

	for _, rel := range rels {
		abs, err := safePath(root, rel)
		if err != nil {
			continue
		}
		fi, err := os.Stat(abs)
		if err != nil {
			continue
		}
		if !fi.IsDir() {
			p.Files++
			p.Bytes += fi.Size()
			n++
			continue
		}
		filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if path != abs {
					p.Dirs++
				}
			} else {
				p.Files++
				if info, err := d.Info(); err == nil {
					p.Bytes += info.Size()
				}
			}
			n++
			if n%50 == 0 {
				flush(false)
			}
			return nil
		})
	}

	flush(true)
}

// GET /api/v1/files/info?path=
//
// Returns media metadata for a single file: image resolution or video
// duration/resolution/codec. Empty object {} if the file has no media info.
func (h *InfoHandler) Meta(c *gin.Context) {
	root := userRoot(c)
	rel := c.Query("path")

	abs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	fi, err := os.Stat(abs)
	if err != nil || fi.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a file"})
		return
	}

	mime := mimeByExt(filepath.Ext(abs))
	var result FileMeta

	switch {
	case strings.HasPrefix(mime, "image/"):
		if w, h, err := probeImage(abs); err == nil {
			result.Image = &ImageMeta{Width: w, Height: h}
		}
	case strings.HasPrefix(mime, "video/"):
		if vm, err := probeVideo(abs); err == nil {
			result.Video = vm
		}
	}

	c.JSON(http.StatusOK, result)
}

func probeImage(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

func probeVideo(path string) (*VideoMeta, error) {
	out, err := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		path,
	).Output()
	if err != nil {
		return nil, err
	}

	var probe struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			Duration  string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, err
	}

	vm := &VideoMeta{}
	for _, s := range probe.Streams {
		if s.CodecType == "video" {
			vm.Width = s.Width
			vm.Height = s.Height
			vm.Codec = s.CodecName
			if s.Duration != "" {
				vm.Duration, _ = strconv.ParseFloat(s.Duration, 64)
			}
			break
		}
	}
	if vm.Duration == 0 && probe.Format.Duration != "" {
		vm.Duration, _ = strconv.ParseFloat(probe.Format.Duration, 64)
	}
	if vm.Width == 0 {
		return nil, fmt.Errorf("no video stream found")
	}
	return vm, nil
}
