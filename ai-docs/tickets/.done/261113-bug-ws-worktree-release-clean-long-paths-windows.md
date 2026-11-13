---
title: "worktree.release clean fails on Windows long paths and leaves the worktree leased"
completed: 2026-11-13
---

# worktree.release clean fails on Windows long paths and leaves the worktree leased

## Background

Dogfood surprise, 2026-11-13: `worktree.release(path: .../guiding-dragonish-consumer)`
failed with `git -c submodule.recurse=false clean -ffdx: exit status 1: warning:
failed to remove agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/node_modules/@aws-sdk/core/dist-types/ts3.4/submodules/client/middleware-recursion-detection/...browser.d.ts:
Filename too long` (git hints `core.longPaths`).

Result: the worktree stays leased with its impl branch checked out, which
blocks `git.merge` of that branch.

Call site: `worktreeHygiene` in
agents-plugin-tool/internal/mcp/worktree_tools.go:526-536 (`reset --hard` at
:527, `clean -ffdx` at :530/:534), called from `releaseHygiene` (:540).

## Idea

- Run the release's clean (and reset) with `-c core.longPaths=true` on Windows
  (or always).
- Consider retrying the clean once on failure.
- Compatibility: T0 (internal git invocation).

## Result

Hygiene reset and clean now enable Git long paths on all platforms. Acquire and release share this hygiene helper; a unit test checks both command lines.
