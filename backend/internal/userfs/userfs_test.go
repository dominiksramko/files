package userfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// newHome creates two neighbouring home folders and returns ann's FS and
// both folders. ann's folder holds links: one staying inside, two leading to
// anna's folder (relative and absolute) and an absolute one pointing inside.
func newHome(t *testing.T) (*FS, string, string) {
	t.Helper()
	users := t.TempDir()
	ann, anna := filepath.Join(users, "ann"), filepath.Join(users, "anna")
	for path, content := range map[string]string{
		filepath.Join(ann, "notes.txt"):    "ann's notes",
		filepath.Join(ann, "docs/plan.md"): "# plan",
		filepath.Join(anna, "secret.txt"):  "anna's secret",
	} {
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(content), 0o644)
	}
	for link, target := range map[string]string{
		"papers":        "docs",                     // stays inside
		"to-anna":       "../anna",                  // leads out
		"anna-secret":   "../anna/secret.txt",       // leads out
		"absolute-anna": anna,                       // absolute, outside
		"absolute-docs": filepath.Join(ann, "docs"), // absolute, inside: still refused
	} {
		if err := os.Symlink(target, filepath.Join(ann, link)); err != nil {
			t.Fatal(err)
		}
	}
	fsys, err := Open(ann)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fsys.Close() })
	return fsys, ann, anna
}

func TestClean(t *testing.T) {
	for rel, want := range map[string]string{
		"": ".", "/": ".", ".": ".", "docs": "docs", "/docs/plan.md": "docs/plan.md",
		"docs/../notes.txt": "notes.txt", "..notes": "..notes",
	} {
		if got, err := Clean(rel); err != nil || got != filepath.FromSlash(want) {
			t.Errorf("Clean(%q) = %q, %v; want %q", rel, got, err, want)
		}
	}
	for _, rel := range []string{"..", "../anna", "docs/../../anna", "/../anna"} {
		if got, err := Clean(rel); !errors.Is(err, ErrOutside) {
			t.Errorf("Clean(%q) = %q, %v; want ErrOutside", rel, got, err)
		}
	}
}

func TestCleanChild(t *testing.T) {
	for _, rel := range []string{"", ".", "/", "docs/.."} {
		if _, err := CleanChild(rel); !errors.Is(err, ErrRoot) {
			t.Errorf("CleanChild(%q): err = %v, want ErrRoot", rel, err)
		}
	}
	if name, err := CleanChild("docs/plan.md"); err != nil || name != filepath.FromSlash("docs/plan.md") {
		t.Errorf("CleanChild(docs/plan.md) = %q, %v", name, err)
	}
}

func TestOpenWithoutFolder(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Error("Open with no folder succeeded")
	}
}

func TestLinksInsideWork(t *testing.T) {
	fsys, _, _ := newHome(t)
	if data, err := fsys.ReadFile("papers/plan.md"); err != nil || string(data) != "# plan" {
		t.Errorf("read through an inside link: %q, %v", data, err)
	}
	if err := fsys.WriteFile("papers/new.md", []byte("x"), 0o644); err != nil {
		t.Errorf("write through an inside link: %v", err)
	}
}

func TestLinksOutsideAreRefused(t *testing.T) {
	fsys, _, anna := newHome(t)
	for _, name := range []string{"to-anna/secret.txt", "anna-secret", "absolute-anna/secret.txt", "absolute-docs/plan.md", "../anna/secret.txt"} {
		if data, err := fsys.ReadFile(name); err == nil {
			t.Errorf("read %s: got %q", name, data)
		}
		if err := fsys.WriteFile(name, []byte("overwritten"), 0o644); err == nil {
			t.Errorf("wrote %s", name)
		}
		if _, err := fsys.RealPath(name); err == nil {
			t.Errorf("RealPath(%s) succeeded", name)
		}
	}
	if _, err := fsys.ReadDir("to-anna"); err == nil {
		t.Error("listed a folder through an outside link")
	}
	if err := fsys.Rename("notes.txt", "to-anna/stolen.txt"); err == nil {
		t.Error("moved a file out through a link")
	}
	if data, _ := os.ReadFile(filepath.Join(anna, "secret.txt")); string(data) != "anna's secret" {
		t.Errorf("anna's file changed: %q", data)
	}
	if _, err := os.Stat(filepath.Join(anna, "stolen.txt")); err == nil {
		t.Error("a file landed in anna's folder")
	}
}

func TestRemovingALinkLeavesItsTarget(t *testing.T) {
	fsys, _, anna := newHome(t)
	if err := fsys.RemoveAll("to-anna"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(anna, "secret.txt")); err != nil {
		t.Errorf("removing the link removed what it pointed to: %v", err)
	}
}

func TestRealPath(t *testing.T) {
	fsys, ann, _ := newHome(t)
	realAnn, _ := filepath.EvalSymlinks(ann)
	for name, want := range map[string]string{
		".":              realAnn,
		"notes.txt":      filepath.Join(realAnn, "notes.txt"),
		"papers/plan.md": filepath.Join(realAnn, "docs", "plan.md"),
	} {
		if got, err := fsys.RealPath(name); err != nil || got != want {
			t.Errorf("RealPath(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
	if _, err := fsys.RealPath("missing.txt"); err == nil {
		t.Error("RealPath of a missing file succeeded")
	}
}

func TestReadDirAndWalkDir(t *testing.T) {
	fsys, _, _ := newHome(t)
	entries, err := fsys.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	want := []string{"absolute-anna", "absolute-docs", "anna-secret", "docs", "notes.txt", "papers", "to-anna"}
	if !slices.Equal(names, want) {
		t.Errorf("ReadDir = %q, want %q", names, want)
	}

	var walked []string
	fsys.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		walked = append(walked, path)
		return err
	})
	if !slices.Contains(walked, filepath.Join("docs", "plan.md")) || slices.Contains(walked, filepath.Join("to-anna", "secret.txt")) {
		t.Errorf("WalkDir visited %q; want docs/plan.md and nothing through links", walked)
	}
}
