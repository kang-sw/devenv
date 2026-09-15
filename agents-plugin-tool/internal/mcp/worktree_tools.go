package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wskey"
)

// worktreeAcquireResult is the return shape of worktree.acquire. WorkerKey is
// filled by the dispatch handler after the git lifecycle (provisionWorktree)
// completes, because minting a session key needs the Server the pure lifecycle
// code deliberately does not carry.
type worktreeAcquireResult struct {
	Path      string   `json:"path"`
	WorkerKey string   `json:"worker_key,omitempty"`
	Reused    bool     `json:"reused"`
	Pool      string   `json:"pool"`
	Warnings  []string `json:"warnings,omitempty"`
}

func (r worktreeAcquireResult) text() string {
	text := fmt.Sprintf("path: %s\nworker_key: %s\nreused: %t\npool: %s\n", r.Path, r.WorkerKey, r.Reused, r.Pool)
	for _, w := range r.Warnings {
		text += "warning: " + w + "\n"
	}
	return text
}

// wtRun runs a git subcommand under root and returns trimmed combined output.
func wtRun(ctx context.Context, runner wsgit.Runner, root string, args ...string) (string, error) {
	out, err := runner.RunGit(ctx, root, args...)
	return strings.TrimSpace(string(out)), err
}

// worktreeEntry is one parsed record from `git worktree list --porcelain`.
type worktreeEntry struct {
	Path     string
	Head     string
	Branch   string
	Detached bool
}

// listWorktrees parses `git worktree list --porcelain`. The first entry is
// always the primary (main) worktree.
func listWorktrees(ctx context.Context, runner wsgit.Runner, root string) ([]worktreeEntry, error) {
	out, err := wtRun(ctx, runner, root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var entries []worktreeEntry
	var cur *worktreeEntry
	flush := func() {
		if cur != nil {
			entries = append(entries, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			// git emits worktree paths with forward slashes even on Windows;
			// filepath.Clean canonicalizes to the OS separator so a reused
			// (git-sourced) path and a freshly created (filepath.Join-sourced)
			// path share one form and compare equal.
			cur = &worktreeEntry{Path: filepath.Clean(strings.TrimSpace(strings.TrimPrefix(line, "worktree ")))}
		case cur == nil:
			// ignore lines before the first worktree header
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimSpace(strings.TrimPrefix(line, "branch "))
		case line == "detached":
			cur.Detached = true
		}
	}
	flush()
	return entries, nil
}

// resolvePoolRoot resolves the configured worktree_pool value against gitRoot
// (the primary worktree root). It substitutes the $(GitRoot) token, defaults an
// empty value, and makes a relative result absolute under gitRoot.
func resolvePoolRoot(configValue, gitRoot string) string {
	v := strings.TrimSpace(configValue)
	if v == "" {
		v = "$(GitRoot)/.ws-worktrees"
	}
	v = strings.ReplaceAll(v, "$(GitRoot)", gitRoot)
	if !filepath.IsAbs(v) {
		v = filepath.Join(gitRoot, v)
	}
	return filepath.Clean(v)
}

// pathUnder reports whether child is at or below parent, comparing cleaned
// paths at directory boundaries so /a/bc is not treated as under /a/b.
func pathUnder(parent, child string) bool {
	p := filepath.Clean(parent)
	c := filepath.Clean(child)
	if p == c {
		return true
	}
	return strings.HasPrefix(c, p+string(os.PathSeparator))
}

// registerPoolExclude adds the in-repo pool path to the repository's local
// .git/info/exclude (never a committed .gitignore) so the pool never shows up
// as untracked. It is a no-op when the pool is not under the repo and is
// idempotent. commonDir is the absolute path returned by `git rev-parse
// --git-common-dir` (the shared administrative dir for all worktrees).
func registerPoolExclude(commonDir, mainRoot, poolRoot string) error {
	rel, err := filepath.Rel(mainRoot, poolRoot)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return nil // pool is not inside the repo working tree
	}
	entry := "/" + filepath.ToSlash(rel) + "/"
	excludePath := filepath.Join(commonDir, "info", "exclude")
	if data, err := os.ReadFile(excludePath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == entry {
				return nil
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(excludePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	// A leading newline keeps the entry on its own line even if the existing
	// file lacked a trailing newline.
	if info, statErr := f.Stat(); statErr == nil && info.Size() > 0 {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(entry + "\n")
	return err
}

// provisionWorktree owns the whole worktree lifecycle for worktree.acquire
// except the session-key mint (which the dispatch handler performs): resolve the
// pool root against the primary worktree root, reuse an eligible idle pooled
// worktree or create a new one, create/check out target_branch on base,
// hygiene-reset the tree, and sync submodules.
//
// root is the caller's repository root; it may be the main worktree or any
// linked worktree. The pool always resolves against the primary root, so a call
// from a linked worktree never nests a per-worktree pool.
func provisionWorktree(ctx context.Context, runner wsgit.Runner, root, base, targetBranch, poolConfigValue string) (worktreeAcquireResult, error) {
	var res worktreeAcquireResult
	base = strings.TrimSpace(base)
	targetBranch = strings.TrimSpace(targetBranch)
	if base == "" || targetBranch == "" {
		return res, fmt.Errorf("worktree provisioning requires base and target_branch")
	}

	entries, err := listWorktrees(ctx, runner, root)
	if err != nil {
		return res, err
	}
	if len(entries) == 0 {
		return res, fmt.Errorf("could not resolve the primary worktree for %q", root)
	}
	mainRoot := entries[0].Path

	commonDir, err := wtRun(ctx, runner, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return res, err
	}

	poolRoot := resolvePoolRoot(poolConfigValue, mainRoot)
	res.Pool = poolRoot
	if err := os.MkdirAll(poolRoot, 0o755); err != nil {
		return res, fmt.Errorf("create worktree pool %q: %w", poolRoot, err)
	}
	if pathUnder(mainRoot, poolRoot) {
		if err := registerPoolExclude(commonDir, mainRoot, poolRoot); err != nil {
			return res, fmt.Errorf("register pool in git exclude: %w", err)
		}
	}

	// Reuse eligibility (conservative golden rule): under the owned pool prefix,
	// at detached HEAD, and clean. A branch-checked-out or dirty worktree is
	// skipped so no in-progress work is stomped.
	var claim string
	for _, e := range entries {
		if e.Path == mainRoot || !pathUnder(poolRoot, e.Path) || !e.Detached {
			continue
		}
		status, err := wtRun(ctx, runner, e.Path, "status", "--porcelain")
		if err != nil {
			continue // an unreadable candidate is skipped, never adopted
		}
		if status == "" {
			claim = e.Path
			break
		}
	}

	wtPath := claim
	res.Reused = claim != ""
	if !res.Reused {
		stem, err := wskey.Generate()
		if err != nil {
			return res, err
		}
		wtPath = filepath.Join(poolRoot, stem)
		if _, err := wtRun(ctx, runner, root, "worktree", "add", "--detach", wtPath, base); err != nil {
			return res, err
		}
	}

	// Create the branch on base if absent, else check the existing branch out —
	// never reset an existing branch to base, which would drop its commits.
	// `switch` (not `checkout`) is the branch-only verb, so a branch name that
	// happens to collide with a path is never misread as a pathspec.
	branchExists := false
	if _, err := wtRun(ctx, runner, wtPath, "show-ref", "--verify", "--quiet", "refs/heads/"+targetBranch); err == nil {
		branchExists = true
	}
	if branchExists {
		if _, err := wtRun(ctx, runner, wtPath, "switch", "--no-guess", "--", targetBranch); err != nil {
			return res, err
		}
	} else {
		if _, err := wtRun(ctx, runner, wtPath, "switch", "-c", targetBranch, base); err != nil {
			return res, err
		}
	}

	// Hygiene: clear any uncommitted change and untracked/ignored cruft so no
	// prior run's working-tree state leaks forward. This never moves a branch
	// pointer, so committed work on target_branch is preserved.
	if _, err := wtRun(ctx, runner, wtPath, "reset", "--hard"); err != nil {
		return res, err
	}
	if _, err := wtRun(ctx, runner, wtPath, "clean", "-ffdx"); err != nil {
		return res, err
	}

	// Sync submodules only when the worktree actually declares them, keeping a
	// no-submodule repo free of an unnecessary (and network-touching) call.
	if _, statErr := os.Stat(filepath.Join(wtPath, ".gitmodules")); statErr == nil {
		if _, err := wtRun(ctx, runner, wtPath, "submodule", "update", "--init", "--recursive"); err != nil {
			res.Warnings = append(res.Warnings, "submodule sync failed: "+err.Error())
		}
	}

	res.Path = wtPath
	return res, nil
}

// releaseWorktree returns a worktree to the pool: clean it and detach HEAD so it
// becomes reuse-eligible. It never deletes the worktree — the expensive
// derivation is what the pool amortizes.
//
// Because reset --hard/clean -ffdx are destructive, release refuses any target
// that is not a linked worktree under the owned pool: it never touches the
// primary worktree or a foreign path. This is the symmetric guard to acquire's
// reuse-eligibility rule — a mistaken path or a lead's own key bound to the main
// root must not hard-reset the primary working tree.
func releaseWorktree(ctx context.Context, runner wsgit.Runner, wtPath, poolConfigValue string) error {
	wtPath = strings.TrimSpace(wtPath)
	if wtPath == "" {
		return fmt.Errorf("worktree release requires a path")
	}
	entries, err := listWorktrees(ctx, runner, wtPath)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("could not resolve the primary worktree for %q", wtPath)
	}
	mainRoot := entries[0].Path
	// Canonicalize the target through git so a symlinked or non-clean path
	// compares against the git-canonical worktree-list paths.
	target, err := wtRun(ctx, runner, wtPath, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		return err
	}
	target = filepath.Clean(target)
	if target == filepath.Clean(mainRoot) {
		return fmt.Errorf("worktree.release refuses to release the primary worktree %q", target)
	}
	poolRoot := resolvePoolRoot(poolConfigValue, mainRoot)
	if !pathUnder(poolRoot, target) {
		return fmt.Errorf("worktree.release refuses %q: not under the owned worktree pool %q", target, poolRoot)
	}
	listed := false
	for _, e := range entries {
		if filepath.Clean(e.Path) == target {
			listed = true
			break
		}
	}
	if !listed {
		return fmt.Errorf("worktree.release refuses %q: not a registered git worktree", target)
	}
	if _, err := wtRun(ctx, runner, target, "reset", "--hard"); err != nil {
		return err
	}
	if _, err := wtRun(ctx, runner, target, "clean", "-ffdx"); err != nil {
		return err
	}
	if _, err := wtRun(ctx, runner, target, "checkout", "--detach"); err != nil {
		return err
	}
	return nil
}
