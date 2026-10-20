package handlers

import (
	"net/http"
	"path/filepath"
	"testing"
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
