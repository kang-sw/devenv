package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

type timeoutRepoRunner struct {
	t              *testing.T
	calls          int
	statusDeadline time.Time
}

func (r *timeoutRepoRunner) RunGit(ctx context.Context, _ string, args ...string) ([]byte, error) {
	r.calls++
	deadline, ok := ctx.Deadline()
	if !ok {
		r.t.Fatal("unbounded repo read")
	}
	if args[0] == "--no-optional-locks" {
		r.statusDeadline = deadline
		<-ctx.Done() // exercise a real timeout, not an immediate I/O error
		return nil, ctx.Err()
	}
	if ctx.Err() != nil || !deadline.After(r.statusDeadline) {
		r.t.Fatalf("branch fallback has no reserved budget: %v", ctx.Err())
	}
	switch args[0] {
	case "symbolic-ref":
		return []byte("refs/heads/main\n"), nil
	case "rev-list":
		return []byte("2\t1\n"), nil
	case "rev-parse":
		if args[1] == "--short" {
			return []byte("abc123\n"), nil
		}
		return []byte("origin/main\n"), nil
	}
	return nil, fmt.Errorf("unexpected git argv: %v", args)
}

func TestSnapshotStatusTimeoutRetainsBranchFallback(t *testing.T) {
	r := &timeoutRepoRunner{t: t}
	repo := readSessionRepoWith(r, filepath.Join(t.TempDir(), "repo"))
	if r.calls != 5 || repo.Branch != "main" || repo.Upstream != "origin/main" || repo.Ahead != 2 || repo.Behind != 1 || repo.Modified != 0 || repo.Staged != 0 || repo.Untracked != 0 {
		t.Fatalf("timed-out status lost branch standing: %+v calls=%d", repo, r.calls)
	}
}
