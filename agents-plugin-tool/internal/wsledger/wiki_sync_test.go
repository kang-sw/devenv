// Wiki sync tests exercise remote reconciliation and failures.
package wsledger

import (
	"context"
	"errors"
	"fmt"
	"github.com/kang-sw/devenv/internal/wsgit"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

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

// TestWikiPushFailureStillPushesOtherWikis: when one local-only wiki's
// push fails, the wikis after it are still pushed by the same sync.
func TestWikiPushFailureStillPushesOtherWikis(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	oa := w.remoteOpts("ada", "c1")
	ada, err := Create(context.Background(), oa)
	if err != nil {
		t.Fatal(err)
	}
	// pushLocalWikis walks refs in order: the refused wiki comes first.
	saveOK(t, ada, wikiCreate("alpha", "p", "1\n"))
	beta := saveOK(t, ada, wikiCreate("beta", "p", "1\n"))
	ada.Close()

	oa.Runner = &wikiFaultRunner{inner: wsgit.ExecRunner{Env: GitEnv}, pushRef: wikiRef("alpha")}
	ada = w.register(oa, url)
	refs := remoteRefs(t, url)
	if refs[wikiRef("alpha")] != "" {
		t.Fatal("the refused wiki reached the remote")
	}
	if refs[wikiRef("beta")] != beta.Commit {
		t.Fatalf("remote beta = %q, want %s: a refused wiki held up the wikis after it", refs[wikiRef("beta")], beta.Commit)
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
