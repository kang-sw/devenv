package wsstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 5, 3, 12, 34, 56, 0, time.UTC)

func TestEnsureCreatesStableProjectAndWorktreeLayout(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")

	layout, project, worktree, err := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return fixedNow },
	}).Ensure(repo)
	if err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}

	canonRepo := canonicalForTest(t, repo)
	wantProjectKey := wantKey(canonRepo)
	if project.ProjectKey != wantProjectKey {
		t.Fatalf("project key = %q, want %q", project.ProjectKey, wantProjectKey)
	}
	if worktree.WorktreeKey != wantProjectKey {
		t.Fatalf("normal repo worktree key = %q, want shared project key %q", worktree.WorktreeKey, wantProjectKey)
	}

	for _, dir := range []string{
		layout.LocksDir,
		layout.AgentsDir,
		layout.ReviewDir,
		layout.SessionsDir,
		layout.WorktreeLocksDir,
		layout.TmpDir,
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected directory %s, stat=%v err=%v", dir, info, err)
		}
	}
	if layout.ProjectDir != filepath.Join(cache, "proj", wantProjectKey) {
		t.Fatalf("project dir = %q, want flat project dir", layout.ProjectDir)
	}
	if layout.WorktreeDir != layout.ProjectDir {
		t.Fatalf("normal repo worktree dir = %q, want project dir %q", layout.WorktreeDir, layout.ProjectDir)
	}
	if !strings.HasPrefix(layout.ProjectDir, filepath.Join(cache, "proj")+string(os.PathSeparator)) {
		t.Fatalf("project dir %q is not under cache proj", layout.ProjectDir)
	}
	if layout.WorktreeLocksDir != filepath.Join(layout.WorktreeDir, "locks") {
		t.Fatalf("worktree locks dir = %q, want worktree-local locks dir", layout.WorktreeLocksDir)
	}

	var projectFile ProjectMetadata
	readJSON(t, layout.ProjectMeta, &projectFile)
	if projectFile.RootPath != canonRepo || projectFile.RootID != shortHash(canonRepo) {
		t.Fatalf("project metadata mismatch: %+v, root %q", projectFile, canonRepo)
	}
	if projectFile.CreatedAt != fixedNow.Format(time.RFC3339) || projectFile.LastSeenAt != fixedNow.Format(time.RFC3339) {
		t.Fatalf("project timestamps mismatch: %+v", projectFile)
	}

	var worktreeFile WorktreeMetadata
	readJSON(t, layout.WorktreeMeta, &worktreeFile)
	if worktreeFile.WorktreePath != canonRepo || worktreeFile.WorktreeID != shortHash(canonRepo) {
		t.Fatalf("worktree metadata mismatch: %+v, root %q", worktreeFile, canonRepo)
	}
}

func TestAcquireOrchestratorLockIsWorktreeLocal(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return fixedNow },
	})

	first, err := manager.AcquireOrchestratorLock(repo, "test-version")
	if err != nil {
		t.Fatalf("first AcquireOrchestratorLock returned error: %v", err)
	}
	if !first.Owner {
		t.Fatalf("first lock acquisition was not owner: %+v", first)
	}
	if first.Lock.PID != os.Getpid() || first.Lock.Version != "test-version" || first.Lock.WorktreeKey == "" {
		t.Fatalf("first lock payload mismatch: %+v", first.Lock)
	}
	if filepath.Base(first.Path) != orchestratorLockFile {
		t.Fatalf("lock path = %q, want orchestrator lock file", first.Path)
	}

	second, err := manager.AcquireOrchestratorLock(repo, "test-version")
	if err != nil {
		t.Fatalf("second AcquireOrchestratorLock returned error: %v", err)
	}
	if second.Owner {
		t.Fatalf("second lock acquisition unexpectedly became owner: %+v", second)
	}
	if second.Lock.PID != os.Getpid() {
		t.Fatalf("second lock did not report existing owner: %+v", second.Lock)
	}
}

func TestAcquireOrchestratorLockRecoversStaleLock(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return fixedNow },
	})
	layout, _, worktree, err := manager.Ensure(repo)
	if err != nil {
		t.Fatal(err)
	}
	stale := OrchestratorLock{
		SchemaVersion: schemaVersion,
		PID:           999999999,
		StartedAt:     fixedNow.Add(-time.Hour).Format(time.RFC3339),
		Root:          worktree.WorktreePath,
		WorktreeKey:   worktree.WorktreeKey,
		Version:       "stale",
	}
	if err := os.WriteFile(filepath.Join(layout.WorktreeLocksDir, orchestratorLockFile), mustMarshalForTest(t, stale), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := manager.AcquireOrchestratorLock(repo, "fresh")
	if err != nil {
		t.Fatalf("AcquireOrchestratorLock returned error: %v", err)
	}
	if !got.Owner {
		t.Fatalf("stale lock was not recovered: %+v", got)
	}
	if got.Lock.PID != os.Getpid() || got.Lock.Version != "fresh" {
		t.Fatalf("fresh lock payload mismatch: %+v", got.Lock)
	}
}

func TestEnsurePreservesCreatedAtAndUpdatesLastSeenAt(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return fixedNow },
	})
	layout, _, _, err := manager.Ensure(repo)
	if err != nil {
		t.Fatalf("first Ensure returned error: %v", err)
	}

	later := fixedNow.Add(2 * time.Hour)
	manager = NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return later },
	})
	if _, _, _, err := manager.Ensure(repo); err != nil {
		t.Fatalf("second Ensure returned error: %v", err)
	}

	var project ProjectMetadata
	readJSON(t, layout.ProjectMeta, &project)
	if project.CreatedAt != fixedNow.Format(time.RFC3339) {
		t.Fatalf("CreatedAt changed: %+v", project)
	}
	if project.LastSeenAt != later.Format(time.RFC3339) {
		t.Fatalf("LastSeenAt not updated: %+v", project)
	}
}

func TestLinkedWorktreeSharesProjectIdentityAndSeparatesWorktreeState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("git worktree temp path behavior is covered on Unix CI/local hosts for now")
	}
	repo := initRepo(t)
	worktreeParent := t.TempDir()
	worktreePath := filepath.Join(worktreeParent, "feature-test")
	runGit(t, repo, "worktree", "add", "-b", "feature/test", worktreePath, "HEAD")

	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return fixedNow },
	})
	rootLayout, rootProject, rootWorktree, err := manager.Ensure(repo)
	if err != nil {
		t.Fatalf("Ensure(root) returned error: %v", err)
	}
	linkedLayout, linkedProject, linkedWorktree, err := manager.Ensure(worktreePath)
	if err != nil {
		t.Fatalf("Ensure(worktree) returned error: %v", err)
	}

	if linkedProject.ProjectKey != rootProject.ProjectKey {
		t.Fatalf("linked worktree project key = %q, want root key %q", linkedProject.ProjectKey, rootProject.ProjectKey)
	}
	if linkedProject.RootPath != rootProject.RootPath {
		t.Fatalf("linked root path = %q, want %q", linkedProject.RootPath, rootProject.RootPath)
	}
	if linkedWorktree.WorktreeKey == rootWorktree.WorktreeKey {
		t.Fatalf("linked worktree reused root worktree key %q", linkedWorktree.WorktreeKey)
	}
	if linkedLayout.WorktreeDir == rootLayout.WorktreeDir {
		t.Fatalf("linked worktree reused root worktree dir %q", linkedLayout.WorktreeDir)
	}

	canonWorktree := canonicalForTest(t, worktreePath)
	wantWorktreeKey := wantKey(canonicalForTest(t, repo)) + "@" + wantKey(canonWorktree)
	if linkedWorktree.WorktreeKey != wantWorktreeKey {
		t.Fatalf("linked worktree key = %q, want %q", linkedWorktree.WorktreeKey, wantWorktreeKey)
	}
	if linkedWorktree.RootPath != canonicalForTest(t, repo) {
		t.Fatalf("linked worktree root path = %q, want main repo path", linkedWorktree.RootPath)
	}

	rootLock, err := manager.AcquireOrchestratorLock(repo, "test")
	if err != nil {
		t.Fatalf("AcquireOrchestratorLock(root) returned error: %v", err)
	}
	linkedLock, err := manager.AcquireOrchestratorLock(worktreePath, "test")
	if err != nil {
		t.Fatalf("AcquireOrchestratorLock(worktree) returned error: %v", err)
	}
	if !rootLock.Owner || !linkedLock.Owner {
		t.Fatalf("linked worktrees should acquire independent locks, root=%+v linked=%+v", rootLock, linkedLock)
	}
	if rootLock.Path == linkedLock.Path {
		t.Fatalf("linked worktree reused root lock path %q", rootLock.Path)
	}
}

func TestResolveDoesNotRequireOriginRemote(t *testing.T) {
	repo := initRepo(t)
	if out := runGit(t, repo, "remote"); strings.TrimSpace(out) != "" {
		t.Fatalf("test repo unexpectedly has remotes: %q", out)
	}
	_, project, _, err := NewManager(Options{
		CacheHome: filepath.Join(t.TempDir(), "cache"),
		Now:       func() time.Time { return fixedNow },
	}).Resolve(repo)
	if err != nil {
		t.Fatalf("Resolve returned error for no-origin repo: %v", err)
	}
	if project.ProjectKey == "" || project.RootPath == "" {
		t.Fatalf("incomplete project metadata for no-origin repo: %+v", project)
	}
}

func TestCacheRootUsesExplicitOptionThenEnvThenHome(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "explicit")
	got, err := CacheRoot(Options{CacheHome: explicit})
	if err != nil {
		t.Fatalf("CacheRoot explicit returned error: %v", err)
	}
	if got != canonicalForTest(t, explicit) {
		t.Fatalf("CacheRoot explicit = %q, want %q", got, explicit)
	}

	env := filepath.Join(t.TempDir(), "env")
	t.Setenv(envCacheHome, env)
	got, err = CacheRoot(Options{})
	if err != nil {
		t.Fatalf("CacheRoot env returned error: %v", err)
	}
	if got != canonicalForTest(t, env) {
		t.Fatalf("CacheRoot env = %q, want %q", got, env)
	}

	t.Setenv(envCacheHome, "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err = CacheRoot(Options{})
	if err != nil {
		t.Fatalf("CacheRoot default returned error: %v", err)
	}
	if got != filepath.Join(home, ".cache", defaultCacheDirName) {
		t.Fatalf("CacheRoot default = %q", got)
	}
}

func TestShortHashUsesEightCharacters(t *testing.T) {
	if got := shortHash("/tmp/example"); len(got) != 8 {
		t.Fatalf("short hash length = %d, value %q", len(got), got)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "devenv")
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

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func readJSON(t *testing.T, path string, dest any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		t.Fatalf("unmarshal %s: %v\n%s", path, err, string(raw))
	}
}

func mustMarshalForTest(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}

func canonicalForTest(t *testing.T, path string) string {
	t.Helper()
	got, err := canonicalPath(path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func wantKey(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])[:8]
}
