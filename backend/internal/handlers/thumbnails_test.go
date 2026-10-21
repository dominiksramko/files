package handlers

import (
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"files/internal/testutil"
	"files/internal/thumbs"
)

// writePNG writes a blank width×height PNG to path.
func writePNG(t *testing.T, path string, width, height int) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, image.NewNRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
}

// makeThumbnails has the service make the thumbnails of the image at path
// and waits for them; it returns the image's content hash.
func makeThumbnails(t *testing.T, service *thumbs.Service, path string) string {
	t.Helper()
	folder := filepath.Dir(path)
	ready, unsubscribe := service.Hub().Subscribe(folder)
	defer unsubscribe()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	service.CheckAndEnqueue(path, filepath.Base(path), folder, "image/png", info.ModTime().Unix())
	select {
	case event := <-ready:
		return event.Hash
	case <-time.After(10 * time.Second):
		t.Fatal("thumbnails not made")
		return ""
	}
}

func requestThumbnail(endpoint gin.HandlerFunc, root, path, hash string) *httptest.ResponseRecorder {
	query := url.Values{"path": {path}, "hash": {hash}}
	return serve(endpoint, root, httptest.NewRequest(http.MethodGet, "/?"+query.Encode(), nil))
}

func TestThumbnailEndpoints(t *testing.T) {
	annRoot, annaRoot := newUserFolders(t)
	writePNG(t, filepath.Join(annRoot, "photo.png"), 1600, 900)
	writePNG(t, filepath.Join(annaRoot, "photo.png"), 1200, 900)
	service, err := thumbs.New(testutil.OpenDB(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	annHash := makeThumbnails(t, service, filepath.Join(annRoot, "photo.png"))
	annaHash := makeThumbnails(t, service, filepath.Join(annaRoot, "photo.png"))
	handler := NewThumbnailHandler(service)

	for _, test := range []struct {
		name     string
		endpoint gin.HandlerFunc
		want     image.Point
	}{
		{"square", handler.Get, image.Pt(thumbs.ThumbMaxDim, thumbs.ThumbMaxDim)},
		{"preview keeps the picture's shape", handler.Preview, image.Pt(640, 360)},
	} {
		response := requestThumbnail(test.endpoint, annRoot, "photo.png", annHash)
		assertStatus(t, response, http.StatusOK)
		img, err := jpeg.Decode(response.Body)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if got := img.Bounds().Size(); got != test.want {
			t.Errorf("%s: %v, want %v", test.name, got, test.want)
		}
	}

	assertStatus(t, requestThumbnail(handler.Preview, annRoot, "photo.png", annaHash), http.StatusNotFound)
	assertStatus(t, requestThumbnail(handler.Preview, annRoot, "photo.png", ""), http.StatusBadRequest)
	assertStatus(t, requestThumbnail(handler.Preview, annRoot, "notes.txt", annHash), http.StatusNotFound)

	// Another user's picture, by path or through a link.
	if response := requestThumbnail(handler.Preview, annRoot, "../anna/photo.png", annaHash); response.Code == http.StatusOK {
		t.Error("preview of another user's file served by path")
	}
	if err := os.Symlink(filepath.Join(annaRoot, "photo.png"), filepath.Join(annRoot, "link.png")); err != nil {
		t.Fatal(err)
	}
	if response := requestThumbnail(handler.Preview, annRoot, "link.png", annaHash); response.Code == http.StatusOK {
		t.Error("preview of another user's file served through a link")
	}
}

func TestThumbnailsDisabled(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	handler := NewThumbnailHandler(nil)
	response := requestThumbnail(handler.Preview, annRoot, "notes.txt", "hash")
	assertStatus(t, response, http.StatusNotFound)
}
