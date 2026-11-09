package wstrackerhost

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
)

// wikiFaultRunner fails, while its switches are on, the wiki namespace
// fetch (a transport failure whose message carries credentials) and the
// push of one wiki ref (a remote hook's refusal).
type wikiFaultRunner struct {
	mu        sync.Mutex
	failFetch bool
	pushRef   string
}

func (r *wikiFaultRunner) set(failFetch bool, pushRef string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failFetch, r.pushRef = failFetch, pushRef
}

func (r *wikiFaultRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	r.mu.Lock()
	failFetch, pushRef := r.failFetch, r.pushRef
	r.mu.Unlock()
	if len(args) > 0 {
		last := args[len(args)-1]
		if failFetch && strings.HasPrefix(last, "+refs/heads/wiki/*:") {
			return nil, errors.New("exit status 128: fatal: unable to access 'https://ada:s3cret@example.com/t.git/'")
		}
		if pushRef != "" && strings.HasSuffix(last, ":"+pushRef) {
			return nil, fmt.Errorf("exit status 1: !\t%s\t[remote rejected] (pre-receive hook declined)", last)
		}
	}
	return batchRunner{}.RunGit(ctx, root, args...)
}

// wikiWarningsOf reads store id's view as the dashboard does (redacted).
func wikiWarningsOf(t *testing.T, h *Host, id string) (bool, []string) {
	t.Helper()
	v, err := h.View(context.Background(), id, 0)
	if err != nil {
		t.Fatal(err)
	}
	v.Redact()
	return v.Sync.Online, v.Sync.WikiWarnings
}

// unpublish makes h's wiki one only its clone holds, as one written before
// the store had a remote: gone from the remote, with no remote snapshot or
// known ref for it on the clone.
func unpublish(t *testing.T, w *world, h *Host, wiki string) {
	t.Helper()
	repo := filepath.Join(h.stores["team"].entry.Dir, "repo.git")
	for _, c := range [][]string{
		{"--git-dir", w.remote, "update-ref", "-d", "refs/heads/wiki/" + wiki},
		{"--git-dir", repo, "update-ref", "-d", "refs/ledger/remote/wiki/" + wiki},
		{"--git-dir", repo, "update-ref", "-d", "refs/ledger/known/wiki/" + wiki},
	} {
		if out, err := exec.Command("git", c...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", c, err, out)
		}
	}
}

// TestWikiSyncFailuresStandInTheView: a failing wiki fetch shows in the
// view's sync state after every poll, redacted, while the store stays
// online, and clears once a fetch works; a wiki push the remote refuses
// shows after the sync that tried it and after the polls that follow it,
// and clears once the wiki syncs.
func TestWikiSyncFailuresStandInTheView(t *testing.T) {
	w := newWorld(t)
	fr := &wikiFaultRunner{}
	a, err := New(Options{Root: filepath.Join(w.dir, "a"), Clock: w.clock, Runner: fr})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	ctx := context.Background()
	if _, err := a.WikiSave(ctx, "team", WikiSave{Edit: wsledger.WikiEdit{Wiki: "docs", NewWiki: true, Op: wsledger.WikiOpWrite, Path: "home", Content: "1\n"}}); err != nil {
		t.Fatal(err)
	}

	fr.set(true, "")
	for round := 1; round <= 2; round++ {
		a.PollOnce(ctx)
		online, warns := wikiWarningsOf(t, a, "team")
		if !online || len(warns) != 1 || !strings.Contains(warns[0], "wiki fetch failed") {
			t.Fatalf("poll %d with a failing wiki fetch: online %v, warnings %q", round, online, warns)
		}
		if strings.Contains(warns[0], "s3cret") {
			t.Fatalf("poll %d: the warning leaks credentials: %q", round, warns[0])
		}
	}
	fr.set(false, "")
	a.PollOnce(ctx)
	if _, warns := wikiWarningsOf(t, a, "team"); len(warns) != 0 {
		t.Fatalf("poll after the fetch recovered: warnings %q", warns)
	}

	// Make the wiki one the remote never held, so the next sync pushes
	// it, and have the remote refuse that push.
	unpublish(t, w, a, "docs")
	fr.set(false, "refs/heads/wiki/docs")
	if _, err := a.Sync(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if _, warns := wikiWarningsOf(t, a, "team"); len(warns) != 1 || !strings.Contains(warns[0], "wiki/docs") || !strings.Contains(warns[0], "pre-receive hook declined") {
		t.Fatalf("sync with a refused wiki push: warnings %q", warns)
	}
	a.PollOnce(ctx)
	if _, warns := wikiWarningsOf(t, a, "team"); len(warns) != 1 || !strings.Contains(warns[0], "wiki/docs") {
		t.Fatalf("poll after a refused wiki push: warnings %q, want the refusal to stand", warns)
	}
	fr.set(false, "")
	if _, err := a.Sync(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if _, warns := wikiWarningsOf(t, a, "team"); len(warns) != 0 {
		t.Fatalf("sync after the wiki synced: warnings %q", warns)
	}
}

// TestWikiPushRefusalStandsAcrossProcesses: a wiki push one process (an
// MCP server's sync) saw refused stays in the shared view through another
// process's polls (the daemon's) and a daemon restart, though neither ever
// tried the push, until a push of it lands; then the next poll clears it.
func TestWikiPushRefusalStandsAcrossProcesses(t *testing.T) {
	w := newWorld(t)
	fr := &wikiFaultRunner{}
	root := filepath.Join(w.dir, "a")
	open := func() *Host {
		h, err := New(Options{Root: root, Clock: w.clock, Runner: fr})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { h.Close() })
		return h
	}
	daemon := open()
	w.register(daemon, "team", "ada", &wstracker.Profile{Given: "Ada"})
	ctx := context.Background()
	if _, err := daemon.WikiSave(ctx, "team", WikiSave{Edit: wsledger.WikiEdit{Wiki: "docs", NewWiki: true, Op: wsledger.WikiOpWrite, Path: "home", Content: "1\n"}}); err != nil {
		t.Fatal(err)
	}
	mcp := open()

	// Make the wiki one the remote never held, so a sync pushes it, and
	// have the remote refuse that push.
	unpublish(t, w, daemon, "docs")
	fr.set(false, "refs/heads/wiki/docs")
	if _, err := mcp.Sync(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	want := func(when string, h *Host, n int) {
		t.Helper()
		_, warns := wikiWarningsOf(t, h, "team")
		if len(warns) != n || n == 1 && !strings.Contains(warns[0], "wiki/docs") {
			t.Fatalf("%s: warnings %q, want %d naming wiki/docs", when, warns, n)
		}
	}
	want("after the mcp sync", daemon, 1)
	daemon.PollOnce(ctx)
	want("after the daemon's poll", daemon, 1)
	daemon.Close()
	daemon = open()
	daemon.PollOnce(ctx)
	want("after a daemon restart and poll", daemon, 1)

	fr.set(false, "")
	if _, err := mcp.Sync(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	daemon.PollOnce(ctx)
	want("after the push landed and the daemon polled", daemon, 0)
}
