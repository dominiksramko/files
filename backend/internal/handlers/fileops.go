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

	paths, err := safeChildPaths(root, body.Paths)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for _, abs := range paths {
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

	srcAbs, err := safeChildPaths(root, body.Paths)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if src, found := sourceContaining(srcAbs, destAbs); found {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s can't go inside itself", filepath.Base(src))})
		return
	}

	if body.ConflictMode == "error" {
		if conflicts := findConflicts(srcAbs, destAbs); len(conflicts) > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": conflicts})
			return
		}
	}

	existing, claimed := buildExistingMap(destAbs)

	for _, src := range srcAbs {
		dstName := batchTargetName(filepath.Base(src), body.ConflictMode, existing, claimed)
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

	srcAbs, err := safeChildPaths(root, body.Paths)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if src, found := sourceContaining(srcAbs, destAbs); found {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s can't go inside itself", filepath.Base(src))})
		return
	}

	if body.ConflictMode == "error" {
		if conflicts := findConflicts(srcAbs, destAbs); len(conflicts) > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": conflicts})
			return
		}
	}

	existing, claimed := buildExistingMap(destAbs)

	for _, src := range srcAbs {
		dstName := batchTargetName(filepath.Base(src), body.ConflictMode, existing, claimed)
		if err := moveEntry(src, filepath.Join(destAbs, dstName)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "moved"})
}

// POST /api/v1/files/upload?path=rel/dest_dir&conflict_mode=error|replace|rename   multipart form field: "files"
// conflict_mode defaults to "error" — returns HTTP 409 with the names already
// in the folder, before writing anything.
func (h *FileOpsHandler) Upload(c *gin.Context) {
	root := userRoot(c)
	rel := c.Query("path")
	conflictMode := c.DefaultQuery("conflict_mode", "error")

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

	names := make([]string, len(uploadedFiles))
	for i, fh := range uploadedFiles {
		names[i] = filepath.Base(fh.Filename)
		if err := validateNewFilename(names[i]); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename: " + err.Error()})
			return
		}
	}
	if conflictMode == "error" {
		if conflicts := findNameConflicts(names, destAbs); len(conflicts) > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": conflicts})
			return
		}
	}

	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	existing, claimed := buildExistingMap(destAbs)
	for i, fh := range uploadedFiles {
		dst := filepath.Join(destAbs, batchTargetName(names[i], conflictMode, existing, claimed))
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

// POST /api/v1/files/create   body: { "path": "rel/new_file.txt", "conflict_mode": "error"|"replace"|"rename" }
// Creates an empty file. conflict_mode defaults to "error" — returns HTTP 409
// when the name is taken, so an existing file is never emptied by accident.
// Responds with the name the file was created under.
func (h *FileOpsHandler) Create(c *gin.Context) {
	root := userRoot(c)

	var body struct {
		Path         string `json:"path"`
		ConflictMode string `json:"conflict_mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}
	if body.ConflictMode == "" {
		body.ConflictMode = "error"
	}

	abs, err := safeChildPath(root, body.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	abs, conflict, err := resolveDestConflict(abs, body.ConflictMode)
	if err == nil && !conflict {
		// O_EXCL: a file created elsewhere in the meantime is a conflict too.
		var file *os.File
		file, err = os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			err = file.Close()
		} else if os.IsExist(err) {
			conflict = true
		}
	}
	if conflict {
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": []string{filepath.Base(abs)}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"name": filepath.Base(abs)})
}

// POST /api/v1/files/rename   body: { "path": "rel/old_path", "name": "new_name", "conflict_mode": "error"|"replace"|"rename" }
// conflict_mode defaults to "error" — returns HTTP 409 when the name is taken.
// Responds with the name the item ended up with.
func (h *FileOpsHandler) Rename(c *gin.Context) {
	root := userRoot(c)

	var body struct {
		Path         string `json:"path"`
		Name         string `json:"name"`
		ConflictMode string `json:"conflict_mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Path == "" || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path and name required"})
		return
	}

	// A plain name: no path separators, "." or ".."
	if err := validateNewFilename(body.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	srcAbs, err := safeChildPath(root, body.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if body.ConflictMode == "" {
		body.ConflictMode = "error"
	}

	dstAbs := filepath.Join(filepath.Dir(srcAbs), body.Name)
	// The same file under another name is no conflict: a change of case on a
	// case-insensitive disk.
	if !isSameFile(srcAbs, dstAbs) {
		var conflict bool
		dstAbs, conflict, err = resolveDestConflict(dstAbs, body.ConflictMode)
		if conflict {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": []string{body.Name}})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if err := os.Rename(srcAbs, dstAbs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "renamed", "name": filepath.Base(dstAbs)})
}

// ── Conflict helpers ──────────────────────────────────────────────────────────

// findConflicts returns base names from srcAbs that already exist in destAbs.
func findConflicts(srcAbs []string, destAbs string) []string {
	names := make([]string, len(srcAbs))
	for i, src := range srcAbs {
		names[i] = filepath.Base(src)
	}
	return findNameConflicts(names, destAbs)
}

// findNameConflicts returns the names (each once) that already exist in destAbs.
func findNameConflicts(names []string, destAbs string) []string {
	existing, reported := buildExistingMap(destAbs)
	var conflicts []string
	for _, name := range names {
		if existing[name] && !reported[name] {
			conflicts = append(conflicts, name)
			reported[name] = true
		}
	}
	return conflicts
}

// batchTargetName picks the name for the next item of a batch going into one
// folder, and claims it in claimed. Items of the same batch never replace each
// other: a name an earlier item took gets " (n)" whatever the mode. With
// "rename" a name already in the folder (existing) does too; otherwise it's
// kept, to be replaced or reported as a conflict beforehand.
func batchTargetName(name, mode string, existing, claimed map[string]bool) string {
	if mode == "rename" || claimed[name] {
		name = findFreeName(name, existing, claimed)
	}
	claimed[name] = true
	return name
}

// sourceContaining returns the first of srcAbs that destAbs is, or is inside:
// copying or moving it there would put a folder inside itself.
func sourceContaining(srcAbs []string, destAbs string) (string, bool) {
	for _, src := range srcAbs {
		if isWithin(src, destAbs) {
			return src, true
		}
	}
	return "", false
}

// isSameFile reports whether both paths exist and are the same file.
func isSameFile(first, second string) bool {
	firstInfo, err := os.Stat(first)
	if err != nil {
		return false
	}
	secondInfo, err := os.Stat(second)
	return err == nil && os.SameFile(firstInfo, secondInfo)
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

// resolveDestConflict applies a conflict_mode to a single destination path
// that may already exist: "error" reports the conflict, "replace" removes the
// existing entry, "rename" returns a free " (n)" name next to it.
func resolveDestConflict(destAbs, mode string) (resolved string, conflict bool, err error) {
	if _, statErr := os.Stat(destAbs); statErr != nil {
		return destAbs, false, nil
	}
	switch mode {
	case "replace":
		if err := os.Remove(destAbs); err != nil {
			return destAbs, false, fmt.Errorf("failed to remove existing file: %w", err)
		}
		return destAbs, false, nil
	case "rename":
		dir := filepath.Dir(destAbs)
		existing, claimed := buildExistingMap(dir)
		return filepath.Join(dir, findFreeName(filepath.Base(destAbs), existing, claimed)), false, nil
	default:
		return destAbs, true, nil
	}
}

// ── Copy helpers ──────────────────────────────────────────────────────────────

// moveEntry moves a file or directory; across disks, by copying then deleting.
func moveEntry(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyEntry(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
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
