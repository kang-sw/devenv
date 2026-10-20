package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// TestWorktreeHookHelperProcess is not a test of its own: hook tests run this
// test binary as the hook command (portable where no POSIX shell exists).
// It acts only when WS_TEST_HOOK_HELPER=1.
func TestWorktreeHookHelperProcess(t *testing.T) {
	if os.Getenv("WS_TEST_HOOK_HELPER") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	cwd, _ := os.Getwd()
	switch args[0] {
	case "env":
		body := strings.Join([]string{os.Getenv("WS_WORKTREE_DIR"), os.Getenv("WS_BASE"), os.Getenv("WS_TARGET_BRANCH"), cwd}, "\n")
		p := filepath.Join(os.Getenv("WS_WORKTREE_DIR"), filepath.FromSlash(args[1]))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			os.Exit(89)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			os.Exit(90)
		}
	case "exit":
		n, _ := strconv.Atoi(args[1])
		os.Stderr.WriteString("hook says no\n")
		os.Exit(n)
	case "sleep":
		time.Sleep(30 * time.Second)
	case "write":
		p := filepath.Join(cwd, filepath.FromSlash(args[1]))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			os.Exit(91)
		}
		if err := os.WriteFile(p, []byte("generated"), 0o644); err != nil {
			os.Exit(92)
		}
	case "sparse-set":
		if out, err := exec.Command("git", "sparse-checkout", "set", args[1]).CombinedOutput(); err != nil {
			os.Stderr.Write(out)
			os.Exit(94)
		}
	case "sparse-add":
		if out, err := exec.Command("git", "sparse-checkout", "add", args[1]).CombinedOutput(); err != nil {
			os.Stderr.Write(out)
			os.Exit(93)
		}
	}
	os.Exit(0)
}

// hookArgv is a hook argv running the helper process in mode.
func hookArgv(t *testing.T, mode string, args ...string) []string {
	t.Helper()
	t.Setenv("WS_TEST_HOOK_HELPER", "1")
	return append([]string{os.Args[0], "-test.run=^TestWorktreeHookHelperProcess$", "--", mode}, args...)
}

// hookProfile builds a profile whose definition carries the given hooks
// (point -> list of {argv, cwd}) on top of base fields.
func hookProfile(t *testing.T, name string, base map[string]any, hooks map[string][]map[string]any) resolvedWorktreeProfile {
	t.Helper()
	def := map[string]any{}
	for k, v := range base {
		def[k] = v
	}
	if len(hooks) > 0 {
		def["hooks"] = hooks
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	return customProfile(t, name, string(raw))
}

func hook(argv []string) map[string]any { return map[string]any{"argv": argv} }

func hookIn(argv []string, cwd string) map[string]any {
	return map[string]any{"argv": argv, "cwd": cwd}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Split(string(data), "\n")
}

func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// TestWorktreeHooksEnvCwdAndProvenance: hooks run in the worktree or their
// relative cwd, receive WS_WORKTREE_DIR / WS_BASE / WS_TARGET_BRANCH, run
// post_provision only on entry and post_acquire on every acquire, and the
// acquire and release results report each executed hook's argv, scope, and
// exit code.
func TestWorktreeHooksEnvCwdAndProvenance(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	profile := hookProfile(t, "build", map[string]any{"preserve": []string{"out/"}}, map[string][]map[string]any{
		worktreeHookPostProvision: {hook(hookArgv(t, "env", "out/provision.txt"))},
		worktreeHookPostAcquire:   {hookIn(hookArgv(t, "env", "out/acquire.txt"), "src")},
		worktreeHookPreRelease:    {hook(hookArgv(t, "env", "out/release.txt"))},
	})

	first, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/a", profile, "")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	prov := readLines(t, filepath.Join(first.Path, "out", "provision.txt"))
	if prov[0] != first.Path || prov[1] != "develop" || prov[2] != "impl/develop/a" || !sameDir(t, prov[3], first.Path) {
		t.Fatalf("post_provision env/cwd = %q (worktree %s)", prov, first.Path)
	}
	acq := readLines(t, filepath.Join(first.Path, "out", "acquire.txt"))
	if !sameDir(t, acq[3], filepath.Join(first.Path, "src")) {
		t.Fatalf("post_acquire must run in its relative cwd, ran in %q", acq[3])
	}
	if len(first.Hooks) != 2 || first.Hooks[0].Point != worktreeHookPostProvision || first.Hooks[1].Point != worktreeHookPostAcquire {
		t.Fatalf("acquire hook provenance = %+v", first.Hooks)
	}
	for _, h := range first.Hooks {
		if h.Scope != "repo" || h.ExitCode != 0 || len(h.Argv) == 0 || h.Argv[0] != os.Args[0] {
			t.Fatalf("hook provenance entry = %+v", h)
		}
	}
	if first.Hooks[1].Cwd != "src" {
		t.Fatalf("post_acquire provenance must carry its cwd: %+v", first.Hooks[1])
	}
	if !strings.Contains(first.text(), "hook: post_acquire [repo] exit 0: ") {
		t.Fatalf("acquire text must list hooks:\n%s", first.text())
	}

	rel, err := releaseWorktree(ctx, runner, first.Path, "", lookupOf(profile))
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if len(rel.Hooks) != 1 || rel.Hooks[0].Point != worktreeHookPreRelease || rel.Hooks[0].ExitCode != 0 {
		t.Fatalf("release hook provenance = %+v", rel.Hooks)
	}
	relEnv := readLines(t, filepath.Join(first.Path, "out", "release.txt"))
	if relEnv[1] != "" || relEnv[2] != "impl/develop/a" {
		t.Fatalf("pre_release env = %q, want empty base and the checked-out branch", relEnv)
	}

	second, err := provisionWorktree(ctx, runner, root, "develop", "", profile, "")
	if err != nil {
		t.Fatalf("re-provision: %v", err)
	}
	if !second.Reused || len(second.Hooks) != 1 || second.Hooks[0].Point != worktreeHookPostAcquire {
		t.Fatalf("reuse must run post_acquire only: %+v", second)
	}
	acq = readLines(t, filepath.Join(second.Path, "out", "acquire.txt"))
	if acq[1] != "develop" || acq[2] != "develop" {
		t.Fatalf("with target_branch omitted WS_TARGET_BRANCH is base: %q", acq)
	}
}

// TestWorktreeHookSparseWideningSurvivesReuse: a post_provision hook that
// widens the declared sparse keeps that widening on plain reuse, because
// declared sparse is re-applied only when the worktree (re-)enters the profile.
func TestWorktreeHookSparseWideningSurvivesReuse(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	profile := hookProfile(t, "docs", map[string]any{"sparse": map[string]any{"mode": "cone", "paths": []string{"ai-docs"}}}, map[string][]map[string]any{
		worktreeHookPostProvision: {hook(hookArgv(t, "sparse-add", "src"))},
	})
	first, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/a", profile, "")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	assertExists(t, first.Path, "src/y.go", true)
	assertExists(t, first.Path, "deps/z.txt", false)
	if _, err := releaseWorktree(ctx, runner, first.Path, "", lookupOf(profile)); err != nil {
		t.Fatal(err)
	}
	second, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/b", profile, "")
	if err != nil || !second.Reused || second.Provisioned {
		t.Fatalf("plain reuse = %+v, %v", second, err)
	}
	assertExists(t, second.Path, "src/y.go", true)
}

// TestWorktreeHookPostProvisionPreserveWarning: an ignored path a
// post_provision hook created and preserve does not cover is warned about;
// a preserved one is not.
func TestWorktreeHookPostProvisionPreserveWarning(t *testing.T) {
	root := profileWorktreeFixture(t)
	profile := hookProfile(t, "build", map[string]any{"preserve": []string{"out/"}}, map[string][]map[string]any{
		worktreeHookPostProvision: {hook(hookArgv(t, "write", "cache/gen.txt")), hook(hookArgv(t, "write", "out/keep.txt"))},
	})
	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, "develop", "impl/develop/a", profile, "")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	joined := strings.Join(res.Warnings, "\n")
	if !strings.Contains(joined, "cache/") || strings.Contains(joined, "out/") {
		t.Fatalf("warnings = %q, want cache/ named and out/ not", res.Warnings)
	}
}

// TestWorktreeHookPostAcquireFailureQuarantines: a failing post_acquire
// quarantines the worktree, frees the target branch, is never reused, and the
// error counts the profile's quarantined worktrees.
func TestWorktreeHookPostAcquireFailureQuarantines(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	broken := hookProfile(t, "build", nil, map[string][]map[string]any{
		worktreeHookPostAcquire: {hook(hookArgv(t, "exit", "3"))},
	})
	_, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/a", broken, "")
	if err == nil {
		t.Fatal("a failing post_acquire must fail acquire")
	}
	for _, want := range []string{"post_acquire", "exit status 3", "hook says no", "quarantined", "1 worktree(s) of profile \"build\""} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error must contain %q: %v", want, err)
		}
	}
	// The branch is free: the primary worktree can check it out.
	runGit(t, root, "switch", "impl/develop/a")
	runGit(t, root, "switch", "impl/develop/foo")

	_, err = provisionWorktree(ctx, runner, root, "develop", "impl/develop/b", broken, "")
	if err == nil || !strings.Contains(err.Error(), "2 worktree(s) of profile \"build\"") || !strings.Contains(err.Error(), "defect in the profile") {
		t.Fatalf("second failure must count two quarantined worktrees: %v", err)
	}

	fixed := hookProfile(t, "build", nil, nil)
	res, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/c", fixed, "")
	if err != nil {
		t.Fatalf("provision with fixed profile: %v", err)
	}
	if res.Reused {
		t.Fatalf("a quarantined worktree must never be reused: %+v", res)
	}
	m, err := readWorktreeMarker(ctx, runner, res.Path)
	if err != nil || m == nil || m.Quarantine != nil {
		t.Fatalf("the fresh worktree must not be quarantined: %+v, %v", m, err)
	}
}

// TestWorktreeHookPostProvisionFailureQuarantines covers post_provision
// failure on a fresh create (through the dispatcher, which must write no lease)
// and on a re-adoption after a definition change.
func TestWorktreeHookPostProvisionFailureQuarantines(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}

	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	s := NewServer(root, "test")
	leadKey, err := s.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	failing := map[string]any{"hooks": map[string]any{worktreeHookPostProvision: []any{hook(hookArgv(t, "exit", "4"))}}}
	if resp := callToolOnce(t, s, 1, "config.tune", map[string]any{"session_key": leadKey, "key": "worktree.profile.build", "scope": "project", "value": failing}); toolIsError(t, resp) {
		t.Fatalf("config.tune: %s", resp)
	}
	resp := callToolOnce(t, s, 2, "worktree.acquire", map[string]any{"session_key": leadKey, "base": "develop", "target_branch": "impl/develop/a", "profile": "build"})
	if !toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "post_provision") || !strings.Contains(toolText(t, resp), "quarantined") {
		t.Fatalf("acquire must fail with a quarantine: %s", resp)
	}
	list := listForTest(t, s, 3, leadKey)
	if len(list.Worktrees) != 1 {
		t.Fatalf("want one pooled worktree, got %+v", list.Worktrees)
	}
	q := list.Worktrees[0]
	if q.WorktreeLease != nil || q.Quarantine == nil || q.Quarantine.Point != worktreeHookPostProvision || q.Quarantine.ExitCode != 4 || q.Release != "" {
		t.Fatalf("quarantined entry = %+v (quarantine %+v)", q, q.Quarantine)
	}
	if text := toolText(t, callToolOnce(t, s, 4, "worktree.list", map[string]any{"session_key": leadKey})); !strings.Contains(text, "quarantined: post_provision hook") {
		t.Fatalf("worktree.list text must show the quarantine:\n%s", text)
	}

	// Re-adoption: a healthy worktree whose profile definition changes to a
	// failing post_provision is quarantined on its next claim.
	healthy := hookProfile(t, "lint", nil, nil)
	first, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/b", healthy, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := releaseWorktree(ctx, runner, first.Path, "", lookupOf(healthy)); err != nil {
		t.Fatal(err)
	}
	changed := hookProfile(t, "lint", nil, map[string][]map[string]any{
		worktreeHookPostProvision: {hook(hookArgv(t, "exit", "5"))},
	})
	if _, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/c", changed, ""); err == nil || !strings.Contains(err.Error(), "quarantined") {
		t.Fatalf("re-adoption with a failing post_provision must quarantine: %v", err)
	}
	m, err := readWorktreeMarker(ctx, runner, first.Path)
	if err != nil || m == nil || m.Quarantine == nil || m.Quarantine.ExitCode != 5 {
		t.Fatalf("re-adopted worktree marker = %+v, %v", m, err)
	}
	if lease, _ := readWorktreeLease(ctx, runner, first.Path); lease != nil {
		t.Fatalf("a quarantined worktree holds no lease: %+v", lease)
	}
	if b := wtBranch(t, first.Path); b != "HEAD" {
		t.Fatalf("quarantine must detach, branch = %q", b)
	}
}

// TestWorktreeHookPreReleaseFailureLeavesWorktree: a failing pre_release fails
// release and leaves the worktree leased, on its branch, with its changes.
func TestWorktreeHookPreReleaseFailureLeavesWorktree(t *testing.T) {
	root := profileWorktreeFixture(t)
	ctx := context.Background()
	runner := wsgit.ExecRunner{}
	profile := hookProfile(t, "build", nil, map[string][]map[string]any{
		worktreeHookPreRelease: {hook(hookArgv(t, "exit", "6"))},
	})
	res, err := provisionWorktree(ctx, runner, root, "develop", "impl/develop/a", profile, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeWorktreeLease(ctx, runner, res.Path, worktreeLease{WorkerKey: "k", ParentKey: "p", AcquiredAt: "2026-10-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	writeFileForTest(t, res.Path, "src/wip.go", "package y\n")
	_, err = releaseWorktree(ctx, runner, res.Path, "", lookupOf(profile))
	if err == nil || !strings.Contains(err.Error(), "pre_release") || !strings.Contains(err.Error(), "exit status 6") {
		t.Fatalf("release must fail on pre_release: %v", err)
	}
	assertExists(t, res.Path, "src/wip.go", true)
	if lease, _ := readWorktreeLease(ctx, runner, res.Path); lease == nil {
		t.Fatal("a failed pre_release must leave the lease")
	}
	if b := wtBranch(t, res.Path); b != "impl/develop/a" {
		t.Fatalf("a failed pre_release must leave the branch checked out, got %q", b)
	}
}

// TestWorktreeHookTimeoutFails: a hook still running at the timeout is killed
// and fails its operation with exit code -1.
func TestWorktreeHookTimeoutFails(t *testing.T) {
	old := worktreeHookTimeout
	worktreeHookTimeout = 300 * time.Millisecond
	t.Cleanup(func() { worktreeHookTimeout = old })
	dir := t.TempDir()
	start := time.Now()
	runs, failure := runWorktreeHooks(context.Background(), worktreeHookPreRelease, []worktreeProfileHook{{Argv: hookArgv(t, "sleep")}}, "repo", worktreeHookEnv{WorktreeDir: dir})
	if failure == nil || !strings.Contains(failure.Error(), "timed out") {
		t.Fatalf("timeout must fail: %v", failure)
	}
	if len(runs) != 1 || runs[0].ExitCode != -1 {
		t.Fatalf("timed-out run = %+v", runs)
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("timeout did not bound the hook: %s", elapsed)
	}
}
