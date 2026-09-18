package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", nil, "")
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
	if _, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/beta", nil, ""); err != nil {
		t.Fatalf("second provision: %v", err)
	}
}

// TestProvisionWorktreeInTreeOverrideRegistersExclude pins the in-tree
// exclude-registration path (registerPoolExclude), now reachable only via an
// explicit in-tree worktree_pool override since the builtin default moved
// out-of-tree.
func TestProvisionWorktreeInTreeOverrideRegistersExclude(t *testing.T) {
	root, base := worktreeFixture(t)
	if _, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", nil, legacyInTreePoolTemplate); err != nil {
		t.Fatalf("provision: %v", err)
	}
	excl := filepath.Join(root, ".git", "info", "exclude")
	data, err := os.ReadFile(excl)
	if err != nil || !strings.Contains(string(data), "/.ws-worktrees/") {
		t.Fatalf("pool not registered in .git/info/exclude: %v\n%s", err, data)
	}
	if _, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/beta", nil, legacyInTreePoolTemplate); err != nil {
		t.Fatalf("second provision: %v", err)
	}
	data2, _ := os.ReadFile(excl)
	if n := strings.Count(string(data2), "/.ws-worktrees/"); n != 1 {
		t.Fatalf("exclude entry duplicated: count=%d\n%s", n, data2)
	}
}

func TestProvisionWorktreeReuseIdle(t *testing.T) {
	root, base := worktreeFixture(t)
	res1, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", nil, "")
	if err != nil {
		t.Fatalf("provision 1: %v", err)
	}
	if err := releaseWorktree(context.Background(), wsgit.ExecRunner{}, res1.Path, ""); err != nil {
		t.Fatalf("release: %v", err)
	}
	res2, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/beta", nil, "")
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
	res1, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", nil, "")
	if err != nil {
		t.Fatalf("provision 1: %v", err)
	}
	res2, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/beta", nil, "")
	if err != nil {
		t.Fatalf("provision 2: %v", err)
	}
	if res2.Reused || res2.Path == res1.Path {
		t.Fatalf("branch-checked-out worktree must be skipped: reused=%v path=%q", res2.Reused, res2.Path)
	}
}

func TestProvisionWorktreeSkipDirty(t *testing.T) {
	root, base := worktreeFixture(t)
	res1, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", nil, "")
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
	res2, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/beta", nil, "")
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
	res1, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", nil, "")
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
	res2, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/beta", nil, "")
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
	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", nil, "")
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

	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, linked, base, "impl/test/gamma", nil, "")
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
	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", nil, pool)
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
// hard-failing. It covers both the raw-empty-config caller shape and the
// shape worktree.acquire's real dispatch path actually produces:
// wsconfig.Resolver.Get substitutes the builtin default before
// provisionWorktree is ever called, so poolConfigValue arrives as the
// literal defaultWorktreePoolTemplate string, never "". A fallback gate keyed
// only on the empty string is unreachable in production; both cases must
// trigger the fallback identically.
func TestProvisionWorktreeDefaultFallsBackWhenParentUnwritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows' read-only attribute does not block creating new entries in
		// a directory the way POSIX write-permission removal does, so this
		// chmod-based probe cannot force the fallback path on that platform.
		t.Skip("chmod-based write-protection probe is not meaningful on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root ignores permission bits; fallback probe needs an enforced read-only parent")
	}
	for _, poolConfigValue := range []string{"", defaultWorktreePoolTemplate} {
		t.Run(fmt.Sprintf("poolConfigValue=%q", poolConfigValue), func(t *testing.T) {
			parent := t.TempDir()
			rawRoot := filepath.Join(parent, "repo")
			if err := os.Mkdir(rawRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			runGit(t, rawRoot, "init")
			runGit(t, rawRoot, "config", "user.email", "test@test.com")
			runGit(t, rawRoot, "config", "user.name", "Test")
			if err := os.WriteFile(filepath.Join(rawRoot, "f.txt"), []byte("base\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runGit(t, rawRoot, "add", ".")
			runGit(t, rawRoot, "commit", "-m", "base")
			base := strings.TrimSpace(string(runGitOutput(t, rawRoot, "rev-parse", "HEAD")))
			// Canonicalize root the same way provisionWorktree derives
			// mainRoot (via `git rev-parse --show-toplevel` through
			// listWorktrees), so wantFallback below and the
			// production-computed res.Pool compare in the same form even
			// where the raw temp path and git's canonical toplevel differ
			// (e.g. a /var -> /private/var symlink).
			root := canonicalRootForTest(t, rawRoot)

			// Remove write on the sibling parent so
			// $(GitRoot)/../.ws-worktrees/... is not creatable. Restored in
			// cleanup (registered before t.TempDir()'s own cleanup runs, so
			// it executes first via LIFO ordering) so the harness can still
			// remove the tree afterward.
			if err := os.Chmod(parent, 0o555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

			res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", nil, poolConfigValue)
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
			// The in-tree fallback still registers in .git/info/exclude
			// (existing in-tree behavior), even though the requested pool was
			// the default.
			if data, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude")); err != nil || !strings.Contains(string(data), "/.ws-worktrees/") {
				t.Fatalf("in-tree fallback pool not registered in .git/info/exclude: %v\n%s", err, data)
			}
			// releaseWorktree must accept a fallback-provisioned worktree as
			// owned-pool-eligible even though resolvePoolRoot(poolConfigValue,
			// mainRoot) recomputes the (still-unwritable) out-of-tree default,
			// not the fallback path acquire actually used.
			if err := releaseWorktree(context.Background(), wsgit.ExecRunner{}, res.Path, poolConfigValue); err != nil {
				t.Fatalf("release of a fallback-provisioned worktree must succeed: %v", err)
			}
		})
	}
}

// --- sparse shape and direct base checkout ---

// sparseFixture extends worktreeFixture with a two-directory tree so a cone
// selecting one of them has something to exclude, and returns the repo root,
// the base commit SHA, and the name of the branch the primary worktree holds
// (which is therefore never free for a direct base checkout).
func sparseFixture(t *testing.T) (root, base, primaryBranch string) {
	t.Helper()
	root, _ = worktreeFixture(t)
	for _, rel := range []string{"ai-docs/x.md", "src/y.go"} {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "tree")
	base = strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	// git init's default branch name is a user config knob, so read it rather
	// than assuming master or main.
	primaryBranch = strings.TrimSpace(string(runGitOutput(t, root, "symbolic-ref", "--short", "HEAD")))
	return root, base, primaryBranch
}

func TestProvisionWorktreeSparsePaths(t *testing.T) {
	root, base, _ := sparseFixture(t)
	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", []string{"ai-docs"}, "")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if !res.Sparse {
		t.Fatal("sparse acquire must report sparse: true")
	}
	if _, err := os.Stat(filepath.Join(res.Path, "ai-docs", "x.md")); err != nil {
		t.Fatalf("cone directory not materialized: %v", err)
	}
	if _, err := os.Stat(filepath.Join(res.Path, "src", "y.go")); !os.IsNotExist(err) {
		t.Fatalf("out-of-cone directory materialized (stat err=%v)", err)
	}
	list := strings.TrimSpace(string(runGitOutput(t, res.Path, "sparse-checkout", "list")))
	if list != "ai-docs" {
		t.Fatalf("sparse-checkout list = %q, want ai-docs", list)
	}
	if b := wtBranch(t, res.Path); b != "impl/test/alpha" {
		t.Fatalf("checked-out branch = %q, want impl/test/alpha", b)
	}
	// The hygiene reset/clean must not undo the cone.
	if _, err := os.Stat(filepath.Join(res.Path, "src")); !os.IsNotExist(err) {
		t.Fatalf("hygiene step re-materialized the out-of-cone tree (stat err=%v)", err)
	}
}

// TestProvisionWorktreeSparseReuseMatchesShape pins Decision 5: the pool is two
// disjoint sub-pools, so a request only ever adopts a candidate whose sparse
// state already matches it, and a sparse reuse re-applies the requested
// patterns rather than inheriting the previous acquire's.
func TestProvisionWorktreeSparseReuseMatchesShape(t *testing.T) {
	root, base, _ := sparseFixture(t)

	sparse1, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", []string{"src"}, "")
	if err != nil {
		t.Fatalf("sparse provision: %v", err)
	}
	if err := releaseWorktree(context.Background(), wsgit.ExecRunner{}, sparse1.Path, ""); err != nil {
		t.Fatalf("release sparse: %v", err)
	}
	// A full request must not adopt the released sparse worktree.
	full1, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/beta", nil, "")
	if err != nil {
		t.Fatalf("full provision: %v", err)
	}
	if full1.Reused || full1.Path == sparse1.Path {
		t.Fatalf("full request adopted a sparse candidate: reused=%v path=%q", full1.Reused, full1.Path)
	}
	if full1.Sparse {
		t.Fatal("full acquire must report sparse: false")
	}
	if err := releaseWorktree(context.Background(), wsgit.ExecRunner{}, full1.Path, ""); err != nil {
		t.Fatalf("release full: %v", err)
	}

	// Both shapes are now idle. A sparse request must adopt the sparse one and
	// re-apply the newly requested pattern set.
	sparse2, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/gamma", []string{"ai-docs"}, "")
	if err != nil {
		t.Fatalf("sparse reuse: %v", err)
	}
	if !sparse2.Reused || sparse2.Path != sparse1.Path {
		t.Fatalf("sparse request did not reuse the idle sparse worktree: reused=%v path=%q want %q", sparse2.Reused, sparse2.Path, sparse1.Path)
	}
	list := strings.TrimSpace(string(runGitOutput(t, sparse2.Path, "sparse-checkout", "list")))
	if list != "ai-docs" {
		t.Fatalf("stale pattern set survived reuse: sparse-checkout list = %q, want ai-docs", list)
	}
	if _, err := os.Stat(filepath.Join(sparse2.Path, "src", "y.go")); !os.IsNotExist(err) {
		t.Fatalf("previous cone still materialized after reuse (stat err=%v)", err)
	}

	// And a full request must adopt the idle full one, not the sparse one.
	full2, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/delta", nil, "")
	if err != nil {
		t.Fatalf("full reuse: %v", err)
	}
	if !full2.Reused || full2.Path != full1.Path {
		t.Fatalf("full request did not reuse the idle full worktree: reused=%v path=%q want %q", full2.Reused, full2.Path, full1.Path)
	}
}

// TestProvisionWorktreeBaseDirectCheckout pins Decision 4: an omitted
// target_branch checks base itself out, so a commit made in the pooled worktree
// advances base with no extra branch and no merge step.
func TestProvisionWorktreeBaseDirectCheckout(t *testing.T) {
	root, base, _ := sparseFixture(t)
	runGit(t, root, "branch", "parent-line", base)

	res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, "parent-line", "", []string{"ai-docs"}, "")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if b := strings.TrimSpace(string(runGitOutput(t, res.Path, "symbolic-ref", "--short", "HEAD"))); b != "parent-line" {
		t.Fatalf("checked-out branch = %q, want parent-line", b)
	}
	if err := os.WriteFile(filepath.Join(res.Path, "ai-docs", "x.md"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, res.Path, "add", "ai-docs/x.md")
	runGit(t, res.Path, "commit", "-m", "housekeeping")
	wtTip := strings.TrimSpace(string(runGitOutput(t, res.Path, "rev-parse", "HEAD")))
	repoTip := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "parent-line")))
	if wtTip != repoTip || repoTip == base {
		t.Fatalf("commit did not advance base: worktree=%s repo=%s base=%s", wtTip, repoTip, base)
	}
}

func TestProvisionWorktreeBaseDirectCheckoutRefusesHeldBranch(t *testing.T) {
	root, base, primaryBranch := sparseFixture(t)
	before, err := listWorktrees(context.Background(), wsgit.ExecRunner{}, root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, primaryBranch, "", []string{"ai-docs"}, "")
	if err == nil {
		t.Fatal("checking out a branch the primary worktree holds must fail")
	}
	if !strings.Contains(err.Error(), "check out base") {
		t.Fatalf("error does not name the failed step: %v", err)
	}
	if !strings.Contains(err.Error(), "target_branch") {
		t.Fatalf("error does not offer the target_branch alternative: %v", err)
	}
	// The refusal must leave nothing behind. A worktree created --no-checkout
	// has an empty index, so `status --porcelain` reports every tracked path
	// deleted and the reuse scan would skip it forever — one dead pool entry per
	// refused acquire, releasable by nobody since no key was ever bound to it.
	after, err := listWorktrees(context.Background(), wsgit.ExecRunner{}, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("refused acquire leaked a worktree: %d registered before, %d after", len(before), len(after))
	}
	// And the next acquire still works, rather than tripping over the leftover.
	if _, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", []string{"ai-docs"}, ""); err != nil {
		t.Fatalf("acquire after a refused one: %v", err)
	}
}

// TestProvisionWorktreeSparsePathsNormalization pins the two input-shape rules
// the sparse cone depends on: a list that carries no usable entry is not a
// sparse request at all, and a cone entry is always a path, never an option.
func TestProvisionWorktreeSparsePathsNormalization(t *testing.T) {
	t.Run("blank entries mean no sparse request", func(t *testing.T) {
		root, base, _ := sparseFixture(t)
		res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", []string{"", "  "}, "")
		if err != nil {
			t.Fatalf("provision: %v", err)
		}
		if res.Sparse {
			t.Fatal("a list of blanks must not report a sparse shape")
		}
		if _, err := os.Stat(filepath.Join(res.Path, "src", "y.go")); err != nil {
			t.Fatalf("full checkout expected, but the tree is filtered: %v", err)
		}
	})

	t.Run("multi-directory cone", func(t *testing.T) {
		root, base, _ := sparseFixture(t)
		res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", []string{" ai-docs ", "src"}, "")
		if err != nil {
			t.Fatalf("provision: %v", err)
		}
		for _, rel := range []string{"ai-docs/x.md", "src/y.go"} {
			if _, err := os.Stat(filepath.Join(res.Path, filepath.FromSlash(rel))); err != nil {
				t.Fatalf("%s not materialized by a two-directory cone: %v", rel, err)
			}
		}
		list := strings.Fields(strings.TrimSpace(string(runGitOutput(t, res.Path, "sparse-checkout", "list"))))
		if len(list) != 2 {
			t.Fatalf("sparse-checkout list = %v, want both directories", list)
		}
	})

	t.Run("a dash-leading entry stays a path", func(t *testing.T) {
		root, base, _ := sparseFixture(t)
		// Without a `--` separator git parses this as the --no-cone option: it
		// exits 0, flips cone mode off, and empties the pattern set, leaving a
		// worktree that reports sparse: true while filtering nothing.
		res, err := provisionWorktree(context.Background(), wsgit.ExecRunner{}, root, base, "impl/test/alpha", []string{"--no-cone"}, "")
		if err != nil {
			t.Fatalf("provision: %v", err)
		}
		if cone := strings.TrimSpace(string(runGitOutput(t, res.Path, "config", "--get", "core.sparseCheckoutCone"))); cone != "true" {
			t.Fatalf("cone mode = %q, want true: the entry was parsed as an option", cone)
		}
		list := strings.TrimSpace(string(runGitOutput(t, res.Path, "sparse-checkout", "list")))
		if list != "--no-cone" {
			t.Fatalf("sparse-checkout list = %q, want the literal directory name", list)
		}
	})
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

// TestWorktreeAcquireSparseDispatch walks the lead's housekeeping call through
// the dispatch path: a sparse worktree on the parent branch itself, and a
// git.commit through the returned worker key landing there.
func TestWorktreeAcquireSparseDispatch(t *testing.T) {
	root, base, _ := sparseFixture(t)
	runGit(t, root, "branch", "parent-line", base)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := NewServer(root, "test")
	leadKey, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	resp := callToolOnce(t, s, 1, "worktree.acquire", map[string]any{
		"session_key": leadKey, "base": "parent-line",
		"sparse_paths": []string{"ai-docs"}, "format": "json",
	})
	var acq worktreeAcquireResult
	if err := json.Unmarshal([]byte(toolText(t, resp)), &acq); err != nil {
		t.Fatalf("unmarshal acquire: %v\n%s", err, resp)
	}
	if !acq.Sparse {
		t.Fatalf("sparse acquire did not report the shape it got: %+v", acq)
	}
	if acq.WorkerKey == "" {
		t.Fatalf("acquire returned no worker_key: %+v", acq)
	}
	if _, err := os.Stat(filepath.Join(acq.Path, "src")); !os.IsNotExist(err) {
		t.Fatalf("out-of-cone directory materialized (stat err=%v)", err)
	}
	// The text response carries the shape too, for the default (non-JSON) caller.
	textResp := callToolOnce(t, s, 2, "worktree.acquire", map[string]any{
		"session_key": leadKey, "base": base, "target_branch": "impl/test/alpha",
	})
	if !strings.Contains(toolText(t, textResp), "sparse: false") {
		t.Fatalf("text response omits the sparse field: %s", textResp)
	}

	if err := os.WriteFile(filepath.Join(acq.Path, "ai-docs", "x.md"), []byte("housekeeping\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit := callToolOnce(t, s, 3, "git.commit", map[string]any{
		"session_key":     acq.WorkerKey,
		"paths":           []string{"ai-docs/x.md"},
		"title":           "docs: housekeeping",
		"ai_context":      []string{"sparse housekeeping worktree"},
		"expected_branch": "parent-line",
	})
	if toolIsError(t, commit) {
		t.Fatalf("git.commit through the worker key failed: %s", commit)
	}
	if tip := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "parent-line"))); tip == base {
		t.Fatal("commit through the worker key did not advance the parent branch")
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
	// Missing target_branch is no longer an error: it means "check base out
	// directly", so a free local branch as base must be accepted.
	runGit(t, root, "branch", "housekeeping-base", base)
	if got := callToolOnce(t, s, 2, "worktree.acquire", map[string]any{
		"session_key": leadKey, "base": "housekeeping-base",
	}); toolIsError(t, got) {
		t.Fatalf("omitted target_branch must be accepted: %s", got)
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
