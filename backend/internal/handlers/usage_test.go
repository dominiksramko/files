package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"files/internal/diskusage"

	"github.com/gin-gonic/gin"
)

// usageFolders is newUserFolders with a media folder in ann's home whose
// sizes stand out.
func usageFolders(t *testing.T) (annRoot, annaRoot string) {
	t.Helper()
	annRoot, annaRoot = newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{
		"media/big.bin":       strings.Repeat("x", 20000),
		"media/small.txt":     "small",
		"media/clips/one.bin": strings.Repeat("y", 3000),
	})
	return annRoot, annaRoot
}

func usageGet(handler gin.HandlerFunc, root string, query url.Values) *httptest.ResponseRecorder {
	return serve(handler, root, httptest.NewRequest(http.MethodGet, "/?"+query.Encode(), nil))
}

// scanUsage scans path in ann's home and fails unless it finished.
func scanUsage(t *testing.T, h *FilesHandler, root, path string) {
	t.Helper()
	response := usageGet(h.UsageScan, root, url.Values{"path": {path}})
	if !strings.Contains(response.Body.String(), "event: done") {
		t.Fatalf("scan of %q didn't finish: %s", path, response.Body.String())
	}
}

func entryNames(entries []UsageEntry) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name
	}
	return names
}

func TestUsageNeedsAScanFirst(t *testing.T) {
	annRoot, _ := usageFolders(t)
	h := NewFilesHandler(nil, "")
	for _, request := range []*httptest.ResponseRecorder{
		usageGet(h.Usage, annRoot, url.Values{"path": {""}}),
		usageGet(h.UsageLargest, annRoot, url.Values{"path": {""}}),
		serveJSON(t, h.UsageDelete, annRoot, map[string]any{"path": "", "paths": []string{"notes.txt"}}),
	} {
		assertStatus(t, request, http.StatusNotFound)
		if body := decodeResponse[map[string]any](t, request); body["scan_needed"] != true {
			t.Errorf("body = %v", body)
		}
	}
	assertFileContent(t, filepath.Join(annRoot, "notes.txt"), "ann's notes")
}

func TestUsageScanThenBrowse(t *testing.T) {
	annRoot, _ := usageFolders(t)
	h := NewFilesHandler(nil, "")
	scanUsage(t, h, annRoot, "")

	response := usageGet(h.Usage, annRoot, url.Values{"path": {""}})
	assertStatus(t, response, http.StatusOK)
	listing := decodeResponse[usageListing](t, response)
	if names := entryNames(listing.Entries); names[0] != "media" || len(names) != 3 {
		t.Fatalf("entries = %v, want media first", names)
	}
	media := listing.Entries[0]
	if !media.Scanned || media.Apparent != 23005 || media.Files != 3 || media.Dirs != 1 || media.Disk < media.Apparent {
		t.Errorf("media = %+v", media)
	}
	if listing.Total.Apparent != 23005+11+6+10 || listing.RootTotal != listing.Total || listing.At != "." {
		t.Errorf("listing totals %+v / %+v, at %q", listing.Total, listing.RootTotal, listing.At)
	}

	response = usageGet(h.Usage, annRoot, url.Values{"path": {""}, "at": {"media"}})
	assertStatus(t, response, http.StatusOK)
	listing = decodeResponse[usageListing](t, response)
	if names := entryNames(listing.Entries); strings.Join(names, ",") != "big.bin,clips,small.txt" {
		t.Errorf("media entries = %v", names)
	}
	if file := listing.Entries[0]; file.Apparent != 20000 || file.Path != "media/big.bin" || file.Scanned {
		t.Errorf("big.bin = %+v", file)
	}
	if listing.Total.Apparent != 23005 || listing.At != "media" {
		t.Errorf("media total %+v at %q", listing.Total, listing.At)
	}

	// A folder made after the scan is listed with only its own size.
	writeFiles(t, annRoot, map[string]string{"media/new/later.bin": strings.Repeat("z", 5000)})
	listing = decodeResponse[usageListing](t, usageGet(h.Usage, annRoot, url.Values{"path": {""}, "at": {"media"}}))
	for _, entry := range listing.Entries {
		if entry.Name == "new" && (entry.Scanned || entry.Apparent != 0) {
			t.Errorf("new folder = %+v", entry)
		}
	}

	// Another folder of the same user isn't scanned.
	assertStatus(t, usageGet(h.Usage, annRoot, url.Values{"path": {"docs"}}), http.StatusNotFound)
}

func TestUsageListsLinksAsThemselves(t *testing.T) {
	annRoot, _ := usageFolders(t)
	if err := os.Symlink("media", filepath.Join(annRoot, "to-media")); err != nil {
		t.Fatal(err)
	}
	h := NewFilesHandler(nil, "")
	scanUsage(t, h, annRoot, "")
	listing := decodeResponse[usageListing](t, usageGet(h.Usage, annRoot, url.Values{"path": {""}}))
	for _, entry := range listing.Entries {
		if entry.Name == "to-media" && (!entry.Link || entry.Scanned || entry.Apparent != int64(len("media"))) {
			t.Errorf("link = %+v", entry)
		}
	}
}

func TestUsageMarksHardLinks(t *testing.T) {
	annRoot, _ := usageFolders(t)
	if err := os.Link(filepath.Join(annRoot, "media", "big.bin"), filepath.Join(annRoot, "media", "again.bin")); err != nil {
		t.Skip("no hard links here:", err)
	}
	h := NewFilesHandler(nil, "")
	scanUsage(t, h, annRoot, "")
	listing := decodeResponse[usageListing](t, usageGet(h.Usage, annRoot, url.Values{"path": {""}, "at": {"media"}}))
	links := map[string]int{}
	for _, entry := range listing.Entries {
		links[entry.Name] = entry.HardLinks
	}
	if links["big.bin"] != 2 || links["again.bin"] != 2 || links["small.txt"] != 0 || links["clips"] != 0 {
		t.Errorf("hard links = %v", links)
	}
}

func TestUsageRefusesOutsideTheScan(t *testing.T) {
	annRoot, annaRoot := usageFolders(t)
	h := NewFilesHandler(nil, "")
	scanUsage(t, h, annRoot, "media")

	for _, at := range []string{".", "docs", "..", "../anna", "media/../../anna", "/etc"} {
		response := usageGet(h.Usage, annRoot, url.Values{"path": {"media"}, "at": {at}})
		if response.Code != http.StatusBadRequest {
			t.Errorf("at %q: status %d", at, response.Code)
		}
	}
	for _, path := range []string{"notes.txt", "media", "media/..", "../anna/secret.txt", "", "/"} {
		response := serveJSON(t, h.UsageDelete, annRoot, map[string]any{"path": "media", "paths": []string{"media/small.txt", path}})
		assertStatus(t, response, http.StatusBadRequest)
	}
	assertUnchanged(t, annRoot, annaRoot)
	assertFileContent(t, filepath.Join(annRoot, "media", "small.txt"), "small")
	for _, path := range []string{"..", "../anna"} {
		assertStatus(t, usageGet(h.UsageScan, annRoot, url.Values{"path": {path}}), http.StatusBadRequest)
	}
}

func TestUsageScansAreTheUsersOwn(t *testing.T) {
	annRoot, annaRoot := usageFolders(t)
	h := NewFilesHandler(nil, "")
	scanUsage(t, h, annRoot, "")
	query := httptest.NewRequest(http.MethodGet, "/?path=", nil)

	otherUser := serveWith(h.Usage, annRoot, query, func(ctx *gin.Context) { ctx.Set("userID", int64(2)) })
	assertStatus(t, otherUser, http.StatusNotFound)
	// The same user with a different home folder (changed by an admin).
	movedHome := serveWith(h.Usage, annaRoot, httptest.NewRequest(http.MethodGet, "/?path=", nil), nil)
	assertStatus(t, movedHome, http.StatusNotFound)
}

func TestUsageScanRefusesWhatIsntAFolder(t *testing.T) {
	annRoot, _ := usageFolders(t)
	h := NewFilesHandler(nil, "")
	for path, message := range map[string]string{"notes.txt": "not a folder", "missing": "folder not found"} {
		body := usageGet(h.UsageScan, annRoot, url.Values{"path": {path}}).Body.String()
		if !strings.Contains(body, "event: failed") || !strings.Contains(body, message) || strings.Contains(body, "event: done") {
			t.Errorf("%s: %s", path, body)
		}
	}
}

func TestUsageDelete(t *testing.T) {
	annRoot, annaRoot := usageFolders(t)
	h := NewFilesHandler(nil, "")
	scanUsage(t, h, annRoot, "")

	response := serveJSON(t, h.UsageDelete, annRoot, map[string]any{"path": "", "paths": []string{"media/big.bin", "media/clips", "media/clips/one.bin"}})
	assertStatus(t, response, http.StatusOK)
	result := decodeResponse[struct {
		Freed     diskusage.Totals `json:"freed"`
		RootTotal diskusage.Totals `json:"root_total"`
	}](t, response)
	if result.Freed.Apparent != 23000 || result.Freed.Files != 2 || result.Freed.Dirs != 1 || result.Freed.Disk <= 0 {
		t.Errorf("freed %+v", result.Freed)
	}
	if result.RootTotal.Apparent != 5+11+6+10 {
		t.Errorf("root total %+v", result.RootTotal)
	}
	assertMissing(t, filepath.Join(annRoot, "media", "big.bin"))
	assertMissing(t, filepath.Join(annRoot, "media", "clips"))
	assertFileContent(t, filepath.Join(annaRoot, "secret.txt"), "anna's secret")

	listing := decodeResponse[usageListing](t, usageGet(h.Usage, annRoot, url.Values{"path": {""}}))
	for _, entry := range listing.Entries {
		if entry.Name == "media" && (entry.Apparent != 5 || entry.Dirs != 0) {
			t.Errorf("media after delete = %+v", entry.Totals)
		}
	}

	// What's gone already is reported and nothing is deleted.
	response = serveJSON(t, h.UsageDelete, annRoot, map[string]any{"path": "", "paths": []string{"media/small.txt", "media/big.bin"}})
	assertStatus(t, response, http.StatusNotFound)
	assertFileContent(t, filepath.Join(annRoot, "media", "small.txt"), "small")
}

func TestUsageLargest(t *testing.T) {
	annRoot, _ := usageFolders(t)
	h := NewFilesHandler(nil, "")
	scanUsage(t, h, annRoot, "")
	if err := os.Remove(filepath.Join(annRoot, "media", "clips", "one.bin")); err != nil {
		t.Fatal(err)
	}

	response := usageGet(h.UsageLargest, annRoot, url.Values{"path": {""}, "size": {"apparent"}})
	assertStatus(t, response, http.StatusOK)
	entries := decodeResponse[[]UsageEntry](t, response)
	var paths []string
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	if strings.Join(paths, ",") != "media/big.bin,notes.txt,docs/todo.todo,docs/plan.md,media/small.txt" {
		t.Errorf("largest = %v", paths)
	}
	if entries[0].Apparent != 20000 || entries[0].Name != "big.bin" || entries[0].Disk <= 0 {
		t.Errorf("first = %+v", entries[0])
	}
}

func TestUsageScanStopsWhenTheSessionEnds(t *testing.T) {
	annRoot, _ := usageFolders(t)
	h := NewFilesHandler(nil, "")
	ended := func() bool { return false }
	response := serveWith(h.UsageScan, annRoot, httptest.NewRequest(http.MethodGet, "/?path=", nil), func(ctx *gin.Context) {
		ctx.Set("sessionStillValid", ended)
	})
	if strings.Contains(response.Body.String(), "event: done") {
		t.Errorf("scan finished for an ended session: %s", response.Body.String())
	}
	assertStatus(t, usageGet(h.Usage, annRoot, url.Values{"path": {""}}), http.StatusNotFound)
}

func TestUsageScanStopsOnShutdown(t *testing.T) {
	annRoot, _ := usageFolders(t)
	h := NewFilesHandler(nil, "")
	h.EndStreams()
	if body := usageGet(h.UsageScan, annRoot, url.Values{"path": {""}}).Body.String(); strings.Contains(body, "event: done") {
		t.Errorf("scan ran during shutdown: %s", body)
	}
	assertStatus(t, usageGet(h.Usage, annRoot, url.Values{"path": {""}}), http.StatusNotFound)
}

func TestUsageScanWaitsForAFreeSlot(t *testing.T) {
	annRoot, _ := usageFolders(t)
	h := NewFilesHandler(nil, "")
	for range usageScansAtOnce {
		h.usage.slots <- struct{}{}
	}
	gone, cancel := context.WithCancel(context.Background())
	cancel() // the client gave up waiting
	body := serve(h.UsageScan, annRoot, httptest.NewRequest(http.MethodGet, "/?path=", nil).WithContext(gone)).Body.String()
	if !strings.Contains(body, "event: queued") || strings.Contains(body, "event: done") {
		t.Errorf("body = %s", body)
	}

	<-h.usage.slots // one is free again
	scanUsage(t, h, annRoot, "")
	if len(h.usage.slots) != usageScansAtOnce-1 {
		t.Errorf("a finished scan kept its slot: %d taken", len(h.usage.slots))
	}
}

func TestStartingAScanStopsTheUsersRunningOne(t *testing.T) {
	store := newUsageStore()
	first, cancelFirst := context.WithCancel(context.Background())
	finishFirst := store.start(1, cancelFirst)
	second, cancelSecond := context.WithCancel(context.Background())
	finishSecond := store.start(1, cancelSecond)
	other, cancelOther := context.WithCancel(context.Background())
	defer store.start(2, cancelOther)()

	if first.Err() == nil {
		t.Error("the user's earlier scan kept running")
	}
	if second.Err() != nil || other.Err() != nil {
		t.Error("a scan was stopped by another")
	}
	finishFirst() // ends after the second started: mustn't forget the second
	store.start(2, func() {})
	if store.running[1] == nil {
		t.Error("the finished first scan removed the running second one")
	}
	finishSecond()
	if store.running[1] != nil {
		t.Error("a finished scan is still running")
	}
}

func TestUsageScansExpire(t *testing.T) {
	annRoot, _ := usageFolders(t)
	h := NewFilesHandler(nil, "")
	scanUsage(t, h, annRoot, "")
	h.usage.scans[1].lastUsed = time.Now().Add(-usageScanKept - time.Minute)
	assertStatus(t, usageGet(h.Usage, annRoot, url.Values{"path": {""}}), http.StatusNotFound)
	if len(h.usage.scans) != 0 {
		t.Error("an expired scan is still kept")
	}
}
