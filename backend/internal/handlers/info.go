package handlers

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"files/internal/diskusage"
	"files/internal/media"
	"files/internal/userfs"

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
	rels := c.QueryArray("paths[]")

	startEventStream(c)

	type Progress struct {
		Files int64 `json:"files"`
		Dirs  int64 `json:"dirs"`
		Bytes int64 `json:"bytes"`
		Done  bool  `json:"done,omitempty"`
	}

	var totals diskusage.Totals
	n := 0

	flush := func(done bool) {
		writeEvent(c, "", Progress{Files: totals.Files, Dirs: totals.Dirs, Bytes: totals.Apparent, Done: done})
	}

	fsys, err := userFS(c)
	if err != nil {
		flush(true)
		return
	}
	ctx := c.Request.Context()
	for _, rel := range rels {
		name, err := userfs.Clean(rel)
		if err != nil {
			continue
		}
		// The client is gone (dialog closed) when ctx ends: stop walking.
		diskusage.Count(ctx, fsys, name, &totals, func() {
			n++
			if n%50 == 0 {
				flush(false)
			}
		})
		if ctx.Err() != nil {
			return
		}
	}

	flush(true)
}

// GET /api/v1/files/info?path=
//
// Returns media metadata for a single file: image resolution or video
// duration/resolution/codec. Empty object {} if the file has no media info.
func (h *InfoHandler) Meta(c *gin.Context) {
	fsys, name, ok := requestFolderName(c, c.Query("path"))
	if !ok {
		return
	}
	fi, err := fsys.Stat(name)
	if err != nil || fi.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a file"})
		return
	}

	mime := mimeByExt(filepath.Ext(name))
	var result FileMeta

	switch {
	case strings.HasPrefix(mime, "image/"):
		if file, err := fsys.Open(name); err == nil {
			if w, h, err := imageSize(file); err == nil {
				result.Image = &ImageMeta{Width: w, Height: h}
			}
			file.Close()
		}
	case strings.HasPrefix(mime, "video/"):
		// ffprobe gets the real location, checked to be inside the folder.
		if realPath, err := fsys.RealPath(name); err == nil {
			if vm, err := probeVideo(realPath); err == nil {
				result.Video = vm
			}
		}
	}

	c.JSON(http.StatusOK, result)
}

// imageSize reads an image's width and height from its header.
func imageSize(content io.Reader) (int, int, error) {
	cfg, _, err := image.DecodeConfig(content)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

func probeVideo(path string) (*VideoMeta, error) {
	out, err := media.FFprobe(path,
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
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
