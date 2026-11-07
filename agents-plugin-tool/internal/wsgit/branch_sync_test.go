package wsgit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func commitFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", name)
	gitIn(t, dir, "commit", "-q", "-m", name)
}

// TestBranchSyncReadsUpstreamStanding pins BranchSync on real repositories:
// a branch with no upstream, an upstream in sync, ahead and behind counts
// against the upstream as of the last fetch, a detached HEAD, and a root
// that is not a repository.
func TestBranchSyncReadsUpstreamStanding(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	base := t.TempDir()
	origin := filepath.Join(base, "origin")
	work := filepath.Join(base, "work")
	gitIn(t, base, "init", "-q", "-b", "main", origin)
	commitFile(t, origin, "a")

	gitIn(t, base, "clone", "-q", origin, work)
	gitIn(t, work, "checkout", "-q", "-b", "topic")
	st, ok := BranchSync(ctx, nil, work)
	if !ok || st.Head != "topic" || st.Upstream != "" || st.Ahead != 0 || st.Behind != 0 {
		t.Fatalf("no upstream: ok=%v %+v", ok, st)
	}

	gitIn(t, work, "checkout", "-q", "main")
	st, ok = BranchSync(ctx, nil, work)
	if !ok || st.Head != "main" || st.Upstream != "origin/main" || st.Ahead != 0 || st.Behind != 0 {
		t.Fatalf("in sync: ok=%v %+v", ok, st)
	}

	commitFile(t, work, "b")
	commitFile(t, work, "c")
	commitFile(t, origin, "d")
	// Not fetched yet: behind reflects the last fetch.
	if st, _ = BranchSync(ctx, nil, work); st.Ahead != 2 || st.Behind != 0 {
		t.Fatalf("before fetch: %+v, want ahead 2 behind 0", st)
	}
	gitIn(t, work, "fetch", "-q")
	if st, _ = BranchSync(ctx, nil, work); st.Ahead != 2 || st.Behind != 1 {
		t.Fatalf("after fetch: %+v, want ahead 2 behind 1", st)
	}

	gitIn(t, work, "checkout", "-q", "--detach")
	st, ok = BranchSync(ctx, nil, work)
	if !ok || st.Head != "" || st.OID == "" || st.Upstream != "" {
		t.Fatalf("detached: ok=%v %+v", ok, st)
	}

	if st, ok = BranchSync(ctx, nil, t.TempDir()); ok || st != (BranchStatus{}) {
		t.Fatalf("not a repository: ok=%v %+v", ok, st)
	}
}

type failingRunner struct{}

func (failingRunner) RunGit(context.Context, string, ...string) ([]byte, error) {
	return nil, errors.New("timed out")
}

// TestWorkingTreeCountsOnRealRepository pins WorkingTreeCounts: a clean tree
// counts nothing, then modified, staged, both-ways, and untracked entries
// count by kind, an ignored file counts nowhere, a failed read reports
// not ok, and a root that is not a repository does too.
func TestWorkingTreeCountsOnRealRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	work := t.TempDir()
	gitIn(t, work, "init", "-q", "-b", "main")
	for _, n := range []string{"a", "b", "c", "d"} {
		commitFile(t, work, n)
	}
	if err := os.WriteFile(filepath.Join(work, ".gitignore"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, work, "add", ".gitignore")
	gitIn(t, work, "commit", "-q", "-m", "ignore")

	if c, ok := WorkingTreeCounts(ctx, nil, work); !ok || c != (TreeCounts{}) {
		t.Fatalf("clean: ok=%v %+v", ok, c)
	}

	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(work, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a", "changed") // modified, unstaged
	write("b", "staged")  // staged only
	gitIn(t, work, "add", "b")
	write("c", "staged then edited") // staged and modified
	gitIn(t, work, "add", "c")
	write("c", "edited again")
	write("new1", "x") // untracked
	if err := os.MkdirAll(filepath.Join(work, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	write("dir/new2", "x") // untracked directory: one entry
	write("ignored", "x")  // ignored: not counted

	c, ok := WorkingTreeCounts(ctx, nil, work)
	if want := (TreeCounts{Modified: 2, Staged: 2, Untracked: 2}); !ok || c != want {
		t.Fatalf("dirty: ok=%v %+v, want %+v", ok, c, want)
	}

	if c, ok = WorkingTreeCounts(ctx, failingRunner{}, work); ok || c != (TreeCounts{}) {
		t.Fatalf("failed read: ok=%v %+v", ok, c)
	}
	if c, ok = WorkingTreeCounts(ctx, nil, t.TempDir()); ok || c != (TreeCounts{}) {
		t.Fatalf("not a repository: ok=%v %+v", ok, c)
	}
}

// TestWorkingTreeCountsUnmerged pins a conflicted path as one modified entry.
func TestWorkingTreeCountsUnmerged(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	work := t.TempDir()
	gitIn(t, work, "init", "-q", "-b", "main")
	commitFile(t, work, "f")
	gitIn(t, work, "checkout", "-q", "-b", "other")
	if err := os.WriteFile(filepath.Join(work, "f"), []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, work, "commit", "-q", "-am", "other")
	gitIn(t, work, "checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(work, "f"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, work, "commit", "-q", "-am", "main")
	cmd := exec.Command("git", "-C", work, "merge", "other")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	_ = cmd.Run() // conflicts by design
	c, ok := WorkingTreeCounts(context.Background(), nil, work)
	if want := (TreeCounts{Modified: 1}); !ok || c != want {
		t.Fatalf("unmerged: ok=%v %+v, want %+v", ok, c, want)
	}
}
