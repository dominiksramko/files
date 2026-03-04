package handlers

import (
	"bytes"
	"crypto/sha256"
	"fmt"
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

// POST /api/v1/files/copy
// body: { "paths": ["rel/src1", ...], "dest": "rel/dest_dir", "conflict_mode": "error"|"replace"|"rename" }
// conflict_mode defaults to "error" — returns HTTP 409 with conflicting names.
func (h *FileOpsHandler) Copy(c *gin.Context) {
	root := userRoot(c)

	var body struct {
		Paths        []string `json:"paths"`
		Dest         string   `json:"dest"`
		ConflictMode string   `json:"conflict_mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths and dest required"})
		return
	}
	if body.ConflictMode == "" {
		body.ConflictMode = "error"
	}

	destAbs, err := safePath(root, body.Dest)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	srcAbs := make([]string, 0, len(body.Paths))
	for _, rel := range body.Paths {
		abs, err := safePath(root, rel)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		srcAbs = append(srcAbs, abs)
	}

	if body.ConflictMode == "error" {
		if conflicts := findConflicts(srcAbs, destAbs); len(conflicts) > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": conflicts})
			return
		}
	}

	existing, claimed := buildExistingMap(destAbs)

	for _, src := range srcAbs {
		dstName := filepath.Base(src)
		if body.ConflictMode == "rename" {
			dstName = findFreeName(dstName, existing, claimed)
			claimed[dstName] = true
		}
		if err := copyEntry(src, filepath.Join(destAbs, dstName)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "copied"})
}

// POST /api/v1/files/move
// body: { "paths": ["rel/src1", ...], "dest": "rel/dest_dir", "conflict_mode": "error"|"replace"|"rename" }
// conflict_mode defaults to "error" — returns HTTP 409 with conflicting names.
func (h *FileOpsHandler) Move(c *gin.Context) {
	root := userRoot(c)

	var body struct {
		Paths        []string `json:"paths"`
		Dest         string   `json:"dest"`
		ConflictMode string   `json:"conflict_mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths and dest required"})
		return
	}
	if body.ConflictMode == "" {
		body.ConflictMode = "error"
	}

	destAbs, err := safePath(root, body.Dest)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	srcAbs := make([]string, 0, len(body.Paths))
	for _, rel := range body.Paths {
		abs, err := safePath(root, rel)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		srcAbs = append(srcAbs, abs)
	}

	if body.ConflictMode == "error" {
		if conflicts := findConflicts(srcAbs, destAbs); len(conflicts) > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": conflicts})
			return
		}
	}

	existing, claimed := buildExistingMap(destAbs)

	for _, src := range srcAbs {
		dstName := filepath.Base(src)
		if body.ConflictMode == "rename" {
			dstName = findFreeName(dstName, existing, claimed)
			claimed[dstName] = true
		}
		target := filepath.Join(destAbs, dstName)
		// Try fast rename first; fall back to copy+delete for cross-device moves
		if err := os.Rename(src, target); err != nil {
			if err2 := copyEntry(src, target); err2 != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err2.Error()})
				return
			}
			if err2 := os.RemoveAll(src); err2 != nil {
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

// ── Conflict helpers ──────────────────────────────────────────────────────────

// findConflicts returns base names from srcAbs that already exist in destAbs.
func findConflicts(srcAbs []string, destAbs string) []string {
	entries, err := os.ReadDir(destAbs)
	if err != nil {
		return nil // dest doesn't exist yet — no conflicts
	}
	existing := make(map[string]bool, len(entries))
	for _, e := range entries {
		existing[e.Name()] = true
	}
	var conflicts []string
	for _, p := range srcAbs {
		name := filepath.Base(p)
		if existing[name] {
			conflicts = append(conflicts, name)
		}
	}
	return conflicts
}

// buildExistingMap returns a set of names already present in dir and an empty claimed set.
func buildExistingMap(dir string) (existing, claimed map[string]bool) {
	existing = make(map[string]bool)
	claimed = make(map[string]bool)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		existing[e.Name()] = true
	}
	return
}

// findFreeName returns a name not present in existing or claimed,
// appending " (1)", " (2)", etc. as needed.
func findFreeName(name string, existing, claimed map[string]bool) string {
	if !existing[name] && !claimed[name] {
		return name
	}
	lastDot := strings.LastIndex(name, ".")
	base, ext := name, ""
	if lastDot >= 0 {
		base = name[:lastDot]
		ext = name[lastDot:]
	}
	for n := 1; ; n++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, n, ext)
		if !existing[candidate] && !claimed[candidate] {
			return candidate
		}
	}
}

// ── Copy helpers ──────────────────────────────────────────────────────────────

// copyEntry copies a file or directory recursively from src to dst.
func copyEntry(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFileViaTemp(src, dst)
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

// copyFileViaTemp copies src to dst via a temporary file with SHA-256 verification.
// The temp file is created in the same directory as dst to guarantee an atomic
// os.Rename (same filesystem). On any error the temp file is removed.
func copyFileViaTemp(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	// Temp file in same directory as dst ensures same filesystem → atomic rename.
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".~tmp_")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	succeeded := false
	defer func() {
		if !succeeded {
			os.Remove(tmpPath)
		}
	}()

	// Stream-copy while hashing the source data.
	srcHasher := sha256.New()
	tee := io.TeeReader(in, srcHasher)
	if _, err = io.Copy(tmp, tee); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}

	// Verify: re-read the temp file and compare hashes.
	tmpIn, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	dstHasher := sha256.New()
	if _, err = io.Copy(dstHasher, tmpIn); err != nil {
		tmpIn.Close()
		return err
	}
	tmpIn.Close()

	if !bytes.Equal(srcHasher.Sum(nil), dstHasher.Sum(nil)) {
		return fmt.Errorf("copy verification failed: hash mismatch after write")
	}

	// Preserve permissions and mtime.
	if err = os.Chmod(tmpPath, info.Mode()); err != nil {
		return err
	}
	_ = os.Chtimes(tmpPath, info.ModTime(), info.ModTime())

	// Atomic rename to final destination.
	if err = os.Rename(tmpPath, dst); err != nil {
		return err
	}
	succeeded = true
	return nil
}
