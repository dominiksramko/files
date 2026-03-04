package handlers

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type FileOpsHandler struct{}

func NewFileOpsHandler() *FileOpsHandler {
	return &FileOpsHandler{}
}

// DELETE /api/v1/files   body: { "paths": ["rel/path1", "rel/path2"] }
func (h *FileOpsHandler) Delete(c *gin.Context) {
	root := userRoot(c)

	var body struct {
		Paths []string `json:"paths"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths required"})
		return
	}

	for _, rel := range body.Paths {
		abs, err := safePath(root, rel)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := os.RemoveAll(abs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// POST /api/v1/files/copy   body: { "paths": ["rel/src1", ...], "dest": "rel/dest_dir", "dest_names": ["name1", ...] }
// dest_names is optional; when provided, dest_names[i] overrides the filename used at the destination.
func (h *FileOpsHandler) Copy(c *gin.Context) {
	root := userRoot(c)

	var body struct {
		Paths     []string `json:"paths"`
		Dest      string   `json:"dest"`
		DestNames []string `json:"dest_names"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths and dest required"})
		return
	}

	destAbs, err := safePath(root, body.Dest)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for i, rel := range body.Paths {
		srcAbs, err := safePath(root, rel)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		dstName := filepath.Base(srcAbs)
		if i < len(body.DestNames) && body.DestNames[i] != "" {
			if strings.ContainsAny(body.DestNames[i], "/\\") {
				c.JSON(http.StatusBadRequest, gin.H{"error": "dest_name must not contain path separators"})
				return
			}
			dstName = body.DestNames[i]
		}
		target := filepath.Join(destAbs, dstName)
		if err := copyEntry(srcAbs, target); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "copied"})
}

// POST /api/v1/files/move   body: { "paths": ["rel/src1", ...], "dest": "rel/dest_dir", "dest_names": ["name1", ...] }
// dest_names is optional; when provided, dest_names[i] overrides the filename used at the destination.
func (h *FileOpsHandler) Move(c *gin.Context) {
	root := userRoot(c)

	var body struct {
		Paths     []string `json:"paths"`
		Dest      string   `json:"dest"`
		DestNames []string `json:"dest_names"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths and dest required"})
		return
	}

	destAbs, err := safePath(root, body.Dest)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for i, rel := range body.Paths {
		srcAbs, err := safePath(root, rel)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		dstName := filepath.Base(srcAbs)
		if i < len(body.DestNames) && body.DestNames[i] != "" {
			if strings.ContainsAny(body.DestNames[i], "/\\") {
				c.JSON(http.StatusBadRequest, gin.H{"error": "dest_name must not contain path separators"})
				return
			}
			dstName = body.DestNames[i]
		}
		target := filepath.Join(destAbs, dstName)
		// Try fast rename first; fall back to copy+delete for cross-device moves
		if err := os.Rename(srcAbs, target); err != nil {
			if err2 := copyEntry(srcAbs, target); err2 != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err2.Error()})
				return
			}
			if err2 := os.RemoveAll(srcAbs); err2 != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err2.Error()})
				return
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "moved"})
}

// POST /api/v1/files/upload?path=rel/dest_dir   multipart form field: "files"
func (h *FileOpsHandler) Upload(c *gin.Context) {
	root := userRoot(c)
	rel := c.Query("path")

	destAbs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart form"})
		return
	}

	uploadedFiles := form.File["files"]
	if len(uploadedFiles) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no files uploaded"})
		return
	}

	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for i, fh := range uploadedFiles {
		dst := filepath.Join(destAbs, filepath.Base(fh.Filename))
		if err := c.SaveUploadedFile(fh, dst); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Restore mtime if provided
		mtimeKey := "mtime_" + strconv.Itoa(i)
		if mtimeStr := form.Value[mtimeKey]; len(mtimeStr) > 0 {
			if mtimeUnix, err := strconv.ParseInt(mtimeStr[0], 10, 64); err == nil {
				mtime := time.Unix(mtimeUnix, 0)
				_ = os.Chtimes(dst, mtime, mtime)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "uploaded", "count": len(uploadedFiles)})
}

// POST /api/v1/files/mkdir   body: { "path": "rel/path/new_folder" }
func (h *FileOpsHandler) Mkdir(c *gin.Context) {
	root := userRoot(c)

	var body struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	abs, err := safePath(root, body.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := os.MkdirAll(abs, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "created"})
}

// POST /api/v1/files/rename   body: { "path": "rel/old_path", "name": "new_name" }
func (h *FileOpsHandler) Rename(c *gin.Context) {
	root := userRoot(c)

	var body struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Path == "" || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path and name required"})
		return
	}

	// Reject names with path separators to prevent traversal
	if strings.ContainsAny(body.Name, "/\\") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must not contain path separators"})
		return
	}

	srcAbs, err := safePath(root, body.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	dstAbs := filepath.Join(filepath.Dir(srcAbs), body.Name)
	if !strings.HasPrefix(dstAbs, filepath.Clean(root)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path traversal detected"})
		return
	}

	if err := os.Rename(srcAbs, dstAbs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "renamed"})
}

// copyEntry copies a file or directory recursively from src to dst.
func copyEntry(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyEntry(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}

	// Preserve directory mtime
	info, err := os.Stat(src)
	if err == nil {
		_ = os.Chtimes(dst, info.ModTime(), info.ModTime())
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}

	// Preserve mtime
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}
