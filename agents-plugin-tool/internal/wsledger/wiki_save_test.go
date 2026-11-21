// Wiki save tests exercise remote commits and push conflicts.
package wsledger

import (
	"context"
	"errors"
	"github.com/kang-sw/devenv/internal/wsgit"
	"strings"
	"sync"
	"testing"
)

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
