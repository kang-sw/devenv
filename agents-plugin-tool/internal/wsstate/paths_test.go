package wsstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

func TestEnsureSkipsALastSeenOnlyRewriteWithinTheWindow(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	ensureAt := func(now time.Time) Layout {
		t.Helper()
		layout, _, _, err := NewManager(Options{CacheHome: cache, Now: func() time.Time { return now }}).Ensure(repo)
		if err != nil {
			t.Fatalf("Ensure at %v: %v", now, err)
		}
		return layout
	}
	layout := ensureAt(fixedNow)
	stamps := func() (os.FileInfo, os.FileInfo) {
		t.Helper()
		p, err := os.Stat(layout.ProjectMeta)
		if err != nil {
			t.Fatal(err)
		}
		w, err := os.Stat(layout.WorktreeMeta)
		if err != nil {
			t.Fatal(err)
		}
		return p, w
	}
	p0, w0 := stamps()

	ensureAt(fixedNow.Add(lastSeenRefresh - time.Second))
	if p1, w1 := stamps(); !SameFileMetadata(p0, p1) || !SameFileMetadata(w0, w1) {
		t.Fatal("an Ensure inside the window rewrote metadata whose only change was last_seen_at")
	}
	var project ProjectMetadata
	readJSON(t, layout.ProjectMeta, &project)
	if project.LastSeenAt != fixedNow.Format(time.RFC3339) {
		t.Fatalf("skipped Ensure changed last_seen_at: %+v", project)
	}

	// A real change is written at once, inside the window.
	project.RepoBasename = "stale"
	if err := os.WriteFile(layout.ProjectMeta, mustMarshalForTest(t, project), 0o644); err != nil {
		t.Fatal(err)
	}
	later := fixedNow.Add(time.Minute)
	ensureAt(later)
	readJSON(t, layout.ProjectMeta, &project)
	if project.RepoBasename != filepath.Base(canonicalForTest(t, repo)) || project.LastSeenAt != later.Format(time.RFC3339) || project.CreatedAt != fixedNow.Format(time.RFC3339) {
		t.Fatalf("Ensure did not rewrite a changed project.json: %+v", project)
	}

	// Past the window last_seen_at alone is refreshed.
	past := later.Add(lastSeenRefresh)
	ensureAt(past)
	var worktree WorktreeMetadata
	readJSON(t, layout.WorktreeMeta, &worktree)
	if worktree.LastSeenAt != past.Format(time.RFC3339) {
		t.Fatalf("Ensure past the window did not refresh last_seen_at: %+v", worktree)
	}
}

// gitEnvWithoutGit is the process's GIT_* environment (the identity cache
// key) with a PATH that holds no git, so any git spawn fails.
func gitEnvWithoutGit(t *testing.T) []string {
	t.Helper()
	env := []string{"PATH=" + t.TempDir()}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(kv), "GIT_") {
			env = append(env, kv)
		}
	}
	return env
}

func TestGitIdentityIsRememberedPerTopLevelAndRevalidatedByItsDotGit(t *testing.T) {
	repo := initRepo(t)
	worktreePath := filepath.Join(t.TempDir(), "linked")
	runGit(t, repo, "worktree", "add", "-b", "feature/cache", worktreePath, "HEAD")
	dotGit := filepath.Join(worktreePath, ".git")
	backdate := func() {
		old := time.Now().Add(-time.Hour)
		if err := os.Chtimes(dotGit, old, old); err != nil {
			t.Fatal(err)
		}
	}
	backdate()
	cache := filepath.Join(t.TempDir(), "cache")
	withGit := NewManager(Options{CacheHome: cache, Now: func() time.Time { return fixedNow }})
	_, wantProject, wantWorktree, err := withGit.Resolve(worktreePath)
	if err != nil {
		t.Fatal(err)
	}

	// No git on PATH: only the remembered identity can answer.
	noGit := NewManager(Options{CacheHome: cache, Now: func() time.Time { return fixedNow }, Env: gitEnvWithoutGit(t)})
	_, project, worktree, err := noGit.Resolve(worktreePath)
	if err != nil {
		t.Fatalf("Resolve without git did not use the remembered identity: %v", err)
	}
	if project != wantProject || worktree != wantWorktree {
		t.Fatalf("remembered identity = %+v %+v, want %+v %+v", project, worktree, wantProject, wantWorktree)
	}

	// Rewriting the worktree's .git file invalidates it: git must run again.
	raw, err := os.ReadFile(dotGit)
	if err != nil {
		t.Fatal(err)
	}
	// Git for Windows hides .git. Open the existing file without O_CREATE:
	// Go 1.23 maps create+truncate to CREATE_ALWAYS with normal attributes,
	// which Windows refuses for an existing hidden file.
	file, err := os.OpenFile(dotGit, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.Write(raw) // same bytes, new mtime
	closeErr := file.Close()      // close before git revalidates the file
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if _, _, _, err := noGit.Resolve(worktreePath); err == nil {
		t.Fatal("Resolve after a .git change answered without running git")
	}
	if _, _, _, err := withGit.Resolve(worktreePath); err != nil {
		t.Fatal(err)
	}

	// A subdirectory is never remembered.
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := withGit.Resolve(sub); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := noGit.Resolve(sub); err == nil {
		t.Fatal("Resolve of a subdirectory answered without running git")
	}
}

// TestLinkedWorktreeSharesProjectIdentityAndSeparatesWorktreeState runs on every
// platform: the assertions are path-separator neutral (filepath.Join,
// canonicalForTest) and both the derived keys and the test expectations flow
// through canonicalPath, so a drive-letter root (Windows t.TempDir lives under
// C:\...\Temp) is exercised the same way as a POSIX root. The earlier Windows
// skip was an untested-surface placeholder, not a known incompatibility.
func TestLinkedWorktreeSharesProjectIdentityAndSeparatesWorktreeState(t *testing.T) {
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

// TestResolveTreatsSubmoduleWorkingTreesAsIndependentProjects covers the
// submodule-root layout resolution contract: a submodule working tree
// resolves as an independent single-worktree project (root == commonRoot,
// so WorktreeKey carries no "@" suffix), both under a superproject's main
// checkout and under a linked git worktree of that superproject. The two
// submodule checkouts are expected to receive distinct ProjectKeys (accepted
// side effect: each superproject worktree's submodule has its own
// .git/worktrees/<wt>/modules/<sub> git-common-dir instance).
func TestResolveTreatsSubmoduleWorkingTreesAsIndependentProjects(t *testing.T) {
	superRepo, mainSubPath := initSubmoduleSuperproject(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return fixedNow },
	})

	var mainProjectKey, linkedProjectKey string

	t.Run("main worktree submodule", func(t *testing.T) {
		layout, project, worktree, err := manager.Ensure(mainSubPath)
		if err != nil {
			t.Fatalf("Ensure(submodule) returned error: %v", err)
		}
		canonSub := canonicalForTest(t, mainSubPath)
		if project.RootPath != canonSub {
			t.Fatalf("submodule project root path = %q, want %q", project.RootPath, canonSub)
		}
		if worktree.WorktreePath != canonSub {
			t.Fatalf("submodule worktree path = %q, want %q", worktree.WorktreePath, canonSub)
		}
		if worktree.WorktreeKey != project.ProjectKey {
			t.Fatalf("submodule worktree key = %q, want project key %q (no @ suffix)", worktree.WorktreeKey, project.ProjectKey)
		}
		if strings.Contains(worktree.WorktreeKey, "@") {
			t.Fatalf("submodule worktree key %q unexpectedly contains @", worktree.WorktreeKey)
		}
		if layout.WorktreeDir != layout.ProjectDir {
			t.Fatalf("submodule worktree dir = %q, want project dir %q", layout.WorktreeDir, layout.ProjectDir)
		}
		mainProjectKey = project.ProjectKey
	})

	t.Run("submodule under superproject worktree", func(t *testing.T) {
		superWorktreeParent := t.TempDir()
		superWorktreePath := filepath.Join(superWorktreeParent, "super-feature")
		runGit(t, superRepo, "worktree", "add", "-b", "feature/submodule-test", superWorktreePath, "HEAD")
		runGit(t, superWorktreePath, "-c", "protocol.file.allow=always", "submodule", "update", "--init", "-q")

		linkedSubPath := filepath.Join(superWorktreePath, "sub")
		layout, project, worktree, err := manager.Ensure(linkedSubPath)
		if err != nil {
			t.Fatalf("Ensure(linked submodule) returned error: %v", err)
		}
		canonSub := canonicalForTest(t, linkedSubPath)
		if project.RootPath != canonSub {
			t.Fatalf("linked submodule project root path = %q, want %q", project.RootPath, canonSub)
		}
		if worktree.WorktreePath != canonSub {
			t.Fatalf("linked submodule worktree path = %q, want %q", worktree.WorktreePath, canonSub)
		}
		if worktree.WorktreeKey != project.ProjectKey {
			t.Fatalf("linked submodule worktree key = %q, want project key %q (no @ suffix)", worktree.WorktreeKey, project.ProjectKey)
		}
		if strings.Contains(worktree.WorktreeKey, "@") {
			t.Fatalf("linked submodule worktree key %q unexpectedly contains @", worktree.WorktreeKey)
		}
		if layout.WorktreeDir != layout.ProjectDir {
			t.Fatalf("linked submodule worktree dir = %q, want project dir %q", layout.WorktreeDir, layout.ProjectDir)
		}
		linkedProjectKey = project.ProjectKey
	})

	if mainProjectKey == "" || linkedProjectKey == "" {
		t.Fatal("subtests failed to populate project keys")
	}
	if mainProjectKey == linkedProjectKey {
		t.Fatalf("submodule under superproject worktree reused main submodule project key %q", linkedProjectKey)
	}
}

// A worktree created inside a submodule, at a path the superproject does not
// track, is an explicitly unsupported layout. It reaches commonRootFromGitDir's
// guard and must keep failing loudly rather than being silently reclassified by
// the submodule probe: git reports no superproject for an untracked checkout, so
// the probe declines and the original error stands. A submodule worktree the
// superproject does record as a gitlink is the documented exception and resolves
// instead of failing; see the probe-decline comment in gitIdentity.
func TestResolveRejectsWorktreeCreatedInsideSubmodule(t *testing.T) {
	_, mainSubPath := initSubmoduleSuperproject(t)
	subWorktreePath := filepath.Join(t.TempDir(), "sub-feature")
	runGit(t, mainSubPath, "worktree", "add", "-b", "feature/inside-submodule", subWorktreePath, "HEAD")

	_, _, _, err := NewManager(Options{
		CacheHome: filepath.Join(t.TempDir(), "cache"),
		Now:       func() time.Time { return fixedNow },
	}).Resolve(subWorktreePath)
	if err == nil {
		t.Fatal("Resolve(worktree inside submodule) unexpectedly succeeded; this layout must stay fail-loud")
	}
	if !strings.Contains(err.Error(), "unsupported git common dir") {
		t.Fatalf("Resolve(worktree inside submodule) error = %v, want the unsupported git common dir guard", err)
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
	t.Setenv(EnvCacheHome, env)
	got, err = CacheRoot(Options{})
	if err != nil {
		t.Fatalf("CacheRoot env returned error: %v", err)
	}
	if got != canonicalForTest(t, env) {
		t.Fatalf("CacheRoot env = %q, want %q", got, env)
	}

	t.Setenv(EnvCacheHome, "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err = CacheRoot(Options{})
	if err != nil {
		t.Fatalf("CacheRoot default returned error: %v", err)
	}
	if got != filepath.Join(home, ".cache", DefaultCacheDirName) {
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
	initRepoAt(t, repo)
	return repo
}

func initRepoAt(t *testing.T, repo string) {
	t.Helper()
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
}

// initSubmoduleSuperproject builds a superproject repo with a separate repo
// added as a submodule at "sub", following the initRepo/runGit fixture
// pattern above. The submodule addition is committed in the superproject:
// a later `git worktree add` on the superproject checks out HEAD, and an
// uncommitted submodule addition would leave that checkout without
// .gitmodules/the submodule gitlink (confirmed empirically during survey).
func initSubmoduleSuperproject(t *testing.T) (superRepo, subPath string) {
	t.Helper()
	base := t.TempDir()
	superRepo = filepath.Join(base, "super")
	subOrigin := filepath.Join(base, "sub-origin")

	initRepoAt(t, superRepo)
	initRepoAt(t, subOrigin)

	runGit(t, superRepo, "-c", "protocol.file.allow=always", "submodule", "add", "-q", subOrigin, "sub")
	runGit(t, superRepo, "commit", "-m", "add sub submodule")

	return superRepo, filepath.Join(superRepo, "sub")
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
