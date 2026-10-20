// Package userfs is the one way the server touches a user's files. An FS is a
// user's home folder opened as an os.Root: every operation is checked by the
// operating system to stay inside it — ".." can't climb out, and symbolic
// links are followed only while they stay inside (absolute links are refused)
// — with no gap between a check and the operation it guards.
//
// Names are slash-separated paths relative to the home folder, as requests
// carry them; Clean turns a request path into one.
package userfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrOutside: a name leads outside the home folder.
var ErrOutside = errors.New("path is outside your folder")

// ErrRoot: an operation that changes or removes an item was given the home
// folder itself (its parent holds the other users' folders).
var ErrRoot = errors.New("the root folder itself can't be changed")

type FS struct {
	root *os.Root
	dir  string // the home folder, as configured
}

// Open opens the home folder dir. Close it when done.
func Open(dir string) (*FS, error) {
	if dir == "" {
		return nil, errors.New("user has no root directory configured")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening your folder: %w", err)
	}
	return &FS{root: root, dir: filepath.Clean(dir)}, nil
}

func (f *FS) Close() error { return f.root.Close() }

// Clean turns a request path into a name for FS methods: "" and "/" become "."
// (the home folder), a leading slash is dropped, and a path climbing above
// the home folder is refused.
func Clean(rel string) (string, error) {
	name := filepath.Clean(strings.TrimLeft(filepath.FromSlash(rel), string(filepath.Separator)))
	if name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
		return "", ErrOutside
	}
	return name, nil
}

// RequestPath is name as the app writes paths: slash-separated, "" for the
// home folder. The reverse of Clean.
func RequestPath(name string) string {
	if name == "." {
		return ""
	}
	return filepath.ToSlash(name)
}

// CleanChild is Clean for an item that gets created, replaced, renamed, moved
// or deleted: the home folder itself is refused.
func CleanChild(rel string) (string, error) {
	name, err := Clean(rel)
	if err == nil && name == "." {
		return "", ErrRoot
	}
	return name, err
}

func (f *FS) Open(name string) (*os.File, error)   { return f.root.Open(name) }
func (f *FS) Create(name string) (*os.File, error) { return f.root.Create(name) }
func (f *FS) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return f.root.OpenFile(name, flag, perm)
}
func (f *FS) Stat(name string) (os.FileInfo, error)  { return f.root.Stat(name) }
func (f *FS) Lstat(name string) (os.FileInfo, error) { return f.root.Lstat(name) }
func (f *FS) ReadFile(name string) ([]byte, error)   { return f.root.ReadFile(name) }
func (f *FS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return f.root.WriteFile(name, data, perm)
}
func (f *FS) Mkdir(name string, perm os.FileMode) error    { return f.root.Mkdir(name, perm) }
func (f *FS) MkdirAll(name string, perm os.FileMode) error { return f.root.MkdirAll(name, perm) }
func (f *FS) Remove(name string) error                     { return f.root.Remove(name) }
func (f *FS) RemoveAll(name string) error                  { return f.root.RemoveAll(name) }
func (f *FS) Rename(oldName, newName string) error         { return f.root.Rename(oldName, newName) }
func (f *FS) Chmod(name string, mode os.FileMode) error    { return f.root.Chmod(name, mode) }
func (f *FS) Readlink(name string) (string, error)         { return f.root.Readlink(name) }

// Symlink creates the link name pointing at target. Like every link, it's
// only followed while it stays inside the folder.
func (f *FS) Symlink(target, name string) error { return f.root.Symlink(target, name) }
func (f *FS) Chtimes(name string, atime, mtime time.Time) error {
	return f.root.Chtimes(name, atime, mtime)
}

// ReadDir lists the folder name, sorted by name.
func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(f.root.FS(), slashName(name))
}

// WalkDir walks the tree at name like fs.WalkDir (symbolic links aren't
// followed into); paths given to walk are names for FS methods.
func (f *FS) WalkDir(name string, walk fs.WalkDirFunc) error {
	return fs.WalkDir(f.root.FS(), slashName(name), func(path string, entry fs.DirEntry, err error) error {
		return walk(filepath.FromSlash(path), entry, err)
	})
}

// RealPath returns where name really is on disk, for programs that need a
// path rather than an open file (ffmpeg, 7z, the file watcher, ...). It
// resolves symbolic links and refuses a location outside the home folder, as
// the other methods do. The item must exist; for one about to be created,
// resolve its folder.
func (f *FS) RealPath(name string) (string, error) {
	if _, err := f.root.Stat(name); err != nil {
		return "", err
	}
	realDir, err := filepath.EvalSymlinks(f.dir)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(filepath.Join(f.dir, name))
	if err != nil {
		return "", err
	}
	if rel, err := filepath.Rel(realDir, real); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrOutside
	}
	return real, nil
}

// ResolveName returns the name of what name really is, following symbolic
// links (which stay inside the folder): for replacing a file's content
// without replacing a link that points at it. The item must exist.
func (f *FS) ResolveName(name string) (string, error) {
	real, err := f.RealPath(name)
	if err != nil {
		return "", err
	}
	realDir, err := filepath.EvalSymlinks(f.dir)
	if err != nil {
		return "", err
	}
	return filepath.Rel(realDir, real)
}

// slashName is name as io/fs wants it: slash-separated, "." for the root.
func slashName(name string) string {
	if name == "" {
		return "."
	}
	return filepath.ToSlash(name)
}
