package wsdashboard

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSSESinkDropsAStalledClient pins the SSE write deadline: a client that
// sent its request and then stopped reading fills the socket buffers, and the
// sink's blocked write fails at the deadline instead of holding the stream's
// goroutine forever.
func TestSSESinkDropsAStalledClient(t *testing.T) {
	const timeout = 300 * time.Millisecond
	failed := make(chan time.Duration, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sink := &sseSink{w: w, rc: http.NewResponseController(w), flusher: w.(http.Flusher), timeout: timeout, done: r.Context().Done()}
		w.WriteHeader(http.StatusOK)
		payload := bytes.Repeat([]byte("x"), 64<<10)
		for {
			start := time.Now()
			if sink.event("region", payload) != nil || sink.flush() != nil {
				failed <- time.Since(start)
				return
			}
		}
	}))
	defer srv.Close()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: stalled\r\n\r\n")
	// The client never reads.
	select {
	case blocked := <-failed:
		if blocked > timeout+5*time.Second {
			t.Fatalf("the stalled write failed after %v, want about %v", blocked, timeout)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the stalled client was not dropped")
	}
}
