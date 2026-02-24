package handlers

import (
	"archive/tar"
	"archive/zip"
	"bufio"
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

	// Try pure-Go for ZIP without password
	if ext == ".zip" && password == "" {
		entries, err := listZip(abs)
		if err == nil {
			c.JSON(http.StatusOK, entries)
			return
		}
	}

	// Try TAR variants
	if isTar(abs) && password == "" {
		entries, err := listTar(abs)
		if err == nil {
			c.JSON(http.StatusOK, entries)
			return
		}
	}

	// Fall back to 7z
	entries, err := list7z(abs, password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("cannot read archive: %s", err)})
		return
	}
	c.JSON(http.StatusOK, entries)
}

// POST /api/v1/files/extract
// Body: { path, dest?, password? }
func (h *ArchiveHandler) Extract(c *gin.Context) {
	root := userRoot(c)

	var req struct {
		Path     string `json:"path" binding:"required"`
		Dest     string `json:"dest"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	abs, err := safePath(root, req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Determine destination
	destRel := req.Dest
	if destRel == "" {
		destRel = filepath.Dir(req.Path)
	}
	destAbs, err := safePath(root, destRel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// For pure-Go zip (no password)
	ext := strings.ToLower(filepath.Ext(abs))
	if ext == ".zip" && req.Password == "" {
		if err := extractZip(abs, destAbs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "extracted"})
		return
	}

	if isTar(abs) && req.Password == "" {
		if err := extractTar(abs, destAbs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "extracted"})
		return
	}

	// Fall back to 7z
	if err := extract7z(abs, destAbs, req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "extracted"})
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

	// Detect single root folder
	rootDir := singleRoot(func(yield func(string) bool) {
		for _, f := range r.File {
			if !yield(f.Name) {
				return
			}
		}
	})

	for _, f := range r.File {
		name := f.Name
		if rootDir != "" {
			name = strings.TrimPrefix(name, rootDir)
			if name == "" {
				continue
			}
		}

		target := filepath.Join(dest, filepath.Clean(name))
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && target != filepath.Clean(dest) {
			continue // skip path traversal
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
			continue
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

// --- 7z ---

func list7z(path, password string) ([]ArchiveEntry, error) {
	args := []string{"l", "-slt"}
	if password != "" {
		args = append(args, "-p"+password)
	}
	args = append(args, path)

	out, err := exec.Command("7z", args...).Output()
	if err != nil {
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
	args := []string{"x", "-y", "-o" + dest}
	if password != "" {
		args = append(args, "-p"+password)
	}
	args = append(args, src)

	cmd := exec.Command("7z", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// --- helpers ---

// singleRoot returns the common root folder prefix if all entries share one,
// so we can strip it during extraction.
func singleRoot(iter func(yield func(string) bool)) string {
	var root string
	first := true
	iter(func(name string) bool {
		parts := strings.SplitN(name, "/", 2)
		if len(parts) < 2 || parts[0] == "" {
			root = ""
			return false
		}
		candidate := parts[0] + "/"
		if first {
			root = candidate
			first = false
		} else if root != candidate {
			root = ""
			return false
		}
		return true
	})
	return root
}
