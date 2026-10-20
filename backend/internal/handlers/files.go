package handlers

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
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
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"files/internal/thumbs"
)

type FilesHandler struct {
	thumbs    *thumbs.Service // nil when thumbnail support is disabled
	convCache *convCache      // nil when conversion cache is disabled

	// Serialises the compare-and-write in WriteContent, so two saves based on the
	// same version can't both pass the check.
	contentWriteMu sync.Mutex
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

	files, err := h.listDir(abs, rel)
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

	c.JSON(http.StatusOK, files)
}

// listDir returns the entries of the folder at abs (rel is its path relative to
// the user root), folders first, then alphabetical.
func (h *FilesHandler) listDir(abs, rel string) ([]FileEntry, error) {
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}

	files := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, h.newFileEntry(filepath.Join(abs, e.Name()), filepath.Join(rel, e.Name()), info))
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	return files, nil
}

// newFileEntry describes one file or folder. ContentHash is set for every
// thumbnailable file so the frontend can match thumb events across duplicate
// files (same content, different names); HasThumb/ThumbHash once it is ready.
func (h *FilesHandler) newFileEntry(absPath, relPath string, info os.FileInfo) FileEntry {
	mime := ""
	if !info.IsDir() {
		mime = mimeByExt(filepath.Ext(info.Name()))
	}
	fe := FileEntry{
		Name:       info.Name(),
		Path:       relPath,
		IsDir:      info.IsDir(),
		Size:       info.Size(),
		ModTime:    info.ModTime(),
		CreatedAt:  getBirthTime(absPath),
		Mime:       mime,
		CanExtract: CanExtract(info.Name()),
	}
	if h.thumbs != nil && !info.IsDir() && thumbs.IsThumbnailable(mime) {
		if hash, err := h.thumbs.CachedHash(absPath, info.ModTime().Unix()); err == nil {
			fe.ContentHash = hash
			if _, ok := h.thumbs.HashIfReady(absPath, info.ModTime().Unix()); ok {
				fe.HasThumb = true
				fe.ThumbHash = hash
			}
		}
	}
	return fe
}

// GET /api/v1/files/stat?path=
// Returns a single FileEntry for the given path (file or directory).
func (h *FilesHandler) Stat(c *gin.Context) {
	root, rel := userRoot(c), c.Query("path")

	abs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	fe := h.newFileEntry(abs, rel, info)

	c.JSON(http.StatusOK, fe)
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

	setUserFileHeaders(c, abs)
	mime := mimeByExt(filepath.Ext(abs))
	if needsImageConversion(mime) && info.Size() <= convertSizeLimit {
		h.convertAndStream(c, abs, mime)
		return
	}
	if needsAudioConversion(mime) {
		h.convertAudioAndStream(c, abs)
		return
	}

	c.File(abs) // Gin handles Range headers automatically
}

// maxEditableSize caps what ReadContent serves and WriteContent accepts.
const maxEditableSize = 10 * 1024 * 1024 // 10 MB

// contentVersion identifies one exact content of a file, for optimistic
// concurrency in the editors (sent as ETag, checked against If-Match).
func contentVersion(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}

// GET /api/v1/files/content?path=
// Returns the file's text with its version in the ETag header; editors send that
// version back as If-Match when saving.
func (h *FilesHandler) ReadContent(c *gin.Context) {
	root, rel := userRoot(c), c.Query("path")

	abs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	if info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is a directory"})
		return
	}
	if info.Size() > maxEditableSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file too large"})
		return
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	setUserFileHeaders(c, abs)
	c.Header("ETag", `"`+contentVersion(data)+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", data)
}

// PUT /api/v1/files/content?path=   body: raw file content
// With an If-Match header (the version the edit is based on), the write only
// happens if the file is still at that version; otherwise 412 with the file's
// current content and version, so the client can merge and retry. Without
// If-Match the file is overwritten (used to create new files).
// Responds with the new version and mod_time.
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

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxEditableSize+1))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if int64(len(body)) > maxEditableSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "content too large"})
		return
	}

	h.contentWriteMu.Lock()
	defer h.contentWriteMu.Unlock()

	if baseVersion := strings.Trim(c.GetHeader("If-Match"), `"`); baseVersion != "" {
		current, err := os.ReadFile(abs)
		if err != nil {
			if os.IsNotExist(err) {
				c.JSON(http.StatusNotFound, gin.H{"error": "file no longer exists"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			}
			return
		}
		if currentVersion := contentVersion(current); currentVersion != baseVersion {
			c.JSON(http.StatusPreconditionFailed, gin.H{
				"error":   "file was changed elsewhere",
				"content": string(current),
				"version": currentVersion,
			})
			return
		}
	}

	if err := os.WriteFile(abs, body, 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	info, err := os.Stat(abs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"version": contentVersion(body), "mod_time": info.ModTime()})
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
		if data, hash, ok := h.convCache.get(srcPath, ".jpg"); ok {
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
			h.convCache.put(hash, ".jpg", out)
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

// needsAudioConversion reports whether an audio format must be transcoded
// server-side because browsers cannot play it natively.
func needsAudioConversion(mime string) bool {
	switch mime {
	case "audio/x-ms-wma", "audio/aiff", "audio/x-ape", "audio/x-wavpack",
		"audio/x-matroska", "audio/ac3", "audio/amr", "audio/x-caf":
		return true
	}
	return false
}

// convertAudioAndStream transcodes srcPath to AAC (M4A) via ffmpeg and serves
// the result as a file, so range requests (seeking) keep working. Results are
// cached by content hash for convCacheTTL (7 days).
func (h *FilesHandler) convertAudioAndStream(c *gin.Context, srcPath string) {
	c.Header("Content-Type", "audio/mp4")

	if h.convCache != nil {
		if hash, cachedPath, ok := h.convCache.lookup(srcPath, ".m4a"); ok {
			c.File(cachedPath)
			return
		} else if hash != "" {
			if err := transcodeAudio(srcPath, cachedPath); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "audio conversion failed"})
				return
			}
			c.File(cachedPath)
			return
		}
	}

	// No cache available — convert to a temp file, serve it, then delete it.
	tmp, err := os.CreateTemp("", "files-audio-*.m4a")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if err := transcodeAudio(srcPath, tmp.Name()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "audio conversion failed"})
		return
	}
	c.File(tmp.Name())
}

// transcodeAudio converts the first audio stream of srcPath to AAC in an MP4
// container at destPath. It writes to a unique sibling temp file and renames it
// into place, so concurrent requests for the same file never see a partial result.
func transcodeAudio(srcPath, destPath string) error {
	part, err := os.CreateTemp(filepath.Dir(destPath), filepath.Base(destPath)+".*.part")
	if err != nil {
		return err
	}
	part.Close()
	defer os.Remove(part.Name()) // no-op once renamed

	// nice -n 10: ffmpeg only uses spare CPU, keeping the server responsive.
	cmd := exec.Command("nice", "-n", "10",
		"ffmpeg", "-y", "-v", "error",
		"-i", srcPath,
		"-map", "0:a:0", "-vn",
		"-c:a", "aac", "-b:a", "256k",
		"-movflags", "+faststart",
		"-f", "mp4", part.Name(),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return os.Rename(part.Name(), destPath)
}

// buildConvertArgs returns the ImageMagick convert arguments for previewing
// srcPath as a JPEG. ICO handling mirrors thumbnail generation:
//   - Composite ICO (frames have different aspect ratios): composite all layers.
//   - Variations ICO (same aspect ratio at different sizes): use only the largest frame.
func buildConvertArgs(srcPath, mime string) []string {
	tail := []string{"-flatten", "-quality", "90", "jpeg:-"}
	if mime == "image/x-icon" {
		largestIdx, frames := thumbs.ICOFrameDimensions(srcPath)
		if thumbs.ICOIsComposite(frames) {
			args := []string{"-background", "white", fmt.Sprintf("%s[%d]", srcPath, largestIdx)}
			for i := range frames {
				if i != largestIdx {
					args = append(args, fmt.Sprintf("%s[%d]", srcPath, i))
				}
			}
			return append(args, tail...)
		}
		// Variations: single best frame is sufficient.
		return append([]string{"-background", "white", fmt.Sprintf("%s[%d]", srcPath, largestIdx)}, tail...)
	}
	return append([]string{"-background", "white", srcPath + "[0]"}, tail...)
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

// safePath resolves rel inside root and rejects anything outside it.
func safePath(root, rel string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("user has no root directory configured")
	}
	abs := filepath.Clean(filepath.Join(root, rel))
	if !isWithin(filepath.Clean(root), abs) {
		return "", fmt.Errorf("path traversal detected")
	}
	return abs, nil
}

// safeChildPath is safePath for an item that gets created, replaced, renamed,
// moved or deleted: it also rejects root itself. Those operations act on the
// item's parent folder too, and root's parent holds the other users' folders.
func safeChildPath(root, rel string) (string, error) {
	abs, err := safePath(root, rel)
	if err != nil {
		return "", err
	}
	if abs == filepath.Clean(root) {
		return "", fmt.Errorf("the root folder itself can't be changed")
	}
	return abs, nil
}

// safeChildPaths resolves each of rels with safeChildPath, failing on the first
// bad one — check them all before touching any.
func safeChildPaths(root string, rels []string) ([]string, error) {
	paths := make([]string, 0, len(rels))
	for _, rel := range rels {
		abs, err := safeChildPath(root, rel)
		if err != nil {
			return nil, err
		}
		paths = append(paths, abs)
	}
	return paths, nil
}

// setUserFileHeaders protects a response that returns a user's file inline,
// from the app's own address. The browser must keep the type it's given rather
// than guess one, and the file is sandboxed: an HTML or SVG file opened
// directly, or anything a browser would show as a page, runs isolated from the
// app, with no access to the session. PDFs aren't sandboxed (browsers refuse to
// show them then); a PDF can't reach the page anyway.
func setUserFileHeaders(c *gin.Context, name string) {
	c.Header("X-Content-Type-Options", "nosniff")
	if strings.ToLower(filepath.Ext(name)) != ".pdf" {
		c.Header("Content-Security-Policy", "sandbox")
	}
}

// isWithin reports whether abs is dir or inside it. It compares whole path
// elements, so /users/anna is not within /users/ann.
func isWithin(dir, abs string) bool {
	rel, err := filepath.Rel(dir, abs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
	// Videos (".ts" is deliberately absent — it is far more often TypeScript than MPEG-TS)
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".mov":
		return "video/quicktime"
	case ".ogv":
		return "video/ogg"
	case ".m2ts", ".mts":
		return "video/mp2t"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".asf":
		return "video/x-ms-asf"
	case ".flv":
		return "video/x-flv"
	case ".mpg", ".mpeg", ".vob":
		return "video/mpeg"
	case ".3gp":
		return "video/3gpp"
	// Audio
	case ".mp3":
		return "audio/mpeg"
	case ".ogg", ".oga", ".opus":
		return "audio/ogg"
	case ".weba":
		return "audio/webm"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	case ".aac":
		return "audio/aac"
	case ".m4a":
		return "audio/mp4"
	case ".wma":
		return "audio/x-ms-wma"
	case ".aif", ".aiff", ".aifc":
		return "audio/aiff"
	case ".ape":
		return "audio/x-ape"
	case ".wv":
		return "audio/x-wavpack"
	case ".mka":
		return "audio/x-matroska"
	case ".ac3":
		return "audio/ac3"
	case ".amr":
		return "audio/amr"
	case ".caf":
		return "audio/x-caf"
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
	case ".txt", ".todo", ".kanban":
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
