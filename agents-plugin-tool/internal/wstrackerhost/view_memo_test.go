package wstrackerhost

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wstracker"
)

// countingRunner is the real git runner, recording every command it runs.
type countingRunner struct {
	mu   sync.Mutex
	runs []string
}

func (r *countingRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.runs = append(r.runs, strings.Join(args, " "))
	r.mu.Unlock()
	return batchRunner{}.RunGit(ctx, root, args...)
}

// take returns the commands run since the last take.
func (r *countingRunner) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.runs
	r.runs = nil
	return out
}

// backdate sets the modification time of every file under dir an hour
// back, as if nothing in it had been written lately, so the ledger's stamps
// (which vouch only for files older than a couple of seconds) can vouch.
func backdate(t *testing.T, dir string) {
	t.Helper()
	at := time.Now().Add(-time.Hour)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(path, at, at)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestViewOfUnchangedStoreRunsNoGit: a view whose store's refs and journal
// did not move since the last view runs no git and reads the same view; a
// journal write by another process shows on the next view still without
// git; a commit another process pushed moves the refs, so the next view
// lists them and counts the commits again.
func TestViewOfUnchangedStoreRunsNoGit(t *testing.T) {
	w := newWorld(t)
	root := filepath.Join(w.dir, "a")
	runner := &countingRunner{}
	a, err := New(Options{Root: root, Clock: w.clock, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	// b is another process on the same machine: the same root, so the same
	// store directory and clone.
	b, err := New(Options{Root: root, Clock: w.clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })

	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	createItem(t, a, "team", "one")
	syncStore(t, a, "team")
	e, err := a.Entry("team")
	if err != nil {
		t.Fatal(err)
	}

	backdate(t, e.Dir)
	first := view(t, a, "team")
	if runs := runner.take(); len(runs) == 0 {
		t.Fatal("the first view after the files settled must read the refs with git")
	}
	again := view(t, a, "team")
	if runs := runner.take(); len(runs) != 0 {
		t.Fatalf("a view of an unchanged store ran git: %q", runs)
	}
	if len(again.Items) != len(first.Items) || again.Stats.Commits != first.Stats.Commits {
		t.Fatalf("unchanged view: %d items %d commits, want %d and %d", len(again.Items), again.Stats.Commits, len(first.Items), first.Stats.Commits)
	}

	// A write that reaches only the journal: the refs did not move, so the
	// view needs no git to show it.
	two := createItem(t, b, "team", "two")
	v := view(t, a, "team")
	if runs := runner.take(); len(runs) != 0 {
		t.Fatalf("a view after a journal-only write ran git: %q", runs)
	}
	if !hasItem(v, two) || v.Stats.Commits != first.Stats.Commits+1 {
		t.Fatalf("after another process's write: has item %v, commits %d, want it and %d plus the open batch", hasItem(v, two), v.Stats.Commits, first.Stats.Commits)
	}

	// The other process commits and pushes: the refs moved.
	syncStore(t, b, "team")
	v = view(t, a, "team")
	runs := runner.take()
	if !hasItem(v, two) || v.Stats.Commits <= first.Stats.Commits || len(v.Sync.Unpushed) != 0 {
		t.Fatalf("after another process's commit: has item %v, commits %d (were %d), unpushed %+v", hasItem(v, two), v.Stats.Commits, first.Stats.Commits, v.Sync.Unpushed)
	}
	if !slicesHasPrefix(runs, "for-each-ref") || !slicesHasPrefix(runs, "rev-list --count") {
		t.Fatalf("a view after the refs moved must list them and count the commits again; ran %q", runs)
	}

	// Refs that moved and settled since the last view: the stamp vouches
	// again, but for other refs, so the view still lists and counts. The
	// store settles first, so the last view vouched for its refs.
	backdate(t, e.Dir)
	view(t, a, "team")
	runner.take()
	view(t, a, "team")
	if runs := runner.take(); len(runs) != 0 {
		t.Fatalf("a view of the settled store ran git: %q", runs)
	}
	three := createItem(t, b, "team", "three")
	syncStore(t, b, "team")
	backdate(t, e.Dir)
	before := v.Stats.Commits
	v = view(t, a, "team")
	runs = runner.take()
	if !hasItem(v, three) || v.Stats.Commits <= before || len(v.Sync.Unpushed) != 0 {
		t.Fatalf("after settled moved refs: has item %v, commits %d (were %d), unpushed %+v", hasItem(v, three), v.Stats.Commits, before, v.Sync.Unpushed)
	}
	if !slicesHasPrefix(runs, "for-each-ref") || !slicesHasPrefix(runs, "rev-list --count") {
		t.Fatalf("a view after settled moved refs must list them and count the commits again; ran %q", runs)
	}
	view(t, a, "team")
	if runs := runner.take(); len(runs) != 0 {
		t.Fatalf("a view of the settled store ran git: %q", runs)
	}
}

func slicesHasPrefix(runs []string, prefix string) bool {
	for _, r := range runs {
		if strings.HasPrefix(r, prefix) {
			return true
		}
	}
	return false
}
