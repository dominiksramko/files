package handlers

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"files/internal/thumbs"
)

type FilesHandler struct {
	thumbs *thumbs.Service // nil when thumbnail support is disabled
}

func NewFilesHandler(thumbs *thumbs.Service) *FilesHandler {
	return &FilesHandler{thumbs: thumbs}
}

type FileEntry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`     // relative to user root
	IsDir    bool      `json:"is_dir"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"mod_time"`
	Mime     string    `json:"mime"`
	HasThumb bool      `json:"has_thumb"` // true when a ready thumbnail exists
}

// GET /api/v1/files/list?path=
func (h *FilesHandler) List(c *gin.Context) {
	root, rel := userRoot(c), c.Query("path")

	abs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "directory not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	files := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		entryRel := filepath.Join(rel, e.Name())
		mime := ""
		if !e.IsDir() {
			mime = mimeByExt(filepath.Ext(e.Name()))
		}
		fe := FileEntry{
			Name:    e.Name(),
			Path:    entryRel,
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			Mime:    mime,
		}
		// Check / enqueue thumbnail for thumbnailable files.
		if h.thumbs != nil && !e.IsDir() && thumbs.IsThumbnailable(mime) {
			entryAbs := filepath.Join(abs, e.Name())
			fe.HasThumb = h.thumbs.CheckAndEnqueue(
				entryAbs, entryRel, abs, mime, info.ModTime().Unix(),
			)
		}
		files = append(files, fe)
	}

	// Folders first, then alphabetical
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	c.JSON(http.StatusOK, files)
}

// GET /api/v1/files/download?path=
func (h *FilesHandler) Download(c *gin.Context) {
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

	c.FileAttachment(abs, filepath.Base(abs))
}

// GET /api/v1/files/stream?path=   (range-aware, for media)
func (h *FilesHandler) Stream(c *gin.Context) {
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

	mime := mimeByExt(filepath.Ext(abs))
	if needsRemux(mime) {
		streamRemuxed(c, abs)
		return
	}

	c.File(abs) // Gin handles Range headers automatically
}

// needsRemux reports whether a video format needs to be transcoded to MP4
// because browsers cannot play the raw container or its typical audio codecs.
func needsRemux(mime string) bool {
	switch mime {
	case "video/x-matroska", "video/x-msvideo", "video/quicktime":
		return true
	}
	return false
}

// streamRemuxed pipes the file through FFmpeg as a fragmented MP4 stream.
// Video is copied as-is; audio is re-encoded to AAC for broad browser support.
func streamRemuxed(c *gin.Context, absPath string) {
	cmd := exec.CommandContext(c.Request.Context(), "ffmpeg",
		"-i", absPath,
		"-c:v", "copy",
		"-c:a", "aac",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"-f", "mp4",
		"pipe:1",
	)
	c.Header("Content-Type", "video/mp4")
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	cmd.Stdout = c.Writer
	cmd.Run() //nolint:errcheck — client disconnect is the normal termination path
}

// GET /api/v1/files/search?q=query&path=optional_subfolder
func (h *FilesHandler) Search(c *gin.Context) {
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	basePath := c.Query("path")
	root := userRoot(c)

	if query == "" {
		c.JSON(http.StatusOK, []FileEntry{})
		return
	}

	abs, err := safePath(root, basePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	const maxResults = 200
	results := make([]FileEntry, 0, 32)

	_ = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == abs {
			return nil
		}
		if len(results) >= maxResults {
			return filepath.SkipAll
		}
		if strings.Contains(strings.ToLower(d.Name()), query) {
			info, ierr := d.Info()
			if ierr != nil {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			mime := ""
			if !d.IsDir() {
				mime = mimeByExt(filepath.Ext(d.Name()))
			}
			results = append(results, FileEntry{
				Name:    d.Name(),
				Path:    rel,
				IsDir:   d.IsDir(),
				Size:    info.Size(),
				ModTime: info.ModTime(),
				Mime:    mime,
			})
		}
		return nil
	})

	c.JSON(http.StatusOK, results)
}

// helpers

func userRoot(c *gin.Context) string {
	return c.GetString("rootDir")
}

// safePath resolves rel inside root and rejects path traversal.
func safePath(root, rel string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("user has no root directory configured")
	}
	abs := filepath.Clean(filepath.Join(root, rel))
	if !strings.HasPrefix(abs, filepath.Clean(root)) {
		return "", fmt.Errorf("path traversal detected")
	}
	return abs, nil
}

func mimeByExt(ext string) string {
	ext = strings.ToLower(ext)
	switch ext {
	// Images
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	// Videos
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".mov":
		return "video/quicktime"
	// Audio
	case ".mp3":
		return "audio/mpeg"
	case ".ogg":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	case ".aac":
		return "audio/aac"
	case ".m4a":
		return "audio/mp4"
	// Office
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".ppt":
		return "application/vnd.ms-powerpoint"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".pdf":
		return "application/pdf"
	// Archives
	case ".zip":
		return "application/zip"
	case ".tar":
		return "application/x-tar"
	case ".gz", ".tgz":
		return "application/gzip"
	case ".bz2":
		return "application/x-bzip2"
	case ".xz":
		return "application/x-xz"
	case ".rar":
		return "application/x-rar-compressed"
	case ".7z":
		return "application/x-7z-compressed"
	// Text
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "text/javascript"
	default:
		return "application/octet-stream"
	}
}
