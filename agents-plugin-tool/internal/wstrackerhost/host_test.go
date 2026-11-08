package wstrackerhost

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
)

// TestMain isolates every git invocation from the developer's global and
// system config (signing defaults, hooks, url rewrites).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wstrackerhost-gitconfig")
	if err != nil {
		panic(err)
	}
	global := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(global, []byte("[init]\n\tdefaultBranch = main\n[protocol \"file\"]\n\tallow = always\n"), 0o644); err != nil {
		panic(err)
	}
	os.Setenv("GIT_CONFIG_GLOBAL", global)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// fakeClock is a controllable clock shared by the hosts of one test; it runs
// the ledgers' push-delay timers when Advance passes them.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	c       *fakeClock
	at      time.Time
	f       func()
	stopped bool
}

func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := !t.stopped
	t.stopped = true
	return was
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) wsledger.Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{c: c, at: c.now.Add(d), f: f}
	c.timers = append(c.timers, t)
	return t
}

// Advance moves the clock and runs every timer that came due.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var due []*fakeTimer
	for _, t := range c.timers {
		if !t.stopped && !t.at.After(c.now) {
			t.stopped = true
			due = append(due, t)
		}
	}
	c.mu.Unlock()
	for _, t := range due {
		t.f()
	}
}

// world is one test: a bare remote and one clock shared by the machines.
type world struct {
	t      *testing.T
	dir    string
	remote string
	clock  *fakeClock
}

func newWorld(t *testing.T) *world {
	t.Helper()
	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.git")
	if out, err := exec.Command("git", "init", "--quiet", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("init remote: %v: %s", err, out)
	}
	return &world{t: t, dir: dir, remote: remote, clock: newFakeClock()}
}

// machine is a Host over its own root, so its own signing key.
func (w *world) machine(name string) *Host {
	w.t.Helper()
	h, err := New(Options{Root: filepath.Join(w.dir, name), Clock: w.clock})
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { h.Close() })
	return h
}

func (w *world) register(h *Host, id, member string, p *wstracker.Profile) RegisterResult {
	w.t.Helper()
	res, err := h.Register(context.Background(), RegisterRequest{ID: id, Remote: w.remote, Member: member, Profile: p})
	if err != nil {
		w.t.Fatalf("register %s as %s: %v", id, member, err)
	}
	return res
}

// remoteTip reads one ref of the bare remote; "" when absent.
func (w *world) remoteTip(ref string) string {
	w.t.Helper()
	out, err := exec.Command("git", "-C", w.remote, "for-each-ref", "--format=%(objectname)", ref).CombinedOutput()
	if err != nil {
		w.t.Fatalf("for-each-ref: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func createItem(t *testing.T, h *Host, store, title string) string {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"kind": "task", "title": title})
	ids, err := h.Dispatch(context.Background(), store, []wsledger.NewEvent{{Type: wstracker.TypeItemCreate, Payload: payload}})
	if err != nil {
		t.Fatalf("create %q: %v", title, err)
	}
	return ids[0].String()
}

func view(t *testing.T, h *Host, store string) *StoreView {
	t.Helper()
	v, err := h.View(context.Background(), store, 0)
	if err != nil {
		t.Fatalf("view %s: %v", store, err)
	}
	return v
}

func hasItem(v *StoreView, id string) bool {
	_, ok := v.Items[id]
	return ok
}

func syncStore(t *testing.T, h *Host, store string) wsledger.SyncReport {
	t.Helper()
	rep, err := h.Sync(context.Background(), store)
	if err != nil {
		t.Fatalf("sync %s: %v", store, err)
	}
	return rep
}

func TestRootFollowsConfigHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WS_CONFIG_HOME", home)
	got, err := Root(wsconfig.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "tracker"); got != want {
		t.Fatalf("Root = %s, want %s", got, want)
	}
	h, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if h.Root() != got {
		t.Fatalf("a zero-option host uses %s, want %s", h.Root(), got)
	}
}

// TestRegisterCreateJoinRoundTrip: one machine creates a store on an empty
// repository, another joins it with a saved Korean profile; the registry
// and saved profiles survive a new Host, and both clones see both members'
// profiles after a check.
func TestRegisterCreateJoinRoundTrip(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	a, b := w.machine("a"), w.machine("b")
	adaProfile := wstracker.Profile{Org: "Search", Family: "Park", Given: "Ada"}
	res := w.register(a, "team", "ada", &adaProfile)
	if !res.Creator || res.Profile != adaProfile {
		t.Fatalf("ada's registration = %+v, want creator with her profile", res)
	}
	if want := (StoreEntry{ID: "team", Remote: w.remote, Dir: filepath.Join(a.Root(), "team"), Member: "ada"}); res.Entry != want {
		t.Fatalf("entry = %+v, want %+v", res.Entry, want)
	}

	jon := wstracker.Profile{Org: "SW개발팀", Family: "홍", Given: "길동"}
	if err := b.SaveProfile(jon); err != nil {
		t.Fatal(err)
	}
	res = w.register(b, "alpha", "jon", nil)
	if res.Creator || res.Profile != jon {
		t.Fatalf("jon's registration = %+v, want a join with the saved profile", res)
	}

	// Registry and profiles round trip through a new Host on the same root.
	again, err := New(Options{Root: b.Root(), Clock: w.clock})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	stores, err := again.Stores()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stores, []StoreEntry{res.Entry}) {
		t.Fatalf("registry = %+v, want [%+v]", stores, res.Entry)
	}
	saved, err := again.Profiles()
	if err != nil {
		t.Fatal(err)
	}
	if last, ok := saved.LastProfile(); !ok || last != jon || len(saved.Profiles) != 1 {
		t.Fatalf("saved profiles = %+v, want jon's alone and last", saved)
	}

	if _, err := a.Check(ctx, "team"); err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, v := range []*StoreView{view(t, a, "team"), view(t, again, "alpha")} {
		if got := v.Members["jon"]; got.Profile != jon || got.Name != "홍길동" {
			t.Fatalf("%s sees jon as %+v", v.Viewer.Stream, got)
		}
		if got := v.Members["ada"]; got.Profile != adaProfile || got.Name != "Ada Park" {
			t.Fatalf("%s sees ada as %+v", v.Viewer.Stream, got)
		}
	}
}

func TestRegisterDefaultsToLastSavedProfile(t *testing.T) {
	w := newWorld(t)
	a := w.machine("a")
	if _, err := a.Register(context.Background(), RegisterRequest{ID: "team", Remote: w.remote, Member: "ada"}); err == nil {
		t.Fatal("a registration with no profile and none saved must fail")
	}
	first := wstracker.Profile{Given: "Ada"}
	edited := wstracker.Profile{Org: "Search", Given: "Ada"}
	for _, p := range []wstracker.Profile{first, edited, first} {
		if err := a.SaveProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	saved, _ := a.Profiles()
	if len(saved.Profiles) != 2 || saved.Last != 0 {
		t.Fatalf("saved = %+v, want two entries (an edit is a new one) with the first used last", saved)
	}
	if err := a.SaveProfile(edited); err != nil {
		t.Fatal(err)
	}
	res := w.register(a, "team", "ada", nil)
	if res.Profile != edited || view(t, a, "team").Members["ada"].Profile != edited {
		t.Fatalf("registration profile = %+v, want the last used %+v", res.Profile, edited)
	}
}

// TestRegisterFailureLeavesNothing: invalid ids are refused before any
// write, and a join that loses its member id leaves no directory or
// registry entry, so registering again under another id works.
func TestRegisterFailureLeavesNothing(t *testing.T) {
	w := newWorld(t)
	a, b := w.machine("a"), w.machine("b")
	p := &wstracker.Profile{Given: "Ada"}
	for _, req := range []RegisterRequest{
		{ID: "keys", Remote: w.remote, Member: "ada", Profile: p},
		{ID: "Team", Remote: w.remote, Member: "ada", Profile: p},
		{ID: "team", Remote: w.remote, Member: "a.b", Profile: p},
		{ID: "team", Remote: w.remote, Member: "ada", Profile: &wstracker.Profile{Org: "x"}},
	} {
		if _, err := a.Register(context.Background(), req); err == nil {
			t.Fatalf("register %+v must fail", req)
		}
	}
	w.register(a, "team", "ada", p)
	if _, err := a.Register(context.Background(), RegisterRequest{ID: "team", Remote: w.remote, Member: "ada", Profile: p}); err == nil {
		t.Fatal("registering an id twice must fail")
	}

	_, err := b.Register(context.Background(), RegisterRequest{ID: "team", Remote: w.remote, Member: "ada", Profile: p})
	if !errors.Is(err, wsledger.ErrIDTaken) {
		t.Fatalf("a second machine joining as ada = %v, want ErrIDTaken", err)
	}
	if _, err := os.Stat(filepath.Join(b.Root(), "team")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the failed registration left its directory: %v", err)
	}
	if b.HasStores() {
		t.Fatal("the failed registration left a registry entry")
	}
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
}

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
	stop := b.StartPoll(20 * time.Millisecond)
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
