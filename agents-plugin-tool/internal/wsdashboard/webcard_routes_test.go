package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// webFrameHeaders are what the session page's iframe sends loading the
// wrapper.
var webFrameHeaders = map[string]string{"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Dest": "iframe"}

// webFromLAN runs one request through f's daemon from a viewer that passes
// LAN mode's guard with the token cookie but is not on this machine (a
// foreign peer, or a loopback peer that addressed a LAN Host), with the
// route's Origin and the wrapper's Sec-Fetch headers.
func webFromLAN(t *testing.T, f *pageFixture, peer, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	host := "192.168.7.20:" + strconv.Itoa(f.d.Endpoint().Port)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = peer
	req.Host = host
	req.Header.Set("Origin", "http://"+host)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range webFrameHeaders {
		req.Header.Set(k, v)
	}
	req.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: readLANToken(t, f.dir)})
	rec := httptest.NewRecorder()
	f.d.Handler().ServeHTTP(rec, req)
	return rec
}

// webCSPDirective is the directive name of the policy csp, "" when absent.
func webCSPDirective(csp, name string) string {
	for _, d := range strings.Split(csp, ";") {
		d = strings.TrimSpace(d)
		if d == name || strings.HasPrefix(d, name+" ") {
			return d
		}
	}
	return ""
}

// TestWebCardRegistrationRefusesTargetsOffTheAllowList pins decision 13's
// allow-list on the registration route: another scheme, a host that is not
// localhost, 127.0.0.1, or [::1] exactly, user info, a bad port, the
// daemon's own port, and a live proxy listener's port answer 400 and
// register nothing.
func TestWebCardRegistrationRefusesTargetsOffTheAllowList(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t)
	_, lp := webDevProxy(t, f, u)
	dp := strconv.Itoa(f.d.Endpoint().Port)
	for _, raw := range []string{
		"ftp://127.0.0.1:21/",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"ws://127.0.0.1:3000/",
		"127.0.0.1:3000",
		"//127.0.0.1:3000/",
		"http://example.com/",
		"http://192.168.1.10:3000/",
		"http://127.0.0.2:3000/",
		"http://0.0.0.0:3000/",
		"http://localhost.evil.example:3000/",
		"http://[::2]:3000/",
		"http://user:pw@127.0.0.1:3000/",
		"http://user@localhost:3000/",
		"http://127.0.0.1:0/",
		"http://127.0.0.1:70000/",
		"http://127.0.0.1:" + dp + "/",
		"http://localhost:" + dp + "/s/livesession",
		"http://[::1]:" + dp + "/",
		"http://127.0.0.1:" + strconv.Itoa(lp) + "/",
		"http://localhost:" + strconv.Itoa(lp) + "/",
	} {
		body, _ := json.Marshal(map[string]string{"url": raw})
		if code, _, msg := postJSON(t, f.base+"/s/livesession/web", f.base, string(body)); code != http.StatusBadRequest {
			t.Errorf("register %q = %d, want 400: %s", raw, code, msg)
		}
	}
	for _, body := range []string{`{}`, `{"url":"` + u.URL + `","path":"index.html"}`, `not json`} {
		if code, _, msg := postJSON(t, f.base+"/s/livesession/web", f.base, body); code != http.StatusBadRequest {
			t.Errorf("register %s = %d, want 400: %s", body, code, msg)
		}
	}
	f.d.web.mu.Lock()
	n := len(f.d.web.byKey)
	f.d.web.mu.Unlock()
	if n != 1 {
		t.Errorf("refused targets left %d registrations, want only the upstream's", n)
	}
	// The three loopback names pass.
	for _, host := range []string{"localhost", "127.0.0.1", "[::1]", "LOCALHOST"} {
		reg := registerWebTarget(t, f, `{"url":"http://`+host+`:10001/"}`)
		if reg.Kind != "dev" || !strings.HasSuffix(reg.Base, ":10001") {
			t.Errorf("register %s = %+v", host, reg)
		}
	}
}

// TestWebCardRoutesRefuseAViewerOffThisMachine pins decision 6's
// same-machine rule on every web card route: a LAN viewer carrying the
// token, as a foreign peer or as a loopback peer under a LAN Host, gets 404
// from registration, the wrapper, and the heartbeat.
func TestWebCardRoutesRefuseAViewerOffThisMachine(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t, lanMode)
	reg, _ := webDevProxy(t, f, u)
	f.d.web.mu.Lock()
	before := len(f.d.web.byKey)
	f.d.web.mu.Unlock()
	for _, peer := range []string{"192.168.7.30:50000", "127.0.0.1:50000"} {
		for _, c := range []struct{ method, path, body string }{
			{http.MethodPost, "/s/livesession/web", `{"url":"http://127.0.0.1:10001/"}`},
			{http.MethodGet, "/s/livesession/web/" + reg.ID + "?p=/", ""},
			{http.MethodPost, "/s/livesession/web/" + reg.ID + "/beat", ""},
		} {
			if rec := webFromLAN(t, f, peer, c.method, c.path, c.body); rec.Code != http.StatusNotFound {
				t.Errorf("%s %s from %s = %d, want 404: %s", c.method, c.path, peer, rec.Code, rec.Body.String())
			}
		}
	}
	f.d.web.mu.Lock()
	after := len(f.d.web.byKey)
	f.d.web.mu.Unlock()
	if after != before {
		t.Errorf("a refused viewer registered a target")
	}
}

// TestWebCardRegistrationAndBeatRefuseAnotherOrigin pins decision 20's
// TrustedWrite gate on the two POST routes: a foreign Origin, an opaque
// one, and none answer 403; the heartbeat of an unknown id answers 404 and
// of a known one 204; a GET answers 405.
func TestWebCardRegistrationAndBeatRefuseAnotherOrigin(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t)
	reg, _ := webDevProxy(t, f, u)
	body := `{"url":"http://127.0.0.1:10001/"}`
	for _, origin := range []string{"http://evil.example", "null", "", u.URL, "http://localhost:" + strconv.Itoa(f.d.Endpoint().Port+1)} {
		if code, _, msg := postJSON(t, f.base+"/s/livesession/web", origin, body); code != http.StatusForbidden {
			t.Errorf("register with Origin %q = %d, want 403: %s", origin, code, msg)
		}
		if code, _, msg := postJSON(t, f.base+"/s/livesession/web/"+reg.ID+"/beat", origin, ""); code != http.StatusForbidden {
			t.Errorf("beat with Origin %q = %d, want 403: %s", origin, code, msg)
		}
	}
	if code, _, msg := postJSON(t, f.base+"/s/livesession/web/nosuchid/beat", f.base, ""); code != http.StatusNotFound {
		t.Errorf("beat of an unknown id = %d, want 404: %s", code, msg)
	}
	if code, _, msg := postJSON(t, f.base+"/s/livesession/web/"+reg.ID+"/beat", f.base, ""); code != http.StatusNoContent {
		t.Errorf("beat of a registered id = %d, want 204: %s", code, msg)
	}
	if code, _, msg := postJSON(t, f.base+"/s/nosuchsession/web", f.base, body); code != http.StatusNotFound {
		t.Errorf("register on an unknown session = %d, want 404: %s", code, msg)
	}
	if code, _, _ := webGet(t, f.base+"/s/livesession/web", nil); code != http.StatusMethodNotAllowed {
		t.Errorf("GET registration = %d, want 405", code)
	}
	if code, _, _ := webGet(t, f.base+"/s/livesession/web/"+reg.ID+"/beat", nil); code != http.StatusMethodNotAllowed {
		t.Errorf("GET beat = %d, want 405", code)
	}
	if code, _, _ := webGet(t, f.base+"/s/livesession/web/"+reg.ID+"/other", webFrameHeaders); code != http.StatusNotFound {
		t.Errorf("unknown action = %d, want 404", code)
	}
}

// TestWebCardSameTargetRegistersAsTheSameID pins decision 28: registering
// a target again (another path on the same dev origin, the same static
// file or a sibling) returns the id it already has.
func TestWebCardSameTargetRegistersAsTheSameID(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t)
	webWrite(t, f.project, "site/index.html", "<html></html>")
	webWrite(t, f.project, "site/other.html", "<html></html>")
	a := registerWebTarget(t, f, `{"url":"`+u.URL+`/a"}`)
	b := registerWebTarget(t, f, `{"url":"`+u.URL+`/b?x=1"}`)
	if a.ID != b.ID || b.Path != "/b?x=1" {
		t.Errorf("same dev origin = %+v then %+v, want one id", a, b)
	}
	s1 := registerWebTarget(t, f, `{"path":"site/index.html"}`)
	s2 := registerWebTarget(t, f, `{"path":"site/index.html"}`)
	s3 := registerWebTarget(t, f, `{"path":"site/other.html"}`)
	if s1.ID != s2.ID || s1.ID != s3.ID || s3.Path != "/other.html" {
		t.Errorf("same directory = %+v, %+v, %+v, want one id", s1, s2, s3)
	}
	if s1.ID == a.ID {
		t.Error("a dev target and a directory share an id")
	}
}

// TestWebCardRegistrationRefusesPastTheCap pins decision 23's cap on the
// route: the seventeenth live target answers 503 naming the cap.
func TestWebCardRegistrationRefusesPastTheCap(t *testing.T) {
	f := startPageFixture(t)
	for i := 0; i < webListenerCap; i++ {
		registerWebTarget(t, f, `{"url":"http://127.0.0.1:`+strconv.Itoa(10001+i)+`/"}`)
	}
	code, _, msg := postJSON(t, f.base+"/s/livesession/web", f.base, `{"url":"http://127.0.0.1:10100/"}`)
	if code != http.StatusServiceUnavailable || !strings.Contains(msg, "at most 16") {
		t.Errorf("seventeenth target = %d, want 503 naming the cap: %s", code, msg)
	}
}

// TestWebCardWrapperFramesOnlyItsCardsProxy pins decisions 20, 22, and 24
// on the wrapper's answer: frame-src is exactly its card's proxy origin on
// the canonical name of the Host it was addressed by (the same name in the
// inner frame's src and data-proxy, never the raw header's spelling);
// frame-ancestors 'self' and SAMEORIGIN; the inner frame's sandbox exactly
// webInnerSandbox, without top navigation.
func TestWebCardWrapperFramesOnlyItsCardsProxy(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t)
	reg, lp := webDevProxy(t, f, u)
	dp := strconv.Itoa(f.d.Endpoint().Port)
	if webInnerSandbox != "allow-scripts allow-same-origin allow-forms allow-popups allow-modals allow-downloads allow-popups-to-escape-sandbox" {
		t.Errorf("webInnerSandbox = %q", webInnerSandbox)
	}
	for host, name := range map[string]string{
		"127.0.0.1:" + dp: "127.0.0.1",
		"localhost:" + dp: "localhost",
		"LocalHost:" + dp: "localhost",
		"LOCALHOST:" + dp: "localhost",
		"[::1]:" + dp:     "[::1]",
	} {
		headers := map[string]string{"Host": host}
		for k, v := range webFrameHeaders {
			headers[k] = v
		}
		code, h, page := webGet(t, f.base+"/s/livesession/web/"+reg.ID+"?p=/app", headers)
		if code != http.StatusOK {
			t.Errorf("wrapper via %s = %d: %s", host, code, page)
			continue
		}
		proxy := "http://" + name + ":" + strconv.Itoa(lp)
		csp := h.Get("Content-Security-Policy")
		if got := webCSPDirective(csp, "frame-src"); got != "frame-src "+proxy {
			t.Errorf("via %s: %s, want frame-src %s", host, got, proxy)
		}
		if got := webCSPDirective(csp, "frame-ancestors"); got != "frame-ancestors 'self'" {
			t.Errorf("via %s: %s, want frame-ancestors 'self'", host, got)
		}
		if got := webCSPDirective(csp, "default-src"); got != "default-src 'none'" {
			t.Errorf("via %s: %s", host, got)
		}
		if got := h.Values("X-Frame-Options"); len(got) != 1 || got[0] != "SAMEORIGIN" {
			t.Errorf("via %s: X-Frame-Options %q, want SAMEORIGIN", host, got)
		}
		iframe := `<iframe id="f" src="` + proxy + `/app" sandbox="` + webInnerSandbox + `" referrerpolicy="no-referrer"></iframe>`
		if !strings.Contains(page, iframe) || strings.Count(page, "<iframe") != 1 {
			t.Errorf("via %s: inner frame is not %s:\n%s", host, iframe, page)
		}
		if !strings.Contains(page, ` data-proxy="`+proxy+`"`) {
			t.Errorf("via %s: data-proxy is not %s", host, proxy)
		}
		if strings.Contains(page, "allow-top-navigation") {
			t.Errorf("via %s: the inner frame may navigate the top level", host)
		}
		if host != strings.ToLower(host) && (strings.Contains(page, strings.Split(host, ":")[0]) || strings.Contains(csp, strings.Split(host, ":")[0])) {
			t.Errorf("via %s: the raw Host spelling leaks into the wrapper", host)
		}
	}
}

// TestWebCardWrapperLoadsOnlyInItsCard pins decision 20's wrapper guard:
// the session page's iframe load passes; a Sec-Fetch-Site other than
// same-origin, a Sec-Fetch-Dest other than iframe, or either header absent
// answers 403; an unknown id or session 404; a POST 405.
func TestWebCardWrapperLoadsOnlyInItsCard(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t)
	reg, _ := webDevProxy(t, f, u)
	wrapper := f.base + "/s/livesession/web/" + reg.ID + "?p=/"
	if code, _, page := webGet(t, wrapper, webFrameHeaders); code != http.StatusOK {
		t.Fatalf("wrapper = %d: %s", code, page)
	}
	for _, headers := range []map[string]string{
		{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Dest": "iframe"},
		{"Sec-Fetch-Site": "same-site", "Sec-Fetch-Dest": "iframe"},
		{"Sec-Fetch-Site": "none", "Sec-Fetch-Dest": "iframe"},
		{"Sec-Fetch-Site": "same-origin"},
		{"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Dest": "document"},
		{"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Dest": "embed"},
		{"Sec-Fetch-Dest": "iframe"},
		{},
	} {
		if code, _, page := webGet(t, wrapper, headers); code != http.StatusForbidden {
			t.Errorf("wrapper with %v = %d, want 403: %s", headers, code, page)
		}
	}
	if code, _, _ := webGet(t, f.base+"/s/livesession/web/nosuchid?p=/", webFrameHeaders); code != http.StatusNotFound {
		t.Errorf("unknown id = %d, want 404", code)
	}
	if code, _, _ := webGet(t, f.base+"/s/nosuchsession/web/"+reg.ID+"?p=/", webFrameHeaders); code != http.StatusNotFound {
		t.Errorf("unknown session = %d, want 404", code)
	}
	if code, _, _ := webDo(t, http.MethodPost, wrapper, webFrameHeaders, ""); code != http.StatusMethodNotAllowed {
		t.Errorf("POST wrapper = %d, want 405", code)
	}
}

// TestWebCardWrapperKeepsTheFramedPathOnItsProxy pins the wrapper's p
// check: a single-slash path opens on the proxy origin; a scheme-relative
// or absolute URL, a backslash trick, white space, or no path falls back to
// "/".
func TestWebCardWrapperKeepsTheFramedPathOnItsProxy(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t)
	reg, lp := webDevProxy(t, f, u)
	proxy := "http://127.0.0.1:" + strconv.Itoa(lp)
	for p, want := range map[string]string{
		"/a/b?c=1#top":         "/a/b?c=1#top",
		"/":                    "/",
		"//evil.example/x":     "/",
		"http://x":             "/",
		"https://evil.example": "/",
		`/\evil.example`:       "/",
		"/a b":                 "/",
		"a/b":                  "/",
		"javascript:alert(1)":  "/",
		"":                     "/",
	} {
		code, _, page := webGet(t, f.base+"/s/livesession/web/"+reg.ID+"?p="+url.QueryEscape(p), webFrameHeaders)
		if code != http.StatusOK || !strings.Contains(page, `<iframe id="f" src="`+proxy+want+`" `) {
			t.Errorf("p %q = %d, want the inner frame at %s%s:\n%s", p, code, proxy, want, page)
		}
	}
}

// TestWebCardWrapperRecreatesAnIdleListener pins decision 28's wrapper
// load: after the target's listener closed, loading the wrapper starts it
// again on its old port and frames that port.
func TestWebCardWrapperRecreatesAnIdleListener(t *testing.T) {
	u := startWebUpstream(t)
	f := startPageFixture(t)
	reg, lp := webDevProxy(t, f, u)
	f.d.web.mu.Lock()
	f.d.web.closeListener(f.d.web.byID[reg.ID].ln)
	f.d.web.mu.Unlock()
	if webListening(lp) {
		t.Fatalf("port %d still accepts connections", lp)
	}
	code, h, page := webGet(t, f.base+"/s/livesession/web/"+reg.ID+"?p=/", webFrameHeaders)
	if code != http.StatusOK || webCSPDirective(h.Get("Content-Security-Policy"), "frame-src") != "frame-src http://127.0.0.1:"+strconv.Itoa(lp) {
		t.Fatalf("wrapper after the close = %d %q: %s", code, h.Get("Content-Security-Policy"), page)
	}
	if code, _, body := webGet(t, "http://127.0.0.1:"+strconv.Itoa(lp)+"/", nil); code != http.StatusOK || !strings.Contains(body, "hello") {
		t.Errorf("recreated listener = %d: %s", code, body)
	}
}
