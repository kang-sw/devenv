package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// trackerSessionFixture is a lead fixture whose tracker root (beside its
// isolated config home) holds store "team", registered as ada on a bare
// remote that another machine, bo, joined too.
type trackerSessionFixture struct {
	*whiteboardFixture
	remote string
	bo     *wstrackerhost.Host
}

// setTrackerGitEnv isolates the tracker's git from the developer's config
// and allows the file transport the local bare remotes use.
func setTrackerGitEnv(t *testing.T) {
	t.Helper()
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, []byte("[init]\n\tdefaultBranch = main\n[protocol \"file\"]\n\tallow = always\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// newTrackerLeadFixture is a lead fixture with a fresh tracker link whose
// session-start check is never throttled.
func newTrackerLeadFixture(t *testing.T) *whiteboardFixture {
	t.Helper()
	setTrackerGitEnv(t)
	f := newWhiteboardFixture(t)
	f.s.tracker = trackerLink{opts: wstrackerhost.Options{CheckInterval: time.Nanosecond}}
	t.Cleanup(func() {
		f.s.tracker.checks.Wait()
		if f.s.tracker.host != nil {
			f.s.tracker.host.Close()
		}
	})
	return f
}

func newTrackerSessionFixture(t *testing.T) *trackerSessionFixture {
	t.Helper()
	f := newTrackerLeadFixture(t)
	ctx := context.Background()
	remote := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "--quiet", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("init remote: %v: %s", err, out)
	}
	root, err := wstrackerhost.Root(wsconfig.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ada, err := wstrackerhost.New(wstrackerhost.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ada.Register(ctx, wstrackerhost.RegisterRequest{ID: "team", Remote: remote, Member: "ada", Profile: &wstracker.Profile{Given: "Ada"}}); err != nil {
		t.Fatalf("register ada: %v", err)
	}
	ada.Close()
	bo, err := wstrackerhost.New(wstrackerhost.Options{Root: filepath.Join(t.TempDir(), "bo")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bo.Close() })
	if _, err := bo.Register(ctx, wstrackerhost.RegisterRequest{ID: "team", Remote: remote, Member: "bo", Profile: &wstracker.Profile{Given: "Bo"}}); err != nil {
		t.Fatalf("register bo: %v", err)
	}
	resolver := f.s.sessionResolver(f.key, builtinConfigDefaults())
	if on, err := f.s.trackerEnabled(&resolver, f.key); err != nil || on {
		t.Fatalf("precondition: workflow.tracker_enabled must resolve off: on=%v err=%v", on, err)
	}
	return &trackerSessionFixture{whiteboardFixture: f, remote: remote, bo: bo}
}

// boWrites publishes one item from bo and returns its id.
func (f *trackerSessionFixture) boWrites(t *testing.T, title string) string {
	t.Helper()
	ctx := context.Background()
	payload, _ := json.Marshal(map[string]any{"kind": "task", "title": title})
	ids, err := f.bo.Dispatch(ctx, "team", []wsledger.NewEvent{{Type: wstracker.TypeItemCreate, Payload: payload}})
	if err != nil {
		t.Fatalf("bo writes %q: %v", title, err)
	}
	if _, err := f.bo.Sync(ctx, "team"); err != nil {
		t.Fatalf("bo syncs: %v", err)
	}
	return ids[0].String()
}

// view waits for the background session-start checks and reads the
// server's own view of the store, which never calls the remote.
func (f *trackerSessionFixture) view(t *testing.T) *wstrackerhost.StoreView {
	t.Helper()
	f.s.tracker.checks.Wait()
	h, err := f.s.trackerHost()
	if err != nil {
		t.Fatal(err)
	}
	v, err := h.View(context.Background(), "team", 0)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	return v
}

func (f *trackerSessionFixture) sees(t *testing.T, item string) bool {
	t.Helper()
	_, ok := f.view(t).Items[item]
	return ok
}

// TestTrackerSessionCheckRunsAtLeadSessionStartWithKnobOff: every top-lead
// session start (the FRESH manual with a root, the ferrule mint, and a
// revive through the CONTINUE manual) fetches the registered stores while
// workflow.tracker_enabled resolves off.
func TestTrackerSessionCheckRunsAtLeadSessionStartWithKnobOff(t *testing.T) {
	f := newTrackerSessionFixture(t)

	item := f.boWrites(t, "before the fresh manual")
	if f.sees(t, item) {
		t.Fatal("precondition: the server has not fetched")
	}
	f.id++
	if resp := callToolWithKey(t, f.s, f.id, freshBootstrapKey, "workflow_manual", map[string]any{"root": f.root}); !strings.Contains(resp, "## Session Key") {
		t.Fatalf("fresh manual: %s", resp)
	}
	if !f.sees(t, item) {
		t.Fatal("the FRESH manual must run the session-start check")
	}

	item = f.boWrites(t, "before ferrule")
	f.id++
	parseLoginResponse(t, callLogin(t, f.s, f.id, f.root, nil))
	if !f.sees(t, item) {
		t.Fatal("a top-lead ferrule mint must run the session-start check")
	}

	item = f.boWrites(t, "before revive")
	if resp := f.call(t, "workflow_manual", map[string]any{}); isToolError(resp) {
		t.Fatalf("continue manual: %s", resp)
	}
	if !f.sees(t, item) {
		t.Fatal("a top-lead revive must run the session-start check")
	}
}

// TestTrackerSessionCheckSkipsSpawnedSessions: a key minted under a parent
// (a worker's) runs no check, through ferrule or the manual.
func TestTrackerSessionCheckSkipsSpawnedSessions(t *testing.T) {
	f := newTrackerSessionFixture(t)
	item := f.boWrites(t, "not for workers")
	f.id++
	child, _ := parseLoginResponse(t, callLogin(t, f.s, f.id, f.root, map[string]any{"parent_session_key": f.key}))
	f.id++
	if resp := callToolWithKey(t, f.s, f.id, child, "workflow_manual", nil); !strings.Contains(resp, "## Session Key") {
		t.Fatalf("child manual: %s", resp)
	}
	if f.sees(t, item) {
		t.Fatal("a spawned session must not run the session-start check")
	}
}

// TestTrackerSessionCheckNeverFailsTheManual: an unreachable remote leaves
// the manual intact and is recorded as offline.
func TestTrackerSessionCheckNeverFailsTheManual(t *testing.T) {
	f := newTrackerSessionFixture(t)
	if err := os.Rename(f.remote, f.remote+".gone"); err != nil {
		t.Fatal(err)
	}
	f.id++
	resp := callToolWithKey(t, f.s, f.id, freshBootstrapKey, "workflow_manual", map[string]any{"root": f.root})
	if !strings.Contains(resp, "## Session Key") {
		t.Fatalf("the manual must render while the remote is unreachable: %s", resp)
	}
	if v := f.view(t); v.Sync.Online || v.Sync.Error == "" {
		t.Fatalf("sync after the failed check = %+v, want offline with the error", v.Sync)
	}
}

// TestTrackerSessionCheckWithoutStoresTouchesNothing: with no registered
// store the session start creates no tracker root.
func TestTrackerSessionCheckWithoutStoresTouchesNothing(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.id++
	if resp := callToolWithKey(t, f.s, f.id, freshBootstrapKey, "workflow_manual", map[string]any{"root": f.root}); !strings.Contains(resp, "## Session Key") {
		t.Fatalf("fresh manual: %s", resp)
	}
	f.s.tracker.checks.Wait()
	root, err := wstrackerhost.Root(wsconfig.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the session start touched the tracker root: %v", err)
	}
}
