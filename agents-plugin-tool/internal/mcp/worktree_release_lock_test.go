package mcp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

// No Git discovery may precede an already-held target guard. In particular,
// Windows process startup can exhaust a short caller deadline before Git's
// --absolute-git-dir returns, hiding contention behind a discovery error.
func TestWorktreeReleaseContentionBeforeGitDiscovery(t *testing.T) {
	for _, relative := range []bool{false, true} {
		t.Run(map[bool]string{false: "absolute-gitfile", true: "relative-gitfile"}[relative], func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "worktree")
			admin := filepath.Join(root, "admin")
			for _, dir := range []string{target, admin} {
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			pointer := admin
			if relative {
				var err error
				pointer, err = filepath.Rel(target, admin)
				if err != nil {
					t.Fatal(err)
				}
			}
			// Git on Windows writes slash-separated paths; the resolver must
			// reach the same lock as native filepath spelling, with CRLF too.
			if err := os.WriteFile(filepath.Join(target, ".git"), []byte("gitdir: "+filepath.ToSlash(pointer)+"\r\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			lock := flock.New(filepath.Join(admin, "ws-worktree-release.lock"))
			if err := lock.Lock(); err != nil {
				t.Fatal(err)
			}
			defer lock.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			_, _, unlock, err := reserveWorktreeRelease(ctx, releaseDiscoveryForbiddenRunner{t}, "  "+target+string(os.PathSeparator)+".  ", "", nil)
			if unlock != nil {
				unlock()
			}
			if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "worktree.release: contention") {
				t.Fatalf("expected target contention before any Git discovery, got %v", err)
			}
		})
	}
}

type releaseDiscoveryForbiddenRunner struct{ t *testing.T }

func (r releaseDiscoveryForbiddenRunner) RunGit(_ context.Context, _ string, args ...string) ([]byte, error) {
	r.t.Errorf("Git discovery before release guard: %v", args)
	return nil, errors.New("Git must not run before release admission")
}
