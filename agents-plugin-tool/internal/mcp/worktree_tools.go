package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wskey"
)

// worktreeAcquireResult is the return shape of worktree.acquire. WorkerKey is
// filled by the dispatch handler after the git lifecycle (provisionWorktree)
// completes, because minting a session key needs the Server the pure lifecycle
// code deliberately does not carry.
type worktreeAcquireResult struct {
	Path      string `json:"path"`
	WorkerKey string `json:"worker_key,omitempty"`
	Reused    bool   `json:"reused"`
	Pool      string `json:"pool"`
	Profile   string `json:"profile"`
	// Provisioned reports that this acquire put the worktree into the profile
	// (fresh, adopted, or re-adopted after a definition change): declared
	// sparse was applied.
	Provisioned bool `json:"provisioned"`
	// Hooks lists every profile hook this acquire ran, in order.
	Hooks []worktreeHookRun `json:"hooks,omitempty"`
	// Nudge lists pooled worktrees worth removing (stale branch holders,
	// detached ones with large preserved artifacts); the once-a-day check
	// fills it on at most one acquire per pool per day.
	Nudge    []string `json:"nudge,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

func (r worktreeAcquireResult) text() string {
	text := fmt.Sprintf("path: %s\nworker_key: %s\nreused: %t\npool: %s\nprofile: %s\nprovisioned: %t\n", r.Path, r.WorkerKey, r.Reused, r.Pool, r.Profile, r.Provisioned)
	for _, h := range r.Hooks {
		text += h.text()
	}
	for _, n := range r.Nudge {
		text += "nudge: " + n + "\n"
	}
	for _, w := range r.Warnings {
		text += "warning: " + w + "\n"
	}
	return text
}

// noSubmoduleRecurse prefixes a checkout-family git invocation (worktree add,
// sparse-checkout set, reset --hard, clean, switch, checkout --detach) with
// -c submodule.recurse=false. Under a user or repository submodule.recurse=true
// those verbs recurse into submodules a pool worktree has not initialized and
// fail with "not a git repository" (worktree add even leaves a broken submodule
// gitfile behind that also breaks the later submodule update). The explicit
// submodule update --init --recursive is what initializes submodules here.
func noSubmoduleRecurse(args ...string) []string {
	return append([]string{"-c", "submodule.recurse=false"}, args...)
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
	// Prunable is set when the porcelain record carries a `prunable` line: the
	// worktree's directory is gone but its record still holds the branch for
	// git's one-checkout-per-branch rule, so a merge into that branch is still
	// blocked until `git worktree prune` runs.
	Prunable bool
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
		case line == "prunable" || strings.HasPrefix(line, "prunable "):
			cur.Prunable = true
		}
	}
	flush()
	return entries, nil
}

const (
	// defaultWorktreePoolTemplate is the builtin default for the worktree_pool
	// config item: a filesystem sibling of the repo (same volume, so `.git`
	// object sharing / hardlinks stay cheap) but outside the repo working
	// tree, so IDEs that auto-index everything under the workspace root do
	// not pick up the per-worker pooled worktrees. server.go's
	// builtinConfigDefaults and the worktree.acquire tool description both
	// read this constant so no default-definition site can state a different
	// value.
	defaultWorktreePoolTemplate = "$(GitRoot)/../.ws-worktrees/$(GitRootDirName)"

	// legacyInTreePoolTemplate is the pre-existing in-tree pool location.
	// resolvePoolRoot no longer defaults to it, but provisionWorktree falls
	// back to it when the out-of-tree default's parent is not
	// creatable/writable (mount root, read-only parent, container
	// /workspace), so a repo never becomes un-provisionable.
	legacyInTreePoolTemplate = "$(GitRoot)/.ws-worktrees"
)

// isDefaultPoolConfig reports whether configValue represents "use the builtin
// worktree_pool default" rather than an explicit override. An empty value
// means exactly that at the call site that reads raw project/global config
// files directly, but the layered wsconfig.Resolver used by worktree.acquire
// already substitutes the builtin default before provisionWorktree ever sees
// it (Resolver.Get falls back to the builtin map, never returns ""), so the
// literal defaultWorktreePoolTemplate string must be recognized too. Without
// this, the unwritable-parent fallback below is unreachable through the real
// dispatch path — a caller-visible bug distinct from resolvePoolRoot's own
// (still correct) empty-string handling.
func isDefaultPoolConfig(configValue string) bool {
	v := strings.TrimSpace(configValue)
	return v == "" || v == defaultWorktreePoolTemplate
}

// resolvePoolRoot resolves the configured worktree_pool value against gitRoot
// (the primary worktree root). It substitutes the $(GitRootDirName) and
// $(GitRoot) tokens, defaults an empty value to defaultWorktreePoolTemplate,
// and makes a relative result absolute under gitRoot.
func resolvePoolRoot(configValue, gitRoot string) string {
	v := strings.TrimSpace(configValue)
	if v == "" {
		v = defaultWorktreePoolTemplate
	}
	v = strings.ReplaceAll(v, "$(GitRootDirName)", filepath.Base(gitRoot))
	v = strings.ReplaceAll(v, "$(GitRoot)", gitRoot)
	if !filepath.IsAbs(v) {
		v = filepath.Join(gitRoot, v)
	}
	return filepath.Clean(v)
}

// ownedPoolRoots returns every pool root whose worktrees ws owns: the resolved
// worktree_pool root and, when the config is the builtin default, the legacy
// in-tree pool too, because acquire may have fallen back to it when the
// out-of-tree default's sibling parent was not writable at provision time — a
// fallback decision a later call cannot see. release, list, and git.merge's
// holder classification all share this one ownership rule, so no worktree is
// releasable but unlisted.
func ownedPoolRoots(poolConfigValue, mainRoot string) []string {
	roots := []string{resolvePoolRoot(poolConfigValue, mainRoot)}
	if isDefaultPoolConfig(poolConfigValue) {
		roots = append(roots, resolvePoolRoot(legacyInTreePoolTemplate, mainRoot))
	}
	return roots
}

// underAnyPool reports whether path is at or below one of the pool roots.
func underAnyPool(poolRoots []string, path string) bool {
	for _, root := range poolRoots {
		if root != "" && pathUnder(root, path) {
			return true
		}
	}
	return false
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
// worktree of the requested profile or create a new one, provision it into the
// profile when it is fresh or its provisioning inputs changed, create/check out
// target_branch on base, hygiene-reset the tree (keeping the profile's
// preserve patterns), sync submodules, run the profile's post_provision (on
// entry) and post_acquire hooks, and check that the ticket board is visible.
// A failing post_provision or post_acquire hook quarantines the worktree.
//
// root is the caller's repository root; it may be the main worktree or any
// linked worktree. The pool always resolves against the primary root, so a call
// from a linked worktree never nests a per-worktree pool.
func provisionWorktree(ctx context.Context, runner wsgit.Runner, root, base, targetBranch string, profile resolvedWorktreeProfile, poolConfigValue string) (worktreeAcquireResult, error) {
	var res worktreeAcquireResult
	base = strings.TrimSpace(base)
	targetBranch = strings.TrimSpace(targetBranch)
	if base == "" {
		return res, fmt.Errorf("worktree provisioning requires base")
	}
	if profile.Name == "" {
		return res, fmt.Errorf("worktree provisioning requires a profile")
	}
	res.Profile = profile.Name
	def := profile.Definition
	hash := def.provisionHash()

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
		// The out-of-tree default's sibling parent can be uncreatable (mount
		// root, read-only parent, container /workspace). Fall back to the
		// legacy in-tree pool rather than leaving the repo un-provisionable.
		// isDefaultPoolConfig is a value comparison, not override provenance:
		// it cannot distinguish "unset" from an explicit override that
		// happens to equal the literal default template, but the resolved
		// path is identical either way, so that one coincidental value is the
		// only override the fallback ever applies to.
		if isDefaultPoolConfig(poolConfigValue) && !pathUnder(mainRoot, poolRoot) {
			fallback := resolvePoolRoot(legacyInTreePoolTemplate, mainRoot)
			if fbErr := os.MkdirAll(fallback, 0o755); fbErr != nil {
				return res, fmt.Errorf("create worktree pool %q (in-tree fallback %q also failed): %w", poolRoot, fallback, fbErr)
			}
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"out-of-tree worktree pool %q not creatable (%v); falling back to in-tree pool %q",
				poolRoot, err, fallback))
			poolRoot = fallback
			res.Pool = poolRoot
		} else {
			return res, fmt.Errorf("create worktree pool %q: %w", poolRoot, err)
		}
	}
	if pathUnder(mainRoot, poolRoot) {
		if err := registerPoolExclude(commonDir, mainRoot, poolRoot); err != nil {
			return res, fmt.Errorf("register pool in git exclude: %w", err)
		}
	}

	// Reuse eligibility (conservative golden rule): under the owned pool prefix,
	// at detached HEAD, and clean — a branch-checked-out or dirty worktree is
	// skipped so no in-progress work is stomped — and in the requested profile:
	// its marker names the profile, so a worktree provisioned for one profile
	// never serves another. An unmarked worktree predates profiles: it is
	// adopted only into the builtin its sparse state matches (sparse-active =>
	// housekeeping, otherwise full); a custom profile never adopts one. A
	// quarantined worktree is never reused.
	var claim string
	var claimMarker *worktreeMarker
	for _, e := range entries {
		if e.Path == mainRoot || !pathUnder(poolRoot, e.Path) || !e.Detached {
			continue
		}
		marker, err := readWorktreeMarker(ctx, runner, e.Path)
		if err != nil {
			continue // an unreadable candidate is skipped, never adopted
		}
		if marker != nil {
			if marker.Profile != profile.Name || marker.Quarantine != nil {
				continue
			}
		} else if legacyWorktreeProfile(e.Path) != profile.Name {
			continue
		}
		status, err := wtRun(ctx, runner, e.Path, "status", "--porcelain")
		if err != nil {
			continue
		}
		if status == "" {
			claim, claimMarker = e.Path, marker
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
		// A sparse profile's worktree is created without a checkout so its
		// declared patterns decide what materializes; the hygiene reset below
		// then populates only what the patterns select.
		addArgs := noSubmoduleRecurse("worktree", "add", "--detach")
		if def.Sparse != nil {
			addArgs = append(addArgs, "--no-checkout")
		}
		addArgs = append(addArgs, wtPath, base)
		if _, err := wtRun(ctx, runner, root, addArgs...); err != nil {
			return res, err
		}
	}

	// A worktree enters the profile when it is fresh, unmarked (legacy), or
	// marked under different provisioning inputs (a definition change). Stamp
	// the profile with an empty hash first: a failure from here on leaves the
	// worktree in this profile's partition, re-provisioned on its next claim,
	// never an unmarked entry a builtin could adopt.
	provision := claimMarker == nil || claimMarker.ProvisionHash != hash
	if provision {
		if err := writeWorktreeMarker(ctx, runner, wtPath, worktreeMarker{Profile: profile.Name}); err != nil {
			return res, fmt.Errorf("worktree.acquire: record profile marker: %w", err)
		}
		// Declared sparse is applied only here, on entry, so sparse changes a
		// hook makes afterwards persist across reuse.
		if err := applyDeclaredSparse(ctx, runner, wtPath, def.Sparse); err != nil {
			return res, err
		}
		res.Provisioned = true
	}

	// Branch step. With target_branch omitted, check base out directly so the
	// lead's commits land on base with no extra branch and no merge step; Git's
	// one-checkout-per-branch rule is the safety net (base is free exactly while
	// a worker holds impl/* in the root). Otherwise create the branch on base if
	// absent, else check the existing branch out — never reset an existing
	// branch to base, which would drop its commits. `switch` (not `checkout`) is
	// the branch-only verb, so a branch name that collides with a path is never
	// misread as a pathspec.
	switch {
	case targetBranch == "":
		if _, err := wtRun(ctx, runner, wtPath, noSubmoduleRecurse("switch", "--no-guess", "--", base)...); err != nil {
			return res, fmt.Errorf("worktree.acquire: check out base %q: %v; the actual checkout is held elsewhere — release the worktree holding it first, or pass target_branch", base, err)
		}
	default:
		branchExists := false
		if _, err := wtRun(ctx, runner, wtPath, "show-ref", "--verify", "--quiet", "refs/heads/"+targetBranch); err == nil {
			branchExists = true
		}
		if branchExists {
			if _, err := wtRun(ctx, runner, wtPath, noSubmoduleRecurse("switch", "--no-guess", "--", targetBranch)...); err != nil {
				return res, err
			}
		} else {
			if _, err := wtRun(ctx, runner, wtPath, noSubmoduleRecurse("switch", "-c", targetBranch, base)...); err != nil {
				return res, err
			}
		}
	}

	// Hygiene: clear any uncommitted change and untracked/ignored cruft so no
	// prior run's working-tree state leaks forward, keeping the profile's
	// preserve patterns. This never moves a branch pointer, so committed work
	// on target_branch is preserved.
	if err := worktreeHygiene(ctx, runner, wtPath, def.Preserve); err != nil {
		return res, err
	}

	// Sync submodules only when the worktree actually declares them, keeping a
	// no-submodule repo free of an unnecessary (and network-touching) call.
	if _, statErr := os.Stat(filepath.Join(wtPath, ".gitmodules")); statErr == nil {
		if _, err := wtRun(ctx, runner, wtPath, "submodule", "update", "--init", "--recursive"); err != nil {
			res.Warnings = append(res.Warnings, "submodule sync failed: "+err.Error())
		}
	}

	hookEnv := worktreeHookEnv{WorktreeDir: wtPath, Base: base, TargetBranch: targetBranch}
	if hookEnv.TargetBranch == "" {
		hookEnv.TargetBranch = base
	}
	var hooks worktreeProfileHooks
	if def.Hooks != nil {
		hooks = *def.Hooks
	}
	if provision {
		runs, failure := runWorktreeHooks(ctx, worktreeHookPostProvision, hooks.PostProvision, profile.Scope, hookEnv)
		res.Hooks = append(res.Hooks, runs...)
		if failure != nil {
			return res, quarantineWorktree(ctx, runner, root, poolRoot, wtPath, profile, failure)
		}
		if len(runs) > 0 {
			// Hygiene ran just before the hooks, so every unpreserved ignored
			// path now present is one a post_provision hook created; the next
			// acquire's hygiene would delete it.
			if paths, err := unpreservedIgnoredPaths(ctx, runner, wtPath, def.Preserve); err != nil {
				res.Warnings = append(res.Warnings, "could not check post_provision outputs against preserve: "+err.Error())
			} else if len(paths) > 0 {
				res.Warnings = append(res.Warnings, fmt.Sprintf("post_provision created ignored paths that preserve does not cover, so the next acquire's hygiene deletes them: %s; add them to profile %q's preserve if they must persist", strings.Join(paths, ", "), profile.Name))
			}
		}
		if err := writeWorktreeMarker(ctx, runner, wtPath, worktreeMarker{Profile: profile.Name, ProvisionHash: hash}); err != nil {
			return res, fmt.Errorf("worktree.acquire: record profile marker: %w", err)
		}
	}
	runs, failure := runWorktreeHooks(ctx, worktreeHookPostAcquire, hooks.PostAcquire, profile.Scope, hookEnv)
	res.Hooks = append(res.Hooks, runs...)
	if failure != nil {
		return res, quarantineWorktree(ctx, runner, root, poolRoot, wtPath, profile, failure)
	}

	// Workers record results into their ticket, so an acquired worktree must
	// expose the ticket board whenever the checked-out commit tracks one.
	if err := checkTicketBoardVisible(ctx, runner, wtPath); err != nil {
		detachErr := releaseHygiene(ctx, runner, wtPath, def.Preserve)
		msg := fmt.Sprintf("worktree.acquire: profile %q leaves %s hidden in %s (%v); a worker could not record its ticket result there. Fix the profile's sparse (or the hook that narrows it) so %s stays checked out", profile.Name, wsdoc.TicketsDir, wtPath, err, wsdoc.TicketsDir)
		if detachErr != nil {
			msg += fmt.Sprintf("; returning the worktree to the pool also failed: %v", detachErr)
		}
		return res, fmt.Errorf("%s", msg)
	}

	res.Path = wtPath
	return res, nil
}

// legacyWorktreeProfile is the builtin an unmarked (pre-profile) pooled
// worktree is adopted into, by the boolean rule pool reuse used before
// profiles: any active sparse-checkout => housekeeping, otherwise full. The
// first claim re-provisions it, normalizing an arbitrary prior cone to the
// builtin's declared sparse.
func legacyWorktreeProfile(wtPath string) string {
	if wsdoc.SparseCheckoutActive(wtPath) {
		return worktreeProfileHousekeeping
	}
	return worktreeProfileFull
}

// applyDeclaredSparse puts wtPath's sparse-checkout in the profile's declared
// shape: none (a full checkout), cone, or non-cone patterns. set rewrites both
// the pattern file and core.sparseCheckoutCone, so a worktree previously in
// the other mode switches cleanly.
func applyDeclaredSparse(ctx context.Context, runner wsgit.Runner, wtPath string, sparse *worktreeProfileSparse) error {
	switch {
	case sparse == nil:
		if !wsdoc.SparseCheckoutActive(wtPath) {
			return nil
		}
		_, err := wtRun(ctx, runner, wtPath, noSubmoduleRecurse("sparse-checkout", "disable")...)
		return err
	case sparse.Mode == worktreeSparseModePatterns:
		_, err := wtRun(ctx, runner, wtPath, noSubmoduleRecurse(append([]string{"sparse-checkout", "set", "--no-cone"}, sparse.Paths...)...)...)
		return err
	default:
		_, err := wtRun(ctx, runner, wtPath, noSubmoduleRecurse(append([]string{"sparse-checkout", "set", "--cone"}, sparse.Paths...)...)...)
		return err
	}
}

// worktreeHygiene discards every uncommitted change and untracked or ignored
// path except those matching preserve. Under -x, git clean still honors the
// -e patterns as ignore rules, so a preserved path survives while every other
// ignored build product is removed.
func worktreeHygiene(ctx context.Context, runner wsgit.Runner, wtPath string, preserve []string) error {
	if _, err := wtRun(ctx, runner, wtPath, noSubmoduleRecurse("reset", "--hard")...); err != nil {
		return err
	}
	clean := []string{"clean", "-ffdx"}
	for _, pattern := range preserve {
		clean = append(clean, "-e", pattern)
	}
	_, err := wtRun(ctx, runner, wtPath, noSubmoduleRecurse(clean...)...)
	return err
}

// releaseHygiene is release's working-tree step: hygiene, then a detached HEAD
// so the checked-out branch is freed and the worktree is reuse-eligible.
func releaseHygiene(ctx context.Context, runner wsgit.Runner, wtPath string, preserve []string) error {
	if err := worktreeHygiene(ctx, runner, wtPath, preserve); err != nil {
		return err
	}
	_, err := wtRun(ctx, runner, wtPath, noSubmoduleRecurse("checkout", "--detach")...)
	return err
}

// checkTicketBoardVisible reports an error when the checked-out commit tracks
// the ticket board but the worktree does not materialize it.
func checkTicketBoardVisible(ctx context.Context, runner wsgit.Runner, wtPath string) error {
	if _, err := wtRun(ctx, runner, wtPath, "cat-file", "-e", "HEAD:"+wsdoc.TicketsDir); err != nil {
		return nil // the commit tracks no ticket board; nothing to expose
	}
	info, err := os.Stat(filepath.Join(wtPath, filepath.FromSlash(wsdoc.TicketsDir)))
	if err != nil {
		return fmt.Errorf("%s is not checked out", wsdoc.TicketsDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", wsdoc.TicketsDir)
	}
	return nil
}

// worktreeProfileLookup resolves a profile name at release time. It reports
// found=false for a name no scope defines any longer, and an error when the
// profiles could not be resolved at all.
type worktreeProfileLookup func(name string) (resolvedWorktreeProfile, bool, error)

// staticProfileLookup serves a lookup from one resolution result.
func staticProfileLookup(profiles []resolvedWorktreeProfile, resolveErr error) worktreeProfileLookup {
	return func(name string) (resolvedWorktreeProfile, bool, error) {
		if resolveErr != nil {
			return resolvedWorktreeProfile{}, false, resolveErr
		}
		p, ok := findWorktreeProfile(profiles, name)
		return p, ok, nil
	}
}

// worktreeReleaseResult is the return shape of worktree.release.
type worktreeReleaseResult struct {
	Path     string            `json:"path"`
	Detached bool              `json:"detached"`
	Profile  string            `json:"profile"`
	Hooks    []worktreeHookRun `json:"hooks,omitempty"`
	Warnings []string          `json:"warnings,omitempty"`
}

func (r worktreeReleaseResult) text() string {
	text := fmt.Sprintf("released: %s\ndetached: %t\nprofile: %s\n", r.Path, r.Detached, r.Profile)
	for _, h := range r.Hooks {
		text += h.text()
	}
	for _, w := range r.Warnings {
		text += "warning: " + w + "\n"
	}
	return text
}

// ownedPoolWorktree canonicalizes wtPath through git and refuses, on behalf of
// tool, anything but a registered linked worktree under the owned pool: the
// primary worktree and foreign paths are never hygiene-reset or removed. It
// returns the canonical target, its worktree-list entry, and the primary root.
func ownedPoolWorktree(ctx context.Context, runner wsgit.Runner, wtPath, poolConfigValue, tool string) (string, worktreeEntry, string, error) {
	wtPath = strings.TrimSpace(wtPath)
	if wtPath == "" {
		return "", worktreeEntry{}, "", fmt.Errorf("%s requires a path", tool)
	}
	entries, err := listWorktrees(ctx, runner, wtPath)
	if err != nil {
		return "", worktreeEntry{}, "", err
	}
	if len(entries) == 0 {
		return "", worktreeEntry{}, "", fmt.Errorf("could not resolve the primary worktree for %q", wtPath)
	}
	mainRoot := entries[0].Path
	// Canonicalize the target through git so a symlinked or non-clean path
	// compares against the git-canonical worktree-list paths.
	target, err := wtRun(ctx, runner, wtPath, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		return "", worktreeEntry{}, "", err
	}
	target = filepath.Clean(target)
	if target == filepath.Clean(mainRoot) {
		return "", worktreeEntry{}, "", fmt.Errorf("%s refuses the primary worktree %q", tool, target)
	}
	poolRoots := ownedPoolRoots(poolConfigValue, mainRoot)
	if !underAnyPool(poolRoots, target) {
		return "", worktreeEntry{}, "", fmt.Errorf("%s refuses %q: not under the owned worktree pool %q", tool, target, poolRoots[0])
	}
	for _, e := range entries {
		if filepath.Clean(e.Path) == target {
			return target, e, mainRoot, nil
		}
	}
	return "", worktreeEntry{}, "", fmt.Errorf("%s refuses %q: not a registered git worktree", tool, target)
}

// worktreeLegacyLabel is the profile an unmarked pooled worktree reports.
const worktreeLegacyLabel = "legacy"

// releaseWorktree returns a worktree to the pool: run its profile's
// pre_release hooks (a failure leaves the worktree untouched and leased), clean
// it (keeping its profile's preserve patterns), detach HEAD so it becomes
// reuse-eligible, and remove its worktree lease. It never deletes the worktree — the expensive
// derivation is what the pool amortizes. An unmarked (legacy) worktree gets
// the pre-profile hygiene only; a worktree whose profile no longer resolves
// gets the same, with a warning.
//
// Because reset --hard/clean -ffdx are destructive, release refuses any target
// that is not a linked worktree under the owned pool: it never touches the
// primary worktree or a foreign path. This is the symmetric guard to acquire's
// reuse-eligibility rule — a mistaken path or a lead's own key bound to the main
// root must not hard-reset the primary working tree.
func releaseWorktree(ctx context.Context, runner wsgit.Runner, wtPath, poolConfigValue string, lookup worktreeProfileLookup) (worktreeReleaseResult, error) {
	var res worktreeReleaseResult
	target, entry, _, err := ownedPoolWorktree(ctx, runner, wtPath, poolConfigValue, "worktree.release")
	if err != nil {
		return res, err
	}
	var branch string
	if !entry.Detached {
		branch = strings.TrimPrefix(entry.Branch, "refs/heads/")
	}
	res.Path = target

	marker, err := readWorktreeMarker(ctx, runner, target)
	if err != nil {
		return res, fmt.Errorf("worktree.release: %w", err)
	}
	var preserve []string
	switch {
	case marker == nil:
		res.Profile = worktreeLegacyLabel
	default:
		res.Profile = marker.Profile
		profile, found, err := lookup(marker.Profile)
		if err != nil {
			return res, fmt.Errorf("worktree.release: resolve profile %q of %s: %w", marker.Profile, target, err)
		}
		if found {
			preserve = profile.Definition.Preserve
			if h := profile.Definition.Hooks; h != nil {
				runs, failure := runWorktreeHooks(ctx, worktreeHookPreRelease, h.PreRelease, profile.Scope, worktreeHookEnv{WorktreeDir: target, TargetBranch: branch})
				res.Hooks = runs
				if failure != nil {
					return res, fmt.Errorf("worktree.release: profile %q: %w. %s is unchanged and still leased; fix the hook or the profile (config.tune), then release again", marker.Profile, failure, target)
				}
			}
		} else {
			res.Warnings = append(res.Warnings, fmt.Sprintf("profile %q no longer resolves; released with plain hygiene (nothing preserved)", marker.Profile))
		}
	}
	if err := releaseHygiene(ctx, runner, target, preserve); err != nil {
		return res, err
	}
	res.Detached = true
	if marker != nil {
		// What hygiene kept is exactly the preserved artifacts; record their
		// size for the stale-pool nudge, which never walks the tree itself.
		var size int64
		if len(preserve) > 0 {
			if size, err = preservedIgnoredBytes(ctx, runner, target); err != nil {
				res.Warnings = append(res.Warnings, "could not measure preserved artifacts: "+err.Error())
			}
		}
		marker.PreservedBytes = size
		marker.PreservedMeasuredAt = time.Now().UTC().Format(time.RFC3339)
		if err := writeWorktreeMarker(ctx, runner, target, *marker); err != nil {
			res.Warnings = append(res.Warnings, "could not record the preserved size in the profile marker: "+err.Error())
		}
	}
	// Drop the lease only once the worktree is actually back in the pool, so a
	// failed reset/clean/detach leaves the holder still identifiable.
	if err := removeWorktreeLease(ctx, runner, target); err != nil {
		return res, fmt.Errorf("worktree.release: %q is detached, but its worktree lease could not be removed: %w", target, err)
	}
	return res, nil
}
