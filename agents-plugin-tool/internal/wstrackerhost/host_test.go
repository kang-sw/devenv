// Shared host test fixtures and root configuration.
package wstrackerhost

import (
	"context"
	"encoding/json"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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
