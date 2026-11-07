package wsledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// remoteRefs lists every ref of the bare remote: name -> oid.
func remoteRefs(t *testing.T, remote string) map[string]string {
	t.Helper()
	out, err := exec.Command("git", "-C", remote, "for-each-ref", "--format=%(objectname) %(refname)").CombinedOutput()
	if err != nil {
		t.Fatalf("for-each-ref %s: %v: %s", remote, err, out)
	}
	refs := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if oid, name, ok := strings.Cut(line, " "); ok {
			refs[name] = oid
		}
	}
	return refs
}

// remoteGit runs git directly on the bare remote.
func remoteGit(t *testing.T, remote string, args ...string) {
	t.Helper()
	argv := append([]string{"-C", remote}, args...)
	if out, err := exec.Command("git", argv...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// hookRunner runs before() once, just before the first push of ref, then
// lets the push proceed. It forces a registration race deterministically:
// the hook completes the rival's registration while this client has already
// decided what to write but has not published it.
type hookRunner struct {
	inner  wsgit.Runner
	ref    string
	once   sync.Once
	before func()
}

func (h *hookRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	if isPushOf(args, h.ref) {
		h.once.Do(h.before)
	}
	return h.inner.RunGit(ctx, root, args...)
}

func isPushOf(args []string, ref string) bool {
	if len(args) == 0 || strings.TrimPrefix(args[len(args)-1], "+") != ref+":"+ref {
		return false
	}
	for _, a := range args {
		if a == "push" {
			return true
		}
	}
	return false
}

func note(n string) map[string]string { return map[string]string{"n": n} }

func payloadNote(t *testing.T, e Event) string {
	t.Helper()
	var p map[string]string
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		t.Fatalf("payload of %s: %v", e.ID, err)
	}
	return p["n"]
}

func idSet(l *Ledger) map[EventID]bool {
	out := map[EventID]bool{}
	for id := range eventsByID(l) {
		out[id] = true
	}
	return out
}

func hasWarning(ws []Warning, ref, kind string) bool {
	for _, w := range ws {
		if w.Ref == ref && w.Kind == kind {
			return true
		}
	}
	return false
}

func warningsFor(ws []Warning, refs ...string) []Warning {
	var out []Warning
	for _, w := range ws {
		for _, r := range refs {
			if w.Ref == r {
				out = append(out, w)
			}
		}
	}
	return out
}

// converge syncs then fetches every client, so each has pushed and seen
// everyone's writes, and checks they hold one view.
func converge(t *testing.T, ls ...*Ledger) {
	t.Helper()
	for _, l := range ls {
		syncOK(t, l)
	}
	for _, l := range ls {
		fetchOK(t, l)
	}
	first := ls[0]
	for _, l := range ls[1:] {
		if a, b := idSet(first), idSet(l); !reflect.DeepEqual(a, b) {
			t.Fatalf("event id sets differ:\n%s: %v\n%s: %v", first.stream, a, l.stream, b)
		}
		if a, b := first.Identity(), l.Identity(); !reflect.DeepEqual(a, b) {
			t.Fatalf("identities differ:\n%s: %+v\n%s: %+v", first.stream, a, l.stream, b)
		}
	}
	for _, l := range ls {
		for id, e := range eventsByID(l) {
			if e.Verdict != Valid {
				t.Fatalf("%s: event %s verdict %s (%s), want Valid", l.stream, id, e.Verdict, e.Reason)
			}
		}
	}
}

func memberIDs(ident IdentityState) []string {
	var out []string
	for _, m := range ident.Members {
		out = append(out, m.ID)
	}
	return out
}

func TestRemoteRegisterEmptyThenJoin(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)

	refs := remoteRefs(t, url)
	for _, ref := range []string{genesisRef, claimRef("ada"), memberRef("ada", "c1")} {
		if refs[ref] == "" {
			t.Fatalf("remote lacks %s after the creating registration: %v", ref, refs)
		}
	}
	if refs[claimRef("ada")] != refs[memberRef("ada", "c1")] {
		t.Fatalf("creator claim %s is not its join commit %s", refs[claimRef("ada")], refs[memberRef("ada", "c1")])
	}
	join := streamEvents(ada, "ada.c1")
	if len(join) != 1 || join[0].Type != TypeMemberJoin || join[0].Verdict != Valid {
		t.Fatalf("creator stream = %+v, want one valid member.join", join)
	}

	bo := w.register(w.remoteOpts("bo", "c1"), url)
	refs = remoteRefs(t, url)
	if refs[claimRef("bo")] == "" || refs[memberRef("bo", "c1")] == "" {
		t.Fatalf("remote lacks bo's claim or ref after the joining registration: %v", refs)
	}
	converge(t, ada, bo)
	if got := memberIDs(ada.Identity()); !reflect.DeepEqual(got, []string{"ada", "bo"}) {
		t.Fatalf("members = %v, want [ada bo]", got)
	}
	if got := ada.Identity().Admins; !reflect.DeepEqual(got, []string{"ada"}) {
		t.Fatalf("admins = %v, want [ada]", got)
	}
}

func TestRemoteTwoJoinsOneID(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)

	o1 := w.optsWithKey("bo", "c1", filepath.Join(w.root, "keys", "boA"))
	o1.RepoDir = ""
	o2 := w.optsWithKey("bo", "c2", filepath.Join(w.root, "keys", "boB"))
	o2.RepoDir = ""
	bo1 := w.register(o1, url)
	firstClaim := remoteRefs(t, url)[claimRef("bo")]
	if firstClaim == "" {
		t.Fatal("first join's claim not on the remote")
	}

	l, err := Register(context.Background(), o2, url)
	if !errors.Is(err, ErrIDTaken) {
		if l != nil {
			l.Close()
		}
		t.Fatalf("second join of bo: err = %v, want ErrIDTaken", err)
	}
	refs := remoteRefs(t, url)
	if refs[claimRef("bo")] != firstClaim {
		t.Fatalf("remote claim of bo = %s, want the first join's %s", refs[claimRef("bo")], firstClaim)
	}
	if oid, ok := refs[memberRef("bo", "c2")]; ok {
		t.Fatalf("the loser's member ref reached the remote at %s", oid)
	}
	converge(t, ada, bo1)
}

func TestRemoteSecondCloneOfMember(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	before := remoteRefs(t, url)

	ada2 := w.register(w.remoteOpts("ada", "c2"), url)
	after := remoteRefs(t, url)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("second clone's registration changed the remote:\nbefore %v\nafter  %v", before, after)
	}
	if n := len(streamEvents(ada2, "ada.c2")); n != 0 {
		t.Fatalf("second clone wrote %d events, want none", n)
	}

	id := appendOne(t, ada2, "note", note("from c2"))
	if rep := syncOK(t, ada2); !rep.Pushed {
		t.Fatalf("second clone's first sync = %+v, want pushed", rep)
	}
	after = remoteRefs(t, url)
	if tip := refTip(t, ada2, memberRef("ada", "c2")); tip == "" || after[memberRef("ada", "c2")] != tip {
		t.Fatalf("remote ada.c2 = %s, want local tip %s", after[memberRef("ada", "c2")], tip)
	}
	if after[claimRef("ada")] != before[claimRef("ada")] {
		t.Fatal("the second clone moved the claim ref")
	}
	fetchOK(t, ada)
	wantVerdict(t, ada, id, Valid)
	converge(t, ada, ada2)
}

func TestRemoteGenesisRace(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	var ada *Ledger
	ob := w.remoteOpts("bo", "c1")
	ob.Runner = &hookRunner{inner: wsgit.ExecRunner{}, ref: genesisRef, before: func() {
		ada = w.register(w.remoteOpts("ada", "c1"), url)
	}}
	bo := w.register(ob, url)
	if ada == nil {
		t.Fatal("the race hook never ran: bo did not try to push a genesis")
	}

	refs := remoteRefs(t, url)
	if refs[genesisRef] != refTip(t, ada, genesisRef) {
		t.Fatalf("remote genesis %s is not ada's %s", refs[genesisRef], refTip(t, ada, genesisRef))
	}
	if refTip(t, bo, genesisRef) != refs[genesisRef] {
		t.Fatal("bo kept its own losing genesis")
	}
	if refs[claimRef("bo")] == "" || refs[memberRef("bo", "c1")] == "" {
		t.Fatalf("bo did not continue as a joiner: remote %v", refs)
	}
	converge(t, ada, bo)
	ident := bo.Identity()
	if !reflect.DeepEqual(ident.Admins, []string{"ada"}) || !reflect.DeepEqual(memberIDs(ident), []string{"ada", "bo"}) {
		t.Fatalf("identity after the genesis race = %+v, want admin ada, members ada and bo", ident)
	}
}

func TestRemoteClaimRace(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)

	oa := w.optsWithKey("bo", "c1", filepath.Join(w.root, "keys", "boA"))
	oa.RepoDir = ""
	ob := w.optsWithKey("bo", "c2", filepath.Join(w.root, "keys", "boB"))
	ob.RepoDir = ""
	var boA *Ledger
	ob.Runner = &hookRunner{inner: wsgit.ExecRunner{}, ref: claimRef("bo"), before: func() {
		boA = w.register(oa, url)
	}}
	l, err := Register(context.Background(), ob, url)
	if boA == nil {
		t.Fatal("the race hook never ran: the second join did not try to push its claim")
	}
	if !errors.Is(err, ErrIDTaken) {
		if l != nil {
			l.Close()
		}
		t.Fatalf("racing join of bo: err = %v, want ErrIDTaken", err)
	}
	refs := remoteRefs(t, url)
	if refs[claimRef("bo")] != refTip(t, boA, claimRef("bo")) {
		t.Fatal("the remote claim of bo is not the winner's")
	}
	if oid, ok := refs[memberRef("bo", "c2")]; ok {
		t.Fatalf("the loser's member ref reached the remote at %s", oid)
	}
	converge(t, ada, boA)
}

func TestRemoteLongOfflineCatchUp(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	ob := w.remoteOpts("bo", "c1")
	bo := w.register(ob, url)
	if err := bo.Close(); err != nil {
		t.Fatal(err)
	}

	var want []EventID
	for round := 0; round < 4; round++ {
		for i := 0; i < 5; i++ {
			want = append(want, appendOne(t, ada, "note", note(fmt.Sprintf("r%d-%d", round, i))))
		}
		if rep := syncOK(t, ada); !rep.Pushed {
			t.Fatalf("round %d sync = %+v, want pushed", round, rep)
		}
	}

	bo, err := Open(context.Background(), ob)
	if err != nil {
		t.Fatalf("reopen bo: %v", err)
	}
	t.Cleanup(func() { bo.Close() })
	rep := fetchOK(t, bo)
	if rep.Received < len(want) {
		t.Fatalf("fetch received %d events, want at least %d", rep.Received, len(want))
	}
	evs, _ := bo.Events(Cursor{})
	var got []EventID
	for _, e := range evs {
		if e.ID.Stream() == "ada.c1" && e.Type == "note" {
			if e.Verdict != Valid {
				t.Fatalf("%s verdict %s (%s), want Valid", e.ID, e.Verdict, e.Reason)
			}
			got = append(got, e.ID)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("offline clone sees ada's notes %v, want %v in order", got, want)
	}
}

func TestRemoteDeletedRefsRestored(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	appendOne(t, ada, "note", note("a1"))
	syncOK(t, ada)
	fetchOK(t, bo)

	adaRef, adaClaim := memberRef("ada", "c1"), claimRef("ada")
	before := remoteRefs(t, url)
	remoteGit(t, url, "update-ref", "-d", adaRef)
	remoteGit(t, url, "update-ref", "-d", adaClaim)

	rep := fetchOK(t, bo)
	for _, ref := range []string{adaRef, adaClaim} {
		if !hasWarning(rep.Warnings, ref, WarnDeleted) {
			t.Fatalf("fetch warnings %+v lack %s for %s", rep.Warnings, WarnDeleted, ref)
		}
		if refTip(t, bo, ref) != before[ref] {
			t.Fatalf("bo's local copy of %s moved to %q", ref, refTip(t, bo, ref))
		}
	}
	for _, ref := range []string{adaRef, adaClaim} {
		if err := bo.Restore(context.Background(), ref); err != nil {
			t.Fatalf("restore %s: %v", ref, err)
		}
	}
	after := remoteRefs(t, url)
	for _, ref := range []string{adaRef, adaClaim} {
		if after[ref] != before[ref] {
			t.Fatalf("remote %s = %q after restore, want %s", ref, after[ref], before[ref])
		}
	}
	if ws := warningsFor(fetchOK(t, bo).Warnings, adaRef, adaClaim); len(ws) != 0 {
		t.Fatalf("fetch after restore still warns: %+v", ws)
	}

	// A deleted own ref blocks the own push until Restore.
	own := memberRef("bo", "c1")
	remoteGit(t, url, "update-ref", "-d", own)
	if rep := fetchOK(t, bo); !hasWarning(rep.Warnings, own, WarnDeleted) {
		t.Fatalf("fetch warnings %+v lack %s for the own ref", rep.Warnings, WarnDeleted)
	}
	id := appendOne(t, bo, "note", note("b1"))
	rep = syncOK(t, bo)
	if rep.Pushed || !hasWarning(rep.Warnings, own, WarnDeleted) {
		t.Fatalf("sync with the own ref deleted = %+v, want not pushed and a deleted warning", rep)
	}
	if _, ok := remoteRefs(t, url)[own]; ok {
		t.Fatal("the deleted own ref reappeared on the remote before Restore")
	}
	if err := bo.Restore(context.Background(), own); err != nil {
		t.Fatalf("restore own: %v", err)
	}
	if got, tip := remoteRefs(t, url)[own], refTip(t, bo, own); got != tip {
		t.Fatalf("remote own ref = %q after restore, want %s", got, tip)
	}
	rep = fetchOK(t, bo)
	if ws := warningsFor(rep.Warnings, own); len(ws) != 0 {
		t.Fatalf("fetch after restoring the own ref still warns: %+v", ws)
	}
	if rep.Pending.Count != 0 {
		t.Fatalf("pending after restore = %+v, want nothing", rep.Pending)
	}
	fetchOK(t, ada)
	wantVerdict(t, ada, id, Valid)
}

func TestRemoteForeignAdvanceMoveToFreshClone(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	x := w.register(w.remoteOpts("bo", "c1"), url)
	appendOne(t, x, "note", note("x1"))
	if rep := syncOK(t, x); !rep.Pushed {
		t.Fatalf("x first sync = %+v, want pushed", rep)
	}

	// y: the same member, key, and explicit clone id from another directory.
	oy := w.remoteOpts("bo", "c1")
	oy.Dir = filepath.Join(w.root, "stores", "bo-c1-twin")
	y := w.register(oy, url)

	t0 := w.clock.Now().UTC()
	appendOne(t, x, "note", note("x2"))
	appendOne(t, x, "note", note("x3"))
	appendOne(t, y, "note", note("y1"))
	if rep := syncOK(t, y); !rep.Pushed {
		t.Fatalf("y sync = %+v, want pushed", rep)
	}
	w.clock.Advance(time.Minute)

	own := memberRef("bo", "c1")
	knownBefore := remoteRefs(t, url)[own]
	rep := syncOK(t, x)
	if rep.Pushed || !rep.Pending.ForeignAdvance || !hasWarning(rep.Warnings, own, WarnForeignAdvance) {
		t.Fatalf("x sync over a foreign advance = %+v, want not pushed, ForeignAdvance, and a warning", rep)
	}
	if !x.Pending().ForeignAdvance {
		t.Fatal("Pending().ForeignAdvance not set")
	}
	if got := remoteRefs(t, url)[own]; got != knownBefore {
		t.Fatalf("remote own ref moved to %s, want y's %s", got, knownBefore)
	}

	fresh, err := x.MoveToFreshClone(context.Background())
	if err != nil {
		t.Fatalf("MoveToFreshClone: %v", err)
	}
	if fresh == "" || fresh == "c1" || x.CloneID() != fresh {
		t.Fatalf("fresh clone id %q (CloneID %q), want a new id", fresh, x.CloneID())
	}
	moved := streamEvents(x, "bo."+fresh)
	if len(moved) != 2 {
		t.Fatalf("fresh stream holds %+v, want x2 and x3", moved)
	}
	for i, wantN := range []string{"x2", "x3"} {
		if got := payloadNote(t, moved[i]); got != wantN || !moved[i].TS.Equal(t0) {
			t.Fatalf("moved event %d = %s ts %s, want %s ts %s", i, got, moved[i].TS, wantN, t0)
		}
	}
	if rep := syncOK(t, x); !rep.Pushed || rep.Pending.ForeignAdvance || rep.Pending.Count != 0 {
		t.Fatalf("sync after the move = %+v, want pushed with nothing pending", rep)
	}
	if got := remoteRefs(t, url)[memberRef("bo", fresh)]; got == "" {
		t.Fatal("the fresh ref is not on the remote")
	}

	converge(t, ada, x, y)
	for _, l := range []*Ledger{ada, x, y} {
		count := map[string]int{}
		for _, e := range eventsByID(l) {
			if e.Type == "note" {
				count[payloadNote(t, e)]++
			}
		}
		want := map[string]int{"x1": 1, "x2": 1, "x3": 1, "y1": 1}
		if !reflect.DeepEqual(count, want) {
			t.Fatalf("%s: note payload counts %v, want %v", l.stream, count, want)
		}
	}
}

func TestRemoteForeignAdvanceWithNothingPending(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	w.register(w.remoteOpts("ada", "c1"), url)
	x := w.register(w.remoteOpts("bo", "c1"), url)
	oy := w.remoteOpts("bo", "c1")
	oy.Dir = filepath.Join(w.root, "stores", "bo-c1-twin")
	y := w.register(oy, url)

	appendOne(t, y, "note", note("y1"))
	if rep := syncOK(t, y); !rep.Pushed {
		t.Fatalf("y sync = %+v, want pushed", rep)
	}

	own := memberRef("bo", "c1")
	localBefore := refTip(t, x, own)
	rep := fetchOK(t, x)
	if rep.Pending.Count != 0 {
		t.Fatalf("x pending = %+v, want nothing", rep.Pending)
	}
	if !rep.Pending.ForeignAdvance || !hasWarning(rep.Warnings, own, WarnForeignAdvance) {
		t.Fatalf("x fetch = %+v, want ForeignAdvance and a warning", rep)
	}
	if got := refTip(t, x, own); got != localBefore {
		t.Fatalf("x took the remote's commits onto its own ref: %s -> %s", localBefore, got)
	}
	rep = syncOK(t, x)
	if rep.Pushed || !rep.Pending.ForeignAdvance {
		t.Fatalf("x sync = %+v, want not pushed and still ForeignAdvance", rep)
	}
	if got := refTip(t, x, own); got != localBefore {
		t.Fatalf("x sync took the remote's commits onto its own ref: %s -> %s", localBefore, got)
	}
	for _, e := range eventsByID(x) {
		if e.Type == "note" {
			t.Fatalf("x sees y's event %s through its own ref", e.ID)
		}
	}
}

func TestRemoteRefForceMovedToNonDescendant(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	adaRef := memberRef("ada", "c1")

	appendOne(t, ada, "note", note("a1"))
	syncOK(t, ada)
	older := refTip(t, ada, adaRef)
	a2 := appendOne(t, ada, "note", note("a2"))
	syncOK(t, ada)
	fetchOK(t, bo)
	localBefore := refTip(t, bo, adaRef)
	if localBefore == older {
		t.Fatal("setup: bo did not receive ada's second commit")
	}

	remoteGit(t, url, "update-ref", adaRef, older)
	rep := fetchOK(t, bo)
	if !hasWarning(rep.Warnings, adaRef, WarnRewound) {
		t.Fatalf("fetch warnings %+v lack %s for %s", rep.Warnings, WarnRewound, adaRef)
	}
	if got := refTip(t, bo, adaRef); got != localBefore {
		t.Fatalf("bo's local copy moved %s -> %s", localBefore, got)
	}
	wantVerdict(t, bo, a2, Valid)
}

func TestRemoteFetchPushesNothing(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	own := memberRef("ada", "c1")
	before := remoteRefs(t, url)[own]

	appendOne(t, ada, "note", note("a1"))
	rep := fetchOK(t, ada)
	if rep.Pushed {
		t.Fatalf("Fetch reported a push: %+v", rep)
	}
	if got := remoteRefs(t, url)[own]; got != before {
		t.Fatalf("remote own ref moved on Fetch: %s -> %s", before, got)
	}
	if rep.Pending.Count != 1 {
		t.Fatalf("pending after Fetch = %+v, want one unpushed event", rep.Pending)
	}
}

func TestRemotePushDelayRestartsOnWrite(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	own := memberRef("ada", "c1")
	before := remoteRefs(t, url)[own]

	t0 := w.clock.Now().UTC()
	appendOne(t, ada, "note", note("a1"))
	if due := ada.Pending().PushDue; !due.Equal(t0.Add(DefaultPushDelay)) {
		t.Fatalf("PushDue after the first write = %s, want %s", due, t0.Add(DefaultPushDelay))
	}
	w.clock.Advance(4 * time.Minute)
	appendOne(t, ada, "note", note("a2"))
	wantDue := t0.Add(4*time.Minute + DefaultPushDelay)
	if due := ada.Pending().PushDue; !due.Equal(wantDue) {
		t.Fatalf("PushDue after the second write = %s, want %s", due, wantDue)
	}
	w.clock.Advance(4 * time.Minute)
	if got := remoteRefs(t, url)[own]; got != before {
		t.Fatal("pushed at the first write's deadline although a later write restarted the delay")
	}
	if p := ada.Pending(); p.Count != 2 {
		t.Fatalf("pending before the deadline = %+v, want two events", p)
	}
	w.clock.Advance(time.Minute + time.Second)
	if got, tip := remoteRefs(t, url)[own], refTip(t, ada, own); got == before || got != tip {
		t.Fatalf("remote own ref after the deadline = %s, want the local tip %s", got, tip)
	}
	if p := ada.Pending(); p.Count != 0 || !p.PushDue.IsZero() {
		t.Fatalf("pending after the timed push = %+v, want nothing and no deadline", p)
	}
}

func TestRemotePendingJournalSurvivesReopen(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	oa := w.remoteOpts("ada", "c1")
	ada := w.register(oa, url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	own := memberRef("ada", "c1")
	before := remoteRefs(t, url)[own]

	t0 := w.clock.Now().UTC()
	id := appendOne(t, ada, "note", note("a1"))
	if err := ada.Close(); err != nil {
		t.Fatal(err)
	}
	w.clock.Advance(2 * time.Minute)
	if got := remoteRefs(t, url)[own]; got != before {
		t.Fatal("a closed store pushed")
	}

	reopenAt := w.clock.Now().UTC()
	ada, err := Open(context.Background(), oa)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { ada.Close() })
	p := ada.Pending()
	if p.Count != 1 || !p.Oldest.Equal(t0) {
		t.Fatalf("pending after reopen = %+v, want one event written at %s", p, t0)
	}
	if want := reopenAt.Add(DefaultPushDelay); !p.PushDue.Equal(want) {
		t.Fatalf("PushDue after reopen = %s, want %s", p.PushDue, want)
	}

	if rep := syncOK(t, ada); !rep.Pushed || rep.Pending.Count != 0 {
		t.Fatalf("sync after reopen = %+v, want pushed with nothing pending", rep)
	}
	if got, tip := remoteRefs(t, url)[own], refTip(t, ada, own); got != tip {
		t.Fatalf("remote own ref = %s, want %s", got, tip)
	}
	fetchOK(t, bo)
	if e := wantVerdict(t, bo, id, Valid); !e.TS.Equal(t0) {
		t.Fatalf("pushed event ts = %s, want the original %s", e.TS, t0)
	}
}

// TestRemoteSyncDoesNotUndoOwnRefRemoval covers a Sync that runs before any
// Fetch noticed the removal (the push-delay timer runs Sync unattended): a
// deleted or rewound own ref is not pushed back; only Restore does that.
func TestRemoteSyncDoesNotUndoOwnRefRemoval(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	own := memberRef("ada", "c1")

	appendOne(t, ada, "note", note("a1"))
	syncOK(t, ada)
	first := remoteRefs(t, url)[own]
	appendOne(t, ada, "note", note("a2"))
	syncOK(t, ada)

	// Rewound behind what ada pushed, then a new write and a Sync.
	remoteGit(t, url, "update-ref", own, first)
	appendOne(t, ada, "note", note("a3"))
	rep := syncOK(t, ada)
	if rep.Pushed || !hasWarning(rep.Warnings, own, WarnRewound) {
		t.Fatalf("sync after a remote rewind = %+v, want not pushed and a rewound warning", rep)
	}
	if got := remoteRefs(t, url)[own]; got != first {
		t.Fatalf("remote own ref = %s, want it left at the rewound %s", got, first)
	}

	// Deleted, then a Sync with no Fetch in between.
	if err := ada.Restore(context.Background(), own); err != nil {
		t.Fatalf("restore: %v", err)
	}
	remoteGit(t, url, "update-ref", "-d", own)
	appendOne(t, ada, "note", note("a4"))
	rep = syncOK(t, ada)
	if rep.Pushed || !hasWarning(rep.Warnings, own, WarnDeleted) {
		t.Fatalf("sync after a remote deletion = %+v, want not pushed and a deleted warning", rep)
	}
	if _, ok := remoteRefs(t, url)[own]; ok {
		t.Fatal("Sync pushed the deleted own ref back")
	}
}

// TestRemoteRestoreRefusesARefMovedMeanwhile: Restore leases on the value
// its own fetch saw, so a ref that moves on the remote between that fetch
// and the push is left alone.
func TestRemoteRestoreRefusesARefMovedMeanwhile(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	adaRef := memberRef("ada", "c1")
	appendOne(t, ada, "note", note("a1"))
	syncOK(t, ada)
	first := remoteRefs(t, url)[adaRef]
	appendOne(t, ada, "note", note("a2"))
	syncOK(t, ada)

	ob := w.remoteOpts("bo", "c1")
	ob.Runner = &hookRunner{inner: wsgit.ExecRunner{}, ref: adaRef, before: func() {
		remoteGit(t, url, "update-ref", adaRef, first)
	}}
	bo := w.register(ob, url)
	remoteGit(t, url, "update-ref", "-d", adaRef)
	if rep := fetchOK(t, bo); !hasWarning(rep.Warnings, adaRef, WarnDeleted) {
		t.Fatalf("fetch warnings %+v lack %s for %s", rep.Warnings, WarnDeleted, adaRef)
	}
	if err := bo.Restore(context.Background(), adaRef); err == nil {
		t.Fatal("Restore pushed over a ref that moved on the remote after its fetch")
	}
	if got := remoteRefs(t, url)[adaRef]; got != first {
		t.Fatalf("remote %s = %s, want the value set meanwhile %s", adaRef, got, first)
	}
}

// TestRemoteRestoreRefusesARemoteAhead: Restore replaces a deletion or a
// rewind; a remote copy that moved ahead of the local one is left alone.
func TestRemoteRestoreRefusesARemoteAhead(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	bo := w.register(w.remoteOpts("bo", "c1"), url)
	adaRef := memberRef("ada", "c1")
	appendOne(t, ada, "note", note("a1"))
	syncOK(t, ada)
	fetchOK(t, bo)
	appendOne(t, ada, "note", note("a2"))
	syncOK(t, ada)
	ahead := remoteRefs(t, url)[adaRef]

	if err := bo.Restore(context.Background(), adaRef); err == nil {
		t.Fatal("Restore pushed a stale local copy over a remote that moved ahead")
	}
	if got := remoteRefs(t, url)[adaRef]; got != ahead {
		t.Fatalf("remote %s = %s, want it left at %s", adaRef, got, ahead)
	}
	if rep := syncOK(t, ada); len(warningsFor(rep.Warnings, adaRef)) != 0 {
		t.Fatalf("ada's sync warns about its own ref: %+v", rep.Warnings)
	}
}

// TestRemoteRefusedPushFailsSync: a remote that refuses the push itself (a
// hook, branch protection, a missing permission) fails Sync and Register
// rather than passing for a lost race; the writes stay pending.
func TestRemoteRefusedPushFailsSync(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	hook := filepath.Join(url, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho refused by policy >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	appendOne(t, ada, "note", note("a1"))
	if _, err := ada.Sync(context.Background()); err == nil {
		t.Fatal("Sync against a remote that refuses the push returned no error")
	}
	if p := ada.Pending(); p.Count != 1 {
		t.Fatalf("pending after a refused push = %+v, want the write still pending", p)
	}
	if l, err := Register(context.Background(), w.remoteOpts("bo", "c1"), url); err == nil {
		l.Close()
		t.Fatal("Register against a remote that refuses the push returned no error")
	}

	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if rep := syncOK(t, ada); !rep.Pushed || rep.Pending.Count != 0 {
		t.Fatalf("sync after the remote accepts again = %+v, want pushed and nothing pending", rep)
	}
}
