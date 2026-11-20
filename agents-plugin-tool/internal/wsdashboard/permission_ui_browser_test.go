package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// permUIWorktrees shows each session folder as a worktree on one branch,
// so the rail row carries the new-session button; no git runs.
type permUIWorktrees struct{}

func (permUIWorktrees) ReadWorktrees(Registration) (WorktreePool, error) { return WorktreePool{}, nil }
func (permUIWorktrees) ReadRepoWorktrees(repo string, _ []Registration) (RepoWorktrees, error) {
	return RepoWorktrees{Worktrees: []RepoWorktree{{Path: repo, Branch: "feat/agent-popup"}}}, nil
}

// The common permission level controls on real pages in headless Chromium:
// the creation popover (default, a level, Pi, the full warning), the
// session menu (legacy-derived and explicit), the state cases, and the
// unsupported/refused cases, in both themes at 1440 and 420 wide. The
// daemon is an isolated fixture; the script answers only /support/choices
// and the hosted session's /agents routes, so no harness or provider runs.
// WS_PERMISSION_UI_SHOTS=<abs dir> saves the captures, and with
// WS_PERMISSION_UI_REFS=<abs dir> (the approved mockup PNGs) side-by-side
// comparisons. Needs WS_PLAYWRIGHT_MODULE.
func TestPermissionUIBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to use cached Chromium")
	}
	shots, refs := os.Getenv("WS_PERMISSION_UI_SHOTS"), os.Getenv("WS_PERMISSION_UI_REFS")
	for _, p := range []string{shots, refs} {
		if p != "" && !filepath.IsAbs(p) {
			t.Fatal("WS_PERMISSION_UI_SHOTS and WS_PERMISSION_UI_REFS must be absolute")
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	SetDevAssets(nil)
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
		// A daemon that hosts lead sessions shows the rail's new-session
		// button; the script answers the agent routes itself.
		o.Agents = &guideAgents{}
		o.Worktrees = permUIWorktrees{}
	})
	// A session in a repository (a .git directory; git never runs) gives the
	// rail a worktree row and so the new-session button.
	repo := filepath.Join(t.TempDir(), "devenv")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	register(t, f.dir, "reposession", repo, time.Now())
	f.d.Index().Refresh()
	waitFor(t, "the repository session to be indexed", func() bool {
		_, ok := f.d.Index().Session("reposession")
		return ok
	})
	// The viewed session runs Claude, as the script's hosted state says.
	now := time.Now().UnixMilli()
	f.d.snapshots.SetLive(f.dir, "livesession", wslive.Snapshot{SchemaVersion: 1, Adapter: "claude", UpdatedAt: now, HeartbeatMs: 30000,
		Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: "streamed <partial> reply"}}})
	f.d.broker.wake(liveDep("livesession"))
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/permission-ui.browser.cjs", f.base, shots, refs)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("permission UI in Chromium: %v", err)
	}
}
