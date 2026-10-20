package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// Paths go back to the browser as it writes them: the home folder is "", not
// ".", so the app can compare them with its own.
func TestStatPaths(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	h := NewFilesHandler(nil, "")
	for request, want := range map[string]string{"": "", "/": "", "docs": "docs", "docs/plan.md": "docs/plan.md"} {
		response := serve(h.Stat, annRoot, httptest.NewRequest(http.MethodGet, "/?"+url.Values{"path": {request}}.Encode(), nil))
		assertStatus(t, response, http.StatusOK)
		if entry := decodeResponse[FileEntry](t, response); entry.Path != want {
			t.Errorf("stat %q: path = %q, want %q", request, entry.Path, want)
		}
	}

	response := serve(h.Stat, annRoot, httptest.NewRequest(http.MethodGet, "/?path=../anna", nil))
	assertStatus(t, response, http.StatusBadRequest)
}
