package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fsViewers are the two viewers the file browser tests ask as, in LAN mode:
// a browser on this machine and a LAN token holder.
func fsViewers(t *testing.T, f *pageFixture) (local, lan viewer) {
	t.Helper()
	port := strconv.Itoa(f.d.Endpoint().Port)
	return viewer{name: "local", peer: "127.0.0.1:50000", host: "127.0.0.1:" + port},
		viewer{name: "lan", peer: "192.168.7.30:50000", host: "192.168.7.20:" + port, cookie: readLANToken(t, f.dir)}
}

func fsList(t *testing.T, f *pageFixture, v viewer, query string) (int, fsListAnswer) {
	t.Helper()
	rec := serve(f.d, v, http.MethodGet, "/s/livesession/fs/list?"+query, "")
	var a fsListAnswer
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
			t.Fatalf("list %q: %v in %s", query, err, rec.Body.String())
		}
	}
	return rec.Code, a
}

func fsPreview(t *testing.T, f *pageFixture, v viewer, query string) (int, fsPreviewAnswer) {
	t.Helper()
	rec := serve(f.d, v, http.MethodGet, "/s/livesession/fs/preview?"+query, "")
	var a fsPreviewAnswer
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
			t.Fatalf("preview %q: %v in %s", query, err, rec.Body.String())
		}
	}
	return rec.Code, a
}

func entryNames(a fsListAnswer) []string {
	var names []string
	for _, e := range a.Entries {
		n := e.Name
		if e.Dir {
			n += "/"
		}
		if e.Link {
			n += "@"
		}
		names = append(names, n)
	}
	return names
}

// escapeTree builds the fixture the containment tests share: files inside
// the root, a folder outside it, and, when the OS allows, a symlinked file
// that leaves the root, a symlinked folder that leaves it, and a symlinked
// file that stays inside. symlinks reports whether the links exist.
func escapeTree(t *testing.T, f *pageFixture) (outside string, symlinks bool) {
	t.Helper()
	outside = filepath.Join(t.TempDir(), "outside")
	writeFile(t, filepath.Join(f.project, "notes.txt"), "TEXT-INSIDE")
	writeFile(t, filepath.Join(f.project, "docs", "deep", "c.go"), "package c // GO-INSIDE")
	writeFile(t, filepath.Join(f.project, "docs", "page.html"), "<h1>HTML-INSIDE</h1>")
	writeFile(t, filepath.Join(f.project, "shot.png"), "PNG-INSIDE")
	writeFile(t, filepath.Join(outside, "secret.txt"), "TEXT-OUTSIDE")
	writeFile(t, filepath.Join(outside, "secret.png"), "PNG-OUTSIDE")
	writeFile(t, filepath.Join(outside, "sub", "page.html"), "<h1>HTML-OUTSIDE</h1>")
	symlinks = os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(f.project, "escape.txt")) == nil &&
		os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(f.project, "escape.png")) == nil &&
		os.Symlink(outside, filepath.Join(f.project, "outlink")) == nil &&
		os.Symlink(filepath.Join(f.project, "notes.txt"), filepath.Join(f.project, "alias.txt")) == nil
	return outside, symlinks
}

// TestFSListStaysInsideTheRoot pins the listing route's containment: the
// root and a folder under it are listed, directories first, a symlink
// listed as a link and never as a folder; a ".." escape, an absolute path
// outside the root, a symlinked folder that leaves it, a file, and a
// missing folder all answer 404.
func TestFSListStaysInsideTheRoot(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, lan := fsViewers(t, f)
	outside, symlinks := escapeTree(t, f)

	for _, v := range []viewer{local, lan} {
		code, a := fsList(t, f, v, "")
		if code != http.StatusOK || !a.Session || a.Dir != "" || a.Root != filepath.ToSlash(f.project) || a.Name != filepath.Base(f.project) {
			t.Fatalf("%s: root listing = %d %+v", v.name, code, a)
		}
		want := []string{"docs/", "notes.txt", "shot.png"}
		if symlinks {
			want = []string{"docs/", "alias.txt@", "escape.png@", "escape.txt@", "notes.txt", "outlink@", "shot.png"}
		}
		if got := entryNames(a); strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("%s: root entries = %v, want %v", v.name, got, want)
		}
		code, a = fsList(t, f, v, "dir="+url.QueryEscape("docs"))
		if got := entryNames(a); code != http.StatusOK || a.Dir != "docs" || strings.Join(got, " ") != "deep/ page.html" ||
			a.Abs != filepath.Join(f.project, "docs") || a.Sep != string(filepath.Separator) {
			t.Errorf("%s: docs listing = %d dir %q abs %q sep %q %v", v.name, code, a.Dir, a.Abs, a.Sep, got)
		}
		if code, a = fsList(t, f, v, "dir="+url.QueryEscape(filepath.Join(f.project, "docs", "deep"))); code != http.StatusOK || a.Dir != "docs/deep" {
			t.Errorf("%s: absolute in-root dir = %d dir %q", v.name, code, a.Dir)
		}
		refused := []string{"..", "docs/../..", "../" + filepath.Base(outside), outside, "notes.txt", "missing", "docs/missing"}
		if symlinks {
			refused = append(refused, "outlink", "outlink/sub")
		}
		for _, dir := range refused {
			rec := serve(f.d, v, http.MethodGet, "/s/livesession/fs/list?dir="+url.QueryEscape(dir), "")
			if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "secret") {
				t.Errorf("%s: list dir %q = %d %s, want 404", v.name, dir, rec.Code, rec.Body.String())
			}
		}
	}
	if rec := serve(f.d, local, http.MethodGet, "/s/nosuchsession/fs/list", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown session listing = %d, want 404", rec.Code)
	}
}

// TestFSPreviewAndRawStayInsideTheRoot pins the containment of the preview
// and raw routes: a traversal, an outside path, a symlink (even one that
// stays inside the root), a path through a symlinked folder, a folder, and
// a missing file all answer 404 without the file's bytes, for a browser on
// this machine and a LAN token holder alike.
func TestFSPreviewAndRawStayInsideTheRoot(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, lan := fsViewers(t, f)
	outside, symlinks := escapeTree(t, f)

	refused := []string{"../" + filepath.Base(outside) + "/secret.txt", "docs/../../x", filepath.Join(outside, "secret.txt"),
		filepath.Join(outside, "secret.png"), "docs", "missing.txt", "missing.png", ""}
	if symlinks {
		refused = append(refused, "escape.txt", "escape.png", "alias.txt", "outlink/secret.txt", "outlink/secret.png", "outlink/sub/page.html")
	}
	for _, v := range []viewer{local, lan} {
		for _, path := range refused {
			for _, route := range []string{"preview", "raw"} {
				rec := serve(f.d, v, http.MethodGet, "/s/livesession/fs/"+route+"?path="+url.QueryEscape(path), "")
				if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "OUTSIDE") || strings.Contains(rec.Body.String(), "INSIDE") {
					t.Errorf("%s: %s %q = %d %s, want 404", v.name, route, path, rec.Code, rec.Body.String())
				}
			}
		}
		if code, a := fsPreview(t, f, v, "path=notes.txt"); code != http.StatusOK || a.Kind != "text" || a.Text != "TEXT-INSIDE" || a.Rel != "notes.txt" || a.Abs != filepath.Join(f.project, "notes.txt") {
			t.Errorf("%s: preview notes.txt = %d %+v", v.name, code, a)
		}
		if rec := serve(f.d, v, http.MethodGet, "/s/livesession/fs/raw?path=shot.png", ""); rec.Code != http.StatusOK || rec.Body.String() != "PNG-INSIDE" {
			t.Errorf("%s: raw shot.png = %d %q", v.name, rec.Code, rec.Body.String())
		}
		// The raw route serves images and video only: a text file is not its.
		if rec := serve(f.d, v, http.MethodGet, "/s/livesession/fs/raw?path=notes.txt", ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s: raw notes.txt = %d, want 404", v.name, rec.Code)
		}
	}
}

// TestFSPreviewKinds pins what a preview card is told to draw for each
// kind of file.
func TestFSPreviewKinds(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, _ := fsViewers(t, f)
	writeFile(t, filepath.Join(f.project, "a.png"), "png")
	writeFile(t, filepath.Join(f.project, "clip.webm"), "webm")
	writeFile(t, filepath.Join(f.project, "docs", "page.html"), "<script>1</script>")
	writeFile(t, filepath.Join(f.project, "art.svg"), "<svg/>")
	writeFile(t, filepath.Join(f.project, "docs", "readme.md"), "# Title\n\n<b>raw</b> [link](other.md)\n")
	writeFile(t, filepath.Join(f.project, "main.go"), "package main\n")
	writeFile(t, filepath.Join(f.project, "blob.bin"), "a\x00b")
	writeFile(t, filepath.Join(f.project, "big.txt"), strings.Repeat("x", fileViewMax+1))
	writeFile(t, filepath.Join(f.project, "fix.patch"), strings.Join([]string{
		"From 1234 Mon Sep 17 00:00:00 2001",
		"Subject: [PATCH] two files",
		"---",
		" a.txt | 2 +-",
		"diff --git a/a.txt b/a.txt",
		"index 1111111..2222222 100644",
		"--- a/a.txt",
		"+++ b/a.txt",
		"@@ -1,2 +1,2 @@",
		" keep",
		"-old line",
		"+new line",
		"diff --git a/gone.txt b/gone.txt",
		"deleted file mode 100644",
		"--- a/gone.txt",
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-bye",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(f.project, "plain.diff"), "--- old/x.c\t2026-01-01\n+++ new/x.c\t2026-01-02\n@@ -3 +3 @@\n-a\n+b\n--- old/y.c\n+++ new/y.c\n@@ -1 +1 @@\n-c\n+d\n")

	code, a := fsPreview(t, f, local, "path=a.png")
	if code != http.StatusOK || a.Kind != "image" || a.Src != "/s/livesession/fs/raw?path=a.png" {
		t.Errorf("image preview = %d %+v", code, a)
	}
	if code, a = fsPreview(t, f, local, "path=clip.webm"); code != http.StatusOK || a.Kind != "video" || a.Src != "/s/livesession/fs/raw?path=clip.webm" {
		t.Errorf("video preview = %d %+v", code, a)
	}
	for _, path := range []string{"docs/page.html", "art.svg"} {
		code, a = fsPreview(t, f, local, "path="+url.QueryEscape(path))
		if code != http.StatusOK || a.Kind != "frame" || !strings.HasPrefix(a.Frame, "/s/livesession/fs/cap/") || !strings.HasSuffix(a.Frame, "/"+path) ||
			a.TTL != int(fsCapabilityTTL/time.Second) || a.Text != "" || a.Src != "" {
			t.Errorf("frame preview of %s = %d %+v", path, code, a)
		}
	}
	code, a = fsPreview(t, f, local, "path="+url.QueryEscape("docs/readme.md"))
	if code != http.StatusOK || a.Kind != "markdown" || !strings.Contains(a.HTML, "<h1") || strings.Contains(a.HTML, "<b>raw</b>") ||
		!strings.Contains(a.HTML, "&lt;b&gt;raw&lt;/b&gt;") || !strings.Contains(a.HTML, "/s/livesession/file?path=docs%2Fother.md") {
		t.Errorf("markdown preview = %d %+v", code, a)
	}
	if code, a = fsPreview(t, f, local, "path=main.go"); code != http.StatusOK || a.Kind != "text" || a.Text != "package main\n" || a.Source {
		t.Errorf("text preview = %d %+v", code, a)
	}
	if code, a = fsPreview(t, f, local, "path=blob.bin"); code != http.StatusOK || a.Kind != "none" || a.Reason == "" || a.Text != "" {
		t.Errorf("binary preview = %d %+v", code, a)
	}
	if code, a = fsPreview(t, f, local, "path=big.txt"); code != http.StatusOK || a.Kind != "none" || a.Reason == "" || a.Text != "" {
		t.Errorf("oversize preview = %d kind %q reason %q", code, a.Kind, a.Reason)
	}
	code, a = fsPreview(t, f, local, "path=fix.patch")
	if code != http.StatusOK || a.Kind != "diff" || len(a.Diff) != 2 || a.Diff[0].Path != "a.txt" || a.Diff[1].Path != "gone.txt" ||
		len(a.Diff[0].Hunks) != 1 || len(a.Diff[0].Hunks[0].Lines) != 3 || a.Diff[0].Hunks[0].Lines[2].Kind != "+" || a.Diff[0].Hunks[0].Lines[2].Pieces[0].Text == "" {
		t.Errorf("patch preview = %d %+v", code, a)
	}
	code, a = fsPreview(t, f, local, "path=plain.diff")
	if code != http.StatusOK || a.Kind != "diff" || len(a.Diff) != 2 || a.Diff[0].Path != "new/x.c" || a.Diff[1].Path != "new/y.c" || len(a.Diff[1].Hunks) != 1 {
		t.Errorf("plain diff preview = %d %+v", code, a)
	}
}

// TestFSRawStreamsVideoWithRange pins Decision 5: a video is served through
// the raw route with byte ranges, so the player seeks; a ranged request
// answers 206 with exactly the bytes asked for.
func TestFSRawStreamsVideoWithRange(t *testing.T) {
	f := startPageFixture(t)
	body := make([]byte, 4096)
	for i := range body {
		body[i] = byte(i % 251)
	}
	if err := os.WriteFile(filepath.Join(f.project, "clip.mp4"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	for name, ctype := range map[string]string{"clip.mp4": "video/mp4", "c.webm": "video/webm", "c.ogg": "video/ogg"} {
		if name != "clip.mp4" {
			writeFile(t, filepath.Join(f.project, name), "video")
		}
		req := httptest.NewRequest(http.MethodGet, "/s/livesession/fs/raw?path="+name, nil)
		req.Host = "127.0.0.1"
		req.RemoteAddr = "127.0.0.1:50000"
		rec := httptest.NewRecorder()
		f.d.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != ctype || rec.Header().Get("Accept-Ranges") != "bytes" ||
			rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Content-Security-Policy") != imageContentSecurityPolicy {
			t.Errorf("raw %s = %d %v", name, rec.Code, rec.Header())
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/s/livesession/fs/raw?path=clip.mp4", nil)
	req.Host = "127.0.0.1"
	req.RemoteAddr = "127.0.0.1:50000"
	req.Header.Set("Range", "bytes=1000-1999")
	rec := httptest.NewRecorder()
	f.d.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Header().Get("Content-Range") != "bytes 1000-1999/4096" || rec.Body.Len() != 1000 || string(rec.Body.Bytes()) != string(body[1000:2000]) {
		t.Errorf("ranged video = %d Content-Range %q, %d bytes", rec.Code, rec.Header().Get("Content-Range"), rec.Body.Len())
	}
}

// TestFSListMarksIgnoredEntries pins the listing's ignore marks: an entry
// git's rules exclude and the repository's own directory are marked, a
// tracked or merely untracked one is not, and entries inside an ignored
// folder are all marked. A root that is no repository marks nothing and
// says so.
func TestFSListMarksIgnoredEntries(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	f := startPageFixture(t, lanMode)
	local, _ := fsViewers(t, f)
	writeFile(t, filepath.Join(f.project, "plain.txt"), "no repository yet")
	if code, a := fsList(t, f, local, ""); code != http.StatusOK || a.Repo || len(a.Entries) != 1 || a.Entries[0].Ignored {
		t.Fatalf("listing outside a repository = %d %+v", code, a)
	}
	gitRun(t, f.project, "init", "-q")
	writeFile(t, filepath.Join(f.project, ".gitignore"), "build/\n*.log\n")
	writeFile(t, filepath.Join(f.project, "src", "main.go"), "package main")
	gitRun(t, f.project, "add", ".")
	gitRun(t, f.project, "commit", "-q", "-m", "init")
	writeFile(t, filepath.Join(f.project, "build", "out.bin"), "ignored")
	writeFile(t, filepath.Join(f.project, "debug.log"), "ignored")
	writeFile(t, filepath.Join(f.project, "untracked.txt"), "untracked")
	writeFile(t, filepath.Join(f.project, ".env"), "dot entry")

	code, a := fsList(t, f, local, "")
	if code != http.StatusOK || !a.Repo {
		t.Fatalf("repository listing = %d %+v", code, a)
	}
	got := map[string]bool{}
	for _, e := range a.Entries {
		got[e.Name] = e.Ignored
	}
	want := map[string]bool{".git": true, "build": true, "debug.log": true, "src": false, "plain.txt": false, "untracked.txt": false, ".env": false, ".gitignore": false}
	for name, ign := range want {
		if v, ok := got[name]; !ok || v != ign {
			t.Errorf("entry %q ignored = %v (listed %v), want %v", name, v, ok, ign)
		}
	}
	if code, a = fsList(t, f, local, "dir=build"); code != http.StatusOK || len(a.Entries) != 1 || !a.Entries[0].Ignored {
		t.Errorf("listing inside an ignored folder = %d %+v", code, a)
	}
}

// TestFSListAbsoluteRootWithoutDir pins the request shape the palette's path
// mode sends: a local viewer's fs/list with root=<abs dir> and no dir answers
// 200 with that directory's own entries, folders first. A path query that
// ends in a separator and one that does not (its parent) both reduce to this
// shape, so the second root here is the first with a trailing slash.
func TestFSListAbsoluteRootWithoutDir(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, _ := fsViewers(t, f)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "note.txt"), "N")
	writeFile(t, filepath.Join(dir, "sub", "deep.txt"), "D")
	for _, root := range []string{dir, dir + string(filepath.Separator)} {
		code, a := fsList(t, f, local, "root="+url.QueryEscape(root))
		if got := strings.Join(entryNames(a), " "); code != http.StatusOK || a.Session || got != "sub/ note.txt" {
			t.Errorf("root=%q without dir = %d session %v entries %q, want 200 and %q", root, code, a.Session, got, "sub/ note.txt")
		}
	}
}

// TestFSChangeRootIsLoopbackOnly pins the root parameter: a browser on
// this machine lists, previews, and reads an image under another absolute
// folder, where an HTML file shows as its source with no capability; a LAN
// token holder, a cross-site request, and a root that is no absolute
// folder get 404; reads still stay inside the chosen root.
func TestFSChangeRootIsLoopbackOnly(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, lan := fsViewers(t, f)
	other := t.TempDir()
	writeFile(t, filepath.Join(other, "page.html"), "<h1>OTHER-HTML</h1>")
	writeFile(t, filepath.Join(other, "art.svg"), "<svg>OTHER-SVG</svg>")
	writeFile(t, filepath.Join(other, "pic.png"), "OTHER-PNG")
	writeFile(t, filepath.Join(other, "sub", "note.md"), "[x](y.md)")
	writeFile(t, filepath.Join(f.project, "inside.txt"), "SESSION-ROOT")
	root := "root=" + url.QueryEscape(other)

	code, a := fsList(t, f, local, root)
	if got := entryNames(a); code != http.StatusOK || a.Session || a.Root != filepath.ToSlash(other) || strings.Join(got, " ") != "sub/ art.svg page.html pic.png" {
		t.Fatalf("local listing of another root = %d %+v", code, a)
	}
	for _, path := range []string{"page.html", "art.svg"} {
		code, p := fsPreview(t, f, local, root+"&path="+path)
		if code != http.StatusOK || p.Kind != "text" || !p.Source || p.Frame != "" || !strings.Contains(p.Text, "OTHER-") {
			t.Errorf("preview of %s under another root = %d %+v, want its source and no capability", path, code, p)
		}
	}
	code, p := fsPreview(t, f, local, root+"&path=pic.png")
	if code != http.StatusOK || p.Kind != "image" || !strings.Contains(p.Src, "root="+url.QueryEscape(filepath.ToSlash(other))) {
		t.Errorf("image preview under another root = %d %+v", code, p)
	}
	if rec := serve(f.d, local, http.MethodGet, p.Src, ""); rec.Code != http.StatusOK || rec.Body.String() != "OTHER-PNG" || rec.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" {
		t.Errorf("raw image under another root = %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	if code, p = fsPreview(t, f, local, root+"&path="+url.QueryEscape("sub/note.md")); code != http.StatusOK || p.Kind != "markdown" || strings.Contains(p.HTML, "/s/livesession/file") {
		t.Errorf("markdown under another root links into the session's file view: %d %+v", code, p)
	}
	// The session root named as the root parameter is the session root.
	if code, a = fsList(t, f, local, "root="+url.QueryEscape(f.project)); code != http.StatusOK || !a.Session {
		t.Errorf("the session root as the root parameter = %d session %v", code, a.Session)
	}
	// Reads stay inside the chosen root.
	for _, q := range []string{root + "&path=" + url.QueryEscape("../"+filepath.Base(f.project)+"/inside.txt"), root + "&path=" + url.QueryEscape(filepath.Join(f.project, "inside.txt"))} {
		if rec := serve(f.d, local, http.MethodGet, "/s/livesession/fs/preview?"+q, ""); rec.Code != http.StatusNotFound {
			t.Errorf("preview %q = %d, want 404", q, rec.Code)
		}
	}
	// A LAN token holder never leaves the session root.
	for _, target := range []string{"/s/livesession/fs/list?" + root, "/s/livesession/fs/preview?" + root + "&path=page.html", "/s/livesession/fs/raw?" + root + "&path=pic.png"} {
		if rec := serve(f.d, lan, http.MethodGet, target, ""); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "OTHER") {
			t.Errorf("LAN %s = %d %s, want 404", target, rec.Code, rec.Body.String())
		}
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.RemoteAddr, req.Host = local.peer, local.host
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rec := httptest.NewRecorder()
		f.d.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("cross-site %s = %d, want 404", target, rec.Code)
		}
	}
	for _, bad := range []string{"relative/dir", "//server/share", `\\server\share`, filepath.Join(other, "page.html"), filepath.Join(other, "missing")} {
		if rec := serve(f.d, local, http.MethodGet, "/s/livesession/fs/list?root="+url.QueryEscape(bad), ""); rec.Code != http.StatusNotFound {
			t.Errorf("root %q = %d, want 404", bad, rec.Code)
		}
	}
}
