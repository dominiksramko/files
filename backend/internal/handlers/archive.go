package handlers

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// POST /api/v1/files/compress
// Body: { paths, dest, format, password? }
// Supported formats: zip, tar, tar.gz, 7z
func (h *ArchiveHandler) Compress(c *gin.Context) {
	root := userRoot(c)

	var req struct {
		Paths            []string `json:"paths" binding:"required"`
		Dest             string   `json:"dest" binding:"required"`
		Format           string   `json:"format" binding:"required"`
		Password         string   `json:"password"`
		ProtectFilenames bool     `json:"protect_filenames"`
		ConflictMode     string   `json:"conflict_mode"` // "error" (default) | "replace" | "rename"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths required"})
		return
	}
	if req.ConflictMode == "" {
		req.ConflictMode = "error"
	}

	destAbs, err := safeChildPath(root, req.Dest)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	destAbs, conflict, err := resolveDestConflict(destAbs, req.ConflictMode)
	if conflict {
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": []string{filepath.Base(destAbs)}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var absInputs []string
	for _, rel := range req.Paths {
		abs, err := safePath(root, rel)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		absInputs = append(absInputs, abs)
	}

	switch req.Format {
	case "zip":
		if req.Password != "" {
			err = compress7zFormat(absInputs, destAbs, "zip", req.Password, false)
		} else {
			err = compressZip(absInputs, destAbs)
		}
	case "tar":
		err = compressTar(absInputs, destAbs, false)
	case "tar.gz":
		err = compressTar(absInputs, destAbs, true)
	case "7z":
		err = compress7zFormat(absInputs, destAbs, "7z", req.Password, req.ProtectFilenames)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported format: " + req.Format})
		return
	}

	if err != nil {
		os.Remove(destAbs)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "compressed", "dest": filepath.Base(destAbs)})
}

// --- Compress helpers ---

// compressEntry holds the resolved info for a single entry to be archived.
type compressEntry struct {
	absPath string
	name    string // path within the archive (slash-separated; dirs end with /)
	info    os.FileInfo
}

// walkInputs iterates over each input path and calls fn for every file and
// directory entry (including descendants of directories).
func walkInputs(inputs []string, fn func(compressEntry) error) error {
	for _, absPath := range inputs {
		info, err := os.Stat(absPath)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if err := fn(compressEntry{absPath, info.Name(), info}); err != nil {
				return err
			}
			continue
		}
		if err := filepath.Walk(absPath, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(filepath.Dir(absPath), path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if fi.IsDir() {
				rel += "/"
			}
			return fn(compressEntry{path, rel, fi})
		}); err != nil {
			return err
		}
	}
	return nil
}

// --- Compress: ZIP (pure Go) ---

func compressZip(inputs []string, dest string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()

	return walkInputs(inputs, func(e compressEntry) error {
		header, err := zip.FileInfoHeader(e.info)
		if err != nil {
			return err
		}
		header.Name = e.name
		if e.info.IsDir() {
			_, err = zw.CreateHeader(header)
			return err
		}
		header.Method = zip.Deflate
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		src, err := os.Open(e.absPath)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
}

// --- Compress: TAR / TAR.GZ (pure Go) ---

func compressTar(inputs []string, dest string, gzipped bool) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	var tw *tar.Writer
	if gzipped {
		gz := gzip.NewWriter(f)
		defer gz.Close()
		tw = tar.NewWriter(gz)
	} else {
		tw = tar.NewWriter(f)
	}
	defer tw.Close()

	return walkInputs(inputs, func(e compressEntry) error {
		header, err := tar.FileInfoHeader(e.info, "")
		if err != nil {
			return err
		}
		header.Name = e.name
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if e.info.IsDir() {
			return nil
		}
		src, err := os.Open(e.absPath)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(tw, src)
		return err
	})
}

// --- Compress: 7z / ZIP-with-password (CLI) ---

func compress7zFormat(inputs []string, dest string, format string, password string, protectFilenames bool) error {
	args := []string{"a", "-t" + format}
	if password != "" {
		args = append(args, "-p"+password)
		if protectFilenames {
			args = append(args, "-mhe=on")
		}
	}
	args = append(args, dest)
	args = append(args, inputs...)

	var stderr bytes.Buffer
	cmd := exec.Command("7z", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("7z failed: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

type ArchiveHandler struct{}

func NewArchiveHandler() *ArchiveHandler {
	return &ArchiveHandler{}
}

type ArchiveEntry struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

// GET /api/v1/files/archive?path=&password=
func (h *ArchiveHandler) Contents(c *gin.Context) {
	root := userRoot(c)
	rel := c.Query("path")
	password := c.Query("password")

	abs, err := safePath(root, rel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ext := strings.ToLower(filepath.Ext(abs))

	if ext == ".zip" && password == "" {
		entries, err := listZip(abs)
		if err == nil {
			c.JSON(http.StatusOK, entries)
			return
		}
	}

	if isTar(abs) && password == "" {
		entries, err := listTar(abs)
		if err == nil {
			c.JSON(http.StatusOK, entries)
			return
		}
	}

	if ext == ".rar" {
		entries, err := listUnrar(abs, password)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("cannot read archive: %s", err)})
			return
		}
		c.JSON(http.StatusOK, entries)
		return
	}

	entries, err := list7z(abs, password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("cannot read archive: %s", err)})
		return
	}
	c.JSON(http.StatusOK, entries)
}

// POST /api/v1/files/extract
// Body: { path, dest?, password?, conflict_mode? }
//
// Extracts next to the archive into a folder named after it (minus extension),
// preserving all internal paths — or, when the archive holds a single item,
// puts that item straight next to the archive. With dest, the contents go
// into that folder instead. Folders that already exist are merged into.
// conflict_mode defaults to "error": HTTP 409 listing the files that are
// already there, before anything is moved into place.
func (h *ArchiveHandler) Extract(c *gin.Context) {
	root := userRoot(c)

	var req struct {
		Path         string `json:"path" binding:"required"`
		Dest         string `json:"dest"`
		Password     string `json:"password"`
		ConflictMode string `json:"conflict_mode"` // "error" (default) | "replace" | "rename"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ConflictMode == "" {
		req.ConflictMode = "error"
	}

	abs, err := safePath(root, req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Destination: explicit override or a folder named after the archive. Never
	// root itself: extraction is staged in the destination's parent, and a
	// single item goes there too.
	destRel := req.Dest
	if destRel == "" {
		destRel = filepath.Join(filepath.Dir(req.Path), archiveBaseName(abs))
	}
	destAbs, err := safeChildPath(root, destRel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Extract into a fresh folder next to the destination first, then move the
	// result into place: a failed extract only ever removes that folder.
	staging, err := os.MkdirTemp(filepath.Dir(destAbs), ".extract-tmp-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer os.RemoveAll(staging)
	contents := filepath.Join(staging, filepath.Base(destAbs))
	if err := os.Mkdir(contents, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := extractArchive(abs, contents, req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// staging holds the folder named after the archive; a single item skips
	// that folder, and an explicit destination receives the contents.
	from, into := staging, filepath.Dir(destAbs)
	if req.Dest != "" {
		from, into = contents, destAbs
	} else if entries, err := os.ReadDir(contents); err == nil && len(entries) == 1 {
		from = contents
	}

	if req.ConflictMode == "error" {
		if conflicts := mergeConflicts(from, into); len(conflicts) > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": conflicts})
			return
		}
	}
	if err := mergeInto(from, into, req.ConflictMode); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "extracted"})
}

// CanExtract reports whether the file at path is a supported archive format.
func CanExtract(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range []string{".zip", ".tar", ".tar.gz", ".tgz", ".tar.bz2", ".tar.xz", ".tar.zst", ".rar", ".7z"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// archiveBaseName strips all archive-related extensions from the filename.
func archiveBaseName(path string) string {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tar.zst"} {
		if strings.HasSuffix(lower, ext) {
			return base[:len(base)-len(ext)]
		}
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// extractArchive extracts the archive at path into the folder dest.
func extractArchive(path, dest, password string) error {
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case ext == ".zip" && password == "":
		return extractZip(path, dest)
	case isTar(path) && password == "":
		return extractTar(path, dest)
	case ext == ".rar":
		return extractUnrar(path, dest, password)
	default:
		return extract7z(path, dest, password)
	}
}

// mergeConflicts lists, as paths relative to into, the entries of the folder
// from that mergeInto would put where something already is. Folders on both
// sides are merged, so only what's inside them counts.
func mergeConflicts(from, into string) []string {
	var conflicts []string
	var walk func(rel string)
	walk = func(rel string) {
		entries, _ := os.ReadDir(filepath.Join(from, rel))
		for _, entry := range entries {
			entryRel := filepath.Join(rel, entry.Name())
			target, err := os.Lstat(filepath.Join(into, entryRel))
			switch {
			case err != nil:
			case entry.IsDir() && target.IsDir():
				walk(entryRel)
			default:
				conflicts = append(conflicts, filepath.ToSlash(entryRel))
			}
		}
	}
	walk("")
	return conflicts
}

// mergeInto moves every entry of the folder from into the folder into,
// merging folders that exist on both sides. Anything else already there is
// replaced with "replace", kept next to the new entry with "rename" (which
// gets " (n)"), and otherwise left alone, failing the merge.
func mergeInto(from, into, mode string) error {
	if err := os.MkdirAll(into, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	existing, claimed := buildExistingMap(into)
	for _, entry := range entries {
		source, target := filepath.Join(from, entry.Name()), filepath.Join(into, entry.Name())
		if info, err := os.Lstat(target); err == nil {
			if entry.IsDir() && info.IsDir() {
				if err := mergeInto(source, target, mode); err != nil {
					return err
				}
				continue
			}
			switch mode {
			case "replace":
				if err := os.RemoveAll(target); err != nil {
					return err
				}
			case "rename":
				target = filepath.Join(into, findFreeName(entry.Name(), existing, claimed))
			default:
				return fmt.Errorf("%s already exists", entry.Name())
			}
		}
		claimed[filepath.Base(target)] = true
		if err := moveEntry(source, target); err != nil {
			return err
		}
	}
	return nil
}

// --- ZIP ---

func listZip(path string) ([]ArchiveEntry, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	entries := make([]ArchiveEntry, 0, len(r.File))
	for _, f := range r.File {
		entries = append(entries, ArchiveEntry{
			Name:  f.Name,
			Size:  int64(f.UncompressedSize64),
			IsDir: f.FileInfo().IsDir(),
		})
	}
	return entries, nil
}

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(dest, filepath.Clean(f.Name))
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && target != filepath.Clean(dest) {
			continue // path traversal
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0o755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			if err == zip.ErrAlgorithm || f.FileHeader.Flags&0x1 != 0 {
				return fmt.Errorf("archive is password protected")
			}
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			if f.FileHeader.Flags&0x1 != 0 {
				return fmt.Errorf("archive is password protected")
			}
			return err
		}
	}
	return nil
}

// --- TAR ---

func isTar(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".tar") ||
		strings.HasSuffix(lower, ".tar.gz") ||
		strings.HasSuffix(lower, ".tgz") ||
		strings.HasSuffix(lower, ".tar.bz2") ||
		strings.HasSuffix(lower, ".tar.xz") ||
		strings.HasSuffix(lower, ".tar.zst")
}

func openTar(path string) (*tar.Reader, io.Closer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	lower := strings.ToLower(path)
	var reader io.Reader = f

	if strings.HasSuffix(lower, ".gz") || strings.HasSuffix(lower, ".tgz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, nil, err
		}
		return tar.NewReader(gz), multiCloser{gz, f}, nil
	}
	if strings.HasSuffix(lower, ".bz2") {
		reader = bzip2.NewReader(f)
	}

	return tar.NewReader(reader), f, nil
}

type multiCloser []io.Closer

func (m multiCloser) Close() error {
	var last error
	for _, c := range m {
		if err := c.Close(); err != nil {
			last = err
		}
	}
	return last
}

func listTar(path string) ([]ArchiveEntry, error) {
	tr, closer, err := openTar(path)
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var entries []ArchiveEntry
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, ArchiveEntry{
			Name:  hdr.Name,
			Size:  hdr.Size,
			IsDir: hdr.Typeflag == tar.TypeDir,
		})
	}
	return entries, nil
}

func extractTar(src, dest string) error {
	tr, closer, err := openTar(src)
	if err != nil {
		return err
	}
	defer closer.Close()

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(dest)) {
			continue // path traversal
		}

		if hdr.Typeflag == tar.TypeDir {
			os.MkdirAll(target, 0o755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, tr)
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// --- RAR (unrar) ---

func listUnrar(path, password string) ([]ArchiveEntry, error) {
	args := []string{"l"}
	if password != "" {
		args = append(args, "-p"+password)
	}
	args = append(args, path)

	out, err := exec.Command("unrar", args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.ToLower(string(exitErr.Stderr) + string(out))
			if strings.Contains(stderr, "password") || strings.Contains(stderr, "wrong password") || strings.Contains(stderr, "encrypted") {
				return nil, fmt.Errorf("archive is password protected")
			}
		}
		return nil, fmt.Errorf("unrar failed: %w", err)
	}
	return parseUnrarList(string(out)), nil
}

// parseUnrarList parses the output of "unrar l".
// Output between the two "---" separator lines looks like:
//
//	 -rw-r--r--      1234  2023-01-01 12:00  file.txt
//	 drwxr-xr-x         0  2023-01-01 12:00  folder/
func parseUnrarList(output string) []ArchiveEntry {
	var entries []ArchiveEntry
	scanner := bufio.NewScanner(strings.NewReader(output))
	inData := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "---") {
			if inData {
				break // second separator — done
			}
			inData = true
			continue
		}
		if !inData {
			continue
		}
		// Fields: attrs  size  date  time  name (name may contain spaces)
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		attrs := fields[0]
		var size int64
		fmt.Sscanf(fields[1], "%d", &size)
		name := strings.Join(fields[4:], " ")
		isDir := strings.HasPrefix(attrs, "d") || strings.HasSuffix(name, "/")
		entries = append(entries, ArchiveEntry{Name: name, Size: size, IsDir: isDir})
	}
	return entries
}

func extractUnrar(src, dest, password string) error {
	args := []string{"x", "-y"}
	if password != "" {
		args = append(args, "-p"+password)
	}
	args = append(args, src, dest+string(os.PathSeparator))

	var stderr bytes.Buffer
	cmd := exec.Command("unrar", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.ToLower(stderr.String())
		if strings.Contains(msg, "password") || strings.Contains(msg, "wrong password") || strings.Contains(msg, "encrypted") {
			return fmt.Errorf("wrong password or archive is encrypted")
		}
		return err
	}
	return nil
}

// --- 7z ---

func list7z(path, password string) ([]ArchiveEntry, error) {
	// Always pass -p (even empty) so 7z doesn't prompt for a password on stdin,
	// which causes it to exit 255 in non-interactive environments.
	args := []string{"l", "-slt", "-p" + password, path}

	out, err := exec.Command("7z", args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.ToLower(string(exitErr.Stderr))
			if strings.Contains(stderr, "password") || strings.Contains(stderr, "wrong password") || strings.Contains(stderr, "encrypted") {
				return nil, fmt.Errorf("archive is password protected")
			}
		}
		return nil, fmt.Errorf("7z not available or failed: %w", err)
	}

	return parse7zList(string(out)), nil
}

func parse7zList(output string) []ArchiveEntry {
	var entries []ArchiveEntry
	var current ArchiveEntry
	scanner := bufio.NewScanner(strings.NewReader(output))
	inBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "----------" {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		if line == "" && current.Name != "" {
			entries = append(entries, current)
			current = ArchiveEntry{}
			continue
		}
		if key, val, ok := strings.Cut(line, " = "); ok {
			key = strings.TrimSpace(key)
			val = strings.TrimSpace(val)
			switch key {
			case "Path":
				current.Name = val
			case "Size":
				fmt.Sscanf(val, "%d", &current.Size)
			case "Attributes":
				current.IsDir = strings.HasPrefix(val, "D")
			}
		}
	}
	if current.Name != "" {
		entries = append(entries, current)
	}
	return entries
}

func extract7z(src, dest, password string) error {
	// Always pass -p so 7z doesn't prompt for a password on stdin.
	args := []string{"x", "-y", "-o" + dest, "-p" + password, src}

	var stderr bytes.Buffer
	cmd := exec.Command("7z", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.ToLower(stderr.String())
		if strings.Contains(msg, "password") || strings.Contains(msg, "wrong password") || strings.Contains(msg, "encrypted") {
			return fmt.Errorf("wrong password or archive is encrypted")
		}
		return err
	}
	return nil
}
