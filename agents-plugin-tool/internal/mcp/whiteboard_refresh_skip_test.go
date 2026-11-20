package mcp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
)

type countingGitRunner struct {
	calls int
	inner wsgit.Runner
}

func (r *countingGitRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	r.calls++
	return r.inner.RunGit(ctx, root, args...)
}

func TestWhiteboardExcludeResolvesCommonDirOnce(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	runner := &countingGitRunner{inner: wsgit.ExecRunner{}}
	dir := filepath.Join(root, "ai-docs", "whiteboards")
	for i := 0; i < 3; i++ {
		if err := registerWhiteboardExclude(runner, root, dir); err != nil {
			t.Fatal(err)
		}
	}
	if runner.calls != 1 || excludeEntries(t, root) != 1 {
		t.Fatalf("git runs=%d exclude entries=%d, want 1 and 1", runner.calls, excludeEntries(t, root))
	}
}
