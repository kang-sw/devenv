package wsledger

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"
)

// TestMain isolates every git invocation from the developer's global and
// system config (signing defaults, hooks, url rewrites).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wsledger-gitconfig")
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

// fakeClock is a controllable Clock shared by the writers of one test.
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

func (c *fakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{c: c, at: c.now.Add(d), f: f}
	c.timers = append(c.timers, t)
	return t
}

// Advance moves the clock and runs every timer that came due, in order.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var due []*fakeTimer
	kept := c.timers[:0]
	for _, t := range c.timers {
		if !t.stopped && !t.at.After(c.now) {
			t.stopped = true
			due = append(due, t)
		} else if !t.stopped {
			kept = append(kept, t)
		}
	}
	c.timers = kept
	c.mu.Unlock()
	sort.Slice(due, func(i, j int) bool { return due[i].at.Before(due[j].at) })
	for _, t := range due {
		t.f()
	}
}

// world is one test's shared repository, clock, and per-writer directories.
type world struct {
	t     *testing.T
	root  string
	repo  string
	clock *fakeClock
	keys  map[string]string // writer name -> key dir
}

func newWorld(t *testing.T) *world {
	t.Helper()
	root := t.TempDir()
	return &world{t: t, root: root, repo: filepath.Join(root, "shared.git"), clock: newFakeClock(), keys: map[string]string{}}
}

// opts returns Options for a writer: its own store dir and key dir, the
// shared repository, a fixed clone id.
func (w *world) opts(member, clone string) Options {
	keyName := member
	if dir, ok := w.keys[keyName]; ok {
		return w.optsWithKey(member, clone, dir)
	}
	dir := filepath.Join(w.root, "keys", keyName)
	w.keys[keyName] = dir
	return w.optsWithKey(member, clone, dir)
}

func (w *world) optsWithKey(member, clone, keyDir string) Options {
	return Options{
		Dir:     filepath.Join(w.root, "stores", member+"-"+clone),
		RepoDir: w.repo,
		Member:  member,
		Name:    member,
		KeyDir:  keyDir,
		CloneID: clone,
		Clock:   w.clock,
	}
}

func (w *world) create(member, clone string) *Ledger {
	w.t.Helper()
	l, err := Create(context.Background(), w.opts(member, clone))
	if err != nil {
		w.t.Fatalf("create %s: %v", member, err)
	}
	return l
}

func (w *world) join(member, clone string) *Ledger {
	w.t.Helper()
	l, err := Join(context.Background(), w.opts(member, clone))
	if err != nil {
		w.t.Fatalf("join %s: %v", member, err)
	}
	return l
}

func appendOne(t *testing.T, l *Ledger, typ string, payload any) EventID {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := l.Append(context.Background(), []NewEvent{{Type: typ, Payload: raw}})
	if err != nil {
		t.Fatalf("append %s: %v", typ, err)
	}
	return ids[0]
}

func syncOK(t *testing.T, l *Ledger) SyncReport {
	t.Helper()
	rep, err := l.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync %s: %v", l.stream, err)
	}
	return rep
}

func fetchOK(t *testing.T, l *Ledger) SyncReport {
	t.Helper()
	rep, err := l.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch %s: %v", l.stream, err)
	}
	return rep
}

// eventsByID reads every event of l's view.
func eventsByID(l *Ledger) map[EventID]Event {
	evs, _ := l.Events(Cursor{})
	out := map[EventID]Event{}
	for _, e := range evs {
		out[e.ID] = e
	}
	return out
}

func wantVerdict(t *testing.T, l *Ledger, id EventID, want Verdict) Event {
	t.Helper()
	e, ok := eventsByID(l)[id]
	if !ok {
		t.Fatalf("%s: event %s not in view", l.stream, id)
	}
	if e.Verdict != want {
		t.Fatalf("%s: event %s verdict %s (%s), want %s", l.stream, id, e.Verdict, e.Reason, want)
	}
	return e
}

// forger opens a writer for o without Join and without the store lock: a
// stand-in for an impostor or a thief who writes refs directly.
func (w *world) forger(o Options) *Ledger {
	w.t.Helper()
	ctx := context.Background()
	l, err := newLedger(ctx, o)
	if err != nil {
		w.t.Fatalf("forger %s: %v", o.Member, err)
	}
	if err := l.initClone(); err != nil {
		w.t.Fatal(err)
	}
	if err := l.refresh(ctx); err != nil {
		w.t.Fatal(err)
	}
	return l
}

// evLine builds one event line for a forged commit.
func evLine(t *testing.T, seq int, ts time.Time, typ string, payload any) journalEvent {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return journalEvent{Seq: seq, TS: ts, Type: typ, Payload: raw}
}

// forgeCommit writes a commit signed by l's key with l.stream in its header
// and arbitrary seqs and seen, without moving any ref.
func forgeCommit(t *testing.T, l *Ledger, parent string, seen map[string]int, ts time.Time, lines ...journalEvent) string {
	t.Helper()
	oid, err := l.writeEventCommit(context.Background(), parent, seen, ts, lines)
	if err != nil {
		t.Fatalf("forge commit: %v", err)
	}
	return oid
}

// refTip reads ref straight from the repository l uses; "" when absent.
func refTip(t *testing.T, l *Ledger, ref string) string {
	t.Helper()
	refs, err := l.git.listRefs(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	return refs[ref]
}

// setRef moves ref from old to oid in the repository l uses; old "" means
// the ref must not exist.
func setRef(t *testing.T, l *Ledger, ref, oid, old string) {
	t.Helper()
	if err := l.git.updateRef(context.Background(), ref, oid, old); err != nil {
		t.Fatalf("update-ref %s: %v", ref, err)
	}
}

// streamEvents returns the causal (non-broken) events of one stream, by seq.
func streamEvents(l *Ledger, stream string) []Event {
	evs, _ := l.Events(Cursor{})
	var out []Event
	for _, e := range evs {
		if e.ID.Stream() == stream && e.ID.Broken == "" {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.Seq < out[j].ID.Seq })
	return out
}

// brokenEvents returns the broken events of one stream.
func brokenEvents(l *Ledger, stream string) []Event {
	evs, _ := l.Events(Cursor{})
	var out []Event
	for _, e := range evs {
		if e.ID.Stream() == stream && e.ID.Broken != "" {
			out = append(out, e)
		}
	}
	return out
}
