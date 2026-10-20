package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEventStreamFraming(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	startEventStream(ctx)
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q", got)
	}
	if !recorder.Flushed {
		t.Error("headers weren't sent before the first event")
	}

	if err := writeEvent(ctx, "progress", gin.H{"files": 2}); err != nil {
		t.Fatal(err)
	}
	if err := writeEvent(ctx, "", gin.H{"done": true}); err != nil {
		t.Fatal(err)
	}
	want := "event: progress\ndata: {\"files\":2}\n\ndata: {\"done\":true}\n\n"
	if recorder.Body.String() != want {
		t.Errorf("body = %q, want %q", recorder.Body.String(), want)
	}
}

// goneWriter fails every write, like a connection the client has closed.
type goneWriter struct{ httptest.ResponseRecorder }

func (w *goneWriter) Write([]byte) (int, error) { return 0, errors.New("connection closed") }

func TestWriteEventReportsAClientThatLeft(t *testing.T) {
	writer := &goneWriter{ResponseRecorder: *httptest.NewRecorder()}
	ctx, _ := gin.CreateTestContext(writer)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if writeEvent(ctx, "ping", struct{}{}) == nil {
		t.Error("a failed write wasn't reported")
	}
	if writeEvent(ctx, "", struct{}{}) == nil {
		t.Error("a failed unnamed write wasn't reported")
	}
}
