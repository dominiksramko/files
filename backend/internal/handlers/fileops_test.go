package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// ── Overwrite protection ─────────────────────────────────────────────────────

func TestRenameNeverOverwritesByDefault(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{"final.txt": "final"})
	response := serveJSON(t, NewFileOpsHandler().Rename, annRoot, map[string]any{"path": "notes.txt", "name": "final.txt"})
	assertStatus(t, response, http.StatusConflict)
	if conflicts := decodeResponse[struct{ Conflicts []string }](t, response).Conflicts; len(conflicts) != 1 || conflicts[0] != "final.txt" {
		t.Errorf("conflicts = %q", conflicts)
	}
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "ann's notes")
	assertFileContent(t, filepath.Join(annRoot, "final.txt"), "final")
}

func TestRenameKeepAll(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{"final.txt": "final"})
	response := serveJSON(t, NewFileOpsHandler().Rename, annRoot, map[string]any{"path": "notes.txt", "name": "final.txt", "conflict_mode": "rename"})
	assertStatus(t, response, http.StatusOK)
	if got := decodeResponse[map[string]string](t, response)["name"]; got != "final (1).txt" {
		t.Errorf("name = %q, want final (1).txt", got)
	}
	assertFileContent(t, filepath.Join(annRoot, "final.txt"), "final")
	assertFileContent(t, filepath.Join(annRoot, "final (1).txt"), "ann's notes")
}

func TestRenameReplace(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{"final.txt": "final"})
	response := serveJSON(t, NewFileOpsHandler().Rename, annRoot, map[string]any{"path": "notes.txt", "name": "final.txt", "conflict_mode": "replace"})
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "final.txt"), "ann's notes")
	assertMissing(t, filepath.Join(annRoot, "notes.txt"))
}

func TestRenameChangingCase(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	response := serveJSON(t, NewFileOpsHandler().Rename, annRoot, map[string]any{"path": "notes.txt", "name": "Notes.txt"})
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "Notes.txt"), "ann's notes")
}

// uploadRequest builds a multipart upload of files (name → content) into folder.
func uploadRequest(t *testing.T, folder, conflictMode string, files [][2]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, file := range files {
		part, err := writer.CreateFormFile("files", file[0])
		if err != nil {
			t.Fatal(err)
		}
		part.Write([]byte(file[1]))
	}
	writer.Close()
	target := "/?path=" + url.QueryEscape(folder)
	if conflictMode != "" {
		target += "&conflict_mode=" + conflictMode
	}
	request := httptest.NewRequest(http.MethodPost, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func TestUpload(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	response := serve(NewFileOpsHandler().Upload, annRoot, uploadRequest(t, "docs", "", [][2]string{{"new.txt", "new"}}))
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "docs/new.txt"), "new")
}

func TestUploadNeverOverwritesByDefault(t *testing.T) {
	// The app checks its folder listing first; the server catches what changed since.
	annRoot, _ := newUserFolders(t)
	response := serve(NewFileOpsHandler().Upload, annRoot, uploadRequest(t, "", "", [][2]string{{"new.txt", "new"}, {"notes.txt", "uploaded"}}))
	assertStatus(t, response, http.StatusConflict)
	if conflicts := decodeResponse[struct{ Conflicts []string }](t, response).Conflicts; len(conflicts) != 1 || conflicts[0] != "notes.txt" {
		t.Errorf("conflicts = %q", conflicts)
	}
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "ann's notes")
	assertMissing(t, filepath.Join(annRoot, "new.txt")) // nothing written
}

func TestUploadKeepAll(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	response := serve(NewFileOpsHandler().Upload, annRoot, uploadRequest(t, "", "rename", [][2]string{{"notes.txt", "uploaded"}}))
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "ann's notes")
	assertFileContent(t, filepath.Join(annRoot, "notes (1).txt"), "uploaded")
}

func TestUploadReplace(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	response := serve(NewFileOpsHandler().Upload, annRoot, uploadRequest(t, "", "replace", [][2]string{{"notes.txt", "uploaded"}}))
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "uploaded")
}

func TestUploadSameNameTwiceKeepsBoth(t *testing.T) {
	for _, conflictMode := range []string{"", "replace", "rename"} {
		annRoot, _ := newUserFolders(t)
		response := serve(NewFileOpsHandler().Upload, annRoot, uploadRequest(t, "docs", conflictMode, [][2]string{{"same.txt", "first"}, {"same.txt", "second"}}))
		assertStatus(t, response, http.StatusOK)
		assertFileContent(t, filepath.Join(annRoot, "docs/same.txt"), "first")
		assertFileContent(t, filepath.Join(annRoot, "docs/same (1).txt"), "second")
	}
}

func TestUploadRejectsBadNames(t *testing.T) {
	annRoot, annaRoot := newUserFolders(t)
	for _, name := range []string{"..", "."} {
		response := serve(NewFileOpsHandler().Upload, annRoot, uploadRequest(t, "docs", "replace", [][2]string{{name, "x"}}))
		assertStatus(t, response, http.StatusBadRequest)
	}
	assertUnchanged(t, annRoot, annaRoot)
}

func TestMoveAndCopySameNameTwiceKeepsBoth(t *testing.T) {
	// work/readme.md and home/readme.md into archive/, which has its own readme.md.
	for _, operation := range []string{"copy", "move"} {
		for _, conflictMode := range []string{"replace", "rename"} {
			annRoot, _ := newUserFolders(t)
			writeFiles(t, annRoot, map[string]string{"work/readme.md": "work", "home/readme.md": "home", "archive/readme.md": "archived"})
			handler := NewFileOpsHandler().Copy
			if operation == "move" {
				handler = NewFileOpsHandler().Move
			}
			response := serveJSON(t, handler, annRoot, map[string]any{
				"paths": []string{"work/readme.md", "home/readme.md"}, "dest": "archive", "conflict_mode": conflictMode,
			})
			assertStatus(t, response, http.StatusOK)

			first := "readme (1).md"
			if conflictMode == "replace" {
				first = "readme.md"
			} else {
				assertFileContent(t, filepath.Join(annRoot, "archive/readme.md"), "archived")
			}
			second := map[string]string{"replace": "readme (1).md", "rename": "readme (2).md"}[conflictMode]
			assertFileContent(t, filepath.Join(annRoot, "archive", first), "work")
			assertFileContent(t, filepath.Join(annRoot, "archive", second), "home")
		}
	}
}

func TestMoveSameNameTwiceWithoutConflictKeepsBoth(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{"work/readme.md": "work", "home/readme.md": "home"})
	response := serveJSON(t, NewFileOpsHandler().Move, annRoot, map[string]any{"paths": []string{"work/readme.md", "home/readme.md"}, "dest": "docs"})
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "docs/readme.md"), "work")
	assertFileContent(t, filepath.Join(annRoot, "docs/readme (1).md"), "home")
}

func TestMoveConflictsListedOnce(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{"work/plan.md": "work", "home/plan.md": "home"})
	response := serveJSON(t, NewFileOpsHandler().Move, annRoot, map[string]any{"paths": []string{"work/plan.md", "home/plan.md"}, "dest": "docs"})
	assertStatus(t, response, http.StatusConflict)
	if conflicts := decodeResponse[struct{ Conflicts []string }](t, response).Conflicts; len(conflicts) != 1 {
		t.Errorf("conflicts = %q, want plan.md once", conflicts)
	}
}
