// Polling, pre-read, sync, and view tests.
package wstrackerhost

import (
	"context"
	"encoding/json"
	"github.com/gofrs/flock"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestPollSyncsTwoClones: the poll, which consults no tracker knob, brings
// one clone's pushed write to the other, and holds the poll lock while it
// runs.
func TestPollSyncsTwoClones(t *testing.T) {
	w := newWorld(t)
	a, b := w.machine("a"), w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	item := createItem(t, a, "team", "ship the caller")
	syncStore(t, a, "team")

	if b.PollerRunning() {
		t.Fatal("precondition: nothing polls")
	}
	stop := b.StartPoll(20*time.Millisecond, nil)
	deadline := time.Now().Add(20 * time.Second)
	for !hasItem(view(t, b, "team"), item) {
		if time.Now().After(deadline) {
			stop()
			t.Fatal("the poll never fetched ada's item")
		}
		time.Sleep(20 * time.Millisecond)
	}
	other, err := New(Options{Root: b.Root(), Clock: w.clock}) // another process on b's machine
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if !other.PollerRunning() || a.PollerRunning() {
		t.Fatal("the poll lock must read as held on the polling machine's root only")
	}
	stop()
	if b.PollerRunning() {
		t.Fatal("stopping the poll must release its lock")
	}
}

// TestPollOnceReportsOnlyAChange: a poll round that fetches another
// clone's new write reports a change; a round after it that fetched
// nothing new and kept the sync standing reports none, though it stamped a
// new attempt and success time.
func TestPollOnceReportsOnlyAChange(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	a, b := w.machine("a"), w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	b.PollOnce(ctx) // settle b's own first fetch

	item := createItem(t, a, "team", "seen by the poll")
	syncStore(t, a, "team")
	if !b.PollOnce(ctx) {
		t.Fatal("a round that fetched a new item reported no change")
	}
	if !hasItem(view(t, b, "team"), item) {
		t.Fatal("the round did not bring the item")
	}
	w.clock.Advance(time.Minute)
	if b.PollOnce(ctx) {
		t.Fatal("a round that fetched nothing new reported a change")
	}
}

// TestPollRunsAtOnce: the poll lock turns every pre-read into a no-op, so
// the poll fetches as soon as it takes the lock rather than one interval
// later; otherwise reads stay stale for a whole interval after it starts.
func TestPollRunsAtOnce(t *testing.T) {
	w := newWorld(t)
	a, b := w.machine("a"), w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	item := createItem(t, a, "team", "fresh at start")
	syncStore(t, a, "team")

	stop := b.StartPoll(time.Hour, nil)
	defer stop()
	deadline := time.Now().Add(20 * time.Second)
	for !hasItem(view(t, b, "team"), item) {
		if time.Now().After(deadline) {
			t.Fatal("the poll must fetch when it takes the lock, not one interval later")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestPreReadWithoutDaemon: with no poller, the pre-read check fetches,
// throttled per store by the check interval from the last attempt.
func TestPreReadWithoutDaemon(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	a, b := w.machine("a"), w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	w.clock.Advance(DefaultCheckInterval)

	first := createItem(t, a, "team", "one")
	syncStore(t, a, "team")
	if hasItem(view(t, b, "team"), first) {
		t.Fatal("precondition: a read alone never fetches")
	}
	if err := b.PreRead(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	v := view(t, b, "team")
	if !hasItem(v, first) || v.Sync.SyncedAt == nil || !v.Sync.Online {
		t.Fatalf("after the pre-read check: item %v, sync %+v", hasItem(v, first), v.Sync)
	}

	second := createItem(t, a, "team", "two")
	syncStore(t, a, "team")
	w.clock.Advance(DefaultCheckInterval - time.Second)
	if err := b.PreRead(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if hasItem(view(t, b, "team"), second) {
		t.Fatal("a pre-read inside the check interval must not fetch")
	}
	w.clock.Advance(time.Second)
	if err := b.PreRead(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if !hasItem(view(t, b, "team"), second) {
		t.Fatal("a pre-read past the check interval must fetch")
	}

	// A clock stepped back leaves the last attempt in the future, which
	// must not throttle until real time catches up.
	third := createItem(t, a, "team", "three")
	syncStore(t, a, "team")
	w.clock.Advance(-time.Hour)
	if err := b.PreRead(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if !hasItem(view(t, b, "team"), third) {
		t.Fatal("a pre-read after the clock stepped back must fetch")
	}
}

// TestPreReadLeavesNetworkToPoller: while another process holds the poll
// lock, the pre-read check does not fetch; the drawer-open check still
// does, unthrottled.
func TestPreReadLeavesNetworkToPoller(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	a, b := w.machine("a"), w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})

	daemon := flock.New(filepath.Join(b.Root(), pollLockFile))
	if ok, err := daemon.TryLock(); err != nil || !ok {
		t.Fatalf("take the poll lock: %v %v", ok, err)
	}
	defer daemon.Unlock()
	item := createItem(t, a, "team", "one")
	syncStore(t, a, "team")
	w.clock.Advance(time.Hour)
	if err := b.PreRead(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if hasItem(view(t, b, "team"), item) {
		t.Fatal("the pre-read check must leave the network to the poller")
	}
	if _, err := b.Check(ctx, "team"); err != nil {
		t.Fatalf("drawer-open check: %v", err)
	}
	if !hasItem(view(t, b, "team"), item) {
		t.Fatal("the drawer-open check must fetch")
	}
	// Unthrottled: a second check right away fetches again.
	again := createItem(t, a, "team", "two")
	syncStore(t, a, "team")
	if _, err := b.Check(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if !hasItem(view(t, b, "team"), again) {
		t.Fatal("the drawer-open check is never throttled")
	}
}

// TestCheckAllAtSessionStart: the session-start check fetches every
// registered store, throttled by the check interval.
func TestCheckAllAtSessionStart(t *testing.T) {
	w := newWorld(t)
	a, b := w.machine("a"), w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	if _, err := b.Register(context.Background(), RegisterRequest{ID: "mine", Member: "bo", Profile: &wstracker.Profile{Given: "Bo"}}); err != nil {
		t.Fatalf("register a store with no remote: %v", err)
	}
	w.clock.Advance(DefaultCheckInterval)
	item := createItem(t, a, "team", "one")
	syncStore(t, a, "team")
	b.CheckAll(context.Background())
	if !hasItem(view(t, b, "team"), item) {
		t.Fatal("the session-start check must fetch every registered store")
	}
}

// TestPanelCollapseSyncPushes: Sync (the panel-collapse push and the
// explicit apply) publishes this clone's writes at once.
func TestPanelCollapseSyncPushes(t *testing.T) {
	w := newWorld(t)
	a, b := w.machine("a"), w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	item := createItem(t, b, "team", "from bo")
	own := "refs/heads/tracker/m/" + strings.ReplaceAll(view(t, b, "team").Viewer.Stream, ".", "/")
	before := w.remoteTip(own)
	if rep := syncStore(t, b, "team"); !rep.Pushed || rep.Pending.Count != 0 {
		t.Fatalf("sync = %+v, want pushed with nothing pending", rep)
	}
	if w.remoteTip(own) == before {
		t.Fatal("the push did not reach the remote")
	}
	if _, err := a.Check(context.Background(), "team"); err != nil {
		t.Fatal(err)
	}
	if !hasItem(view(t, a, "team"), item) {
		t.Fatal("the other clone must see the pushed write after its check")
	}
}

// TestIdlePushWithoutDaemon: a process that wrote keeps the store open, so
// wsledger's idle push fires with no daemon and no sync call.
func TestIdlePushWithoutDaemon(t *testing.T) {
	w := newWorld(t)
	a, b := w.machine("a"), w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	item := createItem(t, b, "team", "idle")
	own := "refs/heads/tracker/m/" + strings.ReplaceAll(view(t, b, "team").Viewer.Stream, ".", "/")
	before := w.remoteTip(own)
	w.clock.Advance(wsledger.DefaultPushDelay - time.Second)
	if w.remoteTip(own) != before {
		t.Fatal("pushed before the idle delay")
	}
	w.clock.Advance(2 * time.Second)
	if w.remoteTip(own) == before {
		t.Fatal("the idle push did not fire")
	}
	if _, err := a.Check(context.Background(), "team"); err != nil {
		t.Fatal(err)
	}
	if !hasItem(view(t, a, "team"), item) {
		t.Fatal("the idle-pushed write must reach the other clone")
	}
}

// TestUnreachableRemoteIsOffline: a failed check is recorded and shown as
// offline; reads keep serving the local state, and the pre-read check does
// not fail the read.
func TestUnreachableRemoteIsOffline(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	a := w.machine("a")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	item := createItem(t, a, "team", "local")
	if err := os.Rename(w.remote, w.remote+".gone"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Check(ctx, "team"); err == nil {
		t.Fatal("a check against a missing remote must fail")
	}
	v := view(t, a, "team")
	if v.Sync.Online || v.Sync.Error == "" || v.Clones["ada"][0].Online == nil || *v.Clones["ada"][0].Online {
		t.Fatalf("sync after the failed check = %+v, clones %+v; want offline with the error", v.Sync, v.Clones)
	}
	if !hasItem(v, item) {
		t.Fatal("reads must serve the local state while offline")
	}
	w.clock.Advance(time.Hour)
	if err := a.PreRead(ctx, "team"); err != nil {
		t.Fatalf("an offline pre-read check must not fail the read: %v", err)
	}
	if err := os.Rename(w.remote+".gone", w.remote); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Check(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if v := view(t, a, "team"); !v.Sync.Online || v.Sync.Error != "" {
		t.Fatalf("sync after reconnecting = %+v, want online", v.Sync)
	}
}

// TestViewFillsCallerFields pins the store, sync, clones, and stats.commits
// the caller adds to wstracker's view state.
func TestViewFillsCallerFields(t *testing.T) {
	w := newWorld(t)
	a, b := w.machine("a"), w.machine("b")
	if _, err := a.Register(context.Background(), RegisterRequest{ID: "team", Label: "Team Alpha", Remote: w.remote, Member: "ada", Profile: &wstracker.Profile{Given: "Ada"}}); err != nil {
		t.Fatal(err)
	}
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	if _, err := a.Check(context.Background(), "team"); err != nil {
		t.Fatal(err)
	}
	pushed := view(t, a, "team").Stats.Commits
	first := createItem(t, a, "team", "one")
	second := createItem(t, a, "team", "two")
	v := view(t, a, "team")
	stream := v.Viewer.Stream

	if v.Store.Name != "team" || v.Store.Label != "Team Alpha" || v.Store.Remote == nil || *v.Store.Remote != w.remote {
		t.Fatalf("store = %+v", v.Store)
	}
	if len(v.Sync.Unpushed) != 1 || !v.Sync.Unpushed[0].Open || !reflect.DeepEqual(v.Sync.Unpushed[0].Events, []string{first, second}) {
		t.Fatalf("unpushed = %+v, want the open batch with both writes", v.Sync.Unpushed)
	}
	if v.Stats.Commits != pushed+1 {
		t.Fatalf("commits = %d, want %d plus the open batch", v.Stats.Commits, pushed)
	}
	if got := v.Clones["ada"]; len(got) != 1 || got[0].Stream != stream || got[0].Online == nil || !*got[0].Online || got[0].Unpushed != 1 {
		t.Fatalf("ada's clones = %+v", got)
	}
	if got := v.Clones["bo"]; len(got) != 1 || got[0].Online != nil || got[0].Unpushed != 0 {
		t.Fatalf("bo's clones = %+v, want one clone with unknown online state", got)
	}

	syncStore(t, a, "team")
	v = view(t, a, "team")
	if len(v.Sync.Unpushed) != 0 || v.Stats.Commits != pushed+1 || v.Sync.SyncedAt == nil {
		t.Fatalf("after sync: unpushed %+v, commits %d, syncedAt %v", v.Sync.Unpushed, v.Stats.Commits, v.Sync.SyncedAt)
	}

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"viewer", "items", "stats", "heads", "store", "sync", "clones"} {
		if _, ok := keys[k]; !ok {
			t.Fatalf("the encoded view state lacks %q", k)
		}
	}
}

// TestStoreWithoutRemote: a store registered with no remote is created
// locally, shows no remote and offline, and Sync closes its batch.
func TestStoreWithoutRemote(t *testing.T) {
	w := newWorld(t)
	a := w.machine("a")
	res, err := a.Register(context.Background(), RegisterRequest{ID: "mine", Member: "ada", Profile: &wstracker.Profile{Family: "Park", Given: "Ada"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Creator {
		t.Fatalf("result = %+v, want creator", res)
	}
	createItem(t, a, "mine", "note to self")
	if rep := syncStore(t, a, "mine"); !rep.Closed {
		t.Fatalf("sync with no remote = %+v, want the batch closed", rep)
	}
	v := view(t, a, "mine")
	if v.Store.Remote != nil || v.Sync.Online || len(v.Sync.Unpushed) != 0 || v.Members["ada"].Name != "Ada Park" {
		t.Fatalf("store %+v sync %+v member %+v", v.Store, v.Sync, v.Members["ada"])
	}
}

// TestStartPollRunsOnRound: onRound runs after a poll round this process
// ran, and a nil onRound is fine.
func TestStartPollRunsOnRound(t *testing.T) {
	w := newWorld(t)
	a := w.machine("a")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})

	rounds := make(chan struct{}, 16)
	stop := a.StartPoll(10*time.Millisecond, func() {
		select {
		case rounds <- struct{}{}:
		default:
		}
	})
	select {
	case <-rounds:
	case <-time.After(20 * time.Second):
		stop()
		t.Fatal("onRound never ran")
	}
	stop()

	// While another process holds the poll lock, this one runs no round.
	holder := w.machine("a") // same root as a
	defer holder.Close()
	stopHolder := holder.StartPoll(10*time.Millisecond, nil)
	defer stopHolder()
	for !holder.PollerRunning() {
		time.Sleep(5 * time.Millisecond)
	}
	for len(rounds) > 0 {
		<-rounds
	}
	stopLoser := a.StartPoll(10*time.Millisecond, func() { rounds <- struct{}{} })
	time.Sleep(100 * time.Millisecond)
	stopLoser()
	if len(rounds) != 0 {
		t.Fatal("onRound ran in a process that does not hold the poll lock")
	}

	stopNil := a.StartPoll(10*time.Millisecond, nil)
	time.Sleep(50 * time.Millisecond)
	stopNil()
}
