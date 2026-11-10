package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fsSearch runs a content search over a real connection, as a browser on
// this machine: the search stops when its request is done, so the recorder
// helper (serve), whose request is already done, sees none of it.
func fsSearch(t *testing.T, f *pageFixture, query string) (int, fsSearchAnswer) {
	t.Helper()
	code, body := get(t, f.base+"/s/livesession/fs/search?"+query)
	var a fsSearchAnswer
	if code == http.StatusOK {
		if err := json.Unmarshal([]byte(body), &a); err != nil {
			t.Fatalf("search %q: %v in %s", query, err, body)
		}
	}
	return code, a
}

// hits flattens a search answer to "path:line" entries, sorted by the
// answer's own order.
func hits(a fsSearchAnswer) []string {
	out := []string{}
	for _, f := range a.Files {
		for _, m := range f.Matches {
			out = append(out, fmt.Sprintf("%s:%d", f.Path, m.Line))
		}
	}
	return out
}

// TestFSSearchModes pins the content search: a literal without case by
// default, with case on request, a regular expression on request (a text
// that does not compile answers 400), each matching line cut around its
// match; a binary file, a file over the text cap, and a symlink out of the
// root are not read; a LAN token holder searches the session root and gets
// 404 for another root.
func TestFSSearchModes(t *testing.T) {
	f := startPageFixture(t, lanMode)
	local, lan := fsViewers(t, f)
	outside := filepath.Join(t.TempDir(), "outside")
	writeFile(t, filepath.Join(outside, "secret.txt"), "hello OUTSIDE")
	writeFile(t, filepath.Join(f.project, "a.txt"), "Hello World\r\n  say hello (again)\nnothing here\n")
	writeFile(t, filepath.Join(f.project, "sub", "b.go"), "package b\n\nfunc Hello() {}\n")
	writeFile(t, filepath.Join(f.project, "bin.dat"), "hello\x00binary")
	writeFile(t, filepath.Join(f.project, "big.txt"), "hello big\n"+strings.Repeat("x", fileViewMax))
	// A symlink out of the root, where the OS allows one: never read.
	_ = os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(f.project, "escape.txt"))

	code, a := fsSearch(t, f, "q=hello")
	if got, want := hits(a), []string{"a.txt:1", "a.txt:2", "sub/b.go:3"}; code != http.StatusOK || a.More || !reflect.DeepEqual(got, want) {
		t.Fatalf("literal search = %d %v (more %v), want %v", code, got, a.More, want)
	}
	if m := a.Files[0].Matches; m[0] != (fsMatch{Line: 1, Hit: "Hello", Post: " World"}) || m[1] != (fsMatch{Line: 2, Pre: "say ", Hit: "hello", Post: " (again)"}) {
		t.Errorf("excerpts = %+v", m)
	}
	for query, want := range map[string][]string{
		"q=Hello&case=1":                                        {"a.txt:1", "sub/b.go:3"},
		"q=" + url.QueryEscape("Hello()"):                       {"sub/b.go:3"},
		"q=" + url.QueryEscape("hello (again)"):                 {"a.txt:2"},
		"q=" + url.QueryEscape(`h.llo\s+\(`) + "&regex=1":       {"a.txt:2"},
		"q=" + url.QueryEscape(`^func \w+`) + "&regex=1&case=1": {"sub/b.go:3"},
		"q=" + url.QueryEscape(`h.llo`):                         {},
		"q=" + url.QueryEscape(`x*`) + "&regex=1":               {},
		"q=OUTSIDE": {},
		"q=binary":  {},
		"q=big":     {},
		"q=":        {},
	} {
		if code, a := fsSearch(t, f, query); code != http.StatusOK || !reflect.DeepEqual(hits(a), want) {
			t.Errorf("search %s = %d %v, want %v", query, code, hits(a), want)
		}
	}
	if rec := serve(f.d, lan, http.MethodGet, "/s/livesession/fs/search?regex=1&q="+url.QueryEscape("a(b"), ""); rec.Code != http.StatusBadRequest {
		t.Errorf("a regular expression that does not compile = %d, want 400", rec.Code)
	}
	if rec := serve(f.d, lan, http.MethodGet, "/s/livesession/fs/search?q="+strings.Repeat("q", fsSearchQueryMax+1), ""); rec.Code != http.StatusNotFound {
		t.Errorf("an oversize query = %d, want 404", rec.Code)
	}

	// A LAN token holder searches the session root; another root is for a
	// browser on this machine only.
	root := "&root=" + url.QueryEscape(outside)
	if code, a := fsSearch(t, f, "q=hello"+root); code != http.StatusOK || !reflect.DeepEqual(hits(a), []string{"secret.txt:1"}) {
		t.Errorf("local search under another root = %d %v", code, hits(a))
	}
	if rec := serve(f.d, lan, http.MethodGet, "/s/livesession/fs/search?q=hello", ""); rec.Code != http.StatusOK {
		t.Errorf("LAN search of the session root = %d, want 200", rec.Code)
	}
	if rec := serve(f.d, lan, http.MethodGet, "/s/livesession/fs/search?q=hello"+root, ""); rec.Code != http.StatusNotFound {
		t.Errorf("LAN search under another root = %d, want 404", rec.Code)
	}
	for _, v := range []viewer{local, lan} {
		rec := serve(f.d, v, http.MethodGet, "/s/livesession/fs/find?q=bgo", "")
		var a fsFindAnswer
		if err := json.Unmarshal(rec.Body.Bytes(), &a); rec.Code != http.StatusOK || err != nil || len(a.Items) == 0 || a.Items[0] != (fileItem{Path: "sub/b.go"}) {
			t.Errorf("%s: find bgo = %d %s", v.name, rec.Code, rec.Body.String())
		}
	}
	if rec := serve(f.d, lan, http.MethodGet, "/s/livesession/fs/find?q=secret"+root, ""); rec.Code != http.StatusNotFound {
		t.Errorf("LAN find under another root = %d, want 404", rec.Code)
	}
	rec := serve(f.d, local, http.MethodGet, "/s/livesession/fs/find?q=secret"+root, "")
	if !strings.Contains(rec.Body.String(), `"path":"secret.txt"`) {
		t.Errorf("local find under another root = %d %s", rec.Code, rec.Body.String())
	}
}

// TestFSSearchStopsAtItsCaps pins the two caps: the search answers at most
// fsSearchFiles files and fsSearchMatches lines, and says more when it
// stopped at either.
func TestFSSearchStopsAtItsCaps(t *testing.T) {
	root := t.TempDir()
	var paths []string
	for i := 0; i < fsSearchFiles+5; i++ {
		p := fmt.Sprintf("f%04d.txt", i)
		writeFile(t, filepath.Join(root, p), "needle\n")
		paths = append(paths, p)
	}
	re, err := searchPattern("needle", false, false)
	if err != nil {
		t.Fatal(err)
	}
	a := searchFiles(context.Background(), root, fileCandidates(paths), re)
	if len(a.Files) != fsSearchFiles || !a.More {
		t.Errorf("a search over %d matching files = %d files, more %v; want %d and more", len(paths), len(a.Files), a.More, fsSearchFiles)
	}
	a = searchFiles(context.Background(), root, fileCandidates(paths[:fsSearchFiles]), re)
	if len(a.Files) != fsSearchFiles || a.More {
		t.Errorf("a search over exactly the cap = %d files, more %v", len(a.Files), a.More)
	}

	lines := t.TempDir()
	writeFile(t, filepath.Join(lines, "a.txt"), strings.Repeat("needle\n", fsSearchMatches-10))
	writeFile(t, filepath.Join(lines, "b.txt"), strings.Repeat("needle\n", 30))
	writeFile(t, filepath.Join(lines, "c.txt"), "needle\n")
	a = searchFiles(context.Background(), lines, fileCandidates([]string{"a.txt", "b.txt", "c.txt"}), re)
	if got := len(hits(a)); got != fsSearchMatches || !a.More || len(a.Files) != 2 || len(a.Files[1].Matches) != 10 {
		t.Errorf("a search over more matching lines than the cap = %d lines in %d files, more %v", got, len(a.Files), a.More)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if a = searchFiles(ctx, lines, fileCandidates([]string{"a.txt"}), re); len(a.Files) != 0 || !a.More {
		t.Errorf("a search past its deadline = %d files, more %v; want none and more", len(a.Files), a.More)
	}
	long := strings.Repeat("é", 300)
	if m, _ := matchLines(re, long+"needle"+long, 5); len(m) != 1 || []rune(m[0].Pre)[0] != '…' || len([]rune(m[0].Pre)) != fsSearchPre+1 || len([]rune(m[0].Post)) != fsSearchSide+1 {
		t.Errorf("a long line's excerpt = %+v", m)
	}
}

// TestStatusMarks pins the reading of git's porcelain status for one
// directory: modified, added, untracked, and deleted files, an untracked
// directory, the roll-up mark of a directory that holds changes, entries
// that are gone from disk, and a directory inside an untracked one.
func TestStatusMarks(t *testing.T) {
	status := strings.Join([]string{
		" M src/mod.go", "A  src/added.go", "AM src/addmod.go", "?? src/new.txt", " D src/gone.go", "D  src/staged-gone.go",
		"?? src/fresh/", "MM src/deep/x/y.go", " D src/dead/only.go", " D src/mixed/a.go", " M src/mixed/b.go", "UU src/conflict.go",
		" M other/elsewhere.go", " M top.go", "?? srcfile", " A src/intent.go",
	}, "\x00") + "\x00"
	marks, dirs, gone := statusMarks([]byte(status), "src/")
	want := map[string]string{"mod.go": "M", "added.go": "A", "addmod.go": "A", "new.txt": "U", "gone.go": "D", "staged-gone.go": "D",
		"fresh": "U", "deep": "*", "dead": "*", "mixed": "*", "conflict.go": "M", "intent.go": "A"}
	if !reflect.DeepEqual(marks, want) {
		t.Errorf("marks under src/ = %v, want %v", marks, want)
	}
	if want := map[string]bool{"fresh": true, "deep": true, "dead": true, "mixed": true}; !reflect.DeepEqual(dirs, want) {
		t.Errorf("directories = %v, want %v", dirs, want)
	}
	if want := map[string]bool{"gone.go": true, "staged-gone.go": true, "dead": true}; !reflect.DeepEqual(gone, want) {
		t.Errorf("gone = %v, want %v", gone, want)
	}
	marks, _, _ = statusMarks([]byte(status), "")
	if want := map[string]string{"src": "*", "other": "*", "top.go": "M", "srcfile": "U"}; !reflect.DeepEqual(marks, want) {
		t.Errorf("marks at the top = %v, want %v", marks, want)
	}
	// The listed directory sits inside an untracked one: all of it is.
	for _, prefix := range []string{"new/", "new/sub/"} {
		if marks, _, _ = statusMarks([]byte("?? new/\x00"), prefix); !reflect.DeepEqual(marks, map[string]string{"": "U"}) {
			t.Errorf("marks under %s inside an untracked directory = %v", prefix, marks)
		}
	}
}

// TestFSListCarriesGitMarks pins the marks on a real repository's listing:
// each changed file's mark, the roll-up on a folder holding changes, a
// deleted file listed as gone, an ignored entry unmarked, and a nested
// directory read by its own path; outside a repository nothing is marked.
func TestFSListCarriesGitMarks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	f := startPageFixture(t, lanMode)
	_, lan := fsViewers(t, f)
	for path, text := range map[string]string{".gitignore": "*.log\n", "keep.txt": "1", "mod.txt": "1", "gone.txt": "1",
		"src/a.go": "1", "src/deep/b.go": "1", "clean/c.go": "1", "dead/d.go": "1"} {
		writeFile(t, filepath.Join(f.project, path), text)
	}
	gitRun(t, f.project, "init", "-q")
	gitRun(t, f.project, "add", "-A")
	gitRun(t, f.project, "commit", "-q", "-m", "base")
	writeFile(t, filepath.Join(f.project, "mod.txt"), "2")
	writeFile(t, filepath.Join(f.project, "new.txt"), "1")
	writeFile(t, filepath.Join(f.project, "staged.txt"), "1")
	writeFile(t, filepath.Join(f.project, "noise.log"), "1")
	writeFile(t, filepath.Join(f.project, "fresh", "f.txt"), "1")
	writeFile(t, filepath.Join(f.project, "src", "deep", "b.go"), "2")
	gitRun(t, f.project, "add", "staged.txt")
	for _, p := range []string{"gone.txt", "dead"} {
		if err := os.RemoveAll(filepath.Join(f.project, p)); err != nil {
			t.Fatal(err)
		}
	}
	// A nested repository added as a gitlink is one path to git; a new
	// commit inside it is a change the folder holds.
	sub := filepath.Join(f.project, "sub")
	writeFile(t, filepath.Join(sub, "s.txt"), "1")
	gitRun(t, sub, "init", "-q")
	gitRun(t, sub, "add", "-A")
	gitRun(t, sub, "commit", "-q", "-m", "sub")
	gitRun(t, f.project, "add", "sub")
	gitRun(t, f.project, "commit", "-q", "-m", "link", "--", "sub")
	writeFile(t, filepath.Join(sub, "s.txt"), "2")
	gitRun(t, sub, "commit", "-q", "-a", "-m", "sub moves")
	marked := func(dir string) map[string]string {
		t.Helper()
		code, a := fsList(t, f, lan, "dir="+url.QueryEscape(dir))
		if code != http.StatusOK {
			t.Fatalf("list %q = %d", dir, code)
		}
		out := map[string]string{}
		for _, e := range a.Entries {
			n := e.Name
			if e.Dir {
				n += "/"
			}
			if e.Gone {
				n += " (gone)"
			}
			if e.Status != "" || e.Gone {
				out[n] = e.Status
			}
		}
		return out
	}
	if got, want := marked(""), map[string]string{"mod.txt": "M", "new.txt": "U", "staged.txt": "A", "gone.txt (gone)": "D",
		"fresh/": "U", "src/": "*", "dead/ (gone)": "*", "sub/": "*"}; !reflect.DeepEqual(got, want) {
		t.Errorf("root marks = %v, want %v", got, want)
	}
	if got, want := marked("src"), map[string]string{"deep/": "*"}; !reflect.DeepEqual(got, want) {
		t.Errorf("src marks = %v, want %v", got, want)
	}
	if got, want := marked("src/deep"), map[string]string{"b.go": "M"}; !reflect.DeepEqual(got, want) {
		t.Errorf("src/deep marks = %v, want %v", got, want)
	}
	if got, want := marked("fresh"), map[string]string{"f.txt": "U"}; !reflect.DeepEqual(got, want) {
		t.Errorf("marks inside an untracked folder = %v, want %v", got, want)
	}
	if got := marked("clean"); len(got) != 0 {
		t.Errorf("a clean folder's marks = %v", got)
	}

	plain := startPageFixture(t, lanMode)
	writeFile(t, filepath.Join(plain.project, "a.txt"), "1")
	_, plainLAN := fsViewers(t, plain)
	if code, a := fsList(t, plain, plainLAN, ""); code != http.StatusOK || len(a.Entries) != 1 || a.Entries[0].Status != "" || a.Entries[0].Gone {
		t.Errorf("listing outside a repository = %d %+v", code, a.Entries)
	}
}

// TestFSPreviewAsPlainText pins text=1: a file the preview would render (a
// markdown file, a diff, an HTML file in the session root) answers as its
// plain text, which a jump to a line needs; an image stays an image.
func TestFSPreviewAsPlainText(t *testing.T) {
	f := startPageFixture(t)
	local := viewer{name: "local", peer: "127.0.0.1:50000", host: "127.0.0.1"}
	writeFile(t, filepath.Join(f.project, "doc.md"), "# Title\n")
	writeFile(t, filepath.Join(f.project, "page.html"), "<h1>x</h1>")
	writeFile(t, filepath.Join(f.project, "c.diff"), "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n")
	writeFile(t, filepath.Join(f.project, "shot.png"), "PNG")
	for path, want := range map[string]string{"doc.md": "# Title\n", "page.html": "<h1>x</h1>", "c.diff": "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n"} {
		code, a := fsPreview(t, f, local, "text=1&path="+path)
		if code != http.StatusOK || a.Kind != "text" || a.Text != want || a.Source || a.Frame != "" || a.HTML != "" || len(a.Diff) != 0 {
			t.Errorf("text=1 preview of %s = %d %+v", path, code, a)
		}
		if _, a = fsPreview(t, f, local, "path="+path); a.Kind == "text" {
			t.Errorf("the plain preview of %s is text without text=1", path)
		}
	}
	if _, a := fsPreview(t, f, local, "text=1&path=shot.png"); a.Kind != "image" {
		t.Errorf("text=1 preview of an image = %+v", a)
	}
}
