package wsdashboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// openRecorder stands in for the OS open command: it records each target
// and never starts a browser.
type openRecorder struct {
	mu      sync.Mutex
	targets []string
	err     error
}

func (o *openRecorder) open(target string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.targets = append(o.targets, target)
	return o.err
}

func (o *openRecorder) take() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := o.targets
	o.targets = nil
	return out
}

// openRequestAs posts a system-open body as a request from peer to host
// with the given Origin ("" sends none) and cookie token ("" sends none).
func openRequestAs(d *Daemon, method, peer, host, origin, cookie, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/s/livesession/open", strings.NewReader(body))
	req.RemoteAddr = peer
	req.Host = host
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: cookie})
	}
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

func openBody(target string) string {
	raw, _ := json.Marshal(openRequest{Target: target})
	return string(raw)
}

// TestSystemOpenRoute pins the system-open route's server-side checks: only
// a browser on this machine (a LAN token holder gets 404), only with the
// daemon's own Origin, only POST, only for a served session, and only an
// http(s) URL or this session's own file view path, which opens on the
// loopback address the request named. A refused request never reaches the
// open command, and a failing command answers 500.
func TestSystemOpenRoute(t *testing.T) {
	rec := &openRecorder{}
	f := startPageFixture(t, lanMode, func(o *DaemonOptions) { o.openSystem = rec.open })
	token := readLANToken(t, f.dir)
	local := "127.0.0.1:" + strconv.Itoa(f.d.Endpoint().Port)
	lanHost := "192.168.7.20:" + strconv.Itoa(f.d.Endpoint().Port)
	const peer = "127.0.0.1:50000"

	opened := map[string]string{
		"https://example.com/a?b=1&c=2|3#frag":           "https://example.com/a?b=1&c=2|3#frag",
		"http://localhost:9/x":                           "http://localhost:9/x",
		"HTTPS://Example.com/":                           "https://Example.com/",
		"/s/livesession/file?path=docs%2Fa.md":           "http://" + local + "/s/livesession/file?path=docs%2Fa.md",
		"http://" + local + "/s/livesession/file?path=x": "http://" + local + "/s/livesession/file?path=x",
	}
	for target, want := range opened {
		r := openRequestAs(f.d, http.MethodPost, peer, local, "http://"+local, "", openBody(target))
		got := rec.take()
		if r.Code != http.StatusNoContent || len(got) != 1 || got[0] != want {
			t.Errorf("open %q = %d %q, opened %q, want 204 and %q", target, r.Code, strings.TrimSpace(r.Body.String()), got, want)
		}
		assertNoCORS(t, r.Header())
	}

	refusedTargets := []string{
		"", "javascript:alert(1)", "file:///etc/passwd", "ftp://example.com/", "mailto:a@b.c",
		"http:/x", "http:x", "http://", "https://user:pw@example.com/", "//evil.example/x", "-a",
		"https://example.com/a b", `https://example.com/"x`, "https://example.com/\n-x", `https://example.com\x`,
		"https://exämple.com/", "https://example.com/" + strings.Repeat("a", openTargetMax),
		"/s/livesession/file", "/s/livesession/file?path=", "/s/livesession/file?path=a&x=1", "/s/livesession/file?path=a#f",
		"/s/boardsession/file?path=a", "/s/livesession/events", "/s/livesession/open", "s/livesession/file?path=a",
	}
	for _, target := range refusedTargets {
		r := openRequestAs(f.d, http.MethodPost, peer, local, "http://"+local, "", openBody(target))
		if r.Code != http.StatusBadRequest {
			t.Errorf("open %q = %d, want 400", target, r.Code)
		}
	}
	if r := openRequestAs(f.d, http.MethodPost, peer, local, "http://"+local, "", `{"target":`); r.Code != http.StatusBadRequest {
		t.Errorf("malformed body = %d, want 400", r.Code)
	}

	good := openBody("https://example.com/")
	for name, r := range map[string]*httptest.ResponseRecorder{
		// A LAN token holder passes the guard but not the route.
		"LAN peer with the token":        openRequestAs(f.d, http.MethodPost, "192.168.7.30:50000", lanHost, "http://"+lanHost, token, good),
		"loopback peer, LAN Host, token": openRequestAs(f.d, http.MethodPost, peer, lanHost, "http://"+lanHost, token, good),
	} {
		if r.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", name, r.Code)
		}
	}
	for name, origin := range map[string]string{"no Origin": "", "foreign Origin": "http://evil.example", "opaque Origin": "null", "https Origin": "https://" + local} {
		if r := openRequestAs(f.d, http.MethodPost, peer, local, origin, "", good); r.Code != http.StatusForbidden {
			t.Errorf("%s = %d, want 403", name, r.Code)
		}
	}
	if r := openRequestAs(f.d, http.MethodGet, peer, local, "http://"+local, "", ""); r.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET open = %d, want 405", r.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/s/nosuchsession/open", strings.NewReader(good))
	req.RemoteAddr, req.Host = peer, local
	req.Header.Set("Origin", "http://"+local)
	unknown := httptest.NewRecorder()
	f.d.Handler().ServeHTTP(unknown, req)
	if unknown.Code != http.StatusNotFound {
		t.Errorf("open on an unknown session = %d, want 404", unknown.Code)
	}
	if got := rec.take(); len(got) != 0 {
		t.Fatalf("refused requests reached the open command: %q", got)
	}

	rec.err = errors.New("no browser")
	if r := openRequestAs(f.d, http.MethodPost, peer, local, "http://"+local, "", good); r.Code != http.StatusInternalServerError {
		t.Errorf("failing open command = %d, want 500", r.Code)
	}
}

// TestSessionPageOffersSystemOpenToLocalViewersOnly: the page carries the
// system-open flag for a browser on this machine, and not for a LAN token
// holder, whose open would land on the host.
func TestSessionPageOffersSystemOpenToLocalViewersOnly(t *testing.T) {
	f := startPageFixture(t, lanMode)
	token := readLANToken(t, f.dir)
	port := strconv.Itoa(f.d.Endpoint().Port)
	localPage := serve(f.d, viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + port}, http.MethodGet, "/s/livesession", "")
	if localPage.Code != http.StatusOK || !strings.Contains(localPage.Body.String(), `data-sysopen="1"`) {
		t.Errorf("local session page = %d, lacks the system-open flag", localPage.Code)
	}
	lanPage := serve(f.d, viewer{peer: "192.168.7.30:50000", host: "192.168.7.20:" + port, cookie: token}, http.MethodGet, "/s/livesession", "")
	if lanPage.Code != http.StatusOK || strings.Contains(lanPage.Body.String(), "data-sysopen") {
		t.Errorf("LAN session page = %d, carries the system-open flag", lanPage.Code)
	}
}
