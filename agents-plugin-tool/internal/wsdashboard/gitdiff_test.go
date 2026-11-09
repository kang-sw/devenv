package wsdashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// lineText joins a diff line's pieces back into its text.
func lineText(l GitDiffLine) string {
	var sb strings.Builder
	for _, p := range l.Pieces {
		sb.WriteString(p.Text)
	}
	return sb.String()
}

// diffShape is a diff's lines as "<kind><text>", hunk headers as "@@".
func diffShape(d GitFileDiff) []string {
	var out []string
	for _, h := range d.Hunks {
		out = append(out, "@@")
		for _, l := range h.Lines {
			out = append(out, l.Kind+lineText(l))
		}
	}
	return out
}

// classOf is the class of the piece holding text on a line, "?" when none
// holds it.
func classOf(l GitDiffLine, text string) string {
	for _, p := range l.Pieces {
		if strings.Contains(p.Text, text) {
			return p.Class
		}
	}
	return "?"
}

func findLine(t *testing.T, d GitFileDiff, kind, text string) GitDiffLine {
	t.Helper()
	for _, h := range d.Hunks {
		for _, l := range h.Lines {
			if l.Kind == kind && strings.Contains(lineText(l), text) {
				return l
			}
		}
	}
	t.Fatalf("no %q line holding %q in %v", kind, text, diffShape(d))
	return GitDiffLine{}
}

func TestReadCommitFileDiff(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	pushRepos(t, work, 0)
	lines := strings.Repeat("line one\nline two\nline three\nline four\n", 4)
	writeFile(t, filepath.Join(work, "old.txt"), lines)
	writeFile(t, filepath.Join(work, "gone"), "1\n2\n")
	writeFile(t, filepath.Join(work, "mod.go"), "package m\n\nvar x = 1\n")
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-q", "-m", "seed")
	gitRun(t, work, "mv", "old.txt", "new.txt")
	writeFile(t, filepath.Join(work, "new.txt"), lines+"tail\n")
	gitRun(t, work, "rm", "-q", "gone")
	writeFile(t, filepath.Join(work, "mod.go"), "package m\n\nvar x = 2\n")
	writeFile(t, filepath.Join(work, "added.txt"), "fresh\n")
	writeFile(t, filepath.Join(work, "bin"), "\x00\x01\x02")
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-q", "-m", "change")
	head := revParse(t, work, "HEAD")

	d, err := readCommitFileDiff(work, head, "mod.go")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"@@", " package m", " ", "-var x = 1", "+var x = 2"}; !reflect.DeepEqual(diffShape(d), want) {
		t.Fatalf("modify = %v, want %v", diffShape(d), want)
	}
	if h := d.Hunks[0]; h.Old != 1 || h.New != 1 || !strings.HasPrefix(h.Header, "@@ -1,3 +1,3 @@") {
		t.Errorf("hunk = %+v", h)
	}
	// Highlighted (Go keyword) and the changed word marked on both sides.
	del, add := findLine(t, d, "-", "var x"), findLine(t, d, "+", "var x")
	if classOf(add, "var") != "k" {
		t.Errorf("added line pieces = %+v", add.Pieces)
	}
	for _, l := range []GitDiffLine{del, add} {
		var marked []string
		for _, p := range l.Pieces {
			if p.Mark {
				marked = append(marked, p.Text)
			}
		}
		if len(marked) != 1 || (marked[0] != "1" && marked[0] != "2") {
			t.Errorf("marks on %q = %v", lineText(l), marked)
		}
	}

	if d, err = readCommitFileDiff(work, head, "added.txt"); err != nil || !reflect.DeepEqual(diffShape(d), []string{"@@", "+fresh"}) {
		t.Errorf("add = %v %v", diffShape(d), err)
	}
	if d, err = readCommitFileDiff(work, head, "gone"); err != nil || !reflect.DeepEqual(diffShape(d), []string{"@@", "-1", "-2"}) {
		t.Errorf("delete = %v %v", diffShape(d), err)
	}
	if d, err = readCommitFileDiff(work, head, "new.txt"); err != nil {
		t.Fatal(err)
	}
	if shape := diffShape(d); len(shape) != 5 || shape[4] != "+tail" || d.Hunks[0].Old != 14 {
		t.Errorf("rename = %v (hunk %+v)", shape, d.Hunks)
	}
	if d, err = readCommitFileDiff(work, head, "bin"); err != nil || !d.Binary || len(d.Hunks) != 0 {
		t.Errorf("binary = %+v %v", d, err)
	}
	for _, p := range []string{"old.txt", "nosuch", "", "../work/mod.go"} {
		if _, err := readCommitFileDiff(work, head, p); err != errNoPath {
			t.Errorf("path %q: err = %v, want errNoPath", p, err)
		}
	}
	if _, err := readCommitFileDiff(work, strings.Repeat("0", 40), "mod.go"); err != errNoCommit {
		t.Errorf("unknown commit: err = %v", err)
	}

	// The root commit diffs against the empty tree.
	root := revParse(t, work, "HEAD~2")
	if d, err = readCommitFileDiff(work, root, "a"); err != nil || !reflect.DeepEqual(diffShape(d), []string{"@@", "+a", `\ No newline at end of file`}) {
		t.Errorf("root = %v %v", diffShape(d), err)
	}
}

func TestReadCommitFileDiffCutsALargeDiff(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	pushRepos(t, work, 0)
	line := strings.Repeat("x", 99) + "\n"
	writeFile(t, filepath.Join(work, "big.txt"), strings.Repeat(line, gitDiffMax/len(line)+500))
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-q", "-m", "big")
	d, err := readCommitFileDiff(work, revParse(t, work, "HEAD"), "big.txt")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, h := range d.Hunks {
		for _, l := range h.Lines {
			if lineText(l) != strings.TrimSuffix(line, "\n") {
				t.Fatalf("a cut line survived: %q", lineText(l))
			}
			n++
		}
	}
	if !d.Truncated || n == 0 || n >= gitDiffMax/len(line) {
		t.Errorf("truncated %v with %d lines", d.Truncated, n)
	}
}

// A block comment that opens above the hunk still colours the hunk's lines,
// because the whole file is highlighted and mapped onto them.
func TestHighlightMapsWholeFileOntoHunks(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	pushRepos(t, work, 0)
	body := "package m\n\n/*\n" + strings.Repeat("comment body\n", 10) + "old words\n" + strings.Repeat("comment tail\n", 5) + "*/\nfunc F() {}\n"
	writeFile(t, filepath.Join(work, "c.go"), body)
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-q", "-m", "seed")
	writeFile(t, filepath.Join(work, "c.go"), strings.Replace(body, "old words", "new words", 1))
	gitRun(t, work, "commit", "-q", "-am", "edit")
	d, err := readCommitFileDiff(work, revParse(t, work, "HEAD"), "c.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(diffShape(d), "\n"), "/*") {
		t.Fatalf("the hunk holds the comment opener; the test needs it above: %v", diffShape(d))
	}
	for _, k := range []string{"-", "+", " "} {
		text := map[string]string{"-": "old words", "+": "new words", " ": "comment body"}[k]
		if c := classOf(findLine(t, d, k, text), text[:3]); c != "c" {
			t.Errorf("%q line class = %q, want comment", text, c)
		}
	}

	// Over the size cap a file is shown plain.
	if hl := highlightLines("big.go", []byte("package m\n"+strings.Repeat("// c\n", hlMaxBytes/5))); hl != nil {
		t.Errorf("a file over hlMaxBytes was highlighted")
	}
	if hl := highlightLines("x.go", []byte("package m\n")); len(hl) == 0 || classOf(GitDiffLine{Pieces: []GitPiece{{Text: hl[0][0].Text, Class: hl[0][0].Class}}}, "package") != "k" {
		t.Errorf("small file = %+v", hl)
	}
	if hl := highlightLines("notes.unknownext", []byte("text\n")); hl != nil {
		t.Errorf("unknown name highlighted: %+v", hl)
	}
}

func TestPairMarks(t *testing.T) {
	// One removed line followed by one added line: paired, changed word marked.
	paired := []diffLine{{' ', "ctx"}, {'-', "a := f(1)"}, {'+', "a := f(22)"}}
	m := pairMarks(paired)
	if m[0] != [2]int{} || paired[1].text[m[1][0]:m[1][1]] != "1" || paired[2].text[m[2][0]:m[2][1]] != "22" {
		t.Errorf("paired marks = %v", m)
	}
	// Two removed, one added: not paired, no marks.
	unpaired := []diffLine{{'-', "a := 1"}, {'-', "b := 2"}, {'+', "a := 3"}}
	for i, r := range pairMarks(unpaired) {
		if r != [2]int{} {
			t.Errorf("unpaired line %d marked %v", i, r)
		}
	}
	// Lines with nothing in common are not marked.
	if r := pairMarks([]diffLine{{'-', "alpha"}, {'+', "beta"}}); r[0] != [2]int{} || r[1] != [2]int{} {
		t.Errorf("disjoint lines marked %v", r)
	}
	// Pieces split at the mark keep their highlight class.
	got := splitPieces([]hlTok{{"var", "k"}, {" x = ", ""}, {"12", "m"}}, 8, 9)
	want := []GitPiece{{Text: "var", Class: "k"}, {Text: " x = "}, {Text: "1", Class: "m", Mark: true}, {Text: "2", Class: "m"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pieces = %+v, want %+v", got, want)
	}
}

func TestHLCacheEvictsLeastRecent(t *testing.T) {
	c := newHLCache(2)
	c.put("a", hlLines{{{Text: "a"}}})
	c.put("b", nil)
	c.get("a")
	c.put("c", nil)
	if _, ok := c.get("b"); ok {
		t.Error("b survived past the bound")
	}
	if _, ok := c.get("a"); !ok {
		t.Error("a, used last, was evicted")
	}
	if _, ok := c.get("c"); !ok {
		t.Error("c missing")
	}
}

func TestReadWorktreeFileDiff(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	pushRepos(t, work, 0)
	writeFile(t, filepath.Join(work, "s.txt"), "one\n")
	writeFile(t, filepath.Join(work, "u.txt"), "keep\nold\n")
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-q", "-m", "seed")
	writeFile(t, filepath.Join(work, "s.txt"), "one\ntwo\n")
	gitRun(t, work, "add", "s.txt")
	writeFile(t, filepath.Join(work, "u.txt"), "keep\nnew\n")
	writeFile(t, filepath.Join(work, "fresh.go"), "package f\n\nfunc G() {}\n")
	writeFile(t, filepath.Join(work, "newdir", "inner.txt"), "x\n")
	outside := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, outside, "SECRET\n")
	if err := os.Symlink(outside, filepath.Join(work, "link")); err != nil {
		t.Fatal(err)
	}

	d, err := readWorktreeFileDiff(work, "staged", "s.txt")
	if err != nil || !reflect.DeepEqual(diffShape(d), []string{"@@", " one", "+two"}) {
		t.Errorf("staged = %v %v", diffShape(d), err)
	}
	d, err = readWorktreeFileDiff(work, "unstaged", "u.txt")
	if err != nil || !reflect.DeepEqual(diffShape(d), []string{"@@", " keep", "-old", "+new"}) {
		t.Errorf("unstaged = %v %v", diffShape(d), err)
	}
	d, err = readWorktreeFileDiff(work, "untracked", "fresh.go")
	if err != nil || !reflect.DeepEqual(diffShape(d), []string{"@@", "+package f", "+", "+func G() {}"}) {
		t.Fatalf("untracked = %v %v", diffShape(d), err)
	}
	if c := classOf(findLine(t, d, "+", "func G"), "G"); c != "nf" {
		t.Errorf("untracked highlight: G class %q", c)
	}
	// A symlink shows its target as text, never the file it points at.
	d, err = readWorktreeFileDiff(work, "untracked", "link")
	if err != nil || !reflect.DeepEqual(diffShape(d), []string{"@@", "+" + outside}) {
		t.Errorf("symlink = %v %v", diffShape(d), err)
	}
	if d, err = readWorktreeFileDiff(work, "untracked", "newdir/"); err != nil || d.Notice != "dir" {
		t.Errorf("untracked dir = %+v %v", d, err)
	}
	// Paths git/status does not list on that side are refused.
	for _, c := range [][2]string{{"staged", "u.txt"}, {"unstaged", "s.txt"}, {"untracked", "s.txt"},
		{"untracked", "newdir/inner.txt"}, {"untracked", outside}, {"untracked", "../secret.txt"}, {"bogus", "s.txt"}} {
		if _, err := readWorktreeFileDiff(work, c[0], c[1]); err != errNoPath {
			t.Errorf("%s %q: err = %v, want errNoPath", c[0], c[1], err)
		}
	}
	// The untracked reader itself refuses a path that leaves the root.
	if d := untrackedFileDiff(work, "../"+filepath.Base(filepath.Dir(outside))+"/secret.txt"); d.Notice != "special" || len(d.Hunks) != 0 {
		t.Errorf("escaping path = %+v", d)
	}
}

// TestDiffPathsAreLiteralFromTheTop pins how a listed path reaches git: a
// glob-shaped name matches only itself, and a session rooted in a
// subdirectory still finds the files its lists name from the top.
func TestDiffPathsAreLiteralFromTheTop(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	pushRepos(t, work, 0)
	writeFile(t, filepath.Join(work, "d", "*.txt"), "star\n")
	writeFile(t, filepath.Join(work, "d", "b.txt"), "bee\n")
	writeFile(t, filepath.Join(work, "top.txt"), "top\n")
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-q", "-m", "seed")
	writeFile(t, filepath.Join(work, "d", "*.txt"), "star\nstar2\n")
	writeFile(t, filepath.Join(work, "d", "b.txt"), "bee\nbee2\n")
	writeFile(t, filepath.Join(work, "top.txt"), "top\ntop2\n")
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-q", "-m", "change")
	head := revParse(t, work, "HEAD")
	writeFile(t, filepath.Join(work, "d", "*.txt"), "star\nstar2\nstar3\n")
	writeFile(t, filepath.Join(work, "d", "b.txt"), "bee\nbee2\nbee3\n")
	writeFile(t, filepath.Join(work, "top.txt"), "top\ntop2\ntop3\n")
	writeFile(t, filepath.Join(work, "d", "new.txt"), "n\n")
	writeFile(t, filepath.Join(work, "loose.txt"), "l\n")

	for _, root := range []string{work, filepath.Join(work, "d")} {
		d, err := readCommitFileDiff(root, head, "d/*.txt")
		if err != nil || !reflect.DeepEqual(diffShape(d), []string{"@@", " star", "+star2"}) {
			t.Errorf("%s commit d/*.txt = %v %v", root, diffShape(d), err)
		}
		d, err = readWorktreeFileDiff(root, "unstaged", "d/*.txt")
		if err != nil || !reflect.DeepEqual(diffShape(d), []string{"@@", " star", " star2", "+star3"}) {
			t.Errorf("%s unstaged d/*.txt = %v %v", root, diffShape(d), err)
		}
		d, err = readWorktreeFileDiff(root, "unstaged", "top.txt")
		if err != nil || !reflect.DeepEqual(diffShape(d), []string{"@@", " top", " top2", "+top3"}) {
			t.Errorf("%s unstaged top.txt = %v %v", root, diffShape(d), err)
		}
		d, err = readWorktreeFileDiff(root, "untracked", "d/new.txt")
		if err != nil || d.Path != "d/new.txt" || !reflect.DeepEqual(diffShape(d), []string{"@@", "+n"}) {
			t.Errorf("%s untracked d/new.txt = %+v %v", root, d, err)
		}
	}
	// From the subdirectory, an untracked file above the root is not read.
	d, err := readWorktreeFileDiff(filepath.Join(work, "d"), "untracked", "loose.txt")
	if err != nil || d.Notice != "outside" || len(d.Hunks) != 0 {
		t.Errorf("untracked above the root = %+v %v", d, err)
	}
}

func TestGitDiffRoutes(t *testing.T) {
	f := startPageFixture(t)
	work := f.project
	pushRepos(t, work, 0)
	gitCommit(t, work, "b")
	writeFile(t, filepath.Join(work, "w.txt"), "w\n")
	head := revParse(t, work, "HEAD")
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}}
	read := func(path string) (int, string) {
		t.Helper()
		resp, err := client.Get(f.base + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp.StatusCode, string(body)
	}
	commitDiff := "/s/livesession/git/commit/" + head + "/diff?path="
	var d GitFileDiff
	if code, raw := read(commitDiff + "b"); code != http.StatusOK || json.Unmarshal([]byte(raw), &d) != nil || d.Path != "b" || len(d.Hunks) != 1 {
		t.Fatalf("commit diff = %d %s", code, raw)
	}
	if code, _ := read(commitDiff + "a"); code != http.StatusNotFound {
		t.Errorf("a path the commit does not change = %d, want 404", code)
	}
	if code, _ := read("/s/livesession/git/commit/HEAD/diff?path=b"); code != http.StatusNotFound {
		t.Errorf("non-hex oid = %d, want 404", code)
	}
	wt := "/s/livesession/git/worktree/diff?side="
	if code, raw := read(wt + "untracked&path=w.txt"); code != http.StatusOK || json.Unmarshal([]byte(raw), &d) != nil || len(d.Hunks) != 1 {
		t.Fatalf("worktree diff = %d %s", code, raw)
	}
	if code, _ := read(wt + "staged&path=w.txt"); code != http.StatusNotFound {
		t.Errorf("unlisted path = %d, want 404", code)
	}
	if code, _ := read(wt + "other&path=" + url.QueryEscape("w.txt")); code != http.StatusBadRequest {
		t.Errorf("bad side = %d, want 400", code)
	}
	for _, route := range []string{commitDiff + "b", wt + "untracked&path=w.txt"} {
		if code, _, _ := postJSON(t, f.base+route, f.base, ""); code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d, want 405", route, code)
		}
	}
}
