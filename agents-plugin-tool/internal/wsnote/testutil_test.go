package wsnote

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initGitFixture creates a throwaway git repository with one commit and
// returns its root path, mirroring internal/wsstate's initRepo fixture — a
// real git worktree is required because WorktreePath resolves through
// wsstate.Manager.Ensure, which shells out to `git rev-parse`.
func initGitFixture(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "init")
	return repo
}

// twoWorktreesFixture creates a git repo with an initial commit at one temp
// dir, then links a second worktree of that same repository at another temp
// dir, returning both roots. Mirrors internal/mcp's twoWorktreesOfOneRepo
// fixture: required to prove ClonePath is keyed on ProjectKey (shared across
// worktrees of one repo), not WorktreeKey (which would differ between them).
func twoWorktreesFixture(t *testing.T) (mainRoot, linkedRoot string) {
	t.Helper()
	mainRoot = initGitFixture(t)
	linkedRoot = filepath.Join(t.TempDir(), "linked")
	runGit(t, mainRoot, "worktree", "add", "-b", "clone-path-linked", linkedRoot)
	return mainRoot, linkedRoot
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
