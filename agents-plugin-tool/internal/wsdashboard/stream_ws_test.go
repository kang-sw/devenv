package wsdashboard

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

// wsClient is a test WebSocket stream: its events in the SSE reader's shape,
// and the connection, which a test may close.
type wsClient struct {
	events <-chan sseEvent
	conn   *websocket.Conn
}

// openWS opens a WebSocket stream on base+path sending origin. A client
// that answers pings with a pong, as the page does; a silent one never
// writes. Pings are not passed on as events. The channel closes when the
// connection ends.
func openWS(t *testing.T, base, path, origin string, answer bool) wsClient {
	t.Helper()
	cfg, err := websocket.NewConfig("ws"+strings.TrimPrefix(base, "http")+path, origin)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := websocket.DialConfig(cfg)
	if err != nil {
		t.Fatalf("websocket %s: %v", path, err)
	}
	t.Cleanup(func() { conn.Close() })
	events := make(chan sseEvent, 64)
	go func() {
		defer close(events)
		for {
			var frame string
			if err := websocket.Message.Receive(conn, &frame); err != nil {
				return
			}
			var m struct {
				Event string          `json:"event"`
				Data  json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal([]byte(frame), &m); err != nil {
				events <- sseEvent{name: "malformed", data: frame}
				continue
			}
			if m.Event == "ping" {
				if answer {
					_ = websocket.Message.Send(conn, `{"event":"pong"}`)
				}
				continue
			}
			events <- sseEvent{name: m.Event, data: string(m.Data)}
		}
	}()
	return wsClient{events: events, conn: conn}
}

// openSSE opens a plain stream request and checks that it is served SSE.
func openSSE(t *testing.T, url string) <-chan sseEvent {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream %s = %d %q, want SSE", url, resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	events := make(chan sseEvent, 64)
	go readEvents(t, bufio.NewReader(resp.Body), events)
	return events
}

// collect reads events until none arrives for settle or the stream ends,
// and reports which.
func collect(events <-chan sseEvent) (got []sseEvent, closed bool) {
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return got, true
			}
			got = append(got, ev)
		case <-time.After(settle):
			return got, false
		}
	}
}

// handshake sends a raw WebSocket handshake for path to the daemon at base,
// addressed to host, with the extra headers, and returns the status line's
// code: 101 when it upgraded.
func handshake(t *testing.T, base, path, host string, headers map[string]string) int {
	t.Helper()
	conn, err := net.Dial("tcp", strings.TrimPrefix(base, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	req := "GET " + path + " HTTP/1.1\r\nHost: " + host + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n"
	for k, v := range headers {
		req += k + ": " + v + "\r\n"
	}
	if _, err := conn.Write([]byte(req + "\r\n")); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("handshake %s: %v", path, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// TestWebSocketStreamsSendWhatSSESends pins the WebSocket framing against
// the SSE stream: for the same page state, a WebSocket stream sends the same
// events with the same data, on connect (the assets identity first, the
// regions, the tracker revision sentinel), with ?h= (held regions skipped) and ?regions= (only those), on
// the sessions index, after a change, and when the session leaves the index
// ("gone", then the connection ends).
func TestWebSocketStreamsSendWhatSSESends(t *testing.T) {
	f := startPageFixture(t, withTracker(newFakeTracker()))
	origin := f.base
	_, page := get(t, f.base+"/s/livesession")
	paths := []string{
		"/s/livesession/events",
		"/s/livesession/events?regions=entries,convlive",
		"/s/livesession/events?h=" + heldOf(page) + ",mode." + newRegion(modeLive).Hash,
		"/events",
	}
	type pair struct {
		path string
		sse  <-chan sseEvent
		ws   <-chan sseEvent
	}
	var pairs []pair
	for _, path := range paths {
		pairs = append(pairs, pair{path, openSSE(t, f.base+path), openWS(t, f.base, path, origin, true).events})
	}
	compare := func(step string, p pair) []sseEvent {
		t.Helper()
		sse, sseClosed := collect(p.sse)
		ws, wsClosed := collect(p.ws)
		if !slices.Equal(sse, ws) || sseClosed != wsClosed {
			t.Fatalf("%s %s: SSE sent %v (closed %v), WebSocket %v (closed %v)", step, p.path, sse, sseClosed, ws, wsClosed)
		}
		return ws
	}
	names := func(evs []sseEvent) []string {
		var out []string
		for _, ev := range evs {
			if ev.name == "region" {
				var re regionEvent
				_ = json.Unmarshal([]byte(ev.data), &re)
				out = append(out, re.Name)
			} else {
				out = append(out, ev.name+":"+ev.data)
			}
		}
		return out
	}

	for i, p := range pairs {
		got := names(compare("connect", p))
		if len(got) == 0 || !strings.HasPrefix(got[0], "assets:") {
			t.Errorf("connect %s sent %v, want the assets identity first", p.path, got)
		} else {
			got = got[1:]
		}
		if !slices.Contains(got, `tracker:{"rev":0}`) {
			t.Errorf("connect %s sent no tracker sentinel: %v", p.path, got)
		}
		switch i {
		case 0:
			if !slices.Contains(got, regionMode) || !slices.Contains(got, regionEntries) {
				t.Errorf("connect %s sent %v, want every region", p.path, got)
			}
		case 1:
			if want := []string{regionEntries, regionConvLive, `tracker:{"rev":0}`}; !slices.Equal(got, want) {
				t.Errorf("connect %s sent %v, want %v", p.path, got, want)
			}
		case 2:
			if want := []string{`tracker:{"rev":0}`}; !slices.Equal(got, want) {
				t.Errorf("connect %s sent %v, want no held region", p.path, got)
			}
		}
	}

	// Separate steps: one wake each, so every stream sends the same batch.
	f.d.TrackerChanged()
	for _, p := range pairs {
		if got := names(compare("tracker change", p)); !slices.Equal(got, []string{`tracker:{"rev":1}`}) {
			t.Errorf("tracker change %s sent %v, want the new revision only", p.path, got)
		}
	}
	f.change(t, "livesession", f.project, "changed board text", 9)
	for i, p := range pairs {
		got := names(compare("change", p))
		if (i == 0 || i == 2) && !slices.Contains(got, regionBoard) {
			t.Errorf("change %s sent %v, want the board", p.path, got)
		}
	}

	if err := RemoveRegistration(f.dir, "livesession"); err != nil {
		t.Fatal(err)
	}
	f.d.Index().Refresh()
	for _, p := range pairs[:3] {
		got, closed := collect(p.ws)
		sse, _ := collect(p.sse)
		if len(got) == 0 || got[len(got)-1] != (sseEvent{name: "gone", data: "{}"}) || !closed || !slices.Equal(got, sse) {
			t.Errorf("gone %s: WebSocket sent %v (closed %v), SSE %v", p.path, got, closed, sse)
		}
	}
}

// TestWebSocketHandshakeChecksOriginAndSession pins the upgrade's checks: a
// foreign Origin is refused (CORS does not cover a handshake), the daemon's
// own Origin or none upgrades, a page gone before its first render answers
// 404 before the upgrade, and a plain request keeps getting SSE.
func TestWebSocketHandshakeChecksOriginAndSession(t *testing.T) {
	f := startPageFixture(t)
	host := strings.TrimPrefix(f.base, "http://")
	for _, c := range []struct {
		name    string
		path    string
		headers map[string]string
		want    int
	}{
		{"foreign Origin", "/s/livesession/events", map[string]string{"Origin": "http://evil.example"}, http.StatusForbidden},
		{"opaque Origin", "/events", map[string]string{"Origin": "null"}, http.StatusForbidden},
		{"another port's Origin", "/events", map[string]string{"Origin": "http://127.0.0.1:1"}, http.StatusForbidden},
		{"own Origin", "/s/livesession/events", map[string]string{"Origin": "http://" + host}, http.StatusSwitchingProtocols},
		{"no Origin", "/events", nil, http.StatusSwitchingProtocols},
		{"unknown session", "/s/nosuchsession/events", map[string]string{"Origin": "http://" + host}, http.StatusNotFound},
	} {
		if got := handshake(t, f.base, c.path, host, c.headers); got != c.want {
			t.Errorf("%s: handshake = %d, want %d", c.name, got, c.want)
		}
	}
	openSSE(t, f.base+"/events")
}

// TestWebSocketStreamNeedsTheLANToken pins the access guard on the upgrade:
// in LAN mode a non-local viewer's handshake without the token is refused as
// any request is, and the token cookie admits it, as a page's own WebSocket
// sends it.
func TestWebSocketStreamNeedsTheLANToken(t *testing.T) {
	f := startPageFixture(t, lanMode)
	token := readLANToken(t, f.dir)
	lanHost := "192.168.7.20:" + strconv.Itoa(f.d.Endpoint().Port)
	own := map[string]string{"Origin": "http://" + lanHost}
	if got := handshake(t, f.base, "/s/livesession/events", lanHost, own); got != http.StatusUnauthorized {
		t.Errorf("LAN handshake without the token = %d, want 401", got)
	}
	own["Cookie"] = lanTokenCookie + "=" + token
	if got := handshake(t, f.base, "/s/livesession/events", lanHost, own); got != http.StatusSwitchingProtocols {
		t.Errorf("LAN handshake with the token cookie = %d, want 101", got)
	}
}

// subscriptions counts the streams the broker holds.
func (b *broker) subscriptions() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

// TestWebSocketStreamDropsASilentPeer pins the heartbeat: a peer that stops
// answering the pings is dropped within the read window and its stream ends,
// one that answers stays connected past it, and a peer that closes ends its
// stream too.
func TestWebSocketStreamDropsASilentPeer(t *testing.T) {
	const keepAlive = 300 * time.Millisecond
	f := startPageFixture(t, func(o *DaemonOptions) { o.keepAlive = keepAlive })
	live := openWS(t, f.base, "/s/livesession/events", f.base, true)
	silent := openWS(t, f.base, "/s/livesession/events", f.base, false)
	go func() {
		for range live.events {
		}
	}()
	// A session stream holds two subscriptions: its page's and the shared
	// session rail's.
	const perStream = 2
	waitFor(t, "both streams to subscribe", func() bool { return f.d.broker.subscriptions() == 2*perStream })

	deadline := time.After(wsReadWindow*keepAlive + 3*time.Second)
	for open := true; open; {
		select {
		case _, open = <-silent.events:
		case <-deadline:
			t.Fatal("the silent peer was not dropped")
		}
	}
	waitFor(t, "the silent peer's stream to end", func() bool { return f.d.broker.subscriptions() == perStream })

	time.Sleep(2 * wsReadWindow * keepAlive)
	if n := f.d.broker.subscriptions(); n != perStream {
		t.Fatalf("the answering peer's subscription count = %d, want %d", n, perStream)
	}
	live.conn.Close()
	waitFor(t, "the closed peer's stream to end", func() bool { return f.d.broker.subscriptions() == 0 })
}
