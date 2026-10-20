package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
)

const testStaleAfter = 7 * 24 * time.Hour

func worktreeRegistered(t *testing.T, root, path string) bool {
	t.Helper()
	entries, err := listWorktrees(context.Background(), wsgit.ExecRunner{}, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Path == path {
			return true
		}
	}
	return false
}

// TestRemoveWorktreeRefusals: dirty trees, a branch holding commits no other
// branch reaches, a detached HEAD holding commits no ref reaches, the primary
// worktree, a path outside the pool, and a lease active within the stale
// threshold are refused before any pre_remove hook runs; a stale lease is
// removable.
func TestRemoveWorktreeRefusals(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	now := time.Now()
	profile := hookProfile(t, "build", nil, map[string][]map[string]any{
		worktreeHookPreRemove: {hook(hookArgv(t, "env", "out/pre_remove.txt"))},
	})
	lookup := lookupOf(profile)
	refuse := func(path string, at time.Time, want string) {
		t.Helper()
		_, err := removeWorktree(ctx, runner, path, "", testStaleAfter, at, lookup)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("remove(%s) = %v, want refusal containing %q", path, err, want)
		}
		if _, statErr := os.Stat(filepath.Join(path, "out", "pre_remove.txt")); statErr == nil {
			t.Fatalf("a refused remove ran pre_remove in %s", path)
		}
	}

	dirty, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/dirty", profile, "")
	if err != nil {
		t.Fatal(err)
	}
	writeFileForTest(t, dirty.Path, "src/wip.go", "package y\n")
	refuse(dirty.Path, now, "uncommitted changes or untracked files")

	unique, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/unique", profile, "")
	if err != nil {
		t.Fatal(err)
	}
	writeFileForTest(t, unique.Path, "src/new.go", "package y\n")
	runGit(t, unique.Path, "add", "src/new.go")
	runGit(t, unique.Path, "commit", "-m", "unmerged")
	refuse(unique.Path, now, "holds 1 commit(s) no other branch reaches")

	detached, err := provisionWorktree(ctx, runner, root, "develop", "", profile, "")
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, detached.Path, "checkout", "--detach")
	writeFileForTest(t, detached.Path, "src/loose.go", "package y\n")
	runGit(t, detached.Path, "add", "src/loose.go")
	runGit(t, detached.Path, "commit", "-m", "loose")
	refuse(detached.Path, now, "detached HEAD holds 1 commit(s) no ref reaches")

	refuse(root, now, "primary worktree")
	outside := filepath.Join(t.TempDir(), "outside")
	runGit(t, root, "worktree", "add", "--detach", outside, "develop")
	refuse(canonicalRootForTest(t, outside), now, "not under the owned worktree pool")

	leased, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/leased", profile, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeWorktreeLease(ctx, runner, leased.Path, worktreeLease{WorkerKey: "k", ParentKey: "p", AcquiredAt: now.UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	refuse(leased.Path, now, "leased to worker k")

	// Thirty days later the same lease is stale, and remove proceeds.
	res, err := removeWorktree(ctx, runner, leased.Path, "", testStaleAfter, now.Add(30*24*time.Hour), lookup)
	if err != nil {
		t.Fatalf("remove of a stale-leased worktree: %v", err)
	}
	if !res.Removed || res.Branch != "impl/develop/leased" || worktreeRegistered(t, root, leased.Path) {
		t.Fatalf("stale-leased remove result = %+v", res)
	}
	if len(res.Hooks) != 1 || res.Hooks[0].Point != worktreeHookPreRemove || res.Hooks[0].Scope != "repo" || res.Hooks[0].ExitCode != 0 || res.Hooks[0].Argv[0] != os.Args[0] {
		t.Fatalf("remove hook provenance = %+v", res.Hooks)
	}
	if _, err := os.Stat(leased.Path); !os.IsNotExist(err) {
		t.Fatalf("removed worktree directory still exists: %v", err)
	}
	// The branch is kept.
	runGit(t, root, "rev-parse", "--verify", "refs/heads/impl/develop/leased")
}

// TestRemoveWorktreeFailingPreRemoveKeepsWorktree: a failing pre_remove fails
// remove and leaves the worktree registered.
func TestRemoveWorktreeFailingPreRemoveKeepsWorktree(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	profile := hookProfile(t, "build", nil, map[string][]map[string]any{
		worktreeHookPreRemove: {hook(hookArgv(t, "exit", "7"))},
	})
	res, err := provisionWorktree(ctx, runner, root, "develop", "", profile, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := releaseWorktree(ctx, runner, res.Path, "", lookupOf(profile)); err != nil {
		t.Fatal(err)
	}
	_, err = removeWorktree(ctx, runner, res.Path, "", testStaleAfter, time.Now(), lookupOf(profile))
	if err == nil || !strings.Contains(err.Error(), "pre_remove") || !strings.Contains(err.Error(), "was not removed") {
		t.Fatalf("remove must fail on pre_remove: %v", err)
	}
	if !worktreeRegistered(t, root, res.Path) {
		t.Fatal("a failed pre_remove must leave the worktree in place")
	}
}

// TestRemoveWorktreeQuarantinedAndVanishedProfile: a quarantined worktree is
// removable (clearing the quarantine), and a worktree whose profile no longer
// resolves is removed with pre_remove skipped and a note saying so.
func TestRemoveWorktreeQuarantinedAndVanishedProfile(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	broken := hookProfile(t, "build", nil, map[string][]map[string]any{
		worktreeHookPostAcquire: {hook(hookArgv(t, "exit", "3"))},
		worktreeHookPreRemove:   {hook(hookArgv(t, "env", "out/pre_remove.txt"))},
	})
	if _, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/a", broken, ""); err == nil {
		t.Fatal("setup: post_acquire must fail")
	}
	quarantined := ""
	entries, _ := listWorktrees(ctx, runner, root)
	for _, e := range entries[1:] {
		if m, _ := readWorktreeMarker(ctx, runner, e.Path); m != nil && m.Quarantine != nil {
			quarantined = e.Path
		}
	}
	if quarantined == "" {
		t.Fatal("setup: no quarantined worktree")
	}
	res, err := removeWorktree(ctx, runner, quarantined, "", testStaleAfter, time.Now(), lookupOf(broken))
	if err != nil {
		t.Fatalf("remove quarantined: %v", err)
	}
	if !res.Removed || worktreeRegistered(t, root, quarantined) || !strings.Contains(strings.Join(res.Notes, "\n"), "cleared its quarantine") {
		t.Fatalf("quarantined remove result = %+v", res)
	}

	orphan, err := provisionWorktree(ctx, runner, root, "develop", "", customProfile(t, "gone", `{}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := releaseWorktree(ctx, runner, orphan.Path, "", lookupOf(customProfile(t, "gone", `{}`))); err != nil {
		t.Fatal(err)
	}
	res, err = removeWorktree(ctx, runner, orphan.Path, "", testStaleAfter, time.Now(), builtinProfileLookup(t))
	if err != nil {
		t.Fatalf("remove with vanished profile: %v", err)
	}
	if !res.Removed || len(res.Hooks) != 0 || !strings.Contains(strings.Join(res.Notes, "\n"), `profile "gone" no longer resolves`) {
		t.Fatalf("vanished-profile remove result = %+v", res)
	}
}

// TestWorktreeRemoveDispatch drives worktree.remove through the server: lead
// only, by path or by worker key, with the threshold read from config.
func TestWorktreeRemoveDispatch(t *testing.T) {
	s, root, base, leadKey := worktreeListServer(t)
	acq := acquireForTest(t, s, 1, leadKey, base, "impl/test/alpha")
	if rel := callToolOnce(t, s, 2, "worktree.release", map[string]any{"session_key": leadKey, "path": acq.Path}); toolIsError(t, rel) {
		t.Fatalf("release: %s", rel)
	}
	delegateKey, err := s.sessions.mint(canonicalRootForTest(t, root), roleDelegate, leadKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := callToolOnce(t, s, 3, "worktree.remove", map[string]any{"session_key": delegateKey, "path": acq.Path}); !jsonrpcHasError(t, got) {
		t.Fatalf("non-lead remove not rejected: %s", got)
	}
	if got := callToolOnce(t, s, 4, "worktree.remove", map[string]any{"session_key": leadKey}); !toolIsError(t, got) {
		t.Fatalf("remove without path/key not rejected: %s", got)
	}
	resp := callToolOnce(t, s, 5, "worktree.remove", map[string]any{"session_key": leadKey, "key": acq.WorkerKey, "format": "json"})
	var res worktreeRemoveResult
	if err := json.Unmarshal([]byte(toolText(t, resp)), &res); err != nil {
		t.Fatalf("unmarshal remove: %v\n%s", err, resp)
	}
	if !res.Removed || res.Profile != worktreeProfileFull {
		t.Fatalf("remove result = %+v", res)
	}
	if list := listForTest(t, s, 6, leadKey); len(list.Worktrees) != 0 {
		t.Fatalf("removed worktree still listed: %+v", list.Worktrees)
	}
}

// TestReleaseRecordsPreservedBytes: release measures the preserved ignored
// artifacts into the marker.
func TestReleaseRecordsPreservedBytes(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	build := customProfile(t, "build", `{"preserve":["out/"]}`)
	res, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/a", build, "")
	if err != nil {
		t.Fatal(err)
	}
	writeFileForTest(t, res.Path, "out/a.bin", strings.Repeat("x", 1000))
	writeFileForTest(t, res.Path, "out/deep/b.bin", strings.Repeat("y", 500))
	writeFileForTest(t, res.Path, "cache/c.bin", strings.Repeat("z", 9000))
	if _, err := releaseWorktree(ctx, runner, res.Path, "", lookupOf(build)); err != nil {
		t.Fatal(err)
	}
	m, err := readWorktreeMarker(ctx, runner, res.Path)
	if err != nil || m == nil || m.PreservedBytes != 1500 || m.PreservedMeasuredAt == "" {
		t.Fatalf("marker after release = %+v, %v; want 1500 preserved bytes", m, err)
	}
}

// TestPoolNudgeThrottleAndThresholds: the nudge lists a stale branch-holding
// worktree and a detached one over the preserved-size limit, honors both
// thresholds, and runs at most once a day per pool.
func TestPoolNudgeThrottleAndThresholds(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	build := customProfile(t, "build", `{"preserve":["out/"]}`)

	held, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/held", build, "")
	if err != nil {
		t.Fatal(err)
	}
	big, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/big", build, "")
	if err != nil {
		t.Fatal(err)
	}
	writeFileForTest(t, big.Path, "out/a.bin", strings.Repeat("x", 2000))
	if _, err := releaseWorktree(ctx, runner, big.Path, "", lookupOf(build)); err != nil {
		t.Fatal(err)
	}
	pool := held.Pool
	now := time.Now()

	// Fresh activity and a limit above the measured size: nothing to report,
	// and the check still consumes the day's run.
	lines, err := poolNudge(ctx, runner, root, pool, testStaleAfter, 5000, now)
	if err != nil || len(lines) != 0 {
		t.Fatalf("nudge under thresholds = %q, %v", lines, err)
	}
	if lines, _ := poolNudge(ctx, runner, root, pool, testStaleAfter, 1, now.Add(30*24*time.Hour)); len(lines) != 2 {
		// The throttle keys on the recorded run time, so a later clock is
		// already past the day; this call runs and must see both.
		t.Fatalf("nudge past thresholds = %q, want two lines", lines)
	}
	later := now.Add(30*24*time.Hour + time.Hour)
	if lines, _ := poolNudge(ctx, runner, root, pool, testStaleAfter, 1, later); lines != nil {
		t.Fatalf("a second nudge within a day must be throttled, got %q", lines)
	}
	lines, err = poolNudge(ctx, runner, root, pool, testStaleAfter, 1, later.Add(25*time.Hour))
	if err != nil || len(lines) != 2 {
		t.Fatalf("nudge the next day = %q, %v", lines, err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, held.Path+": branch impl/develop/held, no activity for") || !strings.Contains(joined, big.Path+": detached") {
		t.Fatalf("nudge lines = %q", lines)
	}
}

// TestWorktreeNudgeKnobsAndAcquireWiring: the nudge knobs validate as
// integers of at least 1 and feed the thresholds, and worktree.acquire
// carries the nudge on the first acquire of the day only.
func TestWorktreeNudgeKnobsAndAcquireWiring(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	s, root, base, leadKey := worktreeListServer(t)
	for i, bad := range []string{"0", "abc"} {
		if resp := callToolOnce(t, s, 10+i, "config.tune", map[string]any{"session_key": leadKey, "key": wsconfig.ItemWorktreeNudgeStaleDays, "value": bad}); !toolIsError(t, resp) {
			t.Fatalf("config.tune accepted stale days %q: %s", bad, resp)
		}
	}
	if resp := callToolOnce(t, s, 12, "config.tune", map[string]any{"session_key": leadKey, "key": wsconfig.ItemWorktreeNudgeStaleDays, "value": "3"}); toolIsError(t, resp) {
		t.Fatalf("config.tune stale days 3: %s", resp)
	}
	resolver := s.sessionResolver(leadKey, builtinConfigDefaults())
	if stale, limit := worktreeNudgeThresholds(&resolver, leadKey); stale != 3*24*time.Hour || limit != bytesPerGB {
		t.Fatalf("thresholds = %v, %d; want 72h and the 1 GB default", stale, limit)
	}

	// A branch worktree whose HEAD commit is a week old is stale under 3 days.
	old, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/old", builtinProfile(t, worktreeProfileFull), "")
	if err != nil {
		t.Fatal(err)
	}
	weekAgo := time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	commit := exec.Command("git", "commit", "--allow-empty", "-m", "old work")
	commit.Dir = old.Path
	commit.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+weekAgo, "GIT_COMMITTER_DATE="+weekAgo)
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("old commit: %v\n%s", err, out)
	}

	first := acquireForTest(t, s, 13, leadKey, base, "impl/test/alpha")
	if len(first.Nudge) != 1 || !strings.Contains(first.Nudge[0], old.Path+": branch impl/test/old, no activity for 7 days") {
		t.Fatalf("first acquire nudge = %q", first.Nudge)
	}
	if second := acquireForTest(t, s, 14, leadKey, base, "impl/test/beta"); len(second.Nudge) != 0 {
		t.Fatalf("second acquire the same day must not nudge, got %q", second.Nudge)
	}
}

// TestRemoveWorktreeWithInitializedSubmodule: git refuses to remove a
// worktree holding an initialized submodule without --force; remove's own
// refusals stand in for git's, so a clean, merged one is removed.
func TestRemoveWorktreeWithInitializedSubmodule(t *testing.T) {
	root, base := submoduleWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	res, err := provisionWorktree(ctx, runner, root, base, "impl/test/sub", builtinProfile(t, worktreeProfileFull), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(res.Path, "deps", "sub", "s.txt")); err != nil {
		t.Fatalf("setup: submodule not initialized: %v", err)
	}
	if _, err := releaseWorktree(ctx, runner, res.Path, "", builtinProfileLookup(t)); err != nil {
		t.Fatal(err)
	}
	out, err := remove(ctx, runner, res.Path, builtinProfileLookup(t))
	if err != nil || !out.Removed || worktreeRegistered(t, root, res.Path) {
		t.Fatalf("remove with an initialized submodule = %+v, %v", out, err)
	}
}

func remove(ctx context.Context, runner wsgit.Runner, path string, lookup worktreeProfileLookup) (worktreeRemoveResult, error) {
	return removeWorktree(ctx, runner, path, "", testStaleAfter, time.Now(), lookup)
}
