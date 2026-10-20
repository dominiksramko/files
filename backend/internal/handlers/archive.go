package handlers

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"crypto/rand"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"files/internal/userfs"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
	"golang.org/x/text/encoding/charmap"
)

// POST /api/v1/files/compress
// Body: { paths, dest, format, password? }
// Supported formats: zip, tar, tar.gz, 7z
func (h *ArchiveHandler) Compress(c *gin.Context) {
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

	fsys, dest, ok := requestChildName(c, req.Dest)
	if !ok {
		return
	}
	inputs := make([]string, len(req.Paths))
	for i, rel := range req.Paths {
		if inputs[i], ok = requestName(c, rel); !ok {
			return
		}
	}
	switch req.Format {
	case "zip", "tar", "tar.gz", "7z":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported format: " + req.Format})
		return
	}

	dest, conflict, err := resolveDestConflict(fsys, dest, req.ConflictMode)
	if conflict {
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": []string{filepath.Base(dest)}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	switch {
	case req.Format == "zip" && req.Password == "":
		err = compressZip(fsys, inputs, dest)
	case req.Format == "tar":
		err = compressTar(fsys, inputs, dest, false)
	case req.Format == "tar.gz":
		err = compressTar(fsys, inputs, dest, true)
	default: // 7z, or zip with a password
		err = compress7zFormat(fsys, inputs, dest, req.Format, req.Password, req.Format == "7z" && req.ProtectFilenames)
	}
	if err != nil {
		fsys.Remove(dest)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "compressed", "dest": filepath.Base(dest)})
}

// --- Compress helpers ---

// compressEntry is one file or folder to be archived.
type compressEntry struct {
	name        string // in the user's folder
	archiveName string // path within the archive (slash-separated; folders end with /)
	info        os.FileInfo
}

// walkInputs calls fn for each input and, for folders, everything inside.
// Symbolic links inside folders are archived as the file they point to; links
// to folders (which could loop) and links that can't be followed (outside
// the user's folder, or broken) are left out.
func walkInputs(fsys *userfs.FS, inputs []string, fn func(compressEntry) error) error {
	for _, input := range inputs {
		info, err := fsys.Stat(input)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if err := fn(compressEntry{input, info.Name(), info}); err != nil {
				return err
			}
			continue
		}
		parent := filepath.Dir(input)
		if err := fsys.WalkDir(input, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				if info, err = fsys.Stat(name); err != nil || info.IsDir() {
					return nil
				}
			}
			archiveName, err := filepath.Rel(parent, name)
			if err != nil {
				return err
			}
			archiveName = filepath.ToSlash(archiveName)
			if info.IsDir() {
				archiveName += "/"
			}
			return fn(compressEntry{name, archiveName, info})
		}); err != nil {
			return err
		}
	}
	return nil
}

// --- Compress: ZIP (pure Go) ---

func compressZip(fsys *userfs.FS, inputs []string, dest string) error {
	f, err := fsys.Create(dest)
	if err != nil {
		return err
	}
	err = writeZip(fsys, inputs, f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// writeZip writes a zip of the inputs (and everything in folders among them) to out.
func writeZip(fsys *userfs.FS, inputs []string, out io.Writer) error {
	zw := zip.NewWriter(out)
	err := walkInputs(fsys, inputs, func(e compressEntry) error {
		header, err := zip.FileInfoHeader(e.info)
		if err != nil {
			return err
		}
		header.Name = e.archiveName
		if e.info.IsDir() {
			_, err = zw.CreateHeader(header)
			return err
		}
		header.Method = zip.Deflate
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		src, err := fsys.Open(e.name)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
	if closeErr := zw.Close(); err == nil {
		err = closeErr
	}
	return err
}

// --- Compress: TAR / TAR.GZ (pure Go) ---

func compressTar(fsys *userfs.FS, inputs []string, dest string, gzipped bool) error {
	f, err := fsys.Create(dest)
	if err != nil {
		return err
	}
	err = writeTar(fsys, inputs, f, gzipped)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// writeTar writes a tar (gzipped if asked) of the inputs to out. Errors
// closing the tar and gzip streams are returned: that's where the last of
// the archive is written, and a full disk shows up there.
func writeTar(fsys *userfs.FS, inputs []string, out io.Writer, gzipped bool) error {
	var gz *gzip.Writer
	if gzipped {
		gz = gzip.NewWriter(out)
		out = gz
	}
	tw := tar.NewWriter(out)
	err := walkInputs(fsys, inputs, func(e compressEntry) error {
		header, err := tar.FileInfoHeader(e.info, "")
		if err != nil {
			return err
		}
		header.Name = e.archiveName
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if e.info.IsDir() {
			return nil
		}
		src, err := fsys.Open(e.name)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(tw, src)
		return err
	})
	if closeErr := tw.Close(); err == nil {
		err = closeErr
	}
	if gz != nil {
		if closeErr := gz.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

// --- Compress: 7z / ZIP-with-password (CLI) ---

// compress7zFormat runs 7z on the inputs' real locations (checked to be in
// the user's folder), writing the archive to dest's real folder. -snl stores
// symbolic links as links rather than following them.
func compress7zFormat(fsys *userfs.FS, inputs []string, dest string, format string, password string, protectFilenames bool) error {
	realDir, err := fsys.RealPath(filepath.Dir(dest))
	if err != nil {
		return err
	}
	args := []string{"a", "-t" + format, "-snl"}
	if password != "" {
		args = append(args, "-p"+password)
		if protectFilenames {
			args = append(args, "-mhe=on")
		}
	}
	args = append(args, filepath.Join(realDir, filepath.Base(dest)))
	for _, input := range inputs {
		realInput, err := fsys.RealPath(input)
		if err != nil {
			return err
		}
		args = append(args, realInput)
	}

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
	password := c.Query("password")
	fsys, name, ok := requestFolderName(c, c.Query("path"))
	if !ok {
		return
	}
	// Read from its real location, checked to be inside the folder.
	abs, err := fsys.RealPath(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "archive not found"})
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

	fsys, archive, ok := requestFolderName(c, req.Path)
	if !ok {
		return
	}
	archivePath, err := fsys.RealPath(archive)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "archive not found"})
		return
	}

	// Destination: explicit override or a folder named after the archive. Never
	// the home folder itself: extraction is staged in the destination's
	// parent, and a single item goes there too.
	destRel := req.Dest
	if destRel == "" {
		destRel = filepath.Join(filepath.Dir(archive), archiveBaseName(archive))
	}
	dest, err := userfs.CleanChild(destRel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Extract into a fresh folder next to the destination first, then move the
	// result into place: a failed extract only ever removes that folder.
	staging, err := createStagingFolder(fsys, filepath.Dir(dest), ".extract-tmp-")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer fsys.RemoveAll(staging)
	contents := filepath.Join(staging, filepath.Base(dest))
	if err := fsys.Mkdir(contents, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := extractArchive(fsys, archivePath, contents, req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Archives can hold symbolic links, which 7z and unrar recreate. None are
	// kept: an extracted file is always a file of the user's own.
	if err := removeLinks(fsys, staging); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// staging holds the folder named after the archive; a single item skips
	// that folder, and an explicit destination receives the contents.
	from, into := staging, filepath.Dir(dest)
	if req.Dest != "" {
		from, into = contents, dest
	} else if entries, err := fsys.ReadDir(contents); err == nil && len(entries) == 1 {
		from = contents
	}

	if req.ConflictMode == "error" {
		if conflicts := mergeConflicts(fsys, from, into); len(conflicts) > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": conflicts})
			return
		}
	}
	if err := mergeInto(fsys, from, into, req.ConflictMode); err != nil {
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

// extractArchive extracts the archive at archivePath (its real location) into
// the folder dest in the user's folder. Our own ZIP and TAR extraction writes
// through fsys; 7z and unrar write to dest's real location — a fresh staging
// folder created for them.
func extractArchive(fsys *userfs.FS, archivePath, dest, password string) error {
	ext := strings.ToLower(filepath.Ext(archivePath))
	switch {
	case ext == ".zip" && password == "":
		return extractZip(fsys, archivePath, dest)
	case isTar(archivePath) && password == "":
		return extractTar(fsys, archivePath, dest)
	}
	realDest, err := fsys.RealPath(dest)
	if err != nil {
		return err
	}
	if ext == ".rar" {
		return extractUnrar(archivePath, realDest, password)
	}
	return extract7z(archivePath, realDest, password)
}

// createStagingFolder creates a new folder with a random name starting with
// prefix in the folder dir, and returns its name.
func createStagingFolder(fsys *userfs.FS, dir, prefix string) (string, error) {
	for attempt := 0; attempt < 100; attempt++ {
		name := filepath.Join(dir, prefix+rand.Text())
		err := fsys.Mkdir(name, 0o755)
		if os.IsExist(err) {
			continue
		}
		return name, err
	}
	return "", fmt.Errorf("no free temporary name in %s", dir)
}

// removeLinks deletes every symbolic link under the folder dir.
func removeLinks(fsys *userfs.FS, dir string) error {
	var links []string
	err := fsys.WalkDir(dir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			links = append(links, name)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, link := range links {
		if err := fsys.Remove(link); err != nil {
			return err
		}
	}
	return nil
}

// mergeConflicts lists, as paths relative to into, the entries of the folder
// from that mergeInto would put where something already is. Folders on both
// sides are merged, so only what's inside them counts.
func mergeConflicts(fsys *userfs.FS, from, into string) []string {
	var conflicts []string
	var walk func(rel string)
	walk = func(rel string) {
		entries, _ := fsys.ReadDir(filepath.Join(from, rel))
		for _, entry := range entries {
			entryRel := filepath.Join(rel, entry.Name())
			target, err := fsys.Lstat(filepath.Join(into, entryRel))
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
func mergeInto(fsys *userfs.FS, from, into, mode string) error {
	if err := fsys.MkdirAll(into, 0o755); err != nil {
		return err
	}
	entries, err := fsys.ReadDir(from)
	if err != nil {
		return err
	}
	existing, claimed := buildExistingMap(fsys, into)
	for _, entry := range entries {
		source, target := filepath.Join(from, entry.Name()), filepath.Join(into, entry.Name())
		if info, err := fsys.Lstat(target); err == nil {
			if entry.IsDir() && info.IsDir() {
				if err := mergeInto(fsys, source, target, mode); err != nil {
					return err
				}
				continue
			}
			switch mode {
			case "replace":
				if err := fsys.RemoveAll(target); err != nil {
					return err
				}
			case "rename":
				target = filepath.Join(into, findFreeName(entry.Name(), existing, claimed))
			default:
				return fmt.Errorf("%s already exists", entry.Name())
			}
		}
		claimed[filepath.Base(target)] = true
		if err := moveEntry(fsys, source, target); err != nil {
			return err
		}
	}
	return nil
}

// archiveTarget is where an archive entry named entryName goes inside the
// folder dest; false when the name would land outside it.
func archiveTarget(dest, entryName string) (string, bool) {
	target := filepath.Join(dest, filepath.Clean(filepath.FromSlash(entryName)))
	return target, isWithin(dest, target) && target != dest
}

// writeEntry writes one extracted file to name in the user's folder, with the
// permission bits it had in the archive (so scripts stay executable); special
// bits like setuid are dropped.
func writeEntry(fsys *userfs.FS, name string, content io.Reader, mode os.FileMode) error {
	if err := fsys.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	out, err := fsys.Create(name)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, content); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if perm := mode.Perm(); perm != 0 {
		return fsys.Chmod(name, perm|0o200) // keep it writable for its owner
	}
	return nil
}

// --- ZIP ---

// zipEntryName is the entry's name as text. A name that isn't valid UTF-8 is
// in the old DOS code page CP437 (as the ZIP spec says and Windows writes),
// and was otherwise extracted as raw bytes that can't be shown, opened or
// deleted. Valid UTF-8 is kept even without the ZIP's UTF-8 flag: many tools
// leave the flag out.
func zipEntryName(f *zip.File) string {
	if utf8.ValidString(f.Name) {
		return f.Name
	}
	if decoded, err := charmap.CodePage437.NewDecoder().String(f.Name); err == nil {
		return decoded
	}
	return strings.ToValidUTF8(f.Name, "_")
}

func listZip(path string) ([]ArchiveEntry, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	entries := make([]ArchiveEntry, 0, len(r.File))
	for _, f := range r.File {
		entries = append(entries, ArchiveEntry{
			Name:  zipEntryName(f),
			Size:  int64(f.UncompressedSize64),
			IsDir: f.FileInfo().IsDir(),
		})
	}
	return entries, nil
}

// extractZip extracts the zip at archivePath into the folder dest, writing
// only regular files and folders (a link entry is written as a plain file
// holding its target's name).
func extractZip(fsys *userfs.FS, archivePath, dest string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target, ok := archiveTarget(dest, zipEntryName(f))
		if !ok {
			continue // would land outside dest
		}
		if f.FileInfo().IsDir() {
			if err := fsys.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			if err == zip.ErrAlgorithm || f.FileHeader.Flags&0x1 != 0 {
				return fmt.Errorf("archive is password protected")
			}
			return err
		}
		err = writeEntry(fsys, target, rc, f.Mode())
		rc.Close()
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
	switch {
	case strings.HasSuffix(lower, ".bz2"):
		reader = bzip2.NewReader(f)
	case strings.HasSuffix(lower, ".xz"):
		xzReader, err := xz.NewReader(f)
		if err != nil {
			f.Close()
			return nil, nil, err
		}
		reader = xzReader
	case strings.HasSuffix(lower, ".zst"):
		zstdReader, err := zstd.NewReader(f)
		if err != nil {
			f.Close()
			return nil, nil, err
		}
		return tar.NewReader(zstdReader), multiCloser{zstdCloser{zstdReader}, f}, nil
	}

	return tar.NewReader(reader), f, nil
}

// zstdCloser adapts zstd.Decoder, whose Close returns nothing, to io.Closer.
type zstdCloser struct{ decoder *zstd.Decoder }

func (closer zstdCloser) Close() error {
	closer.decoder.Close()
	return nil
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

// extractTar extracts the tar at archivePath into the folder dest, writing
// only regular files and folders: links and special entries are skipped.
func extractTar(fsys *userfs.FS, archivePath, dest string) error {
	tr, closer, err := openTar(archivePath)
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

		target, ok := archiveTarget(dest, hdr.Name)
		if !ok {
			continue // would land outside dest
		}
		// Only real files and folders: not links, devices, or metadata entries
		// such as the PAX global header.
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := fsys.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeEntry(fsys, target, tr, hdr.FileInfo().Mode()); err != nil {
				return err
			}
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
//	-rw-r--r--      1234  2023-01-01 12:00  file.txt
//	drwxr-xr-x         0  2023-01-01 12:00  folder/
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
