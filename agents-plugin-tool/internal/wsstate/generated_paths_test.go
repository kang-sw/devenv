package wsstate

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestGeneratePathsAllocatesReviewPathsInStableOrder(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return fixedNow },
	})

	paths, err := manager.GeneratePaths(repo, "review", []string{"correctness", "fit"})
	if err != nil {
		t.Fatalf("GeneratePaths returned error: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("len(paths) = %d, want 2", len(paths))
	}
	if paths[0].Stem != "correctness" || paths[1].Stem != "fit" {
		t.Fatalf("stems not preserved in stable order: %+v", paths)
	}
	layout, _, _, err := manager.Ensure(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, generated := range paths {
		if generated.Kind != "review" {
			t.Fatalf("kind = %q, want review", generated.Kind)
		}
		canonicalGenerated := canonicalForTest(t, generated.Path)
		if !strings.HasPrefix(canonicalGenerated, layout.ReviewDir+string(os.PathSeparator)) {
			t.Fatalf("path %q is not under review dir %q", generated.Path, layout.ReviewDir)
		}
		if !strings.HasSuffix(generated.Path, "-"+generated.Stem+".md") {
			t.Fatalf("path %q does not end with sanitized stem %q", generated.Path, generated.Stem)
		}
		if strings.Contains(filepath.Base(generated.Path), "T") {
			t.Fatalf("path %q unexpectedly includes timestamp", generated.Path)
		}
		if info, err := os.Stat(generated.Path); err != nil || info.IsDir() {
			t.Fatalf("reserved path %q stat=%v err=%v", generated.Path, info, err)
		}
	}
}

func TestGeneratePathsSanitizesStemAndKeepsAllocationsUnique(t *testing.T) {
	repo := initRepo(t)
	manager := NewManager(Options{
		CacheHome: filepath.Join(t.TempDir(), "cache"),
		Now:       func() time.Time { return fixedNow },
	})

	first, err := manager.GeneratePaths(repo, "review", []string{"../bad stem", "***"})
	if err != nil {
		t.Fatalf("first GeneratePaths returned error: %v", err)
	}
	second, err := manager.GeneratePaths(repo, "review", []string{"../bad stem", "***"})
	if err != nil {
		t.Fatalf("second GeneratePaths returned error: %v", err)
	}
	if first[0].Stem != "bad-stem" || first[1].Stem != "unnamed" {
		t.Fatalf("unexpected sanitized stems: %+v", first)
	}
	for i := range first {
		if first[i].Path == second[i].Path {
			t.Fatalf("allocation %d reused path %q", i, first[i].Path)
		}
	}
}

func TestGeneratePathsKeepsDuplicateStemsUniqueWithinAllocation(t *testing.T) {
	repo := initRepo(t)
	manager := NewManager(Options{
		CacheHome: filepath.Join(t.TempDir(), "cache"),
		Now:       func() time.Time { return fixedNow },
	})

	paths, err := manager.GeneratePaths(repo, "review", []string{"direct", "direct"})
	if err != nil {
		t.Fatalf("GeneratePaths returned error: %v", err)
	}
	if paths[0].Path == paths[1].Path {
		t.Fatalf("duplicate stems reused path %q", paths[0].Path)
	}
	for _, path := range paths {
		if path.Stem != "direct" {
			t.Fatalf("stem = %q, want direct", path.Stem)
		}
	}
}

func TestGeneratePathsValidatesKindAndStems(t *testing.T) {
	repo := initRepo(t)
	manager := NewManager(Options{
		CacheHome: filepath.Join(t.TempDir(), "cache"),
		Now:       func() time.Time { return fixedNow },
	})

	if _, err := manager.GeneratePaths(repo, "", []string{"direct"}); err == nil {
		t.Fatalf("missing kind returned nil error")
	}
	if _, err := manager.GeneratePaths(repo, "scratch", []string{"direct"}); err == nil {
		t.Fatalf("unsupported kind returned nil error")
	}
	if _, err := manager.GeneratePaths(repo, "review", nil); err == nil {
		t.Fatalf("missing stems returned nil error")
	}
}

func TestGeneratePathsUsesWorktreeScopedReviewDirectory(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return fixedNow },
	})

	paths, err := manager.GeneratePaths(repo, "review", []string{"direct"})
	if err != nil {
		t.Fatalf("GeneratePaths returned error: %v", err)
	}
	layout, _, _, err := manager.Ensure(repo)
	if err != nil {
		t.Fatal(err)
	}
	gotDir := canonicalForTest(t, filepath.Dir(paths[0].Path))
	if gotDir != layout.ReviewDir {
		t.Fatalf("path dir = %q, want %q", gotDir, layout.ReviewDir)
	}
}

func TestGeneratePathsUsesWorktreeScopedPromptDirectory(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return fixedNow },
	})

	paths, err := manager.GeneratePaths(repo, "prompt", []string{"code-reviewer"})
	if err != nil {
		t.Fatalf("GeneratePaths returned error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("len(paths) = %d, want 1", len(paths))
	}
	if paths[0].Kind != "prompt" {
		t.Fatalf("kind = %q, want prompt", paths[0].Kind)
	}
	layout, _, _, err := manager.Ensure(repo)
	if err != nil {
		t.Fatal(err)
	}
	gotDir := canonicalForTest(t, filepath.Dir(paths[0].Path))
	if gotDir != layout.PromptDir {
		t.Fatalf("path dir = %q, want %q", gotDir, layout.PromptDir)
	}
	if !strings.HasSuffix(paths[0].Path, ".md") {
		t.Fatalf("path %q does not end with .md", paths[0].Path)
	}
	if info, err := os.Stat(paths[0].Path); err != nil || info.IsDir() {
		t.Fatalf("reserved prompt path %q stat=%v err=%v", paths[0].Path, info, err)
	}
}

func TestGeneratePathsAllocatesPlanPathsUnderRepo(t *testing.T) {
	repo := initRepo(t)
	manager := NewManager(Options{
		CacheHome: filepath.Join(t.TempDir(), "cache"),
		Now:       func() time.Time { return fixedNow },
	})

	paths, err := manager.GeneratePaths(repo, "plan", []string{"260628-feat-demo"})
	if err != nil {
		t.Fatalf("GeneratePaths returned error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("len(paths) = %d, want 1", len(paths))
	}
	if paths[0].Kind != "plan" {
		t.Fatalf("kind = %q, want plan", paths[0].Kind)
	}
	if paths[0].Stem != "260628-feat-demo" {
		t.Fatalf("stem = %q, want sanitized ticket stem", paths[0].Stem)
	}
	localNow := fixedNow.Local()
	want := filepath.Join(repo, "ai-docs", ".plans", localNow.Format("2006-01"), localNow.Format("02-1504")+"-260628-feat-demo.md")
	if canonicalForTest(t, paths[0].Path) != canonicalForTest(t, want) {
		t.Fatalf("plan path = %q, want %q", paths[0].Path, want)
	}
	if info, err := os.Stat(paths[0].Path); err != nil || info.IsDir() {
		t.Fatalf("reserved plan path %q stat=%v err=%v", paths[0].Path, info, err)
	}
}

// TestGeneratePathsAllocatesCloneDocsUnderSharedDir verifies the "clone" kind
// allocates a readable <stem>-<6-char-suffix>.md filename under
// layout.SharedDir/docs, not the opaque runID-NN scheme review/prompt use.
func TestGeneratePathsAllocatesCloneDocsUnderSharedDir(t *testing.T) {
	repo := initRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	manager := NewManager(Options{
		CacheHome: cache,
		Now:       func() time.Time { return fixedNow },
	})

	paths, err := manager.GeneratePaths(repo, "clone", []string{"context-budget"})
	if err != nil {
		t.Fatalf("GeneratePaths returned error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("len(paths) = %d, want 1", len(paths))
	}
	if paths[0].Kind != "clone" {
		t.Fatalf("kind = %q, want clone", paths[0].Kind)
	}
	if paths[0].Stem != "context-budget" {
		t.Fatalf("stem = %q, want context-budget", paths[0].Stem)
	}
	layout, _, _, err := manager.Ensure(repo)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := canonicalForTest(t, filepath.Join(layout.SharedDir, "docs"))
	gotDir := canonicalForTest(t, filepath.Dir(paths[0].Path))
	if gotDir != wantDir {
		t.Fatalf("path dir = %q, want %q", gotDir, wantDir)
	}
	base := filepath.Base(paths[0].Path)
	if !regexp.MustCompile(`^context-budget-[a-z0-9]{6}\.md$`).MatchString(base) {
		t.Fatalf("clone path base = %q, want <stem>-<6-char-suffix>.md shape", base)
	}
	if info, err := os.Stat(paths[0].Path); err != nil || info.IsDir() {
		t.Fatalf("reserved clone path %q stat=%v err=%v", paths[0].Path, info, err)
	}
}

// TestGeneratePathsCloneUniquePerCall verifies two clone allocations with the
// same stem produce different paths (fresh random suffix per attempt, not a
// deterministic one), mirroring
// TestGeneratePathsSanitizesStemAndKeepsAllocationsUnique for the review kind.
func TestGeneratePathsCloneUniquePerCall(t *testing.T) {
	repo := initRepo(t)
	manager := NewManager(Options{
		CacheHome: filepath.Join(t.TempDir(), "cache"),
		Now:       func() time.Time { return fixedNow },
	})

	first, err := manager.GeneratePaths(repo, "clone", []string{"../bad stem"})
	if err != nil {
		t.Fatalf("first GeneratePaths returned error: %v", err)
	}
	second, err := manager.GeneratePaths(repo, "clone", []string{"../bad stem"})
	if err != nil {
		t.Fatalf("second GeneratePaths returned error: %v", err)
	}
	if first[0].Stem != "bad-stem" {
		t.Fatalf("stem = %q, want bad-stem", first[0].Stem)
	}
	if first[0].Path == second[0].Path {
		t.Fatalf("second clone allocation reused path %q", first[0].Path)
	}
}

// TestGeneratePathsCloneRetriesOnSuffixCollisionWithoutTruncating forces a
// deterministic suffix collision on the first reservation attempt (via the
// cloneSuffixGenerator test seam) and asserts GeneratePaths retries with the
// next generated suffix rather than truncating the pre-existing sibling clone
// doc (O_EXCL, never O_TRUNC).
func TestGeneratePathsCloneRetriesOnSuffixCollisionWithoutTruncating(t *testing.T) {
	repo := initRepo(t)
	manager := NewManager(Options{
		CacheHome: filepath.Join(t.TempDir(), "cache"),
		Now:       func() time.Time { return fixedNow },
	})
	layout, _, _, err := manager.Ensure(repo)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(layout.SharedDir, "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) returned error: %v", dir, err)
	}

	preCreated := filepath.Join(dir, "collide-aaaaaa.md")
	const sentinel = "pre-existing sibling clone doc content"
	if err := os.WriteFile(preCreated, []byte(sentinel), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) returned error: %v", preCreated, err)
	}

	suffixes := []string{"aaaaaa", "bbbbbb"}
	call := 0
	prevGenerator := cloneSuffixGenerator
	cloneSuffixGenerator = func() (string, error) {
		suffix := suffixes[call]
		if call < len(suffixes)-1 {
			call++
		}
		return suffix, nil
	}
	t.Cleanup(func() { cloneSuffixGenerator = prevGenerator })

	paths, err := manager.GeneratePaths(repo, "clone", []string{"collide"})
	if err != nil {
		t.Fatalf("GeneratePaths returned error: %v", err)
	}
	wantPath := filepath.Join(dir, "collide-bbbbbb.md")
	if canonicalForTest(t, paths[0].Path) != canonicalForTest(t, wantPath) {
		t.Fatalf("clone path after collision = %q, want %q (retried with next suffix)", paths[0].Path, wantPath)
	}
	if call != 1 {
		t.Fatalf("cloneSuffixGenerator called %d times, want exactly 1 retry after the forced collision", call)
	}
	got, err := os.ReadFile(preCreated)
	if err != nil {
		t.Fatalf("ReadFile(%q) returned error: %v", preCreated, err)
	}
	if string(got) != sentinel {
		t.Fatalf("pre-existing sibling clone doc was mutated: got %q, want %q", got, sentinel)
	}
}

// TestGeneratePathsCloneResolvesToSameDirFromLinkedWorktree verifies a linked
// worktree's "clone" generation resolves to the same SharedDir/docs directory
// as the main worktree, confirming SharedDir's clone-shared,
// worktree-agnostic property requires no new cross-worktree resolution logic.
func TestGeneratePathsCloneResolvesToSameDirFromLinkedWorktree(t *testing.T) {
	repo := initRepo(t)
	worktreeParent := t.TempDir()
	worktreePath := filepath.Join(worktreeParent, "feature-test")
	runGit(t, repo, "worktree", "add", "-b", "feature/clone-test", worktreePath, "HEAD")

	manager := NewManager(Options{
		CacheHome: filepath.Join(t.TempDir(), "cache"),
		Now:       func() time.Time { return fixedNow },
	})

	rootPaths, err := manager.GeneratePaths(repo, "clone", []string{"from-root"})
	if err != nil {
		t.Fatalf("GeneratePaths(repo) returned error: %v", err)
	}
	worktreePaths, err := manager.GeneratePaths(worktreePath, "clone", []string{"from-worktree"})
	if err != nil {
		t.Fatalf("GeneratePaths(worktree) returned error: %v", err)
	}

	rootDir := canonicalForTest(t, filepath.Dir(rootPaths[0].Path))
	worktreeDir := canonicalForTest(t, filepath.Dir(worktreePaths[0].Path))
	if rootDir != worktreeDir {
		t.Fatalf("clone dir from worktree = %q, want same as root %q", worktreeDir, rootDir)
	}

	layout, _, _, err := manager.Ensure(repo)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := canonicalForTest(t, filepath.Join(layout.SharedDir, "docs"))
	if rootDir != wantDir {
		t.Fatalf("clone dir = %q, want %q", rootDir, wantDir)
	}
}

func TestGeneratePathsPlanSanitizesStemAndAvoidsCollisions(t *testing.T) {
	repo := initRepo(t)
	manager := NewManager(Options{
		CacheHome: filepath.Join(t.TempDir(), "cache"),
		Now:       func() time.Time { return fixedNow },
	})

	first, err := manager.GeneratePaths(repo, "plan", []string{"../bad stem", "***", "../bad stem"})
	if err != nil {
		t.Fatalf("first GeneratePaths returned error: %v", err)
	}
	second, err := manager.GeneratePaths(repo, "plan", []string{"../bad stem"})
	if err != nil {
		t.Fatalf("second GeneratePaths returned error: %v", err)
	}
	if first[0].Stem != "bad-stem" || first[1].Stem != "unnamed" || first[2].Stem != "bad-stem" {
		t.Fatalf("unexpected sanitized stems: %+v", first)
	}
	localNow := fixedNow.Local()
	wantSuffixes := []string{
		filepath.Join("ai-docs", ".plans", localNow.Format("2006-01"), localNow.Format("02-1504")+"-bad-stem.md"),
		filepath.Join("ai-docs", ".plans", localNow.Format("2006-01"), localNow.Format("02-1504")+"-unnamed.md"),
		filepath.Join("ai-docs", ".plans", localNow.Format("2006-01"), localNow.Format("02-1504")+"-bad-stem-02.md"),
	}
	for i, suffix := range wantSuffixes {
		if !strings.HasSuffix(first[i].Path, suffix) {
			t.Fatalf("path %d = %q, want suffix %q", i, first[i].Path, suffix)
		}
	}
	if !strings.HasSuffix(second[0].Path, filepath.Join("ai-docs", ".plans", localNow.Format("2006-01"), localNow.Format("02-1504")+"-bad-stem-03.md")) {
		t.Fatalf("second path = %q, want collision suffix -03", second[0].Path)
	}
	for _, generated := range append(first, second...) {
		if info, err := os.Stat(generated.Path); err != nil || info.IsDir() {
			t.Fatalf("reserved plan path %q stat=%v err=%v", generated.Path, info, err)
		}
	}
}
