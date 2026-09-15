---
title: "Default worktree pool out-of-tree: add $(GitRootDirName) primitive, relocate default to $(GitRoot)/../.ws-worktrees/$(GitRootDirName)"
related:
  260915-bug-ws-tickets-close-operates-on-server-cwd-not-worktree: same worktree-run dogfood session; both concern the worktree/parallel path
---

# Default worktree pool out-of-tree

## Background

The `worktree_pool` default is `$(GitRoot)/.ws-worktrees` — the pool sits
**inside** the repo working tree. During a parallel `ws:lead-run` (2026-09-15)
this produced real IDE clutter: editors auto-detect the nested worktrees and
index/scan them. `.ws-worktrees` is not even in the root `.gitignore`, so the
directory is plainly visible in the workspace.

`worktree_pool` is already a project-scoped, user-overridable knob, and the
default is a use-time-resolved template (not persisted), so only the builtin
default and one new template token need to change; existing explicit overrides
are unaffected and unset projects pick up the new default automatically.

## Decisions (proposed — confirm at promotion)

- **New template primitive `$(GitRootDirName)` = `filepath.Base(GitRoot)`.**
  The substitution site currently supports only `$(GitRoot)`.
- **New default: `$(GitRoot)/../.ws-worktrees/$(GitRootDirName)`**, then
  `filepath.Clean`'d so the resolved/reported path is not
  `.../repo/../.ws-worktrees/repo`.
- **Sibling (`../`), not `~/.cache`,** to keep worktrees on the **same
  filesystem** as the repo (efficient `.git` object sharing / hardlinks) while
  leaving the IDE workspace root.
- **Namespacing is collision-free by construction:** a basename is unique
  within its parent, so sibling repos sharing one `.ws-worktrees` parent map to
  distinct `<basename>/` subdirs.

## Constraints

- **MUST fall back** to in-tree `$(GitRoot)/.ws-worktrees` (current behavior)
  with a one-time advisory when the `$(GitRoot)/..` parent is not
  creatable/writable (mount root, read-only parent, container `/workspace`),
  so no repo becomes un-provisionable.
- Update **both** default-definition sites or dedupe them:
  `agents-plugin-tool/internal/mcp/server.go` (config builtin default) and
  `agents-plugin-tool/internal/mcp/worktree_tools.go` (`resolvePoolRoot`
  fallback).
- Explicit `worktree_pool` overrides still win.
- `filepath.Clean` the resolved pool root.
- `ai-docs/manuals/ws-mcp.md` applies (paths under
  `agents-plugin-tool/internal/mcp/`): read before editing.
- Legacy in-tree `.ws-worktrees/` pools become harmless orphans after the
  default changes — document manual cleanup; consider a discoverability/prune
  hook so out-of-tree pools do not accumulate unnoticed.

## Open questions

- Should `worktree.release`/prune diagnostics surface the (now less visible)
  out-of-tree pool location so orphans stay discoverable?
- Add `.ws-worktrees/` to `.gitignore` regardless, as defense in depth for any
  project that still resolves the pool in-tree?

## Verification (implementation-time)

- Unit: `$(GitRootDirName)` resolution; new default path; unwritable-parent
  fallback + advisory; `filepath.Clean` output; explicit-override precedence.
- Existing `worktree.acquire` / `worktree.release` suite stays green.
