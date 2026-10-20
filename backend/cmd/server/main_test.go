package main

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
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
