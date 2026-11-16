package wsdashboard

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

// The page streams over WebSocket. A browser keeps at most six HTTP/1.1
// connections per host and the daemon serves HTTP/1.1 only, so open SSE
// streams (one per tab, one more for an open support chat) use up the
// connections every page GET, tracker read, and send shares. A browser
// counts WebSocket connections apart from that pool, so the page's streams
// upgrade; SSE stays on the same routes for a page script older than the
// daemon.
//
// Framing: each event is one text frame {"event": name, "data": json}, with
// the SSE stream's event name and the same data bytes. The idle frame is
// {"event":"ping"}, which the page answers with any frame (it sends
// {"event":"pong"}); opted-in streams may also report visibility.
//
// A hijacked connection's request context does not end when the peer leaves,
// so the connection runs its own read loop: a close frame, a read error, or
// no frame from the page within wsReadWindow keepAlive periods ends the
// stream. x/net/websocket answers protocol pings inside Read and surfaces no
// pong, hence the ping at the application level.

const (
	// wsReadWindow is the number of keep-alive periods the page has to send
	// a frame before the stream drops it: one ping is due within every
	// period, so a live page answers well inside the window.
	wsReadWindow = 3
	// wsWriteTimeout bounds one frame write, so a peer that stopped reading
	// cannot hold the stream forever.
	wsWriteTimeout = 10 * time.Second
	// wsMaxFrame bounds a frame the page sends.
	wsMaxFrame = 1 << 10
)

// wsUpgrade reports whether r is a WebSocket handshake.
func wsUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// wsOriginAllowed admits a handshake that carries the daemon's own Origin or
// none. CORS does not cover a WebSocket handshake, so without this any page
// open in the browser could read the streams from loopback. Browsers always
// send an Origin, so a handshake without one is not a browser's.
func wsOriginAllowed(r *http.Request) bool {
	return r.Header.Get("Origin") == "" || sameOrigin(r)
}

// serveWebSocket upgrades r and runs run over the connection until it
// returns, then closes the connection and waits for its read loop to end.
// The caller has checked the Origin and that w can be hijacked.
func (d *Daemon) serveWebSocket(w http.ResponseWriter, r *http.Request, run func(streamSink), watches ...func(bool) func()) {
	window := wsReadWindow * d.keepAlive
	websocket.Server{Handler: func(ws *websocket.Conn) {
		ws.MaxPayloadBytes = wsMaxFrame
		sink := &wsSink{ws: ws, done: make(chan struct{})}
		if len(watches) > 0 {
			sink.visibility = &displayWatch{acquire: watches[0]}
			sink.visibility.set(r.URL.Query().Get("display") == "1")
			defer sink.visibility.close()
		}
		go sink.read(window)
		run(sink)
		if sink.visibility != nil {
			sink.visibility.close()
		}
		_ = ws.Close()
		<-sink.done
	}}.ServeHTTP(w, r)
}

// wsSink frames events as WebSocket text frames (see the file comment).
type wsSink struct {
	ws         *websocket.Conn
	done       chan struct{}
	visibility *displayWatch
}

// displayWatch serializes visibility transitions with disconnect cleanup.
type displayWatch struct {
	mu      sync.Mutex
	acquire func(bool) func()
	release func()
	closed  bool
}

func (w *displayWatch) set(visible bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	if visible && w.release == nil && w.acquire != nil {
		w.release = w.acquire(true)
	} else if !visible && w.release != nil {
		w.release()
		w.release = nil
	}
}

func (w *displayWatch) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	if w.release != nil {
		w.release()
		w.release = nil
	}
}

// read consumes the page's frames until the connection fails, closes, or
// goes quiet for window, then closes done.
func (s *wsSink) read(window time.Duration) {
	defer close(s.done)
	if s.visibility != nil {
		defer s.visibility.close()
	}
	for {
		_ = s.ws.SetReadDeadline(time.Now().Add(window))
		var frame string
		if err := websocket.Message.Receive(s.ws, &frame); err != nil {
			return
		}
		if s.visibility != nil {
			var message struct {
				Event   string `json:"event"`
				Visible *bool  `json:"visible"`
			}
			if json.Unmarshal([]byte(frame), &message) == nil && message.Event == "visibility" && message.Visible != nil {
				s.visibility.set(*message.Visible)
			}
		}
	}
}

func (s *wsSink) event(name string, data []byte) error {
	frame := make([]byte, 0, len(data)+len(name)+20)
	frame = append(frame, `{"event":"`...)
	frame = append(frame, name...)
	frame = append(frame, `","data":`...)
	frame = append(frame, data...)
	frame = append(frame, '}')
	return s.write(string(frame))
}

func (s *wsSink) write(frame string) error {
	_ = s.ws.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	return websocket.Message.Send(s.ws, frame)
}

// flush has nothing to push: every event is its own frame.
func (s *wsSink) flush() error { return nil }

func (s *wsSink) keepAlive() error { return s.write(`{"event":"ping"}`) }

func (s *wsSink) gone() <-chan struct{} { return s.done }
