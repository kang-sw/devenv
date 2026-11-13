package wsdashboard

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// webClient is the tests' client of the proxy: no environment proxy, no
// redirect following, and no transparent gzip, so a test sees the
// Content-Encoding and Location the proxy sent.
var webClient = &http.Client{
	Timeout:       5 * time.Second,
	Transport:     &http.Transport{Proxy: nil, DisableCompression: true},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// webDo sends method to url with headers (a "Host" entry sets the request's
// Host) and body, and returns the status, the response headers, and the
// body.
func webDo(t *testing.T, method, url string, headers map[string]string, body string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		if strings.EqualFold(k, "Host") {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := webClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(raw)
}

// webGet requests url with headers and returns the status, the response
// headers, and the body.
func webGet(t *testing.T, url string, headers map[string]string) (int, http.Header, string) {
	t.Helper()
	return webDo(t, http.MethodGet, url, headers, "")
}

// registerWebTarget registers body ({url} or {path}) on the fixture's live
// session and returns the registration.
func registerWebTarget(t *testing.T, f *pageFixture, body string) webRegistration {
	t.Helper()
	code, _, raw := postJSON(t, f.base+"/s/livesession/web", f.base, body)
	if code != http.StatusOK {
		t.Fatalf("POST web %s = %d: %s", body, code, raw)
	}
	var reg webRegistration
	if err := json.Unmarshal([]byte(raw), &reg); err != nil {
		t.Fatal(err)
	}
	return reg
}

// webSeen is one request the upstream fixture received.
type webSeen struct {
	method, uri, host string
	header            http.Header
	body              string
}

// webUpstream is a dev server stand-in: an HTML page at every path a test
// does not claim, a WebSocket echo at /ws, and a record of every request it
// received. A test adds paths on mux.
type webUpstream struct {
	*httptest.Server
	mux  *http.ServeMux
	mu   sync.Mutex
	seen []webSeen
}

// webDevPage is the upstream fixture's default page.
const webDevPage = "<!doctype html><html><head><title>dev</title></head><body>hello</body></html>"

func startWebUpstream(t *testing.T) *webUpstream {
	t.Helper()
	u := &webUpstream{mux: http.NewServeMux()}
	u.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, webDevPage)
	})
	u.mux.HandleFunc("/ws", webEchoSocket)
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		u.mu.Lock()
		u.seen = append(u.seen, webSeen{method: r.Method, uri: r.URL.RequestURI(), host: r.Host, header: r.Header.Clone(), body: string(body)})
		u.mu.Unlock()
		u.mux.ServeHTTP(w, r)
	}))
	t.Cleanup(u.Close)
	return u
}

// hits is how many requests the fixture received.
func (u *webUpstream) hits() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.seen)
}

// last is the request the fixture received last.
func (u *webUpstream) last(t *testing.T) webSeen {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.seen) == 0 {
		t.Fatal("the upstream received no request")
	}
	return u.seen[len(u.seen)-1]
}

// hostPort is the fixture's 127.0.0.1:port.
func (u *webUpstream) hostPort() string { return u.Listener.Addr().String() }

// webEchoSocket answers a WebSocket upgrade with a raw 101 and echoes every
// byte the client sends, until either side closes.
func webEchoSocket(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "not an upgrade", http.StatusBadRequest)
		return
	}
	c, brw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		return
	}
	_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	_ = brw.Flush()
	go func() {
		defer c.Close()
		_, _ = io.Copy(c, brw)
	}()
}

// webDialUpgrade sends a WebSocket upgrade for /ws to the listener on port
// over a raw connection, with host as its Host and origin as its Origin
// ("" sends none), and returns the connection, its reader, and the answer.
func webDialUpgrade(t *testing.T, port int, host, origin string) (net.Conn, *bufio.Reader, *http.Response) {
	t.Helper()
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	req := "GET /ws HTTP/1.1\r\nHost: " + host + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n"
	if origin != "" {
		req += "Origin: " + origin + "\r\n"
	}
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, req+"\r\n"); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, br, resp
}

// webEcho writes msg on an upgraded connection and reads it back.
func webEcho(t *testing.T, c net.Conn, br *bufio.Reader, msg string) {
	t.Helper()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, msg); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(msg))
	if _, err := io.ReadFull(br, got); err != nil {
		t.Fatalf("echo of %q: %v", msg, err)
	}
	if string(got) != msg {
		t.Fatalf("echo = %q, want %q", got, msg)
	}
}

// webListening reports whether something accepts connections on the
// loopback port.
func webListening(port int) bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), time.Second)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// webPort is the live listener port of the target registered as id, 0 when
// it has none.
func webPort(g *webRegistry, id string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	if e := g.byID[id]; e != nil && e.ln != nil {
		return e.ln.port
	}
	return 0
}

// webDevProxy registers the upstream's origin on the fixture and returns the
// registration and its listener's port.
func webDevProxy(t *testing.T, f *pageFixture, u *webUpstream) (webRegistration, int) {
	t.Helper()
	reg := registerWebTarget(t, f, `{"url":"`+u.URL+`/"}`)
	port := webPort(f.d.web, reg.ID)
	if port == 0 {
		t.Fatalf("registration %+v has no live listener", reg)
	}
	return reg, port
}

// webClock is an idle clock a test moves by hand.
type webClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *webClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *webClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// newTestWebRegistry is a registry on a fake daemon port with a hand-moved
// clock, a one-minute idle close, and a three-minute live window.
func newTestWebRegistry(t *testing.T) (*webRegistry, *webClock) {
	t.Helper()
	g := newWebRegistry(9, "daemon-token", nil)
	clk := &webClock{t: time.Unix(1_800_000_000, 0)}
	g.now = clk.now
	g.idle = time.Minute
	g.live = 3 * time.Minute
	t.Cleanup(g.close)
	return g, clk
}

// TestWebCardProxiesADevPageWithTheScriptInjected walks the happy path: a
// registered dev-server page, its wrapper frame, and the proxied page with
// the injected script tag and the listener's framing policy.
func TestWebCardProxiesADevPageWithTheScriptInjected(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Frame-Options", "DENY")
		_, _ = io.WriteString(w, "<!doctype html><html><head><title>dev</title></head><body>hello</body></html>")
	}))
	defer upstream.Close()
	f := startPageFixture(t)
	reg := registerWebTarget(t, f, `{"url":"`+upstream.URL+`/app?x=1"}`)
	if reg.Kind != "dev" || reg.Path != "/app?x=1" || reg.ID == "" || reg.Base != upstream.URL {
		t.Fatalf("registration = %+v", reg)
	}
	code, h, page := webGet(t, f.base+"/s/livesession/web/"+reg.ID+"?p=/app%3Fx%3D1", map[string]string{"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Dest": "iframe"})
	if code != http.StatusOK {
		t.Fatalf("wrapper = %d: %s", code, page)
	}
	if h.Get("X-Frame-Options") != "SAMEORIGIN" || !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'self'") {
		t.Errorf("wrapper framing headers = %v", h)
	}
	e, _, _ := f.d.web.frame(reg.ID)
	proxy := "http://127.0.0.1:" + strconv.Itoa(e.port)
	if !strings.Contains(h.Get("Content-Security-Policy"), "frame-src "+proxy+";") || !strings.Contains(page, `src="`+proxy+`/app?x=1"`) {
		t.Errorf("wrapper does not frame %s: %v\n%s", proxy, h.Get("Content-Security-Policy"), page)
	}
	code, ph, body := webGet(t, proxy+"/app?x=1", nil)
	if code != http.StatusOK || !strings.Contains(body, `<head><script src="/.ws-web/inject.js"></script><title>dev</title>`) {
		t.Fatalf("proxied page = %d: %s", code, body)
	}
	if ph.Get("X-Frame-Options") != "" || !strings.Contains(strings.Join(ph.Values("Content-Security-Policy"), "|"), "frame-ancestors http://localhost:"+strconv.Itoa(f.d.Endpoint().Port)) {
		t.Errorf("proxied page framing headers = %v", ph)
	}
	code, _, script := webGet(t, proxy+webInjectPath, nil)
	if code != http.StatusOK || !strings.Contains(script, `var PORT = "`+strconv.Itoa(f.d.Endpoint().Port)+`"`) {
		t.Errorf("inject script = %d: %s", code, script)
	}
}

// TestWebCardListenerRefusesANonLoopbackHost pins the listener's hostGuard:
// a request naming a foreign host (a DNS-rebinding page) answers 403 and
// never reaches the target, while a loopback name passes.
func TestWebCardListenerRefusesANonLoopbackHost(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t)
	_, lp := webDevProxy(t, f, u)
	proxy := "http://127.0.0.1:" + strconv.Itoa(lp)
	for _, host := range []string{"evil.example:" + strconv.Itoa(lp), "evil.example", "192.168.7.20:" + strconv.Itoa(lp), "127.0.0.2:" + strconv.Itoa(lp)} {
		before := u.hits()
		if code, _, body := webGet(t, proxy+"/", map[string]string{"Host": host}); code != http.StatusForbidden {
			t.Errorf("Host %s = %d, want 403: %s", host, code, body)
		}
		if u.hits() != before {
			t.Errorf("Host %s reached the upstream", host)
		}
	}
	if code, _, _ := webGet(t, proxy+"/", map[string]string{"Host": "LocalHost:" + strconv.Itoa(lp)}); code != http.StatusOK {
		t.Errorf("a loopback Host = %d, want 200", code)
	}
}

// TestWebCardStripsOnlyTheDashboardsCookiesAndBearer pins decision 17: the
// theme, language, and LAN token cookies and an Authorization carrying the
// daemon's bearer never reach the target, while the dev app's own cookies
// and any other Authorization do.
func TestWebCardStripsOnlyTheDashboardsCookiesAndBearer(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t)
	_, lp := webDevProxy(t, f, u)
	proxy := "http://127.0.0.1:" + strconv.Itoa(lp)
	webGet(t, proxy+"/", map[string]string{
		"Cookie":        "ws_dashboard_theme=dark; app=1; ws_dashboard_lang=ko; ws_dashboard_token=lan-secret; session=abc",
		"Authorization": "Bearer " + f.d.Endpoint().Token,
	})
	seen := u.last(t)
	if got := seen.header.Values("Cookie"); len(got) != 1 || got[0] != "app=1; session=abc" {
		t.Errorf("forwarded cookies = %q, want only the dev app's", got)
	}
	if got := seen.header.Values("Authorization"); len(got) != 0 {
		t.Errorf("the daemon's bearer was forwarded: %q", got)
	}
	// Only dashboard cookies: no Cookie header at all.
	webGet(t, proxy+"/", map[string]string{"Cookie": "ws_dashboard_theme=dark; ws_dashboard_lang=ko"})
	if got := u.last(t).header.Values("Cookie"); len(got) != 0 {
		t.Errorf("forwarded cookies = %q, want none", got)
	}
	for _, auth := range []string{"Bearer dev-app-token", "Basic dXNlcjpwYXNz", "Bearer " + f.d.Endpoint().Token + "x"} {
		webGet(t, proxy+"/", map[string]string{"Authorization": auth, "Cookie": "app=2"})
		seen := u.last(t)
		if got := seen.header.Get("Authorization"); got != auth {
			t.Errorf("Authorization %q forwarded as %q", auth, got)
		}
		if got := seen.header.Get("Cookie"); got != "app=2" {
			t.Errorf("dev cookie forwarded as %q", got)
		}
	}
}

// TestWebCardRewritesHostAndOnlyItsOwnOrigin pins decision 25's request
// rewriting: the upstream Host is the target's; the listener's own Origin
// under either loopback name is rewritten to the target's; a request with
// no Origin carries none.
func TestWebCardRewritesHostAndOnlyItsOwnOrigin(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t)
	_, lp := webDevProxy(t, f, u)
	port := strconv.Itoa(lp)
	proxy := "http://127.0.0.1:" + port
	for _, host := range []string{"127.0.0.1:" + port, "localhost:" + port} {
		webGet(t, proxy+"/", map[string]string{"Host": host})
		seen := u.last(t)
		if seen.host != u.hostPort() {
			t.Errorf("inbound Host %s reached the upstream as %q, want %q", host, seen.host, u.hostPort())
		}
		if got := seen.header.Values("Origin"); len(got) != 0 {
			t.Errorf("a navigation reached the upstream with Origin %q", got)
		}
	}
	for _, origin := range []string{"http://localhost:" + port, "http://127.0.0.1:" + port, "http://[::1]:" + port} {
		code, _, _ := webDo(t, http.MethodPost, proxy+"/submit", map[string]string{"Origin": origin, "Content-Type": "text/plain"}, "payload")
		if code != http.StatusOK {
			t.Errorf("POST with Origin %s = %d, want 200", origin, code)
			continue
		}
		seen := u.last(t)
		if got := seen.header.Get("Origin"); got != u.URL {
			t.Errorf("Origin %s reached the upstream as %q, want %q", origin, got, u.URL)
		}
		if seen.method != http.MethodPost || seen.body != "payload" || seen.host != u.hostPort() {
			t.Errorf("POST reached the upstream as %+v", seen)
		}
	}
	// A WebSocket upgrade from the listener's own origin is rewritten too.
	c, br, resp := webDialUpgrade(t, lp, "localhost:"+port, "http://localhost:"+port)
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("own-origin upgrade = %d", resp.StatusCode)
	}
	webEcho(t, c, br, "hi")
	if seen := u.last(t); seen.header.Get("Origin") != u.URL || seen.host != u.hostPort() {
		t.Errorf("upgrade reached the upstream with Origin %q Host %q", seen.header.Get("Origin"), seen.host)
	}
}

// TestWebCardRefusesForeignRequestsBeforeTheTarget pins decision 25's
// listener guard: a foreign Origin (another site, another loopback port,
// the dashboard itself, or "null") on a plain request, a POST, or a
// WebSocket upgrade, and a Sec-Fetch-Site cross-site request, answer 403
// and never reach the target.
func TestWebCardRefusesForeignRequestsBeforeTheTarget(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t)
	_, lp := webDevProxy(t, f, u)
	port := strconv.Itoa(lp)
	proxy := "http://127.0.0.1:" + port
	foreign := []string{
		"http://evil.example",
		"null",
		"http://127.0.0.1:" + strconv.Itoa(f.d.Endpoint().Port),
		u.URL,
		"https://localhost:" + port,
		"http://localhost.evil.example:" + port,
		"http://localhost:" + port + "/path",
	}
	for _, origin := range foreign {
		before := u.hits()
		if code, _, _ := webGet(t, proxy+"/", map[string]string{"Origin": origin}); code != http.StatusForbidden {
			t.Errorf("GET with Origin %q = %d, want 403", origin, code)
		}
		if code, _, _ := webDo(t, http.MethodPost, proxy+"/submit", map[string]string{"Origin": origin, "Content-Type": "text/plain"}, "x"); code != http.StatusForbidden {
			t.Errorf("POST with Origin %q = %d, want 403", origin, code)
		}
		_, _, resp := webDialUpgrade(t, lp, "127.0.0.1:"+port, origin)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("upgrade with Origin %q = %d, want 403", origin, resp.StatusCode)
		}
		if u.hits() != before {
			t.Errorf("Origin %q reached the upstream", origin)
		}
	}
	before := u.hits()
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		if code, _, _ := webDo(t, method, proxy+"/", map[string]string{"Sec-Fetch-Site": "cross-site"}, ""); code != http.StatusForbidden {
			t.Errorf("%s with Sec-Fetch-Site cross-site = %d, want 403", method, code)
		}
	}
	// Even with the listener's own Origin, cross-site wins.
	if code, _, _ := webGet(t, proxy+"/", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": proxy}); code != http.StatusForbidden {
		t.Errorf("cross-site with the own Origin = %d, want 403", code)
	}
	if u.hits() != before {
		t.Errorf("a cross-site request reached the upstream")
	}
	// The injected script's reserved path is guarded the same way.
	if code, _, _ := webGet(t, proxy+webInjectPath, map[string]string{"Origin": "http://evil.example"}); code != http.StatusForbidden {
		t.Errorf("inject script with a foreign Origin = %d, want 403", code)
	}
}

// TestWebCardRewritesRedirectsToTheTargetOnly pins decision 25's Location
// rule: an absolute Location naming the target moves to the proxy origin
// as the browser addressed it, a relative one stays, and a redirect to any
// other origin (the other loopback name, another port, https, a
// scheme-relative foreign host) answers 502.
func TestWebCardRewritesRedirectsToTheTargetOnly(t *testing.T) {
	u := startWebUpstream(t)
	up := strconv.Itoa(u.Listener.Addr().(*net.TCPAddr).Port)
	f := startPageFixture(t)
	locs := map[string]string{
		"/r/abs":       "http://127.0.0.1:" + up + "/next?q=1#frag",
		"/r/root":      "/next",
		"/r/rel":       "next/page?x=2",
		"/r/othername": "http://localhost:" + up + "/next",
		"/r/otherport": "http://127.0.0.1:" + strconv.Itoa(f.d.Endpoint().Port) + "/",
		"/r/https":     "https://127.0.0.1:" + up + "/next",
		"/r/schemerel": "//evil.example/x",
		"/r/foreign":   "http://evil.example/",
	}
	u.mux.HandleFunc("/r/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", locs[r.URL.Path])
		w.WriteHeader(http.StatusFound)
	})
	_, lp := webDevProxy(t, f, u)
	port := strconv.Itoa(lp)
	proxy := "http://127.0.0.1:" + port
	for host, want := range map[string]string{
		"localhost:" + port: "http://localhost:" + port + "/next?q=1#frag",
		"127.0.0.1:" + port: "http://127.0.0.1:" + port + "/next?q=1#frag",
	} {
		code, h, _ := webGet(t, proxy+"/r/abs", map[string]string{"Host": host})
		if code != http.StatusFound || h.Get("Location") != want {
			t.Errorf("absolute redirect via %s = %d %q, want %q", host, code, h.Get("Location"), want)
		}
	}
	for _, p := range []string{"/r/root", "/r/rel"} {
		code, h, _ := webGet(t, proxy+p, nil)
		if code != http.StatusFound || h.Get("Location") != locs[p] {
			t.Errorf("relative redirect %s = %d %q, want %q", p, code, h.Get("Location"), locs[p])
		}
	}
	for _, p := range []string{"/r/othername", "/r/otherport", "/r/https", "/r/schemerel", "/r/foreign"} {
		code, h, body := webGet(t, proxy+p, nil)
		if code != http.StatusBadGateway || h.Get("Location") != "" {
			t.Errorf("redirect %s to %s = %d (Location %q), want 502: %s", p, locs[p], code, h.Get("Location"), body)
		}
	}
}

// TestWebCardInjectsOnlyIntoHTML pins decision 25's injection: a script
// tag from the reserved path (never inline) goes into a text/html response
// only; JSON and JavaScript pass byte for byte; a gzip HTML body is decoded,
// injected, and sent without Content-Encoding; a br one passes as is; the
// proxy asks upstream for identity.
func TestWebCardInjectsOnlyIntoHTML(t *testing.T) {
	u := startWebUpstream(t)
	const marked = `{"html":"<html><head></head></html>"}`
	u.mux.HandleFunc("/data.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, marked)
	})
	const script = `document.write("<head></head>");`
	u.mux.HandleFunc("/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = io.WriteString(w, script)
	})
	u.mux.HandleFunc("/gzip", func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = io.WriteString(zw, webDevPage)
		_ = zw.Close()
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
		_, _ = w.Write(buf.Bytes())
	})
	u.mux.HandleFunc("/badgzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = io.WriteString(w, webDevPage)
	})
	// Not real brotli: the proxy must not touch an encoding it cannot decode.
	const brBody = "\x1b\x00<html><head></head>\x03"
	u.mux.HandleFunc("/br", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "br")
		_, _ = io.WriteString(w, brBody)
	})
	f := startPageFixture(t)
	_, lp := webDevProxy(t, f, u)
	proxy := "http://127.0.0.1:" + strconv.Itoa(lp)

	code, _, body := webGet(t, proxy+"/", nil)
	if code != http.StatusOK || !strings.Contains(body, "<head>"+webInjectTag+"<title>") || strings.Count(body, "<script") != 1 {
		t.Errorf("HTML page = %d: %s", code, body)
	}
	if webInjectTag != `<script src="/.ws-web/inject.js"></script>` {
		t.Errorf("inject tag = %s, want a script src from the reserved path", webInjectTag)
	}
	if got := u.last(t).header.Get("Accept-Encoding"); got != "identity" {
		t.Errorf("upstream Accept-Encoding = %q, want identity", got)
	}
	if code, h, body := webGet(t, proxy+"/data.json", nil); code != http.StatusOK || body != marked || h.Get("Content-Type") != "application/json" {
		t.Errorf("JSON = %d %q: %s", code, h.Get("Content-Type"), body)
	}
	if code, _, body := webGet(t, proxy+"/app.js", nil); code != http.StatusOK || body != script {
		t.Errorf("script = %d: %s", code, body)
	}
	code, h, body := webGet(t, proxy+"/gzip", map[string]string{"Accept-Encoding": "gzip"})
	if code != http.StatusOK || h.Get("Content-Encoding") != "" || !strings.Contains(body, "<head>"+webInjectTag+"<title>dev</title>") {
		t.Errorf("gzip page = %d (Content-Encoding %q): %q", code, h.Get("Content-Encoding"), body)
	}
	if cl := h.Get("Content-Length"); cl != "" && cl != strconv.Itoa(len(body)) {
		t.Errorf("gzip page Content-Length %s, body %d bytes", cl, len(body))
	}
	// A gzip header that does not decode: the decoder has read part of the
	// body, so the page answers 502 rather than a truncated gzip body.
	code, h, body = webGet(t, proxy+"/badgzip", map[string]string{"Accept-Encoding": "gzip"})
	if code != http.StatusBadGateway || h.Get("Content-Encoding") != "" || !strings.Contains(body, errWebBadGzip.Error()) {
		t.Errorf("bad gzip page = %d (Content-Encoding %q): %q", code, h.Get("Content-Encoding"), body)
	}
	code, h, body = webGet(t, proxy+"/br", map[string]string{"Accept-Encoding": "br"})
	if code != http.StatusOK || h.Get("Content-Encoding") != "br" || body != brBody {
		t.Errorf("br page = %d (Content-Encoding %q): %q", code, h.Get("Content-Encoding"), body)
	}
}

// TestWebCardPassesWebSocketUpgrades pins decision 15's upgrade
// passthrough: a WebSocket upgrade through the listener reaches the target
// and bytes echo end to end.
func TestWebCardPassesWebSocketUpgrades(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t)
	_, lp := webDevProxy(t, f, u)
	c, br, resp := webDialUpgrade(t, lp, "127.0.0.1:"+strconv.Itoa(lp), "")
	if resp.StatusCode != http.StatusSwitchingProtocols || !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		t.Fatalf("upgrade = %d %v", resp.StatusCode, resp.Header)
	}
	webEcho(t, c, br, "first frame")
	webEcho(t, c, br, strings.Repeat("x", 64<<10))
	if seen := u.last(t); seen.uri != "/ws" || !strings.EqualFold(seen.header.Get("Upgrade"), "websocket") {
		t.Errorf("upstream saw %+v", seen)
	}
}

// TestWebCardKeepsEveryPolicyButFrameAncestors pins decision 14's response
// policy: X-Frame-Options goes; frame-ancestors leaves every
// Content-Security-Policy and -Report-Only header (several headers, a
// comma-joined value, any case) while every other directive stays in
// order; the proxy appends its own frame-ancestors naming the dashboard on
// every loopback name.
func TestWebCardKeepsEveryPolicyButFrameAncestors(t *testing.T) {
	u := startWebUpstream(t)
	u.mux.HandleFunc("/csp", func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Add("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; script-src 'self' 'unsafe-eval'")
		h.Add("Content-Security-Policy", "frame-ancestors https://x.example")
		h.Add("Content-Security-Policy", "img-src *, FRAME-ANCESTORS 'self'; style-src 'self'")
		h.Add("Content-Security-Policy-Report-Only", "frame-ancestors 'none'; connect-src 'self'; report-uri /r")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, webDevPage)
	})
	f := startPageFixture(t)
	_, lp := webDevProxy(t, f, u)
	code, h, _ := webGet(t, "http://127.0.0.1:"+strconv.Itoa(lp)+"/csp", nil)
	if code != http.StatusOK {
		t.Fatalf("csp page = %d", code)
	}
	p := strconv.Itoa(f.d.Endpoint().Port)
	own := "frame-ancestors http://localhost:" + p + " http://127.0.0.1:" + p + " http://[::1]:" + p
	want := []string{"default-src 'self'; script-src 'self' 'unsafe-eval'", "img-src *, style-src 'self'", own}
	if got := h.Values("Content-Security-Policy"); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Content-Security-Policy = %q, want %q", got, want)
	}
	if got := h.Values("Content-Security-Policy-Report-Only"); len(got) != 1 || got[0] != "connect-src 'self'; report-uri /r" {
		t.Errorf("Content-Security-Policy-Report-Only = %q", got)
	}
	if got := h.Values("X-Frame-Options"); len(got) != 0 {
		t.Errorf("X-Frame-Options = %q, want none", got)
	}
	// A page with no policy of its own still gets the proxy's.
	if _, h, _ := webGet(t, "http://127.0.0.1:"+strconv.Itoa(lp)+"/", nil); strings.Join(h.Values("Content-Security-Policy"), "|") != own {
		t.Errorf("plain page policy = %q, want %q", h.Values("Content-Security-Policy"), own)
	}
}

// webWrite writes data to root/rel, creating its directories.
func webWrite(t *testing.T, root, rel, data string) string {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

// TestWebCardServesARegisteredDirectoryOnly pins decision 26's static
// serving: the registered file and its siblings and subdirectories serve
// (HTML injected, the proxy's frame-ancestors set); a ../ path, a symlink
// leading out (file or directory), a dotfile, and a dot-named directory
// answer 404; a dot-named ancestor of the registered directory does not
// block it; the registered directory swapped for a symlink afterwards is
// refused on the next request.
func TestWebCardServesARegisteredDirectoryOnly(t *testing.T) {
	f := startPageFixture(t)
	outside := t.TempDir()
	webWrite(t, outside, "secret.txt", "outside secret")
	webWrite(t, outside, "index.html", "<html><head></head><body>outside page</body></html>")
	webWrite(t, f.project, "site/index.html", "<html><head><title>site</title></head><body>site page</body></html>")
	webWrite(t, f.project, "site/app.js", "console.log('site')")
	webWrite(t, f.project, "site/sub/page.html", "<html><head></head><body>sub page</body></html>")
	webWrite(t, f.project, "site/.env", "TOKEN=secret")
	webWrite(t, f.project, "site/.git/x", "git object")
	webWrite(t, f.project, "secret.txt", "root secret")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(f.project, "site", "leak.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.project, "site", "leakdir")); err != nil {
		t.Fatal(err)
	}

	reg := registerWebTarget(t, f, `{"path":"site/index.html"}`)
	if reg.Kind != "static" || reg.Base != "site" || reg.Path != "/index.html" {
		t.Fatalf("registration = %+v", reg)
	}
	proxy := "http://127.0.0.1:" + strconv.Itoa(webPort(f.d.web, reg.ID))
	p := strconv.Itoa(f.d.Endpoint().Port)
	code, h, body := webGet(t, proxy+"/index.html", nil)
	if code != http.StatusOK || !strings.Contains(body, "<head>"+webInjectTag+"<title>site</title>") {
		t.Errorf("index = %d: %s", code, body)
	}
	if h.Get("Content-Security-Policy") != "frame-ancestors http://localhost:"+p+" http://127.0.0.1:"+p+" http://[::1]:"+p || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("index headers = %v", h)
	}
	if code, h, body := webGet(t, proxy+"/app.js", nil); code != http.StatusOK || body != "console.log('site')" || !strings.Contains(h.Get("Content-Type"), "javascript") {
		t.Errorf("sibling script = %d %q: %s", code, h.Get("Content-Type"), body)
	}
	if code, _, body := webGet(t, proxy+"/sub/page.html", nil); code != http.StatusOK || !strings.Contains(body, webInjectTag) || !strings.Contains(body, "sub page") {
		t.Errorf("subdirectory page = %d: %s", code, body)
	}
	if code, _, body := webGet(t, proxy+"/", nil); code != http.StatusOK || !strings.Contains(body, "site page") {
		t.Errorf("directory index = %d: %s", code, body)
	}
	for _, path := range []string{"/../secret.txt", "/%2e%2e/secret.txt", "/sub/../../secret.txt", "/leak.txt", "/leakdir/secret.txt", "/leakdir/index.html", "/.env", "/.git/x", "/sub/../.env", "/missing.html"} {
		if code, _, body := webGet(t, proxy+path, nil); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404: %s", path, code, body)
		}
	}

	// A dot-named ancestor of the registered directory does not block it.
	webWrite(t, f.project, ".docs/site/index.html", "<html><head></head><body>docs page</body></html>")
	docs := registerWebTarget(t, f, `{"path":".docs/site/index.html"}`)
	if docs.Base != ".docs/site" {
		t.Errorf("dot-ancestor registration = %+v", docs)
	}
	docsProxy := "http://127.0.0.1:" + strconv.Itoa(webPort(f.d.web, docs.ID))
	if code, _, body := webGet(t, docsProxy+"/index.html", nil); code != http.StatusOK || !strings.Contains(body, "docs page") || !strings.Contains(body, webInjectTag) {
		t.Errorf("dot-ancestor page = %d: %s", code, body)
	}

	// The registered directory swapped for a symlink leading out.
	webWrite(t, f.project, "swap/index.html", "<html><head></head><body>swap page</body></html>")
	swap := registerWebTarget(t, f, `{"path":"swap/index.html"}`)
	swapProxy := "http://127.0.0.1:" + strconv.Itoa(webPort(f.d.web, swap.ID))
	if code, _, _ := webGet(t, swapProxy+"/index.html", nil); code != http.StatusOK {
		t.Fatalf("swap page before the swap = %d", code)
	}
	if err := os.Rename(filepath.Join(f.project, "swap"), filepath.Join(f.project, "swap.old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.project, "swap")); err != nil {
		t.Fatal(err)
	}
	if code, _, body := webGet(t, swapProxy+"/index.html", nil); code != http.StatusNotFound {
		t.Errorf("swap page after the swap = %d, want 404: %s", code, body)
	}
}

// TestWebCardRefusesAPathOutsideTheSessionRoot pins decision 26's
// registration check: an absolute path elsewhere, a ../ escape, a symlinked
// directory leading out of the root, and a missing file answer 400 and
// register nothing.
func TestWebCardRefusesAPathOutsideTheSessionRoot(t *testing.T) {
	f := startPageFixture(t)
	outside := t.TempDir()
	abs := webWrite(t, outside, "index.html", "<html></html>")
	if err := os.Symlink(outside, filepath.Join(f.project, "escape")); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(f.project, abs)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{abs, filepath.ToSlash(rel), "../index.html", "escape/index.html", "escape", "missing/index.html"} {
		body, _ := json.Marshal(map[string]string{"path": p})
		code, _, raw := postJSON(t, f.base+"/s/livesession/web", f.base, string(body))
		if code != http.StatusBadRequest {
			t.Errorf("register path %q = %d, want 400: %s", p, code, raw)
		}
	}
	f.d.web.mu.Lock()
	n := len(f.d.web.byKey)
	f.d.web.mu.Unlock()
	if n != 0 {
		t.Errorf("refused paths left %d registrations", n)
	}
}

// TestWebCardIdleListenerClosesAndReopensOnItsPort pins decisions 23 and
// 29: a listener with nothing in flight and no live card closes after the
// idle time (its port then refuses connections), and the next wrapper
// load recreates it on the same port.
func TestWebCardIdleListenerClosesAndReopensOnItsPort(t *testing.T) {
	u := startWebUpstream(t)
	g, clk := newTestWebRegistry(t)
	e, _, err := g.registerURL(u.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	port := webPort(g, e.id)
	if port == 0 || !webListening(port) {
		t.Fatalf("no listener after registration (port %d)", port)
	}
	clk.advance(30 * time.Second)
	g.sweep()
	if webPort(g, e.id) != port {
		t.Fatal("the listener closed before the idle time")
	}
	clk.advance(31 * time.Second)
	g.sweep()
	if webPort(g, e.id) != 0 {
		t.Fatal("the listener outlived the idle time")
	}
	if webListening(port) {
		t.Fatalf("port %d still accepts connections after the idle close", port)
	}
	got, again, err := g.frame(e.id)
	if err != nil || got != e || again != port {
		t.Fatalf("frame after idle close = %v port %d (%v), want port %d", got, again, err, port)
	}
	if code, _, body := webGet(t, "http://127.0.0.1:"+strconv.Itoa(again)+"/", nil); code != http.StatusOK || !strings.Contains(body, "hello") {
		t.Errorf("recreated listener = %d: %s", code, body)
	}
}

// TestWebCardLiveCardKeepsItsListener pins decisions 28 and 29: a card
// that beat within the live window keeps its listener past the idle time,
// and once the beats stop the idle clock runs from the last sweep that saw
// the card live.
func TestWebCardLiveCardKeepsItsListener(t *testing.T) {
	g, clk := newTestWebRegistry(t)
	e, _, err := g.registerURL("http://127.0.0.1:10001/")
	if err != nil {
		t.Fatal(err)
	}
	port := webPort(g, e.id)
	clk.advance(30 * time.Second)
	if !g.heartbeat(e.id) {
		t.Fatal("heartbeat of a registered id = false")
	}
	// t0+2m and t0+3m20s: the beat (t0+30s) is within the live window.
	for _, step := range []time.Duration{90 * time.Second, 80 * time.Second} {
		clk.advance(step)
		g.sweep()
		if webPort(g, e.id) != port {
			t.Fatal("a live card's listener closed")
		}
	}
	// t0+4m: no longer live, idle for 40s only.
	clk.advance(40 * time.Second)
	g.sweep()
	if webPort(g, e.id) != port {
		t.Fatal("the listener closed before an idle time had passed since the card was last live")
	}
	// t0+4m30s: idle for 70s.
	clk.advance(30 * time.Second)
	g.sweep()
	if webPort(g, e.id) != 0 {
		t.Fatal("the listener outlived the idle time after the card stopped beating")
	}
	if g.heartbeat("nosuchid") {
		t.Error("heartbeat of an unknown id = true")
	}
}

// TestWebCardInFlightWorkKeepsItsListener pins decision 23: a request in
// flight and an open upgraded connection each keep a listener open past the
// idle time; once they end, the idle clock runs from then.
func TestWebCardInFlightWorkKeepsItsListener(t *testing.T) {
	u := startWebUpstream(t)
	entered, release := make(chan struct{}, 1), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	u.mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = io.WriteString(w, "done")
	})
	g, clk := newTestWebRegistry(t)
	e, _, err := g.registerURL(u.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	port := webPort(g, e.id)
	done := make(chan int, 1)
	go func() {
		resp, err := webClient.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/slow")
		if err != nil {
			done <- 0
			return
		}
		resp.Body.Close()
		done <- resp.StatusCode
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the slow request never reached the upstream")
	}
	clk.advance(10 * time.Minute)
	g.sweep()
	if webPort(g, e.id) != port {
		t.Fatal("a listener with a request in flight closed")
	}
	once.Do(func() { close(release) })
	if code := <-done; code != http.StatusOK {
		t.Fatalf("slow request = %d", code)
	}

	c, br, resp := webDialUpgrade(t, port, "127.0.0.1:"+strconv.Itoa(port), "")
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade = %d", resp.StatusCode)
	}
	webEcho(t, c, br, "open")
	clk.advance(10 * time.Minute)
	g.sweep()
	if webPort(g, e.id) != port {
		t.Fatal("a listener with an open upgraded connection closed")
	}
	webEcho(t, c, br, "still open")
	_ = c.Close()
	// The proxy notices the close on its own goroutines.
	waitFor(t, "the upgraded connection to end", func() bool {
		g.mu.Lock()
		defer g.mu.Unlock()
		return e.ln != nil && e.ln.inflight == 0 && len(e.ln.conns) == 0
	})
	clk.advance(2 * time.Minute)
	g.sweep()
	if webPort(g, e.id) != 0 {
		t.Fatal("the listener outlived the idle time after its work ended")
	}
}

// TestWebCardRegistryCapsLiveListeners pins decision 23's cap: sixteen
// live targets, the seventeenth refused with errWebCap and left
// unregistered, while a target already live still registers.
func TestWebCardRegistryCapsLiveListeners(t *testing.T) {
	g, _ := newTestWebRegistry(t)
	var first *webEntry
	for i := 0; i < webListenerCap; i++ {
		e, _, err := g.registerURL("http://127.0.0.1:" + strconv.Itoa(10001+i) + "/")
		if err != nil {
			t.Fatalf("target %d: %v", i, err)
		}
		if first == nil {
			first = e
		}
	}
	if _, _, err := g.registerURL("http://127.0.0.1:10100/"); !errors.Is(err, errWebCap) {
		t.Fatalf("seventeenth target = %v, want errWebCap", err)
	}
	g.mu.Lock()
	n := len(g.byKey)
	g.mu.Unlock()
	if n != webListenerCap {
		t.Errorf("registry holds %d targets after the refusal, want %d", n, webListenerCap)
	}
	if e, _, err := g.registerURL("http://127.0.0.1:10001/other"); err != nil || e != first {
		t.Errorf("a live target at the cap = %v, %v", e, err)
	}
}

// TestWebCardSameTargetKeepsItsEntry pins decision 23's keying: the same
// origin under another path is the same entry and listener; another
// loopback name is another target.
func TestWebCardSameTargetKeepsItsEntry(t *testing.T) {
	g, _ := newTestWebRegistry(t)
	a, inner, err := g.registerURL("http://127.0.0.1:10001/a?x=1#h")
	if err != nil || inner != "/a?x=1#h" {
		t.Fatalf("register = %q, %v", inner, err)
	}
	b, inner, err := g.registerURL("HTTP://127.0.0.1:10001/b")
	if err != nil || b != a || inner != "/b" {
		t.Errorf("same origin = %v %q %v, want the first entry", b, inner, err)
	}
	c, _, err := g.registerURL("http://localhost:10001/")
	if err != nil || c == a {
		t.Errorf("another loopback name = %v %v, want another entry", c, err)
	}
	if d, _, err := g.registerURL("http://127.0.0.1/"); err != nil || d.base != "http://127.0.0.1:80" {
		t.Errorf("default port = %+v %v", d, err)
	}
}

// TestWebCardNestedRootsKeepTheirOwnBase pins that a directory's base is
// relative to the registering session's root: a repository session and a
// subfolder session registering the same directory each get their own
// entry and base, while the same root registering again keeps its entry.
func TestWebCardNestedRootsKeepTheirOwnBase(t *testing.T) {
	g, _ := newTestWebRegistry(t)
	repo := t.TempDir()
	webWrite(t, repo, "app/site/index.html", "<html></html>")
	sub := filepath.Join(repo, "app")
	outer, _, err := g.registerDir(repo, "app/site/index.html")
	if err != nil || outer.base != "app/site" {
		t.Fatalf("repository root = %+v %v, want base app/site", outer, err)
	}
	inner, _, err := g.registerDir(sub, "site/index.html")
	if err != nil || inner == outer || inner.base != "site" {
		t.Errorf("subfolder root = %+v %v, want its own entry with base site", inner, err)
	}
	again, _, err := g.registerDir(repo, "app/site")
	if err != nil || again != outer {
		t.Errorf("repository root again = %+v %v, want its first entry", again, err)
	}
}

// TestWebCardRegistryCloseClosesEverything pins decision 23's teardown on
// the registry: close shuts every listener (its port refuses connections)
// and every upgraded connection, and refuses registration, a recreation,
// and a static directory from then on.
func TestWebCardRegistryCloseClosesEverything(t *testing.T) {
	u := startWebUpstream(t)
	g, _ := newTestWebRegistry(t)
	e, _, err := g.registerURL(u.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	webWrite(t, root, "site/index.html", "<html></html>")
	s, _, err := g.registerDir(root, "site/index.html")
	if err != nil {
		t.Fatal(err)
	}
	ports := []int{webPort(g, e.id), webPort(g, s.id)}
	c, br, resp := webDialUpgrade(t, ports[0], "127.0.0.1:"+strconv.Itoa(ports[0]), "")
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade = %d", resp.StatusCode)
	}
	webEcho(t, c, br, "before close")
	g.close()
	for _, p := range ports {
		if webListening(p) {
			t.Errorf("port %d accepts connections after close", p)
		}
	}
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	if n, err := br.ReadByte(); err == nil {
		t.Errorf("the upgraded connection is open after close (read %q)", n)
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Error("the upgraded connection is open after close (read timed out)")
	}
	if _, _, err := g.registerURL("http://127.0.0.1:10001/"); !errors.Is(err, errWebClosed) {
		t.Errorf("register after close = %v, want errWebClosed", err)
	}
	if _, _, err := g.registerDir(root, "site/index.html"); !errors.Is(err, errWebClosed) {
		t.Errorf("register a directory after close = %v, want errWebClosed", err)
	}
	if _, _, err := g.frame(e.id); !errors.Is(err, errWebClosed) {
		t.Errorf("frame after close = %v, want errWebClosed", err)
	}
	g.close() // idempotent
}

// TestWebCardDaemonCloseClosesItsListeners pins decision 23's Daemon.Close
// wiring: every proxy listener port refuses connections once Close
// returns, and the registry refuses new listeners.
func TestWebCardDaemonCloseClosesItsListeners(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t)
	webWrite(t, f.project, "site/index.html", "<html></html>")
	dev, devPort := webDevProxy(t, f, u)
	static := registerWebTarget(t, f, `{"path":"site/index.html"}`)
	staticPort := webPort(f.d.web, static.ID)
	f.d.Close()
	for _, p := range []int{devPort, staticPort} {
		if webListening(p) {
			t.Errorf("port %d accepts connections after Daemon.Close", p)
		}
	}
	if _, _, err := f.d.web.registerURL(u.URL); !errors.Is(err, errWebClosed) {
		t.Errorf("register after Daemon.Close = %v, want errWebClosed", err)
	}
	if _, _, err := f.d.web.frame(dev.ID); !errors.Is(err, errWebClosed) {
		t.Errorf("frame after Daemon.Close = %v, want errWebClosed", err)
	}
}
