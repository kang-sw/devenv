package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
)

type countingBadgeRunner struct {
	gitBadgeRunner
	calls int
}

func (r *countingBadgeRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	r.calls++
	return r.gitBadgeRunner.RunGit(ctx, root, args...)
}

func TestIdleTouchesGitReadFanoutAndExternalEdits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base := t.TempDir()
	origin, work := filepath.Join(base, "origin"), filepath.Join(base, "work")
	gitRun(t, base, "init", "-q", "-b", "main", origin)
	gitCommit(t, origin, "a")
	gitRun(t, base, "clone", "-q", origin, work)
	check := func(label string) {
		t.Helper()
		legacy := &countingBadgeRunner{}
		branch, ok := wsgit.BranchSync(context.Background(), legacy, work)
		if !ok {
			t.Fatal("legacy branch read failed")
		}
		if _, err := legacy.RunGit(context.Background(), work, "rev-parse", "--show-toplevel"); err != nil {
			t.Fatal(err)
		}
		counts, ok := wsgit.WorkingTreeCounts(context.Background(), legacy, work)
		if !ok {
			t.Fatal("legacy counts read failed")
		}
		run := &countingBadgeRunner{}
		b := readGitBadgeWith(context.Background(), run, work)
		if b.State != gitOK || b.Branch != branch.Head || b.Upstream != branch.Upstream || b.Ahead != branch.Ahead || b.Behind != branch.Behind || b.Modified != counts.Modified || b.Staged != counts.Staged || b.Untracked != counts.Untracked {
			t.Fatalf("%s: badge=%+v legacy=%+v counts=%+v", label, b, branch, counts)
		}
		if run.calls != 2 || legacy.calls != 6 {
			t.Fatalf("%s command fanout %d -> %d", label, legacy.calls, run.calls)
		}
		t.Logf("%s: comparable read git spawns %d -> %d", label, legacy.calls, run.calls)
	}
	check("clean")
	// In-place edits need not move HEAD, index, or directory metadata.
	if err := os.WriteFile(filepath.Join(work, "a"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	check("external in-place edit")
	gitRun(t, work, "add", "a")
	if err := os.WriteFile(filepath.Join(work, "a"), []byte("changed again"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "untracked"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, work, "config", "status.showUntrackedFiles", "no")
	check("staged and modified plus untracked")
	gitRun(t, work, "commit", "-q", "-m", "ahead")
	gitCommit(t, origin, "behind")
	gitRun(t, work, "fetch", "-q")
	check("ahead and behind")
	gitRun(t, work, "config", "status.aheadBehind", "false")
	check("ahead and behind despite status config")
	gitRun(t, work, "tag", "main")
	check("shadowed branch")
}
