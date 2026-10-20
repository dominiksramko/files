package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiskSpaceOfTheUsersFolder(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	response := serve(NewSystemHandler("test").DiskSpace, annRoot, httptest.NewRequest(http.MethodGet, "/", nil))
	assertStatus(t, response, http.StatusOK)
	disk := decodeResponse[struct {
		Total, Free, Used uint64
		Pct               float64
	}](t, response)
	if disk.Total == 0 || disk.Free > disk.Total || disk.Used != disk.Total-disk.Free {
		t.Errorf("disk = %+v", disk)
	}

	// A user whose folder is missing gets an answer, not a server error.
	response = serve(NewSystemHandler("test").DiskSpace, annRoot+"/missing", httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code == http.StatusOK || response.Code == http.StatusInternalServerError {
		t.Errorf("missing folder: status %d", response.Code)
	}
}
