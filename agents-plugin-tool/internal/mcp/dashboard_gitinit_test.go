package mcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Both production adapters must accept a freshly initialized repository:
// listing its root in the rail, and admitting it as a hosted lead's root.
// No harness is started and no session key is minted by this check.
func TestDashboardGitInitUnbornRailAndSpawnEligibility(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keys := NewAgentKeyStore()
	if _, err := keys.LeadRoot(root); err == nil {
		t.Fatal("plain folder admitted as a lead root")
	}
	runGit(t, root, "init")
	branch := strings.TrimSpace(string(runGitOutput(t, root, "symbolic-ref", "--short", "HEAD")))
	listed, err := NewDashboardSessionSource().ReadRepoWorktrees(root, nil)
	if err != nil || len(listed.Worktrees) != 1 {
		t.Fatalf("unborn worktrees = %+v (%v)", listed, err)
	}
	wt := listed.Worktrees[0]
	if wt.Path != root || wt.Branch != branch || wt.Detached || wt.Prunable {
		t.Fatalf("unborn root = %+v, want branch %s", wt, branch)
	}
	if got, err := keys.LeadRoot(root); err != nil || got != root {
		t.Fatalf("unborn spawn eligibility = %q (%v)", got, err)
	}
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", "HEAD").Run(); err == nil {
		t.Fatal("eligibility check created a commit")
	}
}
