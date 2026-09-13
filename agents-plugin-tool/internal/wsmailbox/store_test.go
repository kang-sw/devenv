package wsmailbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
	for i := 0; i < MaxQueueLen+10; i++ {
		queues = AppendQueue(queues, "bob", Envelope{Content: "m", SentAt: "t"})
	}
	if len(queues["bob"]) != MaxQueueLen {
		t.Fatalf("queue length = %d, want %d", len(queues["bob"]), MaxQueueLen)
	}
}
