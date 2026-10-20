package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"files/internal/media"
	"files/internal/thumbs"
	"files/internal/userfs"
)

type FilesHandler struct {
	thumbs    *thumbs.Service // nil when thumbnail support is disabled
	convCache *convCache      // nil when conversion cache is disabled

	// Serialises the compare-and-write in WriteContent, so two saves based on the
	// same version can't both pass the check.
	contentWriteMu sync.Mutex

	// Closed by EndStreams when the server shuts down: open watch streams end
	// so they don't hold the shutdown up (clients reconnect to the new server).
	streamsEnd     chan struct{}
	endStreamsOnce sync.Once

	usage *usageStore // disk usage scans
}

// EndStreams tells open watch streams and disk usage scans to end; for http.Server.RegisterOnShutdown.
func (h *FilesHandler) EndStreams() {
	h.endStreamsOnce.Do(func() { close(h.streamsEnd) })
}

// SetConvCacheMaxBytes limits the conversion cache's total size (0 = no limit).
func (h *FilesHandler) SetConvCacheMaxBytes(maxBytes int64) {
	if h.convCache != nil {
		h.convCache.maxBytes.Store(maxBytes)
	}
}

func NewFilesHandler(thumbsSvc *thumbs.Service, convCacheDir string) *FilesHandler {
	h := &FilesHandler{thumbs: thumbsSvc, streamsEnd: make(chan struct{}), usage: newUsageStore()}
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
	Name        string     `json:"name"`
	Path        string     `json:"path"` // relative to user root
	IsDir       bool       `json:"is_dir"`
	Size        int64      `json:"size"`
	ModTime     time.Time  `json:"mod_time"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	Mime        string     `json:"mime"`
	HasThumb    bool       `json:"has_thumb"`              // true when a ready thumbnail exists
	ThumbHash   string     `json:"thumb_hash,omitempty"`   // content hash, present when has_thumb is true
	ContentHash string     `json:"content_hash,omitempty"` // content hash for all thumbnailable files
	CanExtract  bool       `json:"can_extract,omitempty"`
}

// GET /api/v1/files/list?path=
func (h *FilesHandler) List(c *gin.Context) {
	fsys, name, ok := requestFolderName(c, c.Query("path"))
	if !ok {
		return
	}

	files, err := h.listDir(fsys, name)
	if err != nil {
		if os.IsNotExist(err) {
			// Walk up the path until we find an existing directory.
			fallback := name
			for {
				parent := filepath.Dir(fallback)
				if parent == "." || parent == fallback {
					fallback = ""
					break
				}
				fallback = parent
				if info, err := fsys.Stat(fallback); err == nil && info.IsDir() {
					break
				}
			}
			c.JSON(http.StatusNotFound, gin.H{"error": "Directory not found", "fallback_path": userfs.RequestPath(fallback)})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, files)
}

// listDir returns the entries of the folder dir, folders first, then alphabetical.
func (h *FilesHandler) listDir(fsys *userfs.FS, dir string) ([]FileEntry, error) {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	realDir, err := fsys.RealPath(dir)
	if err != nil {
		return nil, err
	}

	files := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, h.newFileEntry(fsys, realDir, filepath.Join(dir, e.Name()), info))
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	return files, nil
}

// newFileEntry describes the file or folder name, whose info came from a
// listing of the folder whose real location is realDir ("" to look it up).
// A symbolic link is described as what it points to — if it can be followed:
// one leading outside the user's folder (or broken) is listed with no type,
// no thumbnail and nothing read through it. ContentHash is set for every
// thumbnailable file so the frontend can match thumb events across duplicate
// files (same content, different names); HasThumb/ThumbHash once it is ready.
func (h *FilesHandler) newFileEntry(fsys *userfs.FS, realDir, name string, info os.FileInfo) FileEntry {
	realPath := entryRealPath(fsys, realDir, name, info)
	if info.Mode()&os.ModeSymlink != 0 && realPath != "" {
		if target, err := fsys.Stat(name); err == nil {
			info = target
		}
	}
	mime := ""
	if !info.IsDir() && realPath != "" {
		mime = mimeByExt(filepath.Ext(info.Name()))
	}
	fe := FileEntry{
		Name:       info.Name(),
		Path:       userfs.RequestPath(name),
		IsDir:      info.IsDir(),
		Size:       info.Size(),
		ModTime:    info.ModTime(),
		Mime:       mime,
		CanExtract: realPath != "" && CanExtract(info.Name()),
	}
	if realPath == "" {
		return fe
	}
	fe.CreatedAt = getBirthTime(realPath)
	if h.thumbs != nil && !info.IsDir() && thumbs.IsThumbnailable(mime) {
		if hash, err := h.thumbs.CachedHash(realPath, info.ModTime().Unix()); err == nil {
			fe.ContentHash = hash
			if _, ok := h.thumbs.HashIfReady(realPath, info.ModTime().Unix()); ok {
				fe.HasThumb = true
				fe.ThumbHash = hash
			}
		}
	}
	return fe
}

// entryRealPath is where an entry named name really is: realDir/<its name>
// for an ordinary entry of a folder already resolved, otherwise looked up
// through fsys. "" when it can't be followed inside the user's folder.
func entryRealPath(fsys *userfs.FS, realDir, name string, info os.FileInfo) string {
	if realDir != "" && info.Mode()&os.ModeSymlink == 0 {
		return filepath.Join(realDir, info.Name())
	}
	realPath, err := fsys.RealPath(name)
	if err != nil {
		return ""
	}
	return realPath
}

// GET /api/v1/files/stat?path=
// Returns a single FileEntry for the given path (file or directory).
func (h *FilesHandler) Stat(c *gin.Context) {
	fsys, name, ok := requestFolderName(c, c.Query("path"))
	if !ok {
		return
	}

	info, err := fsys.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, h.newFileEntry(fsys, "", name, info))
}

// GET /api/v1/files/download?path=
func (h *FilesHandler) Download(c *gin.Context) {
	fsys, name, ok := requestFolderName(c, c.Query("path"))
	if !ok {
		return
	}
	serveUserFile(c, fsys, name, true)
}

// serveUserFile sends the file name from the user's folder (range-aware), as
// a download when attachment is set, otherwise inline with the headers that
// keep it from running as part of the app (setUserFileHeaders).
func serveUserFile(c *gin.Context, fsys *userfs.FS, name string, attachment bool) {
	file, err := fsys.Open(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	if attachment {
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(name)}))
	} else {
		setUserFileHeaders(c, name)
	}
	http.ServeContent(c.Writer, c.Request, filepath.Base(name), info.ModTime(), file)
}

// GET /api/v1/files/stream?path=   (range-aware, for media)
func (h *FilesHandler) Stream(c *gin.Context) {
	fsys, name, ok := requestFolderName(c, c.Query("path"))
	if !ok {
		return
	}
	info, err := fsys.Stat(name)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	fileMime := mimeByExt(filepath.Ext(name))
	convertImage := needsImageConversion(fileMime) && info.Size() <= convertSizeLimit
	if convertImage || needsAudioConversion(fileMime) {
		// Converters get the real location, checked to be inside the folder.
		realPath, err := fsys.RealPath(name)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		setUserFileHeaders(c, name)
		if convertImage {
			h.convertAndStream(c, realPath, fileMime)
		} else {
			h.convertAudioAndStream(c, realPath)
		}
		return
	}

	serveUserFile(c, fsys, name, false)
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
	fsys, name, ok := requestFolderName(c, c.Query("path"))
	if !ok {
		return
	}

	info, err := fsys.Stat(name)
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

	data, err := fsys.ReadFile(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	setUserFileHeaders(c, name)
	c.Header("ETag", `"`+contentVersion(data)+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", data)
}

// PUT /api/v1/files/content?path=   body: raw file content
// With an If-Match header (the version the edit is based on), the write only
// happens if the file is still at that version; otherwise 412 with the file's
// current content and version, so the client can merge and retry. Without
// If-Match the file is overwritten.
// Responds with the new version and mod_time.
func (h *FilesHandler) WriteContent(c *gin.Context) {
	fsys, name, ok := requestFolderName(c, c.Query("path"))
	if !ok {
		return
	}

	if info, err := fsys.Stat(name); err == nil && info.IsDir() {
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
		current, err := fsys.ReadFile(name)
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

	if err := writeFileAtomically(fsys, name, body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	info, err := fsys.Stat(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"version": contentVersion(body), "mod_time": info.ModTime()})
}

// writeFileAtomically replaces the content of the file name: it writes a
// temporary file next to it and renames it into place, so a read meanwhile
// sees the old content or the new, never half of it, and a crash mid-save
// leaves the old file. An existing file keeps its permissions; a link keeps
// pointing at the file, whose content is what's replaced.
func writeFileAtomically(fsys *userfs.FS, name string, content []byte) error {
	mode := os.FileMode(0o644)
	if info, err := fsys.Stat(name); err == nil {
		mode = info.Mode().Perm()
		if name, err = fsys.ResolveName(name); err != nil {
			return err
		}
	}
	tmp, tmpName, err := createTempFile(fsys, filepath.Dir(name), ".~save_")
	if err != nil {
		return err
	}
	_, err = tmp.Write(content)
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = fsys.Chmod(tmpName, mode)
	}
	if err == nil {
		err = fsys.Rename(tmpName, name)
	}
	if err != nil {
		fsys.Remove(tmpName)
	}
	return err
}

// POST /api/v1/files/download-zip   body: { "paths": ["rel/path1", ...] }
// Streams a zip of the requested paths straight into the response: nothing
// is built in temp space first, and the work stops when the client goes.
func (h *FilesHandler) DownloadZip(c *gin.Context) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths required"})
		return
	}
	names := make([]string, len(body.Paths))
	for i, rel := range body.Paths {
		var ok bool
		if names[i], ok = requestName(c, rel); !ok {
			return
		}
	}
	fsys, err := userFS(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Once the zip starts streaming the status can't change, so check first.
	for _, name := range names {
		if _, err := fsys.Stat(name); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found: " + filepath.Base(name)})
			return
		}
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "download.zip"}))
	c.Status(http.StatusOK)
	if err := writeZip(fsys, names, c.Writer); err != nil {
		// The client sees a cut-off download; nothing more can be sent.
		log.Printf("download-zip: %v", err)
	}
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
	args := buildConvertArgs(c.Request.Context(), srcPath, mime)

	// Check cache first.
	if h.convCache != nil {
		if data, hash, ok := h.convCache.get(srcPath, ".jpg"); ok {
			_ = hash
			c.Data(http.StatusOK, "image/jpeg", data)
			return
		} else if hash != "" {
			// Hash computed but cache missed — convert and store.
			out, err := media.MagickConvert(c.Request.Context(), args...).Output()
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
	out, err := media.MagickConvert(c.Request.Context(), args...).Output()
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

	cmd := media.FFmpeg(context.Background(), srcPath, []string{"-y", "-v", "error"},
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
func buildConvertArgs(ctx context.Context, srcPath, mime string) []string {
	tail := []string{"-flatten", "-quality", "90", "jpeg:-"}
	if mime == "image/x-icon" {
		largestIdx, frames := thumbs.ICOFrameDimensions(ctx, srcPath)
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
// followed by "data: {\"done\":true}" when the walk is complete.
func (h *FilesHandler) Search(c *gin.Context) {
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	basePath := c.Query("path")
	showHidden := c.Query("hidden") == "true"

	startEventStream(c)

	done := func() {
		writeEvent(c, "", gin.H{"done": true})
	}

	if query == "" {
		done()
		return
	}

	base, err := userfs.Clean(basePath)
	if err != nil {
		done()
		return
	}
	fsys, err := userFS(c)
	if err != nil {
		done()
		return
	}
	realBase, err := fsys.RealPath(base)
	if err != nil {
		done()
		return
	}

	ctx := c.Request.Context()
	const maxResults = 200
	count := 0

	_ = fsys.WalkDir(base, func(name string, d fs.DirEntry, err error) error {
		if err != nil || name == base {
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
			rel, _ := filepath.Rel(base, name)
			realPath := entryRealPath(fsys, filepath.Join(realBase, filepath.Dir(rel)), name, info)
			fileMime := ""
			if !d.IsDir() && realPath != "" {
				fileMime = mimeByExt(filepath.Ext(d.Name()))
			}
			entry := FileEntry{
				Name:       d.Name(),
				Path:       userfs.RequestPath(name),
				IsDir:      d.IsDir(),
				Size:       info.Size(),
				ModTime:    info.ModTime(),
				Mime:       fileMime,
				CanExtract: realPath != "" && CanExtract(d.Name()),
			}
			if realPath != "" {
				entry.CreatedAt = getBirthTime(realPath)
			}
			if writeEvent(c, "", entry) != nil {
				return filepath.SkipAll
			}
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

// ── The user's folder ─────────────────────────────────────────────────────────
// Handlers reach a user's files only through userfs (see that package): names
// relative to the user's folder, every operation kept inside it.

const userFSKey = "userFS"

// userFS returns the request's user folder, opened on first use and closed by
// CloseUserFS when the request ends.
func userFS(c *gin.Context) (*userfs.FS, error) {
	if opened, ok := c.Get(userFSKey); ok {
		return opened.(*userfs.FS), nil
	}
	fsys, err := userfs.Open(userRoot(c))
	if err != nil {
		return nil, err
	}
	c.Set(userFSKey, fsys)
	return fsys, nil
}

// CloseUserFS is middleware that closes the user folder userFS opened, once
// the request is done.
func CloseUserFS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if opened, ok := c.Get(userFSKey); ok {
			opened.(*userfs.FS).Close()
		}
	}
}

// requestName cleans a request path into a name in the user's folder. On
// failure it has answered the request.
func requestName(c *gin.Context, rel string) (string, bool) {
	name, err := userfs.Clean(rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return "", false
	}
	return name, true
}

// requestFolderName returns the user's folder and rel as a name in it — which
// may be the folder itself. On failure it has answered the request.
func requestFolderName(c *gin.Context, rel string) (*userfs.FS, string, bool) {
	name, ok := requestName(c, rel)
	if !ok {
		return nil, "", false
	}
	fsys, err := userFS(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, "", false
	}
	return fsys, name, true
}

// requestChildName is requestFolderName for an item that gets created,
// replaced, renamed, moved or deleted: never the folder itself.
func requestChildName(c *gin.Context, rel string) (*userfs.FS, string, bool) {
	fsys, names, ok := requestChildNames(c, []string{rel})
	if !ok {
		return nil, "", false
	}
	return fsys, names[0], true
}

// requestChildNames is requestChildName for several items, all checked
// before any is touched.
func requestChildNames(c *gin.Context, rels []string) (*userfs.FS, []string, bool) {
	names := make([]string, len(rels))
	for i, rel := range rels {
		name, err := userfs.CleanChild(rel)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return nil, nil, false
		}
		names[i] = name
	}
	fsys, err := userFS(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, nil, false
	}
	return fsys, names, true
}

// sessionStillValid reports whether the request's session is still live, for
// responses that stay open long after the check made when they started. A
// request without the check (handler tests) counts as live.
func sessionStillValid(c *gin.Context) bool {
	if check, ok := c.Get("sessionStillValid"); ok {
		return check.(func() bool)()
	}
	return true
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
