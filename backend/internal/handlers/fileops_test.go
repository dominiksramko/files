package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// Paths that must never be deleted, renamed or moved: the home folder itself
// (its parent holds the other users' folders) and anything outside it.
var protectedPaths = []string{"", ".", "/", "docs/..", "..", "../anna", "../anna/secret.txt"}

func TestDeleteRejectsRootAndOutsidePaths(t *testing.T) {
	handler := NewFileOpsHandler()
	for _, rel := range protectedPaths {
		annRoot, annaRoot := newUserFolders(t)
		response := serveJSON(t, handler.Delete, annRoot, map[string]any{"paths": []string{rel}})
		assertStatus(t, response, http.StatusBadRequest)
		assertUnchanged(t, annRoot, annaRoot)
	}
}

func TestDeleteChecksEveryPathFirst(t *testing.T) {
	annRoot, annaRoot := newUserFolders(t)
	response := serveJSON(t, NewFileOpsHandler().Delete, annRoot, map[string]any{"paths": []string{"notes.txt", ""}})
	assertStatus(t, response, http.StatusBadRequest)
	assertUnchanged(t, annRoot, annaRoot)
}

func TestDelete(t *testing.T) {
	annRoot, annaRoot := newUserFolders(t)
	response := serveJSON(t, NewFileOpsHandler().Delete, annRoot, map[string]any{"paths": []string{"notes.txt", "docs"}})
	assertStatus(t, response, http.StatusOK)
	assertMissing(t, filepath.Join(annRoot, "notes.txt"))
	assertMissing(t, filepath.Join(annRoot, "docs"))
	assertFileContent(t, filepath.Join(annaRoot, "secret.txt"), "anna's secret")
}

func TestRenameRejectsRootAndOutsidePaths(t *testing.T) {
	handler := NewFileOpsHandler()
	for _, rel := range protectedPaths {
		annRoot, annaRoot := newUserFolders(t)
		response := serveJSON(t, handler.Rename, annRoot, map[string]any{"path": rel, "name": "anna-new"})
		if response.Code == http.StatusOK {
			t.Errorf("rename %q succeeded", rel)
		}
		assertUnchanged(t, annRoot, annaRoot)
		assertMissing(t, filepath.Join(filepath.Dir(annRoot), "anna-new"))
	}
}

func TestRenameRejectsNamesLeavingTheFolder(t *testing.T) {
	handler := NewFileOpsHandler()
	for _, name := range []string{".", "..", "../anna", "docs/notes.txt", `docs\notes.txt`} {
		annRoot, annaRoot := newUserFolders(t)
		response := serveJSON(t, handler.Rename, annRoot, map[string]any{"path": "notes.txt", "name": name})
		assertStatus(t, response, http.StatusBadRequest)
		assertUnchanged(t, annRoot, annaRoot)
	}
}

func TestRename(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	response := serveJSON(t, NewFileOpsHandler().Rename, annRoot, map[string]any{"path": "docs/plan.md", "name": "roadmap.md"})
	assertStatus(t, response, http.StatusOK)
	assertMissing(t, filepath.Join(annRoot, "docs/plan.md"))
	assertFileContent(t, filepath.Join(annRoot, "docs/roadmap.md"), "# plan")
}

func TestMoveAndCopyRejectRootAndOutsideSources(t *testing.T) {
	handler := NewFileOpsHandler()
	for name, operation := range map[string]func(*testing.T, string, string){
		"move": func(t *testing.T, root, source string) {
			response := serveJSON(t, handler.Move, root, map[string]any{"paths": []string{source}, "dest": "docs"})
			assertStatus(t, response, http.StatusBadRequest)
		},
		"copy": func(t *testing.T, root, source string) {
			response := serveJSON(t, handler.Copy, root, map[string]any{"paths": []string{source}, "dest": "docs"})
			assertStatus(t, response, http.StatusBadRequest)
		},
	} {
		for _, rel := range protectedPaths {
			annRoot, annaRoot := newUserFolders(t)
			operation(t, annRoot, rel)
			assertUnchanged(t, annRoot, annaRoot)
			if entries, _ := os.ReadDir(filepath.Join(annRoot, "docs")); len(entries) != 2 {
				t.Errorf("%s %q changed docs/: %d entries", name, rel, len(entries))
			}
		}
	}
}

func TestMoveAndCopyRejectOutsideDestinations(t *testing.T) {
	handler := NewFileOpsHandler()
	for _, operation := range []func(*testing.T, string){
		func(t *testing.T, root string) {
			assertStatus(t, serveJSON(t, handler.Move, root, map[string]any{"paths": []string{"notes.txt"}, "dest": "../anna"}), http.StatusBadRequest)
		},
		func(t *testing.T, root string) {
			assertStatus(t, serveJSON(t, handler.Copy, root, map[string]any{"paths": []string{"notes.txt"}, "dest": "../anna"}), http.StatusBadRequest)
		},
	} {
		annRoot, annaRoot := newUserFolders(t)
		operation(t, annRoot)
		assertUnchanged(t, annRoot, annaRoot)
		assertMissing(t, filepath.Join(annaRoot, "notes.txt"))
	}
}

func TestMoveIntoRootFolder(t *testing.T) {
	// Moving an item to the top of the home folder (dest "") keeps working.
	annRoot, _ := newUserFolders(t)
	response := serveJSON(t, NewFileOpsHandler().Move, annRoot, map[string]any{"paths": []string{"docs/plan.md"}, "dest": ""})
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "plan.md"), "# plan")
}

func TestCreate(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	response := serveJSON(t, NewFileOpsHandler().Create, annRoot, map[string]any{"path": "docs/new.txt"})
	assertStatus(t, response, http.StatusOK)
	if got := decodeResponse[map[string]string](t, response)["name"]; got != "new.txt" {
		t.Errorf("name = %q, want new.txt", got)
	}
	assertFileContent(t, filepath.Join(annRoot, "docs/new.txt"), "")
}

func TestCreateNeverOverwritesByDefault(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	response := serveJSON(t, NewFileOpsHandler().Create, annRoot, map[string]any{"path": "notes.txt"})
	assertStatus(t, response, http.StatusConflict)
	conflicts := decodeResponse[struct{ Conflicts []string }](t, response).Conflicts
	if len(conflicts) != 1 || conflicts[0] != "notes.txt" {
		t.Errorf("conflicts = %q, want [notes.txt]", conflicts)
	}
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "ann's notes")
}

func TestCreateKeepAll(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{"notes (1).txt": "older copy"})
	response := serveJSON(t, NewFileOpsHandler().Create, annRoot, map[string]any{"path": "notes.txt", "conflict_mode": "rename"})
	assertStatus(t, response, http.StatusOK)
	if got := decodeResponse[map[string]string](t, response)["name"]; got != "notes (2).txt" {
		t.Errorf("name = %q, want notes (2).txt", got)
	}
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "ann's notes")
	assertFileContent(t, filepath.Join(annRoot, "notes (1).txt"), "older copy")
	assertFileContent(t, filepath.Join(annRoot, "notes (2).txt"), "")
}

func TestCreateReplace(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	response := serveJSON(t, NewFileOpsHandler().Create, annRoot, map[string]any{"path": "notes.txt", "conflict_mode": "replace"})
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "")
}

func TestCreateRejectsRootAndOutsidePaths(t *testing.T) {
	handler := NewFileOpsHandler()
	for _, rel := range []string{".", "/", "docs/..", "../anna/new.txt", "../anna-new.txt"} {
		annRoot, annaRoot := newUserFolders(t)
		response := serveJSON(t, handler.Create, annRoot, map[string]any{"path": rel, "conflict_mode": "replace"})
		assertStatus(t, response, http.StatusBadRequest)
		assertUnchanged(t, annRoot, annaRoot)
		assertMissing(t, filepath.Join(annaRoot, "new.txt"))
		assertMissing(t, filepath.Join(filepath.Dir(annRoot), "anna-new.txt"))
	}
}

func TestResolveDestConflict(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "a", "a (1).txt": "a1", "b": "b"})

	t.Run("free name is kept", func(t *testing.T) {
		resolved, conflict, err := resolveDestConflict(filepath.Join(dir, "new.txt"), "error")
		if resolved != filepath.Join(dir, "new.txt") || conflict || err != nil {
			t.Errorf("= %q, %v, %v", resolved, conflict, err)
		}
	})
	t.Run("error mode reports the conflict", func(t *testing.T) {
		resolved, conflict, err := resolveDestConflict(filepath.Join(dir, "a.txt"), "error")
		if resolved != filepath.Join(dir, "a.txt") || !conflict || err != nil {
			t.Errorf("= %q, %v, %v", resolved, conflict, err)
		}
		assertFileContent(t, filepath.Join(dir, "a.txt"), "a")
	})
	t.Run("rename picks the next free name", func(t *testing.T) {
		resolved, conflict, err := resolveDestConflict(filepath.Join(dir, "a.txt"), "rename")
		if resolved != filepath.Join(dir, "a (2).txt") || conflict || err != nil {
			t.Errorf("= %q, %v, %v", resolved, conflict, err)
		}
	})
	t.Run("replace removes the existing file", func(t *testing.T) {
		resolved, conflict, err := resolveDestConflict(filepath.Join(dir, "b"), "replace")
		if resolved != filepath.Join(dir, "b") || conflict || err != nil {
			t.Errorf("= %q, %v, %v", resolved, conflict, err)
		}
		assertMissing(t, filepath.Join(dir, "b"))
	})
	t.Run("replace never removes a non-empty folder", func(t *testing.T) {
		writeFiles(t, dir, map[string]string{"folder/inside.txt": "kept"})
		if _, _, err := resolveDestConflict(filepath.Join(dir, "folder"), "replace"); err == nil {
			t.Error("replace removed a folder")
		}
		assertFileContent(t, filepath.Join(dir, "folder/inside.txt"), "kept")
	})
}
