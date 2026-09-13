package wsmailbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

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

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestPathsAreSiblingsOfNoteStores(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	machinePath, err := MachinePath()
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if filepath.Base(machinePath) != "mailbox.json" {
		t.Fatalf("MachinePath = %s, want mailbox.json basename", machinePath)
	}
	configPath, err := wsconfig.GlobalPath(wsconfig.Options{})
	if err != nil {
		t.Fatalf("GlobalPath: %v", err)
	}
	if filepath.Dir(machinePath) != filepath.Dir(configPath) {
		t.Fatalf("MachinePath dir = %s, want sibling of global config dir %s", filepath.Dir(machinePath), filepath.Dir(configPath))
	}

	root := initGitFixture(t)
	worktreePath, err := WorktreePath(root)
	if err != nil {
		t.Fatalf("WorktreePath: %v", err)
	}
	if filepath.Base(worktreePath) != "mailbox.json" {
		t.Fatalf("WorktreePath = %s, want mailbox.json basename", worktreePath)
	}
	clonePath, err := ClonePath(root)
	if err != nil {
		t.Fatalf("ClonePath: %v", err)
	}
	if filepath.Base(clonePath) != "mailbox.json" {
		t.Fatalf("ClonePath = %s, want mailbox.json basename", clonePath)
	}
}

func TestLoadMissingFileReturnsEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mailbox.json")
	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.Presence == nil || len(store.Presence) != 0 {
		t.Fatalf("Load(missing).Presence = %#v, want empty non-nil map", store.Presence)
	}
	if store.Queues == nil || len(store.Queues) != 0 {
		t.Fatalf("Load(missing).Queues = %#v, want empty non-nil map", store.Queues)
	}
}

func TestWithLockRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mailbox.json")

	err := WithLock(path, func(store *StoreFile) error {
		store.Presence["alice"] = Presence{Name: "alice", Scope: ScopeMachine, LastSeen: "2026-09-13T00:00:00Z"}
		store.Queues = AppendQueue(store.Queues, "alice", Envelope{Content: "hi", SentAt: "2026-09-13T00:00:00Z"})
		return nil
	})
	if err != nil {
		t.Fatalf("WithLock: %v", err)
	}

	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.Presence["alice"].Name != "alice" {
		t.Fatalf("Load after WithLock: presence = %#v", store.Presence)
	}
	if len(store.Queues["alice"]) != 1 || store.Queues["alice"][0].Content != "hi" {
		t.Fatalf("Load after WithLock: queue = %#v", store.Queues["alice"])
	}
}

func TestAppendQueueTrimsToMaxLen(t *testing.T) {
	queues := map[string][]Envelope{}
	total := MaxQueueLen + 10
	for i := 0; i < total; i++ {
		queues = AppendQueue(queues, "bob", Envelope{Content: fmt.Sprintf("m%d", i), SentAt: "t"})
	}
	got := queues["bob"]
	if len(got) != MaxQueueLen {
		t.Fatalf("queue length = %d, want %d", len(got), MaxQueueLen)
	}
	// The survivors must be the NEWEST MaxQueueLen messages in send order:
	// trimming drops from the FRONT (oldest first), so the first survivor is
	// message (total-MaxQueueLen) and the last is (total-1). Seeding distinct
	// ordinal content means a wrong-end trim (keeping the oldest) or a reorder
	// fails here, where identical Content:"m" only ever checked length.
	for i, env := range got {
		want := fmt.Sprintf("m%d", total-MaxQueueLen+i)
		if env.Content != want {
			t.Fatalf("survivor[%d].Content = %q, want %q (trim must drop the oldest and keep the newest, in order)", i, env.Content, want)
		}
	}
}

// TestLoadRejectsCorruptStore verifies decodeStore/Load surfaces the
// json.Unmarshal error path for a malformed store file rather than panicking or
// silently returning a zeroed-out (looks-empty) store that a caller would then
// overwrite, dropping every real presence/queue record.
func TestLoadRejectsCorruptStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mailbox.json")
	if err := os.WriteFile(path, []byte("{ this is not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := Load(path)
	if err == nil {
		t.Fatalf("Load(corrupt) returned nil error; a malformed store must surface a parse error, not silently zero out")
	}
	if !strings.Contains(err.Error(), "parse mailbox store") {
		t.Fatalf("Load(corrupt) err = %v, want a parse error naming the store", err)
	}
	// The value returned beside the error is the zero StoreFile, not a usable
	// empty store: nil maps make an accidental "success" path obvious.
	if store.Presence != nil || store.Queues != nil {
		t.Fatalf("Load(corrupt) returned a populated store %#v; want the zero value beside the error", store)
	}
}

// TestWithLockTimesOutWhenLockHeld exercises WithLock's lock-acquisition-timeout
// path: when another handle already holds the store's flock, WithLock waits up to
// LockTimeout, then returns the timeout error without ever running the mutation
// body (so a RMW never proceeds on an unheld lock).
func TestWithLockTimesOutWhenLockHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mailbox.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	holder := flock.New(path + ".lock")
	locked, err := holder.TryLock()
	if err != nil || !locked {
		t.Fatalf("could not pre-acquire the store lock: locked=%v err=%v", locked, err)
	}
	defer holder.Unlock() //nolint:errcheck

	start := time.Now()
	err = WithLock(path, func(*StoreFile) error {
		t.Fatalf("WithLock body ran while the lock was held by another handle")
		return nil
	})
	elapsed := time.Since(start)
	// gofrs/flock surfaces the expired context through TryLockContext's error
	// return (the "acquire mailbox store lock: context deadline exceeded"
	// branch), not the locked==false path, so pin on the lock-acquisition
	// wrapper the timeout actually travels through.
	if err == nil || !strings.Contains(err.Error(), "mailbox store lock") {
		t.Fatalf("WithLock err = %v, want a lock-acquisition-timeout error naming the store lock", err)
	}
	// It must actually have blocked on the contended lock, not returned a
	// spurious instant error: the retry loop runs to roughly the LockTimeout
	// deadline. A lenient lower bound avoids scheduler-jitter flakiness.
	if elapsed < LockTimeout/2 {
		t.Fatalf("WithLock returned after %s, want it to wait ~%s for the contended lock", elapsed, LockTimeout)
	}
}
