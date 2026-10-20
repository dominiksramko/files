package handlers

import (
	"archive/tar"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newLinkedFolders is newUserFolders with two links in ann's folder: "to-anna"
// leading to anna's folder, and "papers" pointing at ann's own docs/.
func newLinkedFolders(t *testing.T) (annRoot, annaRoot string) {
	t.Helper()
	annRoot, annaRoot = newUserFolders(t)
	if err := os.Symlink("../anna", filepath.Join(annRoot, "to-anna")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("docs", filepath.Join(annRoot, "papers")); err != nil {
		t.Fatal(err)
	}
	return annRoot, annaRoot
}

func TestOperationsThroughOutsideLinkFail(t *testing.T) {
	handler := NewFileOpsHandler()
	tests := map[string]func(t *testing.T, root string) int{
		"delete": func(t *testing.T, root string) int {
			return serveJSON(t, handler.Delete, root, map[string]any{"paths": []string{"to-anna/secret.txt"}}).Code
		},
		"rename": func(t *testing.T, root string) int {
			return serveJSON(t, handler.Rename, root, map[string]any{"path": "to-anna/secret.txt", "name": "x.txt"}).Code
		},
		"copy out": func(t *testing.T, root string) int {
			return serveJSON(t, handler.Copy, root, map[string]any{"paths": []string{"to-anna/secret.txt"}, "dest": "docs"}).Code
		},
		"move out": func(t *testing.T, root string) int {
			return serveJSON(t, handler.Move, root, map[string]any{"paths": []string{"to-anna/secret.txt"}, "dest": "docs"}).Code
		},
		"copy in": func(t *testing.T, root string) int {
			return serveJSON(t, handler.Copy, root, map[string]any{"paths": []string{"notes.txt"}, "dest": "to-anna"}).Code
		},
		"move in": func(t *testing.T, root string) int {
			return serveJSON(t, handler.Move, root, map[string]any{"paths": []string{"notes.txt"}, "dest": "to-anna"}).Code
		},
		"create": func(t *testing.T, root string) int {
			return serveJSON(t, handler.Create, root, map[string]any{"path": "to-anna/new.txt"}).Code
		},
		"mkdir": func(t *testing.T, root string) int {
			return serveJSON(t, handler.Mkdir, root, map[string]any{"path": "to-anna/new"}).Code
		},
		"upload": func(t *testing.T, root string) int {
			return serve(handler.Upload, root, uploadRequest(t, "to-anna", "replace", [][2]string{{"secret.txt", "overwritten"}})).Code
		},
		"compress": func(t *testing.T, root string) int {
			return serveJSON(t, NewArchiveHandler().Compress, root, map[string]any{"paths": []string{"to-anna"}, "dest": "out.zip", "format": "zip"}).Code
		},
	}
	for name, operation := range tests {
		annRoot, annaRoot := newLinkedFolders(t)
		if status := operation(t, annRoot); status == http.StatusOK {
			t.Errorf("%s through the link succeeded", name)
		}
		assertUnchanged(t, annRoot, annaRoot)
		entries, _ := os.ReadDir(annaRoot)
		if len(entries) != 1 {
			t.Errorf("%s: anna's folder now has %d entries", name, len(entries))
		}
		assertMissing(t, filepath.Join(annRoot, "docs", "secret.txt"))
	}
}

func TestDeletingALinkKeepsItsTarget(t *testing.T) {
	annRoot, annaRoot := newLinkedFolders(t)
	assertStatus(t, serveJSON(t, NewFileOpsHandler().Delete, annRoot, map[string]any{"paths": []string{"to-anna"}}), http.StatusOK)
	assertMissing(t, filepath.Join(annRoot, "to-anna"))
	assertFileContent(t, filepath.Join(annaRoot, "secret.txt"), "anna's secret")
}

func TestLinksInsideTheFolderWork(t *testing.T) {
	annRoot, _ := newLinkedFolders(t)
	response := serveJSON(t, NewFileOpsHandler().Copy, annRoot, map[string]any{"paths": []string{"papers/plan.md"}, "dest": ""})
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "plan.md"), "# plan")
}

func TestCopyIntoLinkToItselfRefused(t *testing.T) {
	// papers is docs: copying docs into it would copy a folder into itself.
	annRoot, _ := newLinkedFolders(t)
	response := serveJSON(t, NewFileOpsHandler().Copy, annRoot, map[string]any{"paths": []string{"docs"}, "dest": "papers", "conflict_mode": "rename"})
	assertStatus(t, response, http.StatusBadRequest)
	assertMissing(t, filepath.Join(annRoot, "docs", "docs"))
}

// assertNoLinks fails if there's a symbolic link anywhere under dir.
func assertNoLinks(t *testing.T, dir string) {
	t.Helper()
	filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err == nil && entry.Type()&os.ModeSymlink != 0 {
			t.Errorf("link extracted: %s", path)
		}
		return nil
	})
}

func TestTarExtractionSkipsLinks(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	archive, err := os.Create(filepath.Join(annRoot, "linked.tar"))
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(archive)
	writer.WriteHeader(&tar.Header{Name: "linked/a.txt", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	writer.Write([]byte("a"))
	writer.WriteHeader(&tar.Header{Name: "linked/b-link", Linkname: "a.txt", Typeflag: tar.TypeSymlink})
	writer.Close()
	archive.Close()

	assertStatus(t, serveJSON(t, NewArchiveHandler().Extract, annRoot, map[string]any{"path": "linked.tar"}), http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "linked/a.txt"), "a")
	assertMissing(t, filepath.Join(annRoot, "linked/b-link"))
	assertNoLinks(t, annRoot+"/linked")
}

func TestSevenZipExtractionRemovesLinks(t *testing.T) {
	if _, err := exec.LookPath("7z"); err != nil {
		t.Skip("needs 7z (the dev container has it)")
	}
	annRoot, _ := newUserFolders(t)
	source := t.TempDir()
	writeFiles(t, source, map[string]string{"bundle/a.txt": "a"})
	os.Symlink("a.txt", filepath.Join(source, "bundle/b-link"))
	if out, err := exec.Command("7z", "a", "-snl", filepath.Join(annRoot, "bundle.7z"), filepath.Join(source, "bundle")).CombinedOutput(); err != nil {
		t.Fatalf("7z: %v %s", err, out)
	}

	assertStatus(t, serveJSON(t, NewArchiveHandler().Extract, annRoot, map[string]any{"path": "bundle.7z"}), http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "bundle/a.txt"), "a")
	assertNoLinks(t, annRoot)
}
