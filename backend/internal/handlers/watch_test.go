package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestWatchEndsWhenSessionEnds(t *testing.T) {
	previous := watchPingInterval
	watchPingInterval = 50 * time.Millisecond
	t.Cleanup(func() { watchPingInterval = previous })

	annRoot, _ := newUserFolders(t)
	var live atomic.Bool
	live.Store(true)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?path=docs", nil)
	ctx.Set("rootDir", annRoot)
	ctx.Set("sessionStillValid", live.Load)

	done := make(chan struct{})
	go func() {
		NewFilesHandler(nil, "").Watch(ctx)
		close(done)
	}()

	time.Sleep(200 * time.Millisecond) // a few pings while the session is live
	select {
	case <-done:
		t.Fatal("stream ended while the session was live")
	default:
	}
	live.Store(false)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream still open after the session ended")
	}
	if !strings.Contains(recorder.Body.String(), "event: snapshot") || !strings.Contains(recorder.Body.String(), "event: ping") {
		t.Errorf("stream body:\n%s", recorder.Body)
	}
}

func TestWatchEndsOnShutdown(t *testing.T) {
	annRoot, _ := newUserFolders(t)
	handler := NewFilesHandler(nil, "")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?path=docs", nil)
	ctx.Set("rootDir", annRoot)

	done := make(chan struct{})
	go func() {
		handler.Watch(ctx)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	handler.EndStreams()
	handler.EndStreams() // safe to call twice
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watch stream still open after EndStreams")
	}
}
