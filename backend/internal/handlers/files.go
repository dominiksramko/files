package handlers

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
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
	thumbs    *thumbs.Service // nil when thumbnail support is disabled
	convCache *convCache      // nil when conversion cache is disabled
}

func NewFilesHandler(thumbsSvc *thumbs.Service, convCacheDir string) *FilesHandler {
	h := &FilesHandler{thumbs: thumbsSvc}
	if convCacheDir != "" {
		cache, err := newConvCache(convCacheDir)
		if err != nil {
			log.Printf("conv cache: disabled (%v)", err)
		} else {
			h.convCache = cache
		}
	}
	return h
}

type FileEntry struct {
	Name       string     `json:"name"`
	Path       string     `json:"path"` // relative to user root
	IsDir      bool       `json:"is_dir"`
	Size       int64      `json:"size"`
	ModTime    time.Time  `json:"mod_time"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	Mime       string     `json:"mime"`
	HasThumb    bool       `json:"has_thumb"`                // true when a ready thumbnail exists
	ThumbHash   string     `json:"thumb_hash,omitempty"`     // content hash, present when has_thumb is true
	ContentHash string     `json:"content_hash,omitempty"`   // content hash for all thumbnailable files
	CanExtract  bool       `json:"can_extract,omitempty"`
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
			// Walk up the path until we find an existing directory.
			fallback := rel
			for {
				parent := filepath.Dir(fallback)
				if parent == "." || parent == fallback {
					fallback = ""
					break
				}
				fallback = parent
				absParent, err2 := safePath(root, fallback)
				if err2 != nil {
					fallback = ""
					break
				}
				if _, err2 := os.Stat(absParent); err2 == nil {
					break
				}
			}
			c.JSON(http.StatusNotFound, gin.H{"error": "Directory not found", "fallback_path": fallback})
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
		entryAbs := filepath.Join(abs, e.Name())
		mime := ""
		if !e.IsDir() {
			mime = mimeByExt(filepath.Ext(e.Name()))
		}
		fe := FileEntry{
			Name:       e.Name(),
			Path:       entryRel,
			IsDir:      e.IsDir(),
			Size:       info.Size(),
			ModTime:    info.ModTime(),
			CreatedAt:  getBirthTime(entryAbs),
			Mime:       mime,
			CanExtract: CanExtract(e.Name()),
		}
		// Set content hash and thumb status for thumbnailable files.
		// ContentHash is always included so the frontend can match thumb events
		// across duplicate files (same content, different names).
		if h.thumbs != nil && !e.IsDir() && thumbs.IsThumbnailable(mime) {
			entryAbsPath := filepath.Join(abs, e.Name())
			if hash, err := h.thumbs.CachedHash(entryAbsPath, info.ModTime().Unix()); err == nil {
				fe.ContentHash = hash
				if _, ok := h.thumbs.HashIfReady(entryAbsPath, info.ModTime().Unix()); ok {
					fe.HasThumb = true
					fe.ThumbHash = hash
				}
			}
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
	if needsImageConversion(mime) && info.Size() <= convertSizeLimit {
		h.convertAndStream(c, abs, mime)
		return
	}

	c.File(abs) // Gin handles Range headers automatically
}

// PUT /api/v1/files/content?path=
func (h *FilesHandler) WriteContent(c *gin.Context) {
	root, rel := userRoot(c), c.Query("path")

	abs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is a directory"})
		return
	}

	const maxSize = 10 * 1024 * 1024 // 10 MB
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxSize+1))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if int64(len(body)) > maxSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "content too large"})
		return
	}

	if err := os.WriteFile(abs, body, 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "saved"})
}

// POST /api/v1/files/download-zip   body: { "paths": ["rel/path1", ...] }
// Zips the requested paths into a temp file, serves it, then deletes it.
func (h *FilesHandler) DownloadZip(c *gin.Context) {
	root := userRoot(c)

	var body struct {
		Paths []string `json:"paths"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths required"})
		return
	}

	tmp, err := os.CreateTemp("", "files-zip-*.zip")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	zw := zip.NewWriter(tmp)
	for _, rel := range body.Paths {
		abs, err := safePath(root, rel)
		if err != nil {
			tmp.Close()
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := addToZip(zw, abs, filepath.Base(abs)); err != nil {
			tmp.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if err := zw.Close(); err != nil {
		tmp.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tmp.Close()

	c.FileAttachment(tmpName, "download.zip")
}

func addToZip(zw *zip.Writer, abs, name string) error {
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(abs)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := addToZip(zw, filepath.Join(abs, e.Name()), name+"/"+e.Name()); err != nil {
				return err
			}
		}
		return nil
	}

	f, err := os.Open(abs)
	if err != nil {
		return err
	}
	defer f.Close()

	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = name
	hdr.Method = zip.Deflate

	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

// convertSizeLimit is the maximum file size for on-the-fly image conversion.
// Files larger than this are served raw; the browser will show its own error.
const convertSizeLimit = 50 * 1024 * 1024 // 50 MB

// needsImageConversion reports whether a format requires server-side conversion
// to JPEG before the browser can display it.
func needsImageConversion(mime string) bool {
	switch mime {
	case "image/heic", "image/heif", "image/tiff", "image/jxl",
		"image/x-icon", "image/vnd.adobe.photoshop", "image/x-tga", "image/x-xcf",
		"image/vnd.radiance", "image/x-exr", "image/vnd.ms-dds":
		return true
	}
	return false
}

// convertAndStream converts srcPath to JPEG via ImageMagick and writes it to
// the response. Results are cached by content hash for convCacheTTL (7 days).
func (h *FilesHandler) convertAndStream(c *gin.Context, srcPath, mime string) {
	args := buildConvertArgs(srcPath, mime)

	// Check cache first.
	if h.convCache != nil {
		if data, hash, ok := h.convCache.get(srcPath); ok {
			_ = hash
			c.Data(http.StatusOK, "image/jpeg", data)
			return
		} else if hash != "" {
			// Hash computed but cache missed — convert and store.
			out, err := exec.Command("convert", args...).Output()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "image conversion failed"})
				return
			}
			h.convCache.put(hash, out)
			c.Data(http.StatusOK, "image/jpeg", out)
			return
		}
	}

	// No cache available — convert and serve without caching.
	out, err := exec.Command("convert", args...).Output()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "image conversion failed"})
		return
	}
	c.Data(http.StatusOK, "image/jpeg", out)
}

// buildConvertArgs returns the ImageMagick convert arguments for previewing
// srcPath as a JPEG. ICO files are composited largest-frame-first so all
// layers appear correctly (same logic as thumbnail generation).
func buildConvertArgs(srcPath, mime string) []string {
	tail := []string{"-flatten", "-quality", "90", "jpeg:-"}
	if mime == "image/x-icon" {
		largest, count := thumbs.LargestICOFrame(srcPath)
		args := []string{"-background", "white", fmt.Sprintf("%s[%d]", srcPath, largest)}
		for i := 0; i < count; i++ {
			if i != largest {
				args = append(args, fmt.Sprintf("%s[%d]", srcPath, i))
			}
		}
		return append(args, tail...)
	}
	return append([]string{"-background", "white", srcPath + "[0]"}, tail...)
}


// needsRemux reports whether a video format needs to be transcoded to HLS/MP4
// because browsers cannot play the raw container or its typical audio codecs.
func needsRemux(mime string) bool {
	switch mime {
	case "video/x-matroska", "video/x-msvideo", "video/quicktime":
		return true
	}
	return false
}

// GET /api/v1/files/search?q=query&path=optional_subfolder
// Streams results as SSE: each match is a "data: <json>\n\n" event,
// followed by "event: done\ndata: {}\n\n" when the walk is complete.
func (h *FilesHandler) Search(c *gin.Context) {
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	basePath := c.Query("path")
	showHidden := c.Query("hidden") == "true"
	root := userRoot(c)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	done := func() {
		fmt.Fprintf(c.Writer, "data: {\"done\":true}\n\n")
		c.Writer.Flush()
	}

	if query == "" {
		done()
		return
	}

	abs, err := safePath(root, basePath)
	if err != nil {
		done()
		return
	}

	ctx := c.Request.Context()
	const maxResults = 200
	count := 0

	_ = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == abs {
			return nil
		}
		if !showHidden && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return filepath.SkipAll
		default:
		}
		if count >= maxResults {
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
			entry := FileEntry{
				Name:       d.Name(),
				Path:       rel,
				IsDir:      d.IsDir(),
				Size:       info.Size(),
				ModTime:    info.ModTime(),
				CreatedAt:  getBirthTime(path),
				Mime:       mime,
				CanExtract: CanExtract(d.Name()),
			}
			data, _ := json.Marshal(entry)
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
			count++
		}
		return nil
	})

	done()
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
	case ".tiff", ".tif":
		return "image/tiff"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	case ".avif":
		return "image/avif"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	case ".jxl":
		return "image/jxl"
	case ".psd":
		return "image/vnd.adobe.photoshop"
	case ".tga":
		return "image/x-tga"
	case ".xcf":
		return "image/x-xcf"
	case ".hdr":
		return "image/vnd.radiance"
	case ".exr":
		return "image/x-exr"
	case ".dds":
		return "image/vnd.ms-dds"
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
	case ".doc", ".dot":
		return "application/msword"
	case ".docx", ".docm", ".dotx", ".dotm":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls", ".xlt":
		return "application/vnd.ms-excel"
	case ".xlsx", ".xlsm", ".xlsb", ".xltx", ".xltm":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".ppt", ".pot", ".pps":
		return "application/vnd.ms-powerpoint"
	case ".pptx", ".pptm", ".potx", ".potm", ".ppsx", ".ppsm":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".odt", ".ott":
		return "application/vnd.oasis.opendocument.text"
	case ".ods", ".ots":
		return "application/vnd.oasis.opendocument.spreadsheet"
	case ".odp", ".otp":
		return "application/vnd.oasis.opendocument.presentation"
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
