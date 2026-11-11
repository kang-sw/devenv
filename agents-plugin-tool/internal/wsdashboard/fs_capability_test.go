package wsdashboard

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// capClock is a capability clock a test moves.
type capClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *capClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *capClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// capFrame asks the preview route for the capability URL of path as v.
func capFrame(t *testing.T, f *pageFixture, v viewer, session, path string) string {
	t.Helper()
	rec := serve(f.d, v, http.MethodGet, "/s/"+session+"/fs/preview?path="+path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("preview %s as %s = %d %s", path, v.name, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	i := strings.Index(body, `"frame":"`)
	if i < 0 {
		t.Fatalf("preview %s carries no frame: %s", path, body)
	}
	frame := body[i+len(`"frame":"`):]
	return frame[:strings.IndexByte(frame, '"')]
}

// TestFSCapabilityRoute pins the HTML preview's capability route: a valid
// capability serves the document and its relative assets to anyone holding
// it, a LAN peer with no token included, with the policy that sandboxes
// the document and limits every fetch to the capability's own prefix; a
// missing, altered, foreign-session, foreign-root, or expired capability
// answers 404; and a capability reads nothing outside its root.
func TestFSCapabilityRoute(t *testing.T) {
	clock := &capClock{now: time.Unix(1_800_000_000, 0)}
	f := startPageFixture(t, lanMode)
	f.d.fsNow = clock.read
	local, lan := fsViewers(t, f)
	outside, symlinks := escapeTree(t, f)
	writeFile(t, filepath.Join(f.project, "docs", "style.css"), "body{color:red}")
	writeFile(t, filepath.Join(f.project, "docs", "app.mjs"), "export default 1")
	writeFile(t, filepath.Join(f.project, "docs", "data.bin"), "BYTES")
	writeFile(t, filepath.Join(f.project, "art.svg"), "<svg><script>1</script></svg>")
	port := strconv.Itoa(f.d.Endpoint().Port)
	// A LAN peer that holds the capability and nothing else: no token.
	bare := viewer{name: "LAN peer, no token", peer: "192.168.7.31:50000", host: "192.168.7.20:" + port}

	frame := capFrame(t, f, lan, "livesession", "docs/page.html")
	prefix := strings.TrimSuffix(frame, "docs/page.html")
	if !strings.HasPrefix(frame, "/s/livesession/fs/cap/") || prefix == frame {
		t.Fatalf("capability URL = %q", frame)
	}
	for _, v := range []viewer{bare, local, lan} {
		rec := serve(f.d, v, http.MethodGet, frame, "")
		h := rec.Header()
		if rec.Code != http.StatusOK || rec.Body.String() != "<h1>HTML-INSIDE</h1>" || h.Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatalf("%s: capability document = %d %q %v", v.name, rec.Code, rec.Body.String(), h)
		}
		csp := h.Get("Content-Security-Policy")
		origin := "http://" + v.host + prefix
		for _, want := range []string{
			"sandbox allow-scripts;", "default-src " + origin + ";", "connect-src " + origin + ";", "img-src " + origin + " data: blob:;",
			"script-src " + origin + " 'unsafe-inline';", "style-src " + origin + " 'unsafe-inline';", "media-src " + origin + " data: blob:;",
			"font-src " + origin + " data:;", "object-src 'none'", "base-uri 'none'", "form-action 'none'",
		} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: policy lacks %q: %s", v.name, want, csp)
			}
		}
		for _, bad := range []string{"allow-same-origin", "'self'", "*", "'unsafe-eval'"} {
			if strings.Contains(csp, bad) {
				t.Errorf("%s: policy carries %q: %s", v.name, bad, csp)
			}
		}
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") != "no-store" || h.Get("Access-Control-Allow-Origin") != "*" || len(rec.Result().Cookies()) != 0 {
			t.Errorf("%s: capability headers = %v", v.name, h)
		}
	}
	// Relative assets resolve under the same prefix and are served by it.
	for path, want := range map[string][2]string{
		"docs/style.css": {"text/css; charset=utf-8", "body{color:red}"},
		"docs/app.mjs":   {"text/javascript; charset=utf-8", "export default 1"},
		"docs/data.bin":  {"application/octet-stream", "BYTES"},
		"shot.png":       {"image/png", "PNG-INSIDE"},
		"art.svg":        {"image/svg+xml", "<svg><script>1</script></svg>"},
	} {
		rec := serve(f.d, bare, http.MethodGet, prefix+path, "")
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != want[0] || rec.Body.String() != want[1] ||
			!strings.HasPrefix(rec.Header().Get("Content-Security-Policy"), "sandbox allow-scripts;") {
			t.Errorf("asset %s = %d %q %q", path, rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
		}
	}
	// It reads nothing outside its root, and only reads.
	refused := []string{prefix + "../../../../etc/hosts", prefix + "docs/../../secret.txt", prefix + "docs", prefix + "missing.html", prefix,
		prefix + strings.TrimPrefix(filepath.ToSlash(filepath.Join(outside, "secret.txt")), "/"), prefix + "/" + filepath.ToSlash(filepath.Join(outside, "secret.txt"))}
	if symlinks {
		refused = append(refused, prefix+"escape.txt", prefix+"alias.txt", prefix+"outlink/secret.txt", prefix+"outlink/sub/page.html")
	}
	for _, target := range refused {
		rec := serve(f.d, bare, http.MethodGet, target, "")
		if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "OUTSIDE") || strings.Contains(rec.Body.String(), "INSIDE") {
			t.Errorf("capability read of %s = %d %s, want a refusal", target, rec.Code, rec.Body.String())
		}
	}
	if rec := serve(f.d, bare, http.MethodPost, frame, ""); rec.Code == http.StatusOK || !refusedByGuard(rec) {
		t.Errorf("POST to the capability route without the token = %d, want the guard's 401", rec.Code)
	}
	// The capability admits its own route only: every other route still
	// needs the token.
	for _, target := range []string{"/s/livesession/fs/list", "/s/livesession/fs/preview?path=notes.txt", "/s/livesession/fs/raw?path=shot.png", "/s/livesession/file?path=notes.txt", "/s/livesession"} {
		if rec := serve(f.d, bare, http.MethodGet, target, ""); !refusedByGuard(rec) {
			t.Errorf("%s without the token = %d, want the guard's 401", target, rec.Code)
		}
	}

	// A missing, altered, or foreign capability.
	capability := strings.TrimSuffix(strings.TrimPrefix(prefix, "/s/livesession/fs/cap/"), "/")
	exp, mac, _ := strings.Cut(capability, "-")
	flipped := "0"
	if mac[0] == '0' {
		flipped = "1"
	}
	later, _ := strconv.ParseInt(exp, 36, 64)
	for name, target := range map[string]string{
		"no capability":         "/s/livesession/fs/cap/docs/page.html",
		"altered MAC":           "/s/livesession/fs/cap/" + exp + "-" + flipped + mac[1:] + "/docs/page.html",
		"extended expiry":       "/s/livesession/fs/cap/" + strconv.FormatInt(later+3600, 36) + "-" + mac + "/docs/page.html",
		"respelled expiry":      "/s/livesession/fs/cap/+" + exp + "-" + mac + "/docs/page.html",
		"another session's":     "/s/boardsession/fs/cap/" + capability + "/docs/page.html",
		"unknown session":       "/s/nosuchsession/fs/cap/" + capability + "/docs/page.html",
		"capability, no path":   "/s/livesession/fs/cap/" + capability,
		"the LAN token instead": "/s/livesession/fs/cap/" + readLANToken(t, f.dir) + "/docs/page.html",
	} {
		if rec := serve(f.d, bare, http.MethodGet, target, ""); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "INSIDE") {
			t.Errorf("%s: %s = %d %s, want 404", name, target, rec.Code, rec.Body.String())
		}
	}
	// A capability is scoped to the root it was minted for: one minted for
	// another root, by the same daemon and for the same session, fails.
	foreign := "/s/livesession/fs/cap/" + f.d.mintFSCapability("livesession", outside) + "/docs/page.html"
	if rec := serve(f.d, bare, http.MethodGet, foreign, ""); rec.Code != http.StatusNotFound {
		t.Errorf("a capability minted for another root = %d, want 404", rec.Code)
	}
	// Another daemon's capability (another key) fails too.
	other := startPageFixture(t, lanMode)
	other.d.fsNow = clock.read
	if rec := serve(f.d, bare, http.MethodGet, "/s/livesession/fs/cap/"+other.d.mintFSCapability("livesession", f.project)+"/docs/page.html", ""); rec.Code != http.StatusNotFound {
		t.Errorf("another daemon's capability = %d, want 404", rec.Code)
	}

	// It stops working once it expires; the page then asks for a new one.
	clock.advance(fsCapabilityTTL - time.Second)
	if rec := serve(f.d, bare, http.MethodGet, frame, ""); rec.Code != http.StatusOK {
		t.Errorf("a capability one second before its expiry = %d, want 200", rec.Code)
	}
	clock.advance(time.Second)
	for _, target := range []string{frame, prefix + "docs/style.css"} {
		if rec := serve(f.d, bare, http.MethodGet, target, ""); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "INSIDE") {
			t.Errorf("an expired capability on %s = %d, want 404", target, rec.Code)
		}
	}
	if fresh := capFrame(t, f, lan, "livesession", "docs/page.html"); fresh == frame {
		t.Error("a preview after the expiry carries the expired capability")
	} else if rec := serve(f.d, bare, http.MethodGet, fresh, ""); rec.Code != http.StatusOK {
		t.Errorf("a fresh capability = %d, want 200", rec.Code)
	}
}

// TestFSCapabilityRefusesAHostThePolicyCannotCarry pins that the route
// writes only a plain host into its policy: a Host that would read as a
// policy separator is refused instead of echoed.
func TestFSCapabilityRefusesAHostThePolicyCannotCarry(t *testing.T) {
	f := startPageFixture(t)
	writeFile(t, filepath.Join(f.project, "page.html"), "<p>x</p>")
	frame := f.d.fsCapabilityURL("livesession", f.project, "page.html")
	for host, want := range map[string]int{"127.0.0.1:8080": http.StatusOK, "localhost": http.StatusOK, "[::1]:9": http.StatusOK} {
		req := httptest.NewRequest(http.MethodGet, frame, nil)
		req.RemoteAddr, req.Host = "127.0.0.1:50000", host
		rec := httptest.NewRecorder()
		f.d.Handler().ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("Host %q = %d, want %d", host, rec.Code, want)
		}
	}
	for _, host := range []string{"a;b", "a b", "a,b", "a'b", "a/b"} {
		if capHost.MatchString(host) {
			t.Errorf("capHost admits %q", host)
		}
	}
}

// TestFSCapabilityURLEscapesThePath pins that a file name with URL
// metacharacters still resolves to itself through its capability URL.
func TestFSCapabilityURLEscapesThePath(t *testing.T) {
	f := startPageFixture(t)
	name := "a b#c?d%e.html"
	if runtime.GOOS == "windows" {
		name = "a b#c%e.html"
	}
	writeFile(t, filepath.Join(f.project, "sub dir", name), "ODD-NAME")
	frame := f.d.fsCapabilityURL("livesession", f.project, filepath.Join("sub dir", name))
	req := httptest.NewRequest(http.MethodGet, frame, nil)
	req.RemoteAddr, req.Host = "127.0.0.1:50000", "127.0.0.1"
	rec := httptest.NewRecorder()
	f.d.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "ODD-NAME" {
		t.Errorf("GET %s = %d %q", frame, rec.Code, rec.Body.String())
	}
}
