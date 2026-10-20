package handlers

import (
	"archive/zip"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// writeZip creates a zip archive at path holding files (name → content).
func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	archive, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	writer := zip.NewWriter(archive)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}

// zipNames lists the entries of the zip archive at path.
func zipNames(t *testing.T, path string) []string {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer reader.Close()
	var names []string
	for _, entry := range reader.File {
		names = append(names, entry.Name)
	}
	return names
}

func compressRequest(dest, conflictMode string) map[string]any {
	return map[string]any{"paths": []string{"notes.txt"}, "dest": dest, "format": "zip", "conflict_mode": conflictMode}
}

func TestCompress(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	response := serveJSON(t, NewArchiveHandler().Compress, annRoot, compressRequest("docs/out.zip", ""))
	assertStatus(t, response, http.StatusOK)
	if names := zipNames(t, filepath.Join(annRoot, "docs/out.zip")); len(names) != 1 || names[0] != "notes.txt" {
		t.Errorf("archive holds %q", names)
	}
}

func TestCompressNeverOverwritesByDefault(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{"out.zip": "existing"})
	response := serveJSON(t, NewArchiveHandler().Compress, annRoot, compressRequest("out.zip", ""))
	assertStatus(t, response, http.StatusConflict)
	assertFileContent(t, filepath.Join(annRoot, "out.zip"), "existing")
}

func TestCompressKeepAll(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{"out.zip": "existing"})
	response := serveJSON(t, NewArchiveHandler().Compress, annRoot, compressRequest("out.zip", "rename"))
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "out.zip"), "existing")
	zipNames(t, filepath.Join(annRoot, "out (1).zip"))
}

func TestCompressReplace(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{"out.zip": "existing"})
	response := serveJSON(t, NewArchiveHandler().Compress, annRoot, compressRequest("out.zip", "replace"))
	assertStatus(t, response, http.StatusOK)
	zipNames(t, filepath.Join(annRoot, "out.zip"))
}

func TestCompressRejectsRootAndOutsideDestinations(t *testing.T) {
	handler := NewArchiveHandler()
	for _, dest := range []string{".", "/", "docs/..", "../anna/out.zip", "../anna-out.zip"} {
		annRoot, annaRoot := newUserFolders(t)
		response := serveJSON(t, handler.Compress, annRoot, compressRequest(dest, "replace"))
		assertStatus(t, response, http.StatusBadRequest)
		assertUnchanged(t, annRoot, annaRoot)
		assertMissing(t, filepath.Join(annaRoot, "out.zip"))
		assertMissing(t, filepath.Join(filepath.Dir(annRoot), "anna-out.zip"))
	}
}

func TestExtract(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeZip(t, filepath.Join(annRoot, "docs/photos.zip"), map[string]string{"a.txt": "a", "b.txt": "b"})
	response := serveJSON(t, NewArchiveHandler().Extract, annRoot, map[string]any{"path": "docs/photos.zip"})
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "docs/photos/a.txt"), "a")
	assertFileContent(t, filepath.Join(annRoot, "docs/photos/b.txt"), "b")
}

func TestExtractRejectsRootAndOutsideDestinations(t *testing.T) {
	// A failed extract removes its destination, and a single extracted item is
	// moved into the destination's parent — so the destination is never the home
	// folder itself or outside it.
	handler := NewArchiveHandler()
	for _, dest := range []string{".", "/", "docs/..", "../anna", "../anna-files"} {
		annRoot, annaRoot := newUserFolders(t)
		writeFiles(t, annRoot, map[string]string{"broken.zip": "not a zip archive"})
		response := serveJSON(t, handler.Extract, annRoot, map[string]any{"path": "broken.zip", "dest": dest, "conflict_mode": "replace"})
		assertStatus(t, response, http.StatusBadRequest)
		assertUnchanged(t, annRoot, annaRoot)
		assertMissing(t, filepath.Join(filepath.Dir(annRoot), "anna-files"))
	}
}

func TestExtractArchiveWithoutBaseName(t *testing.T) {
	// An archive named just ".zip" extracts into the folder it's in, which at
	// the top is the home folder itself.
	annRoot, annaRoot := newUserFolders(t)
	writeZip(t, filepath.Join(annRoot, ".zip"), map[string]string{"only.txt": "x"})
	response := serveJSON(t, NewArchiveHandler().Extract, annRoot, map[string]any{"path": ".zip", "conflict_mode": "replace"})
	assertStatus(t, response, http.StatusBadRequest)
	assertUnchanged(t, annRoot, annaRoot)
	assertMissing(t, filepath.Join(filepath.Dir(annRoot), "only.txt"))
}
