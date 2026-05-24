package wsstore

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

var testNow = time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)

func TestOpenCloseReopenCreatesWorktreeDatabase(t *testing.T) {
	root := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{CacheHome: cache, Now: func() time.Time { return testNow }})
	store, err := manager.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	path := store.Path()
	if filepath.Base(path) != "state.sqlite" {
		t.Fatalf("db path = %q, want state.sqlite", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state db missing: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := manager.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.Path() != path {
		t.Fatalf("reopened path = %q, want %q", reopened.Path(), path)
	}
}

func TestConcurrentShortActorWrites(t *testing.T) {
	root := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	ctx := context.Background()
	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store, err := NewManager(Options{CacheHome: cache}).Open(root)
			if err != nil {
				errs <- err
				return
			}
			defer store.Close()
			errs <- store.UpsertActor(ctx, Actor{
				ActorID:     fmt.Sprintf("actor-%02d", i),
				Authority:   "lead",
				RootPath:    root,
				WorktreeKey: store.Layout().WorktreeKey,
				Status:      "active",
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	store, err := NewManager(Options{CacheHome: cache}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	count, err := store.Count(ctx, "actors")
	if err != nil {
		t.Fatal(err)
	}
	if count != workers {
		t.Fatalf("actor count = %d, want %d", count, workers)
	}
}

func TestArtifactMetadataKeepsFileBackedStreamPath(t *testing.T) {
	root := initRepo(t)
	store := openStore(t, root)
	defer store.Close()
	stream := filepath.Join(t.TempDir(), "stdout")
	if err := os.WriteFile(stream, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := store.UpsertArtifact(context.Background(), Artifact{
		ArtifactID: "stream-1",
		Kind:       "exec.stdout",
		Path:       stream,
		State:      ArtifactStateCompleted,
		ByteCount:  5,
		ExpiresAt:  testNow.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Artifact(context.Background(), "stream-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.Path != stream || got.ByteCount != 5 || got.Kind != "exec.stdout" {
		t.Fatalf("artifact metadata = %#v, ok=%t", got, ok)
	}
	if err := store.UpsertRetentionPolicy(context.Background(), RetentionPolicy{Scope: "exec.stdout", TTL: time.Hour, MaxRows: 100, MaxBytes: 1024}); err != nil {
		t.Fatal(err)
	}
	count, err := store.Count(context.Background(), "retention_policies")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("retention policy count = %d, want 1", count)
	}
}

func TestPruneSkipsActiveAndPinnedState(t *testing.T) {
	root := initRepo(t)
	store := openStore(t, root)
	defer store.Close()
	ctx := context.Background()
	expired := testNow.Add(-time.Hour)
	activePath := writeArtifactFile(t, "active")
	pinnedPath := writeArtifactFile(t, "pinned")
	for _, artifact := range []Artifact{
		{ArtifactID: "active", Kind: "exec.stdout", Path: activePath, State: ArtifactStateRunning, ExpiresAt: expired},
		{ArtifactID: "pinned", Kind: "exec.stdout", Path: pinnedPath, State: ArtifactStateCompleted, Pinned: true, ExpiresAt: expired},
	} {
		if err := store.UpsertArtifact(ctx, artifact); err != nil {
			t.Fatal(err)
		}
	}
	result, err := store.PruneExpired(ctx, PruneOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 0 || result.Deleted != 0 {
		t.Fatalf("prune result = %#v, want skip", result)
	}
	if _, err := os.Stat(activePath); err != nil {
		t.Fatalf("active artifact was removed: %v", err)
	}
	if _, err := os.Stat(pinnedPath); err != nil {
		t.Fatalf("pinned artifact was removed: %v", err)
	}
}

func TestPruneDeletesExpiredCompletedArtifacts(t *testing.T) {
	root := initRepo(t)
	store := openStore(t, root)
	defer store.Close()
	ctx := context.Background()
	path := writeArtifactFile(t, "done")
	if err := store.UpsertArtifact(ctx, Artifact{ArtifactID: "done", Kind: "agent.output", Path: path, State: ArtifactStateCompleted, ExpiresAt: testNow.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	result, err := store.PruneExpired(ctx, PruneOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 {
		t.Fatalf("prune result = %#v, want one delete", result)
	}
	runs, err := store.Count(ctx, "prune_runs")
	if err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("prune runs = %d, want 1", runs)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("artifact file still exists or stat failed differently: %v", err)
	}
	if _, ok, err := store.Artifact(ctx, "done"); err != nil || ok {
		t.Fatalf("artifact row remains ok=%t err=%v", ok, err)
	}
}

func TestPruneTombstoneRetry(t *testing.T) {
	root := initRepo(t)
	store := openStore(t, root)
	defer store.Close()
	ctx := context.Background()
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "child"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertArtifact(ctx, Artifact{ArtifactID: "blocked", Kind: "exec.stdout", Path: blocked, State: ArtifactStateCompleted, ExpiresAt: testNow.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	result, err := store.PruneExpired(ctx, PruneOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Tombstoned != 1 {
		t.Fatalf("prune result = %#v, want tombstone", result)
	}
	tombs, err := store.Count(ctx, "artifact_tombstones")
	if err != nil {
		t.Fatal(err)
	}
	if tombs != 1 {
		t.Fatalf("tombstones = %d, want 1", tombs)
	}
	if err := os.Remove(filepath.Join(blocked, "child")); err != nil {
		t.Fatal(err)
	}
	result, err = store.PruneExpired(ctx, PruneOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.RetrySucceeded != 1 {
		t.Fatalf("retry result = %#v, want success", result)
	}
	tombs, _ = store.Count(ctx, "artifact_tombstones")
	if tombs != 0 {
		t.Fatalf("tombstones after retry = %d, want 0", tombs)
	}
}

func openStore(t *testing.T, root string) *Store {
	t.Helper()
	store, err := NewManager(Options{
		CacheHome: filepath.Join(t.TempDir(), "cache"),
		Now:       func() time.Time { return testNow },
	}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func writeArtifactFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustRun(t, root, "git", "init")
	mustRun(t, root, "git", "config", "user.email", "a@example.com")
	mustRun(t, root, "git", "config", "user.name", "A")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-m", "init")
	return root
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, string(out))
	}
}
