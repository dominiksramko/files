package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"files/internal/userfs"
)

func TestRenameApplyRejectsRootAndOutsideSources(t *testing.T) {
	// Renaming the home folder itself to a neighbour's name used to delete that
	// user's folder ("replace") and move this one into its place.
	handler := NewAdvancedRenamerHandler(nil)
	for _, conflictMode := range []string{"error", "replace", "rename"} {
		for _, rel := range protectedPaths {
			annRoot, annaRoot := newUserFolders(t)
			response := serveJSON(t, handler.Apply, annRoot, map[string]any{
				"renames":       []map[string]string{{"from": rel, "to": "anna"}},
				"conflict_mode": conflictMode,
			})
			assertStatus(t, response, http.StatusBadRequest)
			assertUnchanged(t, annRoot, annaRoot)
		}
	}
}

func TestRenameApplyChecksEveryRenameFirst(t *testing.T) {
	annRoot, annaRoot := newUserFolders(t)
	response := serveJSON(t, NewAdvancedRenamerHandler(nil).Apply, annRoot, map[string]any{
		"renames": []map[string]string{{"from": "notes.txt", "to": "renamed.txt"}, {"from": "", "to": "anna"}},
	})
	assertStatus(t, response, http.StatusBadRequest)
	assertUnchanged(t, annRoot, annaRoot)
}

func TestRenameApplyRejectsNamesLeavingTheFolder(t *testing.T) {
	handler := NewAdvancedRenamerHandler(nil)
	for _, name := range []string{"", ".", "..", "../anna", "docs/x"} {
		annRoot, annaRoot := newUserFolders(t)
		response := serveJSON(t, handler.Apply, annRoot, map[string]any{
			"renames":       []map[string]string{{"from": "notes.txt", "to": name}},
			"conflict_mode": "replace",
		})
		assertStatus(t, response, http.StatusBadRequest)
		assertUnchanged(t, annRoot, annaRoot)
	}
}

func TestRenameApply(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	response := serveJSON(t, NewAdvancedRenamerHandler(nil).Apply, annRoot, map[string]any{
		"renames": []map[string]string{{"from": "docs/plan.md", "to": "1 plan.md"}, {"from": "docs", "to": "papers"}},
	})
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "papers/1 plan.md"), "# plan")
	assertMissing(t, filepath.Join(annRoot, "docs"))
}

// applyRenames runs the advanced renamer on renames (from → to, in order).
func applyRenames(t *testing.T, root, conflictMode string, renames ...[2]string) *httptest.ResponseRecorder {
	t.Helper()
	ops := make([]map[string]string, len(renames))
	for i, rename := range renames {
		ops[i] = map[string]string{"from": rename[0], "to": rename[1]}
	}
	return serveJSON(t, NewAdvancedRenamerHandler(nil).Apply, root, map[string]any{"renames": ops, "conflict_mode": conflictMode})
}

// assertNoTempNames fails if a temporary rename name was left in dir.
func assertNoTempNames(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".~rename-") {
			t.Errorf("left behind: %s", entry.Name())
		}
	}
}

func TestRenameApplySameNameTwiceKeepsBoth(t *testing.T) {
	// A template like {date}.jpg giving two photos the same name.
	for _, conflictMode := range []string{"error", "replace", "rename"} {
		annRoot, _ := newUserFolders(t)
		writeFiles(t, annRoot, map[string]string{"a.jpg": "a", "b.jpg": "b"})
		response := applyRenames(t, annRoot, conflictMode, [2]string{"a.jpg", "2026.jpg"}, [2]string{"b.jpg", "2026.jpg"})
		assertStatus(t, response, http.StatusOK)
		assertFileContent(t, filepath.Join(annRoot, "2026.jpg"), "a")
		assertFileContent(t, filepath.Join(annRoot, "2026 (1).jpg"), "b")
	}
}

func TestRenameApplyChain(t *testing.T) {
	// b is renamed away in the same batch, so a can take its name.
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{"a.txt": "a", "b.txt": "b"})
	response := applyRenames(t, annRoot, "error", [2]string{"a.txt", "b.txt"}, [2]string{"b.txt", "c.txt"})
	assertStatus(t, response, http.StatusOK)
	assertMissing(t, filepath.Join(annRoot, "a.txt"))
	assertFileContent(t, filepath.Join(annRoot, "b.txt"), "a")
	assertFileContent(t, filepath.Join(annRoot, "c.txt"), "b")
	assertNoTempNames(t, annRoot)
}

func TestRenameApplySwap(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{"a.txt": "a", "b.txt": "b"})
	response := applyRenames(t, annRoot, "error", [2]string{"a.txt", "b.txt"}, [2]string{"b.txt", "a.txt"})
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "a.txt"), "b")
	assertFileContent(t, filepath.Join(annRoot, "b.txt"), "a")
	assertNoTempNames(t, annRoot)
}

func TestRenameApplyConflictWithItemOutsideBatch(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{"a.txt": "a", "taken.txt": "taken"})

	response := applyRenames(t, annRoot, "error", [2]string{"a.txt", "taken.txt"}, [2]string{"notes.txt", "renamed.txt"})
	assertStatus(t, response, http.StatusConflict)
	assertFileContent(t, filepath.Join(annRoot, "a.txt"), "a")
	assertFileContent(t, filepath.Join(annRoot, "taken.txt"), "taken")
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "ann's notes") // nothing renamed

	assertStatus(t, applyRenames(t, annRoot, "rename", [2]string{"a.txt", "taken.txt"}), http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "taken.txt"), "taken")
	assertFileContent(t, filepath.Join(annRoot, "taken (1).txt"), "a")

	writeFiles(t, annRoot, map[string]string{"b.txt": "b"})
	assertStatus(t, applyRenames(t, annRoot, "replace", [2]string{"b.txt", "taken.txt"}), http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "taken.txt"), "b")
}

func TestRenameApplyRejectsSameItemTwice(t *testing.T) {
	annRoot, annaRoot := newUserFolders(t)
	response := applyRenames(t, annRoot, "error", [2]string{"notes.txt", "x.txt"}, [2]string{"./notes.txt", "y.txt"})
	assertStatus(t, response, http.StatusBadRequest)
	assertUnchanged(t, annRoot, annaRoot)
}

func TestRenameApplyFolderAndItemInside(t *testing.T) {
	// Either order: the item inside is renamed before its folder moves.
	for _, renames := range [][][2]string{
		{{"docs/plan.md", "1 plan.md"}, {"docs", "papers"}},
		{{"docs", "papers"}, {"docs/plan.md", "1 plan.md"}},
	} {
		annRoot, _ := newUserFolders(t)
		assertStatus(t, applyRenames(t, annRoot, "error", renames...), http.StatusOK)
		assertFileContent(t, filepath.Join(annRoot, "papers/1 plan.md"), "# plan")
		assertNoTempNames(t, annRoot)
		assertNoTempNames(t, filepath.Join(annRoot, "papers"))
	}
}

func TestRenameAllRestoresOnFailure(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a": "a", "b": "b"})
	fsys, err := userfs.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	err = renameAll(fsys, []pathMove{
		{from: "a", to: "a2"},
		{from: "b", to: filepath.Join("missing-folder", "b2")},
	})
	if err == nil {
		t.Fatal("renaming into a missing folder succeeded")
	}
	assertFileContent(t, filepath.Join(dir, "a2"), "a") // finished before the failure
	assertFileContent(t, filepath.Join(dir, "b"), "b")  // put back
	assertNoTempNames(t, dir)
}

func TestTitleCase(t *testing.T) {
	for value, want := range map[string]string{
		"šramko":          "Šramko",
		"élan vital":      "Élan Vital",
		"hello WORLD":     "Hello World",
		"ǆungla":          "ǅungla", // title case differs from upper case
		"a":               "A",
		"čaj a  káva":     "Čaj A Káva",
		"123 main street": "123 Main Street",
	} {
		if got := applyTransform(value, "title"); got != want {
			t.Errorf("title(%q) = %q, want %q", value, got, want)
		}
	}
}
