package wsreview

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// reviewTestInitRepoOnBranch inits a repo with an explicit initial branch
// name (avoiding any dependency on the test runner's init.defaultBranch git
// config) and seeds one commit on it.
func reviewTestInitRepoOnBranch(t *testing.T, root, branch string) {
	t.Helper()
	reviewTestRunGit(t, root, "init", "-b", branch)
	reviewTestRunGit(t, root, "config", "user.email", "test@example.com")
	reviewTestRunGit(t, root, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed fixture file: %v", err)
	}
	reviewTestRunGit(t, root, "add", "seed.txt")
	reviewTestRunGit(t, root, "commit", "-m", "seed commit")
}

func TestResolveTrackViaOriginHEAD(t *testing.T) {
	root := t.TempDir()
	reviewTestInitRepoOnBranch(t, root, "trunk")

	// Simulate a fetched remote-tracking branch and its symbolic-ref HEAD,
	// without needing a real network remote.
	reviewTestRunGit(t, root, "remote", "add", "origin", root)
	reviewTestRunGit(t, root, "fetch", "origin", "trunk")
	reviewTestRunGit(t, root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")

	got, err := ResolveTrack(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveTrack failed: %v", err)
	}
	if got != "trunk" {
		t.Fatalf("ResolveTrack = %q, want %q", got, "trunk")
	}
}

func TestResolveTrackFallsBackToLocalMain(t *testing.T) {
	root := t.TempDir()
	reviewTestInitRepoOnBranch(t, root, "seed-branch")
	reviewTestRunGit(t, root, "checkout", "-b", "main")

	got, err := ResolveTrack(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveTrack failed: %v", err)
	}
	if got != "main" {
		t.Fatalf("ResolveTrack = %q, want %q", got, "main")
	}
}

func TestResolveTrackFallsBackToLocalMaster(t *testing.T) {
	root := t.TempDir()
	reviewTestInitRepoOnBranch(t, root, "seed-branch")
	reviewTestRunGit(t, root, "checkout", "-b", "master")

	got, err := ResolveTrack(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveTrack failed: %v", err)
	}
	if got != "master" {
		t.Fatalf("ResolveTrack = %q, want %q", got, "master")
	}
}

func TestResolveTrackPrefersAgentsMdDeclarationOverGitDefault(t *testing.T) {
	root := t.TempDir()
	reviewTestInitRepoOnBranch(t, root, "main")

	agentsMD := "# AGENTS.md\n\n## Workflow\n\n### Review Policy\nreview-track: develop\nrelease-boundary: present\nrendezvous-backend: canary\nrelease-tag-glob: v*\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write AGENTS.md fixture: %v", err)
	}

	got, err := ResolveTrack(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveTrack failed: %v", err)
	}
	if got != "develop" {
		t.Fatalf("ResolveTrack = %q, want %q (AGENTS.md declaration should win even though the git default branch is %q)", got, "develop", "main")
	}
}

func TestResolveTrackFailsOpenWithNoOriginNoMainNoMaster(t *testing.T) {
	root := t.TempDir()
	reviewTestInitRepoOnBranch(t, root, "feature-only")

	if _, err := ResolveTrack(context.Background(), root); err == nil {
		t.Fatalf("ResolveTrack should fail when no origin/HEAD and neither main nor master exists")
	}
}
