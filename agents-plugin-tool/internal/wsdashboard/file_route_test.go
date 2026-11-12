package wsdashboard

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// fileView fetches the file view of path in the fixture's live session.
func fileView(t *testing.T, f *pageFixture, path string) (int, http.Header, string) {
	t.Helper()
	resp, err := testClient().Get(f.base + "/s/livesession/file?path=" + url.QueryEscape(path))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(raw)
}

// TestFileViewServesOnlyInsideTheProjectRoot pins the file view's
// containment: a file under the session's project root is served, by
// relative or absolute path; an absolute path outside it, a ".." escape, a
// symlink (even one that stays inside the root), a path through a symlinked
// directory that leads outside, a directory, and a missing file all answer
// the same 404 without the file's bytes.
func TestFileViewServesOnlyInsideTheProjectRoot(t *testing.T) {
	f := startPageFixture(t)
	outside := filepath.Join(t.TempDir(), "outside")
	writeFile(t, filepath.Join(f.project, "notes.txt"), "TEXT-INSIDE")
	writeFile(t, filepath.Join(f.project, "docs", "deep", "c.go"), "package c // GO-INSIDE")
	writeFile(t, filepath.Join(f.project, ".env"), "TOKEN=DOTFILE-INSIDE")
	writeFile(t, filepath.Join(outside, "secret.txt"), "TEXT-OUTSIDE")
	if err := os.MkdirAll(filepath.Join(f.project, "dir.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinks := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(f.project, "escape.txt")) == nil &&
		os.Symlink(outside, filepath.Join(f.project, "outlink")) == nil &&
		os.Symlink(filepath.Join(f.project, "notes.txt"), filepath.Join(f.project, "alias.txt")) == nil

	served := map[string]string{
		"notes.txt":         "TEXT-INSIDE",
		"./notes.txt":       "TEXT-INSIDE",
		"docs/../notes.txt": "TEXT-INSIDE",
		"docs/deep/c.go":    "package c // GO-INSIDE",
		".env":              "TOKEN=DOTFILE-INSIDE",
		filepath.ToSlash(filepath.Join(f.project, "notes.txt")): "TEXT-INSIDE",
		filepath.Join(f.project, "docs", "deep", "c.go"):        "package c // GO-INSIDE",
	}
	for path, want := range served {
		code, h, body := fileView(t, f, path)
		if code != http.StatusOK || body != want {
			t.Errorf("file %q = %d %q, want 200 %q", path, code, body, want)
			continue
		}
		if h.Get("Content-Type") != "text/plain; charset=utf-8" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Content-Security-Policy") != imageContentSecurityPolicy {
			t.Errorf("file %q headers %v", path, h)
		}
		assertNoCORS(t, h)
	}

	refused := []string{
		"",
		filepath.Join(outside, "secret.txt"),
		filepath.ToSlash(filepath.Join(outside, "secret.txt")),
		"../" + filepath.Base(outside) + "/secret.txt",
		"docs/../../outside/secret.txt",
		"dir.txt",
		"missing.txt",
		".",
		"notes.txt\x00.txt",
	}
	if rel, err := filepath.Rel(f.project, filepath.Join(outside, "secret.txt")); err == nil {
		refused = append(refused, rel, filepath.ToSlash(rel))
	}
	if symlinks {
		refused = append(refused, "escape.txt", "outlink/secret.txt", "alias.txt")
	}
	for _, path := range refused {
		code, _, body := fileView(t, f, path)
		if code != http.StatusNotFound {
			t.Errorf("file %q = %d, want 404", path, code)
		}
		if strings.Contains(body, "OUTSIDE") || strings.Contains(body, "TEXT-INSIDE") {
			t.Errorf("file %q leaked %q", path, body)
		}
	}
	if code, _ := get(t, f.base+"/s/nosuchsession/file?path=notes.txt"); code != http.StatusNotFound {
		t.Errorf("file of an unknown session = %d, want 404", code)
	}
	if code, _, _ := postJSON(t, f.base+"/s/livesession/file?path=notes.txt", f.base, ""); code != http.StatusMethodNotAllowed {
		t.Errorf("POST file = %d, want 405", code)
	}
}

// TestFileViewKinds pins what the file view shows per kind: an image is
// served inline under the image route's sandboxing CSP (an SVG included); an
// .html file redirects to the page route (TestFileViewRendersHTMLPage), so
// it never runs in the daemon's origin; a markdown file renders in the escaped mode under a CSP that runs
// no script, its relative links and images resolved against its own
// directory and a target outside the root left as path text; a binary file
// and an oversized one get the "not viewable" page with the path copies and
// one nonce-admitted script.
func TestFileViewKinds(t *testing.T) {
	f := startPageFixture(t)
	writeFile(t, filepath.Join(f.project, "shot.png"), "PNG-BYTES")
	writeFile(t, filepath.Join(f.project, "draw.svg"), `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	writeFile(t, filepath.Join(f.project, "page.html"), "<script>alert(1)</script><b>x</b>")
	writeFile(t, filepath.Join(f.project, "docs", "guide", "a.md"), "# Guide\n\n<script>alert(1)</script> and <b>bold</b>\n\n"+
		"[sibling](b.md) [up](../top.md) [abs](/etc/hosts) [web](https://example.com/) ![pic](pic.png) ![outpic](/elsewhere/out.png)\n\n"+
		"```mermaid\ngraph TD; A-->B\n```\n")
	writeFile(t, filepath.Join(f.project, "blob.bin"), "BIN\x00\x01\x02")
	writeFile(t, filepath.Join(f.project, "huge.txt"), strings.Repeat("x", fileViewMax+1))

	code, h, body := fileView(t, f, "shot.png")
	if code != http.StatusOK || body != "PNG-BYTES" || h.Get("Content-Type") != "image/png" || h.Get("Content-Security-Policy") != imageContentSecurityPolicy {
		t.Errorf("png = %d %q %v", code, body, h)
	}
	code, h, _ = fileView(t, f, "draw.svg")
	if code != http.StatusOK || h.Get("Content-Type") != "image/svg+xml" || h.Get("Content-Security-Policy") != imageContentSecurityPolicy || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("svg = %d %v", code, h)
	}
	local := viewer{name: "local", peer: "127.0.0.1:50000", host: "127.0.0.1:" + strconv.Itoa(f.d.Endpoint().Port)}
	if rec := serve(f.d, local, http.MethodGet, "/s/livesession/file?path=page.html", ""); rec.Code != http.StatusFound ||
		!strings.HasPrefix(rec.Header().Get("Location"), "/s/livesession/fs/view/") || strings.Contains(rec.Body.String(), "<script>") {
		t.Errorf("html = %d %v %q", rec.Code, rec.Header(), rec.Body.String())
	}

	code, h, body = fileView(t, f, "docs/guide/a.md")
	if code != http.StatusOK || h.Get("Content-Type") != "text/html; charset=utf-8" || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("md = %d %v", code, h)
	}
	if csp := h.Get("Content-Security-Policy"); csp != fileMarkdownCSP || strings.Contains(csp, "script-src") {
		t.Errorf("md CSP = %q", csp)
	}
	wantAll := []string{
		"<h1>Guide</h1>", "&lt;script&gt;alert(1)&lt;/script&gt;", "&lt;b&gt;bold&lt;/b&gt;",
		`<a href="/s/livesession/file?path=docs%2Fguide%2Fb.md" data-wl="file" data-rel="docs/guide/b.md"`,
		`<a href="/s/livesession/file?path=docs%2Ftop.md" data-wl="file" data-rel="docs/top.md"`,
		`abs <code class="fpath">/etc/hosts</code>`,
		// An image outside the root stays path text: the page runs no script
		// to put the path back if the image failed to load.
		`<span class="img-text">outpic <code class="fpath">/elsewhere/out.png</code></span>`,
		`<a href="https://example.com/" target="_blank" rel="noopener noreferrer" data-wl="url">web</a>`,
		`<img src="/s/livesession/image?path=docs%2Fguide%2Fpic.png" alt="pic">`,
		`<code class="language-mermaid">`,
		`<code class="fpath">docs/guide/a.md</code>`,
	}
	for _, want := range wantAll {
		if !strings.Contains(body, want) {
			t.Errorf("md page lacks %q:\n%s", want, body)
		}
	}
	for _, bad := range []string{"<script", "<b>bold", `class="mermaid"`, "file:", "data-out"} {
		if strings.Contains(body, bad) {
			t.Errorf("md page carries %q", bad)
		}
	}

	nonce := regexp.MustCompile(`<script nonce="([0-9a-f]{32})">`)
	for path, reason := range map[string]string{"blob.bin": "it is not text", "huge.txt": "it is larger than 2 MiB"} {
		code, h, body := fileView(t, f, path)
		if code != http.StatusOK || h.Get("Content-Type") != "text/html; charset=utf-8" {
			t.Errorf("%s = %d %v", path, code, h)
			continue
		}
		for _, want := range []string{"not viewable here: " + reason, `data-copy="` + path + `"`, `data-copy="` + filepath.Join(f.project, path) + `"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s page lacks %q", path, want)
			}
		}
		if strings.Contains(body, "BIN") || strings.Contains(body, "xxxxxxxx") {
			t.Errorf("%s page carries the file's bytes", path)
		}
		m := nonce.FindStringSubmatch(body)
		if m == nil || strings.Count(body, "<script") != 1 {
			t.Fatalf("%s page scripts: %s", path, body)
		}
		if csp := h.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "script-src 'nonce-"+m[1]+"'") {
			t.Errorf("%s CSP = %q", path, csp)
		}
	}
	// Each answer mints its own nonce.
	_, h1, _ := fileView(t, f, "blob.bin")
	_, h2, _ := fileView(t, f, "blob.bin")
	if h1.Get("Content-Security-Policy") == h2.Get("Content-Security-Policy") {
		t.Error("the not-viewable page reuses its nonce")
	}
}

// TestFileActionsMapsFileTargets pins the served page's link actions: a
// path inside the root, relative or absolute, gets the file view URL, its
// root-relative path, and its absolute path, marked an image when it has an
// image extension; a path outside it gets its absolute path alone, for the
// copy-only menu, an image included (OutsideImage widens that); no root
// gives no action.
func TestFileActionsMapsFileTargets(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "elsewhere", "x.md")
	actions := FileActions(root, "rid")
	for in, want := range map[string]wswhiteboard.FileAction{
		"docs/a b.md": {Href: "/s/rid/file?path=docs%2Fa+b.md", Rel: "docs/a b.md", Abs: filepath.Join(root, "docs", "a b.md")},
		"./x/../y.go": {Href: "/s/rid/file?path=y.go", Rel: "y.go", Abs: filepath.Join(root, "y.go")},
		filepath.ToSlash(filepath.Join(root, "z.txt")): {Href: "/s/rid/file?path=z.txt", Rel: "z.txt", Abs: filepath.Join(root, "z.txt")},
		"shots/a.PNG": {Href: "/s/rid/file?path=shots%2Fa.PNG", Rel: "shots/a.PNG", Abs: filepath.Join(root, "shots", "a.PNG"), Image: true},
		filepath.ToSlash(filepath.Join(filepath.Dir(root), "o.png")): {Abs: filepath.Join(filepath.Dir(root), "o.png")},
		"../elsewhere/x.md":       {Abs: outside},
		filepath.ToSlash(outside): {Abs: outside},
		"":                        {},
	} {
		if got := actions(in); got != want {
			t.Errorf("FileActions(%q) = %+v, want %+v", in, got, want)
		}
	}
	if got := FileActions(root, "Not/A-Reply-ID")("a.md"); got.Href != "" || got.Abs != filepath.Join(root, "a.md") {
		t.Errorf("FileActions with an invalid reply-id = %+v", got)
	}
	if got := FileActions("", "rid")("a.md"); got != (wswhiteboard.FileAction{}) {
		t.Errorf("FileActions without a root = %+v", got)
	}
}

// pageRedirect asks the file view for path as v and returns the page route
// URL it redirects to.
func pageRedirect(t *testing.T, f *pageFixture, v viewer, path string) string {
	t.Helper()
	rec := serve(f.d, v, http.MethodGet, "/s/livesession/file?path="+url.QueryEscape(path), "")
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusFound || !strings.HasPrefix(loc, "/s/livesession/fs/view/") || !strings.HasSuffix(loc, "/"+path) || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("file view of %s as %s = %d %v", path, v.name, rec.Code, rec.Header())
	}
	return loc
}

// TestFileViewRendersHTMLPage pins the file view of an HTML file: it
// redirects, for a loopback viewer and a LAN token holder alike, to the
// page route under a fresh capability, which serves the document as a
// sandboxed page (scripts, an opaque origin, no fetch, no form, no other
// origin) and its relative assets by their own types to anyone holding the
// capability, a LAN peer without the token included, since the page's
// opaque origin sends no cookie. An image keeps the image route's policy;
// an HTML file over fileViewMax or not text, and any path the file view
// refuses, answer 404; a file the capability route refuses (a dot-named
// segment) stays plain text in the file view; a reload after the
// capability expired goes back through the file view.
func TestFileViewRendersHTMLPage(t *testing.T) {
	clock := &capClock{now: time.Unix(1_800_000_000, 0)}
	f := startPageFixture(t, lanMode)
	f.d.fsNow = clock.read
	local, lan := fsViewers(t, f)
	outside, symlinks := escapeTree(t, f)
	port := strconv.Itoa(f.d.Endpoint().Port)
	bare := viewer{name: "LAN peer, no token", peer: "192.168.7.31:50000", host: "192.168.7.20:" + port}
	const doc = `<link rel="stylesheet" href="style.css"><script src="app.js"></script><img src="img/pic.png"><iframe src="sub.html"></iframe>`
	writeFile(t, filepath.Join(f.project, "docs", "guide", "page.html"), doc)
	writeFile(t, filepath.Join(f.project, "docs", "guide", "style.css"), "body{color:red}")
	writeFile(t, filepath.Join(f.project, "docs", "guide", "app.js"), "document.title='x'")
	writeFile(t, filepath.Join(f.project, "docs", "guide", "img", "pic.png"), "PNG-PIC")
	writeFile(t, filepath.Join(f.project, "docs", "guide", "art.svg"), "<svg><script>1</script></svg>")
	writeFile(t, filepath.Join(f.project, "docs", "guide", "font.woff2"), "WOFF2")
	writeFile(t, filepath.Join(f.project, "docs", "guide", "sub.htm"), "<p>SUB</p>")
	writeFile(t, filepath.Join(f.project, "docs", "huge.html"), strings.Repeat("x", fileViewMax+1))
	writeFile(t, filepath.Join(f.project, "docs", "nul.html"), "<p>\x00</p>")
	writeFile(t, filepath.Join(f.project, ".site", "page.html"), "<p>DOT-INSIDE</p>")

	page := pageRedirect(t, f, lan, "docs/guide/page.html")
	if again := pageRedirect(t, f, local, "docs/guide/page.html"); again == "" {
		t.Fatal("no redirect for a loopback viewer")
	}
	prefix := strings.TrimSuffix(page, "docs/guide/page.html")
	for _, v := range []viewer{bare, local, lan} {
		rec := serve(f.d, v, http.MethodGet, page, "")
		h := rec.Header()
		if rec.Code != http.StatusOK || rec.Body.String() != doc || h.Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatalf("%s: page = %d %q %v", v.name, rec.Code, rec.Body.String(), h)
		}
		csp := h.Get("Content-Security-Policy")
		origin := "http://" + v.host + prefix
		if csp != fsCapabilityCSP(origin) {
			t.Errorf("%s: page policy = %q", v.name, csp)
		}
		for _, want := range []string{
			"sandbox allow-scripts;", "default-src " + origin + ";", "script-src " + origin + " 'unsafe-inline';",
			"style-src " + origin + " 'unsafe-inline';", "img-src " + origin + " data: blob:;", "font-src " + origin + " data:;",
			"connect-src 'none'", "object-src 'none'", "base-uri 'none'", "form-action 'none'",
		} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: page policy lacks %q: %s", v.name, want, csp)
			}
		}
		for _, bad := range []string{"allow-same-origin", "allow-top-navigation", "allow-forms", "allow-popups", "'self'", "*", "'unsafe-eval'", "frame-ancestors"} {
			if strings.Contains(csp, bad) {
				t.Errorf("%s: page policy carries %q: %s", v.name, bad, csp)
			}
		}
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") != "no-store" || h.Get("Access-Control-Allow-Origin") != "" || len(rec.Result().Cookies()) != 0 {
			t.Errorf("%s: page headers = %v", v.name, h)
		}
	}
	// The relative assets resolve under the prefix and load by type.
	viewPolicy := fsCapabilityCSP("http://" + bare.host + prefix)
	for path, want := range map[string][3]string{
		"docs/guide/style.css":   {"text/css; charset=utf-8", "body{color:red}", viewPolicy},
		"docs/guide/app.js":      {"text/javascript; charset=utf-8", "document.title='x'", viewPolicy},
		"docs/guide/img/pic.png": {"image/png", "PNG-PIC", imageContentSecurityPolicy},
		"docs/guide/art.svg":     {"image/svg+xml", "<svg><script>1</script></svg>", imageContentSecurityPolicy},
		"docs/guide/font.woff2":  {"font/woff2", "WOFF2", viewPolicy},
		"docs/guide/sub.htm":     {"text/html; charset=utf-8", "<p>SUB</p>", viewPolicy},
		"notes.txt":              {"text/plain; charset=utf-8", "TEXT-INSIDE", viewPolicy},
	} {
		rec := serve(f.d, bare, http.MethodGet, prefix+path, "")
		h := rec.Header()
		if rec.Code != http.StatusOK || h.Get("Content-Type") != want[0] || rec.Body.String() != want[1] ||
			h.Get("Content-Security-Policy") != want[2] || h.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("asset %s = %d %q %q %q", path, rec.Code, h.Get("Content-Type"), rec.Body.String(), h.Get("Content-Security-Policy"))
		}
	}
	// It reads nothing outside the root, nothing through a symlink, no HTML
	// the file view would not show as text, and no dot-named path.
	refused := []string{
		prefix + "../../../../etc/hosts", prefix + "docs/../../secret.txt", prefix + "docs%2F..%2F..%2Fsecret.txt",
		prefix + `docs\..\..\secret.txt`, prefix + "notes.txt%00.html", prefix + "docs", prefix + "missing.html", prefix,
		prefix + "/" + filepath.ToSlash(filepath.Join(outside, "secret.txt")), prefix + "docs/huge.html", prefix + "docs/nul.html",
		prefix + ".site/page.html",
	}
	if symlinks {
		refused = append(refused, prefix+"escape.txt", prefix+"alias.txt", prefix+"outlink/secret.txt", prefix+"outlink/sub/page.html")
	}
	for _, target := range refused {
		rec := serve(f.d, local, http.MethodGet, target, "")
		if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "OUTSIDE") || strings.Contains(rec.Body.String(), "INSIDE") {
			t.Errorf("page route read of %s = %d %s, want a refusal", target, rec.Code, rec.Body.String())
		}
	}
	if symlinks {
		if rec := serve(f.d, local, http.MethodGet, "/s/livesession/file?path=outlink/sub/page.html", ""); rec.Code != http.StatusNotFound {
			t.Errorf("file view of an HTML file through a symlinked folder = %d, want 404", rec.Code)
		}
	}
	if rec := serve(f.d, bare, http.MethodPost, page, ""); !refusedByGuard(rec) {
		t.Errorf("POST to the page route without the token = %d, want the guard's 401", rec.Code)
	}
	// A capability verifies on its own route only: a preview's capability
	// opens no page, so it cannot read what only the page route serves
	// (a file git ignores), and the page's opens no preview.
	capability := strings.TrimSuffix(strings.TrimPrefix(prefix, "/s/livesession/fs/view/"), "/")
	if rec := serve(f.d, bare, http.MethodGet, "/s/livesession/fs/cap/"+capability+"/docs/guide/page.html", ""); rec.Code != http.StatusNotFound {
		t.Errorf("the page's capability on the capability route = %d, want 404", rec.Code)
	}
	if rec := serve(f.d, bare, http.MethodGet, "/s/livesession/fs/view/"+f.d.mintFSCapability("livesession", f.project)+"/docs/guide/page.html", ""); rec.Code != http.StatusNotFound {
		t.Errorf("a preview capability on the page route = %d, want 404", rec.Code)
	}
	// An HTML file the page route refuses stays what the file view showed:
	// the not-viewable page, or plain text.
	for path, want := range map[string]string{"docs/huge.html": "text/html; charset=utf-8", "docs/nul.html": "text/html; charset=utf-8", ".site/page.html": "text/plain; charset=utf-8"} {
		rec := serve(f.d, local, http.MethodGet, "/s/livesession/file?path="+url.QueryEscape(path), "")
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != want || rec.Header().Get("Location") != "" {
			t.Errorf("file view of %s = %d %v", path, rec.Code, rec.Header())
		}
	}

	// After the expiry an asset answers 404, and a document navigation goes
	// back through the file view, which mints a fresh capability.
	clock.advance(fsCapabilityTTL)
	nav := func(target, dest string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.RemoteAddr, req.Host = bare.peer, bare.host
		if dest != "" {
			req.Header.Set("Sec-Fetch-Dest", dest)
		}
		rec := httptest.NewRecorder()
		f.d.Handler().ServeHTTP(rec, req)
		return rec
	}
	if rec := nav(prefix+"docs/guide/style.css", "style"); rec.Code != http.StatusNotFound {
		t.Errorf("an expired asset = %d, want 404", rec.Code)
	}
	if rec := nav(page, "iframe"); rec.Code != http.StatusNotFound {
		t.Errorf("an expired framed document = %d, want 404", rec.Code)
	}
	if rec := nav(page, "document"); rec.Code != http.StatusFound || rec.Header().Get("Location") != "/s/livesession/file?path=docs%2Fguide%2Fpage.html" {
		t.Errorf("an expired document navigation = %d %v", rec.Code, rec.Header())
	}
	if fresh := pageRedirect(t, f, lan, "docs/guide/page.html"); fresh == page {
		t.Error("the file view after the expiry redirects to the expired capability")
	} else if rec := serve(f.d, bare, http.MethodGet, fresh, ""); rec.Code != http.StatusOK {
		t.Errorf("a fresh page = %d, want 200", rec.Code)
	}
}

// TestFileViewPageServesAnIgnoredFolder pins that the page route serves a
// document and its assets in a folder git ignores whole, where a paged
// document's working folder usually lives (its .gitignore holds "*"),
// while the preview's capability route keeps refusing them: the page
// cannot fetch, so the ignored set is not the secret store it is for a
// framed preview. A dot-named path stays refused.
func TestFileViewPageServesAnIgnoredFolder(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	f := startPageFixture(t)
	local := viewer{name: "local", peer: "127.0.0.1:50000", host: "127.0.0.1:" + strconv.Itoa(f.d.Endpoint().Port)}
	gitRun(t, f.project, "init", "-q")
	writeFile(t, filepath.Join(f.project, "paged", "doc", ".gitignore"), "*\n")
	writeFile(t, filepath.Join(f.project, "paged", "doc", "document.html"), `<link rel="stylesheet" href="style.css">IGNORED-DOC`)
	writeFile(t, filepath.Join(f.project, "paged", "doc", "style.css"), "IGNORED-CSS")

	page := pageRedirect(t, f, local, "paged/doc/document.html")
	prefix := strings.TrimSuffix(page, "paged/doc/document.html")
	for path, want := range map[string]int{"paged/doc/document.html": http.StatusOK, "paged/doc/style.css": http.StatusOK, "paged/doc/.gitignore": http.StatusNotFound} {
		if rec := serve(f.d, local, http.MethodGet, prefix+path, ""); rec.Code != want {
			t.Errorf("page route read of %s = %d, want %d", path, rec.Code, want)
		}
	}
	if rec := serve(f.d, local, http.MethodGet, f.d.fsCapabilityURL("livesession", f.project, "paged/doc/style.css"), ""); rec.Code != http.StatusNotFound {
		t.Errorf("capability route read of an ignored file = %d, want 404", rec.Code)
	}
}
