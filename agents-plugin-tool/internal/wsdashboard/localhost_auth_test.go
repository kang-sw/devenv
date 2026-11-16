package wsdashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func authFixture(t *testing.T) *Daemon {
	t.Helper()
	return &Daemon{dir: t.TempDir(), log: io.Discard,
		endpoint: Endpoint{Port: 43210, Token: "endpoint-secret"}}
}

func authRequest(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Host = "127.0.0.1:43210"
	r.RemoteAddr = "127.0.0.1:12345"
	return r
}

func authCookie(t *testing.T, d *Daemon) *http.Cookie {
	t.Helper()
	value, err := d.localhostCookieValue()
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: localhostCookieName, Value: value}
}

func authServe(d *Daemon, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	d.Handler().ServeHTTP(w, r)
	return w
}

func authLink(t *testing.T, d *Daemon, path string) (string, string) {
	t.Helper()
	link, err := d.LoginLink(path)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	return link, u.Query().Get("login")
}

func exchangeRequest(code, next string) *http.Request {
	body, _ := json.Marshal(map[string]string{"code": code, "next": next})
	r := authRequest(http.MethodPost, "/auth/exchange", string(body))
	r.Header.Set("Origin", "http://"+r.Host)
	return r
}

func TestLocalhostAuthCookiePersistenceAndRotation(t *testing.T) {
	d := authFixture(t)
	cookie := authCookie(t, d)
	secretPath := filepath.Join(d.dir, localhostSecretFile)
	raw, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if cookie.Value == strings.TrimSpace(string(raw)) || cookie.Value == d.endpoint.Token {
		t.Fatal("cookie exposes a secret instead of a purpose-derived value")
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(secretPath)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("permissions = %o", st.Mode().Perm())
		}
	}
	r := authRequest(http.MethodGet, "/auth/status", "")
	r.AddCookie(cookie)
	if got := authServe(d, r); got.Code != 200 {
		t.Fatalf("status: %d %s", got.Code, got.Body.String())
	}
	restarted := authFixture(t)
	restarted.dir = d.dir
	if got := authCookie(t, restarted).Value; got != cookie.Value {
		t.Fatal("restart invalidated cookie")
	}
	if err := os.Remove(secretPath); err != nil {
		t.Fatal(err)
	}
	if got := authServe(d, r); got.Code != 401 || !strings.Contains(got.Body.String(), "login_required") {
		t.Fatalf("old cookie after rotation: %d %s", got.Code, got.Body.String())
	}
	if authCookie(t, d).Value == cookie.Value {
		t.Fatal("deletion did not rotate secret")
	}
}

func TestLocalhostAuthGuardRefusalsAndDefaultDeny(t *testing.T) {
	d := authFixture(t)
	for _, target := range []string{"/", "/s/session", "/events", "/s/session/events", "/s/session/terminals/t/events", "/agents", "/agents/id/send", "/support/stores/id/send", "/future-route", "/debug/pprof/"} {
		t.Run(target, func(t *testing.T) {
			r := authRequest(http.MethodPost, target, `{}`)
			if strings.Contains(target, "events") {
				r.Method = http.MethodGet
				r.Header.Set("Upgrade", "websocket")
				r.Header.Set("Connection", "Upgrade")
			}
			w := authServe(d, r)
			if w.Code != 401 || !strings.Contains(w.Body.String(), `"error":"login_required"`) || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("refusal: %d %v %s", w.Code, w.Header(), w.Body.String())
			}
		})
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		r := authRequest(method, "/s/session?x=1", "")
		r.Header.Set("Sec-Fetch-Mode", "navigate")
		w := authServe(d, r)
		if w.Code != 401 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("navigation gate: %d %v", w.Code, w.Header())
		}
		if method == http.MethodHead && w.Body.Len() != 0 {
			t.Fatal("HEAD gate has a body")
		}
	}
	r := authRequest(http.MethodGet, "/health", "")
	r.Host = "attacker.example"
	if w := authServe(d, r); w.Code != 403 {
		t.Fatalf("host guard = %d", w.Code)
	}
}

func TestLocalhostAuthNavigationClassification(t *testing.T) {
	for _, tc := range []struct {
		method, mode, site, accept string
		want                       bool
	}{
		{"GET", "navigate", "cross-site", "", true},
		{"HEAD", "navigate", "", "", true},
		{"POST", "navigate", "", "text/html", false},
		{"GET", "", "", "text/html,application/xhtml+xml", true},
		{"GET", "cors", "", "text/html", false},
		{"GET", "", "same-origin", "text/html", false},
		{"GET", "websocket", "", "text/html", false},
	} {
		r := authRequest(tc.method, "/", "")
		if tc.mode != "" {
			r.Header.Set("Sec-Fetch-Mode", tc.mode)
		}
		if tc.site != "" {
			r.Header.Set("Sec-Fetch-Site", tc.site)
		}
		r.Header.Set("Accept", tc.accept)
		if got := loginNavigationRequest(r); got != tc.want {
			t.Fatalf("%+v: %v", tc, got)
		}
	}
}

func TestLocalhostAuthNavigationExchange(t *testing.T) {
	d := authFixture(t)
	link, code := authLink(t, d, "/s/session?x=1&y=2")
	r := authRequest(http.MethodGet, link, "")
	r.Header.Set("Accept", "text/html")
	w := authServe(d, r)
	if w.Code != 303 || w.Header().Get("Location") != "/s/session?x=1&y=2" {
		t.Fatalf("exchange: %d %v", w.Code, w.Header())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies: %+v", cookies)
	}
	c := cookies[0]
	if c.Name != localhostCookieName || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.MaxAge != 34560000 {
		t.Fatalf("cookie attributes: %+v", c)
	}
	if ok, _ := d.consumeLoginCode(code); ok {
		t.Fatal("navigation code reused")
	}
	if w = authServe(d, r); w.Code != 401 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("used code: %d %v", w.Code, w.Header())
	}
	r.AddCookie(c)
	if w = authServe(d, r); w.Code != 303 {
		t.Fatalf("invalid code with valid cookie: %d", w.Code)
	}
	// A valid code is consumed even when navigation already has a cookie.
	link, code = authLink(t, d, "/")
	r = authRequest(http.MethodGet, link, "")
	r.Header.Set("Accept", "text/html")
	r.AddCookie(c)
	if w = authServe(d, r); w.Code != 303 {
		t.Fatalf("authenticated exchange: %d", w.Code)
	}
	if ok, _ := d.consumeLoginCode(code); ok {
		t.Fatal("authenticated navigation did not consume code")
	}
}

func TestLocalhostAuthPostExchangeAndValidation(t *testing.T) {
	d := authFixture(t)
	link, _ := authLink(t, d, "/s/other")
	r := exchangeRequest(link, "/s/wanted?q=1#topic")
	w := authServe(d, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"next":"/s/wanted?q=1#topic"`) || len(w.Result().Cookies()) != 1 {
		t.Fatalf("paste exchange: %d %s", w.Code, w.Body.String())
	}
	if w = authServe(d, exchangeRequest(link, "/")); w.Code != 401 || !strings.Contains(w.Body.String(), "invalid_code") {
		t.Fatalf("reuse: %d %s", w.Code, w.Body.String())
	}
	_, code := authLink(t, d, "/")
	for _, origin := range []string{"", "null", "http://evil.example", "http://127.0.0.1:1"} {
		r := exchangeRequest(code, "/")
		r.Header.Set("Origin", origin)
		if w := authServe(d, r); w.Code != 403 {
			t.Fatalf("origin %q: %d", origin, w.Code)
		}
	}
	for _, path := range []string{"//evil.example", "https://evil.example/", "/%2fhost", "/\\evil.example", "/%5cevil.example", "/%0d%0aevil", "/%7fevil", "/%00evil", "/?q=%0a", "/\vevil"} {
		if _, err := d.LoginLink(path); err == nil {
			t.Fatalf("mint accepted %q", path)
		}
		if w := authServe(d, exchangeRequest(code, path)); w.Code != 400 {
			t.Fatalf("next %q: %d", path, w.Code)
		}
	}
	if w := authServe(d, exchangeRequest(code, "/")); w.Code != 200 {
		t.Fatalf("refusals consumed valid code: %d", w.Code)
	}
}

func TestLocalhostAuthExpiryCapacityAndRateLimit(t *testing.T) {
	d := authFixture(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d.auth.now = func() time.Time { return now }
	_, expired := authLink(t, d, "/")
	now = now.Add(loginCodeLifetime)
	if ok, _ := d.consumeLoginCode(expired); ok {
		t.Fatal("code valid at expiry")
	}
	_, oldest := authLink(t, d, "/")
	var newest string
	for i := 0; i < loginCodeCapacity; i++ {
		_, newest = authLink(t, d, "/")
	}
	if len(d.auth.codes) != loginCodeCapacity {
		t.Fatalf("capacity = %d", len(d.auth.codes))
	}
	if ok, _ := d.consumeLoginCode(oldest); ok {
		t.Fatal("oldest was not evicted")
	}
	if ok, _ := d.consumeLoginCode(newest); !ok {
		t.Fatal("newest was evicted")
	}
	// Start a fresh rate window, then exhaust it with precisely ten failures.
	now = now.Add(time.Minute)
	for i := 0; i < loginFailureLimit; i++ {
		w := authServe(d, exchangeRequest("wrong", "/"))
		if w.Code != 401 {
			t.Fatalf("failure %d = %d", i, w.Code)
		}
	}
	link, valid := authLink(t, d, "/")
	navigation := authRequest(http.MethodGet, link, "")
	navigation.Header.Set("Sec-Fetch-Mode", "navigate")
	if w := authServe(d, navigation); w.Code != 429 || w.Header().Get("Retry-After") != "60" ||
		!strings.Contains(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(w.Body.String(), "Try again shortly.") {
		t.Fatalf("navigation bypassed limiter: %d %v", w.Code, w.Header())
	}
	w := authServe(d, exchangeRequest(valid, "/"))
	if w.Code != 429 || w.Header().Get("Retry-After") == "" || !strings.Contains(w.Body.String(), "rate_limited") {
		t.Fatalf("rate limit: %d %v %s", w.Code, w.Header(), w.Body.String())
	}
	now = now.Add(time.Minute)
	if w := authServe(d, exchangeRequest(valid, "/")); w.Code != 200 {
		t.Fatalf("window recovery: %d", w.Code)
	}
}

func TestLocalhostAuthCodeConcurrentSingleUse(t *testing.T) {
	d := authFixture(t)
	_, code := authLink(t, d, "/")
	var wg sync.WaitGroup
	results := make(chan bool, 8)
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() { defer wg.Done(); ok, _ := d.consumeLoginCode(code); results <- ok }()
	}
	wg.Wait()
	close(results)
	success := 0
	for ok := range results {
		if ok {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("consumed %d times", success)
	}
}

func TestLocalhostAuthMintingBoundaries(t *testing.T) {
	d := authFixture(t)
	for _, path := range []string{"/auth/link", "/auth/cookie"} {
		for _, tc := range []struct {
			name, peer, origin, bearer string
			cookie                     bool
			want                       int
		}{
			{"machine", "127.0.0.1:3", "", "endpoint-secret", false, 200},
			{"wrong bearer", "127.0.0.1:3", "", "wrong", false, 401},
			{"no credential", "127.0.0.1:3", "", "", false, 401},
			{"nonlocal cookie", "192.0.2.1:3", "http://127.0.0.1:43210", "", true, 401},
			{"nonlocal bearer", "192.0.2.1:3", "", "endpoint-secret", false, 401},
			{"browser bearer", "127.0.0.1:3", "http://127.0.0.1:43210", "endpoint-secret", false, 401},
			{"cookie without origin", "127.0.0.1:3", "", "", true, 403},
			{"browser cookie", "127.0.0.1:3", "http://127.0.0.1:43210", "", true, 200},
			{"foreign origin cookie", "127.0.0.1:3", "http://evil.example", "", true, 403},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				r := authRequest(http.MethodPost, path, `{"path":"/s/session"}`)
				r.RemoteAddr = tc.peer
				if tc.origin != "" {
					r.Header.Set("Origin", tc.origin)
				}
				if tc.bearer != "" {
					r.Header.Set("Authorization", "Bearer "+tc.bearer)
				}
				if tc.cookie {
					r.AddCookie(authCookie(t, d))
				}
				want := tc.want
				if path == "/auth/cookie" && tc.cookie {
					want = 401
				}
				w := authServe(d, r)
				if w.Code != want {
					t.Fatalf("got %d want %d: %s", w.Code, want, w.Body.String())
				}
				if w.Code == 200 && path == "/auth/cookie" {
					var got struct {
						Name, Value string
						MaxAge      int
					}
					if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
						t.Fatal(err)
					}
					if got.Name != localhostCookieName || got.Value != authCookie(t, d).Value || got.MaxAge != localhostCookieMaxAge {
						t.Fatalf("native cookie: %+v", got)
					}
				}
			})
		}
	}
}

func TestLocalhostAuthExemptionsAndLANIsolation(t *testing.T) {
	d := authFixture(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	guard := d.accessGuard(next)
	for _, tc := range []struct{ method, path string }{
		{"GET", "/health"}, {"HEAD", "/health"}, {"GET", ProxyPath}, {"POST", "/shutdown"}, {"POST", "/nudge"},
		{"GET", "/session-hire/roster"}, {"POST", "/agents/by-token/send"}, {"GET", "/pwa/manifest.webmanifest"},
		{"GET", "/s/rid/fs/cap/capability/file"}, {"GET", "/s/rid/fs/view/capability/file"},
	} {
		r := authRequest(tc.method, tc.path, "")
		w := httptest.NewRecorder()
		guard.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatalf("exempt %s %s = %d", tc.method, tc.path, w.Code)
		}
	}
	for _, path := range []string{"/health", "/pwa/manifest.webmanifest"} {
		if w := authServe(d, authRequest(http.MethodGet, path, "")); w.Code != 200 {
			t.Fatalf("actual exempt route %s = %d %s", path, w.Code, w.Body.String())
		}
	}
	name, _ := wswhiteboard.MermaidAsset()
	r := authRequest(http.MethodGet, "/s/"+name, "")
	if name != "" && !accessExempt(r) {
		t.Fatal("public asset not exempt")
	}
	d.lan = true
	token, err := d.lanToken()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		peer, host, query string
		cookie            *http.Cookie
		want              int
	}{
		{"127.0.0.1:3", "127.0.0.1:43210", "?token=" + token, nil, 401},
		{"127.0.0.1:3", "127.0.0.1:43210", "", &http.Cookie{Name: lanTokenCookie, Value: token}, 401},
		{"192.0.2.1:3", "192.0.2.2:43210", "?token=" + token, nil, 204},
		{"192.0.2.1:3", "192.0.2.2:43210", "", authCookie(t, d), 401},
		{"127.0.0.1:3", "127.0.0.1:43210", "", authCookie(t, d), 204},
	} {
		r := authRequest(http.MethodGet, "/private"+tc.query, "")
		r.RemoteAddr, r.Host = tc.peer, tc.host
		if tc.cookie != nil {
			r.AddCookie(tc.cookie)
		}
		w := httptest.NewRecorder()
		guard.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("LAN %+v = %d %s", tc, w.Code, w.Body.String())
		}
		if tc.peer != "127.0.0.1:3" {
			if local, _ := TrustedWrite(r); local {
				t.Fatal("LAN passed trusted gate")
			}
		}
	}
}

func TestLocalhostAuthProxyCookieStrip(t *testing.T) {
	h := http.Header{"Cookie": []string{localhostCookieName + "=secret; app=keep", lanTokenCookie + "=lan"}}
	stripDashboardCookies(h)
	if got := h.Get("Cookie"); got != "app=keep" {
		t.Fatalf("forwarded cookies: %q", got)
	}
}
