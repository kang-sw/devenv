package wsledger

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// blob returns a test attachment and its oid.
func blob(s string) Blob {
	data := []byte(s)
	return Blob{OID: blobOID(data), Data: data}
}

// appendBlob appends one event naming the blob's oid, with the blob.
func appendBlob(t *testing.T, l *Ledger, b Blob) EventID {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"attachments": []map[string]string{{"oid": b.OID}}})
	ids, err := l.AppendWithBlobs(context.Background(), []NewEvent{{Type: "note", Payload: raw}}, []Blob{b})
	if err != nil {
		t.Fatalf("append with blob: %v", err)
	}
	return ids[0]
}

// treePaths lists the paths of a commit's tree.
func treePaths(t *testing.T, l *Ledger, commit string) []string {
	t.Helper()
	out, err := l.git.run(context.Background(), "ls-tree", "-r", "--name-only", commit)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(out))
}

// forgeAttach adds one commit to ref holding the parent's tree plus the
// given path -> content entries, signed by signer's key (unsigned when
// nil), and moves ref to it.
func forgeAttach(t *testing.T, l *Ledger, ref, parent string, entries map[string]string, signer *Ledger) string {
	t.Helper()
	ctx := context.Background()
	tree := ""
	if parent != "" {
		var err error
		if tree, err = l.git.treeOf(ctx, parent); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range entries {
		oid, err := l.git.hashObject(ctx, "blob", []byte(content))
		if err != nil {
			t.Fatal(err)
		}
		if tree, err = l.git.putPath(ctx, tree, strings.Split(path, "/"), oid); err != nil {
			t.Fatal(err)
		}
	}
	var oid string
	if signer != nil {
		var err error
		if oid, err = l.git.commitTree(ctx, tree, parent, []byte("{}"), signer.member, signer.keyPath); err != nil {
			t.Fatal(err)
		}
	} else {
		args := []string{"-c", "user.name=x", "-c", "user.email=x@ws-ledger.invalid", "commit-tree", "-m", "{}"}
		if parent != "" {
			args = append(args, "-p", parent)
		}
		out, err := l.git.run(ctx, append(args, tree)...)
		if err != nil {
			t.Fatal(err)
		}
		if oid, err = lastOID(out); err != nil {
			t.Fatal(err)
		}
	}
	setRef(t, l, ref, oid, parent)
	return oid
}

func reloadOK(t *testing.T, l *Ledger) SyncReport {
	t.Helper()
	rep, err := l.Reload(context.Background())
	if err != nil {
		t.Fatalf("reload %s: %v", l.stream, err)
	}
	return rep
}

func readOK(t *testing.T, l *Ledger, oid, prefer string) []byte {
	t.Helper()
	data, err := l.ReadAttachment(context.Background(), oid, prefer)
	if err != nil {
		t.Fatalf("%s: read attachment %s: %v", l.stream, oid, err)
	}
	return data
}

// TestAttachAppendCommitsBlobsToAttachRef: a write with blobs commits them
// to the writer's attachment ref (one path per blob, never changed), keeps
// the event commit to its one file, and every clone of the repository reads
// them back; a blob that does not hash to its oid is refused before
// anything is written.
func TestAttachAppendCommitsBlobsToAttachRef(t *testing.T) {
	w := newWorld(t)
	ada := w.create("ada", "c1")
	bo := w.join("bo", "c1")
	ctx := context.Background()

	bad := Blob{OID: strings.Repeat("a", 64), Data: []byte("x")}
	raw, _ := json.Marshal(map[string]any{"attachments": []map[string]string{{"oid": bad.OID}}})
	if _, err := ada.AppendWithBlobs(ctx, []NewEvent{{Type: "note", Payload: raw}}, []Blob{bad}); !errors.Is(err, ErrBlobMismatch) {
		t.Fatalf("mismatched blob append = %v, want ErrBlobMismatch", err)
	}
	if p := ada.Pending(); p.Count != 0 {
		t.Fatalf("a refused write left %d pending events", p.Count)
	}

	b1, b2 := blob("first image"), blob("second image")
	id1 := appendBlob(t, ada, b1)
	if !ada.HasAttachment(b1.OID) {
		t.Fatal("a pending write's blob is not HasAttachment")
	}
	if got := readOK(t, ada, b1.OID, ""); string(got) != "first image" {
		t.Fatalf("pending blob read = %q", got)
	}
	syncOK(t, ada)
	wantVerdict(t, ada, id1, Valid)

	aref := attachRef("ada", "c1")
	tip := refTip(t, ada, aref)
	if tip == "" {
		t.Fatal("no attachment ref after the batch closed")
	}
	if paths := treePaths(t, ada, tip); len(paths) != 1 || paths[0] != attachPath(b1.OID) {
		t.Fatalf("attachment tree = %v, want [%s]", paths, attachPath(b1.OID))
	}
	if paths := treePaths(t, ada, refTip(t, ada, memberRef("ada", "c1"))); len(paths) != 1 {
		t.Fatalf("event commit tree = %v, want one file", paths)
	}

	// A second batch adds a path and keeps the first.
	appendBlob(t, ada, b2)
	syncOK(t, ada)
	tip2 := refTip(t, ada, aref)
	if paths := treePaths(t, ada, tip2); len(paths) != 2 {
		t.Fatalf("attachment tree after two batches = %v, want two paths", paths)
	}
	if !ada.git.isAncestor(ctx, tip, tip2) {
		t.Fatal("the second attachment commit does not descend from the first")
	}

	reloadOK(t, bo)
	for _, b := range []Blob{b1, b2} {
		if !bo.HasAttachment(b.OID) {
			t.Fatalf("bo lacks %s", b.OID)
		}
		if got := readOK(t, bo, b.OID, "ada"); string(got) != string(b.Data) {
			t.Fatalf("bo read %s = %q", b.OID, got)
		}
	}
	if _, err := bo.ReadAttachment(ctx, blob("never written").OID, ""); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("read of an unknown oid = %v, want ErrAttachmentNotFound", err)
	}
}

// TestAttachVerifierRefusesMalformedCommits: a commit of an attachment ref
// that is unsigned, signed by another member's key, changes a path, or adds
// a path outside attachments/<sha256> cuts the chain there with a
// WarnBadAttachment; the commits before it stay readable.
func TestAttachVerifierRefusesMalformedCommits(t *testing.T) {
	w := newWorld(t)
	ada := w.create("ada", "c1")
	bo := w.join("bo", "c1")
	good := blob("good")
	ref := attachRef("bo", "c1")

	cases := []struct {
		name    string
		entries map[string]string
		signer  func() *Ledger
		wantBad string
	}{
		{"unsigned", map[string]string{attachPath(blob("u").OID): "u"}, func() *Ledger { return nil }, "unsigned"},
		{"foreign key", map[string]string{attachPath(blob("f").OID): "f"}, func() *Ledger { return ada }, "not a key of bo"},
		{"changed path", map[string]string{attachPath(good.OID): "tampered"}, func() *Ledger { return bo }, "changes or removes"},
		{"stray path", map[string]string{"notes/" + strings.Repeat("b", 64): "n"}, func() *Ledger { return bo }, "not an attachments/<sha256> path"},
		{"short oid path", map[string]string{attachDir + "/abc": "n"}, func() *Ledger { return bo }, "not an attachments/<sha256> path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { // reset the ref for the next case, even after a Fatal
				if tip := refTip(t, ada, ref); tip != "" {
					if err := ada.git.deleteRef(context.Background(), ref, tip); err != nil {
						t.Error(err)
					}
				}
			})
			base := forgeAttach(t, bo, ref, refTip(t, bo, ref), map[string]string{attachPath(good.OID): "good"}, bo)
			badOID := forgeAttach(t, bo, ref, base, tc.entries, tc.signer())
			after := blob("after " + tc.name)
			forgeAttach(t, bo, ref, badOID, map[string]string{attachPath(after.OID): string(after.Data)}, bo)

			rep := reloadOK(t, ada)
			ws := warningsFor(rep.Warnings, ref)
			if len(ws) != 1 || ws[0].Kind != WarnBadAttachment || !strings.Contains(ws[0].Detail, tc.wantBad) || !strings.HasPrefix(ws[0].Detail, badOID[:12]) {
				t.Fatalf("warnings = %+v, want one WarnBadAttachment on %s saying %q", ws, badOID[:12], tc.wantBad)
			}
			if !ada.HasAttachment(good.OID) {
				t.Fatal("the blob before the bad commit is not readable")
			}
			if ada.HasAttachment(after.OID) {
				t.Fatal("a blob after the bad commit is readable")
			}
		})
	}
}

// TestAttachWrongContentInAnotherTreeDoesNotShadow: a structurally valid
// commit of another member holding different content at the oid's path is
// skipped; the read serves the copy that hashes to the oid.
func TestAttachWrongContentInAnotherTreeDoesNotShadow(t *testing.T) {
	w := newWorld(t)
	ada := w.create("ada", "c1")
	bo := w.join("bo", "c1")
	b := blob("real")
	appendBlob(t, ada, b)
	syncOK(t, ada)
	forgeAttach(t, bo, attachRef("bo", "c1"), "", map[string]string{attachPath(b.OID): "fake"}, bo)
	reloadOK(t, bo)
	for _, prefer := range []string{"bo", "ada", ""} {
		if got := readOK(t, bo, b.OID, prefer); string(got) != "real" {
			t.Fatalf("prefer %q: read = %q, want the real content", prefer, got)
		}
	}
	if locs := bo.attachIndex[b.OID]; len(locs) != 2 {
		t.Fatalf("index holds %d copies, want both trees", len(locs))
	}
}

// fileRemote returns a file:// URL for a new bare remote; allowFilter sets
// uploadpack.allowFilter on it so it honors partial-clone filters.
func (w *world) fileRemote(name string, allowFilter bool) (url, dir string) {
	w.t.Helper()
	dir = w.newRemote(name)
	if allowFilter {
		remoteGit(w.t, dir, "config", "uploadpack.allowFilter", "true")
	}
	return "file://" + dir, dir
}

// TestAttachPartialCloneFetchesBlobOnRead: over a remote that honors the
// filter, a clone that fetched a member's attachment ref lacks its blobs
// until a read asks for one, which fetches that blob alone; every event
// blob is present; the filter is not left in the clone's config.
func TestAttachPartialCloneFetchesBlobOnRead(t *testing.T) {
	w := newWorld(t)
	url, _ := w.fileRemote("r", true)
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	b1, b2 := blob("one"), blob("two")
	id := appendBlob(t, ada, b1)
	appendBlob(t, ada, b2)
	if rep := syncOK(t, ada); !rep.Pushed {
		t.Fatalf("ada sync = %+v, want pushed", rep)
	}

	bo := w.register(w.remoteOpts("bo", "c1"), url)
	ctx := context.Background()
	wantVerdict(t, bo, id, Valid)
	if rep := fetchOK(t, bo); hasWarning(rep.Warnings, attachPrefix+"*", WarnNoFilter) {
		t.Fatalf("a filter-honoring remote produced the no-filter warning: %+v", rep.Warnings)
	}
	if out, err := bo.git.run(ctx, "config", "--get", "remote.origin.partialclonefilter"); err == nil {
		t.Fatalf("partialclonefilter stayed set: %q", out)
	}
	locs := bo.attachIndex
	if len(locs[b1.OID]) != 1 || len(locs[b2.OID]) != 1 {
		t.Fatalf("bo's index = %v, want both oids", locs)
	}
	missing := func(b Blob) bool {
		_, ok, err := bo.git.readBlob(ctx, locs[b.OID][0].blob)
		if err != nil {
			t.Fatal(err)
		}
		return !ok
	}
	if !missing(b1) || !missing(b2) {
		t.Fatal("the filtered fetch brought the attachment blobs")
	}
	if got := readOK(t, bo, b1.OID, "ada"); string(got) != "one" {
		t.Fatalf("read = %q", got)
	}
	if missing(b1) {
		t.Fatal("the read did not fetch its blob")
	}
	if !missing(b2) {
		t.Fatal("the read fetched a blob it did not need")
	}
}

// TestAttachNoFilterRemoteWarnsOnce: a remote that ignores the filter
// fetches the attachment refs whole; the store reports it once.
func TestAttachNoFilterRemoteWarnsOnce(t *testing.T) {
	w := newWorld(t)
	url, _ := w.fileRemote("r", false)
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	b := blob("whole")
	appendBlob(t, ada, b)
	syncOK(t, ada)

	bo := w.register(w.remoteOpts("bo", "c1"), url)
	rep := fetchOK(t, bo)
	if n := len(warningsFor(rep.Warnings, attachPrefix+"*")); n != 1 || !hasWarning(rep.Warnings, attachPrefix+"*", WarnNoFilter) {
		t.Fatalf("first report after the fallback = %+v, want one WarnNoFilter", rep.Warnings)
	}
	if rep := fetchOK(t, bo); hasWarning(rep.Warnings, attachPrefix+"*", WarnNoFilter) {
		t.Fatal("the no-filter warning repeated")
	}
	if _, ok, _ := bo.git.readBlob(context.Background(), bo.attachIndex[b.OID][0].blob); !ok {
		t.Fatal("the whole fetch did not bring the blob")
	}
	if got := readOK(t, bo, b.OID, "ada"); string(got) != "whole" {
		t.Fatalf("read = %q", got)
	}
}

// recordRunner records every push's arguments and runs a hook before the
// first push of ref.
type recordRunner struct {
	inner  wsgit.Runner
	ref    string
	once   sync.Once
	before func()
	mu     sync.Mutex
	pushes [][]string
}

func (r *recordRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	if isPushOf(args, r.ref) {
		r.mu.Lock()
		r.pushes = append(r.pushes, append([]string(nil), args...))
		r.mu.Unlock()
		if r.before != nil {
			r.once.Do(r.before)
		}
	}
	return r.inner.RunGit(ctx, root, args...)
}

// TestAttachAtomicPushRejectionMovesNeither: the attachment ref and the
// event ref go in one atomic push, attachment first; when the event ref's
// lease fails, the remote takes neither.
func TestAttachAtomicPushRejectionMovesNeither(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	own, aref := memberRef("ada", "c1"), attachRef("ada", "c1")
	rr := &recordRunner{inner: wsgit.ExecRunner{Env: GitEnv}, ref: own}
	o := w.remoteOpts("ada", "c1")
	o.Runner = rr
	ada := w.register(o, url) // pushes the join; recorded, then dropped
	rr.pushes = nil
	rr.before = func() {
		// The remote's event ref is moved under the lease.
		remoteGit(t, url, "update-ref", own, remoteRefs(t, url)[genesisRef])
	}
	appendBlob(t, ada, blob("atomic"))
	rep := syncOK(t, ada)
	if rep.Pushed {
		t.Fatalf("sync = %+v, want not pushed", rep)
	}
	refs := remoteRefs(t, url)
	if refs[aref] != "" {
		t.Fatal("the attachment ref landed although the event ref was rejected")
	}
	if len(rr.pushes) != 1 {
		t.Fatalf("pushes = %d, want one", len(rr.pushes))
	}
	args := rr.pushes[0]
	ai, ei := -1, -1
	for i, a := range args {
		switch a {
		case aref + ":" + aref:
			ai = i
		case own + ":" + own:
			ei = i
		}
	}
	if ai < 0 || ei < 0 || ai > ei {
		t.Fatalf("push args %v, want the attachment refspec before the event refspec", args)
	}
	hasAtomic := false
	for _, a := range args {
		hasAtomic = hasAtomic || a == "--atomic"
	}
	if !hasAtomic {
		t.Fatalf("push args %v lack --atomic", args)
	}
	if refTip(t, ada, aref) == "" {
		t.Fatal("the local attachment ref is gone")
	}
}

// TestAttachRewoundRefRefused: a rewind of an attachment ref on the remote
// is refused like a rewind of an event ref: the local copy stays, the
// snapshot and known namespaces show the remote's value and the last
// known one.
func TestAttachRewoundRefRefused(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	aref := attachRef("ada", "c1")

	appendBlob(t, ada, blob("v1"))
	syncOK(t, ada)
	fetchOK(t, bo)
	first := remoteRefs(t, url)[aref]
	appendBlob(t, ada, blob("v2"))
	syncOK(t, ada)
	fetchOK(t, bo)
	second := remoteRefs(t, url)[aref]
	if first == "" || second == "" || first == second {
		t.Fatalf("attachment ref tips %q, %q", first, second)
	}

	remoteGit(t, url, "update-ref", aref, first)
	rep := fetchOK(t, bo)
	if !hasWarning(rep.Warnings, aref, WarnRewound) {
		t.Fatalf("warnings = %+v, want WarnRewound on %s", rep.Warnings, aref)
	}
	if got := refTip(t, bo, aref); got != second {
		t.Fatalf("local attachment ref = %s, want the kept %s", got, second)
	}
	if got := refTip(t, bo, snapRef(aref)); got != first {
		t.Fatalf("snapshot = %s, want the remote's %s", got, first)
	}
	if got := refTip(t, bo, knownRef(aref)); got != second {
		t.Fatalf("known = %s, want %s", got, second)
	}
	if !bo.HasAttachment(blob("v2").OID) {
		t.Fatal("the kept copy's blob left the index")
	}

	// Restore of the event ref puts the attachment ref back even when the
	// event ref itself is intact on the remote.
	own := memberRef("ada", "c1")
	if err := bo.Restore(context.Background(), own); err != nil {
		t.Fatalf("restore over an attachment-only rewind: %v", err)
	}
	if got := remoteRefs(t, url)[aref]; got != second {
		t.Fatalf("after restore remote %s=%s, want %s", aref, got, second)
	}

	// Both deleted: one restore puts both back.
	remoteGit(t, url, "update-ref", "-d", own)
	remoteGit(t, url, "update-ref", "-d", aref)
	fetchOK(t, bo)
	if err := bo.Restore(context.Background(), own); err != nil {
		t.Fatalf("restore: %v", err)
	}
	refs := remoteRefs(t, url)
	if refs[aref] != second || refs[own] == "" {
		t.Fatalf("after restore remote %s=%s %s=%s, want both back", aref, refs[aref], own, refs[own])
	}
}

// TestAttachRestoreWithoutBlobsRestoresEventRefAlone: a clone that fetched
// another member's attachment ref filtered lacks its blobs and cannot push
// that ref back; restoring the member's event ref restores the event ref
// alone and reports the attachment ref as not restored. The member's own
// clone, which holds the blobs, restores the attachment ref afterwards.
func TestAttachRestoreWithoutBlobsRestoresEventRefAlone(t *testing.T) {
	w := newWorld(t)
	url, dir := w.fileRemote("r", true)
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	appendBlob(t, ada, blob("kept by ada"))
	syncOK(t, ada)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	own, aref := memberRef("ada", "c1"), attachRef("ada", "c1")
	if _, ok, _ := bo.git.readBlob(context.Background(), bo.attachIndex[blob("kept by ada").OID][0].blob); ok {
		t.Fatal("the filtered fetch brought the blob; the test needs it absent")
	}
	wantOwn, wantAttach := remoteRefs(t, dir)[own], remoteRefs(t, dir)[aref]

	remoteGit(t, dir, "update-ref", "-d", own)
	remoteGit(t, dir, "update-ref", "-d", aref)
	fetchOK(t, bo)
	if err := bo.Restore(context.Background(), own); err != nil {
		t.Fatalf("restore without the attachment blobs: %v", err)
	}
	refs := remoteRefs(t, dir)
	if refs[own] != wantOwn || refs[aref] != "" {
		t.Fatalf("after restore remote %s=%s %s=%s, want the event ref alone back", own, refs[own], aref, refs[aref])
	}
	rep := fetchOK(t, bo)
	if !hasWarning(rep.Warnings, aref, WarnNotRestored) {
		t.Fatalf("warnings = %+v, want WarnNotRestored on %s", rep.Warnings, aref)
	}
	if rep := fetchOK(t, bo); hasWarning(rep.Warnings, aref, WarnNotRestored) {
		t.Fatal("the not-restored warning repeated")
	}

	fetchOK(t, ada)
	if err := ada.Restore(context.Background(), own); err != nil {
		t.Fatalf("restore by the clone holding the blobs: %v", err)
	}
	if got := remoteRefs(t, dir)[aref]; got != wantAttach {
		t.Fatalf("after ada's restore remote %s=%s, want %s", aref, got, wantAttach)
	}
}

// TestAttachRestoreWithoutBlobsAttachmentOnly: when the remote still holds
// the member's event ref and only the attachment ref is gone, a restore by
// a clone lacking the blobs has nothing to push, and still reports the
// attachment ref as not restored rather than succeeding silently.
func TestAttachRestoreWithoutBlobsAttachmentOnly(t *testing.T) {
	w := newWorld(t)
	url, dir := w.fileRemote("r", true)
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	appendBlob(t, ada, blob("kept by ada"))
	syncOK(t, ada)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	own, aref := memberRef("ada", "c1"), attachRef("ada", "c1")
	if _, ok, _ := bo.git.readBlob(context.Background(), bo.attachIndex[blob("kept by ada").OID][0].blob); ok {
		t.Fatal("the filtered fetch brought the blob; the test needs it absent")
	}
	wantOwn := remoteRefs(t, dir)[own]

	remoteGit(t, dir, "update-ref", "-d", aref)
	fetchOK(t, bo)
	if err := bo.Restore(context.Background(), own); err != nil {
		t.Fatalf("restore over an attachment-only deletion without the blobs: %v", err)
	}
	refs := remoteRefs(t, dir)
	if refs[own] != wantOwn || refs[aref] != "" {
		t.Fatalf("after restore remote %s=%s %s=%s, want the event ref untouched and the attachment ref still gone", own, refs[own], aref, refs[aref])
	}
	if rep := fetchOK(t, bo); !hasWarning(rep.Warnings, aref, WarnNotRestored) {
		t.Fatalf("warnings = %+v, want WarnNotRestored on %s", rep.Warnings, aref)
	}
}

// TestAttachFetchFailureLeavesEventsUnabsorbed: when the attachment-ref
// fetch fails the event refs are not fetched, so no absorbed event names a
// blob no tree holds.
func TestAttachFetchFailureLeavesEventsUnabsorbed(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	o := w.remoteOpts("bo", "c1")
	fr := &failFetchRunner{} // n 0: nothing fails during Register
	o.Runner = fr
	bo := w.register(o, url)
	fr.n = fr.fetches + 1 // the next fetch round fails

	id := appendBlob(t, ada, blob("late"))
	syncOK(t, ada)
	if _, err := bo.Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "fetch broke") {
		t.Fatalf("fetch = %v, want the attachment fetch's failure", err)
	}
	if _, ok := eventsByID(bo)[id]; ok {
		t.Fatal("the event was absorbed although the attachment fetch failed")
	}
	fetchOK(t, bo)
	wantVerdict(t, bo, id, Valid)
	if got := readOK(t, bo, blob("late").OID, "ada"); string(got) != "late" {
		t.Fatalf("read = %q", got)
	}
}

// TestAttachMoveToFreshCloneCarriesPendingBlobs: pending events moved to a
// fresh clone take their blobs along; the fresh clone's first batch close
// commits them to its own attachment ref.
func TestAttachMoveToFreshCloneCarriesPendingBlobs(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	x := w.register(w.remoteOpts("bo", "c1"), url)
	appendOne(t, x, "note", note("x1"))
	syncOK(t, x)
	oy := w.remoteOpts("bo", "c1")
	oy.Dir = filepath.Join(w.root, "stores", "bo-c1-twin")
	y := w.register(oy, url)

	b := blob("moved")
	appendBlob(t, x, b)
	appendOne(t, y, "note", note("y1"))
	syncOK(t, y)
	if rep := syncOK(t, x); rep.Pushed || !rep.Pending.ForeignAdvance {
		t.Fatalf("x sync = %+v, want a foreign advance", rep)
	}
	fresh, err := x.MoveToFreshClone(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep := syncOK(t, x); !rep.Pushed {
		t.Fatalf("sync after the move = %+v, want pushed", rep)
	}
	refs := remoteRefs(t, url)
	if refs[attachRef("bo", fresh)] == "" {
		t.Fatal("the fresh clone's attachment ref is not on the remote")
	}
	if refs[attachRef("bo", "c1")] != "" {
		t.Fatal("the old clone's attachment ref reached the remote")
	}
	fetchOK(t, y)
	if got := readOK(t, y, b.OID, "bo"); string(got) != "moved" {
		t.Fatalf("y read = %q", got)
	}
}

// failObjectFetchRunner fails every explicit object fetch (ReadAttachment's
// on-demand fetch) while fail is set.
type failObjectFetchRunner struct {
	mu   sync.Mutex
	fail bool
}

func (r *failObjectFetchRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	r.mu.Lock()
	fail := r.fail
	r.mu.Unlock()
	if fail && isFetch(args) && oidRe.MatchString(args[len(args)-1]) {
		return nil, errors.New("fatal: unable to access the remote: connection reset")
	}
	return wsgit.ExecRunner{Env: GitEnv}.RunGit(ctx, root, args...)
}

// TestAttachFetchFailureIsNotNotFound: a blob this clone lacks whose
// explicit fetch fails is the fetch's error, not ErrAttachmentNotFound
// (the remote may hold it); once the fetch works the blob is served.
func TestAttachFetchFailureIsNotNotFound(t *testing.T) {
	w := newWorld(t)
	url, _ := w.fileRemote("r", true)
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	b := blob("far away")
	appendBlob(t, ada, b)
	syncOK(t, ada)
	o := w.remoteOpts("bo", "c1")
	fr := &failObjectFetchRunner{fail: true}
	o.Runner = fr
	bo := w.register(o, url)

	_, err := bo.ReadAttachment(context.Background(), b.OID, "ada")
	if err == nil || errors.Is(err, ErrAttachmentNotFound) || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("read over a failing fetch = %v, want the fetch's error and not ErrAttachmentNotFound", err)
	}
	fr.mu.Lock()
	fr.fail = false
	fr.mu.Unlock()
	if got := readOK(t, bo, b.OID, "ada"); string(got) != "far away" {
		t.Fatalf("read = %q", got)
	}
}
