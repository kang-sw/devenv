package wsdashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// hostRecorder stands in for the host: it records each command and never
// starts one. Its WSL conversion prefixes the path, so a test tells a
// converted path from a native one.
type hostRecorder struct {
	mu       sync.Mutex
	commands [][]string
	err      error
	winErr   error
}

func (h *hostRecorder) host(goos string, wsl bool) *hostFiles {
	return &hostFiles{goos: goos, wsl: wsl,
		winPath: func(path string) (string, error) {
			return `\\wsl.localhost\distro` + strings.ReplaceAll(path, "/", `\`), h.winErr
		},
		start: func(argv []string) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.commands = append(h.commands, argv)
			return h.err
		}}
}

func (h *hostRecorder) take() [][]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := h.commands
	h.commands = nil
	return out
}

func actRequestAs(d *Daemon, method, peer, host, origin, cookie, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/s/livesession/fs/act", strings.NewReader(body))
	req.RemoteAddr = peer
	req.Host = host
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: cookie})
	}
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, req)
	return rec
}

func actBody(path, action, root string) string {
	raw, _ := json.Marshal(fsActRequest{Path: path, Action: action, Root: root})
	return string(raw)
}

// TestHostFileCommand pins the command each host runs for open and reveal
// (Decision 7), WSL included: the path is one argument, never a shell
// string.
func TestHostFileCommand(t *testing.T) {
	const path, win = "/home/u/proj/a b.html", `\\wsl.localhost\Ubuntu\home\u\proj\a b.html`
	cases := []struct {
		goos   string
		wsl    bool
		action string
		path   string
		want   []string
	}{
		{"windows", false, "open", `C:\p\a b.html`, []string{"rundll32", "url.dll,FileProtocolHandler", `C:\p\a b.html`}},
		{"windows", false, "reveal", `C:\p\a b.html`, []string{"explorer", `/select,C:\p\a b.html`}},
		{"darwin", false, "open", path, []string{"open", path}},
		{"darwin", false, "reveal", path, []string{"open", "-R", path}},
		{"linux", false, "open", path, []string{"xdg-open", path}},
		{"linux", false, "reveal", path, []string{"xdg-open", "/home/u/proj"}},
		{"linux", true, "open", path, []string{"explorer.exe", win}},
		{"linux", true, "reveal", path, []string{"explorer.exe", "/select," + win}},
		{"freebsd", false, "reveal", path, []string{"xdg-open", "/home/u/proj"}},
		{"linux", false, "delete", path, nil},
		{"windows", false, "", path, nil},
	}
	for _, c := range cases {
		want := c.want
		if len(want) == 2 && want[0] == "xdg-open" && c.action == "reveal" {
			// The parent folder, as the test host's path rules spell it.
			want = []string{"xdg-open", filepath.Dir(c.path)}
		}
		if got := hostFileCommand(c.goos, c.wsl, c.action, c.path, win); !reflect.DeepEqual(got, want) {
			t.Errorf("hostFileCommand(%s, wsl=%v, %q) = %q, want %q", c.goos, c.wsl, c.action, got, want)
		}
	}
}

// TestFSActRoute pins the host route's server-side checks: only a browser
// on this machine (a LAN token holder gets 404), only with the daemon's own
// Origin, only POST, only for a served session, only "open" or "reveal",
// and only an entry inside the browsing root. A refused request never
// reaches the host command, and a failing command answers 500.
func TestFSActRoute(t *testing.T) {
	rec := &hostRecorder{}
	f := startPageFixture(t, lanMode, func(o *DaemonOptions) { o.hostFiles = rec.host("linux", false) })
	token := readLANToken(t, f.dir)
	local := "127.0.0.1:" + strconv.Itoa(f.d.Endpoint().Port)
	lanHost := "192.168.7.20:" + strconv.Itoa(f.d.Endpoint().Port)
	const peer = "127.0.0.1:50000"
	outside, symlinks := escapeTree(t, f)
	realProject, err := filepath.EvalSymlinks(f.project)
	if err != nil {
		t.Fatal(err)
	}

	ok := func(body string, want []string) {
		t.Helper()
		r := actRequestAs(f.d, http.MethodPost, peer, local, "http://"+local, "", body)
		got := rec.take()
		if r.Code != http.StatusNoContent || len(got) != 1 || !reflect.DeepEqual(got[0], want) {
			t.Errorf("act %s = %d %q, ran %q, want 204 and %q", body, r.Code, strings.TrimSpace(r.Body.String()), got, want)
		}
		assertNoCORS(t, r.Header())
	}
	ok(actBody("notes.txt", "open", ""), []string{"xdg-open", filepath.Join(realProject, "notes.txt")})
	ok(actBody("docs/deep/c.go", "reveal", ""), []string{"xdg-open", filepath.Join(realProject, "docs", "deep")})
	ok(actBody(filepath.Join(f.project, "docs"), "open", ""), []string{"xdg-open", filepath.Join(realProject, "docs")})
	ok(actBody("", "open", ""), []string{"xdg-open", realProject})
	// A root picked with change root: the entry is resolved inside it.
	realOutside, err := filepath.EvalSymlinks(outside)
	if err != nil {
		t.Fatal(err)
	}
	ok(actBody("secret.txt", "open", outside), []string{"xdg-open", filepath.Join(realOutside, "secret.txt")})
	if symlinks {
		// A symlink that stays inside the root opens what it resolves to.
		ok(actBody("alias.txt", "open", ""), []string{"xdg-open", filepath.Join(realProject, "notes.txt")})
	}

	refused := map[string]string{
		"an unknown action":       actBody("notes.txt", "delete", ""),
		"no action":               actBody("notes.txt", "", ""),
		"an action in other case": actBody("notes.txt", "Open", ""),
		"a traversal":             actBody("../"+filepath.Base(outside)+"/secret.txt", "open", ""),
		"an outside path":         actBody(filepath.Join(outside, "secret.txt"), "reveal", ""),
		"a missing entry":         actBody("missing.txt", "open", ""),
		"a relative root":         actBody("notes.txt", "open", "relative/root"),
		"a root that is a file":   actBody("", "open", filepath.Join(f.project, "notes.txt")),
		"a network root":          actBody("x", "open", "//server/share"),
		"an outside path, root":   actBody(filepath.Join(f.project, "notes.txt"), "open", outside),
		"a malformed body":        `{"path":`,
	}
	if symlinks {
		refused["a symlink out of the root"] = actBody("escape.txt", "open", "")
		refused["a path through a symlinked folder"] = actBody("outlink/secret.txt", "reveal", "")
	}
	for name, body := range refused {
		r := actRequestAs(f.d, http.MethodPost, peer, local, "http://"+local, "", body)
		if r.Code != http.StatusBadRequest || len(rec.take()) != 0 {
			t.Errorf("%s: act %s = %d, want 400 and no command", name, body, r.Code)
		}
	}

	body := actBody("notes.txt", "open", "")
	gates := []struct {
		name                       string
		method, peer, host, origin string
		cookie                     string
		want                       int
	}{
		{"a LAN token holder", http.MethodPost, "192.168.7.30:50000", lanHost, "http://" + lanHost, token, http.StatusNotFound},
		{"a loopback peer on a LAN Host", http.MethodPost, peer, lanHost, "http://" + lanHost, token, http.StatusNotFound},
		{"no Origin", http.MethodPost, peer, local, "", "", http.StatusForbidden},
		{"a foreign Origin", http.MethodPost, peer, local, "http://evil.example", "", http.StatusForbidden},
		{"an opaque Origin", http.MethodPost, peer, local, "null", "", http.StatusForbidden},
		{"GET", http.MethodGet, peer, local, "http://" + local, "", http.StatusMethodNotAllowed},
	}
	for _, g := range gates {
		r := actRequestAs(f.d, g.method, g.peer, g.host, g.origin, g.cookie, body)
		if r.Code != g.want || len(rec.take()) != 0 {
			t.Errorf("%s: act = %d %q, want %d and no command", g.name, r.Code, strings.TrimSpace(r.Body.String()), g.want)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/s/nosuchsession/fs/act", strings.NewReader(body))
	req.RemoteAddr, req.Host = peer, local
	req.Header.Set("Origin", "http://"+local)
	r := httptest.NewRecorder()
	f.d.Handler().ServeHTTP(r, req)
	if r.Code != http.StatusNotFound || len(rec.take()) != 0 {
		t.Errorf("unknown session act = %d, want 404", r.Code)
	}

	rec.err = errors.New("no such command")
	if r := actRequestAs(f.d, http.MethodPost, peer, local, "http://"+local, "", body); r.Code != http.StatusInternalServerError {
		t.Errorf("a failing command = %d, want 500", r.Code)
	}
	rec.take()
}

// TestFSActRouteUnderWSL pins the WSL path: the entry's path is converted
// to its Windows spelling and handed to explorer.exe, and a failed
// conversion runs nothing.
func TestFSActRouteUnderWSL(t *testing.T) {
	rec := &hostRecorder{}
	f := startPageFixture(t, func(o *DaemonOptions) { o.hostFiles = rec.host("linux", true) })
	local := "127.0.0.1:" + strconv.Itoa(f.d.Endpoint().Port)
	if err := os.WriteFile(filepath.Join(f.project, "page.html"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(filepath.Join(f.project, "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	win := `\\wsl.localhost\distro` + strings.ReplaceAll(real, "/", `\`)
	for action, want := range map[string][]string{"open": {"explorer.exe", win}, "reveal": {"explorer.exe", "/select," + win}} {
		r := actRequestAs(f.d, http.MethodPost, "127.0.0.1:50000", local, "http://"+local, "", actBody("page.html", action, ""))
		if got := rec.take(); r.Code != http.StatusNoContent || len(got) != 1 || !reflect.DeepEqual(got[0], want) {
			t.Errorf("WSL %s = %d, ran %q, want %q", action, r.Code, got, want)
		}
	}
	rec.winErr = errors.New("wslpath failed")
	if r := actRequestAs(f.d, http.MethodPost, "127.0.0.1:50000", local, "http://"+local, "", actBody("page.html", "open", "")); r.Code != http.StatusInternalServerError || len(rec.take()) != 0 {
		t.Errorf("a failed conversion = %d, want 500 and no command", r.Code)
	}
}
