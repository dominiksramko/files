package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

// Files that a browser would run as a page if opened directly, and ordinary ones.
var servedFiles = map[string]string{
	"page.html":   `<script>alert(1)</script>`,
	"drawing.svg": `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`,
	"data.xml":    `<x:script xmlns:x="http://www.w3.org/1999/xhtml">alert(1)</x:script>`,
	"readme":      `<!DOCTYPE html><script>alert(1)</script>`, // no extension: the type is guessed from the content
	"notes.txt":   "plain text",
	"photo.jpg":   "\xff\xd8\xff\xe0 not really a jpeg",
	"doc.pdf":     "%PDF-1.4 not really a pdf",
}

// assertUserFileHeaders checks the response can't run as part of the app:
// sandboxed (except PDFs) and never type-guessed.
func assertUserFileHeaders(t *testing.T, name string, response *httptest.ResponseRecorder) {
	t.Helper()
	assertStatus(t, response, http.StatusOK)
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("%s: X-Content-Type-Options = %q, want nosniff", name, got)
	}
	wantPolicy := "sandbox"
	if name == "doc.pdf" {
		wantPolicy = ""
	}
	if got := response.Header().Get("Content-Security-Policy"); got != wantPolicy {
		t.Errorf("%s: Content-Security-Policy = %q, want %q", name, got, wantPolicy)
	}
}

func TestStreamSandboxesFiles(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, servedFiles)
	handler := NewFilesHandler(nil, "").Stream
	for name := range servedFiles {
		request := httptest.NewRequest(http.MethodGet, "/?path="+url.QueryEscape(name), nil)
		assertUserFileHeaders(t, name, serve(handler, annRoot, request))
	}
}

func TestReadContentSandboxesFiles(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, servedFiles)
	handler := NewFilesHandler(nil, "").ReadContent
	for name := range servedFiles {
		assertUserFileHeaders(t, name, serve(handler, annRoot, readContentRequest(name)))
	}
}

func TestOfficeLinkSandboxesFiles(t *testing.T) {
	// The office link works without logging in, so anyone could be sent one.
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, servedFiles)
	handler := NewOfficeHandler("test-secret")
	for name := range servedFiles {
		token, err := handler.makeToken(name, annRoot)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/public/office/"+token, nil)
		ctx.Params = gin.Params{{Key: "token", Value: token}}
		handler.ServeFile(ctx)
		assertUserFileHeaders(t, name, recorder)
	}
}
