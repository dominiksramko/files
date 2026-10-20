package handlers

// Shared test helpers: call any handler as a logged-in user against throwaway
// folders, without the router or a login token.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"files/internal/userfs"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newUserFolders creates a users folder holding two home folders, "ann" and
// "anna" — anna's name starts with ann's, the case a text-prefix check gets
// wrong — each with a few files. Returns both home folders.
func newUserFolders(t *testing.T) (annRoot, annaRoot string) {
	t.Helper()
	usersDir := t.TempDir()
	annRoot = filepath.Join(usersDir, "ann")
	annaRoot = filepath.Join(usersDir, "anna")
	writeFiles(t, annRoot, map[string]string{
		"notes.txt":      "ann's notes",
		"docs/plan.md":   "# plan",
		"docs/todo.todo": "- [ ] task",
	})
	writeFiles(t, annaRoot, map[string]string{
		"secret.txt": "anna's secret",
	})
	return annRoot, annaRoot
}

// writeFiles creates each file (path relative to dir → content), with its folders.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// readFile returns the file's content, failing the test if it can't be read.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// assertFileContent fails unless path holds exactly want.
func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	if got := readFile(t, path); got != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}

// assertMissing fails if anything exists at path.
func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s exists, want it missing (stat error: %v)", path, err)
	}
}

// assertUnchanged fails unless both users' folders still hold what
// newUserFolders put there.
func assertUnchanged(t *testing.T, annRoot, annaRoot string) {
	t.Helper()
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "ann's notes")
	assertFileContent(t, filepath.Join(annRoot, "docs/plan.md"), "# plan")
	assertFileContent(t, filepath.Join(annaRoot, "secret.txt"), "anna's secret")
}

// jsonRequest builds a request with body encoded as JSON.
func jsonRequest(t *testing.T, method, target string, body any) *http.Request {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, target, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	return request
}

// serve runs handler on request as a logged-in user whose home folder is root,
// as the auth and CloseUserFS middleware would set it up.
func serve(handler gin.HandlerFunc, root string, request *http.Request) *httptest.ResponseRecorder {
	return serveWith(handler, root, request, nil)
}

// serveWith is serve with setup changing the request context first (another
// user ID, a session that has ended, ...).
func serveWith(handler gin.HandlerFunc, root string, request *http.Request, setup func(*gin.Context)) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request
	ctx.Set("userID", int64(1))
	ctx.Set("username", "ann")
	ctx.Set("rootDir", root)
	if setup != nil {
		setup(ctx)
	}
	handler(ctx)
	if opened, ok := ctx.Get(userFSKey); ok {
		opened.(*userfs.FS).Close()
	}
	return recorder
}

// serveJSON runs handler with body sent as JSON (for handlers that only read the body).
func serveJSON(t *testing.T, handler gin.HandlerFunc, root string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return serve(handler, root, jsonRequest(t, http.MethodPost, "/", body))
}

// assertStatus fails unless the response has the wanted status code.
func assertStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, want, response.Body.String())
	}
}

// decodeResponse decodes the JSON response body into T.
func decodeResponse[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var decoded T
	body, _ := io.ReadAll(response.Body)
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return decoded
}
