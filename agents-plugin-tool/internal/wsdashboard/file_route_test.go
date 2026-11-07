package wsdashboard

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

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
// .html file is plain text under that CSP, so it never runs in the daemon's
// origin; a markdown file renders in the escaped mode under a CSP that runs
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
		"[sibling](b.md) [up](../top.md) [abs](/etc/hosts) [web](https://example.com/) ![pic](pic.png)\n\n"+
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
	code, h, body = fileView(t, f, "page.html")
	if code != http.StatusOK || body != "<script>alert(1)</script><b>x</b>" || h.Get("Content-Type") != "text/plain; charset=utf-8" || h.Get("Content-Security-Policy") != imageContentSecurityPolicy {
		t.Errorf("html = %d %q %v", code, body, h)
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
	for _, bad := range []string{"<script", "<b>bold", `class="mermaid"`, "file:"} {
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
// root-relative path, and its absolute path; a path outside it gets its
// absolute path alone, for the copy-only menu; no root gives no action.
func TestFileActionsMapsFileTargets(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "elsewhere", "x.md")
	actions := FileActions(root, "rid")
	for in, want := range map[string]wswhiteboard.FileAction{
		"docs/a b.md": {Href: "/s/rid/file?path=docs%2Fa+b.md", Rel: "docs/a b.md", Abs: filepath.Join(root, "docs", "a b.md")},
		"./x/../y.go": {Href: "/s/rid/file?path=y.go", Rel: "y.go", Abs: filepath.Join(root, "y.go")},
		filepath.ToSlash(filepath.Join(root, "z.txt")): {Href: "/s/rid/file?path=z.txt", Rel: "z.txt", Abs: filepath.Join(root, "z.txt")},
		"../elsewhere/x.md":                            {Abs: outside},
		filepath.ToSlash(outside):                      {Abs: outside},
		"":                                             {},
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
