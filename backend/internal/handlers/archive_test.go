package handlers

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestZip creates a zip archive at path holding files (name → content).
func writeTestZip(t *testing.T, path string, files map[string]string) {
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
	writeTestZip(t, filepath.Join(annRoot, "docs/photos.zip"), map[string]string{"a.txt": "a", "b.txt": "b"})
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
	writeTestZip(t, filepath.Join(annRoot, ".zip"), map[string]string{"only.txt": "x"})
	response := serveJSON(t, NewArchiveHandler().Extract, annRoot, map[string]any{"path": ".zip", "conflict_mode": "replace"})
	assertStatus(t, response, http.StatusBadRequest)
	assertUnchanged(t, annRoot, annaRoot)
	assertMissing(t, filepath.Join(filepath.Dir(annRoot), "only.txt"))
}

// ── Overwrite protection ─────────────────────────────────────────────────────

// extract runs Extract on the archive at rel with conflictMode.
func extract(t *testing.T, root, rel, conflictMode string) *httptest.ResponseRecorder {
	t.Helper()
	return serveJSON(t, NewArchiveHandler().Extract, root, map[string]any{"path": rel, "conflict_mode": conflictMode})
}

// assertNoStaging fails if an extraction staging folder was left in dir.
func assertNoStaging(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".extract-tmp-") {
			t.Errorf("left behind: %s", entry.Name())
		}
	}
}

func assertConflicts(t *testing.T, response *httptest.ResponseRecorder, want ...string) {
	t.Helper()
	assertStatus(t, response, http.StatusConflict)
	got := decodeResponse[struct{ Conflicts []string }](t, response).Conflicts
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("conflicts = %q, want %q", got, want)
	}
}

func TestExtractFailureKeepsExistingFolder(t *testing.T) {
	// photos.zip extracts into photos/, which already exists; the extract fails.
	for _, conflictMode := range []string{"error", "replace", "rename"} {
		annRoot, _ := newUserFolders(t)
		writeFiles(t, annRoot, map[string]string{"photos/keep.jpg": "keep", "photos.zip": "not a zip archive"})
		response := extract(t, annRoot, "photos.zip", conflictMode)
		assertStatus(t, response, http.StatusInternalServerError)
		assertFileContent(t, filepath.Join(annRoot, "photos/keep.jpg"), "keep")
		assertNoStaging(t, annRoot)
	}
}

func TestExtractSingleFileNextToExistingOne(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeTestZip(t, filepath.Join(annRoot, "report.zip"), map[string]string{"notes.txt": "from archive"})

	assertConflicts(t, extract(t, annRoot, "report.zip", "error"), "notes.txt")
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "ann's notes")

	assertStatus(t, extract(t, annRoot, "report.zip", "rename"), http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "ann's notes")
	assertFileContent(t, filepath.Join(annRoot, "notes (1).txt"), "from archive")

	assertStatus(t, extract(t, annRoot, "report.zip", "replace"), http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "from archive")
	assertMissing(t, filepath.Join(annRoot, "report"))
	assertNoStaging(t, annRoot)
}

func TestExtractIntoExistingFolder(t *testing.T) {
	setup := func(t *testing.T) string {
		annRoot, _ := newUserFolders(t)
		writeFiles(t, annRoot, map[string]string{"photos/a.txt": "old a", "photos/keep.txt": "keep"})
		writeTestZip(t, filepath.Join(annRoot, "photos.zip"), map[string]string{"a.txt": "new a", "b.txt": "new b"})
		return annRoot
	}

	annRoot := setup(t)
	assertConflicts(t, extract(t, annRoot, "photos.zip", "error"), "photos/a.txt")
	assertFileContent(t, filepath.Join(annRoot, "photos/a.txt"), "old a")
	assertMissing(t, filepath.Join(annRoot, "photos/b.txt"))

	annRoot = setup(t)
	assertStatus(t, extract(t, annRoot, "photos.zip", "rename"), http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "photos/a.txt"), "old a")
	assertFileContent(t, filepath.Join(annRoot, "photos/a (1).txt"), "new a")
	assertFileContent(t, filepath.Join(annRoot, "photos/b.txt"), "new b")
	assertFileContent(t, filepath.Join(annRoot, "photos/keep.txt"), "keep")

	annRoot = setup(t)
	assertStatus(t, extract(t, annRoot, "photos.zip", "replace"), http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "photos/a.txt"), "new a")
	assertFileContent(t, filepath.Join(annRoot, "photos/b.txt"), "new b")
	assertFileContent(t, filepath.Join(annRoot, "photos/keep.txt"), "keep")
	assertNoStaging(t, annRoot)
}

func TestExtractFolderNameTakenByFile(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{"photos": "a file"})
	writeTestZip(t, filepath.Join(annRoot, "photos.zip"), map[string]string{"a.txt": "a", "b.txt": "b"})

	assertConflicts(t, extract(t, annRoot, "photos.zip", "error"), "photos")
	assertStatus(t, extract(t, annRoot, "photos.zip", "rename"), http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "photos"), "a file")
	assertFileContent(t, filepath.Join(annRoot, "photos (1)/a.txt"), "a")
}

func TestExtractSingleFolder(t *testing.T) {
	// project.zip holding project/… extracts to project/, not project/project/.
	annRoot, _ := newUserFolders(t)
	writeTestZip(t, filepath.Join(annRoot, "project.zip"), map[string]string{"project/main.go": "package main"})
	assertStatus(t, extract(t, annRoot, "project.zip", "error"), http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "project/main.go"), "package main")
}

func TestExtractToExplicitDestination(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeTestZip(t, filepath.Join(annRoot, "one.zip"), map[string]string{"only.txt": "only"})
	response := serveJSON(t, NewArchiveHandler().Extract, annRoot, map[string]any{"path": "one.zip", "dest": "docs/out"})
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "docs/out/only.txt"), "only")
	assertNoStaging(t, filepath.Join(annRoot, "docs"))
}
