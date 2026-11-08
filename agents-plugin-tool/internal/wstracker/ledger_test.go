package wstracker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsledger"
)

// TestMain isolates every git invocation of the real-ledger test from the
// developer's global and system config (signing defaults, hooks).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wstracker-gitconfig")
	if err != nil {
		panic(err)
	}
	global := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(global, []byte("[init]\n\tdefaultBranch = main\n"), 0o644); err != nil {
		panic(err)
	}
	os.Setenv("GIT_CONFIG_GLOBAL", global)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type ledgerClock struct{ now time.Time }

func (c *ledgerClock) Now() time.Time { return c.now }
func (c *ledgerClock) AfterFunc(d time.Duration, f func()) wsledger.Timer {
	return time.AfterFunc(time.Hour, func() {}) // no remote: the push delay never runs
}

// TestRealLedgerTwoWriters is the real-ledger coverage: two writers over one
// temporary bare repository with real ssh-keygen keys, made through Create
// and Join (veto set), dispatching and syncing; both derive the same view state, and
// the protocol flags a removal the (209) veto rejects.
func TestRealLedgerTwoWriters(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	clock := &ledgerClock{now: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)}
	opts := func(member string) wsledger.Options {
		return wsledger.Options{Dir: filepath.Join(root, "stores", member), RepoDir: filepath.Join(root, "shared.git"),
			Member: member, Name: member, KeyDir: filepath.Join(root, "keys", member), CloneID: "c1", Clock: clock}
	}
	// The wrappers install the veto the vetoed removal below relies on.
	adaL, err := Create(ctx, opts("ada"))
	if err != nil {
		t.Fatal(err)
	}
	defer adaL.Close()
	boL, err := Join(ctx, opts("bo"))
	if err != nil {
		t.Fatal(err)
	}
	defer boL.Close()
	// ada's store was open while bo joined in the shared repository.
	if _, err := adaL.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	ada, bo := New(adaL, "ada.c1"), New(boL, "bo.c1")

	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	dispatch := func(trk *Tracker, typ string, payload any) wsledger.EventID {
		t.Helper()
		clock.now = clock.now.Add(time.Minute)
		ids, err := trk.Dispatch(ctx, []wsledger.NewEvent{{Type: typ, Payload: raw(payload)}})
		if err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		return ids[0]
	}
	sync := func(l *wsledger.Ledger) {
		t.Helper()
		if _, err := l.Sync(ctx); err != nil {
			t.Fatal(err)
		}
	}

	dispatch(ada, TypeSetLead, map[string]any{"member": "bo", "leads": []string{"ada"}})
	item := dispatch(ada, TypeItemCreate, map[string]any{"kind": "task", "title": "Ship the tracker"})
	assign := dispatch(ada, TypeItemAssign, map[string]any{"item": item.String(), "mode": "chosen", "people": []string{"bo"}})
	sync(adaL)
	sync(boL)
	dispatch(bo, TypeItemStatus, map[string]any{"item": item.String(), "status": "doing", "note": "on it"})
	dispatch(bo, TypeCommentAdd, map[string]any{"item": item.String(), "body": "first cut is up"})
	sync(boL)
	sync(adaL)

	now := clock.now
	va, vb := Derive(adaL, "ada.c1", now, 0), Derive(boL, "ada.c1", now, 0)
	if viewJSON(t, va) != viewJSON(t, vb) {
		t.Fatal("the two writers derive different view states")
	}
	if viewJSON(t, ada.View(now, 0)) != viewJSON(t, va) {
		t.Fatal("the tracker's view differs from a fresh derive")
	}
	a := asg(t, va, assign.String()+">bo")
	wantEq(t, "bo's assignment", a.State, "live")
	wantEq(t, "forced (ada leads bo)", a.Forced, true)
	wantEq(t, "bo's status", a.Status, "doing")
	wantEq(t, "events", va.Stats.Events, len(va.Events))
	for _, e := range va.Events {
		if e.Status != "applied" {
			t.Fatalf("%s %s: %s %v", e.ID, e.Type, e.Status, deref(e.Reason))
		}
	}

	// (209): ada below bo, then removing bo. Dispatch refuses it; written
	// past Dispatch, the protocol flags it through the veto on both clones.
	dispatch(ada, TypeSetLead, map[string]any{"member": "bo", "leads": []string{}})
	dispatch(ada, TypeSetLead, map[string]any{"member": "ada", "leads": []string{"bo"}})
	want := "bo still leads ada; a removed member must lead nobody (move them with roles.set_lead first)"
	_, err = ada.Dispatch(ctx, []wsledger.NewEvent{{Type: wsledger.TypeRemoveMember, Payload: raw(map[string]any{"member": "bo"})}})
	var ref *Refusal
	if !errors.As(err, &ref) || ref.Reason != want {
		t.Fatalf("Dispatch of the removal: %v", err)
	}
	ids, err := adaL.Append(ctx, []wsledger.NewEvent{{Type: wsledger.TypeRemoveMember, Payload: raw(map[string]any{"member": "bo"})}})
	if err != nil {
		t.Fatal(err)
	}
	sync(adaL)
	sync(boL)
	for _, trk := range []*Tracker{ada, bo} {
		e := event(t, trk.View(now, 0), ids[0].String())
		if e.Status != "flagged" || deref(e.Reason) != want {
			t.Fatalf("%s: the removal is %s %q", trk.stream, e.Status, deref(e.Reason))
		}
	}
	if viewJSON(t, ada.View(now, 0)) != viewJSON(t, Derive(boL, "ada.c1", now, 0)) {
		t.Fatal("the writers disagree after the vetoed removal")
	}
}

// TestRegisterInstallsVeto: Register over a bare remote (one creating, one
// joining) returns stores with the (209) veto installed, so a removal
// written past Dispatch is flagged on both clones.
func TestRegisterInstallsVeto(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	if out, err := exec.Command("git", "init", "--quiet", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("init remote: %v: %s", err, out)
	}
	clock := &ledgerClock{now: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)}
	opts := func(member string) wsledger.Options {
		return wsledger.Options{Dir: filepath.Join(root, "stores", member), Member: member, Name: member,
			KeyDir: filepath.Join(root, "keys", member), CloneID: "c1", Clock: clock}
	}
	adaL, err := Register(ctx, opts("ada"), remote)
	if err != nil {
		t.Fatalf("register ada: %v", err)
	}
	defer adaL.Close()
	boL, err := Register(ctx, opts("bo"), remote)
	if err != nil {
		t.Fatalf("register bo: %v", err)
	}
	defer boL.Close()
	if _, err := adaL.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	ada := New(adaL, "ada.c1")
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	if _, err := ada.Dispatch(ctx, []wsledger.NewEvent{{Type: TypeSetLead, Payload: raw(map[string]any{"member": "ada", "leads": []string{"bo"}})}}); err != nil {
		t.Fatal(err)
	}
	ids, err := adaL.Append(ctx, []wsledger.NewEvent{{Type: wsledger.TypeRemoveMember, Payload: raw(map[string]any{"member": "bo"})}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adaL.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := boL.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	for _, l := range []*wsledger.Ledger{adaL, boL} {
		evs, _ := l.Events(wsledger.Cursor{})
		found := false
		for _, e := range evs {
			if e.ID == ids[0] {
				found = true
				if e.Verdict != wsledger.Flagged || !strings.Contains(e.Reason, "bo still leads ada") {
					t.Fatalf("%s.%s: the removal is %s %q, want flagged by the veto", l.MemberID(), l.CloneID(), e.Verdict, e.Reason)
				}
			}
		}
		if !found {
			t.Fatalf("%s.%s: the removal is not in the view", l.MemberID(), l.CloneID())
		}
	}
}

// TestRefreshAfterMoveToFreshCloneHoldsNoDuplicate: MoveToFreshClone drops
// the old stream's unpushed item.create from the view and re-issues it
// under the fresh clone id. The tracker's cursor read must not keep the
// dropped copy, so the folded view holds the item once.
func TestRefreshAfterMoveToFreshCloneHoldsNoDuplicate(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	if out, err := exec.Command("git", "init", "--quiet", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("init remote: %v: %s", err, out)
	}
	clock := &ledgerClock{now: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)}
	opts := func(member, dir string) wsledger.Options {
		return wsledger.Options{Dir: filepath.Join(root, "stores", dir), Member: member, Name: member,
			KeyDir: filepath.Join(root, "keys", member), CloneID: "c1", Clock: clock}
	}
	adaL, err := Register(ctx, opts("ada", "ada"), remote)
	if err != nil {
		t.Fatalf("register ada: %v", err)
	}
	defer adaL.Close()
	x, err := Register(ctx, opts("bo", "bo"), remote)
	if err != nil {
		t.Fatalf("register bo: %v", err)
	}
	defer x.Close()
	// y: the same member, key, and clone id from another directory, so its
	// push makes the remote copy of bo.c1 foreign to x.
	y, err := Register(ctx, opts("bo", "bo-twin"), remote)
	if err != nil {
		t.Fatalf("register twin: %v", err)
	}
	defer y.Close()
	if _, err := y.Append(ctx, []wsledger.NewEvent{{Type: "note", Payload: json.RawMessage(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := y.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	trk := New(x, "bo.c1")
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	if _, err := trk.Dispatch(ctx, []wsledger.NewEvent{{Type: TypeItemCreate, Payload: raw(map[string]any{"kind": "task", "title": "Ship the tracker"})}}); err != nil {
		t.Fatal(err)
	}
	rep, err := x.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Pushed || !rep.Pending.ForeignAdvance {
		t.Fatalf("x sync = %+v, want a foreign advance and nothing pushed", rep)
	}
	if n := len(trk.View(clock.now, 0).Items); n != 1 {
		t.Fatalf("items before the move = %d, want 1", n)
	}

	if _, err := x.MoveToFreshClone(ctx); err != nil {
		t.Fatalf("MoveToFreshClone: %v", err)
	}
	v := trk.View(clock.now, 0)
	if len(v.Items) != 1 {
		t.Fatalf("items after the move = %v, want the moved item once", v.ItemOrder)
	}
	for id := range v.Items {
		if strings.HasPrefix(id, "bo.c1#") {
			t.Fatalf("the view still holds the dropped old-stream item %s", id)
		}
	}
}

// failSecondOpen fails the second `git version`: every ledger open checks
// the git version once, so the first open (wsledger.Register) runs and the
// reopen that installs the veto fails.
type failSecondOpen struct {
	mu       sync.Mutex
	versions int
}

func (r *failSecondOpen) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	if len(args) == 1 && args[0] == "version" {
		r.mu.Lock()
		r.versions++
		n := r.versions
		r.mu.Unlock()
		if n == 2 {
			return nil, errors.New("reopen broke")
		}
	}
	return wsgit.ExecRunner{}.RunGit(ctx, root, args...)
}

// TestRegisterReopenFailureIsPushed: wsledger.Register succeeds, so its push
// landed; a failure of the reopen with the veto installed is then a
// *wsledger.PushedError carrying the store's genesis.
func TestRegisterReopenFailureIsPushed(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	if out, err := exec.Command("git", "init", "--quiet", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("init remote: %v: %s", err, out)
	}
	clock := &ledgerClock{now: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)}
	opts := wsledger.Options{Dir: filepath.Join(root, "stores", "ada"), Member: "ada", Name: "ada",
		KeyDir: filepath.Join(root, "keys", "ada"), CloneID: "c1", Clock: clock, Runner: &failSecondOpen{}}
	l, err := Register(ctx, opts, remote)
	if err == nil {
		l.Close()
		t.Fatal("Register with a failing reopen returned no error")
	}
	var pushed *wsledger.PushedError
	if !errors.As(err, &pushed) || !strings.Contains(err.Error(), "reopen broke") {
		t.Fatalf("a reopen failure after the push = %v, want a PushedError", err)
	}
	if len(pushed.Genesis) == 0 {
		t.Fatal("the PushedError carries no genesis")
	}
}
