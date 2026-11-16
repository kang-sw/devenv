package wsdashboard

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	// Chromium fixtures use either the normal module seam or the opt-in
	// performance seam. Both must seed real cookies before their navigation.
	helper, err := filepath.Abs("testdata/localhost-cookie.cjs")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"WS_PLAYWRIGHT_MODULE", "WS_VIEWED_PERF_PLAYWRIGHT", "BOARD_HEADLESS_PLAYWRIGHT", "PLAYWRIGHT_MODULE"} {
		module := os.Getenv(name)
		if module == "" || module == helper {
			continue
		}
		if os.Getenv("WS_PLAYWRIGHT_REAL_MODULE") == "" {
			t.Setenv("WS_PLAYWRIGHT_REAL_MODULE", module)
		}
		if name == "WS_PLAYWRIGHT_MODULE" || name == "WS_VIEWED_PERF_PLAYWRIGHT" {
			t.Setenv(name, helper)
		}
	}
	if os.Getenv("WS_PLAYWRIGHT_REAL_MODULE") != "" {
		// board-headless imports a directory-shaped ESM module, not the CJS
		// filename used by the other fixtures. Both adapters share one cached
		// wrapper object, including the acceptance safety preloader's hooks.
		shim := t.TempDir()
		source := "import { createRequire } from 'node:module';\nconst playwright = createRequire(import.meta.url)(" + strconv.Quote(helper) + ");\nexport const chromium = playwright.chromium;\nexport default playwright;\n"
		if err := os.WriteFile(filepath.Join(shim, "index.mjs"), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("BOARD_HEADLESS_PLAYWRIGHT", shim)
		if os.Getenv("PLAYWRIGHT_MODULE") != "" {
			t.Setenv("PLAYWRIGHT_MODULE", shim)
		}
	}
}

func TestLocalhostCookieHelperBrowserModuleSeams(t *testing.T) {
	seams := []string{"WS_PLAYWRIGHT_MODULE", "WS_VIEWED_PERF_PLAYWRIGHT", "BOARD_HEADLESS_PLAYWRIGHT", "PLAYWRIGHT_MODULE"}
	for _, seam := range seams {
		t.Run(seam, func(t *testing.T) {
			for _, name := range append([]string{"WS_PLAYWRIGHT_REAL_MODULE"}, seams...) {
				t.Setenv(name, "")
			}
			rawModule := filepath.Join(t.TempDir(), "cached-playwright")
			t.Setenv(seam, rawModule)
			f := startPageFixture(t)
			if os.Getenv("WS_PLAYWRIGHT_REAL_MODULE") != rawModule {
				t.Fatal("wrapper lost the caller's selected cached module")
			}
			helper, err := filepath.Abs("testdata/localhost-cookie.cjs")
			if err != nil {
				t.Fatal(err)
			}
			if (seam == seams[0] || seam == seams[1]) && os.Getenv(seam) != helper {
				t.Fatal("file-shaped module seam did not select the cookie wrapper")
			}
			shim := os.Getenv("BOARD_HEADLESS_PLAYWRIGHT")
			source, err := os.ReadFile(filepath.Join(shim, "index.mjs"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(source), "createRequire(import.meta.url)("+strconv.Quote(helper)+")") || !strings.Contains(string(source), "export const chromium = playwright.chromium;") {
				t.Fatal("ESM shim must reuse the same cookie wrapper and chromium object")
			}
			if seam == "PLAYWRIGHT_MODULE" && os.Getenv(seam) != shim {
				t.Fatal("higher-priority board module seam bypassed the ESM shim")
			}
			var cookies []struct {
				Name, Value, URL, SameSite string
				HTTPOnly                   bool
			}
			if err := json.Unmarshal([]byte(os.Getenv("WS_DASHBOARD_TEST_COOKIES")), &cookies); err != nil {
				t.Fatal(err)
			}
			if len(cookies) != 1 || cookies[0].Name != localhostCookieName || cookies[0].Value != localhostTestCookie(t, f.d).Value || cookies[0].URL != f.base || !cookies[0].HTTPOnly || cookies[0].SameSite != "Strict" {
				t.Fatal("browser seam did not receive the fixture's real HttpOnly Strict cookie")
			}
		})
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
