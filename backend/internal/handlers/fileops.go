package handlers

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"files/internal/userfs"

	"github.com/gin-gonic/gin"
)

type FileOpsHandler struct{}

func NewFileOpsHandler() *FileOpsHandler {
	return &FileOpsHandler{}
}

// DELETE /api/v1/files   body: { "paths": ["rel/path1", "rel/path2"] }
func (h *FileOpsHandler) Delete(c *gin.Context) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths required"})
		return
	}

	fsys, names, ok := requestChildNames(c, body.Paths)
	if !ok {
		return
	}
	// Removing what isn't there succeeds silently; say so instead.
	var missing []string
	for _, name := range names {
		if _, err := fsys.Lstat(name); os.IsNotExist(err) {
			missing = append(missing, filepath.Base(name))
		}
	}
	if len(missing) > 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no longer exists: " + strings.Join(missing, ", ")})
		return
	}
	for _, name := range names {
		if err := fsys.RemoveAll(name); err != nil {
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
	h.transfer(c, "copied", copyEntry)
}

// POST /api/v1/files/move
// body: { "paths": ["rel/src1", ...], "dest": "rel/dest_dir", "conflict_mode": "error"|"replace"|"rename" }
// conflict_mode defaults to "error" — returns HTTP 409 with conflicting names.
func (h *FileOpsHandler) Move(c *gin.Context) {
	h.transfer(c, "moved", moveEntry)
}

// transfer copies or moves (per apply) the requested items into a folder.
func (h *FileOpsHandler) transfer(c *gin.Context, done string, apply func(fsys *userfs.FS, src, dst string) error) {
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

	fsys, sources, ok := requestChildNames(c, body.Paths)
	if !ok {
		return
	}
	dest, ok := requestName(c, body.Dest)
	if !ok {
		return
	}
	if src, found, err := sourceContaining(fsys, sources, dest); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	} else if found {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s can't go inside itself", filepath.Base(src))})
		return
	}

	if body.ConflictMode == "error" {
		if conflicts := findConflicts(fsys, sources, dest); len(conflicts) > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": conflicts})
			return
		}
	}

	existing, claimed := buildExistingMap(fsys, dest)
	for _, src := range sources {
		dstName := batchTargetName(filepath.Base(src), body.ConflictMode, existing, claimed)
		if err := apply(fsys, src, filepath.Join(dest, dstName)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": done})
}

// POST /api/v1/files/upload?path=rel/dest_dir&conflict_mode=error|replace|rename   multipart form field: "files"
// conflict_mode defaults to "error" — returns HTTP 409 with the names already
// in the folder, before writing anything.
func (h *FileOpsHandler) Upload(c *gin.Context) {
	conflictMode := c.DefaultQuery("conflict_mode", "error")
	fsys, dest, ok := requestFolderName(c, c.Query("path"))
	if !ok {
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
		if conflicts := findNameConflicts(fsys, names, dest); len(conflicts) > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": conflicts})
			return
		}
	}

	if err := fsys.MkdirAll(dest, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	existing, claimed := buildExistingMap(fsys, dest)
	for i, fh := range uploadedFiles {
		target := filepath.Join(dest, batchTargetName(names[i], conflictMode, existing, claimed))
		if err := saveUpload(fsys, fh, target); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Restore mtime if provided
		mtimeKey := "mtime_" + strconv.Itoa(i)
		if mtimeStr := form.Value[mtimeKey]; len(mtimeStr) > 0 {
			if mtimeUnix, err := strconv.ParseInt(mtimeStr[0], 10, 64); err == nil {
				mtime := time.Unix(mtimeUnix, 0)
				_ = fsys.Chtimes(target, mtime, mtime)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "uploaded", "count": len(uploadedFiles)})
}

// saveUpload writes an uploaded file to name, replacing what's there.
func saveUpload(fsys *userfs.FS, upload *multipart.FileHeader, name string) error {
	in, err := upload.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := fsys.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// POST /api/v1/files/mkdir   body: { "path": "rel/path/new_folder" }
func (h *FileOpsHandler) Mkdir(c *gin.Context) {
	var body struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	fsys, name, ok := requestFolderName(c, body.Path)
	if !ok {
		return
	}
	if err := fsys.MkdirAll(name, 0o755); err != nil {
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

	fsys, name, ok := requestChildName(c, body.Path)
	if !ok {
		return
	}

	name, conflict, err := resolveDestConflict(fsys, name, body.ConflictMode)
	if err == nil && !conflict {
		// O_EXCL: a file created elsewhere in the meantime is a conflict too.
		var file *os.File
		file, err = fsys.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			err = file.Close()
		} else if os.IsExist(err) {
			conflict = true
		}
	}
	if conflict {
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": []string{filepath.Base(name)}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"name": filepath.Base(name)})
}

// POST /api/v1/files/rename   body: { "path": "rel/old_path", "name": "new_name", "conflict_mode": "error"|"replace"|"rename" }
// conflict_mode defaults to "error" — returns HTTP 409 when the name is taken.
// Responds with the name the item ended up with.
func (h *FileOpsHandler) Rename(c *gin.Context) {
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

	fsys, src, ok := requestChildName(c, body.Path)
	if !ok {
		return
	}

	if body.ConflictMode == "" {
		body.ConflictMode = "error"
	}

	dst := filepath.Join(filepath.Dir(src), body.Name)
	// The same file under another name is no conflict: a change of case on a
	// case-insensitive disk.
	if !isSameFile(fsys, src, dst) {
		var conflict bool
		var err error
		dst, conflict, err = resolveDestConflict(fsys, dst, body.ConflictMode)
		if conflict {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": []string{body.Name}})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if err := fsys.Rename(src, dst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "renamed", "name": filepath.Base(dst)})
}

// ── Conflict helpers ──────────────────────────────────────────────────────────
// All take names in the user's folder (see userfs).

// findConflicts returns base names of sources that already exist in the folder dest.
func findConflicts(fsys *userfs.FS, sources []string, dest string) []string {
	names := make([]string, len(sources))
	for i, src := range sources {
		names[i] = filepath.Base(src)
	}
	return findNameConflicts(fsys, names, dest)
}

// findNameConflicts returns the names (each once) that already exist in the folder dest.
func findNameConflicts(fsys *userfs.FS, names []string, dest string) []string {
	existing, reported := buildExistingMap(fsys, dest)
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

// sourceContaining returns the first of sources that the folder dest is, or
// is inside: copying or moving it there would put a folder inside itself.
// Compares real locations, so a link to the folder counts as the folder.
func sourceContaining(fsys *userfs.FS, sources []string, dest string) (string, bool, error) {
	realDest, err := fsys.RealPath(dest)
	if err != nil {
		return "", false, fmt.Errorf("destination: %w", err)
	}
	for _, src := range sources {
		realSrc, err := fsys.RealPath(src)
		if err != nil {
			return "", false, err
		}
		if isWithin(realSrc, realDest) {
			return src, true, nil
		}
	}
	return "", false, nil
}

// isSameFile reports whether both names exist and are the same file.
func isSameFile(fsys *userfs.FS, first, second string) bool {
	firstInfo, err := fsys.Stat(first)
	if err != nil {
		return false
	}
	secondInfo, err := fsys.Stat(second)
	return err == nil && os.SameFile(firstInfo, secondInfo)
}

// buildExistingMap returns a set of names already present in the folder dir and an empty claimed set.
func buildExistingMap(fsys *userfs.FS, dir string) (existing, claimed map[string]bool) {
	existing = make(map[string]bool)
	claimed = make(map[string]bool)
	entries, _ := fsys.ReadDir(dir)
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

// resolveDestConflict applies a conflict_mode to a single destination that may
// already exist: "error" reports the conflict, "replace" removes the existing
// entry (never a non-empty folder), "rename" returns a free " (n)" name next to it.
func resolveDestConflict(fsys *userfs.FS, dest, mode string) (resolved string, conflict bool, err error) {
	if _, statErr := fsys.Lstat(dest); statErr != nil {
		return dest, false, nil
	}
	switch mode {
	case "replace":
		if err := fsys.Remove(dest); err != nil {
			return dest, false, fmt.Errorf("failed to remove existing file: %w", err)
		}
		return dest, false, nil
	case "rename":
		dir := filepath.Dir(dest)
		existing, claimed := buildExistingMap(fsys, dir)
		return filepath.Join(dir, findFreeName(filepath.Base(dest), existing, claimed)), false, nil
	default:
		return dest, true, nil
	}
}

// ── Copy helpers ──────────────────────────────────────────────────────────────

// moveEntry moves a file or folder; across disks (mounts inside the folder),
// by copying then deleting.
func moveEntry(fsys *userfs.FS, src, dst string) error {
	if err := fsys.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyEntry(fsys, src, dst); err != nil {
		return err
	}
	return fsys.RemoveAll(src)
}

// copyEntry copies a file or folder recursively from src to dst. A symbolic
// link is copied as a link (following it could loop); it still can't lead
// outside the folder, as every use goes through userfs.
func copyEntry(fsys *userfs.FS, src, dst string) error {
	info, err := fsys.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := fsys.Readlink(src)
		if err != nil {
			return err
		}
		return fsys.Symlink(target, dst)
	case info.IsDir():
		return copyDir(fsys, src, dst, info)
	default:
		return copyFileViaTemp(fsys, src, dst)
	}
}

func copyDir(fsys *userfs.FS, src, dst string, info os.FileInfo) error {
	if err := fsys.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := fsys.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyEntry(fsys, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	_ = fsys.Chtimes(dst, info.ModTime(), info.ModTime())
	return nil
}

// copyFileViaTemp copies src to dst via a temporary file with SHA-256 verification.
// The temp file is created in the same folder as dst so the final rename is
// atomic (same filesystem). On any error the temp file is removed.
func copyFileViaTemp(fsys *userfs.FS, src, dst string) error {
	in, err := fsys.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	tmp, tmpName, err := createTempFile(fsys, filepath.Dir(dst), ".~tmp_")
	if err != nil {
		return err
	}

	succeeded := false
	defer func() {
		if !succeeded {
			fsys.Remove(tmpName)
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
	tmpIn, err := fsys.Open(tmpName)
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
	if err = fsys.Chmod(tmpName, info.Mode()); err != nil {
		return err
	}
	_ = fsys.Chtimes(tmpName, info.ModTime(), info.ModTime())

	if err = fsys.Rename(tmpName, dst); err != nil {
		return err
	}
	succeeded = true
	return nil
}

// createTempFile creates a new, empty file with a random name starting with
// prefix in the folder dir, and returns it open for writing with its name.
func createTempFile(fsys *userfs.FS, dir, prefix string) (*os.File, string, error) {
	for attempt := 0; attempt < 100; attempt++ {
		name := filepath.Join(dir, prefix+rand.Text())
		file, err := fsys.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		return file, name, err
	}
	return nil, "", fmt.Errorf("no free temporary name in %s", dir)
}
