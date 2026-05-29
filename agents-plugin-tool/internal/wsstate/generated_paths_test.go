package wsstate

import (
	"os"
	"path/filepath"
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
