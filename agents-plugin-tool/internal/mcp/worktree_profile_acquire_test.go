package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// profileWorktreeFixture builds a repo on `develop` with a ticket board, a
// source tree, a generic dependency directory, and two ignored build-output
// directories (out/ and cache/), leaving the primary worktree on another
// branch so develop is free.
func profileWorktreeFixture(t *testing.T) string {
	t.Helper()
	root := canonicalRootForTest(t, initGitRepo(t))
	for rel, body := range map[string]string{
		".gitignore":                "out/\ncache/\n",
		"ai-docs/tickets/idea/a.md": "a\n",
		"ai-docs/x.md":              "x\n",
		"src/y.go":                  "package y\n",
		"deps/z.txt":                "z\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	runGit(t, root, "branch", "develop")
	runGit(t, root, "switch", "-c", "impl/develop/foo")
	return root
}

func writeFileForTest(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertExists(t *testing.T, dir, rel string, want bool) {
	t.Helper()
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	if got := err == nil; got != want {
		t.Fatalf("%s exists = %v, want %v (stat err %v)", rel, got, want, err)
	}
}

// dropWorktreeMarker turns a pooled worktree into a pre-profile (legacy) one.
func dropWorktreeMarker(t *testing.T, wtPath string) {
	t.Helper()
	dir, err := worktreeAdminDir(context.Background(), wsgit.ExecRunner{}, wtPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, worktreeMarkerFileName)); err != nil {
		t.Fatal(err)
	}
}

func markerProfile(t *testing.T, wtPath string) string {
	t.Helper()
	m, err := readWorktreeMarker(context.Background(), wsgit.ExecRunner{}, wtPath)
	if err != nil || m == nil {
		t.Fatalf("read marker of %s: %+v, %v", wtPath, m, err)
	}
	return m.Profile
}

func lookupOf(profiles ...resolvedWorktreeProfile) worktreeProfileLookup {
	return staticProfileLookup(profiles, nil)
}

// TestProvisionWorktreePreserveSurvivesHygiene: a preserved ignored path
// survives both release hygiene and the next acquire's hygiene; an unpreserved
// ignored path is cleaned at both points.
func TestProvisionWorktreePreserveSurvivesHygiene(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	build := customProfile(t, "build", `{"preserve":["out/"]}`)

	first, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/a", build, "")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	writeFileForTest(t, first.Path, "out/keep.bin", "artifact")
	writeFileForTest(t, first.Path, "cache/drop.bin", "scratch")
	if _, err := releaseWorktree(ctx, runner, first.Path, "", lookupOf(build)); err != nil {
		t.Fatalf("release: %v", err)
	}
	assertExists(t, first.Path, "out/keep.bin", true)
	assertExists(t, first.Path, "cache/drop.bin", false)

	writeFileForTest(t, first.Path, "cache/drop2.bin", "scratch")
	second, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/b", build, "")
	if err != nil {
		t.Fatalf("re-provision: %v", err)
	}
	if !second.Reused || second.Path != first.Path {
		t.Fatalf("same-profile acquire must reuse: %+v", second)
	}
	assertExists(t, second.Path, "out/keep.bin", true)
	assertExists(t, second.Path, "cache/drop2.bin", false)
}

// TestProvisionWorktreeDefinitionChangeReadopts: changing a profile's
// provisioning inputs (sparse) re-provisions the pooled worktree on its next
// claim; a description-only change does not.
func TestProvisionWorktreeDefinitionChangeReadopts(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	v1 := customProfile(t, "build", `{"description":"v1"}`)

	first, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/a", v1, "")
	if err != nil || !first.Provisioned {
		t.Fatalf("fresh provision = %+v, %v", first, err)
	}
	if _, err := releaseWorktree(ctx, runner, first.Path, "", lookupOf(v1)); err != nil {
		t.Fatal(err)
	}
	renamed := customProfile(t, "build", `{"description":"v1 reworded"}`)
	second, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/b", renamed, "")
	if err != nil || !second.Reused || second.Provisioned {
		t.Fatalf("description-only change must reuse without re-provisioning: %+v, %v", second, err)
	}
	if _, err := releaseWorktree(ctx, runner, second.Path, "", lookupOf(renamed)); err != nil {
		t.Fatal(err)
	}
	v2 := customProfile(t, "build", `{"description":"v2","sparse":{"mode":"patterns","paths":["/*","!/deps/"]}}`)
	third, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/c", v2, "")
	if err != nil {
		t.Fatalf("provision v2: %v", err)
	}
	if !third.Reused || third.Path != first.Path || !third.Provisioned {
		t.Fatalf("sparse change must re-adopt the pooled worktree: %+v", third)
	}
	assertExists(t, third.Path, "deps/z.txt", false)
	assertExists(t, third.Path, "src/y.go", true)
}

// TestProvisionWorktreePatternsProfileExcludesDir: a patterns-mode profile
// leaves an excluded directory unchecked-out while the rest (including the
// ticket board) materializes.
func TestProvisionWorktreePatternsProfileExcludesDir(t *testing.T) {
	root := profileWorktreeFixture(t)
	build := customProfile(t, "build", `{"sparse":{"mode":"patterns","paths":["/*","!/deps/"]}}`)
	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, "develop", "impl/develop/a", build, "")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	assertExists(t, res.Path, "deps/z.txt", false)
	assertExists(t, res.Path, "deps", false)
	assertExists(t, res.Path, "src/y.go", true)
	assertExists(t, res.Path, "ai-docs/tickets/idea/a.md", true)
	if cone := strings.TrimSpace(string(runGitOutput(t, res.Path, "config", "--get", "core.sparseCheckoutCone"))); cone != "false" {
		t.Fatalf("patterns profile must use non-cone mode, core.sparseCheckoutCone = %q", cone)
	}
	if markerProfile(t, res.Path) != "build" {
		t.Fatal("marker must name the profile")
	}
}

// TestProvisionWorktreeAdoptsLegacyIntoBuiltinsOnly: an unmarked sparse
// worktree is adopted as housekeeping (its arbitrary prior cone normalized), an
// unmarked full worktree as full, and a custom profile never adopts either.
func TestProvisionWorktreeAdoptsLegacyIntoBuiltinsOnly(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	housekeeping := builtinProfile(t, worktreeProfileHousekeeping)
	full := builtinProfile(t, worktreeProfileFull)
	lookup := builtinProfileLookup(t)

	sparse, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/a", housekeeping, "")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/b", full, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{sparse.Path, plain.Path} {
		if _, err := releaseWorktree(ctx, runner, p, "", lookup); err != nil {
			t.Fatal(err)
		}
		dropWorktreeMarker(t, p)
	}
	// An arbitrary prior cone, as the retired ticket-scope flow could leave.
	runGit(t, sparse.Path, "sparse-checkout", "set", "--cone", "src")

	custom, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/c", customProfile(t, "build", `{}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if custom.Reused || custom.Path == sparse.Path || custom.Path == plain.Path {
		t.Fatalf("a custom profile must never adopt an unmarked worktree: %+v", custom)
	}

	adopted, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/d", housekeeping, "")
	if err != nil {
		t.Fatal(err)
	}
	if adopted.Path != sparse.Path || !adopted.Provisioned {
		t.Fatalf("the unmarked sparse worktree must be adopted and re-provisioned as housekeeping: %+v", adopted)
	}
	if list := strings.TrimSpace(string(runGitOutput(t, adopted.Path, "sparse-checkout", "list"))); list != "ai-docs" {
		t.Fatalf("adoption must normalize the cone to ai-docs, got %q", list)
	}
	assertExists(t, adopted.Path, "src/y.go", false)
	if markerProfile(t, adopted.Path) != worktreeProfileHousekeeping {
		t.Fatal("adoption must stamp the housekeeping marker")
	}

	adoptedFull, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/e", full, "")
	if err != nil {
		t.Fatal(err)
	}
	if adoptedFull.Path != plain.Path || markerProfile(t, plain.Path) != worktreeProfileFull {
		t.Fatalf("the unmarked full worktree must be adopted as full: %+v", adoptedFull)
	}
}

// TestProvisionWorktreeHiddenTicketBoardFails: a profile whose sparse hides the
// ticket board fails acquire naming the profile, and the worktree goes back to
// the pool detached so the target branch is free.
func TestProvisionWorktreeHiddenTicketBoardFails(t *testing.T) {
	root := profileWorktreeFixture(t)
	narrow := customProfile(t, "narrow", `{"sparse":{"mode":"cone","paths":["src"]}}`)
	_, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, "develop", "impl/develop/a", narrow, "")
	if err == nil {
		t.Fatal("a profile hiding the ticket board must fail acquire")
	}
	for _, want := range []string{`profile "narrow"`, "ai-docs/tickets"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q must contain %q", err, want)
		}
	}
	// The branch is free again: the primary worktree can check it out.
	runGit(t, root, "switch", "impl/develop/a")
}

// TestProvisionWorktreeHookNarrowedTicketBoardFails: the ticket-board check
// runs after post_acquire, so a hook that narrows sparse away from the board
// fails acquire the same way a declared sparse does.
func TestProvisionWorktreeHookNarrowedTicketBoardFails(t *testing.T) {
	root := profileWorktreeFixture(t)
	narrowing := hookProfile(t, "narrowing", nil, map[string][]map[string]any{
		"post_acquire": {hook(hookArgv(t, "sparse-set", "src"))},
	})
	_, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, "develop", "impl/develop/a", narrowing, "")
	if err == nil {
		t.Fatal("a post_acquire hook hiding the ticket board must fail acquire")
	}
	for _, want := range []string{`profile "narrowing"`, "ai-docs/tickets"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q must contain %q", err, want)
		}
	}
	runGit(t, root, "switch", "impl/develop/a")
}

// TestReleaseWorktreeLegacyAndVanishedProfile: releasing an unmarked leased
// worktree runs the pre-profile hygiene only (nothing preserved, reported as
// legacy), and a marker naming a profile that no longer resolves gets the same
// hygiene with a warning.
func TestReleaseWorktreeLegacyAndVanishedProfile(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	build := customProfile(t, "build", `{"preserve":["out/"]}`)

	legacy, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/a", build, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeWorktreeLease(ctx, runner, legacy.Path, worktreeLease{WorkerKey: "k", ParentKey: "p", AcquiredAt: "2026-10-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	dropWorktreeMarker(t, legacy.Path)
	writeFileForTest(t, legacy.Path, "out/keep.bin", "artifact")
	res, err := releaseWorktree(ctx, runner, legacy.Path, "", lookupOf(build))
	if err != nil {
		t.Fatalf("release legacy: %v", err)
	}
	if res.Profile != worktreeLegacyLabel || len(res.Warnings) != 0 {
		t.Fatalf("legacy release result = %+v", res)
	}
	assertExists(t, legacy.Path, "out/keep.bin", false)
	if lease, _ := readWorktreeLease(ctx, runner, legacy.Path); lease != nil {
		t.Fatalf("legacy release must drop the lease: %+v", lease)
	}

	vanished, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/b", build, "")
	if err != nil {
		t.Fatal(err)
	}
	writeFileForTest(t, vanished.Path, "out/keep.bin", "artifact")
	res, err = releaseWorktree(ctx, runner, vanished.Path, "", builtinProfileLookup(t))
	if err != nil {
		t.Fatalf("release with vanished profile: %v", err)
	}
	if res.Profile != "build" || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "no longer resolves") {
		t.Fatalf("vanished-profile release result = %+v", res)
	}
	assertExists(t, vanished.Path, "out/keep.bin", false)
}
