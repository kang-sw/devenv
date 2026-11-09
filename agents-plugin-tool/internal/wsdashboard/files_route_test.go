package wsdashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// filesAs runs one files route request through the daemon's handler from
// peer, addressed to host, with the LAN token cookie and Sec-Fetch-Site
// when set.
func filesAs(d *Daemon, peer, host, cookie, fetchSite, q string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/s/livesession/files?q="+url.QueryEscape(q), nil)
	req.RemoteAddr, req.Host = peer, host
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: cookie})
	}
	if fetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, req)
	return rec
}

// filesLocal is filesAs for a browser on this machine.
func filesLocal(f *pageFixture, q string) *httptest.ResponseRecorder {
	return filesAs(f.d, "127.0.0.1:50000", "127.0.0.1:"+strconv.Itoa(f.d.Endpoint().Port), "", "", q)
}

func decodeFiles(t *testing.T, r *httptest.ResponseRecorder) filesAnswer {
	t.Helper()
	if r.Code != http.StatusOK {
		t.Fatalf("files = %d %q, want 200", r.Code, r.Body.String())
	}
	h := r.Header()
	if h.Get("Content-Type") != "application/json" || h.Get("Cache-Control") != "no-store" || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("files headers %v", h)
	}
	if !strings.Contains(r.Body.String(), `"items":[`) {
		t.Errorf("files items is not an array: %s", r.Body.String())
	}
	var a filesAnswer
	if err := json.Unmarshal(r.Body.Bytes(), &a); err != nil {
		t.Fatalf("files answer %q: %v", r.Body.String(), err)
	}
	return a
}

// itemSet maps each answered path to its dir flag.
func itemSet(items []fileItem) map[string]bool {
	set := map[string]bool{}
	for _, it := range items {
		set[it.Path] = it.Dir
	}
	return set
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestFuzzyScoreRanking pins the scorer's ordering: a match inside the
// basename beats one scattered over a deep path, a consecutive run beats a
// scattered one, a path or camel boundary scores, a non-subsequence is
// rejected, and case is ignored.
func TestFuzzyScoreRanking(t *testing.T) {
	score := func(q, c string) int {
		t.Helper()
		s, ok := fuzzyScore(q, c)
		if !ok {
			t.Fatalf("fuzzyScore(%q, %q) rejected", q, c)
		}
		return s
	}
	for _, tc := range []struct{ q, better, worse string }{
		{"main", "src/main.go", "m/a/i/n/x.go"},
		{"abc", "xxabcxx", "xaxbxcx"},
		{"fb", "foo_bar.go", "fxxbxxx.go"},
		{"fb", "foo-bar.go", "fxxbxxx.go"},
		{"fb", "FooBar", "Foobar"},
		{"readme", "README.md", "docs/old/README.md"},
	} {
		if b, w := score(tc.q, tc.better), score(tc.q, tc.worse); b <= w {
			t.Errorf("%q: %q scored %d, not above %q at %d", tc.q, tc.better, b, tc.worse, w)
		}
	}
	for _, tc := range [][2]string{{"xyz", "abc"}, {"ba", "ab"}, {"mainx", "main.go"}} {
		if _, ok := fuzzyScore(tc[0], tc[1]); ok {
			t.Errorf("fuzzyScore(%q, %q) matched a non-subsequence", tc[0], tc[1])
		}
	}
	if score("README", "readme.md") != score("readme", "readme.md") || score("readme", "README.md") <= 0 {
		t.Errorf("fuzzyScore is case-sensitive")
	}
	if s, ok := fuzzyScore("", "anything"); !ok || s != 0 {
		t.Errorf("empty query = %d %v, want 0 true", s, ok)
	}
}

// TestAbsQueryModes pins mode detection: a leading slash in either
// direction or a drive letter and colon followed by a slash or nothing is
// abs mode; anything else is root mode.
func TestAbsQueryModes(t *testing.T) {
	for q, want := range map[string]bool{
		"/x": true, `\x`: true, "C:/": true, `c:\`: true, "C:": true, `\\server\s`: true, "/": true,
		"C:x": false, "src/x": false, "x": false, "": false, "1:/": false, ":/": false, "src\\x": false,
	} {
		if got := absQuery(q); got != want {
			t.Errorf("absQuery(%q) = %v, want %v", q, got, want)
		}
	}
}

// TestFilesRootModeListsGitFiles pins root mode over a git repository:
// tracked and untracked-but-not-ignored files are listed with the
// directories they sit in, ignored files and directories never are, and a
// query ranks the whole relative path.
func TestFilesRootModeListsGitFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	f := startPageFixture(t)
	gitRun(t, f.project, "init", "-q")
	writeFile(t, filepath.Join(f.project, ".gitignore"), "build/\nnode_modules/\nsecret.log\n")
	writeFile(t, filepath.Join(f.project, "src", "main.go"), "package main")
	writeFile(t, filepath.Join(f.project, "src", "deep", "util.go"), "package deep")
	gitRun(t, f.project, "add", ".")
	gitRun(t, f.project, "commit", "-q", "-m", "init")
	writeFile(t, filepath.Join(f.project, "notes", "todo.md"), "untracked")
	writeFile(t, filepath.Join(f.project, "build", "out.bin"), "ignored")
	writeFile(t, filepath.Join(f.project, "node_modules", "pkg", "index.js"), "ignored")
	writeFile(t, filepath.Join(f.project, "secret.log"), "ignored")

	a := decodeFiles(t, filesLocal(f, ""))
	if a.Mode != "root" || a.Dir != "" {
		t.Errorf("root answer mode %q dir %q", a.Mode, a.Dir)
	}
	got := itemSet(a.Items)
	want := map[string]bool{".gitignore": false, "src": true, "src/main.go": false, "src/deep": true, "src/deep/util.go": false, "notes": true, "notes/todo.md": false}
	for path, dir := range want {
		if d, ok := got[path]; !ok || d != dir {
			t.Errorf("root list lacks %q (dir %v): %v", path, dir, got)
		}
	}
	for path := range got {
		if strings.HasPrefix(path, "build") || strings.HasPrefix(path, "node_modules") || path == "secret.log" || strings.HasPrefix(path, ".git/") {
			t.Errorf("root list carries ignored %q", path)
		}
	}
	// Shallowest first, directories before files at one depth.
	if len(a.Items) < 3 || a.Items[0] != (fileItem{Path: "notes", Dir: true}) || a.Items[1] != (fileItem{Path: "src", Dir: true}) || a.Items[2].Path != ".gitignore" {
		t.Errorf("empty query order %v", a.Items)
	}

	a = decodeFiles(t, filesLocal(f, "main"))
	if len(a.Items) == 0 || a.Items[0] != (fileItem{Path: "src/main.go"}) {
		t.Errorf("query main = %v", a.Items)
	}
	a = decodeFiles(t, filesLocal(f, "zzzz"))
	if a.Items == nil || len(a.Items) != 0 {
		t.Errorf("unmatched query = %v", a.Items)
	}
}

// TestWalkFilesCaps pins the non-git fallback: the depth cap, the entry
// cap, dot-directories skipped (dotfiles kept), and a symlinked directory
// listed as an entry but never followed.
func TestWalkFilesCaps(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"top.txt", ".dotfile", "a/one.txt", "a/b/two.txt", "a/b/c/three.txt", ".hidden/x.txt", "a/.git/config"} {
		writeFile(t, filepath.Join(root, filepath.FromSlash(p)), p)
	}
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "leak.txt"), "outside")
	symlink := os.Symlink(outside, filepath.Join(root, "link")) == nil

	got := map[string]bool{}
	for _, p := range walkFiles(root, 3, 1000) {
		got[p] = true
	}
	for _, p := range []string{"top.txt", ".dotfile", "a/one.txt", "a/b/two.txt"} {
		if !got[p] {
			t.Errorf("walk lacks %q: %v", p, got)
		}
	}
	for _, p := range []string{"a/b/c/three.txt", ".hidden/x.txt", "a/.git/config", "link/leak.txt"} {
		if got[p] {
			t.Errorf("walk carries %q", p)
		}
	}
	if symlink && !got["link"] {
		t.Errorf("walk lacks the symlink entry itself: %v", got)
	}
	if all := walkFiles(root, 8, 1000); len(all) != len(got)+1 {
		t.Errorf("walk at depth 8 = %v, want the depth-3 list plus a/b/c/three.txt", all)
	}

	many := t.TempDir()
	for i := range 10 {
		writeFile(t, filepath.Join(many, fmt.Sprintf("f%02d.txt", i)), "x")
	}
	if n := len(walkFiles(many, 8, 4)); n != 4 {
		t.Errorf("walk with an entry cap of 4 listed %d", n)
	}

	cands := map[string]bool{}
	for _, c := range fileCandidates(walkFiles(root, 8, 1000)) {
		cands[c.path] = c.dir
	}
	if d, ok := cands["a/b/c"]; !ok || !d {
		t.Errorf("walk candidates lack the derived directory a/b/c: %v", cands)
	}
	if d, ok := cands["link"]; symlink && (!ok || d) {
		t.Errorf("walk candidates list the symlink as %v %v, want a file", d, ok)
	}
}

// TestFilesRootModeCapsAndCache pins the 50-item cap for an empty and a
// matching query, and the root list's cache: a file added appears only once
// the list is older than filesListTTL.
func TestFilesRootModeCapsAndCache(t *testing.T) {
	f := startPageFixture(t)
	for i := range 80 {
		writeFile(t, filepath.Join(f.project, fmt.Sprintf("file%02d.txt", i)), "x")
	}
	clock := time.Unix(1_000_000, 0)
	f.d.files.now = func() time.Time { return clock }

	for _, q := range []string{"", "file"} {
		if a := decodeFiles(t, filesLocal(f, q)); len(a.Items) != filesMax {
			t.Errorf("query %q answered %d items, want %d", q, len(a.Items), filesMax)
		}
	}

	writeFile(t, filepath.Join(f.project, "zebra.md"), "new")
	clock = clock.Add(filesListTTL - time.Second)
	if a := decodeFiles(t, filesLocal(f, "zebra")); len(a.Items) != 0 {
		t.Errorf("a file added inside the TTL is listed already: %v", a.Items)
	}
	clock = clock.Add(time.Second)
	if a := decodeFiles(t, filesLocal(f, "zebra")); len(a.Items) != 1 || a.Items[0].Path != "zebra.md" {
		t.Errorf("a file added is not listed after the TTL: %v", a.Items)
	}
}

// TestFilesAbsModeListsOneDirectoryForLoopbackViewers pins abs mode for a
// browser on this machine: one directory's entries, directories first for
// an empty basename, filtered by the basename, dot-entries only for a
// basename starting with a dot, a symlink's dir from its target, absolute
// slash paths, the cleaned directory in dir, and CORP same-origin. A missing
// directory or a file named as one is a 404.
func TestFilesAbsModeListsOneDirectoryForLoopbackViewers(t *testing.T) {
	f := startPageFixture(t, lanMode)
	outside := t.TempDir()
	mkdirAll(t, filepath.Join(outside, "sub"))
	mkdirAll(t, filepath.Join(outside, ".config"))
	writeFile(t, filepath.Join(outside, "alpha.txt"), "x")
	writeFile(t, filepath.Join(outside, "beta.md"), "x")
	writeFile(t, filepath.Join(outside, ".hidden"), "x")
	symlinks := os.Symlink(filepath.Join(outside, "sub"), filepath.Join(outside, "lnk")) == nil &&
		os.Symlink(filepath.Join(outside, "missing"), filepath.Join(outside, "broken")) == nil
	slash := filepath.ToSlash(outside)
	path := func(name string) string { return filepath.ToSlash(filepath.Join(outside, name)) }

	r := filesLocal(f, slash+"/")
	a := decodeFiles(t, r)
	if r.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" {
		t.Errorf("abs answer lacks CORP: %v", r.Header())
	}
	if a.Mode != "abs" || a.Dir != slash {
		t.Errorf("abs answer mode %q dir %q, want abs %q", a.Mode, a.Dir, slash)
	}
	want := []fileItem{{path("sub"), true}, {path("alpha.txt"), false}, {path("beta.md"), false}}
	if symlinks {
		want = []fileItem{{path("lnk"), true}, {path("sub"), true}, {path("alpha.txt"), false}, {path("beta.md"), false}, {path("broken"), false}}
	}
	if fmt.Sprint(a.Items) != fmt.Sprint(want) {
		t.Errorf("abs listing = %v, want %v", a.Items, want)
	}

	if a = decodeFiles(t, filesLocal(f, slash+"/al")); fmt.Sprint(a.Items) != fmt.Sprint([]fileItem{{path("alpha.txt"), false}}) {
		t.Errorf("abs basename filter = %v", a.Items)
	}
	got := itemSet(decodeFiles(t, filesLocal(f, slash+"/.")).Items)
	if d, ok := got[path(".config")]; !ok || !d {
		t.Errorf("dot query lacks .config: %v", got)
	}
	if _, ok := got[path(".hidden")]; !ok {
		t.Errorf("dot query lacks .hidden: %v", got)
	}
	// A native-separator spelling and an uncleaned one list the same
	// directory.
	if a = decodeFiles(t, filesLocal(f, filepath.Join(outside, "sub", "..")+string(filepath.Separator))); a.Dir != slash {
		t.Errorf("uncleaned directory answered dir %q, want %q", a.Dir, slash)
	}

	for _, q := range []string{slash + "/missing/", slash + "/alpha.txt/", slash + "/missing/x"} {
		if r := filesLocal(f, q); r.Code != http.StatusNotFound {
			t.Errorf("abs %q = %d %q, want 404", q, r.Code, r.Body.String())
		}
	}
}

// TestFilesAbsModeRefusals pins abs mode's access rule: a LAN viewer with
// the token, a loopback peer addressed by a LAN Host, and a request another
// site started (same-site or cross-site) get a 404 with no listing, while
// root mode still answers the LAN token holder. A UNC, device, or
// NT-namespace spelling is refused for a loopback viewer too.
func TestFilesAbsModeRefusals(t *testing.T) {
	f := startPageFixture(t, lanMode)
	token := readLANToken(t, f.dir)
	port := strconv.Itoa(f.d.Endpoint().Port)
	local, lanHost := "127.0.0.1:"+port, "192.168.7.20:"+port
	const peer, lanPeer = "127.0.0.1:50000", "192.168.7.30:50000"
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "listedname.txt"), "x")
	writeFile(t, filepath.Join(f.project, "inroot.txt"), "x")
	q := filepath.ToSlash(outside) + "/"

	refused := map[string]*httptest.ResponseRecorder{
		"LAN peer with the token":        filesAs(f.d, lanPeer, lanHost, token, "", q),
		"loopback peer, LAN Host, token": filesAs(f.d, peer, lanHost, token, "", q),
		"cross-site fetch":               filesAs(f.d, peer, local, "", "cross-site", q),
		"same-site fetch":                filesAs(f.d, peer, local, "", "same-site", q),
	}
	for _, unc := range []string{`\\server\share\`, "//server/share/", `\\?\C:\`, "//?/C:/", `\\.\x\`, `\??\UNC\s\sh\`, "/??/UNC/s/sh/", `\\server\s`} {
		refused[unc] = filesAs(f.d, peer, local, "", "", unc)
	}
	// The real directory spelled with a leading double slash: cleaning it
	// would list it, so only the refusal on its spelling keeps it out.
	refused["double-slash spelling of a real directory"] = filesAs(f.d, peer, local, "", "", "/"+q)
	for name, r := range refused {
		if r.Code != http.StatusNotFound || strings.Contains(r.Body.String(), "listedname") || strings.Contains(r.Body.String(), "items") {
			t.Errorf("%s = %d %q, want 404", name, r.Code, r.Body.String())
		}
	}

	a := decodeFiles(t, filesAs(f.d, lanPeer, lanHost, token, "cross-site", "inroot"))
	if a.Mode != "root" || len(a.Items) != 1 || a.Items[0].Path != "inroot.txt" {
		t.Errorf("root mode to a LAN token holder = %+v", a)
	}
}

// TestFilesRouteRefusesUnknownSessionsAndBadQueries pins the plain 404s for
// an unknown session, an over-long query, and a NUL in the query, and the
// 405 for a write method.
func TestFilesRouteRefusesUnknownSessionsAndBadQueries(t *testing.T) {
	f := startPageFixture(t)
	local := "127.0.0.1:" + strconv.Itoa(f.d.Endpoint().Port)
	serveAs := func(method, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		req.RemoteAddr, req.Host = "127.0.0.1:50000", local
		rec := httptest.NewRecorder()
		f.d.Handler().ServeHTTP(rec, req)
		return rec
	}
	for name, target := range map[string]string{
		"unknown session": "/s/nosuchsession/files?q=x",
		"long query":      "/s/livesession/files?q=" + strings.Repeat("a", filesQueryMax+1),
		"NUL in query":    "/s/livesession/files?q=a%00b",
	} {
		if r := serveAs(http.MethodGet, target); r.Code != http.StatusNotFound {
			t.Errorf("%s = %d %q, want 404", name, r.Code, r.Body.String())
		}
	}
	if r := serveAs(http.MethodGet, "/s/livesession/files?q="+strings.Repeat("a", filesQueryMax)); r.Code != http.StatusOK {
		t.Errorf("a query at the cap = %d, want 200", r.Code)
	}
	if r := serveAs(http.MethodPost, "/s/livesession/files?q=x"); r.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d, want 405", r.Code)
	}
}

// TestDeviceElementRefusesWindowsReservedNames pins the abs-mode check that
// keeps a Windows reserved device name in any element of the listed
// directory from reaching the filesystem; elsewhere those names are
// ordinary.
func TestDeviceElementRefusesWindowsReservedNames(t *testing.T) {
	if runtime.GOOS != "windows" {
		if deviceElement("/tmp/NUL") {
			t.Error("deviceElement refused an ordinary Unix name")
		}
		return
	}
	for path, want := range map[string]bool{
		`C:\`: false, `C:\Users\me`: false, `C:\COM1`: true, `C:\x\NUL`: true, `C:\x\con.txt`: true, `C:\x\nullish`: false,
	} {
		if got := deviceElement(path); got != want {
			t.Errorf("deviceElement(%q) = %v, want %v", path, got, want)
		}
	}
}
