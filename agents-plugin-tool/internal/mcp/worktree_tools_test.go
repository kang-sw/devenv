package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// worktreeFixture builds a git repo with one base commit and returns the repo
// root and the base commit SHA. The base commit carries a .gitignore ignoring
// *.log so tests can exercise the ignored-cruft purge of hygiene/release.
func worktreeFixture(t *testing.T) (root, base string) {
	t.Helper()
	// Canonicalize so root matches the main-worktree path the production code
	// derives from git: on some hosts (Windows CI) git's toplevel resolves
	// symlinks/short-names that a raw t.TempDir path does not, which would make
	// a legitimate under-pool path compare as outside the pool.
	root = canonicalRootForTest(t, initGitRepo(t))
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	base = strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	return root, base
}

func wtBranch(t *testing.T, path string) string {
	t.Helper()
	return strings.TrimSpace(string(runGitOutput(t, path, "rev-parse", "--abbrev-ref", "HEAD")))
}

// jsonrpcHasError reports whether the response line carries a JSON-RPC protocol
// error object (as the capability gate returns), distinct from a tool isError
// result.
func jsonrpcHasError(t *testing.T, line string) bool {
	t.Helper()
	var resp struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Error != nil
}

func TestResolvePoolRoot(t *testing.T) {
	// Roots must be OS-absolute so the "already absolute" and "make relative
	// absolute under root" branches behave the same on every platform; a
	// Unix-style literal like "/main" is not absolute on Windows (no volume),
	// which would send every case down the relative-join branch.
	root := t.TempDir()
	absPool := t.TempDir()
	base := filepath.Base(root)
	cases := []struct {
		value, gitRoot, want string
	}{
		{"$(GitRoot)/.ws-worktrees", root, filepath.Join(root, ".ws-worktrees")},
		// Empty resolves to the out-of-tree default: a sibling of gitRoot named
		// after $(GitRootDirName), not the legacy in-tree ".ws-worktrees".
		{"", root, filepath.Clean(filepath.Join(root, "..", ".ws-worktrees", base))},
		{absPool, root, filepath.Clean(absPool)},
		{"relative/pool", root, filepath.Join(root, "relative", "pool")},
		{"  $(GitRoot)/p  ", root, filepath.Join(root, "p")},
		// $(GitRootDirName) substitutes filepath.Base(gitRoot).
		{"$(GitRoot)/../pool-of-$(GitRootDirName)", root, filepath.Clean(filepath.Join(root, "..", "pool-of-"+base))},
	}
	for _, c := range cases {
		if got := resolvePoolRoot(c.value, c.gitRoot); got != c.want {
			t.Errorf("resolvePoolRoot(%q,%q) = %q, want %q", c.value, c.gitRoot, got, c.want)
		}
	}
}

// TestResolvePoolRootDefaultTemplate pins the builtin default template itself
// (not just its resolution), so a future edit to defaultWorktreePoolTemplate
// that silently drops the sibling-of-repo or $(GitRootDirName) shape is
// caught here rather than only downstream.
func TestResolvePoolRootDefaultTemplate(t *testing.T) {
	if defaultWorktreePoolTemplate != "$(GitRoot)/../.ws-worktrees/$(GitRootDirName)" {
		t.Fatalf("defaultWorktreePoolTemplate = %q, want the out-of-tree sibling template", defaultWorktreePoolTemplate)
	}
	root := t.TempDir()
	got := resolvePoolRoot("", root)
	want := filepath.Clean(filepath.Join(filepath.Dir(root), ".ws-worktrees", filepath.Base(root)))
	if got != want {
		t.Fatalf("resolvePoolRoot(\"\", %q) = %q, want %q", root, got, want)
	}
	if pathUnder(root, got) {
		t.Fatalf("default pool %q must resolve outside the repo %q", got, root)
	}
}

// TestPathUnder pins the directory-boundary semantics documented on pathUnder:
// child must be at or below parent, comparing at directory boundaries so a
// sibling that merely shares parent as a string prefix (/a/bc vs /a/b) is not
// treated as under it. A naive strings.HasPrefix(child, parent) regression
// would report /a/bc as under /a/b, which the "sibling prefix" case below
// catches.
func TestPathUnder(t *testing.T) {
	cases := []struct {
		name, parent, child string
		want                bool
	}{
		{"exact same path", "/a/b", "/a/b", true},
		{"real child", "/a/b", "/a/b/c", true},
		{"nested descendant", "/a/b", "/a/b/c/d", true},
		{"sibling prefix", "/a/b", "/a/bc", false},
		{"clearly outside", "/a/b", "/x/y", false},
		{"parent under child", "/a/b/c", "/a/b", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pathUnder(c.parent, c.child); got != c.want {
				t.Errorf("pathUnder(%q, %q) = %t, want %t", c.parent, c.child, got, c.want)
			}
		})
	}
}

func TestProvisionWorktreeCreateNew(t *testing.T) {
	root, base := worktreeFixture(t)
	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", "")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if res.Reused {
		t.Fatal("first acquire must not report reuse")
	}
	if !pathUnder(res.Pool, res.Path) {
		t.Fatalf("worktree path %q not under resolved pool %q", res.Path, res.Pool)
	}
	if info, err := os.Stat(res.Path); err != nil || !info.IsDir() {
		t.Fatalf("worktree dir missing: %v", err)
	}
	if b := wtBranch(t, res.Path); b != "impl/test/alpha" {
		t.Fatalf("checked-out branch = %q, want impl/test/alpha", b)
	}
	// The default pool is out-of-tree (sibling of the repo), so it must never
	// be registered in .git/info/exclude: registerPoolExclude is scoped to the
	// in-tree fallback only.
	if pathUnder(root, res.Pool) {
		t.Fatalf("default pool %q must resolve outside the repo %q", res.Pool, root)
	}
	excl := filepath.Join(root, ".git", "info", "exclude")
	if data, err := os.ReadFile(excl); err == nil && strings.Contains(string(data), ".ws-worktrees") {
		t.Fatalf("out-of-tree default pool wrongly registered in exclude:\n%s", data)
	}
	if _, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/beta", ""); err != nil {
		t.Fatalf("second provision: %v", err)
	}
}

// TestProvisionWorktreeInTreeOverrideRegistersExclude pins the in-tree
// exclude-registration path (registerPoolExclude), now reachable only via an
// explicit in-tree worktree_pool override since the builtin default moved
// out-of-tree.
func TestProvisionWorktreeInTreeOverrideRegistersExclude(t *testing.T) {
	root, base := worktreeFixture(t)
	if _, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", legacyInTreePoolTemplate); err != nil {
		t.Fatalf("provision: %v", err)
	}
	excl := filepath.Join(root, ".git", "info", "exclude")
	data, err := os.ReadFile(excl)
	if err != nil || !strings.Contains(string(data), "/.ws-worktrees/") {
		t.Fatalf("pool not registered in .git/info/exclude: %v\n%s", err, data)
	}
	if _, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/beta", legacyInTreePoolTemplate); err != nil {
		t.Fatalf("second provision: %v", err)
	}
	data2, _ := os.ReadFile(excl)
	if n := strings.Count(string(data2), "/.ws-worktrees/"); n != 1 {
		t.Fatalf("exclude entry duplicated: count=%d\n%s", n, data2)
	}
}

func TestProvisionWorktreeReuseIdle(t *testing.T) {
	root, base := worktreeFixture(t)
	res1, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", "")
	if err != nil {
		t.Fatalf("provision 1: %v", err)
	}
	if err := releaseWorktree(context.Background(), wsgit.ExecRunner{}, res1.Path, ""); err != nil {
		t.Fatalf("release: %v", err)
	}
	res2, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/beta", "")
	if err != nil {
		t.Fatalf("provision 2: %v", err)
	}
	if !res2.Reused {
		t.Fatal("second acquire after release must reuse the idle worktree")
	}
	if res2.Path != res1.Path {
		t.Fatalf("reuse path = %q, want the released %q", res2.Path, res1.Path)
	}
	if b := wtBranch(t, res2.Path); b != "impl/test/beta" {
		t.Fatalf("reused worktree branch = %q, want impl/test/beta", b)
	}
}

func TestProvisionWorktreeSkipBranchCheckedOut(t *testing.T) {
	root, base := worktreeFixture(t)
	// res1 stays checked out on a branch (not released → not detached), so it is
	// not reuse-eligible; the next acquire must create a distinct worktree.
	res1, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", "")
	if err != nil {
		t.Fatalf("provision 1: %v", err)
	}
	res2, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/beta", "")
	if err != nil {
		t.Fatalf("provision 2: %v", err)
	}
	if res2.Reused || res2.Path == res1.Path {
		t.Fatalf("branch-checked-out worktree must be skipped: reused=%v path=%q", res2.Reused, res2.Path)
	}
}

func TestProvisionWorktreeSkipDirty(t *testing.T) {
	root, base := worktreeFixture(t)
	res1, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", "")
	if err != nil {
		t.Fatalf("provision 1: %v", err)
	}
	if err := releaseWorktree(context.Background(), wsgit.ExecRunner{}, res1.Path, ""); err != nil {
		t.Fatalf("release: %v", err)
	}
	// Make the now-detached pooled worktree dirty AFTER release so it is
	// detached-but-not-clean; eligibility must skip it.
	if err := os.WriteFile(filepath.Join(res1.Path, "cruft.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res2, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/beta", "")
	if err != nil {
		t.Fatalf("provision 2: %v", err)
	}
	if res2.Reused || res2.Path == res1.Path {
		t.Fatalf("dirty worktree must be skipped: reused=%v path=%q", res2.Reused, res2.Path)
	}
}

func TestProvisionWorktreeHygieneResetToBase(t *testing.T) {
	root, base := worktreeFixture(t)
	// Prior run commits a file on alpha, then releases the worktree.
	res1, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", "")
	if err != nil {
		t.Fatalf("provision 1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(res1.Path, "alpha.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, res1.Path, "add", ".")
	runGit(t, res1.Path, "commit", "-m", "alpha work")
	if err := releaseWorktree(context.Background(), wsgit.ExecRunner{}, res1.Path, ""); err != nil {
		t.Fatalf("release: %v", err)
	}
	// Leave a git-IGNORED file behind after release. It does not show in `status
	// --porcelain`, so the worktree stays reuse-eligible, and a plain `git switch`
	// on reuse would NOT remove it — only acquire's `clean -ffdx` (the -x purge)
	// does. This is the discriminating assertion for the hygiene-reset step.
	debris := filepath.Join(res1.Path, "debris.log")
	if err := os.WriteFile(debris, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Reuse for a fresh branch on base: the prior branch's content must not leak.
	res2, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/beta", "")
	if err != nil {
		t.Fatalf("provision 2: %v", err)
	}
	if !res2.Reused || res2.Path != res1.Path {
		t.Fatalf("expected reuse of the released worktree despite the ignored leftover: reused=%v path=%q", res2.Reused, res2.Path)
	}
	if _, err := os.Stat(filepath.Join(res2.Path, "alpha.txt")); !os.IsNotExist(err) {
		t.Fatalf("prior branch file leaked into reused worktree (stat err=%v)", err)
	}
	if _, err := os.Stat(debris); !os.IsNotExist(err) {
		t.Fatalf("ignored leftover not purged by hygiene clean -ffdx (stat err=%v)", err)
	}
}

func TestReleaseWorktreeDetaches(t *testing.T) {
	root, base := worktreeFixture(t)
	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", "")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if b := wtBranch(t, res.Path); b != "impl/test/alpha" {
		t.Fatalf("pre-release branch = %q", b)
	}
	if err := releaseWorktree(context.Background(), wsgit.ExecRunner{}, res.Path, ""); err != nil {
		t.Fatalf("release: %v", err)
	}
	if b := wtBranch(t, res.Path); b != "HEAD" {
		t.Fatalf("post-release worktree not detached: branch = %q", b)
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Fatalf("release must not delete the worktree: %v", err)
	}
}

func TestReleaseWorktreeRefusesNonPoolTargets(t *testing.T) {
	root, base := worktreeFixture(t)
	// Make the primary worktree dirty so a wrongly-permitted release would be
	// observable (and destructive).
	if err := os.WriteFile(filepath.Join(root, "wip.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Releasing the primary worktree is refused.
	if err := releaseWorktree(context.Background(), wsgit.ExecRunner{}, root, ""); err == nil {
		t.Fatal("release of the primary worktree must be refused")
	}
	if _, err := os.Stat(filepath.Join(root, "wip.txt")); err != nil {
		t.Fatalf("refused release must not have touched the primary worktree: %v", err)
	}
	// A linked worktree OUTSIDE the pool is refused (foreign path).
	foreign := filepath.Join(t.TempDir(), "foreign")
	runGit(t, root, "worktree", "add", "--detach", foreign, base)
	if err := releaseWorktree(context.Background(), wsgit.ExecRunner{}, foreign, ""); err == nil {
		t.Fatal("release of a worktree outside the pool must be refused")
	}
}

func TestProvisionWorktreePoolResolvesFromLinkedWorktree(t *testing.T) {
	root, base := worktreeFixture(t)
	mainRoot := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "--path-format=absolute", "--show-toplevel")))
	// A linked worktree OUTSIDE the pool, used as the caller root.
	linked := filepath.Join(t.TempDir(), "linked")
	runGit(t, root, "worktree", "add", "--detach", linked, base)

	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, linked, base, "impl/test/gamma", "")
	if err != nil {
		t.Fatalf("provision from linked worktree: %v", err)
	}
	wantPool := filepath.Clean(filepath.Join(mainRoot, "..", ".ws-worktrees", filepath.Base(mainRoot)))
	if res.Pool != wantPool {
		t.Fatalf("pool = %q, want the main-root pool %q (must not nest under the linked worktree)", res.Pool, wantPool)
	}
	if !pathUnder(wantPool, res.Path) {
		t.Fatalf("worktree %q not under main-root pool %q", res.Path, wantPool)
	}
}

func TestProvisionWorktreeAbsolutePoolOverride(t *testing.T) {
	root, base := worktreeFixture(t)
	pool := filepath.Join(t.TempDir(), "shared-pool")
	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", pool)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if res.Pool != filepath.Clean(pool) {
		t.Fatalf("pool = %q, want override %q", res.Pool, pool)
	}
	if !pathUnder(pool, res.Path) {
		t.Fatalf("worktree %q not under override pool %q", res.Path, pool)
	}
	// An out-of-repo pool must not be registered in .git/info/exclude.
	if data, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude")); err == nil && strings.Contains(string(data), "shared-pool") {
		t.Fatalf("out-of-repo pool wrongly registered in exclude:\n%s", data)
	}
}

// TestProvisionWorktreeDefaultFallsBackWhenParentUnwritable pins the
// Constraints fallback: when the out-of-tree default's sibling parent
// ($(GitRoot)/..) is not creatable/writable, provisioning must fall back to
// the legacy in-tree pool with a caller-visible advisory rather than
// hard-failing.
func TestProvisionWorktreeDefaultFallsBackWhenParentUnwritable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores permission bits; fallback probe needs an enforced read-only parent")
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@test.com")
	runGit(t, root, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	base := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))

	// Remove write on the sibling parent so $(GitRoot)/../.ws-worktrees/... is
	// not creatable. Restored in cleanup (registered before t.TempDir()'s own
	// cleanup runs, so it executes first via LIFO ordering) so the harness can
	// still remove the tree afterward.
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", "")
	if err != nil {
		t.Fatalf("provisioning must fall back, not hard-fail, on an unwritable sibling parent: %v", err)
	}
	wantFallback := filepath.Clean(filepath.Join(root, ".ws-worktrees"))
	if res.Pool != wantFallback {
		t.Fatalf("pool = %q, want the in-tree fallback %q", res.Pool, wantFallback)
	}
	if !pathUnder(root, res.Path) {
		t.Fatalf("worktree %q not under the in-tree fallback pool", res.Path)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected a fallback advisory warning naming the reason and the in-tree path")
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, wantFallback) {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings do not name the in-tree fallback path %q: %v", wantFallback, res.Warnings)
	}
	// The in-tree fallback still registers in .git/info/exclude (existing
	// in-tree behavior), even though the requested pool was the default.
	if data, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude")); err != nil || !strings.Contains(string(data), "/.ws-worktrees/") {
		t.Fatalf("in-tree fallback pool not registered in .git/info/exclude: %v\n%s", err, data)
	}
}

// --- dispatch integration: acquire mints a worktree-bound worker key; release ---

func TestWorktreeAcquireReleaseDispatch(t *testing.T) {
	root, base := worktreeFixture(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := NewServer(root, "test")
	leadKey, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	resp := callToolOnce(t, s, 1, "worktree.acquire", map[string]any{
		"session_key": leadKey, "base": base, "target_branch": "impl/test/alpha", "format": "json",
	})
	var acq worktreeAcquireResult
	if err := json.Unmarshal([]byte(toolText(t, resp)), &acq); err != nil {
		t.Fatalf("unmarshal acquire: %v\n%s", err, resp)
	}
	if acq.WorkerKey == "" || acq.Path == "" {
		t.Fatalf("acquire returned empty path/worker_key: %+v", acq)
	}
	entry, ok := s.sessions.lookup(acq.WorkerKey)
	if !ok {
		t.Fatalf("worker_key %q not registered", acq.WorkerKey)
	}
	if entry.root != acq.Path {
		t.Fatalf("worker key root = %q, want the worktree path %q", entry.root, acq.Path)
	}
	if entry.scope != roleLead {
		t.Fatalf("worker key scope = %q, want lead (worker executes end to end)", entry.scope)
	}
	if entry.parent != leadKey {
		t.Fatalf("worker key parent = %q, want lead %q", entry.parent, leadKey)
	}

	// Release by worker key resolves to its bound worktree and detaches it.
	rel := callToolOnce(t, s, 2, "worktree.release", map[string]any{"session_key": leadKey, "key": acq.WorkerKey})
	if !strings.Contains(toolText(t, rel), "detached: true") {
		t.Fatalf("release response = %s", rel)
	}
	if b := wtBranch(t, acq.Path); b != "HEAD" {
		t.Fatalf("released worktree not detached: %q", b)
	}

	// Re-acquire (reuses the same detached worktree) and release by explicit
	// path — the other half of the path|key contract, through the dispatch path.
	resp2 := callToolOnce(t, s, 3, "worktree.acquire", map[string]any{
		"session_key": leadKey, "base": base, "target_branch": "impl/test/beta", "format": "json",
	})
	var acq2 worktreeAcquireResult
	if err := json.Unmarshal([]byte(toolText(t, resp2)), &acq2); err != nil {
		t.Fatalf("unmarshal acquire 2: %v\n%s", err, resp2)
	}
	relPath := callToolOnce(t, s, 4, "worktree.release", map[string]any{"session_key": leadKey, "path": acq2.Path})
	if !strings.Contains(toolText(t, relPath), "detached: true") {
		t.Fatalf("release-by-path response = %s", relPath)
	}
	if b := wtBranch(t, acq2.Path); b != "HEAD" {
		t.Fatalf("path-released worktree not detached: %q", b)
	}
}

func TestWorktreeAcquireRejectsNonLeadAndBadArgs(t *testing.T) {
	root, base := worktreeFixture(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := NewServer(root, "test")
	leadKey, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	delegateKey, err := s.sessions.mint(canonicalRootForTest(t, root), roleDelegate, leadKey)
	if err != nil {
		t.Fatal(err)
	}
	// Non-lead key is blocked at the capability gate (lead-only tool). The gate
	// returns a JSON-RPC protocol error, not a tool isError result.
	if got := callToolOnce(t, s, 1, "worktree.acquire", map[string]any{
		"session_key": delegateKey, "base": base, "target_branch": "impl/test/alpha",
	}); !jsonrpcHasError(t, got) {
		t.Fatalf("non-lead acquire not rejected: %s", got)
	}
	// Missing target_branch is a structured error.
	if got := callToolOnce(t, s, 2, "worktree.acquire", map[string]any{
		"session_key": leadKey, "base": base,
	}); !toolIsError(t, got) {
		t.Fatalf("missing target_branch not rejected: %s", got)
	}
	// Missing base is a structured error (the other half of the guard).
	if got := callToolOnce(t, s, 3, "worktree.acquire", map[string]any{
		"session_key": leadKey, "target_branch": "impl/test/alpha",
	}); !toolIsError(t, got) {
		t.Fatalf("missing base not rejected: %s", got)
	}
	// Release with neither path nor key is an error.
	if got := callToolOnce(t, s, 4, "worktree.release", map[string]any{"session_key": leadKey}); !toolIsError(t, got) {
		t.Fatalf("release without path/key not rejected: %s", got)
	}
}
