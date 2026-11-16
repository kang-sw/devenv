package wsdashboard

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Test clients carry the real credential only to explicitly registered fixture
// origins. Raw clients and direct handler calls remain unauthenticated unless
// the test opts in, so security tests still exercise the production guard.
var localhostTestDaemons sync.Map // origin -> *Daemon

func localhostTestCookie(t *testing.T, d *Daemon) *http.Cookie {
	t.Helper()
	value, err := d.localhostCookieValue()
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: localhostCookieName, Value: value}
}

func addLocalhostTestCookie(t *testing.T, d *Daemon, r *http.Request) {
	t.Helper()
	r.AddCookie(localhostTestCookie(t, d))
}

func localhostTestHandler(t *testing.T, d *Daemon) http.Handler {
	if t != nil {
		t.Helper()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value, err := d.localhostCookieValue()
		if err != nil {
			panic(err) // a fixture setup failure, never a guard bypass
		}
		r.AddCookie(&http.Cookie{Name: localhostCookieName, Value: value})
		d.Handler().ServeHTTP(w, r)
	})
}

func registerLocalhostTestDaemon(t *testing.T, d *Daemon) {
	t.Helper()
	origin := endpointURL(d.Endpoint(), "")
	localhostTestDaemons.Store(origin, d)
	t.Cleanup(func() { localhostTestDaemons.Delete(origin) })
	var cookies []map[string]any
	localhostTestDaemons.Range(func(key, value any) bool {
		c := localhostTestCookie(t, value.(*Daemon))
		cookies = append(cookies, map[string]any{"name": c.Name, "value": c.Value, "url": key.(string), "httpOnly": true, "sameSite": "Strict"})
		return true
	})
	raw, err := json.Marshal(cookies)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WS_DASHBOARD_TEST_COOKIES", string(raw))
	// All existing Chromium fixtures import WS_PLAYWRIGHT_MODULE. Point that
	// test-only seam at a wrapper that seeds each browser context's cookie jar.
	if module := os.Getenv("WS_PLAYWRIGHT_MODULE"); module != "" && os.Getenv("WS_PLAYWRIGHT_REAL_MODULE") == "" {
		helper, err := filepath.Abs("testdata/localhost-cookie.cjs")
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("WS_PLAYWRIGHT_REAL_MODULE", module)
		t.Setenv("WS_PLAYWRIGHT_MODULE", helper)
	}
}

// localhostTestTransport is confined to test clients; it neither disables the
// gate nor changes a request's Host, Origin, fetch metadata or other cookies.
type localhostTestTransport struct{ base http.RoundTripper }

func (tr localhostTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if d, ok := localhostTestDaemons.Load(r.URL.Scheme + "://" + r.URL.Host); ok {
		value, err := d.(*Daemon).localhostCookieValue()
		if err != nil {
			return nil, err
		}
		r = r.Clone(r.Context())
		r.AddCookie(&http.Cookie{Name: localhostCookieName, Value: value})
	}
	return tr.base.RoundTrip(r)
}

func localhostTestHTTPTransport() http.RoundTripper {
	return localhostTestTransport{base: &http.Transport{Proxy: nil}}
}
