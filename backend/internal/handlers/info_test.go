package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSizeStreamCountsAndStopsWhenClientLeaves(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	files := map[string]string{}
	for i := 0; i < 500; i++ {
		files[fmt.Sprintf("big/%03d.txt", i)] = "x"
	}
	writeFiles(t, annRoot, files)

	response := serve(NewInfoHandler().SizeStream, annRoot, httptest.NewRequest(http.MethodGet, "/?paths[]=big", nil))
	last := lastLine(response.Body.String())
	if !strings.Contains(last, `"files":500,"dirs":0,"bytes":500,"disk":`) || !strings.Contains(last, `"done":true`) || strings.Contains(last, `"disk":0,`) {
		t.Errorf("full scan: %s", last)
	}

	gone, cancel := context.WithCancel(context.Background())
	cancel() // the client already left
	request := httptest.NewRequest(http.MethodGet, "/?paths[]=big", nil).WithContext(gone)
	response = serve(NewInfoHandler().SizeStream, annRoot, request)
	if strings.Contains(response.Body.String(), `"files":500`) || strings.Contains(response.Body.String(), "done") {
		t.Errorf("scan went on after the client left: %s", lastLine(response.Body.String()))
	}
}

func lastLine(body string) string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	return lines[len(lines)-1]
}
