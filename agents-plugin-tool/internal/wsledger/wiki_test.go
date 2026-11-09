package wsledger

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// Wiki tests run against real git repositories: a bare remote and clones
// with their own signing keys (the remote world), or one shared repository
// without a remote (the plain world).

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

// TestWikiRemoteConcurrentNonOverlappingSavesMerge: two clones save
// different pages from the same base; the second save merges on the moved
// tip and both clones end with both pages, nothing forced.
func TestWikiRemoteConcurrentNonOverlappingSavesMerge(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	ctx := context.Background()

	r0 := saveOK(t, ada, wikiCreate("team", "home", "home\n"))
	if remoteRefs(t, url)[wikiRef("team")] != r0.Commit {
		t.Fatal("the create did not reach the remote")
	}
	fetchOK(t, bo)
	if _, at := readPage(t, bo, "team", "home"); at != r0.Commit {
		t.Fatalf("bo tip = %s, want %s", at, r0.Commit)
	}
	ra := saveOK(t, ada, wikiWrite("team", r0.Commit, "alpha", "a\n"))
	rb := saveOK(t, bo, wikiWrite("team", r0.Commit, "beta", "b\n"))
	if ra.Merged || !rb.Merged {
		t.Fatalf("ada merged %v, bo merged %v; want only bo's save to merge", ra.Merged, rb.Merged)
	}
	if remoteRefs(t, url)[wikiRef("team")] != rb.Commit {
		t.Fatal("bo's merge did not reach the remote")
	}
	if !remoteIsAncestor(t, url, ra.Commit, rb.Commit) {
		t.Fatal("ada's commit is not in the merged history: the push forced")
	}
	fetchOK(t, ada)
	for _, l := range []*Ledger{ada, bo} {
		pages, at, err := l.WikiPages(ctx, "team", "")
		if err != nil || strings.Join(pages, ",") != "alpha,beta,home" || at != rb.Commit {
			t.Fatalf("%s pages = %v at %s, %v", l.stream, pages, at, err)
		}
	}
	hist, err := bo.WikiHistory(ctx, "team", "", 0)
	if err != nil || len(hist) != 4 || len(hist[0].Parents) != 2 {
		t.Fatalf("history = %+v, %v", hist, err)
	}
	for _, c := range hist {
		if !c.Verified {
			t.Fatalf("commit %s by %s unverified: %s", c.OID, c.Author, c.Unverified)
		}
	}
}

// TestWikiRemoteConflictsRefuseWithBothTexts: an overlapping edit, an edit
// of a deleted page, an edit of a renamed page, and two different
// additions are refused with the saver's text, the tip's text, the tip,
// and the base; the remote ref does not move.
func TestWikiRemoteConflictsRefuseWithBothTexts(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	ctx := context.Background()

	r0 := saveOK(t, ada, wikiCreate("k", "page", "one\ntwo\nthree\n"))
	saveOK(t, ada, wikiWrite("k", r0.Commit, "gone", "g\n"))
	_, base := readPage(t, ada, "k", "page")
	fetchOK(t, bo)

	// Overlapping lines.
	ra := saveOK(t, ada, wikiWrite("k", base, "page", "one\nTWO\nthree\n"))
	_, err := bo.WikiSave(ctx, wikiWrite("k", base, "page", "one\ndeux\nthree\n"))
	c := wantConflict(t, err, WikiConflictText)
	if c.Path != "page" || c.Base != base || c.Theirs != ra.Commit || c.OursText != "one\ndeux\nthree\n" || c.TheirsText != "one\nTWO\nthree\n" {
		t.Fatalf("conflict = %+v", c)
	}
	if remoteRefs(t, url)[wikiRef("k")] != ra.Commit {
		t.Fatal("a refused save moved the remote")
	}
	if localRef(t, bo, wikiRef("k")) != ra.Commit {
		t.Fatal("bo's fetch during the save did not follow the remote")
	}

	// Edit of a page the other side deleted.
	rd := saveOK(t, ada, WikiEdit{Wiki: "k", Base: ra.Commit, Op: WikiOpDelete, Path: "gone"})
	_, err = bo.WikiSave(ctx, wikiWrite("k", ra.Commit, "gone", "g2\n"))
	c = wantConflict(t, err, WikiConflictModifyDelete)
	if c.Path != "gone" || c.Theirs != rd.Commit || c.OursText != "g2\n" || c.TheirsText != "" {
		t.Fatalf("modify/delete conflict = %+v", c)
	}
	// Delete of a page the other side edited.
	re := saveOK(t, ada, wikiWrite("k", rd.Commit, "page", "one\nTWO\nthree\nfour\n"))
	_, err = bo.WikiSave(ctx, WikiEdit{Wiki: "k", Base: rd.Commit, Op: WikiOpDelete, Path: "page"})
	c = wantConflict(t, err, WikiConflictModifyDelete)
	if c.Path != "page" || c.TheirsText != "one\nTWO\nthree\nfour\n" || c.OursText != "" {
		t.Fatalf("delete/modify conflict = %+v", c)
	}
	// Edit of a page the other side renamed (a rename is a delete and an add).
	rr := saveOK(t, ada, WikiEdit{Wiki: "k", Base: re.Commit, Op: WikiOpRename, Path: "page", To: "moved"})
	_, err = bo.WikiSave(ctx, wikiWrite("k", re.Commit, "page", "edited after rename\n"))
	c = wantConflict(t, err, WikiConflictModifyDelete)
	if c.Path != "page" || c.Theirs != rr.Commit {
		t.Fatalf("rename conflict = %+v", c)
	}
	// Two additions of one path with different content.
	rn := saveOK(t, ada, wikiWrite("k", rr.Commit, "new", "ada's\n"))
	_, err = bo.WikiSave(ctx, wikiWrite("k", rr.Commit, "new", "bo's\n"))
	c = wantConflict(t, err, WikiConflictAddAdd)
	if c.OursText != "bo's\n" || c.TheirsText != "ada's\n" || c.Theirs != rn.Commit {
		t.Fatalf("add/add conflict = %+v", c)
	}
	// The same addition on both sides is not a conflict.
	res := saveOK(t, bo, wikiWrite("k", rr.Commit, "new", "ada's\n"))
	if res.Commit != rn.Commit {
		t.Fatalf("an identical addition = %+v, want the tip %s unchanged", res, rn.Commit)
	}
	if remoteRefs(t, url)[wikiRef("k")] != rn.Commit {
		t.Fatal("the remote moved")
	}
}

// racePushRunner runs before() once, just before the first push of ref by
// this runner, so a rival can move the remote between the saver's fetch and
// its push.
type racePushRunner struct {
	inner  wsgit.Runner
	ref    string
	once   sync.Once
	before func()
	pushes int
	mu     sync.Mutex
}

func (r *racePushRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] != "fetch" && strings.HasSuffix(args[len(args)-1], ":"+r.ref) {
		r.once.Do(r.before)
		r.mu.Lock()
		r.pushes++
		r.mu.Unlock()
	}
	return r.inner.RunGit(ctx, root, args...)
}

// TestWikiRemoteRejectedPushRetriesWithoutForce: the remote moves between
// the saver's fetch and push; the push is rejected, the saver fetches and
// merges, and the second push lands on top of the rival's commit.
func TestWikiRemoteRejectedPushRetriesWithoutForce(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	ob := w.remoteOpts("bo", "c1")
	var rival string
	runner := &racePushRunner{inner: wsgit.ExecRunner{Env: GitEnv}, ref: wikiRef("race")}
	ob.Runner = runner
	bo := w.register(ob, url)
	ctx := context.Background()

	r0 := saveOK(t, ada, wikiCreate("race", "home", "h\n"))
	fetchOK(t, bo)
	runner.before = func() {
		rival = saveOK(t, ada, wikiWrite("race", r0.Commit, "ada", "a\n")).Commit
	}
	res := saveOK(t, bo, wikiWrite("race", r0.Commit, "bo", "b\n"))
	if !res.Merged {
		t.Fatalf("save = %+v, want merged after the rejected push", res)
	}
	if n := runner.pushes; n != 2 {
		t.Fatalf("pushes = %d, want a rejected one and a landing one", n)
	}
	refs := remoteRefs(t, url)
	if refs[wikiRef("race")] != res.Commit || !remoteIsAncestor(t, url, rival, res.Commit) {
		t.Fatalf("remote tip %s; rival %s ancestor? %v", refs[wikiRef("race")], rival, remoteIsAncestor(t, url, rival, res.Commit))
	}
	pages, _, err := bo.WikiPages(ctx, "race", "")
	if err != nil || strings.Join(pages, ",") != "ada,bo,home" {
		t.Fatalf("pages = %v, %v", pages, err)
	}

	// The rival wins the race on a conflicting line: the retry refuses.
	runner.once = sync.Once{}
	_, tip := readPage(t, bo, "race", "home")
	runner.before = func() { saveOK(t, ada, wikiWrite("race", tip, "home", "ada's home\n")) }
	_, err = bo.WikiSave(ctx, wikiWrite("race", tip, "home", "bo's home\n"))
	wantConflict(t, err, WikiConflictText)
}

// TestWikiBaseRules: a save without a base is refused, a base that is not
// an ancestor of the tip is refused as stale, a new_wiki save on an
// existing wiki is refused, and new_wiki with a base is refused.
func TestWikiBaseRules(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	ctx := context.Background()

	r0 := saveOK(t, ada, wikiCreate("b", "p", "1\n"))
	r1 := saveOK(t, ada, wikiCreate("other", "p", "1\n"))
	if _, err := ada.WikiSave(ctx, wikiWrite("b", "", "p", "2\n")); !errors.Is(err, ErrWikiNoBase) {
		t.Fatalf("no base = %v", err)
	}
	// A commit of another wiki is not an ancestor of this one's tip.
	c := wantConflict(t, func() error { _, err := ada.WikiSave(ctx, wikiWrite("b", r1.Commit, "p", "2\n")); return err }(), WikiConflictStaleBase)
	if c.Theirs != r0.Commit || c.Base != r1.Commit || c.TheirsText != "1\n" || c.OursText != "2\n" {
		t.Fatalf("stale conflict = %+v", c)
	}
	// A base the clone does not hold.
	unknown := strings.Repeat("ab", 20)
	wantConflict(t, func() error { _, err := ada.WikiSave(ctx, wikiWrite("b", unknown, "p", "2\n")); return err }(), WikiConflictStaleBase)
	// A base on a wiki that does not exist.
	c = wantConflict(t, func() error { _, err := ada.WikiSave(ctx, wikiWrite("nope", r0.Commit, "p", "2\n")); return err }(), WikiConflictStaleBase)
	if c.Theirs != "" {
		t.Fatalf("stale on a missing wiki = %+v", c)
	}
	if _, err := ada.WikiSave(ctx, wikiCreate("b", "p", "2\n")); !errors.Is(err, ErrWikiExists) {
		t.Fatalf("new_wiki on existing = %v", err)
	}
	e := wikiCreate("c", "p", "2\n")
	e.Base = r0.Commit
	if _, err := ada.WikiSave(ctx, e); err == nil || !strings.Contains(err.Error(), "exclude") {
		t.Fatalf("new_wiki with a base = %v", err)
	}
	if remoteRefs(t, url)[wikiRef("b")] != r0.Commit {
		t.Fatal("a refused save moved the remote")
	}
}

// failPushRunner fails every push of a wiki ref with a transport error.
type failPushRunner struct{ inner wsgit.Runner }

func (r failPushRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	if len(args) > 0 && strings.HasSuffix(args[len(args)-1], ":"+wikiPrefix+"t") {
		return nil, errors.New("ssh: connect to host: connection refused")
	}
	return r.inner.RunGit(ctx, root, args...)
}

// TestWikiRemoteTransportFailureRefusesSave: when the push cannot reach
// the remote the save is an error and neither the local nor the remote ref
// moves; the edit is not kept as a local commit to push later.
func TestWikiRemoteTransportFailureRefusesSave(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	ob := w.remoteOpts("bo", "c1")
	ob.Runner = failPushRunner{inner: wsgit.ExecRunner{Env: GitEnv}}
	bo := w.register(ob, url)
	ctx := context.Background()

	r0 := saveOK(t, ada, wikiCreate("t", "p", "1\n"))
	fetchOK(t, bo)
	_, err := bo.WikiSave(ctx, wikiWrite("t", r0.Commit, "p", "2\n"))
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("save over a dead transport = %v", err)
	}
	if localRef(t, bo, wikiRef("t")) != r0.Commit || remoteRefs(t, url)[wikiRef("t")] != r0.Commit {
		t.Fatal("a failed save moved a ref")
	}
	if content, _ := readPage(t, bo, "t", "p"); content != "1\n" {
		t.Fatalf("page after the failed save = %q", content)
	}
	if list, _ := bo.WikiList(ctx); len(list) != 1 || list[0].Unpushed {
		t.Fatalf("list = %+v, want nothing unpushed", list)
	}
}

// TestWikiHistoryReportsUnverifiedCommits: a commit signed by no member key
// and an unsigned commit are listed with the reason, not refused; the
// member's commits are verified.
func TestWikiHistoryReportsUnverifiedCommits(t *testing.T) {
	w := newWorld(t)
	ada := w.create("ada", "c1")
	ctx := context.Background()

	r0 := saveOK(t, ada, wikiCreate("h", "p", "1\n"))
	tree, err := ada.git.treeOf(ctx, r0.Commit)
	if err != nil {
		t.Fatal(err)
	}
	foreignKey, _, _, err := ensureKey(ctx, filepath.Join(w.root, "keys", "mallory"))
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := ada.git.commitTree(ctx, tree, r0.Commit, []byte("wiki(h): forged\n"), "ada", foreignKey)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ada.git.run(ctx, "-c", "user.name=ada", "-c", "user.email=ada@ws-ledger.invalid", "commit-tree", "-m", "wiki(h): unsigned", "-p", foreign, tree)
	if err != nil {
		t.Fatal(err)
	}
	unsigned, err := lastOID(out)
	if err != nil {
		t.Fatal(err)
	}
	setRef(t, ada, wikiRef("h"), unsigned, r0.Commit)

	hist, err := ada.WikiHistory(ctx, "h", "", 0)
	if err != nil || len(hist) != 3 {
		t.Fatalf("history = %+v, %v", hist, err)
	}
	if hist[0].Verified || hist[0].Unverified != "unsigned" {
		t.Fatalf("unsigned commit = %+v", hist[0])
	}
	if hist[1].Verified || !strings.Contains(hist[1].Unverified, "not a key of any member") || hist[1].Author != "ada" {
		t.Fatalf("foreign-key commit = %+v", hist[1])
	}
	if !hist[2].Verified || hist[2].Signer != "ada" {
		t.Fatalf("own commit = %+v", hist[2])
	}
	// The branch stays usable: a save on the tip lands.
	saveOK(t, ada, wikiWrite("h", unsigned, "p", "2\n"))
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
		{"page.md", false}, {"a.MD/b", false}, {"a:b", false}, {"a\\b", false}, {"a*b", false}, {"a\tb", false},
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

// TestWikiRemoteDeleteFollowedWithRecovery: a wiki deleted on the remote
// disappears from a clone that knew it, with its tip kept as a recovery
// point and a warning; a save on the old base does not recreate it; a
// new_wiki restore from the recovery point does.
func TestWikiRemoteDeleteFollowedWithRecovery(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	ctx := context.Background()

	r0 := saveOK(t, ada, wikiCreate("gone", "p", "1\n"))
	r1 := saveOK(t, ada, wikiWrite("gone", r0.Commit, "q", "2\n"))
	fetchOK(t, bo)
	remoteGit(t, url, "update-ref", "-d", wikiRef("gone"))

	rep := fetchOK(t, bo)
	if !hasWarning(rep.Warnings, wikiRef("gone"), WarnDeleted) {
		t.Fatalf("warnings = %+v, want deleted on %s", rep.Warnings, wikiRef("gone"))
	}
	if localRef(t, bo, wikiRef("gone")) != "" {
		t.Fatal("the deleted wiki is still local")
	}
	if _, _, err := bo.WikiRead(ctx, "gone", "p", ""); !errors.Is(err, ErrWikiNotFound) {
		t.Fatalf("read after the deletion = %v", err)
	}
	points, err := bo.WikiRecovery(ctx, "gone")
	if err != nil || len(points) != 1 || points[0].OID != r1.Commit || points[0].Wiki != "gone" {
		t.Fatalf("recovery points = %+v, %v", points, err)
	}
	if points[0].Time.IsZero() || !strings.HasPrefix(points[0].Subject, "wiki(gone): write q") {
		t.Fatalf("recovery point = %+v", points[0])
	}
	// A second fetch neither warns again nor loses the recovery point.
	if rep := fetchOK(t, bo); hasWarning(rep.Warnings, wikiRef("gone"), WarnDeleted) {
		t.Fatal("the deletion warned twice")
	}
	if points, _ := bo.WikiRecovery(ctx, ""); len(points) != 1 {
		t.Fatalf("recovery points after a second fetch = %+v", points)
	}
	// Pages at the recovery point are readable by commit.
	if content, _, err := bo.WikiRead(ctx, "gone", "q", r1.Commit); err != nil || content != "2\n" {
		t.Fatalf("read at the recovery point = %q, %v", content, err)
	}
	// A save on the old base does not recreate the wiki.
	c := wantConflict(t, func() error { _, err := bo.WikiSave(ctx, wikiWrite("gone", r1.Commit, "p", "3\n")); return err }(), WikiConflictStaleBase)
	if c.Theirs != "" || !strings.Contains(c.Reason, "recovery") {
		t.Fatalf("stale after deletion = %+v", c)
	}
	if _, ok := remoteRefs(t, url)[wikiRef("gone")]; ok {
		t.Fatal("a refused save recreated the remote ref")
	}
	// ada, who also knew the wiki, follows the deletion on its next sync.
	rep = syncOK(t, ada)
	if !hasWarning(rep.Warnings, wikiRef("gone"), WarnDeleted) || localRef(t, ada, wikiRef("gone")) != "" {
		t.Fatalf("ada sync = %+v, local %s", rep.Warnings, localRef(t, ada, wikiRef("gone")))
	}
	// Restore from the recovery point recreates the wiki as a new commit.
	res := saveOK(t, bo, WikiEdit{Wiki: "gone", NewWiki: true, Op: WikiOpRestore, Path: "q", Commit: r1.Commit})
	if !res.Created || res.Commit == r1.Commit {
		t.Fatalf("restore = %+v", res)
	}
	if content, _ := readPage(t, bo, "gone", "q"); content != "2\n" {
		t.Fatalf("restored = %q", content)
	}
	if remoteRefs(t, url)[wikiRef("gone")] != res.Commit {
		t.Fatal("the restore did not reach the remote")
	}
	fetchOK(t, ada)
	if content, _ := readPage(t, ada, "gone", "q"); content != "2\n" {
		t.Fatalf("ada reads the restored wiki = %q", content)
	}
}

// TestWikiRemoteRewindFollowedWithRecovery: a remote tip moved to a
// non-descendant is followed with the replaced tip kept as a recovery
// point and a warning; a save on the replaced tip is refused as stale and
// pushes nothing.
func TestWikiRemoteRewindFollowedWithRecovery(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	ctx := context.Background()

	r0 := saveOK(t, ada, wikiCreate("rw", "p", "1\n"))
	r1 := saveOK(t, ada, wikiWrite("rw", r0.Commit, "p", "2\n"))
	fetchOK(t, bo)
	remoteGit(t, url, "update-ref", wikiRef("rw"), r0.Commit, r1.Commit)

	rep := fetchOK(t, bo)
	if !hasWarning(rep.Warnings, wikiRef("rw"), WarnRewound) {
		t.Fatalf("warnings = %+v, want rewound", rep.Warnings)
	}
	if content, at := readPage(t, bo, "rw", "p"); content != "1\n" || at != r0.Commit {
		t.Fatalf("after the rewind bo reads %q at %s", content, at)
	}
	points, err := bo.WikiRecovery(ctx, "rw")
	if err != nil || len(points) != 1 || points[0].OID != r1.Commit {
		t.Fatalf("recovery = %+v, %v", points, err)
	}
	c := wantConflict(t, func() error { _, err := bo.WikiSave(ctx, wikiWrite("rw", r1.Commit, "p", "3\n")); return err }(), WikiConflictStaleBase)
	if c.Theirs != r0.Commit || c.TheirsText != "1\n" {
		t.Fatalf("stale after rewind = %+v", c)
	}
	if remoteRefs(t, url)[wikiRef("rw")] != r0.Commit {
		t.Fatal("a refused save moved the remote")
	}
	// Restoring the page from the recovery point onto the current tip.
	res := saveOK(t, bo, WikiEdit{Wiki: "rw", Base: r0.Commit, Op: WikiOpRestore, Path: "p", Commit: r1.Commit})
	if content, at := readPage(t, bo, "rw", "p"); content != "2\n" || at != res.Commit {
		t.Fatalf("restored = %q at %s", content, at)
	}
}

// TestWikiRemoteFetchIsUnfilteredAndBadRefsWarn: over a remote honoring
// partial-clone filters, a wiki image whose blob is also an item
// attachment (fetched filtered) is readable from a second clone; a remote
// ref under the wiki namespace that is not a wiki name warns and is not
// taken; the ledger's own bad-ref handling does not change.
func TestWikiRemoteFetchIsUnfilteredAndBadRefsWarn(t *testing.T) {
	w := newWorld(t)
	url, dir := w.fileRemote("r", true)
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	ctx := context.Background()

	img := []byte("\x89PNG\r\n\x1a\n fake png bytes")
	b := Blob{OID: blobOID(img), Data: img}
	appendBlob(t, ada, b)
	syncOK(t, ada)
	e := wikiCreate("pics", "home", "![x](attachment:"+b.OID+")\n")
	e.Images = []WikiImage{{OID: b.OID, Ext: "png", Data: img}}
	r0 := saveOK(t, ada, e)

	// A ref the wiki namespace does not admit.
	remoteGit(t, dir, "update-ref", "refs/heads/wiki/Not-A-Wiki", r0.Commit)

	bo := w.register(w.remoteOpts("bo", "c1"), url)
	rep := fetchOK(t, bo)
	if !hasWarning(rep.Warnings, "refs/heads/wiki/Not-A-Wiki", WarnBadRef) {
		t.Fatalf("warnings = %+v, want a bad-ref warning", rep.Warnings)
	}
	if localRef(t, bo, "refs/heads/wiki/Not-A-Wiki") != "" {
		t.Fatal("the bad ref was taken")
	}
	data, err := bo.WikiImageRead(ctx, "pics", b.OID)
	if err != nil || string(data) != string(img) {
		t.Fatalf("image read from the second clone = %d bytes, %v", len(data), err)
	}
	if _, err := bo.WikiImageRead(ctx, "pics", blobOID([]byte("other"))); !errors.Is(err, ErrWikiImageNotFound) {
		t.Fatalf("missing image = %v", err)
	}
	if pages, _, _ := bo.WikiPages(ctx, "pics", ""); strings.Join(pages, ",") != "home" {
		t.Fatalf("pages = %v, want the attachments folder hidden", pages)
	}
	// The clone's config is not left filtered for later fetches.
	if out, _ := bo.git.run(ctx, "config", "--get", "remote.origin.partialclonefilter"); strings.TrimSpace(string(out)) != "" {
		t.Fatalf("partialclonefilter left set: %q", out)
	}
	// A page deleted and restored brings its image back.
	r1 := saveOK(t, bo, WikiEdit{Wiki: "pics", Base: r0.Commit, Op: WikiOpDelete, Path: "home"})
	saveOK(t, bo, wikiWrite("pics", r1.Commit, "other", "o\n"))
	_, tip := readPage(t, bo, "pics", "other")
	saveOK(t, bo, WikiEdit{Wiki: "pics", Base: tip, Op: WikiOpRestore, Path: "home", Commit: r0.Commit})
	if data, err := bo.WikiImageRead(ctx, "pics", b.OID); err != nil || len(data) != len(img) {
		t.Fatalf("image after restore = %d bytes, %v", len(data), err)
	}
}

// TestWikiLocalOnlyWikiPushedWhenStoreGainsRemote: a wiki written while
// the store had no remote is not read as deleted when the store registers
// a remote; it is pushed, and another clone reads it.
func TestWikiLocalOnlyWikiPushedWhenStoreGainsRemote(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	oa := w.remoteOpts("ada", "c1")
	ada, err := Create(context.Background(), oa)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r0 := saveOK(t, ada, wikiCreate("local", "p", "1\n"))
	if list, _ := ada.WikiList(ctx); len(list) != 1 || list[0].Unpushed {
		t.Fatalf("list without a remote = %+v", list)
	}
	ada.Close()

	ada = w.register(oa, url)
	if localRef(t, ada, wikiRef("local")) != r0.Commit {
		t.Fatal("registering the remote lost the local wiki")
	}
	if remoteRefs(t, url)[wikiRef("local")] != r0.Commit {
		t.Fatal("registering the remote did not push the local wiki")
	}
	if list, _ := ada.WikiList(ctx); len(list) != 1 || list[0].Unpushed {
		t.Fatalf("list after register = %+v, want pushed", list)
	}
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	if content, at := readPage(t, bo, "local", "p"); content != "1\n" || at != r0.Commit {
		t.Fatalf("bo reads %q at %s", content, at)
	}
	// A save on the pushed wiki lands on the remote.
	r1 := saveOK(t, ada, wikiWrite("local", r0.Commit, "p", "2\n"))
	if remoteRefs(t, url)[wikiRef("local")] != r1.Commit {
		t.Fatal("the save after register did not reach the remote")
	}

	// A second store writes a wiki offline, then registers to a remote
	// that already holds the name: the remote wins, the local tip is a
	// recovery point.
	oc := w.remoteOpts("cy", "c1")
	cy, err := Create(ctx, oc)
	if err != nil {
		t.Fatal(err)
	}
	rc := saveOK(t, cy, wikiCreate("local", "p", "cy's\n"))
	cy.Close()
	cy, err = Register(ctx, oc, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cy.Close() })
	if content, at := readPage(t, cy, "local", "p"); content != "2\n" || at != r1.Commit {
		t.Fatalf("cy after register reads %q at %s", content, at)
	}
	points, err := cy.WikiRecovery(ctx, "local")
	if err != nil || len(points) != 1 || points[0].OID != rc.Commit {
		t.Fatalf("cy recovery = %+v, %v", points, err)
	}
	if remoteRefs(t, url)[wikiRef("local")] != r1.Commit {
		t.Fatal("cy's register moved the remote")
	}
}

// TestWikiSaveDoesNotTouchLedgerRefs: a wiki save moves no ledger ref and
// the event view is unchanged; the fetch of a sync leaves wiki refs
// untouched when nothing moved.
func TestWikiSaveDoesNotTouchLedgerRefs(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	appendOne(t, ada, "note", note("a1"))
	syncOK(t, ada)
	before := remoteRefs(t, url)
	r0 := saveOK(t, ada, wikiCreate("side", "p", "1\n"))
	after := remoteRefs(t, url)
	delete(after, wikiRef("side"))
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatalf("ledger refs moved: %v -> %v", before, after)
	}
	rep := syncOK(t, ada)
	if len(rep.Warnings) != 0 || localRef(t, ada, wikiRef("side")) != r0.Commit {
		t.Fatalf("sync after a save = %+v", rep.Warnings)
	}
}

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

// TestWikiRecoveryListsSeveralPoints: two recovery points of one wiki and
// one of another all carry their time and subject, newest first.
func TestWikiRecoveryListsSeveralPoints(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	ctx := context.Background()

	a0 := saveOK(t, ada, wikiCreate("one", "p", "1\n"))
	a1 := saveOK(t, ada, wikiWrite("one", a0.Commit, "p", "2\n"))
	b0 := saveOK(t, ada, wikiCreate("two", "p", "1\n"))
	b1 := saveOK(t, ada, wikiWrite("two", b0.Commit, "p", "2\n"))
	fetchOK(t, bo)
	remoteGit(t, url, "update-ref", wikiRef("one"), a0.Commit, a1.Commit)
	remoteGit(t, url, "update-ref", wikiRef("two"), b0.Commit, b1.Commit)
	fetchOK(t, bo)
	a2 := saveOK(t, ada, wikiWrite("one", a0.Commit, "p", "3\n"))
	fetchOK(t, bo)
	remoteGit(t, url, "update-ref", wikiRef("one"), a0.Commit, a2.Commit)
	fetchOK(t, bo)

	points, err := bo.WikiRecovery(ctx, "")
	if err != nil || len(points) != 3 {
		t.Fatalf("recovery = %+v, %v", points, err)
	}
	for _, p := range points {
		if p.Time.IsZero() || !strings.HasPrefix(p.Subject, "wiki("+p.Wiki+"): write p") {
			t.Fatalf("point without its commit data: %+v", p)
		}
	}
	one, err := bo.WikiRecovery(ctx, "one")
	if err != nil || len(one) != 2 || one[0].OID == one[1].OID {
		t.Fatalf("recovery of one = %+v, %v", one, err)
	}
}

// TestWikiLocalAheadOfRemoteFastForwards: a wiki whose remote copy is an
// ancestor of a never-pushed local copy (the remote was seeded from the
// store) is pushed as a fast-forward, and saves on it land.
func TestWikiLocalAheadOfRemoteFastForwards(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	oa := w.remoteOpts("ada", "c1")
	ada, err := Create(context.Background(), oa)
	if err != nil {
		t.Fatal(err)
	}
	r0 := saveOK(t, ada, wikiCreate("seed", "p", "1\n"))
	r1 := saveOK(t, ada, wikiWrite("seed", r0.Commit, "p", "2\n"))
	if out, err := exec.Command("git", "-C", filepath.Join(oa.Dir, "repo.git"), "push", url, r0.Commit+":"+wikiRef("seed")).CombinedOutput(); err != nil {
		t.Fatalf("seed the remote: %v: %s", err, out)
	}
	ada.Close()

	ada = w.register(oa, url)
	if remoteRefs(t, url)[wikiRef("seed")] != r1.Commit {
		t.Fatalf("remote = %s, want the local tip %s pushed as a fast-forward", remoteRefs(t, url)[wikiRef("seed")], r1.Commit)
	}
	if list, _ := ada.WikiList(context.Background()); len(list) != 1 || list[0].Unpushed {
		t.Fatalf("list = %+v", list)
	}
	r2 := saveOK(t, ada, wikiWrite("seed", r1.Commit, "p", "3\n"))
	if remoteRefs(t, url)[wikiRef("seed")] != r2.Commit {
		t.Fatal("the save after the fast-forward did not land")
	}
}

// wikiFaultRunner fails, while its switches are on, the push of one wiki
// ref (as a remote hook rejects it) and the wiki namespace fetch (as a
// transport failure).
type wikiFaultRunner struct {
	inner     wsgit.Runner
	mu        sync.Mutex
	pushRef   string // the wiki ref whose push fails; "" for none
	failFetch bool
}

func (r *wikiFaultRunner) set(pushRef string, failFetch bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pushRef, r.failFetch = pushRef, failFetch
}

func (r *wikiFaultRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	r.mu.Lock()
	pushRef, failFetch := r.pushRef, r.failFetch
	r.mu.Unlock()
	if len(args) > 0 {
		last := args[len(args)-1]
		if pushRef != "" && strings.HasSuffix(last, ":"+pushRef) {
			return nil, fmt.Errorf("exit status 1: !\t%s\t[remote rejected] (pre-receive hook declined)", last)
		}
		if failFetch && last == "+"+wikiPrefix+"*:"+snapPrefix+wikiNamespace+"*" {
			return nil, errors.New("exit status 128: fatal: the wiki fetch broke")
		}
	}
	return r.inner.RunGit(ctx, root, args...)
}

// TestWikiPushFailureDoesNotAbortLedgerSync: a local-only wiki the remote
// refuses does not fail Register or Sync: the ledger's own push, fetch,
// and deadline clear still run and the refused wiki is a warning; saves to
// every other wiki land, while a save to the refused wiki itself is
// refused.
func TestWikiPushFailureDoesNotAbortLedgerSync(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	oa := w.remoteOpts("ada", "c1")
	ada, err := Create(context.Background(), oa)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	stuck := saveOK(t, ada, wikiCreate("stuck", "p", "1\n"))
	ada.Close()

	fr := &wikiFaultRunner{inner: wsgit.ExecRunner{Env: GitEnv}, pushRef: wikiRef("stuck")}
	oa.Runner = fr
	ada = w.register(oa, url)
	if remoteRefs(t, url)[wikiRef("stuck")] != "" {
		t.Fatal("the refused wiki reached the remote")
	}
	if rep := fetchOK(t, ada); !hasWarning(rep.Warnings, wikiRef("stuck"), WarnWikiSyncFailed) {
		t.Fatalf("report after register = %+v, want WarnWikiSyncFailed on the refused wiki", rep.Warnings)
	}

	bo := w.register(w.remoteOpts("bo", "c1"), url)
	theirs := appendOne(t, bo, "note", note("b1"))
	syncOK(t, bo)
	appendOne(t, ada, "note", note("a1"))
	rep, err := ada.Sync(ctx)
	if err != nil {
		t.Fatalf("sync with a refused local wiki = %v", err)
	}
	if !rep.Pushed || rep.Pending.Count != 0 || !rep.Pending.PushDue.IsZero() {
		t.Fatalf("sync = %+v, want pushed with nothing pending and no deadline", rep)
	}
	if !hasWarning(rep.Warnings, wikiRef("stuck"), WarnWikiSyncFailed) {
		t.Fatalf("sync warnings = %+v, want WarnWikiSyncFailed on the refused wiki", rep.Warnings)
	}
	wantVerdict(t, ada, theirs, Valid)

	res, err := ada.WikiSave(ctx, wikiCreate("free", "p", "x\n"))
	if err != nil {
		t.Fatalf("save to another wiki = %v", err)
	}
	if remoteRefs(t, url)[wikiRef("free")] != res.Commit || !hasWarning(res.Warnings, wikiRef("stuck"), WarnWikiSyncFailed) {
		t.Fatalf("save to another wiki = %+v, want landed with the refused wiki's warning", res)
	}
	if _, err := ada.WikiSave(ctx, wikiWrite("stuck", stuck.Commit, "p", "2\n")); err == nil || !strings.Contains(err.Error(), "pre-receive hook declined") {
		t.Fatalf("save to the refused wiki = %v, want its push failure", err)
	}

	// Once the remote takes it, the next sync publishes it.
	fr.set("", false)
	if rep := syncOK(t, ada); hasWarning(rep.Warnings, wikiRef("stuck"), WarnWikiSyncFailed) {
		t.Fatalf("sync after the remote relented = %+v", rep.Warnings)
	}
	if remoteRefs(t, url)[wikiRef("stuck")] != stuck.Commit {
		t.Fatal("the local wiki was not pushed once the remote took it")
	}
}

// TestWikiFetchFailureStillAbsorbsEvents: a failed wiki fetch does not
// fail the store fetch: the event refs fetched before it are absorbed, the
// wikis stay as they were, and the failure is a warning.
func TestWikiFetchFailureStillAbsorbsEvents(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	ob := w.remoteOpts("bo", "c1")
	fr := &wikiFaultRunner{inner: wsgit.ExecRunner{Env: GitEnv}}
	ob.Runner = fr
	bo := w.register(ob, url)

	r0 := saveOK(t, ada, wikiCreate("docs", "p", "1\n"))
	id := appendOne(t, ada, "note", note("a1"))
	syncOK(t, ada)
	fr.set("", true)
	for name, run := range map[string]func(context.Context) (SyncReport, error){"fetch": bo.Fetch, "sync": bo.Sync} {
		rep, err := run(context.Background())
		if err != nil {
			t.Fatalf("%s with a failing wiki fetch = %v", name, err)
		}
		if !hasWarning(rep.Warnings, wikiPrefix+"*", WarnWikiSyncFailed) {
			t.Fatalf("%s warnings = %+v, want WarnWikiSyncFailed on the wiki namespace", name, rep.Warnings)
		}
	}
	wantVerdict(t, bo, id, Valid)
	if localRef(t, bo, wikiRef("docs")) != "" {
		t.Fatal("a failed wiki fetch took the wiki")
	}
	fr.set("", false)
	if rep := fetchOK(t, bo); hasWarning(rep.Warnings, wikiPrefix+"*", WarnWikiSyncFailed) {
		t.Fatalf("fetch after recovery = %+v", rep.Warnings)
	}
	if localRef(t, bo, wikiRef("docs")) != r0.Commit {
		t.Fatal("the wiki was not taken once the fetch worked")
	}
}
