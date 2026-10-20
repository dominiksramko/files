package main

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"files/internal/config"

	"github.com/gin-gonic/gin"
)

// startServer serves handler with newHTTPServer's settings, but a short
// header timeout so the test doesn't wait 10 s; returns the address.
func startServer(t *testing.T, handler http.Handler, headerTimeout time.Duration) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := newHTTPServer("", handler)
	server.ReadHeaderTimeout = headerTimeout
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	return listener.Addr().String()
}

func TestNewHTTPServerTimeouts(t *testing.T) {
	server := newHTTPServer(":8080", http.NotFoundHandler())
	if server.ReadHeaderTimeout != 10*time.Second || server.IdleTimeout != 2*time.Minute {
		t.Errorf("ReadHeaderTimeout = %v, IdleTimeout = %v", server.ReadHeaderTimeout, server.IdleTimeout)
	}
	if server.ReadTimeout != 0 || server.WriteTimeout != 0 {
		t.Errorf("ReadTimeout = %v, WriteTimeout = %v; whole requests must not be time-limited", server.ReadTimeout, server.WriteTimeout)
	}
}

func TestSlowHeadersAreCutOff(t *testing.T) {
	address := startServer(t, http.NotFoundHandler(), 200*time.Millisecond)
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n")) // headers never finish

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadAll(conn); err != nil {
		t.Fatalf("connection still open after the header timeout: %v", err)
	}
}

func TestSlowBodyIsNotCutOff(t *testing.T) {
	// An upload whose body takes longer than the header timeout still arrives.
	received := make(chan string, 1)
	address := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
	}), 200*time.Millisecond)
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte("POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 6\r\n\r\n"))
	for _, part := range []string{"sl", "ow", "ly"} {
		time.Sleep(150 * time.Millisecond)
		conn.Write([]byte(part))
	}
	select {
	case body := <-received:
		if body != "slowly" {
			t.Errorf("body = %q", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("slow body never arrived")
	}
	status, _ := bufio.NewReader(conn).ReadString('\n')
	if !strings.Contains(status, "200") {
		t.Errorf("status line = %q", status)
	}
}

// clientIP returns the client address a router trusting proxies sees for a
// request arriving from remoteAddr with the given X-Forwarded-For.
func clientIP(t *testing.T, proxies []string, remoteAddr, forwardedFor string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := router.SetTrustedProxies(proxies); err != nil {
		t.Fatal(err)
	}
	router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = remoteAddr
	request.Header.Set("X-Forwarded-For", forwardedFor)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder.Body.String()
}

func TestClientAddressBehindProxy(t *testing.T) {
	tests := []struct {
		name, remoteAddr, want string
	}{
		{"proxy in another container", "172.18.0.5:41000", "198.51.100.7"},
		{"proxy on the host", "127.0.0.1:41000", "198.51.100.7"},
		{"proxy on the LAN", "192.168.1.10:41000", "198.51.100.7"},
		{"client on the internet claiming another address", "203.0.113.9:41000", "203.0.113.9"},
	}
	for _, test := range tests {
		if got := clientIP(t, config.DefaultTrustedProxies, test.remoteAddr, "198.51.100.7"); got != test.want {
			t.Errorf("%s: client = %s, want %s", test.name, got, test.want)
		}
	}
	if got := clientIP(t, nil, "172.18.0.5:41000", "198.51.100.7"); got != "172.18.0.5" {
		t.Errorf("TRUSTED_PROXIES=none: client = %s, want the direct address", got)
	}
}
