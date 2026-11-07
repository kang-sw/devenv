package wsdashboard

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// lanMode runs a fixture daemon in LAN mode bound to loopback, so a test
// opens no port to the network; a request's peer address is set directly on
// the handler, or is loopback over a real connection.
func lanMode(o *DaemonOptions) {
	o.LAN = true
	o.bindHost = "127.0.0.1"
}

func readLANToken(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, LANTokenFileName))
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		t.Fatal("empty LAN token file")
	}
	return token
}

// viewer is how a request reaches the daemon: its peer address, its Host
// header, and the credential it carries.
type viewer struct {
	name   string
	peer   string
	host   string
	query  string // the token query parameter, when set
	cookie string // the token cookie, when set
}

// serve runs one request through the daemon's handler as v and returns the
// recorder. A stream route returns once its first render is sent, since the
// request's context is already done.
func serve(d *Daemon, v viewer, method, target, body string) *httptest.ResponseRecorder {
	if v.query != "" {
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target += sep + "token=" + v.query
	}
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.RemoteAddr = v.peer
	req.Host = v.host
	if method == http.MethodPost {
		req.Header.Set("Origin", "http://"+v.host)
		req.Header.Set("Content-Type", "application/json")
	}
	if v.cookie != "" {
		req.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: v.cookie})
	}
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

// refusedByGuard reports whether the access guard refused the request, as
// opposed to a route answering it.
func refusedByGuard(rec *httptest.ResponseRecorder) bool {
	return rec.Code == http.StatusUnauthorized && strings.Contains(rec.Body.String(), "LAN access token")
}

// guardedRoutes is every route the daemon serves, with a request that
// reaches it.
func guardedRoutes() []struct{ method, path, body string } {
	mermaid, _ := wswhiteboard.MermaidAsset()
	return []struct{ method, path, body string }{
		{http.MethodGet, "/health", ""},
		{http.MethodPost, "/shutdown", ""},
		{http.MethodPost, "/nudge", `{"reply_id":"livesession"}`},
		{http.MethodGet, "/", ""},
		{http.MethodGet, "/events", ""},
		{http.MethodGet, "/s/livesession", ""},
		{http.MethodGet, "/s/livesession/events", ""},
		{http.MethodGet, "/s/livesession/image?path=shot.png", ""},
		{http.MethodGet, "/s/livesession/attachment/0123456789abcdef-1.png", ""},
		{http.MethodPost, "/s/livesession/message", `{"text":"from the guard test"}`},
		{http.MethodPost, "/s/livesession/cancel", `{"id":"x"}`},
		{http.MethodGet, "/s/" + mermaid, ""},
	}
}

// TestLANModeGuardsEveryRoute pins the LAN access rule on every route: a
// loopback peer that addressed a loopback Host passes with no token; a
// loopback peer with a foreign Host, and a foreign peer with a loopback
// Host, need the token like any other request; the token passes by query
// parameter or cookie, and the endpoint file's shutdown token does not.
func TestLANModeGuardsEveryRoute(t *testing.T) {
	f := startPageFixture(t, lanMode)
	if err := os.WriteFile(filepath.Join(f.project, "shot.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	token := readLANToken(t, f.dir)
	if token == f.d.Endpoint().Token {
		t.Fatal("the LAN token is the endpoint token")
	}
	port := strconv.Itoa(f.d.Endpoint().Port)
	lanHost := "192.168.7.20:" + port
	pass := []viewer{
		{name: "loopback peer and Host", peer: "127.0.0.1:50000", host: "127.0.0.1:" + port},
		{name: "loopback peer and localhost Host", peer: "[::1]:50000", host: "localhost:" + port},
		{name: "foreign peer with the token query", peer: "192.168.7.30:50000", host: lanHost, query: token},
		{name: "foreign peer with the token cookie", peer: "192.168.7.30:50000", host: lanHost, cookie: token},
		{name: "loopback peer, foreign Host, token cookie", peer: "127.0.0.1:50000", host: lanHost, cookie: token},
	}
	refuse := []viewer{
		{name: "foreign peer, no token", peer: "192.168.7.30:50000", host: lanHost},
		{name: "loopback peer, foreign Host", peer: "127.0.0.1:50000", host: lanHost},
		{name: "loopback peer, rebinding Host", peer: "127.0.0.1:50000", host: "evil.example:" + port},
		{name: "foreign peer, loopback Host", peer: "192.168.7.30:50000", host: "127.0.0.1:" + port},
		{name: "foreign peer, wrong token", peer: "192.168.7.30:50000", host: lanHost, query: "wrong", cookie: "wrong"},
		{name: "foreign peer, endpoint token query", peer: "192.168.7.30:50000", host: lanHost, query: f.d.Endpoint().Token},
		{name: "foreign peer, endpoint token cookie", peer: "192.168.7.30:50000", host: lanHost, cookie: f.d.Endpoint().Token},
	}
	for _, route := range guardedRoutes() {
		for _, v := range pass {
			rec := serve(f.d, v, route.method, route.path, route.body)
			if refusedByGuard(rec) || rec.Code == http.StatusForbidden {
				t.Errorf("%s %s as %s = %d (%s), want it to pass the guard", route.method, route.path, v.name, rec.Code, strings.TrimSpace(rec.Body.String()))
			}
			assertNoCORS(t, rec.Header())
		}
		for _, v := range refuse {
			rec := serve(f.d, v, route.method, route.path, route.body)
			if !refusedByGuard(rec) {
				t.Errorf("%s %s as %s = %d (%s), want the guard's 401", route.method, route.path, v.name, rec.Code, strings.TrimSpace(rec.Body.String()))
			}
			if strings.Contains(rec.Body.String(), token) {
				t.Errorf("%s %s as %s: the refusal carries the token", route.method, route.path, v.name)
			}
			assertNoCORS(t, rec.Header())
		}
	}
	// The token query sets the cookie; a request passing by the cookie or
	// by loopback sets none.
	if c := serve(f.d, pass[2], http.MethodGet, "/health", "").Result().Cookies(); len(c) != 1 || c[0].Name != lanTokenCookie || c[0].Value != token || !c[0].HttpOnly || c[0].Path != "/" || c[0].MaxAge <= 0 {
		t.Fatalf("cookie from the token query = %+v", c)
	}
	for _, v := range []viewer{pass[0], pass[3]} {
		if c := serve(f.d, v, http.MethodGet, "/health", "").Result().Cookies(); len(c) != 0 {
			t.Errorf("%s set cookies %+v", v.name, c)
		}
	}
}

// TestLANTokenIsNotAShutdownCredential: the LAN token never authorizes the
// shutdown request, and the endpoint file never holds it.
func TestLANTokenIsNotAShutdownCredential(t *testing.T) {
	f := startPageFixture(t, lanMode)
	token := readLANToken(t, f.dir)
	req := httptest.NewRequest(http.MethodPost, "/shutdown", nil)
	req.RemoteAddr = "127.0.0.1:50000"
	req.Host = "127.0.0.1"
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.d.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("shutdown with the LAN token = %d, want 401", rec.Code)
	}
	select {
	case <-f.d.Done():
		t.Fatal("the LAN token stopped the daemon")
	case <-time.After(50 * time.Millisecond):
	}
	raw, err := os.ReadFile(filepath.Join(f.dir, endpointFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), token) {
		t.Fatal("the endpoint file carries the LAN token")
	}
	if info, err := os.Stat(filepath.Join(f.dir, LANTokenFileName)); err != nil || (info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/') {
		t.Fatalf("token file mode = %v, %v; want owner-only", info.Mode(), err)
	}
}

// TestLANTokenSurvivesARestartAndRotatesWhenDeleted: a replaced daemon keeps
// the token a second device holds; deleting the token file issues a new one
// at once, and the old one stops passing.
func TestLANTokenSurvivesARestartAndRotatesWhenDeleted(t *testing.T) {
	dir := t.TempDir()
	start := func() *Daemon {
		d, err := StartDaemon(DaemonOptions{Dir: dir, Version: "1.0.0", LockWait: 2 * time.Second, LAN: true, bindHost: "127.0.0.1"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(d.Close)
		return d
	}
	lan := func(token string) viewer {
		return viewer{name: "LAN", peer: "192.168.7.30:50000", host: "192.168.7.20:29417", cookie: token}
	}
	first := start()
	token := readLANToken(t, dir)
	if rec := serve(first, lan(token), http.MethodGet, "/health", ""); rec.Code != http.StatusOK {
		t.Fatalf("token before restart = %d", rec.Code)
	}
	first.Close()
	waitDone(t, first)
	second := start()
	if again := readLANToken(t, dir); again != token {
		t.Fatalf("token after restart = %q, want %q", again, token)
	}
	if rec := serve(second, lan(token), http.MethodGet, "/health", ""); rec.Code != http.StatusOK {
		t.Fatalf("token after restart = %d", rec.Code)
	}

	if err := os.Remove(filepath.Join(dir, LANTokenFileName)); err != nil {
		t.Fatal(err)
	}
	if rec := serve(second, lan(token), http.MethodGet, "/health", ""); !refusedByGuard(rec) {
		t.Fatalf("old token after deleting the file = %d", rec.Code)
	}
	rotated := readLANToken(t, dir)
	if rotated == token {
		t.Fatal("deleting the token file kept the token")
	}
	if rec := serve(second, lan(rotated), http.MethodGet, "/health", ""); rec.Code != http.StatusOK {
		t.Fatalf("new token = %d", rec.Code)
	}
}

// TestIndexShowsTheLANTokenToALocalViewerOnly: the index page shows the
// LAN addresses with the token to a browser on this machine, never to a LAN
// browser, and not at all in localhost mode.
func TestIndexShowsTheLANTokenToALocalViewerOnly(t *testing.T) {
	f := startPageFixture(t, lanMode)
	token := readLANToken(t, f.dir)
	port := strconv.Itoa(f.d.Endpoint().Port)
	local := serve(f.d, viewer{peer: "127.0.0.1:50000", host: "localhost:" + port}, http.MethodGet, "/", "")
	if local.Code != http.StatusOK || !strings.Contains(local.Body.String(), `class="lanbox"`) || !strings.Contains(local.Body.String(), ":"+port+"/?token="+token) {
		t.Fatalf("local viewer's index (%d) lacks the token address:\n%.2000s", local.Code, local.Body.String())
	}
	remote := serve(f.d, viewer{peer: "192.168.7.30:50000", host: "192.168.7.20:" + port, cookie: token}, http.MethodGet, "/", "")
	if remote.Code != http.StatusOK || strings.Contains(remote.Body.String(), token) || strings.Contains(remote.Body.String(), `class="lanbox"`) {
		t.Fatalf("LAN viewer's index (%d) shows the token", remote.Code)
	}
	// The session page and the index stream show no token either.
	for _, path := range []string{"/s/livesession", "/events"} {
		if rec := serve(f.d, viewer{peer: "127.0.0.1:50000", host: "localhost:" + port}, http.MethodGet, path, ""); strings.Contains(rec.Body.String(), token) {
			t.Errorf("%s carries the token", path)
		}
	}

	off := startPageFixture(t)
	if _, page := get(t, off.base+"/"); strings.Contains(page, `class="lanbox"`) {
		t.Fatal("localhost mode shows a LAN block")
	}
	if _, err := os.Stat(filepath.Join(off.dir, LANTokenFileName)); !os.IsNotExist(err) {
		t.Fatalf("localhost mode wrote a LAN token: %v", err)
	}
}

// TestLANTokenReachesTheStreamByCookie: over a real connection, a stream
// opened by a non-local viewer needs the token; the token query answers with
// the cookie, and the cookie alone then opens the stream, as a page's
// EventSource does.
func TestLANTokenReachesTheStreamByCookie(t *testing.T) {
	f := startPageFixture(t, lanMode)
	token := readLANToken(t, f.dir)
	lanHost := "192.168.7.20:" + strconv.Itoa(f.d.Endpoint().Port)
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	open := func(query string, cookie *http.Cookie) *http.Response {
		req, _ := http.NewRequest(http.MethodGet, f.base+"/s/livesession/events"+query, nil)
		req.Host = lanHost // a loopback peer that addressed a LAN Host
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := open("", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("stream without the token = %d", resp.StatusCode)
	}
	resp = open("?token="+token, nil)
	resp.Body.Close()
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == lanTokenCookie {
			cookie = c
		}
	}
	if resp.StatusCode != http.StatusOK || cookie == nil || !cookie.HttpOnly {
		t.Fatalf("stream with the token query = %d, cookie %+v", resp.StatusCode, cookie)
	}
	resp = open("", &http.Cookie{Name: cookie.Name, Value: cookie.Value})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream with the cookie = %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	events := make(chan sseEvent, 16)
	go readEvents(t, bufio.NewReader(resp.Body), events)
	if ev, ok := nextRegion(t, events, 2*time.Second); !ok || ev.Name == "" {
		t.Fatalf("no region over the cookie-authenticated stream: %+v", ev)
	}
}

// TestLocalhostModeIgnoresTheToken: with LAN mode off, the loopback Host
// guard alone decides, a token or cookie admits nothing more, and /health
// reports the mode.
func TestLocalhostModeIgnoresTheToken(t *testing.T) {
	f := startPageFixture(t)
	for _, v := range []viewer{
		{name: "foreign Host with a query", peer: "127.0.0.1:50000", host: "192.168.7.20:29417", query: "anything"},
		{name: "foreign Host with a cookie", peer: "127.0.0.1:50000", host: "192.168.7.20:29417", cookie: "anything"},
	} {
		if rec := serve(f.d, v, http.MethodGet, "/", ""); rec.Code != http.StatusForbidden {
			t.Errorf("%s = %d, want 403", v.name, rec.Code)
		}
	}
	info, err := Health(context.Background(), testClient(), f.d.Endpoint())
	if err != nil || info.LAN {
		t.Fatalf("localhost health = %+v, %v", info, err)
	}
	lan := startPageFixture(t, lanMode)
	if info, err := Health(context.Background(), testClient(), lan.d.Endpoint()); err != nil || !info.LAN {
		t.Fatalf("LAN health = %+v, %v", info, err)
	}
}

func TestLoopbackPeer(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:1":          true,
		"127.0.0.9:1":          true,
		"[::1]:1":              true,
		"[::ffff:127.0.0.1]:1": true,
		"192.168.0.2:1":        false,
		"[fe80::1%en0]:1":      false,
		"0.0.0.0:1":            false,
		"127.0.0.1":            false, // no port: not a remote address
		"":                     false,
	} {
		if got := loopbackPeer(addr); got != want {
			t.Errorf("loopbackPeer(%q) = %v, want %v", addr, got, want)
		}
	}
}
