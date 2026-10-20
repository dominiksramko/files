// Package diskusage measures what takes space in a user's folder: totals for
// the Details dialog, and a scan of a whole tree for the disk usage page.
//
// It reaches files only through userfs, and never follows symbolic links (a
// link counts as the link itself). Two sizes are kept: Apparent, the sum of
// the files' lengths (what the file list shows), and Disk, the space actually
// taken (blocks, folders included, a file with several hard links counted
// once), as du and ncdu report it.
package diskusage

import (
	"container/heap"
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"files/internal/userfs"
)

// LargestKept is how many of the largest files a scan remembers, per measure.
const LargestKept = 100

// progressInterval is how often Scan reports progress (a variable for tests).
var progressInterval = 250 * time.Millisecond

// Totals counts what's below a folder (the folder itself isn't in Dirs).
type Totals struct {
	Files    int64 `json:"files"`
	Dirs     int64 `json:"dirs"`
	Disk     int64 `json:"disk"`
	Apparent int64 `json:"apparent"`
}

func (t *Totals) add(other Totals) {
	t.Files += other.Files
	t.Dirs += other.Dirs
	t.Disk += other.Disk
	t.Apparent += other.Apparent
}

func (t *Totals) subtract(other Totals) {
	t.Files -= other.Files
	t.Dirs -= other.Dirs
	t.Disk -= other.Disk
	t.Apparent -= other.Apparent
}

// Dir is a scanned folder with everything below it. Only folders are kept in
// the tree; a folder's files are read again when it's shown.
type Dir struct {
	Totals
	Children map[string]*Dir
	parent   *Dir
}

// File is one of the largest files found.
type File struct {
	Path     string // name in the user's folder
	Disk     int64
	Apparent int64
	ModTime  time.Time
}

// Result is a finished scan of the folder Name.
type Result struct {
	Name       string
	Root       *Dir
	Unreadable int // folders whose contents couldn't be read
	ScannedAt  time.Time

	largestDisk     []File
	largestApparent []File
}

// Measure picks which size to rank by.
type Measure int

const (
	ByDisk Measure = iota
	ByApparent
)

func (m Measure) of(file File) int64 {
	if m == ByApparent {
		return file.Apparent
	}
	return file.Disk
}

// walk calls visit for name and everything below it with its Lstat details
// (name itself is followed if it's a link, like Stat), and counts the folders
// that couldn't be read. It stops when ctx ends. Only an unreadable name is an
// error.
func walk(ctx context.Context, fsys *userfs.FS, name string, visit func(path string, info fs.FileInfo), unreadable func()) error {
	return fsys.WalkDir(name, func(path string, entry fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return fs.SkipAll
		}
		if err != nil {
			if path == name && entry == nil {
				return err
			}
			// The folder was visited already; only its contents are missing.
			unreadable()
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil // removed while scanning
		}
		visit(path, info)
		return nil
	})
}

// Count adds what's at name (a file or a folder with everything below it) to
// totals, calling tick after each item. Hard links are counted once only
// within one call.
func Count(ctx context.Context, fsys *userfs.FS, name string, totals *Totals, tick func()) error {
	links := linkSet{}
	return walk(ctx, fsys, name, func(path string, info fs.FileInfo) {
		disk := links.allocated(info)
		switch {
		case info.IsDir() && path == name:
			totals.Disk += disk
		case info.IsDir():
			totals.Dirs++
			totals.Disk += disk
		default:
			totals.Files++
			totals.Disk += disk
			totals.Apparent += info.Size()
		}
		tick()
	}, func() {})
}

// Scan measures the folder name and everything below it. onProgress gets the
// running totals every so often. A scan stopped by ctx returns ctx's error.
func Scan(ctx context.Context, fsys *userfs.FS, name string, onProgress func(Totals)) (*Result, error) {
	if info, err := fsys.Stat(name); err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, ErrNotFolder
	}
	result := &Result{Name: name, Root: &Dir{Children: map[string]*Dir{}}}
	folders := map[string]*Dir{name: result.Root}
	links := linkSet{}
	byDisk := &fileHeap{measure: ByDisk}
	byApparent := &fileHeap{measure: ByApparent}
	lastReport := time.Now()

	err := walk(ctx, fsys, name, func(path string, info fs.FileInfo) {
		disk := links.allocated(info)
		if path == name {
			result.Root.Disk += disk
			return
		}
		parent := folders[filepath.Dir(path)]
		if parent == nil {
			return
		}
		var added Totals
		if info.IsDir() {
			folder := &Dir{Totals: Totals{Disk: disk}, Children: map[string]*Dir{}, parent: parent}
			parent.Children[filepath.Base(path)] = folder
			folders[path] = folder
			added = Totals{Dirs: 1, Disk: disk}
		} else {
			added = Totals{Files: 1, Disk: disk, Apparent: info.Size()}
			file := File{Path: path, Disk: disk, Apparent: info.Size(), ModTime: info.ModTime()}
			byDisk.keep(file)
			byApparent.keep(file)
		}
		for folder := parent; folder != nil; folder = folder.parent {
			folder.add(added)
		}
		if onProgress != nil && time.Since(lastReport) >= progressInterval {
			lastReport = time.Now()
			onProgress(result.Root.Totals)
		}
	}, func() { result.Unreadable++ })
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	result.largestDisk = byDisk.sorted()
	result.largestApparent = byApparent.sorted()
	result.ScannedAt = time.Now()
	return result, nil
}

// ErrNotFolder is returned when the scanned name isn't a folder.
var ErrNotFolder = errors.New("not a folder")

// relParts splits name into its parts below the scanned folder, or reports
// that it isn't inside it. The scanned folder itself has no parts.
func (r *Result) relParts(name string) ([]string, bool) {
	rel, err := filepath.Rel(r.Name, name)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, false
	}
	if rel == "." {
		return nil, true
	}
	return strings.Split(rel, string(filepath.Separator)), true
}

// Contains reports whether name is the scanned folder or inside it.
func (r *Result) Contains(name string) bool {
	_, ok := r.relParts(name)
	return ok
}

// Find returns the scanned folder at name, or nil when the scan has none
// there (not inside, not a folder, or created after the scan).
func (r *Result) Find(name string) *Dir {
	parts, ok := r.relParts(name)
	if !ok {
		return nil
	}
	folder := r.Root
	for _, part := range parts {
		if folder = folder.Children[part]; folder == nil {
			return nil
		}
	}
	return folder
}

// Largest returns the largest files found, largest first.
func (r *Result) Largest(measure Measure) []File {
	if measure == ByApparent {
		return r.largestApparent
	}
	return r.largestDisk
}

// Remove takes name, which is being deleted, out of the scan: its sizes are
// subtracted from every folder above it and it leaves the largest files. A
// folder's sizes come from the scan; a file's are given (from its Lstat
// details). It returns what was taken off, and nothing for a name outside the
// scan or the scanned folder itself.
func (r *Result) Remove(name string, info fs.FileInfo) Totals {
	parts, ok := r.relParts(name)
	if !ok || len(parts) == 0 {
		return Totals{}
	}
	parent := r.Find(filepath.Dir(name))
	var removed Totals
	if info.IsDir() {
		folder := r.Find(name)
		if folder == nil {
			return Totals{}
		}
		removed = folder.Totals
		removed.Dirs++
		delete(parent.Children, parts[len(parts)-1])
	} else {
		removed = Totals{Files: 1, Disk: linkSet{}.allocated(info), Apparent: info.Size()}
	}
	for folder := parent; folder != nil; folder = folder.parent {
		folder.subtract(removed)
	}
	r.largestDisk = withoutPath(r.largestDisk, name)
	r.largestApparent = withoutPath(r.largestApparent, name)
	return removed
}

// withoutPath drops name and everything below it.
func withoutPath(files []File, name string) []File {
	kept := files[:0:0]
	for _, file := range files {
		if file.Path != name && !strings.HasPrefix(file.Path, name+string(filepath.Separator)) {
			kept = append(kept, file)
		}
	}
	return kept
}

// fileHeap keeps the LargestKept largest files by one measure (smallest on top,
// so it's the one to drop).
type fileHeap struct {
	measure Measure
	files   []File
}

func (h *fileHeap) Len() int           { return len(h.files) }
func (h *fileHeap) Less(i, j int) bool { return h.measure.of(h.files[i]) < h.measure.of(h.files[j]) }
func (h *fileHeap) Swap(i, j int)      { h.files[i], h.files[j] = h.files[j], h.files[i] }
func (h *fileHeap) Push(item any)      { h.files = append(h.files, item.(File)) }
func (h *fileHeap) Pop() any {
	last := h.files[len(h.files)-1]
	h.files = h.files[:len(h.files)-1]
	return last
}

func (h *fileHeap) keep(file File) {
	if h.Len() < LargestKept {
		heap.Push(h, file)
	} else if h.measure.of(file) > h.measure.of(h.files[0]) {
		h.files[0] = file
		heap.Fix(h, 0)
	}
}

// sorted returns the kept files, largest first (by path when equal).
func (h *fileHeap) sorted() []File {
	files := append([]File(nil), h.files...)
	sort.Slice(files, func(i, j int) bool {
		if sizeI, sizeJ := h.measure.of(files[i]), h.measure.of(files[j]); sizeI != sizeJ {
			return sizeI > sizeJ
		}
		return files[i].Path < files[j].Path
	})
	return files
}
