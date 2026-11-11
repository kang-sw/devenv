package wsledger

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// netGauge counts the fetches and pushes in flight on one store directory,
// across every runner that shares it, and keeps the most it saw at once.
type netGauge struct {
	mu       sync.Mutex
	inFlight int
	max      int
	total    int
}

func (g *netGauge) enter() {
	g.mu.Lock()
	g.inFlight++
	g.total++
	if g.inFlight > g.max {
		g.max = g.inFlight
	}
	g.mu.Unlock()
}

func (g *netGauge) leave() {
	g.mu.Lock()
	g.inFlight--
	g.mu.Unlock()
}

func (g *netGauge) read() (max, total int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.max, g.total
}

// holdRunner is the real git runner with one command it stops at, once
// armed, until released: a remote that hangs mid-transfer, or a local step
// caught halfway. The command match selects is held before git runs, or,
// with after set, once git has run, so its result is already in the clone
// while the caller still waits. It holds one command per arming. gauge,
// when set, counts every fetch and push that passes through.
type holdRunner struct {
	match   func(args []string) bool
	after   bool
	gauge   *netGauge
	armed   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func newHoldRunner(match func(args []string) bool) *holdRunner {
	return &holdRunner{match: match, entered: make(chan struct{}, 1), release: make(chan struct{})}
}

func (r *holdRunner) hold(ctx context.Context) {
	r.entered <- struct{}{}
	select {
	case <-r.release:
	case <-ctx.Done():
	}
}

func (r *holdRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	if r.gauge != nil && (isFetch(args) || isPush(args)) {
		r.gauge.enter()
		defer r.gauge.leave()
	}
	held := r.match != nil && r.match(args) && r.armed.CompareAndSwap(true, false)
	if held && !r.after {
		r.hold(ctx)
	}
	out, err := wsgit.ExecRunner{Env: GitEnv}.RunGit(ctx, root, args...)
	if held && r.after {
		r.hold(ctx)
	}
	return out, err
}

func isPush(args []string) bool {
	for _, a := range args {
		if a == "push" {
			return true
		}
	}
	return false
}

// isEventFetch selects the fetch of the event refs: the one that brings
// other writers' events into the snapshot namespace.
func isEventFetch(args []string) bool {
	return isFetch(args) && strings.HasPrefix(args[len(args)-1], "+refs/heads/tracker/")
}

// stillRunning fails the test when done delivers within a moment: the call
// behind it must be waiting.
func stillRunning[T any](t *testing.T, done <-chan T, what string) {
	t.Helper()
	select {
	case <-done:
		t.Fatalf("%s returned while it had to wait", what)
	case <-time.After(150 * time.Millisecond):
	}
}

// TestFetchInFlightHoldsNoLocalCall: while a fetch waits on the remote, its
// result already in the snapshot refs, every local call on the store goes
// through at once, on the fetching Ledger and on a second one over the same
// directory (another process): a reload, an append, a read. None of them
// sees the fetch before its apply step; the apply then merges the appends
// made meanwhile, and the next sync pushes them.
func TestFetchInFlightHoldsNoLocalCall(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	ob := w.remoteOpts("bo", "c1")
	hr := newHoldRunner(isEventFetch)
	hr.after = true
	ob.Runner = hr
	bo := w.register(ob, url)
	twin, err := Open(context.Background(), w.remoteOpts("bo", "c1"))
	if err != nil {
		t.Fatalf("open the second instance: %v", err)
	}
	t.Cleanup(func() { twin.Close() })
	syncOK(t, ada) // ada learns bo's join

	far := appendOne(t, ada, "note", note("from ada"))
	syncOK(t, ada)

	ctx := context.Background()
	hr.armed.Store(true)
	fetched := make(chan error, 1)
	go func() {
		_, err := bo.Fetch(ctx)
		fetched <- err
	}()
	<-hr.entered

	// The local calls carry a deadline only so a regression fails the test
	// instead of hanging it: none of them may wait for the held fetch.
	local, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := twin.Reload(local); err != nil {
		t.Fatalf("reload in another process under a held fetch: %v", err)
	}
	if _, err := bo.Reload(local); err != nil {
		t.Fatalf("reload under a held fetch: %v", err)
	}
	for _, l := range []*Ledger{bo, twin} {
		if _, ok := eventsByID(l)[far]; ok {
			t.Fatal("a reload saw the fetch before its apply step")
		}
	}
	own, err := bo.Append(local, []NewEvent{{Type: "note", Payload: []byte(`{"n":"own"}`)}})
	if err != nil {
		t.Fatalf("append under a held fetch: %v", err)
	}
	theirs, err := twin.Append(local, []NewEvent{{Type: "note", Payload: []byte(`{"n":"twin"}`)}})
	if err != nil {
		t.Fatalf("append in another process under a held fetch: %v", err)
	}
	stillRunning(t, fetched, "the held fetch")

	close(hr.release)
	if err := <-fetched; err != nil {
		t.Fatalf("fetch: %v", err)
	}
	wantVerdict(t, bo, far, Valid)
	got := eventsByID(bo)
	for _, id := range []EventID{own[0], theirs[0]} {
		if _, ok := got[id]; !ok {
			t.Fatalf("the append %s made during the fetch is gone after its apply step", id)
		}
	}
	if rep := syncOK(t, bo); !rep.Pushed || rep.Pending.Count != 0 {
		t.Fatalf("sync after the fetch = %+v, want the appends pushed", rep)
	}
	fetchOK(t, ada)
	for _, id := range []EventID{own[0], theirs[0]} {
		wantVerdict(t, ada, id, Valid)
	}
}

// TestPushInFlightHoldsNoLocalCall: the same while a sync's push waits on
// the remote. An append made during the push is not part of it, survives
// the sync as a pending write, and goes out with the next one.
func TestPushInFlightHoldsNoLocalCall(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	ob := w.remoteOpts("bo", "c1")
	hr := newHoldRunner(func(args []string) bool { return isPushOf(args, memberRef("bo", "c1")) })
	ob.Runner = hr
	bo := w.register(ob, url)
	twin, err := Open(context.Background(), w.remoteOpts("bo", "c1"))
	if err != nil {
		t.Fatalf("open the second instance: %v", err)
	}
	t.Cleanup(func() { twin.Close() })
	syncOK(t, ada)

	first := appendOne(t, bo, "note", note("pushed"))
	ctx := context.Background()
	hr.armed.Store(true)
	type result struct {
		rep SyncReport
		err error
	}
	synced := make(chan result, 1)
	go func() {
		rep, err := bo.Sync(ctx)
		synced <- result{rep, err}
	}()
	<-hr.entered

	local, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := twin.Reload(local); err != nil {
		t.Fatalf("reload in another process under a held push: %v", err)
	}
	late, err := twin.Append(local, []NewEvent{{Type: "note", Payload: []byte(`{"n":"late"}`)}})
	if err != nil {
		t.Fatalf("append in another process under a held push: %v", err)
	}
	if p := bo.Pending(); p.Count == 0 {
		t.Fatalf("pending under a held push = %+v, want the write being pushed", p)
	}
	stillRunning(t, synced, "the held sync")

	close(hr.release)
	r := <-synced
	if r.err != nil {
		t.Fatalf("sync: %v", r.err)
	}
	if !r.rep.Pushed || r.rep.Pending.Count != 1 {
		t.Fatalf("sync = %+v, want pushed with the late append pending", r.rep)
	}
	fetchOK(t, ada)
	wantVerdict(t, ada, first, Valid)
	if _, ok := eventsByID(ada)[late[0]]; ok {
		t.Fatal("the append made during the push went out with it")
	}
	if rep := syncOK(t, bo); !rep.Pushed || rep.Pending.Count != 0 {
		t.Fatalf("next sync = %+v, want the late append pushed", rep)
	}
	fetchOK(t, ada)
	wantVerdict(t, ada, late[0], Valid)
}

// TestFetchAppliesWhole: the step that applies a fetch moves every local
// ref under the store lock in one piece. With the apply caught halfway (one
// writer's ref moved, the next one's not yet), a reload from a second
// Ledger over the directory waits, and when it returns it holds both
// writers' events: never the one without the other.
func TestFetchAppliesWhole(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	cy := w.register(w.remoteOpts("cy", "c1"), url)
	ob := w.remoteOpts("bo", "c1")
	// The apply step takes refs in name order: ada's, then cy's.
	cyRef := memberRef("cy", "c1")
	hr := newHoldRunner(func(args []string) bool {
		return len(args) > 3 && args[0] == "update-ref" && args[3] == cyRef
	})
	ob.Runner = hr
	bo := w.register(ob, url)
	twin, err := Open(context.Background(), w.remoteOpts("bo", "c1"))
	if err != nil {
		t.Fatalf("open the second instance: %v", err)
	}
	t.Cleanup(func() { twin.Close() })
	converge(t, ada, cy)

	fromAda := appendOne(t, ada, "note", note("a"))
	syncOK(t, ada)
	fromCy := appendOne(t, cy, "note", note("c"))
	syncOK(t, cy)

	ctx := context.Background()
	adaRef := memberRef("ada", "c1")
	before := refTip(t, bo, adaRef)
	hr.armed.Store(true)
	fetched := make(chan error, 1)
	go func() {
		_, err := bo.Fetch(ctx)
		fetched <- err
	}()
	<-hr.entered
	if refTip(t, twin, adaRef) == before {
		t.Fatal("fixture: the apply step must be caught after it moved the first ref")
	}

	reloaded := make(chan error, 1)
	go func() {
		_, err := twin.Reload(ctx)
		reloaded <- err
	}()
	stillRunning(t, reloaded, "a reload during the apply step")

	close(hr.release)
	if err := <-fetched; err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := <-reloaded; err != nil {
		t.Fatalf("reload: %v", err)
	}
	wantVerdict(t, twin, fromAda, Valid)
	wantVerdict(t, twin, fromCy, Valid)
}

// TestNetworkLockSerializesAcrossLedgers: two Ledgers over one store
// directory (two processes) never talk to the remote at once. While one
// holds a fetch open, the other's fetch and sync run no remote command; a
// call bounded by WithNetWait answers ErrBusy, on either Ledger, having
// fetched nothing; and once the first is released, both finish.
func TestNetworkLockSerializesAcrossLedgers(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	gauge := &netGauge{}
	ob := w.remoteOpts("bo", "c1")
	hr := newHoldRunner(isEventFetch)
	hr.gauge = gauge
	ob.Runner = hr
	bo := w.register(ob, url)
	ot := w.remoteOpts("bo", "c1")
	ot.Runner = &holdRunner{gauge: gauge}
	twin, err := Open(context.Background(), ot)
	if err != nil {
		t.Fatalf("open the second instance: %v", err)
	}
	t.Cleanup(func() { twin.Close() })
	syncOK(t, ada)
	far := appendOne(t, ada, "note", note("from ada"))
	syncOK(t, ada)

	ctx := context.Background()
	hr.armed.Store(true)
	fetched := make(chan error, 1)
	go func() {
		_, err := bo.Fetch(ctx)
		fetched <- err
	}()
	<-hr.entered
	_, held := gauge.read()

	bounded := WithNetWait(ctx, 30*time.Millisecond)
	if _, err := twin.Fetch(bounded); !errors.Is(err, ErrBusy) {
		t.Fatalf("bounded fetch in another process under a held fetch = %v, want ErrBusy", err)
	}
	if _, err := bo.Sync(bounded); !errors.Is(err, ErrBusy) {
		t.Fatalf("bounded sync on the fetching Ledger = %v, want ErrBusy", err)
	}
	// A context that ends first answers its own error, not ErrBusy.
	canceled, cancel := context.WithCancel(WithNetWait(ctx, time.Hour))
	cancel()
	if _, err := twin.Fetch(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled bounded fetch = %v, want context.Canceled", err)
	}

	own := appendOne(t, twin, "note", note("from the twin"))
	synced := make(chan error, 1)
	go func() {
		_, err := twin.Sync(ctx)
		synced <- err
	}()
	stillRunning(t, synced, "a sync in another process under a held fetch")
	if _, total := gauge.read(); total != held {
		t.Fatalf("%d remote commands started while the fetch was held, want none", total-held)
	}

	close(hr.release)
	if err := <-fetched; err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := <-synced; err != nil {
		t.Fatalf("sync: %v", err)
	}
	if max, _ := gauge.read(); max != 1 {
		t.Fatalf("%d fetches or pushes ran at once on one store directory, want one at a time", max)
	}
	wantVerdict(t, bo, far, Valid)
	fetchOK(t, ada)
	wantVerdict(t, ada, own, Valid)
}

// TestApplyGateRefusalDiscardsTheFetch: a gate that does not run a step
// stops the call with its error. A refused fetch has moved the snapshot
// refs and nothing a reader looks at; a refused sync has pushed nothing.
// With the gate open again the same calls go through.
func TestApplyGateRefusalDiscardsTheFetch(t *testing.T) {
	w := newWorld(t)
	url := w.newRemote("r")
	ada := w.register(w.remoteOpts("ada", "c1"), url)
	ob := w.remoteOpts("bo", "c1")
	errGone := errors.New("the store is gone")
	var refuse atomic.Bool
	var steps atomic.Int64
	ob.ApplyGate = func(step func() error) error {
		if refuse.Load() {
			return errGone
		}
		steps.Add(1)
		return step()
	}
	bo := w.register(ob, url)
	syncOK(t, ada)
	if steps.Load() != 0 {
		t.Fatal("a registration runs no step through the gate")
	}

	far := appendOne(t, ada, "note", note("from ada"))
	syncOK(t, ada)
	ctx := context.Background()
	adaRef, boRef := memberRef("ada", "c1"), memberRef("bo", "c1")
	before := refTip(t, bo, adaRef)

	// Refused from the apply step on: the fetch itself runs.
	var opened atomic.Bool
	ob2 := ob
	ob2.ApplyGate = func(step func() error) error {
		if opened.CompareAndSwap(false, true) {
			return step()
		}
		return errGone
	}
	late, err := Open(ctx, ob2)
	if err != nil {
		t.Fatalf("open the second instance: %v", err)
	}
	t.Cleanup(func() { late.Close() })
	if _, err := late.Fetch(ctx); !errors.Is(err, errGone) {
		t.Fatalf("fetch whose apply step is refused = %v, want the gate's error", err)
	}
	if got := refTip(t, bo, snapPrefix+strings.TrimPrefix(adaRef, "refs/heads/")); got == before || got == "" {
		t.Fatalf("fixture: the refused fetch must have fetched; snapshot ref %q, local %q", got, before)
	}
	if got := refTip(t, bo, adaRef); got != before {
		t.Fatalf("a refused apply step moved %s from %s to %s", adaRef, before, got)
	}
	if _, err := bo.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := eventsByID(bo)[far]; ok {
		t.Fatal("a reader sees an event whose apply step was refused")
	}

	refuse.Store(true)
	own := appendOne(t, bo, "note", note("from bo"))
	remoteBefore := remoteRefs(t, url)[boRef]
	if _, err := bo.Sync(ctx); !errors.Is(err, errGone) {
		t.Fatalf("refused sync = %v, want the gate's error", err)
	}
	if _, err := bo.Fetch(ctx); !errors.Is(err, errGone) {
		t.Fatalf("refused fetch = %v, want the gate's error", err)
	}
	if got := remoteRefs(t, url)[boRef]; got != remoteBefore {
		t.Fatal("a refused sync pushed")
	}

	refuse.Store(false)
	if rep := syncOK(t, bo); !rep.Pushed {
		t.Fatalf("sync with the gate open = %+v, want pushed", rep)
	}
	wantVerdict(t, bo, far, Valid)
	fetchOK(t, ada)
	wantVerdict(t, ada, own, Valid)
}
