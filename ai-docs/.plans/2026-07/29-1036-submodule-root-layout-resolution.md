# Plan: submodule-root-layout-resolution

## Relevant Ticket Contract

- Problem: `commonRootFromGitDir` requires the basename of `--git-common-dir` to
  be `.git`; a submodule's common dir is `<parent>/.git/modules/<sub-path>`
  (or `<parent>/.git/worktrees/<wt>/modules/<sub-path>` when the superproject
  is itself a git worktree), so `gitIdentity` fails today with
  `unsupported git common dir %q: expected a non-bare .git directory`.
- Accepted approach: (1) detect a submodule root with
  `git rev-parse --show-superproject-working-tree` — non-empty means the root
  is a submodule working tree. (2) For a detected submodule, set `commonRoot`
  to the submodule's own worktree root so `root == commonRoot` and it resolves
  as an independent single-worktree project with its own `projectKey`. No
  parent/child federation, no shared identity with the superproject.
- Constraints: non-submodule behavior (ordinary repos, ordinary worktrees)
  must keep today's exact `projectKey`/`worktreeKey` and not invalidate
  existing cache dirs. Keep the error path fail-loud for genuinely unsupported
  shapes — do not broaden the guard to "accept anything." Change stays inside
  `internal/wsstate`. No MCP schema change, no new public symbol, no
  type/schema contract change.
- Non-goals: git worktrees created inside a submodule (must stay fail-loud);
  cross-root federation/reference syntax; `workflow_manual`
  submodule announcement; anything touching the parent superproject's gitlink
  or commit flow.
- Accepted side effect (not a defect): a submodule under a parent git
  worktree gets a distinct `projectKey` per parent worktree, because
  `.git/worktrees/<wt>/modules/<path>` is a distinct git-dir instance per
  worktree. Cache is not shared across parent worktrees for that submodule.
- Verification boundary: new `internal/wsstate` tests with real git fixtures —
  (a) a superproject with a submodule, (b) a superproject git worktree with
  that submodule initialized. Assert `Resolve` succeeds for both submodule
  roots, `root == commonRoot` (`WorktreeKey` has no `@`), and the two
  submodule checkouts get distinct `ProjectKey`s. Regression: ordinary repo
  and ordinary worktree keep today's `projectKey`/`worktreeKey@worktreeID`
  relationship. Run `go build ./...`, `go vet ./...`, `go test ./...` from
  `agents-plugin-tool` and read full output. Local-path submodule
  add/update needs `git -c protocol.file.allow=always` on modern git.

## Out of Scope

- Worktrees nested inside a submodule — must remain fail-loud, no handling.
- `<submodule>#<stem>` cross-root ticket/spec/mental-model reference syntax.
- `workflow_manual` submodule detection/announcement (separate ticket).
- Superproject gitlink/commit flow changes.
- Version bump / commit flow — this plan covers code + tests only.

## Codebase Findings

- `agents-plugin-tool/internal/wsstate/paths.go#L230-L252` — `gitIdentity`
  runs `git rev-parse --show-toplevel` for `root`, then
  `git rev-parse --path-format=absolute --git-common-dir` for `commonGitDir`,
  then `commonRootFromGitDir(commonGitDir)`. This is the sole call site to
  change; `gitIdentity` is only invoked once, from `Manager.Resolve`
  (`paths.go#L99`), so the fix is fully self-contained in this file.
- `agents-plugin-tool/internal/wsstate/paths.go#L254-L263` —
  `commonRootFromGitDir` does the `filepath.Base(gitDir) != ".git"` guard that
  fails for submodule common dirs. Leave this function and its guard
  unmodified (keeps the fail-loud contract for genuinely unsupported shapes,
  e.g. worktree-inside-submodule); branch around it in `gitIdentity` instead
  of loosening it.
- `agents-plugin-tool/internal/wsstate/paths.go#L98-L140` — `Manager.Resolve`:
  `rootID := shortHash(commonRoot)`, `worktreeID := shortHash(root)`,
  `projectKey := rootID`, `worktreeKey := projectKey` unless `root !=
  commonRoot` (then `projectKey + "@" + worktreeID`). Once `gitIdentity`
  returns `root == commonRoot` for a submodule, this logic already produces
  the desired independent single-worktree `projectKey`/`worktreeKey` with no
  further changes needed here.
- `agents-plugin-tool/internal/wsstate/paths.go#L279-L291` — `canonicalPath`
  (Abs + EvalSymlinks + Clean) is the existing normalization helper; reuse it
  for the new `root` result rather than adding new path-cleaning logic.
- Grep confirms `gitIdentity`/`commonRootFromGitDir` have exactly one
  caller each and no other file in the module references
  `--git-common-dir`/`--show-toplevel`/`--show-superproject-working-tree`
  parsing (`internal/mcp/server.go:3106` uses `--show-toplevel` alone for an
  unrelated purpose and is untouched). No other blast-radius file
  (`playbook_tools.go`, `server.go:1446`, `execjob.go`, `wsagent/agent.go`,
  `orchestrator_lock.go`, `wsstore/store.go`) needs changes — they all consume
  `Manager.Resolve`/`Ensure` output and are fixed transitively once
  `gitIdentity` resolves correctly.
- `agents-plugin-tool/internal/wsstate/orchestrator_lock.go#L29-L40` —
  `AcquireOrchestratorLock` calls `m.Ensure(repoPath)` and only reads the
  returned `Layout`/`WorktreeMetadata`; confirms no wsstate-package coupling
  beyond `Resolve`/`Ensure` needs touching.
- `agents-plugin-tool/internal/wsstate/paths_test.go#L300-L326` — `initRepo(t)`
  and `runGit(t, dir, args...)` are the existing fixture helpers (plain
  `git init` + commit, `t.Helper()`, `t.Fatalf` on error). New submodule
  fixtures should follow this exact pattern: build a superproject repo via
  `initRepo`-style init, a second repo to add as submodule, then
  `runGit(t, super, "-c", "protocol.file.allow=always", "submodule", "add",
  "-q", subpath, "sub")` followed by a commit (the submodule addition itself
  must be committed before `git worktree add` on the superproject, otherwise
  the worktree's checkout of `HEAD` won't contain `.gitmodules`/the submodule
  gitlink — confirmed empirically during survey).
- `agents-plugin-tool/internal/wsstate/paths_test.go#L185-L241`
  (`TestLinkedWorktreeSharesProjectIdentityAndSeparatesWorktreeState`) and
  `#L17-L77` (`TestEnsureCreatesStableProjectAndWorktreeLayout`) are the
  existing non-submodule regression tests; they already assert the exact
  `projectKey`/`worktreeKey@worktreeID` relationship the contract requires to
  stay unchanged. No new regression test is needed — running the full suite
  is sufficient to confirm they still pass, since the new code path is only
  reached when `--show-superproject-working-tree` is non-empty.
- Empirically verified during survey (git 2.43.0, local fixtures, matches the
  contract's claims exactly):
  - Ordinary repo / ordinary worktree: `git rev-parse
    --show-superproject-working-tree` prints nothing (exit 0).
  - Submodule under a normal superproject checkout: `--show-toplevel` →
    `<parent>/sub`; `--git-common-dir` → `<parent>/.git/modules/sub`;
    `--show-superproject-working-tree` → `<parent>` (no trailing slash).
  - Submodule under a superproject that is itself a git worktree:
    `--show-toplevel` → `<parent>-wt/sub`; `--git-common-dir` →
    `<parent>/.git/worktrees/<wt>/modules/sub`;
    `--show-superproject-working-tree` → `<parent>-wt` (the worktree path,
    not the main checkout — this is what gives the two submodule checkouts
    distinct `projectKey`s per the accepted side effect).

## Implementation Plan

1. In `agents-plugin-tool/internal/wsstate/paths.go`, edit `gitIdentity`
   (`#L230-L252`): after computing and canonicalizing `root` via
   `--show-toplevel`, run `git rev-parse --show-superproject-working-tree` on
   `abs`. If the (trimmed) output is non-empty, return `root, root, nil`
   immediately — skip `--git-common-dir` and `commonRootFromGitDir` entirely
   for this path. If empty, fall through to the existing
   `--git-common-dir` + `commonRootFromGitDir` logic unchanged. Do not modify
   `commonRootFromGitDir`'s `.git`-basename guard — it stays the fail-loud
   path for worktree-inside-submodule and other unsupported shapes.
2. No changes needed to `Manager.Resolve`, `layoutFor`, `Manager.Ensure`, or
   any other `wsstate` file — `root == commonRoot` from step 1 already drives
   the correct `projectKey == worktreeKey` (no `@` suffix) through existing
   logic at `paths.go#L108-L114`.
3. Add new tests to `agents-plugin-tool/internal/wsstate/paths_test.go`
   following the `initRepo`/`runGit` fixture pattern:
   - A fixture helper that builds a superproject + a separate repo added as
     `sub` via `git -c protocol.file.allow=always submodule add -q <path>
     sub`, then commits the addition in the superproject (required so a
     subsequent `git worktree add` on the superproject checks out a tree that
     already contains `.gitmodules`/the gitlink).
   - Test A (main-worktree submodule): `Resolve`/`Ensure` on the submodule
     path succeeds, `project.RootPath == worktree.WorktreePath == root`
     (canonicalized), and `worktree.WorktreeKey == project.ProjectKey` (no
     `@`).
   - Test B (submodule under a superproject git worktree): `git worktree add`
     a second checkout of the superproject, run
     `git -c protocol.file.allow=always submodule update --init -q` inside
     it, then `Resolve`/`Ensure` on that submodule path too. Assert success
     and the same `root == commonRoot` / no-`@` shape.
   - Assert the two submodule checkouts (Test A's and Test B's) receive
     distinct `ProjectKey`s (per the accepted side effect).
   - Do not delete or weaken the existing non-submodule tests; they already
     cover the regression boundary.

## Verification Plan

- `go build ./...` from `agents-plugin-tool`.
- `go vet ./...` from `agents-plugin-tool`.
- `go test ./...` from `agents-plugin-tool`; read full output, confirm the
  new submodule tests pass and `TestEnsureCreatesStableProjectAndWorktreeLayout`
  / `TestLinkedWorktreeSharesProjectIdentityAndSeparatesWorktreeState` (and
  the rest of the existing `wsstate` suite) still pass unchanged.

## Escalations

- None.
