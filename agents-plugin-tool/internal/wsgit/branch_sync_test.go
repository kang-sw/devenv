package wsgit

import (
	"context"
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
