package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func readContentRequest(rel string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/?path="+url.QueryEscape(rel), nil)
}

// writeContentRequest saves content to rel, based on baseVersion ("" = none).
func writeContentRequest(rel, content, baseVersion string) *http.Request {
	request := httptest.NewRequest(http.MethodPut, "/?path="+url.QueryEscape(rel), strings.NewReader(content))
	if baseVersion != "" {
		request.Header.Set("If-Match", `"`+baseVersion+`"`)
	}
	return request
}

type savedContent struct {
	Version string `json:"version"`
	ModTime string `json:"mod_time"`
}

type refusedSave struct {
	Content string `json:"content"`
	Version string `json:"version"`
}

func TestContentVersion(t *testing.T) {
	if contentVersion([]byte("a")) == contentVersion([]byte("b")) {
		t.Error("different content, same version")
	}
	if contentVersion([]byte("a")) != contentVersion([]byte("a")) {
		t.Error("same content, different versions")
	}
}

func TestReadContent(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	response := serve(NewFilesHandler(nil, "").ReadContent, annRoot, readContentRequest("notes.txt"))
	assertStatus(t, response, http.StatusOK)
	if response.Body.String() != "ann's notes" {
		t.Errorf("body = %q", response.Body.String())
	}
	if got, want := response.Header().Get("ETag"), `"`+contentVersion([]byte("ann's notes"))+`"`; got != want {
		t.Errorf("ETag = %s, want %s", got, want)
	}
}

func TestReadContentRejects(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	handler := NewFilesHandler(nil, "").ReadContent
	assertStatus(t, serve(handler, annRoot, readContentRequest("../anna/secret.txt")), http.StatusBadRequest)
	assertStatus(t, serve(handler, annRoot, readContentRequest("docs")), http.StatusBadRequest)
	assertStatus(t, serve(handler, annRoot, readContentRequest("missing.txt")), http.StatusNotFound)
}

func TestWriteContentOnCurrentVersion(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	base := contentVersion([]byte("ann's notes"))
	response := serve(NewFilesHandler(nil, "").WriteContent, annRoot, writeContentRequest("notes.txt", "edited", base))
	assertStatus(t, response, http.StatusOK)
	saved := decodeResponse[savedContent](t, response)
	if saved.Version != contentVersion([]byte("edited")) || saved.ModTime == "" {
		t.Errorf("response = %+v", saved)
	}
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "edited")
}

func TestWriteContentOnStaleVersion(t *testing.T) {
	// The file changed since the editor loaded it: nothing is written, and the
	// current content comes back for merging.
	annRoot, _ := newUserFolders(t)
	stale := contentVersion([]byte("an older version"))
	response := serve(NewFilesHandler(nil, "").WriteContent, annRoot, writeContentRequest("notes.txt", "edited", stale))
	assertStatus(t, response, http.StatusPreconditionFailed)
	refused := decodeResponse[refusedSave](t, response)
	if refused.Content != "ann's notes" || refused.Version != contentVersion([]byte("ann's notes")) {
		t.Errorf("response = %+v", refused)
	}
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "ann's notes")
}

func TestWriteContentSimultaneousSaves(t *testing.T) {
	// Two editors save on the same version at once: exactly one wins, the other
	// is refused with the winner's content.
	annRoot, _ := newUserFolders(t)
	handler := NewFilesHandler(nil, "").WriteContent
	base := contentVersion([]byte("ann's notes"))

	responses := make([]*httptest.ResponseRecorder, 2)
	var wait sync.WaitGroup
	for index, content := range []string{"from one", "from two"} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			responses[index] = serve(handler, annRoot, writeContentRequest("notes.txt", content, base))
		}()
	}
	wait.Wait()

	saved, refused := 0, 0
	for _, response := range responses {
		switch response.Code {
		case http.StatusOK:
			saved++
		case http.StatusPreconditionFailed:
			refused++
			if winner := decodeResponse[refusedSave](t, response).Content; winner != readFile(t, filepath.Join(annRoot, "notes.txt")) {
				t.Errorf("refused save reported %q, file holds the other", winner)
			}
		}
	}
	if saved != 1 || refused != 1 {
		t.Errorf("%d saved, %d refused; want 1 and 1", saved, refused)
	}
}

func TestWriteContentWithoutVersion(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	response := serve(NewFilesHandler(nil, "").WriteContent, annRoot, writeContentRequest("notes.txt", "overwritten", ""))
	assertStatus(t, response, http.StatusOK)
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "overwritten")
}

func TestWriteContentDeletedFile(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	base := contentVersion([]byte("gone"))
	response := serve(NewFilesHandler(nil, "").WriteContent, annRoot, writeContentRequest("missing.txt", "edited", base))
	assertStatus(t, response, http.StatusNotFound)
	assertMissing(t, filepath.Join(annRoot, "missing.txt"))
}

func TestWriteContentRejects(t *testing.T) {
	annRoot, annaRoot := newUserFolders(t)
	handler := NewFilesHandler(nil, "").WriteContent
	assertStatus(t, serve(handler, annRoot, writeContentRequest("../anna/secret.txt", "x", "")), http.StatusBadRequest)
	assertStatus(t, serve(handler, annRoot, writeContentRequest("docs", "x", "")), http.StatusBadRequest)
	assertStatus(t, serve(handler, annRoot, writeContentRequest("big.txt", strings.Repeat("x", maxEditableSize+1), "")), http.StatusRequestEntityTooLarge)
	assertUnchanged(t, annRoot, annaRoot)
	assertMissing(t, filepath.Join(annRoot, "big.txt"))
}

func TestWriteContentIsAtomic(t *testing.T) {
	// A reader during saves sees either the old or the new content in full.
	annRoot, _ := newUserFolders(t)
	old, updated := strings.Repeat("a", 1<<20), strings.Repeat("b", 1<<20)
	writeFiles(t, annRoot, map[string]string{"big.txt": old})
	handler := NewFilesHandler(nil, "").WriteContent

	stop := make(chan struct{})
	torn := make(chan string, 1)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(filepath.Join(annRoot, "big.txt"))
			if err == nil && string(data) != old && string(data) != updated {
				select {
				case torn <- fmt.Sprintf("%d bytes", len(data)):
				default:
				}
			}
		}
	}()
	for i := 0; i < 20; i++ {
		content := updated
		if i%2 == 1 {
			content = old
		}
		assertStatus(t, serve(handler, annRoot, writeContentRequest("big.txt", content, "")), http.StatusOK)
	}
	close(stop)
	select {
	case got := <-torn:
		t.Errorf("a reader saw a half-written file (%s)", got)
	default:
	}
	entries, _ := os.ReadDir(annRoot)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".~save_") {
			t.Errorf("temporary file left: %s", entry.Name())
		}
	}
}

func TestWriteContentKeepsPermissionsAndLinks(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	os.Chmod(filepath.Join(annRoot, "notes.txt"), 0o600)
	os.Symlink("docs/plan.md", filepath.Join(annRoot, "plan-link.md"))
	handler := NewFilesHandler(nil, "").WriteContent

	assertStatus(t, serve(handler, annRoot, writeContentRequest("notes.txt", "edited", "")), http.StatusOK)
	if info, _ := os.Stat(filepath.Join(annRoot, "notes.txt")); info.Mode().Perm() != 0o600 {
		t.Errorf("permissions now %v, want 0600", info.Mode().Perm())
	}

	assertStatus(t, serve(handler, annRoot, writeContentRequest("plan-link.md", "# new plan", "")), http.StatusOK)
	if info, _ := os.Lstat(filepath.Join(annRoot, "plan-link.md")); info.Mode()&os.ModeSymlink == 0 {
		t.Error("saving through a link replaced the link with a file")
	}
	assertFileContent(t, filepath.Join(annRoot, "docs/plan.md"), "# new plan")
}
