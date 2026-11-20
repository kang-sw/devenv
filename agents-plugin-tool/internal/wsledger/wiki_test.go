// Wiki tests exercise local editing, page paths, and HTML pages.
package wsledger

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func wikiWrite(wiki, base, path, content string) WikiEdit {
	return WikiEdit{Wiki: wiki, Base: base, Op: WikiOpWrite, Path: path, Content: content}
}

func wikiCreate(wiki, path, content string) WikiEdit {
	e := wikiWrite(wiki, "", path, content)
	e.NewWiki = true
	return e
}

func saveOK(t *testing.T, l *Ledger, e WikiEdit) WikiSaveResult {
	t.Helper()
	res, err := l.WikiSave(context.Background(), e)
	if err != nil {
		t.Fatalf("%s: wiki save %s %s/%s: %v", l.stream, e.Op, e.Wiki, e.Path, err)
	}
	return res
}

func readPage(t *testing.T, l *Ledger, wiki, path string) (string, string) {
	t.Helper()
	content, at, err := l.WikiRead(context.Background(), wiki, path, "")
	if err != nil {
		t.Fatalf("%s: read %s/%s: %v", l.stream, wiki, path, err)
	}
	return content, at
}

func wantConflict(t *testing.T, err error, kind string) *WikiConflict {
	t.Helper()
	var c *WikiConflict
	if !errors.As(err, &c) {
		t.Fatalf("error = %v, want a *WikiConflict of kind %s", err, kind)
	}
	if c.Kind != kind {
		t.Fatalf("conflict kind = %s (%s), want %s", c.Kind, c.Reason, kind)
	}
	return c
}

func localRef(t *testing.T, l *Ledger, ref string) string {
	t.Helper()
	refs, err := l.git.listRefs(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	return refs[ref]
}

func remoteIsAncestor(t *testing.T, remote, a, b string) bool {
	t.Helper()
	err := exec.Command("git", "-C", remote, "merge-base", "--is-ancestor", a, b).Run()
	if err == nil {
		return true
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false
	}
	t.Fatalf("merge-base: %v", err)
	return false
}

// TestWikiNoRemoteCreateReadSearchHistoryDiff: without a remote a save
// moves the local ref alone; every reader of the repository sees the page,
// the page list, search hits, a verified history, and a diff.
func TestWikiNoRemoteCreateReadSearchHistoryDiff(t *testing.T) {
	w := newWorld(t)
	ada := w.create("ada", "c1")
	bo := w.join("bo", "c1")
	ctx := context.Background()

	res := saveOK(t, ada, wikiCreate("docs", "guide/intro", "# Intro\n\nHello wiki.\n"))
	if !res.Created || res.Merged || res.Commit == "" {
		t.Fatalf("create = %+v, want Created with a commit", res)
	}
	if _, err := ada.WikiSave(ctx, wikiCreate("docs", "other", "x")); !errors.Is(err, ErrWikiExists) {
		t.Fatalf("new_wiki on an existing wiki = %v, want ErrWikiExists", err)
	}
	res2 := saveOK(t, ada, wikiWrite("docs", res.Commit, "guide/intro", "# Intro\n\nHello wiki, again.\n"))
	saveOK(t, ada, wikiWrite("docs", res2.Commit, "faq", "Q: why?\nA: because.\n"))

	content, at := readPage(t, bo, "docs", "guide/intro")
	if content != "# Intro\n\nHello wiki, again.\n" || at == "" {
		t.Fatalf("bo reads %q at %s", content, at)
	}
	old, _, err := bo.WikiRead(ctx, "docs", "guide/intro", res.Commit)
	if err != nil || old != "# Intro\n\nHello wiki.\n" {
		t.Fatalf("read at the first commit = %q, %v", old, err)
	}
	pages, _, err := bo.WikiPages(ctx, "docs", "")
	if err != nil || strings.Join(pages, ",") != "faq,guide/intro" {
		t.Fatalf("pages = %v, %v", pages, err)
	}
	list, err := bo.WikiList(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "docs" || list[0].Tip != at || list[0].Unpushed {
		t.Fatalf("list = %+v, %v", list, err)
	}
	hits, _, err := bo.WikiSearch(ctx, "docs", "HELLO")
	if err != nil || len(hits) != 1 || hits[0].Path != "guide/intro" || hits[0].Line != 3 {
		t.Fatalf("search = %+v, %v", hits, err)
	}
	if hits, _, err := bo.WikiSearch(ctx, "docs", "nothing-here"); err != nil || len(hits) != 0 {
		t.Fatalf("search with no hit = %+v, %v", hits, err)
	}
	hist, err := bo.WikiHistory(ctx, "docs", "", 0)
	if err != nil || len(hist) != 3 {
		t.Fatalf("history = %+v, %v", hist, err)
	}
	for _, c := range hist {
		if !c.Verified || c.Signer != "ada" || c.Author != "ada" {
			t.Fatalf("commit %+v, want verified by ada", c)
		}
	}
	if !strings.HasPrefix(hist[0].Message, "wiki(docs): write faq") {
		t.Fatalf("newest message = %q", hist[0].Message)
	}
	pageHist, err := bo.WikiHistory(ctx, "docs", "guide/intro", 0)
	if err != nil || len(pageHist) != 2 {
		t.Fatalf("page history = %+v, %v", pageHist, err)
	}
	patch, err := bo.WikiDiff(ctx, "docs", "guide/intro", "", res2.Commit)
	if err != nil || !strings.Contains(patch, "-Hello wiki.\n+Hello wiki, again.") {
		t.Fatalf("diff = %q, %v", patch, err)
	}
	if _, _, err := bo.WikiRead(ctx, "docs", "missing", ""); !errors.Is(err, ErrWikiPageNotFound) {
		t.Fatalf("missing page = %v", err)
	}
	if _, _, err := bo.WikiRead(ctx, "nope", "faq", ""); !errors.Is(err, ErrWikiNotFound) {
		t.Fatalf("missing wiki = %v", err)
	}
	if _, err := bo.WikiSave(ctx, wikiWrite("docs", at, "faq", "Q: why?\nA: because.\n")); !errors.Is(err, ErrWikiNoChange) {
		t.Fatalf("a no-op save = %v, want ErrWikiNoChange", err)
	}
}

// TestWikiRenameDeleteRestore: rename moves the page's blob, delete
// removes it (and an emptied folder), restore brings a page back from an
// older commit as a new commit.
func TestWikiRenameDeleteRestore(t *testing.T) {
	w := newWorld(t)
	ada := w.create("ada", "c1")
	ctx := context.Background()

	r1 := saveOK(t, ada, wikiCreate("w", "a/b/page", "body\n"))
	r2 := saveOK(t, ada, WikiEdit{Wiki: "w", Base: r1.Commit, Op: WikiOpRename, Path: "a/b/page", To: "top"})
	if pages, _, _ := ada.WikiPages(ctx, "w", ""); strings.Join(pages, ",") != "top" {
		t.Fatalf("after rename pages = %v", pages)
	}
	if _, err := ada.WikiSave(ctx, WikiEdit{Wiki: "w", Base: r2.Commit, Op: WikiOpRename, Path: "a/b/page", To: "x"}); !errors.Is(err, ErrWikiPageNotFound) {
		t.Fatalf("rename of a missing page = %v", err)
	}
	saveOK(t, ada, wikiWrite("w", r2.Commit, "other", "o\n"))
	_, tip := readPage(t, ada, "w", "other")
	if _, err := ada.WikiSave(ctx, WikiEdit{Wiki: "w", Base: tip, Op: WikiOpRename, Path: "other", To: "top"}); !errors.Is(err, ErrWikiPageExists) {
		t.Fatalf("rename onto an existing page = %v", err)
	}
	r3 := saveOK(t, ada, WikiEdit{Wiki: "w", Base: tip, Op: WikiOpDelete, Path: "top"})
	if _, _, err := ada.WikiRead(ctx, "w", "top", ""); !errors.Is(err, ErrWikiPageNotFound) {
		t.Fatalf("deleted page read = %v", err)
	}
	if _, err := ada.WikiSave(ctx, WikiEdit{Wiki: "w", Base: r3.Commit, Op: WikiOpDelete, Path: "top"}); !errors.Is(err, ErrWikiPageNotFound) {
		t.Fatalf("delete of a missing page = %v", err)
	}
	r4 := saveOK(t, ada, WikiEdit{Wiki: "w", Base: r3.Commit, Op: WikiOpRestore, Path: "a/b/page", Commit: r1.Commit})
	if content, at := readPage(t, ada, "w", "a/b/page"); content != "body\n" || at != r4.Commit || r4.Commit == r1.Commit {
		t.Fatalf("restored page = %q at %s (restore commit %s)", content, at, r4.Commit)
	}
	if hist, _ := ada.WikiHistory(ctx, "w", "", 0); len(hist) != 5 || !strings.Contains(hist[0].Message, "restore a/b/page from "+r1.Commit[:12]) {
		t.Fatalf("history after restore = %+v", hist)
	}
}

func TestWikiNameAndPathValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		ok   bool
	}{
		{"docs", true}, {"a", true}, {"team-2", true}, {"0x", true},
		{"", false}, {"Docs", false}, {"-lead", false}, {"a_b", false}, {"a/b", false}, {"a b", false},
		{strings.Repeat("a", 64), true}, {strings.Repeat("a", 65), false},
	} {
		if got := ValidWikiName(tc.name); got != tc.ok {
			t.Errorf("ValidWikiName(%q) = %v, want %v", tc.name, got, tc.ok)
		}
	}
	for _, tc := range []struct {
		path string
		ok   bool
	}{
		{"home", true}, {"a/b/c", true}, {"Spaced Name/With Caps", true}, {"한글", true}, {"a.b", true},
		{"", false}, {"/a", false}, {"a/", false}, {"a//b", false}, {".", false}, {"a/../b", false},
		{"_attachments/x", false}, {"_attachments", false}, {"a/_attachments", true},
		{"page.md", true}, {"a.MD/b", false}, {"a:b", false}, {"a\\b", false}, {"a*b", false}, {"a\tb", false},
		{" a", false}, {"a ", false}, {strings.Repeat("a", 513), false},
	} {
		if got := ValidWikiPath(tc.path); got != tc.ok {
			t.Errorf("ValidWikiPath(%q) = %v, want %v", tc.path, got, tc.ok)
		}
	}
	w := newWorld(t)
	ada := w.create("ada", "c1")
	ctx := context.Background()
	if _, err := ada.WikiSave(ctx, wikiCreate("Bad", "p", "1")); !errors.Is(err, ErrWikiBadName) {
		t.Fatalf("bad name = %v", err)
	}
	if _, err := ada.WikiSave(ctx, wikiCreate("ok", "_attachments/p", "1")); !errors.Is(err, ErrWikiBadPath) {
		t.Fatalf("bad path = %v", err)
	}
	e := wikiCreate("ok", "p", "1")
	e.Op = "overwrite"
	if _, err := ada.WikiSave(ctx, e); !errors.Is(err, ErrWikiBadOp) {
		t.Fatalf("bad op = %v", err)
	}
	e = wikiCreate("ok", "p", "1")
	e.Images = []WikiImage{{OID: blobOID([]byte("x")), Ext: "png", Data: []byte("y")}}
	if _, err := ada.WikiSave(ctx, e); !errors.Is(err, ErrBlobMismatch) {
		t.Fatalf("image hash mismatch = %v", err)
	}
}

// TestWikiPathKinds: the last segment's exact lowercase extension decides
// the page kind; a.md and a are one Markdown page, a.html an HTML page;
// every other .md/.html name, and every .md/.html folder, is refused.
func TestWikiPathKinds(t *testing.T) {
	for _, tc := range []struct {
		path, canon, kind string
	}{
		{"a", "a", WikiKindMarkdown}, {"a.md", "a", WikiKindMarkdown}, {"f/a.md", "f/a", WikiKindMarkdown},
		{"a.html", "a.html", WikiKindHTML}, {"f/a.html", "f/a.html", WikiKindHTML}, {"a.b", "a.b", WikiKindMarkdown},
	} {
		got, ok := CanonicalWikiPath(tc.path)
		if !ok || got != tc.canon || WikiPageKind(got) != tc.kind {
			t.Errorf("CanonicalWikiPath(%q) = %q, %v (kind %s), want %q (%s)", tc.path, got, ok, WikiPageKind(got), tc.canon, tc.kind)
		}
	}
	for _, bad := range []string{"a.html.md", "a.md.md", "a.HTML", "a.MD", "a.Md", ".html", ".md", "a.html/b", "a.md/b", "a.HTML/b", "a.Md/b", "a .md", "..md", "..html", "a .html"} {
		if got, ok := CanonicalWikiPath(bad); ok {
			t.Errorf("CanonicalWikiPath(%q) = %q, want refused", bad, got)
		}
	}
	for _, tc := range []struct{ file, page string }{
		{"a.md", "a"}, {"f/a.md", "f/a"}, {"a.html", "a.html"}, {"f/a.html", "f/a.html"},
		{"x.html.md", ""}, {"a.md.md", ""}, {"a.html/b.md", ""}, {"a.md/b.html", ""}, {"a.HTML", ""}, {"a.txt", ""},
		{WikiAttachDir + "/x.html", ""},
	} {
		got, ok := pageOf(tc.file)
		if ok != (tc.page != "") || got != tc.page {
			t.Errorf("pageOf(%q) = %q, %v, want %q", tc.file, got, ok, tc.page)
		}
		if ok && pageFile(got) != tc.file {
			t.Errorf("pageFile(%q) = %q, want %q", got, pageFile(got), tc.file)
		}
	}
}

// plantWikiFile commits file at path in wiki's tree as a raw blob, the way
// an older daemon or a hand edit could have written it, and returns the new
// tip (no-remote world).
func plantWikiFile(t *testing.T, l *Ledger, wiki, base, file, content string) string {
	t.Helper()
	ctx := context.Background()
	tree, err := l.git.treeOf(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := l.git.hashObject(ctx, "blob", []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if tree, err = l.git.putPath(ctx, tree, strings.Split(file, "/"), blob); err != nil {
		t.Fatal(err)
	}
	commit, err := l.git.commitTree(ctx, tree, base, []byte("plant "+file+"\n"), l.member, l.keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.git.updateRef(ctx, wikiRef(wiki), commit, base); err != nil {
		t.Fatal(err)
	}
	return commit
}

// TestWikiHTMLPages: an HTML page lives beside a Markdown page of the same
// stem; listing, search, history, diff, restore, and rename address it by
// its .html path; a .md suffix addresses the Markdown page; blobs no valid
// path maps to are skipped; a write of an HTML page with images is
// refused.
func TestWikiHTMLPages(t *testing.T) {
	w := newWorld(t)
	ada := w.create("ada", "c1")
	ctx := context.Background()

	r1 := saveOK(t, ada, wikiCreate("w", "guide.md", "# Guide\n"))
	if hist, _ := ada.WikiHistory(ctx, "w", "", 0); len(hist) != 1 || hist[0].Message != "wiki(w): write guide" {
		t.Fatalf("subject of a .md write = %+v, want the canonical path", hist)
	}
	html1 := "<!DOCTYPE html>\n<html><body>\n<p id=x>Hello frame</p>\n</body></html>\n"
	r2 := saveOK(t, ada, wikiWrite("w", r1.Commit, "guide.html", html1))
	if content, _ := readPage(t, ada, "w", "guide"); content != "# Guide\n" {
		t.Fatalf("guide = %q", content)
	}
	if content, _ := readPage(t, ada, "w", "guide.md"); content != "# Guide\n" {
		t.Fatalf("guide.md = %q", content)
	}
	if content, _ := readPage(t, ada, "w", "guide.html"); content != html1 {
		t.Fatalf("guide.html = %q", content)
	}
	// Blobs no page path maps to: an old x.html.md and a file under a .html
	// folder.
	tip := plantWikiFile(t, ada, "w", r2.Commit, "x.html.md", "legacy hello\n")
	tip = plantWikiFile(t, ada, "w", tip, "f.html/y.md", "hello under folder\n")
	pages, _, err := ada.WikiPages(ctx, "w", "")
	if err != nil || strings.Join(pages, ",") != "guide,guide.html" {
		t.Fatalf("pages = %v, %v", pages, err)
	}
	hits, _, err := ada.WikiSearch(ctx, "w", "hello")
	if err != nil || len(hits) != 1 || hits[0].Path != "guide.html" || hits[0].Line != 3 || hits[0].Text != "<p id=x>Hello frame</p>" {
		t.Fatalf("search = %+v, %v", hits, err)
	}
	for _, p := range []string{"x.html.md", "f.html/y"} {
		if _, _, err := ada.WikiRead(ctx, "w", p, ""); !errors.Is(err, ErrWikiBadPath) {
			t.Fatalf("read %s = %v, want ErrWikiBadPath", p, err)
		}
	}
	if _, _, err := ada.WikiRead(ctx, "w", "x.html", ""); !errors.Is(err, ErrWikiPageNotFound) {
		t.Fatalf("read x.html = %v, want ErrWikiPageNotFound (x.html.md is no page)", err)
	}
	// The legacy folder f.html/ is no page: a write, restore, or delete of
	// f.html must not replace or remove it.
	if _, err := ada.WikiSave(ctx, wikiWrite("w", tip, "f.html", "<p>x</p>")); !errors.Is(err, ErrWikiBadEdit) {
		t.Fatalf("write over the folder f.html/ = %v, want ErrWikiBadEdit", err)
	}
	if _, err := ada.WikiSave(ctx, WikiEdit{Wiki: "w", Base: tip, Op: WikiOpDelete, Path: "f.html"}); !errors.Is(err, ErrWikiPageNotFound) {
		t.Fatalf("delete of the folder f.html/ = %v, want ErrWikiPageNotFound", err)
	}
	if _, err := ada.WikiSave(ctx, WikiEdit{Wiki: "w", Base: tip, Op: WikiOpRestore, Path: "guide.html", Commit: r2.Commit}); !errors.Is(err, ErrWikiNoChange) {
		t.Fatalf("restore of an unchanged page = %v, want ErrWikiNoChange", err)
	}
	r3 := saveOK(t, ada, wikiWrite("w", tip, "guide.html", strings.Replace(html1, "Hello frame", "Hello again", 1)))
	hist, err := ada.WikiHistory(ctx, "w", "guide.html", 0)
	if err != nil || len(hist) != 2 || hist[0].OID != r3.Commit || hist[1].OID != r2.Commit {
		t.Fatalf("guide.html history = %+v, %v", hist, err)
	}
	if hist, err := ada.WikiHistory(ctx, "w", "guide", 0); err != nil || len(hist) != 1 || hist[0].OID != r1.Commit {
		t.Fatalf("guide history = %+v, %v", hist, err)
	}
	patch, err := ada.WikiDiff(ctx, "w", "guide.html", "", r3.Commit)
	if err != nil || !strings.Contains(patch, "-<p id=x>Hello frame</p>\n+<p id=x>Hello again</p>") || strings.Contains(patch, "guide.md") {
		t.Fatalf("diff = %q, %v", patch, err)
	}
	r4 := saveOK(t, ada, WikiEdit{Wiki: "w", Base: r3.Commit, Op: WikiOpRestore, Path: "guide.html", Commit: r2.Commit})
	if content, _ := readPage(t, ada, "w", "guide.html"); content != html1 {
		t.Fatalf("restored guide.html = %q", content)
	}
	// Images with an HTML write are refused.
	img := []byte("png-bytes")
	e := wikiWrite("w", r4.Commit, "pic.html", "<p>x</p>")
	e.Images = []WikiImage{{OID: blobOID(img), Ext: "png", Data: img}}
	if _, err := ada.WikiSave(ctx, e); !errors.Is(err, ErrWikiBadEdit) {
		t.Fatalf("html write with images = %v, want ErrWikiBadEdit", err)
	}
	// A rename may change the kind; the target must be free.
	if _, err := ada.WikiSave(ctx, WikiEdit{Wiki: "w", Base: r4.Commit, Op: WikiOpRename, Path: "guide.md", To: "guide.html"}); !errors.Is(err, ErrWikiPageExists) {
		t.Fatalf("rename onto the existing html page = %v, want ErrWikiPageExists", err)
	}
	r5 := saveOK(t, ada, WikiEdit{Wiki: "w", Base: r4.Commit, Op: WikiOpRename, Path: "guide", To: "other.html"})
	if pages, _, _ := ada.WikiPages(ctx, "w", ""); strings.Join(pages, ",") != "guide.html,other.html" {
		t.Fatalf("pages after the cross-kind rename = %v", pages)
	}
	if content, _ := readPage(t, ada, "w", "other.html"); content != "# Guide\n" {
		t.Fatalf("renamed page = %q", content)
	}
	if hist, _ := ada.WikiHistory(ctx, "w", "", 1); len(hist) != 1 || hist[0].OID != r5.Commit || !strings.HasPrefix(hist[0].Message, "wiki(w): rename guide -> other.html") {
		t.Fatalf("rename subject = %+v", hist)
	}
}
