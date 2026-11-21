// Wiki edit tests exercise structural and content conflicts.
package wsledger

import (
	"context"
	"strings"
	"testing"
)

// TestWikiRenameConflictsRefuse: a rename of a page the other side renamed
// or deleted, and a delete of a page the other side renamed, are refused;
// a page both sides deleted is not a conflict; a refused restore carries
// the restored text as the saver's text.
func TestWikiRenameConflictsRefuse(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	ctx := context.Background()

	r0 := saveOK(t, ada, wikiCreate("rn", "page", "p\n"))
	r1 := saveOK(t, ada, wikiWrite("rn", r0.Commit, "other", "o\n"))
	r2 := saveOK(t, ada, wikiWrite("rn", r1.Commit, "both", "b\n"))
	fetchOK(t, bo)

	// rename vs rename
	ra := saveOK(t, ada, WikiEdit{Wiki: "rn", Base: r2.Commit, Op: WikiOpRename, Path: "page", To: "a"})
	_, err := bo.WikiSave(ctx, WikiEdit{Wiki: "rn", Base: r2.Commit, Op: WikiOpRename, Path: "page", To: "b"})
	c := wantConflict(t, err, WikiConflictRename)
	if c.Path != "page" || c.Theirs != ra.Commit {
		t.Fatalf("rename/rename = %+v", c)
	}
	if pages, _, _ := ada.WikiPages(ctx, "rn", ""); strings.Join(pages, ",") != "a,both,other" {
		t.Fatalf("pages after the refused rename = %v", pages)
	}
	// delete vs rename (the other side renamed what this side deletes)
	_, err = bo.WikiSave(ctx, WikiEdit{Wiki: "rn", Base: r2.Commit, Op: WikiOpDelete, Path: "page"})
	wantConflict(t, err, WikiConflictRename)
	// rename vs delete (the other side deleted what this side renames)
	rd := saveOK(t, ada, WikiEdit{Wiki: "rn", Base: ra.Commit, Op: WikiOpDelete, Path: "other"})
	_, err = bo.WikiSave(ctx, WikiEdit{Wiki: "rn", Base: ra.Commit, Op: WikiOpRename, Path: "other", To: "elsewhere"})
	wantConflict(t, err, WikiConflictRename)
	// both delete: no conflict, the tip already holds the edit
	res := saveOK(t, bo, WikiEdit{Wiki: "rn", Base: ra.Commit, Op: WikiOpDelete, Path: "other"})
	if res.Commit != rd.Commit {
		t.Fatalf("delete of a page both sides deleted = %+v, want the tip %s", res, rd.Commit)
	}
	// a refused restore carries the restored text
	re := saveOK(t, ada, wikiWrite("rn", rd.Commit, "both", "b2\n"))
	fetchOK(t, bo)
	rf := saveOK(t, ada, wikiWrite("rn", re.Commit, "both", "b3\n"))
	_, err = bo.WikiSave(ctx, WikiEdit{Wiki: "rn", Base: re.Commit, Op: WikiOpRestore, Path: "both", Commit: r2.Commit})
	c = wantConflict(t, err, WikiConflictText)
	if c.OursText != "b\n" || c.TheirsText != "b3\n" || c.Theirs != rf.Commit {
		t.Fatalf("restore conflict = %+v", c)
	}
	if remoteRefs(t, url)[wikiRef("rn")] != rf.Commit {
		t.Fatal("a refused save moved the remote")
	}
}

// TestWikiDeleteOfSharedContentPageBothSidesDeleted: a page both sides
// deleted is not a conflict even when another page the base already held
// has the same content; only the same content at a path the base lacks
// reads as a rename.
func TestWikiDeleteOfSharedContentPageBothSidesDeleted(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)

	r0 := saveOK(t, ada, wikiCreate("dup", "a", "same\n"))
	r1 := saveOK(t, ada, wikiWrite("dup", r0.Commit, "b", "same\n"))
	fetchOK(t, bo)
	rd := saveOK(t, ada, WikiEdit{Wiki: "dup", Base: r1.Commit, Op: WikiOpDelete, Path: "a"})
	res := saveOK(t, bo, WikiEdit{Wiki: "dup", Base: r1.Commit, Op: WikiOpDelete, Path: "a"})
	if res.Commit != rd.Commit {
		t.Fatalf("delete of a page both sides deleted = %+v, want the tip %s", res, rd.Commit)
	}
	// The same content moved to a new path is still a rename.
	rr := saveOK(t, ada, WikiEdit{Wiki: "dup", Base: rd.Commit, Op: WikiOpRename, Path: "b", To: "c"})
	_, err := bo.WikiSave(context.Background(), WikiEdit{Wiki: "dup", Base: rd.Commit, Op: WikiOpDelete, Path: "b"})
	if c := wantConflict(t, err, WikiConflictRename); c.Theirs != rr.Commit {
		t.Fatalf("delete of a renamed page = %+v", c)
	}
}

// TestWikiRemoteHTMLMergesAndConflicts: concurrent non-overlapping edits of
// one HTML page merge line-wise, overlapping ones refuse with the page's
// .html path, and a delete or rename of an HTML page the other side
// renamed is a rename conflict.
func TestWikiRemoteHTMLMergesAndConflicts(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	ctx := context.Background()

	r0 := saveOK(t, ada, wikiCreate("h", "x.html", "<p>one</p>\n<p>two</p>\n<p>three</p>\n<p>four</p>\n<p>five</p>\n"))
	fetchOK(t, bo)
	saveOK(t, ada, wikiWrite("h", r0.Commit, "x.html", "<p>ONE</p>\n<p>two</p>\n<p>three</p>\n<p>four</p>\n<p>five</p>\n"))
	rb := saveOK(t, bo, wikiWrite("h", r0.Commit, "x.html", "<p>one</p>\n<p>two</p>\n<p>three</p>\n<p>four</p>\n<p>FIVE</p>\n"))
	if !rb.Merged {
		t.Fatalf("bo's non-overlapping save = %+v, want merged", rb)
	}
	if content, _ := readPage(t, bo, "h", "x.html"); content != "<p>ONE</p>\n<p>two</p>\n<p>three</p>\n<p>four</p>\n<p>FIVE</p>\n" {
		t.Fatalf("merged page = %q", content)
	}
	fetchOK(t, ada)
	_, base := readPage(t, ada, "h", "x.html")
	saveOK(t, ada, wikiWrite("h", base, "x.html", "<p>ONE!</p>\n<p>two</p>\n<p>three</p>\n<p>four</p>\n<p>FIVE</p>\n"))
	_, err := bo.WikiSave(ctx, wikiWrite("h", base, "x.html", "<p>uno</p>\n<p>two</p>\n<p>three</p>\n<p>four</p>\n<p>FIVE</p>\n"))
	if c := wantConflict(t, err, WikiConflictText); c.Path != "x.html" || c.OursText != "<p>uno</p>\n<p>two</p>\n<p>three</p>\n<p>four</p>\n<p>FIVE</p>\n" {
		t.Fatalf("overlapping html conflict = %+v", c)
	}

	fetchOK(t, ada)
	fetchOK(t, bo)
	_, base = readPage(t, ada, "h", "x.html")
	saveOK(t, ada, WikiEdit{Wiki: "h", Base: base, Op: WikiOpRename, Path: "x.html", To: "moved.html"})
	_, err = bo.WikiSave(ctx, WikiEdit{Wiki: "h", Base: base, Op: WikiOpDelete, Path: "x.html"})
	if c := wantConflict(t, err, WikiConflictRename); c.Path != "x.html" {
		t.Fatalf("delete of a renamed html page = %+v", c)
	}
	_, err = bo.WikiSave(ctx, WikiEdit{Wiki: "h", Base: base, Op: WikiOpRename, Path: "x.html", To: "y.html"})
	wantConflict(t, err, WikiConflictRename)
}
