package wsdashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// webGet requests url with headers and returns the status, the response
// headers, and the body.
func webGet(t *testing.T, url string, headers map[string]string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
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
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(raw)
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
